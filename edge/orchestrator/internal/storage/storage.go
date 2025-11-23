package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// StorageService manages local storage for clips and snapshots
type StorageService struct {
	logger        *logger.Logger
	clipsDir      string
	snapshotsDir  string
	stateManager  StateManager
	mu            sync.RWMutex
	diskMonitor   *DiskMonitor
	retention     *RetentionPolicy
}

// StateManager interface for tracking storage state
type StateManager interface {
	SaveStorageEntry(ctx context.Context, entry StorageEntry) error
	DeleteStorageEntry(ctx context.Context, path string) error
	ListStorageEntries(ctx context.Context, fileType string) ([]StorageEntry, error)
	GetStorageStats(ctx context.Context) (*StorageStats, error)
}

// StorageEntry represents a stored file (clip or snapshot)
type StorageEntry struct {
	Path      string
	FileType  string // "clip" or "snapshot"
	SizeBytes int64
	CameraID  string
	EventID   string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// StorageStats contains storage statistics
type StorageStats struct {
	TotalClips     int
	TotalSnapshots int
	TotalSizeBytes int64
	DiskUsagePercent float64
	AvailableBytes int64
}

// StorageConfig contains storage service configuration
type StorageConfig struct {
	ClipsDir            string
	SnapshotsDir        string
	RetentionDays       int
	MaxDiskUsagePercent float64
	StateManager        StateManager
}

// NewStorageService creates a new storage service
func NewStorageService(config StorageConfig, log *logger.Logger) (*StorageService, error) {
	// Ensure directories exist
	if err := os.MkdirAll(config.ClipsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create clips directory: %w", err)
	}

	if err := os.MkdirAll(config.SnapshotsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	// Default retention days
	retentionDays := config.RetentionDays
	if retentionDays == 0 {
		retentionDays = 7 // Default 7 days
	}

	// Default max disk usage
	maxDiskUsage := config.MaxDiskUsagePercent
	if maxDiskUsage == 0 {
		maxDiskUsage = 80.0 // Default 80%
	}

	service := &StorageService{
		logger:       log,
		clipsDir:     config.ClipsDir,
		snapshotsDir: config.SnapshotsDir,
		stateManager: config.StateManager,
	}

	// Initialize disk monitor
	diskMonitor, err := NewDiskMonitor(config.ClipsDir, maxDiskUsage, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk monitor: %w", err)
	}
	service.diskMonitor = diskMonitor

	// Initialize retention policy
	retention, err := NewRetentionPolicy(retentionDays, maxDiskUsage, config.StateManager, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create retention policy: %w", err)
	}
	service.retention = retention

	log.Info("Storage service initialized",
		"clips_dir", config.ClipsDir,
		"snapshots_dir", config.SnapshotsDir,
		"retention_days", retentionDays,
		"max_disk_usage_percent", maxDiskUsage,
	)

	return service, nil
}

// GetClipsDir returns the clips directory path
func (s *StorageService) GetClipsDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clipsDir
}

// GetSnapshotsDir returns the snapshots directory path
func (s *StorageService) GetSnapshotsDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotsDir
}

// GenerateClipPath generates a path for a clip with date/camera organization
func (s *StorageService) GenerateClipPath(cameraID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Organize by date: YYYY-MM-DD/cameraID_timestamp.mp4
	dateDir := time.Now().Format("2006-01-02")
	timestamp := time.Now().Format("150405")
	filename := fmt.Sprintf("%s_%s.mp4", cameraID, timestamp)

	clipDir := filepath.Join(s.clipsDir, dateDir)
	_ = os.MkdirAll(clipDir, 0755) // Ensure date directory exists

	return filepath.Join(clipDir, filename)
}

// GenerateSnapshotPath generates a path for a snapshot with date/camera organization
func (s *StorageService) GenerateSnapshotPath(cameraID string, isThumbnail bool) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Organize by date: YYYY-MM-DD/cameraID_timestamp.jpg
	dateDir := time.Now().Format("2006-01-02")
	timestamp := time.Now().Format("150405")
	
	var filename string
	if isThumbnail {
		filename = fmt.Sprintf("%s_%s_thumb.jpg", cameraID, timestamp)
	} else {
		filename = fmt.Sprintf("%s_%s.jpg", cameraID, timestamp)
	}

	snapshotDir := filepath.Join(s.snapshotsDir, dateDir)
	_ = os.MkdirAll(snapshotDir, 0755) // Ensure date directory exists

	return filepath.Join(snapshotDir, filename)
}

// SaveClip saves a clip entry to the storage state
func (s *StorageService) SaveClip(ctx context.Context, path string, cameraID string, eventID string, sizeBytes int64) error {
	entry := StorageEntry{
		Path:      path,
		FileType:  "clip",
		SizeBytes: sizeBytes,
		CameraID:  cameraID,
		EventID:   eventID,
		CreatedAt: time.Now(),
	}

	if s.stateManager != nil {
		if err := s.stateManager.SaveStorageEntry(ctx, entry); err != nil {
			return fmt.Errorf("failed to save clip entry: %w", err)
		}
	}

	return nil
}

// SaveSnapshot saves a snapshot entry to the storage state
func (s *StorageService) SaveSnapshot(ctx context.Context, path string, cameraID string, eventID string, sizeBytes int64) error {
	entry := StorageEntry{
		Path:      path,
		FileType:  "snapshot",
		SizeBytes: sizeBytes,
		CameraID:  cameraID,
		EventID:   eventID,
		CreatedAt: time.Now(),
	}

	if s.stateManager != nil {
		if err := s.stateManager.SaveStorageEntry(ctx, entry); err != nil {
			return fmt.Errorf("failed to save snapshot entry: %w", err)
		}
	}

	return nil
}

// GetDiskUsage returns current disk usage statistics
func (s *StorageService) GetDiskUsage(ctx context.Context) (*DiskUsage, error) {
	return s.diskMonitor.GetUsage(ctx)
}

// CheckDiskSpace checks if there's enough disk space
func (s *StorageService) CheckDiskSpace(ctx context.Context) (bool, error) {
	return s.diskMonitor.CheckSpace(ctx)
}

// EnforceRetention enforces retention policy (deletes old files)
func (s *StorageService) EnforceRetention(ctx context.Context) error {
	return s.retention.Enforce(ctx)
}

// GetStorageStats returns storage statistics
func (s *StorageService) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	if s.stateManager == nil {
		// Return basic stats without state manager
		usage, err := s.diskMonitor.GetUsage(ctx)
		if err != nil {
			return nil, err
		}

		return &StorageStats{
			DiskUsagePercent: usage.UsagePercent,
			AvailableBytes:   usage.AvailableBytes,
		}, nil
	}

	return s.stateManager.GetStorageStats(ctx)
}

// DeleteClip deletes a clip file and its entry
func (s *StorageService) DeleteClip(ctx context.Context, path string) error {
	// Delete file
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete clip file: %w", err)
	}

	// Delete entry from state
	if s.stateManager != nil {
		if err := s.stateManager.DeleteStorageEntry(ctx, path); err != nil {
			s.logger.Warn("Failed to delete storage entry", "path", path, "error", err)
		}
	}

	return nil
}

// DeleteSnapshot deletes a snapshot file and its entry
func (s *StorageService) DeleteSnapshot(ctx context.Context, path string) error {
	// Delete file
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete snapshot file: %w", err)
	}

	// Delete entry from state
	if s.stateManager != nil {
		if err := s.stateManager.DeleteStorageEntry(ctx, path); err != nil {
			s.logger.Warn("Failed to delete storage entry", "path", path, "error", err)
		}
	}

	return nil
}

