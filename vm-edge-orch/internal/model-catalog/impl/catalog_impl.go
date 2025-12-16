package impl

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
	modelcatalog "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/model-catalog/types"
	objectstorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/object-storage"
)

// catalog implements the modelcatalog.Catalog interface by combining
// metadata operations with object storage operations.
type catalog struct {
	metaStore  metastorage.MetaDataStore
	objStorage objectstorage.ObjectStorageService
}

// NewCatalog creates a new catalog implementation.
func NewCatalog(metaStore metastorage.MetaDataStore, objStorage objectstorage.ObjectStorageService) (modelcatalog.Catalog, error) {
	if metaStore == nil {
		return nil, fmt.Errorf("meta store is required")
	}
	if objStorage == nil {
		return nil, fmt.Errorf("object storage is required")
	}

	return &catalog{
		metaStore:  metaStore,
		objStorage: objStorage,
	}, nil
}

// GetRawModel retrieves a raw model by ID, including metadata and storage status.
func (c *catalog) GetRawModel(ctx context.Context, id uuid.UUID) (*types.RawModel, error) {
	meta, exists := c.metaStore.GetRawModel(id)
	if !exists {
		return nil, fmt.Errorf("raw model %s not found in metadata", id)
	}

	// Check if model exists in storage
	existsInStorage := c.checkRawModelExists(ctx, id)

	return &types.RawModel{
		Meta:            meta,
		ExistsInStorage: existsInStorage,
	}, nil
}

// GetTrainedModel retrieves a trained model by ID, including metadata and storage status.
// If includeSource is true, also populates the SourceModel field.
func (c *catalog) GetTrainedModel(ctx context.Context, id uuid.UUID, includeSource bool) (*types.TrainedModel, error) {
	meta, exists := c.metaStore.GetTrainedModel(id)
	if !exists {
		return nil, fmt.Errorf("trained model %s not found in metadata", id)
	}

	// Check if model exists in storage
	existsInStorage := c.checkTrainedModelExists(ctx, id)

	trainedModel := &types.TrainedModel{
		Meta:            meta,
		ExistsInStorage: existsInStorage,
	}

	// Optionally load source model
	if includeSource {
		sourceModel, err := c.GetRawModel(ctx, meta.SourceModelID)
		if err == nil {
			trainedModel.SourceModel = sourceModel
		}
		// Don't fail if source model is not found
	}

	return trainedModel, nil
}

// GetModel retrieves a model (raw or trained) by ID.
// The type is determined by checking both stores.
func (c *catalog) GetModel(ctx context.Context, id uuid.UUID) (*types.ModelInfo, error) {
	// Try raw model first
	if rawMeta, exists := c.metaStore.GetRawModel(id); exists {
		existsInStorage := c.checkRawModelExists(ctx, id)
		return &types.ModelInfo{
			ID:   id,
			Type: types.ModelTypeRaw,
			RawModel: &types.RawModel{
				Meta:            rawMeta,
				ExistsInStorage: existsInStorage,
			},
		}, nil
	}

	// Try trained model
	if trainedMeta, exists := c.metaStore.GetTrainedModel(id); exists {
		existsInStorage := c.checkTrainedModelExists(ctx, id)
		return &types.ModelInfo{
			ID:   id,
			Type: types.ModelTypeTrained,
			TrainedModel: &types.TrainedModel{
				Meta:            trainedMeta,
				ExistsInStorage: existsInStorage,
			},
		}, nil
	}

	return nil, fmt.Errorf("model %s not found", id)
}

// RegisterRawModel registers a new raw model in both metadata and storage.
func (c *catalog) RegisterRawModel(ctx context.Context, id uuid.UUID, metadata metastorage.RawModelMetadata, reader io.Reader, size int64, contentType string) error {
	// Store the model in object storage first
	if err := c.objStorage.StoreRawModel(ctx, id, reader, size, contentType); err != nil {
		return fmt.Errorf("failed to store raw model in object storage: %w", err)
	}

	// Register metadata
	metadata.ID = id
	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = time.Now()
	}
	if metadata.UpdatedAt.IsZero() {
		metadata.UpdatedAt = time.Now()
	}
	if err := c.metaStore.RegisterRawModel(id, metadata); err != nil {
		// Try to clean up storage if metadata registration fails
		_ = c.objStorage.DeleteRawModel(ctx, id)
		return fmt.Errorf("failed to register raw model metadata: %w", err)
	}

	return nil
}

// RegisterTrainedModel registers a new trained model in both metadata and storage.
func (c *catalog) RegisterTrainedModel(ctx context.Context, id uuid.UUID, metadata metastorage.TrainedModelMetadata, reader io.Reader, size int64, contentType string) error {
	// Store the model in object storage first
	if err := c.objStorage.StoreTrainedModel(ctx, id, reader, size, contentType); err != nil {
		return fmt.Errorf("failed to store trained model in object storage: %w", err)
	}

	// Register metadata
	metadata.ID = id
	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = time.Now()
	}
	if metadata.UpdatedAt.IsZero() {
		metadata.UpdatedAt = time.Now()
	}
	if err := c.metaStore.RegisterTrainedModel(id, metadata); err != nil {
		// Try to clean up storage if metadata registration fails
		_ = c.objStorage.DeleteTrainedModel(ctx, id)
		return fmt.Errorf("failed to register trained model metadata: %w", err)
	}

	return nil
}

// DeleteRawModel deletes a raw model from both metadata and storage.
func (c *catalog) DeleteRawModel(ctx context.Context, id uuid.UUID) error {
	// Delete from storage first
	if err := c.objStorage.DeleteRawModel(ctx, id); err != nil {
		return fmt.Errorf("failed to delete raw model from storage: %w", err)
	}

	// Then remove metadata
	if err := c.metaStore.UnregisterRawModel(id); err != nil {
		return fmt.Errorf("failed to unregister raw model metadata: %w", err)
	}

	return nil
}

// DeleteTrainedModel deletes a trained model from both metadata and storage.
func (c *catalog) DeleteTrainedModel(ctx context.Context, id uuid.UUID) error {
	// Delete from storage first
	if err := c.objStorage.DeleteTrainedModel(ctx, id); err != nil {
		return fmt.Errorf("failed to delete trained model from storage: %w", err)
	}

	// Then remove metadata
	if err := c.metaStore.UnregisterTrainedModel(id); err != nil {
		return fmt.Errorf("failed to unregister trained model metadata: %w", err)
	}

	return nil
}

// LoadRawModel loads a raw model from storage.
func (c *catalog) LoadRawModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	// Verify metadata exists
	if _, exists := c.metaStore.GetRawModel(id); !exists {
		return nil, fmt.Errorf("raw model %s not found in metadata", id)
	}

	return c.objStorage.LoadRawModel(ctx, id)
}

// LoadTrainedModel loads a trained model from storage.
func (c *catalog) LoadTrainedModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	// Verify metadata exists
	if _, exists := c.metaStore.GetTrainedModel(id); !exists {
		return nil, fmt.Errorf("trained model %s not found in metadata", id)
	}

	return c.objStorage.LoadTrainedModel(ctx, id)
}

// UpdateRawModelMetadata updates the metadata for a raw model.
func (c *catalog) UpdateRawModelMetadata(ctx context.Context, id uuid.UUID, updateFn func(metastorage.RawModelMetadata) metastorage.RawModelMetadata) error {
	_, err := c.metaStore.UpdateRawModel(id, func(meta metastorage.RawModelMetadata) metastorage.RawModelMetadata {
		updated := updateFn(meta)
		updated.UpdatedAt = time.Now()
		return updated
	})
	return err
}

// UpdateTrainedModelMetadata updates the metadata for a trained model.
func (c *catalog) UpdateTrainedModelMetadata(ctx context.Context, id uuid.UUID, updateFn func(metastorage.TrainedModelMetadata) metastorage.TrainedModelMetadata) error {
	_, err := c.metaStore.UpdateTrainedModel(id, func(meta metastorage.TrainedModelMetadata) metastorage.TrainedModelMetadata {
		updated := updateFn(meta)
		updated.UpdatedAt = time.Now()
		return updated
	})
	return err
}

// ModelExists checks if a model exists (either in metadata or storage).
func (c *catalog) ModelExists(ctx context.Context, id uuid.UUID) bool {
	// Check metadata first (faster)
	if _, exists := c.metaStore.GetRawModel(id); exists {
		return true
	}
	if _, exists := c.metaStore.GetTrainedModel(id); exists {
		return true
	}
	return false
}

// RawModelExists checks if a raw model exists in metadata.
func (c *catalog) RawModelExists(id uuid.UUID) bool {
	_, exists := c.metaStore.GetRawModel(id)
	return exists
}

// TrainedModelExists checks if a trained model exists in metadata.
func (c *catalog) TrainedModelExists(id uuid.UUID) bool {
	_, exists := c.metaStore.GetTrainedModel(id)
	return exists
}

// checkRawModelExists checks if a raw model exists in storage by attempting to load it.
func (c *catalog) checkRawModelExists(ctx context.Context, id uuid.UUID) bool {
	reader, err := c.objStorage.LoadRawModel(ctx, id)
	if err != nil {
		return false
	}
	if reader != nil {
		_ = reader.Close()
	}
	return true
}

// checkTrainedModelExists checks if a trained model exists in storage by attempting to load it.
func (c *catalog) checkTrainedModelExists(ctx context.Context, id uuid.UUID) bool {
	reader, err := c.objStorage.LoadTrainedModel(ctx, id)
	if err != nil {
		return false
	}
	if reader != nil {
		_ = reader.Close()
	}
	return true
}
