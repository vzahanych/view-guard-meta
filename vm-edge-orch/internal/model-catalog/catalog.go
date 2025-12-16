package modelcatalog

import (
	"context"
	"io"

	"github.com/google/uuid"

	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
	"github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/model-catalog/types"
)

// Catalog provides a unified interface for managing and querying models
// by combining metadata operations (MetaDataStore) with object storage operations (ObjectStorageService).
type Catalog interface {
	// GetRawModel retrieves a raw model by ID, including metadata and storage status.
	GetRawModel(ctx context.Context, id uuid.UUID) (*types.RawModel, error)

	// GetTrainedModel retrieves a trained model by ID, including metadata and storage status.
	// If includeSource is true, also populates the SourceModel field.
	GetTrainedModel(ctx context.Context, id uuid.UUID, includeSource bool) (*types.TrainedModel, error)

	// GetModel retrieves a model (raw or trained) by ID.
	// The type is determined by checking both stores.
	GetModel(ctx context.Context, id uuid.UUID) (*types.ModelInfo, error)

	// RegisterRawModel registers a new raw model in both metadata and storage.
	RegisterRawModel(ctx context.Context, id uuid.UUID, metadata metastorage.RawModelMetadata, reader io.Reader, size int64, contentType string) error

	// RegisterTrainedModel registers a new trained model in both metadata and storage.
	RegisterTrainedModel(ctx context.Context, id uuid.UUID, metadata metastorage.TrainedModelMetadata, reader io.Reader, size int64, contentType string) error

	// DeleteRawModel deletes a raw model from both metadata and storage.
	DeleteRawModel(ctx context.Context, id uuid.UUID) error

	// DeleteTrainedModel deletes a trained model from both metadata and storage.
	DeleteTrainedModel(ctx context.Context, id uuid.UUID) error

	// LoadRawModel loads a raw model from storage.
	LoadRawModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)

	// LoadTrainedModel loads a trained model from storage.
	LoadTrainedModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)

	// UpdateRawModelMetadata updates the metadata for a raw model.
	UpdateRawModelMetadata(ctx context.Context, id uuid.UUID, updateFn func(metastorage.RawModelMetadata) metastorage.RawModelMetadata) error

	// UpdateTrainedModelMetadata updates the metadata for a trained model.
	UpdateTrainedModelMetadata(ctx context.Context, id uuid.UUID, updateFn func(metastorage.TrainedModelMetadata) metastorage.TrainedModelMetadata) error

	// ModelExists checks if a model exists (either in metadata or storage).
	ModelExists(ctx context.Context, id uuid.UUID) bool

	// RawModelExists checks if a raw model exists in metadata.
	RawModelExists(id uuid.UUID) bool

	// TrainedModelExists checks if a trained model exists in metadata.
	TrainedModelExists(id uuid.UUID) bool
}
