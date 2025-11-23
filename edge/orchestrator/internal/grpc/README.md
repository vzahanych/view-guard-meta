# gRPC Client

This package provides gRPC client functionality for communicating with the KVM VM over the WireGuard tunnel.

## Overview

The gRPC client service:
- Manages gRPC connections to KVM VM over WireGuard tunnel
- Provides clients for all Edge ↔ KVM VM services (events, telemetry, control, streaming)
- Handles connection lifecycle and reconnection
- Integrates with WireGuard client for secure tunnel communication

## Features

- **Service Clients**: Provides clients for all Edge ↔ KVM VM gRPC services
- **Connection Management**: Automatic connection establishment and reconnection
- **WireGuard Integration**: Waits for WireGuard tunnel before connecting
- **Error Handling**: Retryable error detection and handling
- **Event Transmission**: Dedicated event sender for transmitting events

## Usage

### Initialize gRPC Client

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/grpc"

// Create client (requires WireGuard client)
wgConfig := &config.WireGuardConfig{
    Enabled:     true,
    KVMEndpoint: "kvm.example.com:51820",
}
wgClient := wireguard.NewClient(wgConfig, logger)
grpcClient := grpc.NewClient(wgConfig, wgClient, logger)
```

### Register with Service Manager

```go
// Register with service manager (WireGuard must be registered first)
serviceManager.Register(wgClient)
serviceManager.Register(grpcClient)

// Start all services
err := serviceManager.Start(ctx, cfg)
if err != nil {
    log.Fatal("Failed to start services", err)
}
defer serviceManager.Shutdown(ctx)
```

### Send Events

```go
// Create event sender
eventSender := grpc.NewEventSender(grpcClient, logger)

// Send single event
err := eventSender.SendEvent(ctx, event)
if err != nil {
    log.Error("Failed to send event", err)
}

// Send batch of events
err := eventSender.SendEvents(ctx, events)
if err != nil {
    log.Error("Failed to send events", err)
}
```

### Use Service Clients Directly

```go
// Get event client
eventClient := grpcClient.GetEventClient()
if eventClient != nil {
    // Use event client
}

// Get telemetry client
telemetryClient := grpcClient.GetTelemetryClient()

// Get control client
controlClient := grpcClient.GetControlClient()

// Get streaming client
streamingClient := grpcClient.GetStreamingClient()
```

## Integration with Event Transmitter

The gRPC client integrates with the event transmitter using the integration helper:

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/grpc"

// After both transmitter and gRPC client are started
err := grpc.IntegrateEventTransmitter(transmitter, grpcClient, logger)
if err != nil {
    log.Error("Failed to integrate event transmitter", err)
}
```

Or manually:

```go
// Create event sender
eventSender := grpc.NewEventSender(grpcClient, logger)

// Configure transmitter with gRPC sender
transmitterConfig := transmitter.GetConfig()
transmitterConfig.OnTransmit = func(ctx context.Context, events []*events.Event) error {
    return eventSender.SendEvents(ctx, events)
}
transmitter.SetConfig(transmitterConfig)
```

## Clip Streaming

The streaming service handles on-demand clip streaming:

```go
// Create streaming service
streamingService := grpc.NewStreamingService(grpcClient, logger)

// Stream a clip
err := streamingService.StreamClip(ctx, eventID, clipPath, 0)
if err != nil {
    log.Error("Failed to stream clip", err)
}

// Get clip info
info, err := streamingService.GetClipInfo(ctx, eventID, clipPath)
if err != nil {
    log.Error("Failed to get clip info", err)
}
```

## Configuration

The gRPC client uses WireGuard configuration:

```yaml
edge:
  wireguard:
    enabled: true
    kvm_endpoint: kvm.example.com:51820
```

The gRPC endpoint is derived from the KVM endpoint (default port 50051).

## Protocol Definitions

Proto definitions are in `proto/proto/edge/`:
- `events.proto` - Event transmission
- `telemetry.proto` - Telemetry and health
- `control.proto` - Control commands
- `streaming.proto` - Clip streaming

Generated Go stubs are in `proto/go/generated/edge/`.

## Error Handling

The client detects retryable errors:
- `Unavailable` - Service unavailable (retryable)
- `DeadlineExceeded` - Request timeout (retryable)
- `ResourceExhausted` - Rate limited (retryable)
- `Aborted` - Request aborted (retryable)
- `Internal` - Internal error (retryable)

Non-retryable errors (e.g., `InvalidArgument`) are returned immediately.

## Connection Lifecycle

1. **Start**: Waits for WireGuard connection, then establishes gRPC connection
2. **Health Monitoring**: Connection health monitored via keepalive
3. **Reconnection**: Automatic reconnection on connection loss
4. **Stop**: Graceful shutdown of gRPC connection

## Dependencies

- **WireGuard Client**: Must be started before gRPC client
- **Proto Stubs**: Generated from `proto/proto/edge/*.proto`
- **gRPC**: `google.golang.org/grpc`
- **Protobuf**: `google.golang.org/protobuf`

## Limitations (PoC)

- Uses insecure credentials (WireGuard provides encryption)
- Basic reconnection logic
- Endpoint hardcoded to localhost:50051 (production would use WireGuard interface IP)
- No connection pooling or advanced retry strategies

## Future Enhancements

- TLS over WireGuard for additional security
- Advanced retry strategies with exponential backoff
- Connection pooling for multiple streams
- Metrics and observability
- Circuit breaker pattern for resilience

