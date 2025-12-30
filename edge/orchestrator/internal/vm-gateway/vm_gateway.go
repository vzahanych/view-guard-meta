package vmgateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Sentinel errors for common VM Gateway error conditions.
// These errors can be checked using errors.Is() for programmatic error handling.
var (
	// ErrNotInitialized indicates that the service or a required component is not initialized.
	// This typically occurs when trying to use the gateway before it has been properly started
	// or when a required service (e.g., transport service) is nil.
	ErrNotInitialized = errors.New("vm-gateway: service not initialized")

	// ErrAlreadyStarted indicates that an operation was attempted on a service that is already started.
	// This prevents double-starting the gateway.
	ErrAlreadyStarted = errors.New("vm-gateway: service already started")

	// ErrNotConnected indicates that the gateway is not connected to the VM.
	// This can occur when trying to communicate with the VM before authentication or connection is established.
	ErrNotConnected = errors.New("vm-gateway: not connected to VM")

	// ErrInvalidState indicates that an invalid state transition was attempted.
	// This is used by the connection state machine to reject invalid transitions.
	ErrInvalidState = errors.New("vm-gateway: invalid state transition")
)

// VMGateway provides a unified interface for Edge↔VM bidirectional secure communication.
// It combines two core services:
//   - Tunnel client service (tunnel management - WireGuard, OpenVPN, IPSec, etc.)
//   - Transport service (transport layer - HTTP, gRPC, WebSocket, etc.)
//
// The gateway provides:
//   - Unified lifecycle management for tunnel and transport services
//   - Coordinated startup and shutdown
//   - Complete API for Edge → VM communication (transport-agnostic)
//   - Tunnel status and management
//   - Connection state machine for tracking connection lifecycle

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_vm_gateway.go -package=mocks
type VMGateway interface {
	// Lifecycle methods

	// Start starts all underlying services (tunnel client, transport service).
	// Services are started in the correct order: tunnel first, then transport.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down all underlying services.
	// Services are stopped in reverse order: transport first, then tunnel.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// Tunnel status and management

	// IsConnected returns whether the tunnel is connected and ready for communication.
	IsConnected() bool

	// IsTransportConnected returns whether the transport connection to VM is established and authenticated.
	// In production mode, this requires the tunnel to be connected.
	// In dev mode (localhost), this checks if authentication with VM has succeeded.
	// This is provider-agnostic and works with any transport (HTTP, gRPC, WebSocket, etc.).
	IsTransportConnected() bool

	// GetTunnelInterfaceName returns the tunnel interface name.
	// This is provider-agnostic and works with any tunnel provider (WireGuard, OpenVPN, IPSec, etc.).
	GetTunnelInterfaceName() string

	// GetTunnelEndpoint returns the VM endpoint address.
	// This is provider-agnostic and works with any tunnel provider (WireGuard, OpenVPN, IPSec, etc.).
	GetTunnelEndpoint() string

	// VM communication methods (Edge → VM)

	// Authenticate authenticates Edge with VM and establishes the connection.
	// This should be called when Edge orchestrator starts (after tunnel is connected).
	//
	// Timeout: Uses AuthenticationTimeout from configuration (default: 30s).
	// Retry: Uses authentication-specific retry strategy (10s, 20s, 40s, max 5min backoff).
	// Errors: Returns error if authentication fails after all retries.
	// Time Sync: Validates time synchronization during authentication (if enabled).
	// Certificate: Validates certificate pinning and revocation (if enabled).
	Authenticate(ctx context.Context, edgeID string) error

	// GetConfig retrieves VM configuration.
	//
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if request fails after all retries.
	// Response: Returns GetConfigResponse with VM configuration JSON.
	GetConfig(ctx context.Context) (*types.GetConfigResponse, error)

	// SyncCapabilities syncs device capabilities to the VM.
	// Supports all device types (cameras, sensors, etc.), not just cameras.
	//
	// Idempotency: Idempotency key is auto-generated if not provided in request.
	// Format: {EdgeID}-sync-capabilities-{UUID}
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if sync fails after all retries.
	// Response: May include CertRotationScheduledAt for certificate rotation.
	SyncCapabilities(ctx context.Context, req *types.SyncCapabilitiesRequest) (*types.SyncCapabilitiesResponse, error)

	// SyncDevices syncs discovered devices to the VM. VM decides which devices should be enabled.
	// Supports all device types (cameras, sensors, etc.), not just cameras.
	//
	// Idempotency: Idempotency key is auto-generated if not provided in request.
	// Format: {EdgeID}-sync-devices-{UUID}
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if sync fails after all retries.
	// Response: Returns list of devices that VM has enabled.
	SyncDevices(ctx context.Context, req *types.SyncDevicesRequest) (*types.SyncDevicesResponse, error)

	// SyncDataUnits syncs labeled data units to the VM for model training.
	// This is device-agnostic and supports all IoT device types (cameras, sensors, audio devices, etc.).
	// Data units can be screenshots/images, sensor readings, audio samples, or any other labeled data.
	//
	// Idempotency: Idempotency key is auto-generated if not provided in request.
	// Format: {EdgeID}-sync-data-units-{UUID}
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if sync fails after all retries.
	// Response: Returns success status and optional message.
	SyncDataUnits(ctx context.Context, req *types.SyncDataUnitsRequest) (*types.SyncDataUnitsResponse, error)

	// ReportDeploymentStatus reports model deployment status to the VM.
	//
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if report fails after all retries.
	// Status: Valid values: "deployed", "active", "failed", "removed".
	// Usage: Called after model deployment completes or fails.
	ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error

	// Heartbeat sends a heartbeat to the VM to maintain connection.
	//
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if heartbeat fails after all retries.
	// Usage: Should be called periodically to maintain connection (e.g., every 30s).
	Heartbeat(ctx context.Context, req *types.HeartbeatRequest) error

	// SendTelemetry sends telemetry data to the VM.
	SendTelemetry(ctx context.Context, data *types.TelemetryData) error

	// SendEvents sends events to the VM.
	//
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if send fails after all retries.
	// Events: All events must be typed with proper event data structures.
	SendEvents(ctx context.Context, events []*types.Event) error

	// SyncAuditLogs syncs audit logs to the VM for long-term storage and analysis.
	// Audit logs are sent in batches with metadata for efficient transfer.
	//
	// Idempotency: Idempotency key is auto-generated if not provided in request.
	// Format: {EdgeID}-sync-audit-logs-{UUID}
	// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
	// Retry: Uses standard retry strategy (1s, 2s, 4s, 8s, max 60s backoff).
	// Errors: Returns error if sync fails after all retries.
	// Response: Returns success status and number of logs synced.
	SyncAuditLogs(ctx context.Context, req *types.SyncAuditLogsRequest) (*types.SyncAuditLogsResponse, error)

	// Connection state machine methods

	// GetConnectionState returns the current connection state
	GetConnectionState() types.ConnectionState

	// GetConnectionStateInfo returns detailed connection state information
	GetConnectionStateInfo() types.ConnectionStateInfo

	// TransitionConnectionState transitions to a new connection state
	// Returns error if transition is invalid
	TransitionConnectionState(newState types.ConnectionState, errorMsg string) error

	// CanTransitionConnectionState checks if a transition from current state to new state is valid
	CanTransitionConnectionState(newState types.ConnectionState) bool

	// Observability methods

	// HealthSnapshot returns a comprehensive, structured health snapshot of the gateway.
	// This includes connection state, tunnel status, transport status, and sub-service status.
	// The snapshot is JSON-serializable and useful for debugging and monitoring.
	HealthSnapshot() GatewayStatus
}

// GatewayStatus represents a comprehensive health snapshot of the VM Gateway.
// This is tunnel/transport agnostic and works with any provider combination.
type GatewayStatus struct {
	// ConnectionState contains the current connection state information
	ConnectionState types.ConnectionStateInfo `json:"connection_state"`

	// TunnelStatus contains tunnel-specific status information
	// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
	TunnelStatus TunnelStatus `json:"tunnel_status"`

	// TransportStatus contains transport-specific status information
	// This is provider-agnostic and works with HTTP, gRPC, WebSocket, etc.
	TransportStatus TransportStatus `json:"transport_status"`

	// SubServices contains status information for sub-services
	// Key is service name, value is service status
	SubServices map[string]ServiceStatus `json:"sub_services"`

	// CertificateRotationStatus contains certificate rotation status
	CertificateRotationStatus *types.CertificateRotationStatus `json:"certificate_rotation_status,omitempty"`

	// TimeSyncStatus contains time synchronization status
	TimeSyncStatus *types.TimeSyncStatus `json:"time_sync_status,omitempty"`

	// RateLimitStats contains rate limiting statistics
	RateLimitStats *types.RateLimitStats `json:"rate_limit_stats,omitempty"`

	// Timestamp is when this snapshot was taken
	Timestamp time.Time `json:"timestamp"`
}

// RetryStats represents retry and backoff statistics.
// Deprecated: Retry stats have been removed from GatewayStatus as they were never mutated.
// Retry statistics are available via transport-specific metrics if needed.
type RetryStats struct {
	// MaxRetries is the configured maximum number of retries
	MaxRetries int `json:"max_retries,omitempty"`

	// InitialBackoff is the configured initial backoff duration
	InitialBackoff time.Duration `json:"initial_backoff,omitempty"`

	// MaxBackoff is the configured maximum backoff duration
	MaxBackoff time.Duration `json:"max_backoff,omitempty"`

	// TotalRetries is the total number of retry attempts made
	TotalRetries int64 `json:"total_retries,omitempty"`

	// TotalRetryFailures is the total number of operations that failed after all retries
	TotalRetryFailures int64 `json:"total_retry_failures,omitempty"`
}

// EventEmissionStats represents event emission statistics.
// Deprecated: Event emission stats have been removed from GatewayStatus as they were never mutated.
// Event emission statistics are available via transport-specific metrics if needed.
type EventEmissionStats struct {
	// TotalEventsEmitted is the total number of events emitted
	TotalEventsEmitted int64 `json:"total_events_emitted,omitempty"`

	// TotalEmissionFailures is the total number of event emission failures
	TotalEmissionFailures int64 `json:"total_emission_failures,omitempty"`

	// LastEmissionTime is when the last event was emitted
	LastEmissionTime *time.Time `json:"last_emission_time,omitempty"`

	// LastEmissionFailureTime is when the last emission failure occurred
	LastEmissionFailureTime *time.Time `json:"last_emission_failure_time,omitempty"`
}

// CertificateRotationStatus is an alias for types.CertificateRotationStatus.
// Deprecated: Use types.CertificateRotationStatus directly.
type CertificateRotationStatus = types.CertificateRotationStatus

// TimeSyncStatus is an alias for types.TimeSyncStatus.
// Deprecated: Use types.TimeSyncStatus directly.
type TimeSyncStatus = types.TimeSyncStatus

// RateLimitStats is an alias for types.RateLimitStats.
// Deprecated: Use types.RateLimitStats directly.
type RateLimitStats = types.RateLimitStats

// TunnelStatus represents the status of the tunnel service.
// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
type TunnelStatus struct {
	// Enabled indicates whether tunnel is configured and enabled
	Enabled bool `json:"enabled"`

	// Connected indicates whether tunnel is connected
	Connected bool `json:"connected"`

	// InterfaceName is the tunnel network interface name (e.g., "wg0", "tun0")
	InterfaceName string `json:"interface_name,omitempty"`

	// Endpoint is the remote endpoint address
	Endpoint string `json:"endpoint,omitempty"`

	// ServiceName is the name of the tunnel service (e.g., "wireguard-client")
	ServiceName string `json:"service_name,omitempty"`
}

// TransportStatus represents the status of the transport service.
// This is provider-agnostic and works with HTTP, gRPC, WebSocket, etc.
type TransportStatus struct {
	// Connected indicates whether transport connection is established and authenticated
	Connected bool `json:"connected"`

	// ServiceName is the name of the transport service (e.g., "http-transport")
	ServiceName string `json:"service_name,omitempty"`
}

// ServiceStatus represents the status of a sub-service.
type ServiceStatus struct {
	// Name is the service name
	Name string `json:"name"`

	// Started indicates whether the service has been started
	Started bool `json:"started"`

	// Connected indicates whether the service is connected (if applicable)
	Connected bool `json:"connected,omitempty"`

	// Error contains any error message if the service is in an error state
	Error string `json:"error,omitempty"`
}

// NewVMGateway creates a new VM gateway instance.
// This factory function should typically not be called directly;
// use VMGatewayProvider instead for proper dependency injection.
// Dependencies (meta-storage, object-storage, ai-gateway, event-bus) are injected via fx.
//
// The gateway now uses transport-service for transport-agnostic communication.
// This allows switching between HTTP, gRPC, WebSocket, or other transports
// without changing the gateway code.
func NewVMGateway(
	ctx context.Context,
	config *types.VMGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (VMGateway, error) {
	// Use the new transport-agnostic implementation
	return NewVMGatewayImpl(ctx, config, metaStore, objectStore, eventBus, logger)
}

// VMGatewayProvider creates the VM gateway with fx lifecycle management.
//
// The gateway is a complex service with two internal components:
//   - Tunnel client service (tunnel management - WireGuard, OpenVPN, IPSec, etc.)
//   - Transport service (transport layer - HTTP, gRPC, WebSocket, etc.)
//   - Requires meta-storage, event-bus, object-storage for server-side operations
//
// All dependencies are injected via fx at construction time.
//
// Architecture decision (Section 1.0): Gateway-owned lifecycle.
//   - Fx manages only the gateway lifecycle.
//   - Gateway Start/Stop is the single place that starts/stops sub-services.
//   - Sub-services do not register their own fx.Lifecycle hooks.
//
// Fail-fast behavior: This provider will return an error (not nil) if:
//   - Configuration is invalid or unsupported
//   - Gateway creation fails
//   - Required dependencies are missing
//
// The application will not start if gateway creation fails, ensuring production reliability.
func VMGatewayProvider(
	lc fx.Lifecycle,
	cfg *types.VMGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (VMGateway, error) {
	// Validate configuration before creating gateway
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid VM gateway configuration: %w", err)
	}

	// Create the gateway (this constructs all three internal components with dependencies)
	// Note: We use context.Background() here because construction should not be cancellable.
	// The context is only used for passing to sub-service constructors that may need it for
	// initialization (not for cancellation). The actual Start() method will receive a proper
	// context from fx lifecycle for cancellable operations.
	gateway, err := NewVMGateway(context.Background(), cfg, metaStore, objectStore, eventBus, logger)
	if err != nil {
		logger.Error("Failed to create VM gateway", zap.Error(err))
		return nil, fmt.Errorf("failed to create VM gateway: %w", err)
	}

	// Setup lifecycle hooks - gateway owns the lifecycle of all sub-services
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting VM gateway (tunnel client, transport service)...")
			return gateway.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping VM gateway...")
			return gateway.Stop(ctx)
		},
	})

	return gateway, nil
}
