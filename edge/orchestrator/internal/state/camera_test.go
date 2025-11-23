package state

import (
	"context"
	"testing"
	"time"
)

func TestManager_SaveCamera(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()
	now := time.Now()

	camera := CameraState{
		ID:       "cam-1",
		Name:     "Test Camera",
		RTSPURL:  "rtsp://test:554/stream",
		Enabled:  true,
		LastSeen: &now,
	}

	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	retrieved, err := mgr.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetCamera returned nil")
	}

	if retrieved.ID != "cam-1" {
		t.Errorf("Expected ID 'cam-1', got '%s'", retrieved.ID)
	}

	if retrieved.Name != "Test Camera" {
		t.Errorf("Expected Name 'Test Camera', got '%s'", retrieved.Name)
	}

	if retrieved.RTSPURL != "rtsp://test:554/stream" {
		t.Errorf("Expected RTSPURL 'rtsp://test:554/stream', got '%s'", retrieved.RTSPURL)
	}

	if !retrieved.Enabled {
		t.Error("Expected Enabled=true")
	}

	if retrieved.LastSeen == nil {
		t.Error("Expected LastSeen to be set")
	}
}

func TestManager_SaveCamera_Update(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()
	now := time.Now()

	// Create initial camera
	camera := CameraState{
		ID:       "cam-1",
		Name:     "Original Name",
		RTSPURL:  "rtsp://original",
		Enabled:  true,
		LastSeen: &now,
	}

	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	// Update camera
	updatedNow := time.Now()
	updatedCamera := CameraState{
		ID:       "cam-1",
		Name:     "Updated Name",
		RTSPURL:  "rtsp://updated",
		Enabled:  false,
		LastSeen: &updatedNow,
	}

	err = mgr.SaveCamera(ctx, updatedCamera)
	if err != nil {
		t.Fatalf("SaveCamera update failed: %v", err)
	}

	retrieved, err := mgr.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected Name 'Updated Name', got '%s'", retrieved.Name)
	}

	if retrieved.RTSPURL != "rtsp://updated" {
		t.Errorf("Expected RTSPURL 'rtsp://updated', got '%s'", retrieved.RTSPURL)
	}

	if retrieved.Enabled {
		t.Error("Expected Enabled=false")
	}
}

func TestManager_GetCamera_NotFound(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	camera, err := mgr.GetCamera(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetCamera should not error for nonexistent camera: %v", err)
	}

	if camera != nil {
		t.Error("GetCamera should return nil for nonexistent camera")
	}
}

func TestManager_ListCameras(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save multiple cameras
	cameras := []CameraState{
		{ID: "cam-1", Name: "Camera 1", RTSPURL: "rtsp://cam1", Enabled: true},
		{ID: "cam-2", Name: "Camera 2", RTSPURL: "rtsp://cam2", Enabled: true},
		{ID: "cam-3", Name: "Camera 3", RTSPURL: "rtsp://cam3", Enabled: false},
	}

	for _, cam := range cameras {
		err := mgr.SaveCamera(ctx, cam)
		if err != nil {
			t.Fatalf("SaveCamera failed: %v", err)
		}
	}

	// List all cameras
	allCameras, err := mgr.ListCameras(ctx, false)
	if err != nil {
		t.Fatalf("ListCameras failed: %v", err)
	}

	if len(allCameras) != 3 {
		t.Errorf("Expected 3 cameras, got %d", len(allCameras))
	}

	// List only enabled cameras
	enabledCameras, err := mgr.ListCameras(ctx, true)
	if err != nil {
		t.Fatalf("ListCameras failed: %v", err)
	}

	if len(enabledCameras) != 2 {
		t.Errorf("Expected 2 enabled cameras, got %d", len(enabledCameras))
	}

	// Verify ordering (should be sorted by name)
	if enabledCameras[0].Name != "Camera 1" {
		t.Errorf("Expected first camera to be 'Camera 1', got '%s'", enabledCameras[0].Name)
	}

	if enabledCameras[1].Name != "Camera 2" {
		t.Errorf("Expected second camera to be 'Camera 2', got '%s'", enabledCameras[1].Name)
	}
}

func TestManager_UpdateCameraLastSeen(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera without LastSeen
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

	// Update last seen
	err = mgr.UpdateCameraLastSeen(ctx, "cam-1")
	if err != nil {
		t.Fatalf("UpdateCameraLastSeen failed: %v", err)
	}

	// Verify update
	retrieved, err := mgr.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if retrieved.LastSeen == nil {
		t.Error("Expected LastSeen to be set after UpdateCameraLastSeen")
	}

	// Verify it's recent (within last second)
	now := time.Now()
	if retrieved.LastSeen.After(now) {
		t.Error("LastSeen should not be in the future")
	}

	if now.Sub(*retrieved.LastSeen) > time.Second {
		t.Error("LastSeen should be recent")
	}
}

func TestManager_DeleteCamera(t *testing.T) {
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

	// Verify it exists
	retrieved, err := mgr.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Camera should exist before deletion")
	}

	// Delete camera
	err = mgr.DeleteCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("DeleteCamera failed: %v", err)
	}

	// Verify it's gone
	retrieved, err = mgr.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Camera should not exist after deletion")
	}
}

func TestManager_CameraState_LastSeen_Nil(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	// Save camera without LastSeen
	camera := CameraState{
		ID:      "cam-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
		LastSeen: nil,
	}

	err := mgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("SaveCamera failed: %v", err)
	}

	retrieved, err := mgr.GetCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if retrieved.LastSeen != nil {
		t.Error("Expected LastSeen to be nil")
	}
}

func TestManager_CameraCRUD_Concurrent(t *testing.T) {
	mgr := setupTestManager(t)
	defer mgr.Close()

	ctx := context.Background()

	done := make(chan bool, 10)

	// Concurrent saves
	for i := 0; i < 10; i++ {
		go func(idx int) {
			camera := CameraState{
				ID:      "cam-" + string(rune(idx)),
				Name:    "Camera " + string(rune(idx)),
				RTSPURL: "rtsp://cam" + string(rune(idx)),
				Enabled: true,
			}
			err := mgr.SaveCamera(ctx, camera)
			if err != nil {
				t.Errorf("Concurrent SaveCamera failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all saves
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all cameras were saved
	cameras, err := mgr.ListCameras(ctx, false)
	if err != nil {
		t.Fatalf("ListCameras failed: %v", err)
	}

	if len(cameras) != 10 {
		t.Errorf("Expected 10 cameras, got %d", len(cameras))
	}
}

