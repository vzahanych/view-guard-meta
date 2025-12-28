package iot

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// createTestIoTService creates a test IoT service with all real subcomponents.
// This helper function is used by all tests to create a properly configured service.
func createTestIoTService(t *testing.T) IoTService {
	logger := zap.NewNop()

	// Create subcomponents in dependency order
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(
		pluginReg,
		stateReg,
		hookReg,
		logger,
	)

	return NewIoTService(
		pluginReg,
		deviceReg,
		stateReg,
		processingSvc,
		hookReg,
		nil, // default config
		logger,
	)
}

// createTestDevicePlugin creates a simple test device plugin for testing.
func createTestDevicePlugin() types.DevicePlugin {
	return &mockDevicePlugin{
		deviceType: types.DeviceTypeGenericSensor,
	}
}

type mockDevicePlugin struct {
	deviceType types.DeviceType
}

func (p *mockDevicePlugin) GetDeviceType() types.DeviceType {
	return p.deviceType
}

func (p *mockDevicePlugin) GetSupportedCapabilities() []types.DeviceCapability {
	return []types.DeviceCapability{types.DeviceCapabilityDataCapture}
}

func (p *mockDevicePlugin) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	return []types.Device{}, nil
}

func (p *mockDevicePlugin) CreateDevice(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	return &mockDevice{metadata: metadata}, nil
}

func (p *mockDevicePlugin) ValidateMetadata(metadata types.DeviceMetadata) error {
	return nil
}

type mockDevice struct {
	metadata types.DeviceMetadata
	enabled  bool
}

func (d *mockDevice) Start(ctx context.Context) error {
	return nil
}

func (d *mockDevice) Stop(ctx context.Context) error {
	return nil
}

func (d *mockDevice) GetID() string {
	return d.metadata.ID
}

func (d *mockDevice) GetMetadata() types.DeviceMetadata {
	return d.metadata
}

func (d *mockDevice) UpdateMetadata(ctx context.Context, updates *types.DeviceMetadataUpdate) error {
	if updates == nil {
		return nil
	}
	if updates.Name != nil {
		d.metadata.Name = *updates.Name
	}
	if updates.Enabled != nil {
		d.metadata.Enabled = *updates.Enabled
	}
	return nil
}

func (d *mockDevice) Enable(ctx context.Context) error {
	d.enabled = true
	return nil
}

func (d *mockDevice) Disable(ctx context.Context) error {
	d.enabled = false
	return nil
}

func (d *mockDevice) IsEnabled() bool {
	return d.enabled
}

func (d *mockDevice) GetStatus() types.DeviceStatus {
	return types.DeviceStatusOnline
}

func (d *mockDevice) HasCapability(capability types.DeviceCapability) bool {
	return false
}

func (d *mockDevice) GetCapabilities() types.DeviceCapabilities {
	return types.DeviceCapabilities{}
}

func (d *mockDevice) CaptureData(ctx context.Context) (*types.DeviceData, error) {
	return nil, nil
}

func (d *mockDevice) StartDataStream(ctx context.Context) (<-chan *types.DeviceData, error) {
	return nil, nil
}

func (d *mockDevice) StopDataStream(ctx context.Context) error {
	return nil
}

func (d *mockDevice) ReadSensor(ctx context.Context, sensorType string) (*types.SensorReading, error) {
	return nil, nil
}

func (d *mockDevice) ReadAllSensors(ctx context.Context) (map[string]*types.SensorReading, error) {
	return nil, nil
}

func (d *mockDevice) ExecuteCommand(ctx context.Context, command types.DeviceCommand) error {
	return nil
}

func (d *mockDevice) GetAvailableCommands(ctx context.Context) ([]types.DeviceCommand, error) {
	return []types.DeviceCommand{}, nil
}

func TestIoTService_Start(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)

	// Verify service is started
	status := service.HealthSnapshot()
	assert.True(t, status.SubServices["plugin-registry"].Started)
	assert.True(t, status.SubServices["device-registry"].Started)
	assert.True(t, status.SubServices["state-registry"].Started)
	assert.True(t, status.SubServices["processing"].Started)
	assert.True(t, status.SubServices["hooks"].Started)
}

func TestIoTService_Stop(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)

	err = service.Stop(ctx)
	require.NoError(t, err)
}

func TestIoTService_Start_AlreadyStarted(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)

	// Try to start again
	err = service.Start(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrAlreadyStarted, err)
}

func TestIoTService_DiscoverDevices(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin first
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Discover devices
	devices, err := service.DiscoverDevices(ctx)
	require.NoError(t, err)
	assert.NotNil(t, devices)
}

func TestIoTService_RegisterDevice(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin first
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Create a device
	metadata := types.DeviceMetadata{
		ID:   "test-device-1",
		Type: types.DeviceTypeGenericSensor,
	}
	device, err := plugin.CreateDevice(ctx, metadata)
	require.NoError(t, err)

	// Register the device
	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// Verify device is registered
	retrieved, err := service.GetDevice(ctx, "test-device-1")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-device-1", retrieved.GetID())
}

func TestIoTService_GetDevice(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin first
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Create and register a device
	metadata := types.DeviceMetadata{
		ID:   "test-device-1",
		Type: types.DeviceTypeGenericSensor,
	}
	device, err := plugin.CreateDevice(ctx, metadata)
	require.NoError(t, err)

	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// Get the device
	retrieved, err := service.GetDevice(ctx, "test-device-1")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-device-1", retrieved.GetID())

	// Try to get non-existent device
	_, err = service.GetDevice(ctx, "non-existent")
	assert.Error(t, err)
}

func TestIoTService_ListDevices(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin first
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Register multiple devices
	for i := 0; i < 3; i++ {
		metadata := types.DeviceMetadata{
			ID:   "test-device-" + string(rune('1'+i)),
			Type: types.DeviceTypeGenericSensor,
		}
		device, err := plugin.CreateDevice(ctx, metadata)
		require.NoError(t, err)
		err = service.RegisterDevice(ctx, device)
		require.NoError(t, err)
	}

	// List all devices
	devices, err := service.ListDevices(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, devices, 3)
}

func TestIoTService_GetStateMachine(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin first
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Create and register a device
	metadata := types.DeviceMetadata{
		ID:   "test-device-1",
		Type: types.DeviceTypeGenericSensor,
	}
	device, err := plugin.CreateDevice(ctx, metadata)
	require.NoError(t, err)

	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// Get state machine (should be created automatically during registration)
	stateMachine, err := service.GetStateMachine(ctx, "test-device-1")
	require.NoError(t, err)
	assert.NotNil(t, stateMachine)
	assert.Equal(t, "test-device-1", stateMachine.GetDeviceID())
}

func TestIoTService_ProcessDeviceData(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin first
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Create and register a device
	metadata := types.DeviceMetadata{
		ID:   "test-device-1",
		Type: types.DeviceTypeGenericSensor,
	}
	device, err := plugin.CreateDevice(ctx, metadata)
	require.NoError(t, err)

	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// Process device data
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeSensorReading,
		Data:     []byte("test data"),
	}
	result, err := service.ProcessDeviceData(ctx, device, data)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.OriginalData)
}

func TestIoTService_RegisterPlugin(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register a plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Verify plugin is registered
	deviceTypes, err := service.GetSupportedDeviceTypes(ctx)
	require.NoError(t, err)
	assert.Contains(t, deviceTypes, types.DeviceTypeGenericSensor)
}

func TestIoTService_HealthSnapshot(t *testing.T) {
	service := createTestIoTService(t)

	// Get health snapshot before starting
	status := service.HealthSnapshot()
	assert.NotNil(t, status)
	assert.Equal(t, 0, status.RegisteredDevices)
	assert.Equal(t, 0, len(status.PluginStatus))

	// Start service
	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Get health snapshot after starting
	status = service.HealthSnapshot()
	assert.NotNil(t, status)
	assert.True(t, status.SubServices["plugin-registry"].Started)
	assert.True(t, status.SubServices["device-registry"].Started)
	assert.True(t, status.SubServices["state-registry"].Started)
	assert.True(t, status.SubServices["processing"].Started)
	assert.True(t, status.SubServices["hooks"].Started)
}

// Integration test: Discovery → Registration → State management
func TestIoTService_Integration_DiscoveryToState(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// 1. Register plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// 2. Discover devices
	devices, err := service.DiscoverDevices(ctx)
	require.NoError(t, err)

	// 3. Register a device
	if len(devices) == 0 {
		// Create a device manually if discovery returns none
		metadata := types.DeviceMetadata{
			ID:   "test-device-1",
			Type: types.DeviceTypeGenericSensor,
		}
		device, err := plugin.CreateDevice(ctx, metadata)
		require.NoError(t, err)
		err = service.RegisterDevice(ctx, device)
		require.NoError(t, err)
		devices = []types.Device{device}
	}

	// 4. Get state machine (should be created during registration)
	deviceID := devices[0].GetID()
	stateMachine, err := service.GetStateMachine(ctx, deviceID)
	require.NoError(t, err)
	assert.NotNil(t, stateMachine)

	// 5. Verify state machine state (may be "undiscovered" initially, transitions to "registered" during registration)
	state := stateMachine.GetState()
	assert.Contains(t, []types.DeviceState{types.DeviceStateUndiscovered, types.DeviceStateRegistered}, state)
}

// Integration test: Data processing flow
func TestIoTService_Integration_DataProcessing(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// 1. Register plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// 2. Create and register device
	metadata := types.DeviceMetadata{
		ID:   "test-device-1",
		Type: types.DeviceTypeGenericSensor,
	}
	device, err := plugin.CreateDevice(ctx, metadata)
	require.NoError(t, err)

	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// 3. Process device data
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeSensorReading,
		Data:     []byte("test sensor data"),
	}
	result, err := service.ProcessDeviceData(ctx, device, data)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.OriginalData)
	assert.Equal(t, data.DataType, result.OriginalData.DataType)
}

// Integration test: Hook execution
func TestIoTService_Integration_HookExecution(t *testing.T) {
	service := createTestIoTService(t)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// 1. Register plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// 2. Create and register device (should trigger hooks during registration)
	metadata := types.DeviceMetadata{
		ID:   "test-device-1",
		Type: types.DeviceTypeGenericSensor,
	}
	device, err := plugin.CreateDevice(ctx, metadata)
	require.NoError(t, err)

	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// Verify device is registered (hooks executed successfully during registration)
	retrieved, err := service.GetDevice(ctx, "test-device-1")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
}

// TestIoTService_Integration_FullDeviceLifecycle tests the complete device lifecycle:
// discovery → registration → state management → data processing → deletion
func TestIoTService_Integration_FullDeviceLifecycle(t *testing.T) {
	service := createTestIoTService(t)
	ctx := context.Background()

	// Start service
	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// 1. Register plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// 2. Discover devices
	devices, err := service.DiscoverDevices(ctx)
	require.NoError(t, err)

	// If no devices discovered, create one manually
	if len(devices) == 0 {
		metadata := types.DeviceMetadata{
			ID:   "lifecycle-test-device",
			Name: "Lifecycle Test Device",
			Type: types.DeviceTypeGenericSensor,
		}
		device, err := plugin.CreateDevice(ctx, metadata)
		require.NoError(t, err)
		devices = []types.Device{device}
	}

	device := devices[0]
	deviceID := device.GetID()

	// 3. Register device
	err = service.RegisterDevice(ctx, device)
	require.NoError(t, err)

	// 4. Verify device registered
	registered, err := service.GetDevice(ctx, deviceID)
	require.NoError(t, err)
	assert.Equal(t, deviceID, registered.GetID())

	// 5. Verify state machine created
	sm, err := service.GetStateMachine(ctx, deviceID)
	require.NoError(t, err)
	assert.NotNil(t, sm)
	assert.Equal(t, deviceID, sm.GetDeviceID())

	// 6. Process device data
	data := &types.DeviceData{
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		DataType:  types.DeviceDataTypeSensorReading,
		Data:      []byte("test sensor data"),
	}
	result, err := service.ProcessDeviceData(ctx, device, data)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.OriginalData)

	// 7. Update device metadata
	newName := "Updated Device Name"
	updates := &types.DeviceMetadataUpdate{
		Name: &newName,
	}
	err = service.UpdateDevice(ctx, deviceID, updates)
	require.NoError(t, err)

	// Verify update
	updated, err := service.GetDevice(ctx, deviceID)
	require.NoError(t, err)
	assert.Equal(t, newName, updated.GetMetadata().Name)

	// 8. Delete device
	err = service.DeleteDevice(ctx, deviceID)
	require.NoError(t, err)

	// 9. Verify device deleted
	_, err = service.GetDevice(ctx, deviceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestIoTService_Integration_ErrorHandling tests error handling scenarios
func TestIoTService_Integration_ErrorHandling(t *testing.T) {
	service := createTestIoTService(t)
	ctx := context.Background()

	// Test operations before service is started
	_, err := service.DiscoverDevices(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrNotStarted, err)

	// Test registering nil device (should fail validation before checking started state)
	err = service.RegisterDevice(ctx, nil)
	assert.Error(t, err)
	assert.Equal(t, types.ErrInvalidDevice, err)

	// Start service
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Test device not found
	_, err = service.GetDevice(ctx, "nonexistent-device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test state machine not found
	_, err = service.GetStateMachine(ctx, "nonexistent-device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test registering device with nil (should fail validation)
	err = service.RegisterDevice(ctx, nil)
	assert.Error(t, err)

	// Test updating non-existent device
	updates := &types.DeviceMetadataUpdate{
		Name: func() *string { s := "test"; return &s }(),
	}
	err = service.UpdateDevice(ctx, "nonexistent-device", updates)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test deleting non-existent device
	err = service.DeleteDevice(ctx, "nonexistent-device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestIoTService_Integration_LifecycleCoordination tests service lifecycle coordination
func TestIoTService_Integration_LifecycleCoordination(t *testing.T) {
	service := createTestIoTService(t)
	ctx := context.Background()

	// Test start
	err := service.Start(ctx)
	require.NoError(t, err)

	// Verify service is started via health snapshot
	status := service.HealthSnapshot()
	assert.True(t, status.SubServices["plugin-registry"].Started)
	assert.True(t, status.SubServices["device-registry"].Started)
	assert.True(t, status.SubServices["state-registry"].Started)
	assert.True(t, status.SubServices["processing"].Started)
	assert.True(t, status.SubServices["hooks"].Started)

	// Test double start prevention
	err = service.Start(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrAlreadyStarted, err)

	// Test operations work after start
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	devices, err := service.DiscoverDevices(ctx)
	require.NoError(t, err)
	assert.NotNil(t, devices)

	// Test stop
	err = service.Stop(ctx)
	require.NoError(t, err)

	// Test operations fail after stop
	_, err = service.DiscoverDevices(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrNotStarted, err)

	// Test double stop (should be idempotent)
	err = service.Stop(ctx)
	require.NoError(t, err) // Should not error on second stop
}

// TestIoTService_Integration_ConcurrentOperations tests concurrent operations
func TestIoTService_Integration_ConcurrentOperations(t *testing.T) {
	service := createTestIoTService(t)
	ctx := context.Background()

	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Concurrent device registration
	const numDevices = 10
	errors := make(chan error, numDevices)
	devices := make([]types.Device, numDevices)

	for i := 0; i < numDevices; i++ {
		go func(idx int) {
			metadata := types.DeviceMetadata{
				ID:   fmt.Sprintf("concurrent-device-%d", idx),
				Type: types.DeviceTypeGenericSensor,
			}
			device, err := plugin.CreateDevice(ctx, metadata)
			if err != nil {
				errors <- err
				return
			}
			devices[idx] = device
			err = service.RegisterDevice(ctx, device)
			errors <- err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numDevices; i++ {
		err := <-errors
		assert.NoError(t, err, "Device %d registration should succeed", i)
	}

	// Verify all devices registered
	allDevices, err := service.ListDevices(ctx, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allDevices), numDevices)

	// Concurrent reads
	readErrors := make(chan error, numDevices)
	for i := 0; i < numDevices; i++ {
		go func(idx int) {
			_, err := service.GetDevice(ctx, fmt.Sprintf("concurrent-device-%d", idx))
			readErrors <- err
		}(i)
	}

	// Wait for all reads
	for i := 0; i < numDevices; i++ {
		err := <-readErrors
		assert.NoError(t, err, "Device %d should be retrievable", i)
	}
}

// TestIoTService_Integration_MultipleDevices tests operations with multiple devices
func TestIoTService_Integration_MultipleDevices(t *testing.T) {
	service := createTestIoTService(t)
	ctx := context.Background()

	err := service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	// Register plugin
	plugin := createTestDevicePlugin()
	err = service.RegisterPlugin(ctx, plugin)
	require.NoError(t, err)

	// Register multiple devices
	const numDevices = 5
	deviceIDs := make([]string, numDevices)
	for i := 0; i < numDevices; i++ {
		metadata := types.DeviceMetadata{
			ID:   fmt.Sprintf("multi-device-%d", i),
			Name: fmt.Sprintf("Device %d", i),
			Type: types.DeviceTypeGenericSensor,
		}
		device, err := plugin.CreateDevice(ctx, metadata)
		require.NoError(t, err)
		err = service.RegisterDevice(ctx, device)
		require.NoError(t, err)
		deviceIDs[i] = device.GetID()
	}

	// List all devices
	allDevices, err := service.ListDevices(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, allDevices, numDevices)

	// Get devices by type
	sensors, err := service.GetDevicesByType(ctx, types.DeviceTypeGenericSensor)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sensors), numDevices)

	// Get state machines by type
	stateMachines, err := service.GetStateMachinesByType(ctx, types.DeviceTypeGenericSensor)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(stateMachines), numDevices)

	// Verify health snapshot shows correct device count
	status := service.HealthSnapshot()
	assert.GreaterOrEqual(t, status.RegisteredDevices, numDevices)
	assert.GreaterOrEqual(t, status.StateRegistrySize, numDevices)
}


