package metastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/zap"
)

const (
	// Bucket names for audit log storage
	bucketAuditLogs        = "audit_logs"         // Stores audit log entries (key: EntryID)
	bucketAuditLogChain    = "audit_log_chain"    // Stores hash chain state (key: "last_hash")
	bucketAuditLogSyncQueue = "audit_log_sync_queue" // Stores sync queue entries (key: EntryID)
	
	// Key names for special records
	keyLastHash = "last_hash" // Key for storing last hash in chain
)

// MetaStorageProvider implements the AuditLogProvider interface using meta-storage.
// This provider uses meta-storage buckets to store audit log entries, hash chain state, and sync queue entries.
type MetaStorageProvider struct {
	provider metastoragetypes.MetaStorageProvider
	logger   *zap.Logger
}

// NewMetaStorageProvider creates a new meta-storage provider for audit logs.
func NewMetaStorageProvider(provider metastoragetypes.MetaStorageProvider, logger *zap.Logger) (*MetaStorageProvider, error) {
	if provider == nil {
		return nil, fmt.Errorf("meta-storage provider cannot be nil")
	}

	msp := &MetaStorageProvider{
		provider: provider,
		logger:   logger,
	}

	// Initialize buckets
	ctx := context.Background()
	if err := msp.initializeBuckets(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize buckets: %w", err)
	}

	return msp, nil
}

// initializeBuckets creates all required buckets in meta-storage.
func (p *MetaStorageProvider) initializeBuckets(ctx context.Context) error {
	buckets := []string{
		bucketAuditLogs,
		bucketAuditLogChain,
		bucketAuditLogSyncQueue,
	}

	for _, bucketName := range buckets {
		exists := p.provider.BucketExists(ctx, bucketName)
		if !exists {
			if err := p.provider.CreateBucket(ctx, bucketName); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
			}
			if p.logger != nil {
				p.logger.Debug("Created audit log bucket", zap.String("bucket", bucketName))
			}
		}
	}

	return nil
}

// SaveEntry saves an audit log entry to meta-storage.
func (p *MetaStorageProvider) SaveEntry(ctx context.Context, entry types.AuditEntry) error {
	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	// Store in audit_logs bucket with EntryID as key
	if err := p.provider.Put(ctx, bucketAuditLogs, []byte(entry.ID), data); err != nil {
		return fmt.Errorf("failed to save audit entry: %w", err)
	}

	if p.logger != nil {
		p.logger.Debug("Saved audit log entry to meta-storage",
			zap.String("entry_id", entry.ID),
			zap.String("type", string(entry.Type)))
	}

	return nil
}

// LoadEntry loads an audit log entry from meta-storage by ID.
func (p *MetaStorageProvider) LoadEntry(ctx context.Context, entryID string) (*types.AuditEntry, error) {
	if entryID == "" {
		return nil, fmt.Errorf("entry ID cannot be empty")
	}

	// Get entry from audit_logs bucket
	data, err := p.provider.Get(ctx, bucketAuditLogs, []byte(entryID))
	if err != nil {
		// meta-storage provider (BoltDB) returns error "key not found" for missing keys
		// Check if error indicates missing key
		errStr := err.Error()
		if errStr == fmt.Sprintf("key not found in bucket %s", bucketAuditLogs) {
			return nil, types.ErrRecordNotFound
		}
		// Other errors should be wrapped
		return nil, fmt.Errorf("failed to load audit entry: %w", err)
	}

	if len(data) == 0 {
		return nil, types.ErrRecordNotFound
	}

	// Unmarshal entry
	var entry types.AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal audit entry: %w", err)
	}

	return &entry, nil
}

// ListEntries lists audit log entries matching the provided filters.
func (p *MetaStorageProvider) ListEntries(ctx context.Context, filters types.QueryFilters) ([]types.AuditEntry, error) {
	// List all entries from audit_logs bucket
	keyValues, err := p.provider.List(ctx, bucketAuditLogs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit entries: %w", err)
	}

	var entries []types.AuditEntry
	for _, kv := range keyValues {
		var entry types.AuditEntry
		if err := json.Unmarshal(kv.Value, &entry); err != nil {
			if p.logger != nil {
				p.logger.Warn("Failed to unmarshal audit entry",
					zap.String("entry_id", string(kv.Key)),
					zap.Error(err))
			}
			continue
		}

		// Apply filters
		if p.matchesFilters(entry, filters) {
			entries = append(entries, entry)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	// Apply limit and offset
	start := filters.Offset
	if start < 0 {
		start = 0
	}
	end := start + filters.Limit
	if filters.Limit <= 0 {
		end = len(entries)
	}
	if end > len(entries) {
		end = len(entries)
	}

	if start >= len(entries) {
		return []types.AuditEntry{}, nil
	}

	return entries[start:end], nil
}

// matchesFilters checks if an entry matches the provided filters.
func (p *MetaStorageProvider) matchesFilters(entry types.AuditEntry, filters types.QueryFilters) bool {
	// Filter by start time
	if filters.StartTime != nil && entry.Timestamp.Before(*filters.StartTime) {
		return false
	}

	// Filter by end time
	if filters.EndTime != nil && entry.Timestamp.After(*filters.EndTime) {
		return false
	}

	// Filter by entry type
	if filters.EntryType != "" && string(entry.Type) != filters.EntryType {
		return false
	}

	// Filter by user ID
	if filters.UserID != "" && entry.UserID != filters.UserID {
		return false
	}

	// Filter by IP address
	if filters.IPAddress != "" && entry.IPAddress != filters.IPAddress {
		return false
	}

	// Filter by result
	if filters.Result != "" && entry.Result != filters.Result {
		return false
	}

	// Note: ResourceType and ResourceID filters would require checking entry-specific fields
	// which is not available in the base AuditEntry. This could be extended later.

	return true
}

// DeleteEntry deletes an audit log entry from meta-storage by ID.
func (p *MetaStorageProvider) DeleteEntry(ctx context.Context, entryID string) error {
	if entryID == "" {
		return fmt.Errorf("entry ID cannot be empty")
	}

	// Delete entry from audit_logs bucket
	if err := p.provider.Delete(ctx, bucketAuditLogs, []byte(entryID)); err != nil {
		return fmt.Errorf("failed to delete audit entry: %w", err)
	}

	if p.logger != nil {
		p.logger.Debug("Deleted audit log entry from meta-storage",
			zap.String("entry_id", entryID))
	}

	return nil
}

// GetLastHash retrieves the hash of the last audit log entry in the chain.
func (p *MetaStorageProvider) GetLastHash(ctx context.Context) (string, error) {
	// Get last hash from audit_log_chain bucket
	data, err := p.provider.Get(ctx, bucketAuditLogChain, []byte(keyLastHash))
	if err != nil {
		// If key doesn't exist, return empty string (first entry scenario)
		return "", nil
	}

	if len(data) == 0 {
		return "", nil
	}

	// Unmarshal hash (stored as JSON string)
	var hash string
	if err := json.Unmarshal(data, &hash); err != nil {
		// If unmarshaling fails, try treating as plain string
		hash = string(data)
	}

	return hash, nil
}

// SaveLastHash saves the hash of the last audit log entry in the chain.
func (p *MetaStorageProvider) SaveLastHash(ctx context.Context, hash string) error {
	// Marshal hash as JSON string
	data, err := json.Marshal(hash)
	if err != nil {
		return fmt.Errorf("failed to marshal hash: %w", err)
	}

	// Store in audit_log_chain bucket
	if err := p.provider.Put(ctx, bucketAuditLogChain, []byte(keyLastHash), data); err != nil {
		return fmt.Errorf("failed to save last hash: %w", err)
	}

	if p.logger != nil {
		p.logger.Debug("Saved last hash to meta-storage",
			zap.String("hash", hash))
	}

	return nil
}

// HealthCheck performs a health check on the meta-storage provider.
func (p *MetaStorageProvider) HealthCheck(ctx context.Context) (string, error) {
	// Check provider health
	if err := p.provider.HealthCheck(ctx); err != nil {
		return "unhealthy", fmt.Errorf("meta-storage provider health check failed: %w", err)
	}

	// Verify buckets exist
	buckets := []string{bucketAuditLogs, bucketAuditLogChain, bucketAuditLogSyncQueue}
	for _, bucketName := range buckets {
		if !p.provider.BucketExists(ctx, bucketName) {
			return "degraded", fmt.Errorf("required bucket does not exist: %s", bucketName)
		}
	}

	return "healthy", nil
}

// SaveSyncQueueEntry saves a sync queue entry to meta-storage.
// This is a helper method for SyncQueueManager to persist queue entries.
func (p *MetaStorageProvider) SaveSyncQueueEntry(ctx context.Context, entry types.SyncQueueEntry) error {
	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal sync queue entry: %w", err)
	}

	// Store in audit_log_sync_queue bucket with EntryID as key
	if err := p.provider.Put(ctx, bucketAuditLogSyncQueue, []byte(entry.EntryID), data); err != nil {
		return fmt.Errorf("failed to save sync queue entry: %w", err)
	}

	return nil
}

// LoadSyncQueueEntry loads a sync queue entry from meta-storage by EntryID.
func (p *MetaStorageProvider) LoadSyncQueueEntry(ctx context.Context, entryID string) (*types.SyncQueueEntry, error) {
	// Get entry from audit_log_sync_queue bucket
	data, err := p.provider.Get(ctx, bucketAuditLogSyncQueue, []byte(entryID))
	if err != nil {
		return nil, fmt.Errorf("failed to load sync queue entry: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("sync queue entry not found: %s", entryID)
	}

	// Unmarshal entry
	var entry types.SyncQueueEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync queue entry: %w", err)
	}

	return &entry, nil
}

// ListSyncQueueEntries lists all sync queue entries from meta-storage.
func (p *MetaStorageProvider) ListSyncQueueEntries(ctx context.Context) ([]types.SyncQueueEntry, error) {
	// List all entries from audit_log_sync_queue bucket
	keyValues, err := p.provider.List(ctx, bucketAuditLogSyncQueue, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list sync queue entries: %w", err)
	}

	var entries []types.SyncQueueEntry
	for _, kv := range keyValues {
		var entry types.SyncQueueEntry
		if err := json.Unmarshal(kv.Value, &entry); err != nil {
			if p.logger != nil {
				p.logger.Warn("Failed to unmarshal sync queue entry",
					zap.String("entry_id", string(kv.Key)),
					zap.Error(err))
			}
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// DeleteSyncQueueEntry deletes a sync queue entry from meta-storage by EntryID.
func (p *MetaStorageProvider) DeleteSyncQueueEntry(ctx context.Context, entryID string) error {
	// Delete entry from audit_log_sync_queue bucket
	if err := p.provider.Delete(ctx, bucketAuditLogSyncQueue, []byte(entryID)); err != nil {
		return fmt.Errorf("failed to delete sync queue entry: %w", err)
	}

	return nil
}

