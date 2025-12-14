#!/bin/bash
# Quick start script for development environment
# Usage: ./start-dev.sh [service-name]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.dev.yml"

cd "$SCRIPT_DIR"

if [ $# -eq 0 ]; then
    echo "Starting development environment..."
    docker compose -f "$COMPOSE_FILE" up -d
    echo ""
    echo "Services started! View logs with:"
    echo "  docker compose -f $COMPOSE_FILE logs -f"
    echo ""
    echo "Or view specific service logs:"
    echo "  docker compose -f $COMPOSE_FILE logs -f user-vm-api"
    echo "  docker compose -f $COMPOSE_FILE logs -f edge-orchestrator"
else
    # Start specific service
    SERVICE="$1"
    echo "Starting service: $SERVICE"
    docker compose -f "$COMPOSE_FILE" up -d "$SERVICE"
    echo ""
    echo "View logs:"
    echo "  docker compose -f $COMPOSE_FILE logs -f $SERVICE"
fi
