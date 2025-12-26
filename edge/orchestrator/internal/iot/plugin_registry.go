package iot

import (
	"context"
	"fmt"
	"sync"
)

// DevicePluginRegistry is an interface for managing device type plugins
// This allows new device types to be registered at runtime
//
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_plugin_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DevicePluginRegistry
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

// devicePluginRegistryImpl is the default implementation of DevicePluginRegistry
type devicePluginRegistryImpl struct {
	// plugins maps device type to plugin
	plugins map[DeviceType]DevicePlugin

	// mu protects the plugins map
	mu sync.RWMutex
}

// NewDevicePluginRegistry creates a new device plugin registry
func NewDevicePluginRegistry() DevicePluginRegistry {
	return &devicePluginRegistryImpl{
		plugins: make(map[DeviceType]DevicePlugin),
	}
}

// RegisterPlugin registers a device plugin for a specific device type
func (r *devicePluginRegistryImpl) RegisterPlugin(plugin DevicePlugin) error {
	if plugin == nil {
		return fmt.Errorf("plugin cannot be nil")
	}

	deviceType := plugin.GetDeviceType()
	if deviceType == DeviceTypeUnknown {
		return fmt.Errorf("plugin device type cannot be unknown")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if plugin is already registered
	if existing, exists := r.plugins[deviceType]; exists {
		return fmt.Errorf("plugin for device type %s is already registered: %T", deviceType, existing)
	}

	// Validate plugin
	if err := r.validatePlugin(plugin); err != nil {
		return fmt.Errorf("plugin validation failed: %w", err)
	}

	// Register plugin
	r.plugins[deviceType] = plugin

	return nil
}

// UnregisterPlugin unregisters a device plugin
func (r *devicePluginRegistryImpl) UnregisterPlugin(deviceType DeviceType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[deviceType]; !exists {
		return fmt.Errorf("plugin for device type %s is not registered", deviceType)
	}

	delete(r.plugins, deviceType)
	return nil
}

// GetPlugin retrieves a plugin for a specific device type
func (r *devicePluginRegistryImpl) GetPlugin(deviceType DeviceType) (DevicePlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.plugins[deviceType]
	if !exists {
		return nil, fmt.Errorf("plugin for device type %s is not registered", deviceType)
	}

	return plugin, nil
}

// ListPlugins returns all registered plugins
func (r *devicePluginRegistryImpl) ListPlugins() []DevicePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]DevicePlugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}

// GetPluginForDeviceType returns the plugin that handles a specific device type
func (r *devicePluginRegistryImpl) GetPluginForDeviceType(deviceType DeviceType) (DevicePlugin, error) {
	return r.GetPlugin(deviceType)
}

// DiscoverDevices discovers devices using all registered plugins
func (r *devicePluginRegistryImpl) DiscoverDevices(ctx context.Context) ([]Device, error) {
	r.mu.RLock()
	plugins := make([]DevicePlugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	r.mu.RUnlock()

	allDevices := make([]Device, 0)

	// Discover devices from all plugins
	for _, plugin := range plugins {
		devices, err := plugin.DiscoverDevices(ctx)
		if err != nil {
			// Log error but continue with other plugins
			// In production, you might want to use a logger here
			continue
		}
		allDevices = append(allDevices, devices...)
	}

	return allDevices, nil
}

// DiscoverDevicesByType discovers devices of a specific type using the appropriate plugin
func (r *devicePluginRegistryImpl) DiscoverDevicesByType(ctx context.Context, deviceType DeviceType) ([]Device, error) {
	plugin, err := r.GetPlugin(deviceType)
	if err != nil {
		return nil, err
	}

	return plugin.DiscoverDevices(ctx)
}

// CreateDevice creates a device instance from metadata using the appropriate plugin
func (r *devicePluginRegistryImpl) CreateDevice(ctx context.Context, metadata DeviceMetadata) (Device, error) {
	if metadata.Type == DeviceTypeUnknown {
		return nil, fmt.Errorf("device type cannot be unknown")
	}

	plugin, err := r.GetPlugin(metadata.Type)
	if err != nil {
		return nil, fmt.Errorf("no plugin registered for device type %s: %w", metadata.Type, err)
	}

	// Validate metadata
	if err := plugin.ValidateMetadata(metadata); err != nil {
		return nil, fmt.Errorf("metadata validation failed: %w", err)
	}

	// Create device
	return plugin.CreateDevice(ctx, metadata)
}

// ValidateMetadata validates device metadata using the appropriate plugin
func (r *devicePluginRegistryImpl) ValidateMetadata(metadata DeviceMetadata) error {
	if metadata.Type == DeviceTypeUnknown {
		return fmt.Errorf("device type cannot be unknown")
	}

	plugin, err := r.GetPlugin(metadata.Type)
	if err != nil {
		return fmt.Errorf("no plugin registered for device type %s: %w", metadata.Type, err)
	}

	return plugin.ValidateMetadata(metadata)
}

// GetSupportedDeviceTypes returns all device types that have registered plugins
func (r *devicePluginRegistryImpl) GetSupportedDeviceTypes() []DeviceType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]DeviceType, 0, len(r.plugins))
	for deviceType := range r.plugins {
		types = append(types, deviceType)
	}

	return types
}

// IsDeviceTypeSupported checks if a device type has a registered plugin
func (r *devicePluginRegistryImpl) IsDeviceTypeSupported(deviceType DeviceType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.plugins[deviceType]
	return exists
}

// validatePlugin validates a plugin before registration
func (r *devicePluginRegistryImpl) validatePlugin(plugin DevicePlugin) error {
	// Check device type
	deviceType := plugin.GetDeviceType()
	if deviceType == DeviceTypeUnknown {
		return fmt.Errorf("plugin device type cannot be unknown")
	}

	// Check supported capabilities
	capabilities := plugin.GetSupportedCapabilities()
	if len(capabilities) == 0 {
		return fmt.Errorf("plugin must support at least one capability")
	}

	// Validate capability dependencies
	for _, cap := range capabilities {
		// Check if capability is valid (basic validation)
		if cap == "" {
			return fmt.Errorf("plugin has empty capability")
		}
	}

	return nil
}

// PluginDiscoveryConfig contains configuration for plugin discovery
type PluginDiscoveryConfig struct {
	// AutoDiscover enables automatic plugin discovery
	AutoDiscover bool `json:"auto_discover"`

	// PluginPaths are paths to search for plugins (for future file-based discovery)
	PluginPaths []string `json:"plugin_paths,omitempty"`

	// EnabledPlugins is a list of plugin identifiers to enable (for selective loading)
	EnabledPlugins []string `json:"enabled_plugins,omitempty"`

	// DisabledPlugins is a list of plugin identifiers to disable
	DisabledPlugins []string `json:"disabled_plugins,omitempty"`
}

// PluginDiscoveryResult represents the result of plugin discovery
type PluginDiscoveryResult struct {
	// DiscoveredPlugins are plugins that were discovered
	DiscoveredPlugins []DevicePlugin `json:"discovered_plugins"`

	// FailedPlugins are plugins that failed to load
	FailedPlugins []PluginDiscoveryError `json:"failed_plugins,omitempty"`

	// TotalPlugins is the total number of plugins discovered
	TotalPlugins int `json:"total_plugins"`
}

// PluginDiscoveryError represents an error during plugin discovery
type PluginDiscoveryError struct {
	// PluginIdentifier is the identifier of the plugin that failed
	PluginIdentifier string `json:"plugin_identifier"`

	// Error is the error that occurred
	Error error `json:"error"`
}

// DiscoverPlugins discovers and registers plugins
// This is a placeholder for future file-based plugin discovery
// Currently, plugins must be registered manually via RegisterPlugin
func DiscoverPlugins(ctx context.Context, registry DevicePluginRegistry, config *PluginDiscoveryConfig) (*PluginDiscoveryResult, error) {
	if registry == nil {
		return nil, fmt.Errorf("plugin registry cannot be nil")
	}

	result := &PluginDiscoveryResult{
		DiscoveredPlugins: make([]DevicePlugin, 0),
		FailedPlugins:     make([]PluginDiscoveryError, 0),
	}

	// For now, plugin discovery is manual (plugins must be registered via RegisterPlugin)
	// Future implementation could:
	// 1. Scan plugin paths for .so files (Go plugins)
	// 2. Load plugins dynamically
	// 3. Validate and register plugins

	// Return currently registered plugins
	registeredPlugins := registry.ListPlugins()
	result.DiscoveredPlugins = registeredPlugins
	result.TotalPlugins = len(registeredPlugins)

	return result, nil
}

// PluginManager manages device plugins and provides high-level operations
type PluginManager struct {
	registry DevicePluginRegistry
}

// NewPluginManager creates a new plugin manager
func NewPluginManager(registry DevicePluginRegistry) *PluginManager {
	return &PluginManager{
		registry: registry,
	}
}

// RegisterPlugin registers a plugin with the manager
func (m *PluginManager) RegisterPlugin(plugin DevicePlugin) error {
	return m.registry.RegisterPlugin(plugin)
}

// UnregisterPlugin unregisters a plugin
func (m *PluginManager) UnregisterPlugin(deviceType DeviceType) error {
	return m.registry.UnregisterPlugin(deviceType)
}

// GetPlugin retrieves a plugin
func (m *PluginManager) GetPlugin(deviceType DeviceType) (DevicePlugin, error) {
	return m.registry.GetPlugin(deviceType)
}

// DiscoverAllDevices discovers devices from all registered plugins
func (m *PluginManager) DiscoverAllDevices(ctx context.Context) ([]Device, error) {
	return m.registry.DiscoverDevices(ctx)
}

// DiscoverDevicesByType discovers devices of a specific type
func (m *PluginManager) DiscoverDevicesByType(ctx context.Context, deviceType DeviceType) ([]Device, error) {
	return m.registry.DiscoverDevicesByType(ctx, deviceType)
}

// CreateDeviceFromMetadata creates a device from metadata using the appropriate plugin
func (m *PluginManager) CreateDeviceFromMetadata(ctx context.Context, metadata DeviceMetadata) (Device, error) {
	return m.registry.CreateDevice(ctx, metadata)
}

// GetSupportedDeviceTypes returns all supported device types
func (m *PluginManager) GetSupportedDeviceTypes() []DeviceType {
	return m.registry.GetSupportedDeviceTypes()
}

// IsDeviceTypeSupported checks if a device type is supported
func (m *PluginManager) IsDeviceTypeSupported(deviceType DeviceType) bool {
	return m.registry.IsDeviceTypeSupported(deviceType)
}

