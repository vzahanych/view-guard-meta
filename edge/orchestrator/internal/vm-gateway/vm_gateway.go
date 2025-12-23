package vmgateway

import (
	"context"
	"fmt"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	httpimpl "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// VMGateway provides a unified interface for Edge↔VM bidirectional secure communication.
// It combines three core services:
//   - WireGuard client service (tunnel management)
//   - HTTPS/HTTP2 server service (VM → Edge communication)
//   - HTTPS/HTTP2 client service (Edge → VM communication)
//
// The gateway provides:
//   - Unified lifecycle management for all three services
//   - Coordinated startup and shutdown
//   - Complete API for Edge → VM communication
//   - WireGuard tunnel status and management
type VMGateway interface {
	// Lifecycle methods

	// Start starts all underlying services (WireGuard client, HTTPS server, HTTPS client).
	// Services are started in the correct order: WireGuard first, then HTTPS services.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down all underlying services.
	// Services are stopped in reverse order: HTTPS services first, then WireGuard.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// WireGuard tunnel status and management

	// IsConnected returns whether the WireGuard tunnel is connected and ready for communication.
	IsConnected() bool

	// GetWireGuardInterfaceName returns the WireGuard interface name.
	GetWireGuardInterfaceName() string

	// GetWireGuardEndpoint returns the VM endpoint address.
	GetWireGuardEndpoint() string

	// VM communication methods (Edge → VM)

	// Authenticate authenticates Edge with VM and establishes the connection.
	// This should be called when Edge orchestrator starts (after WireGuard is connected).
	Authenticate(ctx context.Context, edgeID string) error

	// GetConfig retrieves VM configuration.
	GetConfig(ctx context.Context) (*httpsclienttypes.GetConfigResponse, error)

	// SyncCapabilities syncs camera capabilities to the VM.
	SyncCapabilities(ctx context.Context, req *httpsclienttypes.SyncCapabilitiesRequest) (*httpsclienttypes.SyncCapabilitiesResponse, error)

	// ReportDeploymentStatus reports model deployment status to the VM.
	ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error

	// Heartbeat sends a heartbeat to the VM to maintain connection.
	Heartbeat(ctx context.Context, req *wgclienttypes.HeartbeatRequest) error

	// SendTelemetry sends telemetry data to the VM.
	SendTelemetry(ctx context.Context, data *wgclienttypes.TelemetryData) error

	// SendEvents sends events to the VM.
	SendEvents(ctx context.Context, events []*wgclienttypes.Event) error

	// Access to underlying services (for advanced use cases)

	// GetWGClientService returns the underlying WireGuard client service.
	// This is primarily for advanced use cases or monitoring.
	GetWGClientService() interface{}

	// GetHTTPSServerService returns the underlying HTTPS server service.
	// This is primarily for advanced use cases or monitoring.
	GetHTTPSServerService() interface{}

	// GetHTTPSClientService returns the underlying HTTPS client service.
	// This is primarily for advanced use cases or monitoring.
	GetHTTPSClientService() interface{}
}

// NewVMGateway creates a new VM gateway instance.
// This factory function should typically not be called directly;
// use VMGatewayProvider instead for proper dependency injection.
// Dependencies (meta-storage, object-storage, ai-gateway, event-bus) are injected via fx.
func NewVMGateway(
	ctx context.Context,
	config *types.VMGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (VMGateway, error) {
	switch config.Provider {
	case "http":
		return httpimpl.NewVmGatewayHttpImpl(ctx, config, metaStore, objectStore, eventBus, logger)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}
}

// VMGatewayProvider creates the VM gateway with fx lifecycle management.
// The gateway is optional and will be nil if WireGuard is disabled.
// For development, the gateway can run in localhost mode even when WireGuard is disabled.
//
// The gateway is a complex service with three internal components:
//   - WireGuard client service (tunnel management)
//   - HTTPS server service (VM → Edge communication) - requires meta-storage, event-bus, object-storage, ai-gateway
//   - HTTPS client service (Edge → VM communication)
//
// All dependencies are injected via fx at construction time.
func VMGatewayProvider(
	lc fx.Lifecycle,
	cfg *types.VMGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (VMGateway, error) {
	// Allow localhost mode for development even when WireGuard is disabled
	// Check if we have localhost endpoints configured
	isLocalhostMode := !cfg.WireGuard.Enabled &&
		(cfg.HTTPServerConfig.ListenAddress == "localhost:8443" || cfg.HTTPServerConfig.ListenAddress == "127.0.0.1:8443") &&
		(cfg.HTTPSClientConfig.VMEndpoint == "localhost:8443" || cfg.HTTPSClientConfig.VMEndpoint == "127.0.0.1:8443")

	if !cfg.WireGuard.Enabled && !isLocalhostMode {
		logger.Info("WireGuard disabled and not in localhost mode, VM gateway will not be available")
		return nil, nil
	}

	if isLocalhostMode {
		logger.Info("VM gateway running in localhost development mode (WireGuard disabled)")
	}

	// Create the gateway (this constructs all three internal components with dependencies)
	gateway, err := NewVMGateway(context.Background(), cfg, metaStore, objectStore, eventBus, logger)
	if err != nil {
		logger.Warn("Failed to create VM gateway", zap.Error(err))
		return nil, nil // Return nil instead of error to make it optional
	}

	// Setup lifecycle hooks
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if gateway != nil {
				logger.Info("Starting VM gateway (WireGuard client, HTTPS server, HTTPS client)...")
				return gateway.Start(ctx)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if gateway != nil {
				logger.Info("Stopping VM gateway...")
				return gateway.Stop(ctx)
			}
			return nil
		},
	})

	return gateway, nil
}
