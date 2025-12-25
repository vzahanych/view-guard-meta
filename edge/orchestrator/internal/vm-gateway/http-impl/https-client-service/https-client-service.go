package httpsclient

import (
	"context"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	wgclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// HTTPSClientService provides HTTPS/HTTP2 client functionality for the Edge
// to make requests to VM devices over WireGuard connections.
//
// The service provides:
//   - HTTPS/HTTP2 client for Edge → VM communication
//   - mTLS support for secure VM authentication
//   - Request routing over WireGuard tunnel
type HTTPSClientService interface {
	// Start starts the HTTPS client service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the HTTPS client service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// IsConnected returns whether the connection is ready (WireGuard is connected).
	IsConnected() bool

	// VM communication methods (Edge → VM)
	Authenticate(ctx context.Context, edgeID string) error
	GetConfig(ctx context.Context) (*types.GetConfigResponse, error)
	SyncCapabilities(ctx context.Context, req *types.SyncCapabilitiesRequest) (*types.SyncCapabilitiesResponse, error)
	SyncCameras(ctx context.Context, req *types.SyncCamerasRequest) (*types.SyncCamerasResponse, error)
	SyncScreenshots(ctx context.Context, req *types.SyncScreenshotsRequest) (*types.SyncScreenshotsResponse, error)
	ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error
	Heartbeat(ctx context.Context, req *wgclienttypes.HeartbeatRequest) error
	SendTelemetry(ctx context.Context, data *wgclienttypes.TelemetryData) error
	SendEvents(ctx context.Context, events []*wgclienttypes.Event) error
}

// NewHTTPSClientService creates a new HTTPS client service
func NewHTTPSClientService(clientCfg *types.HTTPSClientConfig, wgCfg *wgclienttypes.WGClientConfig, wgClient wgclient.WGClientService, edgeID string, log *zap.Logger) (HTTPSClientService, error) {
	return impl.NewHTTPSClient(clientCfg, wgCfg, wgClient, edgeID, log)
}

// HTTPSClientProvider creates the HTTPS client service with fx lifecycle management
func HTTPSClientProvider(
	lc fx.Lifecycle,
	clientCfg *types.HTTPSClientConfig,
	wgCfg *wgclienttypes.WGClientConfig,
	wgClient wgclient.WGClientService,
	edgeID string,
	logger *zap.Logger,
) (HTTPSClientService, error) {
	service, err := NewHTTPSClientService(clientCfg, wgCfg, wgClient, edgeID, logger)
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
			logger.Info("HTTPS client service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if service != nil {
				if err := service.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("HTTPS client service stopped")
			return nil
		},
	})

	return service, nil
}
