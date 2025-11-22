# Edge Orchestrator

The Edge Orchestrator is the main Go service that coordinates all Edge Appliance components.

## Overview

The orchestrator manages:
- Service lifecycle (startup, shutdown)
- Configuration management
- Structured logging
- Graceful shutdown handling
- Service coordination

## Structure

```
orchestrator/
├── main.go                    # Main entry point
├── internal/
│   ├── config/               # Configuration management
│   │   └── config.go         # YAML config loading and validation
│   ├── logger/               # Structured logging
│   │   └── logger.go         # JSON/text logging with zap
│   └── service/              # Service management
│       ├── manager.go        # Service lifecycle manager
│       ├── event.go          # Event bus for inter-service communication
│       ├── status.go         # Service status tracking
│       └── example.go        # Example service implementation
└── go.mod                     # Go module dependencies
```

## Building

```bash
cd edge/orchestrator
go build -o orchestrator .
```

## Running

```bash
# With default config (searches common locations)
./orchestrator

# With custom config
./orchestrator -config ../config/config.dev.yaml
```

## Configuration

The orchestrator reads YAML configuration files. See `../config/config.dev.yaml` for an example.

Key configuration sections:
- `edge.orchestrator` - Orchestrator settings
- `edge.wireguard` - WireGuard client configuration
- `edge.cameras` - Camera discovery and RTSP settings
- `edge.storage` - Local storage configuration
- `edge.ai` - AI service configuration
- `edge.events` - Event management settings
- `edge.telemetry` - Telemetry collection settings
- `log` - Logging configuration

## Logging

The orchestrator supports structured logging with:
- **JSON format** (production) - Machine-readable logs
- **Text format** (development) - Human-readable logs

Log levels: `debug`, `info`, `warn`, `error`, `fatal`

## Service Architecture

### Service Manager Pattern

The orchestrator uses a service manager pattern to coordinate all services:
- **Service Registration**: Services implement the `Service` interface and register with the manager
- **Lifecycle Management**: Manager handles startup and shutdown of all services
- **Status Tracking**: Each service has status tracking (stopped, starting, running, stopping, error)
- **Ordered Shutdown**: Services are stopped in reverse order of startup

### Inter-Service Communication

Services communicate through an event bus:
- **Event Bus**: Central event bus for pub/sub communication
- **Event Types**: Predefined event types for system, camera, video, AI, storage, and network events
- **Subscriptions**: Services can subscribe to specific event types or all events
- **Non-blocking**: Event publishing is non-blocking with buffered channels

### Service Interface

```go
type Service interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Name() string
}

// Optional: Services can implement ServiceWithEvents for event support
type ServiceWithEvents interface {
    Service
    SetEventBus(bus *EventBus)
}
```

### Example Service

See `internal/service/example.go` for a complete example of:
- Implementing the Service interface
- Using the event bus for communication
- Handling events from other services
- Publishing events

### ServiceBase Helper

The `ServiceBase` struct provides common functionality:
- Event bus access
- Status tracking
- Logging helpers
- Event publishing helpers

## Graceful Shutdown

The orchestrator handles graceful shutdown:
- Listens for SIGINT/SIGTERM signals
- Stops all services in reverse order
- Waits for services to finish (30s timeout)
- Flushes logs before exit

## Health Check System

The orchestrator includes a comprehensive health check system with HTTP endpoints:

### Endpoints

- **`GET /health`** - Complete health report with all checks and service statuses
- **`GET /health/live`** - Liveness probe (is the process alive?)
- **`GET /health/ready`** - Readiness probe (is the service ready to accept traffic?)
- **`GET /health/services`** - Service status report

### Health Checkers

The system includes built-in health checkers for:
- **System** - System resources (disk, memory)
- **Database** - SQLite database connectivity
- **AI Service** - AI inference service connectivity
- **Storage** - Storage directories accessibility
- **Network** - Network connectivity

### Health Status

- **`healthy`** - All checks passing
- **`degraded`** - Some checks failing but service still functional
- **`unhealthy`** - Critical checks failing, service not functional

### Example Response

```json
{
  "status": "healthy",
  "timestamp": "2024-11-22T22:00:00Z",
  "uptime": "5m30s",
  "checks": {
    "system": {
      "name": "system",
      "status": "healthy",
      "message": "System resources OK",
      "timestamp": "2024-11-22T22:00:00Z"
    },
    "database": {
      "name": "database",
      "status": "healthy",
      "message": "Database connection OK",
      "timestamp": "2024-11-22T22:00:00Z"
    }
  },
  "services": {
    "camera-service": {
      "status": "running",
      "uptime": "5m30s",
      "error": null
    }
  }
}
```

### Kubernetes Integration

The health endpoints are designed for Kubernetes probes:
- **Liveness**: Use `/health/live` - restarts container if unhealthy
- **Readiness**: Use `/health/ready` - removes from service if not ready

## Dependencies

- `go.uber.org/zap` - Structured logging
- `gopkg.in/yaml.v3` - YAML configuration parsing
- `github.com/mattn/go-sqlite3` - SQLite database driver

