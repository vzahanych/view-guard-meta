#!/bin/bash
# Investigation helper functions for test failures

# Source dependencies (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"
source "${SCRIPTS_DIR}/api_helpers.sh"

# COMPOSE_FILE and VM_API must be set by the calling script

investigate_service_health() {
    local service_name=$1
    local service_url=$2
    log_info "Investigating $service_name health..."
    
    # Check service health endpoint
    if curl -sf "${service_url}/health" > /dev/null 2>&1; then
        log_info "  ✓ $service_name health endpoint is accessible"
    else
        log_error "  ✗ $service_name health endpoint is not accessible"
        
        # Check container status
        if docker compose -f "$COMPOSE_FILE" ps "$service_name" 2>/dev/null | grep -q "Up"; then
            log_info "  → Container is running"
        else
            log_error "  → Container is not running"
            docker compose -f "$COMPOSE_FILE" ps "$service_name" 2>/dev/null | head -5
        fi
        
        # Show recent logs
        log_info "  Recent logs from $service_name:"
        docker compose -f "$COMPOSE_FILE" logs --tail 20 "$service_name" 2>/dev/null | sed 's/^/    /' || true
    fi
}

investigate_vm_https_server() {
    log_info "Investigating VM HTTPS server..."
    
    # Check if port 8443 is listening
    if docker compose -f "$COMPOSE_FILE" exec -T user-vm-api sh -c "netstat -tlnp 2>/dev/null | grep -q ':8443' || ss -tlnp 2>/dev/null | grep -q ':8443'" 2>/dev/null; then
        log_info "  ✓ Port 8443 is listening in user-vm-api container"
    else
        log_error "  ✗ Port 8443 is not listening in user-vm-api container"
    fi
    
    # Check VM API logs for HTTPS server
    log_info "  Recent user-vm-api logs (HTTPS server):"
    docker compose -f "$COMPOSE_FILE" logs --tail 30 user-vm-api 2>/dev/null | grep -E "(HTTPS|8443|https.*server|listening|error|ERROR)" | tail -20 | sed 's/^/    /' || true
}

investigate_edge_https_server() {
    log_info "Investigating Edge HTTPS server..."
    
    # Check if port 8443 is listening
    if docker compose -f "$COMPOSE_FILE" exec -T edge-orchestrator sh -c "netstat -tlnp 2>/dev/null | grep -q ':8443' || ss -tlnp 2>/dev/null | grep -q ':8443'" 2>/dev/null; then
        log_info "  ✓ Port 8443 is listening in edge-orchestrator container"
    else
        log_error "  ✗ Port 8443 is not listening in edge-orchestrator container"
    fi
    
    # Check Edge orchestrator logs for HTTPS server
    log_info "  Recent edge-orchestrator logs (HTTPS server):"
    docker compose -f "$COMPOSE_FILE" logs --tail 30 edge-orchestrator 2>/dev/null | grep -E "(HTTPS|8443|https.*server|listening|error|ERROR)" | tail -20 | sed 's/^/    /' || true
}

investigate_https_connection() {
    local edge_id="${1:-poc-edge-1}"
    log_info "Investigating HTTPS connection for Edge: $edge_id"
    
    # Check VM HTTPS server
    investigate_vm_https_server
    
    # Check Edge HTTPS server
    investigate_edge_https_server
    
    # Check WireGuard tunnel
    investigate_wireguard_tunnel "$edge_id"
}

investigate_edge_registration() {
    log_info "Investigating Edge registration..."
    
    # Check VM API edges endpoint
    edges_response=$(curl -sfL "${VM_API}/api/edges" 2>&1 || echo "FAILED")
    if [ "$edges_response" != "FAILED" ]; then
        log_info "  VM API edges endpoint response:"
        echo "$edges_response" | head -20 | sed 's/^/    /'
    else
        log_error "  ✗ VM API edges endpoint is not accessible"
    fi
    
    # Check edge orchestrator logs
    log_info "  Recent edge-orchestrator logs:"
    docker compose -f "$COMPOSE_FILE" logs --tail 30 edge-orchestrator 2>/dev/null | grep -E "(telemetry|heartbeat|HTTPS|WireGuard|register|error|ERROR)" | tail -20 | sed 's/^/    /' || true
    
    # Check user-vm-api logs for edge registration
    log_info "  Recent user-vm-api logs (edge-related):"
    docker compose -f "$COMPOSE_FILE" logs --tail 30 user-vm-api 2>/dev/null | grep -E "(edge|register|telemetry|heartbeat|error|ERROR)" | tail -20 | sed 's/^/    /' || true
}

investigate_wireguard_tunnel() {
    local edge_id=${1:-"poc-edge-1"}
    log_info "Investigating WireGuard tunnel for edge: $edge_id..."
    
    # Check edge status
    status_response=$(curl -sf "${VM_API}/api/edges/${edge_id}/status" || echo "FAILED")
    if [ "$status_response" != "FAILED" ]; then
        log_info "  Edge status response:"
        echo "$status_response" | head -30 | sed 's/^/    /'
    else
        log_error "  ✗ Edge status endpoint is not accessible"
    fi
    
    # Check WireGuard health
    health_response=$(curl -sf "${VM_API}/api/edges/${edge_id}/health" || echo "FAILED")
    if [ "$health_response" != "FAILED" ]; then
        log_info "  WireGuard health response:"
        echo "$health_response" | head -20 | sed 's/^/    /'
    fi
    
    # Check WireGuard interface on VM
    log_info "  WireGuard interface status on VM:"
    docker compose -f "$COMPOSE_FILE" exec -T user-vm-api wg show 2>/dev/null | sed 's/^/    /' || log_warn "    Could not check WireGuard interface"
    
    # Check WireGuard interface on Edge
    log_info "  WireGuard interface status on Edge:"
    docker compose -f "$COMPOSE_FILE" exec -T edge-orchestrator wg show 2>/dev/null | sed 's/^/    /' || log_warn "    Could not check WireGuard interface"
}

investigate_grpc_connection() {
    local edge_id=${1:-"poc-edge-1"}
    log_info "Investigating gRPC connection for edge: $edge_id..."
    
    # Check edge status for gRPC info
    status_response=$(curl -sf "${VM_API}/api/edges/${edge_id}/status" || echo "FAILED")
    if [ "$status_response" != "FAILED" ]; then
        if command -v jq >/dev/null 2>&1; then
            grpc_connected=$(echo "$status_response" | jq -r '.grpc_connection.connected // false' 2>/dev/null || echo "false")
            last_heartbeat=$(echo "$status_response" | jq -r '.grpc_connection.last_heartbeat // empty' 2>/dev/null || echo "")
            log_info "  gRPC connected: $grpc_connected"
            log_info "  Last heartbeat: $last_heartbeat"
        fi
    fi
    
    # Check edge orchestrator logs for gRPC
    log_info "  Recent gRPC-related logs from edge-orchestrator:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 edge-orchestrator 2>/dev/null | grep -E "(grpc|gRPC|telemetry|heartbeat|error|ERROR)" | tail -15 | sed 's/^/    /' || true
}

investigate_capability_sync() {
    local edge_id=${1:-"poc-edge-1"}
    log_info "Investigating capability sync for edge: $edge_id..."
    
    # Check cameras in VM API
    cameras_response=$(curl -sfL "${VM_API}/api/cameras?edge_id=${edge_id}" 2>&1 || echo "FAILED")
    if [ "$cameras_response" != "FAILED" ]; then
        if command -v jq >/dev/null 2>&1; then
            camera_count=$(echo "$cameras_response" | jq '.cameras | length' 2>/dev/null || echo "0")
        else
            camera_count=$(echo "$cameras_response" | grep -o '"id"' | wc -l || echo "0")
        fi
        log_info "  Cameras in VM API: $camera_count"
        echo "$cameras_response" | head -30 | sed 's/^/    /'
    else
        log_error "  ✗ VM API cameras endpoint is not accessible"
    fi
    
    # Check edge orchestrator logs for capability sync
    log_info "  Recent capability sync logs from edge-orchestrator:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 edge-orchestrator 2>/dev/null | grep -E "(capability|sync|camera|error|ERROR)" | tail -15 | sed 's/^/    /' || true
}

investigate_dataset_sync() {
    local camera_id=${1:-""}
    local edge_id=${2:-"poc-edge-1"}
    log_info "Investigating dataset sync for camera: $camera_id, edge: $edge_id..."
    
    # Check camera dataset status
    if [ -n "$camera_id" ]; then
        dataset_response=$(curl -sfL "${VM_API}/api/cameras/${camera_id}/dataset?edge_id=${edge_id}" 2>&1 || echo "FAILED")
        if [ "$dataset_response" != "FAILED" ]; then
            log_info "  Camera dataset status:"
            echo "$dataset_response" | head -30 | sed 's/^/    /'
        fi
    fi
    
    # Check datasets in filesystem
    log_info "  Datasets in python-ai-service container:"
    docker compose -f "$COMPOSE_FILE" exec -T python-ai-service find /app/data/datasets -type d -maxdepth 3 2>/dev/null | head -20 | sed 's/^/    /' || log_warn "    Could not list datasets"
    
    # Check edge orchestrator logs for dataset sync
    log_info "  Recent dataset sync logs from edge-orchestrator:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 edge-orchestrator 2>/dev/null | grep -E "(dataset|sync|upload|error|ERROR)" | tail -15 | sed 's/^/    /' || true
}

investigate_baseline_model() {
    log_info "Investigating baseline model availability..."
    
    # Check baseline models API
    baseline_response=$(curl -sf "${VM_API}/api/models/baseline" || echo "FAILED")
    if [ "$baseline_response" != "FAILED" ]; then
        log_info "  Baseline models API response:"
        echo "$baseline_response" | head -30 | sed 's/^/    /'
    else
        log_error "  ✗ Baseline models API is not accessible"
    fi
    
    # Check model filesystem
    log_info "  Models in python-ai-service container:"
    docker compose -f "$COMPOSE_FILE" exec -T python-ai-service ls -la /app/data/models/ 2>/dev/null | sed 's/^/    /' || log_warn "    Could not list models"
    docker compose -f "$COMPOSE_FILE" exec -T python-ai-service find /app/data/models -name "metadata.json" 2>/dev/null | sed 's/^/    /' || log_warn "    Could not find metadata files"
    
    # Check baseline-setup logs
    log_info "  Baseline model setup logs:"
    docker compose -f "$COMPOSE_FILE" logs baseline-model-setup 2>/dev/null | tail -30 | sed 's/^/    /' || true
}

investigate_training_job() {
    local job_id=${1:-""}
    log_info "Investigating training job: $job_id..."
    
    if [ -z "$job_id" ]; then
        log_warn "  No job ID provided"
        return
    fi
    
    # Check training job status
    job_response=$(curl -sf "${VM_API}/api/training/${job_id}" || echo "FAILED")
    if [ "$job_response" != "FAILED" ]; then
        log_info "  Training job response:"
        echo "$job_response" | head -50 | sed 's/^/    /'
    else
        log_error "  ✗ Training job API is not accessible"
    fi
    
    # Check python-ai-service logs
    log_info "  Recent training logs from python-ai-service:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 python-ai-service 2>/dev/null | grep -E "(training|job|${job_id}|error|ERROR)" | tail -20 | sed 's/^/    /' || true
    
    # Check user-vm-api logs for training
    log_info "  Recent training logs from user-vm-api:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 user-vm-api 2>/dev/null | grep -E "(training|job|${job_id}|error|ERROR)" | tail -20 | sed 's/^/    /' || true
}

investigate_model_deployment() {
    local deployment_id=${1:-""}
    local edge_id=${2:-"poc-edge-1"}
    log_info "Investigating model deployment: $deployment_id..."
    
    if [ -z "$deployment_id" ]; then
        log_warn "  No deployment ID provided"
        return
    fi
    
    # Check deployment status
    deployment_response=$(curl -sf "${VM_API}/api/deployments/${deployment_id}" || echo "FAILED")
    if [ "$deployment_response" != "FAILED" ]; then
        log_info "  Deployment response:"
        echo "$deployment_response" | head -50 | sed 's/^/    /'
    else
        log_error "  ✗ Deployment API is not accessible"
    fi
    
    # Check edge orchestrator logs for deployment
    log_info "  Recent deployment logs from edge-orchestrator:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 edge-orchestrator 2>/dev/null | grep -E "(deployment|model|transfer|error|ERROR)" | tail -20 | sed 's/^/    /' || true
    
    # Check user-vm-api logs for deployment
    log_info "  Recent deployment logs from user-vm-api:"
    docker compose -f "$COMPOSE_FILE" logs --tail 50 user-vm-api 2>/dev/null | grep -E "(deployment|transfer|error|ERROR)" | tail -20 | sed 's/^/    /' || true
}

investigate_trained_model() {
    log_info "Investigating trained model availability..."
    
    # Check model catalog for trained models
    log_info "  Checking model catalog for trained models:"
    models_response=$(curl -sf "${VM_API}/api/models" || echo "FAILED")
    if [ "$models_response" != "FAILED" ]; then
        if command -v jq >/dev/null 2>&1; then
            trained_count=$(echo "$models_response" | jq '[.[] | select(.status != "baseline")] | length' 2>/dev/null || echo "0")
            trained_models=$(echo "$models_response" | jq -r '.[] | select(.status != "baseline") | .model_id' 2>/dev/null || echo "")
        else
            trained_count=$(echo "$models_response" | grep -v "baseline" | grep -o '"model_id"' | wc -l || echo "0")
            trained_models=$(echo "$models_response" | grep -v "baseline" | grep -o '"model_id":"[^"]*"' | cut -d'"' -f4 || echo "")
        fi
        log_info "    Trained models found: $trained_count"
        if [ -n "$trained_models" ]; then
            echo "$trained_models" | sed 's/^/      /'
        fi
    fi
    
    # Check training jobs
    log_info "  Checking training jobs:"
    training_response=$(curl -sf "${VM_API}/api/training?limit=10" || echo "FAILED")
    if [ "$training_response" != "FAILED" ]; then
        if command -v jq >/dev/null 2>&1; then
            jobs=$(echo "$training_response" | jq 'if type == "array" then . else .jobs? // [] end | length' 2>/dev/null || echo "0")
            completed_jobs=$(echo "$training_response" | jq -r 'if type == "array" then . else .jobs? // [] end | .[] | select(.status == "completed") | .job_id' 2>/dev/null || echo "")
        else
            jobs=$(echo "$training_response" | grep -o '"job_id"' | wc -l || echo "0")
            completed_jobs=$(echo "$training_response" | grep -A 5 '"status":"completed"' | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4 || echo "")
        fi
        log_info "    Total jobs: $jobs"
        if [ -n "$completed_jobs" ]; then
            log_info "    Completed job IDs:"
            echo "$completed_jobs" | sed 's/^/      /'
        fi
    fi
    
    # Check python-ai-service for trained models
    log_info "  Trained models in filesystem:"
    docker compose -f "$COMPOSE_FILE" exec -T python-ai-service find /app/data/models/trained -type d -maxdepth 1 2>/dev/null | sed 's/^/    /' || log_warn "    Could not list trained models"
}

investigate_general_system() {
    log_info "Investigating general system state..."
    
    # Check all service containers
    log_info "  Container status:"
    docker compose -f "$COMPOSE_FILE" ps 2>/dev/null | sed 's/^/    /' || true
    
    # Check for recent errors in all services
    log_info "  Recent errors across all services:"
    for service in user-vm-api edge-orchestrator python-ai-service; do
        errors=$(docker compose -f "$COMPOSE_FILE" logs --tail 100 "$service" 2>/dev/null | grep -i "error\|failed\|fatal" | tail -5 || true)
        if [ -n "$errors" ]; then
            log_warn "    $service errors:"
            echo "$errors" | sed 's/^/      /'
        fi
    done
}
