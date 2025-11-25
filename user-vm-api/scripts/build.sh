#!/bin/bash
set -euo pipefail

# Build script for User VM API

VERSION=${VERSION:-"dev"}
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building User VM API..."
echo "  Version: $VERSION"
echo "  Build Time: $BUILD_TIME"
echo "  Git Commit: $GIT_COMMIT"

cd "$(dirname "$0")/.."

# Build
go build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME -X main.gitCommit=$GIT_COMMIT" \
  -o user-vm-api ./cmd/server

echo "Build complete: ./user-vm-api"

