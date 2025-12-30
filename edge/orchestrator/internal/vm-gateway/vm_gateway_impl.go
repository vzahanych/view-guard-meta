package vmgateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/connection-state-machine/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"
	"go.uber.org/zap"
)

// vmGatewayImpl implements the VMGateway interface using transport-service.
// This provides a transport-agnostic gateway that can work with any transport implementation.
type vmGatewayImpl struct {
	tunnelService          tunnelclient.TunnelClientService
	transportService       transport.TransportService
	connectionStateMachine types.ConnectionStateMachine
	logger                 *zap.Logger
	mu                     sync.RWMutex
	started                bool
}

// NewVMGatewayImpl creates a new VMGateway implementation that uses transport-service.
// This is transport-agnostic and can work with HTTP, gRPC, WebSocket, or any other transport.
func NewVMGatewayImpl(
	ctx context.Context,
	cfg *types.VMGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (*vmGatewayImpl, error) {
	// Normalize logger: if nil, use a no-op logger
	if logger == nil {
		logger = zap.NewNop()
	}
	// Step 1: Get tunnel config (provider-agnostic)
	tunnelCfg := cfg.GetTunnelConfig()
	var tunnelSvc tunnelclient.TunnelClientService
	if tunnelCfg != nil && tunnelCfg.Enabled {
		var err error
		tunnelSvc, err = tunnelclient.NewTunnelClientService(tunnelCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create tunnel service: %w", err)
		}
	}

	// Step 2: Create transport service (HTTP, gRPC, WebSocket, etc.)
	transportSvc, err := transport.NewTransportService(
		ctx,
		cfg,
		tunnelSvc,
		metaStore,
		objectStore,
		eventBus,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport service: %w", err)
	}

	return &vmGatewayImpl{
		tunnelService:          tunnelSvc,
		transportService:       transportSvc,
		connectionStateMachine:  impl.NewConnectionStateMachine(),
		logger:                  logger,
	}, nil
}

// Name returns the service name
func (g *vmGatewayImpl) Name() string {
	return "vm-gateway"
}

// Start starts all underlying services in the correct order.
// Locking strategy: Copy service references under lock, call them outside lock to avoid deadlocks.
func (g *vmGatewayImpl) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.started {
		g.mu.Unlock()
		return ErrAlreadyStarted
	}

	// Copy service references under lock
	tunnelSvc := g.tunnelService
	transportSvc := g.transportService
	g.mu.Unlock()

	g.logger.Info("Starting VM Gateway (all services)")

	// Step 1: Start tunnel service first (required for transport services in production)
	// Skip tunnel if disabled (for localhost dev mode)
	if tunnelSvc != nil {
		g.logger.Info("Starting tunnel service...")
		if err := tunnelSvc.Start(ctx); err != nil {
			return fmt.Errorf("start tunnel service: %w", err)
		}
	} else {
		g.logger.Info("Tunnel disabled - skipping tunnel startup (localhost dev mode)")
	}

	// Step 2: Start transport service (depends on tunnel in production, localhost for dev)
	g.logger.Info("Starting transport service...")
	if transportSvc != nil {
		if err := transportSvc.Start(ctx); err != nil {
			// Try to stop tunnel if transport service fails (only if it was started)
			if tunnelSvc != nil {
				_ = tunnelSvc.Stop(ctx)
			}
			return fmt.Errorf("start transport service: %w", err)
		}
	}

	// Mark as started under lock
	g.mu.Lock()
	g.started = true
	g.mu.Unlock()

	g.logger.Info("VM Gateway started successfully (all services running)")

	return nil
}

// Stop stops all underlying services in reverse order.
// Locking strategy: Copy service references under lock, call them outside lock to avoid deadlocks.
// Uses errors.Join() to preserve individual error values (Go 1.20+).
func (g *vmGatewayImpl) Stop(ctx context.Context) error {
	g.mu.Lock()
	if !g.started {
		g.mu.Unlock()
		return nil // Already stopped
	}

	// Copy service references under lock
	transportSvc := g.transportService
	tunnelSvc := g.tunnelService
	g.mu.Unlock()

	g.logger.Info("Stopping VM Gateway (all services)")

	var errs []error

	// Stop transport service first
	g.logger.Info("Stopping transport service...")
	if transportSvc != nil {
		if err := transportSvc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop transport service: %w", err))
		}
	}

	// Stop tunnel service last
	g.logger.Info("Stopping tunnel service...")
	if tunnelSvc != nil {
		if err := tunnelSvc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop tunnel service: %w", err))
		}
	}

	// Mark as stopped under lock
	g.mu.Lock()
	g.started = false
	g.mu.Unlock()

	if len(errs) > 0 {
		g.logger.Error("Some services failed to stop", zap.Errors("errors", errs))
		return errors.Join(errs...)
	}

	g.logger.Info("VM Gateway stopped successfully")

	return nil
}

// WireGuard tunnel status methods

// IsConnected returns whether the tunnel is connected.
// Locking strategy: Copy service reference under lock, call outside lock.
func (g *vmGatewayImpl) IsConnected() bool {
	g.mu.RLock()
	tunnelSvc := g.tunnelService
	g.mu.RUnlock()
	return tunnelSvc != nil && tunnelSvc.IsConnected()
}

// IsTransportConnected returns whether the transport connection is established and authenticated.
// This is provider-agnostic and works with any transport (HTTP, gRPC, WebSocket, etc.).
// Locking strategy: Copy service reference under lock, call outside lock.
func (g *vmGatewayImpl) IsTransportConnected() bool {
	g.mu.RLock()
	transportSvc := g.transportService
	g.mu.RUnlock()
	if transportSvc == nil {
		return false
	}
	return transportSvc.IsConnected()
}

// GetTunnelInterfaceName returns the tunnel interface name.
// This is provider-agnostic and works with any tunnel provider (WireGuard, OpenVPN, IPSec, etc.).
// Locking strategy: Copy service reference under lock, call outside lock.
func (g *vmGatewayImpl) GetTunnelInterfaceName() string {
	g.mu.RLock()
	tunnelSvc := g.tunnelService
	g.mu.RUnlock()
	if tunnelSvc == nil {
		return ""
	}
	return tunnelSvc.GetInterfaceName()
}

// GetTunnelEndpoint returns the VM endpoint address.
// This is provider-agnostic and works with any tunnel provider (WireGuard, OpenVPN, IPSec, etc.).
// Locking strategy: Copy service reference under lock, call outside lock.
func (g *vmGatewayImpl) GetTunnelEndpoint() string {
	g.mu.RLock()
	tunnelSvc := g.tunnelService
	g.mu.RUnlock()
	if tunnelSvc == nil {
		return ""
	}
	return tunnelSvc.GetEndpoint()
}

// VM communication methods (Edge → VM) - delegate to transport service

// Authenticate authenticates Edge with VM
func (g *vmGatewayImpl) Authenticate(ctx context.Context, edgeID string) error {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return ErrNotInitialized
	}
	if err := transport.Authenticate(ctx, edgeID); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	return nil
}

// GetConfig retrieves VM configuration
func (g *vmGatewayImpl) GetConfig(ctx context.Context) (*types.GetConfigResponse, error) {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return nil, ErrNotInitialized
	}
	resp, err := transport.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	return resp, nil
}

// SyncCapabilities syncs device capabilities to the VM
func (g *vmGatewayImpl) SyncCapabilities(ctx context.Context, req *types.SyncCapabilitiesRequest) (*types.SyncCapabilitiesResponse, error) {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return nil, ErrNotInitialized
	}
	resp, err := transport.SyncCapabilities(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sync capabilities: %w", err)
	}
	return resp, nil
}

// SyncDevices syncs discovered devices to the VM
func (g *vmGatewayImpl) SyncDevices(ctx context.Context, req *types.SyncDevicesRequest) (*types.SyncDevicesResponse, error) {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return nil, ErrNotInitialized
	}
	resp, err := transport.SyncDevices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sync devices: %w", err)
	}
	return resp, nil
}

// SyncDataUnits syncs labeled data units to the VM for model training
func (g *vmGatewayImpl) SyncDataUnits(ctx context.Context, req *types.SyncDataUnitsRequest) (*types.SyncDataUnitsResponse, error) {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return nil, ErrNotInitialized
	}
	resp, err := transport.SyncDataUnits(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sync data units: %w", err)
	}
	return resp, nil
}

// SyncAuditLogs syncs audit logs to the VM
func (g *vmGatewayImpl) SyncAuditLogs(ctx context.Context, req *types.SyncAuditLogsRequest) (*types.SyncAuditLogsResponse, error) {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return nil, ErrNotInitialized
	}
	resp, err := transport.SyncAuditLogs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sync audit logs: %w", err)
	}
	return resp, nil
}

// ReportDeploymentStatus reports deployment status to the VM
func (g *vmGatewayImpl) ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return ErrNotInitialized
	}
	if err := transport.ReportDeploymentStatus(ctx, deploymentID, status, errorMessage, modelPath); err != nil {
		return fmt.Errorf("report deployment status: %w", err)
	}
	return nil
}

// Heartbeat sends a heartbeat to the VM
func (g *vmGatewayImpl) Heartbeat(ctx context.Context, req *types.HeartbeatRequest) error {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return ErrNotInitialized
	}
	if err := transport.Heartbeat(ctx, req); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	return nil
}

// SendTelemetry sends telemetry data to the VM
func (g *vmGatewayImpl) SendTelemetry(ctx context.Context, data *types.TelemetryData) error {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return ErrNotInitialized
	}
	if err := transport.SendTelemetry(ctx, data); err != nil {
		return fmt.Errorf("send telemetry: %w", err)
	}
	return nil
}

// SendEvents sends events to the VM
func (g *vmGatewayImpl) SendEvents(ctx context.Context, events []*types.Event) error {
	g.mu.RLock()
	transport := g.transportService
	g.mu.RUnlock()
	if transport == nil {
		return ErrNotInitialized
	}
	if err := transport.SendEvents(ctx, events); err != nil {
		return fmt.Errorf("send events: %w", err)
	}
	return nil
}

// Connection state machine methods

// GetConnectionState returns the current connection state.
// Locking strategy: Copy state machine reference under lock, call outside lock.
func (g *vmGatewayImpl) GetConnectionState() types.ConnectionState {
	g.mu.RLock()
	stateMachine := g.connectionStateMachine
	g.mu.RUnlock()
	return stateMachine.GetState()
}

// GetConnectionStateInfo returns detailed connection state information.
// Locking strategy: Copy state machine reference under lock, call outside lock.
func (g *vmGatewayImpl) GetConnectionStateInfo() types.ConnectionStateInfo {
	g.mu.RLock()
	stateMachine := g.connectionStateMachine
	g.mu.RUnlock()
	return stateMachine.GetStateInfo()
}

// TransitionConnectionState transitions to a new connection state.
// Locking strategy: Copy state machine reference under lock, call outside lock.
func (g *vmGatewayImpl) TransitionConnectionState(newState types.ConnectionState, errorMsg string) error {
	g.mu.RLock()
	stateMachine := g.connectionStateMachine
	g.mu.RUnlock()
	if err := stateMachine.Transition(newState, errorMsg); err != nil {
		// Wrap state machine errors with context
		return fmt.Errorf("transition connection state: %w", err)
	}
	return nil
}

// CanTransitionConnectionState checks if a transition from current state to new state is valid.
// Locking strategy: Copy state machine reference under lock, call outside lock.
func (g *vmGatewayImpl) CanTransitionConnectionState(newState types.ConnectionState) bool {
	g.mu.RLock()
	stateMachine := g.connectionStateMachine
	g.mu.RUnlock()
	return stateMachine.CanTransition(newState)
}

// HealthSnapshot returns a comprehensive, structured health snapshot of the gateway.
// Locking strategy: Copy service references under lock, call methods outside lock.
func (g *vmGatewayImpl) HealthSnapshot() GatewayStatus {
	g.mu.RLock()
	// Copy all references under lock
	tunnelSvc := g.tunnelService
	transportSvc := g.transportService
	stateMachine := g.connectionStateMachine
	started := g.started
	g.mu.RUnlock()

	// Get connection state information
	connectionStateInfo := stateMachine.GetStateInfo()

	// Get tunnel status
	tunnelStatus := TunnelStatus{
		Enabled: tunnelSvc != nil,
	}
	if tunnelSvc != nil {
		tunnelStatus.Connected = tunnelSvc.IsConnected()
		tunnelStatus.InterfaceName = tunnelSvc.GetInterfaceName()
		tunnelStatus.Endpoint = tunnelSvc.GetEndpoint()
		tunnelStatus.ServiceName = tunnelSvc.Name()
	}

	// Get transport status
	transportStatus := TransportStatus{
		Connected: false,
	}
	if transportSvc != nil {
		transportStatus.Connected = transportSvc.IsConnected()
		transportStatus.ServiceName = transportSvc.Name()
	}

	// Build sub-services status map
	subServices := make(map[string]ServiceStatus)
	if tunnelSvc != nil {
		subServices["tunnel"] = ServiceStatus{
			Name:      tunnelSvc.Name(),
			Started:   started,
			Connected: tunnelSvc.IsConnected(),
		}
	}
	if transportSvc != nil {
		subServices["transport"] = ServiceStatus{
			Name:      transportSvc.Name(),
			Started:   started,
			Connected: transportSvc.IsConnected(),
		}
	}

	// Get transport-specific health metrics (certificate rotation, time sync, rate limiting)
	// Use typed HealthReporter interface instead of interface{} and map parsing
	var certRotationStatus *types.CertificateRotationStatus
	var timeSyncStatus *types.TimeSyncStatus
	var rateLimitStats *types.RateLimitStats
	
	// Use type assertion to access HTTP transport service health metrics via HealthReporter interface
	if healthReporter, ok := transportSvc.(types.HealthReporter); ok {
		certRotationStatus = healthReporter.GetCertificateRotationStatus()
		timeSyncStatus = healthReporter.GetTimeSyncStatus()
		rateLimitStats = healthReporter.GetRateLimitStats()
	}

	return GatewayStatus{
		ConnectionState:          connectionStateInfo,
		TunnelStatus:             tunnelStatus,
		TransportStatus:          transportStatus,
		SubServices:              subServices,
		CertificateRotationStatus: certRotationStatus,
		TimeSyncStatus:           timeSyncStatus,
		RateLimitStats:           rateLimitStats,
		Timestamp:                time.Now(),
	}
}

// Remove helper functions - no longer needed with typed interface


