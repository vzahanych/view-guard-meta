# Object Storage Refactoring Plan

**Date**: 2025-12-28  
**Target Documents**: 
- `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`
- `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- `edge/orchestrator/internal/vm-gateway/doc.go` (architectural pattern reference)

**Scope**: Complete refactoring of `object-storage` package to align with production workflow requirements and follow vm-gateway architectural pattern  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the Object Storage service implementation into full compliance with the production workflow specification and aligns it with the vm-gateway architectural pattern. The current implementation is camera-centric, lacks production features (quota enforcement, retention policies, encryption, health monitoring), and doesn't follow the provider-agnostic architecture pattern.

**Key Transformation Areas**:
1. **Device-agnostic architecture**: Replace camera-centric terminology with device-agnostic types
2. **Provider-agnostic design**: Follow vm-gateway pattern with interface, types, and implementation separation
3. **Production features**: Add quota enforcement, retention policies, encryption support, health monitoring
4. **Data unit abstraction**: Support all device types (cameras, sensors, audio devices) with unified data unit storage
5. **Storage integrity**: Add hash verification, corruption detection, and health checks
6. **Observability**: Add health snapshot API and operational metrics

---

## Epic 1: Provider-Agnostic Architecture (Following vm-gateway Pattern)

**Goal**: Restructure the codebase to follow the vm-gateway architectural pattern with clear separation of concerns.

### Section 1.1: Interface and Types Separation

#### Subsection 1.1.1: Main Interface File
- **Files**: `object_storage.go` (rename from `object-storage-iface.go`)
- **Changes**:
  - Define `ObjectStorageService` interface (main service interface)
  - Define sentinel errors (similar to vm-gateway):
    - `ErrNotInitialized`
    - `ErrAlreadyStarted`
    - `ErrQuotaExceeded`
    - `ErrObjectNotFound`
    - `ErrCorruptionDetected`
  - Define factory function `NewObjectStorageService(ctx, config, logger)`
  - Define provider function `ObjectStorageProvider(lc, cfg, logger)` with fx lifecycle
  - Add comprehensive package documentation (similar to vm-gateway/doc.go)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Types Package Structure
- **Files**: `types/` directory
- **Changes**:
  - Move all configuration types to `types/config.go`
  - Create `types/storage.go` for storage-related types:
    - `ObjectMetadata` struct (size, content type, hash, created at, device ID, device type)
    - `StorageQuota` struct (used, limit, warning threshold, full threshold)
    - `RetentionPolicy` struct (retention days, cleanup schedule)
    - `HealthStatus` enum (healthy, warning, full, corrupted)
  - Create `types/provider.go` for provider interface:
    - `ObjectStorageProvider` interface (provider-agnostic operations)
    - Provider-specific configuration types
  - Create `types/errors.go` for error types
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Implementation Package Structure
- **Files**: `impl/` directory (new, rename from `minio-imp/`)
- **Changes**:
  - Create `impl/object_storage_impl.go` (main implementation)
  - Create provider-specific implementations:
    - `impl/minio/minio_provider.go` (MinIO implementation)
    - `impl/s3/s3_provider.go` (future: AWS S3 implementation)
    - `impl/filesystem/filesystem_provider.go` (future: local filesystem implementation)
  - Each provider implements `types.ObjectStorageProvider` interface
  - Main implementation delegates to provider
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

### Section 1.2: Lifecycle Management

#### Subsection 1.2.1: Service Lifecycle
- **Files**: `impl/object_storage_impl.go`
- **Changes**:
  - Implement `Start(ctx)` method:
    - Initialize provider
    - Verify connectivity
    - Create required buckets/namespaces
    - Initialize quota monitoring
    - Start background tasks (retention cleanup, health checks)
  - Implement `Stop(ctx)` method:
    - Stop background tasks gracefully
    - Close provider connections
    - Flush pending operations
  - Follow vm-gateway pattern: service owns lifecycle of sub-components
- **Dependencies**: 1.1.3
- **Estimated Effort**: 1 day

#### Subsection 1.2.2: Provider Lifecycle
- **Files**: `impl/minio/minio_provider.go` (and other providers)
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
- **Files**: All files in `object_storage.go`, `types/`, `impl/`
- **Changes**:
  - Replace all `CameraID` references with `DeviceID`
  - Update function signatures: `GenerateClipKey(cameraID)` → `GenerateDataUnitKey(deviceID, deviceType, dataType)`
  - Update variable names and map keys
  - Update key generation methods to be device-agnostic
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 2.1.2: Device-Agnostic Data Unit Types
- **Files**: `types/storage.go`
- **Changes**:
  - Create `DeviceID` type alias (string)
  - Create `DeviceType` enum (camera, sensor, audio_device, etc.)
  - Create `DataType` enum (video_frame, video_clip, image, sensor_reading, audio_sample, model_artifact, etc.)
  - Create `DataUnit` struct:
    - `DeviceID`, `DeviceType`, `DataType`
    - `Key` (object key), `Size`, `ContentType`
    - `Hash` (SHA-256), `CreatedAt`, `Metadata`
  - Update `ObjectMetadata` to include `DeviceID` and `DeviceType`
- **Dependencies**: 2.1.1
- **Estimated Effort**: 1 day

#### Subsection 2.1.3: Unified Data Unit Storage Interface
- **Files**: `object_storage.go`
- **Changes**:
  - Replace camera-specific methods with device-agnostic methods:
    - `StoreClip` → `StoreDataUnit(ctx, deviceID, deviceType, dataType, key, r, size, contentType)`
    - `LoadClip` → `LoadDataUnit(ctx, key)`
    - `DeleteClip` → `DeleteDataUnit(ctx, key)`
    - `StoreSnapshot` → (use `StoreDataUnit` with `DataTypeImage`)
    - `StoreFrame` → (use `StoreDataUnit` with `DataTypeVideoFrame`)
    - `StoreModel` → `StoreModelArtifacts(ctx, modelID, deviceID, artifacts)`
  - Keep legacy methods for backward compatibility during migration (deprecated)
  - Add `GenerateDataUnitKey(deviceID, deviceType, dataType, isThumbnail)` method
- **Dependencies**: 2.1.2
- **Estimated Effort**: 2 days

### Section 2.2: Key Generation Refactoring

#### Subsection 2.2.1: Device-Agnostic Key Generation
- **Files**: `impl/key_generation.go` (new file)
- **Changes**:
  - Implement `GenerateDataUnitKey(deviceID, deviceType, dataType, isThumbnail) string`:
    - Organize by device type and date: `{dataType}/{deviceType}/{deviceID}/{YYYY-MM-DD}/{timestamp_uuid.ext}`
    - Examples:
      - Video clip: `video_clips/camera/cam-001/2025-12-28/cam-001_120000_uuid.mp4`
      - Image: `images/camera/cam-001/2025-12-28/cam-001_120000_uuid.jpg`
      - Sensor reading: `sensor_readings/temperature/temp-001/2025-12-28/temp-001_120000_uuid.json`
      - Audio sample: `audio_samples/microphone/mic-001/2025-12-28/mic-001_120000_uuid.wav`
  - Implement `GenerateModelKey(modelID, deviceID) string`:
    - `models/{deviceID}/{modelID}/model.{ext}`
    - `models/{deviceID}/{modelID}/metadata.json`
  - Implement `GenerateSecurityEventAttachmentKey(eventID, deviceID, dataType) string`:
    - `security-events/{deviceID}/{YYYY-MM-DD}/{eventID}_{dataType}.{ext}`
- **Dependencies**: 2.1.3
- **Estimated Effort**: 1 day

---

## Epic 3: Production Features - Quota and Retention

**Goal**: Implement quota enforcement and retention policies as specified in the workflow document.

### Section 3.1: Quota Management

#### Subsection 3.1.1: Quota Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `QuotaConfig` struct:
    - `MaxSizeMB int` (default: 100,000 MB)
    - `WarningThresholdPercent int` (default: 80)
    - `FullThresholdPercent int` (default: 95)
  - Add quota configuration to `ObjectStorageConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Quota Tracking
- **Files**: `impl/quota_manager.go` (new file)
- **Changes**:
  - Implement `QuotaManager` struct:
    - Track current usage (sum of all object sizes)
    - Track quota limits
    - Track thresholds
  - Implement `GetQuotaStatus(ctx) (*StorageQuota, error)`:
    - Query provider for total size
    - Calculate usage percentage
    - Return quota status
  - Implement periodic quota checks (background task, every 5 minutes)
  - Emit events: `storage.warning` (80-90%), `storage.full` (>95%)
- **Dependencies**: 3.1.1, Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 3.1.3: Quota Enforcement
- **Files**: `impl/object_storage_impl.go`
- **Changes**:
  - Implement quota checks before storage operations:
    - Before `StoreDataUnit`: check quota, reject if >95% full
    - Before `StoreModelArtifacts`: check quota, reject if >95% full
  - Implement gradual backpressure:
    - 80-90%: emit warning, continue normal operation
    - 90-95%: throttle operations (reduce attachment sizes, reject large objects)
    - >95%: reject new storage operations, emit critical alert
  - Return `ErrQuotaExceeded` when quota is exceeded
- **Dependencies**: 3.1.2
- **Estimated Effort**: 2 days

### Section 3.2: Retention Policies

#### Subsection 3.2.1: Retention Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `RetentionConfig` struct:
    - `DatasetRetentionDays int` (default: 30 days after upload)
    - `EventRetentionDays int` (default: 7 days after VM ack)
    - `ModelRetentionVersions int` (default: 2 versions per device)
    - `ModelRetentionGracePeriodDays int` (default: 7 days after purge eligibility)
    - `UnassignedDeviceDataRetentionDays int` (default: 30 days after unassignment)
  - Add retention configuration to `ObjectStorageConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.2.2: Retention Cleanup
- **Files**: `impl/retention_manager.go` (new file)
- **Changes**:
  - Implement `RetentionManager` struct:
    - Track retention policies per data type
    - Track object creation times and metadata
  - Implement `CleanupExpiredObjects(ctx) error`:
    - Query objects by data type and creation time
    - Delete objects that exceed retention period
    - Respect grace periods
    - Handle model version retention (keep last N versions per device)
  - Implement background cleanup task (runs every 6 hours, configurable)
  - Emit events: `storage.cleanup_started`, `storage.cleanup_completed`
- **Dependencies**: 3.2.1, Section 1.1
- **Estimated Effort**: 3 days

#### Subsection 3.2.3: Retention Metadata Tracking
- **Files**: `impl/object_storage_impl.go`
- **Changes**:
  - Store retention metadata with objects:
    - Creation timestamp
    - Data type (for retention policy lookup)
    - Device ID (for model version tracking)
    - VM acknowledgment timestamp (for event retention)
    - Upload completion timestamp (for dataset retention)
  - Integrate with meta-storage for retention metadata (or store in object metadata)
  - Update retention metadata on VM acknowledgment
- **Dependencies**: 3.2.2
- **Estimated Effort**: 2 days

---

## Epic 4: Storage Integrity and Health

**Goal**: Implement storage integrity verification, corruption detection, and health monitoring.

### Section 4.1: Integrity Verification

#### Subsection 4.1.1: Hash Calculation and Storage
- **Files**: `impl/integrity_manager.go` (new file)
- **Changes**:
  - Implement hash calculation on store:
    - Calculate SHA-256 hash of object content
    - Store hash in object metadata (or separate integrity metadata store)
  - Implement hash verification on load:
    - Calculate hash of loaded content
    - Compare with stored hash
    - Return error if mismatch (corruption detected)
  - Store integrity evidence:
    - Content hash (SHA-256)
    - Creation time, source VM identity (if applicable)
    - Audit record reference
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 4.1.2: Corruption Detection
- **Files**: `impl/integrity_manager.go`
- **Changes**:
  - Implement periodic integrity checks (background task, daily):
    - Sample objects and verify hashes
    - Check for missing objects referenced by meta-storage
    - Check for orphaned objects (not referenced by meta-storage)
  - Implement on-demand integrity check:
    - `VerifyObjectIntegrity(ctx, key) error`
    - `VerifyStorageIntegrity(ctx) (*IntegrityReport, error)`
  - Emit events: `storage.corruption_detected` (with details)
  - Return `ErrCorruptionDetected` when corruption is detected
- **Dependencies**: 4.1.1
- **Estimated Effort**: 2 days

### Section 4.2: Health Monitoring

#### Subsection 4.2.1: Health Status Tracking
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
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.2.2: Health Snapshot API
- **Files**: `object_storage.go`, `impl/object_storage_impl.go`
- **Changes**:
  - Add `HealthSnapshot() StorageHealth` method to interface
  - Implement health snapshot:
    - Query quota status
    - Query integrity error count
    - Query provider health
    - Aggregate into `StorageHealth` struct
  - Follow vm-gateway pattern for health snapshots
- **Dependencies**: 4.2.1, Section 3.1, Section 4.1
- **Estimated Effort**: 1 day

#### Subsection 4.2.3: Provider Health Checks
- **Files**: `types/provider.go`, `impl/minio/minio_provider.go`
- **Changes**:
  - Add `HealthCheck(ctx) error` method to `ObjectStorageProvider` interface
  - Implement provider-specific health checks:
    - MinIO: check bucket accessibility, connection status
    - S3: check bucket accessibility, credentials validity
  - Return health status string (healthy, degraded, unhealthy)
- **Dependencies**: 4.2.2
- **Estimated Effort**: 1 day

---

## Epic 5: Encryption at Rest Support

**Goal**: Add encryption support for sensitive data (datasets, security event attachments).

### Section 5.1: Encryption Configuration

#### Subsection 5.1.1: Encryption Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `EncryptionConfig` struct:
    - `Enabled bool` (default: false, implementation-dependent)
    - `Provider string` (kms, hardware-backed, software)
    - `KeySource string` (KMS endpoint, hardware module path, key file path)
    - `Algorithm string` (AES-256-GCM default)
  - Add encryption configuration to `ObjectStorageConfig`
  - Note: Encryption is implementation-dependent (provider must support it)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Encryption Interface
- **Files**: `types/encryption.go` (new file)
- **Changes**:
  - Define `EncryptionProvider` interface:
    - `Encrypt(ctx, data []byte) ([]byte, error)`
    - `Decrypt(ctx, encryptedData []byte) ([]byte, error)`
    - `GenerateKey(ctx) ([]byte, error)` (for per-object keys)
  - Define encryption metadata:
    - Algorithm, key ID, IV (initialization vector)
  - Store encryption metadata with encrypted objects
- **Dependencies**: 5.1.1
- **Estimated Effort**: 1 day

#### Subsection 5.1.3: Encryption Integration
- **Files**: `impl/object_storage_impl.go`
- **Changes**:
  - Integrate encryption into storage operations:
    - Before `StoreDataUnit`: encrypt if encryption enabled and data type requires it (datasets, security events)
    - After `LoadDataUnit`: decrypt if object is encrypted
  - Handle encryption errors gracefully
  - Store encryption metadata with object metadata
  - Note: This is optional and depends on provider support
- **Dependencies**: 5.1.2
- **Estimated Effort**: 2 days

---

## Epic 6: Observability and Metrics

**Goal**: Add comprehensive observability following vm-gateway pattern.

### Section 6.1: Health Snapshot Enhancement

#### Subsection 6.1.1: Comprehensive Health Snapshot
- **Files**: `types/storage.go`
- **Changes**:
  - Enhance `StorageHealth` struct:
    - Add `ObjectCounts` map (data type → count)
    - Add `TotalSizeMB int64`
    - Add `LastCleanupTime time.Time`
    - Add `CleanupStats` (objects deleted, space freed)
    - Add `ProviderStatus` (provider-specific status details)
  - Follow vm-gateway `GatewayStatus` pattern
- **Dependencies**: Section 4.2
- **Estimated Effort**: 1 day

#### Subsection 6.1.2: Operational Metrics
- **Files**: `impl/metrics.go` (new file)
- **Changes**:
  - Track operational metrics:
    - Storage operations count (store, load, delete) by data type
    - Operation latency (P50, P95, P99)
    - Error rates by operation type
    - Quota utilization over time
    - Retention cleanup statistics
  - Expose metrics via health snapshot or separate metrics endpoint
- **Dependencies**: 6.1.1
- **Estimated Effort**: 2 days

### Section 6.2: Event Emission

#### Subsection 6.2.1: Event Bus Integration
- **Files**: `impl/object_storage_impl.go`
- **Changes**:
  - Add event bus dependency (similar to vm-gateway)
  - Emit operational events:
    - `storage.warning` (quota 80-90%)
    - `storage.full` (quota >95%)
    - `storage.cleanup_started`, `storage.cleanup_completed`
    - `storage.corruption_detected`
    - `storage.quota_exceeded`
  - Use structured event types (similar to vm-gateway event types)
- **Dependencies**: Section 1.1
- **Estimated Effort**: 1 day

---

## Epic 7: Model Artifact Storage

**Goal**: Enhance model storage to support model verification and versioning requirements.

### Section 7.1: Model Artifact Structure

#### Subsection 7.1.1: Model Artifact Storage
- **Files**: `object_storage.go`, `impl/object_storage_impl.go`
- **Changes**:
  - Enhance `StoreModelArtifacts` method:
    - Accept `ModelManifest` (from state-mng types)
    - Store model binary, metadata, and manifest separately
    - Store integrity hashes for all artifacts
    - Store model version information
  - Implement `LoadModelArtifacts(ctx, modelID) (*ModelArtifacts, error)`:
    - Load model binary, metadata, and manifest
    - Verify integrity hashes
    - Return structured model artifacts
- **Dependencies**: Epic 2
- **Estimated Effort**: 2 days

#### Subsection 7.1.2: Model Version Management
- **Files**: `impl/model_version_manager.go` (new file)
- **Changes**:
  - Implement model version tracking:
    - Track model versions per device
    - Enforce retention policy (keep last N versions)
    - Support model rollback (load previous version)
  - Implement `ListModelVersions(ctx, deviceID) ([]ModelVersion, error)`
  - Implement `DeleteOldModelVersions(ctx, deviceID, keepN int) error`
- **Dependencies**: 7.1.1, Section 3.2
- **Estimated Effort**: 2 days

---

## Epic 8: Security Event Attachment Storage

**Goal**: Optimize security event attachment storage for production requirements.

### Section 8.1: Event Attachment Storage

#### Subsection 8.1.1: Attachment Storage Interface
- **Files**: `object_storage.go`
- **Changes**:
  - Add `StoreSecurityEventAttachment(ctx, eventID, deviceID, dataType, data []byte) (string, error)`:
    - Store attachment with optimized key structure
    - Return object key for metadata storage
    - Support multiple attachment types (images, sensor readings, audio samples)
  - Add `LoadSecurityEventAttachment(ctx, key) (io.ReadCloser, error)`
  - Add `DeleteSecurityEventAttachment(ctx, key) error`
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 8.1.2: Attachment Optimization
- **Files**: `impl/attachment_manager.go` (new file)
- **Changes**:
  - Implement attachment size optimization:
    - When storage >90% full: compress attachments
    - When storage >95% full: reduce image quality, smaller samples
  - Implement attachment retention:
    - Delete attachments after retention period (7 days after VM ack, configurable)
    - Cleanup orphaned attachments (not referenced by events)
- **Dependencies**: 8.1.1, Section 3.2
- **Estimated Effort**: 2 days

---

## Epic 9: Provider Implementation Refactoring

**Goal**: Refactor MinIO implementation to follow provider-agnostic pattern.

### Section 9.1: MinIO Provider Refactoring

#### Subsection 9.1.1: Provider Interface Implementation
- **Files**: `impl/minio/minio_provider.go`
- **Changes**:
  - Implement `ObjectStorageProvider` interface:
    - `StoreObject(ctx, key, r, size, contentType, metadata) error`
    - `LoadObject(ctx, key) (io.ReadCloser, error)`
    - `DeleteObject(ctx, key) error`
    - `ListObjects(ctx, prefix) ([]ObjectInfo, error)`
    - `GetObjectMetadata(ctx, key) (*ObjectMetadata, error)`
    - `HealthCheck(ctx) error`
  - Remove camera-specific logic
  - Make provider-agnostic
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 9.1.2: MinIO Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `MinIOConfig` struct:
    - `Endpoint string`
    - `AccessKey string`
    - `SecretKey string`
    - `Region string`
    - `Bucket string` (default: "edge-storage")
    - `UseSSL bool`
    - `InsecureSkipVerify bool` (for dev)
  - Add MinIO-specific configuration to `ObjectStorageConfig`
- **Dependencies**: 9.1.1
- **Estimated Effort**: 1 day

---

## Epic 10: Documentation and Testing

**Goal**: Add comprehensive documentation and testing following vm-gateway pattern.

### Section 10.1: Documentation

#### Subsection 10.1.1: Package Documentation
- **Files**: `doc.go` (new file)
- **Changes**:
  - Add comprehensive package documentation (similar to vm-gateway/doc.go):
    - Architecture overview
    - Provider-agnostic design
    - Configuration examples
    - Usage examples
    - Lifecycle management
    - Health monitoring
  - Document device-agnostic design
  - Document production features (quota, retention, encryption, integrity)
- **Dependencies**: All epics
- **Estimated Effort**: 1 day

#### Subsection 10.1.2: API Documentation
- **Files**: All interface files
- **Changes**:
  - Add comprehensive method documentation
  - Document error conditions
  - Document return values
  - Add usage examples
- **Dependencies**: 10.1.1
- **Estimated Effort**: 1 day

### Section 10.2: Testing

#### Subsection 10.2.1: Unit Tests
- **Files**: `*_test.go` files
- **Changes**:
  - Test quota enforcement
  - Test retention policies
  - Test integrity verification
  - Test health monitoring
  - Test key generation
  - Test provider abstraction
- **Dependencies**: All epics
- **Estimated Effort**: 3 days

#### Subsection 10.2.2: Integration Tests
- **Files**: `*_integration_test.go` files
- **Changes**:
  - Test full storage lifecycle (store, load, delete)
  - Test quota and retention with real provider
  - Test integrity verification with corruption injection
  - Test health monitoring
- **Dependencies**: 10.2.1
- **Estimated Effort**: 2 days

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~1 week
- **Epics**: 1 (Provider-Agnostic Architecture), 2 (Device-Agnostic Architecture)
- **Rationale**: Establishes the architectural foundation and type system

### Phase 2: Core Features (Epics 3, 4)
- **Duration**: ~2 weeks
- **Epics**: 3 (Quota and Retention), 4 (Storage Integrity and Health)
- **Rationale**: Implements core production features

### Phase 3: Advanced Features (Epics 5, 6, 7, 8)
- **Duration**: ~2 weeks
- **Epics**: 5 (Encryption), 6 (Observability), 7 (Model Artifacts), 8 (Security Event Attachments)
- **Rationale**: Adds advanced production features

### Phase 4: Provider and Polish (Epics 9, 10)
- **Duration**: ~1 week
- **Epics**: 9 (Provider Refactoring), 10 (Documentation and Testing)
- **Rationale**: Completes provider implementation and documentation

**Total Estimated Duration**: ~6 weeks

---

## Migration Notes

### Breaking Changes
- All `CameraID` references become `DeviceID`
- Camera-specific methods replaced with device-agnostic methods
- Key generation structure changes (device-agnostic paths)
- Configuration structure changes (quota, retention, encryption configs)

### Data Migration
- Existing object keys may need migration (if key structure changes significantly)
- Retention metadata needs to be added to existing objects
- Integrity hashes need to be calculated for existing objects (background task)

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Gradual rollout to production with monitoring
- Monitor quota and retention behavior
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Provider-agnostic architecture implemented (following vm-gateway pattern)
2. ✅ Device-agnostic types and methods implemented
3. ✅ Quota enforcement implemented and tested
4. ✅ Retention policies implemented and tested
5. ✅ Storage integrity verification implemented and tested
6. ✅ Health monitoring implemented and tested
7. ✅ Encryption support added (if provider supports it)
8. ✅ Model artifact storage enhanced
9. ✅ Security event attachment storage optimized
10. ✅ Comprehensive documentation added
11. ✅ Full test coverage (unit, integration)
12. ✅ Health snapshot API implemented
13. ✅ Event emission implemented

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **Architecture should follow vm-gateway pattern** (but simpler, as object storage is a simpler service)
- **Provider-agnostic design is mandatory** (support MinIO now, S3/filesystem in future)
- **Device-agnostic implementation is mandatory** (not just camera support)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

