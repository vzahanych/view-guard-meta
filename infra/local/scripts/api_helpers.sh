#!/bin/bash
# API call helper functions

# Source JSON helpers (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/json_helpers.sh"

call_api() {
    local url="$1"
    curl -sfL "$url" 2>&1 || echo "FAILED"
}

check_edge_registered() {
    local edge_id="$1"
    local response=$(call_api "${VM_API}/api/edges")
    [ "$response" = "FAILED" ] && return 1
    
    if command -v jq >/dev/null 2>&1; then
        local found=$(echo "$response" | jq -r ".[] | select(.edge_id == \"$edge_id\" or .id == \"$edge_id\") | .edge_id // .id" 2>/dev/null | head -1)
        [ -n "$found" ] && [ "$found" != "null" ] && [ "$found" != "" ]
    else
        echo "$response" | grep -q "\"edge_id\":\"$edge_id\"" || echo "$response" | grep -q "\"id\":\"$edge_id\""
    fi
}

get_edge_status() {
    local edge_id="$1"
    call_api "${VM_API}/api/edges/${edge_id}/status"
}
