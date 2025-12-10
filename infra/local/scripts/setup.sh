#!/bin/bash
# Setup helper functions

# Source dependencies (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"
source "${SCRIPTS_DIR}/test_helpers.sh"
source "${SCRIPTS_DIR}/polling.sh"
source "${SCRIPTS_DIR}/investigation.sh"

# SCRIPT_DIR, COMPOSE_FILE, VM_API, TRAINING_SERVICE must be set by the calling script

check_all_services_healthy() {
    curl -sf "${VM_API}/health" > /dev/null 2>&1 && \
    curl -sf "http://localhost:8182/health" > /dev/null 2>&1 && \
    curl -sf "${TRAINING_SERVICE}/health" > /dev/null 2>&1 && \
    curl -sf "http://localhost:9000/minio/health/live" > /dev/null 2>&1 && \
    curl -sf "http://localhost:8180/health" > /dev/null 2>&1
}

start_docker_compose() {
    log_section "Step 2: Starting docker-compose and waiting for services to be healthy"
    
    cd "$SCRIPT_DIR"
    
    # CRITICAL: Run wg-setup FIRST to generate config files before any service tries to mount them
    # Docker Compose creates directories if files don't exist when parsing volume mounts
    log_info "Running WireGuard setup first to generate config files..."
    docker compose -f "$COMPOSE_FILE" run --rm wg-setup
    
    # Verify config files were created (not directories)
    if [ ! -f "${SCRIPT_DIR}/wg/config/edge-wg0.conf" ] || [ ! -f "${SCRIPT_DIR}/wg/config/server-wg0.conf" ]; then
        test_failed "WireGuard config files not generated"
        return 1
    fi
    
    # Ensure they are files, not directories
    if [ ! -f "${SCRIPT_DIR}/wg/config/edge-wg0.conf" ] || [ -d "${SCRIPT_DIR}/wg/config/edge-wg0.conf" ]; then
        test_failed "edge-wg0.conf is not a file (may be a directory)"
        return 1
    fi
    if [ ! -f "${SCRIPT_DIR}/wg/config/server-wg0.conf" ] || [ -d "${SCRIPT_DIR}/wg/config/server-wg0.conf" ]; then
        test_failed "server-wg0.conf is not a file (may be a directory)"
        return 1
    fi
    
    log_info "WireGuard config files verified"
    
    # Read EDGE_ID from generated file
    EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
    if [ ! -f "$EDGE_ID_FILE" ]; then
        log_error "EDGE_ID file not found at $EDGE_ID_FILE"
        test_failed "EDGE_ID generation failed"
        return 1
    fi
    
    EDGE_ID=$(cat "$EDGE_ID_FILE")
    log_info "EDGE_ID generated: $EDGE_ID"
    
    # Export EDGE_ID for docker-compose
    export EDGE_ID
    
    log_info "Starting docker-compose services..."
    docker compose -f "$COMPOSE_FILE" up -d
    
    # Single polling loop to check all services simultaneously
    log_info "Waiting for all services to be healthy..."
    if poll_until_success "all services to be healthy" \
        "check_all_services_healthy" \
        120 3 \
        "investigate_service_health \"user-vm-api\" \"${VM_API}\"; \
         investigate_service_health \"edge-orchestrator\" \"http://localhost:8182\"; \
         investigate_service_health \"python-ai-service\" \"${TRAINING_SERVICE}\"; \
         investigate_service_health \"minio\" \"http://localhost:9000\"; \
         investigate_service_health \"edge-ai-service\" \"http://localhost:8180\""; then
        # All services are healthy - report each one
        test_passed "user-vm-api is healthy"
        test_passed "edge-orchestrator is healthy"
        test_passed "python-ai-service is healthy"
        test_passed "minio is healthy"
        test_passed "edge-ai-service is healthy"
        log_info "All services are healthy and ready for testing"
        
        # Register EDGE_ID in VM edge management system now that services are up
        # Read EDGE_ID from generated file
        EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
        if [ -f "$EDGE_ID_FILE" ]; then
            EDGE_ID=$(cat "$EDGE_ID_FILE")
            log_info "Registering EDGE_ID in VM edge management system..."
            
            # Read WireGuard public key for this edge
            EDGE_PUBLIC_KEY_FILE="${SCRIPT_DIR}/wg/keys/edge.public"
            if [ -f "$EDGE_PUBLIC_KEY_FILE" ]; then
                EDGE_PUBLIC_KEY=$(cat "$EDGE_PUBLIC_KEY_FILE" | tr -d '\n\r')
                
                # Check if edge already exists
                existing_edge=$(curl -sfL "${VM_API}/api/edges" 2>/dev/null | jq -r ".[] | select(.edge_id == \"$EDGE_ID\") | .edge_id" 2>/dev/null || echo "")
                if [ -z "$existing_edge" ]; then
                    # Edge doesn't exist, register it in VM database
                    # Note: In production, SaaS components would set this before edge deployment
                    # For local test, we register it here after services are up
                    log_info "Registering new edge: $EDGE_ID with WireGuard public key"
                    
                    # Register edge directly in VM database (using SQLite via docker exec)
                    # Get the user-vm-api container name
                    VM_CONTAINER=$(docker compose -f "$COMPOSE_FILE" ps -q user-vm-api 2>/dev/null | head -1)
                    if [ -n "$VM_CONTAINER" ]; then
                        # Register edge in database
                        now=$(date +%s)
                        if docker exec "$VM_CONTAINER" sqlite3 /app/data/view-guard.db \
                            "INSERT OR REPLACE INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) \
                             VALUES ('$EDGE_ID', 'PoC Edge 1', '$EDGE_PUBLIC_KEY', $now, 'active', $now, $now);" 2>/dev/null; then
                            log_info "Successfully registered edge $EDGE_ID in VM database"
                        else
                            log_warn "Failed to register edge in database, edge connection may fail validation"
                        fi
                    else
                        log_warn "VM API container not found, edge registration failed"
                    fi
                else
                    log_info "Edge $EDGE_ID already registered in VM"
                fi
            else
                log_warn "WireGuard public key file not found at $EDGE_PUBLIC_KEY_FILE"
            fi
        fi
        
        return 0
    else
        # Polling loop timed out - check which services are still unhealthy
        log_error "Timeout waiting for all services to be healthy. Checking individual service status..."
        
        local failed_services=()
        local healthy_services=()
        
        # Check each service and categorize
        if curl -sf "${VM_API}/health" > /dev/null 2>&1; then
            healthy_services+=("user-vm-api")
        else
            failed_services+=("user-vm-api")
        fi
        
        if curl -sf "http://localhost:8182/health" > /dev/null 2>&1; then
            healthy_services+=("edge-orchestrator")
        else
            failed_services+=("edge-orchestrator")
        fi
        
        if curl -sf "${TRAINING_SERVICE}/health" > /dev/null 2>&1; then
            healthy_services+=("python-ai-service")
        else
            failed_services+=("python-ai-service")
        fi
        
        if curl -sf "http://localhost:9000/minio/health/live" > /dev/null 2>&1; then
            healthy_services+=("minio")
        else
            failed_services+=("minio")
        fi
        
        if curl -sf "http://localhost:8180/health" > /dev/null 2>&1; then
            healthy_services+=("edge-ai-service")
        else
            failed_services+=("edge-ai-service")
        fi
        
        # Log summary
        if [ ${#healthy_services[@]} -gt 0 ]; then
            log_info "Healthy services: ${healthy_services[*]}"
        fi
        if [ ${#failed_services[@]} -gt 0 ]; then
            log_error "Unhealthy services: ${failed_services[*]}"
        fi
        
        # Report each failed service with investigation
        for service in "${failed_services[@]}"; do
            case "$service" in
                "user-vm-api")
                    test_failed "user-vm-api not healthy" "investigate_service_health \"user-vm-api\" \"${VM_API}\""
                    ;;
                "edge-orchestrator")
                    test_failed "edge-orchestrator not healthy" "investigate_service_health \"edge-orchestrator\" \"http://localhost:8182\""
                    ;;
                "python-ai-service")
                    test_failed "python-ai-service not healthy" "investigate_service_health \"python-ai-service\" \"${TRAINING_SERVICE}\""
                    ;;
                "minio")
                    test_failed "minio not healthy" "investigate_service_health \"minio\" \"http://localhost:9000\""
                    ;;
                "edge-ai-service")
                    test_failed "edge-ai-service not healthy" "investigate_service_health \"edge-ai-service\" \"http://localhost:8180\""
                    ;;
            esac
        done
        
        return 1
    fi
}
