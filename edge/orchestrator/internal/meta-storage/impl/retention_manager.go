package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// CleanupStats contains statistics about a cleanup operation.
type CleanupStats struct {
	// RecordsDeleted is the number of records deleted
	RecordsDeleted int64

	// SpaceFreedBytes is the approximate space freed in bytes
	SpaceFreedBytes int64

	// BucketsProcessed is the number of buckets processed
	BucketsProcessed int

	// Duration is how long the cleanup took
	Duration time.Duration
}

// RetentionManager manages retention policies and cleanup operations.
// It tracks retention policies per bucket, identifies expired records,
// and performs cleanup operations.
type RetentionManager struct {
	provider     types.MetaStorageProvider
	config       *types.RetentionConfig
	logger       *zap.Logger
	eventEmitter types.StorageEventEmitter // Optional event emitter for emitting cleanup events

	// Cleanup statistics
	mu            sync.RWMutex
	lastCleanup   time.Time
	lastStats     *CleanupStats
	cleanupRunning bool
}

// NewRetentionManager creates a new retention manager.
func NewRetentionManager(provider types.MetaStorageProvider, config *types.RetentionConfig, logger *zap.Logger) *RetentionManager {
	if config == nil {
		config = &types.RetentionConfig{}
		config.Validate()
	} else {
		config.Validate()
	}

	return &RetentionManager{
		provider: provider,
		config:   config,
		logger:   logger,
	}
}

// SetEventEmitter sets the event emitter for this retention manager.
// This is optional - if not set, events will not be emitted.
func (r *RetentionManager) SetEventEmitter(eventEmitter types.StorageEventEmitter) {
	r.eventEmitter = eventEmitter
}

// CleanupExpiredRecords removes records that exceed their retention period.
// This operation:
// 1. Queries records by creation time (or timestamp field)
// 2. Deletes records that exceed retention period
// 3. Handles per-bucket retention policies
// 4. Returns cleanup statistics
func (r *RetentionManager) CleanupExpiredRecords(ctx context.Context) (*CleanupStats, error) {
	r.mu.Lock()
	if r.cleanupRunning {
		r.mu.Unlock()
		return nil, fmt.Errorf("cleanup already running")
	}
	r.cleanupRunning = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.cleanupRunning = false
		r.mu.Unlock()
	}()

	startTime := time.Now()
	stats := &CleanupStats{}

	if r.logger != nil {
		r.logger.Info("Starting retention cleanup")
	}

	// Emit cleanup started event
	if r.eventEmitter != nil {
		r.eventEmitter.EmitStorageEvent("storage.cleanup_started", map[string]interface{}{})
	}

	// Get buckets that have retention policies
	buckets := AllStandardBuckets()
	now := time.Now()

	for _, bucketName := range buckets {
		retentionHours := r.config.GetBucketRetentionHours(bucketName)
		if retentionHours == 0 {
			// No retention policy for this bucket
			continue
		}

		if !r.provider.BucketExists(ctx, bucketName) {
			continue
		}

		// Process bucket
		bucketStats, err := r.cleanupBucket(ctx, bucketName, retentionHours, now)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("Failed to cleanup bucket",
					zap.String("bucket", bucketName),
					zap.Error(err))
			}
			continue
		}

		stats.RecordsDeleted += bucketStats.RecordsDeleted
		stats.SpaceFreedBytes += bucketStats.SpaceFreedBytes
		stats.BucketsProcessed++
	}

	stats.Duration = time.Since(startTime)

	r.mu.Lock()
	r.lastCleanup = now
	r.lastStats = stats
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("Retention cleanup completed",
			zap.Int64("records_deleted", stats.RecordsDeleted),
			zap.Int64("space_freed_bytes", stats.SpaceFreedBytes),
			zap.Int("buckets_processed", stats.BucketsProcessed),
			zap.Duration("duration", stats.Duration))
	}

	// Emit cleanup completed event
	if r.eventEmitter != nil {
		durationStr := stats.Duration.String()
		r.eventEmitter.EmitStorageEvent("storage.cleanup_completed", map[string]interface{}{
			"records_deleted":   stats.RecordsDeleted,
			"space_freed_bytes": stats.SpaceFreedBytes,
			"buckets_processed": stats.BucketsProcessed,
			"duration":          durationStr,
		})
	}

	return stats, nil
}

// cleanupBucket cleans up expired records in a specific bucket.
func (r *RetentionManager) cleanupBucket(ctx context.Context, bucketName string, retentionHours int, now time.Time) (*CleanupStats, error) {
	stats := &CleanupStats{}
	retentionCutoff := now.Add(-time.Duration(retentionHours) * time.Hour)

	// List all records in the bucket
	keyValues, err := r.provider.List(ctx, bucketName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list records in bucket %s: %w", bucketName, err)
	}

	var keysToDelete [][]byte
	var totalSizeDeleted int64

	for _, kv := range keyValues {
		// Try to extract timestamp from record
		recordTime, err := r.extractRecordTime(kv.Value, bucketName)
		if err != nil {
			// If we can't extract time, skip the record (don't delete)
			if r.logger != nil {
				r.logger.Debug("Skipping record with unparseable timestamp",
					zap.String("bucket", bucketName),
					zap.String("key", string(kv.Key)),
					zap.Error(err))
			}
			continue
		}

		// Check if record is expired
		if recordTime.Before(retentionCutoff) {
			keysToDelete = append(keysToDelete, kv.Key)
			totalSizeDeleted += int64(len(kv.Value))
		}
	}

	// Delete expired records
	for _, key := range keysToDelete {
		if err := r.provider.Delete(ctx, bucketName, key); err != nil {
			if r.logger != nil {
				r.logger.Warn("Failed to delete expired record",
					zap.String("bucket", bucketName),
					zap.String("key", string(key)),
					zap.Error(err))
			}
			continue
		}
		stats.RecordsDeleted++
	}

	stats.SpaceFreedBytes = totalSizeDeleted

	return stats, nil
}

// extractRecordTime extracts the creation/timestamp time from a record.
// This method tries to parse common timestamp fields from JSON records.
func (r *RetentionManager) extractRecordTime(data []byte, bucketName string) (time.Time, error) {
	// Try to parse as JSON first
	var record map[string]interface{}
	if err := json.Unmarshal(data, &record); err == nil {
		// Try common timestamp field names
		timestampFields := []string{"created_at", "CreatedAt", "timestamp", "Timestamp", "created", "Created"}
		for _, field := range timestampFields {
			if ts, ok := record[field]; ok {
				return r.parseTimestamp(ts)
			}
		}

		// For edge_state_history, try "timestamp" or "time" field
		if bucketName == "edge_state_history" {
			if ts, ok := record["timestamp"]; ok {
				return r.parseTimestamp(ts)
			}
			if ts, ok := record["time"]; ok {
				return r.parseTimestamp(ts)
			}
		}
	}

	// If we can't parse, return error (record won't be deleted)
	return time.Time{}, fmt.Errorf("unable to extract timestamp from record")
}

// parseTimestamp parses a timestamp value into time.Time.
func (r *RetentionManager) parseTimestamp(ts interface{}) (time.Time, error) {
	switch v := ts.(type) {
	case string:
		// Try RFC3339 format first
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t, nil
		}
		// Try RFC3339Nano
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t, nil
		}
		// Try Unix timestamp as string
		if t, err := time.Parse(time.UnixDate, v); err == nil {
			return t, nil
		}
		return time.Time{}, fmt.Errorf("unable to parse timestamp string: %s", v)
	case float64:
		// Unix timestamp (seconds)
		return time.Unix(int64(v), 0), nil
	case int64:
		// Unix timestamp (seconds)
		return time.Unix(v, 0), nil
	case int:
		// Unix timestamp (seconds)
		return time.Unix(int64(v), 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type: %T", v)
	}
}

// StartPeriodicCleanup starts a background goroutine that periodically runs cleanup.
// This runs every CleanupIntervalHours (default: 6 hours).
func (r *RetentionManager) StartPeriodicCleanup(ctx context.Context) {
	interval := time.Duration(r.config.CleanupIntervalHours) * time.Hour
	if interval <= 0 {
		interval = 6 * time.Hour
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial cleanup (after first interval)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.CleanupExpiredRecords(ctx); err != nil && r.logger != nil {
				r.logger.Warn("Initial retention cleanup failed", zap.Error(err))
			}
		}

		// Periodic cleanup
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := r.CleanupExpiredRecords(ctx); err != nil && r.logger != nil {
					r.logger.Warn("Periodic retention cleanup failed", zap.Error(err))
				}
			}
		}
	}()
}

// GetLastCleanupStats returns the statistics from the last cleanup operation.
func (r *RetentionManager) GetLastCleanupStats() *CleanupStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.lastStats == nil {
		return nil
	}

	// Return a copy
	return &CleanupStats{
		RecordsDeleted:  r.lastStats.RecordsDeleted,
		SpaceFreedBytes: r.lastStats.SpaceFreedBytes,
		BucketsProcessed: r.lastStats.BucketsProcessed,
		Duration:        r.lastStats.Duration,
	}
}

// GetLastCleanupTime returns when the last cleanup was performed.
func (r *RetentionManager) GetLastCleanupTime() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastCleanup
}

// IsCleanupRunning returns whether a cleanup operation is currently running.
func (r *RetentionManager) IsCleanupRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cleanupRunning
}

