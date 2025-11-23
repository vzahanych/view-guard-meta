# Protocol Definitions

Protocol buffer definitions and SDKs for The Private AI Guardian platform APIs.

## Overview

This is the single source of truth for all `.proto` definitions:
- Edge ↔ KVM VM communication protocols
- KVM VM ↔ SaaS communication protocols
- Generated language stubs (Go, TypeScript, Python)

## Directory Structure

- `proto/edge/` - Edge ↔ KVM VM protocol definitions
- `proto/kvm/` - KVM VM ↔ SaaS protocol definitions
- `go/generated/` - Generated Go stubs
- `typescript/generated/` - Generated TypeScript stubs
- `python/generated/` - Generated Python stubs
- `docs/` - API reference and protocol documentation

## Generating Stubs

### Prerequisites

1. Install Protocol Buffers compiler:
   ```bash
   # Ubuntu/Debian
   sudo apt-get install protobuf-compiler
   
   # macOS
   brew install protobuf
   ```

2. Install Go plugins:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

### Generate Go Stubs

```bash
cd proto
make generate
```

Or manually:
```bash
protoc \
  --go_out=go/generated \
  --go_opt=paths=source_relative \
  --go-grpc_out=go/generated \
  --go-grpc_opt=paths=source_relative \
  --proto_path=proto \
  proto/edge/*.proto
```

## Protocol Definitions

### Edge ↔ KVM VM (`proto/edge/`)

- **`events.proto`** - Event transmission service
  - `EventService.SendEvents` - Batch event transmission
  - `EventService.SendEvent` - Single event transmission

- **`telemetry.proto`** - Telemetry and health reporting
  - `TelemetryService.SendTelemetry` - System and application metrics
  - `TelemetryService.Heartbeat` - Heartbeat for connection monitoring

- **`control.proto`** - Control commands from KVM VM to Edge
  - `ControlService.GetConfig` - Retrieve Edge configuration
  - `ControlService.UpdateConfig` - Update Edge configuration
  - `ControlService.RestartService` - Restart Edge services

- **`streaming.proto`** - On-demand clip streaming
  - `StreamingService.StreamClip` - Stream video clip
  - `StreamingService.GetClipInfo` - Get clip metadata

### KVM VM ↔ SaaS (`proto/kvm/`)

(To be defined in Phase 2)

## Usage

### In Edge Orchestrator

```go
import edgeevents "github.com/vzahanych/view-guard-meta/proto/go/generated/edge/events"

// Use generated stubs
client := edgeevents.NewEventServiceClient(conn)
```

### In KVM VM Agent

```go
import edgeevents "github.com/vzahanych/view-guard-meta/proto/go/generated/edge/events"

// Implement server
type EventServer struct {
    edgeevents.UnimplementedEventServiceServer
}
```

## License

Apache 2.0
