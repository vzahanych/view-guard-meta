package tunnelclient

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service/wireguard"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// TunnelClientService is an alias for types.TunnelClientService for backward compatibility.
// Use types.TunnelClientService directly in new code.
type TunnelClientService = types.TunnelClientService

// TunnelServiceFactory creates tunnel client services based on provider type.
type TunnelServiceFactory interface {
	// Provider returns the tunnel provider type this factory supports.
	Provider() types.TunnelProvider

	// CreateService creates a tunnel client service from the given configuration.
	// Returns an error if the configuration is invalid or service creation fails.
	CreateService(cfg *types.TunnelConfig, logger *zap.Logger) (TunnelClientService, error)
}

// NewTunnelClientService creates a tunnel client service based on the tunnel configuration.
// It uses the factory pattern to select the appropriate tunnel implementation.
//
// Currently supported providers:
//   - wireguard: WireGuard tunnel implementation
//   - none: No tunnel (returns nil, for localhost/dev mode)
//
// Future providers:
//   - openvpn: OpenVPN tunnel implementation
//   - ipsec: IPSec tunnel implementation
func NewTunnelClientService(cfg *types.TunnelConfig, logger *zap.Logger) (TunnelClientService, error) {
	if cfg == nil {
		return nil, nil
	}

	if !cfg.Enabled || cfg.Provider == types.TunnelProviderNone {
		return nil, nil
	}

	// Get factory for the provider
	factory := GetTunnelFactory(cfg.Provider)
	if factory == nil {
		return nil, fmt.Errorf("unsupported tunnel provider: %s", cfg.Provider)
	}

	return factory.CreateService(cfg, logger)
}

// GetTunnelFactory returns the factory for the given tunnel provider.
// This function is used internally by NewTunnelClientService.
func GetTunnelFactory(provider types.TunnelProvider) TunnelServiceFactory {
	switch provider {
	case types.TunnelProviderWireGuard:
		return &wireguardFactory{}
	case types.TunnelProviderNone:
		return nil // No factory needed for "none"
	default:
		return nil // Unknown provider
	}
}

// wireguardFactory creates WireGuard tunnel services.
// It's defined here to avoid import cycles.
type wireguardFactory struct{}

func (f *wireguardFactory) Provider() types.TunnelProvider {
	return types.TunnelProviderWireGuard
}

func (f *wireguardFactory) CreateService(cfg *types.TunnelConfig, logger *zap.Logger) (TunnelClientService, error) {
	return wireguard.CreateService(cfg, logger)
}
