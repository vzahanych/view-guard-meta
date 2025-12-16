#!/usr/bin/env bash
# Generate TLS/mTLS certificates for zero-trust security
# Creates CA, VM services, Edge services, and database certificates

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERTS_DIR="${SCRIPT_DIR}/certs"
KEYS_DIR="${SCRIPT_DIR}/keys"

# Ensure certs directory exists
mkdir -p "${CERTS_DIR}"

# Check if openssl is available
if ! command -v openssl >/dev/null 2>&1; then
    echo "[generate-certs] ERROR: 'openssl' command not found. Please install openssl." >&2
    exit 1
fi

# Check if certificates already exist - if so, regenerate to include required SANs
# (Previous certificates may not have included all required names in Subject Alternative Names)
REGENERATE=false
if [ -f "${CERTS_DIR}/vm-server.crt" ]; then
    # Check if certificate includes "vm-server" in SANs (required for HTTPS client verification)
    if ! openssl x509 -in "${CERTS_DIR}/vm-server.crt" -noout -text 2>/dev/null | grep -q "DNS:vm-server"; then
        echo "[generate-certs] Existing VM server certificate does not include 'vm-server' in SANs, regenerating..."
        REGENERATE=true
    fi
fi

# If certificates exist and don't need regeneration, verify they're signed by the same CA
if [ "$REGENERATE" = "false" ] && [ -f "${CERTS_DIR}/ca.crt" ] && [ -f "${CERTS_DIR}/ca.key" ] && \
   [ -f "${CERTS_DIR}/vm-server.crt" ] && [ -f "${CERTS_DIR}/vm-server.key" ] && \
   [ -f "${CERTS_DIR}/edge-server.crt" ] && [ -f "${CERTS_DIR}/edge-server.key" ] && \
   [ -f "${CERTS_DIR}/vm-db.crt" ] && [ -f "${CERTS_DIR}/vm-db.key" ] && \
   [ -f "${CERTS_DIR}/edge-db.crt" ] && [ -f "${CERTS_DIR}/edge-db.key" ]; then
    # Verify all certificates are signed by the same CA
    echo "[generate-certs] Certificates exist, verifying they're signed by the same CA..."
    invalid_certs=false
    for cert in "${CERTS_DIR}/vm-server.crt" "${CERTS_DIR}/edge-server.crt" "${CERTS_DIR}/vm-client.crt" "${CERTS_DIR}/edge-client.crt" "${CERTS_DIR}/vm-db.crt" "${CERTS_DIR}/edge-db.crt"; do
        if [ -f "$cert" ]; then
            if ! openssl verify -CAfile "${CERTS_DIR}/ca.crt" "$cert" >/dev/null 2>&1; then
                echo "[generate-certs] Certificate $(basename "$cert") is not signed by current CA, will regenerate"
                invalid_certs=true
                break
            fi
        fi
    done
    
    if [ "$invalid_certs" = "false" ]; then
        echo "[generate-certs] All certificates are valid and signed by the same CA, skipping generation"
        echo "[generate-certs] To regenerate certificates, delete files in ${CERTS_DIR}/ first"
        # Display certificate info
        if [ -f "${CERTS_DIR}/ca.crt" ]; then
            ca_subject=$(openssl x509 -in "${CERTS_DIR}/ca.crt" -noout -subject 2>/dev/null | sed 's/subject=//' || echo "N/A")
            echo "[generate-certs] CA Subject: ${ca_subject}"
        fi
        exit 0
    else
        echo "[generate-certs] Some certificates are invalid or signed by different CA, regenerating..."
        # Remove invalid certificates but keep CA
        rm -f "${CERTS_DIR}"/vm-server.* "${CERTS_DIR}"/edge-server.* "${CERTS_DIR}"/vm-client.* "${CERTS_DIR}"/edge-client.* "${CERTS_DIR}"/vm-db.* "${CERTS_DIR}"/edge-db.* 2>/dev/null || true
    fi
fi

# If regenerating, remove old certificates (but keep CA if it exists and is valid)
if [ "$REGENERATE" = "true" ]; then
    echo "[generate-certs] Removing old certificates for regeneration..."
    rm -f "${CERTS_DIR}"/vm-server.* "${CERTS_DIR}"/edge-server.* "${CERTS_DIR}"/vm-client.* "${CERTS_DIR}"/edge-client.* "${CERTS_DIR}"/vm-db.* "${CERTS_DIR}"/edge-db.* 2>/dev/null || true
fi

echo "[generate-certs] Generating TLS/mTLS certificates for zero-trust security..."

# Read Edge ID if available (for certificate CN)
EDGE_ID="poc-edge-1"
if [ -f "${KEYS_DIR}/edge-id" ]; then
    EDGE_ID=$(cat "${KEYS_DIR}/edge-id" | tr -d '\n\r')
fi

# Certificate validity period (10 years for CA, 1 year for server certs)
CA_VALIDITY_DAYS=3650
SERVER_VALIDITY_DAYS=365

# ============================================================================
# 1. Generate CA (Certificate Authority) - Persistent for local dev
# In production, this will be managed by HashiCorp Vault
# For local development, we keep the CA persistent to avoid invalidating
# existing certificates on each run
# ============================================================================

# Check if CA already exists and is valid
if [ -f "${CERTS_DIR}/ca.crt" ] && [ -f "${CERTS_DIR}/ca.key" ]; then
    # Verify CA certificate is valid
    if openssl x509 -in "${CERTS_DIR}/ca.crt" -noout -checkend 86400 >/dev/null 2>&1; then
        echo "[generate-certs] Using existing CA certificate (persistent for local dev)"
        ca_subject=$(openssl x509 -in "${CERTS_DIR}/ca.crt" -noout -subject 2>/dev/null | sed 's/subject=//' || echo "N/A")
        echo "[generate-certs] CA Subject: ${ca_subject}"
    else
        echo "[generate-certs] Existing CA certificate is expired or invalid, generating new CA..."
        rm -f "${CERTS_DIR}/ca.crt" "${CERTS_DIR}/ca.key" 2>/dev/null || true
        # Fall through to generate new CA
    fi
fi

# Generate CA only if it doesn't exist
if [ ! -f "${CERTS_DIR}/ca.crt" ] || [ ! -f "${CERTS_DIR}/ca.key" ]; then
    echo "[generate-certs] Generating new CA certificate and key (will be persistent for local dev)..."
    
    # Generate CA private key
    openssl genrsa -out "${CERTS_DIR}/ca.key" 4096 2>/dev/null
    
    # Generate CA certificate
    openssl req -new -x509 -days "${CA_VALIDITY_DAYS}" \
        -key "${CERTS_DIR}/ca.key" \
        -out "${CERTS_DIR}/ca.crt" \
        -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=Security/CN=ViewGuard Root CA" \
        -extensions v3_ca \
        -config <(cat <<EOF
[req]
distinguished_name = req_distinguished_name
[req_distinguished_name]
[v3_ca]
basicConstraints = critical,CA:true
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer:always
EOF
) 2>/dev/null
    
    echo "[generate-certs] CA certificate generated (valid for ${CA_VALIDITY_DAYS} days)"
    ca_subject=$(openssl x509 -in "${CERTS_DIR}/ca.crt" -noout -subject 2>/dev/null | sed 's/subject=//' || echo "N/A")
    echo "[generate-certs] CA Subject: ${ca_subject}"
fi

# ============================================================================
# 2. Generate VM Server Certificate (for user-vm-api gRPC/TLS)
# ============================================================================
echo "[generate-certs] Generating VM server certificate..."

# Generate VM server private key
openssl genrsa -out "${CERTS_DIR}/vm-server.key" 2048 2>/dev/null

# Generate VM server certificate signing request
openssl req -new \
    -key "${CERTS_DIR}/vm-server.key" \
    -out "${CERTS_DIR}/vm-server.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=VM Services/CN=user-vm-api" \
    2>/dev/null

# Generate VM server certificate signed by CA
openssl x509 -req -days "${SERVER_VALIDITY_DAYS}" \
    -in "${CERTS_DIR}/vm-server.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/vm-server.crt" \
    -extensions v3_server \
    -extfile <(cat <<EOF
[v3_server]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth,clientAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = user-vm-api
DNS.2 = vm-server
DNS.3 = localhost
DNS.4 = minio
IP.1 = 127.0.0.1
IP.2 = 10.0.0.1
EOF
) 2>/dev/null

# ============================================================================
# 3. Generate Edge Server Certificate (for edge-orchestrator gRPC/TLS)
# ============================================================================
echo "[generate-certs] Generating Edge server certificate..."

# Generate Edge server private key
openssl genrsa -out "${CERTS_DIR}/edge-server.key" 2048 2>/dev/null

# Generate Edge server certificate signing request
openssl req -new \
    -key "${CERTS_DIR}/edge-server.key" \
    -out "${CERTS_DIR}/edge-server.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=Edge Services/CN=edge-orchestrator" \
    2>/dev/null

# Generate Edge server certificate signed by CA
openssl x509 -req -days "${SERVER_VALIDITY_DAYS}" \
    -in "${CERTS_DIR}/edge-server.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/edge-server.crt" \
    -extensions v3_server \
    -extfile <(cat <<EOF
[v3_server]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth,clientAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = edge-orchestrator
DNS.2 = localhost
IP.1 = 127.0.0.1
IP.2 = 10.0.0.2
EOF
) 2>/dev/null

# ============================================================================
# 4. Generate VM Client Certificate (for VM → Edge mTLS)
# ============================================================================
echo "[generate-certs] Generating VM client certificate..."

# Generate VM client private key
openssl genrsa -out "${CERTS_DIR}/vm-client.key" 2048 2>/dev/null

# Generate VM client certificate signing request
openssl req -new \
    -key "${CERTS_DIR}/vm-client.key" \
    -out "${CERTS_DIR}/vm-client.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=VM Services/CN=vm-client" \
    2>/dev/null

# Generate VM client certificate signed by CA
openssl x509 -req -days "${SERVER_VALIDITY_DAYS}" \
    -in "${CERTS_DIR}/vm-client.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/vm-client.crt" \
    -extensions v3_client \
    -extfile <(cat <<EOF
[v3_client]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = clientAuth
EOF
) 2>/dev/null

# ============================================================================
# 5. Generate Edge Client Certificate (for Edge → VM mTLS)
# ============================================================================
echo "[generate-certs] Generating Edge client certificate..."

# Generate Edge client private key
openssl genrsa -out "${CERTS_DIR}/edge-client.key" 2048 2>/dev/null

# Generate Edge client certificate signing request
openssl req -new \
    -key "${CERTS_DIR}/edge-client.key" \
    -out "${CERTS_DIR}/edge-client.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=Edge Services/CN=edge-client-${EDGE_ID}" \
    2>/dev/null

# Generate Edge client certificate signed by CA
openssl x509 -req -days "${SERVER_VALIDITY_DAYS}" \
    -in "${CERTS_DIR}/edge-client.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/edge-client.crt" \
    -extensions v3_client \
    -extfile <(cat <<EOF
[v3_client]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = clientAuth
EOF
) 2>/dev/null

# ============================================================================
# 6. Generate VM Database Certificate (for VM services ↔ SQLite/TLS)
# ============================================================================
echo "[generate-certs] Generating VM database certificate..."

# Generate VM database private key
openssl genrsa -out "${CERTS_DIR}/vm-db.key" 2048 2>/dev/null

# Generate VM database certificate signing request
openssl req -new \
    -key "${CERTS_DIR}/vm-db.key" \
    -out "${CERTS_DIR}/vm-db.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=VM Services/CN=vm-database" \
    2>/dev/null

# Generate VM database certificate signed by CA
openssl x509 -req -days "${SERVER_VALIDITY_DAYS}" \
    -in "${CERTS_DIR}/vm-db.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/vm-db.crt" \
    -extensions v3_server \
    -extfile <(cat <<EOF
[v3_server]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth,clientAuth
EOF
) 2>/dev/null

# ============================================================================
# 7. Generate Edge Database Certificate (for Edge services ↔ SQLite/TLS)
# ============================================================================
echo "[generate-certs] Generating Edge database certificate..."

# Generate Edge database private key
openssl genrsa -out "${CERTS_DIR}/edge-db.key" 2048 2>/dev/null

# Generate Edge database certificate signing request
openssl req -new \
    -key "${CERTS_DIR}/edge-db.key" \
    -out "${CERTS_DIR}/edge-db.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=ViewGuard/OU=Edge Services/CN=edge-database" \
    2>/dev/null

# Generate Edge database certificate signed by CA
openssl x509 -req -days "${SERVER_VALIDITY_DAYS}" \
    -in "${CERTS_DIR}/edge-db.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/edge-db.crt" \
    -extensions v3_server \
    -extfile <(cat <<EOF
[v3_server]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth,clientAuth
EOF
) 2>/dev/null

# ============================================================================
# Set proper permissions
# ============================================================================
# Private keys: read-only by owner
chmod 600 "${CERTS_DIR}"/*.key 2>/dev/null || true
# Certificates: readable by owner and group
chmod 644 "${CERTS_DIR}"/*.crt 2>/dev/null || true
# CSRs: readable (can be deleted after cert generation)
chmod 644 "${CERTS_DIR}"/*.csr 2>/dev/null || true

# Try to make files accessible to host user if running in container
if [ "$(id -u)" = "0" ]; then
    # Running as root, try common host UIDs
    for uid in 1000 1001 $(stat -c '%u' "${CERTS_DIR}" 2>/dev/null | head -1); do
        if chown "${uid}:${uid}" "${CERTS_DIR}"/*.key "${CERTS_DIR}"/*.crt 2>/dev/null; then
            break
        fi
    done
    # If chown failed, make files group-readable
    chmod g+r "${CERTS_DIR}"/*.crt 2>/dev/null || true
fi

# Clean up CSRs (no longer needed after certificate generation)
rm -f "${CERTS_DIR}"/*.csr 2>/dev/null || true
rm -f "${CERTS_DIR}"/ca.srl 2>/dev/null || true

# Verify all certificates are signed by the same CA
echo "[generate-certs] Verifying all certificates are signed by the same CA..."
ca_fingerprint=$(openssl x509 -in "${CERTS_DIR}/ca.crt" -noout -fingerprint -sha256 2>/dev/null | cut -d'=' -f2 || echo "")

verify_cert() {
    local cert_file="$1"
    local cert_name="$2"
    if [ ! -f "$cert_file" ]; then
        echo "[generate-certs] WARNING: $cert_name certificate not found: $cert_file"
        return 1
    fi
    
    # Verify certificate is signed by CA
    if openssl verify -CAfile "${CERTS_DIR}/ca.crt" "$cert_file" >/dev/null 2>&1; then
        issuer=$(openssl x509 -in "$cert_file" -noout -issuer 2>/dev/null | sed 's/issuer=//' || echo "")
        echo "[generate-certs]   ✓ $cert_name: signed by CA (issuer: $issuer)"
        return 0
    else
        echo "[generate-certs]   ✗ $cert_name: NOT signed by CA - certificate invalid!"
        return 1
    fi
}

verify_cert "${CERTS_DIR}/vm-server.crt" "VM Server"
verify_cert "${CERTS_DIR}/edge-server.crt" "Edge Server"
verify_cert "${CERTS_DIR}/vm-client.crt" "VM Client"
verify_cert "${CERTS_DIR}/edge-client.crt" "Edge Client"
verify_cert "${CERTS_DIR}/vm-db.crt" "VM Database"
verify_cert "${CERTS_DIR}/edge-db.crt" "Edge Database"

echo "[generate-certs] Certificates generated and stored in ${CERTS_DIR}/"
echo "[generate-certs]   CA: ca.crt, ca.key (persistent for local dev, managed by Vault in production)"
echo "[generate-certs]   VM Server: vm-server.crt, vm-server.key"
echo "[generate-certs]   Edge Server: edge-server.crt, edge-server.key"
echo "[generate-certs]   VM Client: vm-client.crt, vm-client.key"
echo "[generate-certs]   Edge Client: edge-client.crt, edge-client.key"
echo "[generate-certs]   VM Database: vm-db.crt, vm-db.key"
echo "[generate-certs]   Edge Database: edge-db.crt, edge-db.key"
