package wireguard

import (
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// Config contains WireGuard-specific client configuration.
// It embeds the common TunnelBaseConfig and adds WireGuard-specific fields.
type Config struct {
	types.TunnelBaseConfig `yaml:",inline" json:",inline"`

	// ConfigPath is the file system path to the WireGuard configuration file.
	// The config file should contain [Interface] and [Peer] sections as per WireGuard specification.
	// Default: "/etc/wireguard/wg0.conf" if empty.
	// Example: "/etc/wireguard/wg0.conf"
	ConfigPath string `yaml:"config_path"`
}
