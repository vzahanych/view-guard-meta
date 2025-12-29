# Meta Storage Refactoring Plan

**Date**: 2025-12-28  
**Target Documents**: 
- `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`
- `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- `edge/orchestrator/internal/vm-gateway/doc.go` (architectural pattern reference)

**Scope**: Complete refactoring of `meta-storage` package to align with production workflow requirements and follow vm-gateway architectural pattern  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the Meta Storage service implementation into full compliance with the production workflow specification and aligns it with the vm-gateway architectural pattern. The current implementation is camera-centric, lacks production features (ML lifecycle state management, quota enforcement, schema versioning, corruption detection, health monitoring), and doesn't follow the provider-agnostic architecture pattern.

**Key Transformation Areas**:
1. **Device-agnostic architecture**: Replace camera-centric terminology with device-agnostic types
2. **Provider-agnostic design**: Follow vm-gateway pattern with interface, types, and implementation separation
3. **ML lifecycle state management**: Add dedicated bucket and operations for per-device ML lifecycle state
4. **Production features**: Add quota enforcement, schema versioning, corruption detection, health monitoring
5. **Bucket organization**: Reorganize buckets according to workflow requirements (ml_lifecycle, pending_model_deployments, etc.)
6. **Observability**: Add health snapshot API and operational metrics

---

## Epic 1: Provider-Agnostic Architecture (Following vm-gateway Pattern)

**Goal**: Restructure the codebase to follow the vm-gateway architectural pattern with clear separation of concerns.

### Section 1.1: Interface and Types Separation

#### Subsection 1.1.1: Main Interface File
- **Files**: `meta_storage.go` (rename from `meta-storage-iface.go`)
- **Changes**:
  - Define `MetaDataStore` interface (main service interface)
  - Define sentinel errors (similar to vm-gateway):
    - `ErrNotInitialized`
    - `ErrAlreadyStarted`
    - `ErrQuotaExceeded`
    - `ErrRecordNotFound`
    - `ErrCorruptionDetected`
    - `ErrInvalidSchemaVersion`
  - Define factory function `NewMetaDataStore(ctx, config, logger)`
  - Define provider function `MetaStorageProvider(lc, cfg, logger)` with fx lifecycle
  - Add comprehensive package documentation (similar to vm-gateway/doc.go)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Types Package Structure
- **Files**: `types/` directory
- **Changes**:
  - Move all configuration types to `types/config.go`
  - Create `types/storage.go` for storage-related types:
    - `RecordMetadata` struct (key, bucket, created at, updated at, version)
    - `StorageQuota` struct (used, limit, warning threshold, full threshold)
    - `BucketInfo` struct (name, record count, size bytes)
    - `HealthStatus` enum (healthy, warning, full, corrupted)
  - Create `types/schema.go` for schema versioning:
    - `SchemaVersion` struct (version number, migration function)
    - `SchemaMigration` interface
  - Create `types/provider.go` for provider interface:
    - `MetaStorageProvider` interface (provider-agnostic operations)
    - Provider-specific configuration types
  - Create `types/errors.go` for error types
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Implementation Package Structure
- **Files**: `impl/` directory (rename from `bbolt-imp/`)
- **Changes**:
  - Create `impl/meta_storage_impl.go` (main implementation)
  - Create provider-specific implementations:
    - `impl/bbolt/bbolt_provider.go` (BoltDB implementation)
    - `impl/sqlite/sqlite_provider.go` (future: SQLite implementation)
    - `impl/postgres/postgres_provider.go` (future: PostgreSQL implementation)
  - Each provider implements `types.MetaStorageProvider` interface
  - Main implementation delegates to provider
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

### Section 1.2: Lifecycle Management

#### Subsection 1.2.1: Service Lifecycle
- **Files**: `impl/meta_storage_impl.go`
- **Changes**:
  - Implement `Start(ctx)` method:
    - Initialize provider
    - Verify connectivity
    - Create required buckets/namespaces
    - Run schema migrations
    - Initialize quota monitoring
    - Start background tasks (retention cleanup, health checks, integrity verification)
  - Implement `Stop(ctx)` method:
    - Stop background tasks gracefully
    - Close provider connections
    - Flush pending operations
    - Close database connections
  - Follow vm-gateway pattern: service owns lifecycle of sub-components
- **Dependencies**: 1.1.3
- **Estimated Effort**: 1 day

#### Subsection 1.2.2: Provider Lifecycle
- **Files**: `impl/bbolt/bbolt_provider.go` (and other providers)
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
- **Files**: All files in `meta_storage.go`, `types/`, `impl/`
- **Changes**:
  - Replace all `CameraID` references with `DeviceID`
  - Update function signatures: `SaveCamera` → `SaveDevice`, `GetCamera` → `GetDevice`
  - Update type definitions: `CameraMetadata` → `DeviceMetadata`
  - Update variable names and map keys
  - Update bucket names if camera-specific
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 2.1.2: Device-Agnostic Type Definitions
- **Files**: `types/storage.go`
- **Changes**:
  - Create `DeviceID` type alias (string)
  - Create `DeviceType` enum (camera, sensor, audio_device, etc.)
  - Update `DeviceMetadata` struct (replaces `CameraMetadata`):
    - `ID`, `Name`, `DeviceType` (replaces `Type` field)
    - `Manufacturer`, `Model`
    - `Enabled`, `Status`
    - `LastSeen`, `DiscoveredAt`
    - Device-specific fields (IPAddress, ONVIFEndpoint, RTSPURLs, etc.)
    - `SyncedWithVM`, `SyncedAt`, `VMDeviceID`
    - `CreatedAt`, `UpdatedAt`
  - Update `DeviceFilters` struct (replaces `CameraFilters`)
  - Update `ScreenshotMetadata` → `DataUnitMetadata` (device-agnostic)
  - Update `ClipMetadata` → `VideoClipMetadata` (device-agnostic, part of data units)
- **Dependencies**: 2.1.1
- **Estimated Effort**: 2 days

#### Subsection 2.1.3: Unified Device Operations Interface
- **Files**: `meta_storage.go`
- **Changes**:
  - Replace camera-specific methods with device-agnostic methods:
    - `SaveCamera` → `SaveDevice(ctx, device DeviceMetadata) error`
    - `UpdateCamera` → `UpdateDevice(ctx, deviceID string, updateFn func(DeviceMetadata) DeviceMetadata) (DeviceMetadata, error)`
    - `GetCamera` → `GetDevice(ctx, deviceID string) (DeviceMetadata, bool)`
    - `ListCameras` → `ListDevices(ctx, filters *DeviceFilters) ([]DeviceMetadata, error)`
    - `DeleteCamera` → `DeleteDevice(ctx, deviceID string) error`
  - Keep legacy methods for backward compatibility during migration (deprecated)
- **Dependencies**: 2.1.2
- **Estimated Effort**: 1 day

---

## Epic 3: ML Lifecycle State Management

**Goal**: Implement dedicated ML lifecycle state management as specified in the workflow document.

### Section 3.1: ML Lifecycle State Types

#### Subsection 3.1.1: ML Lifecycle State Types
- **Files**: `types/ml_lifecycle.go` (new file)
- **Changes**:
  - Define `MLLifecycleState` enum (from state-mng types):
    - `Unassigned`, `Assigned`, `AwaitingDataset`
    - `DatasetReadyLocal`, `DatasetUploadInProgress`, `DatasetUploaded`
    - `TrainingPending`, `ModelAvailable`, `ModelStored`
    - `InferenceActive`, `DegradedNoModel`, `RecoveryRequired`
  - Define `MLLifecycleStateInfo` struct:
    - `DeviceID`, `State`, `LastUpdated`, `Error`
    - `ModelID`, `ModelVersion`, `DatasetID`
    - `OfflineInferenceAllowed bool` (policy flag)
    - `LastKnownGoodState` (for recovery)
    - `Version int` (for CAS operations)
  - Define `MLLifecycleFilters` struct for querying
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: ML Lifecycle Operations Interface
- **Files**: `meta_storage.go`
- **Changes**:
  - Add ML lifecycle operations:
    - `SaveMLLifecycleState(ctx, deviceID string, stateInfo MLLifecycleStateInfo) error`
    - `GetMLLifecycleState(ctx, deviceID string) (*MLLifecycleStateInfo, bool)`
    - `UpdateMLLifecycleState(ctx, deviceID string, updateFn func(MLLifecycleStateInfo) MLLifecycleStateInfo) (*MLLifecycleStateInfo, error)`
    - `ListMLLifecycleStates(ctx, filters *MLLifecycleFilters) ([]MLLifecycleStateInfo, error)`
    - `DeleteMLLifecycleState(ctx, deviceID string) error`
  - Implement CAS (Compare-And-Swap) for idempotent updates:
    - `UpdateMLLifecycleStateCAS(ctx, deviceID string, expectedVersion int, updateFn func(MLLifecycleStateInfo) MLLifecycleStateInfo) (*MLLifecycleStateInfo, error)`
- **Dependencies**: 3.1.1
- **Estimated Effort**: 1 day

### Section 3.2: ML Lifecycle Bucket Implementation

#### Subsection 3.2.1: ML Lifecycle Bucket
- **Files**: `impl/ml_lifecycle_storage.go` (new file)
- **Changes**:
  - Define bucket name: `ml_lifecycle` (key: `DeviceID`)
  - Implement persistence format (JSON with version field for CAS)
  - Implement `SaveMLLifecycleState`:
    - Store state info in `ml_lifecycle` bucket
    - Include version field for CAS operations
    - Store with crash-safe semantics (atomic write)
  - Implement `GetMLLifecycleState`:
    - Load from `ml_lifecycle` bucket
    - Return nil if not found
  - Implement `UpdateMLLifecycleState`:
    - Load current state
    - Apply update function
    - Save updated state atomically
  - **CRITICAL: CAS (Compare-And-Swap) operations are REQUIRED for ML lifecycle state updates**
    - All state transitions must use `UpdateMLLifecycleStateCAS` to ensure atomicity and prevent race conditions
    - CAS prevents concurrent state transitions from overwriting each other
    - State Manager must use CAS for all ML lifecycle state updates (see state-mng Epic 2, Section 2.2.1)
  - Implement `UpdateMLLifecycleStateCAS`:
    - Load current state
    - Verify version matches expected
    - Apply update function
    - Increment version
    - Save updated state atomically
    - Return error if version mismatch (CAS failure)
- **Dependencies**: 3.1.2, Section 1.1
- **Estimated Effort**: 3 days

#### Subsection 3.2.2: Pending Model Deployments Bucket
- **Files**: `impl/pending_deployments_storage.go` (new file)
- **Changes**:
  - Define bucket name: `pending_model_deployments` (key: `DeviceID`)
  - Implement `SavePendingModelDeployment(ctx, deviceID string, deployment PendingModelDeployment) error`
  - Implement `GetPendingModelDeployment(ctx, deviceID string) (*PendingModelDeployment, bool)`
  - Implement `ListPendingModelDeployments(ctx) ([]PendingModelDeployment, error)`
  - Implement `DeletePendingModelDeployment(ctx, deviceID string) error`
  - Implement TTL cleanup (24 hours default, configurable)
  - Define `PendingModelDeployment` struct:
    - `DeviceID`, `ModelID`, `EventData`, `ReceivedAt`, `TTL`
- **Dependencies**: 3.2.1
- **Estimated Effort**: 2 days

---

## Epic 4: Bucket Organization and Schema

**Goal**: Reorganize buckets according to workflow requirements and implement schema versioning.

### Section 4.1: Bucket Organization

#### Subsection 4.1.1: Standard Bucket Names
- **Files**: `impl/buckets.go` (new file)
- **Changes**:
  - Define standard bucket names (constants):
    - `ml_lifecycle` - ML lifecycle state per device
    - `pending_model_deployments` - Pending model deployments
    - `devices` - Device metadata (replaces `cameras`)
    - `data_units` - Data unit metadata (replaces `screenshots`)
    - `video_clips` - Video clip metadata (replaces `clips`)
    - `security_events` - Security event metadata
    - `model_deployments` - Model deployment metadata (replaces `deployed_models`)
    - `edge_state` - Edge state metadata
    - `edge_capabilities` - Edge capabilities metadata
    - `event_bus` - Event bus persistence
    - `event_queue` - Event queue
    - `dead_letter_events` - Dead letter events
    - `pending_data_unit_requests` - Pending data unit capture requests (replaces `pending_snapshot_requests`)
    - `_meta` - Schema version and metadata
  - Implement bucket creation on startup
  - Implement bucket existence checks
- **Dependencies**: Section 1.1
- **Estimated Effort**: 1 day

#### Subsection 4.1.2: Bucket Migration
- **Files**: `impl/migration.go` (new file)
- **Changes**:
  - Implement bucket migration from old names to new names:
    - `cameras` → `devices`
    - `screenshots` → `data_units`
    - `clips` → `video_clips`
    - `deployed_models` → `model_deployments`
    - `pending_snapshot_requests` → `pending_data_unit_requests`
  - Implement data migration (copy records from old buckets to new buckets)
  - Implement migration rollback (if needed)
- **Dependencies**: 4.1.1
- **Estimated Effort**: 2 days

### Section 4.2: Schema Versioning

#### Subsection 4.2.1: Schema Version Management
- **Files**: `types/schema.go`
- **Changes**:
  - Define `SchemaVersion` struct:
    - `Version int` (incrementing number)
    - `AppliedAt time.Time`
    - `Description string`
  - Define `SchemaMigration` interface:
    - `Up(ctx) error` (apply migration)
    - `Down(ctx) error` (rollback migration)
    - `Version() int`
    - `Description() string`
  - Store schema version in `_meta` bucket (key: `schema_version`)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.2.2: Schema Migration System
- **Files**: `impl/schema_migration.go` (new file)
- **Changes**:
  - Implement `SchemaMigrator` struct:
    - Track registered migrations
    - Track current schema version
  - Implement `RegisterMigration(migration SchemaMigration)`
  - Implement `Migrate(ctx) error`:
    - Load current schema version
    - Apply pending migrations in order
    - Update schema version after each migration
    - Support rollback (if needed)
  - Implement idempotent migrations (safe to run multiple times)
  - Implement reversible migrations (for rollback)
  - Example migrations:
    - v1 → v2: Add `ml_lifecycle` bucket
    - v2 → v3: Migrate `cameras` → `devices`
    - v3 → v4: Add version field to ML lifecycle state (for CAS)
- **Dependencies**: 4.2.1
- **Estimated Effort**: 3 days

---

## Epic 5: Production Features - Quota and Retention

**Goal**: Implement quota enforcement and retention policies as specified in the workflow document.

### Section 5.1: Quota Management

#### Subsection 5.1.1: Quota Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `QuotaConfig` struct:
    - `MaxSizeMB int` (default: 1,000 MB)
    - `WarningThresholdPercent int` (default: 80)
    - `FullThresholdPercent int` (default: 95)
    - `MaxRecordsPerBucket int` (default: 1,000,000, configurable per bucket)
  - Add quota configuration to `MetaStorageConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Quota Tracking
- **Files**: `impl/quota_manager.go` (new file)
- **Changes**:
  - Implement `QuotaManager` struct:
    - Track current usage (database file size, record counts per bucket)
    - Track quota limits
    - Track thresholds
  - Implement `GetQuotaStatus(ctx) (*StorageQuota, error)`:
    - Query provider for database size
    - Count records per bucket
    - Calculate usage percentage
    - Return quota status
  - Implement periodic quota checks (background task, every 5 minutes)
  - Emit events: `storage.warning` (80-90%), `storage.full` (>95%)
- **Dependencies**: 5.1.1, Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 5.1.3: Quota Enforcement
- **Files**: `impl/meta_storage_impl.go`
- **Changes**:
  - Implement quota checks before write operations:
    - Before `Save*` operations: check quota, reject if >95% full
    - Before `Update*` operations: check quota (if size increases)
  - Implement gradual backpressure:
    - 80-90%: emit warning, continue normal operation
    - 90-95%: throttle operations (reject large records, prioritize critical operations)
    - >95%: reject new write operations, emit critical alert
  - Return `ErrQuotaExceeded` when quota is exceeded
  - Implement per-bucket record limits (enforce max records per bucket)
- **Dependencies**: 5.1.2
- **Estimated Effort**: 2 days

### Section 5.2: Retention Policies

#### Subsection 5.2.1: Retention Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `RetentionConfig` struct:
    - `EventBusRetentionHours int` (default: 24 hours)
    - `DeadLetterRetentionDays int` (default: 90 days)
    - `EdgeStateHistoryRetentionDays int` (default: 30 days)
    - `CleanupIntervalHours int` (default: 6 hours)
  - Add retention configuration to `MetaStorageConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.2.2: Retention Cleanup
- **Files**: `impl/retention_manager.go` (new file)
- **Changes**:
  - Implement `RetentionManager` struct:
    - Track retention policies per bucket
    - Track record creation times
  - Implement `CleanupExpiredRecords(ctx) error`:
    - Query records by creation time
    - Delete records that exceed retention period
    - Handle per-bucket retention policies
  - Implement background cleanup task (runs every 6 hours, configurable)
  - Emit events: `storage.cleanup_started`, `storage.cleanup_completed`
  - Implement cleanup statistics (records deleted, space freed)
- **Dependencies**: 5.2.1, Section 1.1
- **Estimated Effort**: 2 days

---

## Epic 6: Storage Integrity and Health

**Goal**: Implement storage integrity verification, corruption detection, and health monitoring.

### Section 6.1: Integrity Verification

#### Subsection 6.1.1: Database Integrity Checks
- **Files**: `impl/integrity_manager.go` (new file)
- **Changes**:
  - Implement database integrity verification:
    - Check database file integrity (provider-specific)
    - Verify bucket existence and accessibility
    - Verify record format (JSON unmarshaling)
    - Check for orphaned records (references to non-existent objects)
  - Implement `VerifyDatabaseIntegrity(ctx) (*IntegrityReport, error)`:
    - Scan all buckets
    - Verify record formats
    - Check for corruption indicators
    - Return integrity report
  - Implement periodic integrity checks (background task, daily)
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 6.1.2: Corruption Detection
- **Files**: `impl/integrity_manager.go`
- **Changes**:
  - Implement corruption detection:
    - Detect corrupted database files (provider-specific checks)
    - Detect invalid record formats
    - Detect missing buckets
    - Detect schema version mismatches
  - Emit events: `storage.corruption_detected` (with details)
  - Return `ErrCorruptionDetected` when corruption is detected
  - Implement corruption recovery suggestions (VM-assisted resync)
- **Dependencies**: 6.1.1
- **Estimated Effort**: 2 days

### Section 6.2: Health Monitoring

#### Subsection 6.2.1: Health Status Tracking
- **Files**: `types/storage.go`
- **Changes**:
  - Define `HealthStatus` enum:
    - `HealthStatusHealthy`
    - `HealthStatusWarning` (80-90% quota)
    - `HealthStatusFull` (>95% quota)
    - `HealthStatusCorrupted` (integrity failures)
  - Define `StorageHealth` struct:
    - `Status HealthStatus`
    - `Quota *StorageQuota`
    - `IntegrityErrors int` (count of integrity failures)
    - `LastHealthCheck time.Time`
    - `ProviderHealth string` (provider-specific health status)
    - `BucketCounts map[string]int` (record counts per bucket)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 6.2.2: Health Snapshot API
- **Files**: `meta_storage.go`, `impl/meta_storage_impl.go`
- **Changes**:
  - Add `HealthSnapshot() StorageHealth` method to interface
  - Implement health snapshot:
    - Query quota status
    - Query integrity error count
    - Query provider health
    - Query bucket record counts
    - Aggregate into `StorageHealth` struct
  - Follow vm-gateway pattern for health snapshots
- **Dependencies**: 6.2.1, Section 5.1, Section 6.1
- **Estimated Effort**: 1 day

#### Subsection 6.2.3: Provider Health Checks
- **Files**: `types/provider.go`, `impl/bbolt/bbolt_provider.go`
- **Changes**:
  - Add `HealthCheck(ctx) error` method to `MetaStorageProvider` interface
  - Implement provider-specific health checks:
    - BoltDB: check database file accessibility, verify database integrity
    - SQLite: check database file accessibility, run integrity check
    - PostgreSQL: check connection, run health query
  - Return health status string (healthy, degraded, unhealthy)
- **Dependencies**: 6.2.2
- **Estimated Effort**: 1 day

---

## Epic 7: Data Unit and Model Metadata

**Goal**: Refactor data unit and model metadata to be device-agnostic and align with workflow requirements.

### Section 7.1: Data Unit Metadata

#### Subsection 7.1.1: Data Unit Metadata Types
- **Files**: `types/storage.go`
- **Changes**:
  - Define `DataUnitMetadata` struct (replaces `ScreenshotMetadata`):
    - `ID`, `DeviceID`, `DeviceType` (replaces `CameraID`)
    - `DataType` (image, sensor_reading, audio_sample, etc.)
    - `ObjectKey`, `ThumbnailKey`
    - `Label`, `CustomLabel`, `Description`
    - `Metadata map[string]interface{}`
    - `CreatedAt`, `UpdatedAt`
  - Define `DataUnitFilters` struct for querying
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 7.1.2: Data Unit Operations
- **Files**: `meta_storage.go`
- **Changes**:
  - Replace screenshot methods with data unit methods:
    - `SaveScreenshot` → `SaveDataUnit(ctx, dataUnit DataUnitMetadata) error`
    - `UpdateScreenshot` → `UpdateDataUnit(ctx, dataUnitID string, updateFn func(DataUnitMetadata) DataUnitMetadata) (DataUnitMetadata, error)`
    - `GetScreenshot` → `GetDataUnit(ctx, dataUnitID string) (DataUnitMetadata, bool)`
    - `ListScreenshots` → `ListDataUnits(ctx, filters *DataUnitFilters) ([]DataUnitMetadata, error)`
    - `DeleteScreenshot` → `DeleteDataUnit(ctx, dataUnitID string) error`
  - Keep legacy methods for backward compatibility (deprecated)
- **Dependencies**: 7.1.1
- **Estimated Effort**: 1 day

### Section 7.2: Model Deployment Metadata

#### Subsection 7.2.1: Model Deployment Metadata Types
- **Files**: `types/storage.go`
- **Changes**:
  - Update `DeployedModelMetadata` → `ModelDeploymentMetadata`:
    - `ModelID`, `DeploymentID`, `DeviceID` (replaces `CameraID`)
    - `DeviceType` (new field)
    - `ModelPath`, `MetadataPath`, `ManifestPath` (new field for model manifest)
    - `DeployedAt`, `Status`, `EdgeID`
    - `Version`, `ModelType`, `Framework`
    - `VerificationResults` (new field: signature, hash, compatibility checks)
    - `CreatedAt`, `UpdatedAt`
  - Update `ModelFilters` to use `DeviceID` instead of `CameraID`
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 7.2.2: Model Deployment Operations
- **Files**: `meta_storage.go`
- **Changes**:
  - Update model deployment methods:
    - `SaveDeployedModel` → `SaveModelDeployment(ctx, deployment ModelDeploymentMetadata) error`
    - `UpdateDeployedModel` → `UpdateModelDeployment(ctx, modelID string, updateFn func(ModelDeploymentMetadata) ModelDeploymentMetadata) (ModelDeploymentMetadata, error)`
    - `GetDeployedModel` → `GetModelDeployment(ctx, modelID string) (ModelDeploymentMetadata, bool)`
    - `ListDeployedModels` → `ListModelDeployments(ctx, filters *ModelFilters) ([]ModelDeploymentMetadata, error)`
    - `DeleteDeployedModel` → `DeleteModelDeployment(ctx, modelID string) error`
  - Add model version tracking methods:
    - `ListModelVersions(ctx, deviceID string) ([]ModelDeploymentMetadata, error)`
    - `GetLatestModelVersion(ctx, deviceID string) (*ModelDeploymentMetadata, bool)`
  - Keep legacy methods for backward compatibility (deprecated)
- **Dependencies**: 7.2.1
- **Estimated Effort**: 1 day

---

## Epic 8: Security Event and Event Bus Metadata

**Goal**: Enhance security event and event bus metadata storage for production requirements.

### Section 8.1: Security Event Metadata

#### Subsection 8.1.1: Security Event Metadata Enhancement
- **Files**: `types/storage.go`
- **Changes**:
  - Define structured `SecurityEventMetadata` struct (replaces `map[string]interface{}`):
    - `EventID`, `DeviceID`, `DeviceType` (new field)
    - `EventType`, `Timestamp`, `Confidence`
    - `ModelID`, `ModelVersion`
    - `Status` (pending_delivery, delivery_failed, delivered)
    - `DeliveryAttempts int`, `LastDeliveryAttempt time.Time`
    - `VMACKTimestamp time.Time` (when VM acknowledged)
    - `AttachmentRefs []string` (object storage keys)
    - `Metadata map[string]interface{}`
  - Update security event operations to use structured type
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 8.1.2: Security Event Operations Enhancement
- **Files**: `meta_storage.go`
- **Changes**:
  - Update security event methods:
    - `SaveSecurityEvent(ctx, eventID string, eventData map[string]interface{})` → `SaveSecurityEvent(ctx, event SecurityEventMetadata) error`
    - `GetSecurityEvent(ctx, eventID string) (map[string]interface{}, bool)` → `GetSecurityEvent(ctx, eventID string) (*SecurityEventMetadata, bool)`
    - `ListSecurityEvents(ctx, filters map[string]interface{})` → `ListSecurityEvents(ctx, filters *SecurityEventFilters) ([]SecurityEventMetadata, error)`
    - Add `UpdateSecurityEventStatus(ctx, eventID string, status string, vmACKTime *time.Time) error`
    - Add `GetPendingSecurityEvents(ctx, limit int) ([]SecurityEventMetadata, error)`
  - Keep legacy methods for backward compatibility (deprecated)
- **Dependencies**: 8.1.1
- **Estimated Effort**: 2 days

### Section 8.2: Event Bus Metadata

#### Subsection 8.2.1: Event Bus Metadata Enhancement
- **Files**: `types/storage.go`
- **Changes**:
  - Define structured `EventBusEventMetadata` struct:
    - `EventID`, `EventType`, `Timestamp`
    - `Data map[string]interface{}`
    - `ProcessingStatus string` (pending, processing, completed, failed, dead_letter)
    - `RetryCount int`, `LastError string`
    - `NextRetryTime *time.Time`
    - `CreatedAt`, `UpdatedAt`
  - Update event bus operations to use structured type
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 8.2.2: Event Bus Operations Enhancement
- **Files**: `meta_storage.go`
- **Changes**:
  - Update event bus methods:
    - `SaveEvent(ctx, eventID string, eventData map[string]interface{})` → `SaveEvent(ctx, event EventBusEventMetadata) error`
    - `GetEvent(ctx, eventID string) (map[string]interface{}, bool)` → `GetEvent(ctx, eventID string) (*EventBusEventMetadata, bool)`
    - `ListEvents(ctx, filters map[string]interface{})` → `ListEvents(ctx, filters *EventBusFilters) ([]EventBusEventMetadata, error)`
    - `UpdateEventProcessingStatus` → use structured type
    - `GetFailedEvents` → return structured type
    - `GetDeadLetterEvents` → return structured type
  - Keep legacy methods for backward compatibility (deprecated)
- **Dependencies**: 8.2.1
- **Estimated Effort**: 2 days

---

## Epic 9: Pending Data Unit Requests

**Goal**: Refactor pending snapshot requests to be device-agnostic data unit requests.

### Section 9.1: Data Unit Request Metadata

#### Subsection 9.1.1: Data Unit Request Types
- **Files**: `types/storage.go`
- **Changes**:
  - Define `PendingDataUnitRequest` struct (replaces pending snapshot request):
    - `DeviceID`, `DeviceType` (replaces `CameraID`)
    - `DataType` (image, sensor_reading, audio_sample, etc.)
    - `Label`, `CustomLabel`
    - `Count int32`
    - `RequestedAt time.Time`
    - `Status string` (pending, in_progress, completed, failed)
  - Update bucket name: `pending_data_unit_requests` (replaces `pending_snapshot_requests`)
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 9.1.2: Data Unit Request Operations
- **Files**: `meta_storage.go`
- **Changes**:
  - Replace pending snapshot request methods:
    - `SavePendingSnapshotRequest` → `SavePendingDataUnitRequest(ctx, deviceID string, request PendingDataUnitRequest) error`
    - `GetPendingSnapshotRequest` → `GetPendingDataUnitRequest(ctx, deviceID string) (*PendingDataUnitRequest, bool)`
    - `ListPendingSnapshotRequests` → `ListPendingDataUnitRequests(ctx) ([]PendingDataUnitRequest, error)`
    - `DeletePendingSnapshotRequest` → `DeletePendingDataUnitRequest(ctx, deviceID string) error`
  - Keep legacy methods for backward compatibility (deprecated)
- **Dependencies**: 9.1.1
- **Estimated Effort**: 1 day

---

## Epic 10: Observability and Metrics

**Goal**: Add comprehensive observability following vm-gateway pattern.

### Section 10.1: Health Snapshot Enhancement

#### Subsection 10.1.1: Comprehensive Health Snapshot
- **Files**: `types/storage.go`
- **Changes**:
  - Enhance `StorageHealth` struct:
    - Add `BucketCounts map[string]int` (record counts per bucket)
    - Add `TotalRecords int64`
    - Add `DatabaseSizeMB float64`
    - Add `LastCleanupTime time.Time`
    - Add `CleanupStats` (records deleted, space freed)
    - Add `SchemaVersion int`
    - Add `ProviderStatus` (provider-specific status details)
  - Follow vm-gateway `GatewayStatus` pattern
- **Dependencies**: Section 6.2
- **Estimated Effort**: 1 day

#### Subsection 10.1.2: Operational Metrics
- **Files**: `impl/metrics.go` (new file)
- **Changes**:
  - Track operational metrics:
    - Storage operations count (save, get, update, delete) by bucket
    - Operation latency (P50, P95, P99)
    - Error rates by operation type
    - Quota utilization over time
    - Retention cleanup statistics
    - Schema migration statistics
  - Expose metrics via health snapshot or separate metrics endpoint
- **Dependencies**: 10.1.1
- **Estimated Effort**: 2 days

### Section 10.2: Event Emission

#### Subsection 10.2.1: Event Bus Integration
- **Files**: `impl/meta_storage_impl.go`
- **Changes**:
  - Add event bus dependency (similar to vm-gateway)
  - Emit operational events:
    - `storage.warning` (quota 80-90%)
    - `storage.full` (quota >95%)
    - `storage.cleanup_started`, `storage.cleanup_completed`
    - `storage.corruption_detected`
    - `storage.quota_exceeded`
    - `storage.schema_migration_started`, `storage.schema_migration_completed`
  - Use structured event types (similar to vm-gateway event types)
- **Dependencies**: Section 1.1
- **Estimated Effort**: 1 day

---

## Epic 11: Provider Implementation Refactoring

**Goal**: Refactor BoltDB implementation to follow provider-agnostic pattern.

### Section 11.1: BoltDB Provider Refactoring

#### Subsection 11.1.1: Provider Interface Implementation
- **Files**: `impl/bbolt/bbolt_provider.go`
- **Changes**:
  - Implement `MetaStorageProvider` interface:
    - `CreateBucket(ctx, name) error`
    - `DeleteBucket(ctx, name) error`
    - `BucketExists(ctx, name) bool`
    - `Put(ctx, bucket, key, value) error`
    - `Get(ctx, bucket, key) ([]byte, error)`
    - `Delete(ctx, bucket, key) error`
    - `List(ctx, bucket, prefix) ([]KeyValue, error)`
    - `HealthCheck(ctx) error`
  - Remove camera-specific logic
  - Make provider-agnostic
- **Dependencies**: Section 1.1
- **Estimated Effort**: 3 days

#### Subsection 11.1.2: BoltDB Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `BoltDBConfig` struct:
    - `DataDir string` (database file directory)
    - `DatabaseFile string` (default: "meta.db")
    - `FileMode os.FileMode` (default: 0600)
    - `Timeout time.Duration` (default: 1 second)
    - `NoSync bool` (for performance, default: false)
  - Add BoltDB-specific configuration to `MetaStorageConfig`
- **Dependencies**: 11.1.1
- **Estimated Effort**: 1 day

---

## Epic 12: Documentation and Testing

**Goal**: Add comprehensive documentation and testing following vm-gateway pattern.

### Section 12.1: Documentation

#### Subsection 12.1.1: Package Documentation
- **Files**: `doc.go` (new file)
- **Changes**:
  - Add comprehensive package documentation (similar to vm-gateway/doc.go):
    - Architecture overview
    - Provider-agnostic design
    - Configuration examples
    - Usage examples
    - Lifecycle management
    - Health monitoring
    - Schema versioning
    - Bucket organization
  - Document device-agnostic design
  - Document production features (quota, retention, integrity, schema versioning)
- **Dependencies**: All epics
- **Estimated Effort**: 1 day

#### Subsection 12.1.2: API Documentation
- **Files**: All interface files
- **Changes**:
  - Add comprehensive method documentation
  - Document error conditions
  - Document return values
  - Add usage examples
  - Document CAS operations
  - Document schema migrations
- **Dependencies**: 12.1.1
- **Estimated Effort**: 1 day

### Section 12.2: Testing

#### Subsection 12.2.1: Unit Tests
- **Files**: `*_test.go` files
- **Changes**:
  - Test quota enforcement
  - Test retention policies
  - Test integrity verification
  - Test health monitoring
  - Test schema migrations
  - Test CAS operations
  - Test provider abstraction
  - Test ML lifecycle state management
- **Dependencies**: All epics
- **Estimated Effort**: 3 days

#### Subsection 12.2.2: Integration Tests
- **Files**: `*_integration_test.go` files
- **Changes**:
  - Test full storage lifecycle (save, get, update, delete)
  - Test quota and retention with real provider
  - Test integrity verification with corruption injection
  - Test health monitoring
  - Test schema migrations
  - Test ML lifecycle state transitions
- **Dependencies**: 12.2.1
- **Estimated Effort**: 2 days

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~1.5 weeks
- **Epics**: 1 (Provider-Agnostic Architecture), 2 (Device-Agnostic Architecture)
- **Rationale**: Establishes the architectural foundation and type system

### Phase 2: Core Features (Epics 3, 4)
- **Duration**: ~2 weeks
- **Epics**: 3 (ML Lifecycle State Management), 4 (Bucket Organization and Schema)
- **Rationale**: Implements core ML lifecycle state management and schema versioning

### Phase 3: Production Features (Epics 5, 6)
- **Duration**: ~2 weeks
- **Epics**: 5 (Quota and Retention), 6 (Storage Integrity and Health)
- **Rationale**: Implements production reliability features

### Phase 4: Data Refactoring (Epics 7, 8, 9)
- **Duration**: ~1.5 weeks
- **Epics**: 7 (Data Unit and Model Metadata), 8 (Security Event and Event Bus Metadata), 9 (Pending Data Unit Requests)
- **Rationale**: Refactors all data types to be device-agnostic

### Phase 5: Provider and Polish (Epics 10, 11, 12)
- **Duration**: ~1.5 weeks
- **Epics**: 10 (Observability), 11 (Provider Refactoring), 12 (Documentation and Testing)
- **Rationale**: Completes provider implementation, observability, and documentation

**Total Estimated Duration**: ~8.5 weeks

---

## Migration Notes

### Breaking Changes
- All `CameraID` references become `DeviceID`
- Camera-specific methods replaced with device-agnostic methods
- Bucket names change (cameras → devices, screenshots → data_units, etc.)
- Security event and event bus metadata use structured types instead of `map[string]interface{}`
- Configuration structure changes (quota, retention, schema versioning configs)

### Data Migration
- Existing camera metadata must be migrated to device metadata
- Existing screenshot metadata must be migrated to data unit metadata
- Existing bucket data must be migrated to new bucket names
- Schema migrations must be run on startup
- ML lifecycle state must be initialized for existing devices

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Run schema migrations in staging
- Verify data migration correctness
- Gradual rollout to production with monitoring
- Monitor quota and retention behavior
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Provider-agnostic architecture implemented (following vm-gateway pattern)
2. ✅ Device-agnostic types and methods implemented
3. ✅ ML lifecycle state management implemented and tested
4. ✅ Schema versioning implemented and tested
5. ✅ Quota enforcement implemented and tested
6. ✅ Retention policies implemented and tested
7. ✅ Storage integrity verification implemented and tested
8. ✅ Health monitoring implemented and tested
9. ✅ All data types refactored to device-agnostic
10. ✅ Comprehensive documentation added
11. ✅ Full test coverage (unit, integration)
12. ✅ Health snapshot API implemented
13. ✅ Event emission implemented
14. ✅ CAS operations implemented for ML lifecycle state

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **Architecture should follow vm-gateway pattern** (but simpler, as meta-storage is a simpler service)
- **Provider-agnostic design is mandatory** (support BoltDB now, SQLite/PostgreSQL in future)
- **Device-agnostic implementation is mandatory** (not just camera support)
- **ML lifecycle state management is critical** (required by state-mng refactoring)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

