package edgemng

import (
	"context"
	"io"

	"github.com/google/uuid"

	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
)

// EdgeMng provides a unified interface for managing edge-related data on the VM side.
// It combines metadata operations (MetaDataStore) with object storage operations (ObjectStorageService)
// to serve as a single point of truth for working with edge data.
//
// This service handles:
//   - Edge state and lifecycle management (metadata only)
//   - IoT device management (metadata only)
//   - Edge events (metadata only)
//   - Clips/snapshots (metadata + object storage)
//   - Training datasets (metadata + object storage)
type EdgeMng interface {
	// Edge state management (metadata only)
	// GetEdgeState returns the current state of an edge.
	GetEdgeState(ctx context.Context, edgeID uuid.UUID) (metastorage.EdgeState, error)

	// UpdateEdgeState atomically updates an edge state using the provided function.
	UpdateEdgeState(ctx context.Context, edgeID uuid.UUID, updateFn func(metastorage.EdgeState) metastorage.EdgeState) (metastorage.EdgeState, error)

	// IoT device management (metadata only)
	// ListDevices returns all IoT devices attached to the given edge.
	ListDevices(ctx context.Context, edgeID uuid.UUID) ([]metastorage.IoTDevice, error)

	// GetDevice returns a single IoT device for the given edge and device ID.
	GetDevice(ctx context.Context, edgeID uuid.UUID, deviceID uuid.UUID) (metastorage.IoTDevice, error)

	// UpsertDevice creates or updates an IoT device for the given edge.
	UpsertDevice(ctx context.Context, edgeID uuid.UUID, device metastorage.IoTDevice) (metastorage.EdgeState, error)

	// DeleteDevice removes an IoT device from the given edge.
	DeleteDevice(ctx context.Context, edgeID uuid.UUID, deviceID uuid.UUID) (metastorage.EdgeState, error)

	// Edge events (metadata only)
	// RegisterEdgeEvent registers metadata for an event reported by an edge.
	RegisterEdgeEvent(ctx context.Context, id uuid.UUID, meta metastorage.EdgeEventMetadata) error

	// GetEdgeEvent returns metadata for a specific edge event.
	GetEdgeEvent(ctx context.Context, id uuid.UUID) (metastorage.EdgeEventMetadata, error)

	// UpdateEdgeEvent updates metadata for an edge event.
	UpdateEdgeEvent(ctx context.Context, id uuid.UUID, updateFn func(metastorage.EdgeEventMetadata) metastorage.EdgeEventMetadata) error

	// DeleteEdgeEvent deletes an edge event from metadata.
	DeleteEdgeEvent(ctx context.Context, id uuid.UUID) error

	// Clips (metadata + object storage)
	// RegisterClip registers a clip in both metadata and object storage.
	RegisterClip(ctx context.Context, id uuid.UUID, meta metastorage.ClipMetadata, reader io.Reader, size int64, contentType string) error

	// GetClip returns metadata for a specific clip.
	GetClip(ctx context.Context, id uuid.UUID) (metastorage.ClipMetadata, error)

	// LoadClip loads a clip from object storage.
	LoadClip(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)

	// DeleteClip deletes a clip from both metadata and object storage.
	DeleteClip(ctx context.Context, id uuid.UUID) error

	// UpdateClipMetadata updates the metadata for a clip.
	UpdateClipMetadata(ctx context.Context, id uuid.UUID, updateFn func(metastorage.ClipMetadata) metastorage.ClipMetadata) error

	// ClipExists checks if a clip exists in metadata.
	ClipExists(ctx context.Context, id uuid.UUID) bool

	// Training datasets (metadata + object storage)
	// RegisterTrainingDataset registers a training dataset in both metadata and object storage.
	RegisterTrainingDataset(ctx context.Context, id uuid.UUID, meta metastorage.TrainingDatasetMetadata, reader io.Reader, size int64, contentType string) error

	// GetTrainingDataset returns metadata for a specific training dataset.
	GetTrainingDataset(ctx context.Context, id uuid.UUID) (metastorage.TrainingDatasetMetadata, error)

	// LoadTrainingDataset loads a training dataset from object storage.
	LoadTrainingDataset(ctx context.Context, id uuid.UUID) (io.ReadCloser, error)

	// DeleteTrainingDataset deletes a training dataset from both metadata and object storage.
	DeleteTrainingDataset(ctx context.Context, id uuid.UUID) error

	// UpdateTrainingDatasetMetadata updates the metadata for a training dataset.
	UpdateTrainingDatasetMetadata(ctx context.Context, id uuid.UUID, updateFn func(metastorage.TrainingDatasetMetadata) metastorage.TrainingDatasetMetadata) error

	// TrainingDatasetExists checks if a training dataset exists in metadata.
	TrainingDatasetExists(ctx context.Context, id uuid.UUID) bool
}
