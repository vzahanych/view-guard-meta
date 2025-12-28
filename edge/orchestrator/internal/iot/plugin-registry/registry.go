package pluginregistry

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// devicePluginRegistryImpl is the default implementation of DevicePluginRegistry.
// It provides thread-safe plugin registration, discovery, and device creation.
type devicePluginRegistryImpl struct {
	// plugins maps device type to plugin
	plugins map[types.DeviceType]types.DevicePlugin

	// mu protects the plugins map
	mu sync.RWMutex

	// logger provides structured logging
	logger *zap.Logger
}

// NewDevicePluginRegistry creates a new device plugin registry.
// If logger is nil, a no-op logger will be used.
func NewDevicePluginRegistry(logger *zap.Logger) types.DevicePluginRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &devicePluginRegistryImpl{
		plugins: make(map[types.DeviceType]types.DevicePlugin),
		logger:  logger,
	}
}

// RegisterPlugin registers a device plugin for a specific device type.
// Returns error if plugin is nil, device type is unknown, plugin is already registered, or validation fails.
func (r *devicePluginRegistryImpl) RegisterPlugin(plugin types.DevicePlugin) error {
	if plugin == nil {
		r.logger.Error("Attempted to register nil plugin")
		return fmt.Errorf("plugin cannot be nil")
	}

	deviceType := plugin.GetDeviceType()
	if deviceType == types.DeviceTypeUnknown {
		r.logger.Error("Attempted to register plugin with unknown device type")
		return fmt.Errorf("plugin device type cannot be unknown")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if plugin is already registered
	if existing, exists := r.plugins[deviceType]; exists {
		r.logger.Warn("Plugin already registered for device type",
			zap.String("device_type", string(deviceType)),
			zap.String("existing_plugin_type", fmt.Sprintf("%T", existing)))
		return fmt.Errorf("plugin for device type %s is already registered: %w", deviceType, types.ErrPluginExists)
	}

	// Validate plugin
	if err := r.validatePlugin(plugin); err != nil {
		r.logger.Error("Plugin validation failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err))
		return fmt.Errorf("plugin validation failed: %w", err)
	}

	// Register plugin
	r.plugins[deviceType] = plugin
	r.logger.Info("Plugin registered successfully",
		zap.String("device_type", string(deviceType)),
		zap.Strings("capabilities", capabilitiesToStrings(plugin.GetSupportedCapabilities())))

	return nil
}

// UnregisterPlugin unregisters a device plugin.
// Returns error if plugin is not registered.
func (r *devicePluginRegistryImpl) UnregisterPlugin(deviceType types.DeviceType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[deviceType]; !exists {
		r.logger.Warn("Attempted to unregister non-existent plugin",
			zap.String("device_type", string(deviceType)))
		return fmt.Errorf("plugin for device type %s is not registered: %w", deviceType, types.ErrPluginNotFound)
	}

	delete(r.plugins, deviceType)
	r.logger.Info("Plugin unregistered successfully",
		zap.String("device_type", string(deviceType)))

	return nil
}

// GetPlugin retrieves a plugin for a specific device type.
// Returns error if plugin is not found.
func (r *devicePluginRegistryImpl) GetPlugin(deviceType types.DeviceType) (types.DevicePlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.plugins[deviceType]
	if !exists {
		r.logger.Debug("Plugin not found for device type",
			zap.String("device_type", string(deviceType)))
		return nil, fmt.Errorf("plugin for device type %s is not registered: %w", deviceType, types.ErrPluginNotFound)
	}

	return plugin, nil
}

// ListPlugins returns all registered plugins.
// Locking strategy: Copy plugins map under lock, return slice outside lock.
func (r *devicePluginRegistryImpl) ListPlugins() []types.DevicePlugin {
	r.mu.RLock()
	plugins := make([]types.DevicePlugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	r.mu.RUnlock()

	return plugins
}

// GetPluginForDeviceType returns the plugin that handles a specific device type.
// This is an alias for GetPlugin for clarity.
func (r *devicePluginRegistryImpl) GetPluginForDeviceType(deviceType types.DeviceType) (types.DevicePlugin, error) {
	return r.GetPlugin(deviceType)
}

// DiscoverDevices discovers devices using all registered plugins.
// Locking strategy: Copy plugins slice under lock, call plugin methods outside lock.
func (r *devicePluginRegistryImpl) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	r.mu.RLock()
	plugins := make([]types.DevicePlugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	r.mu.RUnlock()

	r.logger.Info("Starting device discovery using all registered plugins",
		zap.Int("plugin_count", len(plugins)))

	allDevices := make([]types.Device, 0)
	var discoveryErrors []error

	// Discover devices from all plugins (call outside lock to avoid deadlocks)
	for _, plugin := range plugins {
		deviceType := plugin.GetDeviceType()
		devices, err := plugin.DiscoverDevices(ctx)
		if err != nil {
			r.logger.Warn("Plugin discovery failed",
				zap.String("device_type", string(deviceType)),
				zap.Error(err))
			discoveryErrors = append(discoveryErrors, fmt.Errorf("plugin %s discovery failed: %w", deviceType, err))
			continue
		}
		allDevices = append(allDevices, devices...)
		r.logger.Debug("Plugin discovery completed",
			zap.String("device_type", string(deviceType)),
			zap.Int("device_count", len(devices)))
	}

	r.logger.Info("Device discovery completed",
		zap.Int("total_devices", len(allDevices)),
		zap.Int("plugin_count", len(plugins)),
		zap.Int("error_count", len(discoveryErrors)))

	// Return devices even if some plugins failed (non-fatal errors)
	return allDevices, nil
}

// DiscoverDevicesByType discovers devices of a specific type using the appropriate plugin.
func (r *devicePluginRegistryImpl) DiscoverDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	r.logger.Debug("Starting device discovery by type",
		zap.String("device_type", string(deviceType)))

	plugin, err := r.GetPlugin(deviceType)
	if err != nil {
		return nil, fmt.Errorf("discover devices by type %s: %w", deviceType, err)
	}

	devices, err := plugin.DiscoverDevices(ctx)
	if err != nil {
		r.logger.Error("Plugin discovery failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err))
		return nil, fmt.Errorf("plugin discovery failed: %w", err)
	}

	r.logger.Debug("Device discovery by type completed",
		zap.String("device_type", string(deviceType)),
		zap.Int("device_count", len(devices)))

	return devices, nil
}

// CreateDevice creates a device instance from metadata using the appropriate plugin.
func (r *devicePluginRegistryImpl) CreateDevice(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	if metadata.Type == types.DeviceTypeUnknown {
		r.logger.Error("Attempted to create device with unknown type")
		return nil, fmt.Errorf("device type cannot be unknown")
	}

	r.logger.Debug("Creating device from metadata",
		zap.String("device_id", metadata.ID),
		zap.String("device_type", string(metadata.Type)))

	plugin, err := r.GetPlugin(metadata.Type)
	if err != nil {
		return nil, fmt.Errorf("no plugin registered for device type %s: %w", metadata.Type, types.ErrNoPluginForType)
	}

	// Validate metadata
	if err := plugin.ValidateMetadata(metadata); err != nil {
		r.logger.Error("Metadata validation failed",
			zap.String("device_id", metadata.ID),
			zap.String("device_type", string(metadata.Type)),
			zap.Error(err))
		return nil, fmt.Errorf("metadata validation failed: %w", err)
	}

	// Create device
	device, err := plugin.CreateDevice(ctx, metadata)
	if err != nil {
		r.logger.Error("Device creation failed",
			zap.String("device_id", metadata.ID),
			zap.String("device_type", string(metadata.Type)),
			zap.Error(err))
		return nil, fmt.Errorf("device creation failed: %w", err)
	}

	r.logger.Info("Device created successfully",
		zap.String("device_id", metadata.ID),
		zap.String("device_type", string(metadata.Type)))

	return device, nil
}

// ValidateMetadata validates device metadata using the appropriate plugin.
func (r *devicePluginRegistryImpl) ValidateMetadata(metadata types.DeviceMetadata) error {
	if metadata.Type == types.DeviceTypeUnknown {
		r.logger.Error("Attempted to validate metadata with unknown device type")
		return fmt.Errorf("device type cannot be unknown")
	}

	r.logger.Debug("Validating device metadata",
		zap.String("device_id", metadata.ID),
		zap.String("device_type", string(metadata.Type)))

	plugin, err := r.GetPlugin(metadata.Type)
	if err != nil {
		return fmt.Errorf("no plugin registered for device type %s: %w", metadata.Type, types.ErrNoPluginForType)
	}

	err = plugin.ValidateMetadata(metadata)
	if err != nil {
		r.logger.Warn("Metadata validation failed",
			zap.String("device_id", metadata.ID),
			zap.String("device_type", string(metadata.Type)),
			zap.Error(err))
		return fmt.Errorf("metadata validation failed: %w", err)
	}

	r.logger.Debug("Metadata validation successful",
		zap.String("device_id", metadata.ID),
		zap.String("device_type", string(metadata.Type)))

	return nil
}

// GetSupportedDeviceTypes returns all device types that have registered plugins.
// Locking strategy: Copy device types under lock, return slice outside lock.
func (r *devicePluginRegistryImpl) GetSupportedDeviceTypes() []types.DeviceType {
	r.mu.RLock()
	deviceTypes := make([]types.DeviceType, 0, len(r.plugins))
	for deviceType := range r.plugins {
		deviceTypes = append(deviceTypes, deviceType)
	}
	r.mu.RUnlock()

	return deviceTypes
}

// IsDeviceTypeSupported checks if a device type has a registered plugin.
func (r *devicePluginRegistryImpl) IsDeviceTypeSupported(deviceType types.DeviceType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.plugins[deviceType]
	return exists
}

// validatePlugin validates a plugin before registration.
// This is a private helper method.
func (r *devicePluginRegistryImpl) validatePlugin(plugin types.DevicePlugin) error {
	// Check device type
	deviceType := plugin.GetDeviceType()
	if deviceType == types.DeviceTypeUnknown {
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

// capabilitiesToStrings converts a slice of DeviceCapability to a slice of strings.
// This is a helper function for logging.
func capabilitiesToStrings(capabilities []types.DeviceCapability) []string {
	result := make([]string, len(capabilities))
	for i, cap := range capabilities {
		result[i] = string(cap)
	}
	return result
}

// PluginDiscoveryError represents an error during plugin discovery.
// This is implementation-specific and stays in the plugin-registry package.
type PluginDiscoveryError struct {
	// PluginIdentifier is the identifier of the plugin that failed
	PluginIdentifier string `json:"plugin_identifier"`

	// Error is the error that occurred
	Error error `json:"error"`
}

// DiscoverPlugins discovers and registers plugins.
// This is a placeholder for future file-based plugin discovery.
// Currently, plugins must be registered manually via RegisterPlugin.
func DiscoverPlugins(ctx context.Context, registry types.DevicePluginRegistry, config *types.PluginDiscoveryConfig) (*types.PluginDiscoveryResult, error) {
	if registry == nil {
		return nil, fmt.Errorf("plugin registry cannot be nil")
	}

	// For now, plugin discovery is manual (plugins must be registered via RegisterPlugin)
	// Future implementation could:
	// 1. Scan plugin paths for .so files (Go plugins)
	// 2. Load plugins dynamically
	// 3. Validate and register plugins

	// Return currently registered plugins
	deviceTypes := registry.GetSupportedDeviceTypes()

	// Build discovery results for each device type
	results := make([]types.PluginDiscoveryResult, 0, len(deviceTypes))
	for _, deviceType := range deviceTypes {
		plugin, _ := registry.GetPlugin(deviceType)
		devices, err := plugin.DiscoverDevices(ctx)
		results = append(results, types.PluginDiscoveryResult{
			DeviceType: deviceType,
			Devices:    devices,
			Error:      err,
		})
	}

	// Note: types.PluginDiscoveryResult is per-device-type, not a collection
	// This function returns the first result for now (placeholder behavior)
	if len(results) > 0 {
		return &results[0], nil
	}

	return &types.PluginDiscoveryResult{
		DeviceType: types.DeviceTypeUnknown,
		Devices:     []types.Device{},
	}, nil
}

