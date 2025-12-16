package objectstorage

import (
	"context"
	"io"

	"github.com/google/uuid"

)

// ObjectStorageService provides high-level operations for storing and retrieving
// binary objects that correspond to metadata entities defined in MetaDataStore.
//
// Meta storage (see MetaDataStore) keeps lightweight records about:
//   - Raw models              -> RawModelMetadata
//   - Trained models          -> TrainedModelMetadata
//   - Clips / snapshots       -> ClipMetadata
//   - Training datasets       -> TrainingDatasetMetadata
//
// This service owns the actual binary payloads for those entities and maps them
// to stable object keys in S3/MinIO.
type ObjectStorageService interface {
	// Lifecycle hooks for orchestrator-managed services.
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string

	// Raw models (baseline models, not bound to a specific Edge)
	StoreRawModel(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error
	LoadRawModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)
	DeleteRawModel(ctx context.Context, id uuid.UUID) error

	// Trained models (produced for a specific IoT device on an Edge)
	StoreTrainedModel(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error
	LoadTrainedModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)
	DeleteTrainedModel(ctx context.Context, id uuid.UUID) error

	// Training datasets (IoT device training datasets)
	StoreTrainingDataset(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error
	LoadTrainingDataset(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)
	DeleteTrainingDataset(ctx context.Context, id uuid.UUID) error

	// Clips / snapshots (video clips or images stored for an event)
	StoreClip(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error
	LoadClip(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)
	DeleteClip(ctx context.Context, id uuid.UUID) error


	// MetaData returns the meta-store view that should stay consistent with stored objects.
	// Implementations are expected to keep MetaDataStore and object contents in sync
	// (e.g. when deleting or moving objects).
	// Note: concrete implementations wire this to MetaDataStore without creating
	// an import cycle in the orchestrator services package.
}
