package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// TunnelProvider represents the type of tunnel implementation.
type TunnelProvider string

const (
	// TunnelProviderWireGuard uses WireGuard for secure tunneling.
	TunnelProviderWireGuard TunnelProvider = "wireguard"
	// TunnelProviderOpenVPN uses OpenVPN for secure tunneling (future).
	TunnelProviderOpenVPN TunnelProvider = "openvpn"
	// TunnelProviderIPSec uses IPSec for secure tunneling (future).
	TunnelProviderIPSec TunnelProvider = "ipsec"
	// TunnelProviderNone indicates no tunnel (localhost/dev mode).
	TunnelProviderNone TunnelProvider = "none"
)

// TunnelConfig is a transport-agnostic tunnel configuration.
// It uses a provider pattern to support multiple tunnel implementations.
//
// The actual tunnel-specific configuration is stored in RawConfig as JSON,
// which is parsed by the specific tunnel implementation.
type TunnelConfig struct {
	// Provider specifies which tunnel implementation to use.
	// Valid values: "wireguard", "openvpn", "ipsec", "none"
	Provider TunnelProvider `yaml:"provider"`

	// Enabled controls whether the tunnel is enabled.
	// When false, the tunnel service will not be started.
	Enabled bool `yaml:"enabled"`

	// RawConfig contains tunnel-specific configuration as raw YAML/JSON.
	// This is parsed by the specific tunnel implementation.
	// For WireGuard, this would contain WireGuard-specific fields.
	// For OpenVPN, this would contain OpenVPN-specific fields.
	// This allows tunnel-specific configs to live in their service packages.
	RawConfig map[string]interface{} `yaml:"raw_config,omitempty"`

	// Common tunnel configuration fields (shared across all tunnel types)
	// These are extracted for convenience and validation at the top level.
	KVMEndpoint string        `yaml:"kvm_endpoint,omitempty"` // VM endpoint address
	InterfaceName string     `yaml:"interface_name,omitempty"` // Network interface name
	HealthCheckInterval time.Duration `yaml:"health_check_interval,omitempty"`
	ConnectionTimeout   time.Duration `yaml:"connection_timeout,omitempty"`
	ReconnectTimeout    time.Duration `yaml:"reconnect_timeout,omitempty"`
	MaxReconnectAttempts int          `yaml:"max_reconnect_attempts,omitempty"`
}

// Validate validates the tunnel configuration.
func (c *TunnelConfig) Validate() error {
	if c.Provider == "" {
		return fmt.Errorf("tunnel provider is required")
	}

	validProviders := map[TunnelProvider]bool{
		TunnelProviderWireGuard: true,
		TunnelProviderOpenVPN:   true,
		TunnelProviderIPSec:     true,
		TunnelProviderNone:      true,
	}

	if !validProviders[c.Provider] {
		return fmt.Errorf("unsupported tunnel provider: %s (supported: wireguard, openvpn, ipsec, none)", c.Provider)
	}

	// If enabled, provider must not be "none"
	if c.Enabled && c.Provider == TunnelProviderNone {
		return fmt.Errorf("tunnel cannot be enabled with provider 'none'")
	}

	// If disabled, provider can be "none" or any other (for future enablement)
	if !c.Enabled && c.Provider != TunnelProviderNone {
		// This is OK - tunnel is configured but disabled
	}

	return nil
}

// UnmarshalWireGuardConfig extracts WireGuard-specific config from RawConfig.
// This is a helper for WireGuard implementation to parse its specific fields.
// Other tunnel implementations would have similar helpers.
func (c *TunnelConfig) UnmarshalWireGuardConfig() (*WireGuardTunnelConfig, error) {
	if c.Provider != TunnelProviderWireGuard {
		return nil, fmt.Errorf("cannot unmarshal WireGuard config from provider: %s", c.Provider)
	}

	// Convert RawConfig to JSON, then unmarshal into WireGuardTunnelConfig
	jsonData, err := json.Marshal(c.RawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw config: %w", err)
	}

	var wgCfg WireGuardTunnelConfig
	if err := json.Unmarshal(jsonData, &wgCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal WireGuard config: %w", err)
	}

	// Override with common fields if provided at top level
	if c.KVMEndpoint != "" {
		wgCfg.TunnelBaseConfig.KVMEndpoint = c.KVMEndpoint
	}
	if c.InterfaceName != "" {
		wgCfg.TunnelBaseConfig.InterfaceName = c.InterfaceName
	}
	if c.HealthCheckInterval != 0 {
		wgCfg.TunnelBaseConfig.HealthCheckInterval = c.HealthCheckInterval
	}
	if c.ConnectionTimeout != 0 {
		wgCfg.TunnelBaseConfig.ConnectionTimeout = c.ConnectionTimeout
	}
	// PingTimeout is not in TunnelConfig, it's only in WireGuardTunnelConfig
	// It will be set from RawConfig during unmarshal
	if c.ReconnectTimeout != 0 {
		wgCfg.TunnelBaseConfig.ReconnectTimeout = c.ReconnectTimeout
	}
	if c.MaxReconnectAttempts != 0 {
		wgCfg.TunnelBaseConfig.MaxReconnectAttempts = c.MaxReconnectAttempts
	}
	if c.Enabled {
		wgCfg.TunnelBaseConfig.Enabled = c.Enabled
	}

	return &wgCfg, nil
}

// TunnelBaseConfig contains common tunnel configuration fields that are shared across
// all tunnel implementations (WireGuard, OpenVPN, IPSec, etc.).
// Tunnel-specific implementations can embed this struct and add their own fields.
type TunnelBaseConfig struct {
	// Enabled controls whether the tunnel is enabled.
	// When disabled, the service will skip tunnel initialization.
	Enabled bool `yaml:"enabled"`

	// KVMEndpoint is the endpoint address of the KVM VM that the Edge connects to.
	// This is the remote peer endpoint for the tunnel.
	// Example: "10.0.0.1:51820" or "vm.example.com:51820"
	KVMEndpoint string `yaml:"kvm_endpoint"`

	// InterfaceName is the name of the tunnel network interface to create.
	// Must be a valid Linux interface name (alphanumeric, max 15 chars).
	// Default: implementation-specific (e.g., "wg0" for WireGuard)
	// Example: "wg0", "tun0", "ipsec0"
	InterfaceName string `yaml:"interface_name"`

	// HealthCheckInterval is the interval between health checks of the tunnel.
	// The health check verifies the tunnel is up and measures latency.
	// Default: 10 seconds if zero.
	// Minimum recommended: 5 seconds.
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`

	// ConnectionTimeout is the maximum time to wait for initial tunnel establishment.
	// If the tunnel cannot be established within this time, Start() will fail.
	// Default: 30 seconds if zero.
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`

	// PingTimeout is the timeout for latency measurement pings during health checks.
	// Used to measure tunnel latency.
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

// WireGuardTunnelConfig contains WireGuard-specific configuration.
// It embeds TunnelBaseConfig and adds WireGuard-specific fields.
type WireGuardTunnelConfig struct {
	TunnelBaseConfig `yaml:",inline" json:",inline"`

	// ConfigPath is the file system path to the WireGuard configuration file.
	// The config file should contain [Interface] and [Peer] sections as per WireGuard specification.
	// Default: "/etc/wireguard/wg0.conf" if empty.
	ConfigPath string `yaml:"config_path" json:"config_path"`
}

// OpenVPNTunnelConfig contains OpenVPN-specific configuration (future).
// It embeds TunnelBaseConfig and adds OpenVPN-specific fields.
type OpenVPNTunnelConfig struct {
	TunnelBaseConfig `yaml:",inline" json:",inline"`

	// ConfigPath is the file system path to the OpenVPN configuration file.
	ConfigPath string `yaml:"config_path" json:"config_path"`
	// OpenVPN-specific fields would go here
}

// IPSecTunnelConfig contains IPSec-specific configuration (future).
// It embeds TunnelBaseConfig and adds IPSec-specific fields.
type IPSecTunnelConfig struct {
	TunnelBaseConfig `yaml:",inline" json:",inline"`

	// ConfigPath is the file system path to the IPSec configuration file.
	ConfigPath string `yaml:"config_path" json:"config_path"`
	// IPSec-specific fields would go here
}

// NewTunnelConfigFromWireGuard creates a TunnelConfig from WireGuardConfig.
// This is a helper function for backward compatibility during migration.
func NewTunnelConfigFromWireGuard(wgCfg *WireGuardConfig) *TunnelConfig {
	if wgCfg == nil {
		return nil
	}

	// Convert WireGuardConfig to WireGuardTunnelConfig for RawConfig
	wgTunnelCfg := &WireGuardTunnelConfig{
		TunnelBaseConfig: TunnelBaseConfig{
			Enabled:              wgCfg.Enabled,
			KVMEndpoint:          wgCfg.KVMEndpoint,
			InterfaceName:        wgCfg.InterfaceName,
			HealthCheckInterval:  wgCfg.HealthCheckInterval,
			ConnectionTimeout:    wgCfg.ConnectionTimeout,
			PingTimeout:          wgCfg.PingTimeout,
			ReconnectTimeout:     wgCfg.ReconnectTimeout,
			MaxReconnectAttempts: wgCfg.MaxReconnectAttempts,
		},
		ConfigPath: wgCfg.ConfigPath,
	}

	// Marshal to JSON for RawConfig
	jsonData, _ := json.Marshal(wgTunnelCfg)
	var rawConfig map[string]interface{}
	json.Unmarshal(jsonData, &rawConfig)

	return &TunnelConfig{
		Provider:             TunnelProviderWireGuard,
		Enabled:              wgCfg.Enabled,
		RawConfig:            rawConfig,
		KVMEndpoint:          wgCfg.KVMEndpoint,
		InterfaceName:        wgCfg.InterfaceName,
		HealthCheckInterval:  wgCfg.HealthCheckInterval,
		ConnectionTimeout:   wgCfg.ConnectionTimeout,
		ReconnectTimeout:     wgCfg.ReconnectTimeout,
		MaxReconnectAttempts: wgCfg.MaxReconnectAttempts,
	}
}

