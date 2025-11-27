#!/usr/bin/env bash
# Generate WireGuard keys and store them in keys/ directory

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KEYS_DIR="${SCRIPT_DIR}/keys"

# Ensure keys directory exists
mkdir -p "${KEYS_DIR}"

# Check if wireguard-tools is available
if ! command -v wg >/dev/null 2>&1; then
    echo "[generate-keys] ERROR: 'wg' command not found. Please install wireguard-tools." >&2
    exit 1
fi

echo "[generate-keys] Generating WireGuard keys..."

# Generate keys
server_priv=$(wg genkey)
edge_priv=$(wg genkey)
server_pub=$(printf '%s' "${server_priv}" | wg pubkey)
edge_pub=$(printf '%s' "${edge_priv}" | wg pubkey)
psk=$(wg genpsk)

# Store keys in separate files
echo "${server_priv}" > "${KEYS_DIR}/server.private"
echo "${server_pub}" > "${KEYS_DIR}/server.public"
echo "${edge_priv}" > "${KEYS_DIR}/edge.private"
echo "${edge_pub}" > "${KEYS_DIR}/edge.public"
echo "${psk}" > "${KEYS_DIR}/preshared.key"

# Set permissions - for local dev, make readable by owner and group
# In production, these should be more restrictive
chmod 640 "${KEYS_DIR}"/*.private "${KEYS_DIR}"/preshared.key 2>/dev/null || chmod 600 "${KEYS_DIR}"/*.private "${KEYS_DIR}"/preshared.key
chmod 644 "${KEYS_DIR}"/*.public

# Try to make files accessible to host user if running in container
# This is for local development only
if [ "$(id -u)" = "0" ]; then
    # Running as root, try common host UIDs
    for uid in 1000 1001 $(stat -c '%u' "${KEYS_DIR}" 2>/dev/null | head -1); do
        if chown "${uid}:${uid}" "${KEYS_DIR}"/*.private "${KEYS_DIR}"/*.public "${KEYS_DIR}"/preshared.key 2>/dev/null; then
            break
        fi
    done
    # If chown failed, make files group-readable
    chmod g+r "${KEYS_DIR}"/*.private "${KEYS_DIR}"/preshared.key 2>/dev/null || true
fi

echo "[generate-keys] Keys generated and stored in ${KEYS_DIR}/"
echo "[generate-keys] Server public key: ${server_pub}"
echo "[generate-keys] Edge public key:   ${edge_pub}"

