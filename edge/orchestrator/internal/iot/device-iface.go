package iot

import (
	"context"
	"time"
)

// Device represents a generic IoT device that can be discovered, registered, and managed by the Edge.
// This interface abstracts common device operations while allowing device-specific capabilities
// through the capability system.
//
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_device.go -package=mocks
type Device interface {
	// Lifecycle methods
	// Start initializes the device and begins operation
	Start(ctx context.Context) error

	// Stop stops the device and releases resources
	Stop(ctx context.Context) error

	// GetID returns the unique identifier for this device
	GetID() string

	// GetMetadata returns device metadata including type, capabilities, and status
	GetMetadata() DeviceMetadata

	// UpdateMetadata updates device metadata
	UpdateMetadata(ctx context.Context, updates *DeviceMetadataUpdate) error

	// Enable enables the device for operation
	Enable(ctx context.Context) error

	// Disable disables the device
	Disable(ctx context.Context) error

	// IsEnabled returns whether the device is currently enabled
	IsEnabled() bool

	// GetStatus returns the current operational status of the device
	GetStatus() DeviceStatus

	// Capability-based operations
	// These methods check device capabilities before performing operations

	// HasCapability checks if the device supports a specific capability
	HasCapability(capability DeviceCapability) bool

	// GetCapabilities returns all capabilities supported by this device
	GetCapabilities() DeviceCapabilities

	// Data collection operations (capability-dependent)
	// These methods are only available if the device supports the corresponding capability

	// CaptureData captures data from the device (e.g., frame, sensor reading, audio sample)
	// Returns capability-specific data format
	// Requires: DeviceCapabilityDataCapture
	CaptureData(ctx context.Context) (*DeviceData, error)

	// StartDataStream starts a continuous data stream from the device
	// Returns a channel that receives data samples
	// Requires: DeviceCapabilityDataStreaming
	StartDataStream(ctx context.Context) (<-chan *DeviceData, error)

	// StopDataStream stops an active data stream
	// Requires: DeviceCapabilityDataStreaming
	StopDataStream(ctx context.Context) error

	// ReadSensor reads a sensor value from the device
	// Returns sensor-specific data
	// Requires: DeviceCapabilitySensorReadings
	ReadSensor(ctx context.Context, sensorType string) (*SensorReading, error)

	// ReadAllSensors reads all available sensor values
	// Requires: DeviceCapabilitySensorReadings
	ReadAllSensors(ctx context.Context) (map[string]*SensorReading, error)

	// Control operations (capability-dependent)
	// These methods are only available if the device supports the corresponding capability

	// ExecuteCommand executes a control command on the device
	// Command format is device-specific
	// Requires: DeviceCapabilityControl
	ExecuteCommand(ctx context.Context, command DeviceCommand) error

	// GetAvailableCommands returns list of commands supported by this device
	// Requires: DeviceCapabilityControl
	GetAvailableCommands(ctx context.Context) ([]DeviceCommand, error)
}

// DeviceMetadata contains metadata about a device
type DeviceMetadata struct {
	// Core identification
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Type         DeviceType `json:"type"`
	Manufacturer string     `json:"manufacturer,omitempty"`
	Model        string     `json:"model,omitempty"`
	SerialNumber string     `json:"serial_number,omitempty"`
	Firmware     string     `json:"firmware,omitempty"`

	// Status
	Enabled      bool         `json:"enabled"`
	Status       DeviceStatus `json:"status"`
	LastSeen     *time.Time   `json:"last_seen,omitempty"`
	DiscoveredAt time.Time    `json:"discovered_at"`

	// Capabilities
	Capabilities DeviceCapabilities `json:"capabilities"`

	// Device-specific configuration
	// This is a flexible map that can hold device-type-specific configuration
	Config map[string]interface{} `json:"config,omitempty"`

	// Network/connection information (for network devices)
	IPAddress  string   `json:"ip_address,omitempty"`
	MACAddress string   `json:"mac_address,omitempty"`
	Endpoints  []string `json:"endpoints,omitempty"` // e.g., RTSP URLs, API endpoints

	// Physical connection information (for USB/serial devices)
	DevicePath string `json:"device_path,omitempty"` // e.g., /dev/video0, /dev/ttyUSB0

	// Location/placement information
	Location string `json:"location,omitempty"`
	Zone     string `json:"zone,omitempty"` // Security zone identifier

	// Additional metadata
	Tags     []string               `json:"tags,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceMetadataUpdate represents updates to device metadata
type DeviceMetadataUpdate struct {
	Name     *string                `json:"name,omitempty"`
	Enabled  *bool                  `json:"enabled,omitempty"`
	Config   map[string]interface{} `json:"config,omitempty"`
	Location *string                `json:"location,omitempty"`
	Zone     *string                `json:"zone,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceType represents the type of IoT device
type DeviceType string

const (
	// Video devices
	DeviceTypeCamera DeviceType = "camera" // CCTV camera (USB, RTSP, ONVIF)

	// Sensor devices
	DeviceTypeMotionSensor      DeviceType = "motion_sensor"      // Motion detection sensor
	DeviceTypeTemperatureSensor DeviceType = "temperature_sensor" // Temperature sensor
	DeviceTypeHumiditySensor    DeviceType = "humidity_sensor"    // Humidity sensor
	DeviceTypeDoorSensor        DeviceType = "door_sensor"        // Door open/close sensor
	DeviceTypeWindowSensor      DeviceType = "window_sensor"      // Window open/close sensor
	DeviceTypeSmokeDetector     DeviceType = "smoke_detector"     // Smoke/fire detector
	DeviceTypeCO2Sensor         DeviceType = "co2_sensor"         // CO2 sensor
	DeviceTypeGenericSensor     DeviceType = "sensor"             // Generic sensor device

	// Access control devices
	DeviceTypeDoorLock   DeviceType = "door_lock"   // Smart door lock
	DeviceTypeKeypad     DeviceType = "keypad"      // Access keypad
	DeviceTypeCardReader DeviceType = "card_reader" // RFID/NFC card reader
	DeviceTypeBiometric  DeviceType = "biometric"   // Biometric scanner (fingerprint, face)

	// Audio devices
	DeviceTypeMicrophone DeviceType = "microphone" // Audio capture device

	// Other device types
	DeviceTypeUnknown DeviceType = "unknown" // Unknown/unsupported device type
)

// DeviceStatus represents the operational status of a device
type DeviceStatus string

const (
	DeviceStatusUnknown     DeviceStatus = "unknown"     // Status not yet determined
	DeviceStatusOnline      DeviceStatus = "online"      // Device is online and operational
	DeviceStatusOffline     DeviceStatus = "offline"     // Device is offline or unreachable
	DeviceStatusConnecting  DeviceStatus = "connecting"  // Device is in the process of connecting
	DeviceStatusError       DeviceStatus = "error"       // Device has an error condition
	DeviceStatusMaintenance DeviceStatus = "maintenance" // Device is in maintenance mode
)

// DeviceCapability represents a capability that a device can support
type DeviceCapability string

const (
	// Data capture capabilities
	DeviceCapabilityDataCapture   DeviceCapability = "data_capture"   // Device can capture data (frames, samples, etc.)
	DeviceCapabilityDataStreaming DeviceCapability = "data_streaming" // Device can stream continuous data

	// Video-specific capabilities
	DeviceCapabilityVideoCapture   DeviceCapability = "video_capture"   // Device can capture video frames
	DeviceCapabilityVideoStreaming DeviceCapability = "video_streaming" // Device can stream video
	DeviceCapabilityVideoRecording DeviceCapability = "video_recording" // Device can record video clips
	DeviceCapabilitySnapshot       DeviceCapability = "snapshot"        // Device can capture snapshots/images

	// Audio capabilities
	DeviceCapabilityAudioCapture   DeviceCapability = "audio_capture"   // Device can capture audio
	DeviceCapabilityAudioStreaming DeviceCapability = "audio_streaming" // Device can stream audio

	// Sensor capabilities
	DeviceCapabilitySensorReadings DeviceCapability = "sensor_readings" // Device can read sensor values

	// Control capabilities
	DeviceCapabilityControl DeviceCapability = "control" // Device can be controlled (e.g., PTZ, door lock)

	// Access control capabilities
	DeviceCapabilityAccessControl DeviceCapability = "access_control" // Device can control access (locks, gates)

	// PTZ (Pan-Tilt-Zoom) capabilities (for cameras)
	DeviceCapabilityPTZ DeviceCapability = "ptz" // Camera supports PTZ control

	// Motion detection capabilities
	DeviceCapabilityMotionDetection DeviceCapability = "motion_detection" // Device has built-in motion detection

	// Event generation capabilities
	DeviceCapabilityEventGeneration DeviceCapability = "event_generation" // Device can generate events (door open, motion detected, etc.)
)

// DeviceCapabilities is a set of capabilities supported by a device
type DeviceCapabilities map[DeviceCapability]bool

// Has returns true if the device has the specified capability
func (c DeviceCapabilities) Has(capability DeviceCapability) bool {
	return c[capability]
}

// Add adds a capability to the set
func (c DeviceCapabilities) Add(capability DeviceCapability) {
	c[capability] = true
}

// Remove removes a capability from the set
func (c DeviceCapabilities) Remove(capability DeviceCapability) {
	delete(c, capability)
}

// HasAll returns true if the device has all of the specified capabilities
func (c DeviceCapabilities) HasAll(capabilities ...DeviceCapability) bool {
	for _, cap := range capabilities {
		if !c.Has(cap) {
			return false
		}
	}
	return true
}

// HasAny returns true if the device has any of the specified capabilities
func (c DeviceCapabilities) HasAny(capabilities ...DeviceCapability) bool {
	for _, cap := range capabilities {
		if c.Has(cap) {
			return true
		}
	}
	return false
}

// Count returns the number of capabilities
func (c DeviceCapabilities) Count() int {
	return len(c)
}

// List returns all capabilities as a slice
func (c DeviceCapabilities) List() []DeviceCapability {
	result := make([]DeviceCapability, 0, len(c))
	for cap := range c {
		result = append(result, cap)
	}
	return result
}

// Intersect returns capabilities that are present in both sets
func (c DeviceCapabilities) Intersect(other DeviceCapabilities) DeviceCapabilities {
	result := make(DeviceCapabilities)
	for cap := range c {
		if other.Has(cap) {
			result.Add(cap)
		}
	}
	return result
}

// Union returns capabilities that are present in either set
func (c DeviceCapabilities) Union(other DeviceCapabilities) DeviceCapabilities {
	result := make(DeviceCapabilities)
	for cap := range c {
		result.Add(cap)
	}
	for cap := range other {
		result.Add(cap)
	}
	return result
}

// Difference returns capabilities that are in this set but not in the other
func (c DeviceCapabilities) Difference(other DeviceCapabilities) DeviceCapabilities {
	result := make(DeviceCapabilities)
	for cap := range c {
		if !other.Has(cap) {
			result.Add(cap)
		}
	}
	return result
}

// DeviceData represents data captured from a device
// The format depends on the device type and capability
type DeviceData struct {
	// Core fields
	DeviceID  string         `json:"device_id"`
	Timestamp time.Time      `json:"timestamp"`
	DataType  DeviceDataType `json:"data_type"`

	// Data payload (format depends on DataType)
	// For video: JPEG-encoded frame bytes
	// For audio: PCM/WAV audio samples
	// For sensors: JSON-encoded sensor values
	Data []byte `json:"data"`

	// Metadata
	Width    int                    `json:"width,omitempty"`    // For video/image data
	Height   int                    `json:"height,omitempty"`   // For video/image data
	Format   string                 `json:"format,omitempty"`   // Data format (jpeg, png, wav, json, etc.)
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Additional metadata
}

// DeviceDataType represents the type of data captured
type DeviceDataType string

const (
	DeviceDataTypeVideoFrame    DeviceDataType = "video_frame"    // Video frame (JPEG/PNG)
	DeviceDataTypeAudioSample   DeviceDataType = "audio_sample"   // Audio sample (PCM/WAV)
	DeviceDataTypeSensorReading DeviceDataType = "sensor_reading" // Sensor reading (JSON)
	DeviceDataTypeEvent         DeviceDataType = "event"          // Device event (JSON)
	DeviceDataTypeGeneric       DeviceDataType = "generic"        // Generic data
)

// SensorReading represents a sensor value reading
type SensorReading struct {
	SensorType string                 `json:"sensor_type"`    // e.g., "temperature", "humidity", "motion"
	Value      float64                `json:"value"`          // Numeric value
	Unit       string                 `json:"unit,omitempty"` // Unit of measurement (e.g., "Celsius", "percent")
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// DeviceCommand represents a control command for a device
type DeviceCommand struct {
	CommandType string                 `json:"command_type"` // e.g., "ptz_move", "lock_door", "set_brightness"
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// DeviceFilters contains filters for listing devices
type DeviceFilters struct {
	Type       *DeviceType       `json:"type,omitempty"`
	Capability *DeviceCapability `json:"capability,omitempty"`
	Enabled    *bool             `json:"enabled,omitempty"`
	Status     *DeviceStatus     `json:"status,omitempty"`
	Zone       *string           `json:"zone,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
}

// DeviceDataStream represents a continuous data stream from a device
type DeviceDataStream struct {
	DeviceID  string
	DataChan  <-chan *DeviceData
	Done      <-chan struct{}
	ErrorChan <-chan error
	Close     func() error
}

// DevicePlugin is an interface for device type plugins
// This allows new device types to be added via plugins
type DevicePlugin interface {
	// GetDeviceType returns the device type this plugin handles
	GetDeviceType() DeviceType

	// GetSupportedCapabilities returns the capabilities this device type can support
	GetSupportedCapabilities() []DeviceCapability

	// DiscoverDevices discovers devices of this type
	DiscoverDevices(ctx context.Context) ([]Device, error)

	// CreateDevice creates a device instance from metadata
	CreateDevice(ctx context.Context, metadata DeviceMetadata) (Device, error)

	// ValidateMetadata validates device metadata for this type
	ValidateMetadata(metadata DeviceMetadata) error
}
