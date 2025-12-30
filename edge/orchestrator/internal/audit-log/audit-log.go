package auditlog

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/impl"
	auditlogimplmetastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/impl/metastorage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Re-export errors from types package for convenience.
// These errors can be checked using errors.Is() for programmatic error handling.
var (
	// ErrNotInitialized indicates that the service or a required component is not initialized.
	ErrNotInitialized = types.ErrNotInitialized

	// ErrAlreadyStarted indicates that an operation was attempted on a service that is already started.
	ErrAlreadyStarted = types.ErrAlreadyStarted

	// ErrQueueFull indicates that the sync queue is full (reached max capacity).
	ErrQueueFull = types.ErrQueueFull

	// ErrSyncFailed indicates that syncing audit logs to VM failed.
	ErrSyncFailed = types.ErrSyncFailed

	// ErrTamperDetected indicates that hash chain integrity verification detected tampering.
	ErrTamperDetected = types.ErrTamperDetected
)

// Re-export health types for convenience.
type (
	HealthStatus   = types.HealthStatus
	AuditLogHealth = types.AuditLogHealth
)

// AuditLogService provides tamper-proof audit logging for security-sensitive operations.
// Audit logs are stored temporarily in edge storage and synced to VM for long-term retention.
//
// This interface follows the provider-agnostic architecture pattern from vm-gateway, meta-storage,
// and object-storage services.
//
// Hash Chain Integrity:
//   - Each audit log entry includes a cryptographic hash (SHA256)
//   - Entries form a hash chain: each entry's hash includes the previous entry's hash
//   - Any modification to an entry breaks the chain (tamper detection)
//   - Hash chain integrity is verified on startup and periodically (daily)
//
// Sync Queue Management:
//   - Failed syncs are queued for retry (max 100,000 entries)
//   - Entries remain in queue until VM acknowledgment (at-least-once delivery)
//   - When queue is full, operations pause (ErrQueueFull) - entries are NEVER dropped
//   - Exponential backoff with jitter for retry attempts (max 10 retries)
//
// At-Least-Once Delivery Guarantee:
//   - Entries are persisted locally before sync attempt
//   - Entries remain in queue until VM acknowledgment
//   - Retry on failure with exponential backoff
//   - Never drop entries (pause operations instead)
//
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_audit_log.go -package=mocks
type AuditLogService interface {
	// Lifecycle methods

	// Start initializes and starts the audit log service.
	// This method:
	//   - Initializes and verifies the storage provider
	//   - Loads and verifies hash chain integrity
	//   - Loads sync queue state from provider (crash recovery)
	//   - Initializes managers (sync trigger, cleanup, hash chain, metrics)
	//   - Starts background tasks (sync worker, cleanup worker, integrity check worker)
	//
	// Returns:
	//   - nil if the service started successfully
	//   - ErrAlreadyStarted if the service is already started
	//   - ErrNotInitialized if required components are not initialized
	//   - An error if initialization fails
	//
	// Background tasks started:
	//   - Sync worker: Syncs audit logs to VM (runs at sync interval, default: 5 minutes)
	//   - Cleanup worker: Cleans up expired entries (runs at cleanup interval, default: 24 hours)
	//   - Integrity check worker: Verifies hash chain integrity (runs daily)
	//
	// Example:
	//   if err := auditLogService.Start(ctx); err != nil {
	//       if errors.Is(err, auditlog.ErrAlreadyStarted) {
	//           // Service already started
	//       }
	//       // Handle error
	//   }
	Start(ctx context.Context) error

	// Stop gracefully shuts down the audit log service.
	// This method:
	//   - Stops all background tasks gracefully
	//   - Performs final sync to VM
	//   - Flushes pending operations
	//   - Closes provider connections
	//
	// Returns:
	//   - nil if the service stopped successfully
	//   - An error if shutdown fails (errors are aggregated, service attempts cleanup)
	//
	// The service will attempt to sync all pending entries to VM before shutdown.
	// However, if shutdown is urgent, some entries may remain in queue and will be
	// synced on next service startup (queue is persisted to provider).
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	// This follows the naming pattern from vm-gateway, meta-storage, and object-storage.
	//
	// Returns:
	//   - The service name (typically "audit-log-service")
	Name() string

	// HealthSnapshot returns the current health status of the audit log service.
	// This follows the health monitoring pattern from vm-gateway, meta-storage, and object-storage.
	//
	// The health snapshot includes:
	//   - Status: Overall health status (healthy, warning, queue_full, sync_failed, degraded)
	//   - Queue metrics: Depth, max size, usage percent, paused state
	//   - Sync metrics: Last sync time, success status, failure count, entries synced
	//   - Entry metrics: Entries logged, entries synced, entries pending
	//   - Hash chain integrity: Integrity status from last verification
	//   - Provider health: Provider-specific health status
	//
	// Health status calculation (priority order):
	//   1. HealthStatusQueueFull: Queue is 100% full and operations are paused
	//   2. HealthStatusSyncFailed: Sync failures detected (within last hour or last sync failed)
	//   3. HealthStatusDegraded: Hash chain integrity issues or provider errors
	//   4. HealthStatusWarning: Queue >80% full (but not paused)
	//   5. HealthStatusHealthy: All systems operating normally
	//
	// Returns:
	//   - AuditLogHealth struct with comprehensive health information
	//
	// Example:
	//   health := auditLogService.HealthSnapshot()
	//   if health.Status == types.HealthStatusQueueFull {
	//       // Queue is full - operations are paused
	//   }
	HealthSnapshot() types.AuditLogHealth

	// Logging methods

	// LogDataAccess logs a data access operation (read, write, delete, list).
	// This entry type is device-agnostic and supports access to various resource types.
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//   - Entry is linked to the hash chain for tamper-evidence
	//
	// Sync Queue:
	//   - Entry is persisted locally (to storage provider) first
	//   - Entry is then enqueued to sync queue for VM synchronization
	//   - Entry remains in queue until VM acknowledgment
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (100% capacity) - operations paused
	//     Callers should pause sensitive operations when this error is returned
	//   - ErrNotInitialized: Service not initialized (call Start() first)
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full (operations paused, entry was persisted locally)
	//   - ErrNotInitialized if service not started
	//   - An error if logging fails
	//
	// Pause-on-Full Behavior:
	//   - When queue is full, this method returns ErrQueueFull
	//   - Entry is still persisted locally (never dropped)
	//   - Callers should pause operations until queue has space again
	//   - Event emitted: audit_log.queue_full (operational health priority)
	//
	// Example:
	//   entry := types.DataAccessEntry{
	//       AuditEntry: types.AuditEntry{
	//           UserID:   "user-123",
	//           IPAddress: "192.168.1.100",
	//           Result:   "success",
	//       },
	//       ResourceType: types.ResourceTypeDataUnit,
	//       ResourceID:   "screenshot-001",
	//       Action:       "read",
	//       DeviceID:     types.DeviceID("camera-001"),
	//       DeviceType:   types.DeviceTypeCamera,
	//   }
	//   if err := auditLogService.LogDataAccess(ctx, entry); err != nil {
	//       if errors.Is(err, auditlog.ErrQueueFull) {
	//           // Queue full - pause operations
	//       }
	//       // Handle error
	//   }
	LogDataAccess(ctx context.Context, entry types.DataAccessEntry) error

	// LogAuthentication logs an authentication attempt.
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	//
	// Example:
	//   entry := types.AuthenticationEntry{
	//       AuditEntry: types.AuditEntry{
	//           UserID:   "user-123",
	//           IPAddress: "192.168.1.100",
	//           Result:   "success",
	//       },
	//       Method:   "api_key",
	//       Identity: "user-123",
	//   }
	//   if err := auditLogService.LogAuthentication(ctx, entry); err != nil {
	//       // Handle error
	//   }
	LogAuthentication(ctx context.Context, entry types.AuthenticationEntry) error

	// LogAuthorization logs an authorization decision (granted or denied).
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	LogAuthorization(ctx context.Context, entry types.AuthorizationEntry) error

	// LogConfigurationChange logs a configuration change operation.
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	LogConfigurationChange(ctx context.Context, entry types.ConfigurationChangeEntry) error

	// LogModelDeployment logs a model deployment operation (deploy, verify, activate, deactivate, remove).
	// This entry type is device-agnostic and supports deployment to any device type.
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	//
	// Example:
	//   entry := types.ModelDeploymentEntry{
	//       AuditEntry: types.AuditEntry{
	//           UserID:   "operator-001",
	//           Result:   "success",
	//       },
	//       ModelID:      "model-001",
	//       ModelVersion: "v1.0.0",
	//       DeviceID:     types.DeviceID("camera-001"),
	//       DeviceType:   types.DeviceTypeCamera,
	//       Action:       "deploy",
	//       VerificationResults: map[string]interface{}{
	//           "signature_valid": true,
	//           "hash_match":      true,
	//       },
	//       DeploymentStatus: "deployed",
	//   }
	//   if err := auditLogService.LogModelDeployment(ctx, entry); err != nil {
	//       // Handle error
	//   }
	LogModelDeployment(ctx context.Context, entry types.ModelDeploymentEntry) error

	// LogSecurityEvent logs a security-related event (intrusion, anomaly, etc.).
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	LogSecurityEvent(ctx context.Context, entry types.SecurityEventEntry) error

	// LogDatasetLifecycle logs a dataset lifecycle operation (created, labeled, uploaded, deleted).
	// This entry type is device-agnostic and supports dataset operations for any device.
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	//
	// Example:
	//   entry := types.DatasetLifecycleEntry{
	//       AuditEntry: types.AuditEntry{
	//           UserID:   "user-123",
	//           Result:   "success",
	//       },
	//       DatasetID:    "dataset-001",
	//       Action:       "created",
	//       DeviceID:     types.DeviceID("camera-001"),
	//       DeviceType:   types.DeviceTypeCamera,
	//       DataUnitCount: 1000,
	//   }
	//   if err := auditLogService.LogDatasetLifecycle(ctx, entry); err != nil {
	//       // Handle error
	//   }
	LogDatasetLifecycle(ctx context.Context, entry types.DatasetLifecycleEntry) error

	// LogRecoveryAction logs a recovery action taken by the system or operator.
	// This entry type is device-agnostic and supports recovery actions on any device.
	//
	// Hash Chain:
	//   - Entry is assigned a unique ID (UUID)
	//   - Entry hash is calculated as SHA256(previousHash:entryJSON)
	//   - PreviousHash references the hash of the last entry in the chain
	//
	// Error Conditions:
	//   - ErrQueueFull: Sync queue is full (operations paused)
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if the entry was logged successfully
	//   - ErrQueueFull if sync queue is full
	//   - An error if logging fails
	//
	// Example:
	//   entry := types.RecoveryActionEntry{
	//       AuditEntry: types.AuditEntry{
	//           UserID:   "operator-001",
	//           Result:   "failure",
	//       },
	//       RecoveryReason:     "integrity_failure",
	//       CorruptedResources: []string{"model", "dataset"},
	//       DeviceID:           types.DeviceID("camera-001"),
	//       DeviceType:         types.DeviceTypeCamera,
	//   }
	//   if err := auditLogService.LogRecoveryAction(ctx, entry); err != nil {
	//       // Handle error
	//   }
	LogRecoveryAction(ctx context.Context, entry types.RecoveryActionEntry) error

	// Sync and cleanup methods

	// SyncToVM syncs audit log entries from the sync queue to VM for long-term retention.
	// This method is called automatically by the background sync worker, but can also be
	// called manually for on-demand synchronization.
	//
	// Sync Protocol:
	//   - Processes entries from sync queue (up to SyncBatchSize, default: 1000)
	//   - Uses VM sync protocol with idempotency keys (per-entry and request-level)
	//   - Handles VM acknowledgment: entries are removed from queue only after VM confirms
	//   - Handles failures: failed entries are marked for retry with exponential backoff
	//   - Handles duplicates: duplicate entries (VM already has them) are marked as synced
	//
	// Sync Queue Behavior:
	//   - Processes queue entries first (priority over legacy storage-based sync)
	//   - Dequeues entries ready for sync (pending or failed status, retry time reached)
	//   - Marks entries as syncing during sync attempt
	//   - Marks successfully synced entries as synced (removes from queue)
	//   - Marks failed entries for retry (updates retry count, calculates next retry time)
	//
	// At-Least-Once Delivery:
	//   - Entries are persisted locally before sync attempt
	//   - Entries remain in queue until VM acknowledgment
	//   - Failed entries are retried with exponential backoff
	//   - Entries are NEVER dropped, even if sync fails repeatedly
	//
	// Error Conditions:
	//   - ErrNotInitialized: Service not initialized
	//   - ErrSyncFailed: Sync to VM failed (entries remain in queue for retry)
	//   - VM gateway errors: Network errors, authentication failures, etc.
	//
	// Returns:
	//   - nil if sync completed successfully (or no entries to sync)
	//   - ErrSyncFailed if sync failed (entries remain in queue, will be retried)
	//   - An error if sync operation fails critically
	//
	// Event Emission:
	//   - audit_log.sync_succeeded: Emitted on successful sync (with entries synced count)
	//   - audit_log.sync_failed: Emitted on sync failure (with error message)
	//
	// Metrics:
	//   - Sync operations, successes, failures tracked
	//   - Sync latency tracked (P50, P95, P99 percentiles)
	//   - Entries synced count tracked
	//
	// Example:
	//   if err := auditLogService.SyncToVM(ctx); err != nil {
	//       if errors.Is(err, auditlog.ErrSyncFailed) {
	//           // Sync failed - entries remain in queue for retry
	//       }
	//       // Handle error
	//   }
	SyncToVM(ctx context.Context) error

	// CleanupOldLogs removes expired audit log entries based on the retention policy.
	// This method is called automatically by the background cleanup worker, but can also be
	// called manually for on-demand cleanup.
	//
	// Cleanup Behavior:
	//   - Only deletes entries that are synced to VM (never deletes unsynced entries)
	//   - Uses conservative approach: retention + 7 days buffer for safe deletion
	//   - Processes entries in batches (up to CleanupBatchSize, default: 1000)
	//   - Continues with next batch on failure (doesn't stop entire cleanup)
	//
	// Retention Policy:
	//   - Default retention: 90 days (configurable)
	//   - Entries older than retention period are candidates for deletion
	//   - Only synced entries are deleted (unsynced entries are preserved)
	//
	// Error Conditions:
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Returns:
	//   - nil if cleanup completed successfully (or no entries to cleanup)
	//   - An error if cleanup operation fails critically (some entries may have been deleted)
	//
	// Event Emission:
	//   - audit_log.cleanup_started: Emitted when cleanup begins
	//   - audit_log.cleanup_completed: Emitted when cleanup finishes (with statistics)
	//
	// Cleanup Statistics:
	//   - EntriesDeleted: Number of entries deleted
	//   - EntriesSkipped: Number of entries skipped (not synced)
	//   - ErrorsEncountered: Number of errors encountered
	//   - CleanupDuration: Duration of cleanup operation
	//
	// Example:
	//   if err := auditLogService.CleanupOldLogs(ctx); err != nil {
	//       // Handle error
	//   }
	CleanupOldLogs(ctx context.Context) error

	// Query methods

	// QueryAuditLogs queries audit log entries matching the provided filters.
	//
	// Filtering:
	//   - StartTime: Filter entries after this time (inclusive)
	//   - EndTime: Filter entries before this time (inclusive)
	//   - EntryType: Filter by entry type (data_access, authentication, etc.)
	//   - UserID: Filter by user ID
	//   - IPAddress: Filter by IP address
	//   - Result: Filter by result (success, failure, denied)
	//   - Limit: Maximum number of entries to return (default: 100)
	//   - Offset: Number of entries to skip (for pagination)
	//
	// Returns:
	//   - A slice of audit log entries (as interface{} - caller must type assert)
	//   - Empty slice if no entries match
	//   - An error if query operation fails
	//
	// Entry Type Assertion:
	//   The returned entries are interface{} types. Callers must type assert to the
	//   appropriate entry type based on the EntryType field:
	//
	//   for _, entryInterface := range entries {
	//       // Type assert based on entry type
	//       switch e := entryInterface.(type) {
	//       case types.DataAccessEntry:
	//           // Handle data access entry
	//       case types.AuthenticationEntry:
	//           // Handle authentication entry
	//       // ... other entry types
	//       }
	//   }
	//
	// Error Conditions:
	//   - ErrNotInitialized: Service not initialized
	//   - Provider errors: Storage provider operation failures
	//
	// Example:
	//   filters := types.QueryFilters{
	//       StartTime: timePtr(time.Now().Add(-24 * time.Hour)),
	//       EndTime:   timePtr(time.Now()),
	//       EntryType: string(types.EntryTypeDataAccess),
	//       UserID:     "user-123",
	//       Result:     "success",
	//       Limit:      100,
	//       Offset:     0,
	//   }
	//   entries, err := auditLogService.QueryAuditLogs(ctx, filters)
	//   if err != nil {
	//       // Handle error
	//   }
	QueryAuditLogs(ctx context.Context, filters types.QueryFilters) ([]interface{}, error)

	// GetAuditLogEntry retrieves a specific audit log entry by ID.
	//
	// Returns:
	//   - The audit log entry as interface{} (caller must type assert)
	//   - nil and error if entry not found (types.ErrRecordNotFound)
	//   - nil and error if query operation fails
	//
	// Entry Type Assertion:
	//   The returned entry is interface{} type. Caller must type assert to the
	//   appropriate entry type based on the EntryType field:
	//
	//   entry, err := auditLogService.GetAuditLogEntry(ctx, entryID)
	//   if err != nil {
	//       if errors.Is(err, types.ErrRecordNotFound) {
	//           // Entry not found
	//       }
	//       // Handle error
	//   }
	//
	//   // Type assert based on entry type
	//   switch e := entry.(type) {
	//   case types.DataAccessEntry:
	//       // Handle data access entry
	//   case types.AuthenticationEntry:
	//       // Handle authentication entry
	//   // ... other entry types
	//   }
	//
	// Error Conditions:
	//   - ErrNotInitialized: Service not initialized
	//   - types.ErrRecordNotFound: Entry not found
	//   - Provider errors: Storage provider operation failures
	//
	// Example:
	//   entry, err := auditLogService.GetAuditLogEntry(ctx, "entry-id-123")
	//   if err != nil {
	//       if errors.Is(err, types.ErrRecordNotFound) {
	//           // Entry not found
	//       }
	//       // Handle error
	//   }
	GetAuditLogEntry(ctx context.Context, entryID string) (interface{}, error)
}

// NewAuditLogService creates a new audit log service instance.
// This factory function should typically not be called directly;
// use AuditLogProvider instead for proper dependency injection with Fx lifecycle management.
//
// Parameters:
//   - config: Audit log configuration (will be validated automatically)
//   - objectStorage: Object storage service (required, but deprecated - use provider instead)
//   - vmGateway: VM gateway for syncing audit logs to VM (required)
//   - logger: Logger for logging operations (required)
//   - edgeID: Edge device identifier (required)
//   - provider: Storage provider for audit log entries (optional, but recommended - meta-storage preferred)
//   - eventBus: Event bus for operational event emission (optional)
//
// Configuration:
//   - Config is automatically validated and defaults are set if not provided
//   - Default retention: 90 days
//   - Default sync interval: 5 minutes
//   - Default sync batch size: 1000 records
//   - Default sync trigger mode: hybrid (time-based or count-based)
//
// Returns:
//   - AuditLogService instance (not yet started - call Start() to initialize)
//
// Usage:
//   This function is primarily used internally by AuditLogProvider.
//   For manual creation, call Start() after creation:
//
//   service := auditlog.NewAuditLogService(config, objectStorage, vmGateway, logger, edgeID, provider, eventBus)
//   if err := service.Start(ctx); err != nil {
//       // Handle error
//   }
//   defer service.Stop(ctx)
func NewAuditLogService(
	config *types.AuditLogConfig,
	objectStorage objectstorage.ObjectStorageService,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	edgeID string,
	provider types.AuditLogProvider, // Optional provider (preferred: meta-storage)
	eventBus eventbus.EventBus, // Optional event bus for operational event emission
) AuditLogService {
	return impl.NewAuditLogService(config, objectStorage, vmGateway, logger, edgeID, provider, eventBus)
}

// AuditLogProvider creates the audit log service with fx lifecycle management.
//
// The audit log service provides tamper-proof audit logging for security-sensitive operations.
// Audit logs are stored temporarily in edge storage and synced to VM for long-term retention.
//
// Architecture decision: Service-owned lifecycle.
//   - Fx manages only the audit log service lifecycle.
//   - Service Start/Stop is the single place that initializes/closes storage providers.
//   - Storage providers do not register their own fx.Lifecycle hooks.
//
// Parameters:
//   - lc: Fx lifecycle for automatic service startup/shutdown
//   - config: Audit log configuration (will be validated automatically)
//   - objectStorage: Object storage service (required, but deprecated - provider is used instead)
//   - vmGateway: VM gateway for syncing audit logs to VM (required)
//   - logger: Logger for logging operations (required)
//   - edgeID: Edge device identifier (required)
//   - metaStorage: Meta-storage service for creating audit log provider (required)
//   - eventBus: Event bus for operational event emission (optional, but recommended)
//
// Lifecycle:
//   - Service is automatically started when Fx starts (OnStart hook)
//   - Service is automatically stopped when Fx stops (OnStop hook)
//   - Service initialization includes:
//     - Provider initialization and verification
//     - Hash chain integrity verification
//     - Sync queue state loading (crash recovery)
//     - Background task startup
//
// Returns:
//   - AuditLogService instance (already registered with Fx lifecycle)
//   - Error if provider creation fails or service cannot be created
//
// Error Conditions:
//   - Failed to create meta-storage provider from MetaDataStore
//   - Configuration validation failures (handled automatically)
//
// Usage Example:
//   // In your Fx module
//   var Module = fx.Module("audit-log",
//       fx.Provide(auditlog.AuditLogProvider),
//   )
//
//   // Service will be automatically started and stopped by Fx
//   // Use the service in other components via dependency injection
func AuditLogProvider(
	lc fx.Lifecycle,
	config *types.AuditLogConfig,
	objectStorage objectstorage.ObjectStorageService,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	edgeID string,
	metaStorage metastorage.MetaDataStore,
	eventBus eventbus.EventBus, // Optional event bus for operational event emission
) (AuditLogService, error) {
	// Create the meta-storage provider for audit logs
	// Note: Currently requires MetaStorageProvider, not MetaDataStore
	// TODO: Implement NewAuditLogProviderFromMetaStorage to extract provider from MetaDataStore
	auditLogMetaProvider, err := auditlogimplmetastorage.NewAuditLogProviderFromMetaStorage(metaStorage, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create meta-storage audit log provider: %w", err)
	}

	service := NewAuditLogService(config, objectStorage, vmGateway, logger, edgeID, auditLogMetaProvider, eventBus)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := service.Start(ctx); err != nil {
				return fmt.Errorf("failed to start audit log service: %w", err)
			}
			logger.Info("Audit log service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := service.Stop(ctx); err != nil {
				return fmt.Errorf("failed to stop audit log service: %w", err)
			}
			logger.Info("Audit log service stopped")
			return nil
		},
	})

	return service, nil
}
