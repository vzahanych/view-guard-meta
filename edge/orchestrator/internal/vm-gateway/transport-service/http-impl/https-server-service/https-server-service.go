package httpsserver

import (
	"context"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service/impl"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"

	"go.uber.org/zap"
)

// HTTPSServerService provides HTTPS/HTTP2 server functionality for VM clients
// to connect to the Edge. The server is exposed on tunnel connections (WireGuard, OpenVPN, etc.).
//
// The service provides:
//   - HTTPS/HTTP2 server listening on tunnel interface
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
	tunnelClient tunnelclient.TunnelClientService,
	edgeID string,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	log *zap.Logger,
) (HTTPSServerService, error) {
	server := impl.NewHTTPServer(httpCfg, log, edgeID, tunnelClient, metaStore, objectStore, eventBus)
	return server, nil
}
