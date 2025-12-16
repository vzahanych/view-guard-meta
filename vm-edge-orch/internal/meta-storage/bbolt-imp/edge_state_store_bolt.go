package bboltimp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/config"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
)

type BoltEdgeStateStore struct {
	mu     sync.RWMutex
	db     *bbolt.DB
	path   string
	logger *zap.Logger
}

const (
	edgeStateBucketName       = "edge_state"
	rawModelBucketName        = "raw_model_metadata"
	trainedModelBucketName    = "trained_model_metadata"
	edgeEventBucketName       = "edge_event_metadata"
	clipBucketName            = "clip_metadata"
	trainingDatasetBucketName = "training_dataset_metadata"
)

// BoltEdgeStateStoreConfig contains configuration for BoltEdgeStateStore.
type BoltEdgeStateStoreConfig struct {
	// Path is the directory where the BoltDB file will be stored.
	Path string
	// FileName is the database file name. If empty, a default name is used.
	FileName string
	// FileMode controls filesystem permissions for the DB file.
	// If zero, 0o600 is used.
	FileMode os.FileMode
}

// NewBoltEdgeStateStore creates a new BoltEdgeStateStore.
func NewBoltEdgeStateStore(logger *zap.Logger, cfg BoltEdgeStateStoreConfig) (*BoltEdgeStateStore, error) {
	if cfg.Path == "" {
		return nil, errors.New("bolt edge state store path is required")
	}

	if cfg.FileName == "" {
		cfg.FileName = "edge_state.db"
	}

	if cfg.FileMode == 0 {
		cfg.FileMode = 0o600
	}

	if err := os.MkdirAll(cfg.Path, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create bolt store directory: %w", err)
	}

	dbPath := filepath.Join(cfg.Path, cfg.FileName)

	db, err := bbolt.Open(dbPath, cfg.FileMode, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt edge state db: %w", err)
	}

	store := &BoltEdgeStateStore{
		db:     db,
		path:   dbPath,
		logger: logger,
	}

	if err := store.initBuckets(); err != nil {
		if closeErr := db.Close(); closeErr != nil && logger != nil {
			logger.Warn("failed to close bolt edge state db after init error", zap.Error(closeErr))
		}
		return nil, err
	}

	return store, nil
}

func (s *BoltEdgeStateStore) initBuckets() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(edgeStateBucketName)); err != nil {
			return fmt.Errorf("failed to create edge_state bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(rawModelBucketName)); err != nil {
			return fmt.Errorf("failed to create raw_model_metadata bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(trainedModelBucketName)); err != nil {
			return fmt.Errorf("failed to create trained_model_metadata bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(edgeEventBucketName)); err != nil {
			return fmt.Errorf("failed to create edge_event_metadata bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(clipBucketName)); err != nil {
			return fmt.Errorf("failed to create clip_metadata bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(trainingDatasetBucketName)); err != nil {
			return fmt.Errorf("failed to create training_dataset_metadata bucket: %w", err)
		}
		return nil
	})
}

// Init initializes the store from orchestrator config.
// For BoltEdgeStateStore the DB is created in the constructor, so Init is a no-op.
func (s *BoltEdgeStateStore) Init(cfg *config.Config) error {
	return nil
}

// Name returns the service name.
func (s *BoltEdgeStateStore) Name() string {
	return "edge-state-store-bolt"
}

// Start marks the service as running. The DB is already opened in the constructor.
func (s *BoltEdgeStateStore) Start(ctx context.Context) error {
	return nil
}

// Stop closes the underlying BoltDB file.
func (s *BoltEdgeStateStore) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close bolt edge state db: %w", err)
		}
		s.db = nil
	}

	return nil
}

// Shutdown implements the orchestrator lifecycle interface.
func (s *BoltEdgeStateStore) Shutdown(ctx context.Context) error {
	return s.Stop(ctx)
}

// RegisterEdge stores a new edge state.
func (s *BoltEdgeStateStore) RegisterEdge(id uuid.UUID, state metastorage.EdgeState) error {
	if id == uuid.Nil {
		return fmt.Errorf("uuid is required")
	}

	state.UUID = id
	now := time.Now().UTC()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now

	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal edge state: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeStateBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeStateBucketName)
		}

		if err := b.Put([]byte(id.String()), raw); err != nil {
			return fmt.Errorf("failed to store edge state: %w", err)
		}
		s.logState("register", state)
		return nil
	})
}

// UnregisterEdge removes an edge state.
func (s *BoltEdgeStateStore) UnregisterEdge(id uuid.UUID) error {
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeStateBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeStateBucketName)
		}

		if err := b.Delete([]byte(id.String())); err != nil {
			return fmt.Errorf("failed to delete edge state: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	s.logger.Info("Edge unregistered", zap.String("uuid", id.String()))
	return nil
}

// GetEdge returns an edge state if it exists.
func (s *BoltEdgeStateStore) GetEdge(id uuid.UUID) (metastorage.EdgeState, bool) {
	var result metastorage.EdgeState
	found := false

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeStateBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeStateBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return nil
		}

		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("failed to unmarshal edge state: %w", err)
		}
		found = true
		return nil
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to get edge state", zap.Error(err))
		}
		return metastorage.EdgeState{}, false
	}

	return result, found
}

// UpdateEdge performs a read-modify-write transaction on an existing edge state.
func (s *BoltEdgeStateStore) UpdateEdge(id uuid.UUID, updateFn func(metastorage.EdgeState) metastorage.EdgeState) (metastorage.EdgeState, error) {
	if id == uuid.Nil {
		return metastorage.EdgeState{}, fmt.Errorf("uuid is required")
	}

	var updated metastorage.EdgeState

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeStateBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeStateBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return fmt.Errorf("edge %s not found", id.String())
		}

		var current metastorage.EdgeState
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("failed to unmarshal edge state: %w", err)
		}

		updated = updateFn(current)
		updated.UUID = id
		updated.UpdatedAt = time.Now().UTC()

		newRaw, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal updated edge state: %w", err)
		}

		if err := b.Put([]byte(id.String()), newRaw); err != nil {
			return fmt.Errorf("failed to store updated edge state: %w", err)
		}

		return nil
	})

	if err != nil {
		return metastorage.EdgeState{}, err
	}

	s.logState("update", updated)
	return updated, nil
}

func (s *BoltEdgeStateStore) logState(action string, state metastorage.EdgeState) {
	if s.logger == nil {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	s.logger.Info("Edge state change",
		zap.String("action", action),
		zap.String("uuid", state.UUID.String()),
		zap.ByteString("state", raw),
	)
}

// RegisterRawModel stores metadata for a raw (baseline) model.
func (s *BoltEdgeStateStore) RegisterRawModel(id uuid.UUID, meta metastorage.RawModelMetadata) error {
	if id == uuid.Nil {
		return fmt.Errorf("uuid is required")
	}

	meta.ID = id
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal raw model metadata: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(rawModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", rawModelBucketName)
		}
		if err := b.Put([]byte(id.String()), raw); err != nil {
			return fmt.Errorf("failed to store raw model metadata: %w", err)
		}
		return nil
	})
}

// UnregisterRawModel removes raw model metadata.
func (s *BoltEdgeStateStore) UnregisterRawModel(id uuid.UUID) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(rawModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", rawModelBucketName)
		}
		if err := b.Delete([]byte(id.String())); err != nil {
			return fmt.Errorf("failed to delete raw model metadata: %w", err)
		}
		return nil
	})
}

// GetRawModel returns raw model metadata if it exists.
func (s *BoltEdgeStateStore) GetRawModel(id uuid.UUID) (metastorage.RawModelMetadata, bool) {
	var result metastorage.RawModelMetadata
	found := false

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(rawModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", rawModelBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return nil
		}

		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("failed to unmarshal raw model metadata: %w", err)
		}
		found = true
		return nil
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to get raw model metadata", zap.Error(err))
		}
		return metastorage.RawModelMetadata{}, false
	}

	return result, found
}

// UpdateRawModel performs a read-modify-write transaction on raw model metadata.
func (s *BoltEdgeStateStore) UpdateRawModel(id uuid.UUID, updateFn func(metastorage.RawModelMetadata) metastorage.RawModelMetadata) (metastorage.RawModelMetadata, error) {
	if id == uuid.Nil {
		return metastorage.RawModelMetadata{}, fmt.Errorf("uuid is required")
	}

	var updated metastorage.RawModelMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(rawModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", rawModelBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return fmt.Errorf("raw model %s not found", id.String())
		}

		var current metastorage.RawModelMetadata
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("failed to unmarshal raw model metadata: %w", err)
		}

		updated = updateFn(current)
		updated.ID = id
		updated.UpdatedAt = time.Now().UTC()

		newRaw, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal updated raw model metadata: %w", err)
		}

		if err := b.Put([]byte(id.String()), newRaw); err != nil {
			return fmt.Errorf("failed to store updated raw model metadata: %w", err)
		}

		return nil
	})

	if err != nil {
		return metastorage.RawModelMetadata{}, err
	}

	return updated, nil
}

// RegisterTrainedModel stores metadata for a trained model.
func (s *BoltEdgeStateStore) RegisterTrainedModel(id uuid.UUID, meta metastorage.TrainedModelMetadata) error {
	if id == uuid.Nil {
		return fmt.Errorf("uuid is required")
	}

	meta.ID = id
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal trained model metadata: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainedModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainedModelBucketName)
		}
		if err := b.Put([]byte(id.String()), raw); err != nil {
			return fmt.Errorf("failed to store trained model metadata: %w", err)
		}
		return nil
	})
}

// UnregisterTrainedModel removes trained model metadata.
func (s *BoltEdgeStateStore) UnregisterTrainedModel(id uuid.UUID) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainedModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainedModelBucketName)
		}
		if err := b.Delete([]byte(id.String())); err != nil {
			return fmt.Errorf("failed to delete trained model metadata: %w", err)
		}
		return nil
	})
}

// GetTrainedModel returns trained model metadata if it exists.
func (s *BoltEdgeStateStore) GetTrainedModel(id uuid.UUID) (metastorage.TrainedModelMetadata, bool) {
	var result metastorage.TrainedModelMetadata
	found := false

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainedModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainedModelBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return nil
		}

		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("failed to unmarshal trained model metadata: %w", err)
		}
		found = true
		return nil
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to get trained model metadata", zap.Error(err))
		}
		return metastorage.TrainedModelMetadata{}, false
	}

	return result, found
}

// UpdateTrainedModel performs a read-modify-write transaction on trained model metadata.
func (s *BoltEdgeStateStore) UpdateTrainedModel(id uuid.UUID, updateFn func(metastorage.TrainedModelMetadata) metastorage.TrainedModelMetadata) (metastorage.TrainedModelMetadata, error) {
	if id == uuid.Nil {
		return metastorage.TrainedModelMetadata{}, fmt.Errorf("uuid is required")
	}

	var updated metastorage.TrainedModelMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainedModelBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainedModelBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return fmt.Errorf("trained model %s not found", id.String())
		}

		var current metastorage.TrainedModelMetadata
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("failed to unmarshal trained model metadata: %w", err)
		}

		updated = updateFn(current)
		updated.ID = id
		updated.UpdatedAt = time.Now().UTC()

		newRaw, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal updated trained model metadata: %w", err)
		}

		if err := b.Put([]byte(id.String()), newRaw); err != nil {
			return fmt.Errorf("failed to store updated trained model metadata: %w", err)
		}

		return nil
	})

	if err != nil {
		return metastorage.TrainedModelMetadata{}, err
	}

	return updated, nil
}

// RegisterEdgeEvent stores metadata for an edge event.
func (s *BoltEdgeStateStore) RegisterEdgeEvent(id uuid.UUID, meta metastorage.EdgeEventMetadata) error {
	if id == uuid.Nil {
		return fmt.Errorf("uuid is required")
	}

	meta.ID = id
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal edge event metadata: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeEventBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeEventBucketName)
		}
		if err := b.Put([]byte(id.String()), raw); err != nil {
			return fmt.Errorf("failed to store edge event metadata: %w", err)
		}
		return nil
	})
}

// UnregisterEdgeEvent removes edge event metadata.
func (s *BoltEdgeStateStore) UnregisterEdgeEvent(id uuid.UUID) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeEventBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeEventBucketName)
		}
		if err := b.Delete([]byte(id.String())); err != nil {
			return fmt.Errorf("failed to delete edge event metadata: %w", err)
		}
		return nil
	})
}

// GetEdgeEvent returns edge event metadata if it exists.
func (s *BoltEdgeStateStore) GetEdgeEvent(id uuid.UUID) (metastorage.EdgeEventMetadata, bool) {
	var result metastorage.EdgeEventMetadata
	found := false

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeEventBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeEventBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return nil
		}

		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("failed to unmarshal edge event metadata: %w", err)
		}
		found = true
		return nil
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to get edge event metadata", zap.Error(err))
		}
		return metastorage.EdgeEventMetadata{}, false
	}

	return result, found
}

// UpdateEdgeEvent performs a read-modify-write transaction on edge event metadata.
func (s *BoltEdgeStateStore) UpdateEdgeEvent(id uuid.UUID, updateFn func(metastorage.EdgeEventMetadata) metastorage.EdgeEventMetadata) (metastorage.EdgeEventMetadata, error) {
	if id == uuid.Nil {
		return metastorage.EdgeEventMetadata{}, fmt.Errorf("uuid is required")
	}

	var updated metastorage.EdgeEventMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeEventBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeEventBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return fmt.Errorf("edge event %s not found", id.String())
		}

		var current metastorage.EdgeEventMetadata
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("failed to unmarshal edge event metadata: %w", err)
		}

		updated = updateFn(current)
		updated.ID = id
		updated.UpdatedAt = time.Now().UTC()

		newRaw, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal updated edge event metadata: %w", err)
		}

		if err := b.Put([]byte(id.String()), newRaw); err != nil {
			return fmt.Errorf("failed to store updated edge event metadata: %w", err)
		}

		return nil
	})

	if err != nil {
		return metastorage.EdgeEventMetadata{}, err
	}

	return updated, nil
}

// RegisterClip stores metadata for a clip.
func (s *BoltEdgeStateStore) RegisterClip(id uuid.UUID, meta metastorage.ClipMetadata) error {
	if id == uuid.Nil {
		return fmt.Errorf("uuid is required")
	}

	meta.ID = id
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal clip metadata: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipBucketName)
		}
		if err := b.Put([]byte(id.String()), raw); err != nil {
			return fmt.Errorf("failed to store clip metadata: %w", err)
		}
		return nil
	})
}

// UnregisterClip removes clip metadata.
func (s *BoltEdgeStateStore) UnregisterClip(id uuid.UUID) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipBucketName)
		}
		if err := b.Delete([]byte(id.String())); err != nil {
			return fmt.Errorf("failed to delete clip metadata: %w", err)
		}
		return nil
	})
}

// GetClip returns clip metadata if it exists.
func (s *BoltEdgeStateStore) GetClip(id uuid.UUID) (metastorage.ClipMetadata, bool) {
	var result metastorage.ClipMetadata
	found := false

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return nil
		}

		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("failed to unmarshal clip metadata: %w", err)
		}
		found = true
		return nil
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to get clip metadata", zap.Error(err))
		}
		return metastorage.ClipMetadata{}, false
	}

	return result, found
}

// UpdateClip performs a read-modify-write transaction on clip metadata.
func (s *BoltEdgeStateStore) UpdateClip(id uuid.UUID, updateFn func(metastorage.ClipMetadata) metastorage.ClipMetadata) (metastorage.ClipMetadata, error) {
	if id == uuid.Nil {
		return metastorage.ClipMetadata{}, fmt.Errorf("uuid is required")
	}

	var updated metastorage.ClipMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return fmt.Errorf("clip %s not found", id.String())
		}

		var current metastorage.ClipMetadata
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("failed to unmarshal clip metadata: %w", err)
		}

		updated = updateFn(current)
		updated.ID = id
		updated.UpdatedAt = time.Now().UTC()

		newRaw, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal updated clip metadata: %w", err)
		}

		if err := b.Put([]byte(id.String()), newRaw); err != nil {
			return fmt.Errorf("failed to store updated clip metadata: %w", err)
		}

		return nil
	})

	if err != nil {
		return metastorage.ClipMetadata{}, err
	}

	return updated, nil
}

// RegisterTrainingDataset stores metadata for a training dataset.
func (s *BoltEdgeStateStore) RegisterTrainingDataset(id uuid.UUID, meta metastorage.TrainingDatasetMetadata) error {
	if id == uuid.Nil {
		return fmt.Errorf("uuid is required")
	}

	meta.ID = id
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal training dataset metadata: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainingDatasetBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainingDatasetBucketName)
		}
		if err := b.Put([]byte(id.String()), raw); err != nil {
			return fmt.Errorf("failed to store training dataset metadata: %w", err)
		}
		return nil
	})
}

// UnregisterTrainingDataset removes training dataset metadata.
func (s *BoltEdgeStateStore) UnregisterTrainingDataset(id uuid.UUID) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainingDatasetBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainingDatasetBucketName)
		}
		if err := b.Delete([]byte(id.String())); err != nil {
			return fmt.Errorf("failed to delete training dataset metadata: %w", err)
		}
		return nil
	})
}

// GetTrainingDataset returns training dataset metadata if it exists.
func (s *BoltEdgeStateStore) GetTrainingDataset(id uuid.UUID) (metastorage.TrainingDatasetMetadata, bool) {
	var result metastorage.TrainingDatasetMetadata
	found := false

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainingDatasetBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainingDatasetBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return nil
		}

		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("failed to unmarshal training dataset metadata: %w", err)
		}
		found = true
		return nil
	})

	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to get training dataset metadata", zap.Error(err))
		}
		return metastorage.TrainingDatasetMetadata{}, false
	}

	return result, found
}

// UpdateTrainingDataset performs a read-modify-write transaction on training dataset metadata.
func (s *BoltEdgeStateStore) UpdateTrainingDataset(id uuid.UUID, updateFn func(metastorage.TrainingDatasetMetadata) metastorage.TrainingDatasetMetadata) (metastorage.TrainingDatasetMetadata, error) {
	if id == uuid.Nil {
		return metastorage.TrainingDatasetMetadata{}, fmt.Errorf("uuid is required")
	}

	var updated metastorage.TrainingDatasetMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(trainingDatasetBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", trainingDatasetBucketName)
		}

		raw := b.Get([]byte(id.String()))
		if raw == nil {
			return fmt.Errorf("training dataset %s not found", id.String())
		}

		var current metastorage.TrainingDatasetMetadata
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("failed to unmarshal training dataset metadata: %w", err)
		}

		updated = updateFn(current)
		updated.ID = id
		updated.UpdatedAt = time.Now().UTC()

		newRaw, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("failed to marshal updated training dataset metadata: %w", err)
		}

		if err := b.Put([]byte(id.String()), newRaw); err != nil {
			return fmt.Errorf("failed to store updated training dataset metadata: %w", err)
		}

		return nil
	})

	if err != nil {
		return metastorage.TrainingDatasetMetadata{}, err
	}

	return updated, nil
}
