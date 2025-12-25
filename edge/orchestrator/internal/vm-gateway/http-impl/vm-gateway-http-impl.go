package httpimpl

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	httpsclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	httpsserver "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-server-service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	wgclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"
)

// vmGateway implements the VMGateway interface
type VmGatewayHttpImpl struct {
	wgClientService    wgclient.WGClientService
	httpsServerService httpsserver.HTTPSServerService
	httpsClientService httpsclient.HTTPSClientService // Use interface instead of concrete type
	wgCfg              *wgclienttypes.WGClientConfig
	logger             *zap.Logger
	mu                 sync.RWMutex
	started            bool
}

// NewVMGateway creates a new VMGateway implementation that composes
// WireGuard client, HTTPS server, and HTTPS client services.
// All services are constructed from the provided configuration.
// Dependencies (meta-storage, object-storage, ai-gateway, event-bus) are injected via fx.
func NewVmGatewayHttpImpl(
	ctx context.Context,
	cfg *types.VMGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (*VmGatewayHttpImpl, error) {
	// Step 1: Construct WireGuard client from config
	// Use the provider pattern instead of direct impl
	wgClientSvc := wgclient.NewWGClientService(&cfg.WireGuard, logger)

	// Step 2: Construct HTTPS server from config
	httpsServerSvc, err := httpsserver.NewHTTPSServerService(
		&cfg.HTTPServerConfig,
		&cfg.WireGuard,
		wgClientSvc,
		cfg.EdgeID,
		metaStore,
		objectStore,
		eventBus,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTPS server: %w", err)
	}

	// Step 3: Construct HTTPS client from config
	httpsClientSvc, err := httpsclient.NewHTTPSClientService(
		&cfg.HTTPSClientConfig,
		&cfg.WireGuard,
		wgClientSvc,
		cfg.EdgeID,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTPS client: %w", err)
	}

	return &VmGatewayHttpImpl{
		wgClientService:    wgClientSvc,
		httpsServerService: httpsServerSvc,
		httpsClientService: httpsClientSvc,
		wgCfg:              &cfg.WireGuard,
		logger:             logger,
	}, nil
}

// Name returns the service name
func (g *VmGatewayHttpImpl) Name() string {
	return "vm-gateway"
}

// Start starts all underlying services in the correct order
func (g *VmGatewayHttpImpl) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.started {
		return fmt.Errorf("vm gateway is already started")
	}

	if g.logger != nil {
		g.logger.Info("Starting VM Gateway (all services)")
	}

	// Step 1: Start WireGuard client first (required for HTTPS services in production)
	// Skip WireGuard if disabled (for localhost dev mode)
	if g.wgClientService != nil && g.wgCfg != nil && g.wgCfg.Enabled {
		if g.logger != nil {
			g.logger.Info("Starting WireGuard client service...")
		}
		if err := g.wgClientService.Start(ctx); err != nil {
			return fmt.Errorf("failed to start WireGuard client service: %w", err)
		}
	} else if g.wgCfg != nil && !g.wgCfg.Enabled {
		if g.logger != nil {
			g.logger.Info("WireGuard disabled - skipping WireGuard client startup (localhost dev mode)")
		}
	}

	// Step 2: Start HTTPS server (depends on WireGuard in production, localhost for dev)
	if g.logger != nil {
		g.logger.Info("Starting HTTPS server service...")
	}
	if g.httpsServerService != nil {
		if err := g.httpsServerService.Start(ctx); err != nil {
			// Try to stop WireGuard if HTTPS server fails (only if it was started)
			if g.wgClientService != nil && g.wgCfg != nil && g.wgCfg.Enabled {
				_ = g.wgClientService.Stop(ctx)
			}
			return fmt.Errorf("failed to start HTTPS server service: %w", err)
		}
	}

	// Step 3: Start HTTPS client (depends on WireGuard in production, localhost for dev)
	if g.logger != nil {
		g.logger.Info("Starting HTTPS client service...")
	}
	if g.httpsClientService != nil {
		if err := g.httpsClientService.Start(ctx); err != nil {
			// Try to stop already started services
			if g.httpsServerService != nil {
				_ = g.httpsServerService.Stop(ctx)
			}
			if g.wgClientService != nil && g.wgCfg != nil && g.wgCfg.Enabled {
				_ = g.wgClientService.Stop(ctx)
			}
			return fmt.Errorf("failed to start HTTPS client service: %w", err)
		}
	}

	g.started = true

	if g.logger != nil {
		g.logger.Info("VM Gateway started successfully (all services running)")
	}

	return nil
}

// Stop stops all underlying services in reverse order
func (g *VmGatewayHttpImpl) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.started {
		return nil // Already stopped
	}

	if g.logger != nil {
		g.logger.Info("Stopping VM Gateway (all services)")
	}

	var errors []error

	// Stop HTTPS client first
	if g.logger != nil {
		g.logger.Info("Stopping HTTPS client service...")
	}
	if g.httpsClientService != nil {
		if err := g.httpsClientService.Stop(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop HTTPS client service: %w", err))
		}
	}

	// Stop HTTPS server
	if g.logger != nil {
		g.logger.Info("Stopping HTTPS server service...")
	}
	if g.httpsServerService != nil {
		if err := g.httpsServerService.Stop(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop HTTPS server service: %w", err))
		}
	}

	// Stop WireGuard client last
	if g.logger != nil {
		g.logger.Info("Stopping WireGuard client service...")
	}
	if g.wgClientService != nil {
		if err := g.wgClientService.Stop(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop WireGuard client service: %w", err))
		}
	}

	g.started = false

	if len(errors) > 0 {
		if g.logger != nil {
			g.logger.Error("Some services failed to stop", zap.Errors("errors", errors))
		}
		return fmt.Errorf("errors stopping services: %v", errors)
	}

	if g.logger != nil {
		g.logger.Info("VM Gateway stopped successfully")
	}

	return nil
}

// GetWGClientService returns the underlying WireGuard client service
func (g *VmGatewayHttpImpl) GetWGClientService() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.wgClientService
}

// GetHTTPSServerService returns the underlying HTTPS server service
func (g *VmGatewayHttpImpl) GetHTTPSServerService() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.httpsServerService
}

// GetHTTPSClientService returns the underlying HTTPS client service
func (g *VmGatewayHttpImpl) GetHTTPSClientService() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.httpsClientService
}

// WireGuard tunnel status methods

// IsConnected returns whether the WireGuard tunnel is connected
func (g *VmGatewayHttpImpl) IsConnected() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.wgClientService != nil && g.wgClientService.IsConnected()
}

// IsHTTPConnected returns whether the HTTP/HTTPS client connection to VM is established and authenticated
func (g *VmGatewayHttpImpl) IsHTTPConnected() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return false
	}
	return g.httpsClientService.IsConnected()
}

// GetWireGuardInterfaceName returns the WireGuard interface name
func (g *VmGatewayHttpImpl) GetWireGuardInterfaceName() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.wgClientService == nil {
		return ""
	}
	return g.wgClientService.GetInterfaceName()
}

// GetWireGuardEndpoint returns the VM endpoint address
func (g *VmGatewayHttpImpl) GetWireGuardEndpoint() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.wgClientService == nil {
		return ""
	}
	return g.wgClientService.GetEndpoint()
}

// VM communication methods (Edge → VM)

// Authenticate authenticates Edge with VM
func (g *VmGatewayHttpImpl) Authenticate(ctx context.Context, edgeID string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.Authenticate(ctx, edgeID)
}

// GetConfig retrieves VM configuration
func (g *VmGatewayHttpImpl) GetConfig(ctx context.Context) (*httpsclienttypes.GetConfigResponse, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.GetConfig(ctx)
}

// SyncCapabilities syncs camera capabilities to the VM
func (g *VmGatewayHttpImpl) SyncCapabilities(ctx context.Context, req *httpsclienttypes.SyncCapabilitiesRequest) (*httpsclienttypes.SyncCapabilitiesResponse, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.SyncCapabilities(ctx, req)
}

// SyncCameras syncs discovered cameras to the VM. VM decides which cameras should be enabled.
func (g *VmGatewayHttpImpl) SyncCameras(ctx context.Context, req *httpsclienttypes.SyncCamerasRequest) (*httpsclienttypes.SyncCamerasResponse, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.SyncCameras(ctx, req)
}

// SyncScreenshots syncs labeled screenshots to the VM for model training.
func (g *VmGatewayHttpImpl) SyncScreenshots(ctx context.Context, req *httpsclienttypes.SyncScreenshotsRequest) (*httpsclienttypes.SyncScreenshotsResponse, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.SyncScreenshots(ctx, req)
}

// ReportDeploymentStatus reports deployment status to the VM
func (g *VmGatewayHttpImpl) ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.ReportDeploymentStatus(ctx, deploymentID, status, errorMessage, modelPath)
}

// Heartbeat sends a heartbeat to the VM
func (g *VmGatewayHttpImpl) Heartbeat(ctx context.Context, req *wgclienttypes.HeartbeatRequest) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.Heartbeat(ctx, req)
}

// SendTelemetry sends telemetry data to the VM
func (g *VmGatewayHttpImpl) SendTelemetry(ctx context.Context, data *wgclienttypes.TelemetryData) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.SendTelemetry(ctx, data)
}

// SendEvents sends events to the VM
func (g *VmGatewayHttpImpl) SendEvents(ctx context.Context, events []*wgclienttypes.Event) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.httpsClientService == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return g.httpsClientService.SendEvents(ctx, events)
}
