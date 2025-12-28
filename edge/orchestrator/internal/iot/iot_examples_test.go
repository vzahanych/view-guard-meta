package iot

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// createTestIoTServiceForExamples creates a test IoT service for use in examples.
// This is similar to createTestIoTService in iot_impl_test.go but doesn't require testing.T.
func createTestIoTServiceForExamples() IoTService {
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

// createTestDevicePluginForExamples creates a simple test device plugin for examples.
func createTestDevicePluginForExamples() types.DevicePlugin {
	return &exampleDevicePlugin{
		deviceType: types.DeviceTypeGenericSensor,
	}
}

type exampleDevicePlugin struct {
	deviceType types.DeviceType
}

func (p *exampleDevicePlugin) GetDeviceType() types.DeviceType {
	return p.deviceType
}

func (p *exampleDevicePlugin) GetSupportedCapabilities() []types.DeviceCapability {
	return []types.DeviceCapability{types.DeviceCapabilityDataCapture}
}

func (p *exampleDevicePlugin) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	// Return a test device for examples
	capabilities := make(types.DeviceCapabilities)
	capabilities.Add(types.DeviceCapabilityDataCapture)
	device := &exampleDevice{
		metadata: types.DeviceMetadata{
			ID:           "example-device-1",
			Name:         "Example Sensor",
			Type:         p.deviceType,
			Capabilities: capabilities,
		},
	}
	return []types.Device{device}, nil
}

func (p *exampleDevicePlugin) CreateDevice(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	// Ensure capabilities are set if not provided
	if metadata.Capabilities == nil || len(metadata.Capabilities) == 0 {
		capabilities := make(types.DeviceCapabilities)
		capabilities.Add(types.DeviceCapabilityDataCapture)
		metadata.Capabilities = capabilities
	}
	return &exampleDevice{metadata: metadata}, nil
}

func (p *exampleDevicePlugin) ValidateMetadata(metadata types.DeviceMetadata) error {
	return nil
}

type exampleDevice struct {
	metadata types.DeviceMetadata
	enabled  bool
}

func (d *exampleDevice) Start(ctx context.Context) error {
	return nil
}

func (d *exampleDevice) Stop(ctx context.Context) error {
	return nil
}

func (d *exampleDevice) GetID() string {
	return d.metadata.ID
}

func (d *exampleDevice) GetMetadata() types.DeviceMetadata {
	return d.metadata
}

func (d *exampleDevice) UpdateMetadata(ctx context.Context, updates *types.DeviceMetadataUpdate) error {
	if updates.Name != nil {
		d.metadata.Name = *updates.Name
	}
	if updates.Enabled != nil {
		d.metadata.Enabled = *updates.Enabled
	}
	return nil
}

func (d *exampleDevice) Enable(ctx context.Context) error {
	d.enabled = true
	return nil
}

func (d *exampleDevice) Disable(ctx context.Context) error {
	d.enabled = false
	return nil
}

func (d *exampleDevice) IsEnabled() bool {
	return d.enabled
}

func (d *exampleDevice) GetStatus() types.DeviceStatus {
	return types.DeviceStatusOnline
}

func (d *exampleDevice) HasCapability(capability types.DeviceCapability) bool {
	return capability == types.DeviceCapabilityDataCapture
}

func (d *exampleDevice) GetCapabilities() types.DeviceCapabilities {
	capabilities := make(types.DeviceCapabilities)
	capabilities.Add(types.DeviceCapabilityDataCapture)
	return capabilities
}

func (d *exampleDevice) CaptureData(ctx context.Context) (*types.DeviceData, error) {
	return &types.DeviceData{
		DeviceID:  d.metadata.ID,
		Timestamp: time.Now(),
		DataType:  types.DeviceDataTypeSensorReading,
	}, nil
}

func (d *exampleDevice) StartDataStream(ctx context.Context) (<-chan *types.DeviceData, error) {
	ch := make(chan *types.DeviceData, 1)
	return ch, nil
}

func (d *exampleDevice) StopDataStream(ctx context.Context) error {
	return nil
}

func (d *exampleDevice) ReadSensor(ctx context.Context, sensorType string) (*types.SensorReading, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *exampleDevice) ReadAllSensors(ctx context.Context) (map[string]*types.SensorReading, error) {
	return nil, fmt.Errorf("not implemented")
}

func (d *exampleDevice) ExecuteCommand(ctx context.Context, command types.DeviceCommand) error {
	return fmt.Errorf("not implemented")
}

func (d *exampleDevice) GetAvailableCommands(ctx context.Context) ([]types.DeviceCommand, error) {
	return []types.DeviceCommand{}, nil
}

func (d *exampleDevice) RefreshMetadata(ctx context.Context) error {
	return nil
}

// ExampleIoTServiceProvider demonstrates how to create an IoT Service using dependency injection.
//
// This example shows the typical usage pattern with Fx lifecycle management.
// The service will automatically start and stop sub-services in the correct order.
func ExampleIoTServiceProvider() {
	// In a real application, the service would be created via Fx:
	//
	//   service, err := IoTServiceProvider(
	//       lc,     // fx.Lifecycle
	//       config, // *types.IoTServiceConfig
	//       logger, // *zap.Logger
	//   )

	fmt.Println("IoT Service created via IoTServiceProvider with Fx lifecycle")
	// Output:
	// IoT Service created via IoTServiceProvider with Fx lifecycle
}

// ExampleIoTService_Start demonstrates how to start the IoT service and its sub-services.
func ExampleIoTService_Start() {
	// Create service with real subcomponents
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	service := NewIoTService(pluginReg, deviceReg, stateReg, processingSvc, hookReg, nil, logger)

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		fmt.Printf("Error starting service: %v\n", err)
		return
	}
	defer service.Stop(ctx)

	fmt.Println("Service started successfully")
	// Output:
	// Service started successfully
}

// ExampleIoTService_DiscoverDevices demonstrates how to discover devices using all registered plugins.
func ExampleIoTService_DiscoverDevices() {
	// Create and start service
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	service := NewIoTService(pluginReg, deviceReg, stateReg, processingSvc, hookReg, nil, logger)

	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Discover devices
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		fmt.Printf("Error discovering devices: %v\n", err)
		return
	}

	fmt.Printf("Discovered %d devices\n", len(devices))
	// Output:
	// Discovered 0 devices
}

// ExampleIoTService_RegisterDevice demonstrates how to register a discovered device.
func ExampleIoTService_RegisterDevice() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin first
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)

	// Discover devices
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		fmt.Printf("Error discovering devices: %v\n", err)
		return
	}

	// Register first discovered device
	if len(devices) > 0 {
		err = service.RegisterDevice(ctx, devices[0])
		if err != nil {
			fmt.Printf("Error registering device: %v\n", err)
			return
		}
		fmt.Printf("Registered device: %s\n", devices[0].GetID())
	}
	// Output:
	// Registered device: example-device-1
}

// ExampleIoTService_GetDevice demonstrates how to retrieve a device by ID.
func ExampleIoTService_GetDevice() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
		deviceID := devices[0].GetID()

		// Get device by ID
		device, err := service.GetDevice(ctx, deviceID)
		if err != nil {
			fmt.Printf("Error getting device: %v\n", err)
			return
		}

		fmt.Printf("Retrieved device: %s (%s)\n", device.GetID(), device.GetMetadata().Type)
	}
	// Output:
	// Retrieved device: example-device-1 (sensor)
}

// ExampleIoTService_ListDevices demonstrates how to list devices with optional filters.
func ExampleIoTService_ListDevices() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
	}

	// List all devices
	allDevices, err := service.ListDevices(ctx, nil)
	if err != nil {
		fmt.Printf("Error listing devices: %v\n", err)
		return
	}

	fmt.Printf("Total registered devices: %d\n", len(allDevices))
	for _, device := range allDevices {
		fmt.Printf("  - %s (%s)\n", device.GetID(), device.GetMetadata().Type)
	}
	// Output:
	// Total registered devices: 1
	//   - example-device-1 (sensor)
}

// ExampleIoTService_GetStateMachine demonstrates how to get a device state machine.
func ExampleIoTService_GetStateMachine() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
		deviceID := devices[0].GetID()

		// Get state machine
		sm, err := service.GetStateMachine(ctx, deviceID)
		if err != nil {
			fmt.Printf("Error getting state machine: %v\n", err)
			return
		}

		state := sm.GetState()
		fmt.Printf("Device %s state: %s\n", deviceID, state)
	}
	// Output:
	// Device example-device-1 state: undiscovered
}

// ExampleIoTService_ProcessDeviceData demonstrates how to process device data through the pipeline.
func ExampleIoTService_ProcessDeviceData() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) == 0 {
		return
	}
	_ = service.RegisterDevice(ctx, devices[0])

	// Create device data
	data := &types.DeviceData{
		DeviceID:  devices[0].GetID(),
		Timestamp: time.Now(),
		DataType:  types.DeviceDataTypeSensorReading,
		Data:      []byte("sensor reading data"),
	}

	// Process data
	result, err := service.ProcessDeviceData(ctx, devices[0], data)
	if err != nil {
		fmt.Printf("Error processing data: %v\n", err)
		return
	}

	fmt.Printf("Processed data with %d processors\n", len(result.ProcessorsApplied))
	// Output:
	// Processed data with 0 processors
}

// ExampleIoTService_RegisterPlugin demonstrates how to register a device plugin.
func ExampleIoTService_RegisterPlugin() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Create and register plugin
	plugin := createTestDevicePluginForExamples()
	err := service.RegisterPlugin(ctx, plugin)
	if err != nil {
		fmt.Printf("Error registering plugin: %v\n", err)
		return
	}

	// Verify plugin registered
	deviceTypes, err := service.GetSupportedDeviceTypes(ctx)
	if err != nil {
		fmt.Printf("Error getting supported types: %v\n", err)
		return
	}

	fmt.Printf("Supported device types: %v\n", deviceTypes)
	// Output:
	// Supported device types: [sensor]
}

// ExampleIoTService_HealthSnapshot demonstrates how to get a health snapshot of the service.
func ExampleIoTService_HealthSnapshot() {
	// Create and start service
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	service := NewIoTService(pluginReg, deviceReg, stateReg, processingSvc, hookReg, nil, logger)

	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Get health snapshot
	status := service.HealthSnapshot()
	fmt.Printf("Registered devices: %d, Plugins: %d, State machines: %d\n",
		status.RegisteredDevices,
		len(status.PluginStatus),
		status.StateRegistrySize)
	// Output:
	// Registered devices: 0, Plugins: 0, State machines: 0
}

// ExampleIoTServiceConfig demonstrates how to configure the IoT service.
func ExampleIoTServiceConfig() {
	config := &types.IoTServiceConfig{
		Discovery: types.DiscoveryConfig{
			AutoDiscover:      true,
			DiscoveryInterval: 30 * time.Second,
			DiscoveryTimeout:  10 * time.Second,
			ParallelDiscovery: true,
		},
		Processing: types.ProcessingConfig{
			Enabled:          true,
			ProcessorTimeout: 5 * time.Second,
		},
		StateMachine: types.StateMachineConfig{
			Enabled: true,
		},
		Hooks: types.HooksConfig{
			Enabled: true,
		},
	}

	// Validate config
	if err := config.Validate(); err != nil {
		fmt.Printf("Invalid config: %v\n", err)
		return
	}

	fmt.Println("IoT Service configuration created and validated")
	// Output:
	// IoT Service configuration created and validated
}

// ExampleIoTService_DiscoverDevicesByType demonstrates how to discover devices of a specific type.
func ExampleIoTService_DiscoverDevicesByType() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)

	// Discover devices by type
	deviceType := types.DeviceTypeGenericSensor
	devices, err := service.DiscoverDevicesByType(ctx, deviceType)
	if err != nil {
		fmt.Printf("Error discovering devices: %v\n", err)
		return
	}

	fmt.Printf("Discovered %d devices of type %s\n", len(devices), deviceType)
	for _, device := range devices {
		fmt.Printf("  - %s\n", device.GetID())
	}
	// Output:
	// Discovered 1 devices of type sensor
	//   - example-device-1
}

// ExampleIoTService_GetDevicesByType demonstrates how to get registered devices by type.
func ExampleIoTService_GetDevicesByType() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
	}

	// Get devices by type
	deviceType := types.DeviceTypeGenericSensor
	cameras, err := service.GetDevicesByType(ctx, deviceType)
	if err != nil {
		fmt.Printf("Error getting devices: %v\n", err)
		return
	}

	fmt.Printf("Devices of type %s: %d\n", deviceType, len(cameras))
	// Output:
	// Devices of type sensor: 1
}

// ExampleIoTService_GetDevicesByCapability demonstrates how to get devices by capability.
func ExampleIoTService_GetDevicesByCapability() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
	}

	// Get devices by capability
	capability := types.DeviceCapabilityDataCapture
	capableDevices, err := service.GetDevicesByCapability(ctx, capability)
	if err != nil {
		fmt.Printf("Error getting devices: %v\n", err)
		return
	}

	fmt.Printf("Devices with capability %s: %d\n", capability, len(capableDevices))
	// Output:
	// Devices with capability data_capture: 1
}

// ExampleIoTService_UpdateDevice demonstrates how to update device metadata.
func ExampleIoTService_UpdateDevice() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
		deviceID := devices[0].GetID()

		// Update device metadata
		newName := "Updated Sensor Name"
		updates := &types.DeviceMetadataUpdate{
			Name: &newName,
		}
		err := service.UpdateDevice(ctx, deviceID, updates)
		if err != nil {
			fmt.Printf("Error updating device: %v\n", err)
			return
		}

		// Verify update
		device, _ := service.GetDevice(ctx, deviceID)
		fmt.Printf("Device name updated to: %s\n", device.GetMetadata().Name)
	}
	// Output:
	// Device name updated to: Updated Sensor Name
}

// ExampleIoTService_DeleteDevice demonstrates how to delete a device.
func ExampleIoTService_DeleteDevice() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
		deviceID := devices[0].GetID()

		// Delete device
		err := service.DeleteDevice(ctx, deviceID)
		if err != nil {
			fmt.Printf("Error deleting device: %v\n", err)
			return
		}

		fmt.Printf("Device %s deleted\n", deviceID)
	}
	// Output:
	// Device example-device-1 deleted
}

// ExampleIoTService_GetStateMachinesByType demonstrates how to get state machines by device type.
func ExampleIoTService_GetStateMachinesByType() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin and device
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)
	devices, _ := service.DiscoverDevices(ctx)
	if len(devices) > 0 {
		_ = service.RegisterDevice(ctx, devices[0])
	}

	// Get state machines by type
	deviceType := types.DeviceTypeGenericSensor
	stateMachines, err := service.GetStateMachinesByType(ctx, deviceType)
	if err != nil {
		fmt.Printf("Error getting state machines: %v\n", err)
		return
	}

	fmt.Printf("State machines for type %s: %d\n", deviceType, len(stateMachines))
	// Output:
	// State machines for type sensor: 1
}

// ExampleIoTService_GetSupportedDeviceTypes demonstrates how to get all supported device types.
func ExampleIoTService_GetSupportedDeviceTypes() {
	// Create and start service
	service := createTestIoTServiceForExamples()
	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	// Register plugin
	plugin := createTestDevicePluginForExamples()
	_ = service.RegisterPlugin(ctx, plugin)

	// Get supported device types
	deviceTypes, err := service.GetSupportedDeviceTypes(ctx)
	if err != nil {
		fmt.Printf("Error getting supported types: %v\n", err)
		return
	}

	fmt.Printf("Supported device types: %v\n", deviceTypes)
	// Output:
	// Supported device types: [sensor]
}

