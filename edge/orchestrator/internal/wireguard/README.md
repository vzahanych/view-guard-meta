# WireGuard Client

This package provides WireGuard client functionality for connecting the Edge Appliance to the KVM VM over an encrypted tunnel.

## Overview

The WireGuard client service:
- Manages WireGuard tunnel connection to KVM VM
- Handles configuration file management
- Monitors tunnel health and latency
- Provides automatic reconnection on failure
- Tracks connection state and statistics

## Features

- **Tunnel Management**: Start/stop WireGuard tunnel using `wg-quick`
- **Health Monitoring**: Periodic health checks and latency measurement
- **Automatic Reconnection**: Reconnects automatically when tunnel goes down
- **Connection State Tracking**: Tracks connection status and publishes events
- **Statistics**: Provides tunnel statistics and latency metrics

## Usage

### Initialize WireGuard Client

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"

// Create client
wgConfig := &config.WireGuardConfig{
    Enabled:     true,
    ConfigPath:  "/etc/wireguard/wg0.conf",
    KVMEndpoint: "kvm.example.com:51820",
}

client := wireguard.NewClient(wgConfig, logger)
```

### Register with Service Manager

```go
// Register with service manager
serviceManager.Register(client)

// Start all services
err := serviceManager.Start(ctx, cfg)
if err != nil {
    log.Fatal("Failed to start services", err)
}
defer serviceManager.Shutdown(ctx)
```

### Check Connection Status

```go
// Check if connected
if client.IsConnected() {
    log.Info("WireGuard tunnel is connected")
}

// Get latency
latency := client.GetLatency()
log.Info("Tunnel latency", "latency", latency)

// Get statistics
stats, err := client.GetStats()
if err != nil {
    log.Error("Failed to get stats", err)
}
log.Info("Tunnel stats", "connected", stats.Connected, "latency", stats.Latency)
```

## Configuration

Configure WireGuard in `config.yaml`:

```yaml
edge:
  wireguard:
    enabled: true
    config_path: /etc/wireguard/wg0.conf
    kvm_endpoint: kvm.example.com:51820
```

Or via environment variables:

```bash
EDGE_WIREGUARD_ENABLED=true
EDGE_WIREGUARD_CONFIG_PATH=/etc/wireguard/wg0.conf
EDGE_WIREGUARD_KVM_ENDPOINT=kvm.example.com:51820
```

## WireGuard Configuration File

The client expects a WireGuard configuration file in the standard format:

```ini
[Interface]
PrivateKey = <private-key>
Address = 10.0.0.2/24

[Peer]
PublicKey = <kvm-vm-public-key>
Endpoint = kvm.example.com:51820
AllowedIPs = 10.0.0.0/24
PersistentKeepalive = 25
```

For PoC, the client can generate a template config file, but in production:
- Configuration should come from the ISO bootstrap process
- Keys should be generated during provisioning
- Config should be injected during ISO installation

## Health Monitoring

The client monitors tunnel health every 10 seconds:
- Checks if tunnel interface is up
- Measures latency to KVM endpoint
- Publishes connection/disconnection events
- Automatically attempts reconnection on failure

## Events

The client publishes the following events:
- `EventTypeWireGuardConnected` - Tunnel connected
- `EventTypeWireGuardDisconnected` - Tunnel disconnected

## Dependencies

- **WireGuard Tools**: Requires `wg` and `wg-quick` commands to be installed
  - On Ubuntu/Debian: `apt-get install wireguard-tools`
  - On other systems: Install WireGuard tools package

## Limitations (PoC)

- Uses `wg-quick` command-line tool (not direct library usage)
- Basic health monitoring (ping-based latency)
- Simple reconnection logic
- Config file template generation (production would use ISO/bootstrap)

## Future Enhancements

- Direct library usage (`golang.zx2c4.com/wireguard`) for more control
- Advanced health monitoring (packet loss, bandwidth)
- Configurable reconnection strategies
- Integration with gRPC client for event transmission
- Support for multiple tunnels (if needed)

## Integration Points

### Current Integration
- **Service Manager**: Registered as a service, lifecycle managed by service manager
- **Event Bus**: Publishes connection/disconnection events
- **Configuration**: Uses `config.WireGuardConfig` from main config

### Future Integration (Epic 1.6.2)
- **gRPC Client**: Will use WireGuard tunnel for gRPC communication to KVM VM
- **Event Transmitter**: Will transmit events over WireGuard tunnel
- **Clip Streaming**: Will stream clips over WireGuard tunnel

## Testing

Unit tests are provided for:
- Client creation and configuration
- Config file generation
- Connection state management
- Health monitoring
- Statistics retrieval

Note: Some tests require WireGuard tools to be installed. Tests will skip or fail gracefully if tools are not available.

