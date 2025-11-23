package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// TestConfigState_Integration tests configuration and state management integration
func TestConfigState_Integration(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Save system state
	err := env.StateMgr.SaveSystemState(ctx, "test_key", "test_value")
	if err != nil {
		t.Fatalf("Failed to save system state: %v", err)
	}

	// Retrieve system state
	value, err := env.StateMgr.GetSystemState(ctx, "test_key")
	if err != nil {
		t.Fatalf("Failed to get system state: %v", err)
	}

	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}
}

// TestConfigState_StateRecovery tests state recovery after restart
func TestConfigState_StateRecovery(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Save a camera state
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test.example.com/stream",
		Enabled: true,
	}

	err := env.StateMgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("Failed to save camera: %v", err)
	}

	// Close state manager (simulating restart)
	env.StateMgr.Close()

	// Recreate state manager (simulating restart)
	stateMgr2, err := state.NewManager(env.Config, env.Logger)
	if err != nil {
		t.Fatalf("Failed to recreate state manager: %v", err)
	}
	defer stateMgr2.Close()

	// Recover state
	recovered, err := stateMgr2.RecoverState(ctx)
	if err != nil {
		t.Fatalf("Failed to recover state: %v", err)
	}

	// Verify camera was recovered
	if len(recovered.Cameras) != 1 {
		t.Fatalf("Expected 1 camera, got %d", len(recovered.Cameras))
	}

	recoveredCamera := recovered.Cameras[0]
	if recoveredCamera.ID != "camera-1" {
		t.Errorf("Expected camera ID 'camera-1', got '%s'", recoveredCamera.ID)
	}

	if recoveredCamera.Name != "Test Camera" {
		t.Errorf("Expected camera name 'Test Camera', got '%s'", recoveredCamera.Name)
	}
}

// TestConfigState_ConfigFileIntegration tests configuration file loading and state persistence
func TestConfigState_ConfigFileIntegration(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create a test config file
	configPath := filepath.Join(env.TempDir, "config.yaml")
	configContent := `
edge:
  orchestrator:
    log_level: "info"
    log_format: "json"
    data_dir: "` + env.Config.Edge.Orchestrator.DataDir + `"
  storage:
    clips_dir: "` + env.Config.Edge.Storage.ClipsDir + `"
    snapshots_dir: "` + env.Config.Edge.Storage.SnapshotsDir + `"
    retention_days: 7
    max_disk_usage_percent: 80.0
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	loadedConfig, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify config was loaded correctly
	if loadedConfig.Edge.Orchestrator.LogLevel != "info" {
		t.Errorf("Expected log level 'info', got '%s'", loadedConfig.Edge.Orchestrator.LogLevel)
	}

	if loadedConfig.Edge.Storage.RetentionDays != 7 {
		t.Errorf("Expected retention days 7, got %d", loadedConfig.Edge.Storage.RetentionDays)
	}

	// Create state manager with loaded config
	stateMgr, err := state.NewManager(loadedConfig, env.Logger)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}
	defer stateMgr.Close()

	// Verify state manager can use the config
	ctx := context.Background()
	err = stateMgr.SaveSystemState(ctx, "config_loaded", "true")
	if err != nil {
		t.Fatalf("Failed to save system state: %v", err)
	}
}

// TestConfigState_CameraStatePersistence tests camera state persistence and retrieval
func TestConfigState_CameraStatePersistence(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Save multiple cameras
	cameras := []state.CameraState{
		{
			ID:      "camera-1",
			Name:    "Front Door",
			RTSPURL: "rtsp://192.168.1.100/stream",
			Enabled: true,
		},
		{
			ID:      "camera-2",
			Name:    "Back Yard",
			RTSPURL: "rtsp://192.168.1.101/stream",
			Enabled: true,
		},
		{
			ID:      "camera-3",
			Name:    "Garage",
			RTSPURL: "rtsp://192.168.1.102/stream",
			Enabled: false,
		},
	}

	for _, cam := range cameras {
		err := env.StateMgr.SaveCamera(ctx, cam)
		if err != nil {
			t.Fatalf("Failed to save camera %s: %v", cam.ID, err)
		}
	}

	// List all cameras
	allCameras, err := env.StateMgr.ListCameras(ctx, false)
	if err != nil {
		t.Fatalf("Failed to list cameras: %v", err)
	}

	if len(allCameras) != 3 {
		t.Errorf("Expected 3 cameras, got %d", len(allCameras))
	}

	// List enabled cameras only
	enabledCameras, err := env.StateMgr.ListCameras(ctx, true)
	if err != nil {
		t.Fatalf("Failed to list enabled cameras: %v", err)
	}

	if len(enabledCameras) != 2 {
		t.Errorf("Expected 2 enabled cameras, got %d", len(enabledCameras))
	}

	// Get specific camera
	camera, err := env.StateMgr.GetCamera(ctx, "camera-1")
	if err != nil {
		t.Fatalf("Failed to get camera: %v", err)
	}

	if camera.Name != "Front Door" {
		t.Errorf("Expected camera name 'Front Door', got '%s'", camera.Name)
	}

	// Update camera last seen
	now := time.Now()
	err = env.StateMgr.UpdateCameraLastSeen(ctx, "camera-1")
	if err != nil {
		t.Fatalf("Failed to update camera last seen: %v", err)
	}

	// Verify last seen was updated
	updatedCamera, err := env.StateMgr.GetCamera(ctx, "camera-1")
	if err != nil {
		t.Fatalf("Failed to get updated camera: %v", err)
	}

	if updatedCamera.LastSeen == nil || updatedCamera.LastSeen.Before(now) {
		t.Error("Camera last seen was not updated correctly")
	}
}

