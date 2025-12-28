package deviceregistry_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// mockDevicePluginRegistry is a test implementation of DevicePluginRegistry
type mockDevicePluginRegistry struct {
	discoverDevicesFunc      func(ctx context.Context) ([]types.Device, error)
	discoverDevicesByTypeFunc func(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error)
}

func (m *mockDevicePluginRegistry) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	if m.discoverDevicesFunc != nil {
		return m.discoverDevicesFunc(ctx)
	}
	return []types.Device{}, nil
}

func (m *mockDevicePluginRegistry) DiscoverDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	if m.discoverDevicesByTypeFunc != nil {
		return m.discoverDevicesByTypeFunc(ctx, deviceType)
	}
	return []types.Device{}, nil
}

func (m *mockDevicePluginRegistry) RegisterPlugin(plugin types.DevicePlugin) error {
	return nil
}

func (m *mockDevicePluginRegistry) UnregisterPlugin(deviceType types.DeviceType) error {
	return nil
}

func (m *mockDevicePluginRegistry) GetPlugin(deviceType types.DeviceType) (types.DevicePlugin, error) {
	return nil, nil
}

func (m *mockDevicePluginRegistry) ListPlugins() []types.DevicePlugin {
	return nil
}

func (m *mockDevicePluginRegistry) GetPluginForDeviceType(deviceType types.DeviceType) (types.DevicePlugin, error) {
	return nil, nil
}

func (m *mockDevicePluginRegistry) GetSupportedDeviceTypes() []types.DeviceType {
	return nil
}

func (m *mockDevicePluginRegistry) CreateDevice(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	return nil, nil
}

func (m *mockDevicePluginRegistry) ValidateMetadata(metadata types.DeviceMetadata) error {
	return nil
}

func (m *mockDevicePluginRegistry) IsDeviceTypeSupported(deviceType types.DeviceType) bool {
	return false
}

// mockDeviceStateMachineRegistry is a test implementation of DeviceStateMachineRegistry
type mockDeviceStateMachineRegistry struct {
	getOrCreateFunc func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error)
	getFunc         func(deviceID string) (types.DeviceStateMachine, error)
	createFunc      func(deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error)
	removeFunc      func(deviceID string) error
}

func (m *mockDeviceStateMachineRegistry) GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
	if m.getOrCreateFunc != nil {
		return m.getOrCreateFunc(ctx, deviceID, deviceType)
	}
	return nil, nil
}

func (m *mockDeviceStateMachineRegistry) GetStateMachine(deviceID string) (types.DeviceStateMachine, error) {
	if m.getFunc != nil {
		return m.getFunc(deviceID)
	}
	return nil, nil
}

func (m *mockDeviceStateMachineRegistry) CreateStateMachine(deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
	if m.createFunc != nil {
		return m.createFunc(deviceID, deviceType)
	}
	return nil, nil
}

func (m *mockDeviceStateMachineRegistry) RemoveStateMachine(deviceID string) error {
	if m.removeFunc != nil {
		return m.removeFunc(deviceID)
	}
	return nil
}

func (m *mockDeviceStateMachineRegistry) GetAllStateMachines() []types.DeviceStateMachine {
	return nil
}

func (m *mockDeviceStateMachineRegistry) GetStateMachinesByType(deviceType types.DeviceType) []types.DeviceStateMachine {
	return nil
}

// mockLifecycleHookRegistry is a test implementation of LifecycleHookRegistry
type mockLifecycleHookRegistry struct {
	discoveryHookFunc      func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error
	registrationHookFunc   func(ctx context.Context, hookCtx *types.RegistrationHookContext) error
	teardownHookFunc       func(ctx context.Context, hookCtx *types.TeardownHookContext) error
	dataCollectionHookFunc func(ctx context.Context, hookCtx *types.DataCollectionHookContext) error
}

func (m *mockLifecycleHookRegistry) RegisterHook(hook *types.LifecycleHook) error {
	return nil
}

func (m *mockLifecycleHookRegistry) UnregisterHook(hookID string) error {
	return nil
}

func (m *mockLifecycleHookRegistry) GetHook(hookID string) (*types.LifecycleHook, error) {
	return nil, nil
}

func (m *mockLifecycleHookRegistry) ListHooks(hookType *types.LifecycleHookType) []*types.LifecycleHook {
	return nil
}

func (m *mockLifecycleHookRegistry) ExecuteDiscoveryHooks(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
	if m.discoveryHookFunc != nil {
		return m.discoveryHookFunc(ctx, hookCtx)
	}
	return nil
}

func (m *mockLifecycleHookRegistry) ExecuteRegistrationHooks(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
	if m.registrationHookFunc != nil {
		return m.registrationHookFunc(ctx, hookCtx)
	}
	return nil
}

func (m *mockLifecycleHookRegistry) ExecuteTeardownHooks(ctx context.Context, hookCtx *types.TeardownHookContext) error {
	if m.teardownHookFunc != nil {
		return m.teardownHookFunc(ctx, hookCtx)
	}
	return nil
}

func (m *mockLifecycleHookRegistry) ExecuteDataCollectionHooks(ctx context.Context, hookCtx *types.DataCollectionHookContext) error {
	if m.dataCollectionHookFunc != nil {
		return m.dataCollectionHookFunc(ctx, hookCtx)
	}
	return nil
}

// mockDevice is a test implementation of Device
type mockDevice struct {
	id          string
	deviceType  types.DeviceType
	capabilities types.DeviceCapabilities
	enabled     bool
	status      types.DeviceStatus
	metadata    types.DeviceMetadata
}

func newMockDevice(id string, deviceType types.DeviceType, capabilities types.DeviceCapabilities) *mockDevice {
	return &mockDevice{
		id:          id,
		deviceType:  deviceType,
		capabilities: capabilities,
		enabled:     true,
		status:      types.DeviceStatusOnline,
		metadata: types.DeviceMetadata{
			ID:           id,
			Type:         deviceType,
			Capabilities: capabilities,
			Name:         id,
			Enabled:      true,
		},
	}
}

func (m *mockDevice) GetID() string {
	return m.id
}

func (m *mockDevice) GetMetadata() types.DeviceMetadata {
	return m.metadata
}

func (m *mockDevice) UpdateMetadata(ctx context.Context, updates *types.DeviceMetadataUpdate) error {
	if updates.Name != nil {
		m.metadata.Name = *updates.Name
	}
	if updates.Enabled != nil {
		m.metadata.Enabled = *updates.Enabled
		m.enabled = *updates.Enabled
	}
	return nil
}

func (m *mockDevice) Start(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Stop(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Enable(ctx context.Context) error {
	m.enabled = true
	m.metadata.Enabled = true
	return nil
}

func (m *mockDevice) Disable(ctx context.Context) error {
	m.enabled = false
	m.metadata.Enabled = false
	return nil
}

func (m *mockDevice) IsEnabled() bool {
	return m.enabled
}

func (m *mockDevice) GetStatus() types.DeviceStatus {
	return m.status
}

func (m *mockDevice) HasCapability(capability types.DeviceCapability) bool {
	return m.capabilities.Has(capability)
}

func (m *mockDevice) GetCapabilities() types.DeviceCapabilities {
	return m.capabilities
}

func (m *mockDevice) GetAvailableCommands(ctx context.Context) ([]types.DeviceCommand, error) {
	return []types.DeviceCommand{}, nil
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

// TestNewDeviceRegistry tests creating a new device registry
func TestNewDeviceRegistry(t *testing.T) {
	t.Run("creates registry with all dependencies", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		assert.NotNil(t, registry)
	})

	t.Run("creates registry with nil logger", func(t *testing.T) {
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, nil)

		assert.NotNil(t, registry)
	})
}

// TestDeviceRegistry_DiscoverDevices tests device discovery by type
func TestDeviceRegistry_DiscoverDevices(t *testing.T) {
	t.Run("discovers devices successfully", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})
		device2 := newMockDevice("device-2", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})

		pluginReg := &mockDevicePluginRegistry{
			discoverDevicesByTypeFunc: func(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
				assert.Equal(t, types.DeviceTypeCamera, deviceType)
				return []types.Device{device1, device2}, nil
			},
		}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		devices, err := registry.DiscoverDevices(context.Background(), types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})

	t.Run("handles plugin discovery error", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{
			discoverDevicesByTypeFunc: func(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
				return nil, errors.New("discovery failed")
			},
		}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		devices, err := registry.DiscoverDevices(context.Background(), types.DeviceTypeCamera)
		require.Error(t, err)
		assert.Nil(t, devices)
		assert.Contains(t, err.Error(), "plugin discovery failed")
	})

	t.Run("executes discovery hooks", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		hookExecuted := false
		pluginReg := &mockDevicePluginRegistry{
			discoverDevicesByTypeFunc: func(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
				return []types.Device{device}, nil
			},
		}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{
			discoveryHookFunc: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
				hookExecuted = true
				assert.Equal(t, types.DeviceTypeCamera, hookCtx.DeviceType)
				assert.Len(t, hookCtx.DiscoveredDevices, 1)
				return nil
			},
		}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		_, err := registry.DiscoverDevices(context.Background(), types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.True(t, hookExecuted)
	})
}

// TestDeviceRegistry_DiscoverAllDevices tests discovering all devices
func TestDeviceRegistry_DiscoverAllDevices(t *testing.T) {
	t.Run("discovers all devices successfully", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})
		device2 := newMockDevice("device-2", types.DeviceTypeGenericSensor, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{
			discoverDevicesFunc: func(ctx context.Context) ([]types.Device, error) {
				return []types.Device{device1, device2}, nil
			},
		}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		devices, err := registry.DiscoverAllDevices(context.Background())
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})

	t.Run("handles plugin discovery error", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{
			discoverDevicesFunc: func(ctx context.Context) ([]types.Device, error) {
				return nil, errors.New("discovery failed")
			},
		}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		devices, err := registry.DiscoverAllDevices(context.Background())
		require.Error(t, err)
		assert.Nil(t, devices)
	})
}

// TestDeviceRegistry_RegisterDevice tests device registration
func TestDeviceRegistry_RegisterDevice(t *testing.T) {
	t.Run("registers device successfully", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})

		stateMachineCreated := false
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				stateMachineCreated = true
				assert.Equal(t, "device-1", deviceID)
				assert.Equal(t, types.DeviceTypeCamera, deviceType)
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{
			registrationHookFunc: func(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
				assert.Equal(t, device, hookCtx.Device)
				return nil
			},
		}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)
		assert.True(t, stateMachineCreated)

		// Verify device is registered
		registered, err := registry.GetDevice(context.Background(), "device-1")
		require.NoError(t, err)
		assert.Equal(t, device, registered)
	})

	t.Run("rejects nil device", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device cannot be nil")
	})

	t.Run("rejects device with empty ID", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device ID cannot be empty")
	})

	t.Run("rejects device with unknown type", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeUnknown, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device type cannot be unknown")
	})

	t.Run("rejects duplicate registration", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		// Try to register again
		err = registry.RegisterDevice(context.Background(), device)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("handles state machine creation error", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, errors.New("state machine creation failed")
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create state machine")
	})

	t.Run("updates indexes on registration", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
			types.DeviceCapabilitySnapshot:     true,
		})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		// Verify device is in type index
		devicesByType, err := registry.GetDevicesByType(context.Background(), types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Len(t, devicesByType, 1)

		// Verify device is in capability indexes
		devicesByCap, err := registry.GetDevicesByCapability(context.Background(), types.DeviceCapabilityVideoCapture)
		require.NoError(t, err)
		assert.Len(t, devicesByCap, 1)

		devicesByCap2, err := registry.GetDevicesByCapability(context.Background(), types.DeviceCapabilitySnapshot)
		require.NoError(t, err)
		assert.Len(t, devicesByCap2, 1)
	})
}

// TestDeviceRegistry_GetDevice tests device retrieval
func TestDeviceRegistry_GetDevice(t *testing.T) {
	t.Run("retrieves registered device", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		retrieved, err := registry.GetDevice(context.Background(), "device-1")
		require.NoError(t, err)
		assert.Equal(t, device, retrieved)
	})

	t.Run("returns error for non-existent device", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		device, err := registry.GetDevice(context.Background(), "non-existent")
		require.Error(t, err)
		assert.Nil(t, device)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("rejects empty device ID", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		device, err := registry.GetDevice(context.Background(), "")
		require.Error(t, err)
		assert.Nil(t, device)
		assert.Contains(t, err.Error(), "device ID cannot be empty")
	})
}

// TestDeviceRegistry_ListDevices tests device listing
func TestDeviceRegistry_ListDevices(t *testing.T) {
	t.Run("lists all devices when filters are nil", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})
		device2 := newMockDevice("device-2", types.DeviceTypeGenericSensor, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device1)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device2)
		require.NoError(t, err)

		devices, err := registry.ListDevices(context.Background(), nil)
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})

	t.Run("filters devices by type", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})
		device2 := newMockDevice("device-2", types.DeviceTypeGenericSensor, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device1)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device2)
		require.NoError(t, err)

		filters := &types.DeviceFilters{
			Type: func() *types.DeviceType { t := types.DeviceTypeCamera; return &t }(),
		}
		devices, err := registry.ListDevices(context.Background(), filters)
		require.NoError(t, err)
		assert.Len(t, devices, 1)
		assert.Equal(t, "device-1", devices[0].GetID())
	})

	t.Run("filters devices by capability", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})
		device2 := newMockDevice("device-2", types.DeviceTypeGenericSensor, types.DeviceCapabilities{
			types.DeviceCapabilitySensorReadings: true,
		})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device1)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device2)
		require.NoError(t, err)

		filters := &types.DeviceFilters{
			Capability: func() *types.DeviceCapability { c := types.DeviceCapabilityVideoCapture; return &c }(),
		}
		devices, err := registry.ListDevices(context.Background(), filters)
		require.NoError(t, err)
		assert.Len(t, devices, 1)
		assert.Equal(t, "device-1", devices[0].GetID())
	})
}

// TestDeviceRegistry_GetDevicesByType tests getting devices by type
func TestDeviceRegistry_GetDevicesByType(t *testing.T) {
	t.Run("returns devices of specific type", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})
		device2 := newMockDevice("device-2", types.DeviceTypeCamera, types.DeviceCapabilities{})
		device3 := newMockDevice("device-3", types.DeviceTypeGenericSensor, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device1)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device2)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device3)
		require.NoError(t, err)

		devices, err := registry.GetDevicesByType(context.Background(), types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})
}

// TestDeviceRegistry_GetDevicesByCapability tests getting devices by capability
func TestDeviceRegistry_GetDevicesByCapability(t *testing.T) {
	t.Run("returns devices with specific capability", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device1 := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})
		device2 := newMockDevice("device-2", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})
		device3 := newMockDevice("device-3", types.DeviceTypeGenericSensor, types.DeviceCapabilities{
			types.DeviceCapabilitySensorReadings: true,
		})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device1)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device2)
		require.NoError(t, err)
		err = registry.RegisterDevice(context.Background(), device3)
		require.NoError(t, err)

		devices, err := registry.GetDevicesByCapability(context.Background(), types.DeviceCapabilityVideoCapture)
		require.NoError(t, err)
		assert.Len(t, devices, 2)
	})
}

// TestDeviceRegistry_UpdateDevice tests device metadata updates
func TestDeviceRegistry_UpdateDevice(t *testing.T) {
	t.Run("updates device metadata successfully", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		newName := "Updated Device"
		updates := &types.DeviceMetadataUpdate{
			Name: &newName,
		}

		err = registry.UpdateDevice(context.Background(), "device-1", updates)
		require.NoError(t, err)

		// Verify update
		updated, err := registry.GetDevice(context.Background(), "device-1")
		require.NoError(t, err)
		assert.Equal(t, newName, updated.GetMetadata().Name)
	})

	t.Run("returns error for non-existent device", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		updates := &types.DeviceMetadataUpdate{}
		err := registry.UpdateDevice(context.Background(), "non-existent", updates)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("rejects nil updates", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		err = registry.UpdateDevice(context.Background(), "device-1", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "updates cannot be nil")
	})
}

// TestDeviceRegistry_DeleteDevice tests device deletion
func TestDeviceRegistry_DeleteDevice(t *testing.T) {
	t.Run("deletes device successfully", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})

		stateMachineRemoved := false
		teardownHookExecuted := false

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
			removeFunc: func(deviceID string) error {
				stateMachineRemoved = true
				assert.Equal(t, "device-1", deviceID)
				return nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{
			teardownHookFunc: func(ctx context.Context, hookCtx *types.TeardownHookContext) error {
				teardownHookExecuted = true
				assert.Equal(t, device, hookCtx.Device)
				return nil
			},
		}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		err = registry.DeleteDevice(context.Background(), "device-1")
		require.NoError(t, err)
		assert.True(t, stateMachineRemoved)
		assert.True(t, teardownHookExecuted)

		// Verify device is deleted
		_, err = registry.GetDevice(context.Background(), "device-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns error for non-existent device", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.DeleteDevice(context.Background(), "non-existent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("cleans up indexes on deletion", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{
			types.DeviceCapabilityVideoCapture: true,
		})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
			removeFunc: func(deviceID string) error {
				return nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		// Verify device is in indexes
		devicesByType, err := registry.GetDevicesByType(context.Background(), types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Len(t, devicesByType, 1)

		err = registry.DeleteDevice(context.Background(), "device-1")
		require.NoError(t, err)

		// Verify device is removed from indexes
		devicesByType, err = registry.GetDevicesByType(context.Background(), types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.Len(t, devicesByType, 0)

		devicesByCap, err := registry.GetDevicesByCapability(context.Background(), types.DeviceCapabilityVideoCapture)
		require.NoError(t, err)
		assert.Len(t, devicesByCap, 0)
	})
}

// TestDeviceRegistry_ConcurrentAccess tests concurrent access safety
func TestDeviceRegistry_ConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent registrations", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				device := newMockDevice(
					"device-"+string(rune('0'+id)),
					types.DeviceTypeCamera,
					types.DeviceCapabilities{},
				)
				_ = registry.RegisterDevice(context.Background(), device)
			}(i)
		}

		wg.Wait()

		devices, err := registry.ListDevices(context.Background(), nil)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(devices), numGoroutines) // May have duplicates due to same ID
	})

	t.Run("handles concurrent reads and writes", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		device := newMockDevice("device-1", types.DeviceTypeCamera, types.DeviceCapabilities{})

		pluginReg := &mockDevicePluginRegistry{}
		stateReg := &mockDeviceStateMachineRegistry{
			getOrCreateFunc: func(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
				return nil, nil
			},
		}
		hookReg := &mockLifecycleHookRegistry{}

		registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)

		err := registry.RegisterDevice(context.Background(), device)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numReaders := 10
		numWriters := 5

		// Concurrent reads
		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = registry.GetDevice(context.Background(), "device-1")
				_, _ = registry.ListDevices(context.Background(), nil)
			}()
		}

		// Concurrent writes (updates)
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				newName := "Updated"
				updates := &types.DeviceMetadataUpdate{Name: &newName}
				_ = registry.UpdateDevice(context.Background(), "device-1", updates)
			}()
		}

		wg.Wait()

		// Verify device still exists
		retrieved, err := registry.GetDevice(context.Background(), "device-1")
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
	})
}

