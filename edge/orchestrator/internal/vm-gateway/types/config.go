package types

import (
	"fmt"
	"strings"
	"time"
)

// TransportProvider represents the type of transport implementation.
type TransportProvider string

const (
	// TransportProviderHTTP uses HTTP/HTTPS for transport.
	TransportProviderHTTP TransportProvider = "http"
	// TransportProviderGRPC uses gRPC for transport (future).
	TransportProviderGRPC TransportProvider = "grpc"
	// TransportProviderWebSocket uses WebSocket for transport (future).
	TransportProviderWebSocket TransportProvider = "websocket"
	// TransportProviderNone indicates no transport (for testing/dev mode).
	TransportProviderNone TransportProvider = "none"
)

// Config types for VM Gateway sub-services.
// These are transport-agnostic configuration types that can be used
// by any transport implementation (HTTP, gRPC, etc.).

// WireGuardConfig contains WireGuard client configuration.
type WireGuardConfig struct {
	// Enabled controls whether WireGuard client is enabled.
	Enabled bool `yaml:"enabled"`

	// ConfigPath is the file system path to the WireGuard configuration file.
	ConfigPath string `yaml:"config_path"`

	// KVMEndpoint is the endpoint address of the KVM VM that the Edge connects to.
	KVMEndpoint string `yaml:"kvm_endpoint"`

	// InterfaceName is the name of the WireGuard network interface to create.
	InterfaceName string `yaml:"interface_name"`

	// HealthCheckInterval is the interval between health checks of the WireGuard tunnel.
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`

	// ConnectionTimeout is the maximum time to wait for initial tunnel establishment.
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`

	// PingTimeout is the timeout for latency measurement pings.
	PingTimeout time.Duration `yaml:"ping_timeout"`

	// ReconnectTimeout is the time to wait before attempting to reconnect after a tunnel failure.
	ReconnectTimeout time.Duration `yaml:"reconnect_timeout"`

	// MaxReconnectAttempts is the maximum number of consecutive reconnection attempts.
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`
}

// HTTPServerConfig contains configuration for the HTTPS server.
type HTTPServerConfig struct {
	// ListenAddress is the address the HTTPS server listens on.
	ListenAddress string `yaml:"listen_address"`

	// ServerCertPath is the path to the server certificate used for TLS.
	ServerCertPath string `yaml:"server_cert_path"`

	// ServerKeyPath is the path to the server private key used for TLS.
	ServerKeyPath string `yaml:"server_key_path"`

	// CACertPath is the path to the CA certificate used to verify client certificates (for mTLS).
	CACertPath string `yaml:"ca_cert_path"`

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// IdleTimeout is the maximum amount of time to wait for the next request when keep-alives are enabled.
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// TunnelInterfaceWaitTimeout is the maximum time to wait for tunnel interface to be ready.
	// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
	TunnelInterfaceWaitTimeout time.Duration `yaml:"tunnel_interface_wait_timeout"`

	// TunnelInterfaceCheckInterval is the interval between checks for tunnel interface readiness.
	// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
	TunnelInterfaceCheckInterval time.Duration `yaml:"tunnel_interface_check_interval"`

	// MultipartFormMaxMemory is the maximum memory size for multipart form parsing.
	MultipartFormMaxMemory int64 `yaml:"multipart_form_max_memory"`
}

// HTTPSClientConfig contains configuration for the HTTPS client.
type HTTPSClientConfig struct {
	// VMEndpoint is the HTTPS endpoint of the VM.
	VMEndpoint string `yaml:"vm_endpoint"`

	// ClientCertPath is the path to the client certificate used for mTLS.
	ClientCertPath string `yaml:"client_cert_path"`

	// ClientKeyPath is the path to the client private key used for mTLS.
	ClientKeyPath string `yaml:"client_key_path"`

	// CACertPath is the path to the CA certificate used to verify the VM certificate.
	CACertPath string `yaml:"ca_cert_path"`

	// Timeout is the HTTP request timeout.
	Timeout time.Duration `yaml:"timeout"`
}

// VMGatewayConfig contains transport-agnostic VM gateway configuration.
// Transport-specific configs (e.g., HTTP) are parsed separately and converted
// to the transport-agnostic types defined in this package.
type VMGatewayConfig struct {
	// TransportProvider specifies the transport implementation (e.g., "http").
	// This field is the primary way to specify the transport.
	TransportProvider TransportProvider `yaml:"transport_provider"`

	// Tunnel contains tunnel-agnostic configuration.
	// This is the only way to configure tunnels (WireGuard, OpenVPN, IPSec, etc.).
	// Use `provider: wireguard` in Tunnel config for WireGuard tunnels.
	Tunnel TunnelConfig `yaml:"tunnel"`

	// HTTPServerConfig contains HTTPS server configuration.
	HTTPServerConfig HTTPServerConfig `yaml:"https_server_config"`

	// HTTPSClientConfig contains HTTPS client configuration.
	HTTPSClientConfig HTTPSClientConfig `yaml:"https_client_config"`

	// EdgeID is the unique identifier for this Edge node.
	EdgeID string `yaml:"edge_id"`
}

// GetTransportProvider returns the transport provider.
func (c *VMGatewayConfig) GetTransportProvider() TransportProvider {
	if c.TransportProvider != "" {
		return c.TransportProvider
	}
	return TransportProviderNone
}

// GetTunnelConfig returns the tunnel configuration.
// Returns nil if no tunnel is configured (for localhost/dev mode).
func (c *VMGatewayConfig) GetTunnelConfig() *TunnelConfig {
	// Return tunnel config if present and configured
	if c.Tunnel.Provider != "" || c.Tunnel.Enabled {
		return &c.Tunnel
	}

	// No tunnel configured - return nil (for localhost/dev mode)
	return nil
}

// Validate validates the VM gateway configuration and returns an error if invalid.
// This implements "shift-left" validation - config errors are caught at parse time.
func (c *VMGatewayConfig) Validate() error {
	// Validate transport provider
	if c.TransportProvider == "" {
		return fmt.Errorf("transport_provider is required")
	}

	if c.TransportProvider != TransportProviderHTTP {
		return fmt.Errorf("unsupported transport provider: %s (only 'http' is supported)", c.TransportProvider)
	}

	if c.EdgeID == "" {
		return fmt.Errorf("edge_id is required")
	}

	// Get tunnel config
	tunnelCfg := c.GetTunnelConfig()

	// Validate tunnel configuration if present
	tunnelEnabled := false
	if tunnelCfg != nil {
		// Validate tunnel configuration
		if err := tunnelCfg.Validate(); err != nil {
			return fmt.Errorf("tunnel configuration validation failed: %w", err)
		}

		tunnelEnabled = tunnelCfg.Enabled

		// If tunnel is enabled, validate required fields
		if tunnelEnabled {
			if tunnelCfg.KVMEndpoint == "" {
				return fmt.Errorf("tunnel.kvm_endpoint is required when tunnel is enabled")
			}
		}
	}

	// When tunnel is disabled, we allow localhost mode for development
	// but require explicit localhost configuration (not 0.0.0.0 for security)
	if !tunnelEnabled {
		serverAddr := c.HTTPServerConfig.ListenAddress
		isLocalhostServer := serverAddr == "" || // Empty defaults to localhost
			serverAddr == "localhost:8443" ||
			serverAddr == "127.0.0.1:8443"

		clientEndpoint := c.HTTPSClientConfig.VMEndpoint
		isLocalhostClient := clientEndpoint == "" || // Empty defaults to localhost
			strings.HasPrefix(clientEndpoint, "localhost:") ||
			strings.HasPrefix(clientEndpoint, "127.0.0.1:")

		if !isLocalhostServer || !isLocalhostClient {
			return fmt.Errorf("tunnel is disabled but configuration is not in localhost mode (server: %s, client: %s). "+
				"For production, enable tunnel. For development, use localhost addresses",
				serverAddr, clientEndpoint)
		}
	}

	// Validate HTTPS server configuration
	if c.HTTPServerConfig.ServerCertPath == "" {
		return fmt.Errorf("https_server_config.server_cert_path is required")
	}
	if c.HTTPServerConfig.ServerKeyPath == "" {
		return fmt.Errorf("https_server_config.server_key_path is required")
	}

	// Validate HTTPS client configuration
	if c.HTTPSClientConfig.VMEndpoint == "" {
		return fmt.Errorf("https_client_config.vm_endpoint is required")
	}
	if c.HTTPSClientConfig.ClientCertPath == "" {
		return fmt.Errorf("https_client_config.client_cert_path is required")
	}
	if c.HTTPSClientConfig.ClientKeyPath == "" {
		return fmt.Errorf("https_client_config.client_key_path is required")
	}
	if c.HTTPSClientConfig.CACertPath == "" {
		return fmt.Errorf("https_client_config.ca_cert_path is required")
	}

	return nil
}
