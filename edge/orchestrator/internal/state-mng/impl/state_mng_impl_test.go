package impl

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/mocks"
	eventbusmocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/mocks"
	eventbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
)

// getGoroutineCount returns the current number of goroutines
func getGoroutineCount() int {
	return runtime.NumGoroutine()
}

// waitForGoroutines waits for goroutines to stabilize, allowing time for cleanup
func waitForGoroutines(t *testing.T, initialCount int, timeout time.Duration) int {
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
			_ = sm.startFrameProcessingForCamera(ctx, cameraID)
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
