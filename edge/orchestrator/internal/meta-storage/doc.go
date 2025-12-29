/*
Package metastorage provides a unified, provider-agnostic interface for metadata storage operations.

The Meta Storage service manages metadata about stored objects, devices, models, events, and system state.
It abstracts away the details of storage provider implementations, allowing the system to work with any
storage backend (BoltDB, SQLite, PostgreSQL, etc.).

# Architecture Overview

The storage service follows a provider-agnostic architecture pattern similar to vm-gateway:

	┌─────────────────────────────────────────────────────────┐
	│              MetaDataStore Interface                      │
	│  (Unified API for Metadata Storage Operations)           │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│  BoltDB Provider   │         │  SQLite Provider     │
	│  (Current)         │         │  (Future)            │
	│                    │         │                      │
	│  - Local file      │         │  - Local file        │
	│  - Key-value       │         │  - SQL database      │
	│  - Embedded        │         │  - Embedded          │
	└────────────────────┘         └──────────────────────┘

The service is composed of:
  - MetaDataStore interface: High-level operations (CRUD, queries, lifecycle)
  - MetaStorageProvider interface: Low-level storage operations (buckets, keys, values)
  - Provider implementations: BoltDB (current), SQLite, PostgreSQL (future)
  - Managers: Quota, Retention, Integrity, Schema Migration

# Provider-Agnostic Design

The storage service is designed to be provider-agnostic:

  - Storage providers: BoltDB (current), SQLite, PostgreSQL (future)
  - Device types: Cameras, sensors, audio devices, and other IoT devices
  - Data types: Images, sensor readings, audio samples, video clips, etc.

This allows the system to:
  - Switch storage providers without changing application code
  - Support different deployment scenarios (embedded with BoltDB, production with PostgreSQL)
  - Add new providers by implementing the MetaStorageProvider interface

The MetaStorageProvider interface defines low-level operations:
  - CreateBucket, DeleteBucket, BucketExists
  - Put, Get, Delete, List
  - HealthCheck

The MetaDataStore interface defines high-level operations:
  - Device metadata operations
  - Data unit operations
  - Model deployment operations
  - Security event operations
  - Event bus operations
  - ML lifecycle state operations

# Device-Agnostic Design

The storage service is device-agnostic, supporting various device types:

  - Cameras (IP cameras, USB cameras)
  - Sensors (temperature, motion, door sensors, etc.)
  - Audio devices (microphones, speakers)
  - Other IoT devices

All device-specific operations use generic DeviceID and DeviceType:
  - DeviceID: Unique identifier for any device
  - DeviceType: Type of device (camera, sensor, audio, etc.)

Legacy camera-specific methods are deprecated in favor of device-agnostic methods:
  - SaveCamera → SaveDevice
  - GetCamera → GetDevice
  - ListCameras → ListDevices

# Configuration

The storage service uses a provider-agnostic configuration structure.

## Basic Configuration (BoltDB)

	provider: bbolt
	data_dir: /var/lib/view-guard-meta/data

## Advanced Configuration (BoltDB with custom settings)

	provider: bbolt
	data_dir: /var/lib/view-guard-meta/data
	bbolt:
	  database_file: meta.db
	  file_mode: 0600
	  timeout: 1  # seconds
	  no_sync: false  # for performance tuning

## Configuration with Quota Management

	provider: bbolt
	data_dir: /var/lib/view-guard-meta/data
	quota:
	  max_size_mb: 1000
	  warning_threshold_percent: 80
	  full_threshold_percent: 95
	  max_records_per_bucket: 1000000
	  per_bucket_limits:
	    event_bus: 500000
	    security_events: 200000

## Configuration with Retention Policies

	provider: bbolt
	data_dir: /var/lib/view-guard-meta/data
	retention:
	  event_bus_retention_hours: 24
	  dead_letter_retention_days: 90
	  edge_state_history_retention_days: 30
	  cleanup_interval_hours: 6
	  per_bucket_retention:
	    event_bus: 24
	    dead_letter_events: 2160  # 90 days in hours

## Future PostgreSQL Configuration

	provider: postgres
	endpoint: postgresql://localhost:5432/viewguard
	username: viewguard
	password: secret
	database: viewguard
	max_connections: 10
	timeout: 5  # seconds

# Usage Examples

## Basic Usage with Dependency Injection (Fx)

	import (
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
		"go.uber.org/fx"
	)

	// In your Fx module
	var Module = fx.Module("meta-storage",
		fx.Provide(metastorage.MetaStorageProvider),
	)

	// The service will be automatically started and stopped by Fx

## Manual Creation

	import (
		"context"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
		"go.uber.org/zap"
	)

	cfg := &types.MetaStorageConfig{
		Provider: "bbolt",
		DataDir:  "/var/lib/view-guard-meta/data",
		Quota: &types.QuotaConfig{
			MaxSizeMB:            1000,
			WarningThresholdPercent: 80,
			FullThresholdPercent:    95,
		},
		Retention: &types.RetentionConfig{
			EventBusRetentionHours: 24,
			CleanupIntervalHours:   6,
		},
	}

	logger := zap.NewNop()
	store, err := metastorage.NewMetaDataStore(context.Background(), cfg, logger)
	if err != nil {
		// handle error
	}

	// Start the service
	if err := store.Start(context.Background()); err != nil {
		// handle error
	}
	defer store.Stop(context.Background())

## Saving Device Metadata

	device := types.DeviceMetadata{
		DeviceID:   types.DeviceID("camera-001"),
		DeviceType: types.DeviceTypeCamera,
		Name:       "Front Door Camera",
		Location:   "Front Door",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := store.SaveDevice(ctx, device); err != nil {
		// handle error
	}

## Saving Data Unit Metadata

	dataUnit := types.DataUnitMetadata{
		ID:         "screenshot-001",
		DeviceID:   types.DeviceID("camera-001"),
		DeviceType: types.DeviceTypeCamera,
		DataType:   "image",
		Label:      "motion",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := store.SaveDataUnit(ctx, dataUnit); err != nil {
		// handle error
	}

## Querying with Filters

	filters := &types.DataUnitFilters{
		DeviceID: &deviceID,
		DataType: stringPtr("image"),
		Label:    stringPtr("motion"),
	}

	dataUnits, err := store.ListDataUnits(ctx, filters)
	if err != nil {
		// handle error
	}

## ML Lifecycle State Management

	// Save ML lifecycle state
	state := types.MLLifecycleStateInfo{
		DeviceID:    types.DeviceID("camera-001"),
		DeviceType:  types.DeviceTypeCamera,
		State:       types.MLLifecycleStateActive,
		ModelID:     "model-001",
		ModelVersion: "v1.0.0",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := store.SaveMLLifecycleState(ctx, string(state.DeviceID), state); err != nil {
		// handle error
	}

	// Update with Compare-And-Swap (CAS)
	updated, err := store.UpdateMLLifecycleStateCAS(ctx, string(state.DeviceID), state.Version, func(s types.MLLifecycleStateInfo) types.MLLifecycleStateInfo {
		s.State = types.MLLifecycleStateDeploying
		s.UpdatedAt = time.Now()
		return s
	})
	if err != nil {
		// handle error (may be version conflict)
	}

# Lifecycle Management

The storage service follows the vm-gateway pattern for lifecycle management:

  - Service-owned lifecycle: The MetaDataStore manages provider lifecycle
  - Start/Stop methods: Service initialization and cleanup
  - Fx integration: Automatic lifecycle management via Fx hooks

## Service Lifecycle

The service lifecycle includes:

  1. Provider initialization: Open database connections
  2. Bucket initialization: Create required buckets/namespaces
  3. Schema migration: Apply pending schema migrations
  4. Manager initialization: Start quota, retention, integrity managers
  5. Background tasks: Start periodic cleanup and integrity checks

## Provider Lifecycle

Storage providers are managed by the service:
  - Providers are opened during service Start()
  - Providers are closed during service Stop()
  - Providers do not register their own Fx lifecycle hooks

# Health Monitoring

The storage service provides comprehensive health monitoring:

## Health Snapshot

	health := store.HealthSnapshot(ctx)

The health snapshot includes:
  - Overall status: healthy, warning, full, corrupted
  - Quota information: usage, limits, thresholds
  - Integrity errors: count of detected errors
  - Provider health: provider-specific status
  - Bucket counts: record counts per bucket
  - Total records: total number of records
  - Database size: database file size in MB
  - Last cleanup time: when retention cleanup last ran
  - Cleanup statistics: records deleted, space freed
  - Schema version: current schema version

## Health Status Values

  - HealthStatusHealthy: Service is operating normally
  - HealthStatusWarning: Service is degraded but operational (quota warning, etc.)
  - HealthStatusFull: Storage quota exceeded, write operations rejected
  - HealthStatusCorrupted: Integrity errors detected, manual intervention required

## Health Check Integration

The health snapshot can be exposed via HTTP endpoints for monitoring systems.

# Schema Versioning

The storage service includes a schema versioning system:

## Schema Migrations

Schema migrations are applied automatically on service startup:
  - Migrations are registered in order (by version number)
  - Only pending migrations are applied
  - Migrations are idempotent (safe to run multiple times)

## Migration Registration

	import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/impl"

	migrator := impl.NewSchemaMigrator(provider, logger)
	if err := impl.RegisterDefaultMigrations(migrator, provider, logger); err != nil {
		// handle error
	}

## Custom Migrations

	migration := &MyCustomMigration{
		version:     5,
		description: "Add new field to device metadata",
	}
	if err := migrator.RegisterMigration(migration); err != nil {
		// handle error
	}

## Migration Events

Schema migrations emit events:
  - storage.schema_migration_started: Before each migration
  - storage.schema_migration_completed: After each successful migration

# Bucket Organization

The storage service organizes data into buckets (namespaces):

## Standard Buckets

  - devices: Device metadata (replaces cameras)
  - data_units: Data unit metadata (replaces screenshots)
  - video_clips: Video clip metadata (replaces clips)
  - ml_lifecycle: ML lifecycle state per device
  - pending_model_deployments: Pending model deployments
  - model_deployments: Model deployment metadata (replaces deployed_models)
  - security_events: Security event metadata
  - event_bus: Event bus persistence
  - event_queue: Event queue
  - dead_letter_events: Dead letter events
  - pending_data_unit_requests: Pending data unit capture requests
  - edge_state: Edge state metadata
  - edge_state_history: Edge state history
  - edge_capabilities: Edge capabilities metadata
  - storage_state: Storage entry metadata
  - _meta: Schema version and metadata

## Bucket Naming Convention

  - Use snake_case
  - Be descriptive and clear
  - Avoid abbreviations unless widely understood
  - Prefix system buckets with underscore (_meta)

# Production Features

The storage service includes production-ready features:

## Quota Management

Quota management tracks storage usage and enforces limits:

  - Database size tracking: Monitors database file size
  - Per-bucket record limits: Limits records per bucket
  - Warning thresholds: Emits warnings at 80% usage
  - Full thresholds: Rejects writes at 95% usage
  - Periodic monitoring: Checks quota every 5 minutes

Quota events:
  - storage.warning: Emitted when quota usage >= warning threshold
  - storage.full: Emitted when quota usage >= full threshold
  - storage.quota_exceeded: Emitted when write operation is rejected

## Retention Policies

Retention policies automatically clean up expired records:

  - Per-bucket retention: Configurable retention per bucket
  - Automatic cleanup: Runs periodically (default: every 6 hours)
  - Timestamp extraction: Automatically extracts timestamps from records
  - Cleanup statistics: Tracks records deleted and space freed

Retention events:
  - storage.cleanup_started: Emitted when cleanup starts
  - storage.cleanup_completed: Emitted when cleanup completes (with statistics)

Default retention policies:
  - event_bus: 24 hours
  - dead_letter_events: 90 days
  - edge_state_history: 30 days
  - Other buckets: No retention (infinite)

## Integrity Verification

Integrity verification detects corruption and data inconsistencies:

  - Database file integrity: Checks database file accessibility
  - Bucket existence: Verifies all required buckets exist
  - Record format: Validates JSON format of records
  - Orphaned records: Detects references to non-existent objects
  - Periodic checks: Runs daily by default

Integrity events:
  - storage.corruption_detected: Emitted when corruption is detected

Recovery suggestions:
  - VM-assisted resync: Recommendations for recovering from corruption
  - Schema migration: Suggestions for fixing missing buckets

## Event Emission

The storage service emits operational events for monitoring:

  - Quota events: storage.warning, storage.full, storage.quota_exceeded
  - Cleanup events: storage.cleanup_started, storage.cleanup_completed
  - Corruption events: storage.corruption_detected
  - Migration events: storage.schema_migration_started, storage.schema_migration_completed

Events are emitted via the StorageEventEmitter interface, which can be implemented
by the event-bus package to avoid import cycles.

# Error Handling

The storage service uses sentinel errors for common error conditions:

  - ErrNotInitialized: Service or component not initialized
  - ErrAlreadyStarted: Service already started
  - ErrQuotaExceeded: Storage quota exceeded
  - ErrRecordNotFound: Record not found
  - ErrCorruptionDetected: Storage corruption detected
  - ErrInvalidSchemaVersion: Invalid schema version

These errors can be checked using errors.Is() for programmatic error handling:

	if errors.Is(err, metastorage.ErrQuotaExceeded) {
		// Handle quota exceeded
	}

# Thread Safety

The storage service is thread-safe:
  - All operations are safe for concurrent use
  - Providers use transactions for atomic operations
  - Managers use mutexes for state protection

# Performance Considerations

  - BoltDB: Embedded key-value store, suitable for embedded deployments
  - NoSync option: Can disable fsync for better performance (with data loss risk)
  - Batch operations: Use List() for bulk reads
  - Quota caching: Quota status is cached and updated periodically

# Testing

See the test files for usage examples:
  - *_test.go: Unit tests
  - *_integration_test.go: Integration tests
*/
package metastorage

