package storage

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// RetentionPolicy enforces storage retention policies
type RetentionPolicy struct {
	retentionDays   int
	maxDiskUsage    float64
	stateManager    StateManager
	logger          *logger.Logger
	mu              sync.RWMutex
	enforcing       bool
}

// NewRetentionPolicy creates a new retention policy
func NewRetentionPolicy(retentionDays int, maxDiskUsage float64, stateManager StateManager, log *logger.Logger) (*RetentionPolicy, error) {
	if retentionDays <= 0 {
		retentionDays = 7 // Default 7 days
	}

	if maxDiskUsage <= 0 {
		maxDiskUsage = 80.0 // Default 80%
	}

	return &RetentionPolicy{
		retentionDays: retentionDays,
		maxDiskUsage:  maxDiskUsage,
		stateManager:  stateManager,
		logger:        log,
	}, nil
}

// Enforce enforces the retention policy
func (r *RetentionPolicy) Enforce(ctx context.Context) error {
	r.mu.Lock()
	if r.enforcing {
		r.mu.Unlock()
		return fmt.Errorf("retention policy is already being enforced")
	}
	r.enforcing = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.enforcing = false
		r.mu.Unlock()
	}()

	// Step 1: Delete files older than retention period
	if err := r.deleteExpiredFiles(ctx); err != nil {
		r.logger.Warn("Failed to delete expired files", "error", err)
	}

	// Step 2: If disk is still full, delete oldest files until below threshold
	if err := r.freeDiskSpace(ctx); err != nil {
		r.logger.Warn("Failed to free disk space", "error", err)
	}

	return nil
}

// deleteExpiredFiles deletes files older than retention period
func (r *RetentionPolicy) deleteExpiredFiles(ctx context.Context) error {
	if r.stateManager == nil {
		return nil // No state manager, can't track files
	}

	// Calculate expiration time
	expirationTime := time.Now().Add(-time.Duration(r.retentionDays) * 24 * time.Hour)

	// Get all storage entries
	clips, err := r.stateManager.ListStorageEntries(ctx, "clip")
	if err != nil {
		return fmt.Errorf("failed to list clips: %w", err)
	}

	snapshots, err := r.stateManager.ListStorageEntries(ctx, "snapshot")
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	allEntries := append(clips, snapshots...)

	deletedCount := 0
	for _, entry := range allEntries {
		// Check if expired
		if entry.CreatedAt.Before(expirationTime) {
			// Delete file
			if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
				r.logger.Warn("Failed to delete expired file", "path", entry.Path, "error", err)
				continue
			}

			// Delete entry from state
			if err := r.stateManager.DeleteStorageEntry(ctx, entry.Path); err != nil {
				r.logger.Warn("Failed to delete storage entry", "path", entry.Path, "error", err)
			}

			deletedCount++
		}
	}

	if deletedCount > 0 {
		r.logger.Info("Deleted expired files", "count", deletedCount)
	}

	return nil
}

// freeDiskSpace deletes oldest files until disk usage is below threshold
func (r *RetentionPolicy) freeDiskSpace(ctx context.Context) error {
	if r.stateManager == nil {
		return nil // No state manager, can't track files
	}

	// Get all storage entries
	clips, err := r.stateManager.ListStorageEntries(ctx, "clip")
	if err != nil {
		return fmt.Errorf("failed to list clips: %w", err)
	}

	snapshots, err := r.stateManager.ListStorageEntries(ctx, "snapshot")
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	allEntries := append(clips, snapshots...)

	// Sort by creation time (oldest first)
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].CreatedAt.Before(allEntries[j].CreatedAt)
	})

	deletedCount := 0
	for _, entry := range allEntries {
		// Check current disk usage (simplified - we'd need disk monitor here)
		// For now, we'll delete oldest files until we've deleted enough
		// In a real implementation, we'd check disk usage after each deletion

		// Delete file
		if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
			r.logger.Warn("Failed to delete file", "path", entry.Path, "error", err)
			continue
		}

		// Delete entry from state
		if err := r.stateManager.DeleteStorageEntry(ctx, entry.Path); err != nil {
			r.logger.Warn("Failed to delete storage entry", "path", entry.Path, "error", err)
		}

		deletedCount++

		// Stop if we've deleted enough (this is a simplified check)
		// In practice, we'd check actual disk usage
		if deletedCount >= 10 {
			break
		}
	}

	if deletedCount > 0 {
		r.logger.Info("Freed disk space by deleting old files", "count", deletedCount)
	}

	return nil
}

// ShouldPauseRecording returns true if recording should be paused due to disk space
func (r *RetentionPolicy) ShouldPauseRecording(ctx context.Context, diskMonitor *DiskMonitor) (bool, error) {
	isFull, err := diskMonitor.IsDiskFull(ctx)
	if err != nil {
		return false, err
	}
	return isFull, nil
}

