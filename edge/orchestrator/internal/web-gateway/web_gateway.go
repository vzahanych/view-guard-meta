package webgateway

import (
	"context"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// WebGateway provides a unified interface for Edge UI web server.
// It includes:
//   - HTTPS/HTTP server (serves UI and static files)
//   - REST API endpoints for UI (cameras, events, screenshots, metrics, etc.)
//
// The gateway provides:
//   - Unified lifecycle management
//   - Coordinated startup and shutdown
//   - All API endpoints for the Edge UI
type WebGateway interface {
	// Start starts the web gateway server.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the web gateway server.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string
}

// NewWebGateway creates a new WebGateway instance.
// This factory function should typically not be called directly;
// use WebGatewayProvider instead for proper dependency injection.
func NewWebGateway(cfg *types.WebGatewayConfig, logger *zap.Logger) (WebGateway, error) {
	return impl.NewWebGateway(cfg, logger)
}

// WebGatewayProvider creates the WebGateway with fx lifecycle management.
func WebGatewayProvider(
	lc fx.Lifecycle,
	cfg *types.WebGatewayConfig,
	logger *zap.Logger,
) (WebGateway, error) {
	gateway, err := NewWebGateway(cfg, logger)
	if err != nil {
		return nil, err
	}

	// Setup lifecycle hooks
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if gateway != nil {
				if err := gateway.Start(ctx); err != nil {
					return err
				}
				logger.Info("Web gateway started")
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if gateway != nil {
				if err := gateway.Stop(ctx); err != nil {
					return err
				}
				logger.Info("Web gateway stopped")
			}
			return nil
		},
	})

	return gateway, nil
}
