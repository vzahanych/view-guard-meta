#!/bin/bash

# Protocol Buffer Generation Script for Python
# Generates Python code from .proto files for edge-internal communication

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTO_DIR="${SCRIPT_DIR}/proto"
PYTHON_OUT_DIR="${SCRIPT_DIR}/../edge/ai-service/ai_service"

echo -e "${GREEN}Generating Python protocol buffer code...${NC}"

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo -e "${RED}Error: protoc is not installed${NC}"
    echo "Install it with: sudo apt-get install protobuf-compiler"
    exit 1
fi

# Check if Python grpc_tools is available
if ! python3 -c "import grpc_tools.protoc" 2>/dev/null; then
    echo -e "${YELLOW}Warning: grpc_tools not found, installing...${NC}"
    pip3 install grpcio-tools --quiet
fi

# Create output directory if it doesn't exist
mkdir -p "${PYTHON_OUT_DIR}"

# Generate Python code for edge-internal
if [ -d "${PROTO_DIR}/edge-internal" ]; then
    echo -e "${GREEN}Generating Python code for edge-internal...${NC}"
    
    for proto_file in "${PROTO_DIR}"/edge-internal/*.proto; do
        if [ -f "$proto_file" ]; then
            echo "  Processing: $(basename $proto_file)"
            
            python3 -m grpc_tools.protoc \
                --proto_path="${PROTO_DIR}" \
                --python_out="${PYTHON_OUT_DIR}" \
                --grpc_python_out="${PYTHON_OUT_DIR}" \
                "${proto_file}"
        fi
    done
else
    echo -e "${YELLOW}Directory edge-internal does not exist, skipping...${NC}"
fi

echo -e "${GREEN}✓ Python protocol buffer generation complete!${NC}"
echo -e "Generated files are in: ${PYTHON_OUT_DIR}/"

