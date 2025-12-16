#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WG_DIR="${SCRIPT_DIR}/wg"
COMPOSE_FILE="${COMPOSE_FILE:-${SCRIPT_DIR}/docker-compose.yml}"

# Configuration - always use localhost since we run outside containers
VM_API="${VM_API:-http://localhost:8280}"
TRAINING_SERVICE="${TRAINING_SERVICE:-http://localhost:8000}"

usage() {
  cat <<EOF
Usage: $(basename "$0") [step-2.0|step-2.1|start|restart|stop]

  step-2.0  Step 2.0: Generate WireGuard configuration (keys and configs)
  step-2.1  Step 2.1: Start VM services and wait for initialization
  start     Start local stack (step-2.0 + step-2.1 + all services)
  restart   docker compose down -v, then start stack
  stop      Stop local stack (docker compose down -v)
EOF
}

# Step 2.0: Generate WireGuard configuration and TLS/mTLS certificates
step_2_0_generate_config() {
  echo "[start-local-env] Step 2.0: Generating WireGuard configuration and TLS/mTLS certificates..."
  
  # Check if keys already exist
  if [ ! -f "${WG_DIR}/keys/edge.public" ] || [ ! -f "${WG_DIR}/keys/server.public" ]; then
    echo "[start-local-env] WireGuard keys not found, generating new keys..."
    docker compose -f "$COMPOSE_FILE" run --rm wg-setup
  else
    echo "[start-local-env] WireGuard keys already exist, regenerating configs from existing keys..."
    # Only regenerate configs, not keys
    docker compose -f "$COMPOSE_FILE" run --rm wg-setup
  fi
  
  # Verify config files were created (not directories)
  if [ ! -f "${WG_DIR}/config/edge-wg0.conf" ] || [ ! -f "${WG_DIR}/config/server-wg0.conf" ]; then
    echo "[start-local-env] ERROR: WireGuard config files not generated" >&2
    return 1
  fi
  
  # Ensure they are files, not directories
  if [ ! -f "${WG_DIR}/config/edge-wg0.conf" ] || [ -d "${WG_DIR}/config/edge-wg0.conf" ]; then
    echo "[start-local-env] ERROR: edge-wg0.conf is not a file (may be a directory)" >&2
    return 1
  fi
  if [ ! -f "${WG_DIR}/config/server-wg0.conf" ] || [ -d "${WG_DIR}/config/server-wg0.conf" ]; then
    echo "[start-local-env] ERROR: server-wg0.conf is not a file (may be a directory)" >&2
    return 1
  fi
  
  echo "[start-local-env] WireGuard config files verified"
  
  # Verify certificates were generated
  if [ ! -f "${WG_DIR}/certs/ca.crt" ] || [ ! -f "${WG_DIR}/certs/vm-server.crt" ] || [ ! -f "${WG_DIR}/certs/edge-server.crt" ]; then
    echo "[start-local-env] ERROR: TLS/mTLS certificates not generated" >&2
    return 1
  fi
  
  echo "[start-local-env] TLS/mTLS certificates verified"
  
  # Read EDGE_ID from generated file
  EDGE_ID_FILE="${WG_DIR}/keys/edge-id"
  if [ ! -f "$EDGE_ID_FILE" ]; then
    echo "[start-local-env] ERROR: EDGE_ID file not found at $EDGE_ID_FILE" >&2
    return 1
  fi
  
  EDGE_ID=$(cat "$EDGE_ID_FILE")
  echo "[start-local-env] EDGE_ID generated: $EDGE_ID"

  # Export EDGE_ID for docker-compose
  export EDGE_ID

  # Detect WireGuard key rotation and reset state if keys changed
  EDGE_PUB_FILE="${WG_DIR}/keys/edge.public"
  SERVER_PUB_FILE="${WG_DIR}/keys/server.public"
  KEY_FINGERPRINT_FILE="${WG_DIR}/keys/.last-key-fingerprint"

  if [ -f "$EDGE_PUB_FILE" ] && [ -f "$SERVER_PUB_FILE" ]; then
    current_fingerprint="$(cat "$EDGE_PUB_FILE" 2>/dev/null):$(cat "$SERVER_PUB_FILE" 2>/dev/null)"
    previous_fingerprint=""
    if [ -f "$KEY_FINGERPRINT_FILE" ]; then
      previous_fingerprint="$(cat "$KEY_FINGERPRINT_FILE" 2>/dev/null || true)"
    fi

    if [ "$current_fingerprint" != "$previous_fingerprint" ]; then
      echo "[start-local-env] Detected new WireGuard keys (fingerprint changed). Resetting state to avoid stale peer/db entries..."
      # Remove containers and volumes so the user-vm-api DB and WireGuard state realign with the new keys
      docker compose -f "$COMPOSE_FILE" down -v || true
      echo "$current_fingerprint" > "$KEY_FINGERPRINT_FILE"
      echo "[start-local-env] State reset complete. Volumes will be recreated on next start."
    fi
  fi

  echo "[start-local-env] Step 2.0 complete: WireGuard configuration and TLS/mTLS certificates generated"
}

# Step 2.1: Start VM services sequentially and wait for initialization
step_2_1_start_vm_services() {
  echo "[start-local-env] Step 2.1: Starting VM services sequentially..."
  
  # 2.1.1: Start MinIO first (storage dependency)
  echo "[start-local-env] Starting MinIO service..."
  if ! docker compose -f "$COMPOSE_FILE" up -d minio; then
    echo "[start-local-env] ERROR: Failed to start MinIO service" >&2
    return 1
  fi
  
  # Wait for MinIO to be healthy (using HTTPS with self-signed cert, so use -k flag)
  echo "[start-local-env] Waiting for MinIO to be healthy (TLS enabled)..."
  local max_wait=60
  local waited=0
  while [ $waited -lt $max_wait ]; do
    if curl -sfk "https://localhost:9000/minio/health/live" > /dev/null 2>&1; then
      echo "[start-local-env] MinIO is healthy (TLS enabled)"
      break
    fi
    sleep 2
    waited=$((waited + 2))
  done
  
  if [ $waited -ge $max_wait ]; then
    echo "[start-local-env] ERROR: MinIO failed to become healthy" >&2
    return 1
  fi
  
  # 2.1.2: Start Python AI Service (training dependency)
  echo "[start-local-env] Starting Python AI Service..."
  if ! docker compose -f "$COMPOSE_FILE" up -d python-ai-service; then
    echo "[start-local-env] ERROR: Failed to start Python AI Service" >&2
    return 1
  fi
  
  # Wait for Python AI Service to be healthy
  echo "[start-local-env] Waiting for Python AI Service to be healthy..."
  max_wait=120
  waited=0
  while [ $waited -lt $max_wait ]; do
    if curl -sf "${TRAINING_SERVICE}/health" > /dev/null 2>&1; then
      echo "[start-local-env] Python AI Service is healthy"
      break
    fi
    sleep 3
    waited=$((waited + 3))
  done
  
  if [ $waited -ge $max_wait ]; then
    echo "[start-local-env] ERROR: Python AI Service failed to become healthy" >&2
    return 1
  fi
  
  # 2.1.3: Start User VM API (main service, depends on MinIO)
  echo "[start-local-env] Starting User VM API service..."
  if ! docker compose -f "$COMPOSE_FILE" up -d user-vm-api; then
    echo "[start-local-env] ERROR: Failed to start User VM API service" >&2
    return 1
  fi
  
  # Wait for User VM API to be healthy (with enhanced health check)
  echo "[start-local-env] Waiting for User VM API to be healthy and fully initialized..."
  max_wait=120
  waited=0
  while [ $waited -lt $max_wait ]; do
    # Check enhanced health endpoint that verifies database, MinIO, and training service
    health_response=$(curl -sf "${VM_API}/health" 2>/dev/null || echo "")
    if [ -n "$health_response" ]; then
      # Check if health status is "healthy" (not "unhealthy")
      if echo "$health_response" | grep -q '"status":"healthy"'; then
        echo "[start-local-env] User VM API is healthy and fully initialized"
        break
      fi
    fi
    sleep 3
    waited=$((waited + 3))
  done
  
  if [ $waited -ge $max_wait ]; then
    echo "[start-local-env] ERROR: User VM API failed to become healthy" >&2
    return 1
  fi
  
  # 2.1.4: Start baseline model setup (runs once, non-blocking)
  echo "[start-local-env] Starting baseline model setup..."
  docker compose -f "$COMPOSE_FILE" up -d baseline-model-setup || true
  
  # Verify all VM services are fully functional
  echo "[start-local-env] Verifying all VM services are fully functional..."
  max_wait=30
  waited=0
  while [ $waited -lt $max_wait ]; do
    # Check VM API health (should return "healthy" status)
    vm_health=$(curl -sf "${VM_API}/health" 2>/dev/null | grep -q '"status":"healthy"' && echo "ok" || echo "fail")
    # Check Python AI Service health
    python_health=$(curl -sf "${TRAINING_SERVICE}/health" > /dev/null 2>&1 && echo "ok" || echo "fail")
    # Check MinIO health (using HTTPS with self-signed cert, so use -k flag)
    minio_health=$(curl -sfk "https://localhost:9000/minio/health/live" > /dev/null 2>&1 && echo "ok" || echo "fail")
    
    if [ "$vm_health" = "ok" ] && [ "$python_health" = "ok" ] && [ "$minio_health" = "ok" ]; then
      echo "[start-local-env] All VM services are healthy and fully functional"
      echo "[start-local-env] Step 2.1 complete: VM services started and initialized"
      return 0
    fi
    sleep 2
    waited=$((waited + 2))
  done
  
  echo "[start-local-env] ERROR: VM services failed to become fully functional" >&2
  return 1
}

compose_down() {
  echo "[start-local-env] Stopping local environment (docker compose down -v)..."
  docker compose -f "$COMPOSE_FILE" down -v
}

start_stack() {
  step_2_0_generate_config
  step_2_1_start_vm_services
  # Note: Edge services and edge registration are handled by integration test script
}

main() {
  local cmd="${1:-start}"

  case "${cmd}" in
    step-2.0)
      step_2_0_generate_config
      ;;
    step-2.1)
      step_2_1_start_vm_services
      ;;
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
