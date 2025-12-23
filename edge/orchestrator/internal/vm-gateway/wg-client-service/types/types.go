package types

import "time"

// WGClientConfig contains WireGuard client configuration for establishing
// and managing secure tunnels to the KVM VM.
//
// This configuration controls:
//   - Tunnel interface setup and management
//   - Connection health monitoring
//   - Automatic reconnection behavior
//   - Network operation timeouts
type WGClientConfig struct {
	// Enabled controls whether WireGuard client is enabled.
	// When disabled, the service will skip tunnel initialization.
	Enabled bool `yaml:"enabled"`

	// ConfigPath is the file system path to the WireGuard configuration file.
	// The config file should contain [Interface] and [Peer] sections as per WireGuard specification.
	// Default: "/etc/wireguard/wg0.conf" if empty.
	// Example: "/etc/wireguard/wg0.conf"
	ConfigPath string `yaml:"config_path"`

	// KVMEndpoint is the endpoint address of the KVM VM that the Edge connects to.
	// This is typically the peer endpoint defined in the WireGuard config.
	// Example: "10.0.0.1:51820" or "vm.example.com:51820"
	KVMEndpoint string `yaml:"kvm_endpoint"`

	// InterfaceName is the name of the WireGuard network interface to create.
	// Must be a valid Linux interface name (alphanumeric, max 15 chars).
	// Default: "wg0" if empty.
	// Example: "wg0", "wg-edge", "wg-tunnel"
	InterfaceName string `yaml:"interface_name"`

	// HealthCheckInterval is the interval between health checks of the WireGuard tunnel.
	// The health check verifies the tunnel is up and measures latency.
	// Default: 10 seconds if zero.
	// Minimum recommended: 5 seconds.
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`

	// ConnectionTimeout is the maximum time to wait for initial tunnel establishment.
	// If the tunnel cannot be established within this time, Start() will fail.
	// Default: 30 seconds if zero.
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`

	// PingTimeout is the timeout for latency measurement pings.
	// Used during health checks to measure tunnel latency.
	// Default: 2 seconds if zero.
	PingTimeout time.Duration `yaml:"ping_timeout"`

	// ReconnectTimeout is the time to wait before attempting to reconnect after a tunnel failure.
	// Prevents aggressive reconnection attempts that could overwhelm the system.
	// Default: 5 seconds if zero.
	ReconnectTimeout time.Duration `yaml:"reconnect_timeout"`

	// MaxReconnectAttempts is the maximum number of consecutive reconnection attempts.
	// After this many failed attempts, the service will stop trying to reconnect.
	// Set to 0 for unlimited attempts (not recommended for production).
	// Default: 10 if zero.
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`
}

// HeartbeatRequest contains minimal heartbeat data sent from Edge to VM over HTTPS.
// This type mirrors only the fields we actually need on the HTTPS side and
// intentionally avoids a dependency on the protobuf-generated types.
type HeartbeatRequest struct {
	// Timestamp is the Unix timestamp (in seconds or milliseconds, depending on VM contract)
	// when the heartbeat was generated on the Edge.
	Timestamp int64 `json:"timestamp"`

	// EdgeId identifies the Edge node sending the heartbeat.
	EdgeId string `json:"edge_id"`
}

// TelemetryData contains minimal telemetry data sent from Edge to VM over HTTPS.
// As with HeartbeatRequest, this is a lightweight DTO independent from proto types.
type TelemetryData struct {
	// Timestamp is the time when telemetry was captured.
	Timestamp int64 `json:"timestamp"`

	// EdgeId identifies the Edge node that produced this telemetry snapshot.
	EdgeId string `json:"edge_id"`

	// Fields can be extended later as needed without touching protobufs.
}

// Event represents a generic event that can be sent to the VM over HTTPS.
// The exact schema is intentionally flexible; we only require basic metadata.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}
