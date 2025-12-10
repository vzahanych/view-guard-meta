#!/bin/bash
# Polling helper function

# Source logging functions (SCRIPTS_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"

# Polling helper function
# Usage: poll_until_success <description> <check_command> <timeout_seconds> <interval_seconds> [failure_investigation]
# Returns 0 on success, 1 on timeout
poll_until_success() {
    local description="$1"
    local check_command="$2"
    local timeout_seconds="${3:-60}"
    local interval_seconds="${4:-2}"
    local failure_investigation="${5:-}"
    local elapsed=0
    
    log_info "Waiting for: $description (timeout: ${timeout_seconds}s, interval: ${interval_seconds}s)..."
    
    while [ "$elapsed" -lt "$timeout_seconds" ]; do
        # Suppress output from check_command to avoid interfering with variable comparisons
        if eval "$check_command" >/dev/null 2>&1; then
            log_info "✓ $description - success after ${elapsed}s"
            return 0
        fi
        
        sleep "$interval_seconds"
        elapsed=$((elapsed + interval_seconds))
        
        # Show progress every 10 seconds
        if [ $((elapsed % 10)) -eq 0 ]; then
            log_info "  Still waiting... (${elapsed}/${timeout_seconds}s)"
        fi
    done
    
    # Timeout reached
    log_error "✗ $description - timeout after ${timeout_seconds}s"
    if [ -n "$failure_investigation" ]; then
        eval "$failure_investigation"
    fi
    return 1
}
