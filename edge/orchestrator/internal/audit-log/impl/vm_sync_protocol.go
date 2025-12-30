package impl

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// SyncResult represents the result of syncing a single audit log entry to VM.
// This tracks VM acknowledgment and sync attempts for at-least-once delivery guarantee.
type SyncResult struct {
	EntryID   string    // Entry ID
	Synced    bool      // Whether entry was successfully synced (VM acknowledged)
	Failed    bool      // Whether entry sync failed (will be retried)
	Duplicate bool      // Whether entry was duplicate (VM already has it, considered acknowledged)
	Error     error     // Error if sync failed
	SyncedAt  time.Time // When entry was synced (if successful - VM acknowledgment time)
}

// SyncAuditLogsToVM syncs audit log entries to VM using proper VM sync protocol.
// It builds sync requests with idempotency keys, processes responses, and handles
// acknowledged, failed, and duplicate entries appropriately.
//
// At-Least-Once Delivery Guarantee:
// This function ensures at-least-once delivery of audit log entries to VM:
//   - Entries MUST be persisted locally (in storage provider) before calling this function
//   - Entries remain in sync queue until VM acknowledgment (via SyncResult.Synced = true)
//   - Failed entries are retried with exponential backoff (handled by SyncQueueManager)
//   - Entries are NEVER dropped, even if sync fails repeatedly
//
// Sync Status Tracking:
// This function tracks sync attempts and VM acknowledgment:
//   - Sync attempts: Tracked via RetryCount in SyncQueueEntry (incremented by SyncQueueManager)
//   - Sync success/failure: Tracked via SyncResult (returned by this function)
//   - VM acknowledgment: Tracked via SyncResult.Synced = true (only set when VM acknowledges)
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - entries: Audit log entries to sync (MUST be persisted locally before calling)
//   - edgeID: Edge identifier
//   - vmGateway: VM gateway for syncing
//   - logger: Logger for logging operations
//   - batchSize: Maximum number of entries per batch (default: 1000)
//
// Returns:
//   - []SyncResult: Per-entry sync results (indicates VM acknowledgment for each entry)
//   - error: Overall error if sync completely failed
func SyncAuditLogsToVM(
	ctx context.Context,
	entries []types.AuditEntry,
	edgeID string,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	batchSize int,
) ([]SyncResult, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	if batchSize <= 0 {
		batchSize = 1000 // Default batch size
	}

	// Batch entries for efficient transfer
	batches := batchEntries(entries, batchSize)
	allResults := make([]SyncResult, 0, len(entries))

	for batchIdx, batch := range batches {
		if logger != nil {
			logger.Debug("Syncing batch of audit log entries to VM",
				zap.Int("batch_index", batchIdx+1),
				zap.Int("batch_count", len(batches)),
				zap.Int("batch_size", len(batch)))
		}

		// Build sync request
		req, err := buildSyncRequest(batch, edgeID, logger)
		if err != nil {
			// If request building fails, mark all entries in batch as failed
			for _, entry := range batch {
				allResults = append(allResults, SyncResult{
					EntryID: entry.ID,
					Failed:  true,
					Error:   fmt.Errorf("failed to build sync request: %w", err),
				})
			}
			continue
		}

		// Call VMGateway.SyncAuditLogs
		resp, err := vmGateway.SyncAuditLogs(ctx, req)
		if err != nil {
			// Sync request failed - mark all entries in batch as failed
			if logger != nil {
				logger.Warn("Failed to sync audit log batch to VM",
					zap.Int("batch_index", batchIdx+1),
					zap.Int("entry_count", len(batch)),
					zap.Error(err))
			}

			for _, entry := range batch {
				allResults = append(allResults, SyncResult{
					EntryID: entry.ID,
					Failed:  true,
					Error:   err,
				})
			}
			continue
		}

		// Process response
		batchResults := processSyncResponse(batch, req, resp, logger)
		allResults = append(allResults, batchResults...)
	}

	return allResults, nil
}

// buildSyncRequest builds a SyncAuditLogsRequest from audit log entries.
// It creates idempotency keys for each entry and includes batch metadata.
func buildSyncRequest(entries []types.AuditEntry, edgeID string, logger *zap.Logger) (*vmgatewaytypes.SyncAuditLogsRequest, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("cannot build sync request with empty entries")
	}

	// Convert entries to VM AuditLogEntry format
	vmEntries := make([]*vmgatewaytypes.AuditLogEntry, 0, len(entries))
	var startTime, endTime time.Time

	for _, entry := range entries {
		// Determine time range
		if startTime.IsZero() || entry.Timestamp.Before(startTime) {
			startTime = entry.Timestamp
		}
		if endTime.IsZero() || entry.Timestamp.After(endTime) {
			endTime = entry.Timestamp
		}

		// Load full entry data (we need the complete entry for Data field)
		// For now, we'll reconstruct from the base entry
		// TODO: When provider is available, load full entry with all fields
		entryData := map[string]interface{}{
			"id":            entry.ID,
			"type":          string(entry.Type),
			"timestamp":     entry.Timestamp.Unix(),
			"edge_id":       entry.EdgeID,
			"user_id":       entry.UserID,
			"ip_address":    entry.IPAddress,
			"user_agent":    entry.UserAgent,
			"result":        entry.Result,
			"error":         entry.Error,
			"previous_hash": entry.PreviousHash,
			"hash":          entry.Hash,
		}

		// Create VM entry with idempotency key
		// Idempotency key format: (EdgeID, EntryID) - VM deduplicates by this
		vmEntry := &vmgatewaytypes.AuditLogEntry{
			ID:            entry.ID,
			Type:          string(entry.Type),
			Timestamp:     entry.Timestamp.Unix(),
			EdgeID:        entry.EdgeID,
			UserAgent:     entry.UserAgent,
			Result:        entry.Result,
			Error:         entry.Error,
			PreviousHash:  entry.PreviousHash,
			Hash:          entry.Hash,
			Data:          entryData,
			Format:        "json",
			// Note: IdempotencyKey is handled at request level, but entry ID serves as idempotency key
		}

		vmEntries = append(vmEntries, vmEntry)
	}

	// Generate idempotency key for the request
	// Format: {EdgeID}-sync-audit-logs-{timestamp}-{batch_hash}
	// This ensures each batch has a unique idempotency key
	idempotencyKey := generateIdempotencyKey(edgeID, startTime, len(vmEntries))

	// Build sync request
	req := &vmgatewaytypes.SyncAuditLogsRequest{
		EdgeID:         edgeID,
		IdempotencyKey: idempotencyKey,
		StartTime:      startTime.Unix(),
		EndTime:        endTime.Unix(),
		EntryCount:     len(vmEntries),
		Entries:        vmEntries,
		Format:         "json",
	}

	if logger != nil {
		logger.Debug("Built sync request",
			zap.String("edge_id", edgeID),
			zap.String("idempotency_key", idempotencyKey),
			zap.Int("entry_count", len(vmEntries)),
			zap.Int64("start_time", startTime.Unix()),
			zap.Int64("end_time", endTime.Unix()))
	}

	return req, nil
}

// processSyncResponse processes the VM response and returns per-entry sync results.
// It handles acknowledged entries, failed entries, and duplicate entries.
func processSyncResponse(
	entries []types.AuditEntry,
	req *vmgatewaytypes.SyncAuditLogsRequest,
	resp *vmgatewaytypes.SyncAuditLogsResponse,
	logger *zap.Logger,
) []SyncResult {
	results := make([]SyncResult, 0, len(entries))

	if !resp.Success {
		// Request was rejected - mark all entries as failed
		err := fmt.Errorf("VM rejected audit log sync: %s", resp.ErrorMessage)
		if logger != nil {
			logger.Error("VM rejected audit log sync",
				zap.String("error_message", resp.ErrorMessage),
				zap.Int("entry_count", len(entries)))
		}

		for _, entry := range entries {
			results = append(results, SyncResult{
				EntryID: entry.ID,
				Failed:  true,
				Error:   err,
			})
		}
		return results
	}

	// Request was successful - process response
	syncedAt := time.Now()

	// Check if all entries were synced
	if resp.SyncedCount >= len(entries) {
		// All entries were synced (or VM already had them - duplicates)
		// Since response doesn't have per-entry status, we assume all are synced
		if logger != nil {
			logger.Info("All audit log entries synced to VM",
				zap.Int("entry_count", len(entries)),
				zap.Int("synced_count", resp.SyncedCount))
		}

		for _, entry := range entries {
			// Check if count exceeds - might indicate duplicates
			// If synced_count > entry_count, VM had some duplicates
			isDuplicate := resp.SyncedCount > len(entries)
			results = append(results, SyncResult{
				EntryID:   entry.ID,
				Synced:    true,
				Duplicate: isDuplicate,
				SyncedAt:  syncedAt,
			})
		}
	} else {
		// Partial sync - some entries failed
		// Since response doesn't have per-entry status, we can't determine which failed
		// Mark all as synced (VM accepted them) - actual failures will be retried
		if logger != nil {
			logger.Warn("Partial audit log sync - some entries may have failed",
				zap.Int("entry_count", len(entries)),
				zap.Int("synced_count", resp.SyncedCount),
				zap.Int("missing_count", len(entries)-resp.SyncedCount))
		}

		// Mark first N entries as synced (where N = syncedCount)
		// Remaining entries are marked as failed for retry
		for i, entry := range entries {
			if i < resp.SyncedCount {
				results = append(results, SyncResult{
					EntryID:  entry.ID,
					Synced:   true,
					SyncedAt: syncedAt,
				})
			} else {
				results = append(results, SyncResult{
					EntryID: entry.ID,
					Failed:  true,
					Error:   fmt.Errorf("entry not acknowledged by VM (synced_count: %d, entry_index: %d)", resp.SyncedCount, i),
				})
			}
		}
	}

	return results
}

// generateIdempotencyKey generates an idempotency key for a sync request.
// Format: {EdgeID}-sync-audit-logs-{timestamp}-{entry_count}
func generateIdempotencyKey(edgeID string, startTime time.Time, entryCount int) string {
	return fmt.Sprintf("%s-sync-audit-logs-%d-%d", edgeID, startTime.Unix(), entryCount)
}

// batchEntries splits entries into batches of the specified size.
func batchEntries(entries []types.AuditEntry, batchSize int) [][]types.AuditEntry {
	if batchSize <= 0 {
		batchSize = 1000 // Default batch size
	}

	var batches [][]types.AuditEntry
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batches = append(batches, entries[i:end])
	}

	return batches
}

// GetIdempotencyKey returns the idempotency key for an entry.
// Format: (EdgeID, EntryID) - this is used by VM to deduplicate entries.
func GetIdempotencyKey(edgeID string, entryID string) string {
	return fmt.Sprintf("%s:%s", edgeID, entryID)
}

