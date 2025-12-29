package impl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// StorageEventEmitter defines the interface for emitting storage events.
// This allows the quota manager to emit events without directly depending on the event bus.
type StorageEventEmitter interface {
	EmitStorageEvent(eventType string, data map[string]interface{})
}

// QuotaManager manages storage quota tracking and enforcement for object storage.
// It tracks current usage (sum of all object sizes), monitors quota limits,
// and provides quota status information.
type QuotaManager struct {
	provider     types.ObjectStorageProvider
	config       *types.QuotaConfig
	logger       *zap.Logger
	eventEmitter StorageEventEmitter // Optional event emitter for emitting quota events

	// Cached quota status (updated periodically)
	mu           sync.RWMutex
	lastCheck    time.Time
	cachedStatus *types.StorageQuota
	objectCounts map[string]int64 // Data type -> count
}

// NewQuotaManager creates a new quota manager for object storage.
func NewQuotaManager(provider types.ObjectStorageProvider, config *types.QuotaConfig, logger *zap.Logger) *QuotaManager {
	if config == nil {
		config = &types.QuotaConfig{}
		config.Validate()
	} else {
		config.Validate()
	}

	return &QuotaManager{
		provider:     provider,
		config:       config,
		logger:       logger,
		objectCounts: make(map[string]int64),
	}
}

// SetEventEmitter sets the event emitter for this quota manager.
// This is optional - if not set, events will not be emitted.
func (q *QuotaManager) SetEventEmitter(eventEmitter StorageEventEmitter) {
	q.eventEmitter = eventEmitter
}

// GetQuotaStatus returns the current quota status.
// This queries the provider for total object size by listing all objects,
// calculates usage percentage, and returns quota status.
func (q *QuotaManager) GetQuotaStatus(ctx context.Context) (*types.StorageQuota, error) {
	// List all objects to calculate total size
	objects, err := q.provider.ListObjects(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	// Calculate total size and count objects by data type
	var totalSize int64
	objectCounts := make(map[string]int64)

	for _, obj := range objects {
		totalSize += obj.Size

		// Extract data type from key prefix (e.g., "video_clips/", "images/", "models/")
		dataType := q.extractDataTypeFromKey(obj.Key)
		objectCounts[dataType]++
	}

	// Update cached counts
	q.mu.Lock()
	q.objectCounts = objectCounts
	q.lastCheck = time.Now()
	q.mu.Unlock()

	// Calculate usage
	limitBytes := int64(q.config.MaxSizeMB) * 1024 * 1024

	quota := &types.StorageQuota{
		Used:             totalSize,
		Limit:            limitBytes,
		WarningThreshold: q.config.WarningThresholdPercent,
		FullThreshold:    q.config.FullThresholdPercent,
	}

	// Update cached status
	q.mu.Lock()
	q.cachedStatus = quota
	q.mu.Unlock()

	return quota, nil
}

// extractDataTypeFromKey extracts the data type from an object key.
// Keys are organized as: {dataType}/{deviceType}/{deviceID}/{date}/{filename}
// Returns "unknown" if the key format is unexpected.
func (q *QuotaManager) extractDataTypeFromKey(key string) string {
	// Key format: {dataType}/{deviceType}/{deviceID}/{date}/{filename}
	// Extract first component as data type
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[:i]
		}
	}
	return "unknown"
}

// CheckQuotaBeforeWrite checks if a write operation should be allowed based on quota.
// Returns an error if quota is exceeded.
func (q *QuotaManager) CheckQuotaBeforeWrite(ctx context.Context, objectSize int64) error {
	// Get current quota status
	quota, err := q.GetQuotaStatus(ctx)
	if err != nil {
		// If we can't get quota status, allow the write (fail open)
		if q.logger != nil {
			q.logger.Warn("Failed to get quota status, allowing write",
				zap.Error(err))
		}
		return nil
	}

	// Calculate usage percentage (avoid division by zero)
	if quota.Limit == 0 {
		// If limit is 0, allow the write (shouldn't happen in practice)
		return nil
	}

	// Calculate projected usage after this write
	projectedUsed := quota.Used + objectSize
	usagePercent := int((quota.Used * 100) / quota.Limit)
	projectedUsagePercent := int((projectedUsed * 100) / quota.Limit)

	// Check thresholds
	if projectedUsagePercent >= quota.FullThreshold {
		// >95%: reject write operations
		if q.eventEmitter != nil {
			q.eventEmitter.EmitStorageEvent("storage.quota_exceeded", map[string]interface{}{
				"used_bytes":         quota.Used,
				"limit_bytes":        quota.Limit,
				"usage_percent":      float64(usagePercent),
				"object_size":        objectSize,
				"projected_usage":    projectedUsed,
				"projected_percent":  float64(projectedUsagePercent),
			})
		}
		return fmt.Errorf("storage quota exceeded: projected usage %d%% >= %d%% (limit: %d MB, current: %d MB, object size: %d bytes)",
			projectedUsagePercent, quota.FullThreshold, q.config.MaxSizeMB, quota.Used/(1024*1024), objectSize)
	}

	// 90-95%: throttle large objects (allow small/critical operations)
	if usagePercent >= 90 && objectSize > 10*1024*1024 { // 10 MB
		if q.logger != nil {
			q.logger.Warn("Throttling large object write due to high quota usage",
				zap.Int("usage_percent", usagePercent),
				zap.Int64("object_size", objectSize))
		}
		return fmt.Errorf("storage quota high (%d%%): rejecting large object (%d bytes)",
			usagePercent, objectSize)
	}

	// 80-90%: emit warning but allow operation
	if usagePercent >= quota.WarningThreshold && usagePercent < 90 {
		if q.logger != nil {
			q.logger.Warn("Storage quota warning",
				zap.Int("usage_percent", usagePercent),
				zap.Int("warning_threshold", quota.WarningThreshold))
		}
		// Emit storage.warning event
		if q.eventEmitter != nil {
			q.eventEmitter.EmitStorageEvent("storage.warning", map[string]interface{}{
				"used_bytes":    quota.Used,
				"limit_bytes":   quota.Limit,
				"usage_percent": float64(usagePercent),
			})
		}
	}

	return nil
}

// GetObjectCounts returns the cached object counts by data type.
func (q *QuotaManager) GetObjectCounts() map[string]int64 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Return a copy to prevent external modification
	counts := make(map[string]int64)
	for k, v := range q.objectCounts {
		counts[k] = v
	}
	return counts
}

// GetCachedQuotaStatus returns the cached quota status without querying the provider.
// This is faster but may be slightly stale.
func (q *QuotaManager) GetCachedQuotaStatus() *types.StorageQuota {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.cachedStatus == nil {
		return nil
	}

	// Return a copy
	return &types.StorageQuota{
		Used:             q.cachedStatus.Used,
		Limit:            q.cachedStatus.Limit,
		WarningThreshold: q.cachedStatus.WarningThreshold,
		FullThreshold:    q.cachedStatus.FullThreshold,
	}
}

// StartPeriodicChecks starts a background goroutine that periodically checks quota.
// This runs every 5 minutes by default.
func (q *QuotaManager) StartPeriodicChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial check
		if _, err := q.GetQuotaStatus(ctx); err != nil && q.logger != nil {
			q.logger.Warn("Initial quota check failed", zap.Error(err))
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				quota, err := q.GetQuotaStatus(ctx)
				if err != nil {
					if q.logger != nil {
						q.logger.Warn("Periodic quota check failed", zap.Error(err))
					}
					continue
				}

				// Check thresholds and emit events
				if quota.Limit == 0 {
					continue
				}
				usagePercent := int((quota.Used * 100) / quota.Limit)

				if usagePercent >= quota.FullThreshold {
					if q.logger != nil {
						q.logger.Error("Storage quota exceeded",
							zap.Int("usage_percent", usagePercent),
							zap.Int("full_threshold", quota.FullThreshold))
					}
					// Emit storage.full event
					if q.eventEmitter != nil {
						q.eventEmitter.EmitStorageEvent("storage.full", map[string]interface{}{
							"used_bytes":    quota.Used,
							"limit_bytes":   quota.Limit,
							"usage_percent": float64(usagePercent),
						})
					}
				} else if usagePercent >= quota.WarningThreshold {
					if q.logger != nil {
						q.logger.Warn("Storage quota warning",
							zap.Int("usage_percent", usagePercent),
							zap.Int("warning_threshold", quota.WarningThreshold))
					}
					// Emit storage.warning event
					if q.eventEmitter != nil {
						q.eventEmitter.EmitStorageEvent("storage.warning", map[string]interface{}{
							"used_bytes":    quota.Used,
							"limit_bytes":   quota.Limit,
							"usage_percent": float64(usagePercent),
						})
					}
				}
			}
		}
	}()
}

// StopPeriodicChecks stops the periodic quota checks.
// This is called when the service is stopped.
func (q *QuotaManager) StopPeriodicChecks() {
	// The periodic checks are managed by context cancellation,
	// so this is a no-op. The context passed to StartPeriodicChecks
	// should be cancelled when the service stops.
}

