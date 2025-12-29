package impl

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// CleanupStats contains statistics about a cleanup operation for object storage.
type CleanupStats struct {
	// ObjectsDeleted is the number of objects deleted
	ObjectsDeleted int64

	// SpaceFreedBytes is the approximate space freed in bytes
	SpaceFreedBytes int64

	// DataTypesProcessed is the number of data types processed
	DataTypesProcessed int

	// Duration is how long the cleanup took
	Duration time.Duration
}

// RetentionManager manages retention policies and cleanup operations for object storage.
// It tracks retention policies per data type, identifies expired objects,
// and performs cleanup operations.
type RetentionManager struct {
	provider     types.ObjectStorageProvider
	config       *types.RetentionConfig
	logger       *zap.Logger
	eventEmitter StorageEventEmitter // Optional event emitter for emitting cleanup events

	// Cleanup statistics
	mu            sync.RWMutex
	lastCleanup   time.Time
	lastStats     *CleanupStats
	cleanupRunning bool
}

// NewRetentionManager creates a new retention manager for object storage.
func NewRetentionManager(provider types.ObjectStorageProvider, config *types.RetentionConfig, logger *zap.Logger) *RetentionManager {
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
func (r *RetentionManager) SetEventEmitter(eventEmitter StorageEventEmitter) {
	r.eventEmitter = eventEmitter
}

// CleanupExpiredObjects removes objects that exceed their retention period.
// This operation:
// 1. Queries objects by data type and creation time
// 2. Deletes objects that exceed retention period
// 3. Respects grace periods
// 4. Handles model version retention (keep last N versions per device)
// 5. Returns cleanup statistics
func (r *RetentionManager) CleanupExpiredObjects(ctx context.Context) (*CleanupStats, error) {
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
		r.logger.Info("Starting retention cleanup for object storage")
	}

	// Emit cleanup started event
	if r.eventEmitter != nil {
		r.eventEmitter.EmitStorageEvent("storage.cleanup_started", map[string]interface{}{})
	}

	now := time.Now()

	// Process dataset objects (video clips, images, sensor readings, audio samples)
	// These use DatasetRetentionDays (default: 30 days after upload)
	datasetDataTypes := []types.DataType{
		types.DataTypeVideoClip,
		types.DataTypeVideoFrame,
		types.DataTypeImage,
		types.DataTypeSensorReading,
		types.DataTypeAudioSample,
	}

	for _, dataType := range datasetDataTypes {
		retentionDays := r.config.DatasetRetentionDays
		if retentionDays <= 0 {
			continue
		}

		typeStats, err := r.cleanupDataType(ctx, dataType, retentionDays, now, false)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("Failed to cleanup data type",
					zap.String("data_type", string(dataType)),
					zap.Error(err))
			}
			continue
		}

		stats.ObjectsDeleted += typeStats.ObjectsDeleted
		stats.SpaceFreedBytes += typeStats.SpaceFreedBytes
		stats.DataTypesProcessed++
	}

	// Process security event attachments
	// These use EventRetentionDays (default: 7 days after VM ack)
	// Note: We need to check VM ack timestamp from metadata, but for now we'll use creation time
	// TODO: Integrate with meta-storage to get VM ack timestamps
	eventRetentionDays := r.config.EventRetentionDays
	if eventRetentionDays > 0 {
		// Security event attachments are stored with prefix "security-events/"
		eventStats, err := r.cleanupSecurityEventAttachments(ctx, eventRetentionDays, now)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("Failed to cleanup security event attachments",
					zap.Error(err))
			}
		} else {
			stats.ObjectsDeleted += eventStats.ObjectsDeleted
			stats.SpaceFreedBytes += eventStats.SpaceFreedBytes
			stats.DataTypesProcessed++
		}
	}

	// Process model artifacts
	// These use ModelRetentionVersions (keep last N versions per device)
	modelStats, err := r.cleanupModelArtifacts(ctx, now)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("Failed to cleanup model artifacts",
				zap.Error(err))
		}
	} else {
		stats.ObjectsDeleted += modelStats.ObjectsDeleted
		stats.SpaceFreedBytes += modelStats.SpaceFreedBytes
		stats.DataTypesProcessed++
	}

	stats.Duration = time.Since(startTime)

	r.mu.Lock()
	r.lastCleanup = now
	r.lastStats = stats
	r.mu.Unlock()

	if r.logger != nil {
		r.logger.Info("Retention cleanup completed",
			zap.Int64("objects_deleted", stats.ObjectsDeleted),
			zap.Int64("space_freed_bytes", stats.SpaceFreedBytes),
			zap.Int("data_types_processed", stats.DataTypesProcessed),
			zap.Duration("duration", stats.Duration))
	}

	// Emit cleanup completed event
	if r.eventEmitter != nil {
		durationStr := stats.Duration.String()
		r.eventEmitter.EmitStorageEvent("storage.cleanup_completed", map[string]interface{}{
			"objects_deleted":      stats.ObjectsDeleted,
			"space_freed_bytes":    stats.SpaceFreedBytes,
			"data_types_processed": stats.DataTypesProcessed,
			"duration":             durationStr,
		})
	}

	return stats, nil
}

// cleanupDataType cleans up expired objects for a specific data type.
func (r *RetentionManager) cleanupDataType(ctx context.Context, dataType types.DataType, retentionDays int, now time.Time, checkVMAck bool) (*CleanupStats, error) {
	stats := &CleanupStats{}
	retentionCutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

	// List objects with data type prefix
	prefix := string(dataType) + "/"
	objects, err := r.provider.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects with prefix %s: %w", prefix, err)
	}

	var keysToDelete []string
	var totalSizeDeleted int64

	for _, obj := range objects {
		// Extract creation time from object metadata or key
		createdAt, err := r.extractObjectTime(ctx, obj)
		if err != nil {
			// If we can't extract time, skip the object (don't delete)
			if r.logger != nil {
				r.logger.Debug("Skipping object with unparseable timestamp",
					zap.String("key", obj.Key),
					zap.Error(err))
			}
			continue
		}

		// Check if object is expired
		if createdAt.Before(retentionCutoff) {
			keysToDelete = append(keysToDelete, obj.Key)
			totalSizeDeleted += obj.Size
		}
	}

	// Delete expired objects
	for _, key := range keysToDelete {
		if err := r.provider.DeleteObject(ctx, key); err != nil {
			if r.logger != nil {
				r.logger.Warn("Failed to delete expired object",
					zap.String("key", key),
					zap.Error(err))
			}
			continue
		}
		stats.ObjectsDeleted++
	}

	stats.SpaceFreedBytes = totalSizeDeleted

	return stats, nil
}

// cleanupSecurityEventAttachments cleans up expired security event attachments.
func (r *RetentionManager) cleanupSecurityEventAttachments(ctx context.Context, retentionDays int, now time.Time) (*CleanupStats, error) {
	stats := &CleanupStats{}
	retentionCutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

	// List security event attachments
	prefix := "security-events/"
	objects, err := r.provider.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list security event attachments: %w", err)
	}

	var keysToDelete []string
	var totalSizeDeleted int64

	for _, obj := range objects {
		// Extract creation time from object metadata or key
		createdAt, err := r.extractObjectTime(ctx, obj)
		if err != nil {
			// If we can't extract time, skip the object
			if r.logger != nil {
				r.logger.Debug("Skipping security event attachment with unparseable timestamp",
					zap.String("key", obj.Key),
					zap.Error(err))
			}
			continue
		}

		// TODO: Check VM ack timestamp from metadata
		// For now, use creation time
		if createdAt.Before(retentionCutoff) {
			keysToDelete = append(keysToDelete, obj.Key)
			totalSizeDeleted += obj.Size
		}
	}

	// Delete expired attachments
	for _, key := range keysToDelete {
		if err := r.provider.DeleteObject(ctx, key); err != nil {
			if r.logger != nil {
				r.logger.Warn("Failed to delete expired security event attachment",
					zap.String("key", key),
					zap.Error(err))
			}
			continue
		}
		stats.ObjectsDeleted++
	}

	stats.SpaceFreedBytes = totalSizeDeleted

	return stats, nil
}

// cleanupModelArtifacts cleans up old model artifacts based on version retention policy.
// This keeps the last N versions per device (default: 2 versions).
func (r *RetentionManager) cleanupModelArtifacts(ctx context.Context, now time.Time) (*CleanupStats, error) {
	stats := &CleanupStats{}
	keepVersions := r.config.ModelRetentionVersions
	if keepVersions <= 0 {
		keepVersions = 2 // Default
	}

	// List all model artifacts
	prefix := "models/"
	objects, err := r.provider.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list model artifacts: %w", err)
	}

	// Group models by device ID
	// Key format: models/{deviceID}/{modelID}/{artifactType}.{ext}
	modelsByDevice := make(map[string][]types.ObjectInfo)

	for _, obj := range objects {
		// Parse device ID from key: models/{deviceID}/{modelID}/...
		parts := strings.Split(strings.TrimPrefix(obj.Key, prefix), "/")
		if len(parts) < 2 {
			continue
		}
		deviceID := parts[0]
		modelsByDevice[deviceID] = append(modelsByDevice[deviceID], obj)
	}

	// For each device, keep only the last N model versions
	for deviceID, models := range modelsByDevice {
		// Group by model ID
		modelsByModelID := make(map[string][]types.ObjectInfo)
		for _, obj := range models {
			// Extract model ID from key: models/{deviceID}/{modelID}/...
			parts := strings.Split(strings.TrimPrefix(obj.Key, prefix+deviceID+"/"), "/")
			if len(parts) < 1 {
				continue
			}
			modelID := parts[0]
			modelsByModelID[modelID] = append(modelsByModelID[modelID], obj)
		}

		// For each model, get creation time and sort
		type modelVersion struct {
			modelID  string
			createdAt time.Time
			objects  []types.ObjectInfo
		}

		var versions []modelVersion
		for modelID, objs := range modelsByModelID {
			// Get creation time from first object (all artifacts for a model have same time)
			if len(objs) == 0 {
				continue
			}
			createdAt, err := r.extractObjectTime(ctx, objs[0])
			if err != nil {
				// Skip if we can't get time
				continue
			}
			versions = append(versions, modelVersion{
				modelID:  modelID,
				createdAt: createdAt,
				objects:  objs,
			})
		}

		// Sort by creation time (newest first)
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].createdAt.After(versions[j].createdAt)
		})

		// Delete old versions (keep only last N)
		for i := keepVersions; i < len(versions); i++ {
			version := versions[i]
			// Check grace period
			gracePeriodCutoff := now.Add(-time.Duration(r.config.ModelRetentionGracePeriodDays) * 24 * time.Hour)
			if version.createdAt.After(gracePeriodCutoff) {
				// Still in grace period, don't delete
				continue
			}

			// Delete all artifacts for this model version
			for _, obj := range version.objects {
				if err := r.provider.DeleteObject(ctx, obj.Key); err != nil {
					if r.logger != nil {
						r.logger.Warn("Failed to delete old model artifact",
							zap.String("device_id", deviceID),
							zap.String("model_id", version.modelID),
							zap.String("key", obj.Key),
							zap.Error(err))
					}
					continue
				}
				stats.ObjectsDeleted++
				stats.SpaceFreedBytes += obj.Size
			}
		}
	}

	return stats, nil
}

// extractObjectTime extracts the creation time from an object.
// It tries to get it from metadata first, then falls back to parsing the key.
func (r *RetentionManager) extractObjectTime(ctx context.Context, obj types.ObjectInfo) (time.Time, error) {
	// Try to get metadata
	metadata, err := r.provider.GetObjectMetadata(ctx, obj.Key)
	if err == nil && !metadata.CreatedAt.IsZero() {
		return metadata.CreatedAt, nil
	}

	// Fall back to parsing key
	// Key format: {dataType}/{deviceType}/{deviceID}/{YYYY-MM-DD}/{filename}
	// Extract date from key
	parts := strings.Split(obj.Key, "/")
	if len(parts) >= 4 {
		dateStr := parts[3] // Should be YYYY-MM-DD
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			return t, nil
		}
	}

	// Fall back to LastModified if available
	if obj.LastModified > 0 {
		return time.Unix(obj.LastModified, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to extract creation time from object %s", obj.Key)
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
			if _, err := r.CleanupExpiredObjects(ctx); err != nil && r.logger != nil {
				r.logger.Warn("Initial retention cleanup failed", zap.Error(err))
			}
		}

		// Periodic cleanup
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := r.CleanupExpiredObjects(ctx); err != nil && r.logger != nil {
					r.logger.Warn("Periodic retention cleanup failed", zap.Error(err))
				}
			}
		}
	}()
}

// StopPeriodicCleanup stops the periodic cleanup.
// This is called when the service is stopped.
func (r *RetentionManager) StopPeriodicCleanup() {
	// The periodic cleanup is managed by context cancellation,
	// so this is a no-op. The context passed to StartPeriodicCleanup
	// should be cancelled when the service stops.
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
		ObjectsDeleted:    r.lastStats.ObjectsDeleted,
		SpaceFreedBytes:   r.lastStats.SpaceFreedBytes,
		DataTypesProcessed: r.lastStats.DataTypesProcessed,
		Duration:          r.lastStats.Duration,
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

