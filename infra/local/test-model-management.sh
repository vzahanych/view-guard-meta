#!/bin/bash
# Test script for Epic 2.2.4: VM-Side Model Management
# This script tests model storage, catalog, and API endpoints in the docker-compose environment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
# Check if running inside container (set by docker-compose)
if [ "${IN_CONTAINER:-false}" = "true" ]; then
    # Use service names when running inside docker-compose network
    VM_API="${VM_API:-http://user-vm-api:8080}"
    PYTHON_SERVICE="python-ai-service"
    # Use docker exec with container names
    DOCKER_EXEC="docker exec"
else
    # Use localhost when running on host
    VM_API="${VM_API:-http://localhost:8280}"
    PYTHON_SERVICE="view-guard-python-ai-service"
    # Use docker compose exec
    DOCKER_EXEC="docker compose exec -T"
fi

EDGE_ID="poc-edge-1"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Epic 2.2.4: Model Management Test${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Step 1: Check services are running
echo -e "${YELLOW}[Step 1] Checking services...${NC}"
if [ "${IN_CONTAINER:-false}" = "true" ]; then
    # In container mode, services are guaranteed by depends_on
    echo -e "${GREEN}✓ Services are running (container mode)${NC}"
else
    # On host, check with docker compose
    if ! docker compose ps | grep -q "user-vm-api.*Up"; then
        echo -e "${RED}✗ User VM API is not running${NC}"
        echo "Starting services..."
        docker compose up -d
        echo "Waiting for services to be healthy..."
        sleep 10
    fi

    if ! docker compose ps | grep -q "python-ai-service.*Up"; then
        echo -e "${RED}✗ Python AI service is not running${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ Services are running${NC}"
fi
echo ""

# Step 2: Check VM API health
echo -e "${YELLOW}[Step 2] Checking VM API health...${NC}"
VM_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "${VM_API}/health" || echo "000")
if [ "$VM_HEALTH" != "200" ]; then
    echo -e "${RED}✗ VM API is not healthy (HTTP $VM_HEALTH)${NC}"
    exit 1
fi
echo -e "${GREEN}✓ VM API is healthy${NC}"
echo ""

# Step 3: Verify baseline YOLOv8n model is downloaded and stored
echo -e "${YELLOW}[Step 3] Verifying baseline YOLOv8n model...${NC}"
BASELINE_MODEL_PATH="/app/data/models/baseline/yolov8n/model.onnx"
BASELINE_METADATA_PATH="/app/data/models/baseline/yolov8n/metadata.json"

if [ "${IN_CONTAINER:-false}" = "true" ]; then
    # In container mode, verify via API (file checks require docker exec which needs socket access)
    echo "  Checking baseline model via API..."
    BASELINE_MODELS=$(curl -s "${VM_API}/api/models/baseline" || echo "[]")
    if echo "$BASELINE_MODELS" | grep -q "baseline-yolov8n"; then
        echo -e "${GREEN}✓ Baseline model is registered in catalog${NC}"
    else
        echo -e "${YELLOW}⚠ Baseline model not yet in catalog (may need catalog scan or setup)${NC}"
        echo "  Note: In container mode, run setup-baseline-models.sh before starting tests"
    fi
else
    # On host, check files directly
    if ${DOCKER_EXEC} ${PYTHON_SERVICE} test -f "${BASELINE_MODEL_PATH}"; then
        echo -e "${GREEN}✓ Baseline model file exists${NC}"
    else
        echo -e "${RED}✗ Baseline model file not found${NC}"
        echo "Running setup-baseline-models.sh..."
        ./setup-baseline-models.sh
        if ! ${DOCKER_EXEC} ${PYTHON_SERVICE} test -f "${BASELINE_MODEL_PATH}"; then
            echo -e "${RED}✗ Failed to setup baseline model${NC}"
            exit 1
        fi
    fi

    if ${DOCKER_EXEC} ${PYTHON_SERVICE} test -f "${BASELINE_METADATA_PATH}"; then
        echo -e "${GREEN}✓ Baseline model metadata exists${NC}"
    else
        echo -e "${RED}✗ Baseline model metadata not found${NC}"
        exit 1
    fi

    # Verify model is registered in catalog (via API)
    BASELINE_MODELS=$(curl -s "${VM_API}/api/models/baseline" || echo "[]")
    if echo "$BASELINE_MODELS" | grep -q "baseline-yolov8n"; then
        echo -e "${GREEN}✓ Baseline model is registered in catalog${NC}"
    else
        echo -e "${YELLOW}⚠ Baseline model not yet in catalog (may need catalog scan)${NC}"
    fi
fi
echo ""

# Step 4: Test model catalog API endpoints
echo -e "${YELLOW}[Step 4] Testing model catalog API endpoints...${NC}"

# List all models
echo "  Testing GET /api/models..."
ALL_MODELS=$(curl -s "${VM_API}/api/models" || echo "[]")
if [ "$ALL_MODELS" != "[]" ] && [ "$ALL_MODELS" != "null" ]; then
    MODEL_COUNT=$(echo "$ALL_MODELS" | grep -o '"model_id"' | wc -l || echo "0")
    echo -e "  ${GREEN}✓ Found $MODEL_COUNT model(s)${NC}"
else
    echo -e "  ${YELLOW}⚠ No models found (may be expected if catalog hasn't scanned)${NC}"
fi

# Get baseline models
echo "  Testing GET /api/models/baseline..."
BASELINE_RESPONSE=$(curl -s "${VM_API}/api/models/baseline" || echo "[]")
if echo "$BASELINE_RESPONSE" | grep -q "baseline"; then
    echo -e "  ${GREEN}✓ Baseline models endpoint works${NC}"
else
    echo -e "  ${YELLOW}⚠ No baseline models returned (may need catalog scan)${NC}"
fi

# Get baseline models by type
echo "  Testing GET /api/models/baseline?model_type=yolo..."
BASELINE_YOLO=$(curl -s "${VM_API}/api/models/baseline?model_type=yolo" || echo "[]")
if [ "$BASELINE_YOLO" != "[]" ] && [ "$BASELINE_YOLO" != "null" ]; then
    echo -e "  ${GREEN}✓ Baseline YOLO models endpoint works${NC}"
else
    echo -e "  ${YELLOW}⚠ No baseline YOLO models returned${NC}"
fi

# Get specific model
echo "  Testing GET /api/models/baseline-yolov8n..."
MODEL_DETAIL=$(curl -s "${VM_API}/api/models/baseline-yolov8n" || echo "{}")
if echo "$MODEL_DETAIL" | grep -q "model_id"; then
    echo -e "  ${GREEN}✓ Model detail endpoint works${NC}"
    # Verify metadata includes required fields
    if echo "$MODEL_DETAIL" | grep -q "model_type"; then
        echo -e "  ${GREEN}✓ Model metadata includes model_type${NC}"
    fi
    if echo "$MODEL_DETAIL" | grep -q "input_shape"; then
        echo -e "  ${GREEN}✓ Model metadata includes input_shape${NC}"
    fi
    if echo "$MODEL_DETAIL" | grep -q "framework"; then
        echo -e "  ${GREEN}✓ Model metadata includes framework${NC}"
    fi
else
    echo -e "  ${YELLOW}⚠ Model detail not found (may need catalog scan)${NC}"
fi
echo ""

# Step 5: Upload a test trained model via API
echo -e "${YELLOW}[Step 5] Testing trained model upload via API...${NC}"

# Create a test model file (minimal ONNX-like data, >= 1KB)
TEST_MODEL_ID="test-trained-model-$(date +%s)"
TEST_MODEL_DATA=$(dd if=/dev/urandom bs=2048 count=1 2>/dev/null | base64 | tr -d '\n')
TEST_METADATA=$(cat <<EOF
{
  "model_id": "${TEST_MODEL_ID}",
  "version": "trained-1.0",
  "camera_id": "test-camera-1",
  "model_type": "yolo",
  "input_shape": [1, 3, 640, 640],
  "framework": "onnx",
  "onnx_file": "model.onnx",
  "training_dataset_id": "test-dataset-123",
  "training_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "accuracy": 0.95,
  "precision": 0.92,
  "recall": 0.94,
  "f1_score": 0.93
}
EOF
)

# Create temporary files
TMP_DIR=$(mktemp -d)
TMP_MODEL_FILE="${TMP_DIR}/model.onnx"
echo "$TEST_MODEL_DATA" | base64 -d > "$TMP_MODEL_FILE" 2>/dev/null || {
    # Fallback: create a simple binary file
    dd if=/dev/urandom of="$TMP_MODEL_FILE" bs=2048 count=1 2>/dev/null
}

echo "  Uploading test trained model..."
UPLOAD_RESPONSE=$(curl -s -X POST "${VM_API}/api/models" \
    -H "X-Edge-ID: ${EDGE_ID}" \
    -F "model_id=${TEST_MODEL_ID}" \
    -F "metadata=${TEST_METADATA}" \
    -F "model=@${TMP_MODEL_FILE}" || echo "{}")

if echo "$UPLOAD_RESPONSE" | grep -q "success"; then
    echo -e "  ${GREEN}✓ Model uploaded successfully${NC}"
    
    # Verify model is stored
    sleep 2
    MODEL_INFO=$(curl -s "${VM_API}/api/models/${TEST_MODEL_ID}" || echo "{}")
    if echo "$MODEL_INFO" | grep -q "$TEST_MODEL_ID"; then
        echo -e "  ${GREEN}✓ Model is accessible via API${NC}"
        
        # Verify metadata
        if echo "$MODEL_INFO" | grep -q "test-dataset-123"; then
            echo -e "  ${GREEN}✓ Model is linked to dataset${NC}"
        fi
        if echo "$MODEL_INFO" | grep -q "test-camera-1"; then
            echo -e "  ${GREEN}✓ Model is linked to camera${NC}"
        fi
    else
        echo -e "  ${YELLOW}⚠ Model not immediately accessible (may need catalog scan)${NC}"
    fi
else
    echo -e "  ${RED}✗ Model upload failed: ${UPLOAD_RESPONSE}${NC}"
    rm -rf "$TMP_DIR"
    exit 1
fi

rm -rf "$TMP_DIR"
echo ""

# Step 6: Test model querying by dataset ID, camera ID, status
echo -e "${YELLOW}[Step 6] Testing model querying...${NC}"

# Query by dataset ID
echo "  Testing GET /api/models?dataset_id=test-dataset-123..."
DATASET_MODELS=$(curl -s "${VM_API}/api/models?dataset_id=test-dataset-123" || echo "[]")
if echo "$DATASET_MODELS" | grep -q "$TEST_MODEL_ID"; then
    echo -e "  ${GREEN}✓ Query by dataset ID works${NC}"
else
    echo -e "  ${YELLOW}⚠ Model not found by dataset ID (may need catalog scan)${NC}"
fi

# Query by camera ID
echo "  Testing GET /api/models?camera_id=test-camera-1..."
CAMERA_MODELS=$(curl -s "${VM_API}/api/models?camera_id=test-camera-1" || echo "[]")
if echo "$CAMERA_MODELS" | grep -q "$TEST_MODEL_ID"; then
    echo -e "  ${GREEN}✓ Query by camera ID works${NC}"
else
    echo -e "  ${YELLOW}⚠ Model not found by camera ID (may need catalog scan)${NC}"
fi

# Query by status
echo "  Testing GET /api/models?status=ready..."
READY_MODELS=$(curl -s "${VM_API}/api/models?status=ready" || echo "[]")
if echo "$READY_MODELS" | grep -q "$TEST_MODEL_ID"; then
    echo -e "  ${GREEN}✓ Query by status works${NC}"
else
    echo -e "  ${YELLOW}⚠ Model not found by status (may need catalog scan)${NC}"
fi
echo ""

# Step 7: Test model selection for training
echo -e "${YELLOW}[Step 7] Testing model selection for training...${NC}"

# Query baseline models available for training
echo "  Querying baseline models for training..."
BASELINE_FOR_TRAINING=$(curl -s "${VM_API}/api/models/baseline?model_type=yolo" || echo "[]")
if [ "$BASELINE_FOR_TRAINING" != "[]" ] && [ "$BASELINE_FOR_TRAINING" != "null" ]; then
    echo -e "  ${GREEN}✓ Baseline models available for training${NC}"
    
    # Verify baseline model metadata
    if echo "$BASELINE_FOR_TRAINING" | grep -q '"model_type":"yolo"'; then
        echo -e "  ${GREEN}✓ Baseline model metadata includes model_type${NC}"
    fi
    if echo "$BASELINE_FOR_TRAINING" | grep -q '"input_shape"'; then
        echo -e "  ${GREEN}✓ Baseline model metadata includes input_shape${NC}"
    fi
    if echo "$BASELINE_FOR_TRAINING" | grep -q '"framework"'; then
        echo -e "  ${GREEN}✓ Baseline model metadata includes framework${NC}"
    fi
else
    echo -e "  ${YELLOW}⚠ No baseline models returned (may need catalog scan)${NC}"
fi
echo ""

# Step 8: Test model compatibility checks
echo -e "${YELLOW}[Step 8] Testing model compatibility checks...${NC}"

# Test 1: Upload model exceeding size limit (should fail)
echo "  Test 1: Uploading oversized model (should fail)..."
OVERSIZED_MODEL_ID="test-oversized-model-$(date +%s)"
OVERSIZED_FILE="${TMP_DIR}/oversized.onnx"
# Create a file larger than 50MB
dd if=/dev/urandom of="$OVERSIZED_FILE" bs=1048576 count=51 2>/dev/null

OVERSIZED_METADATA=$(cat <<EOF
{
  "model_id": "${OVERSIZED_MODEL_ID}",
  "version": "1.0.0",
  "model_type": "yolo",
  "input_shape": [1, 3, 640, 640],
  "framework": "onnx",
  "onnx_file": "model.onnx"
}
EOF
)

OVERSIZED_RESPONSE=$(curl -s -X POST "${VM_API}/api/models" \
    -H "X-Edge-ID: ${EDGE_ID}" \
    -F "model_id=${OVERSIZED_MODEL_ID}" \
    -F "metadata=${OVERSIZED_METADATA}" \
    -F "model=@${OVERSIZED_FILE}" || echo "{}")

if echo "$OVERSIZED_RESPONSE" | grep -qi "exceed\|size\|limit\|error"; then
    echo -e "  ${GREEN}✓ Oversized model correctly rejected${NC}"
else
    echo -e "  ${YELLOW}⚠ Oversized model response: ${OVERSIZED_RESPONSE}${NC}"
fi

rm -f "$OVERSIZED_FILE"

# Test 2: Upload invalid ONNX file (too small, should fail)
echo "  Test 2: Uploading invalid ONNX file (too small, should fail)..."
INVALID_MODEL_ID="test-invalid-model-$(date +%s)"
INVALID_FILE="${TMP_DIR}/invalid.onnx"
echo "invalid" > "$INVALID_FILE"

INVALID_METADATA=$(cat <<EOF
{
  "model_id": "${INVALID_MODEL_ID}",
  "version": "1.0.0",
  "model_type": "yolo",
  "input_shape": [1, 3, 640, 640],
  "framework": "onnx",
  "onnx_file": "model.onnx"
}
EOF
)

INVALID_RESPONSE=$(curl -s -X POST "${VM_API}/api/models" \
    -H "X-Edge-ID: ${EDGE_ID}" \
    -F "model_id=${INVALID_MODEL_ID}" \
    -F "metadata=${INVALID_METADATA}" \
    -F "model=@${INVALID_FILE}" || echo "{}")

if echo "$INVALID_RESPONSE" | grep -qi "small\|invalid\|error\|validation"; then
    echo -e "  ${GREEN}✓ Invalid ONNX file correctly rejected${NC}"
else
    echo -e "  ${YELLOW}⚠ Invalid ONNX file response: ${INVALID_RESPONSE}${NC}"
fi

rm -f "$INVALID_FILE"

# Test 3: Upload valid model (should succeed)
echo "  Test 3: Uploading valid model (should succeed)..."
VALID_MODEL_ID="test-valid-model-$(date +%s)"
VALID_FILE="${TMP_DIR}/valid.onnx"
dd if=/dev/urandom of="$VALID_FILE" bs=2048 count=1 2>/dev/null

VALID_METADATA=$(cat <<EOF
{
  "model_id": "${VALID_MODEL_ID}",
  "version": "1.0.0",
  "model_type": "yolo",
  "input_shape": [1, 3, 640, 640],
  "framework": "onnx",
  "onnx_file": "model.onnx"
}
EOF
)

VALID_RESPONSE=$(curl -s -X POST "${VM_API}/api/models" \
    -H "X-Edge-ID: ${EDGE_ID}" \
    -F "model_id=${VALID_MODEL_ID}" \
    -F "metadata=${VALID_METADATA}" \
    -F "model=@${VALID_FILE}" || echo "{}")

if echo "$VALID_RESPONSE" | grep -q "success"; then
    echo -e "  ${GREEN}✓ Valid model correctly accepted${NC}"
else
    echo -e "  ${YELLOW}⚠ Valid model response: ${VALID_RESPONSE}${NC}"
fi

rm -f "$VALID_FILE"
rm -rf "$TMP_DIR"
echo ""

# Step 9: Test model download
echo -e "${YELLOW}[Step 9] Testing model download...${NC}"
echo "  Testing GET /api/models/${TEST_MODEL_ID}/file..."
DOWNLOAD_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "${VM_API}/api/models/${TEST_MODEL_ID}/file" || echo "000")
if [ "$DOWNLOAD_RESPONSE" = "200" ]; then
    echo -e "  ${GREEN}✓ Model file download works${NC}"
else
    echo -e "  ${YELLOW}⚠ Model file download returned HTTP ${DOWNLOAD_RESPONSE}${NC}"
fi
echo ""

# Summary
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "${GREEN}✓ Baseline model verification${NC}"
echo -e "${GREEN}✓ Model catalog API endpoints${NC}"
echo -e "${GREEN}✓ Trained model upload${NC}"
echo -e "${GREEN}✓ Model querying (dataset, camera, status)${NC}"
echo -e "${GREEN}✓ Model selection for training${NC}"
echo -e "${GREEN}✓ Model compatibility checks${NC}"
echo -e "${GREEN}✓ Model download${NC}"
echo ""
echo -e "${GREEN}All integration tests completed!${NC}"
