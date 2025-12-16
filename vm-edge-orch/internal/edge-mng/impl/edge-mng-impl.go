package impl

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	edgemng "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-mng"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/object-storage"
)

type edgeMng struct {
	metaStore  metastorage.MetaDataStore
	objStorage objectstorage.ObjectStorageService
}

// NewEdgeMng creates a new edge management implementation that combines
// metadata operations (MetaDataStore) with object storage operations (ObjectStorageService).
func NewEdgeMng(metaStore metastorage.MetaDataStore, objStorage objectstorage.ObjectStorageService) (edgemng.EdgeMng, error) {
	if metaStore == nil {
		return nil, fmt.Errorf("meta store is required")
	}
	if objStorage == nil {
		return nil, fmt.Errorf("object storage is required")
	}

	return &edgeMng{
		metaStore:  metaStore,
		objStorage: objStorage,
	}, nil
}

// Edge state management (metadata only)
func (e *edgeMng) GetEdgeState(_ context.Context, edgeID uuid.UUID) (metastorage.EdgeState, error) {
	state, ok := e.metaStore.GetEdge(edgeID)
	if !ok {
		return metastorage.EdgeState{}, fmt.Errorf("edge %s not found", edgeID)
	}
	return state, nil
}

func (e *edgeMng) UpdateEdgeState(_ context.Context, edgeID uuid.UUID, updateFn func(metastorage.EdgeState) metastorage.EdgeState) (metastorage.EdgeState, error) {
	return e.metaStore.UpdateEdge(edgeID, updateFn)
}

// IoT device management (metadata only)
func (e *edgeMng) ListDevices(_ context.Context, edgeID uuid.UUID) ([]metastorage.IoTDevice, error) {
	state, ok := e.metaStore.GetEdge(edgeID)
	if !ok {
		return nil, fmt.Errorf("edge %s not found", edgeID)
	}
	return append([]metastorage.IoTDevice(nil), state.Devices...), nil
}

func (e *edgeMng) GetDevice(_ context.Context, edgeID uuid.UUID, deviceID uuid.UUID) (metastorage.IoTDevice, error) {
	state, ok := e.metaStore.GetEdge(edgeID)
	if !ok {
		return metastorage.IoTDevice{}, fmt.Errorf("edge %s not found", edgeID)
	}

	for _, d := range state.Devices {
		if d.UUID == deviceID {
			return d, nil
		}
	}

	return metastorage.IoTDevice{}, fmt.Errorf("device %s not found for edge %s", deviceID, edgeID)
}

func (e *edgeMng) UpsertDevice(_ context.Context, edgeID uuid.UUID, device metastorage.IoTDevice) (metastorage.EdgeState, error) {
	updated, err := e.metaStore.UpdateEdge(edgeID, func(state metastorage.EdgeState) metastorage.EdgeState {
		replaced := false
		for i, d := range state.Devices {
			if d.UUID == device.UUID {
				state.Devices[i] = device
				replaced = true
				break
			}
		}
		if !replaced {
			state.Devices = append(state.Devices, device)
		}
		return state
	})
	if err != nil {
		return metastorage.EdgeState{}, err
	}
	return updated, nil
}

func (e *edgeMng) DeleteDevice(_ context.Context, edgeID uuid.UUID, deviceID uuid.UUID) (metastorage.EdgeState, error) {
	updated, err := e.metaStore.UpdateEdge(edgeID, func(state metastorage.EdgeState) metastorage.EdgeState {
		devices := state.Devices[:0]
		for _, d := range state.Devices {
			if d.UUID != deviceID {
				devices = append(devices, d)
			}
		}
		state.Devices = devices
		return state
	})
	if err != nil {
		return metastorage.EdgeState{}, err
	}
	return updated, nil
}

// Edge events (metadata only)
func (e *edgeMng) RegisterEdgeEvent(_ context.Context, id uuid.UUID, meta metastorage.EdgeEventMetadata) error {
	meta.ID = id
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now()
	}
	return e.metaStore.RegisterEdgeEvent(id, meta)
}

func (e *edgeMng) GetEdgeEvent(_ context.Context, id uuid.UUID) (metastorage.EdgeEventMetadata, error) {
	meta, ok := e.metaStore.GetEdgeEvent(id)
	if !ok {
		return metastorage.EdgeEventMetadata{}, fmt.Errorf("edge event %s not found", id)
	}
	return meta, nil
}

func (e *edgeMng) UpdateEdgeEvent(_ context.Context, id uuid.UUID, updateFn func(metastorage.EdgeEventMetadata) metastorage.EdgeEventMetadata) error {
	_, err := e.metaStore.UpdateEdgeEvent(id, func(meta metastorage.EdgeEventMetadata) metastorage.EdgeEventMetadata {
		updated := updateFn(meta)
		updated.UpdatedAt = time.Now()
		return updated
	})
	return err
}

func (e *edgeMng) DeleteEdgeEvent(_ context.Context, id uuid.UUID) error {
	return e.metaStore.UnregisterEdgeEvent(id)
}

// Clips (metadata + object storage)
func (e *edgeMng) RegisterClip(ctx context.Context, id uuid.UUID, meta metastorage.ClipMetadata, reader io.Reader, size int64, contentType string) error {
	// Store the clip in object storage first
	if err := e.objStorage.StoreClip(ctx, id, reader, size, contentType); err != nil {
		return fmt.Errorf("failed to store clip in object storage: %w", err)
	}

	// Register metadata
	meta.ID = id
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now()
	}
	if err := e.metaStore.RegisterClip(id, meta); err != nil {
		// Try to clean up storage if metadata registration fails
		_ = e.objStorage.DeleteClip(ctx, id)
		return fmt.Errorf("failed to register clip metadata: %w", err)
	}

	return nil
}

func (e *edgeMng) GetClip(_ context.Context, id uuid.UUID) (metastorage.ClipMetadata, error) {
	meta, ok := e.metaStore.GetClip(id)
	if !ok {
		return metastorage.ClipMetadata{}, fmt.Errorf("clip %s not found", id)
	}
	return meta, nil
}

func (e *edgeMng) LoadClip(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	// Verify metadata exists
	if _, exists := e.metaStore.GetClip(id); !exists {
		return nil, fmt.Errorf("clip %s not found in metadata", id)
	}

	return e.objStorage.LoadClip(ctx, id)
}

func (e *edgeMng) DeleteClip(ctx context.Context, id uuid.UUID) error {
	// Delete from storage first
	if err := e.objStorage.DeleteClip(ctx, id); err != nil {
		return fmt.Errorf("failed to delete clip from storage: %w", err)
	}

	// Then remove metadata
	if err := e.metaStore.UnregisterClip(id); err != nil {
		return fmt.Errorf("failed to unregister clip metadata: %w", err)
	}

	return nil
}

func (e *edgeMng) UpdateClipMetadata(_ context.Context, id uuid.UUID, updateFn func(metastorage.ClipMetadata) metastorage.ClipMetadata) error {
	_, err := e.metaStore.UpdateClip(id, func(meta metastorage.ClipMetadata) metastorage.ClipMetadata {
		updated := updateFn(meta)
		updated.UpdatedAt = time.Now()
		return updated
	})
	return err
}

func (e *edgeMng) ClipExists(_ context.Context, id uuid.UUID) bool {
	_, exists := e.metaStore.GetClip(id)
	return exists
}

// Training datasets (metadata + object storage)
func (e *edgeMng) RegisterTrainingDataset(ctx context.Context, id uuid.UUID, meta metastorage.TrainingDatasetMetadata, reader io.Reader, size int64, contentType string) error {
	// Store the dataset in object storage first
	if err := e.objStorage.StoreTrainingDataset(ctx, id, reader, size, contentType); err != nil {
		return fmt.Errorf("failed to store training dataset in object storage: %w", err)
	}

	// Register metadata
	meta.ID = id
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now()
	}
	if err := e.metaStore.RegisterTrainingDataset(id, meta); err != nil {
		// Try to clean up storage if metadata registration fails
		_ = e.objStorage.DeleteTrainingDataset(ctx, id)
		return fmt.Errorf("failed to register training dataset metadata: %w", err)
	}

	return nil
}

func (e *edgeMng) GetTrainingDataset(_ context.Context, id uuid.UUID) (metastorage.TrainingDatasetMetadata, error) {
	meta, ok := e.metaStore.GetTrainingDataset(id)
	if !ok {
		return metastorage.TrainingDatasetMetadata{}, fmt.Errorf("training dataset %s not found", id)
	}
	return meta, nil
}

func (e *edgeMng) LoadTrainingDataset(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	// Verify metadata exists
	if _, exists := e.metaStore.GetTrainingDataset(id); !exists {
		return nil, fmt.Errorf("training dataset %s not found in metadata", id)
	}

	return e.objStorage.LoadTrainingDataset(ctx, id)
}

func (e *edgeMng) DeleteTrainingDataset(ctx context.Context, id uuid.UUID) error {
	// Delete from storage first
	if err := e.objStorage.DeleteTrainingDataset(ctx, id); err != nil {
		return fmt.Errorf("failed to delete training dataset from storage: %w", err)
	}

	// Then remove metadata
	if err := e.metaStore.UnregisterTrainingDataset(id); err != nil {
		return fmt.Errorf("failed to unregister training dataset metadata: %w", err)
	}

	return nil
}

func (e *edgeMng) UpdateTrainingDatasetMetadata(_ context.Context, id uuid.UUID, updateFn func(metastorage.TrainingDatasetMetadata) metastorage.TrainingDatasetMetadata) error {
	_, err := e.metaStore.UpdateTrainingDataset(id, func(meta metastorage.TrainingDatasetMetadata) metastorage.TrainingDatasetMetadata {
		updated := updateFn(meta)
		updated.UpdatedAt = time.Now()
		return updated
	})
	return err
}

func (e *edgeMng) TrainingDatasetExists(_ context.Context, id uuid.UUID) bool {
	_, exists := e.metaStore.GetTrainingDataset(id)
	return exists
}
