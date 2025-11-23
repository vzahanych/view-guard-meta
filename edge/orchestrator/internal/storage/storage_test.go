package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestStorage(t *testing.T) (*StorageService, string) {
	tmpDir := t.TempDir()
	clipsDir := filepath.Join(tmpDir, "clips")
	snapshotsDir := filepath.Join(tmpDir, "snapshots")

	config := StorageConfig{
		ClipsDir:            clipsDir,
		SnapshotsDir:        snapshotsDir,
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
		StateManager:        nil, // No state manager for basic tests
	}

	storage, err := NewStorageService(config, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create storage service: %v", err)
	}

	return storage, tmpDir
}

func TestNewStorageService(t *testing.T) {
	tmpDir := t.TempDir()
	clipsDir := filepath.Join(tmpDir, "clips")
	snapshotsDir := filepath.Join(tmpDir, "snapshots")

	config := StorageConfig{
		ClipsDir:            clipsDir,
		SnapshotsDir:        snapshotsDir,
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
	}

	storage, err := NewStorageService(config, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create storage service: %v", err)
	}

	if storage == nil {
		t.Fatal("Storage service should not be nil")
	}

	// Verify directories were created
	if _, err := os.Stat(clipsDir); err != nil {
		t.Errorf("Clips directory was not created: %v", err)
	}

	if _, err := os.Stat(snapshotsDir); err != nil {
		t.Errorf("Snapshots directory was not created: %v", err)
	}
}

func TestStorageService_GetClipsDir(t *testing.T) {
	storage, _ := setupTestStorage(t)

	clipsDir := storage.GetClipsDir()
	if clipsDir == "" {
		t.Error("Clips directory should not be empty")
	}
}

func TestStorageService_GetSnapshotsDir(t *testing.T) {
	storage, _ := setupTestStorage(t)

	snapshotsDir := storage.GetSnapshotsDir()
	if snapshotsDir == "" {
		t.Error("Snapshots directory should not be empty")
	}
}

func TestStorageService_GenerateClipPath(t *testing.T) {
	storage, _ := setupTestStorage(t)

	path := storage.GenerateClipPath("camera-1")

	// Verify path format
	if path == "" {
		t.Error("Generated path should not be empty")
	}

	// Verify it's in the clips directory
	clipsDir := storage.GetClipsDir()
	if !filepath.HasPrefix(path, clipsDir) {
		t.Errorf("Path '%s' should be in clips dir '%s'", path, clipsDir)
	}

	// Verify filename format (cameraID_timestamp.mp4)
	filename := filepath.Base(path)
	if len(filename) < 15 { // camera-1_HHMMSS.mp4 is at least 15 chars
		t.Errorf("Filename '%s' seems too short", filename)
	}

	// Verify extension
	if filepath.Ext(path) != ".mp4" {
		t.Errorf("Expected .mp4 extension, got '%s'", filepath.Ext(path))
	}

	// Verify date directory structure (YYYY-MM-DD)
	relPath, _ := filepath.Rel(clipsDir, path)
	dateDir := filepath.Dir(relPath)
	if len(dateDir) != 10 || dateDir[4] != '-' || dateDir[7] != '-' {
		t.Errorf("Expected date directory format YYYY-MM-DD, got '%s'", dateDir)
	}
}

func TestStorageService_GenerateSnapshotPath(t *testing.T) {
	storage, _ := setupTestStorage(t)

	// Test regular snapshot
	path := storage.GenerateSnapshotPath("camera-1", false)

	if path == "" {
		t.Error("Generated path should not be empty")
	}

	snapshotsDir := storage.GetSnapshotsDir()
	if !filepath.HasPrefix(path, snapshotsDir) {
		t.Errorf("Path '%s' should be in snapshots dir '%s'", path, snapshotsDir)
	}

	if filepath.Ext(path) != ".jpg" {
		t.Errorf("Expected .jpg extension, got '%s'", filepath.Ext(path))
	}

	// Test thumbnail
	thumbPath := storage.GenerateSnapshotPath("camera-1", true)

	if thumbPath == "" {
		t.Error("Generated thumbnail path should not be empty")
	}

	if !filepath.HasPrefix(thumbPath, snapshotsDir) {
		t.Errorf("Thumbnail path '%s' should be in snapshots dir '%s'", thumbPath, snapshotsDir)
	}

	// Verify thumbnail has _thumb in filename
	filename := filepath.Base(thumbPath)
	if !strings.Contains(filename, "_thumb") {
		t.Errorf("Thumbnail filename should contain '_thumb', got '%s'", filename)
	}
}

func TestStorageService_SaveClip(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	path := storage.GenerateClipPath("camera-1")

	// Create a dummy file
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	file.WriteString("fake video data")
	file.Close()
	defer os.Remove(path)

	// Get file size
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Save clip entry (without state manager, this should not error)
	err = storage.SaveClip(ctx, path, "camera-1", "event-1", fileInfo.Size())
	if err != nil {
		t.Errorf("SaveClip should not error without state manager: %v", err)
	}
}

func TestStorageService_SaveSnapshot(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	path := storage.GenerateSnapshotPath("camera-1", false)

	// Create a dummy file
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	file.WriteString("fake image data")
	file.Close()
	defer os.Remove(path)

	// Get file size
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Save snapshot entry (without state manager, this should not error)
	err = storage.SaveSnapshot(ctx, path, "camera-1", "event-1", fileInfo.Size())
	if err != nil {
		t.Errorf("SaveSnapshot should not error without state manager: %v", err)
	}
}

func TestStorageService_DeleteClip(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	path := storage.GenerateClipPath("camera-1")

	// Create a dummy file
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	file.WriteString("fake video data")
	file.Close()

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("File should exist: %v", err)
	}

	// Delete clip
	err = storage.DeleteClip(ctx, path)
	if err != nil {
		t.Errorf("DeleteClip failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}
}

func TestStorageService_DeleteSnapshot(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	path := storage.GenerateSnapshotPath("camera-1", false)

	// Create a dummy file
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	file.WriteString("fake image data")
	file.Close()

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("File should exist: %v", err)
	}

	// Delete snapshot
	err = storage.DeleteSnapshot(ctx, path)
	if err != nil {
		t.Errorf("DeleteSnapshot failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}
}

func TestStorageService_GetDiskUsage(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	usage, err := storage.GetDiskUsage(ctx)
	if err != nil {
		t.Fatalf("GetDiskUsage failed: %v", err)
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
}

func TestStorageService_CheckDiskSpace(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	hasSpace, err := storage.CheckDiskSpace(ctx)
	if err != nil {
		t.Fatalf("CheckDiskSpace failed: %v", err)
	}

	// Should have space (we're in a temp directory)
	if !hasSpace {
		t.Error("Should have disk space in temp directory")
	}
}

func TestStorageService_EnforceRetention(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	// Should not error even without state manager
	err := storage.EnforceRetention(ctx)
	if err != nil {
		t.Errorf("EnforceRetention should not error: %v", err)
	}
}

func TestStorageService_GetStorageStats(t *testing.T) {
	storage, _ := setupTestStorage(t)

	ctx := context.Background()
	stats, err := storage.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Storage stats should not be nil")
	}

	if stats.DiskUsagePercent < 0 || stats.DiskUsagePercent > 100 {
		t.Errorf("DiskUsagePercent should be between 0 and 100, got %f", stats.DiskUsagePercent)
	}
}

