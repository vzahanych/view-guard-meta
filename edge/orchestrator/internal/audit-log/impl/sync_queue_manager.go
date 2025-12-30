package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	"go.uber.org/zap"
)

// SyncQueueManager manages the sync queue for audit log entries that need to be synced to VM.
// It tracks queued entries, manages retry logic with exponential backoff, and handles queue state persistence.
type SyncQueueManager struct {
	config     *types.SyncQueueConfig
	logger     *zap.Logger
	mu         sync.RWMutex
	queue      map[string]*types.SyncQueueEntry // In-memory queue: EntryID -> SyncQueueEntry
	queueOrder []string                          // Maintain order of entries for FIFO processing
	provider   types.AuditLogProvider            // Provider for persistence (Epic 8)
	// TODO: Add event emitter for alerts (will be implemented when event-bus integration is added)
	// eventEmitter EventEmitter
}

// NewSyncQueueManager creates a new sync queue manager.
func NewSyncQueueManager(config *types.SyncQueueConfig, logger *zap.Logger) *SyncQueueManager {
	if config == nil {
		config = &types.SyncQueueConfig{}
		config.Validate()
	}

	return &SyncQueueManager{
		config:     config,
		logger:     logger,
		queue:      make(map[string]*types.SyncQueueEntry),
		queueOrder: make([]string, 0),
		provider:   nil, // Will be set via SetProvider when available
	}
}

// SetProvider sets the audit log provider for persistence.
func (m *SyncQueueManager) SetProvider(provider types.AuditLogProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = provider
}

// EnqueueEntry adds an audit log entry to the sync queue.
// If the queue is full, it returns ErrQueueFull.
// This method never drops entries - if the queue is full, it returns an error and operations should be paused.
func (m *SyncQueueManager) EnqueueEntry(ctx context.Context, entry types.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize entry to JSON bytes
	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	// Check if entry already exists in queue
	if _, exists := m.queue[entry.ID]; exists {
		// Entry already in queue, update it
		m.queue[entry.ID].EntryData = entryData
		m.queue[entry.ID].QueuedAt = time.Now()
		if m.logger != nil {
			m.logger.Debug("Updated existing queue entry",
				zap.String("entry_id", entry.ID),
				zap.Int("queue_depth", len(m.queue)))
		}
		return nil
	}

	// Check queue size limit
	currentQueueSize := len(m.queue)
	if currentQueueSize >= m.config.MaxQueueSize {
		// Queue is full - emit critical alert and return error
		// NEVER drop audit records - caller should pause operations instead
		if m.logger != nil {
			m.logger.Error("Sync queue is full - operations should be paused",
				zap.Int("queue_size", currentQueueSize),
				zap.Int("max_queue_size", m.config.MaxQueueSize),
				zap.String("entry_id", entry.ID))
		}
		// TODO: Emit event: audit_log.queue_full (will be implemented when event-bus integration is added)
		// m.emitEvent("audit_log.queue_full", map[string]interface{}{
		// 	"queue_size": currentQueueSize,
		// 	"max_queue_size": m.config.MaxQueueSize,
		// })
		return types.ErrQueueFull
	}

	// Create queue entry
	now := time.Now()
	queueEntry := &types.SyncQueueEntry{
		EntryID:            entry.ID,
		EntryData:          entryData,
		QueuedAt:           now,
		RetryCount:         0,
		SyncStatus:         types.SyncStatusPending,
		NextRetryTime:      now, // Ready for immediate sync
		FirstSyncAttempt:   time.Time{}, // Will be set on first sync attempt
		LastVMResponseTime: time.Time{}, // Will be set on VM response
		VMAcknowledged:     false,       // Not yet acknowledged by VM
	}

	// Add to queue
	m.queue[entry.ID] = queueEntry
	m.queueOrder = append(m.queueOrder, entry.ID)

	if m.logger != nil {
		m.logger.Debug("Entry added to sync queue",
			zap.String("entry_id", entry.ID),
			zap.Int("queue_depth", len(m.queue)),
			zap.Float64("queue_usage_percent", m.getQueueUsagePercentLocked()))
	}

	// Check if queue was previously full and now has space (for queue_resumed event)
	if currentQueueSize == m.config.MaxQueueSize-1 {
		// Queue was at max-1, now at max, so this is the last entry before full
		// But since we're at max now, we don't emit resumed yet
	} else if currentQueueSize < m.config.MaxQueueSize {
		// Queue has space - if we were previously full, emit resumed event
		// TODO: Track previous state and emit audit_log.queue_resumed if transitioning from full to not-full
		// This will be implemented when event-bus integration is added
	}

	// Persist queue entry to provider if available
	if m.provider != nil {
		// Try to use SaveSyncQueueEntry if provider implements it (meta-storage provider)
		if metaProvider, ok := m.provider.(interface {
			SaveSyncQueueEntry(ctx context.Context, entry types.SyncQueueEntry) error
		}); ok {
			if err := metaProvider.SaveSyncQueueEntry(ctx, *queueEntry); err != nil {
				m.logger.Warn("Failed to persist queue entry to provider", zap.Error(err))
				// Continue - entry is still in memory queue
			}
		}
	}

	return nil
}

// DequeueEntries retrieves entries ready for sync (pending status and retry time reached).
// It marks entries as syncing and returns up to the specified limit.
func (m *SyncQueueManager) DequeueEntries(ctx context.Context, limit int) ([]types.SyncQueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var readyEntries []types.SyncQueueEntry
	var entryIDsToMark []string

	// Find entries ready for sync (pending or failed status, and retry time reached)
	for _, entryID := range m.queueOrder {
		entry := m.queue[entryID]
		if entry == nil {
			continue
		}

		// Skip entries that are currently syncing
		if entry.SyncStatus == types.SyncStatusSyncing {
			continue
		}

		// Check if entry is ready for sync (pending or failed status, and retry time reached)
		if entry.SyncStatus == types.SyncStatusPending || entry.SyncStatus == types.SyncStatusFailed {
			if now.After(entry.NextRetryTime) || now.Equal(entry.NextRetryTime) {
				// Mark as syncing (this is a sync attempt)
				entry.SyncStatus = types.SyncStatusSyncing
				entry.LastRetryTime = now

				// Track first sync attempt
				if entry.FirstSyncAttempt.IsZero() {
					entry.FirstSyncAttempt = now
				}

				// Create a copy for return
				entryCopy := *entry
				readyEntries = append(readyEntries, entryCopy)
				entryIDsToMark = append(entryIDsToMark, entryID)

				if len(readyEntries) >= limit {
					break
				}
			}
		}
	}

	if len(readyEntries) > 0 && m.logger != nil {
		m.logger.Debug("Dequeued entries for sync",
			zap.Int("count", len(readyEntries)),
			zap.Int("remaining_queue_depth", len(m.queue)))
	}

	// Persist updated sync status to provider if available
	if m.provider != nil {
		for _, entryID := range entryIDsToMark {
			if entry, exists := m.queue[entryID]; exists {
				// Try to use SaveSyncQueueEntry to update status
				if metaProvider, ok := m.provider.(interface {
					SaveSyncQueueEntry(ctx context.Context, entry types.SyncQueueEntry) error
				}); ok {
					if err := metaProvider.SaveSyncQueueEntry(ctx, *entry); err != nil {
						m.logger.Warn("Failed to persist queue entry status update",
							zap.String("entry_id", entryID),
							zap.Error(err))
					}
				}
			}
		}
	}

	return readyEntries, nil
}

// MarkSynced marks an entry as successfully synced and removes it from the queue.
// This is called only after VM has acknowledged the entry (via SyncAuditLogsResponse).
// This ensures at-least-once delivery: entries remain in queue until VM acknowledgment.
func (m *SyncQueueManager) MarkSynced(ctx context.Context, entryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.queue[entryID]
	if !exists {
		return nil // Entry not in queue, consider it already synced
	}

	// Mark as synced (VM acknowledged this entry)
	now := time.Now()
	entry.SyncStatus = types.SyncStatusSynced
	entry.LastVMResponseTime = now
	entry.VMAcknowledged = true

	// Remove from queue
	delete(m.queue, entryID)
	
	// Remove from order slice
	for i, id := range m.queueOrder {
		if id == entryID {
			m.queueOrder = append(m.queueOrder[:i], m.queueOrder[i+1:]...)
			break
		}
	}

	if m.logger != nil {
		m.logger.Debug("Entry marked as synced and removed from queue",
			zap.String("entry_id", entryID),
			zap.Int("remaining_queue_depth", len(m.queue)))
	}

	// Check if queue was previously full and now has space (for queue_resumed event)
	previousSize := len(m.queue) + 1
	if previousSize == m.config.MaxQueueSize && len(m.queue) < m.config.MaxQueueSize {
		// Queue transitioned from full to not-full
		if m.logger != nil {
			m.logger.Info("Sync queue has space again - operations can resume",
				zap.Int("previous_size", previousSize),
				zap.Int("current_size", len(m.queue)),
				zap.Int("max_queue_size", m.config.MaxQueueSize))
		}
		// TODO: Emit event: audit_log.queue_resumed (will be implemented when event-bus integration is added)
		// m.emitEvent("audit_log.queue_resumed", map[string]interface{}{
		// 	"previous_size": previousSize,
		// 	"current_size": len(m.queue),
		// 	"max_queue_size": m.config.MaxQueueSize,
		// })
	}

	// Remove from persistent storage if provider is available
	if m.provider != nil {
		if metaProvider, ok := m.provider.(interface {
			DeleteSyncQueueEntry(ctx context.Context, entryID string) error
		}); ok {
			if err := metaProvider.DeleteSyncQueueEntry(ctx, entryID); err != nil {
				m.logger.Warn("Failed to remove queue entry from persistent storage",
					zap.String("entry_id", entryID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// MarkFailed marks an entry as failed and calculates the next retry time with exponential backoff.
// If max retries are exceeded, the entry is kept in the queue but marked as failed (never dropped).
// This ensures at-least-once delivery: entries are retried until VM acknowledgment or max retries.
func (m *SyncQueueManager) MarkFailed(ctx context.Context, entryID string, syncError error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.queue[entryID]
	if !exists {
		return fmt.Errorf("entry not found in queue: %s", entryID)
	}

	// Increment retry count (track sync attempts)
	entry.RetryCount++
	now := time.Now()
	entry.LastRetryTime = now
	entry.LastVMResponseTime = now // VM responded (with failure)
	entry.VMAcknowledged = false   // Not acknowledged (sync failed)

	// Check if max retries exceeded
	if entry.RetryCount > m.config.MaxRetries {
		// Max retries exceeded - mark as failed but keep in queue (NEVER drop)
		entry.SyncStatus = types.SyncStatusFailed
		// Don't calculate next retry time - entry will remain failed
		
		if m.logger != nil {
			m.logger.Error("Entry exceeded max retries but kept in queue (never dropped)",
				zap.String("entry_id", entryID),
				zap.Int("retry_count", entry.RetryCount),
				zap.Int("max_retries", m.config.MaxRetries),
				zap.Error(syncError))
		}
		// TODO: Emit critical alert event (will be implemented when event-bus integration is added)
		// m.emitEvent("audit_log.sync_failed_max_retries", map[string]interface{}{
		// 	"entry_id": entryID,
		// 	"retry_count": entry.RetryCount,
		// 	"max_retries": m.config.MaxRetries,
		// 	"error": syncError.Error(),
		// })
	} else {
		// Calculate next retry time with exponential backoff
		backoffDuration := calculateExponentialBackoff(entry.RetryCount, m.config.RetryBackoff)
		entry.NextRetryTime = time.Now().Add(backoffDuration)
		entry.SyncStatus = types.SyncStatusFailed

		if m.logger != nil {
			m.logger.Warn("Entry sync failed, will retry",
				zap.String("entry_id", entryID),
				zap.Int("retry_count", entry.RetryCount),
				zap.Int("max_retries", m.config.MaxRetries),
				zap.Duration("next_retry_in", backoffDuration),
				zap.Error(syncError))
		}
		// TODO: Emit event: audit_log.sync_failed (will be implemented when event-bus integration is added)
		// m.emitEvent("audit_log.sync_failed", map[string]interface{}{
		// 	"entry_id": entryID,
		// 	"retry_count": entry.RetryCount,
		// 	"next_retry_time": entry.NextRetryTime,
		// 	"error": syncError.Error(),
		// })
	}

	// TODO: Persist updated entry to provider (will be implemented in Epic 8)
	// if err := m.provider.UpdateSyncQueueEntry(ctx, entry); err != nil {
	// 	return fmt.Errorf("failed to persist queue entry update: %w", err)
	// }

	return nil
}

// GetQueueDepth returns the current number of entries in the sync queue.
func (m *SyncQueueManager) GetQueueDepth() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.queue)
}

// GetQueueUsagePercent returns the percentage of queue capacity used (0-100).
func (m *SyncQueueManager) GetQueueUsagePercent() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getQueueUsagePercentLocked()
}

// getQueueUsagePercentLocked calculates queue usage percentage (assumes lock is held).
func (m *SyncQueueManager) getQueueUsagePercentLocked() float64 {
	if m.config.MaxQueueSize == 0 {
		return 0
	}
	return float64(len(m.queue)) / float64(m.config.MaxQueueSize) * 100
}

// IsQueueFull returns true if the queue is at or above its maximum capacity.
func (m *SyncQueueManager) IsQueueFull() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.queue) >= m.config.MaxQueueSize
}

// LoadQueueFromProvider loads the queue state from persistent storage.
// This is called during service initialization for crash recovery.
// TODO: Implement when provider is available in Epic 8
func (m *SyncQueueManager) LoadQueueFromProvider(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: Load queue entries from provider in Epic 8
	// entries, err := m.provider.ListSyncQueueEntries(ctx)
	// if err != nil {
	// 	return fmt.Errorf("failed to load queue from provider: %w", err)
	// }
	//
	// for _, entry := range entries {
	// 	m.queue[entry.EntryID] = &entry
	// 	m.queueOrder = append(m.queueOrder, entry.EntryID)
	// }

	if m.logger != nil {
		m.logger.Info("Queue loaded from provider (stub - will be implemented in Epic 8)",
			zap.Int("loaded_entries", 0))
	}

	return nil
}

// calculateExponentialBackoff calculates the backoff duration for a retry attempt.
// Uses exponential backoff: baseDuration * 2^(retryCount-1)
// Adds jitter (±25%) to prevent thundering herd.
func calculateExponentialBackoff(retryCount int, baseDuration time.Duration) time.Duration {
	if retryCount <= 0 {
		return baseDuration
	}

	// Exponential backoff: baseDuration * 2^(retryCount-1)
	exponent := math.Pow(2, float64(retryCount-1))
	backoff := float64(baseDuration) * exponent

	// Add jitter (±25%)
	// TODO: Use crypto/rand for production, math/rand for now
	jitter := backoff * 0.25 * (2*rand.Float64() - 1) // -0.25 to +0.25
	backoffWithJitter := backoff + jitter

	// Cap at 1 hour maximum
	maxBackoff := float64(1 * time.Hour)
	if backoffWithJitter > maxBackoff {
		backoffWithJitter = maxBackoff
	}

	return time.Duration(backoffWithJitter)
}

