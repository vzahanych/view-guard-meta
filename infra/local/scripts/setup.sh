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
    log_section "Completing environment setup (edge registration and Edge services)"
    
    cd "$SCRIPT_DIR"
    
    # Rebuild images if requested
    if [ "${REBUILD:-false}" = "true" ]; then
        log_info "Rebuilding Docker images (--rebuild flag set)..."
        docker compose -f "$COMPOSE_FILE" build --no-cache
        if [ $? -ne 0 ]; then
            test_failed "Failed to rebuild Docker images"
            return 1
        fi
        log_info "Docker images rebuilt successfully"
    fi
    
    # Note: Epic 2.0 (config generation) and Epic 2.1 (VM services) should already be completed
    # This function only handles steps 3 and 4: edge registration and Edge services
    
    # Read EDGE_ID from generated file (should exist from Epic 2.0)
    EDGE_ID_FILE="${SCRIPT_DIR}/wg/keys/edge-id"
    if [ ! -f "$EDGE_ID_FILE" ]; then
        log_error "EDGE_ID file not found at $EDGE_ID_FILE"
        log_error "Epic 2.0 (configuration generation) must be completed first"
        test_failed "EDGE_ID not available"
        return 1
    fi
    
    EDGE_ID=$(cat "$EDGE_ID_FILE")
    log_info "Using EDGE_ID: $EDGE_ID (from Epic 2.0)"
    
    # Export EDGE_ID for docker-compose
    export EDGE_ID
    
    # ============================================================
    # SaaS Architecture Simulation: Sequential Service Startup
    # ============================================================
    # Step 2.0: Generate config (wg-setup) - DONE above via start-local-env.sh
    # Step 2.1: Start VM services (independent, central part) - DONE above via start-local-env.sh
    # Step 3: Register edge in VM (SaaS simulation) - NEXT
    # Step 4: Start Edge services (edge faces fully functional VM) - AFTER step 3
    # ============================================================
    
    # VM services are now fully initialized (database, MinIO, training service all verified via enhanced health check)
    # Proceed with edge registration
    
    # Register EDGE_ID in VM edge management system (SaaS simulation)
    # This simulates SaaS registering edges in VM after customer configures them in SaaS UI
    # VM now has knowledge of which edges will connect
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
                # Edge doesn't exist, register it in VM database (SaaS simulation)
                # In production: SaaS components register edges in VM edge management system
                #   when customer configures edges in SaaS UI, before VM starts
                # In local test: We simulate this by registering after VM starts but before edge starts
                log_info "Simulating SaaS registration: Registering edge $EDGE_ID with WireGuard public key in VM edge management system"
                
                # Register edge directly in VM database (using SQLite via docker exec)
                # Get the user-vm-api container name
                VM_CONTAINER=$(docker compose -f "$COMPOSE_FILE" ps -q user-vm-api 2>/dev/null | head -1)
                if [ -n "$VM_CONTAINER" ]; then
                    # Wait for database to be initialized (edges table must exist)
                    log_info "Waiting for VM database to be initialized..."
                    local db_ready=false
                    for i in {1..30}; do
                        if docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                            "SELECT name FROM sqlite_master WHERE type='table' AND name='edges';" 2>/dev/null | grep -q "edges"; then
                            db_ready=true
                            log_info "Database initialized - edges table exists"
                            break
                        fi
                        sleep 1
                    done
                    
                    if [ "$db_ready" != "true" ]; then
                        log_error "Database not initialized - edges table does not exist after 30 seconds"
                        log_error "VM API may not have started properly or database initialization failed"
                        return 1
                    fi
                    
                    # Register edge in database (database path from config: /app/data/events.db)
                    now=$(date +%s)
                    log_info "Registering edge with public key: ${EDGE_PUBLIC_KEY:0:20}..."
                    if docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                        "INSERT OR REPLACE INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) \
                         VALUES ('$EDGE_ID', 'PoC Edge 1', '$EDGE_PUBLIC_KEY', $now, 'active', $now, $now);" 2>&1; then
                        log_info "Successfully registered edge $EDGE_ID in VM database"
                        
                        # Verify registration by querying the database
                        log_info "Verifying edge registration..."
                        sleep 1  # Small delay to ensure database write is committed
                        registered_key=$(docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                            "SELECT wireguard_public_key FROM edges WHERE edge_id = '$EDGE_ID' AND status = 'active';" 2>/dev/null | tr -d '\n\r' || echo "")
                        
                        if [ "$registered_key" = "$EDGE_PUBLIC_KEY" ]; then
                            log_info "Edge registration verified - public key matches"
                        else
                            log_warn "Edge registration verification failed - public key mismatch"
                            log_warn "Expected: $EDGE_PUBLIC_KEY"
                            log_warn "Found: $registered_key"
                        fi
                    else
                        log_error "Failed to register edge in database, edge connection may fail validation"
                        log_error "Attempted database path: /app/data/events.db"
                        log_error "You may need to check if the database file exists and the edges table is created"
                        # Show database error for debugging
                        docker exec "$VM_CONTAINER" sqlite3 /app/data/events.db \
                            "SELECT name FROM sqlite_master WHERE type='table' AND name='edges';" 2>&1 || true
                    fi
                else
                    log_error "VM API container not found, edge registration failed"
                fi
            else
                log_info "Edge $EDGE_ID already registered in VM"
            fi
        else
            log_warn "WireGuard public key file not found at $EDGE_PUBLIC_KEY_FILE"
        fi
    else
        log_warn "EDGE_ID file not found at $EDGE_ID_FILE"
    fi
    
    # ============================================================
    # Step 4: Start Edge services (edge faces fully functional VM)
    # ============================================================
    log_section "Step 4: Starting Edge services sequentially (edge faces fully functional VM with edge already registered)"
    log_info "VM is fully functional and edge is registered. Starting Edge services one by one..."
    
    # Small delay to ensure database registration is fully committed and visible
    # This prevents race conditions where Edge tries to connect before registration is visible
    log_info "Waiting 2 seconds to ensure edge registration is committed..."
    sleep 2
    
    # 4.1: Start Edge AI Service first (dependency for edge-orchestrator)
    log_info "Starting Edge AI Service..."
    if ! docker compose -f "$COMPOSE_FILE" up -d edge-ai-service; then
        test_failed "Failed to start Edge AI Service"
        return 1
    fi
    log_info "Waiting for Edge AI Service to be healthy..."
    if poll_until_success "Edge AI Service to be healthy" \
        "curl -sf \"http://localhost:8180/health\" > /dev/null 2>&1" \
        120 3 \
        "investigate_service_health \"edge-ai-service\" \"http://localhost:8180\""; then
        test_passed "Edge AI Service is healthy"
    else
        test_failed "Edge AI Service failed to become healthy"
        return 1
    fi
    
    # 4.2: Start Edge Orchestrator (main Edge service, depends on edge-ai-service)
    log_info "Starting Edge Orchestrator service..."
    if ! docker compose -f "$COMPOSE_FILE" up -d edge-orchestrator; then
        test_failed "Failed to start Edge Orchestrator service"
        return 1
    fi
    log_info "Waiting for Edge Orchestrator to be healthy..."
    if poll_until_success "Edge Orchestrator to be healthy" \
        "curl -sf \"http://localhost:8182/health\" > /dev/null 2>&1" \
        120 3 \
        "investigate_service_health \"edge-orchestrator\" \"http://localhost:8182\""; then
        test_passed "Edge Orchestrator is healthy"
    else
        test_failed "Edge Orchestrator failed to become healthy"
        return 1
    fi
    
    # Verify all services (VM + Edge) are fully functional
    log_info "Verifying all services (VM + Edge) are fully functional..."
    if poll_until_success "All services to be fully functional" \
        "check_all_services_healthy" \
        30 2 \
        "investigate_service_health \"user-vm-api\" \"${VM_API}\"; \
         investigate_service_health \"edge-orchestrator\" \"http://localhost:8182\"; \
         investigate_service_health \"python-ai-service\" \"${TRAINING_SERVICE}\"; \
         investigate_service_health \"minio\" \"http://localhost:9000\"; \
         investigate_service_health \"edge-ai-service\" \"http://localhost:8180\""; then
        log_info "All services are healthy and ready for testing"
        log_info "✓ Edge services started successfully facing fully functional VM with edge already registered"
        
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
