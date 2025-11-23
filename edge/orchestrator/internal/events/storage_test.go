package events

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestStorage(t *testing.T) (*Storage, *state.Manager) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	// Create test state manager using helper
	stateMgr := setupTestManager(t)

	storage := NewStorage(stateMgr, log)
	return storage, stateMgr
}

func TestStorage_SaveEvent(t *testing.T) {
	storage, stateMgr := setupTestStorage(t)

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

	err = storage.SaveEvent(ctx, event)

	if err != nil {
		t.Fatalf("Failed to save event: %v", err)
	}
}

func TestStorage_GetPendingEvents(t *testing.T) {
	storage, stateMgr := setupTestStorage(t)

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

	// Save some events
	for i := 0; i < 3; i++ {
		event := NewEvent()
		event.CameraID = "camera-1"
		event.EventType = EventTypePersonDetected
		event.Timestamp = time.Now()
		event.Confidence = 0.85

		err := storage.SaveEvent(ctx, event)
		if err != nil {
			t.Fatalf("Failed to save event: %v", err)
		}
	}

	// Get pending events
	pending, err := storage.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to get pending events: %v", err)
	}

	if len(pending) != 3 {
		t.Errorf("Expected 3 pending events, got %d", len(pending))
	}
}

func TestStorage_ListEvents(t *testing.T) {
	storage, stateMgr := setupTestStorage(t)

	ctx := context.Background()

	// Create cameras first (required for foreign key)
	camera1 := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera 1",
		RTSPURL: "rtsp://test1",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera1)

	camera2 := state.CameraState{
		ID:      "camera-2",
		Name:    "Test Camera 2",
		RTSPURL: "rtsp://test2",
		Enabled: true,
	}
	stateMgr.SaveCamera(ctx, camera2)

	// Save events with different types and cameras
	event1 := NewEvent()
	event1.CameraID = "camera-1"
	event1.EventType = EventTypePersonDetected
	event1.Timestamp = time.Now()
	event1.Confidence = 0.85
	storage.SaveEvent(ctx, event1)

	event2 := NewEvent()
	event2.CameraID = "camera-2"
	event2.EventType = EventTypeVehicleDetected
	event2.Timestamp = time.Now()
	event2.Confidence = 0.9
	storage.SaveEvent(ctx, event2)

	// Filter by camera
	events, err := storage.GetEventsByCamera(ctx, "camera-1", 10)
	if err != nil {
		t.Fatalf("Failed to get events by camera: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event for camera-1, got %d", len(events))
	}

	// Filter by type
	events, err = storage.GetEventsByType(ctx, EventTypePersonDetected, 10)
	if err != nil {
		t.Fatalf("Failed to get events by type: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 person event, got %d", len(events))
	}
}

func TestStorage_MarkEventTransmitted(t *testing.T) {
	storage, stateMgr := setupTestStorage(t)

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

	err = storage.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("Failed to save event: %v", err)
	}

	// Mark as transmitted
	err = storage.MarkEventTransmitted(ctx, event.ID)
	if err != nil {
		t.Fatalf("Failed to mark event as transmitted: %v", err)
	}

	// Get pending events - should not include the transmitted one
	pending, err := storage.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to get pending events: %v", err)
	}

	for _, e := range pending {
		if e.ID == event.ID {
			t.Error("Transmitted event should not be in pending list")
		}
	}
}

func TestStorage_GetEventsByTimeRange(t *testing.T) {
	storage, stateMgr := setupTestStorage(t)

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

	now := time.Now()

	// Save events at different times
	event1 := NewEvent()
	event1.CameraID = "camera-1"
	event1.EventType = EventTypePersonDetected
	event1.Timestamp = now.Add(-2 * time.Hour)
	event1.Confidence = 0.85
	storage.SaveEvent(ctx, event1)

	event2 := NewEvent()
	event2.CameraID = "camera-1"
	event2.EventType = EventTypePersonDetected
	event2.Timestamp = now.Add(-1 * time.Hour)
	event2.Confidence = 0.85
	storage.SaveEvent(ctx, event2)

	// Get events in time range
	startTime := now.Add(-3 * time.Hour)
	endTime := now.Add(-30 * time.Minute)

	events, err := storage.GetEventsByTimeRange(ctx, startTime, endTime, 10)
	if err != nil {
		t.Fatalf("Failed to get events by time range: %v", err)
	}

	// Should get both events
	if len(events) != 2 {
		t.Errorf("Expected 2 events in time range, got %d", len(events))
	}
}

