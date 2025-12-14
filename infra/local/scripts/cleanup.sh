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
    
    log_info "Stopping any running docker-compose services..."
    docker compose -f "$COMPOSE_FILE" down 2>/dev/null || true
    
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
