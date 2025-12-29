package impl

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// QuotaManager manages storage quota tracking and enforcement.
// It tracks current usage (database file size, record counts per bucket),
// monitors quota limits, and provides quota status information.
type QuotaManager struct {
	provider     types.MetaStorageProvider
	config       *types.QuotaConfig
	logger       *zap.Logger
	eventEmitter types.StorageEventEmitter // Optional event emitter for emitting quota events

	// Cached quota status (updated periodically)
	mu           sync.RWMutex
	lastCheck    time.Time
	cachedStatus *types.StorageQuota
	bucketCounts map[string]int64

	// Database file path (for local providers like BoltDB)
	databasePath string
}

// NewQuotaManager creates a new quota manager.
func NewQuotaManager(provider types.MetaStorageProvider, config *types.QuotaConfig, logger *zap.Logger, databasePath string) *QuotaManager {
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
		bucketCounts: make(map[string]int64),
		databasePath: databasePath,
	}
}

// SetEventEmitter sets the event emitter for this quota manager.
// This is optional - if not set, events will not be emitted.
func (q *QuotaManager) SetEventEmitter(eventEmitter types.StorageEventEmitter) {
	q.eventEmitter = eventEmitter
}

// GetQuotaStatus returns the current quota status.
// This queries the provider for database size, counts records per bucket,
// calculates usage percentage, and returns quota status.
func (q *QuotaManager) GetQuotaStatus(ctx context.Context) (*types.StorageQuota, error) {
	// Get database size
	var databaseSize int64
	if q.databasePath != "" {
		// For local providers, get file size
		if info, err := os.Stat(q.databasePath); err == nil {
			databaseSize = info.Size()
		}
	}

	// Count records per bucket
	bucketCounts, err := q.countRecordsPerBucket(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count records per bucket: %w", err)
	}

	// Update cached counts
	q.mu.Lock()
	q.bucketCounts = bucketCounts
	q.lastCheck = time.Now()
	q.mu.Unlock()

	// Calculate usage
	limitBytes := int64(q.config.MaxSizeMB) * 1024 * 1024

	quota := &types.StorageQuota{
		Used:             databaseSize,
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

// countRecordsPerBucket counts records in each bucket.
func (q *QuotaManager) countRecordsPerBucket(ctx context.Context) (map[string]int64, error) {
	bucketCounts := make(map[string]int64)
	buckets := AllStandardBuckets()

	for _, bucketName := range buckets {
		if !q.provider.BucketExists(ctx, bucketName) {
			continue
		}

		keyValues, err := q.provider.List(ctx, bucketName, nil)
		if err != nil {
			// Log error but continue with other buckets
			if q.logger != nil {
				q.logger.Warn("Failed to count records in bucket",
					zap.String("bucket", bucketName),
					zap.Error(err))
			}
			continue
		}

		bucketCounts[bucketName] = int64(len(keyValues))
	}

	return bucketCounts, nil
}

// CheckQuotaBeforeWrite checks if a write operation should be allowed based on quota.
// Returns an error if quota is exceeded.
func (q *QuotaManager) CheckQuotaBeforeWrite(ctx context.Context, bucketName string, recordSize int64) error {
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

	// Check per-bucket record limit
	bucketLimit := q.config.GetBucketLimit(bucketName)
	q.mu.RLock()
	bucketCount := q.bucketCounts[bucketName]
	q.mu.RUnlock()

	if bucketCount >= int64(bucketLimit) {
		return fmt.Errorf("bucket %s has reached record limit (%d records)", bucketName, bucketLimit)
	}

	// Calculate usage percentage (avoid division by zero)
	if quota.Limit == 0 {
		// If limit is 0, allow the write (shouldn't happen in practice)
		return nil
	}
	usagePercent := int((quota.Used * 100) / quota.Limit)

	// Check thresholds
	if usagePercent >= quota.FullThreshold {
		// >95%: reject write operations
		return fmt.Errorf("storage quota exceeded: %d%% >= %d%% (limit: %d MB)",
			usagePercent, quota.FullThreshold, q.config.MaxSizeMB)
	}

	// 90-95%: throttle large records (allow small/critical operations)
	if usagePercent >= 90 && recordSize > 1024*1024 { // 1 MB
		if q.logger != nil {
			q.logger.Warn("Throttling large record write due to high quota usage",
				zap.Int("usage_percent", usagePercent),
				zap.String("bucket", bucketName),
				zap.Int64("record_size", recordSize))
		}
		return fmt.Errorf("storage quota high (%d%%): rejecting large record (%d bytes)",
			usagePercent, recordSize)
	}

	// 80-90%: emit warning but allow operation
	if usagePercent >= quota.WarningThreshold && usagePercent < 90 {
		if q.logger != nil {
			q.logger.Warn("Storage quota warning",
				zap.Int("usage_percent", usagePercent),
				zap.Int("warning_threshold", quota.WarningThreshold),
				zap.String("bucket", bucketName))
		}
		// Emit storage.warning event
		if q.eventEmitter != nil {
			usagePercentFloat := float64(usagePercent)
			q.eventEmitter.EmitStorageEvent("storage.warning", map[string]interface{}{
				"used_bytes":    quota.Used,
				"limit_bytes":   quota.Limit,
				"usage_percent": usagePercentFloat,
			})
		}
	}

	return nil
}

// GetBucketCounts returns the cached record counts per bucket.
func (q *QuotaManager) GetBucketCounts() map[string]int64 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Return a copy to prevent external modification
	counts := make(map[string]int64)
	for k, v := range q.bucketCounts {
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
				usagePercent := int((quota.Used * 100) / quota.Limit)

				if usagePercent >= quota.FullThreshold {
					if q.logger != nil {
						q.logger.Error("Storage quota exceeded",
							zap.Int("usage_percent", usagePercent),
							zap.Int("full_threshold", quota.FullThreshold))
					}
					// Emit storage.full event
					if q.eventEmitter != nil {
						usagePercentFloat := float64(usagePercent)
						q.eventEmitter.EmitStorageEvent("storage.full", map[string]interface{}{
							"used_bytes":    quota.Used,
							"limit_bytes":   quota.Limit,
							"usage_percent": usagePercentFloat,
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
						usagePercentFloat := float64(usagePercent)
						q.eventEmitter.EmitStorageEvent("storage.warning", map[string]interface{}{
							"used_bytes":    quota.Used,
							"limit_bytes":   quota.Limit,
							"usage_percent": usagePercentFloat,
						})
					}
				}
			}
		}
	}()
}

// GetDatabasePath returns the database file path.
// This is used to get the database file size for quota tracking.
func (q *QuotaManager) GetDatabasePath() string {
	return q.databasePath
}

// SetDatabasePath sets the database file path.
func (q *QuotaManager) SetDatabasePath(path string) {
	q.databasePath = path
}

