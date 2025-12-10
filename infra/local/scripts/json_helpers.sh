#!/bin/bash
# JSON parsing helper functions (with jq fallback)
# No dependencies - standalone functions

get_json_value() {
    local json="$1"
    local key="$2"
    if command -v jq >/dev/null 2>&1; then
        echo "$json" | jq -r "$key // empty" 2>/dev/null || echo ""
    else
        # Fallback: simple grep-based extraction
        echo "$json" | grep -o "\"${key//.//}\":[^,}]*" | cut -d'"' -f4 || echo ""
    fi
}

get_json_bool() {
    local json="$1"
    local key="$2"
    local default="${3:-false}"
    if command -v jq >/dev/null 2>&1; then
        local value=$(echo "$json" | jq -r "$key // $default" 2>/dev/null || echo "$default")
        [ "$value" = "true" ] || [ "$value" = "True" ] && echo "true" || echo "false"
    else
        local value=$(echo "$json" | grep -o "\"${key//.//}\":[^,}]*" | grep -o 'true\|false' | head -1 || echo "$default")
        [ "$value" = "true" ] || [ "$value" = "True" ] && echo "true" || echo "false"
    fi
}
