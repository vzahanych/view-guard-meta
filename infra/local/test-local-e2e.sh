#!/bin/bash
# Local End-to-End Integration Test
# Tests the complete workflow: WireGuard Connection (Epic 2.2) → Training (Epic 2.7) → Deployment (Epic 2.8) → Event Processing (Epic 2.9)
#
# This script runs OUTSIDE of docker-compose and manages the entire test environment lifecycle:
# 1. Cleans old data from previous runs
# 2. Starts docker-compose and waits for all services to be healthy
# 3. Runs Epic 2.2: VM Edge Status Monitoring & WireGuard Connection Management
# 4. Runs Epic 2.7: Model Training Pipeline
# 5. Runs Epic 2.8: VM → Edge Trained Model Sync & Deployment
# 6. Runs Epic 2.9: Edge-Side Event Detection & Processing
# 7. Cleans up on exit (optional, controlled by CLEANUP_ON_EXIT env var)
#
# Usage:
#   ./test-local-e2e.sh                    # Run all tests (default)
#   ./test-local-e2e.sh --all              # Run all tests
#   ./test-local-e2e.sh --epic 2.2         # Run only Epic 2.2
#   ./test-local-e2e.sh --epic 2.7         # Run only Epic 2.7
#   ./test-local-e2e.sh --epic 2.8         # Run only Epic 2.8
#   ./test-local-e2e.sh --epic 2.9         # Run only Epic 2.9
#   ./test-local-e2e.sh --epic 2.2 --epic 2.7  # Run multiple epics
#   ./test-local-e2e.sh --skip-cleanup     # Skip cleanup step
#   ./test-local-e2e.sh --skip-start       # Skip docker-compose start (assumes already running)
#   ./test-local-e2e.sh --help             # Show help message

set -euo pipefail

# Get script directory (infra/local)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"

# Configuration - always use localhost since we run outside containers
VM_API="${VM_API:-http://localhost:8280}"
EDGE_API="${EDGE_API:-http://localhost:8081}"
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

# Test execution flags
RUN_EPIC_2_2=false
RUN_EPIC_2_3=false
RUN_EPIC_2_4=false
RUN_EPIC_2_5=false
RUN_EPIC_2_6=false
RUN_EPIC_2_7=false
RUN_EPIC_2_8=false
RUN_EPIC_2_9=false
SKIP_CLEANUP=false
SKIP_START=false
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
# Step 1: Clean old data from previous runs
# ============================================================================

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
    # 3. gRPC connection is established through WireGuard tunnel (application layer)
    # 4. Edge registration/validation happens via gRPC telemetry
    # 5. Connection state and monitoring
    
    test_edge_accessible "$edge_id" || return 1
    test_wireguard_tunnel "$edge_id" || return 1
    test_wireguard_functionality "$edge_id" || return 1
    test_grpc_connection "$edge_id" || return 1
    test_edge_registered "$edge_id" || return 1
    local connection_state=$(test_edge_connection_status "$edge_id") || return 1
    test_connection_state "$edge_id" "connected" || return 1
    test_connection_keepalive "$edge_id" || return 1
    test_wireguard_health "$edge_id" || return 1
    
    log_info "Epic 2.2: $EPIC_TESTS tests completed"
    log_info "✓ Edge is registered and accessible"
    log_info "✓ WireGuard tunnel is established and functional"
    log_info "✓ Connection monitoring and keepalive are working"
    
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
    
    # Get camera ID from Edge API
    local camera_id=$(test_get_camera_id "$edge_api_url") || return 1
    
    # Test 1: Verify initial dataset status
    local initial_labeled_count=$(test_initial_dataset_status "$camera_id" "$edge_api_url") || return 1
    
    # Test 2: Capture and save a snapshot
    local save_response=$(test_capture_and_save_snapshot "$camera_id" "$edge_api_url") || return 1
    
    # Test 3: Verify dataset status in save response
    test_dataset_status_in_save_response "$save_response" "$initial_labeled_count" || return 1
    
    # Test 4: Verify dataset status updated in cameras API
    test_dataset_status_updated_in_cameras_api "$camera_id" "$initial_labeled_count" "$edge_api_url" || return 1
    
    # Test 5: Test multiple snapshot capture
    test_multiple_snapshot_capture "$camera_id" 10 "$edge_api_url" || return 1
    
    # Test 6: Test dataset status refresh endpoint
    test_dataset_status_refresh "$camera_id" "$edge_api_url" || return 1
    
    # Test 7: Verify real-time progress updates
    test_realtime_progress_updates "$camera_id" "$initial_labeled_count" "$edge_api_url" || return 1
    
    log_info "Epic 2.4: $EPIC_TESTS tests completed"
    log_info "✓ Snapshot capture working"
    log_info "✓ Dataset status updates after saving"
    log_info "✓ Multiple snapshot capture supported"
    log_info "✓ Dataset progress tracking functional"
    
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
    
    # Get camera ID from Edge API (reuse function from epic_2_4_tests.sh)
    local camera_id=$(test_get_camera_id "$edge_api_url") || return 1
    
    # Test 1: Verify dataset readiness
    local readiness_info=$(test_verify_dataset_readiness "$camera_id" "$edge_api_url") || return 1
    local labeled_count=$(echo "$readiness_info" | cut -d'|' -f1)
    local required_count=$(echo "$readiness_info" | cut -d'|' -f2)
    local needs_more=$(echo "$readiness_info" | cut -d'|' -f3)
    
    # Capture additional snapshots if needed
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
    local trained_model_id=$(test_find_trained_model) || return 1
    
    # Test 2: Verify Edge is connected
    test_verify_edge_connected "$edge_id" || return 1
    
    # Test 3: Trigger model deployment
    local deployment_id=$(test_trigger_model_deployment "$trained_model_id" "$edge_id") || return 1
    
    if [ -z "$deployment_id" ]; then
        log_warn "No deployment_id returned, but deployment may have been triggered" >&2
        # Try to get it from file
        if [ -f /tmp/test_deployment_id.txt ]; then
            deployment_id=$(cat /tmp/test_deployment_id.txt)
        fi
    fi
    
    if [ -z "$deployment_id" ]; then
        test_failed "Could not get deployment_id" >&2
        return 1
    fi
    
    # Test 4: Wait for deployment completion
    local deployment_status=$(test_wait_for_deployment_completion "$deployment_id") || return 1
    
    # Test 5: Verify Edge received model
    test_verify_edge_received_model "$deployment_id" || return 1
    
    # Test 6: Verify Edge model activation
    test_verify_edge_model_activation "$deployment_id" || return 1
    
    # Test 7: Verify Edge status reporting
    test_verify_edge_status_reporting "$deployment_id" || return 1
    
    log_info "Epic 2.8: $EPIC_TESTS tests completed"
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
    
    # Show which epics will be run
    echo ""
    log_info "Epics to run (with prerequisites):"
    local epics=(
        "2.2:VM Edge Status Monitoring & WireGuard Connection Management"
        "2.3:Post-WireGuard Edge ↔ VM Coordination (requires 2.2)"
        "2.4:Snapshot Capture & Dataset Progress Fixes (requires 2.2, 2.3)"
        "2.5:Edge → VM Dataset Sync & Upload (requires 2.2, 2.3, 2.4)"
        "2.6:VM-Side Model Management for Training Readiness (requires 2.2, 2.3, 2.4, 2.5)"
        "2.7:Model Training Pipeline (requires 2.2, 2.3, 2.4, 2.5, 2.6)"
        "2.8:VM → Edge Trained Model Sync & Deployment (requires 2.2, 2.3, 2.4, 2.5, 2.6, 2.7)"
        "2.9:Edge-Side Event Detection & Processing (requires 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8)"
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
    
    # Step 1: Clean old data (unless skipped)
    if [ "$SKIP_CLEANUP" = "false" ]; then
        if ! cleanup_old_data; then
            log_error "Failed to clean old data"
            exit 1
        fi
    else
        log_info "Skipping cleanup step (--skip-cleanup)"
    fi
    
    # Step 2: Start docker-compose and wait for services (unless skipped)
    if [ "$SKIP_START" = "false" ]; then
        if ! start_docker_compose; then
            log_error "Failed to start docker-compose or services not healthy"
            exit 1
        fi
    else
        log_info "Skipping docker-compose start (--skip-start)"
        log_info "Verifying that services are already running"
        if ! check_all_services_healthy; then
            log_error "Services are not healthy. Please start them or remove --skip-start flag"
            exit 1
        fi
        log_info "All services are verified healthy"
    fi
    
    # Step 3: Run Epic 2.2 tests (WireGuard Connection & Edge Status Monitoring)
    if [ "$RUN_EPIC_2_2" = "true" ]; then
        if ! test_epic_2_2; then
            log_error "Epic 2.2 tests failed - WireGuard connection is required for all subsequent tests"
            if [ "$RUN_EPIC_2_3" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_error "Cannot continue without Edge connection"
                exit 1
            fi
        fi
    fi
    
    # Step 4: Run Epic 2.3 tests (Post-WireGuard Edge ↔ VM Coordination)
    if [ "$RUN_EPIC_2_3" = "true" ]; then
        if ! test_epic_2_3; then
            log_warn "Epic 2.3 tests had failures"
            if [ "$RUN_EPIC_2_4" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Step 5: Run Epic 2.4 tests (Snapshot Capture & Dataset Progress Fixes)
    if [ "$RUN_EPIC_2_4" = "true" ]; then
        if ! test_epic_2_4; then
            log_warn "Epic 2.4 tests had failures"
            if [ "$RUN_EPIC_2_5" = "true" ] || [ "$RUN_EPIC_2_7" = "true" ] || [ "$RUN_EPIC_2_8" = "true" ] || [ "$RUN_EPIC_2_9" = "true" ]; then
                log_warn "Continuing to next epic..."
            fi
        fi
    fi
    
    # Step 6: Run Epic 2.5 tests (Edge → VM Dataset Sync & Upload)
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
        exit 0
    else
        log_error "Some tests failed"
        exit 1
    fi
}

# Run main function
main "$@"
