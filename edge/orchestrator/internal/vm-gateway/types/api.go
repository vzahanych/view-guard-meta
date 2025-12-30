package types

import (
	"context"
	"time"
)

// IdempotencyKey is a string type representing an idempotency key in UUID format.
// Format: {EdgeID}-{operation}-{UUID}
// Example: "edge-123-sync-devices-550e8400-e29b-41d4-a716-446655440000"
//
// Idempotency keys ensure that repeated requests have the same effect as a single request.
// If an idempotency key is not provided in a request, one will be auto-generated.
// When retrying a failed request, use the same idempotency key to prevent duplicate processing.
//
// Supported operations:
//   - sync-capabilities: {EdgeID}-sync-capabilities-{UUID}
//   - sync-devices: {EdgeID}-sync-devices-{UUID}
//   - sync-data-units: {EdgeID}-sync-data-units-{UUID}
//   - sync-audit-logs: {EdgeID}-sync-audit-logs-{UUID}
type IdempotencyKey string

// Transport-agnostic API types for VM Gateway.
// These types are used in the public VMGateway interface and are independent
// of any specific transport implementation (HTTP, gRPC, etc.).

// TransportService provides an abstract interface for transport functionality.
// This interface is implemented by specific transport providers (HTTP, gRPC, WebSocket, etc.).
//
// The service provides:
//   - Transport client and server management
//   - Edge ↔ VM bidirectional communication
//   - Provider-agnostic API for VM Gateway
type TransportService interface {
	// Start starts the transport service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the transport service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// IsConnected returns whether the transport connection is ready.
	IsConnected() bool

	// VM communication methods (Edge → VM)
	Authenticate(ctx context.Context, edgeID string) error
	GetConfig(ctx context.Context) (*GetConfigResponse, error)
	SyncCapabilities(ctx context.Context, req *SyncCapabilitiesRequest) (*SyncCapabilitiesResponse, error)
	SyncDevices(ctx context.Context, req *SyncDevicesRequest) (*SyncDevicesResponse, error)
	SyncDataUnits(ctx context.Context, req *SyncDataUnitsRequest) (*SyncDataUnitsResponse, error)
	SyncAuditLogs(ctx context.Context, req *SyncAuditLogsRequest) (*SyncAuditLogsResponse, error)
	ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error
	Heartbeat(ctx context.Context, req *HeartbeatRequest) error
	SendTelemetry(ctx context.Context, data *TelemetryData) error
	SendEvents(ctx context.Context, events []*Event) error
}

// TunnelClientService provides an abstract interface for tunnel client functionality.
// This interface is implemented by specific tunnel providers (WireGuard, OpenVPN, IPSec, etc.).
//
// The service provides:
//   - Tunnel interface management
//   - Connection monitoring
//   - Tunnel health checks
//   - Provider-agnostic status reporting
type TunnelClientService interface {
	// Start starts the tunnel client service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the tunnel client service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// IsConnected returns whether the tunnel is connected and ready.
	IsConnected() bool

	// GetInterfaceName returns the tunnel network interface name.
	GetInterfaceName() string

	// GetEndpoint returns the remote endpoint address.
	GetEndpoint() string
}

// GetConfigResponse represents the response from the VM GetConfig endpoint.
type GetConfigResponse struct {
	// Success indicates whether configuration retrieval succeeded.
	Success bool `json:"success"`

	// ConfigJSON contains the VM configuration as a JSON string.
	ConfigJSON string `json:"config_json"`

	// ErrorMessage is populated when Success is false.
	ErrorMessage string `json:"error_message,omitempty"`
}

// DeviceCapability describes a single device's dataset readiness state
// for capability synchronization with the VM.
// This is device-agnostic but currently used primarily for cameras (video devices).
type DeviceCapability struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // Device type (e.g., "camera", "sensor", etc.)
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status"`
	// LabelCounts holds per-label snapshot counts (for video devices).
	// For non-video devices, this may be empty or used for other labeling schemes.
	LabelCounts map[string]uint32 `json:"label_counts"`
	// LabeledSnapshotCount is the total number of labeled snapshots (for video devices).
	// For non-video devices, this may represent labeled data samples.
	LabeledSnapshotCount uint32 `json:"labeled_snapshot_count"`
	// RequiredSnapshotCount is the number of snapshots required for readiness (for video devices).
	// For non-video devices, this may represent required data samples.
	RequiredSnapshotCount uint32 `json:"required_snapshot_count"`
	// SnapshotRequired indicates whether more snapshots are needed (for video devices).
	// For non-video devices, this may indicate if more data samples are needed.
	SnapshotRequired bool `json:"snapshot_required"`
}

// SyncCapabilitiesRequest contains the payload for syncing device capabilities to the VM.
//
// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
// Retry: Uses standard retry strategy with exponential backoff.
// Idempotency: Idempotency key is auto-generated if not provided.
// Response: May include CertRotationScheduledAt for certificate rotation scheduling.
type SyncCapabilitiesRequest struct {
	SyncedAt int64               `json:"synced_at"`
	Devices  []*DeviceCapability `json:"devices"` // Device-agnostic: supports cameras, sensors, etc.
}

// SyncCapabilitiesResponse represents the VM response for capability sync.
type SyncCapabilitiesResponse struct {
	Success                bool       `json:"success"`
	ErrorMessage           string     `json:"error_message,omitempty"`
	CertRotationScheduledAt *time.Time `json:"cert_rotation_scheduled_at,omitempty"` // Optional, signals upcoming certificate rotation
}

// DeviceInfo represents basic device information for sync.
// This is device-agnostic and supports cameras, sensors, and other IoT devices.
type DeviceInfo struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`   // Device type (e.g., "camera", "motion_sensor", etc.)
	Source   string `json:"source,omitempty"` // Discovery source (e.g., "onvif", "usb", "rtsp", etc.)
	Enabled  bool   `json:"enabled,omitempty"`
}

// SyncDevicesRequest contains the payload for syncing discovered devices to the VM.
// VM decides which devices should be enabled based on configuration and policies.
//
// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
// Retry: Uses standard retry strategy with exponential backoff.
// Idempotency: Idempotency key is auto-generated if not provided.
// Format: {EdgeID}-sync-devices-{UUID}
// Errors: Returns error if sync fails after all retries (transient errors are retried).
type SyncDevicesRequest struct {
	EdgeID         string        `json:"edge_id"`
	IdempotencyKey string        `json:"idempotency_key,omitempty"` // Format: {EdgeID}-sync-devices-{UUID}
	Devices        []*DeviceInfo `json:"devices"`                    // Device-agnostic: supports cameras, sensors, etc.
}

// EnabledDevice represents a device that VM has decided to enable.
type EnabledDevice struct {
	DeviceID string `json:"device_id"`
	Enabled  bool   `json:"enabled"`
}

// SyncDevicesResponse represents the VM response for device sync.
type SyncDevicesResponse struct {
	Success        bool             `json:"success"`
	ErrorMessage   string           `json:"error_message,omitempty"`
	EnabledDevices []*EnabledDevice `json:"enabled_devices,omitempty"`
}

// DataUnitInfo represents a labeled data unit for model training.
// This is device-agnostic and can represent:
//   - Screenshots/images from cameras (video devices)
//   - Sensor readings from sensors
//   - Audio samples from audio devices
//   - Any other labeled data unit for training
type DataUnitInfo struct {
	DataUnitID    string                 `json:"data_unit_id"`              // Unique identifier for this data unit
	DeviceID      string                 `json:"device_id"`                 // Device that produced this data unit
	ObjectKey     string                 `json:"object_key"`                // Path to data in object storage
	RawData       string                 `json:"raw_data,omitempty"`        // Base64 encoded raw data (image, sensor reading, audio, etc.)
	RawDataFormat string                 `json:"raw_data_format,omitempty"` // Data format (e.g., "jpeg", "png", "json", "wav", etc.)
	Label         string                 `json:"label"`                     // Label for training
	CustomLabel   string                 `json:"custom_label,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     int64                  `json:"created_at"` // Unix timestamp
}

// SyncDataUnitsRequest contains the payload for syncing labeled data units to the VM for model training.
// This is device-agnostic and supports all IoT device types (cameras, sensors, audio devices, etc.).
//
// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
// Retry: Uses standard retry strategy with exponential backoff.
// Idempotency: Idempotency key is auto-generated if not provided.
// Format: {EdgeID}-sync-data-units-{UUID}
// Errors: Returns error if sync fails after all retries (transient errors are retried).
// Size: Request body size should be reasonable (consider batching large datasets).
type SyncDataUnitsRequest struct {
	EdgeID         string          `json:"edge_id"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"` // Format: {EdgeID}-sync-data-units-{UUID}
	DeviceID       string          `json:"device_id"`                  // Device that produced the data units
	DataUnits      []*DataUnitInfo `json:"data_units"`                // Labeled data units for training
}

// SyncDataUnitsResponse represents the VM response for data unit sync.
type SyncDataUnitsResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	Message      string `json:"message,omitempty"`
}

// AuditLogEntry represents a single audit log entry for sync to VM.
// Note: VM only knows about EdgeID, not individual users, so UserID is not included.
// Note: Edge devices are typically behind NAT, so IPAddress (internal IP) is not meaningful to VM.
type AuditLogEntry struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Timestamp    int64                  `json:"timestamp"` // Unix timestamp
	EdgeID       string                 `json:"edge_id"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	Result       string                 `json:"result"`
	Error        string                 `json:"error,omitempty"`
	PreviousHash string                 `json:"previous_hash,omitempty"`
	Hash         string                 `json:"hash"`
	Data         map[string]interface{} `json:"data"`             // Entry-specific data
	Format       string                 `json:"format,omitempty"` // "json" or "cef"
	CEF          string                 `json:"cef,omitempty"`    // CEF formatted entry if format is "cef"
}

// SyncAuditLogsRequest contains the payload for syncing audit logs to the VM.
//
// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
// Retry: Uses standard retry strategy with exponential backoff.
// Idempotency: Idempotency key is auto-generated if not provided.
// Format: {EdgeID}-sync-audit-logs-{UUID}
// Errors: Returns error if sync fails after all retries (transient errors are retried).
// Batching: Logs should be sent in batches for efficiency (recommended: 100-1000 entries per batch).
type SyncAuditLogsRequest struct {
	EdgeID         string           `json:"edge_id"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"` // Format: {EdgeID}-sync-audit-logs-{UUID}
	StartTime      int64            `json:"start_time"`               // Unix timestamp of first entry
	EndTime        int64            `json:"end_time"`                  // Unix timestamp of last entry
	EntryCount     int              `json:"entry_count"`               // Number of entries in this batch
	Entries        []*AuditLogEntry `json:"entries"`                  // Audit log entries
	Format         string           `json:"format"`                   // "json" or "cef"
}

// SyncAuditLogsResponse represents the VM response for audit log sync.
type SyncAuditLogsResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	SyncedCount  int    `json:"synced_count,omitempty"` // Number of entries successfully synced
}

// HeartbeatRequest contains minimal heartbeat data sent from Edge to VM.
//
// Timeout: Uses VMAPIRequestTimeout from configuration (default: 30s).
// Retry: Uses standard retry strategy with exponential backoff.
// Errors: Returns error if heartbeat fails after all retries (transient errors are retried).
// Usage: Should be sent periodically (e.g., every 30s) to maintain connection.
// Failure: If heartbeat fails repeatedly, connection may be considered lost.
type HeartbeatRequest struct {
	// Timestamp is the Unix timestamp when the heartbeat was generated on the Edge.
	Timestamp int64 `json:"timestamp"`

	// EdgeID identifies the Edge node sending the heartbeat.
	EdgeID string `json:"edge_id"`
}

// TelemetryData contains minimal telemetry data sent from Edge to VM.
type TelemetryData struct {
	// Timestamp is the time when telemetry was captured.
	Timestamp int64 `json:"timestamp"`

	// EdgeID identifies the Edge node that produced this telemetry snapshot.
	EdgeID string `json:"edge_id"`

	// Fields can be extended later as needed.
}

// Event represents a generic event that can be sent to the VM.
// The exact schema is intentionally flexible; we only require basic metadata.
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}
