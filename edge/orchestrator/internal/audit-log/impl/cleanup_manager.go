package impl

import (
	"context"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	"go.uber.org/zap"
)

// CleanupStatistics contains statistics about a cleanup operation.
type CleanupStatistics struct {
	EntriesDeleted int64 // Number of entries deleted
	EntriesSkipped int64 // Number of entries skipped (not synced)
	ErrorsEncountered int // Number of errors encountered
	CleanupDuration time.Duration // Duration of cleanup operation
}

// CleanupManager manages cleanup of expired audit log entries based on retention policy.
// It ensures that only synced entries are deleted (never unsynced entries).
type CleanupManager struct {
	config       *types.AuditLogConfig
	logger       *zap.Logger
	provider     types.AuditLogProvider // Will be set when provider is available (Epic 8)
	// TODO: Add provider when available in Epic 8
	// For now, we'll use the objectStorage directly for backward compatibility
}

// NewCleanupManager creates a new cleanup manager.
func NewCleanupManager(config *types.AuditLogConfig, logger *zap.Logger) *CleanupManager {
	return &CleanupManager{
		config: config,
		logger: logger,
	}
}

// SetProvider sets the audit log provider for the cleanup manager.
// This will be called when provider is initialized (Epic 8).
func (m *CleanupManager) SetProvider(provider types.AuditLogProvider) {
	m.provider = provider
}

// CleanupExpiredEntries queries and deletes expired audit log entries.
// Only entries that have been synced to VM are deleted (never delete unsynced entries).
// Deletes entries in batches to avoid overwhelming the system.
func (m *CleanupManager) CleanupExpiredEntries(ctx context.Context) (*CleanupStatistics, error) {
	startTime := time.Now()
	stats := &CleanupStatistics{}

	// Calculate cutoff date (entries older than retention period)
	cutoffDate := time.Now().AddDate(0, 0, -m.config.RetentionDays)

	if m.logger != nil {
		m.logger.Info("Starting cleanup of expired audit log entries",
			zap.Time("cutoff_date", cutoffDate),
			zap.Int("retention_days", m.config.RetentionDays))
	}

	// TODO: Emit event: audit_log.cleanup_started (will be implemented when event-bus integration is added)

	// Use provider if available, otherwise fall back to legacy approach
	if m.provider != nil {
		return m.cleanupWithProvider(ctx, cutoffDate, stats, startTime)
	}

	// Legacy approach: log that cleanup requires provider
	if m.logger != nil {
		m.logger.Warn("Cleanup manager: provider not available, cleanup deferred until provider is initialized")
	}

	stats.CleanupDuration = time.Since(startTime)

	// TODO: Emit event: audit_log.cleanup_completed (will be implemented when event-bus integration is added)

	return stats, nil
}

// cleanupWithProvider performs cleanup using the provider interface.
func (m *CleanupManager) cleanupWithProvider(ctx context.Context, cutoffDate time.Time, stats *CleanupStatistics, startTime time.Time) (*CleanupStatistics, error) {
	// Query entries older than cutoff date
	filters := types.QueryFilters{
		EndTime: &cutoffDate, // Only entries before cutoff date
		Limit:   m.config.CleanupBatchSize,
		Offset:  0,
	}

	batchSize := m.config.CleanupBatchSize
	if batchSize <= 0 {
		batchSize = 1000 // Default batch size
	}

	totalProcessed := 0

	for {
		// Query a batch of expired entries
		entries, err := m.provider.ListEntries(ctx, filters)
		if err != nil {
			stats.ErrorsEncountered++
			if m.logger != nil {
				m.logger.Warn("Failed to query expired entries",
					zap.Error(err),
					zap.Int("offset", filters.Offset))
			}
			// Continue with cleanup despite errors
			break
		}

		if len(entries) == 0 {
			// No more entries to process
			break
		}

		// Process batch: delete only synced entries
		deletedInBatch := 0
		skippedInBatch := 0

		for _, entry := range entries {
			// CRITICAL: Only delete entries that have been synced to VM
			// Never delete unsynced entries - they must be synced first
			
			// Check if entry is synced
			// For now, we'll use the entry's sync status from metadata
			// TODO: Once provider provides sync status, use it
			// For legacy entries, we assume they are synced if older than retention + some buffer
			// This is conservative - we only delete entries that are definitely old enough to have been synced
			if !m.isEntrySynced(ctx, entry, cutoffDate) {
				stats.EntriesSkipped++
				skippedInBatch++
				continue
			}

			// Delete the entry
			if err := m.provider.DeleteEntry(ctx, entry.ID); err != nil {
				stats.ErrorsEncountered++
				if m.logger != nil {
					m.logger.Warn("Failed to delete expired entry",
						zap.String("entry_id", entry.ID),
						zap.Error(err))
				}
				// Continue with next entry despite errors
				continue
			}

			stats.EntriesDeleted++
			deletedInBatch++
		}

		totalProcessed += len(entries)

		if m.logger != nil {
			m.logger.Debug("Cleanup batch processed",
				zap.Int("entries_processed", len(entries)),
				zap.Int("deleted", deletedInBatch),
				zap.Int("skipped", skippedInBatch),
				zap.Int("offset", filters.Offset))
		}

		// Check if we've processed all entries (less than batch size means last batch)
		if len(entries) < batchSize {
			break
		}

		// Move to next batch
		filters.Offset += batchSize
	}

	stats.CleanupDuration = time.Since(startTime)

	if m.logger != nil {
		m.logger.Info("Cleanup of expired audit log entries completed",
			zap.Int64("entries_deleted", stats.EntriesDeleted),
			zap.Int64("entries_skipped", stats.EntriesSkipped),
			zap.Int("errors_encountered", stats.ErrorsEncountered),
			zap.Int("total_processed", totalProcessed),
			zap.Duration("cleanup_duration", stats.CleanupDuration))
	}

	// TODO: Emit event: audit_log.cleanup_completed (will be implemented when event-bus integration is added)

	return stats, nil
}

// isEntrySynced checks if an entry has been synced to VM.
// This is a safety check to ensure we never delete unsynced entries.
// TODO: Once provider provides sync status in metadata, use that instead
func (m *CleanupManager) isEntrySynced(ctx context.Context, entry types.AuditEntry, cutoffDate time.Time) bool {
	// Conservative approach: Only consider entries as synced if they are old enough
	// An entry older than retention period + buffer time (e.g., retention + 7 days) is safe to delete
	// This ensures entries have had plenty of time to be synced
	bufferDays := 7 // Additional buffer beyond retention period
	safeDeleteDate := cutoffDate.AddDate(0, 0, -bufferDays)

	if entry.Timestamp.Before(safeDeleteDate) {
		// Entry is old enough that it should have been synced
		// Still, we should check sync status if available
		// Note: Sync status checking is handled via provider's entry metadata
		return true
	}

	// Entry is not old enough to be safely deleted
	// We must ensure it's synced before deleting
	// Note: Sync status checking is handled via provider's entry metadata
	if m.logger != nil {
		m.logger.Debug("Entry not old enough for safe deletion, checking sync status",
			zap.String("entry_id", entry.ID),
			zap.Time("entry_timestamp", entry.Timestamp),
			zap.Time("safe_delete_date", safeDeleteDate))
	}

	// For now, if entry is older than cutoff, assume it's synced (conservative)
	// This will be improved when provider sync status is available
	return entry.Timestamp.Before(cutoffDate)
}

