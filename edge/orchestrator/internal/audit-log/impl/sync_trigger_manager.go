package impl

import (
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	"go.uber.org/zap"
)

// SyncTriggerManager manages sync triggers based on time-based, count-based, or hybrid modes.
// It tracks pending entry count and last sync time to determine when sync should be triggered.
type SyncTriggerManager struct {
	config         *types.AuditLogConfig
	logger         *zap.Logger
	mu             sync.RWMutex
	pendingCount   int           // Number of entries pending sync
	lastSyncTime   time.Time     // Time of last successful sync
	syncInterval   time.Duration // Interval for time-based sync (from config)
	syncBatchSize  int           // Batch size / count threshold (from config)
	triggerMode    string        // "time_based", "count_based", or "hybrid"
}

// NewSyncTriggerManager creates a new sync trigger manager.
func NewSyncTriggerManager(config *types.AuditLogConfig, logger *zap.Logger) *SyncTriggerManager {
	if config == nil {
		// Create default config if not provided
		config = &types.AuditLogConfig{}
		config.Validate()
	}

	return &SyncTriggerManager{
		config:        config,
		logger:        logger,
		pendingCount:  0,
		lastSyncTime:  time.Time{}, // Zero time means no sync yet
		syncInterval:  config.SyncInterval,
		syncBatchSize: config.SyncBatchSize,
		triggerMode:   config.SyncTriggerMode,
	}
}

// ShouldSync returns true if sync should be triggered based on the configured trigger mode.
// Hybrid mode: returns true if either time threshold OR count threshold is reached.
// Time-based mode: returns true if sync interval has passed since last sync.
// Count-based mode: returns true if pending count >= batch size.
func (m *SyncTriggerManager) ShouldSync() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch m.triggerMode {
	case "time_based":
		return m.shouldSyncTimeBasedLocked()

	case "count_based":
		return m.shouldSyncCountBasedLocked()

	case "hybrid":
		// Hybrid mode: trigger on either condition (time OR count)
		return m.shouldSyncTimeBasedLocked() || m.shouldSyncCountBasedLocked()

	default:
		// Default to hybrid mode if unknown mode
		if m.logger != nil {
			m.logger.Warn("Unknown sync trigger mode, defaulting to hybrid",
				zap.String("mode", m.triggerMode))
		}
		return m.shouldSyncTimeBasedLocked() || m.shouldSyncCountBasedLocked()
	}
}

// shouldSyncTimeBasedLocked checks if time-based sync should be triggered (assumes lock is held).
func (m *SyncTriggerManager) shouldSyncTimeBasedLocked() bool {
	if m.lastSyncTime.IsZero() {
		// No sync yet - trigger immediately if interval has passed (initial sync)
		return true
	}

	timeSinceLastSync := time.Since(m.lastSyncTime)
	return timeSinceLastSync >= m.syncInterval
}

// shouldSyncCountBasedLocked checks if count-based sync should be triggered (assumes lock is held).
func (m *SyncTriggerManager) shouldSyncCountBasedLocked() bool {
	return m.pendingCount >= m.syncBatchSize
}

// RecordPendingEntry increments the pending entry count.
// This should be called when an entry is added to the sync queue.
func (m *SyncTriggerManager) RecordPendingEntry() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pendingCount++

	if m.logger != nil {
		m.logger.Debug("Recorded pending entry",
			zap.Int("pending_count", m.pendingCount),
			zap.Int("batch_size", m.syncBatchSize))
	}
}

// RecordSync resets the pending count and updates the last sync time.
// This should be called after a successful sync operation.
func (m *SyncTriggerManager) RecordSync() {
	m.mu.Lock()
	defer m.mu.Unlock()

	previousCount := m.pendingCount
	m.pendingCount = 0
	m.lastSyncTime = time.Now()

	if m.logger != nil {
		m.logger.Debug("Recorded sync",
			zap.Int("synced_count", previousCount),
			zap.Time("last_sync_time", m.lastSyncTime))
	}
}

// GetPendingCount returns the current number of pending entries.
func (m *SyncTriggerManager) GetPendingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingCount
}

// GetLastSyncTime returns the time of the last sync.
func (m *SyncTriggerManager) GetLastSyncTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSyncTime
}

// Reset resets the trigger manager state (useful for testing or recovery).
func (m *SyncTriggerManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pendingCount = 0
	m.lastSyncTime = time.Time{}
}

