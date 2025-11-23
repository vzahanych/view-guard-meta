# Proto Stub Generation Notes

## Status

Proto definitions are created in `proto/edge/`:
- `events.proto` ✅
- `telemetry.proto` ✅
- `control.proto` ✅
- `streaming.proto` ✅

## Generating Go Stubs

To generate Go stubs, you need:

1. **Install Protocol Buffers compiler**:
   ```bash
   # Ubuntu/Debian
   sudo apt-get install protobuf-compiler
   
   # macOS
   brew install protobuf
   
   # Verify installation
   protoc --version
   ```

2. **Install Go plugins**:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

3. **Generate stubs**:
   ```bash
   cd proto
   make generate
   ```

   Or manually:
   ```bash
   mkdir -p go/generated/edge
   
   protoc \
     --go_out=go/generated \
     --go_opt=paths=source_relative \
     --go-grpc_out=go/generated \
     --go-grpc_opt=paths=source_relative \
     --proto_path=proto \
     proto/edge/events.proto \
     proto/edge/telemetry.proto \
     proto/edge/control.proto \
     proto/edge/streaming.proto
   ```

## Expected Output

After generation, you should have:
```
proto/go/generated/edge/
├── events/
│   ├── events.pb.go
│   └── events_grpc.pb.go
├── telemetry/
│   ├── telemetry.pb.go
│   └── telemetry_grpc.pb.go
├── control/
│   ├── control.pb.go
│   └── control_grpc.pb.go
└── streaming/
    ├── streaming.pb.go
    └── streaming_grpc.pb.go
```

## For CI/CD

Add to CI/CD pipeline:
```yaml
- name: Install protoc
  run: |
    sudo apt-get update
    sudo apt-get install -y protobuf-compiler

- name: Install Go plugins
  run: |
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

- name: Generate proto stubs
  run: |
    cd proto
    make generate
```

## Current Status

**Proto definitions**: ✅ Created
**Go stubs**: ⏸️ Pending `protoc` installation

The gRPC client code is ready but requires generated stubs to compile.

