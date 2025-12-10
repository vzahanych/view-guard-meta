#!/bin/bash
# Test helper functions

# Source logging functions (SCRIPT_DIR should be set by calling script)
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPTS_DIR}/logging.sh"

test_passed() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    EPIC_TESTS=$((EPIC_TESTS + 1))
    log_info "✓ Test passed: $1"
}

test_failed() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    EPIC_TESTS=$((EPIC_TESTS + 1))
    log_error "✗ Test failed: $1"
    
    # Run investigation if provided (second argument is investigation function name or command)
    if [ -n "${2:-}" ]; then
        log_section "Investigating test failure: $1"
        # If it's a function name, call it; otherwise eval the command
        if type "$2" >/dev/null 2>&1; then
            "$2"
        else
            eval "$2"
        fi
    fi
}

# Helper to compose multiple investigation functions
investigate_all() {
    local functions=("$@")
    for func in "${functions[@]}"; do
        if type "$func" >/dev/null 2>&1; then
            "$func"
        fi
    done
}
