package events

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestQueue(t *testing.T) (*Queue, *state.Manager) {
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

	return queue, stateMgr
}

func TestQueue_Enqueue(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)

	ctx := context.Background()

	// Create camera first (required for foreign key)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("Failed to save camera: %v", err)
	}

	event := NewEvent()
	event.CameraID = "camera-1"
	event.EventType = EventTypePersonDetected
	event.Timestamp = time.Now()
	event.Confidence = 0.85

	err = queue.Enqueue(ctx, event, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue event: %v", err)
	}

	// Check queue size
	size, err := queue.Size(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue size: %v", err)
	}
	if size != 1 {
		t.Errorf("Expected queue size 1, got %d", size)
	}
}

func TestQueue_Dequeue(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)

	ctx := context.Background()

	// Create camera first
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

	// Dequeue event
	dequeued, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Failed to dequeue event: %v", err)
	}

	if dequeued == nil {
		t.Fatal("Expected event, got nil")
	}

	if dequeued.ID != event.ID {
		t.Errorf("Expected event ID %s, got %s", event.ID, dequeued.ID)
	}
}

func TestQueue_Priority(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue events with different priorities
	event1 := NewEvent()
	event1.CameraID = "camera-1"
	event1.EventType = EventTypePersonDetected
	event1.Timestamp = time.Now()
	queue.Enqueue(ctx, event1, 1) // Low priority

	event2 := NewEvent()
	event2.CameraID = "camera-1"
	event2.EventType = EventTypePersonDetected
	event2.Timestamp = time.Now()
	queue.Enqueue(ctx, event2, 10) // High priority

	// Dequeue should return high priority event first
	dequeued, err := queue.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Failed to dequeue event: %v", err)
	}

	if dequeued.ID != event2.ID {
		t.Errorf("Expected high priority event %s, got %s", event2.ID, dequeued.ID)
	}
}

func TestQueue_SizeLimit(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)
	queue.maxSize = 2 // Set small limit

	ctx := context.Background()

	// Create camera
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera)

	// Enqueue 2 events (should succeed)
	event1 := NewEvent()
	event1.CameraID = "camera-1"
	event1.EventType = EventTypePersonDetected
	event1.Timestamp = time.Now()
	err := queue.Enqueue(ctx, event1, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue first event: %v", err)
	}

	event2 := NewEvent()
	event2.CameraID = "camera-1"
	event2.EventType = EventTypePersonDetected
	event2.Timestamp = time.Now()
	err = queue.Enqueue(ctx, event2, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue second event: %v", err)
	}

	// Third event should fail (queue full)
	event3 := NewEvent()
	event3.CameraID = "camera-1"
	event3.EventType = EventTypePersonDetected
	event3.Timestamp = time.Now()
	err = queue.Enqueue(ctx, event3, 0)
	if err == nil {
		t.Error("Expected error when queue is full, got nil")
	}
}

func TestQueue_BatchDequeue(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)

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

	// Batch dequeue
	events, err := queue.BatchDequeue(ctx, 3)
	if err != nil {
		t.Fatalf("Failed to batch dequeue: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
}

func TestQueue_RetryCount(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)

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
	queue.Enqueue(ctx, event, 0)

	// Increment retry count
	err := queue.IncrementRetryCount(ctx, event.ID)
	if err != nil {
		t.Fatalf("Failed to increment retry count: %v", err)
	}

	// Get retry count
	count, err := queue.GetRetryCount(ctx, event.ID)
	if err != nil {
		t.Fatalf("Failed to get retry count: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected retry count 1, got %d", count)
	}
}

func TestQueue_GetQueueStats(t *testing.T) {
	queue, stateMgr := setupTestQueue(t)

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
	queue.Enqueue(ctx, event, 0)

	// Get stats
	stats, err := queue.GetQueueStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue stats: %v", err)
	}

	if stats.Size != 1 {
		t.Errorf("Expected queue size 1, got %d", stats.Size)
	}

	if stats.MaxSize != 100 {
		t.Errorf("Expected max size 100, got %d", stats.MaxSize)
	}
}

