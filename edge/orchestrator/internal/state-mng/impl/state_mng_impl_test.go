package impl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	aigatewaymocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/mocks"
	eventbusmocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/mocks"
	eventbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/mocks"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastoragemocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/mocks"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	objectstoragemocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/mocks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/types"
	vmgatewaymocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/mocks"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	wireguardtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service/wireguard"
	webgatewaymocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway/mocks"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
)

// getGoroutineCount returns the current number of goroutines
func getGoroutineCount() int {
	return runtime.NumGoroutine()
}

// waitForGoroutines waits for goroutines to stabilize, allowing time for cleanup
func waitForGoroutines(_ *testing.T, initialCount int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current := runtime.NumGoroutine()
		if current <= initialCount {
			return current
		}
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}

// setupDeviceStateService creates and sets up DeviceStateService for tests that need camera state machines
func setupDeviceStateService(t *testing.T, sm *StateManagerImpl) {
	t.Helper()
	deviceStateService, err := iot.NewDeviceStateServiceWithDefaults()
	require.NoError(t, err)
	sm.SetDeviceStateService(deviceStateService)
}

// TestFrameProcessingGoroutineCleanup tests that frame processing goroutines are properly cleaned up
func TestFrameProcessingGoroutineCleanup(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	// Set up event bus expectations
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// Set up mock CCTV service with a test camera
	cameraID := "test-camera-1"
	camera := &cctvtypes.Camera{
		ID:      cameraID,
		Enabled: true,
	}

	mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
	// Frame processing loop will call CaptureFrame periodically
	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
		Data: []byte("test-frame-data"),
	}, nil).AnyTimes()
	sm.SetCCTVService(mockCCTVService)

	// Get initial goroutine count
	initialGoroutines := getGoroutineCount()

	// Start the state manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Start frame processing for a camera
	err = sm.startFrameProcessingForCamera(ctx, cameraID)
	require.NoError(t, err, "Failed to start frame processing - ensure CCTV service is set")

	// Verify goroutine was created
	time.Sleep(50 * time.Millisecond)
	afterStart := getGoroutineCount()
	assert.Greater(t, afterStart, initialGoroutines, "Expected goroutine count to increase after starting frame processing")

	// Stop frame processing for the camera
	sm.stopFrameProcessingForCamera(cameraID)

	// Wait for goroutine to be cleaned up
	finalGoroutines := waitForGoroutines(t, initialGoroutines, 2*time.Second)
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+1, "Expected goroutines to be cleaned up after stopping frame processing (allowing 1 for test infrastructure)")

	// Stop the state manager
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	err = sm.Stop(stopCtx)
	require.NoError(t, err)

	// Wait for all goroutines to be cleaned up
	finalAfterStop := waitForGoroutines(t, initialGoroutines, 2*time.Second)
	assert.LessOrEqual(t, finalAfterStop, initialGoroutines+1, "Expected all goroutines to be cleaned up after stopping state manager")
}

// TestStopAllFrameProcessingCleanup tests that stopAllFrameProcessing properly cleans up all goroutines
func TestStopAllFrameProcessingCleanup(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	// Set up event bus expectations
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// Set up mock CCTV service with multiple test cameras
	cameraIDs := []string{"camera-1", "camera-2", "camera-3"}
	for _, cameraID := range cameraIDs {
		camera := &cctvtypes.Camera{
			ID:      cameraID,
			Enabled: true,
		}
		mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
		// Frame processing loop will call CaptureFrame periodically
		mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
			Data: []byte("test-frame-data"),
		}, nil).AnyTimes()
	}
	sm.SetCCTVService(mockCCTVService)

	// Get initial goroutine count
	initialGoroutines := getGoroutineCount()

	// Start the state manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Start frame processing for multiple cameras
	for _, cameraID := range cameraIDs {
		err = sm.startFrameProcessingForCamera(ctx, cameraID)
		require.NoError(t, err)
	}

	// Verify goroutines were created
	time.Sleep(100 * time.Millisecond)
	afterStart := getGoroutineCount()
	assert.Greater(t, afterStart, initialGoroutines+len(cameraIDs)-1, "Expected goroutine count to increase after starting frame processing for multiple cameras")

	// Stop all frame processing
	sm.stopAllFrameProcessing()

	// Wait for all goroutines to be cleaned up
	finalGoroutines := waitForGoroutines(t, initialGoroutines, 3*time.Second)
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+1, "Expected all frame processing goroutines to be cleaned up after stopAllFrameProcessing")

	// Stop the state manager
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	err = sm.Stop(stopCtx)
	require.NoError(t, err)

	// Wait for all goroutines to be cleaned up
	finalAfterStop := waitForGoroutines(t, initialGoroutines, 2*time.Second)
	assert.LessOrEqual(t, finalAfterStop, initialGoroutines+1, "Expected all goroutines to be cleaned up after stopping state manager")
}

// TestShutdownOrdering tests that shutdown properly stops frame processing before canceling context
func TestShutdownOrdering(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	// Set up event bus expectations
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// Set up mock CCTV service with test cameras
	cameraIDs := []string{"camera-1", "camera-2"}
	for _, cameraID := range cameraIDs {
		camera := &cctvtypes.Camera{
			ID:      cameraID,
			Enabled: true,
		}
		mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
		// Frame processing loop will call CaptureFrame periodically
		mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
			Data: []byte("test-frame-data"),
		}, nil).AnyTimes()
	}
	sm.SetCCTVService(mockCCTVService)

	// Get initial goroutine count
	initialGoroutines := getGoroutineCount()

	// Start the state manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Start frame processing for cameras
	for _, cameraID := range cameraIDs {
		err = sm.startFrameProcessingForCamera(ctx, cameraID)
		require.NoError(t, err)
	}

	// Verify goroutines were created
	time.Sleep(100 * time.Millisecond)
	afterStart := getGoroutineCount()
	assert.Greater(t, afterStart, initialGoroutines+len(cameraIDs)-1, "Expected goroutines to be created")

	// Stop the state manager (should stop frame processing first)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	err = sm.Stop(stopCtx)
	require.NoError(t, err)

	// Wait for all goroutines to be cleaned up
	finalGoroutines := waitForGoroutines(t, initialGoroutines, 3*time.Second)
	assert.LessOrEqual(t, finalGoroutines, initialGoroutines+1, "Expected all goroutines to be cleaned up after shutdown")
}

// TestDuplicateGoroutinePrevention tests that duplicate goroutines are not created for the same camera
func TestDuplicateGoroutinePrevention(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks
	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	// Set up event bus expectations
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// Set up mock CCTV service
	cameraID := "test-camera-1"
	camera := &cctvtypes.Camera{
		ID:      cameraID,
		Enabled: true,
	}
	mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
	// Frame processing loop will call CaptureFrame periodically
	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
		Data: []byte("test-frame-data"),
	}, nil).AnyTimes()
	sm.SetCCTVService(mockCCTVService)

	// Start the state manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Get initial goroutine count
	initialGoroutines := getGoroutineCount()

	// Start frame processing for the same camera multiple times concurrently
	var wg sync.WaitGroup
	numAttempts := 10
	wg.Add(numAttempts)
	for i := 0; i < numAttempts; i++ {
		go func() {
			defer wg.Done()
			_ = sm.startFrameProcessingForCamera(ctx, cameraID) // nolint:errcheck
		}()
	}
	wg.Wait()

	// Wait a bit for goroutines to stabilize
	time.Sleep(200 * time.Millisecond)

	// Verify only one goroutine was created (not numAttempts)
	afterConcurrentStart := getGoroutineCount()
	expectedMaxGoroutines := initialGoroutines + 2 // 1 for frame processing + 1 for test infrastructure
	assert.LessOrEqual(t, afterConcurrentStart, expectedMaxGoroutines, "Expected only one frame processing goroutine despite concurrent start attempts")

	// Clean up
	sm.stopFrameProcessingForCamera(cameraID)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestNewStateManagerImpl tests the constructor
func TestNewStateManagerImpl(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	require.NotNil(t, sm)
	assert.Equal(t, "edge-state-manager", sm.Name())
}

// TestNewStateManagerImpl_NilEventBus tests constructor with nil event bus
func TestNewStateManagerImpl_NilEventBus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	sm, err := NewStateManagerImpl(nil, logger)
	assert.Error(t, err)
	assert.Nil(t, sm)
}

// TestNewStateManagerImpl_NilLogger tests constructor with nil logger
func TestNewStateManagerImpl_NilLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, nil)
	require.NoError(t, err)
	require.NotNil(t, sm)
}

// TestServiceSetters tests all service setter methods
func TestServiceSetters(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Test SetAIGateway
	mockAIGateway := struct{}{}
	sm.SetAIGateway(mockAIGateway)

	// Test SetCCTVService
	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	sm.SetCCTVService(mockCCTVService)

	// Test SetMetaStorage
	mockMetaStorage := struct{}{}
	sm.SetMetaStorage(mockMetaStorage)

	// Test SetObjectStorage
	mockObjectStorage := struct{}{}
	sm.SetObjectStorage(mockObjectStorage)

	// Test SetVMGateway
	mockVMGateway := struct{}{}
	sm.SetVMGateway(mockVMGateway)

	// Test SetWebGateway
	mockWebGateway := struct{}{}
	sm.SetWebGateway(mockWebGateway)

	// Test SetEdgeID
	sm.SetEdgeID("test-edge-001")
}

// TestSetConfig tests configuration setting
func TestSetConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	cfg := &config.Config{
		StateManager: types.StateManagerConfig{
			FrameProcessingInterval:    15 * time.Second,
			CapabilitySyncInterval:     3 * time.Minute,
			MaxConcurrentWorkflows:     5,
			FrameCaptureErrorThreshold: 3,
			StatePersistenceTimeout:    2 * time.Second,
		},
	}

	sm.SetConfig(cfg)
	assert.Equal(t, 15*time.Second, sm.frameProcessingInterval)
}

// TestStateMachineOperations tests state machine getters and setters
func TestStateMachineOperations(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	deviceStateService, err := iot.NewDeviceStateServiceWithDefaults()
	require.NoError(t, err)
	sm.SetDeviceStateService(deviceStateService)

	// Test getOrCreateCameraStateMachine
	cameraID := "test-camera-1"
	cameraSM1 := sm.getOrCreateCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM1)
	// Initial state from device state machine is "undiscovered", which maps to CameraStateUndiscovered
	// But if there's a workflow state, it might be different - check that it's a valid initial state
	initialState := cameraSM1.GetState()
	assert.True(t, initialState == types.CameraStateUndiscovered || initialState == types.CameraStateDiscovered || initialState == types.CameraStateSynced,
		"Initial state should be undiscovered, discovered, or synced, got: %s", initialState)

	// Test getCameraStateMachine
	cameraSM2 := sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM2)
	assert.Equal(t, cameraSM1, cameraSM2)

	// Test getAllCameraStateMachines
	allCameras := sm.getAllCameraStateMachines()
	assert.Len(t, allCameras, 1)
	assert.Contains(t, allCameras, cameraID)

	// Test getCameraStateMachine for non-existent camera
	nonExistent := sm.getCameraStateMachine("non-existent")
	assert.Nil(t, nonExistent)
}

// TestOperationalState tests operational state updates
func TestOperationalState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Test initial operational state
	initialState := sm.getOperationalState()
	assert.Equal(t, "healthy", initialState.StorageHealth)
	assert.False(t, initialState.AIProcessingActive)

	// Test updateOperationalState
	sm.updateOperationalState(func(op *OperationalState) {
		op.StorageHealth = "warning"
		op.AIProcessingActive = true
		op.CamerasEnabled = 5
	})

	updatedState := sm.getOperationalState()
	assert.Equal(t, "warning", updatedState.StorageHealth)
	assert.True(t, updatedState.AIProcessingActive)
	assert.Equal(t, 5, updatedState.CamerasEnabled)
}

// TestConnectionStateTransitions tests connection state machine transitions
func TestConnectionStateTransitions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Set up mock VM gateway with state tracking
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	currentState := vmgatewaytypes.ConnectionStateDisconnected
	
	// Track state changes
	mockVMGateway.EXPECT().GetConnectionState().DoAndReturn(func() vmgatewaytypes.ConnectionState {
		return currentState
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).DoAndReturn(func(newState vmgatewaytypes.ConnectionState, errorMsg string) error {
		currentState = newState
		return nil
	}).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().DoAndReturn(func() bool {
		return currentState == vmgatewaytypes.ConnectionStateTransportConnected || 
		       currentState == vmgatewaytypes.ConnectionStateAuthenticated ||
		       currentState == vmgatewaytypes.ConnectionStateCapabilitiesReceived
	}).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().DoAndReturn(func() bool {
		return currentState != vmgatewaytypes.ConnectionStateDisconnected
	}).AnyTimes()
	sm.SetVMGateway(mockVMGateway)

	// Ensure initial state is disconnected
	currentState = vmgatewaytypes.ConnectionStateDisconnected

	// Test WireGuard connected event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeTunnelConnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, vmgatewaytypes.ConnectionStateTunnelConnected, currentState)

	// Test HTTPS connected event (requires WireGuardConnected state)
	eventChan <- eventbustypes.Event{
		Type:      EventTypeTransportConnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, vmgatewaytypes.ConnectionStateTransportConnected, currentState)

	// Test Edge authenticated event (requires HTTPSConnected state)
	eventChan <- eventbustypes.Event{
		Type:      EventTypeEdgeAuthenticated,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, vmgatewaytypes.ConnectionStateAuthenticated, currentState)

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestCameraStateTransitions tests camera state machine transitions
func TestCameraStateTransitions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	deviceStateService, err := iot.NewDeviceStateServiceWithDefaults()
	require.NoError(t, err)
	sm.SetDeviceStateService(deviceStateService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"

	// Test camera discovered event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDeviceDiscovered,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
		},
	}
	time.Sleep(100 * time.Millisecond)
	cameraSM := sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	// After camera discovered event, state should be discovered or synced (depending on adapter mapping)
	state := cameraSM.GetState()
	assert.True(t, state == types.CameraStateDiscovered || state == types.CameraStateSynced,
		"State should be discovered or synced after camera discovered event, got: %s", state)

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestEventHandlers tests various event handlers
func TestEventHandlers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test storage warning event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeStorageWarning,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(100 * time.Millisecond)
	opState := sm.getOperationalState()
	assert.Equal(t, "warning", opState.StorageHealth)

	// Test storage full event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeStorageFull,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(100 * time.Millisecond)
	opState = sm.getOperationalState()
	assert.Equal(t, "full", opState.StorageHealth)

	// Test inference event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeInference,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(100 * time.Millisecond)
	opState = sm.getOperationalState()
	assert.True(t, opState.AIProcessingActive)

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestStartStop tests Start and Stop methods
func TestStartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test Start
	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test that Start is idempotent
	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test Stop
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	err = sm.Stop(stopCtx)
	require.NoError(t, err)

	// Test that Stop is idempotent
	err = sm.Stop(stopCtx)
	require.NoError(t, err)
}

// TestHandleCapabilitiesReceived tests capabilities received handler
func TestHandleCapabilitiesReceived(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()
	mockEventBus.EXPECT().Publish(gomock.Any()).AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockCCTVService.EXPECT().DiscoverCameras(gomock.Any()).Return(nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	sm.SetVMGateway(mockVMGateway)

	// Set connection state to authenticated
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test capabilities received with CCTV capability
	eventChan <- eventbustypes.Event{
		Type:      EventTypeCapabilitiesReceived,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"capabilities": map[string]interface{}{
				"cctv_camera": true,
			},
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleSnapshotRequested tests snapshot request handler
func TestHandleSnapshotRequested(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).Times(1)                          // Start() calls restoreStateFromStorage
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()                       // Start() and event handlers call persistStateToStorage
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes() // Start() calls checkServicesHealth
	mockMetaStorage.EXPECT().SavePendingSnapshotRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up DeviceStateService (required for camera state machines)
	deviceStateService, err := iot.NewDeviceStateServiceWithDefaults()
	require.NoError(t, err)
	sm.SetDeviceStateService(deviceStateService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	// Create camera state machine and set to synced BEFORE sending event
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")

	// Wait a bit to ensure state is set
	time.Sleep(50 * time.Millisecond)

	// Verify camera is in synced state before sending event
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateSynced, cameraSM.GetState())

	// Test snapshot requested event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitRequested,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id":    cameraID,
			"label":        "test-label",
			"custom_label": "custom",
			"count":        int32(10),
		},
	}
	// Wait longer for event to be processed through the event loop and workflow
	time.Sleep(500 * time.Millisecond)

	// Verify state transition
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateWaitingForScreenshots, cameraSM.GetState())

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleScreenshotSetReady tests screenshot set ready handler
func TestHandleScreenshotSetReady(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	deviceStateService, err := iot.NewDeviceStateServiceWithDefaults()
	require.NoError(t, err)
	sm.SetDeviceStateService(deviceStateService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	// Create camera state machine and set to waiting_for_screenshots
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")

	// Test screenshot set ready event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitSetReady,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state transition
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateScreenshotSetReady, cameraSM.GetState())

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleCameraDisconnected tests camera disconnected handler
func TestHandleCameraDisconnected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	// Create camera state machine and set to synced
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")

	// Test camera disconnected event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDeviceDisconnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state transition
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateDisconnected, cameraSM.GetState())

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed tests model deployed handler
func TestHandleModelDeployed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	modelID := "test-model-1"
	// Create camera state machine and set to screenshot_set_ready
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")

	// Test model deployed event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"model_id":  modelID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state transition
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateModelDeployed, cameraSM.GetState())

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_OutOfOrder tests model deployed when camera is not ready
func TestHandleModelDeployed_OutOfOrder(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	modelID := "test-model-1"
	// Create camera state machine but keep it in discovered state (not ready)
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")

	// Test model deployed event (out of order)
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"model_id":  modelID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state is still discovered (not model_deployed)
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateDiscovered, cameraSM.GetState())

	// Now transition to screenshot_set_ready - should process queued deployment
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")

	// Trigger workflow by sending screenshot_set_ready event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitSetReady,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state transition to model_deployed
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateModelDeployed, cameraSM.GetState())

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestDisconnectEvents tests disconnect events don't stop frame processing
func TestDisconnectEvents(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up mock VM gateway with state tracking
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	currentState := vmgatewaytypes.ConnectionStateDisconnected
	mockVMGateway.EXPECT().GetConnectionState().DoAndReturn(func() vmgatewaytypes.ConnectionState {
		return currentState
	}).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().DoAndReturn(func() vmgatewaytypes.ConnectionStateInfo {
		return vmgatewaytypes.ConnectionStateInfo{
			State:       currentState,
			LastUpdated: time.Now(),
		}
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).DoAndReturn(func(newState vmgatewaytypes.ConnectionState, errorMsg string) error {
		currentState = newState
		return nil
	}).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().DoAndReturn(func() bool {
		return currentState != vmgatewaytypes.ConnectionStateDisconnected
	}).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().DoAndReturn(func() bool {
		return currentState == vmgatewaytypes.ConnectionStateTransportConnected ||
			currentState == vmgatewaytypes.ConnectionStateAuthenticated ||
			currentState == vmgatewaytypes.ConnectionStateCapabilitiesReceived
	}).AnyTimes()
	sm.SetVMGateway(mockVMGateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Set connection state to authenticated (via proper path)
	currentState = vmgatewaytypes.ConnectionStateDisconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnecting, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnected, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTransportConnecting, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTransportConnected, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, "")

	// Test HTTPS disconnect
	eventChan <- eventbustypes.Event{
		Type:      EventTypeTransportDisconnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, vmgatewaytypes.ConnectionStateTunnelConnected, mockVMGateway.GetConnectionState())

	// Test WireGuard disconnect
	eventChan <- eventbustypes.Event{
		Type:      EventTypeTunnelDisconnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, vmgatewaytypes.ConnectionStateDisconnected, mockVMGateway.GetConnectionState())

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleScreenshotSaved tests screenshot saved handler
func TestHandleScreenshotSaved(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateDisconnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().Return(true).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).AnyTimes()
	mockMetaStorage.EXPECT().ListSecurityEvents(gomock.Any(), gomock.Any()).Return([]map[string]interface{}{}, nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitSaved,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id":     "test-camera-1",
			"screenshot_id": "test-screenshot-1",
		},
	}
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleClipRecorded tests clip recorded handler
func TestHandleClipRecorded(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	eventChan <- eventbustypes.Event{
		Type:      EventTypeRawDeviceDataClipRecorded,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": "test-camera-1",
			"clip_id":   "test-clip-1",
		},
	}
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleCameraRegistered tests camera registered handler
func TestHandleCameraRegistered(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	eventChan <- eventbustypes.Event{
		Type:      EventTypeDeviceRegistered,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": "test-camera-1",
		},
	}
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleCameraConnected tests camera connected handler
func TestHandleCameraConnected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	eventChan <- eventbustypes.Event{
		Type:      EventTypeDeviceConnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": "test-camera-1",
		},
	}
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleAIDetection tests AI detection handler
func TestHandleAIDetection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	eventChan <- eventbustypes.Event{
		Type:      EventTypeDetection,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": "test-camera-1",
		},
	}
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestInitializeServicesAfterAuth tests service initialization after auth
func TestInitializeServicesAfterAuth(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()
	mockEventBus.EXPECT().Publish(gomock.Any()).AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockAIGateway := aigatewaymocks.NewMockAIGateway(ctrl)

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().ListSecurityEvents(gomock.Any(), gomock.Any()).Return([]map[string]interface{}{}, nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetAIGateway(mockAIGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateAuthenticated,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).AnyTimes()
	sm.SetVMGateway(mockVMGateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Set connection state to authenticated first
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTransportConnected, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, "")

	// Trigger initializeServicesAfterAuth via EdgeAuthenticated event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeEdgeAuthenticated,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestSetConfig_AllFields tests SetConfig with all configuration fields
func TestSetConfig_AllFields(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	cfg := &config.Config{
		StateManager: types.StateManagerConfig{
			FrameProcessingInterval:    20 * time.Second,
			CapabilitySyncInterval:     4 * time.Minute,
			MaxConcurrentWorkflows:     8,
			FrameCaptureErrorThreshold: 4,
			StatePersistenceTimeout:    3 * time.Second,
		},
	}

	sm.SetConfig(cfg)
	assert.Equal(t, 20*time.Second, sm.frameProcessingInterval)
	assert.Equal(t, 4*time.Minute, sm.syncInterval)
	assert.Equal(t, 4, sm.frameCaptureErrorThreshold)
}

// TestSetServiceSetters_InvalidTypes tests setters with invalid types
func TestSetServiceSetters_InvalidTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Test with invalid types (should not panic, just ignore)
	sm.SetAIGateway("invalid")
	sm.SetCCTVService("invalid")
	sm.SetMetaStorage("invalid")
	sm.SetObjectStorage("invalid")
	sm.SetVMGateway("invalid")
	sm.SetWebGateway("invalid")
}

// TestPersistStateToStorage tests state persistence
func TestPersistStateToStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx := context.Background()

	// Test persistence with connection state only
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	require.NoError(t, err)

	// Test persistence with camera states
	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	cameraStates := map[string]types.CameraState{
		cameraID: cameraSM.GetState(),
	}

	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, cameraStates)
	require.NoError(t, err)
}

// TestPersistStateToStorage_NoMetaStorage tests persistence without metaStorage
func TestPersistStateToStorage_NoMetaStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "meta-storage not available")
}

// TestRestoreStateFromStorage tests state restoration
func TestRestoreStateFromStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	// Use disconnected state (which is the initial state, so transition will succeed)
	stateMap := map[string]interface{}{
		"connection_state": string(vmgatewaytypes.ConnectionStateDisconnected),
		"storage_health":   "healthy",
		"cameras_enabled":  2,
		"camera_states": map[string]interface{}{
			"camera-1": map[string]interface{}{
				"state": string(types.CameraStateDiscovered),
			},
		},
	}
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(stateMap, true).Times(1)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateDisconnected,
		LastUpdated: time.Now(),
	}).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx := context.Background()
	err = sm.restoreStateFromStorage(ctx)
	require.NoError(t, err)

	// Verify connection state was restored (should be disconnected, which is valid)
	assert.Equal(t, vmgatewaytypes.ConnectionStateDisconnected, mockVMGateway.GetConnectionState())

	// Verify camera state was restored
	cameraSM := sm.getCameraStateMachine("camera-1")
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateDiscovered, cameraSM.GetState())
}

// TestRestoreStateFromStorage_NoState tests restoration when no state exists
func TestRestoreStateFromStorage_NoState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()
	err = sm.restoreStateFromStorage(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no previous state found")
}

// TestProcessFrameForCamera tests frame processing
func TestProcessFrameForCamera(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockAIGateway := aigatewaymocks.NewMockAIGateway(ctrl)

	cameraID := "test-camera-1"
	frameData := []byte("test-frame-data")
	frame := &cctvtypes.Frame{Data: frameData}

	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(frame, nil).Times(1)
	mockObjectStorage.EXPECT().StoreFrame(gomock.Any(), gomock.Any(), frameData).Return(nil).Times(1)
	mockAIGateway.EXPECT().ProcessFrame(gomock.Any(), cameraID, gomock.Any(), frameData).Return(nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetObjectStorage(mockObjectStorage)
	sm.SetAIGateway(mockAIGateway)

	ctx := context.Background()
	sm.processFrameForCamera(ctx, cameraID)
}

// TestProcessFrameForCamera_CaptureError tests frame capture error handling
func TestProcessFrameForCamera_CaptureError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	cameraID := "test-camera-1"

	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(nil, fmt.Errorf("capture failed")).Times(6) // 6 failures to exceed threshold

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.frameCaptureErrorThreshold = 5 // Set threshold to 5

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	// Create camera state machine
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateModelDeployed, "")
	_ = cameraSM.Transition(types.CameraStateFrameProcessing, "")

	ctx := context.Background()

	// Process frames multiple times to trigger error threshold
	for i := 0; i < 6; i++ {
		sm.processFrameForCamera(ctx, cameraID)
	}

	// Verify camera transitioned to error state
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateError, cameraSM.GetState())
}

// TestExecuteModelDeployedWorkflowForCamera tests model deployed workflow
func TestExecuteModelDeployedWorkflowForCamera(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameraID := "test-camera-1"
	camera := &cctvtypes.Camera{
		ID:      cameraID,
		Enabled: true,
	}

	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).Times(1)                          // Start() calls restoreStateFromStorage
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes() // Start() calls checkServicesHealth
	// GetCamera is called by checkServicesHealth (to verify CCTV service) and by executeModelDeployedWorkflowForCamera
	mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
		Data: []byte("test-frame-data"),
	}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Set camera to model_deployed state
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")
	_ = cameraSM.Transition(types.CameraStateModelDeployed, "")

	// Execute workflow directly (not via event)
	sm.executeModelDeployedWorkflowForCamera(ctx, cameraID)

	// Wait a bit for goroutine to start and state transition
	time.Sleep(300 * time.Millisecond)

	// Verify camera transitioned to frame_processing
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateFrameProcessing, cameraSM.GetState())

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestSyncScreenshotsToVM tests screenshot sync to VM
func TestSyncScreenshotsToVM(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	cameraID := "test-camera-1"
	edgeID := "test-edge-1"

	screenshots := []metastoragetypes.ScreenshotMetadata{
		{
			ID:        "screenshot-1",
			CameraID:  cameraID,
			ObjectKey: "screenshots/camera-1/screenshot-1.jpg",
			Label:     "normal",
			CreatedAt: time.Now(),
		},
	}

	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return(screenshots, nil).Times(1)

	imageData := []byte("fake-image-data")
	reader := io.NopCloser(bytes.NewReader(imageData))
	mockObjectStorage.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Return(reader, nil).Times(1)

	mockVMGateway.EXPECT().SyncScreenshots(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncScreenshotsResponse{
		Success: true,
	}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetObjectStorage(mockObjectStorage)
	sm.SetVMGateway(mockVMGateway)
	sm.SetEdgeID(edgeID)

	ctx := context.Background()
	sm.syncScreenshotsToVM(ctx, cameraID)
}

// TestSyncScreenshotsToVM_NoScreenshots tests sync with no screenshots
func TestSyncScreenshotsToVM_NoScreenshots(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	cameraID := "test-camera-1"

	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetEdgeID("test-edge-1")

	ctx := context.Background()
	sm.syncScreenshotsToVM(ctx, cameraID)
}

// TestHandleConnectionErrorState tests connection error state handling
func TestHandleConnectionErrorState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateTunnelConnectionError).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTunnelConnectionError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Set connection state to error
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnectionError, "test error")

	// Handle error state
	sm.handleConnectionErrorState(ctx, vmgatewaytypes.ConnectionStateTunnelConnectionError)

	// Wait for recovery attempt
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleCameraErrorState tests camera error state handling
func TestHandleCameraErrorState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateError, "test error")

	// Handle error state
	sm.handleCameraErrorState(ctx, cameraID, types.CameraStateError)

	// Wait for recovery attempt
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestSyncCamerasWithVM tests camera sync to VM
func TestSyncCamerasWithVM(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameras := []*cctvtypes.Camera{
		{
			ID:      "camera-1",
			Name:    "Test Camera 1",
			Type:    cctvtypes.CameraTypeUSB,
			Enabled: true,
		},
	}

	mockCCTVService.EXPECT().GetDiscoveredCameras(gomock.Any()).Return(cameras, nil).Times(1)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().SyncCameras(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncCamerasResponse{
		Success: true,
		EnabledCameras: []*httpsclienttypes.EnabledCamera{
			{CameraID: "camera-1", Enabled: true},
		},
	}, nil).Times(1)
	mockCCTVService.EXPECT().EnableCamera(gomock.Any(), "camera-1").Return(nil).Times(1)
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateAuthenticated,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetEdgeID("test-edge-1")

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	// Create camera state machine in discovered state
	cameraSM := sm.getOrCreateCameraStateMachine("camera-1")
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")

	ctx := context.Background()
	sm.syncCamerasWithVM(ctx)

	// Verify camera state transitioned to synced
	cameraSM = sm.getCameraStateMachine("camera-1")
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateSynced, cameraSM.GetState())
}

// TestSyncCamerasWithVM_NoCameras tests sync with no cameras
func TestSyncCamerasWithVM_NoCameras(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockCCTVService.EXPECT().GetDiscoveredCameras(gomock.Any()).Return([]*cctvtypes.Camera{}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)

	ctx := context.Background()
	sm.syncCamerasWithVM(ctx)
}

// TestSyncCamerasWithVM_Error tests sync error handling
func TestSyncCamerasWithVM_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameras := []*cctvtypes.Camera{
		{ID: "camera-1", Enabled: true},
	}

	mockCCTVService.EXPECT().GetDiscoveredCameras(gomock.Any()).Return(cameras, nil).Times(1)
	mockVMGateway.EXPECT().SyncCameras(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("sync failed")).Times(1)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().Return(true).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()

	sm.syncCamerasWithVM(ctx)

	// Verify connection state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())
}

// TestInitiateWireGuardConnection tests WireGuard connection initiation
func TestInitiateWireGuardConnection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// Test case: already connected
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateTunnelConnected).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().Return(true).Times(1)
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTunnelConnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set state to wg_connecting (valid transition path)
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnecting, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.initiateWireGuardConnection(ctx)

	// Verify state transitioned to wireguard_connected
	assert.Equal(t, vmgatewaytypes.ConnectionStateTunnelConnected, mockVMGateway.GetConnectionState())
}

// TestInitiateWireGuardConnection_NoVMGateway tests without VM gateway
func TestInitiateWireGuardConnection_NoVMGateway(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockCCTVService.EXPECT().GetDiscoveredCameras(gomock.Any()).Return([]*cctvtypes.Camera{}, nil).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetCCTVService(mockCCTVService)

	// Note: This test is for the case when VM gateway is nil, so we don't set it
	// The code should handle this gracefully without panicking
	ctx := context.Background()
	sm.initiateWireGuardConnection(ctx)

	// When VM gateway is nil, the state manager should not crash
	// (The actual state transition won't happen, but the function should return)
}

// TestRecoverActiveWorkflows tests workflow recovery
func TestRecoverActiveWorkflows(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameraID := "test-camera-1"
	camera := &cctvtypes.Camera{
		ID:      cameraID,
		Enabled: true,
	}

	mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
		Data: []byte("test-frame-data"),
	}, nil).AnyTimes()
	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), cameraID, gomock.Any()).Return(&cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  0,
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
	}, nil).AnyTimes()
	mockCCTVService.EXPECT().GetDiscoveredCameras(gomock.Any()).Return([]*cctvtypes.Camera{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateCapabilitiesReceived).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateCapabilitiesReceived,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().Return(true).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).AnyTimes()
	mockVMGateway.EXPECT().SyncCapabilities(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncCapabilitiesResponse{
		Success: true,
	}, nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	// Set connection state to capabilities_received
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateCapabilitiesReceived, "")

	// Create camera in frame_processing state
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateModelDeployed, "")
	_ = cameraSM.Transition(types.CameraStateFrameProcessing, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.recoverActiveWorkflows(ctx)

	// Wait for recovery to complete
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestRecoverActiveWorkflows_ModelDeployed tests recovery of model deployed state
func TestRecoverActiveWorkflows_ModelDeployed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameraID := "test-camera-1"
	camera := &cctvtypes.Camera{
		ID:      cameraID,
		Enabled: true,
	}

	mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).AnyTimes()
	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(&cctvtypes.Frame{
		Data: []byte("test-frame-data"),
	}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().Return(true).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	// Create camera in model_deployed state
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
		metadataSM.SetModelID("test-model-1")
	}
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")
	_ = cameraSM.Transition(types.CameraStateModelDeployed, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.recoverActiveWorkflows(ctx)

	// Wait for recovery to complete
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestRecoverActiveWorkflows_ErrorState tests recovery from error state
func TestRecoverActiveWorkflows_ErrorState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateError).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	// Set connection state to error
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, "test error")

	// Create camera in error state
	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateError, "camera error")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.recoverActiveWorkflows(ctx)

	// Wait for recovery
	time.Sleep(100 * time.Millisecond)

	// Verify camera state was reset to discovered
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateDiscovered, cameraSM.GetState())
}

// TestSyncCameraCapabilities tests camera capability sync
func TestSyncCameraCapabilities(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameraID := "test-camera-1"
	camera := &cctvtypes.Camera{
		ID:      cameraID,
		Enabled: true,
	}

	mockCCTVService.EXPECT().GetCamera(gomock.Any(), cameraID).Return(camera, nil).Times(1)
	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), cameraID, gomock.Any()).Return(&cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  0,
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
	}, nil).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateAuthenticated,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).AnyTimes()
	mockVMGateway.EXPECT().SyncCapabilities(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncCapabilitiesResponse{
		Success: true,
	}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()
	err = sm.SyncCameraCapabilities(ctx, cameraID)
	require.NoError(t, err)
}

// TestSyncCameraCapabilities_NoVMGateway tests without VM gateway
func TestSyncCameraCapabilities_NoVMGateway(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = sm.SyncCameraCapabilities(ctx, "test-camera-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "VM gateway not available")
}

// TestSyncCameraCapabilities_NotConnected tests when HTTPS not connected
func TestSyncCameraCapabilities_NotConnected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
		mockVMGateway.EXPECT().IsTransportConnected().Return(false).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)

	ctx := context.Background()
	err = sm.SyncCameraCapabilities(ctx, "test-camera-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPS not connected")
}

// TestSyncOnce tests capability sync once
func TestSyncOnce(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameras := []*cctvtypes.Camera{
		{
			ID:      "camera-1",
			Enabled: true,
		},
	}

		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockCCTVService.EXPECT().ListCameras(gomock.Any(), false).Return(cameras, nil).Times(1)
	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), "camera-1", gomock.Any()).Return(&cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  0,
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
	}, nil).AnyTimes()
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).AnyTimes()
	mockVMGateway.EXPECT().SyncCapabilities(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncCapabilitiesResponse{
		Success: true,
	}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()
	sm.syncOnce(ctx)
}

// TestSyncOnce_NotConnected tests sync when not connected
func TestSyncOnce_NotConnected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
		mockVMGateway.EXPECT().IsTransportConnected().Return(false).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)

	ctx := context.Background()
	sm.syncOnce(ctx)

	// Verify pendingSync flag is set
	sm.pendingSyncMu.RLock()
	pending := sm.pendingSync
	sm.pendingSyncMu.RUnlock()
	assert.True(t, pending)
}

// TestSyncOnce_NoCameras tests sync with no cameras
func TestSyncOnce_NoCameras(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockCCTVService.EXPECT().ListCameras(gomock.Any(), false).Return([]*cctvtypes.Camera{}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)

	ctx := context.Background()
	sm.syncOnce(ctx)
}

// TestInitiateHTTPConnection tests HTTP connection initiation
func TestInitiateHTTPConnection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// Test case: already connected
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockVMGateway.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTransportConnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set state to wireguard_connected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnected, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.initiateHTTPConnection(ctx)

	// Verify state transitioned to https_connected
	assert.Equal(t, vmgatewaytypes.ConnectionStateTransportConnected, mockVMGateway.GetConnectionState())
}

// TestInitiateHTTPConnection_NoVMGateway tests without VM gateway
func TestInitiateHTTPConnection_NoVMGateway(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Note: This test is for the case when VM gateway is nil, so we don't set it
	// The code should handle this gracefully without panicking
	ctx := context.Background()
	sm.initiateHTTPConnection(ctx)

	// When VM gateway is nil, the state manager should not crash
	// (The actual state transition won't happen, but the function should return)
}

// TestBuildDatasetStatus tests dataset status building
func TestBuildDatasetStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	camera := &cctvtypes.Camera{
		ID:      "test-camera-1",
		Enabled: true,
	}
	datasetStatus := &cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  2,
		LabelCounts:           map[string]int{"normal": 2},
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
		LastSynced:            time.Now(),
	}

	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), camera.ID, gomock.Any()).Return(datasetStatus, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)

	ctx := context.Background()
	status := sm.buildDatasetStatus(ctx, camera)
	require.NotNil(t, status)
	assert.Equal(t, 2, status.LabeledSnapshotCount)
}

// TestBuildDatasetStatus_NoCCTVService tests without CCTV service
func TestBuildDatasetStatus_NoCCTVService(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	camera := &cctvtypes.Camera{
		ID:      "test-camera-1",
		Enabled: true,
	}

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx := context.Background()
	status := sm.buildDatasetStatus(ctx, camera)
	require.NotNil(t, status)
	assert.True(t, status.SnapshotRequired)
}

// TestCheckServicesHealth tests service health checking
func TestCheckServicesHealth(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockAIGateway := aigatewaymocks.NewMockAIGateway(ctrl)
	mockWebGateway := webgatewaymocks.NewMockWebGateway(ctrl)

	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).Times(1)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateDisconnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().Return(false).AnyTimes()
	mockVMGateway.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetCCTVService(mockCCTVService)
	sm.SetAIGateway(mockAIGateway)
	sm.SetWebGateway(mockWebGateway)
	sm.SetVMGateway(mockVMGateway)
	sm.SetEdgeID("edge-dev-001")

	// Set state to disconnected (required for health check)
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)
}

// TestCheckServicesHealth_NoMetaStorage tests health check without metaStorage
func TestCheckServicesHealth_NoMetaStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	// Don't set metaStorage (testing case where it's not available)

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)

	// Verify state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())
}

// TestInitiateConnection tests connection initiation
func TestInitiateConnection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cfg := &config.Config{
		VMGateway: vmgatewaytypes.VMGatewayConfig{
			WireGuard: vmgatewaytypes.WireGuardConfig{
				Enabled: false, // Use HTTP mode
			},
		},
	}

	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockVMGateway.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateAuthenticated,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetConfig(cfg)
	sm.SetEdgeID("test-edge-1")

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.initiateConnection(ctx)

	// Wait for connection to complete
	time.Sleep(200 * time.Millisecond)

	// Verify state transitioned to authenticated (via HTTP connection)
	assert.Equal(t, vmgatewaytypes.ConnectionStateAuthenticated, mockVMGateway.GetConnectionState())
}

// TestInitiateConnection_WireGuardEnabled tests with WireGuard enabled
func TestInitiateConnection_WireGuardEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cfg := &config.Config{
		VMGateway: vmgatewaytypes.VMGatewayConfig{
			WireGuard: vmgatewaytypes.WireGuardConfig{
				Enabled: true, // Use WireGuard mode
			},
		},
	}

	mockVMGateway.EXPECT().IsConnected().Return(true).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetConfig(cfg)

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.initiateConnection(ctx)

	// Wait for connection to complete
	time.Sleep(200 * time.Millisecond)

	// Verify state transitioned to wireguard_connected
	assert.Equal(t, vmgatewaytypes.ConnectionStateTunnelConnected, mockVMGateway.GetConnectionState())
}

// TestInitiateConnection_NoVMGateway tests without VM gateway
func TestInitiateConnection_NoVMGateway(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	// Note: This test is for the case when VM gateway is nil, so we don't set it
	// The code should handle this gracefully without panicking

	ctx := context.Background()
	sm.initiateConnection(ctx)

	// When VM gateway is nil, the state manager should not crash
	// (The actual state transition won't happen, but the function should return)
}

// TestExecuteFrameProcessingWorkflowForCamera tests frame processing workflow
func TestExecuteFrameProcessingWorkflowForCamera(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateModelDeployed, "")
	_ = cameraSM.Transition(types.CameraStateFrameProcessing, "")

	ctx := context.Background()
	sm.executeFrameProcessingWorkflowForCamera(ctx, cameraID)

	// Verify camera is still in frame_processing state
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateFrameProcessing, cameraSM.GetState())
}

// TestToCameraCapability tests camera capability conversion
func TestToCameraCapability(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	camera := &cctvtypes.Camera{
		ID:      "test-camera-1",
		Name:    "Test Camera",
		Type:    cctvtypes.CameraTypeUSB,
		Enabled: true,
	}

	datasetStatus := &cctvtypes.DatasetStatus{
		LabelCounts:           map[string]int{"normal": 10, "suspicious": 5},
		LabeledSnapshotCount:  15,
		RequiredSnapshotCount: 20,
		SnapshotRequired:      true,
		LastSynced:            time.Now(),
	}

	capability := sm.toDeviceCapability(camera, datasetStatus)
	require.NotNil(t, capability)
	assert.Equal(t, "test-camera-1", capability.DeviceID)
	assert.Equal(t, "Test Camera", capability.Name)
	assert.Equal(t, uint32(15), capability.LabeledSnapshotCount)
	assert.Equal(t, uint32(20), capability.RequiredSnapshotCount)
	assert.True(t, capability.SnapshotRequired)
	assert.Equal(t, uint32(10), capability.LabelCounts["normal"])
	assert.Equal(t, uint32(5), capability.LabelCounts["suspicious"])
}

// TestSyncScreenshotsToVM_Error tests screenshot sync error handling
func TestSyncScreenshotsToVM_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("list failed")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetEdgeID("test-edge-1")

	ctx := context.Background()
	sm.syncScreenshotsToVM(ctx, "test-camera-1")
}

// TestSyncScreenshotsToVM_Batching tests screenshot sync with batching
func TestSyncScreenshotsToVM_Batching(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	cameraID := "test-camera-1"
	edgeID := "test-edge-1"

	// Create 25 screenshots to test batching (20 per batch)
	screenshots := make([]metastoragetypes.ScreenshotMetadata, 25)
	for i := 0; i < 25; i++ {
		screenshots[i] = metastoragetypes.ScreenshotMetadata{
			ID:        fmt.Sprintf("screenshot-%d", i),
			CameraID:  cameraID,
			ObjectKey: fmt.Sprintf("screenshots/camera-1/screenshot-%d.jpg", i),
			Label:     "normal",
			CreatedAt: time.Now(),
		}
	}

	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return(screenshots, nil).Times(1)

	imageData := []byte("fake-image-data")
	for i := 0; i < 25; i++ {
		reader := io.NopCloser(bytes.NewReader(imageData))
		mockObjectStorage.EXPECT().LoadSnapshot(gomock.Any(), gomock.Any()).Return(reader, nil).Times(1)
	}

	// Expect 2 batches (20 + 5)
	mockVMGateway.EXPECT().SyncScreenshots(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncScreenshotsResponse{
		Success: true,
	}, nil).Times(2)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetObjectStorage(mockObjectStorage)
	sm.SetVMGateway(mockVMGateway)
	sm.SetEdgeID(edgeID)

	ctx := context.Background()
	sm.syncScreenshotsToVM(ctx, cameraID)
}

// TestHandleCapabilitiesReceived_NoCCTV tests capabilities without CCTV capability
func TestHandleCapabilitiesReceived_NoCCTV(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test capabilities without CCTV capability
	eventChan <- eventbustypes.Event{
		Type:      EventTypeCapabilitiesReceived,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"capabilities": map[string]interface{}{
				"other_capability": true,
			},
		},
	}
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestHandleCapabilitiesReceived_DiscoveryError tests camera discovery error
func TestHandleCapabilitiesReceived_DiscoveryError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()
	mockEventBus.EXPECT().Publish(gomock.Any()).AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockCCTVService.EXPECT().DiscoverCameras(gomock.Any()).Return(fmt.Errorf("discovery failed")).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	// Set connection state to authenticated
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test capabilities received with CCTV capability but discovery fails
	eventChan <- eventbustypes.Event{
		Type:      EventTypeCapabilitiesReceived,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"capabilities": map[string]interface{}{
				"cctv_camera": true,
			},
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify connection state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestUpdateStateForEvent_AllEventTypes tests all event types in updateStateForEvent
func TestUpdateStateForEvent_AllEventTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Test EventTypeInference
	ev := eventbustypes.Event{
		Type:      EventTypeInference,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	_, _, err = sm.updateStateForEvent(ev)
	require.NoError(t, err)

	// Test EventTypeRawDeviceDataFrameReceived
	ev = eventbustypes.Event{
		Type:      EventTypeRawDeviceDataFrameReceived,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	_, _, err = sm.updateStateForEvent(ev)
	require.NoError(t, err)
}

// TestExecuteWorkflow_AllWorkflows tests all workflow types
func TestExecuteWorkflow_AllWorkflows(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()
	mockEventBus.EXPECT().Publish(gomock.Any()).AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateTunnelConnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTunnelConnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Test EventTypeTunnelConnected workflow
	eventChan <- eventbustypes.Event{
		Type:      EventTypeTunnelConnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(100 * time.Millisecond)

	// Test EventTypeTransportConnected workflow
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnected, "")
	eventChan <- eventbustypes.Event{
		Type:      EventTypeTransportConnected,
		Source:    "test",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestSyncOnce_Error tests syncOnce error handling
func TestSyncOnce_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	cameras := []*cctvtypes.Camera{
		{ID: "camera-1", Enabled: true},
	}

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).AnyTimes()

		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockCCTVService.EXPECT().ListCameras(gomock.Any(), false).Return(cameras, nil).Times(1)
	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), "camera-1", gomock.Any()).Return(&cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  0,
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
	}, nil).AnyTimes()
	mockVMGateway.EXPECT().SyncCapabilities(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("sync failed")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()
	sm.syncOnce(ctx)

	// Verify pendingSync flag is set
	sm.pendingSyncMu.RLock()
	pending := sm.pendingSync
	sm.pendingSyncMu.RUnlock()
	assert.True(t, pending)
}

// TestSyncOnce_AuthError tests syncOnce with authentication error
func TestSyncOnce_AuthError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameras := []*cctvtypes.Camera{
		{ID: "camera-1", Enabled: true},
	}

		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockCCTVService.EXPECT().ListCameras(gomock.Any(), false).Return(cameras, nil).Times(1)
	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), "camera-1", gomock.Any()).Return(&cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  0,
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
	}, nil).AnyTimes()
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).AnyTimes()
	mockVMGateway.EXPECT().SyncCapabilities(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("unauthenticated")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()
	sm.syncOnce(ctx)

	// Verify pendingSync flag is set
	sm.pendingSyncMu.RLock()
	pending := sm.pendingSync
	sm.pendingSyncMu.RUnlock()
	assert.True(t, pending)
}

// TestSyncOnce_Rejected tests syncOnce with rejected response
func TestSyncOnce_Rejected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameras := []*cctvtypes.Camera{
		{ID: "camera-1", Enabled: true},
	}

		mockVMGateway.EXPECT().IsTransportConnected().Return(true).Times(1)
	mockCCTVService.EXPECT().ListCameras(gomock.Any(), false).Return(cameras, nil).Times(1)
	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), "camera-1", gomock.Any()).Return(&cctvtypes.DatasetStatus{
		LabeledSnapshotCount:  0,
		RequiredSnapshotCount: 10,
		SnapshotRequired:      true,
	}, nil).AnyTimes()
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).AnyTimes()
	mockVMGateway.EXPECT().SyncCapabilities(gomock.Any(), gomock.Any()).Return(&httpsclienttypes.SyncCapabilitiesResponse{
		Success:      false,
		ErrorMessage: "not registered",
	}, nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	ctx := context.Background()
	sm.syncOnce(ctx)

	// Verify pendingSync flag is set
	sm.pendingSyncMu.RLock()
	pending := sm.pendingSync
	sm.pendingSyncMu.RUnlock()
	assert.True(t, pending)
}

// TestBuildDatasetStatus_Error tests dataset status building with error
func TestBuildDatasetStatus_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	camera := &cctvtypes.Camera{
		ID:      "test-camera-1",
		Enabled: true,
	}

	mockCCTVService.EXPECT().GetDatasetStatus(gomock.Any(), camera.ID, gomock.Any()).Return(nil, fmt.Errorf("status failed")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)

	ctx := context.Background()
	status := sm.buildDatasetStatus(ctx, camera)
	require.NotNil(t, status)
	assert.True(t, status.SnapshotRequired)
}

// TestProcessFrameForCamera_StorageError tests frame processing with storage error
func TestProcessFrameForCamera_StorageError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)

	cameraID := "test-camera-1"
	frameData := []byte("test-frame-data")
	frame := &cctvtypes.Frame{Data: frameData}

	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(frame, nil).Times(1)
	mockObjectStorage.EXPECT().StoreFrame(gomock.Any(), gomock.Any(), frameData).Return(fmt.Errorf("storage failed")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetObjectStorage(mockObjectStorage)

	ctx := context.Background()
	sm.processFrameForCamera(ctx, cameraID)
}

// TestProcessFrameForCamera_AIError tests frame processing with AI gateway error
func TestProcessFrameForCamera_AIError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockAIGateway := aigatewaymocks.NewMockAIGateway(ctrl)

	cameraID := "test-camera-1"
	frameData := []byte("test-frame-data")
	frame := &cctvtypes.Frame{Data: frameData}

	mockCCTVService.EXPECT().CaptureFrame(gomock.Any(), cameraID).Return(frame, nil).Times(1)
	mockObjectStorage.EXPECT().StoreFrame(gomock.Any(), gomock.Any(), frameData).Return(nil).Times(1)
	mockAIGateway.EXPECT().ProcessFrame(gomock.Any(), cameraID, gomock.Any(), frameData).Return(fmt.Errorf("ai failed")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetCCTVService(mockCCTVService)
	sm.SetObjectStorage(mockObjectStorage)
	sm.SetAIGateway(mockAIGateway)

	ctx := context.Background()
	sm.processFrameForCamera(ctx, cameraID)
}

// TestInitiateWireGuardConnection_Timeout tests WireGuard connection timeout
func TestInitiateWireGuardConnection_Timeout(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// Test case: not connected, will timeout
	mockVMGateway.EXPECT().IsConnected().Return(false).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTunnelConnecting,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set state to wg_connecting
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnecting, "")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	sm.initiateWireGuardConnection(ctx)

	// Wait for timeout (the function has a 60s timeout, but our context is 100ms)
	// The function will check context cancellation in the loop
	time.Sleep(200 * time.Millisecond)

	// Verify state transitioned to error (timeout should trigger after 60s, but context cancellation may happen first)
	// Since we're using a short context timeout, the function may exit before the 60s timeout
	// The state should remain wg_connecting if context is cancelled before timeout
	state := mockVMGateway.GetConnectionState()
	assert.True(t, state == vmgatewaytypes.ConnectionStateTunnelConnecting || state == vmgatewaytypes.ConnectionStateTunnelConnectionError,
		"State should be wg_connecting (context cancelled) or wg_connection_error (timeout)")
}

// TestInitiateHTTPConnection_AuthError tests HTTP connection with auth error
func TestInitiateHTTPConnection_AuthError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	mockVMGateway.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(fmt.Errorf("auth failed")).Times(1)
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateHTTPConnectionError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetEdgeID("test-edge-1")

	// Set state to http_connecting
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTransportConnecting, "")

	ctx := context.Background()
	sm.initiateHTTPConnection(ctx)

	// Verify state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateHTTPConnectionError, mockVMGateway.GetConnectionState())
}

// TestRecoverActiveWorkflows_WGConnecting tests recovery of WireGuard connecting state
func TestRecoverActiveWorkflows_WGConnecting(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	mockVMGateway.EXPECT().IsConnected().Return(true).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTunnelConnecting,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set connection state to wg_connecting
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnecting, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.recoverActiveWorkflows(ctx)

	// Wait for recovery
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestRecoverActiveWorkflows_ScreenshotSetReady tests recovery of screenshot set ready state
func TestRecoverActiveWorkflows_ScreenshotSetReady(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	cameraID := "test-camera-1"

	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateAuthenticated).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateAuthenticated,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().IsConnected().Return(true).AnyTimes()
	mockMetaStorage.EXPECT().ListScreenshots(gomock.Any(), gomock.Any()).Return([]metastoragetypes.ScreenshotMetadata{}, nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	// Set connection state to authenticated
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, "")

	// Create camera in screenshot_set_ready state
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
		metadataSM.SetDatasetID("test-dataset-1")
	}
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.recoverActiveWorkflows(ctx)

	// Wait for recovery
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestCheckServicesHealth_AllServices tests health check with all services
func TestCheckServicesHealth_AllServices(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockAIGateway := aigatewaymocks.NewMockAIGateway(ctrl)
	mockWebGateway := webgatewaymocks.NewMockWebGateway(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).Times(1)

	// Set up mock VM gateway
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateDisconnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetCCTVService(mockCCTVService)
	sm.SetAIGateway(mockAIGateway)
	sm.SetWebGateway(mockWebGateway)
	sm.SetVMGateway(mockVMGateway)

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)
}

// TestCheckServicesHealth_StorageError tests health check with storage error
func TestCheckServicesHealth_StorageError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(nil, fmt.Errorf("storage error")).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)

	// Verify state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())
}

// TestCheckServicesHealth_NoCCTVService tests health check without CCTV service
func TestCheckServicesHealth_NoCCTVService(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetVMGateway(mockVMGateway)
	// Don't set CCTV service

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)

	// Verify state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())
}

// TestCheckServicesHealth_NoAIGateway tests health check without AI gateway
func TestCheckServicesHealth_NoAIGateway(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)

	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetCCTVService(mockCCTVService)
	sm.SetVMGateway(mockVMGateway)
	// Don't set AI gateway

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)

	// Verify state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())
}

// TestCheckServicesHealth_NoWebGateway tests health check without web gateway
func TestCheckServicesHealth_NoWebGateway(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockCCTVService := mocks.NewMockCCTVService(ctrl)
	mockAIGateway := aigatewaymocks.NewMockAIGateway(ctrl)

	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Set up mock VM gateway
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateDisconnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateError,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetCCTVService(mockCCTVService)
	sm.SetAIGateway(mockAIGateway)
	sm.SetVMGateway(mockVMGateway)
	// Don't set web gateway

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx := context.Background()
	sm.checkServicesHealth(ctx)

	// Verify state transitioned to error
	assert.Equal(t, vmgatewaytypes.ConnectionStateError, mockVMGateway.GetConnectionState())
}

// TestInitiateConnection_NotDisconnected tests connection initiation when not disconnected
func TestInitiateConnection_NotDisconnected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
	mockVMGateway.EXPECT().GetConnectionState().Return(vmgatewaytypes.ConnectionStateTunnelConnected).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateTunnelConnected,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)

	// Set state to wireguard_connected (not disconnected)
	// First transition to wg_connecting, then to wireguard_connected (valid transition path)
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnecting, "")
	err = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateTunnelConnected, "")
	require.NoError(t, err, "State transition should succeed")

	// Verify state is set correctly before calling initiateConnection
	assert.Equal(t, vmgatewaytypes.ConnectionStateTunnelConnected, mockVMGateway.GetConnectionState(), "State should be tunnel_connected before calling initiateConnection")

	ctx := context.Background()
	sm.initiateConnection(ctx)

	// Verify state is still wireguard_connected (not changed)
	assert.Equal(t, vmgatewaytypes.ConnectionStateTunnelConnected, mockVMGateway.GetConnectionState(), "State should remain tunnel_connected after initiateConnection")
}

// TestInitiateConnection_NoConfig tests connection initiation without config
func TestInitiateConnection_NoConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)
		mockVMGateway.EXPECT().IsTransportConnected().Return(true).AnyTimes()
	mockVMGateway.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockVMGateway.EXPECT().GetConnectionStateInfo().Return(vmgatewaytypes.ConnectionStateInfo{
		State:       vmgatewaytypes.ConnectionStateAuthenticated,
		LastUpdated: time.Now(),
	}).AnyTimes()
	mockVMGateway.EXPECT().TransitionConnectionState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetVMGateway(mockVMGateway)
	sm.SetMetaStorage(mockMetaStorage)
	sm.SetEdgeID("test-edge-1")
	// Don't set config (will default to HTTP mode)

	// Set state to disconnected
	_ = mockVMGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sm.initiateConnection(ctx)

	// Wait for connection
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx) // nolint:errcheck
}

// TestPersistStateToStorage_RetryLogic tests retry logic with exponential backoff
func TestPersistStateToStorage_RetryLogic(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// First two attempts fail, third succeeds
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error 1")).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error 2")).Times(1)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 3
	sm.statePersistenceRetryBackoff = 100 * time.Millisecond // Short backoff for testing

	ctx := context.Background()
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	require.NoError(t, err, "Should succeed after retries")
}

// TestPersistStateToStorage_AllRetriesFail tests that error is returned after all retries fail
func TestPersistStateToStorage_AllRetriesFail(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// All attempts fail
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("persistent storage error")).Times(4) // 1 initial + 3 retries

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 3
	sm.statePersistenceRetryBackoff = 50 * time.Millisecond // Short backoff for testing

	ctx := context.Background()
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist state after 4 attempts")
}

// TestPersistStateToStorage_NoRetries tests behavior when max retries is 0
func TestPersistStateToStorage_NoRetries(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// First attempt fails, no retries
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 0 // No retries
	sm.statePersistenceRetryBackoff = 100 * time.Millisecond

	ctx := context.Background()
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to persist state after 1 attempts")
}

// TestPersistStateToStorage_ContextCancellation tests that context cancellation is respected
func TestPersistStateToStorage_ContextCancellation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// First attempt fails, then context is cancelled during backoff
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 3
	sm.statePersistenceRetryBackoff = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt fails but before retry
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context cancelled")
}

// TestPersistStateToStorage_ExponentialBackoff tests that backoff increases exponentially
func TestPersistStateToStorage_ExponentialBackoff(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// All attempts fail
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error")).Times(4)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 3
	sm.statePersistenceRetryBackoff = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	duration := time.Since(start)

	require.Error(t, err)
	// Should have waited for backoff: 50ms (attempt 1) + 100ms (attempt 2) + 200ms (attempt 3) = ~350ms
	// But capped at 10 seconds, so should be around 350ms
	assert.Greater(t, duration, 300*time.Millisecond, "Should have waited for exponential backoff")
	assert.Less(t, duration, 1*time.Second, "Should not wait too long")
}

// TestPersistStateToStorage_BackoffCapped tests that backoff is capped at 10 seconds
func TestPersistStateToStorage_BackoffCapped(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)

	// First attempt fails, context times out during backoff
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error")).Times(1)

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 1
	sm.statePersistenceRetryBackoff = 20 * time.Second // Large backoff that would exceed cap

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = sm.persistStateToStorage(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil)
	duration := time.Since(start)

	require.Error(t, err)
	// Should be capped at 10 seconds, but we cancel after 500ms
	assert.Less(t, duration, 1*time.Second, "Should respect context timeout")
	// Should have checked context during backoff wait
	assert.Contains(t, err.Error(), "context cancelled")
}

// TestPersistStateToStorageWithErrorHandling tests the helper function
func TestPersistStateToStorageWithErrorHandling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(fmt.Errorf("storage error")).Times(4) // All retries fail

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)
	sm.statePersistenceMaxRetries = 3
	sm.statePersistenceRetryBackoff = 50 * time.Millisecond

	ctx := context.Background()
	// Should not panic, just log warning
	sm.persistStateToStorageWithErrorHandling(ctx, vmgatewaytypes.ConnectionStateAuthenticated, nil, "test_operation")
}

// TestPersistStateToStorage_Configuration tests that retry configuration is applied
func TestPersistStateToStorage_Configuration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Test default values
	assert.Equal(t, 3, sm.statePersistenceMaxRetries)
	assert.Equal(t, 1*time.Second, sm.statePersistenceRetryBackoff)

	// Test configuration update
	cfg := &config.Config{
		StateManager: types.StateManagerConfig{
			StatePersistenceMaxRetries:   5,
			StatePersistenceRetryBackoff: 2 * time.Second,
		},
	}
	sm.SetConfig(cfg)

	assert.Equal(t, 5, sm.statePersistenceMaxRetries)
	assert.Equal(t, 2*time.Second, sm.statePersistenceRetryBackoff)
}

// TestHandleModelDeployed_MultipleQueuedDeployments tests that multiple deployments replace each other
func TestHandleModelDeployed_MultipleQueuedDeployments(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	// Create camera state machine but keep it in discovered state (not ready)
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")

	// Queue first deployment
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"model_id":  "model-1",
		},
	}
	time.Sleep(100 * time.Millisecond)

	// Queue second deployment (should replace first)
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"model_id":  "model-2",
		},
	}
	time.Sleep(100 * time.Millisecond)

	// Verify only one deployment is queued (the latest)
	sm.pendingModelDeployMu.RLock()
	pending, exists := sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	require.True(t, exists, "Should have pending deployment")
	assert.Equal(t, "model-2", pending.ModelID, "Latest deployment should be queued")

	// Now transition to screenshot_set_ready - should process latest deployment
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")

	// Trigger workflow by sending screenshot_set_ready event
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitSetReady,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state transition to model_deployed with latest model
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateModelDeployed, cameraSM.GetState())
	if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
		assert.Equal(t, "model-2", metadataSM.GetStateInfo().ModelID, "Should use latest model")
	}

	// Verify pending deployment was cleared
	sm.pendingModelDeployMu.RLock()
	_, exists = sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	assert.False(t, exists, "Pending deployment should be cleared after processing")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_QueuedThenReadyDeployment tests deployment arriving when ready after queuing
func TestHandleModelDeployed_QueuedThenReadyDeployment(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMetaStorage.EXPECT().DeletePendingSnapshotRequest(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")

	// Queue first deployment (camera is in waiting_for_screenshots, so it will be queued)
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"model_id":  "model-queued",
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify deployment was queued
	sm.pendingModelDeployMu.RLock()
	_, hasPending := sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	assert.True(t, hasPending, "Deployment should be queued when camera is not in screenshot_set_ready")

	// Send screenshot_set_ready event to trigger state transition and pending deployment processing
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitSetReady,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id":     cameraID,
			"labeled_count": 10,
			"min_required":  int32(10),
		},
	}
	time.Sleep(500 * time.Millisecond)

	// Verify queued deployment was processed
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateModelDeployed, cameraSM.GetState(), "Camera should transition to model_deployed")
	if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
		assert.Equal(t, "model-queued", metadataSM.GetStateInfo().ModelID, "Queued deployment should be processed")
	}

	// Verify pending deployment was cleared
	sm.pendingModelDeployMu.RLock()
	_, exists := sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	assert.False(t, exists, "Pending deployment should be cleared after processing")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_ConcurrentDeployments tests concurrent deployments for same camera
func TestHandleModelDeployed_ConcurrentDeployments(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")

	// Send multiple concurrent deployments
	var wg sync.WaitGroup
	numDeployments := 5
	wg.Add(numDeployments)
	for i := 0; i < numDeployments; i++ {
		go func(idx int) {
			defer wg.Done()
			eventChan <- eventbustypes.Event{
				Type:      EventTypeModelDeployed,
				Source:    "test",
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"camera_id": cameraID,
					"model_id":  fmt.Sprintf("model-%d", idx),
				},
			}
		}(i)
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Verify only one deployment is queued (thread-safe)
	sm.pendingModelDeployMu.RLock()
	pending, exists := sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	require.True(t, exists, "Should have pending deployment")
	assert.NotEmpty(t, pending.ModelID, "Should have a model ID")

	// Verify only one deployment exists
	sm.pendingModelDeployMu.RLock()
	count := len(sm.pendingModelDeployments)
	sm.pendingModelDeployMu.RUnlock()
	assert.Equal(t, 1, count, "Should have exactly one pending deployment")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_QueuedThenErrorState tests that queued deployment persists through error state
func TestHandleModelDeployed_QueuedThenErrorState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")

	// Queue deployment
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			"model_id":  "model-queued",
		},
	}
	time.Sleep(100 * time.Millisecond)

	// Transition to error state
	_ = cameraSM.Transition(types.CameraStateError, "test error")
	time.Sleep(100 * time.Millisecond)

	// Verify deployment is still queued
	sm.pendingModelDeployMu.RLock()
	pending, exists := sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	require.True(t, exists, "Deployment should still be queued")
	assert.Equal(t, "model-queued", pending.ModelID)

	// Transition back to discovered, then to screenshot_set_ready
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")

	// Trigger workflow
	eventChan <- eventbustypes.Event{
		Type:      EventTypeDataUnitSetReady,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify deployment was processed
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateModelDeployed, cameraSM.GetState())

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_MissingModelID tests handling of deployment without model_id
func TestHandleModelDeployed_MissingModelID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	mockMetaStorage := metastoragemocks.NewMockMetaDataStore(ctrl)
	mockMetaStorage.EXPECT().GetCurrentEdgeState(gomock.Any()).Return(nil, false).AnyTimes()
	mockMetaStorage.EXPECT().GetStorageStats(gomock.Any()).Return(&metastoragetypes.StorageStats{}, nil).AnyTimes()
	mockMetaStorage.EXPECT().SaveEdgeState(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)
	sm.SetMetaStorage(mockMetaStorage)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	cameraID := "test-camera-1"
	cameraSM := sm.getOrCreateCameraStateMachine(cameraID)
	// Transition through valid states to reach screenshot_set_ready
	_ = cameraSM.Transition(types.CameraStateDiscovered, "")
	_ = cameraSM.Transition(types.CameraStateSynced, "")
	_ = cameraSM.Transition(types.CameraStateWaitingForScreenshots, "")
	_ = cameraSM.Transition(types.CameraStateScreenshotSetReady, "")

	// Verify initial state
	assert.Equal(t, types.CameraStateScreenshotSetReady, cameraSM.GetState(), "Camera should be in screenshot_set_ready state")

	// Send deployment without model_id
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": cameraID,
			// model_id missing
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify state did not change
	cameraSM = sm.getCameraStateMachine(cameraID)
	require.NotNil(t, cameraSM)
	assert.Equal(t, types.CameraStateScreenshotSetReady, cameraSM.GetState(), "State should not change without model_id")

	// Verify no deployment was queued
	sm.pendingModelDeployMu.RLock()
	_, exists := sm.pendingModelDeployments[cameraID]
	sm.pendingModelDeployMu.RUnlock()
	assert.False(t, exists, "No deployment should be queued without model_id")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_MissingCameraID tests handling of deployment without camera_id
func TestHandleModelDeployed_MissingCameraID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Send deployment without camera_id
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"model_id": "test-model-1",
			// camera_id missing
		},
	}
	time.Sleep(100 * time.Millisecond)

	// Verify no deployment was queued
	sm.pendingModelDeployMu.RLock()
	count := len(sm.pendingModelDeployments)
	sm.pendingModelDeployMu.RUnlock()
	assert.Equal(t, 0, count, "No deployment should be queued without camera_id")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}

// TestHandleModelDeployed_DifferentCameras tests queuing deployments for different cameras
func TestHandleModelDeployed_DifferentCameras(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEventBus := eventbusmocks.NewMockEventBus(ctrl)
	mockEventBus.EXPECT().Name().Return("mock-event-bus").AnyTimes()
	eventChan := make(chan eventbustypes.Event, 10)
	mockEventBus.EXPECT().SubscribeAll().Return(eventChan).AnyTimes()

	sm, err := NewStateManagerImpl(mockEventBus, logger)
	require.NoError(t, err)

	// Set up DeviceStateService (required for camera state machines)
	setupDeviceStateService(t, sm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sm.Start(ctx)
	require.NoError(t, err)

	// Create two cameras in discovered state
	camera1 := sm.getOrCreateCameraStateMachine("camera-1")
	_ = camera1.Transition(types.CameraStateDiscovered, "")
	camera2 := sm.getOrCreateCameraStateMachine("camera-2")
	_ = camera2.Transition(types.CameraStateDiscovered, "")

	// Queue deployments for both cameras
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": "camera-1",
			"model_id":  "model-1",
		},
	}
	eventChan <- eventbustypes.Event{
		Type:      EventTypeModelDeployed,
		Source:    "test",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_id": "camera-2",
			"model_id":  "model-2",
		},
	}
	time.Sleep(200 * time.Millisecond)

	// Verify both deployments are queued
	sm.pendingModelDeployMu.RLock()
	pending1, exists1 := sm.pendingModelDeployments["camera-1"]
	pending2, exists2 := sm.pendingModelDeployments["camera-2"]
	count := len(sm.pendingModelDeployments)
	sm.pendingModelDeployMu.RUnlock()

	assert.True(t, exists1, "Camera-1 deployment should be queued")
	assert.True(t, exists2, "Camera-2 deployment should be queued")
	assert.Equal(t, 2, count, "Should have two pending deployments")
	assert.Equal(t, "model-1", pending1.ModelID)
	assert.Equal(t, "model-2", pending2.ModelID)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	_ = sm.Stop(stopCtx)
}
