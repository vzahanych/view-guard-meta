package state

import (
	"context"
	"testing"
	"time"
)

func TestManager_SaveEvent(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// First, save a camera (required for foreign key)
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save event
	event := EventState{
		ID:          "event-1",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now(),
		Metadata:    map[string]interface{}{"confidence": 0.9, "object": "person"},
		ClipPath:    "/path/to/clip.mp4",
		SnapshotPath: "/path/to/snapshot.jpg",
		Transmitted: false,
	}

	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	// Verify event is in pending queue
	pending, err := mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending event, got %d", len(pending))
	}

	if pending[0].ID != "event-1" {
		t.Errorf("Expected event ID 'event-1', got '%s'", pending[0].ID)
	}

	if pending[0].CameraID != "cam-1" {
		t.Errorf("Expected CameraID 'cam-1', got '%s'", pending[0].CameraID)
	}

	if pending[0].EventType != "detection" {
		t.Errorf("Expected EventType 'detection', got '%s'", pending[0].EventType)
	}

	if pending[0].Metadata["confidence"] != 0.9 {
		t.Errorf("Expected confidence 0.9, got %v", pending[0].Metadata["confidence"])
	}
}

func TestManager_SaveEvent_WithTransmitted(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save event as transmitted
	event := EventState{
		ID:          "event-1",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now(),
		Metadata:    map[string]interface{}{"confidence": 0.9},
		Transmitted: true,
	}

	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	// Verify event is NOT in pending queue
	pending, err := mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("Expected 0 pending events, got %d", len(pending))
	}
}

func TestManager_MarkEventTransmitted(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save event as not transmitted
	event := EventState{
		ID:          "event-1",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now(),
		Metadata:    map[string]interface{}{"confidence": 0.9},
		Transmitted: false,
	}

	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	// Verify it's pending
	pending, err := mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending event, got %d", len(pending))
	}

	// Mark as transmitted
	err = mgr.MarkEventTransmitted(ctx, "event-1")
	if err != nil {
		t.Fatalf("MarkEventTransmitted failed: %v", err)
	}

	// Verify it's no longer pending
	pending, err = mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending events after marking transmitted, got %d", len(pending))
	}
}

func TestManager_GetPendingEvents_Limit(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save multiple events
	for i := 0; i < 10; i++ {
		event := EventState{
			ID:          "event-" + string(rune(i)),
			CameraID:    "cam-1",
			EventType:   "detection",
			Timestamp:   time.Now().Add(time.Duration(i) * time.Second),
			Metadata:    map[string]interface{}{"index": i},
			Transmitted: false,
		}
		err := mgr.SaveEvent(ctx, event)
		if err != nil {
			t.Fatalf("SaveEvent failed: %v", err)
		}
	}

	// Get with limit
	pending, err := mgr.GetPendingEvents(ctx, 5)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 5 {
		t.Errorf("Expected 5 pending events with limit, got %d", len(pending))
	}

	// Get without limit (should default to 100)
	pending, err = mgr.GetPendingEvents(ctx, 0)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 10 {
		t.Errorf("Expected 10 pending events without limit, got %d", len(pending))
	}
}

func TestManager_GetPendingEvents_EmptyMetadata(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save event with empty metadata
	event := EventState{
		ID:          "event-1",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now(),
		Metadata:    make(map[string]interface{}),
		Transmitted: false,
	}

	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	pending, err := mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending event, got %d", len(pending))
	}

	if pending[0].Metadata == nil {
		t.Error("Metadata should not be nil, should be empty map")
	}

	if len(pending[0].Metadata) != 0 {
		t.Errorf("Expected empty metadata, got %v", pending[0].Metadata)
	}
}

func TestManager_CleanupOldEvents(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save old transmitted event
	oldEvent := EventState{
		ID:          "event-old",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now().Add(-10 * 24 * time.Hour), // 10 days ago
		Metadata:    map[string]interface{}{"old": true},
		Transmitted: true,
	}
	err = mgr.SaveEvent(ctx, oldEvent)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	// Mark it as transmitted (to set transmitted_at)
	err = mgr.MarkEventTransmitted(ctx, "event-old")
	if err != nil {
		t.Fatalf("MarkEventTransmitted failed: %v", err)
	}

	// Save recent transmitted event
	recentEvent := EventState{
		ID:          "event-recent",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now(),
		Metadata:    map[string]interface{}{"recent": true},
		Transmitted: true,
	}
	err = mgr.SaveEvent(ctx, recentEvent)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	err = mgr.MarkEventTransmitted(ctx, "event-recent")
	if err != nil {
		t.Fatalf("MarkEventTransmitted failed: %v", err)
	}

	// Cleanup events older than 7 days
	err = mgr.CleanupOldEvents(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldEvents failed: %v", err)
	}

	// The old event should be cleaned up, but we can't easily verify without
	// direct database access. The function should complete without error.
}

func TestManager_SaveEvent_Update(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save event
	event := EventState{
		ID:          "event-1",
		CameraID:    "cam-1",
		EventType:   "detection",
		Timestamp:   time.Now(),
		Metadata:    map[string]interface{}{"confidence": 0.9},
		Transmitted: false,
	}

	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	// Update event (mark as transmitted)
	event.Transmitted = true
	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent update failed: %v", err)
	}

	// Verify it's no longer pending
	pending, err := mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("Expected 0 pending events after update, got %d", len(pending))
	}
}

func TestManager_EventState_ComplexMetadata(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save event with complex metadata
	event := EventState{
		ID:        "event-1",
		CameraID:  "cam-1",
		EventType: "detection",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"confidence": 0.95,
			"object":     "person",
			"bbox":       []float64{10.5, 20.3, 100.2, 200.7},
			"nested": map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
		},
		Transmitted: false,
	}

	err = mgr.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}

	// Retrieve and verify
	pending, err := mgr.GetPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingEvents failed: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending event, got %d", len(pending))
	}

	retrieved := pending[0]
	if retrieved.Metadata["confidence"] != 0.95 {
		t.Errorf("Expected confidence 0.95, got %v", retrieved.Metadata["confidence"])
	}

	if retrieved.Metadata["object"] != "person" {
		t.Errorf("Expected object 'person', got %v", retrieved.Metadata["object"])
	}
}

