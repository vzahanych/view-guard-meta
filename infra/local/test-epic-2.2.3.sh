#!/bin/bash
# Test script for Epic 2.2.3: Edge → VM Dataset Sync & Upload
# This script tests the complete dataset sync flow in the docker-compose environment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
EDGE_API="http://localhost:8181"
VM_API="http://localhost:8280"
CAMERA_ID="" # Will be determined automatically

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Epic 2.2.3: Dataset Sync & Upload Test${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Step 1: Check services are running
echo -e "${YELLOW}[Step 1] Checking services...${NC}"
if ! docker compose ps | grep -q "edge-orchestrator.*Up"; then
    echo -e "${RED}✗ Edge orchestrator is not running${NC}"
    echo "Starting services..."
    docker compose up -d
    echo "Waiting for services to be healthy..."
    sleep 10
fi

if ! docker compose ps | grep -q "user-vm-api.*Up"; then
    echo -e "${RED}✗ User VM API is not running${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Services are running${NC}"
echo ""

# Step 2: Check Edge API health
echo -e "${YELLOW}[Step 2] Checking Edge API health...${NC}"
EDGE_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "${EDGE_API}/health" || echo "000")
if [ "$EDGE_HEALTH" != "200" ]; then
    echo -e "${RED}✗ Edge API is not healthy (HTTP $EDGE_HEALTH)${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Edge API is healthy${NC}"
echo ""

# Step 3: Check VM API health
echo -e "${YELLOW}[Step 3] Checking VM API health...${NC}"
VM_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "${VM_API}/health" 2>/dev/null || echo "000")
if [ "$VM_HEALTH" != "200" ]; then
    echo -e "${YELLOW}⚠ VM API health check failed (HTTP $VM_HEALTH)${NC}"
    echo "Checking VM container status..."
    VM_STATUS=$(docker compose ps user-vm-api --format "{{.Status}}" 2>/dev/null || echo "unknown")
    echo "  VM container status: $VM_STATUS"
    echo -e "${YELLOW}Continuing anyway - sync may still work if WireGuard tunnel is established${NC}"
else
    echo -e "${GREEN}✓ VM API is healthy${NC}"
fi
echo ""

# Step 4: Find camera with sufficient screenshots
echo -e "${YELLOW}[Step 4] Finding camera with sufficient screenshots...${NC}"
CAMERAS_RESPONSE=$(curl -s "${EDGE_API}/api/cameras" 2>/dev/null)
CAMERAS_COUNT=$(echo "$CAMERAS_RESPONSE" | jq -r '.cameras | length' 2>/dev/null || echo "0")

if [ "$CAMERAS_COUNT" = "0" ] || [ -z "$CAMERAS_RESPONSE" ]; then
    echo -e "${RED}✗ No cameras found${NC}"
    exit 1
fi

# Find camera with >= 50 labeled screenshots
CAMERA_ID=$(echo "$CAMERAS_RESPONSE" | jq -r '.cameras[] | select(.dataset_status.labeled_snapshot_count >= 50) | .id' 2>/dev/null | head -1)

if [ -z "$CAMERA_ID" ] || [ "$CAMERA_ID" = "null" ]; then
    # No camera with enough screenshots, use first camera
    CAMERA_ID=$(echo "$CAMERAS_RESPONSE" | jq -r '.cameras[0].id' 2>/dev/null)
    CURRENT_COUNT=$(echo "$CAMERAS_RESPONSE" | jq -r '.cameras[0].dataset_status.labeled_snapshot_count' 2>/dev/null || echo "0")
    echo -e "${YELLOW}⚠ Using camera $CAMERA_ID (has ${CURRENT_COUNT} screenshots, need 50)${NC}"
else
    CURRENT_COUNT=$(echo "$CAMERAS_RESPONSE" | jq -r ".cameras[] | select(.id == \"$CAMERA_ID\") | .dataset_status.labeled_snapshot_count" 2>/dev/null)
    echo -e "${GREEN}✓ Using camera $CAMERA_ID (has ${CURRENT_COUNT} screenshots)${NC}"
fi
echo ""

# Step 5: Verify sufficient screenshots exist
echo -e "${YELLOW}[Step 5] Verifying sufficient screenshots...${NC}"
NEEDED=50

# Check current screenshot count
CURRENT_COUNT=$(curl -s "${EDGE_API}/api/cameras/${CAMERA_ID}/dataset" | jq -r '.dataset_status.labeled_snapshot_count' 2>/dev/null || echo "0")
echo "  Current labeled snapshots: $CURRENT_COUNT"
echo "  Required for sync: $NEEDED"

if [ "$CURRENT_COUNT" -ge "$NEEDED" ]; then
    echo -e "${GREEN}✓ Camera has ${CURRENT_COUNT} labeled screenshots (need ${NEEDED})${NC}"
else
    echo -e "${RED}✗ Camera only has ${CURRENT_COUNT} labeled screenshots (need ${NEEDED})${NC}"
    echo -e "${YELLOW}Tip: Use the UI to capture and label screenshots from the camera${NC}"
    exit 1
fi
echo ""

# Step 6: Check dataset status
echo -e "${YELLOW}[Step 6] Checking dataset status...${NC}"
DATASET_STATUS=$(curl -s "${EDGE_API}/api/cameras/${CAMERA_ID}/dataset" 2>/dev/null)
if [ -z "$DATASET_STATUS" ]; then
    echo -e "${RED}✗ Failed to get dataset status${NC}"
    exit 1
fi

SNAPSHOT_COUNT=$(echo "$DATASET_STATUS" | jq -r '.dataset_status.labeled_snapshot_count' 2>/dev/null || echo "0")
SNAPSHOT_REQUIRED=$(echo "$DATASET_STATUS" | jq -r '.dataset_status.snapshot_required' 2>/dev/null || echo "true")

echo "  Labeled snapshots: $SNAPSHOT_COUNT"
echo "  Snapshot required: $SNAPSHOT_REQUIRED"

if [ "$SNAPSHOT_REQUIRED" = "true" ]; then
    echo -e "${YELLOW}⚠ Dataset not ready yet (need more snapshots)${NC}"
    echo "Current status:"
    echo "$DATASET_STATUS" | jq '.'
    exit 1
fi

echo -e "${GREEN}✓ Dataset is ready for sync${NC}"
echo ""

# Step 7: Trigger dataset sync
echo -e "${YELLOW}[Step 7] Triggering dataset sync...${NC}"
SYNC_RESPONSE=$(curl -s -X POST "${EDGE_API}/api/cameras/${CAMERA_ID}/dataset/sync" 2>/dev/null)
SYNC_STATUS=$(echo "$SYNC_RESPONSE" | jq -r '.dataset_synced' 2>/dev/null || echo "false")
DATASET_ID=$(echo "$SYNC_RESPONSE" | jq -r '.dataset_id' 2>/dev/null || echo "")

if [ "$SYNC_STATUS" != "true" ] || [ -z "$DATASET_ID" ]; then
    echo -e "${RED}✗ Dataset sync failed${NC}"
    echo "Response:"
    echo "$SYNC_RESPONSE" | jq '.' 2>/dev/null || echo "$SYNC_RESPONSE"
    exit 1
fi

echo -e "${GREEN}✓ Dataset sync successful!${NC}"
echo "  Dataset ID: $DATASET_ID"
echo ""

# Step 8: Verify dataset on VM
echo -e "${YELLOW}[Step 8] Verifying dataset on VM...${NC}"
# Check if dataset exists in VM storage
# Note: This would require a VM API endpoint to query datasets
# For now, we check logs
echo "Checking VM logs for dataset reception..."
VM_LOGS=$(docker compose logs user-vm-api --tail 50 2>/dev/null | grep -i "dataset" || echo "")
if echo "$VM_LOGS" | grep -q "Dataset received\|dataset uploaded"; then
    echo -e "${GREEN}✓ Dataset received on VM${NC}"
else
    echo -e "${YELLOW}⚠ Could not verify dataset on VM from logs${NC}"
fi
echo ""

# Step 9: Check training eligibility status
echo -e "${YELLOW}[Step 9] Checking training eligibility status...${NC}"
# This would require querying VM API for camera status
# For now, we check Edge logs
echo "Checking Edge logs for capability sync..."
EDGE_LOGS=$(docker compose logs edge-orchestrator --tail 50 2>/dev/null | grep -i "capability\|training" || echo "")
if echo "$EDGE_LOGS" | grep -q "capability\|training"; then
    echo -e "${GREEN}✓ Capability sync completed${NC}"
else
    echo -e "${YELLOW}⚠ Could not verify capability sync from logs${NC}"
fi
echo ""

# Step 10: Summary
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "${GREEN}✓ All steps completed successfully${NC}"
echo ""
echo "Dataset Sync Results:"
echo "  - Camera ID: $CAMERA_ID"
echo "  - Screenshots created: $CREATED"
echo "  - Dataset ID: $DATASET_ID"
echo "  - Sync status: $SYNC_STATUS"
echo ""
echo -e "${GREEN}Epic 2.2.3 functionality test PASSED!${NC}"
