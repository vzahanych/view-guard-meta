#!/bin/bash
# Cleanup helper functions

# Source dependencies (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"
source "${SCRIPTS_DIR}/test_helpers.sh"

# SCRIPT_DIR and COMPOSE_FILE must be set by the calling script

cleanup() {
    if [ "$CLEANUP_DONE" = "true" ]; then
        return 0
    fi
    
    CLEANUP_DONE=true
    
    # Check if test failed - if so, keep containers running for log inspection
    if [ "${TEST_FAILED:-false}" = "true" ]; then
        log_section "Test failed - keeping containers running for log inspection"
        log_info "Containers are still running. You can inspect logs with:"
        log_info "  docker compose -f $COMPOSE_FILE logs <service-name>"
        log_info "  docker compose -f $COMPOSE_FILE ps"
        log_info "To stop containers manually: docker compose -f $COMPOSE_FILE down"
        log_info "Cleanup skipped (containers kept running for debugging)"
        return 0
    fi
    
    if [ "$CLEANUP_ON_EXIT" = "true" ]; then
        log_section "Cleaning up test environment"
        log_info "Stopping docker-compose services..."
        cd "$SCRIPT_DIR"
        docker compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true
        
        # Clean up temporary test files
        rm -f /tmp/test_job_id.txt /tmp/test_model_id.txt /tmp/test_deployment_id.txt /tmp/test_edge_id.txt
        
        log_info "Cleanup complete"
    else
        log_info "Cleanup skipped (CLEANUP_ON_EXIT=false)"
    fi
}

cleanup_old_data() {
    log_section "Step 1: Cleaning old data from previous runs"
    
    cd "$SCRIPT_DIR"
    
    # Check if containers are already running
    RUNNING_CONTAINERS=$(docker compose -f "$COMPOSE_FILE" ps -q 2>/dev/null | wc -l || echo "0")
    RUNNING_CONTAINERS=${RUNNING_CONTAINERS:-0}
    
    if [ "$RUNNING_CONTAINERS" -gt 0 ] && [ "${REBUILD:-false}" != "true" ]; then
        log_info "Containers are already running ($RUNNING_CONTAINERS containers detected)"
        log_info "Restarting containers to ensure clean state (rebuilding not required)..."
        docker compose -f "$COMPOSE_FILE" restart 2>/dev/null || {
            log_warn "Restart failed, stopping and starting containers..."
            docker compose -f "$COMPOSE_FILE" down 2>/dev/null || true
        }
    else
        log_info "Stopping any running docker-compose services..."
        docker compose -f "$COMPOSE_FILE" down 2>/dev/null || true
    fi
    
    # Check if volumes exist - only clean keys if we're actually removing volumes
    # Keys should persist across test runs unless volumes are cleaned
    VOLUMES_EXIST=$(docker volume ls -q 2>/dev/null | grep -E "(edge-data|ai-models|ai-data|user-vm-data|user-vm-datasets|user-vm-models|minio-data|baseline-models)" | wc -l)
    VOLUMES_EXIST=${VOLUMES_EXIST:-0}
    VOLUMES_EXIST=$((VOLUMES_EXIST + 0))
    
    if [ "$VOLUMES_EXIST" -gt 0 ]; then
        log_info "Volumes detected, but not removing them (keys will be preserved)"
        log_info "Note: Keys persist across test runs. To regenerate keys, manually remove volumes first"
    else
        log_info "No volumes detected, keys will be preserved"
    fi
    
    # Never remove keys during normal cleanup - only remove when volumes are explicitly cleaned
    # This ensures keys persist across test runs, preventing authentication issues
    
    # Ensure directories exist
    mkdir -p "${SCRIPT_DIR}/wg/keys" "${SCRIPT_DIR}/wg/config"
    
    log_info "Removing temporary test files..."
    rm -f /tmp/test_job_id.txt /tmp/test_model_id.txt /tmp/test_deployment_id.txt /tmp/test_edge_id.txt
    
    log_info "Old data cleanup complete"
    test_passed "Old data cleaned from previous runs"
}
