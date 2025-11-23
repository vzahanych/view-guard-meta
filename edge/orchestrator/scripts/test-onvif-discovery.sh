#!/usr/bin/env bash
# Test script for camera discovery (ONVIF and USB)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ORCHESTRATOR_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Camera Discovery Test ==="
echo ""
echo "This script will test both ONVIF network cameras and USB cameras."
echo ""

cd "$ORCHESTRATOR_DIR"

# Build test binaries
echo "Building test binaries..."
go build -o bin/test-onvif-discovery ./cmd/test-onvif-discovery
go build -o bin/test-usb-discovery ./cmd/test-usb-discovery

# Test USB cameras first
echo ""
echo "=== Testing USB Camera Discovery ==="
echo ""
./bin/test-usb-discovery

echo ""
echo "=== Testing ONVIF Network Camera Discovery ==="
echo "Make sure your development laptop and cameras are on the same WiFi network."
echo ""
./bin/test-onvif-discovery

echo ""
echo "Test complete!"

