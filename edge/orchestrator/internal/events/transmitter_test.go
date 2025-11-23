package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestTransmitter(t *testing.T) (*Transmitter, *Queue, *Storage, *state.Manager) {
	stateMgr := setupTestManager(t)

	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	queue := NewQueue(QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)

	storage := NewStorage(stateMgr, log)

	transmitter := NewTransmitter(TransmitterConfig{
		Queue:               queue,
		Storage:             storage,
		BatchSize:           5,
		TransmissionInterval: 1 * time.Second,
		MaxRetries:          3,
		RetryDelay:          100 * time.Millisecond,
	}, log)

	return transmitter, queue, storage, stateMgr
}

func TestTransmitter_StartStop(t *testing.T) {
	transmitter, _, _, _ := setupTestTransmitter(t)

	ctx := context.Background()

	// Start
	err := transmitter.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transmitter: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop
	err = transmitter.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop transmitter: %v", err)
	}
}

func TestTransmitter_ProcessQueue_Empty(t *testing.T) {
	transmitter, _, _, _ := setupTestTransmitter(t)

	ctx := context.Background()

	// Process empty queue should not error
	err := transmitter.TransmitNow(ctx)
	if err != nil {
		t.Fatalf("Failed to process empty queue: %v", err)
	}
}

func TestTransmitter_ProcessQueue_WithEvents(t *testing.T) {
	transmitter, queue, _, stateMgr := setupTestTransmitter(t)

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue event
	event := NewEvent()
	event.CameraID = "camera-1"
	event.EventType = EventTypePersonDetected
	event.Timestamp = time.Now()
	event.Confidence = 0.85

	err := queue.Enqueue(ctx, event, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue event: %v", err)
	}

	// Process queue (no callback configured, should succeed)
	err = transmitter.TransmitNow(ctx)
	if err != nil {
		t.Fatalf("Failed to process queue: %v", err)
	}

	// Event should be marked as transmitted
	pending, err := queue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if pending != 0 {
		t.Errorf("Expected queue to be empty after transmission, got %d events", pending)
	}
}

func TestTransmitter_RetryLogic(t *testing.T) {
	transmitter, queue, _, stateMgr := setupTestTransmitter(t)

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue event
	event := NewEvent()
	event.CameraID = "camera-1"
	event.EventType = EventTypePersonDetected
	event.Timestamp = time.Now()
	event.Confidence = 0.85

	err := queue.Enqueue(ctx, event, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue event: %v", err)
	}

	// Configure transmission callback that fails
	transmissionAttempts := 0
	transmitter.config.OnTransmit = func(ctx context.Context, events []*Event) error {
		transmissionAttempts++
		if transmissionAttempts < 2 {
			return errors.New("transmission failed")
		}
		return nil // Succeed on second attempt
	}

	// First attempt should fail
	err = transmitter.TransmitNow(ctx)
	if err == nil {
		t.Error("Expected transmission to fail, got nil")
	}

	// Check retry count
	retryCount, err := queue.GetRetryCount(ctx, event.ID)
	if err != nil {
		t.Fatalf("Failed to get retry count: %v", err)
	}
	if retryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", retryCount)
	}

	// Second attempt should succeed
	err = transmitter.TransmitNow(ctx)
	if err != nil {
		t.Fatalf("Expected transmission to succeed, got error: %v", err)
	}

	// Event should be marked as transmitted
	pending, err := queue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if pending != 0 {
		t.Errorf("Expected queue to be empty after successful transmission, got %d events", pending)
	}
}

func TestTransmitter_MaxRetries(t *testing.T) {
	transmitter, queue, _, stateMgr := setupTestTransmitter(t)
	transmitter.config.MaxRetries = 2 // Set low max retries

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue event
	event := NewEvent()
	event.CameraID = "camera-1"
	event.EventType = EventTypePersonDetected
	event.Timestamp = time.Now()
	event.Confidence = 0.85

	err := queue.Enqueue(ctx, event, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue event: %v", err)
	}

	// Configure transmission callback that always fails
	transmitter.config.OnTransmit = func(ctx context.Context, events []*Event) error {
		return errors.New("transmission failed")
	}

	// Attempt transmission multiple times to exceed max retries
	for i := 0; i < 3; i++ {
		transmitter.TransmitNow(ctx)
	}

	// Event should be removed from queue (marked as transmitted to prevent infinite retries)
	pending, err := queue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if pending != 0 {
		t.Errorf("Expected queue to be empty after max retries, got %d events", pending)
	}
}

func TestTransmitter_BatchProcessing(t *testing.T) {
	transmitter, queue, _, stateMgr := setupTestTransmitter(t)
	transmitter.config.BatchSize = 3

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue multiple events
	for i := 0; i < 5; i++ {
		event := NewEvent()
		event.CameraID = "camera-1"
		event.EventType = EventTypePersonDetected
		event.Timestamp = time.Now()
		queue.Enqueue(ctx, event, 0)
	}

	// Track transmitted events
	transmittedCount := 0
	transmitter.config.OnTransmit = func(ctx context.Context, events []*Event) error {
		transmittedCount += len(events)
		return nil
	}

	// Process queue (will process in batches of 3)
	// First call processes 3 events
	err := transmitter.TransmitNow(ctx)
	if err != nil {
		t.Fatalf("Failed to process queue: %v", err)
	}

	// Second call processes remaining 2 events
	err = transmitter.TransmitNow(ctx)
	if err != nil {
		t.Fatalf("Failed to process queue: %v", err)
	}

	// Should process all 5 events in batches
	if transmittedCount != 5 {
		t.Errorf("Expected 5 events transmitted, got %d", transmittedCount)
	}
}

func TestTransmitter_RecoverQueue(t *testing.T) {
	transmitter, queue, _, stateMgr := setupTestTransmitter(t)

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue some events
	for i := 0; i < 3; i++ {
		event := NewEvent()
		event.CameraID = "camera-1"
		event.EventType = EventTypePersonDetected
		event.Timestamp = time.Now()
		queue.Enqueue(ctx, event, 0)
	}

	// Recover queue
	err := transmitter.RecoverQueue(ctx)
	if err != nil {
		t.Fatalf("Failed to recover queue: %v", err)
	}

	// Check that events are still in queue
	size, err := queue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if size != 3 {
		t.Errorf("Expected 3 events in queue after recovery, got %d", size)
	}
}

