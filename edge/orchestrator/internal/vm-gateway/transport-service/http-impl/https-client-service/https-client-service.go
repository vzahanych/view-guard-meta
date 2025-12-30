package httpsclient

import (
	"context"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/impl"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"

	"go.uber.org/zap"
)

// HTTPSClientService provides HTTPS/HTTP2 client functionality for the Edge
// to make requests to VM devices over tunnel connections.
//
// The service provides:
//   - HTTPS/HTTP2 client for Edge → VM communication
//   - mTLS support for secure VM authentication
//   - Request routing over tunnel (WireGuard, OpenVPN, etc.)
type HTTPSClientService interface {
	// Start starts the HTTPS client service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the HTTPS client service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// IsConnected returns whether the connection is ready (tunnel is connected).
	IsConnected() bool

	// VM communication methods (Edge → VM)
	Authenticate(ctx context.Context, edgeID string) error
	GetConfig(ctx context.Context) (*vmgatewaytypes.GetConfigResponse, error)
	SyncCapabilities(ctx context.Context, req *vmgatewaytypes.SyncCapabilitiesRequest) (*vmgatewaytypes.SyncCapabilitiesResponse, error)
	SyncDevices(ctx context.Context, req *vmgatewaytypes.SyncDevicesRequest) (*vmgatewaytypes.SyncDevicesResponse, error)
	SyncDataUnits(ctx context.Context, req *vmgatewaytypes.SyncDataUnitsRequest) (*vmgatewaytypes.SyncDataUnitsResponse, error)
	SyncAuditLogs(ctx context.Context, req *vmgatewaytypes.SyncAuditLogsRequest) (*vmgatewaytypes.SyncAuditLogsResponse, error)
	ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error
	Heartbeat(ctx context.Context, req *vmgatewaytypes.HeartbeatRequest) error
	SendTelemetry(ctx context.Context, data *vmgatewaytypes.TelemetryData) error
	SendEvents(ctx context.Context, events []*vmgatewaytypes.Event) error
}

// NewHTTPSClientService creates a new HTTPS client service
func NewHTTPSClientService(clientCfg *httpsclienttypes.HTTPSClientConfig, tunnelClient tunnelclient.TunnelClientService, edgeID string, eventBus eventbus.EventBus, log *zap.Logger) (HTTPSClientService, error) {
	return impl.NewHTTPSClient(clientCfg, tunnelClient, edgeID, eventBus, log)
}
