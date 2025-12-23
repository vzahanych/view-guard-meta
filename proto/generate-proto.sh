#!/bin/bash

# Protocol Buffer Generation Script
# Generates Go code from .proto files in vm-to-edge/ and edge-to-vm/ directories

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTO_DIR="${SCRIPT_DIR}/proto"
GO_OUT_DIR="${SCRIPT_DIR}/go/generated"

echo -e "${GREEN}Generating protocol buffer code...${NC}"

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo -e "${RED}Error: protoc is not installed${NC}"
    echo "Install it with: sudo apt-get install protobuf-compiler"
    exit 1
fi

# Check if Go protoc plugins are installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo -e "${YELLOW}Warning: protoc-gen-go not found, installing...${NC}"
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo -e "${YELLOW}Warning: protoc-gen-go-grpc not found, installing...${NC}"
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# Output directories will be created by protoc based on source structure

# Function to generate proto files for a directory
generate_proto() {
    local proto_subdir=$1
    local go_package_subdir=$2
    
    if [ ! -d "${PROTO_DIR}/${proto_subdir}" ]; then
        echo -e "${YELLOW}Directory ${proto_subdir} does not exist, skipping...${NC}"
        return
    fi
    
    local proto_files=$(find "${PROTO_DIR}/${proto_subdir}" -name "*.proto" 2>/dev/null)
    
    if [ -z "$proto_files" ]; then
        echo -e "${YELLOW}No .proto files found in ${proto_subdir}${NC}"
        return
    fi
    
    echo -e "${GREEN}Generating code for ${proto_subdir}...${NC}"
    
    for proto_file in $proto_files; do
        echo "  Processing: $(basename $proto_file)"
        
        protoc \
            --proto_path="${PROTO_DIR}" \
            --go_out="${SCRIPT_DIR}/go" \
            --go_opt=module=github.com/vzahanych/view-guard-meta/proto/go \
            --go-grpc_out="${SCRIPT_DIR}/go" \
            --go-grpc_opt=module=github.com/vzahanych/view-guard-meta/proto/go \
            "${proto_file}"
    done
}

# Generate for vm-to-edge (VM → Edge)
generate_proto "vm-to-edge" "vm_to_edge"

# Generate for edge-to-vm (Edge → VM)
generate_proto "edge-to-vm" "edge_to_vm"

# Generate for edge-internal (Edge internal communication)
generate_proto "edge-internal" "edge_internal"

echo -e "${GREEN}✓ Protocol buffer generation complete!${NC}"
echo -e "Generated files are in: ${GO_OUT_DIR}/"

# Run go mod tidy if go.mod exists
if [ -f "${SCRIPT_DIR}/go/go.mod" ]; then
    echo -e "${GREEN}Running go mod tidy...${NC}"
    cd "${SCRIPT_DIR}/go"
    go mod tidy
    echo -e "${GREEN}✓ Go modules updated${NC}"
fi

