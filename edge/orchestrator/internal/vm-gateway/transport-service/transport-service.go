package transport

import (
	"context"
	"fmt"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	httpimpl "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"
	"go.uber.org/zap"
)

// TransportService is an alias for types.TransportService for backward compatibility.
// Use types.TransportService directly in new code.
type TransportService = types.TransportService

// TransportServiceFactory creates transport services based on provider type.
type TransportServiceFactory interface {
	// Provider returns the transport provider type this factory supports.
	Provider() types.TransportProvider

	// CreateService creates a transport service from the given configuration.
	// Returns an error if the configuration is invalid or service creation fails.
	CreateService(
		ctx context.Context,
		cfg *types.VMGatewayConfig,
		tunnelService tunnelclient.TunnelClientService,
		metaStore metastorage.MetaDataStore,
		objectStore objectstorage.ObjectStorageService,
		eventBus eventbus.EventBus,
		logger *zap.Logger,
	) (TransportService, error)
}

// NewTransportService creates a transport service based on the transport configuration.
// It uses the factory pattern to select the appropriate transport implementation.
//
// Currently supported providers:
//   - http: HTTP/HTTPS transport implementation
//
// Future providers:
//   - grpc: gRPC transport implementation
//   - websocket: WebSocket transport implementation
func NewTransportService(
	ctx context.Context,
	cfg *types.VMGatewayConfig,
	tunnelService tunnelclient.TunnelClientService,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (TransportService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("transport configuration is required")
	}

	// Get transport provider (handle backward compatibility)
	transportProvider := cfg.GetTransportProvider()
	if transportProvider == types.TransportProviderNone {
		return nil, fmt.Errorf("transport provider is required")
	}

	// Get factory for the provider
	factory := GetTransportFactory(transportProvider)
	if factory == nil {
		return nil, fmt.Errorf("unsupported transport provider: %s", transportProvider)
	}

	return factory.CreateService(ctx, cfg, tunnelService, metaStore, objectStore, eventBus, logger)
}

// GetTransportFactory returns the factory for the given transport provider.
// This function is used internally by NewTransportService.
func GetTransportFactory(provider types.TransportProvider) TransportServiceFactory {
	switch provider {
	case types.TransportProviderHTTP:
		return getHTTPFactory()
	case types.TransportProviderNone:
		return nil // No factory needed for "none"
	default:
		return nil // Unknown provider
	}
}

// httpFactory creates HTTP transport services.
// It's defined here to avoid import cycles, similar to wireguardFactory in tunnel-client-service.
type httpFactory struct{}

func (f *httpFactory) Provider() types.TransportProvider {
	return types.TransportProviderHTTP
}

func (f *httpFactory) CreateService(
	ctx context.Context,
	cfg *types.VMGatewayConfig,
	tunnelService tunnelclient.TunnelClientService,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (TransportService, error) {
	// Call the HTTP implementation's factory function
	// Now returns typed TransportService interface (no type assertion needed)
	return httpimpl.CreateHTTPTransportService(ctx, cfg, tunnelService, metaStore, objectStore, eventBus, logger)
}

// getHTTPFactory returns the HTTP factory instance.
func getHTTPFactory() TransportServiceFactory {
	return &httpFactory{}
}

