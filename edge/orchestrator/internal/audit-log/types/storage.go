package types

import "time"

// SyncStatus represents the sync status of an audit log entry.
type SyncStatus int

const (
	// SyncStatusPending indicates the entry has not yet been synced to VM.
	SyncStatusPending SyncStatus = iota

	// SyncStatusSyncing indicates the entry is currently being synced to VM.
	SyncStatusSyncing

	// SyncStatusSynced indicates the entry has been successfully synced to VM.
	SyncStatusSynced

	// SyncStatusFailed indicates the entry sync failed and will be retried.
	SyncStatusFailed
)

// String returns the string representation of SyncStatus.
func (s SyncStatus) String() string {
	switch s {
	case SyncStatusPending:
		return "pending"
	case SyncStatusSyncing:
		return "syncing"
	case SyncStatusSynced:
		return "synced"
	case SyncStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// AuditEntryMetadata contains metadata about an audit log entry for storage and sync tracking.
type AuditEntryMetadata struct {
	// ID is the unique identifier of the audit log entry.
	ID string

	// Timestamp is when the audit log entry was created.
	Timestamp time.Time

	// Hash is the hash of this entry (for tamper-proofing).
	Hash string

	// PreviousHash is the hash of the previous entry in the chain (for tamper-proofing).
	PreviousHash string

	// Synced indicates whether the entry has been synced to VM.
	// This field is used for tracking sync status.
	Synced bool

	// SyncStatus is the current sync status of the entry.
	SyncStatus SyncStatus
}

// SyncQueueEntry represents an entry in the sync queue for failed or pending syncs.
// This is used to track entries that need to be synced to VM and manage retry logic.
// 
// At-Least-Once Delivery Guarantee:
// - Entries are persisted locally before sync attempt (stored in provider first)
// - Entries remain in queue until VM acknowledgment (only removed on MarkSynced)
// - Retry on failure with exponential backoff (via MarkFailed)
// - Never dropped (even if queue is full, pause operations instead)
type SyncQueueEntry struct {
	// EntryID is the unique identifier of the audit log entry.
	EntryID string

	// EntryData is the serialized audit entry (JSON bytes).
	// This contains the complete audit entry data for retry attempts.
	EntryData []byte

	// QueuedAt is when the entry was added to the sync queue.
	QueuedAt time.Time

	// RetryCount is the number of sync attempts made for this entry.
	// Incremented each time MarkFailed is called (tracks sync attempts).
	RetryCount int

	// LastRetryTime is when the last retry attempt was made.
	// Updated when entry is marked as syncing (in DequeueEntries).
	LastRetryTime time.Time

	// NextRetryTime is when the next retry attempt should be made (calculated with exponential backoff).
	// Set by MarkFailed when retry is needed, or immediately for new entries.
	NextRetryTime time.Time

	// SyncStatus is the current sync status of this queue entry.
	// Tracks: pending -> syncing -> synced (or failed -> retry)
	SyncStatus SyncStatus

	// FirstSyncAttempt is when the first sync attempt was made.
	// This tracks when sync attempts started for this entry.
	FirstSyncAttempt time.Time

	// LastVMResponseTime is when VM last responded (acknowledged or rejected) for this entry.
	// This tracks VM acknowledgment timing.
	LastVMResponseTime time.Time

	// VMAcknowledged indicates whether VM has acknowledged this entry.
	// Set to true only when VM explicitly acknowledges via MarkSynced.
	VMAcknowledged bool
}

