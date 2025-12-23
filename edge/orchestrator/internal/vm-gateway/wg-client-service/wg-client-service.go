package wgclient

import (
	"context"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// WGClientService provides WireGuard client functionality for configuring
// Edge WireGuard client to connect to VM WireGuard server.
//
// The service provides:
//   - WireGuard interface management
//   - Connection monitoring
//   - Tunnel health checks
type WGClientService interface {
	// Start starts the WireGuard client service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the WireGuard client service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// IsConnected returns whether the tunnel is connected.
	IsConnected() bool

	// GetInterfaceName returns the WireGuard interface name.
	GetInterfaceName() string

	// GetEndpoint returns the VM endpoint.
	GetEndpoint() string
}

// NewWGClientService creates a new WireGuard client service
func NewWGClientService(cfg *types.WGClientConfig, log *zap.Logger) WGClientService {
	return impl.NewWGClientService(cfg, log)
}

// WGClientProvider creates the WireGuard client service with fx lifecycle management
func WGClientProvider(lc fx.Lifecycle, cfg *types.WGClientConfig, logger *zap.Logger) (WGClientService, error) {
	service := NewWGClientService(cfg, logger)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if service != nil {
				if err := service.Start(ctx); err != nil {
					return err
				}
			}
			logger.Info("WireGuard client service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if service != nil {
				if err := service.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("WireGuard client service stopped")
			return nil
		},
	})

	return service, nil
}
