package wireguard

import (
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// CreateService creates a WireGuard tunnel client service from the given configuration.
// This function is called by the tunnel factory in the parent package.
// Returns typed TunnelClientService interface (no type assertion needed).
func CreateService(cfg *types.TunnelConfig, logger *zap.Logger) (types.TunnelClientService, error) {
	// Extract WireGuard-specific config from TunnelConfig
	wgCfg, err := cfg.UnmarshalWireGuardConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal WireGuard config: %w", err)
	}

	// Convert to WireGuard Config type
	// Use defaults for missing fields
	pingTimeout := wgCfg.PingTimeout
	if pingTimeout == 0 {
		pingTimeout = 2 * time.Second // Default
	}

	wireguardCfg := &Config{
		TunnelBaseConfig: types.TunnelBaseConfig{
			Enabled:              cfg.Enabled,
			KVMEndpoint:          wgCfg.KVMEndpoint,
			InterfaceName:        wgCfg.InterfaceName,
			HealthCheckInterval:  wgCfg.HealthCheckInterval,
			ConnectionTimeout:    wgCfg.ConnectionTimeout,
			PingTimeout:          pingTimeout,
			ReconnectTimeout:     wgCfg.ReconnectTimeout,
			MaxReconnectAttempts: wgCfg.MaxReconnectAttempts,
		},
		ConfigPath: wgCfg.ConfigPath,
	}

	// Create WireGuard service directly (service implements TunnelClientService)
	svc := NewService(wireguardCfg, logger)
	return svc, nil
}
