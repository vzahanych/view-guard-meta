package types

import "context"

// AuditLogProvider defines the interface for provider-agnostic audit log storage operations.
// This interface abstracts away the details of storage provider implementations (meta-storage, object-storage, etc.),
// allowing the audit log service to work with any storage backend.
//
// The provider is responsible for:
//   - Storing and retrieving audit log entries
//   - Managing hash chain state (last hash persistence)
//   - Managing sync queue state (persistence for crash recovery)
//   - Providing health status
//
// Provider implementations should be created in impl/metastorage/ and impl/objectstorage/ directories.
//
// Thread Safety:
//   - All provider methods must be thread-safe
//   - Providers should use transactions for atomic operations
//   - Concurrent access from multiple goroutines must be supported
//
// Storage Organization:
//   - Meta-storage provider uses buckets:
//     - audit_logs: Audit log entries (key: EntryID)
//     - audit_log_chain: Hash chain state (key: "last_hash")
//     - audit_log_sync_queue: Sync queue entries (key: EntryID)
//
// Error Handling:
//   - Providers should return ErrRecordNotFound when entries don't exist
//   - Providers should wrap underlying storage errors for context
//   - Providers should validate entry data before storing
type AuditLogProvider interface {
	// SaveEntry saves an audit log entry to storage.
	// The entry must have a valid ID that uniquely identifies it.
	//
	// Hash Chain:
	//   - The entry includes Hash and PreviousHash fields for chain integrity
	//   - Providers should store these fields as-is (no modification)
	//   - The hash chain state is managed separately via GetLastHash/SaveLastHash
	//
	// Storage:
	//   - Entry is stored with EntryID as the key
	//   - Entry is serialized as JSON for storage
	//   - Storage should be atomic (transaction-based if supported)
	//
	// Error Conditions:
	//   - Entry missing required fields (ID, Type, Timestamp, Hash)
	//   - Storage quota exceeded (provider-specific)
	//   - Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was saved successfully
	//   - An error if the entry is invalid or storage operation fails
	//
	// Example:
	//   entry := types.AuditEntry{
	//       ID:        "entry-123",
	//       Type:      types.EntryTypeDataAccess,
	//       Timestamp: time.Now(),
	//       Hash:      "sha256:abc123...",
	//   }
	//   if err := provider.SaveEntry(ctx, entry); err != nil {
	//       // Handle error
	//   }
	SaveEntry(ctx context.Context, entry AuditEntry) error

	// LoadEntry loads an audit log entry from storage by ID.
	//
	// Returns:
	//   - The audit log entry if found (with all fields populated, including hash chain fields)
	//   - nil and ErrRecordNotFound if the entry does not exist
	//   - nil and error if the provider operation fails
	//
	// Hash Chain:
	//   - Returns entry with Hash and PreviousHash fields populated
	//   - These fields are used for hash chain verification
	//
	// Error Conditions:
	//   - ErrRecordNotFound: Entry does not exist
	//   - Storage provider operation failures
	//   - Data corruption (invalid JSON, missing fields)
	//
	// Example:
	//   entry, err := provider.LoadEntry(ctx, "entry-123")
	//   if err != nil {
	//       if errors.Is(err, types.ErrRecordNotFound) {
	//           // Entry not found
	//       }
	//       // Handle error
	//   }
	LoadEntry(ctx context.Context, entryID string) (*AuditEntry, error)

	// ListEntries lists audit log entries matching the provided filters.
	//
	// Filtering:
	//   - StartTime: Filter entries after this time (inclusive)
	//   - EndTime: Filter entries before this time (inclusive)
	//   - EntryType: Filter by entry type
	//   - UserID: Filter by user ID
	//   - IPAddress: Filter by IP address
	//   - Result: Filter by result
	//   - ResourceType: Filter by resource type
	//   - ResourceID: Filter by resource ID
	//   - Limit: Maximum number of entries to return (0 = no limit)
	//   - Offset: Number of entries to skip (for pagination)
	//
	// Returns:
	//   - A slice of audit log entries matching the filters (sorted by timestamp, oldest first)
	//   - Empty slice if no entries match
	//   - An error if the provider operation fails
	//
	// Performance:
	//   - Providers should implement efficient filtering (indexes, if supported)
	//   - Large result sets should be paginated using Limit and Offset
	//   - Providers may return entries in batches for memory efficiency
	//
	// Error Conditions:
	//   - Storage provider operation failures
	//   - Invalid filter parameters
	//
	// Example:
	//   filters := types.QueryFilters{
	//       StartTime: timePtr(time.Now().Add(-24 * time.Hour)),
	//       EndTime:   timePtr(time.Now()),
	//       EntryType: string(types.EntryTypeDataAccess),
	//       Limit:     100,
	//       Offset:    0,
	//   }
	//   entries, err := provider.ListEntries(ctx, filters)
	//   if err != nil {
	//       // Handle error
	//   }
	ListEntries(ctx context.Context, filters QueryFilters) ([]AuditEntry, error)

	// DeleteEntry deletes an audit log entry from storage by ID.
	// If the entry does not exist, this is a no-op and returns no error.
	//
	// Cleanup:
	//   - This method is used during retention cleanup
	//   - Only synced entries should be deleted (enforced by service layer)
	//   - Deletion should be atomic (transaction-based if supported)
	//
	// Hash Chain:
	//   - Deletion does not affect hash chain state (last hash persists separately)
	//   - Deleted entries are removed from the chain (chain continues from remaining entries)
	//
	// Returns:
	//   - nil if the entry was deleted successfully or does not exist
	//   - An error if the provider operation fails
	//
	// Error Conditions:
	//   - Storage provider operation failures
	//
	// Example:
	//   if err := provider.DeleteEntry(ctx, "entry-123"); err != nil {
	//       // Handle error
	//   }
	DeleteEntry(ctx context.Context, entryID string) error

	// GetLastHash retrieves the hash of the last audit log entry in the chain.
	// This is used for hash chain continuation when creating new entries.
	//
	// Hash Chain Continuation:
	//   - When creating a new entry, the PreviousHash field should be set to the last hash
	//   - The last hash is updated after each entry is saved (via SaveLastHash)
	//
	// Returns:
	//   - The hash of the last entry (hex-encoded SHA256 hash)
	//   - Empty string if no entries exist (first entry scenario)
	//   - An error if the provider operation fails
	//
	// Storage:
	//   - Last hash is stored separately from entries (in audit_log_chain bucket for meta-storage)
	//   - Key is typically "last_hash"
	//   - If no last hash exists, returns empty string (not an error)
	//
	// Error Conditions:
	//   - Storage provider operation failures
	//
	// Example:
	//   lastHash, err := provider.GetLastHash(ctx)
	//   if err != nil {
	//       // Handle error
	//   }
	//   if lastHash == "" {
	//       // First entry - PreviousHash should be empty
	//   } else {
	//       // Use lastHash as PreviousHash for new entry
	//   }
	GetLastHash(ctx context.Context) (string, error)

	// SaveLastHash saves the hash of the last audit log entry in the chain.
	// This is used for hash chain state persistence after each entry is saved.
	//
	// Hash Chain State:
	//   - Last hash should be updated after each entry is successfully saved
	//   - This enables hash chain continuation across service restarts
	//   - Last hash is used to initialize PreviousHash for new entries
	//
	// Storage:
	//   - Last hash is stored separately from entries (in audit_log_chain bucket for meta-storage)
	//   - Key is typically "last_hash"
	//   - Storage should be atomic (transaction-based if supported)
	//
	// Returns:
	//   - nil if the last hash was saved successfully
	//   - An error if the provider operation fails
	//
	// Error Conditions:
	//   - Storage provider operation failures
	//
	// Example:
	//   newEntryHash := calculateHash(previousHash, entryJSON)
	//   if err := provider.SaveLastHash(ctx, newEntryHash); err != nil {
	//       // Handle error - hash chain state may be inconsistent
	//   }
	SaveLastHash(ctx context.Context, hash string) error

	// HealthCheck performs a health check on the provider.
	// This method verifies that the provider is accessible and operational.
	//
	// Health Status Values:
	//   - "healthy": Provider is operational and accessible
	//   - "degraded": Provider is operational but with reduced performance or warnings
	//   - "unhealthy": Provider is not operational or inaccessible
	//
	// Checks Performed:
	//   - Provider connectivity (database connection, file system access, etc.)
	//   - Storage availability (disk space, quota limits, etc.)
	//   - Provider-specific health indicators
	//
	// Returns:
	//   - Health status string ("healthy", "degraded", "unhealthy")
	//   - An error if the health check fails critically (provider is definitely unhealthy)
	//
	// Error Handling:
	//   - If health check fails with an error, provider should be considered unhealthy
	//   - Health status "degraded" or "unhealthy" may be returned without error
	//
	// Example:
	//   status, err := provider.HealthCheck(ctx)
	//   if err != nil {
	//       // Provider is definitely unhealthy
	//   }
	//   if status != "healthy" {
	//       // Provider health issue - may affect operations
	//   }
	HealthCheck(ctx context.Context) (string, error)
}

