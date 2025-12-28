package pluginregistry

import (
	"context"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// PluginManager manages device plugins and provides high-level operations.
// It wraps a DevicePluginRegistry and adds convenience methods.
type PluginManager struct {
	registry types.DevicePluginRegistry
	logger   *zap.Logger
}

// NewPluginManager creates a new plugin manager.
// If logger is nil, a no-op logger will be used.
func NewPluginManager(registry types.DevicePluginRegistry, logger *zap.Logger) *PluginManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PluginManager{
		registry: registry,
		logger:   logger,
	}
}

// RegisterPlugin registers a plugin with the manager.
func (m *PluginManager) RegisterPlugin(plugin types.DevicePlugin) error {
	m.logger.Debug("Registering plugin via manager",
		zap.String("device_type", string(plugin.GetDeviceType())))
	return m.registry.RegisterPlugin(plugin)
}

// UnregisterPlugin unregisters a plugin.
func (m *PluginManager) UnregisterPlugin(deviceType types.DeviceType) error {
	m.logger.Debug("Unregistering plugin via manager",
		zap.String("device_type", string(deviceType)))
	return m.registry.UnregisterPlugin(deviceType)
}

// GetPlugin retrieves a plugin.
func (m *PluginManager) GetPlugin(deviceType types.DeviceType) (types.DevicePlugin, error) {
	return m.registry.GetPlugin(deviceType)
}

// DiscoverAllDevices discovers devices from all registered plugins.
func (m *PluginManager) DiscoverAllDevices(ctx context.Context) ([]types.Device, error) {
	m.logger.Debug("Discovering all devices via manager")
	return m.registry.DiscoverDevices(ctx)
}

// DiscoverDevicesByType discovers devices of a specific type.
func (m *PluginManager) DiscoverDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	m.logger.Debug("Discovering devices by type via manager",
		zap.String("device_type", string(deviceType)))
	return m.registry.DiscoverDevicesByType(ctx, deviceType)
}

// CreateDeviceFromMetadata creates a device from metadata using the appropriate plugin.
func (m *PluginManager) CreateDeviceFromMetadata(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	m.logger.Debug("Creating device from metadata via manager",
		zap.String("device_id", metadata.ID),
		zap.String("device_type", string(metadata.Type)))
	return m.registry.CreateDevice(ctx, metadata)
}

// GetSupportedDeviceTypes returns all supported device types.
func (m *PluginManager) GetSupportedDeviceTypes() []types.DeviceType {
	return m.registry.GetSupportedDeviceTypes()
}

// IsDeviceTypeSupported checks if a device type is supported.
func (m *PluginManager) IsDeviceTypeSupported(deviceType types.DeviceType) bool {
	return m.registry.IsDeviceTypeSupported(deviceType)
}

