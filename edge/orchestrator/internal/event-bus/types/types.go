package types

import (
	"encoding/json"
	"time"
)

// EventType represents the type of an event.
// This is a custom type with enum constants for all possible event types.
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
	EventTypeStorageFull    EventType = "storage.full"
	EventTypeStorageWarning EventType = "storage.warning"

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
type EventData interface {
	ModelDeployedEventData | ModelDeploymentStatusEventData | SnapshotRequestedEventData | CapabilitiesReceivedEventData | NetworkEventData | DeviceEventData | DeviceFrameReceivedEventData | DeviceDiscoveredEventData | DeviceRegisteredEventData | DataUnitSavedEventData | DataUnitUpdatedEventData | DataUnitDeletedEventData
}

// Event represents an application-level event flowing through the bus.
// T must be one of the types in the EventData type set.
type Event[T EventData] struct {
	Type           EventType
	Source         string    // Component that emitted the event
	Timestamp      time.Time // When the event was created
	Data           T         // Type-safe event-specific payload
	SequenceNumber int64     // Sequence number per source (0 if not set)
}

// EventAny is a type-erased event for use in EventBus interface.
// It stores data as JSON bytes to maintain type safety while allowing
// the bus to handle all event types uniformly.
type EventAny struct {
	Type           EventType
	Source         string
	Timestamp      time.Time
	Data           json.RawMessage // Type-erased data as JSON
	SequenceNumber int64
}

// ToEventAny converts a typed Event[T] to EventAny for bus operations.
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

// EventProcessingStatus represents the processing status of an event
type EventProcessingStatus string

const (
	EventStatusPending    EventProcessingStatus = "pending"     // Event is pending processing
	EventStatusProcessing EventProcessingStatus = "processing"  // Event is currently being processed
	EventStatusSucceeded  EventProcessingStatus = "succeeded"   // Event was successfully processed
	EventStatusFailed     EventProcessingStatus = "failed"      // Event processing failed (will be retried)
	EventStatusDeadLetter EventProcessingStatus = "dead_letter" // Event moved to dead letter queue after max retries
)

// OrderingMode defines how events are ordered
type OrderingMode string

const (
	OrderingModeNone       OrderingMode = "none"        // No ordering guarantees
	OrderingModeBestEffort OrderingMode = "best_effort" // Best-effort ordering (reorder if possible)
	OrderingModeStrict     OrderingMode = "strict"      // Strict ordering (buffer and wait for missing sequences)
)

// EventBusConfig contains event bus configuration
type EventBusConfig struct {
	Provider           string        `yaml:"provider"`             // Event bus provider (must be "metastorage")
	BufferSize         int           `yaml:"buffer_size"`          // Buffer size for event channels
	DataDir            string        `yaml:"data_dir"`             // Data directory (deprecated, no longer used)
	MaxRetries         int           `yaml:"max_retries"`          // Maximum number of retry attempts for failed events
	InitialBackoff     time.Duration `yaml:"initial_backoff"`      // Initial backoff duration for retries
	MaxBackoff         time.Duration `yaml:"max_backoff"`          // Maximum backoff duration (caps exponential backoff)
	BackoffMultiplier  float64       `yaml:"backoff_multiplier"`   // Multiplier for exponential backoff (e.g., 2.0)
	RetryInterval      time.Duration `yaml:"retry_interval"`       // Interval between retry worker runs
	OrderingMode       string        `yaml:"ordering_mode"`        // Event ordering mode: "none", "best_effort", "strict"
	OrderingBufferSize int           `yaml:"ordering_buffer_size"` // Buffer size for out-of-order event buffering
	OrderingTimeout    time.Duration `yaml:"ordering_timeout"`     // Timeout for waiting for missing sequences in strict mode
}

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
