package types

import "context"

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

// DevicePluginRegistry is an interface for managing device type plugins
// This allows new device types to be registered at runtime
//
//go:generate go run go.uber.org/mock/mockgen -destination=../mocks/mock_plugin_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types DevicePluginRegistry
type DevicePluginRegistry interface {
	// RegisterPlugin registers a device plugin for a specific device type
	// Returns error if plugin is already registered or validation fails
	RegisterPlugin(plugin DevicePlugin) error

	// UnregisterPlugin unregisters a device plugin
	// Returns error if plugin is not registered
	UnregisterPlugin(deviceType DeviceType) error

	// GetPlugin retrieves a plugin for a specific device type
	// Returns error if plugin is not found
	GetPlugin(deviceType DeviceType) (DevicePlugin, error)

	// ListPlugins returns all registered plugins
	ListPlugins() []DevicePlugin

	// GetPluginForDeviceType returns the plugin that handles a specific device type
	// This is an alias for GetPlugin for clarity
	GetPluginForDeviceType(deviceType DeviceType) (DevicePlugin, error)

	// DiscoverDevices discovers devices using all registered plugins
	// Returns devices discovered by all plugins
	DiscoverDevices(ctx context.Context) ([]Device, error)

	// DiscoverDevicesByType discovers devices of a specific type using the appropriate plugin
	DiscoverDevicesByType(ctx context.Context, deviceType DeviceType) ([]Device, error)

	// CreateDevice creates a device instance from metadata using the appropriate plugin
	CreateDevice(ctx context.Context, metadata DeviceMetadata) (Device, error)

	// ValidateMetadata validates device metadata using the appropriate plugin
	ValidateMetadata(metadata DeviceMetadata) error

	// GetSupportedDeviceTypes returns all device types that have registered plugins
	GetSupportedDeviceTypes() []DeviceType

	// IsDeviceTypeSupported checks if a device type has a registered plugin
	IsDeviceTypeSupported(deviceType DeviceType) bool
}

// PluginDiscoveryConfig contains configuration for plugin discovery
type PluginDiscoveryConfig struct {
	// Timeout for discovery operation
	Timeout int `json:"timeout,omitempty"`

	// Additional configuration
	Config map[string]interface{} `json:"config,omitempty"`
}

// PluginDiscoveryResult contains the result of a plugin discovery operation
type PluginDiscoveryResult struct {
	// DeviceType is the device type that was discovered
	DeviceType DeviceType `json:"device_type"`

	// Devices are the devices that were discovered
	Devices []Device `json:"devices"`

	// Error is any error that occurred during discovery
	Error error `json:"error,omitempty"`
}

