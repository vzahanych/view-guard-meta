package cctv_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/mocks"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// createTestCCTVService creates a mock CCTV service for testing
func createTestCCTVService(t *testing.T) *mocks.MockCCTVService {
	ctrl := gomock.NewController(t)
	return mocks.NewMockCCTVService(ctrl)
}

// createTestCamera creates a test camera
func createTestCamera(id, name string) *cctvtypes.Camera {
	now := time.Now()
	return &cctvtypes.Camera{
		ID:           id,
		Name:         name,
		Type:         cctvtypes.CameraTypeRTSP,
		Manufacturer: "Test Manufacturer",
		Model:        "Test Model",
		Enabled:      true,
		Status:       cctvtypes.CameraStatusOnline,
		LastSeen:     &now,
		DiscoveredAt: now,
		IPAddress:    "192.168.1.100",
		RTSPURLs:     []string{"rtsp://192.168.1.100:554/stream"},
		Config: cctvtypes.CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  false,
			Quality:          "high",
			FrameRate:        30,
			Resolution:       "1920x1080",
		},
		Capabilities: cctvtypes.CameraCapabilities{
			HasPTZ:          false,
			HasSnapshot:     true,
			HasVideoStreams: true,
		},
	}
}

func TestNewCCTVDevicePlugin(t *testing.T) {
	t.Run("creates plugin with logger", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		logger := zap.NewNop()

		plugin := cctv.NewCCTVDevicePlugin(mockService, logger)

		assert.NotNil(t, plugin)
		// Verify plugin implements DevicePlugin interface
		assert.Equal(t, types.DeviceTypeCamera, plugin.GetDeviceType())
	})

	t.Run("creates plugin with nil logger", func(t *testing.T) {
		mockService := createTestCCTVService(t)

		plugin := cctv.NewCCTVDevicePlugin(mockService, nil)

		assert.NotNil(t, plugin)
		assert.Equal(t, types.DeviceTypeCamera, plugin.GetDeviceType())
	})

	t.Run("creates plugin with nil service", func(t *testing.T) {
		logger := zap.NewNop()

		plugin := cctv.NewCCTVDevicePlugin(nil, logger)

		assert.NotNil(t, plugin)
		// Plugin creation succeeds, but operations will fail
		assert.Equal(t, types.DeviceTypeCamera, plugin.GetDeviceType())
	})
}

func TestCCTVDevicePlugin_GetDeviceType(t *testing.T) {
	mockService := createTestCCTVService(t)
	plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

	deviceType := plugin.GetDeviceType()

	assert.Equal(t, types.DeviceTypeCamera, deviceType)
}

func TestCCTVDevicePlugin_GetSupportedCapabilities(t *testing.T) {
	mockService := createTestCCTVService(t)
	plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

	capabilities := plugin.GetSupportedCapabilities()

	expectedCapabilities := []types.DeviceCapability{
		types.DeviceCapabilityDataCapture,
		types.DeviceCapabilityDataStreaming,
		types.DeviceCapabilityVideoCapture,
		types.DeviceCapabilityVideoStreaming,
		types.DeviceCapabilityVideoRecording,
		types.DeviceCapabilitySnapshot,
	}

	assert.Equal(t, len(expectedCapabilities), len(capabilities))
	for _, expectedCap := range expectedCapabilities {
		assert.Contains(t, capabilities, expectedCap)
	}
}

func TestCCTVDevicePlugin_DiscoverDevices(t *testing.T) {
	t.Run("successful discovery", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		camera1 := createTestCamera("camera-1", "Camera 1")
		camera2 := createTestCamera("camera-2", "Camera 2")

		// Setup mock expectations
		mockService.EXPECT().
			DiscoverCameras(ctx).
			Return(nil)
		mockService.EXPECT().
			GetDiscoveredCameras(ctx).
			Return([]*cctvtypes.Camera{camera1, camera2}, nil)
		mockService.EXPECT().
			GetCamera(ctx, "camera-1").
			Return(camera1, nil)
		mockService.EXPECT().
			GetCamera(ctx, "camera-2").
			Return(camera2, nil)

		devices, err := plugin.DiscoverDevices(ctx)

		require.NoError(t, err)
		assert.Len(t, devices, 2)
		assert.Equal(t, "camera-1", devices[0].GetID())
		assert.Equal(t, "camera-2", devices[1].GetID())
	})

	t.Run("discovery failure", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		discoveryError := errors.New("discovery failed")

		// Setup mock expectations
		mockService.EXPECT().
			DiscoverCameras(ctx).
			Return(discoveryError)

		devices, err := plugin.DiscoverDevices(ctx)

		require.Error(t, err)
		assert.Nil(t, devices)
		assert.Contains(t, err.Error(), "failed to discover cameras")
	})

	t.Run("get discovered cameras failure", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		getError := errors.New("get cameras failed")

		// Setup mock expectations
		mockService.EXPECT().
			DiscoverCameras(ctx).
			Return(nil)
		mockService.EXPECT().
			GetDiscoveredCameras(ctx).
			Return(nil, getError)

		devices, err := plugin.DiscoverDevices(ctx)

		require.Error(t, err)
		assert.Nil(t, devices)
		assert.Contains(t, err.Error(), "failed to get discovered cameras")
	})

	t.Run("partial failure - skip invalid camera", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		camera1 := createTestCamera("camera-1", "Camera 1")
		camera2 := createTestCamera("camera-2", "Camera 2")

		// Setup mock expectations
		mockService.EXPECT().
			DiscoverCameras(ctx).
			Return(nil)
		mockService.EXPECT().
			GetDiscoveredCameras(ctx).
			Return([]*cctvtypes.Camera{camera1, camera2}, nil)
		mockService.EXPECT().
			GetCamera(ctx, "camera-1").
			Return(camera1, nil)
		mockService.EXPECT().
			GetCamera(ctx, "camera-2").
			Return(nil, errors.New("camera not found"))

		devices, err := plugin.DiscoverDevices(ctx)

		require.NoError(t, err) // Partial failure is handled gracefully
		assert.Len(t, devices, 1) // Only camera-1 is created
		assert.Equal(t, "camera-1", devices[0].GetID())
	})

	t.Run("empty discovery result", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()

		// Setup mock expectations
		mockService.EXPECT().
			DiscoverCameras(ctx).
			Return(nil)
		mockService.EXPECT().
			GetDiscoveredCameras(ctx).
			Return([]*cctvtypes.Camera{}, nil)

		devices, err := plugin.DiscoverDevices(ctx)

		require.NoError(t, err)
		assert.Empty(t, devices)
	})
}

func TestCCTVDevicePlugin_CreateDevice(t *testing.T) {
	t.Run("successful device creation", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		camera := createTestCamera("camera-1", "Camera 1")
		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Name: "Camera 1",
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		// Setup mock expectations
		mockService.EXPECT().
			GetCamera(ctx, "camera-1").
			Return(camera, nil).
			Times(2) // Once for verification, once in NewCameraDevice

		device, err := plugin.CreateDevice(ctx, metadata)

		require.NoError(t, err)
		assert.NotNil(t, device)
		assert.Equal(t, "camera-1", device.GetID())
		assert.Equal(t, types.DeviceTypeCamera, device.GetMetadata().Type)
	})

	t.Run("invalid device type", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		metadata := types.DeviceMetadata{
			ID:   "device-1",
			Type: types.DeviceTypeGenericSensor, // Wrong type
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		device, err := plugin.CreateDevice(ctx, metadata)

		require.Error(t, err)
		assert.Nil(t, device)
		assert.Contains(t, err.Error(), "invalid camera metadata")
	})

	t.Run("missing camera ID", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		metadata := types.DeviceMetadata{
			ID:   "", // Missing ID
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		device, err := plugin.CreateDevice(ctx, metadata)

		require.Error(t, err)
		assert.Nil(t, device)
		assert.Contains(t, err.Error(), "invalid camera metadata")
	})

	t.Run("camera not found", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		// Setup mock expectations
		mockService.EXPECT().
			GetCamera(ctx, "camera-1").
			Return(nil, errors.New("camera not found"))

		device, err := plugin.CreateDevice(ctx, metadata)

		require.Error(t, err)
		assert.Nil(t, device)
		assert.Contains(t, err.Error(), "camera not found")
	})

	t.Run("device creation success", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		ctx := context.Background()
		camera := createTestCamera("camera-1", "Camera 1")
		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		// Setup mock expectations - GetCamera is called twice (once in CreateDevice, once in NewCameraDevice)
		mockService.EXPECT().
			GetCamera(ctx, "camera-1").
			Return(camera, nil).
			Times(2)

		device, err := plugin.CreateDevice(ctx, metadata)

		require.NoError(t, err)
		assert.NotNil(t, device)
		assert.Equal(t, "camera-1", device.GetID())
	})
}

func TestCCTVDevicePlugin_ValidateMetadata(t *testing.T) {
	t.Run("valid metadata with RTSP", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		err := plugin.ValidateMetadata(metadata)

		assert.NoError(t, err)
	})

	t.Run("valid metadata with ONVIF", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"http://192.168.1.100:8080/onvif"},
		}

		err := plugin.ValidateMetadata(metadata)

		assert.NoError(t, err)
	})

	t.Run("valid metadata with USB", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:        "camera-1",
			Type:      types.DeviceTypeCamera,
			DevicePath: "/dev/video0",
		}

		err := plugin.ValidateMetadata(metadata)

		assert.NoError(t, err)
	})

	t.Run("invalid device type", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "device-1",
			Type: types.DeviceTypeGenericSensor, // Wrong type
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		err := plugin.ValidateMetadata(metadata)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid device type")
		assert.Contains(t, err.Error(), "expected camera")
	})

	t.Run("missing camera ID", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "", // Missing ID
			Type: types.DeviceTypeCamera,
			Endpoints: []string{"rtsp://192.168.1.100:554/stream"},
		}

		err := plugin.ValidateMetadata(metadata)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "camera ID is required")
	})

	t.Run("missing connection method", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
			// No Endpoints, no DevicePath
		}

		err := plugin.ValidateMetadata(metadata)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "camera must have at least one of")
	})

	t.Run("valid with multiple endpoints", func(t *testing.T) {
		mockService := createTestCCTVService(t)
		plugin := cctv.NewCCTVDevicePlugin(mockService, zap.NewNop())

		metadata := types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
			Endpoints: []string{
				"rtsp://192.168.1.100:554/stream",
				"http://192.168.1.100:8080/onvif",
			},
		}

		err := plugin.ValidateMetadata(metadata)

		assert.NoError(t, err)
	})
}

// Integration test with IoTService
func TestCCTVDevicePlugin_Integration(t *testing.T) {
	t.Run("plugin registration and discovery", func(t *testing.T) {
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

		// Discover devices by type
		devices, err := iotService.DiscoverDevicesByType(ctx, types.DeviceTypeCamera)
		require.NoError(t, err)
		assert.NotEmpty(t, devices)

		// Verify devices are cameras
		for _, device := range devices {
			assert.Equal(t, types.DeviceTypeCamera, device.GetMetadata().Type)
			assert.NotEmpty(t, device.GetID())
		}

		// Verify plugin is registered
		deviceTypes, err := iotService.GetSupportedDeviceTypes(ctx)
		require.NoError(t, err)
		assert.Contains(t, deviceTypes, types.DeviceTypeCamera)
	})
}

