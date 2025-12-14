#!/bin/bash
# Local End-to-End Integration Test - Development Mode
# Uses docker-compose.dev.yml with source code mounted
#
# Usage:
#   ./test-local-e2e-dev.sh                    # Run all tests
#   ./test-local-e2e-dev.sh --epic 2.4         # Run only Epic 2.4
#   ./test-local-e2e-dev.sh --skip-cleanup     # Skip cleanup step

set -euo pipefail

# Get script directory (infra/local)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.dev.yml"

# Use the same test script but with dev compose file
export COMPOSE_FILE

# Source the main test script
exec "${SCRIPT_DIR}/test-local-e2e.sh" "$@"
