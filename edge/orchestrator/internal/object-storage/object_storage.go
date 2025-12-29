package objectstorage

import (
	"context"
	"fmt"
	"io"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/impl/minio"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Re-export errors from types package for convenience
var (
	ErrNotInitialized     = types.ErrNotInitialized
	ErrAlreadyStarted     = types.ErrAlreadyStarted
	ErrQuotaExceeded      = types.ErrQuotaExceeded
	ErrObjectNotFound     = types.ErrObjectNotFound
	ErrCorruptionDetected = types.ErrCorruptionDetected
)

// ObjectStorageService provides operations for storing and retrieving binary objects
// (clips, snapshots, models, sensor data, etc.) using provider-agnostic object storage.
//
// This service handles the actual object storage operations, while metadata about these objects
// is managed by MetaDataStore in the meta-storage package.
//
// The service is provider-agnostic and supports multiple storage backends:
//   - MinIO (current)
//   - S3 (future)
//   - Local filesystem (future)
//
// The service provides production features:
//   - Quota enforcement
//   - Retention policies
//   - Integrity verification
//   - Health monitoring
//
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_object_storage.go -package=mocks
type ObjectStorageService interface {
	// Lifecycle hooks
	
	// Start initializes the storage service and starts background tasks.
	// This method:
	//   - Verifies provider connectivity
	//   - Initializes quota monitoring (if configured)
	//   - Starts background tasks (retention cleanup, integrity checks, metrics collection)
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns:
	//   - ErrAlreadyStarted if the service is already started
	//   - Other errors if initialization fails
	Start(ctx context.Context) error

	// Stop stops the storage service and releases resources.
	// This method:
	//   - Stops background tasks gracefully
	//   - Closes provider connections
	//   - Flushes pending operations
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns:
	//   - Errors if shutdown fails (errors are aggregated)
	Stop(ctx context.Context) error

	// Name returns the service name for identification.
	//
	// Returns:
	//   - The service name string
	Name() string

	// Health monitoring
	
	// HealthSnapshot returns the current health status of the storage service.
	// This provides a comprehensive view of storage health including:
	//   - Overall health status (healthy, warning, full, corrupted)
	//   - Quota status (usage, limits, thresholds)
	//   - Integrity status (error count, last check time)
	//   - Provider health (provider-specific status)
	//   - Object counts by data type
	//   - Cleanup statistics
	//   - Operational metrics summary
	//
	// Returns:
	//   - StorageHealth struct containing all health information
	HealthSnapshot() types.StorageHealth

	// Data Unit Storage (device-agnostic, replaces camera-specific methods)
	
	// StoreDataUnit stores a data unit (image, video clip, sensor reading, etc.).
	// The data unit is automatically encrypted if encryption is enabled and the data type requires it.
	// Quota is checked before storage, and the operation is rejected if quota is exceeded.
	// Integrity hash is calculated and stored for verification.
	// Retention metadata (created_at, upload_completed_at) is stored for cleanup.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - deviceID: The device that generated this data unit
	//   - deviceType: The type of device (camera, sensor, audio_device, etc.)
	//   - dataType: The type of data (image, video_clip, sensor_reading, etc.)
	//   - key: The object storage key (use GenerateDataUnitKey to generate)
	//   - r: Reader containing the data unit content
	//   - size: Size of the data unit in bytes
	//   - contentType: MIME type of the data unit
	//
	// Returns:
	//   - ErrQuotaExceeded if storage quota is exceeded
	//   - ErrNotInitialized if service is not started
	//   - Other errors if storage operation fails
	StoreDataUnit(ctx context.Context, deviceID types.DeviceID, deviceType types.DeviceType, dataType types.DataType, key string, r io.Reader, size int64, contentType string) error

	// LoadDataUnit loads a data unit by key.
	// The data unit is automatically decrypted if it was encrypted.
	// Integrity hash is verified before returning the data.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object storage key
	//
	// Returns:
	//   - io.ReadCloser containing the data unit content (caller must close)
	//   - ErrObjectNotFound if the object does not exist
	//   - ErrCorruptionDetected if integrity verification fails
	//   - ErrNotInitialized if service is not started
	//   - Other errors if load operation fails
	LoadDataUnit(ctx context.Context, key string) (io.ReadCloser, error)

	// DeleteDataUnit deletes a data unit by key.
	// This operation is idempotent - deleting a non-existent object returns no error.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object storage key
	//
	// Returns:
	//   - ErrNotInitialized if service is not started
	//   - Other errors if delete operation fails
	DeleteDataUnit(ctx context.Context, key string) error

	// GenerateDataUnitKey generates a unique key for a data unit.
	// The key organizes objects by data type, device type, device ID, and date for efficient querying.
	// Format: {dataType}/{deviceType}/{deviceID}/{YYYY-MM-DD}/{deviceID_timestamp_uuid.ext}
	//
	// Parameters:
	//   - deviceID: The device that generated this data unit
	//   - deviceType: The type of device (camera, sensor, audio_device, etc.)
	//   - dataType: The type of data (image, video_clip, sensor_reading, etc.)
	//   - isThumbnail: Whether this is a thumbnail version
	//
	// Returns:
	//   - A unique key string for the data unit
	GenerateDataUnitKey(deviceID types.DeviceID, deviceType types.DeviceType, dataType types.DataType, isThumbnail bool) string

	// Model Artifact Storage
	
	// StoreModelArtifacts stores model artifacts (model binary, metadata, manifest).
	// The manifest parameter is optional - if provided, it will be stored and used for version tracking.
	// Artifacts map should contain keys: "model", "metadata", "manifest".
	// All artifacts are automatically encrypted if encryption is enabled.
	// Integrity hashes are calculated and stored for all artifacts.
	// Quota is checked before storage (total size of all artifacts).
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - modelID: The model identifier
	//   - deviceID: The device this model is deployed to
	//   - manifest: Optional model manifest for version tracking
	//   - artifacts: Map of artifact type to artifact data (keys: "model", "metadata", "manifest")
	//
	// Returns:
	//   - ErrQuotaExceeded if storage quota is exceeded
	//   - ErrNotInitialized if service is not started
	//   - Other errors if storage operation fails
	StoreModelArtifacts(ctx context.Context, modelID string, deviceID types.DeviceID, manifest *types.ModelManifest, artifacts map[string][]byte) error

	// LoadModelArtifacts loads model artifacts by model ID.
	// Returns structured ModelArtifacts with all artifacts and their integrity hashes.
	// All artifacts are automatically decrypted if they were encrypted.
	// Integrity hashes are verified before returning.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - modelID: The model identifier
	//   - deviceID: The device this model is deployed to
	//
	// Returns:
	//   - ModelArtifacts containing model, metadata, manifest, hashes, and version info
	//   - ErrObjectNotFound if the model artifacts do not exist
	//   - ErrCorruptionDetected if integrity verification fails
	//   - ErrNotInitialized if service is not started
	//   - Other errors if load operation fails
	LoadModelArtifacts(ctx context.Context, modelID string, deviceID types.DeviceID) (*types.ModelArtifacts, error)

	// DeleteModelArtifacts deletes model artifacts by model ID.
	// This operation is idempotent - deleting non-existent artifacts returns no error.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - modelID: The model identifier
	//   - deviceID: The device this model is deployed to
	//
	// Returns:
	//   - ErrNotInitialized if service is not started
	//   - Other errors if delete operation fails
	DeleteModelArtifacts(ctx context.Context, modelID string, deviceID types.DeviceID) error

	// GenerateModelKey generates a key for a model artifact.
	// Format: models/{deviceID}/{modelID}/{artifactType}.{ext}
	//
	// Parameters:
	//   - modelID: The model identifier
	//   - deviceID: The device this model is deployed to
	//   - artifactType: The artifact type ("model", "metadata", "manifest")
	//
	// Returns:
	//   - A unique key string for the model artifact
	GenerateModelKey(modelID string, deviceID types.DeviceID, artifactType string) string

	// Model Version Management
	
	// ListModelVersions lists all model versions for a specific device.
	// Versions are sorted by creation time (newest first).
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - deviceID: The device to list model versions for
	//
	// Returns:
	//   - Slice of ModelVersion structs sorted by creation time (newest first)
	//   - ErrNotInitialized if service is not started
	//   - Other errors if list operation fails
	ListModelVersions(ctx context.Context, deviceID types.DeviceID) ([]types.ModelVersion, error)

	// DeleteOldModelVersions deletes old model versions for a device, keeping only the last N versions.
	// This enforces the retention policy for model versions.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - deviceID: The device to delete old model versions for
	//   - keepN: Number of versions to keep (e.g., 2)
	//
	// Returns:
	//   - ErrNotInitialized if service is not started
	//   - Other errors if delete operation fails
	DeleteOldModelVersions(ctx context.Context, deviceID types.DeviceID, keepN int) error

	// Security Event Attachment Storage
	
	// StoreSecurityEventAttachment stores an attachment for a security event.
	// The attachment is automatically optimized (compressed/reduced quality) if quota is high.
	// The attachment is automatically encrypted if encryption is enabled.
	// Integrity hash is calculated and stored for verification.
	// Retention metadata (created_at, vm_ack_at placeholder) is stored for cleanup.
	// Quota is checked before storage.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - eventID: The security event identifier
	//   - deviceID: The device that generated this event
	//   - dataType: The type of attachment data (image, sensor_reading, audio_sample, video_clip)
	//   - data: The attachment data bytes
	//
	// Returns:
	//   - The object storage key for the attachment (store in meta-storage for event reference)
	//   - ErrQuotaExceeded if storage quota is exceeded
	//   - ErrNotInitialized if service is not started
	//   - Other errors if storage operation fails
	StoreSecurityEventAttachment(ctx context.Context, eventID string, deviceID types.DeviceID, dataType types.DataType, data []byte) (string, error)

	// LoadSecurityEventAttachment loads a security event attachment by key.
	// The attachment is automatically decrypted if it was encrypted.
	// The attachment is automatically decompressed if it was compressed.
	// Integrity hash is verified before returning the data.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object storage key (returned by StoreSecurityEventAttachment)
	//
	// Returns:
	//   - io.ReadCloser containing the attachment data (caller must close)
	//   - ErrObjectNotFound if the attachment does not exist
	//   - ErrCorruptionDetected if integrity verification fails
	//   - ErrNotInitialized if service is not started
	//   - Other errors if load operation fails
	LoadSecurityEventAttachment(ctx context.Context, key string) (io.ReadCloser, error)

	// DeleteSecurityEventAttachment deletes a security event attachment by key.
	// This operation is idempotent - deleting a non-existent attachment returns no error.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object storage key
	//
	// Returns:
	//   - ErrNotInitialized if service is not started
	//   - Other errors if delete operation fails
	DeleteSecurityEventAttachment(ctx context.Context, key string) error

	// GenerateSecurityEventAttachmentKey generates a key for a security event attachment.
	// Format: security-events/{deviceID}/{YYYY-MM-DD}/{eventID}_{dataType}.{ext}
	//
	// Parameters:
	//   - eventID: The security event identifier
	//   - deviceID: The device that generated this event
	//   - dataType: The type of attachment data
	//
	// Returns:
	//   - A unique key string for the security event attachment
	GenerateSecurityEventAttachmentKey(eventID string, deviceID types.DeviceID, dataType types.DataType) string
}

// NewObjectStorageService creates a new ObjectStorageService instance.
// This is the factory function for creating storage service instances.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - config: Storage configuration (provider-agnostic)
//   - logger: Logger instance
//
// Returns:
//   - ObjectStorageService instance
//   - Error if the provider is unsupported or initialization fails
//
// Example:
//
//	cfg := &types.ObjectStorageConfig{
//		Provider: "minio",
//		Endpoint: "localhost:9000",
//		AccessKey: "minioadmin",
//		SecretKey: "minioadmin",
//	}
//	store, err := NewObjectStorageService(ctx, cfg, logger)
func NewObjectStorageService(ctx context.Context, config *types.ObjectStorageConfig, logger *zap.Logger) (ObjectStorageService, error) {
	if config == nil {
		return nil, fmt.Errorf("object-storage: config is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("object-storage: logger is required")
	}

	// Create provider based on configuration
	var provider types.ObjectStorageProvider
	var err error

	switch config.Provider {
	case "minio":
		// Use MinIOConfig if provided, otherwise fall back to top-level config
		minioCfg := config.MinIOConfig
		if minioCfg == nil {
			// Create MinIOConfig from top-level config (backward compatibility)
			minioCfg = &types.MinIOConfig{
				Endpoint: config.Endpoint,
				AccessKey: config.AccessKey,
				SecretKey: config.SecretKey,
				Region: config.Region,
				Bucket: "edge-storage", // Default bucket
				UseSSL: false,          // Default to false
			}
		}
		provider, err = minio.NewMinIOProvider(ctx, minioCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create MinIO provider: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported object-storage provider: %s", config.Provider)
	}

	// Create service implementation with provider
	return impl.NewObjectStorageImpl(provider, config, logger), nil
}

// ObjectStorageProvider creates the object storage service with fx lifecycle management.
// This is the Fx provider function that integrates the storage service with the Fx dependency injection framework.
//
// The service lifecycle is managed by Fx:
//   - OnStart: Service is started automatically when the application starts
//   - OnStop: Service is stopped automatically when the application shuts down
//
// Parameters:
//   - lc: Fx lifecycle manager
//   - cfg: Storage configuration (provider-agnostic)
//   - logger: Logger instance
//
// Returns:
//   - ObjectStorageService instance
//   - Error if initialization fails
//
// Example:
//
//	var Module = fx.Module("object-storage",
//		fx.Provide(ObjectStorageProvider),
//	)
func ObjectStorageProvider(lc fx.Lifecycle, cfg *types.ObjectStorageConfig, logger *zap.Logger) (ObjectStorageService, error) {
	store, err := NewObjectStorageService(context.Background(), cfg, logger)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if store != nil {
				if err := store.Start(ctx); err != nil {
					return err
				}
			}
			logger.Info("Object storage started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if store != nil {
				if err := store.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("Object storage stopped")
			return nil
		},
	})

	return store, nil
}
