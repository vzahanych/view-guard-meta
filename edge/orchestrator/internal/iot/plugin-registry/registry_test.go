package pluginregistry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// mockPlugin is a test implementation of DevicePlugin
type mockPlugin struct {
	deviceType     types.DeviceType
	capabilities   []types.DeviceCapability
	discoverError  error
	createError    error
	validateError  error
	discoveredDevs []types.Device
}

func (m *mockPlugin) GetDeviceType() types.DeviceType {
	return m.deviceType
}

func (m *mockPlugin) GetSupportedCapabilities() []types.DeviceCapability {
	return m.capabilities
}

func (m *mockPlugin) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	if m.discoverError != nil {
		return nil, m.discoverError
	}
	return m.discoveredDevs, nil
}

func (m *mockPlugin) CreateDevice(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	return &mockDevice{metadata: metadata}, nil
}

func (m *mockPlugin) ValidateMetadata(metadata types.DeviceMetadata) error {
	return m.validateError
}

// mockDevice is a minimal test implementation of Device
type mockDevice struct {
	metadata types.DeviceMetadata
}

func (m *mockDevice) GetID() string {
	return m.metadata.ID
}

func (m *mockDevice) GetMetadata() types.DeviceMetadata {
	return m.metadata
}

func (m *mockDevice) Start(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Stop(ctx context.Context) error {
	return nil
}

func (m *mockDevice) UpdateMetadata(ctx context.Context, updates *types.DeviceMetadataUpdate) error {
	return nil
}

func (m *mockDevice) Enable(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Disable(ctx context.Context) error {
	return nil
}

func (m *mockDevice) IsEnabled() bool {
	return true
}

func (m *mockDevice) GetStatus() types.DeviceStatus {
	return types.DeviceStatusOnline
}

func (m *mockDevice) GetAvailableCommands(ctx context.Context) ([]types.DeviceCommand, error) {
	return []types.DeviceCommand{}, nil
}

func (m *mockDevice) HasCapability(capability types.DeviceCapability) bool {
	return false
}

func (m *mockDevice) GetCapabilities() types.DeviceCapabilities {
	return types.DeviceCapabilities{}
}

func (m *mockDevice) CaptureData(ctx context.Context) (*types.DeviceData, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDevice) StartDataStream(ctx context.Context) (<-chan *types.DeviceData, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDevice) StopDataStream(ctx context.Context) error {
	return errors.New("not implemented")
}

func (m *mockDevice) ReadSensor(ctx context.Context, sensorType string) (*types.SensorReading, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDevice) ReadAllSensors(ctx context.Context) (map[string]*types.SensorReading, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDevice) ExecuteCommand(ctx context.Context, command types.DeviceCommand) error {
	return errors.New("not implemented")
}

func TestNewDevicePluginRegistry(t *testing.T) {
	t.Run("creates registry with logger", func(t *testing.T) {
		logger := zap.NewNop()
		registry := pluginregistry.NewDevicePluginRegistry(logger)

		assert.NotNil(t, registry)
	})

	t.Run("creates registry with nil logger", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(nil)

		assert.NotNil(t, registry)
	})
}

func TestDevicePluginRegistry_RegisterPlugin(t *testing.T) {
	t.Run("registers valid plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		// Verify plugin is registered
		registered, err := registry.GetPlugin(types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Equal(t, plugin, registered)
	})

	t.Run("rejects nil plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		err := registry.RegisterPlugin(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin cannot be nil")
	})

	t.Run("rejects plugin with unknown device type", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeUnknown,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device type cannot be unknown")
	})

	t.Run("rejects plugin with no capabilities", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{},
		}

		err := registry.RegisterPlugin(plugin)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must support at least one capability")
	})

	t.Run("rejects duplicate plugin registration", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin1 := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}
		plugin2 := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin1)
		require.NoError(t, err)

		err = registry.RegisterPlugin(plugin2)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrPluginExists)
	})

	t.Run("registers multiple different device types", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		cameraPlugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}
		sensorPlugin := &mockPlugin{
			deviceType:   types.DeviceTypeGenericSensor,
			capabilities: []types.DeviceCapability{types.DeviceCapabilitySensorReadings},
		}

		err := registry.RegisterPlugin(cameraPlugin)
		require.NoError(t, err)

		err = registry.RegisterPlugin(sensorPlugin)
		require.NoError(t, err)

		// Verify both are registered
		camera, err := registry.GetPlugin(types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Equal(t, cameraPlugin, camera)

		sensor, err := registry.GetPlugin(types.DeviceTypeGenericSensor)
		require.NoError(t, err)
		assert.Equal(t, sensorPlugin, sensor)
	})
}

func TestDevicePluginRegistry_UnregisterPlugin(t *testing.T) {
	t.Run("unregisters existing plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		err = registry.UnregisterPlugin(types.DeviceTypeCamera)
		require.NoError(t, err)

		// Verify plugin is no longer registered
		_, err = registry.GetPlugin(types.DeviceTypeCamera)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrPluginNotFound)
	})

	t.Run("returns error for non-existent plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		err := registry.UnregisterPlugin(types.DeviceTypeCamera)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrPluginNotFound)
	})
}

func TestDevicePluginRegistry_GetPlugin(t *testing.T) {
	t.Run("retrieves registered plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		retrieved, err := registry.GetPlugin(types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Equal(t, plugin, retrieved)
	})

	t.Run("returns error for non-existent plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		_, err := registry.GetPlugin(types.DeviceTypeCamera)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrPluginNotFound)
	})
}

func TestDevicePluginRegistry_ListPlugins(t *testing.T) {
	t.Run("returns empty list when no plugins registered", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		plugins := registry.ListPlugins()
		assert.Empty(t, plugins)
	})

	t.Run("returns all registered plugins", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		cameraPlugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}
		sensorPlugin := &mockPlugin{
			deviceType:   types.DeviceTypeGenericSensor,
			capabilities: []types.DeviceCapability{types.DeviceCapabilitySensorReadings},
		}

		err := registry.RegisterPlugin(cameraPlugin)
		require.NoError(t, err)

		err = registry.RegisterPlugin(sensorPlugin)
		require.NoError(t, err)

		plugins := registry.ListPlugins()
		assert.Len(t, plugins, 2)
		assert.Contains(t, plugins, cameraPlugin)
		assert.Contains(t, plugins, sensorPlugin)
	})
}

func TestDevicePluginRegistry_DiscoverDevices(t *testing.T) {
	t.Run("discovers devices from all plugins", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		device1 := &mockDevice{metadata: types.DeviceMetadata{ID: "device1", Type: types.DeviceTypeCamera}}
		device2 := &mockDevice{metadata: types.DeviceMetadata{ID: "device2", Type: types.DeviceTypeGenericSensor}}

		cameraPlugin := &mockPlugin{
			deviceType:     types.DeviceTypeCamera,
			capabilities:   []types.DeviceCapability{types.DeviceCapabilityDataCapture},
			discoveredDevs: []types.Device{device1},
		}
		sensorPlugin := &mockPlugin{
			deviceType:     types.DeviceTypeGenericSensor,
			capabilities:   []types.DeviceCapability{types.DeviceCapabilitySensorReadings},
			discoveredDevs: []types.Device{device2},
		}

		err := registry.RegisterPlugin(cameraPlugin)
		require.NoError(t, err)

		err = registry.RegisterPlugin(sensorPlugin)
		require.NoError(t, err)

		ctx := context.Background()
		devices, err := registry.DiscoverDevices(ctx)
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})

	t.Run("handles plugin discovery errors gracefully", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		device1 := &mockDevice{metadata: types.DeviceMetadata{ID: "device1", Type: types.DeviceTypeCamera}}

		cameraPlugin := &mockPlugin{
			deviceType:     types.DeviceTypeCamera,
			capabilities:   []types.DeviceCapability{types.DeviceCapabilityDataCapture},
			discoveredDevs: []types.Device{device1},
		}
		errorPlugin := &mockPlugin{
			deviceType:    types.DeviceTypeGenericSensor,
			capabilities:  []types.DeviceCapability{types.DeviceCapabilitySensorReadings},
			discoverError: errors.New("discovery failed"),
		}

		err := registry.RegisterPlugin(cameraPlugin)
		require.NoError(t, err)

		err = registry.RegisterPlugin(errorPlugin)
		require.NoError(t, err)

		ctx := context.Background()
		devices, err := registry.DiscoverDevices(ctx)
		// Should not return error, but should return devices from successful plugins
		require.NoError(t, err)
		assert.Len(t, devices, 1)
	})

	t.Run("returns empty list when no plugins registered", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		ctx := context.Background()
		devices, err := registry.DiscoverDevices(ctx)
		require.NoError(t, err)
		assert.Empty(t, devices)
	})
}

func TestDevicePluginRegistry_DiscoverDevicesByType(t *testing.T) {
	t.Run("discovers devices for specific type", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		device1 := &mockDevice{metadata: types.DeviceMetadata{ID: "device1", Type: types.DeviceTypeCamera}}
		device2 := &mockDevice{metadata: types.DeviceMetadata{ID: "device2", Type: types.DeviceTypeCamera}}

		cameraPlugin := &mockPlugin{
			deviceType:     types.DeviceTypeCamera,
			capabilities:   []types.DeviceCapability{types.DeviceCapabilityDataCapture},
			discoveredDevs: []types.Device{device1, device2},
		}

		err := registry.RegisterPlugin(cameraPlugin)
		require.NoError(t, err)

		ctx := context.Background()
		devices, err := registry.DiscoverDevicesByType(ctx, types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})

	t.Run("returns error for non-existent plugin", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		ctx := context.Background()
		_, err := registry.DiscoverDevicesByType(ctx, types.DeviceTypeCamera)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrPluginNotFound)
	})
}

func TestDevicePluginRegistry_CreateDevice(t *testing.T) {
	t.Run("creates device from valid metadata", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeCamera,
		}

		ctx := context.Background()
		device, err := registry.CreateDevice(ctx, metadata)
		require.NoError(t, err)
		assert.NotNil(t, device)
		assert.Equal(t, "test-device", device.GetID())
	})

	t.Run("returns error for unknown device type", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeUnknown,
		}

		ctx := context.Background()
		_, err := registry.CreateDevice(ctx, metadata)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device type cannot be unknown")
	})

	t.Run("returns error when no plugin registered", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeCamera,
		}

		ctx := context.Background()
		_, err := registry.CreateDevice(ctx, metadata)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrNoPluginForType)
	})

	t.Run("returns error when metadata validation fails", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:    types.DeviceTypeCamera,
			capabilities:  []types.DeviceCapability{types.DeviceCapabilityDataCapture},
			validateError: errors.New("invalid metadata"),
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeCamera,
		}

		ctx := context.Background()
		_, err = registry.CreateDevice(ctx, metadata)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "metadata validation failed")
	})
}

func TestDevicePluginRegistry_ValidateMetadata(t *testing.T) {
	t.Run("validates metadata successfully", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeCamera,
		}

		err = registry.ValidateMetadata(metadata)
		require.NoError(t, err)
	})

	t.Run("returns error for unknown device type", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeUnknown,
		}

		err := registry.ValidateMetadata(metadata)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device type cannot be unknown")
	})

	t.Run("returns error when no plugin registered", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "test-device",
			Type: types.DeviceTypeCamera,
		}

		err := registry.ValidateMetadata(metadata)
		require.Error(t, err)
		assert.ErrorIs(t, err, types.ErrNoPluginForType)
	})
}

func TestDevicePluginRegistry_GetSupportedDeviceTypes(t *testing.T) {
	t.Run("returns empty list when no plugins registered", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		types := registry.GetSupportedDeviceTypes()
		assert.Empty(t, types)
	})

	t.Run("returns all supported device types", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		cameraPlugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}
		sensorPlugin := &mockPlugin{
			deviceType:   types.DeviceTypeGenericSensor,
			capabilities: []types.DeviceCapability{types.DeviceCapabilitySensorReadings},
		}

		err := registry.RegisterPlugin(cameraPlugin)
		require.NoError(t, err)

		err = registry.RegisterPlugin(sensorPlugin)
		require.NoError(t, err)

		deviceTypes := registry.GetSupportedDeviceTypes()
		assert.Len(t, deviceTypes, 2)
		assert.Contains(t, deviceTypes, types.DeviceTypeCamera)
		assert.Contains(t, deviceTypes, types.DeviceTypeGenericSensor)
	})
}

func TestDevicePluginRegistry_IsDeviceTypeSupported(t *testing.T) {
	t.Run("returns false for unsupported type", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())

		supported := registry.IsDeviceTypeSupported(types.DeviceTypeCamera)
		assert.False(t, supported)
	})

	t.Run("returns true for supported type", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		supported := registry.IsDeviceTypeSupported(types.DeviceTypeCamera)
		assert.True(t, supported)
	})
}

func TestDevicePluginRegistry_ConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent registration", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		const numGoroutines = 10

		errors := make(chan error, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				deviceType := types.DeviceTypeCamera
				if id%2 == 1 {
					deviceType = types.DeviceTypeGenericSensor
				}
				plugin := &mockPlugin{
					deviceType:   deviceType,
					capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
				}
				errors <- registry.RegisterPlugin(plugin)
			}(i)
		}

		// Collect errors
		for i := 0; i < numGoroutines; i++ {
			err := <-errors
			// Some may succeed, some may fail (duplicate registration)
			// But no panic should occur
			_ = err
		}
	})

	t.Run("handles concurrent read and write", func(t *testing.T) {
		registry := pluginregistry.NewDevicePluginRegistry(zap.NewNop())
		plugin := &mockPlugin{
			deviceType:   types.DeviceTypeCamera,
			capabilities: []types.DeviceCapability{types.DeviceCapabilityDataCapture},
		}

		err := registry.RegisterPlugin(plugin)
		require.NoError(t, err)

		// Concurrent reads and writes
		done := make(chan bool)
		go func() {
			for i := 0; i < 100; i++ {
				_, _ = registry.GetPlugin(types.DeviceTypeCamera)
				_ = registry.IsDeviceTypeSupported(types.DeviceTypeCamera)
			}
			done <- true
		}()

		go func() {
			for i := 0; i < 10; i++ {
				_ = registry.ListPlugins()
				_ = registry.GetSupportedDeviceTypes()
			}
			done <- true
		}()

		<-done
		<-done
		// No panic should occur
	})
}

