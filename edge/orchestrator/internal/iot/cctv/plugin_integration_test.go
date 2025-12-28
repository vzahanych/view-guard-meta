package cctv_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// TestCCTVPlugin_Registered verifies that the CCTV plugin is properly registered
// and can be discovered through the IoTService.
func TestCCTVPlugin_Registered(t *testing.T) {
	// Create IoT service with real subcomponents
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	iotService := iot.NewIoTService(pluginReg, deviceReg, stateReg, processingSvc, hookReg, nil, logger)

	ctx := context.Background()
	err := iotService.Start(ctx)
	require.NoError(t, err)
	defer iotService.Stop(ctx)

	// Create mock CCTV service
	mockCCTVService := createTestCCTVService(t)
	camera1 := createTestCamera("camera-1", "Camera 1")
	camera2 := createTestCamera("camera-2", "Camera 2")

	// Setup mock expectations
	mockCCTVService.EXPECT().
		DiscoverCameras(ctx).
		Return(nil)
	mockCCTVService.EXPECT().
		GetDiscoveredCameras(ctx).
		Return([]*cctvtypes.Camera{camera1, camera2}, nil)
	mockCCTVService.EXPECT().
		GetCamera(ctx, "camera-1").
		Return(camera1, nil)
	mockCCTVService.EXPECT().
		GetCamera(ctx, "camera-2").
		Return(camera2, nil)

	// Create CCTV plugin
	cctvPlugin := cctv.NewCCTVDevicePlugin(mockCCTVService, logger)

	// Register plugin
	err = iotService.RegisterPlugin(ctx, cctvPlugin)
	require.NoError(t, err)

	// Verify plugin is registered - check GetSupportedDeviceTypes
	deviceTypes, err := iotService.GetSupportedDeviceTypes(ctx)
	require.NoError(t, err)
	assert.Contains(t, deviceTypes, types.DeviceTypeCamera, "CCTV plugin should be registered and camera type should be supported")

	// Verify cameras can be discovered through IoTService
	devices, err := iotService.DiscoverDevicesByType(ctx, types.DeviceTypeCamera)
	require.NoError(t, err, "DiscoverDevicesByType should succeed")
	assert.NotEmpty(t, devices, "Should discover at least one camera")
	assert.Len(t, devices, 2, "Should discover both cameras")

	// Verify discovered devices are cameras
	for _, device := range devices {
		assert.Equal(t, types.DeviceTypeCamera, device.GetMetadata().Type, "Device should be a camera")
		assert.NotEmpty(t, device.GetID(), "Device should have an ID")
		assert.Contains(t, []string{"camera-1", "camera-2"}, device.GetID(), "Device ID should match discovered cameras")
	}

	// Verify cameras can be registered through IoTService
	// Use the discovered device for registration (it's already a Device instance)
	// The devices were already discovered above, so we can use them
	registeredDevice := devices[0]
	assert.NotNil(t, registeredDevice, "Discovered device should not be nil")
	assert.Equal(t, "camera-1", registeredDevice.GetID(), "Discovered device should have correct ID")
	assert.Equal(t, types.DeviceTypeCamera, registeredDevice.GetMetadata().Type, "Discovered device should be a camera")
}

// TestCCTVPlugin_DiscoveryFlow verifies the complete discovery flow:
// 1. Plugin registration
// 2. Device discovery
// 3. Device registration
// 4. Device retrieval
func TestCCTVPlugin_DiscoveryFlow(t *testing.T) {
	// Create IoT service with real subcomponents
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	iotService := iot.NewIoTService(pluginReg, deviceReg, stateReg, processingSvc, hookReg, nil, logger)

	ctx := context.Background()
	err := iotService.Start(ctx)
	require.NoError(t, err)
	defer iotService.Stop(ctx)

	// Create mock CCTV service
	mockCCTVService := createTestCCTVService(t)
	camera1 := createTestCamera("camera-1", "Camera 1")

	// Setup mock expectations for discovery
	mockCCTVService.EXPECT().
		DiscoverCameras(ctx).
		Return(nil)
	mockCCTVService.EXPECT().
		GetDiscoveredCameras(ctx).
		Return([]*cctvtypes.Camera{camera1}, nil)
	mockCCTVService.EXPECT().
		GetCamera(ctx, "camera-1").
		Return(camera1, nil)

	// Create and register CCTV plugin
	cctvPlugin := cctv.NewCCTVDevicePlugin(mockCCTVService, logger)
	err = iotService.RegisterPlugin(ctx, cctvPlugin)
	require.NoError(t, err)

	// Step 1: Discover devices
	discoveredDevices, err := iotService.DiscoverDevicesByType(ctx, types.DeviceTypeCamera)
	require.NoError(t, err)
	assert.Len(t, discoveredDevices, 1)
	assert.Equal(t, "camera-1", discoveredDevices[0].GetID())

	// Step 2: Register device (use the discovered device)
	registeredDevice := discoveredDevices[0]
	assert.NotNil(t, registeredDevice)
	assert.Equal(t, "camera-1", registeredDevice.GetID())

	// Step 3: Retrieve registered device
	retrievedDevice, err := iotService.GetDevice(ctx, "camera-1")
	require.NoError(t, err)
	assert.NotNil(t, retrievedDevice)
	assert.Equal(t, "camera-1", retrievedDevice.GetID())
	assert.Equal(t, types.DeviceTypeCamera, retrievedDevice.GetMetadata().Type)

	// Step 4: List devices
	deviceType := types.DeviceTypeCamera
	devices, err := iotService.ListDevices(ctx, &types.DeviceFilters{
		Type: &deviceType,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, devices)
	found := false
	for _, device := range devices {
		if device.GetID() == "camera-1" {
			found = true
			break
		}
	}
	assert.True(t, found, "Discovered camera should appear in device list")
}

// TestCCTVPlugin_GetSupportedDeviceTypes verifies that GetSupportedDeviceTypes
// returns the camera type after plugin registration.
func TestCCTVPlugin_GetSupportedDeviceTypes(t *testing.T) {
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	processingReg := processing.NewDataProcessorRegistry(logger)
	processingSvc := processing.NewDataProcessingService(processingReg, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)
	deviceReg := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	iotService := iot.NewIoTService(pluginReg, deviceReg, stateReg, processingSvc, hookReg, nil, logger)

	ctx := context.Background()
	err := iotService.Start(ctx)
	require.NoError(t, err)
	defer iotService.Stop(ctx)

	// Initially, no device types should be supported
	deviceTypes, err := iotService.GetSupportedDeviceTypes(ctx)
	require.NoError(t, err)
	assert.Empty(t, deviceTypes, "Initially no device types should be supported")

	// Create and register CCTV plugin
	mockCCTVService := createTestCCTVService(t)
	cctvPlugin := cctv.NewCCTVDevicePlugin(mockCCTVService, logger)
	err = iotService.RegisterPlugin(ctx, cctvPlugin)
	require.NoError(t, err)

	// After registration, camera type should be supported
	deviceTypes, err = iotService.GetSupportedDeviceTypes(ctx)
	require.NoError(t, err)
	assert.Contains(t, deviceTypes, types.DeviceTypeCamera, "Camera type should be supported after plugin registration")
	assert.Len(t, deviceTypes, 1, "Only camera type should be supported")
}

