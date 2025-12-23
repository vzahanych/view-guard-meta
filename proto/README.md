# Protocol Buffer Definitions

This directory contains protocol buffer definitions for communication between VM and Edge components.

## Directory Structure

```
proto/
├── proto/                          # Protocol buffer source files
│   ├── vm-to-edge/                 # Messages/services from VM → Edge
│   │   └── (proto files here)
│   └── edge-to-vm/                 # Messages/services from Edge → VM
│       └── auth.proto
├── go/                             # Generated Go code
│   ├── generated/
│   │   ├── vm_to_edge/             # Generated VM → Edge code
│   │   └── edge_to_vm/             # Generated Edge → VM code
│   └── go.mod
└── generate-proto.sh               # Generation script
```

## Naming Conventions

- **Directory names**: Use kebab-case (`vm-to-edge`, `edge-to-vm`)
- **Package names**: Use snake_case (`vm_to_edge`, `edge_to_vm`)
- **Go package paths**: Use snake_case in generated paths (`vm_to_edge`, `edge_to_vm`)

## Generating Code

Run the generation script:

```bash
./generate-proto.sh
```

This script will:
1. Check for required tools (`protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`)
2. Generate Go code for all `.proto` files in `vm-to-edge/` and `edge-to-vm/`
3. Run `go mod tidy` to update dependencies

### Prerequisites

- `protoc` (Protocol Buffer Compiler)
- `protoc-gen-go` (Go plugin for protoc)
- `protoc-gen-go-grpc` (gRPC Go plugin for protoc)

Install with:
```bash
# Install protoc
sudo apt-get install protobuf-compiler

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

## Usage in Go Code

Import generated code:

```go
import (
    "github.com/vzahanych/view-guard-meta/proto/go/generated/edge_to_vm"
    "github.com/vzahanych/view-guard-meta/proto/go/generated/vm_to_edge"
)
```

The package names are snake_case (e.g., `edge_to_vm`, `vm_to_edge`) matching the proto package declarations.

## Adding New Proto Files

1. Create your `.proto` file in the appropriate directory:
   - `proto/vm-to-edge/` for VM → Edge messages/services
   - `proto/edge-to-vm/` for Edge → VM messages/services

2. Set the package name to match the directory (snake_case):
   ```protobuf
   package vm_to_edge;  // or edge_to_vm
   ```

3. Set the go_package option:
   ```protobuf
   option go_package = "github.com/vzahanych/view-guard-meta/proto/go/generated/vm_to_edge";
   ```

4. Run `./generate-proto.sh` to generate code
