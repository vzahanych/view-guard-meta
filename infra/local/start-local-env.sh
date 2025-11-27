#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WG_DIR="${SCRIPT_DIR}/wg"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"

usage() {
  cat <<EOF
Usage: $(basename "$0") [start|restart|stop]

  start    Start local stack:
           1. Run wg-setup to generate keys
           2. Generate configs from templates
           3. Start all services
  restart  docker compose down -v, then start stack
  stop     Stop local stack (docker compose down -v)
EOF
}

generate_keys() {
  echo "[start-local-env] Step 1: Generating WireGuard keys..."
  (cd "${SCRIPT_DIR}/.." && docker compose -f local/docker-compose.yml run --rm wg-setup)
}

generate_configs() {
  echo "[start-local-env] Step 2: Generating WireGuard configs from templates..."
  if [ ! -f "${WG_DIR}/generate-configs.sh" ]; then
    echo "[start-local-env] ERROR: ${WG_DIR}/generate-configs.sh not found" >&2
    exit 1
  fi
  bash "${WG_DIR}/generate-configs.sh"
}

compose_up() {
  echo "[start-local-env] Step 3: Starting all services..."
  (cd "${SCRIPT_DIR}/.." && docker compose -f local/docker-compose.yml up -d)
  echo "[start-local-env] Stack is up. Inspect WireGuard state with:"
  echo "  docker exec view-guard-user-vm-api wg show wg0"
  echo "  docker exec view-guard-edge-orchestrator wg show"
}

compose_down() {
  echo "[start-local-env] Stopping local environment (docker compose down -v)..."
  (cd "${SCRIPT_DIR}/.." && docker compose -f local/docker-compose.yml down -v)
}

start_stack() {
  generate_keys
  generate_configs
  compose_up
}

main() {
  local cmd="${1:-start}"

  case "${cmd}" in
    start)
      start_stack
      ;;
    restart)
      compose_down || true
      start_stack
      ;;
    stop)
      compose_down
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
