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
    docker compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true
    
    log_info "Removing docker volumes (if any remain)..."
    # Remove volumes by name pattern (docker-compose creates volumes with project prefix)
    # The -v flag in docker compose down should handle most, but we clean up any remaining
    VOLUMES=$(docker volume ls -q | grep -E "(edge-data|ai-models|ai-data|user-vm-data|user-vm-datasets|user-vm-models|minio-data)" || true)
    if [ -n "$VOLUMES" ]; then
        echo "$VOLUMES" | xargs docker volume rm 2>/dev/null || true
    fi
    
    log_info "Cleaning WireGuard configuration and keys (will be regenerated)..."
    # Remove config files (will be regenerated from keys)
    rm -rf "${SCRIPT_DIR}/wg/config"/*.conf 2>/dev/null || true
    rm -rf "${SCRIPT_DIR}/wg/config/edge-wg0.conf" 2>/dev/null || true
    rm -rf "${SCRIPT_DIR}/wg/config/server-wg0.conf" 2>/dev/null || true
    # Remove keys directory to force regeneration (ensures fresh keys on clean start)
    rm -rf "${SCRIPT_DIR}/wg/keys"/* 2>/dev/null || true
    # Ensure directories exist
    mkdir -p "${SCRIPT_DIR}/wg/keys" "${SCRIPT_DIR}/wg/config"
    
    log_info "Removing temporary test files..."
    rm -f /tmp/test_job_id.txt /tmp/test_model_id.txt /tmp/test_deployment_id.txt /tmp/test_edge_id.txt
    
    log_info "Old data cleanup complete"
    test_passed "Old data cleaned from previous runs"
}
