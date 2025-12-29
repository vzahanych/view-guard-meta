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
	EventTypeNetworkTunnelConnected       EventType = "network.tunnel.connected"
	EventTypeNetworkTunnelDisconnected    EventType = "network.tunnel.disconnected"
	EventTypeNetworkTransportConnected    EventType = "network.transport.connected"
	EventTypeNetworkTransportDisconnected EventType = "network.transport.disconnected"

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
	ModelDeployedEventData | ModelDeploymentStatusEventData | SnapshotRequestedEventData | CapabilitiesReceivedEventData | NetworkEventData | DeviceEventData | DeviceFrameReceivedEventData | DeviceDiscoveredEventData | DeviceRegisteredEventData | DataUnitSavedEventData | DataUnitUpdatedEventData | DataUnitDeletedEventData | StorageEventData
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
type NetworkEventData struct {
	Interface   string                 `json:"interface,omitempty"`   // Network interface name
	Endpoint    string                 `json:"endpoint,omitempty"`    // Network endpoint
	Reason      string                 `json:"reason,omitempty"`      // Disconnection reason
	Reconnected bool                   `json:"reconnected,omitempty"` // Whether this is a reconnection
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // Additional metadata
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
