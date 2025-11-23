package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
)

// TestStorage_ClipStorageIntegration tests clip storage with state manager integration
func TestStorage_ClipStorageIntegration(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create storage state manager
	stateMgr := storage.NewStorageStateManager(env.StateMgr.GetDB(), env.Logger)

	// Create storage service with state manager
	storageConfig := storage.StorageConfig{
		ClipsDir:            env.Config.Edge.Storage.ClipsDir,
		SnapshotsDir:        env.Config.Edge.Storage.SnapshotsDir,
		RetentionDays:       env.Config.Edge.Storage.RetentionDays,
		MaxDiskUsagePercent: env.Config.Edge.Storage.MaxDiskUsagePercent,
		StateManager:        stateMgr,
	}

	storageService, err := storage.NewStorageService(storageConfig, env.Logger)
	if err != nil {
		t.Fatalf("Failed to create storage service: %v", err)
	}

	ctx := context.Background()

	// Create a camera entry first (required for foreign key)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test.example.com/stream",
		Enabled: true,
	}
	err = env.StateMgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("Failed to save camera: %v", err)
	}

	// Generate clip path
	clipPath := storageService.GenerateClipPath("camera-1")

	// Create a dummy clip file
	if err := os.MkdirAll(filepath.Dir(clipPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	file, err := os.Create(clipPath)
	if err != nil {
		t.Fatalf("Failed to create clip file: %v", err)
	}
	file.WriteString("fake video data")
	file.Close()
	defer os.Remove(clipPath)

	// Get file size
	fileInfo, err := os.Stat(clipPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Save clip entry (event_id can be empty, camera_id must exist)
	err = storageService.SaveClip(ctx, clipPath, "camera-1", "", fileInfo.Size())
	if err != nil {
		t.Fatalf("Failed to save clip: %v", err)
	}

	// Verify clip entry was saved
	entries, err := stateMgr.ListStorageEntries(ctx, "clip")
	if err != nil {
		t.Fatalf("Failed to list storage entries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 clip entry, got %d", len(entries))
	}

	if entries[0].Path != clipPath {
		t.Errorf("Expected path '%s', got '%s'", clipPath, entries[0].Path)
	}

	if entries[0].CameraID != "camera-1" {
		t.Errorf("Expected camera ID 'camera-1', got '%s'", entries[0].CameraID)
	}
}

// TestStorage_RetentionPolicyIntegration tests retention policy with storage state
func TestStorage_RetentionPolicyIntegration(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create storage state manager
	stateMgr := storage.NewStorageStateManager(env.StateMgr.GetDB(), env.Logger)

	// Create retention policy
	retentionPolicy, err := storage.NewRetentionPolicy(1, 80.0, stateMgr, env.Logger)
	if err != nil {
		t.Fatalf("Failed to create retention policy: %v", err)
	}

	ctx := context.Background()

	// Create a camera entry first (required for foreign key)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test.example.com/stream",
		Enabled: true,
	}
	err = env.StateMgr.SaveCamera(ctx, camera)
	if err != nil {
		t.Fatalf("Failed to save camera: %v", err)
	}

	// Create old storage entries (older than 1 day)
	oldTime := time.Now().Add(-2 * 24 * time.Hour)
	oldEntry := storage.StorageEntry{
		Path:      filepath.Join(env.TempDir, "old_clip.mp4"),
		FileType:  "clip",
		SizeBytes: 1000,
		CameraID:  "camera-1",
		EventID:   "", // Empty event ID is allowed
		CreatedAt: oldTime,
	}

	err = stateMgr.SaveStorageEntry(ctx, oldEntry)
	if err != nil {
		t.Fatalf("Failed to save old entry: %v", err)
	}

	// Create a dummy old file
	oldFile, err := os.Create(oldEntry.Path)
	if err == nil {
		oldFile.WriteString("old data")
		oldFile.Close()
		defer os.Remove(oldEntry.Path)
	}

	// Create recent storage entry
	recentEntry := storage.StorageEntry{
		Path:      filepath.Join(env.TempDir, "recent_clip.mp4"),
		FileType:  "clip",
		SizeBytes: 1000,
		CameraID:  "camera-1",
		EventID:   "", // Empty event ID is allowed
		CreatedAt: time.Now(),
	}

	err = stateMgr.SaveStorageEntry(ctx, recentEntry)
	if err != nil {
		t.Fatalf("Failed to save recent entry: %v", err)
	}

	// Create a dummy recent file
	recentFile, err := os.Create(recentEntry.Path)
	if err == nil {
		recentFile.WriteString("recent data")
		recentFile.Close()
		defer os.Remove(recentEntry.Path)
	}

	// Enforce retention policy
	err = retentionPolicy.Enforce(ctx)
	if err != nil {
		t.Fatalf("Failed to enforce retention policy: %v", err)
	}

	// Verify retention policy ran (old entry should be deleted)
	entries, err := stateMgr.ListStorageEntries(ctx, "clip")
	if err != nil {
		t.Fatalf("Failed to list storage entries: %v", err)
	}

	// The old entry should be deleted (it's older than retention period)
	// The recent entry may or may not be deleted depending on freeDiskSpace logic
	// In a real scenario, freeDiskSpace only runs if disk is full
	// For this test, we verify that the retention policy ran without error
	// and that at least the old entry was processed
	
	// Verify old entry is not in the list
	oldEntryFound := false
	for _, entry := range entries {
		if entry.Path == oldEntry.Path {
			oldEntryFound = true
			break
		}
	}
	if oldEntryFound {
		t.Error("Old entry should have been deleted by retention policy")
	}
}

// TestStorage_DiskMonitoringIntegration tests disk monitoring with storage service
func TestStorage_DiskMonitoringIntegration(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create storage service
	storageConfig := storage.StorageConfig{
		ClipsDir:            env.Config.Edge.Storage.ClipsDir,
		SnapshotsDir:        env.Config.Edge.Storage.SnapshotsDir,
		RetentionDays:       env.Config.Edge.Storage.RetentionDays,
		MaxDiskUsagePercent: env.Config.Edge.Storage.MaxDiskUsagePercent,
	}

	storageService, err := storage.NewStorageService(storageConfig, env.Logger)
	if err != nil {
		t.Fatalf("Failed to create storage service: %v", err)
	}

	ctx := context.Background()

	// Get disk usage
	usage, err := storageService.GetDiskUsage(ctx)
	if err != nil {
		t.Fatalf("Failed to get disk usage: %v", err)
	}

	if usage == nil {
		t.Fatal("Disk usage should not be nil")
	}

	if usage.TotalBytes <= 0 {
		t.Error("TotalBytes should be greater than 0")
	}

	if usage.UsagePercent < 0 || usage.UsagePercent > 100 {
		t.Errorf("UsagePercent should be between 0 and 100, got %f", usage.UsagePercent)
	}

	// Check disk space
	hasSpace, err := storageService.CheckDiskSpace(ctx)
	if err != nil {
		t.Fatalf("Failed to check disk space: %v", err)
	}

	// Should have space in temp directory
	if !hasSpace {
		t.Error("Should have disk space in temp directory")
	}
}

