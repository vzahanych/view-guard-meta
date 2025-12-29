# Audit Log Refactoring Plan

**Date**: 2025-12-28  
**Target Documents**: 
- `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`
- `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- `edge/orchestrator/internal/vm-gateway/doc.go` (architectural pattern reference)

**Scope**: Complete refactoring of `audit-log` package to align with production workflow requirements and follow vm-gateway architectural pattern  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the Audit Log service implementation into full compliance with the production workflow specification and aligns it with the vm-gateway architectural pattern. The current implementation has good foundations (hash chaining, sync to VM, cleanup) but lacks production features (queue management for sync failures, proper sync triggers, retention configuration, device-agnostic design) and doesn't follow the provider-agnostic architecture pattern.

**Key Transformation Areas**:
1. **Provider-agnostic architecture**: Follow vm-gateway pattern with interface, types, and implementation separation
2. **Device-agnostic design**: Replace camera-centric terminology with device-agnostic types
3. **Sync queue management**: Implement queue for failed syncs (max 100,000 records) with pause-on-full behavior
4. **Sync trigger optimization**: Sync every 5 minutes or 1000 records, whichever comes first
5. **Retention configuration**: Update default retention to 90 days (configurable)
6. **Storage provider**: Consider meta-storage instead of object-storage for better integration
7. **Health monitoring**: Add health snapshot API and operational metrics
8. **Observability**: Add comprehensive observability following vm-gateway pattern

---

## Epic 1: Provider-Agnostic Architecture (Following vm-gateway Pattern)

**Goal**: Restructure the codebase to follow the vm-gateway architectural pattern with clear separation of concerns.

### Section 1.1: Interface and Types Separation

#### Subsection 1.1.1: Main Interface File
- **Files**: `audit_log.go` (already exists, enhance)
- **Changes**:
  - Enhance `AuditLogService` interface:
    - Keep lifecycle methods (`Start`, `Stop`, `Name`)
    - Add `HealthSnapshot() AuditLogHealth` method
  - Define sentinel errors (similar to vm-gateway):
    - `ErrNotInitialized`
    - `ErrAlreadyStarted`
    - `ErrQueueFull` (when sync queue is full)
    - `ErrSyncFailed` (when sync to VM fails)
    - `ErrTamperDetected` (when hash chain is broken)
  - Define factory function `NewAuditLogService(...)`
  - Define provider function `AuditLogProvider(...)` with fx lifecycle
  - Add comprehensive package documentation (similar to vm-gateway/doc.go)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Types Package Structure
- **Files**: `types/` directory (already exists, enhance)
- **Changes**:
  - Move all configuration types to `types/config.go`
  - Create `types/health.go` for health-related types:
    - `HealthStatus` enum (healthy, warning, queue_full, sync_failed, degraded)
    - `AuditLogHealth` struct (status, metrics, queue depth, sync status)
  - Create `types/storage.go` for storage-related types:
    - `AuditEntryMetadata` struct (id, timestamp, hash, previous_hash, synced)
    - `SyncStatus` enum (pending, syncing, synced, failed)
  - Create `types/provider.go` for provider interface:
    - `AuditLogProvider` interface (provider-agnostic operations)
    - Provider-specific configuration types
  - Create `types/errors.go` for error types
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Implementation Package Structure
- **Files**: `impl/` directory (already exists, enhance)
- **Changes**:
  - Create `impl/audit_log_impl.go` (main implementation, rename from `audit-log-impl.go`)
  - Create provider-specific implementations:
    - `impl/metastorage/metastorage_provider.go` (meta-storage implementation, preferred)
    - `impl/objectstorage/objectstorage_provider.go` (object-storage implementation, keep for backward compatibility)
  - Each provider implements `types.AuditLogProvider` interface
  - Main implementation delegates to provider
  - **Decision**: Prefer meta-storage over object-storage for better integration with meta-storage refactoring
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

### Section 1.2: Lifecycle Management

#### Subsection 1.2.1: Service Lifecycle
- **Files**: `impl/audit_log_impl.go`
- **Changes**:
  - Implement `Start(ctx)` method:
    - Initialize provider (meta-storage or object-storage)
    - Verify connectivity
    - Initialize hash chain (load last hash from storage)
    - Start background tasks:
      - Sync worker (runs every 5 minutes or when 1000 records queued)
      - Cleanup worker (runs daily)
      - Health check worker (runs every 1 minute)
    - Initialize sync queue
  - Implement `Stop(ctx)` method:
    - Stop background tasks gracefully
    - Final sync before shutdown
    - Close provider connections
    - Flush pending operations
  - Follow vm-gateway pattern: service owns lifecycle of sub-components
- **Dependencies**: 1.1.3
- **Estimated Effort**: 1 day

#### Subsection 1.2.2: Provider Lifecycle
- **Files**: `impl/metastorage/metastorage_provider.go` (and other providers)
- **Changes**:
  - Implement provider-specific initialization
  - Implement provider-specific cleanup
  - Providers do NOT register their own fx.Lifecycle hooks (gateway-owned lifecycle pattern)
- **Dependencies**: 1.2.1
- **Estimated Effort**: 1 day

---

## Epic 2: Device-Agnostic Architecture

**Goal**: Transform the codebase from camera-centric to device-agnostic terminology and types.

### Section 2.1: Type System Refactoring

#### Subsection 2.1.1: Replace CameraID with DeviceID
- **Files**: All files in `audit_log.go`, `types/`, `impl/`
- **Changes**:
  - Replace all `CameraID` references with `DeviceID`
  - Update `ModelDeploymentEntry`:
    - `CameraID` → `DeviceID`
    - Add `DeviceType` field (camera, sensor, audio_device, etc.)
  - Update `DataAccessEntry`:
    - `ResourceType` should use device-agnostic terms (data_unit instead of screenshot, device instead of camera)
    - `ResourceID` should reference DeviceID when applicable
  - Update variable names and map keys
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 2.1.2: Device-Agnostic Entry Types
- **Files**: `types/types.go`
- **Changes**:
  - Create `DeviceID` type alias (string)
  - Create `DeviceType` enum (camera, sensor, audio_device, etc.)
  - Update `ModelDeploymentEntry`:
    - `DeviceID string` (replaces `CameraID`)
    - `DeviceType string` (new field)
  - Update `DataAccessEntry`:
    - `ResourceType` enum values: `data_unit`, `video_clip`, `device`, `security_event`, `model`, `dataset`
    - Update documentation to be device-agnostic
- **Dependencies**: 2.1.1
- **Estimated Effort**: 1 day

---

## Epic 3: Sync Queue Management

**Goal**: Implement queue management for failed syncs with pause-on-full behavior.

### Section 3.1: Sync Queue Implementation

#### Subsection 3.1.1: Sync Queue Types
- **Files**: `types/storage.go`
- **Changes**:
  - Define `SyncQueueEntry` struct:
    - `EntryID string`
    - `EntryData []byte` (serialized audit entry)
    - `QueuedAt time.Time`
    - `RetryCount int`
    - `LastRetryTime time.Time`
    - `NextRetryTime time.Time`
  - Define `SyncStatus` enum:
    - `SyncStatusPending` (not yet synced)
    - `SyncStatusSyncing` (currently being synced)
    - `SyncStatusSynced` (successfully synced)
    - `SyncStatusFailed` (sync failed, will retry)
  - Define `SyncQueueConfig` struct:
    - `MaxQueueSize int` (default: 100,000 records)
    - `RetryBackoff time.Duration` (exponential backoff)
    - `MaxRetries int` (default: 10)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Sync Queue Manager
- **Files**: `impl/sync_queue_manager.go` (new file)
- **Changes**:
  - Implement `SyncQueueManager` struct:
    - Track queued entries (in-memory and persisted)
    - Track sync status per entry
    - Track queue depth
  - Implement `EnqueueEntry(ctx, entry AuditEntry) error`:
    - Add entry to sync queue
    - Check queue size limit
    - If queue full: return `ErrQueueFull`, emit critical alert
  - Implement `DequeueEntries(ctx, limit int) ([]SyncQueueEntry, error)`:
    - Get entries ready for sync (pending status, retry time reached)
    - Mark entries as syncing
    - Return up to limit entries
  - Implement `MarkSynced(ctx, entryID string) error`:
    - Mark entry as synced
    - Remove from queue (or mark for cleanup)
  - Implement `MarkFailed(ctx, entryID string, error error) error`:
    - Increment retry count
    - Calculate next retry time (exponential backoff)
    - If max retries exceeded: keep in queue but mark as failed (do NOT drop)
  - Persist queue state to meta-storage (for crash recovery)
- **Dependencies**: 3.1.1, Section 1.1
- **Estimated Effort**: 3 days

#### Subsection 3.1.3: Pause-on-Full Behavior
- **Files**: `impl/audit_log_impl.go`
- **Changes**:
  - **CRITICAL PRODUCTION REQUIREMENT**: Audit records must NEVER be dropped, even if queue is full
    - This is a compliance requirement: audit logs are tamper-evident and must be preserved
    - If queue is full: **PAUSE SENSITIVE OPERATIONS** until sync resumes (do NOT drop records)
    - Sensitive operations include: dataset creation, model deployment, security event creation, recovery actions
  - Implement pause mechanism:
    - Track `IsPaused bool` flag (when queue is full)
    - When queue full: set `IsPaused = true`, emit critical alert
    - When queue has space again: set `IsPaused = false`
  - Implement `IsOperationPaused() bool` method:
    - Return true if queue is full
    - Used by callers to pause sensitive operations
  - Update `Log*` methods to check pause status:
    - If paused: return error (caller should pause operations)
    - If not paused: proceed with logging
  - **Never drop audit records**: Even if queue is full, audit records must be queued (extend queue if needed) or pause operations
  - Emit events: `audit_log.queue_full`, `audit_log.queue_resumed`
- **Dependencies**: 3.1.2
- **Estimated Effort**: 2 days

---

## Epic 4: Sync Trigger Optimization

**Goal**: Implement sync every 5 minutes or 1000 records, whichever comes first.

### Section 4.1: Sync Trigger Configuration

#### Subsection 4.1.1: Sync Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Update `AuditLogConfig`:
    - `SyncInterval time.Duration` (default: 5 minutes)
    - `SyncBatchSize int` (default: 1000 records)
    - `SyncTriggerMode string` (time_based, count_based, hybrid - default: hybrid)
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.1.2: Sync Trigger Manager
- **Files**: `impl/sync_trigger_manager.go` (new file)
- **Changes**:
  - Implement `SyncTriggerManager` struct:
    - Track pending entry count
    - Track last sync time
    - Track sync triggers (time-based, count-based)
  - Implement `ShouldSync() bool`:
    - Return true if:
      - 5 minutes have passed since last sync, OR
      - 1000 records are pending
    - Hybrid mode: trigger on either condition
  - Implement `RecordPendingEntry()`:
    - Increment pending count
    - Check if count threshold reached
  - Implement `RecordSync()`:
    - Reset pending count
    - Update last sync time
- **Dependencies**: 4.1.1
- **Estimated Effort**: 1 day

#### Subsection 4.1.3: Sync Worker Enhancement
- **Files**: `impl/audit_log_impl.go`
- **Changes**:
  - Update sync worker to use trigger manager:
    - Check `ShouldSync()` before syncing
    - Sync in batches (up to 1000 records per sync)
    - Process sync queue entries
    - Update sync status after VM acknowledgment
  - Implement sync metrics (sync count, sync duration, records synced, sync failures)
- **Dependencies**: 4.1.2, Epic 3
- **Estimated Effort**: 2 days

---

## Epic 5: Retention and Cleanup

**Goal**: Update retention to 90 days and implement proper cleanup.

### Section 5.1: Retention Configuration

#### Subsection 5.1.1: Retention Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Update `AuditLogConfig`:
    - `RetentionDays int` (default: 90 days, was 7 days)
    - `CleanupInterval time.Duration` (default: 24 hours)
    - `CleanupBatchSize int` (default: 1000 entries per batch)
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Cleanup Manager
- **Files**: `impl/cleanup_manager.go` (new file)
- **Changes**:
  - Implement `CleanupManager` struct:
    - Track retention policy
    - Track cleanup schedule
  - Implement `CleanupExpiredEntries(ctx) error`:
    - Query entries older than retention period (90 days)
    - Delete expired entries in batches
    - Only delete entries that are synced (never delete unsynced entries)
    - Handle cleanup errors gracefully (log and continue)
    - Return cleanup statistics (entries deleted, space freed)
  - Implement background cleanup task (runs daily, configurable)
  - Emit events: `audit_log.cleanup_started`, `audit_log.cleanup_completed`
- **Dependencies**: 5.1.1, Section 1.1
- **Estimated Effort**: 2 days

---

## Epic 6: Hash Chain Integrity

**Goal**: Enhance hash chain integrity verification and tamper detection.

### Section 6.1: Hash Chain Verification

#### Subsection 6.1.1: Hash Chain Verification
- **Files**: `impl/hash_chain_manager.go` (new file)
- **Changes**:
  - Implement `HashChainManager` struct:
    - Track last hash (for chain continuation)
    - Track hash chain integrity
  - Implement `VerifyHashChain(ctx) (*HashChainReport, error)`:
    - Load all entries from storage
    - Verify hash chain integrity:
      - Each entry's hash matches calculated hash
      - Each entry's previous_hash matches previous entry's hash
      - Chain is unbroken
    - Return integrity report (broken links, tamper indicators)
  - Implement periodic integrity checks (background task, daily)
  - Emit events: `audit_log.tamper_detected` (if tampering detected)
  - Return `ErrTamperDetected` when tampering is detected
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 6.1.2: Hash Chain Recovery
- **Files**: `impl/hash_chain_manager.go`
- **Changes**:
  - Implement hash chain recovery:
    - If chain is broken: identify break point
    - Mark entries after break as suspicious
    - Attempt to repair chain (if possible)
    - Emit critical alert for operator investigation
  - Implement hash chain initialization:
    - Load last hash from storage on startup
    - Verify chain continuity
    - Handle missing last hash (first entry scenario)
- **Dependencies**: 6.1.1
- **Estimated Effort**: 1 day

---

## Epic 7: VM Sync Protocol

**Goal**: Implement proper VM sync protocol with idempotency and at-least-once delivery.

### Section 7.1: Sync Protocol Implementation

#### Subsection 7.1.1: Sync Request/Response
- **Files**: `impl/vm_sync_protocol.go` (new file)
- **Changes**:
  - Implement `SyncAuditLogsToVM(ctx, entries []AuditEntry) error`:
    - Build sync request:
      - EdgeID
      - Entries array (with idempotency keys)
      - Batch metadata
    - Call `VMGateway.SyncAuditLogs(ctx, request)`
    - Process response:
      - Acknowledged entries (mark as synced)
      - Failed entries (mark as failed, retry)
      - Duplicate entries (VM already has them, mark as synced)
  - Implement idempotency:
    - Each entry has idempotency key: `(EdgeID, EntryID)`
    - VM deduplicates by idempotency key
    - Handle VM duplicate response gracefully
  - Implement batching (up to 1000 entries per request)
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 7.1.2: At-Least-Once Delivery
- **Files**: `impl/vm_sync_protocol.go`
- **Changes**:
  - Implement at-least-once delivery guarantee:
    - Entries are persisted locally before sync attempt
    - Entries remain in queue until VM acknowledgment
    - Retry on failure with exponential backoff
    - Never drop entries (even if queue is full, pause operations instead)
  - Implement sync status tracking:
    - Track sync attempts per entry
    - Track sync success/failure
    - Track VM acknowledgment
- **Dependencies**: 7.1.1
- **Estimated Effort**: 1 day

---

## Epic 8: Storage Provider Migration

**Goal**: Migrate from object-storage to meta-storage for better integration.

### Section 8.1: Meta-Storage Provider

#### Subsection 8.1.1: Meta-Storage Provider Implementation
- **Files**: `impl/metastorage/metastorage_provider.go` (new file)
- **Changes**:
  - Implement `AuditLogProvider` interface using meta-storage:
    - `SaveEntry(ctx, entry AuditEntry) error`
    - `LoadEntry(ctx, entryID string) (*AuditEntry, error)`
    - `ListEntries(ctx, filters QueryFilters) ([]AuditEntry, error)`
    - `DeleteEntry(ctx, entryID string) error`
    - `GetLastHash(ctx) (string, error)`
    - `SaveLastHash(ctx, hash string) error`
  - Use meta-storage bucket: `audit_logs` (key: `EntryID`)
  - Store hash chain state in meta-storage: `audit_log_chain` bucket (key: `last_hash`)
  - Store sync queue in meta-storage: `audit_log_sync_queue` bucket (key: `EntryID`)
- **Dependencies**: Section 1.1, Epic 1
- **Estimated Effort**: 3 days

#### Subsection 8.1.2: Object-Storage Provider (Backward Compatibility)
- **Files**: `impl/objectstorage/objectstorage_provider.go` (new file)
- **Changes**:
  - Implement `AuditLogProvider` interface using object-storage:
    - Keep existing object-storage implementation for backward compatibility
    - Mark as deprecated (prefer meta-storage)
  - Support migration path from object-storage to meta-storage
- **Dependencies**: 8.1.1
- **Estimated Effort**: 2 days

---

## Epic 9: Health Monitoring and Observability

**Goal**: Add comprehensive health monitoring following vm-gateway pattern.

### Section 9.1: Health Status Tracking

#### Subsection 9.1.1: Health Status Types
- **Files**: `types/health.go`
- **Changes**:
  - Define `HealthStatus` enum:
    - `HealthStatusHealthy`
    - `HealthStatusWarning` (queue >80% full)
    - `HealthStatusQueueFull` (queue 100% full, operations paused)
    - `HealthStatusSyncFailed` (sync failures detected)
    - `HealthStatusDegraded` (hash chain issues, provider errors)
  - Define `AuditLogHealth` struct:
    - `Status HealthStatus`
    - `QueueDepth int` (current queue size)
    - `QueueMaxSize int` (max queue size)
    - `QueueUsagePercent float64`
    - `IsPaused bool` (operations paused due to queue full)
    - `LastSyncTime time.Time`
    - `LastSyncSuccess bool`
    - `SyncFailures int` (count of recent sync failures)
    - `EntriesLogged int64` (total count)
    - `EntriesSynced int64` (total count)
    - `EntriesPending int64` (total count)
    - `HashChainIntegrity bool` (hash chain is intact)
    - `ProviderHealth string` (provider-specific health status)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 9.1.2: Health Snapshot API
- **Files**: `audit_log.go`, `impl/audit_log_impl.go`
- **Changes**:
  - Add `HealthSnapshot() AuditLogHealth` method to interface
  - Implement health snapshot:
    - Query queue status
    - Query sync status
    - Query hash chain integrity
    - Query provider health
    - Aggregate into `AuditLogHealth` struct
  - Follow vm-gateway pattern for health snapshots
- **Dependencies**: 9.1.1, Epic 3, Epic 4, Epic 6
- **Estimated Effort**: 2 days

### Section 9.2: Operational Metrics

#### Subsection 9.2.1: Metrics Tracking
- **Files**: `impl/metrics.go` (new file)
- **Changes**:
  - Track operational metrics:
    - Entries logged per second (by entry type)
    - Entries synced per second
    - Sync latency (P50, P95, P99)
    - Sync success rate
    - Queue depth over time
    - Hash chain verification results
    - Cleanup statistics
  - Expose metrics via health snapshot or separate metrics endpoint
- **Dependencies**: 9.1.2
- **Estimated Effort**: 2 days

#### Subsection 9.2.2: Event Emission
- **Files**: `impl/audit_log_impl.go`
- **Changes**:
  - Emit operational events (via event-bus):
    - `audit_log.queue_full` (when queue is full)
    - `audit_log.queue_resumed` (when queue has space again)
    - `audit_log.sync_failed` (when sync fails)
    - `audit_log.sync_succeeded` (when sync succeeds)
    - `audit_log.tamper_detected` (when hash chain is broken)
    - `audit_log.cleanup_started`, `audit_log.cleanup_completed`
    - `audit_log.health_degraded` (when health issues detected)
  - Use structured event types (similar to vm-gateway event types)
- **Dependencies**: 9.2.1
- **Estimated Effort**: 1 day

---

## Epic 10: Entry Type Enhancements

**Goal**: Add missing entry types and enhance existing ones for workflow requirements.

### Section 10.1: New Entry Types

#### Subsection 10.1.1: Dataset Lifecycle Entries
- **Files**: `types/types.go`
- **Changes**:
  - Add `DatasetLifecycleEntry` struct:
    - `AuditEntry`
    - `DeviceID`, `DeviceType`
    - `DatasetID`
    - `Action string` (created, labeled, uploaded, deleted)
    - `DataUnitCount int`
    - `Metadata map[string]interface{}`
  - Add `EntryTypeDatasetLifecycle` constant
  - Add `LogDatasetLifecycle(ctx, entry DatasetLifecycleEntry) error` method
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 10.1.2: Recovery Action Entries
- **Files**: `types/types.go`
- **Changes**:
  - Add `RecoveryActionEntry` struct:
    - `AuditEntry`
    - `DeviceID`, `DeviceType`
    - `RecoveryReason string` (storage_corruption, integrity_failure, operator_initiated)
    - `CorruptedResources []string` (model, dataset, etc.)
    - `LastKnownGoodState map[string]interface{}`
    - `VMResponseStatus string`
    - `Metadata map[string]interface{}`
  - Add `EntryTypeRecoveryAction` constant
  - Add `LogRecoveryAction(ctx, entry RecoveryActionEntry) error` method
- **Dependencies**: 10.1.1
- **Estimated Effort**: 1 day

#### Subsection 10.1.3: Model Deployment Entry Enhancement
- **Files**: `types/types.go`
- **Changes**:
  - Enhance `ModelDeploymentEntry`:
    - Add `DeviceType` field
    - Add `Action` field values: `deploy`, `verify`, `activate`, `deactivate`, `remove`
    - Add `VerificationResults map[string]interface{}` (signature, hash, compatibility checks)
    - Add `DeploymentStatus string` (deployed, verification_failed, activation_failed)
  - Update documentation
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

---

## Epic 11: Documentation and Testing

**Goal**: Add comprehensive documentation and testing following vm-gateway pattern.

### Section 11.1: Documentation

#### Subsection 11.1.1: Package Documentation
- **Files**: `doc.go` (new file)
- **Changes**:
  - Add comprehensive package documentation (similar to vm-gateway/doc.go):
    - Architecture overview
    - Provider-agnostic design
    - Hash chain integrity
    - Sync queue management
    - Retention and cleanup
    - VM sync protocol
    - Configuration examples
    - Usage examples
    - Lifecycle management
    - Health monitoring
  - Document device-agnostic design
  - Document tamper-evident properties
  - Document pause-on-full behavior
- **Dependencies**: All epics
- **Estimated Effort**: 1 day

#### Subsection 11.1.2: API Documentation
- **Files**: All interface files
- **Changes**:
  - Add comprehensive method documentation
  - Document error conditions
  - Document return values
  - Add usage examples
  - Document hash chain integrity
  - Document sync queue behavior
- **Dependencies**: 11.1.1
- **Estimated Effort**: 1 day

### Section 11.2: Testing

#### Subsection 11.2.1: Unit Tests
- **Files**: `*_test.go` files
- **Changes**:
  - Test hash chain integrity
  - Test sync queue management
  - Test pause-on-full behavior
  - Test retention and cleanup
  - Test VM sync protocol
  - Test health monitoring
  - Test provider abstraction
  - Test device-agnostic types
- **Dependencies**: All epics
- **Estimated Effort**: 3 days

#### Subsection 11.2.2: Integration Tests
- **Files**: `*_integration_test.go` files
- **Changes**:
  - Test full audit log lifecycle (log, persist, sync, cleanup)
  - Test sync queue with real VM failures
  - Test hash chain integrity with corruption injection
  - Test retention and cleanup with real provider
  - Test pause-on-full behavior
  - Test health monitoring
- **Dependencies**: 11.2.1
- **Estimated Effort**: 2 days

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~1.5 weeks
- **Epics**: 1 (Provider-Agnostic Architecture), 2 (Device-Agnostic Architecture)
- **Rationale**: Establishes the architectural foundation and type system

### Phase 2: Core Features (Epics 3, 4, 5)
- **Duration**: ~2.5 weeks
- **Epics**: 3 (Sync Queue Management), 4 (Sync Trigger Optimization), 5 (Retention and Cleanup)
- **Rationale**: Implements core production features

### Phase 3: Integrity and Sync (Epics 6, 7, 8)
- **Duration**: ~2 weeks
- **Epics**: 6 (Hash Chain Integrity), 7 (VM Sync Protocol), 8 (Storage Provider Migration)
- **Rationale**: Implements integrity verification and VM sync

### Phase 4: Enhancement and Polish (Epics 9, 10, 11)
- **Duration**: ~1.5 weeks
- **Epics**: 9 (Health Monitoring), 10 (Entry Type Enhancements), 11 (Documentation and Testing)
- **Rationale**: Completes observability, entry types, and documentation

**Total Estimated Duration**: ~7.5 weeks

---

## Migration Notes

### Breaking Changes
- All `CameraID` references become `DeviceID`
- Storage provider preference changes (meta-storage preferred over object-storage)
- Configuration structure changes (sync interval, retention, queue configs)
- Entry types enhanced (new fields, new entry types)

### Data Migration
- Existing audit entries may need migration (if storage provider changes)
- Hash chain state needs to be migrated to meta-storage
- Sync queue state needs to be initialized

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Verify hash chain integrity
- Verify sync queue behavior
- Verify pause-on-full behavior
- Gradual rollout to production with monitoring
- Monitor sync success rate and queue depth
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Provider-agnostic architecture implemented (following vm-gateway pattern)
2. ✅ Device-agnostic types and methods implemented
3. ✅ Sync queue management implemented and tested
4. ✅ Pause-on-full behavior implemented and tested
5. ✅ Sync trigger optimization implemented (5 minutes or 1000 records)
6. ✅ Retention updated to 90 days and cleanup implemented
7. ✅ Hash chain integrity verification implemented and tested
8. ✅ VM sync protocol implemented with idempotency
9. ✅ Storage provider migration to meta-storage completed
10. ✅ Health monitoring implemented and tested
11. ✅ Entry type enhancements completed
12. ✅ Comprehensive documentation added
13. ✅ Full test coverage (unit, integration)

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **Architecture should follow vm-gateway pattern** (but simpler, as audit-log is a simpler service)
- **Provider-agnostic design is mandatory** (support meta-storage now, object-storage for backward compatibility)
- **Device-agnostic implementation is mandatory** (not just camera support)
- **Hash chain integrity is critical** (tamper-evident property must be maintained)
- **Never drop audit records** (even if queue is full, pause operations instead)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

