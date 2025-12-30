package types

import (
	"encoding/json"
	"time"
)

// EventType represents the type of an event.
// This is a custom type with enum constants for all possible event types.
//
// Event types follow a hierarchical naming convention:
//   - Category: subcategory: action (e.g., "device.discovered")
//   - Category: action (e.g., "storage.full")
//
// Event types are used for:
//   - Event categorization (drop policy)
//   - Event filtering (queries)
//   - Event subscription (Subscribe method)
//
// Example:
//   eventType := EventTypeDeviceDiscovered
//   category := dropPolicy.GetCategory(eventType)
type EventType string

// Event type constants - organized by category
const (
	// Network events
	EventTypeNetworkTunnelConnecting      EventType = "network.tunnel.connecting"
	EventTypeNetworkTunnelConnected       EventType = "network.tunnel.connected"
	EventTypeNetworkTunnelDisconnected    EventType = "network.tunnel.disconnected"
	EventTypeNetworkTunnelConnectionError EventType = "network.tunnel.connection_error"
	EventTypeNetworkTransportConnecting   EventType = "network.transport.connecting"
	EventTypeNetworkTransportConnected    EventType = "network.transport.connected"
	EventTypeNetworkTransportDisconnected EventType = "network.transport.disconnected"
	EventTypeNetworkTransportConnectionError EventType = "network.transport.connection_error"

	// Edge/VM events
	EventTypeEdgeAuthenticated        EventType = "edge.authenticated"
	EventTypeEdgeCapabilitiesReceived EventType = "edge.capabilities_received"

	// Device events (device-agnostic: cameras, sensors, etc.)
	EventTypeDeviceDiscovered   EventType = "device.discovered"
	EventTypeDeviceRegistered   EventType = "device.registered"
	EventTypeDeviceConnected    EventType = "device.connected"
	EventTypeDeviceDisconnected EventType = "device.disconnected"
	EventTypeDeviceCaptureFrame EventType = "device.capture_frame"

	// Data unit events
	EventTypeDataUnitRequested EventType = "data_unit.requested"
	EventTypeDataUnitSaved     EventType = "data_unit.saved"
	EventTypeDataUnitSetReady  EventType = "data_unit.set_ready"
	EventTypeDataUnitUpdated   EventType = "data_unit.updated"
	EventTypeDataUnitDeleted   EventType = "data_unit.deleted"

	// Raw device data events
	// Frame means a single frame of video, audio, or other data from a device.
	// Clip means a collection of frames of video, audio, or other data from a device.
	EventTypeRawDeviceDataFrameReceived EventType = "raw_device_data.frame_received"
	EventTypeRawDeviceDataClipRecorded  EventType = "raw_device_data.clip_recorded"

	// Storage events
	EventTypeStorageFull                EventType = "storage.full"
	EventTypeStorageWarning             EventType = "storage.warning"
	EventTypeStorageQuotaExceeded       EventType = "storage.quota_exceeded"
	EventTypeStorageCleanupStarted      EventType = "storage.cleanup_started"
	EventTypeStorageCleanupCompleted    EventType = "storage.cleanup_completed"
	EventTypeStorageCorruptionDetected  EventType = "storage.corruption_detected"
	EventTypeStorageSchemaMigrationStarted  EventType = "storage.schema_migration_started"
	EventTypeStorageSchemaMigrationCompleted EventType = "storage.schema_migration_completed"

	// Audit log events
	EventTypeAuditLogQueueFull      EventType = "audit_log.queue_full"
	EventTypeAuditLogQueueResumed   EventType = "audit_log.queue_resumed"
	EventTypeAuditLogSyncFailed     EventType = "audit_log.sync_failed"
	EventTypeAuditLogSyncSucceeded  EventType = "audit_log.sync_succeeded"
	EventTypeAuditLogTamperDetected EventType = "audit_log.tamper_detected"
	EventTypeAuditLogCleanupStarted EventType = "audit_log.cleanup_started"
	EventTypeAuditLogCleanupCompleted EventType = "audit_log.cleanup_completed"
	EventTypeAuditLogHealthDegraded EventType = "audit_log.health_degraded"

	// Security events
	EventTypeSecurityEventCreated EventType = "security.event.created"

	// Workflow events
	EventTypeWorkflowDeviceDiscover    EventType = "workflow.device.discover"
	EventTypeWorkflowAIStartProcessing EventType = "workflow.ai.start_processing"
)

// String returns the string representation of the event type.
func (e EventType) String() string {
	return string(e)
}

// EventData is a type set (interface constraint) for all event data types.
// This ensures type safety while allowing different event payload types.
//
// All event data types must be included in this type set to be used with Event[T].
// This provides compile-time type safety when working with typed events.
//
// Example:
//   event := Event[DeviceDiscoveredEventData]{
//       Type: EventTypeDeviceDiscovered,
//       Data: DeviceDiscoveredEventData{...},
//   }
type EventData interface {
	ModelDeployedEventData | ModelDeploymentStatusEventData | SnapshotRequestedEventData | CapabilitiesReceivedEventData | NetworkEventData | TunnelConnectingEventData | TunnelConnectionErrorEventData | TunnelConnectedEventData | TunnelDisconnectedEventData | TransportConnectingEventData | TransportConnectionErrorEventData | TransportConnectedEventData | TransportDisconnectedEventData | EdgeAuthenticatedEventData | RateLimitExceededEventData | TimeSyncCriticalDriftEventData | TimeSyncDriftWarningEventData | CertificateRotationScheduledEventData | CertificateRotationCompletedEventData | CertificateRotationFailedEventData | DeviceEventData | DeviceFrameReceivedEventData | DeviceDiscoveredEventData | DeviceRegisteredEventData | DataUnitSavedEventData | DataUnitUpdatedEventData | DataUnitDeletedEventData | StorageEventData | AuditLogEventData
}

// Event represents an application-level event flowing through the bus.
// T must be one of the types in the EventData type set.
//
// This is the typed event structure that provides compile-time type safety.
// Use ToEventAny() to convert to EventAny for bus operations.
//
// Fields:
//   - Type: The event type (e.g., EventTypeDeviceDiscovered)
//   - Source: The component that emitted the event (e.g., "device-manager")
//   - Timestamp: When the event was created (should be set to time.Now() at creation)
//   - Data: Type-safe event-specific payload (must match EventType)
//   - SequenceNumber: Sequence number per source for ordering (0 if not set)
//
// Example:
//   event := Event[DeviceDiscoveredEventData]{
//       Type:      EventTypeDeviceDiscovered,
//       Source:    "device-manager",
//       Timestamp: time.Now(),
//       Data: DeviceDiscoveredEventData{
//           DeviceID: "camera-001",
//           Name:     "Front Door Camera",
//       },
//       SequenceNumber: 1,
//   }
type Event[T EventData] struct {
	// Type is the event type identifier (e.g., "device.discovered").
	// Must match one of the EventType constants.
	Type EventType

	// Source is the component that emitted the event (e.g., "device-manager", "ai-service").
	// Used for event ordering per-source and debugging.
	Source string

	// Timestamp is when the event was created.
	// Should be set to time.Now() at event creation time.
	// Used for retention cleanup and event ordering.
	Timestamp time.Time

	// Data is the type-safe event-specific payload.
	// The type T must match the EventType (e.g., DeviceDiscoveredEventData for EventTypeDeviceDiscovered).
	Data T

	// SequenceNumber is the sequence number per source for ordering guarantees.
	// Set to 0 if ordering is not required.
	// Events from the same source with sequence numbers are ordered.
	SequenceNumber int64
}

// EventAny is a type-erased event for use in EventBus interface.
// It stores data as JSON bytes to maintain type safety while allowing
// the bus to handle all event types uniformly.
//
// This is the runtime event structure used by the EventBus interface.
// Use FromEventAny() to convert back to typed Event[T].
//
// Fields:
//   - Type: The event type (e.g., EventTypeDeviceDiscovered)
//   - Source: The component that emitted the event
//   - Timestamp: When the event was created
//   - Data: Type-erased data as JSON bytes (use FromEventAny to convert back)
//   - SequenceNumber: Sequence number per source for ordering
//
// Example:
//   eventAny := EventAny{
//       Type:      EventTypeDeviceDiscovered,
//       Source:    "device-manager",
//       Timestamp: time.Now(),
//       Data:      json.RawMessage(`{"device_id":"camera-001"}`),
//   }
//   bus.Publish(eventAny)
type EventAny struct {
	// Type is the event type identifier (e.g., "device.discovered").
	Type EventType

	// Source is the component that emitted the event.
	Source string

	// Timestamp is when the event was created.
	Timestamp time.Time

	// Data is the type-erased event data as JSON bytes.
	// Use FromEventAny[T]() to convert back to typed Event[T].
	Data json.RawMessage

	// SequenceNumber is the sequence number per source for ordering guarantees.
	SequenceNumber int64
}

// ToEventAny converts a typed Event[T] to EventAny for bus operations.
// This is used to convert typed events to the type-erased format required by EventBus.Publish().
//
// The event data is marshaled to JSON bytes. If marshaling fails, an error is returned.
//
// Example:
//   typedEvent := Event[DeviceDiscoveredEventData]{...}
//   eventAny, err := ToEventAny(typedEvent)
//   if err != nil {
//       // Handle error
//   }
//   bus.Publish(eventAny)
func ToEventAny[T EventData](e Event[T]) (EventAny, error) {
	dataBytes, err := json.Marshal(e.Data)
	if err != nil {
		return EventAny{}, err
	}
	return EventAny{
		Type:           e.Type,
		Source:         e.Source,
		Timestamp:      e.Timestamp,
		Data:           dataBytes,
		SequenceNumber: e.SequenceNumber,
	}, nil
}

// FromEventAny converts EventAny back to typed Event[T].
// This is used to convert type-erased events back to typed events for type-safe access.
//
// The event data is unmarshaled from JSON bytes. If unmarshaling fails or the type
// doesn't match, an error is returned.
//
// Example:
//   eventAny := <-ch // Received from Subscribe()
//   typedEvent, err := FromEventAny[DeviceDiscoveredEventData](eventAny)
//   if err != nil {
//       // Handle error (wrong type or invalid JSON)
//   }
//   deviceID := typedEvent.Data.DeviceID // Type-safe access
func FromEventAny[T EventData](e EventAny) (Event[T], error) {
	var data T
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return Event[T]{}, err
	}
	return Event[T]{
		Type:           e.Type,
		Source:         e.Source,
		Timestamp:      e.Timestamp,
		Data:           data,
		SequenceNumber: e.SequenceNumber,
	}, nil
}

// EventProcessingStatus represents the processing status of an event.
// This is used for retry logic and dead letter queue management.
//
// Event processing flow:
//   pending → processing → succeeded (or failed → retry → succeeded/dead_letter)
type EventProcessingStatus string

const (
	// EventStatusPending indicates the event is pending processing.
	// This is the initial status for newly persisted events.
	EventStatusPending EventProcessingStatus = "pending"

	// EventStatusProcessing indicates the event is currently being processed.
	// This status is set when a subscriber starts processing the event.
	EventStatusProcessing EventProcessingStatus = "processing"

	// EventStatusSucceeded indicates the event was successfully processed.
	// This status is set when processing completes successfully.
	EventStatusSucceeded EventProcessingStatus = "succeeded"

	// EventStatusFailed indicates event processing failed (will be retried).
	// This status is set when processing fails but retries are still available.
	EventStatusFailed EventProcessingStatus = "failed"

	// EventStatusDeadLetter indicates the event was moved to dead letter queue after max retries.
	// This status is set when the event has exceeded the maximum retry attempts.
	EventStatusDeadLetter EventProcessingStatus = "dead_letter"
)

// OrderingMode defines how events are ordered during delivery.
// Ordering is applied per-source (events from the same source are ordered).
type OrderingMode string

const (
	// OrderingModeNone provides no ordering guarantees (fastest).
	// Events are delivered as soon as they are available, in any order.
	// Use this for events where order doesn't matter.
	OrderingModeNone OrderingMode = "none"

	// OrderingModeBestEffort provides best-effort ordering (balanced).
	// Events are reordered if possible, but missing sequences don't block delivery.
	// Use this for events where order is preferred but not critical.
	OrderingModeBestEffort OrderingMode = "best_effort"

	// OrderingModeStrict provides strict ordering guarantees (slowest, strongest guarantee).
	// Events are buffered and delivery waits for missing sequences up to a timeout.
	// Use this for events where strict ordering is critical.
	OrderingModeStrict OrderingMode = "strict"
)

// EventBusConfig is now defined in types/config.go.
// This file no longer contains the EventBusConfig definition.

// Event data types for standardized event payloads.
// These types provide type safety and discoverability for event data.

// ModelDeployedEventData contains data for model.deployed events.
type ModelDeployedEventData struct {
	ModelID      string  `json:"model_id"`
	DeploymentID *string `json:"deployment_id,omitempty"`
	Version      string  `json:"version,omitempty"`
	ModelType    string  `json:"model_type,omitempty"`
	Framework    string  `json:"framework,omitempty"`
	ModelPath    string  `json:"model_path,omitempty"`
	MetadataPath string  `json:"metadata_path,omitempty"`
	DeviceID     string  `json:"device_id,omitempty"` // Device ID (device-agnostic)
}

// ModelDeploymentStatusEventData contains data for model.deployment.status events.
type ModelDeploymentStatusEventData struct {
	DeploymentID string  `json:"deployment_id"`
	Status       string  `json:"status"` // "deployed", "active", "failed", "error"
	ModelPath    string  `json:"model_path,omitempty"`
	ModelID      string  `json:"model_id,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

// SnapshotRequestedEventData contains data for data_unit.requested events.
// Note: This event is used for data unit capture requests (snapshots, sensor readings, etc.)
type SnapshotRequestedEventData struct {
	DeviceID    string `json:"device_id"` // Device ID (device-agnostic)
	Label       string `json:"label"`
	CustomLabel string `json:"custom_label,omitempty"`
	Count       int32  `json:"count"`
}

// CapabilitiesReceivedEventData contains data for edge.capabilities_received events.
type CapabilitiesReceivedEventData struct {
	Capabilities interface{} `json:"capabilities"` // Array of device capabilities
}

// NetworkEventData contains data for network-related events (tunnel.connected, tunnel.disconnected, etc.).
// This is a flexible structure that can accommodate various network event payloads.
// Note: For new events, prefer using specific typed event data structures instead of this generic one.
type NetworkEventData struct {
	Interface   string                 `json:"interface,omitempty"`   // Network interface name
	Endpoint    string                 `json:"endpoint,omitempty"`    // Network endpoint
	Reason      string                 `json:"reason,omitempty"`      // Disconnection reason
	Reconnected bool                   `json:"reconnected,omitempty"` // Whether this is a reconnection
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // Additional metadata (deprecated: use specific typed structures)
}

// TunnelConnectingEventData contains data for network.tunnel.connecting events.
type TunnelConnectingEventData struct {
	Interface string `json:"interface,omitempty"` // Network interface name
	Endpoint  string `json:"endpoint,omitempty"` // Tunnel endpoint
	Provider  string `json:"provider,omitempty"` // Tunnel provider (e.g., "wireguard", "openvpn")
}

// TunnelConnectionErrorEventData contains data for network.tunnel.connection_error events.
type TunnelConnectionErrorEventData struct {
	Interface string `json:"interface,omitempty"` // Network interface name
	Endpoint  string `json:"endpoint,omitempty"`  // Tunnel endpoint
	Provider  string `json:"provider,omitempty"`  // Tunnel provider
	Error     string `json:"error"`               // Error message
	Retryable bool   `json:"retryable,omitempty"` // Whether the error is retryable
}

// TunnelConnectedEventData contains data for network.tunnel.connected events.
type TunnelConnectedEventData struct {
	Interface   string `json:"interface"`              // Network interface name
	Endpoint    string `json:"endpoint,omitempty"`     // Tunnel endpoint
	Provider    string `json:"provider,omitempty"`     // Tunnel provider
	Reconnected bool   `json:"reconnected,omitempty"` // Whether this is a reconnection
	IPAddress   string `json:"ip_address,omitempty"`  // Assigned IP address
}

// TunnelDisconnectedEventData contains data for network.tunnel.disconnected events.
type TunnelDisconnectedEventData struct {
	Interface string `json:"interface"`          // Network interface name
	Endpoint  string `json:"endpoint,omitempty"` // Tunnel endpoint
	Provider  string `json:"provider,omitempty"` // Tunnel provider
	Reason    string `json:"reason,omitempty"`   // Disconnection reason
}

// TransportConnectingEventData contains data for network.transport.connecting events.
type TransportConnectingEventData struct {
	Service   string `json:"service"`            // Transport service name (e.g., "https-server", "https-client")
	Endpoint  string `json:"endpoint,omitempty"` // Transport endpoint
	Protocol  string `json:"protocol,omitempty"`  // Transport protocol (e.g., "https", "http2")
}

// TransportConnectionErrorEventData contains data for network.transport.connection_error events.
type TransportConnectionErrorEventData struct {
	Service   string `json:"service"`            // Transport service name
	Endpoint  string `json:"endpoint,omitempty"` // Transport endpoint
	Protocol  string `json:"protocol,omitempty"`  // Transport protocol
	Error     string `json:"error"`              // Error message
	Retryable bool   `json:"retryable,omitempty"` // Whether the error is retryable
}

// TransportConnectedEventData contains data for network.transport.connected events.
type TransportConnectedEventData struct {
	Service    string `json:"service"`              // Transport service name
	Endpoint   string `json:"endpoint,omitempty"`  // Transport endpoint
	Protocol   string `json:"protocol,omitempty"`  // Transport protocol
	Reconnected bool  `json:"reconnected,omitempty"` // Whether this is a reconnection
}

// TransportDisconnectedEventData contains data for network.transport.disconnected events.
type TransportDisconnectedEventData struct {
	Service string `json:"service"`            // Transport service name
	Endpoint string `json:"endpoint,omitempty"` // Transport endpoint
	Protocol string `json:"protocol,omitempty"` // Transport protocol
	Reason   string `json:"reason,omitempty"`  // Disconnection reason
}

// EdgeAuthenticatedEventData contains data for edge.authenticated events.
type EdgeAuthenticatedEventData struct {
	EdgeID     string    `json:"edge_id"`              // Edge ID
	VMEndpoint string    `json:"vm_endpoint,omitempty"` // VM endpoint
	Timestamp  time.Time `json:"timestamp"`            // Authentication timestamp
}

// RateLimitExceededEventData contains data for vm_gateway.rate_limit_exceeded events.
type RateLimitExceededEventData struct {
	ClientFingerprint string `json:"client_fingerprint"` // Client certificate fingerprint
	Endpoint          string `json:"endpoint"`           // Endpoint path
	LimitPerMinute    int    `json:"limit_per_minute"`   // Rate limit per minute
	RetryAfterSeconds int    `json:"retry_after_seconds"` // Retry after seconds
}

// TimeSyncCriticalDriftEventData contains data for time_sync.critical_drift events.
type TimeSyncCriticalDriftEventData struct {
	DriftMinutes      float64 `json:"drift_minutes"`       // Clock drift in minutes
	CriticalThreshold float64 `json:"critical_threshold"`   // Critical drift threshold in minutes
	EdgeTime          string  `json:"edge_time"`           // Edge time (RFC3339)
	VMTime            string  `json:"vm_time"`            // VM time (RFC3339)
	ToleranceMinutes  float64 `json:"tolerance_minutes"`   // Tolerance in minutes
}

// TimeSyncDriftWarningEventData contains data for time_sync.drift_warning events.
type TimeSyncDriftWarningEventData struct {
	DriftMinutes     float64 `json:"drift_minutes"`      // Clock drift in minutes
	ToleranceMinutes float64 `json:"tolerance_minutes"`  // Tolerance in minutes
	EdgeTime         string  `json:"edge_time"`          // Edge time (RFC3339)
	VMTime           string  `json:"vm_time"`           // VM time (RFC3339)
}

// CertificateRotationScheduledEventData contains data for certificate.rotation_scheduled events.
type CertificateRotationScheduledEventData struct {
	ScheduledAt time.Time `json:"scheduled_at"` // Scheduled rotation time
}

// CertificateRotationCompletedEventData contains data for certificate.rotation_completed events.
type CertificateRotationCompletedEventData struct {
	OldFingerprint string    `json:"old_fingerprint"`  // Old certificate fingerprint
	NewFingerprint string    `json:"new_fingerprint"`  // New certificate fingerprint
	GracePeriodEnd time.Time `json:"grace_period_end"`  // Grace period end time
}

// CertificateRotationFailedEventData contains data for certificate.rotation_failed events.
type CertificateRotationFailedEventData struct {
	Error string `json:"error"` // Error message
}

// DeviceEventData contains data for device connection/disconnection events.
// Used for device.connected and device.disconnected event types.
type DeviceEventData struct {
	DeviceID string  `json:"device_id,omitempty"` // Device ID (device-agnostic)
	URL      string  `json:"url,omitempty"`       // Device URL/endpoint
	Reason   string  `json:"reason,omitempty"`    // Disconnection reason (for disconnected events)
}

// DeviceFrameReceivedEventData contains data for device frame received events.
// Used for raw_device_data.frame_received and similar frame events.
type DeviceFrameReceivedEventData struct {
	DeviceID  string `json:"device_id"`  // Device ID (device-agnostic)
	FrameData []byte `json:"frame_data"` // Frame data as bytes (will be base64 encoded in JSON)
}

// DeviceDiscoveredEventData contains data for device.discovered events.
// Used when a device (camera, sensor, etc.) is discovered on the network or system.
type DeviceDiscoveredEventData struct {
	DeviceID    string  `json:"device_id"`              // Device ID (device-agnostic)
	Name        string  `json:"name,omitempty"`         // Device name
	Type        string  `json:"type,omitempty"`         // Device type (e.g., "camera", "sensor")
	Manufacturer string `json:"manufacturer,omitempty"` // Device manufacturer
	Model       string  `json:"model,omitempty"`        // Device model
	IPAddress   string  `json:"ip_address,omitempty"`   // IP address (for network devices)
	DevicePath  string  `json:"device_path,omitempty"`  // Device path (for USB/local devices)
}

// DeviceRegisteredEventData contains data for device.registered events.
// Used when a device is registered with the system.
type DeviceRegisteredEventData struct {
	DeviceID string `json:"device_id"`        // Device ID (device-agnostic)
	Name     string `json:"name,omitempty"`    // Device name
	Type     string `json:"type,omitempty"`    // Device type (e.g., "camera", "sensor")
}

// DataUnitSavedEventData contains data for data_unit.saved events.
// Used when a data unit (screenshot, sensor reading, etc.) is saved.
type DataUnitSavedEventData struct {
	DataUnitID string `json:"data_unit_id"`   // Data unit ID (e.g., screenshot_id)
	DeviceID   string `json:"device_id"`      // Device ID (device-agnostic)
	Label      string `json:"label,omitempty"` // Label for the data unit
}

// DataUnitUpdatedEventData contains data for data_unit.updated events.
// Used when a data unit is updated.
type DataUnitUpdatedEventData struct {
	DataUnitID string `json:"data_unit_id"` // Data unit ID (e.g., screenshot_id)
}

// DataUnitDeletedEventData contains data for data_unit.deleted events.
// Used when a data unit is deleted.
type DataUnitDeletedEventData struct {
	DataUnitID string `json:"data_unit_id"` // Data unit ID (e.g., screenshot_id)
	DeviceID   string `json:"device_id"`    // Device ID (device-agnostic)
}

// StorageEventData contains data for storage-related events.
// Used for storage.warning, storage.full, storage.quota_exceeded, storage.cleanup_started,
// storage.cleanup_completed, storage.corruption_detected, storage.schema_migration_started,
// and storage.schema_migration_completed events.
type StorageEventData struct {
	// For quota events (warning, full, quota_exceeded)
	UsedBytes      *int64   `json:"used_bytes,omitempty"`      // Current storage usage in bytes
	LimitBytes     *int64   `json:"limit_bytes,omitempty"`      // Storage limit in bytes
	UsagePercent   *float64 `json:"usage_percent,omitempty"`   // Usage percentage (0-100)
	
	// For cleanup events (cleanup_started, cleanup_completed)
	RecordsDeleted   *int64         `json:"records_deleted,omitempty"`   // Number of records deleted
	SpaceFreedBytes  *int64         `json:"space_freed_bytes,omitempty"`  // Space freed in bytes
	BucketsProcessed *int           `json:"buckets_processed,omitempty"`  // Number of buckets processed
	Duration         *string        `json:"duration,omitempty"`           // Cleanup duration (e.g., "5s")
	
	// For corruption events (corruption_detected)
	ErrorCount       *int           `json:"error_count,omitempty"`         // Number of integrity errors
	ErrorDetails     *string        `json:"error_details,omitempty"`      // Error details
	
	// For schema migration events (schema_migration_started, schema_migration_completed)
	SchemaVersion    *int           `json:"schema_version,omitempty"`    // Schema version
	Description      *string        `json:"description,omitempty"`       // Migration description
	
	// Additional metadata
	Metadata         map[string]interface{} `json:"metadata,omitempty"`    // Additional event metadata
}

// AuditLogEventData contains data for audit log operational events.
// Used for audit_log.queue_full, audit_log.queue_resumed, audit_log.sync_failed,
// audit_log.sync_succeeded, audit_log.tamper_detected, audit_log.cleanup_started,
// audit_log.cleanup_completed, and audit_log.health_degraded events.
type AuditLogEventData struct {
	// For queue events (queue_full, queue_resumed)
	QueueDepth      *int     `json:"queue_depth,omitempty"`      // Current queue depth
	QueueMaxSize    *int     `json:"queue_max_size,omitempty"`   // Maximum queue size
	QueueUsagePercent *float64 `json:"queue_usage_percent,omitempty"` // Queue usage percentage (0-100)

	// For sync events (sync_failed, sync_succeeded)
	EntriesSynced   *int64   `json:"entries_synced,omitempty"`   // Number of entries synced
	SyncDuration    *string  `json:"sync_duration,omitempty"`    // Sync duration (e.g., "5s")
	ErrorMessage    *string  `json:"error_message,omitempty"`    // Error message (for sync_failed)

	// For tamper detection events (tamper_detected)
	BrokenLinks     *int     `json:"broken_links,omitempty"`     // Number of broken links detected
	TamperIndicators *int    `json:"tamper_indicators,omitempty"` // Number of tamper indicators
	VerifiedEntries *int     `json:"verified_entries,omitempty"` // Number of verified entries
	TotalEntries    *int     `json:"total_entries,omitempty"`    // Total entries in chain

	// For cleanup events (cleanup_started, cleanup_completed)
	EntriesDeleted  *int64   `json:"entries_deleted,omitempty"`  // Number of entries deleted
	EntriesSkipped  *int64   `json:"entries_skipped,omitempty"`  // Number of entries skipped
	CleanupDuration *string  `json:"cleanup_duration,omitempty"` // Cleanup duration (e.g., "30s")

	// For health events (health_degraded)
	HealthStatus    *string  `json:"health_status,omitempty"`    // Health status (e.g., "degraded", "warning")
	HealthReason    *string  `json:"health_reason,omitempty"`    // Reason for degraded health

	// Additional metadata
	Metadata        map[string]interface{} `json:"metadata,omitempty"` // Additional event metadata
}
