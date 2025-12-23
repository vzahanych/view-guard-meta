package httpsserver

import (
	"context"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/types"
	wgclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// HTTPSServerService provides HTTPS/HTTP2 server functionality for VM clients
// to connect to the Edge. The server is exposed only on WireGuard connections.
//
// The service provides:
//   - HTTPS/HTTP2 server listening on WireGuard interface
//   - VM authentication and connection management
//   - REST API endpoints for VM → Edge communication
type HTTPSServerService interface {
	// Start starts the HTTPS/HTTP2 server.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the HTTPS/HTTP2 server.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string
}

// NewHTTPSServerService constructs the Edge HTTPS server implementation.
func NewHTTPSServerService(
	httpCfg *httpsservertypes.HTTPServerConfig,
	wgCfg *wgclienttypes.WGClientConfig,
	wgClient wgclient.WGClientService,
	edgeID string,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	log *zap.Logger,
) (HTTPSServerService, error) {
	server := impl.NewHTTPServer(httpCfg, wgCfg, log, edgeID, wgClient, metaStore, objectStore, eventBus)
	return server, nil
}

// HTTPSServerProvider creates the HTTPS server service with fx lifecycle management.
// Dependencies (meta-storage, object-storage, ai-gateway, event-bus) are injected via fx.
func HTTPSServerProvider(
	lc fx.Lifecycle,
	httpCfg *httpsservertypes.HTTPServerConfig,
	wgCfg *wgclienttypes.WGClientConfig,
	wgClient wgclient.WGClientService,
	edgeID string,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (HTTPSServerService, error) {
	service, err := NewHTTPSServerService(httpCfg, wgCfg, wgClient, edgeID, metaStore, objectStore, eventBus, logger)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if service != nil {
				if err := service.Start(ctx); err != nil {
					return err
				}
			}
			logger.Info("HTTPS server service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if service != nil {
				if err := service.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("HTTPS server service stopped")
			return nil
		},
	})

	return service, nil
}
