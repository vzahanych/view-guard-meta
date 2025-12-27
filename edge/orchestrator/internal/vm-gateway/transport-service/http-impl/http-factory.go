package httpimpl

import (
	"context"
	"fmt"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	httpsclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service"
	httpsserver "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"
	"go.uber.org/zap"
)

// CreateHTTPTransportService creates an HTTP transport service.
// This function is called by the httpFactory in transport-service.go.
// Returns typed TransportService interface (no type assertion needed).
func CreateHTTPTransportService(
	ctx context.Context,
	cfg *types.VMGatewayConfig,
	tunnelService tunnelclient.TunnelClientService,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (types.TransportService, error) {
	// Translate transport-agnostic config to HTTP-specific config
	httpsServerCfg := ToHTTPHTTPServerConfig(&cfg.HTTPServerConfig)
	httpsClientCfg := ToHTTPHTTPSClientConfig(&cfg.HTTPSClientConfig)

	// Construct HTTPS server from config
	httpsServerSvc, err := httpsserver.NewHTTPSServerService(
		httpsServerCfg,
		tunnelService,
		cfg.EdgeID,
		metaStore,
		objectStore,
		eventBus,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTPS server: %w", err)
	}

	// Construct HTTPS client from config
	httpsClientSvc, err := httpsclient.NewHTTPSClientService(
		httpsClientCfg,
		tunnelService,
		cfg.EdgeID,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTPS client: %w", err)
	}

	return NewHTTPTransportService(httpsServerSvc, httpsClientSvc, logger), nil
}

