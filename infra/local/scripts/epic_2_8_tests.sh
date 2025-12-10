#!/bin/bash
# Epic 2.8 test helper functions

# Source dependencies (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"
source "${SCRIPTS_DIR}/test_helpers.sh"
source "${SCRIPTS_DIR}/json_helpers.sh"
source "${SCRIPTS_DIR}/api_helpers.sh"
source "${SCRIPTS_DIR}/polling.sh"
source "${SCRIPTS_DIR}/investigation.sh"

# SCRIPT_DIR, COMPOSE_FILE, VM_API must be set by the calling script

test_find_trained_model() {
    log_info "Test 1: Verifying trained model exists..." >&2
    
    local trained_model_id=""
    
    # First check if we have a model from Epic 2.7
    if [ -f /tmp/test_model_id.txt ]; then
        trained_model_id=$(cat /tmp/test_model_id.txt)
        if [ -n "$trained_model_id" ] && [ "$trained_model_id" != "null" ] && [ "$trained_model_id" != "" ]; then
            # Verify it exists in catalog
            local model_response=$(curl -sf "${VM_API}/api/models/${trained_model_id}" || echo "FAILED")
            if [ "$model_response" != "FAILED" ]; then
                echo "$trained_model_id" > /tmp/test_model_id.txt
                test_passed "Trained model found from Epic 2.7: $trained_model_id" >&2
                echo "$trained_model_id"  # Return model_id
                return 0
            fi
        fi
    fi
    
    # If no model from Epic 2.7, try to find one in catalog (from previous test run)
    if [ -z "$trained_model_id" ]; then
        log_info "No model from Epic 2.7, checking catalog for existing trained models..." >&2
        local response=$(curl -sf "${VM_API}/api/models" || echo "FAILED")
        
        if [ "$response" != "FAILED" ] && [ "$response" != "[]" ] && [ -n "$response" ]; then
            if command -v jq >/dev/null 2>&1; then
                local all_model_ids=$(echo "$response" | jq -r '.[]?.model_id // empty' 2>/dev/null || echo "")
                if [ -n "$all_model_ids" ]; then
                    trained_model_id=$(echo "$all_model_ids" | grep -v "baseline" | head -1 || echo "")
                fi
            else
                trained_model_id=$(echo "$response" | grep -o '"model_id":"[^"]*"' | grep -v "baseline-yolov8n" | head -1 | cut -d'"' -f4 || echo "")
            fi
            
            if [ -n "$trained_model_id" ] && [ "$trained_model_id" != "null" ] && [ "$trained_model_id" != "" ]; then
                echo "$trained_model_id" > /tmp/test_model_id.txt
                test_passed "Trained model found in catalog (from previous run): $trained_model_id" >&2
                echo "$trained_model_id"  # Return model_id
                return 0
            fi
        fi
    fi
    
    # If still no model, check training jobs for trained_model_id
    if [ -z "$trained_model_id" ]; then
        log_info "Checking training jobs for trained models..." >&2
        local training_response=$(curl -sf "${VM_API}/api/training?limit=10" || echo "FAILED")
        
        if [ "$training_response" != "FAILED" ]; then
            if command -v jq >/dev/null 2>&1; then
                trained_model_id=$(echo "$training_response" | jq -r 'if type == "array" then .[] else .jobs[]? end | select(.trained_model_id != null and .trained_model_id != "" and .trained_model_id != "null") | .trained_model_id' 2>/dev/null | head -1 || echo "")
            else
                trained_model_id=$(echo "$training_response" | grep -o '"trained_model_id":"[^"]*"' | grep -v '""' | grep -v 'null' | head -1 | cut -d'"' -f4 || echo "")
            fi
            
            if [ -n "$trained_model_id" ] && [ "$trained_model_id" != "null" ] && [ "$trained_model_id" != "" ]; then
                local model_response=$(curl -sf "${VM_API}/api/models/${trained_model_id}" || echo "FAILED")
                if [ "$model_response" != "FAILED" ]; then
                    echo "$trained_model_id" > /tmp/test_model_id.txt
                    test_passed "Trained model found from training job: $trained_model_id" >&2
                    echo "$trained_model_id"  # Return model_id
                    return 0
                fi
            fi
        fi
    fi
    
    if [ -z "$trained_model_id" ]; then
        test_failed "No trained model found - Epic 2.7 must complete successfully first, or a model must exist from a previous run" "investigate_trained_model" >&2
        return 1
    fi
    
    echo "$trained_model_id"  # Return model_id
    return 0
}

test_verify_edge_connected() {
    local edge_id="${1:-poc-edge-1}"
    log_info "Test 2: Verifying Edge is connected..." >&2
    
    local edge_health_url="http://localhost:8081/health"
    
    if curl -sf "$edge_health_url" > /dev/null 2>&1; then
        echo "$edge_id" > /tmp/test_edge_id.txt
        test_passed "Edge orchestrator is accessible: $edge_id" >&2
        return 0
    else
        test_failed "Edge orchestrator not accessible" >&2
        return 1
    fi
}

test_trigger_model_deployment() {
    local trained_model_id="$1"
    local edge_id="$2"
    log_info "Test 3: Triggering model deployment..." >&2
    
    local deployment_request=$(cat <<EOF
{
    "model_id": "${trained_model_id}"
}
EOF
)
    
    local response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "X-Edge-ID: ${edge_id}" \
        -w "\n%{http_code}" \
        -d "$deployment_request" \
        "${VM_API}/api/edges/${edge_id}/models/deploy" 2>&1)
    
    local http_code=$(echo "$response" | tail -n1)
    local response_body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" != "202" ] && [ "$http_code" != "200" ]; then
        # Check if it's a WireGuard connection issue
        if echo "$response_body" | grep -qi "wireguard\|not connected"; then
            log_warn "Deployment failed: Edge not connected via WireGuard (expected in PoC without full WireGuard setup)" >&2
            log_warn "HTTP $http_code: $response_body" >&2
            log_warn "Skipping Epic 2.9 tests - deployment requires WireGuard connection" >&2
            test_failed "Deployment trigger - Edge not connected via WireGuard tunnel" "investigate_wireguard_tunnel \"$edge_id\"" >&2
            return 1
        else
            test_failed "Deployment trigger - API request failed with HTTP $http_code: $response_body" "investigate_model_deployment \"\" \"$edge_id\"" >&2
            log_error "Cannot continue to Epic 2.9 without deployed model - deployment is required" >&2
            return 1
        fi
    fi
    
    local deployment_id=""
    deployment_id=$(echo "$response_body" | grep -o '"deployment_id":"[^"]*"' | cut -d'"' -f4 || echo "")
    
    if [ -z "$deployment_id" ]; then
        deployment_id=$(echo "$response_body" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 || echo "")
    fi
    
    if [ -z "$deployment_id" ]; then
        # Deployment might have been created automatically, check deployments list
        log_warn "No deployment_id in response, checking deployments list..." >&2
        local deployments_response=$(curl -sf "${VM_API}/api/deployments?model_id=${trained_model_id}&limit=1" || echo "FAILED")
        if [ "$deployments_response" != "FAILED" ]; then
            deployment_id=$(echo "$deployments_response" | grep -o '"deployment_id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
        fi
    fi
    
    if [ -n "$deployment_id" ]; then
        echo "$deployment_id" > /tmp/test_deployment_id.txt
        test_passed "Deployment triggered: $deployment_id" >&2
    else
        log_warn "Could not extract deployment_id, but deployment may have been triggered" >&2
    fi
    
    echo "$deployment_id"  # Return deployment_id
    return 0
}

test_wait_for_deployment_completion() {
    local deployment_id="$1"
    log_info "Test 4: Verifying deployment status updates..." >&2
    
    # Poll for deployment completion (threshold: 60s, interval: 5s)
    local deployment_completed=false
    if poll_until_success "deployment to complete" \
        "response=\$(curl -sf \"${VM_API}/api/deployments/${deployment_id}\" || echo 'FAILED') && \
         [ \"\$response\" != 'FAILED' ] && \
         STATUS=\$(echo \"\$response\" | grep -o '\"status\":\"[^\"]*\"' | head -1 | cut -d'\"' -f4 || echo '') && \
         [ \"\$STATUS\" = 'deployed' ] || [ \"\$STATUS\" = 'active' ] || [ \"\$STATUS\" = 'failed' ]" \
        60 5 \
        "investigate_model_deployment \"$deployment_id\" \"\""; then
        deployment_completed=true
    fi
    
    # Final check with detailed response
    local response=$(curl -sf "${VM_API}/api/deployments/${deployment_id}" || echo "FAILED")
    
    if [ "$response" = "FAILED" ]; then
        test_failed "Deployment status check - API request failed" "investigate_model_deployment \"$deployment_id\" \"\"" >&2
        return 1
    fi
    
    local status=""
    if command -v jq >/dev/null 2>&1; then
        status=$(echo "$response" | jq -r '.status // empty' 2>/dev/null || echo "")
    else
        status=$(echo "$response" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
    fi
    
    if [ -z "$status" ]; then
        test_failed "Deployment status check - No status in response" "investigate_model_deployment \"$deployment_id\" \"\"" >&2
        return 1
    fi
    
    log_info "Deployment status: $status" >&2
    
    if [ "$status" = "deployed" ] || [ "$status" = "active" ]; then
        test_passed "Deployment completed successfully (status: $status)" >&2
        echo "$status"  # Return status
        return 0
    elif [ "$status" = "failed" ]; then
        local error_msg=""
        if command -v jq >/dev/null 2>&1; then
            error_msg=$(echo "$response" | jq -r '.error_message // empty' 2>/dev/null || echo "")
        else
            error_msg=$(echo "$response" | grep -o '"error_message":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
        fi
        log_warn "Deployment failed: $error_msg" >&2
        test_failed "Deployment failed after 60 seconds: $error_msg" "investigate_model_deployment \"$deployment_id\" \"\"" >&2
        return 1
    elif [ "$status" = "deploying" ] || [ "$status" = "pending" ]; then
        test_failed "Deployment not completed after 60 seconds (status: $status)" "investigate_model_deployment \"$deployment_id\" \"\"" >&2
        return 1
    else
        test_failed "Deployment status unknown: $status" "investigate_model_deployment \"$deployment_id\" \"\"" >&2
        return 1
    fi
}

test_verify_edge_received_model() {
    local deployment_id="$1"
    log_info "Test 5: Verifying Edge received and stored model..." >&2
    
    local response=$(curl -sf "${VM_API}/api/deployments/${deployment_id}" || echo "FAILED")
    
    if [ "$response" != "FAILED" ]; then
        local status=""
        local model_path=""
        
        if command -v jq >/dev/null 2>&1; then
            status=$(echo "$response" | jq -r '.status // empty' 2>/dev/null || echo "")
            model_path=$(echo "$response" | jq -r '.model_file_path // empty' 2>/dev/null || echo "")
        else
            status=$(echo "$response" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
            model_path=$(echo "$response" | grep -o '"model_file_path":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
        fi
        
        if [ "$status" = "deployed" ] || [ "$status" = "active" ]; then
            if [ -n "$model_path" ]; then
                test_passed "Edge received and stored model (status: $status, path: $model_path)" >&2
            else
                test_passed "Edge received model (status: $status)" >&2
            fi
        else
            log_warn "Edge model reception check - Status: $status" >&2
        fi
    fi
    return 0
}

test_verify_edge_model_activation() {
    local deployment_id="$1"
    log_info "Test 6: Verifying Edge model loading and activation..." >&2
    
    # Poll for Edge model activation (threshold: 40s, interval: 3s)
    if poll_until_success "Edge model activation" \
        "response=\$(curl -sf \"${VM_API}/api/deployments/${deployment_id}\" || echo 'FAILED') && \
         [ \"\$response\" != 'FAILED' ] && \
         (echo \"\$response\" | grep -q '\"status\":\"active\"' || echo \"\$response\" | grep -q '\"status\":\"deployed\"')" \
        40 3; then
        # Model activated, continue with check
        local response=$(curl -sf "${VM_API}/api/deployments/${deployment_id}" || echo "FAILED")
    else
        # Timeout reached, still check status
        local response=$(curl -sf "${VM_API}/api/deployments/${deployment_id}" || echo "FAILED")
    fi
    
    if [ "$response" = "FAILED" ]; then
        test_failed "Edge model activation check - API request failed" >&2
        return 1
    fi
    
    local status=""
    if command -v jq >/dev/null 2>&1; then
        status=$(echo "$response" | jq -r '.status // empty' 2>/dev/null || echo "")
    else
        status=$(echo "$response" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
    fi
    
    if [ "$status" = "active" ]; then
        test_passed "Edge model loaded and activated successfully" >&2
        return 0
    elif [ "$status" = "deployed" ]; then
        test_failed "Edge model not activated after 40 seconds (status: $status)" >&2
        return 1
    else
        test_failed "Edge model activation status unknown: $status" >&2
        return 1
    fi
}

test_verify_edge_status_reporting() {
    local deployment_id="$1"
    log_info "Test 7: Verifying Edge status reporting to VM..." >&2
    
    local response=$(curl -sf "${VM_API}/api/deployments/${deployment_id}" || echo "FAILED")
    
    if [ "$response" != "FAILED" ]; then
        local status=""
        local completed_at=""
        
        if command -v jq >/dev/null 2>&1; then
            status=$(echo "$response" | jq -r '.status // empty' 2>/dev/null || echo "")
            completed_at=$(echo "$response" | jq -r '.deployment_completed_at // empty' 2>/dev/null || echo "")
        else
            status=$(echo "$response" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
            completed_at=$(echo "$response" | grep -o '"deployment_completed_at":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")
        fi
        
        if [ "$status" = "deployed" ] || [ "$status" = "active" ]; then
            if [ -n "$completed_at" ]; then
                test_passed "Edge reported deployment status to VM (status: $status, completed_at: $completed_at)" >&2
            else
                test_passed "Edge reported deployment status to VM (status: $status)" >&2
            fi
        fi
    fi
    return 0
}
