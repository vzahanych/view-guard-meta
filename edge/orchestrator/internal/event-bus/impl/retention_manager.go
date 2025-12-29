package impl

import (
	"context"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// RetentionManager manages event retention and cleanup operations.
// It tracks retention policy, cleanup schedule, and cleanup statistics.
type RetentionManager struct {
	provider types.EventBusProvider
	config   *types.RetentionConfig
	logger   *zap.Logger

	// Cleanup state
	mu                sync.RWMutex
	lastCleanupTime   time.Time
	lastCleanupStats  *types.CleanupStats
	cleanupRunning    bool
}

// NewRetentionManager creates a new retention manager.
func NewRetentionManager(provider types.EventBusProvider, config *types.RetentionConfig, logger *zap.Logger) *RetentionManager {
	if config == nil {
		// Use defaults
		config = &types.RetentionConfig{}
		config.Validate()
	}

	return &RetentionManager{
		provider: provider,
		config:   config,
		logger:   logger,
	}
}

// CleanupExpiredEvents deletes events older than the retention period.
// It handles cleanup errors gracefully (logs and continues) and returns cleanup statistics.
func (r *RetentionManager) CleanupExpiredEvents(ctx context.Context) (*types.CleanupStats, error) {
	r.mu.Lock()
	if r.cleanupRunning {
		r.mu.Unlock()
		return nil, nil // Cleanup already running, skip
	}
	r.cleanupRunning = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.cleanupRunning = false
		r.mu.Unlock()
	}()

	startTime := time.Now()
	retentionPeriod := time.Duration(r.config.RetentionHours) * time.Hour
	cutoffTime := time.Now().Add(-retentionPeriod)

	if r.logger != nil {
		r.logger.Info("Starting retention cleanup",
			zap.Time("cutoff_time", cutoffTime),
			zap.Duration("retention_period", retentionPeriod))
	}

	// Use provider's DeleteExpiredEvents which handles batching internally
	// This is more efficient than deleting one by one
	deletedCount, err := r.provider.DeleteExpiredEvents(ctx, cutoffTime)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to delete expired events",
				zap.Error(err))
		}
		// Return partial stats if available
		duration := time.Since(startTime)
		spaceFreedBytes := int64(0) // Unknown on error
		return &types.CleanupStats{
			EventsDeleted:   0,
			SpaceFreedBytes: spaceFreedBytes,
			Duration:        duration,
		}, err
	}

	duration := time.Since(startTime)

	// Calculate approximate space freed (rough estimate: 1KB per event)
	spaceFreedBytes := int64(deletedCount) * 1024

	stats := &types.CleanupStats{
		EventsDeleted:   int64(deletedCount),
		SpaceFreedBytes: spaceFreedBytes,
		Duration:        duration,
	}

	// Update cached stats
	r.mu.Lock()
	r.lastCleanupTime = startTime
	r.lastCleanupStats = stats
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("Retention cleanup completed",
			zap.Int64("events_deleted", stats.EventsDeleted),
			zap.Int64("space_freed_bytes", stats.SpaceFreedBytes),
			zap.Duration("duration", stats.Duration))
	}

	return stats, nil
}

// GetLastCleanupTime returns the time of the last cleanup operation.
func (r *RetentionManager) GetLastCleanupTime() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastCleanupTime
}

// GetLastCleanupStats returns the statistics from the last cleanup operation.
func (r *RetentionManager) GetLastCleanupStats() *types.CleanupStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.lastCleanupStats == nil {
		return nil
	}
	// Return a copy to avoid race conditions
	return &types.CleanupStats{
		EventsDeleted:   r.lastCleanupStats.EventsDeleted,
		SpaceFreedBytes: r.lastCleanupStats.SpaceFreedBytes,
		Duration:        r.lastCleanupStats.Duration,
	}
}

// IsCleanupRunning returns true if a cleanup operation is currently running.
func (r *RetentionManager) IsCleanupRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cleanupRunning
}

