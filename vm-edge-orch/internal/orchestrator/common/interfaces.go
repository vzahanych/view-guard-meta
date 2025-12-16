package common

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

)

// RawModelMetadata describes metadata for a raw (baseline) model that is not bound to a specific Edge.
type RawModelMetadata struct {
	ID        uuid.UUID         `json:"id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// TrainedModelMetadata describes metadata for a trained model produced for a specific IoT device on an Edge.
type TrainedModelMetadata struct {
	ID            uuid.UUID         `json:"id"`
	EdgeID        uuid.UUID         `json:"edge_id"`
	DeviceID      uuid.UUID         `json:"device_id"`
	SourceModelID uuid.UUID         `json:"source_model_id"` // Raw/baseline model this was trained from
	DatasetID     uuid.UUID         `json:"dataset_id"`      // Training dataset used
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// MetaDataStore defines KV-style operations for model metadata.
// This interface focuses on model-related operations that the catalog needs.
type MetaDataStore interface {
	// Raw model metadata
	RegisterRawModel(id uuid.UUID, meta RawModelMetadata) error
	UnregisterRawModel(id uuid.UUID) error
	GetRawModel(id uuid.UUID) (RawModelMetadata, bool)
	UpdateRawModel(id uuid.UUID, updateFn func(RawModelMetadata) RawModelMetadata) (RawModelMetadata, error)

	// Trained model metadata
	RegisterTrainedModel(id uuid.UUID, meta TrainedModelMetadata) error
	UnregisterTrainedModel(id uuid.UUID) error
	GetTrainedModel(id uuid.UUID) (TrainedModelMetadata, bool)
	UpdateTrainedModel(id uuid.UUID, updateFn func(TrainedModelMetadata) TrainedModelMetadata) (TrainedModelMetadata, error)
}

// ObjectStorageService provides high-level operations for storing and retrieving
// binary objects for models.
//
// This service owns the actual binary payloads for model entities and maps them
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

}

