package impl

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	"go.uber.org/zap"
)

// ObjectStorageImpl is the main implementation of ObjectStorageService.
// It delegates low-level operations to the provider and handles business logic.
//
// This implementation follows the provider-agnostic architecture pattern:
//   - Provider handles low-level storage operations
//   - Implementation handles business logic (marshaling, filtering, quota, retention)
//   - Managers handle cross-cutting concerns (quota, retention, integrity)
//
// This follows the vm-gateway pattern: service owns lifecycle of sub-components.
type ObjectStorageImpl struct {
	provider types.ObjectStorageProvider
	logger   *zap.Logger
	config   *types.ObjectStorageConfig

	// Lifecycle state
	mu      sync.RWMutex
	started bool
	stopCh  chan struct{}

	// Managers (initialized in Start if config is provided)
	quotaManager        *QuotaManager
	retentionManager    *RetentionManager
	integrityManager    *IntegrityManager
	modelVersionManager *ModelVersionManager
	attachmentManager   *AttachmentManager
	encryptionProvider  types.EncryptionProvider // Initialized in Start if encryption is enabled
	metricsManager      *MetricsManager
}

// NewObjectStorageImpl creates a new ObjectStorageImpl instance.
// This is an internal constructor - use the factory function in the main package.
func NewObjectStorageImpl(provider types.ObjectStorageProvider, config *types.ObjectStorageConfig, logger *zap.Logger) *ObjectStorageImpl {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &ObjectStorageImpl{
		provider: provider,
		logger:   logger,
		config:   config,
		started:  false,
		stopCh:   make(chan struct{}),
	}
}

// Start initializes the storage service and starts background tasks.
// This method:
//   - Verifies provider connectivity
//   - Creates required buckets/namespaces (if needed)
//   - Initializes quota monitoring (if configured)
//   - Starts background tasks (retention cleanup, health checks)
//
// Returns an error if:
//   - The service is already started (ErrAlreadyStarted)
//   - Provider initialization fails
//   - Required buckets cannot be created
//
// This follows the vm-gateway pattern: service owns lifecycle of sub-components.
func (s *ObjectStorageImpl) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return types.ErrAlreadyStarted
	}

	if s.provider == nil {
		return types.ErrNotInitialized
	}

	s.logger.Info("Starting object storage service...")

	// Step 1: Verify provider connectivity
	if err := s.provider.HealthCheck(ctx); err != nil {
		return fmt.Errorf("object-storage: provider health check failed: %w", err)
	}

	// Step 2: Create required buckets/namespaces
	// For object storage providers (MinIO, S3), buckets are typically created during provider initialization.
	// For filesystem providers, directories may need to be created.
	// This is provider-specific and handled by the provider implementation.
	// No explicit bucket creation needed here as providers handle this.

	// Step 3: Initialize quota monitoring (if configured)
	if s.config != nil && s.config.QuotaConfig != nil {
		s.quotaManager = NewQuotaManager(s.provider, s.config.QuotaConfig, s.logger)
		// Set event emitter for quota manager
		s.quotaManager.SetEventEmitter(s)
		// Start periodic quota checks (every 5 minutes)
		s.quotaManager.StartPeriodicChecks(ctx, 5*time.Minute)
		s.logger.Info("Quota monitoring started")
	} else {
		s.logger.Info("Quota monitoring: not configured")
	}

	// Step 4: Start background tasks
	// Start retention cleanup if retention manager is configured
	if s.config != nil && s.config.RetentionConfig != nil {
		s.retentionManager = NewRetentionManager(s.provider, s.config.RetentionConfig, s.logger)
		// Set event emitter for retention manager
		s.retentionManager.SetEventEmitter(s)
		s.retentionManager.StartPeriodicCleanup(ctx)
		s.logger.Info("Retention cleanup started",
			zap.Int("cleanup_interval_hours", s.config.RetentionConfig.CleanupIntervalHours))
	} else {
		s.logger.Info("Retention cleanup: not configured")
	}

	// Step 5: Initialize integrity manager (always enabled for hash verification)
	s.integrityManager = NewIntegrityManager(s.provider, s.logger)
	// Set event emitter for integrity manager
	s.integrityManager.SetEventEmitter(s)
	s.integrityManager.StartPeriodicIntegrityChecks(ctx, 24*time.Hour)
	s.logger.Info("Integrity checks started")

	// Step 6: Initialize model version manager (always enabled)
	s.modelVersionManager = NewModelVersionManager(s.provider, s.logger)
	s.logger.Info("Model version manager initialized")

	// Step 6.5: Initialize attachment manager (always enabled for optimization)
	s.attachmentManager = NewAttachmentManager(s.provider, s.quotaManager, s.logger)
	s.logger.Info("Attachment manager initialized")

	// Step 7: Initialize metrics manager (always enabled for observability)
	s.metricsManager = NewMetricsManager(s.logger)
	// Start periodic quota sampling
	if s.quotaManager != nil {
		s.metricsManager.StartPeriodicQuotaSampling(ctx, s.quotaManager, 5*time.Minute)
		s.logger.Info("Metrics collection started")
	}

	// Step 8: Initialize encryption provider (if encryption is enabled)
	if s.config != nil && s.config.EncryptionConfig != nil && s.config.EncryptionConfig.Enabled {
		// Note: Encryption provider implementation is optional and depends on provider support
		// For now, we'll create a placeholder - actual implementation will be in Epic 9 or later
		// The encryption provider should be created based on EncryptionConfig.Provider
		s.logger.Info("Encryption is enabled",
			zap.String("provider", s.config.EncryptionConfig.Provider),
			zap.String("algorithm", s.config.EncryptionConfig.Algorithm))
		// TODO: Initialize encryption provider based on config
		// s.encryptionProvider = NewEncryptionProvider(s.config.EncryptionConfig, s.logger)
	} else {
		s.logger.Info("Encryption: not enabled")
	}

	s.started = true
	s.logger.Info("Object storage service started successfully")

	return nil
}

// Stop stops the storage service and cleans up resources.
// This method:
//   - Stops background tasks gracefully
//   - Closes provider connections
//   - Flushes pending operations
//
// Returns an error if cleanup fails.
//
// This follows the vm-gateway pattern: service owns lifecycle of sub-components.
func (s *ObjectStorageImpl) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil // Already stopped
	}

	s.logger.Info("Stopping object storage service...")

	var errs []error

	// Step 1: Stop background tasks gracefully
	close(s.stopCh)
	s.stopCh = make(chan struct{}) // Reset for potential restart
	s.logger.Info("Background tasks stopped")

	// Stop managers
	if s.quotaManager != nil {
		s.quotaManager.StopPeriodicChecks()
	}
	if s.retentionManager != nil {
		s.retentionManager.StopPeriodicCleanup()
	}
	if s.integrityManager != nil {
		s.integrityManager.StopPeriodicIntegrityChecks()
	}

	// Step 2: Flush pending operations
	// For object storage providers, operations are typically synchronous.
	// For providers with buffering, this would flush the buffer.
	// This is provider-specific and handled by the provider implementation.
	s.logger.Info("Pending operations flushed")

	// Step 3: Close provider connections
	if s.provider != nil {
		if err := s.provider.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close provider: %w", err))
			s.logger.Warn("Error closing provider", zap.Error(err))
		}
	}

	s.started = false

	if len(errs) > 0 {
		s.logger.Error("Some operations failed during stop", zap.Errors("errors", errs))
		return errors.Join(errs...)
	}

	s.logger.Info("Object storage service stopped successfully")

	return nil
}

// Name returns the service name.
func (s *ObjectStorageImpl) Name() string {
	return "object-storage"
}

// HealthSnapshot returns the current health status of the storage service.
// This follows the vm-gateway pattern for health snapshots.
// TODO: Implement full health monitoring in Epic 4.
func (s *ObjectStorageImpl) HealthSnapshot() types.StorageHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	health := types.StorageHealth{
		Status:          types.HealthStatusHealthy,
		LastHealthCheck: time.Now(),
		ProviderHealth:  "healthy",
		ObjectCounts:    make(map[string]int64),
		ProviderStatus:  make(map[string]interface{}),
	}

	// Check if service is started
	if !s.started {
		health.Status = types.HealthStatusWarning
		health.ProviderHealth = "not_started"
		return health
	}

	// Check provider health
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.provider.HealthCheck(ctx); err != nil {
		health.Status = types.HealthStatusWarning
		health.ProviderHealth = "unhealthy"
		health.ProviderStatus["error"] = err.Error()
	} else {
		health.ProviderHealth = "healthy"
	}

	// Query quota status
	if s.quotaManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		quota, err := s.quotaManager.GetQuotaStatus(ctx)
		if err == nil && quota != nil {
			health.Quota = quota
			// Calculate total size in MB
			health.TotalSizeMB = quota.Used / (1024 * 1024)
			// Determine health status based on quota
			if quota.Limit > 0 {
				usagePercent := float64(quota.Used) / float64(quota.Limit) * 100
				if usagePercent >= float64(quota.FullThreshold) {
					health.Status = types.HealthStatusFull
				} else if usagePercent >= float64(quota.WarningThreshold) {
					if health.Status != types.HealthStatusFull {
						health.Status = types.HealthStatusWarning
					}
				}
			}
		}
	}

	// Query integrity error count
	if s.integrityManager != nil {
		health.IntegrityErrors = s.integrityManager.GetErrorCount()
		if health.IntegrityErrors > 0 {
			health.Status = types.HealthStatusCorrupted
		}
	}

	// Query object counts by data type
	if s.quotaManager != nil {
		objectCounts := s.quotaManager.GetObjectCounts()
		for dataType, count := range objectCounts {
			health.ObjectCounts[dataType] = count
		}
	}

	// Query retention cleanup statistics
	if s.retentionManager != nil {
		health.LastCleanupTime = s.retentionManager.GetLastCleanupTime()
		cleanupStats := s.retentionManager.GetLastCleanupStats()
		if cleanupStats != nil {
			// Convert impl.CleanupStats to types.CleanupStats
			health.CleanupStats = &types.CleanupStats{
				ObjectsDeleted:     cleanupStats.ObjectsDeleted,
				SpaceFreedBytes:    cleanupStats.SpaceFreedBytes,
				DataTypesProcessed: cleanupStats.DataTypesProcessed,
				Duration:           cleanupStats.Duration,
			}
			
			// Record cleanup sample in metrics
			if s.metricsManager != nil {
				s.metricsManager.RecordCleanupSample(cleanupStats)
			}
		}
	}

	return health
}

// GetMetricsSummary returns a summary of all operational metrics.
// This can be exposed via a separate metrics endpoint or included in health snapshot.
func (s *ObjectStorageImpl) GetMetricsSummary() MetricsSummary {
	if s.metricsManager == nil {
		return MetricsSummary{}
	}
	return s.metricsManager.GetMetricsSummary()
}

// StoreDataUnit stores a data unit (device-agnostic).
// This replaces StoreClip, StoreSnapshot, StoreFrame.
func (s *ObjectStorageImpl) StoreDataUnit(ctx context.Context, deviceID types.DeviceID, deviceType types.DeviceType, dataType types.DataType, key string, r io.Reader, size int64, contentType string) error {
	startTime := time.Now()

	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		if s.metricsManager != nil {
			s.metricsManager.RecordOperation(OperationTypeStore, string(dataType), time.Since(startTime), types.ErrNotInitialized)
		}
		return types.ErrNotInitialized
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, size); err != nil {
			return types.ErrQuotaExceeded
		}
	}

	// Read all data to calculate hash
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	// Encrypt data if encryption is enabled and data type requires it
	// Datasets (video clips, images, sensor readings) and security events require encryption
	requiresEncryption := s.requiresEncryption(dataType)
	if requiresEncryption && s.encryptionProvider != nil {
		encryptedData, err := s.encryptionProvider.Encrypt(ctx, data)
		if err != nil {
			return fmt.Errorf("failed to encrypt data: %w", err)
		}
		data = encryptedData
	}

	// Calculate hash for integrity verification (on encrypted data if encrypted)
	var hash string
	if s.integrityManager != nil {
		hash = s.integrityManager.CalculateHash(data)
	}

	// Store object via provider with retention, integrity, and encryption metadata
	metadata := make(map[string]string)
	metadata["device_id"] = string(deviceID)
	metadata["device_type"] = string(deviceType)
	metadata["data_type"] = string(dataType)
	// Store retention metadata: creation timestamp
	metadata["created_at"] = time.Now().Format(time.RFC3339)
	// Upload completion timestamp (for dataset retention)
	metadata["upload_completed_at"] = time.Now().Format(time.RFC3339)
	// Store integrity metadata: hash
	if hash != "" {
		metadata["hash"] = hash
	}
	// Store encryption metadata if encrypted
	if requiresEncryption && s.encryptionProvider != nil {
		metadata["encrypted"] = "true"
		if s.config != nil && s.config.EncryptionConfig != nil {
			metadata["encryption_algorithm"] = s.config.EncryptionConfig.Algorithm
		}
	}

	// Create reader from data (encrypted if encryption was applied)
	dataReader := bytes.NewReader(data)
	err = s.provider.StoreObject(ctx, key, dataReader, int64(len(data)), contentType, metadata)

	// Record operation metrics
	if s.metricsManager != nil {
		latency := time.Since(startTime)
		s.metricsManager.RecordOperation(OperationTypeStore, string(dataType), latency, err)
	}

	return err
}

// LoadDataUnit loads a data unit by key (device-agnostic).
// This replaces LoadClip, LoadSnapshot, LoadFrame.
func (s *ObjectStorageImpl) LoadDataUnit(ctx context.Context, key string) (io.ReadCloser, error) {
	startTime := time.Now()

	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		if s.metricsManager != nil {
			s.metricsManager.RecordOperation(OperationTypeLoad, "unknown", time.Since(startTime), types.ErrNotInitialized)
		}
		return nil, types.ErrNotInitialized
	}

	reader, err := s.provider.LoadObject(ctx, key)
	if err != nil {
		// Try to extract data type from key for metrics
		dataType := s.extractDataTypeFromKey(key)
		if s.metricsManager != nil {
			s.metricsManager.RecordOperation(OperationTypeLoad, dataType, time.Since(startTime), err)
		}
		return nil, err
	}

	// Read all data
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		dataType := s.extractDataTypeFromKey(key)
		if s.metricsManager != nil {
			s.metricsManager.RecordOperation(OperationTypeLoad, dataType, time.Since(startTime), err)
		}
		return nil, fmt.Errorf("failed to read object: %w", err)
	}

	// Decrypt if encrypted
	if s.encryptionProvider != nil && s.isObjectEncrypted(ctx, key) {
		decryptedData, err := s.encryptionProvider.Decrypt(ctx, data)
		if err != nil {
			dataType := s.extractDataTypeFromKey(key)
			if s.metricsManager != nil {
				s.metricsManager.RecordOperation(OperationTypeLoad, dataType, time.Since(startTime), err)
			}
			return nil, fmt.Errorf("failed to decrypt object: %w", err)
		}
		data = decryptedData
	}

	// Verify integrity if integrity manager is available
	if s.integrityManager != nil {
		// Verify hash (on encrypted data if encrypted)
		if err := s.verifyObjectHash(ctx, key, data); err != nil {
			dataType := s.extractDataTypeFromKey(key)
			if s.metricsManager != nil {
				s.metricsManager.RecordOperation(OperationTypeLoad, dataType, time.Since(startTime), err)
			}
			return nil, err
		}
	}

	// Record successful operation
	dataType := s.extractDataTypeFromKey(key)
	if s.metricsManager != nil {
		s.metricsManager.RecordOperation(OperationTypeLoad, dataType, time.Since(startTime), nil)
	}

	// Return data as reader (decrypted if it was encrypted)
	return io.NopCloser(bytes.NewReader(data)), nil
}

// requiresEncryption checks if a data type requires encryption.
// Datasets (video clips, images, sensor readings) and security events require encryption.
func (s *ObjectStorageImpl) requiresEncryption(dataType types.DataType) bool {
	if s.config == nil || s.config.EncryptionConfig == nil || !s.config.EncryptionConfig.Enabled {
		return false
	}

	// Datasets and security events require encryption
	encryptedTypes := map[types.DataType]bool{
		types.DataTypeVideoClip:               true,
		types.DataTypeVideoFrame:              true,
		types.DataTypeImage:                   true,
		types.DataTypeSensorReading:           true,
		types.DataTypeAudioSample:             true,
		types.DataTypeSecurityEventAttachment: true,
	}

	return encryptedTypes[dataType]
}

// isObjectEncrypted checks if an object is encrypted by checking metadata.
func (s *ObjectStorageImpl) isObjectEncrypted(ctx context.Context, key string) bool {
	// Get metadata from provider
	// Note: Provider should return metadata map, but ObjectMetadata doesn't have a metadata map field
	// For now, we'll check if encryption is enabled in config
	// TODO: Provider should support returning metadata map or we need to store encryption flag differently
	if s.config == nil || s.config.EncryptionConfig == nil || !s.config.EncryptionConfig.Enabled {
		return false
	}

	// If encryption is enabled, check if this data type requires encryption
	// Note: This is a simplified check - in production, we should check the actual object metadata
	// For now, we assume objects stored with encryption enabled are encrypted
	return true
}

// verifyObjectHash verifies the hash of an object.
func (s *ObjectStorageImpl) verifyObjectHash(ctx context.Context, key string, data []byte) error {
	// Get metadata to get stored hash
	metadata, err := s.provider.GetObjectMetadata(ctx, key)
	if err != nil {
		// If we can't get metadata, skip verification (object may be from before integrity tracking)
		return nil
	}

	// If no hash is stored, we can't verify (object may be from before integrity tracking)
	if metadata.Hash == "" {
		return nil
	}

	// Calculate hash of data
	calculatedHash := s.integrityManager.CalculateHash(data)

	// Compare hashes
	if calculatedHash != metadata.Hash {
		if s.logger != nil {
			s.logger.Error("Hash mismatch detected",
				zap.String("key", key),
				zap.String("expected", metadata.Hash),
				zap.String("calculated", calculatedHash))
		}
		return types.ErrCorruptionDetected
	}

	return nil
}

// DeleteDataUnit deletes a data unit by key (device-agnostic).
// This replaces DeleteClip, DeleteSnapshot, DeleteFrame.
func (s *ObjectStorageImpl) DeleteDataUnit(ctx context.Context, key string) error {
	startTime := time.Now()

	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		if s.metricsManager != nil {
			s.metricsManager.RecordOperation(OperationTypeDelete, "unknown", time.Since(startTime), types.ErrNotInitialized)
		}
		return types.ErrNotInitialized
	}

	err := s.provider.DeleteObject(ctx, key)
	
	// Record operation metrics
	dataType := s.extractDataTypeFromKey(key)
	if s.metricsManager != nil {
		s.metricsManager.RecordOperation(OperationTypeDelete, dataType, time.Since(startTime), err)
	}
	
	return err
}

// extractDataTypeFromKey extracts the data type from an object key.
// Key format: {dataType}/{deviceType}/{deviceID}/{YYYY-MM-DD}/{filename}
func (s *ObjectStorageImpl) extractDataTypeFromKey(key string) string {
	// Extract first part of key path
	parts := strings.Split(key, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

// GenerateDataUnitKey generates a unique key for a data unit (device-agnostic).
// This replaces GenerateClipKey, GenerateSnapshotKey.
func (s *ObjectStorageImpl) GenerateDataUnitKey(deviceID types.DeviceID, deviceType types.DeviceType, dataType types.DataType, isThumbnail bool) string {
	return GenerateDataUnitKey(deviceID, deviceType, dataType, isThumbnail)
}

// StoreModelArtifacts stores model artifacts (model binary, metadata, manifest).
// This replaces StoreModel.
// The manifest parameter is optional - if provided, it will be stored and used for version tracking.
// Artifacts map should contain keys: "model", "metadata", "manifest"
func (s *ObjectStorageImpl) StoreModelArtifacts(ctx context.Context, modelID string, deviceID types.DeviceID, manifest *types.ModelManifest, artifacts map[string][]byte) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return types.ErrNotInitialized
	}

	// Extract version from manifest if provided, otherwise use empty string
	version := ""
	if manifest != nil {
		version = manifest.Version
		// Validate that manifest.ModelID matches modelID
		if manifest.ModelID != modelID {
			return fmt.Errorf("manifest model ID (%s) does not match provided model ID (%s)", manifest.ModelID, modelID)
		}
		// Validate that manifest.DeviceID matches deviceID
		if manifest.DeviceID != deviceID {
			return fmt.Errorf("manifest device ID (%s) does not match provided device ID (%s)", manifest.DeviceID, deviceID)
		}
	}

	// Calculate total size of all artifacts
	var totalSize int64
	for _, data := range artifacts {
		totalSize += int64(len(data))
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, totalSize); err != nil {
			return types.ErrQuotaExceeded
		}
	}

	// Calculate hashes for all artifacts before storing
	// This allows us to verify integrity on load
	artifactHashes := make(map[string]string)
	for artifactType, data := range artifacts {
		if s.integrityManager != nil {
			artifactHashes[artifactType] = s.integrityManager.CalculateHash(data)
		}
	}

	// Store each artifact
	for artifactType, data := range artifacts {
		key := s.GenerateModelKey(modelID, deviceID, artifactType)

		// Encrypt artifact if encryption is enabled
		// Model artifacts may contain sensitive data, so encrypt if enabled
		if s.encryptionProvider != nil && s.config != nil && s.config.EncryptionConfig != nil && s.config.EncryptionConfig.Enabled {
			encryptedData, err := s.encryptionProvider.Encrypt(ctx, data)
			if err != nil {
				return fmt.Errorf("failed to encrypt artifact %s: %w", artifactType, err)
			}
			data = encryptedData
		}

		// Calculate hash for integrity verification (on encrypted data if encrypted)
		var hash string
		if s.integrityManager != nil {
			hash = s.integrityManager.CalculateHash(data)
		}

		var contentType string
		switch artifactType {
		case "model":
			contentType = "application/octet-stream"
		case "metadata", "manifest":
			contentType = "application/json"
		default:
			contentType = "application/octet-stream"
		}

		metadata := make(map[string]string)
		metadata["model_id"] = modelID
		metadata["device_id"] = string(deviceID)
		metadata["artifact_type"] = artifactType
		// Store model version information if available
		if version != "" {
			metadata["model_version"] = version
		}
		// Store retention metadata: creation timestamp
		createdAt := time.Now()
		if manifest != nil && !manifest.CreatedAt.IsZero() {
			createdAt = manifest.CreatedAt
		}
		metadata["created_at"] = createdAt.Format(time.RFC3339)
		// Store integrity metadata: hash
		if hash != "" {
			metadata["hash"] = hash
		}
		// Store artifact hash from manifest if available (for cross-verification)
		if manifest != nil && manifest.ArtifactHashes != nil {
			if manifestHash, ok := manifest.ArtifactHashes[artifactType]; ok {
				metadata["manifest_hash"] = manifestHash
			}
		}
		// Store encryption metadata if encrypted
		if s.encryptionProvider != nil && s.config != nil && s.config.EncryptionConfig != nil && s.config.EncryptionConfig.Enabled {
			metadata["encrypted"] = "true"
			metadata["encryption_algorithm"] = s.config.EncryptionConfig.Algorithm
		}

		reader := bytes.NewReader(data)
		if err := s.provider.StoreObject(ctx, key, reader, int64(len(data)), contentType, metadata); err != nil {
			return fmt.Errorf("failed to store artifact %s: %w", artifactType, err)
		}
	}

	return nil
}

// LoadModelArtifacts loads model artifacts by model ID.
// This replaces LoadModel and LoadModelMetadata.
// Returns structured ModelArtifacts with all artifacts and their integrity hashes.
func (s *ObjectStorageImpl) LoadModelArtifacts(ctx context.Context, modelID string, deviceID types.DeviceID) (*types.ModelArtifacts, error) {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return nil, types.ErrNotInitialized
	}

	result := &types.ModelArtifacts{
		ModelID:  modelID,
		DeviceID: deviceID,
		Hashes:   make(map[string]string),
	}

	artifactTypes := []string{"model", "metadata", "manifest"}
	var foundArtifacts []string

	for _, artifactType := range artifactTypes {
		key := s.GenerateModelKey(modelID, deviceID, artifactType)
		reader, err := s.provider.LoadObject(ctx, key)
		if err != nil {
			// Artifact might not exist, skip it
			continue
		}

		data, err := io.ReadAll(reader)
		reader.Close() // Close immediately after reading
		if err != nil {
			return nil, fmt.Errorf("failed to read artifact %s: %w", artifactType, err)
		}

		// Decrypt if encrypted
		if s.encryptionProvider != nil && s.isObjectEncrypted(ctx, key) {
			decryptedData, err := s.encryptionProvider.Decrypt(ctx, data)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt artifact %s: %w", artifactType, err)
			}
			data = decryptedData
		}

		// Get metadata to extract version and hash
		objMetadata, err := s.provider.GetObjectMetadata(ctx, key)
		if err == nil && objMetadata != nil {
			// Extract version from metadata map (stored as "model_version")
			if result.Version == "" && objMetadata.Metadata != nil {
				if versionStr, ok := objMetadata.Metadata["model_version"]; ok && versionStr != "" {
					result.Version = versionStr
				}
			}
			// Extract hash from metadata
			if objMetadata.Hash != "" {
				result.Hashes[artifactType] = objMetadata.Hash
			}
			// Extract created at time
			if !objMetadata.CreatedAt.IsZero() {
				result.CreatedAt = objMetadata.CreatedAt
			}
		}

		// Verify integrity before returning
		if s.integrityManager != nil {
			if err := s.integrityManager.VerifyObjectIntegrity(ctx, key); err != nil {
				return nil, fmt.Errorf("integrity verification failed for artifact %s: %w", artifactType, err)
			}
		}

		// Store artifact in result
		switch artifactType {
		case "model":
			result.Model = data
		case "metadata":
			result.Metadata = data
		case "manifest":
			result.Manifest = data
		}

		foundArtifacts = append(foundArtifacts, artifactType)
	}

	if len(foundArtifacts) == 0 {
		return nil, fmt.Errorf("no artifacts found for model %s", modelID)
	}

	// Extract version from metadata if available
	// Try to get version from manifest metadata
	if len(result.Manifest) > 0 {
		// Try to parse manifest JSON to extract version
		// For now, we'll rely on metadata stored in object metadata
		// Full manifest parsing can be added later
	}

	// If version is still empty, try to get it from any artifact's metadata
	if result.Version == "" {
		for _, artifactType := range artifactTypes {
			key := s.GenerateModelKey(modelID, deviceID, artifactType)
			objMetadata, err := s.provider.GetObjectMetadata(ctx, key)
			if err == nil && objMetadata != nil && objMetadata.Metadata != nil {
				if versionStr, ok := objMetadata.Metadata["model_version"]; ok && versionStr != "" {
					result.Version = versionStr
					break
				}
			}
		}
	}

	return result, nil
}

// DeleteModelArtifacts deletes model artifacts by model ID.
// This replaces DeleteModel.
func (s *ObjectStorageImpl) DeleteModelArtifacts(ctx context.Context, modelID string, deviceID types.DeviceID) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return types.ErrNotInitialized
	}

	artifactTypes := []string{"model", "metadata", "manifest"}
	for _, artifactType := range artifactTypes {
		key := s.GenerateModelKey(modelID, deviceID, artifactType)
		if err := s.provider.DeleteObject(ctx, key); err != nil {
			s.logger.Warn("Failed to delete model artifact",
				zap.String("model_id", modelID),
				zap.String("artifact_type", artifactType),
				zap.String("key", key),
				zap.Error(err),
			)
		}
	}

	return nil
}

// GenerateModelKey generates a key for a model artifact.
func (s *ObjectStorageImpl) GenerateModelKey(modelID string, deviceID types.DeviceID, artifactType string) string {
	return GenerateModelKey(modelID, deviceID, artifactType)
}

// StoreSecurityEventAttachment stores an attachment for a security event.
func (s *ObjectStorageImpl) StoreSecurityEventAttachment(ctx context.Context, eventID string, deviceID types.DeviceID, dataType types.DataType, data []byte) (string, error) {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return "", types.ErrNotInitialized
	}

	// Optimize attachment based on quota usage
	optimizedData := data
	if s.attachmentManager != nil {
		optData, wasOptimized, err := s.attachmentManager.OptimizeAttachment(ctx, dataType, data)
		if err != nil {
			s.logger.Warn("Failed to optimize attachment, using original",
				zap.String("event_id", eventID),
				zap.String("data_type", string(dataType)),
				zap.Error(err))
		} else {
			optimizedData = optData
			if wasOptimized {
				s.logger.Info("Attachment optimized",
					zap.String("event_id", eventID),
					zap.String("data_type", string(dataType)),
					zap.Int("original_size", len(data)),
					zap.Int("optimized_size", len(optimizedData)))
			}
		}
	}

	// Check quota before write (using optimized size)
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, int64(len(optimizedData))); err != nil {
			return "", types.ErrQuotaExceeded
		}
	}

	key := s.GenerateSecurityEventAttachmentKey(eventID, deviceID, dataType)

	// Encrypt if encryption is enabled (security event attachments always require encryption if enabled)
	// Note: Encrypt after optimization to ensure optimization works on plaintext
	dataToStore := optimizedData
	if s.encryptionProvider != nil && s.config != nil && s.config.EncryptionConfig != nil && s.config.EncryptionConfig.Enabled {
		encryptedData, err := s.encryptionProvider.Encrypt(ctx, optimizedData)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt security event attachment: %w", err)
		}
		dataToStore = encryptedData
	}

	// Calculate hash for integrity verification (on encrypted data if encrypted)
	var hash string
	if s.integrityManager != nil {
		hash = s.integrityManager.CalculateHash(dataToStore)
	}

	var contentType string
	switch dataType {
	case types.DataTypeImage:
		contentType = "image/jpeg"
	case types.DataTypeSensorReading:
		contentType = "application/json"
	case types.DataTypeAudioSample:
		contentType = "audio/wav"
	case types.DataTypeVideoClip:
		contentType = "video/mp4"
	default:
		contentType = "application/octet-stream"
	}

	metadata := make(map[string]string)
	metadata["event_id"] = eventID
	metadata["device_id"] = string(deviceID)
	metadata["data_type"] = string(dataType)
	// Store retention metadata: creation timestamp
	metadata["created_at"] = time.Now().Format(time.RFC3339)
	// VM acknowledgment timestamp (will be updated when VM acks the event)
	// For now, set to zero - will be updated via UpdateRetentionMetadata
	metadata["vm_ack_at"] = ""
	// Store integrity metadata: hash
	if hash != "" {
		metadata["hash"] = hash
	}
	// Store encryption metadata if encrypted
	if s.encryptionProvider != nil && s.config != nil && s.config.EncryptionConfig != nil && s.config.EncryptionConfig.Enabled {
		metadata["encrypted"] = "true"
		metadata["encryption_algorithm"] = s.config.EncryptionConfig.Algorithm
	}

	// Store optimization metadata if optimization was applied
	if s.attachmentManager != nil && len(optimizedData) < len(data) {
		metadata["optimized"] = "true"
		metadata["original_size"] = fmt.Sprintf("%d", len(data))
		// Check if it's likely gzip compressed (starts with gzip magic bytes)
		if len(optimizedData) >= 2 && optimizedData[0] == 0x1f && optimizedData[1] == 0x8b {
			metadata["content_encoding"] = "gzip"
		}
	}

	reader := bytes.NewReader(dataToStore)
	if err := s.provider.StoreObject(ctx, key, reader, int64(len(dataToStore)), contentType, metadata); err != nil {
		return "", fmt.Errorf("failed to store security event attachment: %w", err)
	}

	return key, nil
}

// LoadSecurityEventAttachment loads a security event attachment by key.
func (s *ObjectStorageImpl) LoadSecurityEventAttachment(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return nil, types.ErrNotInitialized
	}

	reader, err := s.provider.LoadObject(ctx, key)
	if err != nil {
		return nil, err
	}

	// Read all data
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read security event attachment: %w", err)
	}

	// Decrypt if encrypted
	if s.encryptionProvider != nil && s.isObjectEncrypted(ctx, key) {
		decryptedData, err := s.encryptionProvider.Decrypt(ctx, data)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt security event attachment: %w", err)
		}
		data = decryptedData
	}

	// Decompress if compressed (gzip)
	// Check if data is gzip compressed (starts with gzip magic bytes)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gzipReader, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			decompressed, err := io.ReadAll(gzipReader)
			gzipReader.Close()
			if err == nil {
				data = decompressed
			}
		}
	}

	// Verify integrity if integrity manager is available
	if s.integrityManager != nil {
		// Verify hash (on encrypted data if encrypted)
		if err := s.verifyObjectHash(ctx, key, data); err != nil {
			return nil, err
		}
	}

	// Return data as reader (decrypted if it was encrypted)
	return io.NopCloser(bytes.NewReader(data)), nil
}

// DeleteSecurityEventAttachment deletes a security event attachment by key.
func (s *ObjectStorageImpl) DeleteSecurityEventAttachment(ctx context.Context, key string) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return types.ErrNotInitialized
	}

	return s.provider.DeleteObject(ctx, key)
}

// GenerateSecurityEventAttachmentKey generates a key for a security event attachment.
func (s *ObjectStorageImpl) GenerateSecurityEventAttachmentKey(eventID string, deviceID types.DeviceID, dataType types.DataType) string {
	return GenerateSecurityEventAttachmentKey(eventID, deviceID, dataType)
}

// UpdateRetentionMetadata updates retention metadata for an object.
// This is used to update VM acknowledgment timestamps for event attachments.
// Note: This requires provider support for metadata updates. If not supported,
// the metadata will need to be updated through meta-storage integration.
func (s *ObjectStorageImpl) UpdateRetentionMetadata(ctx context.Context, key string, vmAckAt time.Time) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return types.ErrNotInitialized
	}

	// Get current metadata
	metadata, err := s.provider.GetObjectMetadata(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get object metadata: %w", err)
	}

	// Update VM ack timestamp in metadata
	// Note: This is a placeholder - actual implementation depends on provider support
	// for metadata updates. Some providers may require re-uploading the object.
	// For now, this metadata is stored and can be retrieved, but updating it
	// may require provider-specific implementation.
	_ = metadata
	_ = vmAckAt

	// TODO: Implement provider-specific metadata update
	// This may require:
	// 1. Provider support for metadata-only updates (S3/MinIO support this)
	// 2. Or integration with meta-storage to track VM ack timestamps separately

	return nil
}

// EmitStorageEvent emits a storage-related event.
// This implements the StorageEventEmitter interface, allowing the service
// to emit events that can be consumed by other services via the event bus.
//
// Event types:
//   - storage.warning: Quota usage between 80-90%
//   - storage.full: Quota usage >95%
//   - storage.quota_exceeded: Quota exceeded during write operation
//   - storage.cleanup_started: Retention cleanup operation started
//   - storage.cleanup_completed: Retention cleanup operation completed
//   - storage.corruption_detected: Storage integrity corruption detected
//
// This method logs the event and can be extended to integrate with an actual event bus.
// For now, events are logged for observability. Full event bus integration
// will be completed when the event bus service is available.
func (s *ObjectStorageImpl) EmitStorageEvent(eventType string, data map[string]interface{}) {
	// Log the event for observability
	s.logger.Info("Storage event emitted",
		zap.String("event_type", eventType),
		zap.Any("data", data))

	// TODO: Integrate with actual event bus when available
	// This would typically involve:
	// 1. Getting event bus from dependency injection
	// 2. Publishing event to event bus with structured event type
	// 3. Event bus would handle delivery to subscribers
	//
	// Example integration (when event bus is available):
	// if s.eventBus != nil {
	//     s.eventBus.Publish(eventType, data)
	// }
}

// ListModelVersions lists all model versions for a specific device.
// Versions are sorted by creation time (newest first).
func (s *ObjectStorageImpl) ListModelVersions(ctx context.Context, deviceID types.DeviceID) ([]types.ModelVersion, error) {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return nil, types.ErrNotInitialized
	}

	if s.modelVersionManager == nil {
		return nil, fmt.Errorf("model version manager not initialized")
	}

	return s.modelVersionManager.ListModelVersions(ctx, deviceID)
}

// DeleteOldModelVersions deletes old model versions for a device, keeping only the last N versions.
// This enforces the retention policy for model versions.
func (s *ObjectStorageImpl) DeleteOldModelVersions(ctx context.Context, deviceID types.DeviceID, keepN int) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		return types.ErrNotInitialized
	}

	if s.modelVersionManager == nil {
		return fmt.Errorf("model version manager not initialized")
	}

	return s.modelVersionManager.DeleteOldModelVersions(ctx, deviceID, keepN)
}

