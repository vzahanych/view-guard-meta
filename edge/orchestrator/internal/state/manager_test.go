package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)


func TestNewManager(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.GetDB() == nil {
		t.Error("Database should be initialized")
	}
}

func TestManager_SaveSystemState(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	err := mgr.SaveSystemState(ctx, "test_key", "test_value")
	if err != nil {
		t.Fatalf("SaveSystemState failed: %v", err)
	}

	value, err := mgr.GetSystemState(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetSystemState failed: %v", err)
	}

	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}
}

func TestManager_GetSystemState_NotFound(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	value, err := mgr.GetSystemState(ctx, "nonexistent_key")
	if err != nil {
		t.Fatalf("GetSystemState failed: %v", err)
	}

	if value != "" {
		t.Errorf("Expected empty string for nonexistent key, got '%s'", value)
	}
}

func TestManager_SaveSystemState_Update(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	err := mgr.SaveSystemState(ctx, "test_key", "initial_value")
	if err != nil {
		t.Fatalf("SaveSystemState failed: %v", err)
	}

	err = mgr.SaveSystemState(ctx, "test_key", "updated_value")
	if err != nil {
		t.Fatalf("SaveSystemState update failed: %v", err)
	}

	value, err := mgr.GetSystemState(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetSystemState failed: %v", err)
	}

	if value != "updated_value" {
		t.Errorf("Expected 'updated_value', got '%s'", value)
	}
}

func TestManager_RecoverState_Empty(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	recovered, err := mgr.RecoverState(ctx)
	if err != nil {
		t.Fatalf("RecoverState failed: %v", err)
	}

	if recovered == nil {
		t.Fatal("RecoveredState should not be nil")
	}

	if len(recovered.Cameras) != 0 {
		t.Errorf("Expected 0 cameras, got %d", len(recovered.Cameras))
	}

	if len(recovered.Events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(recovered.Events))
	}

	if len(recovered.SystemState) != 0 {
		t.Errorf("Expected 0 system state entries, got %d", len(recovered.SystemState))
	}
}

func TestManager_RecoverState_WithData(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save some system state
	err := mgr.SaveSystemState(ctx, "key1", "value1")
	if err != nil {
		t.Fatalf("SaveSystemState failed: %v", err)
	}

	err = mgr.SaveSystemState(ctx, "key2", "value2")
	if err != nil {
		t.Fatalf("SaveSystemState failed: %v", err)
	}

	// Save a camera
	now := time.Now()
	camera := CameraState{
		ID:       "cam-1",
		Name:     "Test Camera",
		RTSPURL:  "rtsp://test",
		Enabled:  true,
		LastSeen: &now,
	}
	err = mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Save an event
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

	// Close and recreate manager to test recovery
	dbPath := mgr.db.dbPath
	dataDir := filepath.Dir(filepath.Dir(dbPath)) // Go up from db/edge.db to data dir
	mgr.Close()

	// Recreate manager with same database
	cfg := &config.Config{}
	cfg.Edge.Orchestrator.DataDir = dataDir
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr2, err := NewManager(cfg, log)
	if err != nil {
		t.Fatalf("Failed to recreate manager: %v", err)
	}
	defer mgr2.Close()

	// Recover state
	recovered, err := mgr2.RecoverState(ctx)
	if err != nil {
		t.Fatalf("RecoverState failed: %v", err)
	}

	// Verify cameras
	if len(recovered.Cameras) != 1 {
		t.Fatalf("Expected 1 camera, got %d", len(recovered.Cameras))
	}
	if recovered.Cameras[0].ID != "cam-1" {
		t.Errorf("Expected camera ID 'cam-1', got '%s'", recovered.Cameras[0].ID)
	}

	// Verify events
	if len(recovered.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(recovered.Events))
	}
	if recovered.Events[0].ID != "event-1" {
		t.Errorf("Expected event ID 'event-1', got '%s'", recovered.Events[0].ID)
	}

	// Verify system state
	if len(recovered.SystemState) != 2 {
		t.Fatalf("Expected 2 system state entries, got %d", len(recovered.SystemState))
	}
	if recovered.SystemState["key1"] != "value1" {
		t.Errorf("Expected system state key1='value1', got '%s'", recovered.SystemState["key1"])
	}
	if recovered.SystemState["key2"] != "value2" {
		t.Errorf("Expected system state key2='value2', got '%s'", recovered.SystemState["key2"])
	}
}

func TestManager_SyncState(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// SyncState is a placeholder, should not error
	err := mgr.SyncState(ctx)
	if err != nil {
		t.Errorf("SyncState should not error (placeholder): %v", err)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	done := make(chan bool, 10)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := "key_" + string(rune(idx))
			err := mgr.SaveSystemState(ctx, key, "value")
			if err != nil {
				t.Errorf("Concurrent SaveSystemState failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all values were saved
	for i := 0; i < 10; i++ {
		key := "key_" + string(rune(i))
		value, err := mgr.GetSystemState(ctx, key)
		if err != nil {
			t.Errorf("GetSystemState failed for %s: %v", key, err)
		}
		if value != "value" {
			t.Errorf("Expected 'value' for %s, got '%s'", key, value)
		}
	}
}

