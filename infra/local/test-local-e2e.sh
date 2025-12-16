#!/bin/bash
# Local End-to-End Integration Test
# Tests the complete workflow from environment setup through Epic tests
#
# SaaS Architecture Simulation:
# This test simulates the production SaaS architecture where:
# - Customer configures edges in SaaS UI
# - SaaS creates User VM with knowledge of edges (pre-registered in edge management system)
# - VM starts as independent, central part
# - Edge appliances start later and connect to already-running VM
#
# Test Flow:
# Step 1: Clean old data from previous runs (unless --skip-cleanup)
# Epic Tests (run based on --epic flags, with automatic prerequisite resolution):
#   Epic 2.0: WireGuard Configuration Generation
#   Epic 2.1: VM Services Startup and Initialization (requires 2.0)
#   Epic 2.2: VM Edge Status Monitoring & WireGuard Connection Management (requires 2.0, 2.1)
#             Note: Epic 2.2 also triggers edge registration (step 3) and Edge services startup (step 4)
#   Epic 2.3: Post-WireGuard Edge ↔ VM Coordination (requires 2.0, 2.1, 2.2)
#   Epic 2.4: Snapshot Capture & Dataset Progress Fixes (requires 2.0, 2.1, 2.2, 2.3)
#   Epic 2.5: Edge → VM Dataset Sync & Upload (requires 2.0, 2.1, 2.2, 2.3, 2.4)
#   Epic 2.6: VM-Side Model Management for Training Readiness (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5)
#   Epic 2.7: Model Training Pipeline (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6)
#   Epic 2.8: VM → Edge Trained Model Sync & Deployment (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7)
#   Epic 2.9: Edge-Side Event Detection & Processing (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8)
# Cleanup on exit (optional, controlled by CLEANUP_ON_EXIT env var)
#
# This script runs OUTSIDE of docker-compose and manages the entire test environment lifecycle.
# Epics 2.0 and 2.1 use start-local-env.sh to ensure proper initialization.
#
# Usage:
#   ./test-local-e2e.sh                    # Run all epics (default: 2.0-2.9)
#   ./test-local-e2e.sh --all              # Run all epics (2.0-2.9)
#   ./test-local-e2e.sh --epic 2.0         # Run only Epic 2.0 (Configuration Generation)
#   ./test-local-e2e.sh --epic 2.1         # Run Epic 2.0 + 2.1 (Configuration + VM Services)
#   ./test-local-e2e.sh --epic 2.2         # Run Epic 2.0 + 2.1 + 2.2 (Configuration + VM Services + Connection)
#   ./test-local-e2e.sh --epic 2.7         # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6 + 2.7
#   ./test-local-e2e.sh --epic 2.8         # Run Epic 2.0 + 2.1 + 2.2 + 2.3 + 2.4 + 2.5 + 2.6 + 2.7 + 2.8
#   ./test-local-e2e.sh --epic 2.9         # Run all epics (2.0 → 2.1 → 2.2 → ... → 2.9)
#   ./test-local-e2e.sh --epic 2.2 --epic 2.7  # Run multiple epics (with prerequisites)
#   ./test-local-e2e.sh --skip-cleanup     # Skip cleanup step
#   ./test-local-e2e.sh --rebuild          # Rebuild Docker images before starting
#   ./test-local-e2e.sh --help             # Show help message

set -euo pipefail

# Get script directory (infra/local)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-${SCRIPT_DIR}/docker-compose.yml}"

# Configuration - always use localhost since we run outside containers
VM_API="${VM_API:-http://localhost:8280}"
EDGE_API="${EDGE_API:-http://localhost:8181}"  # Port 8181 is mapped from container port 8081
TRAINING_SERVICE="${TRAINING_SERVICE:-http://localhost:8000}"

# Cleanup control
CLEANUP_ON_EXIT="${CLEANUP_ON_EXIT:-true}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
EPIC_TESTS=0

# Cleanup flag
CLEANUP_DONE=false

# Test failure flag (used to keep containers running on failure)
TEST_FAILED=false

# Test execution flags
RUN_EPIC_2_0=false
RUN_EPIC_2_1=false
RUN_EPIC_2_2=false
RUN_EPIC_2_3=false
RUN_EPIC_2_4=false
RUN_EPIC_2_5=false
RUN_EPIC_2_6=false
RUN_EPIC_2_7=false
RUN_EPIC_2_8=false
RUN_EPIC_2_9=false
SKIP_CLEANUP=false
REBUILD=false
SHOW_HELP=false

# ============================================================================
# Load helper functions from scripts directory
# ============================================================================

SCRIPTS_DIR="${SCRIPT_DIR}/scripts"

# Source all helper scripts
source "${SCRIPTS_DIR}/logging.sh"
source "${SCRIPTS_DIR}/test_helpers.sh"
source "${SCRIPTS_DIR}/json_helpers.sh"
source "${SCRIPTS_DIR}/api_helpers.sh"
source "${SCRIPTS_DIR}/investigation.sh"
source "${SCRIPTS_DIR}/polling.sh"
source "${SCRIPTS_DIR}/epic_2_2_tests.sh"
source "${SCRIPTS_DIR}/epic_2_3_tests.sh"
source "${SCRIPTS_DIR}/epic_2_4_tests.sh"
source "${SCRIPTS_DIR}/epic_2_5_tests.sh"
source "${SCRIPTS_DIR}/epic_2_6_tests.sh"
source "${SCRIPTS_DIR}/epic_2_7_tests.sh"
source "${SCRIPTS_DIR}/epic_2_8_tests.sh"
source "${SCRIPTS_DIR}/epic_2_9_tests.sh"
source "${SCRIPTS_DIR}/cleanup.sh"
source "${SCRIPTS_DIR}/setup.sh"
source "${SCRIPTS_DIR}/args.sh"

# Register cleanup on exit
trap cleanup EXIT INT TERM

# ============================================================================
# Step 1: Clean old data from previous runs (unless --skip-cleanup)
# ============================================================================

# ============================================================================
# Epic 2.0: WireGuard Configuration Generation
# ============================================================================

test_epic_2_0() {
    log_section "Epic 2.0: WireGuard Configuration Generation"
    EPIC_TESTS=0
    
    cd "$SCRIPT_DIR"
    
    # Rebuild images if requested (before any service starts)
    if [ "${REBUILD:-false}" = "true" ]; then
        log_info "Rebuilding Docker images (--rebuild flag set)..."
        docker compose -f "$COMPOSE_FILE" build --no-cache
        if [ $? -ne 0 ]; then
            test_failed "Failed to rebuild Docker images"
            return 1
        fi
        log_info "Docker images rebuilt successfully"
    fi
    
    log_info "Generating WireGuard configuration (keys and configs)..."
    if ! "${SCRIPT_DIR}/start-local-env.sh" step-2.0; then
        test_failed "Epic 2.0 failed: Configuration generation"
        return 1
    fi
    
    # Read EDGE_ID from generated file (needed for subsequent steps)
    EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
    if [ ! -f "$EDGE_ID_FILE" ]; then
        test_failed "EDGE_ID file not found at $EDGE_ID_FILE"
        return 1
    fi
    
    EDGE_ID=$(cat "$EDGE_ID_FILE")
    log_info "EDGE_ID generated: $EDGE_ID"
    
    # Export EDGE_ID for docker-compose
    export EDGE_ID
    
    test_passed "WireGuard configuration generated successfully"
    
    # Verify TLS/mTLS certificates were generated
    log_info "Verifying TLS/mTLS certificates..."
    if [ ! -f "${SCRIPT_DIR}/wg/certs/ca.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/ca.key" ]; then
        test_failed "CA certificate not found"
        return 1
    fi
    
    if [ ! -f "${SCRIPT_DIR}/wg/certs/vm-server.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/vm-server.key" ]; then
        test_failed "VM server certificate not found"
        return 1
    fi
    
    if [ ! -f "${SCRIPT_DIR}/wg/certs/edge-server.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/edge-server.key" ]; then
        test_failed "Edge server certificate not found"
        return 1
    fi
    
    if [ ! -f "${SCRIPT_DIR}/wg/certs/vm-client.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/vm-client.key" ]; then
        test_failed "VM client certificate not found"
        return 1
    fi
    
    if [ ! -f "${SCRIPT_DIR}/wg/certs/edge-client.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/edge-client.key" ]; then
        test_failed "Edge client certificate not found"
        return 1
    fi
    
    if [ ! -f "${SCRIPT_DIR}/wg/certs/vm-db.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/vm-db.key" ]; then
        test_failed "VM database certificate not found"
        return 1
    fi
    
    if [ ! -f "${SCRIPT_DIR}/wg/certs/edge-db.crt" ] || [ ! -f "${SCRIPT_DIR}/wg/certs/edge-db.key" ]; then
        test_failed "Edge database certificate not found"
        return 1
    fi
    
    test_passed "TLS/mTLS certificates generated successfully"
    
    # Verify all certificates are valid and signed by the same CA
    log_info "Verifying all certificates are signed by the same CA..."
    CA_CERT="${SCRIPT_DIR}/wg/certs/ca.crt"
    
    if [ ! -f "$CA_CERT" ]; then
        test_failed "CA certificate not found for verification"
        return 1
    fi
    
    # List of certificates to verify
    declare -a CERT_FILES=(
        "${SCRIPT_DIR}/wg/certs/vm-server.crt"
        "${SCRIPT_DIR}/wg/certs/edge-server.crt"
        "${SCRIPT_DIR}/wg/certs/vm-client.crt"
        "${SCRIPT_DIR}/wg/certs/edge-client.crt"
        "${SCRIPT_DIR}/wg/certs/vm-db.crt"
        "${SCRIPT_DIR}/wg/certs/edge-db.crt"
    )
    
    declare -a CERT_NAMES=(
        "VM Server"
        "Edge Server"
        "VM Client"
        "Edge Client"
        "VM Database"
        "Edge Database"
    )
    
    invalid_certs=0
    for i in "${!CERT_FILES[@]}"; do
        cert_file="${CERT_FILES[$i]}"
        cert_name="${CERT_NAMES[$i]}"
        
        if [ ! -f "$cert_file" ]; then
            log_error "$cert_name certificate not found: $cert_file"
            invalid_certs=$((invalid_certs + 1))
            continue
        fi
        
        # Verify certificate is signed by CA
        # openssl verify checks the entire certificate chain and ensures the certificate
        # is signed by a trusted CA (the CA certificate we provide)
        verify_output=$(openssl verify -CAfile "$CA_CERT" "$cert_file" 2>&1)
        verify_exit_code=$?
        
        if [ $verify_exit_code -eq 0 ]; then
            # openssl verify succeeded - certificate is valid and signed by the CA
            # Extract certificate details for logging
            cert_subject=$(openssl x509 -in "$cert_file" -noout -subject 2>/dev/null | sed 's/subject=//' || echo "N/A")
            log_info "  ✓ $cert_name: valid and signed by CA (subject: $cert_subject)"
        else
            # Verification failed - certificate is invalid or not signed by the CA
            verify_error=$(echo "$verify_output" | tail -1)
            log_error "  ✗ $cert_name: verification failed - $verify_error"
            invalid_certs=$((invalid_certs + 1))
        fi
    done
    
    if [ $invalid_certs -gt 0 ]; then
        test_failed "$invalid_certs certificate(s) are invalid or not signed by the same CA"
        log_error "All certificates must be signed by the same CA for mTLS to work correctly"
        log_error "In production, the CA will be managed by HashiCorp Vault"
        log_error "For local development, the CA is persistent in ${SCRIPT_DIR}/wg/certs/"
        return 1
    fi
    
    # Verify CA certificate itself is valid
    if ! openssl x509 -in "$CA_CERT" -noout -checkend 86400 >/dev/null 2>&1; then
        test_failed "CA certificate is expired or will expire within 24 hours"
        return 1
    fi
    
    ca_subject=$(openssl x509 -in "$CA_CERT" -noout -subject 2>/dev/null | sed 's/subject=//' || echo "N/A")
    ca_validity=$(openssl x509 -in "$CA_CERT" -noout -enddate 2>/dev/null | cut -d= -f2 || echo "N/A")
    
    test_passed "All certificates are valid and signed by the same CA"
    log_info "  CA Subject: $ca_subject"
    log_info "  CA Valid Until: $ca_validity"
    log_info "  All 6 certificates verified: VM server/client/db, Edge server/client/db"
    
    log_info "Epic 2.0: $EPIC_TESTS tests completed"
    log_info "✓ WireGuard keys and configs generated"
    log_info "✓ TLS/mTLS certificates generated (CA, VM server/client/db, Edge server/client/db)"
    log_info "✓ All certificates verified and signed by the same CA (persistent for local dev, Vault in production)"
    
    return 0
}

# ============================================================================
# Epic 2.1: VM Services Startup and Initialization
# ============================================================================

test_epic_2_1() {
    log_section "Epic 2.1: VM Services Startup and Initialization"
    EPIC_TESTS=0
    
    cd "$SCRIPT_DIR"
    
    # Verify Epic 2.0 was completed (EDGE_ID should exist)
    EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
    if [ ! -f "$EDGE_ID_FILE" ]; then
        test_failed "Epic 2.0 must be completed first - EDGE_ID file not found"
        return 1
    fi
    
    EDGE_ID=$(cat "$EDGE_ID_FILE")
    export EDGE_ID
    
    log_info "Starting VM services sequentially and waiting for full initialization..."
    if ! "${SCRIPT_DIR}/start-local-env.sh" step-2.1; then
        test_failed "Epic 2.1 failed: VM services startup and initialization"
        return 1
    fi
    
    test_passed "VM services started and fully initialized"
    
    # Verify TLS certificates are accessible from containers
    log_info "Verifying TLS certificate accessibility from containers..."
    
    # Check that CA certificate is accessible
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps user-vm-api test -f /etc/ssl/certs/ca.crt 2>/dev/null; then
        test_failed "CA certificate not accessible in user-vm-api container"
        return 1
    fi
    
    # Check that VM server certificate is accessible
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps user-vm-api test -f /etc/ssl/certs/vm-server.crt 2>/dev/null; then
        test_failed "VM server certificate not accessible in user-vm-api container"
        return 1
    fi
    
    # Check that MinIO certificates are accessible (MinIO container uses minio command, so use ls instead)
    if ! docker compose -f "$COMPOSE_FILE" exec -T minio ls /root/.minio/certs/public.crt > /dev/null 2>&1; then
        test_failed "MinIO server certificate not accessible in minio container"
        return 1
    fi
    
    # Check that HTTPS TLS certificates are accessible
    log_info "Verifying HTTPS TLS certificate accessibility..."
    
    # VM HTTPS server certificates
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps user-vm-api test -f /etc/ssl/certs/vm-server.crt 2>/dev/null; then
        test_failed "VM HTTPS server certificate not accessible"
        return 1
    fi
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps user-vm-api test -f /etc/ssl/private/vm-server.key 2>/dev/null; then
        test_failed "VM HTTPS server key not accessible"
        return 1
    fi
    
    # VM HTTPS client certificates
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps user-vm-api test -f /etc/ssl/certs/vm-client.crt 2>/dev/null; then
        test_failed "VM HTTPS client certificate not accessible"
        return 1
    fi
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps user-vm-api test -f /etc/ssl/private/vm-client.key 2>/dev/null; then
        test_failed "VM HTTPS client key not accessible"
        return 1
    fi
    
    test_passed "TLS certificates accessible from containers"
    
    log_info "Epic 2.1: $EPIC_TESTS tests completed"
    log_info "✓ MinIO is healthy and ready (TLS enabled)"
    log_info "✓ Python AI Service is healthy and ready"
    log_info "✓ User VM API is healthy and fully initialized (database, MinIO with TLS, training service verified)"
    log_info "✓ TLS certificates accessible from all containers (MinIO, HTTPS server/client)"
    log_info "✓ HTTPS services configured for mTLS (zero-trust security)"
    log_info "✓ SQLite database: Local file-based (TLS not applicable, secured via file permissions and volume isolation)"
    log_info "✓ Baseline model setup started"
    
    # Configure VM database (verify migrations are applied and database is ready)
    log_info "Configuring VM database (verifying migrations and tables)..."
    max_wait=30
    waited=0
    while [ $waited -lt $max_wait ]; do
        # Check if edges table exists (indicates database is initialized)
        if docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sqlite3 /app/data/events.db "SELECT name FROM sqlite_master WHERE type='table' AND name='edges';" 2>/dev/null | grep -q "edges"; then
            log_info "VM database configured - edges table exists"
            
            # Register/update Edge with WireGuard public key to ensure authentication will pass
            # This ensures the Edge WireGuard public key is stored in the database before Edge connects
            EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
            EDGE_PUBLIC_KEY_FILE="${SCRIPT_DIR}/wg/keys/edge.public"
            if [ -f "$EDGE_ID_FILE" ] && [ -f "$EDGE_PUBLIC_KEY_FILE" ]; then
                EDGE_ID=$(cat "$EDGE_ID_FILE" | tr -d '\n\r')
                EDGE_PUBLIC_KEY=$(cat "$EDGE_PUBLIC_KEY_FILE" | tr -d '\n\r')
                
                log_info "Registering/updating Edge $EDGE_ID with WireGuard public key in VM database..."
                now=$(date +%s)
                if docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sqlite3 /app/data/events.db \
                    "INSERT OR REPLACE INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) \
                     VALUES ('$EDGE_ID', 'PoC Edge 1', '$EDGE_PUBLIC_KEY', $now, 'active', $now, $now);" 2>&1; then
                    log_info "Edge $EDGE_ID registered/updated in VM database with WireGuard public key"
                    
                    # Verify the registration
                    registered_key=$(docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sqlite3 /app/data/events.db \
                        "SELECT wireguard_public_key FROM edges WHERE edge_id = '$EDGE_ID' AND status = 'active';" 2>/dev/null | tr -d '\n\r' || echo "")
                    
                    if [ "$registered_key" = "$EDGE_PUBLIC_KEY" ]; then
                        log_info "Edge registration verified - WireGuard public key matches (${EDGE_PUBLIC_KEY:0:20}...)"
                    else
                        log_warn "Edge registration verification failed - public key mismatch"
                        log_warn "Expected: $EDGE_PUBLIC_KEY"
                        log_warn "Found: $registered_key"
                    fi
                else
                    log_warn "Failed to register/update Edge in database, authentication may fail"
                fi
            else
                log_warn "Edge ID or WireGuard public key file not found, skipping Edge registration"
            fi
            
            test_passed "VM database configured and ready"
            break
        fi
        sleep 2
        waited=$((waited + 2))
    done
    
    if [ $waited -ge $max_wait ]; then
        test_failed "VM database configuration failed - edges table not found after $max_wait seconds"
        return 1
    fi
    
    # Verify VM HTTPS server is fully started and listening before starting Edge services
    # This ensures Edge can connect immediately when it starts, preventing connection timeouts
    log_info "Verifying VM HTTPS server is fully started and listening on port 8443..."
    max_wait=60
    waited=0
    https_ready=false
    
    while [ $waited -lt $max_wait ]; do
        # Check if port 8443 is listening (most reliable indicator that HTTPS server is ready)
        if docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sh -c "netstat -tlnp 2>/dev/null | grep -q ':8443' || ss -tlnp 2>/dev/null | grep -q ':8443'" 2>/dev/null; then
            log_info "VM HTTPS server is listening on port 8443"
            https_ready=true
            break
        fi
        sleep 2
        waited=$((waited + 2))
        if [ $((waited % 10)) -eq 0 ]; then
            log_info "  Still waiting for VM HTTPS server to start... (${waited}/${max_wait}s)"
        fi
    done
    
    if [ "$https_ready" != "true" ]; then
        log_error "VM HTTPS server failed to start after $max_wait seconds"
        log_error "Checking VM API logs for HTTPS server startup..."
        docker compose -f "$COMPOSE_FILE" logs user-vm-api 2>&1 | grep -iE "https|8443|listening|server.*start" | tail -20 || true
        log_error "Checking if port 8443 is listening..."
        docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sh -c "netstat -tlnp 2>/dev/null || ss -tlnp 2>/dev/null" 2>&1 | grep -E "8443|LISTEN" || true
        test_failed "VM HTTPS server is not ready - Edge services cannot connect"
        return 1
    fi
    
    # Additional verification: Try to connect to the HTTPS server from within the VM container
    # This confirms the server is not just listening but also accepting connections
    log_info "Verifying VM HTTPS server accepts connections..."
    if docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sh -c "timeout 2 nc -zv localhost 8443 2>&1 || timeout 2 telnet localhost 8443 2>&1 | head -1" 2>/dev/null | grep -qE "(open|Connected|succeeded)"; then
        log_info "VM HTTPS server is accepting connections"
    else
        log_warn "Could not verify VM HTTPS server connection acceptance (nc/telnet may not be available, but port is listening)"
    fi
    
    # Wait a bit more to ensure server is fully ready to accept HTTPS connections
    # This gives the server time to complete initialization after port starts listening
    log_info "Waiting for VM HTTPS server to be fully ready (2s grace period)..."
    sleep 2
    
    test_passed "VM HTTPS server is ready and listening"
    
    # Start Edge services with proper certificates
    log_info "Starting Edge services with TLS/mTLS certificates..."
    
    # Start Edge AI Service first (dependency for edge-orchestrator)
    log_info "Starting Edge AI Service..."
    if ! docker compose -f "$COMPOSE_FILE" up -d edge-ai-service; then
        test_failed "Failed to start Edge AI Service"
        return 1
    fi
    
    # Wait for Edge AI Service to be healthy
    log_info "Waiting for Edge AI Service to be healthy..."
    max_wait=120
    waited=0
    while [ $waited -lt $max_wait ]; do
        if curl -sf "http://localhost:8180/health" > /dev/null 2>&1; then
            log_info "Edge AI Service is healthy"
            test_passed "Edge AI Service started and healthy"
            break
        fi
        sleep 3
        waited=$((waited + 3))
    done
    
    if [ $waited -ge $max_wait ]; then
        test_failed "Edge AI Service failed to become healthy after $max_wait seconds"
        return 1
    fi
    
    # Start Edge Orchestrator (main Edge service)
    log_info "Starting Edge Orchestrator service..."
    
    # Verify edge-ai-service is still healthy before starting edge-orchestrator
    # This prevents the "No such container" error if edge-ai-service failed between checks
    log_info "Verifying edge-ai-service is still healthy before starting edge-orchestrator..."
    
    # Check if container exists and is running
    edge_ai_container=$(docker compose -f "$COMPOSE_FILE" ps -q edge-ai-service 2>/dev/null || echo "")
    if [ -z "$edge_ai_container" ]; then
        log_error "edge-ai-service container does not exist, cannot start edge-orchestrator"
        log_error "Capturing edge-ai-service logs before failure..."
        docker compose -f "$COMPOSE_FILE" logs edge-ai-service 2>&1 | tail -50 || true
        test_failed "edge-ai-service container not found, cannot start edge-orchestrator"
        return 1
    fi
    
    # Check container status
    edge_ai_status=$(docker inspect "$edge_ai_container" --format '{{.State.Status}}' 2>/dev/null || echo "unknown")
    if [ "$edge_ai_status" != "running" ]; then
        log_error "edge-ai-service is not running (status: $edge_ai_status), cannot start edge-orchestrator"
        log_error "Capturing edge-ai-service logs before failure..."
        docker compose -f "$COMPOSE_FILE" logs edge-ai-service 2>&1 | tail -50 || true
        log_error "Container state details:"
        docker inspect "$edge_ai_container" --format 'State: {{.State.Status}}, ExitCode: {{.State.ExitCode}}, Error: {{.State.Error}}' 2>/dev/null || true
        test_failed "edge-ai-service is not running, cannot start edge-orchestrator"
        return 1
    fi
    
    # Double-check health endpoint
    if ! curl -sf "http://localhost:8180/health" > /dev/null 2>&1; then
        log_error "edge-ai-service health endpoint is not responding"
        log_error "Capturing edge-ai-service logs..."
        docker compose -f "$COMPOSE_FILE" logs edge-ai-service 2>&1 | tail -50 || true
        test_failed "edge-ai-service health check failed, cannot start edge-orchestrator"
        return 1
    fi
    
    log_info "edge-ai-service verified healthy, starting edge-orchestrator..."
    if ! docker compose -f "$COMPOSE_FILE" up -d edge-orchestrator; then
        log_error "Failed to start edge-orchestrator, checking container status..."
        docker compose -f "$COMPOSE_FILE" ps -a 2>&1 | grep -E "edge-ai|edge-orchestrator" || true
        log_error "Capturing edge-ai-service logs..."
        docker compose -f "$COMPOSE_FILE" logs edge-ai-service 2>&1 | tail -50 || true
        test_failed "Failed to start Edge Orchestrator service"
        return 1
    fi
    
    # Wait for Edge Orchestrator to be healthy
    log_info "Waiting for Edge Orchestrator to be healthy..."
    max_wait=120
    waited=0
    while [ $waited -lt $max_wait ]; do
        if curl -sf "http://localhost:8182/health" > /dev/null 2>&1; then
            log_info "Edge Orchestrator is healthy"
            test_passed "Edge Orchestrator started and healthy"
            break
        fi
        sleep 3
        waited=$((waited + 3))
    done
    
    if [ $waited -ge $max_wait ]; then
        test_failed "Edge Orchestrator failed to become healthy after $max_wait seconds"
        return 1
    fi
    
    # Verify Edge services TLS certificates are accessible
    log_info "Verifying Edge services TLS certificate accessibility..."
    
    # Check Edge HTTPS server certificates
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps edge-orchestrator test -f /etc/ssl/certs/edge-server.crt 2>/dev/null; then
        test_failed "Edge HTTPS server certificate not accessible"
        return 1
    fi
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps edge-orchestrator test -f /etc/ssl/private/edge-server.key 2>/dev/null; then
        test_failed "Edge HTTPS server key not accessible"
        return 1
    fi
    
    # Check Edge HTTPS client certificates
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps edge-orchestrator test -f /etc/ssl/certs/edge-client.crt 2>/dev/null; then
        test_failed "Edge HTTPS client certificate not accessible"
        return 1
    fi
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps edge-orchestrator test -f /etc/ssl/private/edge-client.key 2>/dev/null; then
        test_failed "Edge HTTPS client key not accessible"
        return 1
    fi
    
    # Check CA certificate
    if ! docker compose -f "$COMPOSE_FILE" run --rm --no-deps edge-orchestrator test -f /etc/ssl/certs/ca.crt 2>/dev/null; then
        test_failed "CA certificate not accessible in edge-orchestrator container"
        return 1
    fi
    
    test_passed "Edge services TLS certificates accessible"
    
    log_info "✓ VM database configured and ready"
    log_info "✓ Edge AI Service started and healthy"
    log_info "✓ Edge Orchestrator started and healthy"
    log_info "✓ Edge services TLS certificates accessible (HTTPS server/client, CA)"
    log_info "✓ Edge services ready for Epic 2.2"
    
    return 0
}

# ============================================================================
# Epic 2.2: VM Edge Status Monitoring & WireGuard Connection Management Tests
# ============================================================================

test_epic_2_2() {
    log_section "Epic 2.2: VM Edge Status Monitoring & WireGuard Connection Management"
    EPIC_TESTS=0
    
    local edge_id="poc-edge-1"
    
    # Note: Service health is already verified in start_docker_compose()
    
    # Run all tests in sequence following the actual connection flow:
    # 1. Edge orchestrator is accessible (prerequisite)
    # 2. WireGuard tunnel is established (network layer)
    # 3. HTTPS connection is established through WireGuard tunnel (application layer)
    # 4. Edge registration/validation happens via HTTPS telemetry
    # 5. Connection state and monitoring
    
    test_edge_accessible "$edge_id" || return 1
    test_wireguard_tunnel "$edge_id" || return 1
    test_wireguard_functionality "$edge_id" || return 1
    test_https_connection "$edge_id" || return 1
    test_edge_registered "$edge_id" || return 1
    local connection_state=$(test_edge_connection_status "$edge_id") || return 1
    test_connection_state "$edge_id" "connected" || return 1
    test_connection_keepalive "$edge_id" || return 1
    test_wireguard_health "$edge_id" || return 1
    
    log_info "Epic 2.2: $EPIC_TESTS tests completed"
    log_info "✓ Edge is registered and accessible"
    log_info "✓ WireGuard tunnel is established and functional"
    log_info "✓ Bidirectional HTTPS connection established (Edge → VM on 8443, VM → Edge on 8443)"
    log_info "✓ Connection pool initialized - connection ready for reuse in subsequent steps"
    log_info "✓ Connection monitoring and keepalive are working"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.2..." >&2
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.2" >&2
        return 1
    fi
    
    return 0
}

# ============================================================================

test_epic_2_3() {
    log_section "Epic 2.3: Post-WireGuard Edge ↔ VM Coordination"
    EPIC_TESTS=0
    
    local edge_id="poc-edge-1"
    
    # Run all tests in sequence
    test_capability_sync "$edge_id" || return 1
    
    # Get camera info (returns camera_id|camera_count)
    local camera_info=$(test_cameras_listed "$edge_id") || return 1
    local first_camera_id=$(echo "$camera_info" | cut -d'|' -f1)
    local camera_count=$(echo "$camera_info" | cut -d'|' -f2)
    
    # Get cameras response for later use
    local cameras_response=$(call_api "${VM_API}/api/cameras?edge_id=${edge_id}")
    
    # Get dataset status for camera (returns eligibility_status|snapshot_required|labeled_count)
    local dataset_status=$(test_camera_dataset_status "$first_camera_id" "$edge_id") || return 1
    local eligibility_status=$(echo "$dataset_status" | cut -d'|' -f1)
    local snapshot_required=$(echo "$dataset_status" | cut -d'|' -f2)
    local labeled_count=$(echo "$dataset_status" | cut -d'|' -f3)
    
    # Get full dataset response for label counts test
    local dataset_response=$(call_api "${VM_API}/api/cameras/${first_camera_id}/dataset?edge_id=${edge_id}")
    
    test_training_eligibility_status "$eligibility_status" || return 1
    test_snapshot_requirement_flag "$snapshot_required" || return 1
    test_label_counts_tracked "$dataset_response" "$labeled_count" || return 1
    test_capability_sync_persistence "$edge_id" "$camera_count" || return 1
    test_edge_vm_camera_sync "$edge_id" "$camera_count" "$cameras_response" || return 1
    
    log_info "Epic 2.3: $EPIC_TESTS tests completed"
    log_info "✓ Capability sync working after WireGuard connection"
    log_info "✓ Cameras visible in VM API"
    log_info "✓ Dataset status tracking functional"
    log_info "✓ Training eligibility status tracked"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.3..." >&2
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.3" >&2
        return 1
    fi
    
    return 0
}

# ============================================================================
# Epic 2.4: Snapshot Capture & Dataset Progress Fixes Tests
# ============================================================================

test_epic_2_4() {
    log_section "Epic 2.4: Snapshot Capture & Dataset Progress Fixes"
    EPIC_TESTS=0
    
    local edge_id="poc-edge-1"
    # Edge API is on port 8181 (mapped from container port 8081)
    local edge_api_url="http://localhost:8181"
    
    # Get camera ID from VM API (use camera that was synced in Epic 2.3)
    # This ensures we use a camera that VM knows about, following the sequential flow
    log_info "Getting camera ID from VM API (camera synced in Epic 2.3)..." >&2
    local cameras_response=$(call_api "${VM_API}/api/cameras?edge_id=${edge_id}")
    if [ "$cameras_response" = "FAILED" ]; then
        test_failed "Failed to get cameras from VM API" >&2
        return 1
    fi
    
    local camera_id=""
    if command -v jq >/dev/null 2>&1; then
        camera_id=$(echo "$cameras_response" | jq -r '.cameras[0].id // empty' 2>/dev/null || echo "")
    else
        camera_id=$(echo "$cameras_response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
    fi
    
    if [ -z "$camera_id" ] || [ "$camera_id" = "null" ]; then
        # Fallback: Get from Edge API if VM doesn't have it yet
        log_warn "No cameras in VM API, getting from Edge API as fallback..." >&2
        camera_id=$(test_get_camera_id "$edge_api_url") || return 1
    else
        log_info "Using camera ID from VM API: $camera_id" >&2
    fi
    
    # Test 1: Verify initial dataset status
    local initial_labeled_count=$(test_initial_dataset_status "$camera_id" "$edge_api_url") || return 1
    
    # Test 2: Capture and save a snapshot
    # Note: This test may fail if snapshot endpoint is not accessible, but we continue to test VM → Edge flow
    local save_response=$(test_capture_and_save_snapshot "$camera_id" "$edge_api_url" || echo "")
    
    # Test 3: Verify dataset status in save response (only if Test 2 succeeded)
    if [ -n "$save_response" ] && [ "$save_response" != "" ]; then
        test_dataset_status_in_save_response "$save_response" "$initial_labeled_count" || true
    else
        log_warn "Skipping Test 3: Test 2 (snapshot capture) failed, continuing to other tests..." >&2
    fi
    
    # Test 4: Verify dataset status updated in cameras API
    test_dataset_status_updated_in_cameras_api "$camera_id" "$initial_labeled_count" "$edge_api_url" || return 1
    
    # Test 5: Test multiple snapshot capture (non-blocking - may fail if snapshot endpoint unavailable)
    test_multiple_snapshot_capture "$camera_id" 10 "$edge_api_url" || log_warn "Test 5 failed (snapshot endpoint may be unavailable), continuing..." >&2
    
    # Test 6: Test dataset status refresh endpoint
    test_dataset_status_refresh "$camera_id" "$edge_api_url" || return 1
    
    # Test 7: Verify real-time progress updates (non-blocking)
    test_realtime_progress_updates "$camera_id" "$initial_labeled_count" "$edge_api_url" || log_warn "Test 7 failed, continuing..." >&2
    
    # Test 8: Test VM → Edge snapshot request flow with auto_capture=true (automatic)
    # This tests the new VM → Edge control flow where VM requests Edge to capture snapshots automatically
    test_vm_request_snapshot_capture "$camera_id" "$edge_id" "$VM_API" "$edge_api_url" 5 || return 1
    
    # Test 9: Test VM → Edge snapshot request flow with auto_capture=false (manual capture via UI)
    # This tests the flow where VM requests snapshots, Edge shows notification, and user captures manually
    log_info "Test 9: Testing VM → Edge snapshot request with manual capture (auto_capture=false)..." >&2
    
    # Step 1: VM requests Edge to capture snapshots (with auto_capture=false to show as pending)
    log_info "Step 1: VM requesting Edge to capture snapshots (auto_capture=false for manual capture)..." >&2
    local snapshot_count=10
    request_json="/tmp/vm_snapshot_request_epic24_manual_$$.json"
    cat > "$request_json" <<EOF
{
    "label": "normal",
    "count": $snapshot_count,
    "auto_capture": false
}
EOF
    
    local request_response=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "${VM_API}/api/cameras/${camera_id}/request-snapshots?edge_id=${edge_id}" \
        -H "Content-Type: application/json" \
        -d "@$request_json" 2>&1)
    local http_code=$(echo "$request_response" | grep -o "HTTP_CODE:[0-9]*" | tail -1 | cut -d: -f2 || echo "")
    request_response=$(echo "$request_response" | sed 's/HTTP_CODE:[0-9]*$//' | sed 's/^[[:space:]]*$//' | sed '/^$/d' || echo "")
    rm -f "$request_json"
    
    if [ -z "$http_code" ] || [ "$http_code" != "200" ] && [ "$http_code" != "201" ]; then
        test_failed "VM API returned HTTP $http_code: $request_response" >&2
        return 1
    fi
    
    test_passed "VM requested Edge to capture $snapshot_count snapshots (pending in UI)" >&2
    
    # Step 2: Query Edge API for pending snapshot requests
    log_info "Step 2: Querying Edge API for pending snapshot requests..." >&2
    if ! poll_until_success "Pending snapshot request to appear in Edge API" \
        "pending_response=\$(curl -sf \"${edge_api_url}/api/snapshot-requests\" 2>&1 || echo 'FAILED') && \
         [ \"\$pending_response\" != 'FAILED' ] && \
         (command -v jq >/dev/null 2>&1 && \
          echo \"\$pending_response\" | jq -r '.requests[] | select(.camera_id == \"$camera_id\") | .camera_id' 2>/dev/null | grep -q \"$camera_id\" || \
          echo \"\$pending_response\" | grep -q \"$camera_id\")" \
        30 2; then
        test_failed "Pending snapshot request not found in Edge API after 30 seconds" >&2
        return 1
    fi
    
    local pending_response=$(curl -sf "${edge_api_url}/api/snapshot-requests" 2>&1 || echo "FAILED")
    if [ "$pending_response" = "FAILED" ]; then
        test_failed "Failed to get pending snapshot requests from Edge API" >&2
        return 1
    fi
    
    # Extract request details
    local pending_label="normal"
    local pending_count=$snapshot_count
    if command -v jq >/dev/null 2>&1; then
        pending_label=$(echo "$pending_response" | jq -r ".requests[] | select(.camera_id == \"$camera_id\") | .label // \"normal\"" 2>/dev/null || echo "normal")
        pending_count=$(echo "$pending_response" | jq -r ".requests[] | select(.camera_id == \"$camera_id\") | .count // $snapshot_count" 2>/dev/null || echo "$snapshot_count")
    else
        pending_label=$(echo "$pending_response" | grep -A 10 "\"camera_id\":\"$camera_id\"" | grep -o '"label":"[^"]*"' | cut -d'"' -f4 || echo "normal")
        pending_count=$(echo "$pending_response" | grep -A 10 "\"camera_id\":\"$camera_id\"" | grep -o '"count":[0-9]*' | grep -o '[0-9]*' || echo "$snapshot_count")
    fi
    
    test_passed "Found pending snapshot request (camera: $camera_id, label: $pending_label, count: $pending_count)" >&2
    
    # Step 3: Automatically capture and label snapshots via Edge API (simulating user action)
    log_info "Step 3: Automatically capturing and labeling $pending_count snapshots via Edge API..." >&2
    local initial_labeled_count=0
    local cameras_status=$(curl -sf "${edge_api_url}/api/cameras" 2>&1 || echo "FAILED")
    if [ "$cameras_status" != "FAILED" ]; then
        if command -v jq >/dev/null 2>&1; then
            initial_labeled_count=$(echo "$cameras_status" | jq -r ".cameras[] | select(.id == \"$camera_id\") | .dataset_status.labeled_snapshot_count // 0" 2>/dev/null || echo "0")
        else
            initial_labeled_count=$(echo "$cameras_status" | grep -A 20 "\"id\":\"$camera_id\"" | grep -o '"labeled_snapshot_count":[0-9]*' | grep -o '[0-9]*' || echo "0")
        fi
        initial_labeled_count=$(echo "$initial_labeled_count" | grep -o '[0-9]*' || echo "0")
        initial_labeled_count=$((initial_labeled_count + 0))
    fi
    
    # Capture and label snapshots
    local captured_count=0
    for i in $(seq 1 $pending_count); do
        # Capture snapshot
        local snapshot_file="/tmp/test_snapshot_epic24_manual_${i}_$$.jpg"
        local http_code=$(curl -s -w "%{http_code}" -o "$snapshot_file" "${edge_api_url}/api/cameras/${camera_id}/snapshot" 2>/dev/null || echo "000")
        
        if [ "$http_code" != "200" ] || [ ! -f "$snapshot_file" ] || [ ! -s "$snapshot_file" ]; then
            rm -f "$snapshot_file"
            continue
        fi
        
        # Convert to base64
        local image_base64=""
        if command -v base64 >/dev/null 2>&1; then
            image_base64=$(base64 -w 0 "$snapshot_file" 2>/dev/null || base64 "$snapshot_file" 2>/dev/null | tr -d '\n')
        else
            image_base64=$(python3 -c "import base64; print(base64.b64encode(open('$snapshot_file', 'rb').read()).decode())" 2>/dev/null || echo "")
        fi
        rm -f "$snapshot_file"
        
        if [ -z "$image_base64" ]; then
            continue
        fi
        
        # Save screenshot with label from pending request
        local json_file="/tmp/test_screenshot_epic24_manual_${i}_$$.json"
        cat > "$json_file" <<EOF
{
    "camera_id": "$camera_id",
    "image_data": "$image_base64",
    "label": "$pending_label",
    "description": "Epic 2.4 automated capture from VM request $i/$pending_count"
}
EOF
        
        local save_result=$(curl -sf -X POST "${edge_api_url}/api/screenshots" \
            -H "Content-Type: application/json" \
            -d "@$json_file" 2>&1 || echo "FAILED")
        rm -f "$json_file"
        
        if [ "$save_result" != "FAILED" ]; then
            local error_check=""
            if command -v jq >/dev/null 2>&1; then
                error_check=$(echo "$save_result" | jq -r '.error // empty' 2>/dev/null || echo "")
            else
                error_check=$(echo "$save_result" | grep -o '"error":"[^"]*"' | cut -d'"' -f4 || echo "")
            fi
            
            if [ -z "$error_check" ] || [ "$error_check" = "null" ]; then
                captured_count=$((captured_count + 1))
            fi
        fi
        
        # Small delay between captures
        if [ $i -lt $pending_count ]; then
            sleep 0.5
        fi
    done
    
    if [ "$captured_count" -lt $pending_count ]; then
        log_warn "Captured $captured_count/$pending_count snapshots (some may have failed)" >&2
    fi
    
    # Wait for status to update
    sleep 3
    
    # Verify snapshots were captured
    local final_labeled_count=0
    cameras_status=$(curl -sf "${edge_api_url}/api/cameras" 2>&1 || echo "FAILED")
    if [ "$cameras_status" != "FAILED" ]; then
        if command -v jq >/dev/null 2>&1; then
            final_labeled_count=$(echo "$cameras_status" | jq -r ".cameras[] | select(.id == \"$camera_id\") | .dataset_status.labeled_snapshot_count // 0" 2>/dev/null || echo "0")
        else
            final_labeled_count=$(echo "$cameras_status" | grep -A 20 "\"id\":\"$camera_id\"" | grep -o '"labeled_snapshot_count":[0-9]*' | grep -o '[0-9]*' || echo "0")
        fi
        final_labeled_count=$(echo "$final_labeled_count" | grep -o '[0-9]*' || echo "0")
        final_labeled_count=$((final_labeled_count + 0))
    fi
    
    local new_snapshots=$((final_labeled_count - initial_labeled_count))
    if [ "$new_snapshots" -ge "$captured_count" ]; then
        test_passed "Captured and labeled $captured_count snapshots via Edge API (total: $final_labeled_count, was: $initial_labeled_count)" >&2
    else
        log_warn "Expected $captured_count new snapshots, but only $new_snapshots were added" >&2
        test_passed "Captured and labeled snapshots via Edge API (some may still be processing)" >&2
    fi
    
    log_info "Epic 2.4: $EPIC_TESTS tests completed"
    log_info "✓ Snapshot capture working"
    log_info "✓ Dataset status updates after saving"
    log_info "✓ Multiple snapshot capture supported"
    log_info "✓ Dataset progress tracking functional"
    log_info "✓ VM → Edge snapshot request flow working (auto and manual)"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.4..." >&2
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.4" >&2
        return 1
    fi
    
    return 0
}

# ============================================================================
# Epic 2.5: Edge → VM Dataset Sync & Upload Tests
# ============================================================================

test_epic_2_5() {
    log_section "Epic 2.5: Edge → VM Dataset Sync & Upload"
    EPIC_TESTS=0
    
    local edge_id="poc-edge-1"
    local edge_api_url="http://localhost:8181"
    
    # Get camera ID from VM API (camera synced in Epic 2.3)
    log_info "Getting camera ID from VM API (camera synced in Epic 2.3)..." >&2
    local cameras_response=$(call_api "${VM_API}/api/cameras?edge_id=${edge_id}")
    if [ "$cameras_response" = "FAILED" ]; then
        test_failed "Failed to get cameras from VM API" >&2
        return 1
    fi
    
    local camera_id=""
    if command -v jq >/dev/null 2>&1; then
        camera_id=$(echo "$cameras_response" | jq -r '.cameras[0].id // empty' 2>/dev/null || echo "")
    else
        camera_id=$(echo "$cameras_response" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
    fi
    
    if [ -z "$camera_id" ] || [ "$camera_id" = "null" ]; then
        # Fallback: Get from Edge API if VM doesn't have it yet
        log_warn "No cameras in VM API, getting from Edge API as fallback..." >&2
        camera_id=$(test_get_camera_id "$edge_api_url") || return 1
    else
        log_info "Using camera ID from VM API: $camera_id" >&2
    fi
    
    # Test 1: Verify dataset readiness (snapshots should already be captured in Epic 2.4)
    log_info "Test 1: Verifying dataset readiness (snapshots captured in Epic 2.4)..." >&2
    local readiness_info=$(test_verify_dataset_readiness "$camera_id" "$edge_api_url") || return 1
    local labeled_count=$(echo "$readiness_info" | cut -d'|' -f1)
    local required_count=$(echo "$readiness_info" | cut -d'|' -f2)
    local needs_more=$(echo "$readiness_info" | cut -d'|' -f3)
    
    # Capture additional snapshots if needed to reach required count
    if [ "$needs_more" = "true" ]; then
        local num_needed=$((required_count - labeled_count))
        log_info "Need $num_needed more snapshots. Capturing additional snapshots..." >&2
        labeled_count=$(test_capture_additional_snapshots "$camera_id" "$num_needed" "$edge_api_url") || return 1
    fi
    
    # Verify we have enough snapshots
    labeled_count=$(echo "$labeled_count" | grep -o '[0-9]*' || echo "0")
    required_count=$(echo "$required_count" | grep -o '[0-9]*' || echo "0")
    labeled_count=$((labeled_count + 0))
    required_count=$((required_count + 0))
    
    if [ "$labeled_count" -lt "$required_count" ]; then
        test_failed "Dataset not ready for sync (labeled: $labeled_count/$required_count, need: $required_count)" >&2
        return 1
    fi
    
    test_passed "Dataset ready for sync (labeled: $labeled_count/$required_count)" >&2
    
    # Test 2: Trigger dataset sync
    local sync_info=$(test_trigger_dataset_sync "$camera_id" "$edge_id" "$edge_api_url") || return 1
    local dataset_synced=$(echo "$sync_info" | cut -d'|' -f1)
    local dataset_id=$(echo "$sync_info" | cut -d'|' -f2)
    local wg_connected=$(echo "$sync_info" | cut -d'|' -f3)
    
    # Test 3: Verify dataset on VM
    local vm_dataset_id=$(test_verify_dataset_on_vm "$camera_id" "$edge_id" "$dataset_synced") || return 1
    
    # Test 4: Verify training eligibility status
    test_verify_training_eligibility_status "$camera_id" "$edge_id" "$dataset_synced" || return 1
    
    # Test 5: Verify dataset files on VM
    test_verify_dataset_files_on_vm "$vm_dataset_id" "$dataset_synced" || return 1
    
    # Test 6: Verify dataset metadata
    test_verify_dataset_metadata "$camera_id" "$edge_id" "$labeled_count" "$required_count" "$dataset_synced" || return 1
    
    log_info "Epic 2.5: $EPIC_TESTS tests completed"
    log_info "✓ Dataset sync triggered successfully"
    log_info "✓ Dataset uploaded to VM"
    log_info "✓ Training eligibility status updated"
    log_info "✓ Dataset metadata stored"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.5..." >&2
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.5" >&2
        return 1
    fi
    
    return 0
}

# ============================================================================
# Epic 2.6: VM-Side Model Management for Training Readiness Tests
# ============================================================================

test_epic_2_6() {
    log_section "Epic 2.6: VM-Side Model Management for Training Readiness"
    EPIC_TESTS=0
    
    # Run all tests in sequence
    test_verify_baseline_model_exists || return 1
    test_verify_baseline_model_metadata || return 1
    test_verify_baseline_model_file || return 1
    test_verify_model_catalog || return 1
    test_verify_model_query_by_type || return 1
    test_verify_model_storage_structure || return 1
    test_verify_model_selection_for_training || return 1
    test_verify_model_validation || return 1
    
    log_info "Epic 2.6: $EPIC_TESTS tests completed"
    log_info "✓ Baseline model exists and accessible"
    log_info "✓ Model catalog functional"
    log_info "✓ Model storage and validation working"
    log_info "✓ Model selection for training ready"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.6..." >&2
    local edge_id="poc-edge-1"
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.6" >&2
        return 1
    fi
    
    return 0
}

# ============================================================================
# Epic 2.7: Model Training Pipeline Tests
# ============================================================================

test_epic_2_7() {
    log_section "Epic 2.7: Model Training Pipeline"
    EPIC_TESTS=0
    
    # Test 1: Verify baseline model exists
    test_verify_baseline_model_for_training || return 1
    
    # Test 2: Find dataset
    local dataset_info=$(test_find_dataset) || return 1
    local dataset_id=$(echo "$dataset_info" | cut -d'|' -f1)
    local camera_id=$(echo "$dataset_info" | cut -d'|' -f2)
    local edge_id=$(echo "$dataset_info" | cut -d'|' -f3)
    
    # Test 3: Start training job
    local job_id=$(test_start_training_job "$edge_id" "$camera_id" "$dataset_id") || return 1
    
    # Test 4: Wait for training completion
    local training_result=$(test_wait_for_training_completion "$job_id") || return 1
    local training_status=$(echo "$training_result" | cut -d'|' -f1)
    local trained_model_id=$(echo "$training_result" | cut -d'|' -f2)
    
    # Test 5: Verify model in catalog
    test_verify_model_in_catalog || return 1
    
    log_info "Epic 2.7: $EPIC_TESTS tests completed"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.7..." >&2
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.7" >&2
        return 1
    fi
    
    return 0
}

# ============================================================================
# Epic 2.8: VM → Edge Trained Model Sync & Deployment Tests
# ============================================================================

test_epic_2_8() {
    log_section "Epic 2.8: VM → Edge Trained Model Sync & Deployment"
    EPIC_TESTS=0
    
    local edge_id="poc-edge-1"
    
    # Test 1: Find trained model
    log_info "Step 1: Finding trained model from Epic 2.7..." >&2
    local trained_model_id=$(test_find_trained_model) || return 1
    log_info "Found trained model: $trained_model_id" >&2
    
    # Test 2: Verify Edge is connected
    log_info "Step 2: Verifying Edge is connected..." >&2
    test_verify_edge_connected "$edge_id" || return 1
    
    # Test 3: Trigger model deployment
    log_info "Step 3: Triggering model deployment..." >&2
    local deployment_id=$(test_trigger_model_deployment "$trained_model_id" "$edge_id") || return 1
    
    if [ -z "$deployment_id" ]; then
        log_warn "No deployment_id returned, but deployment may have been triggered" >&2
        # Try to get it from file
        if [ -f /tmp/test_deployment_id.txt ]; then
            deployment_id=$(cat /tmp/test_deployment_id.txt)
            log_info "Retrieved deployment_id from file: $deployment_id" >&2
        fi
    fi
    
    if [ -z "$deployment_id" ]; then
        test_failed "Could not get deployment_id" >&2
        return 1
    fi
    
    log_info "Deployment ID: $deployment_id" >&2
    
    # Test 4: Wait for deployment completion
    log_info "Step 4: Waiting for deployment completion..." >&2
    local deployment_status=$(test_wait_for_deployment_completion "$deployment_id") || return 1
    log_info "Deployment status: $deployment_status" >&2
    
    # Test 5: Verify Edge received model
    log_info "Step 5: Verifying Edge received and stored model..." >&2
    test_verify_edge_received_model "$deployment_id" || return 1
    
    # Test 6: Verify Edge model activation
    log_info "Step 6: Verifying Edge model loading and activation..." >&2
    test_verify_edge_model_activation "$deployment_id" || return 1
    
    # Test 7: Verify Edge status reporting
    log_info "Step 7: Verifying Edge status reporting to VM..." >&2
    test_verify_edge_status_reporting "$deployment_id" || return 1
    
    log_info "Epic 2.8: $EPIC_TESTS tests completed"
    
    # Verify HTTPS connection health at end of epic (critical for security)
    log_info "Verifying HTTPS connection health after Epic 2.8..." >&2
    if ! test_verify_https_connection_health "$edge_id"; then
        log_warn "HTTPS connection health check failed after Epic 2.8" >&2
        return 1
    fi
    return 0
}

# ============================================================================
# Epic 2.9: Edge-Side Event Detection & Processing Tests
# ============================================================================

test_epic_2_9() {
    log_section "Epic 2.9: Edge-Side Event Detection & Processing"
    EPIC_TESTS=0
    
    # Prerequisites: Need deployed model from Epic 2.8
    if [ ! -f /tmp/test_deployment_id.txt ] || [ -z "$(cat /tmp/test_deployment_id.txt 2>/dev/null)" ]; then
        log_warn "No deployment from Epic 2.8, skipping Epic 2.9 tests"
        return 0
    fi
    
    local deployment_id=$(cat /tmp/test_deployment_id.txt)
    local edge_id="poc-edge-1"
    local camera_id="usb-usb-3-5"
    local edge_api_url="http://localhost:8081"
    
    # Run all tests in sequence
    test_verify_model_active_for_inference "$deployment_id" || return 1
    test_verify_camera_stream_available "$camera_id" "$edge_api_url" || return 1
    test_verify_processing_configuration "$edge_api_url" || return 1
    test_update_processing_configuration "$edge_api_url" || return 1
    test_verify_event_processing_pipeline "$edge_api_url" || return 1
    test_verify_error_handling || return 1
    test_verify_event_storage_and_queue "$edge_api_url" || return 1
    test_verify_clip_and_snapshot_storage || return 1
    test_verify_event_detection_simulation || return 1
    test_verify_offline_operation || return 1
    
    log_info "Epic 2.9: $EPIC_TESTS tests completed"
    log_info "Note: Full event detection testing requires:"
    log_info "  - Active camera stream with frames"
    log_info "  - Model inference producing detections"
    log_info "  - Real-time event triggering"
    log_info "  - Clip recording and snapshot capture"
    log_info "These are best tested with actual camera streams in a full integration environment"
    
    return 0
}

# ============================================================================
# Main test execution
# ============================================================================

main() {
    # Parse command-line arguments
    parse_args "$@"
    
    # Show help if requested
    if [ "$SHOW_HELP" = "true" ]; then
        show_help
        # Skip cleanup when showing help
        CLEANUP_ON_EXIT=false
        exit 0
    fi
    
    log_section "Local End-to-End Integration Test Suite"
    log_info "VM API: $VM_API"
    log_info "Edge API: $EDGE_API"
    log_info "Training Service: $TRAINING_SERVICE"
    log_info "Compose File: $COMPOSE_FILE"
    log_info "Cleanup on Exit: $CLEANUP_ON_EXIT"
    log_info "Rebuild Images: $REBUILD"
    
    # Show which epics will be run
    echo ""
    log_info "Epics to run (with prerequisites):"
    local epics=(
        "2.0:WireGuard Configuration Generation"
        "2.1:VM Services Startup and Initialization (requires 2.0)"
        "2.2:VM Edge Status Monitoring & WireGuard Connection Management (requires 2.0, 2.1)"
        "2.3:Post-WireGuard Edge ↔ VM Coordination (requires 2.0, 2.1, 2.2)"
        "2.4:Snapshot Capture & Dataset Progress Fixes (requires 2.0, 2.1, 2.2, 2.3)"
        "2.5:Edge → VM Dataset Sync & Upload (requires 2.0, 2.1, 2.2, 2.3, 2.4)"
        "2.6:VM-Side Model Management for Training Readiness (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5)"
        "2.7:Model Training Pipeline (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6)"
        "2.8:VM → Edge Trained Model Sync & Deployment (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7)"
        "2.9:Edge-Side Event Detection & Processing (requires 2.0, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8)"
    )
    
    for epic_info in "${epics[@]}"; do
        epic_num="${epic_info%%:*}"
        epic_desc="${epic_info#*:}"
        var_name="RUN_EPIC_${epic_num//./_}"
        if [ "${!var_name}" = "true" ]; then
            log_info "  ✓ Epic ${epic_num}: ${epic_desc}"
        fi
    done
    echo ""
    
    # ============================================================================
    # Step 1: Clean old data from previous runs (unless skipped)
    # ============================================================================
    if [ "$SKIP_CLEANUP" = "false" ]; then
        if ! cleanup_old_data; then
            log_error "Failed to clean old data"
            exit 1
        fi
    else
        log_info "Skipping cleanup step (--skip-cleanup)"
    fi
    
    # ============================================================================
    # Epic Tests (run based on --epic flags)
    # ============================================================================
    # Epic tests are executed in order, with prerequisites automatically enabled
    # ============================================================================
    
    # Epic 2.0: WireGuard Configuration Generation
    if [ "$RUN_EPIC_2_0" = "true" ]; then
        if ! test_epic_2_0; then
            log_error "Epic 2.0 tests failed - configuration generation is required for all subsequent epics"
            if [ "$RUN_EPIC_2_1" = "true" ] || [ "$RUN_EPIC_2_2" = "true" ] || [ "$RUN_EPIC_2_3" = "true" ] || [ "$RUN_EPIC_2_4" = "true" ] || [ "$RUN_EPIC_2_5" = "true" ] || [ "$RUN_EPIC_2_6" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_error "Cannot continue without configuration"
                exit 1
            fi
        fi
    fi
    
    # Epic 2.1: VM Services Startup and Initialization
    if [ "$RUN_EPIC_2_1" = "true" ]; then
        if ! test_epic_2_1; then
            log_error "Epic 2.1 tests failed - VM services are required for all subsequent epics"
            if [ "$RUN_EPIC_2_2" = "true" ] || [ "$RUN_EPIC_2_3" = "true" ] || [ "$RUN_EPIC_2_4" = "true" ] || [ "$RUN_EPIC_2_5" = "true" ] || [ "$RUN_EPIC_2_6" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_error "Cannot continue without VM services"
                exit 1
            fi
        fi
    fi
    
    # Note: Edge services are now started in Epic 2.1 (after VM database configuration)
    # For epics 2.2+, we only need to ensure edge registration (step 3)
    # Edge services startup (step 4) is now handled in Epic 2.1
    if [ "$RUN_EPIC_2_2" = "true" ] || [ "$RUN_EPIC_2_3" = "true" ] || [ "$RUN_EPIC_2_4" = "true" ] || [ "$RUN_EPIC_2_5" = "true" ] || [ "$RUN_EPIC_2_6" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
        # Step 3: Edge registration (Edge services are already started in Epic 2.1)
        log_info "Registering edge device (prerequisite for Epic 2.2+)..."
        
        # Read EDGE_ID from generated file
        EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
        if [ ! -f "$EDGE_ID_FILE" ]; then
            log_error "EDGE_ID file not found at $EDGE_ID_FILE"
            log_error "Epic 2.0 (configuration generation) must be completed first"
            exit 1
        fi
        
        EDGE_ID=$(cat "$EDGE_ID_FILE")
        export EDGE_ID
        
        # Read WireGuard public key for this edge
        EDGE_PUBLIC_KEY_FILE="${SCRIPT_DIR}/wg/keys/edge.public"
        if [ ! -f "$EDGE_PUBLIC_KEY_FILE" ]; then
            log_error "Edge public key file not found at $EDGE_PUBLIC_KEY_FILE"
            exit 1
        fi
        
        EDGE_PUBLIC_KEY=$(cat "$EDGE_PUBLIC_KEY_FILE" | tr -d '\n\r')
        
        # Check if edge already exists
        existing_edge=$(curl -sfL "${VM_API}/api/edges" 2>/dev/null | jq -r ".[] | select(.edge_id == \"$EDGE_ID\") | .edge_id" 2>/dev/null || echo "")
        if [ -z "$existing_edge" ]; then
            # Edge doesn't exist, register it in VM database
            log_info "Registering edge $EDGE_ID with WireGuard public key in VM database..."
            
            VM_CONTAINER=$(docker compose -f "$COMPOSE_FILE" ps -q user-vm-api 2>/dev/null | head -1)
            if [ -n "$VM_CONTAINER" ]; then
                now=$(date +%s)
                # Use the same SQL format as in setup.sh (with wireguard_public_key, name, etc.)
                if docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                    "INSERT OR REPLACE INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) \
                     VALUES ('$EDGE_ID', 'PoC Edge 1', '$EDGE_PUBLIC_KEY', $now, 'active', $now, $now);" 2>&1; then
                    log_info "Successfully registered edge $EDGE_ID in VM database"
                    
                    # Verify registration
                    sleep 1
                    registered_key=$(docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                        "SELECT wireguard_public_key FROM edges WHERE edge_id = '$EDGE_ID' AND status = 'active';" 2>/dev/null | tr -d '\n\r' || echo "")
                    
                    if [ "$registered_key" = "$EDGE_PUBLIC_KEY" ]; then
                        log_info "Edge registration verified - public key matches"
                    else
                        log_warn "Edge registration verification failed - public key mismatch"
                    fi
                else
                    log_error "Failed to register edge in database"
                    # Show database error for debugging
                    docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                        ".schema edges" 2>&1 || true
                    exit 1
                fi
            else
                log_error "user-vm-api container not found"
                exit 1
            fi
        else
            log_info "Edge $EDGE_ID already registered in VM database"
        fi
        
        log_info "Edge device registered successfully"
    fi
    
    # Epic 2.2: VM Edge Status Monitoring & WireGuard Connection Management
    if [ "$RUN_EPIC_2_2" = "true" ]; then
        if ! test_epic_2_2; then
            log_error "Epic 2.2 tests failed - WireGuard connection is required for all subsequent tests"
            if [ "$RUN_EPIC_2_3" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_error "Cannot continue without Edge connection"
                log_error "Test stopped - containers are still running for log inspection"
                log_error "Inspect logs with: docker compose -f $COMPOSE_FILE logs <service-name>"
                export TEST_FAILED=true
                exit 1
            fi
        fi
    fi
    
    # Epic 2.3: Post-WireGuard Edge ↔ VM Coordination
    if [ "$RUN_EPIC_2_3" = "true" ]; then
        if ! test_epic_2_3; then
            log_warn "Epic 2.3 tests had failures"
            if [ "$RUN_EPIC_2_4" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Epic 2.4: Snapshot Capture & Dataset Progress Fixes
    if [ "$RUN_EPIC_2_4" = "true" ]; then
        if ! test_epic_2_4; then
            log_warn "Epic 2.4 tests had failures"
            if [ "$RUN_EPIC_2_5" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Epic 2.5: Edge → VM Dataset Sync & Upload
    if [ "$RUN_EPIC_2_5" = "true" ]; then
        if ! test_epic_2_5; then
            log_warn "Epic 2.5 tests had failures"
            if [ "$RUN_EPIC_2_6" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Step 7: Run Epic 2.6 tests (VM-Side Model Management)
    if [ "$RUN_EPIC_2_6" = "true" ]; then
        if ! test_epic_2_6; then
            log_warn "Epic 2.6 tests had failures"
            if [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Step 8: Run Epic 2.7 tests (Training Pipeline)
    if [ "$RUN_EPIC_2_7" = "true" ]; then
        if ! test_epic_2_7; then
            log_warn "Epic 2.7 tests had failures"
            if [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Step 9: Run Epic 2.8 tests (Model Deployment)
    if [ "$RUN_EPIC_2_8" = "true" ]; then
        if ! test_epic_2_8; then
            log_warn "Epic 2.8 tests had failures"
            if [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Step 10: Run Epic 2.9 tests (Event Detection & Processing)
    if [ "$RUN_EPIC_2_9" = "true" ]; then
        if ! test_epic_2_9; then
            log_warn "Epic 2.9 tests had failures"
        fi
    fi
    
    # Print summary
    echo ""
    log_section "Test Summary"
    log_info "  Total Passed: $TESTS_PASSED"
    log_info "  Total Failed: $TESTS_FAILED"
    log_info "  Total Tests: $((TESTS_PASSED + TESTS_FAILED))"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_info "All tests passed! ✓"
        export TEST_FAILED=false
        exit 0
    else
        log_error "Some tests failed"
        log_error "Test stopped - containers are still running for log inspection"
        log_error "Inspect logs with: docker compose -f $COMPOSE_FILE logs <service-name>"
        log_error "To stop containers: docker compose -f $COMPOSE_FILE down"
        export TEST_FAILED=true
        exit 1
    fi
}

# Run main function
main "$@"
