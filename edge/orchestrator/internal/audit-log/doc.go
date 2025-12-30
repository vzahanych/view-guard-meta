/*
Package auditlog provides tamper-proof audit logging for security-sensitive operations.

The Audit Log service manages audit trails of security-sensitive operations, including data access,
authentication, authorization, configuration changes, model deployments, and security events.
Audit logs are stored temporarily in edge storage and synced to VM for long-term retention.

# Architecture Overview

The audit log service follows a provider-agnostic architecture pattern similar to vm-gateway and meta-storage:

	┌─────────────────────────────────────────────────────────┐
	│              AuditLogService Interface                   │
	│  (Unified API for Audit Logging Operations)             │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│  Meta-Storage      │         │  Object-Storage      │
	│  Provider          │         │  Provider            │
	│  (Preferred)       │         │  (Deprecated)        │
	│                    │         │                      │
	│  - Buckets         │         │  - Key-value         │
	│  - Transactions    │         │  - JSON storage      │
	│  - Provider-agnostic│         │  - Legacy support    │
	└────────────────────┘         └──────────────────────┘

The service is composed of:
  - AuditLogService interface: High-level operations (logging, querying, syncing)
  - AuditLogProvider interface: Low-level storage operations (save, load, list, delete)
  - Provider implementations: Meta-storage (current), object-storage (deprecated)
  - Managers: Sync Queue, Sync Trigger, Cleanup, Hash Chain, Metrics
  - Event Emitter: Operational event emission via event-bus

# Provider-Agnostic Design

The audit log service is designed to be provider-agnostic:

  - Storage providers: Meta-storage (preferred), object-storage (deprecated)
  - Device types: Cameras, sensors, audio devices, and other IoT devices
  - Entry types: Data access, authentication, authorization, configuration, model deployment, security events, dataset lifecycle, recovery actions

This allows the system to:
  - Switch storage providers without changing application code
  - Support different deployment scenarios (meta-storage for production)
  - Add new providers by implementing the AuditLogProvider interface

The AuditLogProvider interface defines storage operations:
  - SaveEntry, LoadEntry, ListEntries, DeleteEntry
  - GetLastHash, SaveLastHash (for hash chain management)
  - HealthCheck

The AuditLogService interface defines high-level operations:
  - Logging operations: LogDataAccess, LogAuthentication, LogAuthorization, etc.
  - Query operations: QueryAuditLogs, GetAuditLogEntry
  - Sync operations: SyncToVM
  - Cleanup operations: CleanupOldLogs
  - Health monitoring: HealthSnapshot

# Device-Agnostic Design

The audit log service is device-agnostic, supporting various device types:

  - Cameras (IP cameras, USB cameras)
  - Sensors (temperature, motion, door sensors, etc.)
  - Audio devices (microphones, speakers)
  - Other IoT devices

All device-specific operations use generic DeviceID and DeviceType:
  - DeviceID: Unique identifier for any device
  - DeviceType: Type of device (camera, sensor, audio, etc.)

Entry types are device-agnostic:
  - ModelDeploymentEntry: Supports deployment to any device type
  - DatasetLifecycleEntry: Supports dataset operations for any device
  - RecoveryActionEntry: Supports recovery actions on any device

# Hash Chain Integrity

The audit log service implements a cryptographic hash chain to ensure tamper-evident audit logs:

## Hash Chain Structure

Each audit log entry includes:
  - Hash: SHA256 hash of the entry (calculated as SHA256(previousHash:entryJSON))
  - PreviousHash: Hash of the previous entry in the chain
  - ID: Unique entry identifier

The hash chain creates an unbreakable link between entries:
  - First entry: PreviousHash is empty (or zero hash)
  - Subsequent entries: PreviousHash references the hash of the previous entry
  - Any modification to an entry breaks the chain (tamper detection)

## Integrity Verification

Hash chain integrity is verified:
  - On service startup: Full chain verification during initialization
  - Periodically: Daily integrity checks in background
  - On-demand: Via HealthSnapshot() method

Verification checks:
  - Each entry's hash matches calculated hash
  - Each entry's previous_hash matches previous entry's hash
  - Chain is unbroken (stops verification at first broken link)

## Tamper Detection

When tampering is detected:
  - Critical alert logged with detailed information
  - Event emitted: audit_log.tamper_detected (critical priority, cannot be dropped)
  - Entries after break point marked as suspicious
  - Chain continuation possible from last verified entry
  - Operator intervention required for recovery

## Recovery

Hash chain recovery:
  - Identifies break point using verification report
  - Marks entries after break as suspicious
  - Sets last hash to last verified entry (allows chain continuation)
  - Returns error indicating operator intervention needed

# Sync Queue Management

The audit log service implements a sync queue for managing failed syncs and ensuring at-least-once delivery:

## Queue Configuration

  - MaxQueueSize: 100,000 records (default)
  - RetryBackoff: Exponential backoff with jitter (default: 1 second initial)
  - MaxRetries: 10 retries (default)
  - Max backoff capped at 1 hour

## Queue Behavior

  - Entries are persisted locally before enqueueing
  - Entries remain in queue until VM acknowledgment
  - Failed syncs are retried with exponential backoff
  - Entries are NEVER dropped, even if queue is full

## Pause-on-Full Behavior

When sync queue is full (100% capacity):
  - Operations are PAUSED (never dropped)
  - ErrQueueFull is returned to callers
  - Event emitted: audit_log.queue_full (operational health priority)
  - Sensitive operations checked: dataset creation, model deployment, security events, recovery actions

When queue has space again:
  - Operations resume automatically
  - Event emitted: audit_log.queue_resumed (operational health priority)

This ensures audit records are never lost, even during network outages or VM unavailability.

# Sync Trigger Optimization

The audit log service implements flexible sync triggers to optimize VM synchronization:

## Sync Trigger Modes

  - time_based: Sync when sync interval has passed (default: 5 minutes)
  - count_based: Sync when batch size reached (default: 1000 records)
  - hybrid: Sync when EITHER time threshold OR count threshold is reached (default, recommended)

Hybrid mode ensures:
  - Timely sync: At least every 5 minutes
  - Efficient sync: Up to 1000 records per sync
  - Optimal balance: Neither too frequent nor too delayed

## Sync Batching

  - SyncBatchSize: 1000 records per batch (default)
  - Batches are processed sequentially
  - Continues with next batch on failure (doesn't stop entire sync)

# Retention and Cleanup

The audit log service implements retention policies and automatic cleanup:

## Retention Configuration

  - RetentionDays: 90 days (default, updated from 7 days for production)
  - CleanupInterval: 24 hours (default)
  - CleanupBatchSize: 1000 entries per batch (default)

## Cleanup Behavior

  - Only deletes entries that are synced to VM (never deletes unsynced entries)
  - Uses conservative approach: retention + 7 days buffer for safe deletion
  - Processes entries in batches for efficient cleanup
  - Emits events: audit_log.cleanup_started, audit_log.cleanup_completed

## Cleanup Statistics

Cleanup tracks:
  - EntriesDeleted: Number of entries deleted
  - EntriesSkipped: Number of entries skipped (not synced)
  - ErrorsEncountered: Number of errors encountered
  - CleanupDuration: Duration of cleanup operation

# VM Sync Protocol

The audit log service implements a robust VM sync protocol with idempotency and at-least-once delivery:

## Sync Protocol Features

  - Idempotency: Per-entry and request-level idempotency keys
  - At-least-once delivery: Entries persisted locally before sync, remain in queue until VM acknowledgment
  - Batch processing: Syncs up to 1000 entries per batch
  - Error handling: Continues with next batch on failure
  - Response processing: Handles acknowledged, failed, and duplicate entries

## Sync Request Structure

  - EdgeID: Edge identifier
  - IdempotencyKey: Request-level idempotency key
  - Entries: Array of audit log entries with per-entry idempotency keys
  - Batch metadata: Start time, end time, entry count

## Sync Response Processing

  - Acknowledged entries: Marked as synced when VM confirms (SyncedCount >= entry_count)
  - Failed entries: Marked for retry when sync fails
  - Duplicate entries: Detected when SyncedCount > entry_count (marked as synced)
  - Partial sync: Handles cases where SyncedCount < entry_count

## At-Least-Once Delivery Guarantee

  - Entries persisted locally before sync attempt
  - Entries remain in queue until VM acknowledgment
  - Retry on failure with exponential backoff
  - Never drop entries (pause operations instead)

# Configuration

The audit log service uses a provider-agnostic configuration structure.

## Basic Configuration

	audit_log:
	  enabled: true
	  provider: metastorage
	  retention_days: 90
	  sync_interval: 5m
	  sync_batch_size: 1000
	  sync_trigger_mode: hybrid

## Advanced Configuration

	audit_log:
	  enabled: true
	  provider: metastorage
	  retention_days: 90
	  sync_interval: 5m
	  sync_batch_size: 1000
	  sync_trigger_mode: hybrid
	  sync_queue_config:
	    max_queue_size: 100000
	    retry_backoff: 1s
	    max_retries: 10
	  cleanup_interval: 24h
	  cleanup_batch_size: 1000

## Configuration with Custom Sync Trigger

	audit_log:
	  enabled: true
	  provider: metastorage
	  retention_days: 90
	  sync_interval: 10m  # Time-based: sync every 10 minutes
	  sync_batch_size: 500  # Count-based: sync when 500 records queued
	  sync_trigger_mode: hybrid  # Use both triggers

# Usage Examples

## Basic Usage with Dependency Injection (Fx)

	import (
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log"
		"go.uber.org/fx"
	)

	// In your Fx module
	var Module = fx.Module("audit-log",
		fx.Provide(auditlog.AuditLogProvider),
	)

	// The service will be automatically started and stopped by Fx

## Manual Creation

	import (
		"context"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
		"go.uber.org/zap"
	)

	cfg := &types.AuditLogConfig{
		Enabled:       true,
		Provider:      "metastorage",
		RetentionDays: 90,
		SyncInterval:  5 * time.Minute,
		SyncBatchSize: 1000,
		SyncTriggerMode: "hybrid",
		SyncQueueConfig: &types.SyncQueueConfig{
			MaxQueueSize: 100000,
			RetryBackoff: 1 * time.Second,
			MaxRetries:   10,
		},
		CleanupInterval: 24 * time.Hour,
		CleanupBatchSize: 1000,
	}

	logger := zap.NewNop()
	// ... create dependencies (objectStorage, vmGateway, metaStorage, eventBus) ...
	
	service, err := auditlog.AuditLogProvider(
		lc,        // fx.Lifecycle
		cfg,
		objectStorage,
		vmGateway,
		logger,
		"edge-001", // edgeID
		metaStorage,
		eventBus,
	)
	if err != nil {
		// handle error
	}

	// Service will be automatically started and stopped by Fx

## Logging Data Access

	import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"

	entry := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{
			UserID:   "user-123",
			IPAddress: "192.168.1.100",
			Result:   "success",
		},
		ResourceType: types.ResourceTypeDataUnit,
		ResourceID:   "screenshot-001",
		Action:       "read",
		DeviceID:     types.DeviceID("camera-001"),
		DeviceType:   types.DeviceTypeCamera,
	}

	if err := auditLogService.LogDataAccess(ctx, entry); err != nil {
		if errors.Is(err, auditlog.ErrQueueFull) {
			// Queue is full - pause operations
			return fmt.Errorf("audit log queue full, operations paused")
		}
		// handle other errors
	}

## Logging Authentication

	entry := types.AuthenticationEntry{
		AuditEntry: types.AuditEntry{
			UserID:   "user-123",
			IPAddress: "192.168.1.100",
			Result:   "success",
		},
		Method:   "api_key",
		Identity: "user-123",
	}

	if err := auditLogService.LogAuthentication(ctx, entry); err != nil {
		// handle error
	}

## Logging Model Deployment

	entry := types.ModelDeploymentEntry{
		AuditEntry: types.AuditEntry{
			UserID:   "operator-001",
			IPAddress: "192.168.1.100",
			Result:   "success",
		},
		ModelID:    "model-001",
		ModelVersion: "v1.0.0",
		DeviceID:   types.DeviceID("camera-001"),
		DeviceType: types.DeviceTypeCamera,
		Action:     "deploy",
		Checksum:   "sha256:abc123...",
		VerificationResults: map[string]interface{}{
			"signature_valid": true,
			"hash_match":      true,
		},
		DeploymentStatus: "deployed",
	}

	if err := auditLogService.LogModelDeployment(ctx, entry); err != nil {
		// handle error
	}

## Querying Audit Logs

	filters := types.QueryFilters{
		StartTime:  timePtr(time.Now().Add(-24 * time.Hour)),
		EndTime:    timePtr(time.Now()),
		EntryType:  string(types.EntryTypeDataAccess),
		UserID:     "user-123",
		Result:     "success",
		Limit:      100,
		Offset:     0,
	}

	entries, err := auditLogService.QueryAuditLogs(ctx, filters)
	if err != nil {
		// handle error
	}

	for _, entry := range entries {
		// Process entry
	}

## Getting a Specific Entry

	entry, err := auditLogService.GetAuditLogEntry(ctx, "entry-id-123")
	if err != nil {
		if errors.Is(err, types.ErrRecordNotFound) {
			// Entry not found
			return
		}
		// handle other errors
	}

	// Process entry
	switch e := entry.(type) {
	case types.DataAccessEntry:
		// Handle data access entry
	case types.AuthenticationEntry:
		// Handle authentication entry
	// ... other entry types
	}

# Lifecycle Management

The audit log service follows the vm-gateway pattern for lifecycle management:

  - Service-owned lifecycle: The AuditLogService manages provider lifecycle
  - Start/Stop methods: Service initialization and cleanup
  - Fx integration: Automatic lifecycle management via Fx hooks

## Service Lifecycle

The service lifecycle includes:

  1. Provider initialization: Verify and initialize storage provider
  2. Hash chain initialization: Load last hash and verify chain continuity
  3. Sync queue initialization: Load queue state from provider (crash recovery)
  4. Manager initialization: Initialize sync trigger, cleanup, and hash chain managers
  5. Background tasks: Start sync worker, cleanup worker, integrity check worker

## Provider Lifecycle

Storage providers are managed by the service:
  - Providers are initialized during service Start()
  - Providers are closed during service Stop()
  - Providers do not register their own Fx lifecycle hooks

## Background Tasks

The service runs background tasks:
  - Sync worker: Syncs audit logs to VM (runs at sync interval)
  - Cleanup worker: Cleans up expired entries (runs at cleanup interval)
  - Integrity check worker: Verifies hash chain integrity (runs daily)

# Health Monitoring

The audit log service provides comprehensive health monitoring:

## Health Snapshot

	health := auditLogService.HealthSnapshot()

The health snapshot includes:
  - Status: Overall health status (healthy, warning, queue_full, sync_failed, degraded)
  - Queue metrics: Depth, max size, usage percent, paused state
  - Sync metrics: Last sync time, success status, failure count, entries synced
  - Entry metrics: Entries logged, entries synced, entries pending
  - Hash chain integrity: Integrity status from last verification
  - Provider health: Provider-specific health status

## Health Status Values

  - HealthStatusHealthy: Service is operating normally
  - HealthStatusWarning: Service is in warning state (e.g., queue >80% full)
  - HealthStatusQueueFull: Sync queue is 100% full and operations are paused
  - HealthStatusSyncFailed: Sync failures detected
  - HealthStatusDegraded: Service is degraded (hash chain issues or provider errors)

## Health Check Integration

The health snapshot can be exposed via HTTP endpoints for monitoring systems.

# Event Emission

The audit log service emits operational events via the event-bus for monitoring:

## Event Types

  - audit_log.queue_full: When queue becomes full (operational health priority)
  - audit_log.queue_resumed: When queue has space again (operational health priority)
  - audit_log.sync_failed: When sync fails (operational health priority)
  - audit_log.sync_succeeded: When sync succeeds (operational health priority)
  - audit_log.tamper_detected: When hash chain tampering detected (critical priority)
  - audit_log.cleanup_started: When cleanup begins (operational health priority)
  - audit_log.cleanup_completed: When cleanup finishes (operational health priority)
  - audit_log.health_degraded: When health status degrades (operational health priority)

## Event Drop Policy

  - Operational/health events: Cannot be dropped during storage pressure
  - Critical events (tamper_detected): Highest priority, cannot be dropped

## Event Data Structure

Events include structured data:
  - Queue events: Queue depth, max size, usage percent, paused state
  - Sync events: Entries synced, sync duration, error messages
  - Tamper events: Broken links, tamper indicators, verified entries, total entries
  - Cleanup events: Entries deleted, entries skipped, cleanup duration
  - Health events: Health status, health reason

# Error Handling

The audit log service uses sentinel errors for common error conditions:

  - ErrNotInitialized: Service or component not initialized
  - ErrAlreadyStarted: Service already started
  - ErrQueueFull: Sync queue is full (operations paused)
  - ErrSyncFailed: Sync to VM failed
  - ErrTamperDetected: Hash chain integrity verification detected tampering
  - ErrRecordNotFound: Record not found

These errors can be checked using errors.Is() for programmatic error handling:

	if errors.Is(err, auditlog.ErrQueueFull) {
		// Handle queue full condition
	}

# Thread Safety

The audit log service is thread-safe:
  - All operations are safe for concurrent use
  - Providers use transactions for atomic operations
  - Managers use mutexes for state protection

# Performance Considerations

  - Meta-storage provider: Efficient bucket-based storage with transactions
  - Sync batching: Up to 1000 entries per sync for efficient transfer
  - Queue management: In-memory queue with periodic persistence
  - Hash chain: Efficient SHA256 calculation, cached last hash
  - Cleanup: Batch processing with configurable batch size

# Integration with Other Services

The audit log service integrates with:

  - meta-storage: Storage provider for audit entry persistence (preferred)
  - event-bus: Operational event emission for monitoring
  - vm-gateway: VM sync protocol for long-term retention
  - object-storage: Legacy storage provider (deprecated)

# Testing

See the test files for usage examples:
  - *_test.go: Unit tests
  - test_provider.go: In-memory test provider for testing
*/
package auditlog

