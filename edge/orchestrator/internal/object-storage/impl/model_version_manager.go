package impl

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	"go.uber.org/zap"
)

// ModelVersionManager manages model version tracking and retention.
// It provides functionality to list model versions, delete old versions,
// and support model rollback operations.
type ModelVersionManager struct {
	provider types.ObjectStorageProvider
	logger   *zap.Logger
}

// NewModelVersionManager creates a new ModelVersionManager.
func NewModelVersionManager(provider types.ObjectStorageProvider, logger *zap.Logger) *ModelVersionManager {
	return &ModelVersionManager{
		provider: provider,
		logger:   logger,
	}
}

// ListModelVersions lists all model versions for a specific device.
// Versions are sorted by creation time (newest first).
func (m *ModelVersionManager) ListModelVersions(ctx context.Context, deviceID types.DeviceID) ([]types.ModelVersion, error) {
	// List all model artifacts for this device
	prefix := fmt.Sprintf("models/%s/", string(deviceID))
	objects, err := m.provider.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list model artifacts: %w", err)
	}

	// Group objects by model ID
	// Key format: models/{deviceID}/{modelID}/{artifactType}.{ext}
	modelsByModelID := make(map[string][]types.ObjectInfo)
	for _, obj := range objects {
		// Extract model ID from key: models/{deviceID}/{modelID}/...
		parts := strings.Split(strings.TrimPrefix(obj.Key, prefix), "/")
		if len(parts) < 1 {
			continue
		}
		modelID := parts[0]
		modelsByModelID[modelID] = append(modelsByModelID[modelID], obj)
	}

	// Build ModelVersion list
	var versions []types.ModelVersion
	for modelID, objs := range modelsByModelID {
		if len(objs) == 0 {
			continue
		}

		// Get creation time and version from first object's metadata
		// All artifacts for a model should have the same creation time and version
		createdAt := time.Time{}
		version := ""
		var totalSize int64

		for _, obj := range objs {
			totalSize += obj.Size

			// Try to get metadata for creation time and version
			metadata, err := m.provider.GetObjectMetadata(ctx, obj.Key)
			if err == nil && metadata != nil {
				if createdAt.IsZero() && !metadata.CreatedAt.IsZero() {
					createdAt = metadata.CreatedAt
				}
				// Version would be in metadata map, but we'll extract from key for now
				// Full version extraction requires metadata map support
			}

			// Fall back to extracting time from key or LastModified
			if createdAt.IsZero() {
				if obj.LastModified > 0 {
					createdAt = time.Unix(obj.LastModified, 0)
				}
			}
		}

		// If we still don't have a creation time, use current time as fallback
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		// Extract version from model ID if it contains version info
		// For now, we'll use modelID as version identifier
		// Full version extraction requires metadata support
		if version == "" {
			version = modelID // Use modelID as version identifier
		}

		versions = append(versions, types.ModelVersion{
			ModelID:        modelID,
			DeviceID:       deviceID,
			Version:        version,
			CreatedAt:      createdAt,
			ArtifactCount:  len(objs),
			TotalSizeBytes: totalSize,
		})
	}

	// Sort by creation time (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].CreatedAt.After(versions[j].CreatedAt)
	})

	return versions, nil
}

// DeleteOldModelVersions deletes old model versions for a device, keeping only the last N versions.
// This enforces the retention policy for model versions.
func (m *ModelVersionManager) DeleteOldModelVersions(ctx context.Context, deviceID types.DeviceID, keepN int) error {
	if keepN < 0 {
		return fmt.Errorf("keepN must be non-negative")
	}

	// List all model versions for this device
	versions, err := m.ListModelVersions(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("failed to list model versions: %w", err)
	}

	// If we have fewer versions than keepN, nothing to delete
	if len(versions) <= keepN {
		return nil
	}

	// Delete old versions (keep only last N)
	deletedCount := 0
	for i := keepN; i < len(versions); i++ {
		version := versions[i]
		if err := m.deleteModelVersion(ctx, deviceID, version.ModelID); err != nil {
			if m.logger != nil {
				m.logger.Warn("Failed to delete old model version",
					zap.String("device_id", string(deviceID)),
					zap.String("model_id", version.ModelID),
					zap.String("version", version.Version),
					zap.Error(err))
			}
			continue
		}
		deletedCount++
	}

	if m.logger != nil {
		m.logger.Info("Deleted old model versions",
			zap.String("device_id", string(deviceID)),
			zap.Int("deleted_count", deletedCount),
			zap.Int("kept_count", keepN))
	}

	return nil
}

// deleteModelVersion deletes all artifacts for a specific model version.
func (m *ModelVersionManager) deleteModelVersion(ctx context.Context, deviceID types.DeviceID, modelID string) error {
	// List all artifacts for this model
	prefix := fmt.Sprintf("models/%s/%s/", string(deviceID), modelID)
	objects, err := m.provider.ListObjects(ctx, prefix)
	if err != nil {
		return fmt.Errorf("failed to list model artifacts: %w", err)
	}

	// Delete all artifacts
	for _, obj := range objects {
		if err := m.provider.DeleteObject(ctx, obj.Key); err != nil {
			if m.logger != nil {
				m.logger.Warn("Failed to delete model artifact",
					zap.String("device_id", string(deviceID)),
					zap.String("model_id", modelID),
					zap.String("key", obj.Key),
					zap.Error(err))
			}
			// Continue deleting other artifacts even if one fails
		}
	}

	return nil
}

// LoadModelVersion loads a specific model version by model ID.
// This supports model rollback by loading a previous version.
func (m *ModelVersionManager) LoadModelVersion(ctx context.Context, deviceID types.DeviceID, modelID string) (*types.ModelArtifacts, error) {
	// This is a helper method that can be used by ObjectStorageImpl
	// to load a specific model version for rollback
	// The actual loading is done by ObjectStorageImpl.LoadModelArtifacts
	// This method just validates that the model version exists
	prefix := fmt.Sprintf("models/%s/%s/", string(deviceID), modelID)
	objects, err := m.provider.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list model artifacts: %w", err)
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("model version not found: %s for device %s", modelID, deviceID)
	}

	// Model version exists, but actual loading is done by ObjectStorageImpl
	// This is just a validation method
	return nil, nil
}

