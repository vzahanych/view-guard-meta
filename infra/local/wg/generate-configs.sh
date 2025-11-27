#!/usr/bin/env bash
# Generate WireGuard configs from templates and keys

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KEYS_DIR="${SCRIPT_DIR}/keys"
CONFIG_DIR="${SCRIPT_DIR}/config"
TEMPLATE_DIR="${SCRIPT_DIR}"

# Ensure directories exist
mkdir -p "${CONFIG_DIR}"

# Check if keys exist
if [ ! -f "${KEYS_DIR}/server.private" ] || [ ! -f "${KEYS_DIR}/edge.private" ]; then
    echo "[generate-configs] ERROR: Keys not found. Please run generate-keys.sh first." >&2
    exit 1
fi

# Read keys
server_priv=$(cat "${KEYS_DIR}/server.private")
server_pub=$(cat "${KEYS_DIR}/server.public")
edge_priv=$(cat "${KEYS_DIR}/edge.private")
edge_pub=$(cat "${KEYS_DIR}/edge.public")
psk=$(cat "${KEYS_DIR}/preshared.key")

echo "[generate-configs] Generating WireGuard configs from templates..."

# Generate server config from template
if [ ! -f "${TEMPLATE_DIR}/server-wg0.conf" ]; then
    echo "[generate-configs] ERROR: Template ${TEMPLATE_DIR}/server-wg0.conf not found." >&2
    exit 1
fi

sed \
    -e "s|SERVER_PRIVATE_KEY_PLACEHOLDER|${server_priv}|g" \
    -e "s|EDGE_PUBLIC_KEY_PLACEHOLDER|${edge_pub}|g" \
    -e "s|PRESHARED_KEY_PLACEHOLDER|${psk}|g" \
    "${TEMPLATE_DIR}/server-wg0.conf" > "${CONFIG_DIR}/server-wg0.conf"

# Generate edge config from template
if [ ! -f "${TEMPLATE_DIR}/edge-wg0.conf" ]; then
    echo "[generate-configs] ERROR: Template ${TEMPLATE_DIR}/edge-wg0.conf not found." >&2
    exit 1
fi

sed \
    -e "s|EDGE_PRIVATE_KEY_PLACEHOLDER|${edge_priv}|g" \
    -e "s|SERVER_PUBLIC_KEY_PLACEHOLDER|${server_pub}|g" \
    -e "s|PRESHARED_KEY_PLACEHOLDER|${psk}|g" \
    "${TEMPLATE_DIR}/edge-wg0.conf" > "${CONFIG_DIR}/edge-wg0.conf"

# Set restrictive permissions
chmod 600 "${CONFIG_DIR}"/*.conf

echo "[generate-configs] Configs generated in ${CONFIG_DIR}/"
echo "[generate-configs]   - ${CONFIG_DIR}/server-wg0.conf"
echo "[generate-configs]   - ${CONFIG_DIR}/edge-wg0.conf"

