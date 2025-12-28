package iot

import (
	"context"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// IoTService provides a unified, device-agnostic interface for IoT device management.
// It coordinates device discovery, registration, state management, data processing,
// and lifecycle hooks across all device types (cameras, sensors, etc.).
//
// The service is device-agnostic and works with any device type through the
// DevicePlugin system. Device-specific implementations (e.g., CCTV) are integrated
// via DevicePlugin adapters.
//
// Architecture:
//   - Device-agnostic: Works with any device type through plugins
//   - Plugin-based: New device types added via DevicePlugin implementations
//   - State management: Tracks device lifecycle through state machines
//   - Data processing: Processes device data through configurable pipelines
//   - Lifecycle hooks: Extensible hook system for custom behavior
//
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_iot_service.go -package=mocks
type IoTService interface {
	// Lifecycle methods

	// Start starts all underlying services (plugin registry, device registry, etc.).
	// Services are started in the correct order.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down all underlying services.
	// Services are stopped in reverse order.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// HealthSnapshot returns a comprehensive health snapshot of the service.
	// This includes device counts, plugin status, processing status, and sub-service health.
	// The snapshot is JSON-serializable and useful for debugging and monitoring.
	HealthSnapshot() IoTServiceStatus

	// Device Discovery

	// DiscoverDevices discovers devices using all registered plugins.
	// Returns devices discovered by all plugins.
	// This is device-agnostic and works with any device type that has a registered plugin.
	DiscoverDevices(ctx context.Context) ([]types.Device, error)

	// DiscoverDevicesByType discovers devices of a specific type using the appropriate plugin.
	// Returns error if no plugin is registered for the device type.
	DiscoverDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error)

	// Device Registry

	// RegisterDevice registers a discovered device.
	// This creates a state machine for the device and executes registration hooks.
	// Returns error if device is already registered or registration fails.
	RegisterDevice(ctx context.Context, device types.Device) error

	// GetDevice retrieves a device by ID.
	// Returns error if device is not found.
	GetDevice(ctx context.Context, deviceID string) (types.Device, error)

	// ListDevices lists all registered devices, optionally filtered by type or capability.
	// Returns empty slice if no devices match the filters.
	ListDevices(ctx context.Context, filters *types.DeviceFilters) ([]types.Device, error)

	// UpdateDevice updates device metadata.
	// Returns error if device is not found or update fails.
	UpdateDevice(ctx context.Context, deviceID string, updates *types.DeviceMetadataUpdate) error

	// DeleteDevice removes a device from the registry.
	// This also removes the device's state machine and executes teardown hooks.
	// Returns error if device is not found or deletion fails.
	DeleteDevice(ctx context.Context, deviceID string) error

	// GetDevicesByCapability returns all devices that support a specific capability.
	// This is useful for finding devices that can perform specific operations
	// (e.g., all devices that can capture video, all devices that can read sensors).
	GetDevicesByCapability(ctx context.Context, capability types.DeviceCapability) ([]types.Device, error)

	// GetDevicesByType returns all devices of a specific type.
	// This is useful for finding all devices of a particular category
	// (e.g., all cameras, all sensors).
	GetDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error)

	// State Management

	// GetStateMachine retrieves a state machine for a device.
	// Returns error if state machine is not found.
	GetStateMachine(ctx context.Context, deviceID string) (types.DeviceStateMachine, error)

	// GetStateMachinesByType returns all state machines for a specific device type.
	// This is useful for monitoring the state of all devices of a particular type.
	GetStateMachinesByType(ctx context.Context, deviceType types.DeviceType) ([]types.DeviceStateMachine, error)

	// Data Processing

	// ProcessDeviceData processes data from a device through the processing pipeline.
	// Returns the processing context with results.
	// Returns error if processing fails or service is not initialized.
	ProcessDeviceData(ctx context.Context, device types.Device, data *types.DeviceData) (*types.DataProcessingContext, error)

	// Plugin Management

	// RegisterPlugin registers a device plugin for a specific device type.
	// This is typically called during service initialization.
	// Returns error if plugin is already registered or validation fails.
	RegisterPlugin(ctx context.Context, plugin types.DevicePlugin) error

	// GetSupportedDeviceTypes returns all device types that have registered plugins.
	// This is useful for determining which device types can be discovered and managed.
	GetSupportedDeviceTypes(ctx context.Context) ([]types.DeviceType, error)
}

// IoTServiceStatus provides a comprehensive health snapshot of the IoT service.
// This is device-agnostic and works with any device type and plugin combination.
type IoTServiceStatus struct {
	// RegisteredDevices is the total number of registered devices
	RegisteredDevices int `json:"registered_devices"`

	// DevicesByType is a map of device type to count of registered devices
	DevicesByType map[types.DeviceType]int `json:"devices_by_type"`

	// PluginStatus is a map of device type to plugin status
	PluginStatus map[types.DeviceType]PluginStatus `json:"plugin_status"`

	// ProcessingStatus contains the status of the data processing pipeline
	ProcessingStatus ProcessingStatus `json:"processing_status"`

	// StateRegistrySize is the number of state machines in the registry
	StateRegistrySize int `json:"state_registry_size"`

	// SubServices contains status information for sub-services
	// Key is service name, value is service status
	SubServices map[string]ServiceStatus `json:"sub_services"`

	// Timestamp is when this snapshot was taken
	Timestamp time.Time `json:"timestamp"`
}

// PluginStatus represents the status of a device plugin.
type PluginStatus struct {
	// Registered indicates whether the plugin is registered
	Registered bool `json:"registered"`

	// Capabilities lists the capabilities supported by this plugin's device type
	Capabilities []types.DeviceCapability `json:"capabilities"`
}

// ProcessingStatus represents the status of the data processing pipeline.
type ProcessingStatus struct {
	// Enabled indicates whether processing is enabled
	Enabled bool `json:"enabled"`

	// RegisteredProcessors is the number of registered processors
	RegisteredProcessors int `json:"registered_processors"`
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

