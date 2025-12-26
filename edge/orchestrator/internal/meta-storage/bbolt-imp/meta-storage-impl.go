package bboltimp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

const (
	storageStateBucket            = "storage_state"
	deployedModelsBucket          = "deployed_models"
	camerasBucket                 = "cameras"
	screenshotsBucket             = "screenshots"
	clipsBucket                   = "clips"
	securityEventsBucket          = "security_events"
	eventQueueBucket              = "event_queue"
	eventsBucket                  = "events"
	deadLetterEventsBucket        = "dead_letter_events"
	pendingSnapshotRequestsBucket = "pending_snapshot_requests"
	edgeStateBucket               = "edge_state"
	edgeStateHistoryBucket        = "edge_state_history"
	edgeCapabilitiesBucket        = "edge_capabilities"
	currentEdgeStateKey           = "current"
	currentEdgeCapabilitiesKey    = "current"
)

// BboltMetaStorage implements MetaDataStore using BoltDB
type BboltMetaStorage struct {
	db     *bbolt.DB
	logger *zap.Logger
}

// NewBboltMetaDataStore creates a new metadata store implementation
func NewBboltMetaDataStore(ctx context.Context, cfg *types.MetaStorageConfig, log *zap.Logger) (*BboltMetaStorage, error) {
	dbPath := filepath.Join(cfg.DataDir, "db", "meta.db")
	
	// Ensure parent directories exist (bbolt.Open does not create them)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}
	
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open meta-storage database: %w", err)
	}

	store := &BboltMetaStorage{
		db:     db,
		logger: log,
	}

	// Initialize buckets
	if err := db.Update(func(tx *bbolt.Tx) error {
		buckets := []string{
			storageStateBucket,
			deployedModelsBucket,
			camerasBucket,
			screenshotsBucket,
			clipsBucket,
			securityEventsBucket,
			eventQueueBucket,
			eventsBucket,
			deadLetterEventsBucket,
			pendingSnapshotRequestsBucket,
			edgeStateBucket,
			edgeStateHistoryBucket,
			edgeCapabilitiesBucket,
		}
		for _, bucketName := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
				return fmt.Errorf("failed to create %s bucket: %w", bucketName, err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return store, nil
}

// SaveStorageEntry saves a storage entry metadata to the database
func (s *BboltMetaStorage) SaveStorageEntry(ctx context.Context, entry types.StorageEntryMetadata) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal storage entry: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storageStateBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", storageStateBucket)
		}
		return b.Put([]byte(entry.Path), data)
	})
}

// DeleteStorageEntry deletes a storage entry metadata from the database
func (s *BboltMetaStorage) DeleteStorageEntry(ctx context.Context, path string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storageStateBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", storageStateBucket)
		}
		return b.Delete([]byte(path))
	})
}

// ListStorageEntries lists storage entry metadata by file type
func (s *BboltMetaStorage) ListStorageEntries(ctx context.Context, fileType string) ([]types.StorageEntryMetadata, error) {
	var entries []types.StorageEntryMetadata

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storageStateBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", storageStateBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var entry types.StorageEntryMetadata
			if err := json.Unmarshal(v, &entry); err != nil {
				s.logger.Warn("Failed to unmarshal storage entry", zap.String("path", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}
			if fileType == "" || entry.FileType == fileType {
				entries = append(entries, entry)
			}
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list storage entries: %w", err)
	}

	// Sort by created_at ascending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})

	return entries, nil
}

// GetStorageStats returns storage statistics
func (s *BboltMetaStorage) GetStorageStats(ctx context.Context) (*types.StorageStats, error) {
	stats := &types.StorageStats{
		TotalClips:     0,
		TotalSnapshots: 0,
		TotalSizeBytes: 0,
	}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(storageStateBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", storageStateBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var entry types.StorageEntryMetadata
			if err := json.Unmarshal(v, &entry); err != nil {
				return nil // Skip invalid entries
			}

			if entry.FileType == "clip" {
				stats.TotalClips++
			} else if entry.FileType == "snapshot" {
				stats.TotalSnapshots++
			}
			stats.TotalSizeBytes += entry.SizeBytes
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}

	// Note: DiskUsagePercent and AvailableBytes would need disk monitor
	// For now, we'll return 0 and let the caller combine with disk monitor
	return stats, nil
}

// SaveDeployedModel saves deployed model metadata to the database
func (s *BboltMetaStorage) SaveDeployedModel(ctx context.Context, model types.DeployedModelMetadata) error {
	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("failed to marshal deployed model: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(deployedModelsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", deployedModelsBucket)
		}
		return b.Put([]byte(model.ModelID), data)
	})
}

// UpdateDeployedModel updates deployed model metadata
func (s *BboltMetaStorage) UpdateDeployedModel(ctx context.Context, modelID string, updateFn func(types.DeployedModelMetadata) types.DeployedModelMetadata) (types.DeployedModelMetadata, error) {
	var updatedModel types.DeployedModelMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(deployedModelsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", deployedModelsBucket)
		}

		data := b.Get([]byte(modelID))
		if data == nil {
			return fmt.Errorf("model not found: %s", modelID)
		}

		var model types.DeployedModelMetadata
		if err := json.Unmarshal(data, &model); err != nil {
			return fmt.Errorf("failed to unmarshal model: %w", err)
		}

		updatedModel = updateFn(model)

		updatedData, err := json.Marshal(updatedModel)
		if err != nil {
			return fmt.Errorf("failed to marshal updated model: %w", err)
		}

		return b.Put([]byte(modelID), updatedData)
	})

	if err != nil {
		return types.DeployedModelMetadata{}, err
	}

	return updatedModel, nil
}

// GetDeployedModel retrieves deployed model metadata by ID
func (s *BboltMetaStorage) GetDeployedModel(ctx context.Context, modelID string) (types.DeployedModelMetadata, bool) {
	var model types.DeployedModelMetadata
	var found bool

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(deployedModelsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", deployedModelsBucket)
		}

		data := b.Get([]byte(modelID))
		if data == nil {
			return nil // Not found
		}

		found = true
		return json.Unmarshal(data, &model)
	})

	if err != nil || !found {
		return types.DeployedModelMetadata{}, false
	}

	return model, true
}

// ListDeployedModels lists deployed model metadata with optional filters
func (s *BboltMetaStorage) ListDeployedModels(ctx context.Context, filters *types.ModelFilters) ([]types.DeployedModelMetadata, error) {
	var models []types.DeployedModelMetadata

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(deployedModelsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", deployedModelsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var model types.DeployedModelMetadata
			if err := json.Unmarshal(v, &model); err != nil {
				s.logger.Warn("Failed to unmarshal model", zap.String("model_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Apply filters
			if filters != nil {
				if filters.EdgeID != nil && model.EdgeID != *filters.EdgeID {
					return nil
				}
				if filters.CameraID != nil && (model.CameraID == nil || *model.CameraID != *filters.CameraID) {
					return nil
				}
				if filters.Status != nil && model.Status != *filters.Status {
					return nil
				}
			}

			models = append(models, model)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	// Sort by deployed_at descending
	sort.Slice(models, func(i, j int) bool {
		return models[i].DeployedAt.After(models[j].DeployedAt)
	})

	return models, nil
}

// DeleteDeployedModel deletes deployed model metadata from the database
func (s *BboltMetaStorage) DeleteDeployedModel(ctx context.Context, modelID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(deployedModelsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", deployedModelsBucket)
		}
		return b.Delete([]byte(modelID))
	})
}

// Camera metadata methods

// SaveCamera saves camera metadata to the database
func (s *BboltMetaStorage) SaveCamera(ctx context.Context, camera types.CameraMetadata) error {
	camera.UpdatedAt = time.Now()
	if camera.CreatedAt.IsZero() {
		camera.CreatedAt = time.Now()
	}
	data, err := json.Marshal(camera)
	if err != nil {
		return fmt.Errorf("failed to marshal camera: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(camerasBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", camerasBucket)
		}
		return b.Put([]byte(camera.ID), data)
	})
}

// UpdateCamera updates camera metadata
func (s *BboltMetaStorage) UpdateCamera(ctx context.Context, cameraID string, updateFn func(types.CameraMetadata) types.CameraMetadata) (types.CameraMetadata, error) {
	var updatedCamera types.CameraMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(camerasBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", camerasBucket)
		}

		data := b.Get([]byte(cameraID))
		if data == nil {
			return fmt.Errorf("camera not found: %s", cameraID)
		}

		var camera types.CameraMetadata
		if err := json.Unmarshal(data, &camera); err != nil {
			return fmt.Errorf("failed to unmarshal camera: %w", err)
		}

		updatedCamera = updateFn(camera)
		updatedCamera.UpdatedAt = time.Now()

		updatedData, err := json.Marshal(updatedCamera)
		if err != nil {
			return fmt.Errorf("failed to marshal updated camera: %w", err)
		}

		return b.Put([]byte(cameraID), updatedData)
	})

	if err != nil {
		return types.CameraMetadata{}, err
	}

	return updatedCamera, nil
}

// GetCamera retrieves camera metadata by ID
func (s *BboltMetaStorage) GetCamera(ctx context.Context, cameraID string) (types.CameraMetadata, bool) {
	var camera types.CameraMetadata
	var found bool

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(camerasBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", camerasBucket)
		}

		data := b.Get([]byte(cameraID))
		if data == nil {
			return nil // Not found
		}

		found = true
		return json.Unmarshal(data, &camera)
	})

	if err != nil || !found {
		return types.CameraMetadata{}, false
	}

	return camera, true
}

// ListCameras lists camera metadata with optional filters
func (s *BboltMetaStorage) ListCameras(ctx context.Context, filters *types.CameraFilters) ([]types.CameraMetadata, error) {
	var cameras []types.CameraMetadata

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(camerasBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", camerasBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var camera types.CameraMetadata
			if err := json.Unmarshal(v, &camera); err != nil {
				s.logger.Warn("Failed to unmarshal camera", zap.String("camera_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Apply filters
			if filters != nil {
				if filters.EnabledOnly != nil && *filters.EnabledOnly && !camera.Enabled {
					return nil
				}
				if filters.Status != nil && camera.Status != *filters.Status {
					return nil
				}
				if filters.SyncedWithVM != nil && camera.SyncedWithVM != *filters.SyncedWithVM {
					return nil
				}
				if filters.Type != nil && camera.Type != *filters.Type {
					return nil
				}
			}

			cameras = append(cameras, camera)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list cameras: %w", err)
	}

	// Sort by discovered_at descending
	sort.Slice(cameras, func(i, j int) bool {
		return cameras[i].DiscoveredAt.After(cameras[j].DiscoveredAt)
	})

	return cameras, nil
}

// DeleteCamera deletes camera metadata from the database
func (s *BboltMetaStorage) DeleteCamera(ctx context.Context, cameraID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(camerasBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", camerasBucket)
		}
		return b.Delete([]byte(cameraID))
	})
}

// Screenshot metadata methods

// SaveScreenshot saves screenshot metadata to the database
func (s *BboltMetaStorage) SaveScreenshot(ctx context.Context, screenshot types.ScreenshotMetadata) error {
	screenshot.UpdatedAt = time.Now()
	if screenshot.CreatedAt.IsZero() {
		screenshot.CreatedAt = time.Now()
	}
	data, err := json.Marshal(screenshot)
	if err != nil {
		return fmt.Errorf("failed to marshal screenshot: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(screenshotsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", screenshotsBucket)
		}
		return b.Put([]byte(screenshot.ID), data)
	})
}

// UpdateScreenshot updates screenshot metadata
func (s *BboltMetaStorage) UpdateScreenshot(ctx context.Context, screenshotID string, updateFn func(types.ScreenshotMetadata) types.ScreenshotMetadata) (types.ScreenshotMetadata, error) {
	var updatedScreenshot types.ScreenshotMetadata

	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(screenshotsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", screenshotsBucket)
		}

		data := b.Get([]byte(screenshotID))
		if data == nil {
			return fmt.Errorf("screenshot not found: %s", screenshotID)
		}

		var screenshot types.ScreenshotMetadata
		if err := json.Unmarshal(data, &screenshot); err != nil {
			return fmt.Errorf("failed to unmarshal screenshot: %w", err)
		}

		updatedScreenshot = updateFn(screenshot)
		updatedScreenshot.UpdatedAt = time.Now()

		updatedData, err := json.Marshal(updatedScreenshot)
		if err != nil {
			return fmt.Errorf("failed to marshal updated screenshot: %w", err)
		}

		return b.Put([]byte(screenshotID), updatedData)
	})

	if err != nil {
		return types.ScreenshotMetadata{}, err
	}

	return updatedScreenshot, nil
}

// GetScreenshot retrieves screenshot metadata by ID
func (s *BboltMetaStorage) GetScreenshot(ctx context.Context, screenshotID string) (types.ScreenshotMetadata, bool) {
	var screenshot types.ScreenshotMetadata
	var found bool

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(screenshotsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", screenshotsBucket)
		}

		data := b.Get([]byte(screenshotID))
		if data == nil {
			return nil // Not found
		}

		found = true
		return json.Unmarshal(data, &screenshot)
	})

	if err != nil || !found {
		return types.ScreenshotMetadata{}, false
	}

	return screenshot, true
}

// ListScreenshots lists screenshot metadata with optional filters
func (s *BboltMetaStorage) ListScreenshots(ctx context.Context, filters map[string]interface{}) ([]types.ScreenshotMetadata, error) {
	var screenshots []types.ScreenshotMetadata

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(screenshotsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", screenshotsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var screenshot types.ScreenshotMetadata
			if err := json.Unmarshal(v, &screenshot); err != nil {
				s.logger.Warn("Failed to unmarshal screenshot", zap.String("screenshot_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Apply filters
			if filters != nil {
				if cameraID, ok := filters["camera_id"].(string); ok && screenshot.CameraID != cameraID {
					return nil
				}
				// Handle unlabeled filter (screenshots with empty label)
				if unlabeledOnly, ok := filters["unlabeled_only"].(bool); ok && unlabeledOnly {
					if screenshot.Label != "" {
						return nil
					}
				} else if label, ok := filters["label"].(string); ok {
					// If label filter is provided and not unlabeled_only, match exact label
					if screenshot.Label != label {
						return nil
					}
				}
				if customLabel, ok := filters["custom_label"].(string); ok && screenshot.CustomLabel != customLabel {
					return nil
				}
				if startTime, ok := filters["start_time"].(time.Time); ok && screenshot.CreatedAt.Before(startTime) {
					return nil
				}
				if endTime, ok := filters["end_time"].(time.Time); ok && screenshot.CreatedAt.After(endTime) {
					return nil
				}
			}

			screenshots = append(screenshots, screenshot)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list screenshots: %w", err)
	}

	// Sort by created_at descending
	sort.Slice(screenshots, func(i, j int) bool {
		return screenshots[i].CreatedAt.After(screenshots[j].CreatedAt)
	})

	return screenshots, nil
}

// DeleteScreenshot deletes screenshot metadata from the database
func (s *BboltMetaStorage) DeleteScreenshot(ctx context.Context, screenshotID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(screenshotsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", screenshotsBucket)
		}
		return b.Delete([]byte(screenshotID))
	})
}

// Clip metadata methods

// SaveClip saves clip metadata to the database
func (s *BboltMetaStorage) SaveClip(ctx context.Context, clip types.ClipMetadata) error {
	if clip.CreatedAt.IsZero() {
		clip.CreatedAt = time.Now()
	}
	data, err := json.Marshal(clip)
	if err != nil {
		return fmt.Errorf("failed to marshal clip: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipsBucket)
		}
		return b.Put([]byte(clip.ID), data)
	})
}

// GetClip retrieves clip metadata by ID
func (s *BboltMetaStorage) GetClip(ctx context.Context, clipID string) (types.ClipMetadata, bool) {
	var clip types.ClipMetadata
	var found bool

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipsBucket)
		}

		data := b.Get([]byte(clipID))
		if data == nil {
			return nil // Not found
		}

		found = true
		return json.Unmarshal(data, &clip)
	})

	if err != nil || !found {
		return types.ClipMetadata{}, false
	}

	return clip, true
}

// ListClips lists clip metadata with optional filters
func (s *BboltMetaStorage) ListClips(ctx context.Context, filters map[string]interface{}) ([]types.ClipMetadata, error) {
	var clips []types.ClipMetadata

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var clip types.ClipMetadata
			if err := json.Unmarshal(v, &clip); err != nil {
				s.logger.Warn("Failed to unmarshal clip", zap.String("clip_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Apply filters
			if filters != nil {
				if cameraID, ok := filters["camera_id"].(string); ok && clip.CameraID != cameraID {
					return nil
				}
				if eventID, ok := filters["event_id"].(string); ok && clip.EventID != eventID {
					return nil
				}
				if startTime, ok := filters["start_time"].(time.Time); ok && clip.CreatedAt.Before(startTime) {
					return nil
				}
				if endTime, ok := filters["end_time"].(time.Time); ok && clip.CreatedAt.After(endTime) {
					return nil
				}
			}

			clips = append(clips, clip)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list clips: %w", err)
	}

	// Sort by created_at descending
	sort.Slice(clips, func(i, j int) bool {
		return clips[i].CreatedAt.After(clips[j].CreatedAt)
	})

	return clips, nil
}

// DeleteClip deletes clip metadata from the database
func (s *BboltMetaStorage) DeleteClip(ctx context.Context, clipID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(clipsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", clipsBucket)
		}
		return b.Delete([]byte(clipID))
	})
}

// Security event metadata methods (for state-mng)

// SaveSecurityEvent saves security event metadata
func (s *BboltMetaStorage) SaveSecurityEvent(ctx context.Context, eventID string, eventData map[string]interface{}) error {
	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal security event: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(securityEventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", securityEventsBucket)
		}
		return b.Put([]byte(eventID), data)
	})
}

// GetSecurityEvent retrieves security event metadata
func (s *BboltMetaStorage) GetSecurityEvent(ctx context.Context, eventID string) (map[string]interface{}, bool) {
	var data []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(securityEventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", securityEventsBucket)
		}
		data = b.Get([]byte(eventID))
		return nil
	})
	if err != nil || data == nil {
		return nil, false
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(data, &eventData); err != nil {
		s.logger.Warn("Failed to unmarshal security event", zap.String("event_id", eventID), zap.Error(err))
		return nil, false
	}

	return eventData, true
}

// ListSecurityEvents lists security events with filters
func (s *BboltMetaStorage) ListSecurityEvents(ctx context.Context, filters map[string]interface{}) ([]map[string]interface{}, error) {
	var events []map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(securityEventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", securityEventsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var eventData map[string]interface{}
			if err := json.Unmarshal(v, &eventData); err != nil {
				s.logger.Warn("Failed to unmarshal security event", zap.String("event_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Apply filters if provided
			if filters != nil {
				if cameraID, ok := filters["camera_id"].(string); ok && cameraID != "" {
					if eventCameraID, ok := eventData["camera_id"].(string); !ok || eventCameraID != cameraID {
						return nil // Skip if camera ID doesn't match
					}
				}
				if eventType, ok := filters["event_type"].(string); ok && eventType != "" {
					if eventEventType, ok := eventData["event_type"].(string); !ok || eventEventType != eventType {
						return nil // Skip if event type doesn't match
					}
				}
			}

			events = append(events, eventData)
			return nil
		})
	})

	return events, err
}

// DeleteSecurityEvent deletes security event metadata
func (s *BboltMetaStorage) DeleteSecurityEvent(ctx context.Context, eventID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(securityEventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", securityEventsBucket)
		}
		return b.Delete([]byte(eventID))
	})
}

// Pending snapshot request metadata methods (for state-mng)

// SavePendingSnapshotRequest saves pending snapshot request metadata
func (s *BboltMetaStorage) SavePendingSnapshotRequest(ctx context.Context, cameraID string, requestData map[string]interface{}) error {
	data, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal pending snapshot request: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(pendingSnapshotRequestsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", pendingSnapshotRequestsBucket)
		}
		return b.Put([]byte(cameraID), data)
	})
}

// GetPendingSnapshotRequest retrieves pending snapshot request metadata
func (s *BboltMetaStorage) GetPendingSnapshotRequest(ctx context.Context, cameraID string) (map[string]interface{}, bool) {
	var data []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(pendingSnapshotRequestsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", pendingSnapshotRequestsBucket)
		}
		data = b.Get([]byte(cameraID))
		return nil
	})
	if err != nil || data == nil {
		return nil, false
	}

	var requestData map[string]interface{}
	if err := json.Unmarshal(data, &requestData); err != nil {
		s.logger.Warn("Failed to unmarshal pending snapshot request", zap.String("camera_id", cameraID), zap.Error(err))
		return nil, false
	}

	return requestData, true
}

// ListPendingSnapshotRequests lists all pending snapshot requests
func (s *BboltMetaStorage) ListPendingSnapshotRequests(ctx context.Context) ([]map[string]interface{}, error) {
	var requests []map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(pendingSnapshotRequestsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", pendingSnapshotRequestsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var requestData map[string]interface{}
			if err := json.Unmarshal(v, &requestData); err != nil {
				s.logger.Warn("Failed to unmarshal pending snapshot request", zap.String("camera_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}
			requests = append(requests, requestData)
			return nil
		})
	})

	return requests, err
}

// DeletePendingSnapshotRequest deletes pending snapshot request metadata
func (s *BboltMetaStorage) DeletePendingSnapshotRequest(ctx context.Context, cameraID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(pendingSnapshotRequestsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", pendingSnapshotRequestsBucket)
		}
		return b.Delete([]byte(cameraID))
	})
}

// SaveEdgeState saves the current edge state and appends to history
func (s *BboltMetaStorage) SaveEdgeState(ctx context.Context, state map[string]interface{}) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal edge state: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		// Save current state
		currentBucket := tx.Bucket([]byte(edgeStateBucket))
		if currentBucket == nil {
			return fmt.Errorf("bucket %s not found", edgeStateBucket)
		}
		if err := currentBucket.Put([]byte(currentEdgeStateKey), data); err != nil {
			return fmt.Errorf("failed to save current edge state: %w", err)
		}

		// Append to history (with timestamp as key for ordering)
		historyBucket := tx.Bucket([]byte(edgeStateHistoryBucket))
		if historyBucket == nil {
			return fmt.Errorf("bucket %s not found", edgeStateHistoryBucket)
		}

		// Use timestamp as key (nanoseconds since epoch for uniqueness)
		timestampKey := []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
		if err := historyBucket.Put(timestampKey, data); err != nil {
			return fmt.Errorf("failed to save edge state history: %w", err)
		}

		return nil
	})
}

// GetCurrentEdgeState retrieves the current edge state
func (s *BboltMetaStorage) GetCurrentEdgeState(ctx context.Context) (map[string]interface{}, bool) {
	var result map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeStateBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeStateBucket)
		}

		data := b.Get([]byte(currentEdgeStateKey))
		if data == nil {
			return fmt.Errorf("current edge state not found")
		}

		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("failed to unmarshal edge state: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, false
	}

	return result, true
}

// GetEdgeStateHistory retrieves edge state history, most recent first
func (s *BboltMetaStorage) GetEdgeStateHistory(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	var history []map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeStateHistoryBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeStateHistoryBucket)
		}

		c := b.Cursor()
		// Iterate in reverse order (most recent first)
		var keys [][]byte
		for k, _ := c.Last(); k != nil; k, _ = c.Prev() {
			keys = append(keys, k)
			if limit > 0 && len(keys) >= limit {
				break
			}
		}

		// Read values in reverse order
		for i := len(keys) - 1; i >= 0; i-- {
			data := b.Get(keys[i])
			if data == nil {
				continue
			}

			var state map[string]interface{}
			if err := json.Unmarshal(data, &state); err != nil {
				s.logger.Warn("failed to unmarshal edge state history entry", zap.Error(err))
				continue
			}

			history = append(history, state)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return history, nil
}

// SaveEdgeCapabilities saves the Edge capabilities received from VM
func (s *BboltMetaStorage) SaveEdgeCapabilities(ctx context.Context, capabilities map[string]interface{}) error {
	data, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal edge capabilities: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeCapabilitiesBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeCapabilitiesBucket)
		}
		if err := b.Put([]byte(currentEdgeCapabilitiesKey), data); err != nil {
			return fmt.Errorf("failed to save edge capabilities: %w", err)
		}
		return nil
	})
}

// GetEdgeCapabilities retrieves the Edge capabilities
func (s *BboltMetaStorage) GetEdgeCapabilities(ctx context.Context) (map[string]interface{}, bool) {
	var result map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(edgeCapabilitiesBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", edgeCapabilitiesBucket)
		}

		data := b.Get([]byte(currentEdgeCapabilitiesKey))
		if data == nil {
			return fmt.Errorf("edge capabilities not found")
		}

		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("failed to unmarshal edge capabilities: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, false
	}

	return result, true
}

// Close closes the database and releases all resources
func (s *BboltMetaStorage) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Event bus metadata methods

// SaveEvent saves event metadata
func (s *BboltMetaStorage) SaveEvent(ctx context.Context, eventID string, eventData map[string]interface{}) error {
	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(eventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}
		return b.Put([]byte(eventID), data)
	})
}

// GetEvent retrieves event metadata
func (s *BboltMetaStorage) GetEvent(ctx context.Context, eventID string) (map[string]interface{}, bool) {
	var data []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(eventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}
		data = b.Get([]byte(eventID))
		return nil
	})
	if err != nil || data == nil {
		return nil, false
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(data, &eventData); err != nil {
		s.logger.Warn("Failed to unmarshal event", zap.String("event_id", eventID), zap.Error(err))
		return nil, false
	}

	return eventData, true
}

// ListEvents lists events with filters
func (s *BboltMetaStorage) ListEvents(ctx context.Context, filters map[string]interface{}) ([]map[string]interface{}, error) {
	var events []map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(eventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var eventData map[string]interface{}
			if err := json.Unmarshal(v, &eventData); err != nil {
				s.logger.Warn("Failed to unmarshal event", zap.String("event_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Apply filters if provided
			if filters != nil {
				// Filter by type
				if eventType, ok := filters["type"].(string); ok && eventType != "" {
					if eventEventType, ok := eventData["type"].(string); !ok || eventEventType != eventType {
						return nil // Skip if event type doesn't match
					}
				}

				// Filter by source
				if source, ok := filters["source"].(string); ok && source != "" {
					if eventSource, ok := eventData["source"].(string); !ok || eventSource != source {
						return nil // Skip if source doesn't match
					}
				}

				// Filter by time range
				if startTimeStr, ok := filters["start_time"].(string); ok && startTimeStr != "" {
					startTime, err := time.Parse(time.RFC3339Nano, startTimeStr)
					if err == nil {
						if timestampStr, ok := eventData["timestamp"].(string); ok {
							if eventTime, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
								if eventTime.Before(startTime) {
									return nil // Skip if before start time
								}
							}
						}
					}
				}

				if endTimeStr, ok := filters["end_time"].(string); ok && endTimeStr != "" {
					endTime, err := time.Parse(time.RFC3339Nano, endTimeStr)
					if err == nil {
						if timestampStr, ok := eventData["timestamp"].(string); ok {
							if eventTime, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
								if eventTime.After(endTime) {
									return nil // Skip if after end time
								}
							}
						}
					}
				}
			}

			events = append(events, eventData)
			return nil
		})
	})

	// Sort by timestamp (newest first)
	sort.Slice(events, func(i, j int) bool {
		timeI, okI := events[i]["timestamp"].(string)
		timeJ, okJ := events[j]["timestamp"].(string)
		if !okI || !okJ {
			return false
		}
		tI, errI := time.Parse(time.RFC3339Nano, timeI)
		tJ, errJ := time.Parse(time.RFC3339Nano, timeJ)
		if errI != nil || errJ != nil {
			return false
		}
		return tI.After(tJ)
	})

	// Apply limit if provided
	if filters != nil {
		if limit, ok := filters["limit"].(int); ok && limit > 0 && len(events) > limit {
			events = events[:limit]
		}
	}

	return events, err
}

// DeleteEvent deletes event metadata
func (s *BboltMetaStorage) DeleteEvent(ctx context.Context, eventID string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(eventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}
		return b.Delete([]byte(eventID))
	})
}

// GetEventCount returns the total number of events stored
func (s *BboltMetaStorage) GetEventCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(eventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}
		count = b.Stats().KeyN
		return nil
	})
	return count, err
}

// UpdateEventProcessingStatus updates the processing status and retry metadata for an event
func (s *BboltMetaStorage) UpdateEventProcessingStatus(ctx context.Context, eventID string, status string, retryCount int, lastError string, nextRetryTime *time.Time) error {
	// Get existing event data
	eventData, exists := s.GetEvent(ctx, eventID)
	if !exists {
		return fmt.Errorf("event %s not found", eventID)
	}

	// Update processing metadata
	eventData["processing_status"] = status
	eventData["retry_count"] = retryCount
	if lastError != "" {
		eventData["last_error"] = lastError
	}
	if nextRetryTime != nil {
		eventData["next_retry_time"] = nextRetryTime.Format(time.RFC3339Nano)
	} else {
		delete(eventData, "next_retry_time")
	}
	eventData["last_updated"] = time.Now().Format(time.RFC3339Nano)

	// Save updated event
	return s.SaveEvent(ctx, eventID, eventData)
}

// GetFailedEvents retrieves events that have failed and are ready for retry
func (s *BboltMetaStorage) GetFailedEvents(ctx context.Context, beforeTime time.Time) ([]map[string]interface{}, error) {
	var failedEvents []map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(eventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			var eventData map[string]interface{}
			if err := json.Unmarshal(v, &eventData); err != nil {
				s.logger.Warn("Failed to unmarshal event", zap.String("event_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			// Check if event is in failed status
			status, ok := eventData["processing_status"].(string)
			if !ok || status != "failed" {
				return nil // Skip non-failed events
			}

			// Check if next retry time is before the specified time
			nextRetryStr, ok := eventData["next_retry_time"].(string)
			if !ok {
				return nil // Skip if no next_retry_time
			}

			nextRetryTime, err := time.Parse(time.RFC3339Nano, nextRetryStr)
			if err != nil {
				s.logger.Warn("Failed to parse next_retry_time", zap.String("event_id", string(k)), zap.Error(err))
				return nil
			}

			if nextRetryTime.After(beforeTime) {
				return nil // Not ready for retry yet
			}

			failedEvents = append(failedEvents, eventData)
			return nil
		})
	})

	return failedEvents, err
}

// GetDeadLetterEvents retrieves events from the dead letter queue
func (s *BboltMetaStorage) GetDeadLetterEvents(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	var deadLetterEvents []map[string]interface{}

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(deadLetterEventsBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", deadLetterEventsBucket)
		}

		return b.ForEach(func(k, v []byte) error {
			if limit > 0 && len(deadLetterEvents) >= limit {
				return nil // Stop if limit reached
			}

			var eventData map[string]interface{}
			if err := json.Unmarshal(v, &eventData); err != nil {
				s.logger.Warn("Failed to unmarshal dead letter event", zap.String("event_id", string(k)), zap.Error(err))
				return nil // Skip invalid entries
			}

			deadLetterEvents = append(deadLetterEvents, eventData)
			return nil
		})
	})

	// Sort by timestamp (newest first)
	sort.Slice(deadLetterEvents, func(i, j int) bool {
		timeI, okI := deadLetterEvents[i]["timestamp"].(string)
		timeJ, okJ := deadLetterEvents[j]["timestamp"].(string)
		if !okI || !okJ {
			return false
		}
		tI, errI := time.Parse(time.RFC3339Nano, timeI)
		tJ, errJ := time.Parse(time.RFC3339Nano, timeJ)
		if errI != nil || errJ != nil {
			return false
		}
		return tI.After(tJ)
	})

	return deadLetterEvents, err
}

// MoveEventToDeadLetter moves an event from the events bucket to the dead letter queue
func (s *BboltMetaStorage) MoveEventToDeadLetter(ctx context.Context, eventID string) error {
	// Get event data
	eventData, exists := s.GetEvent(ctx, eventID)
	if !exists {
		return fmt.Errorf("event %s not found", eventID)
	}

	// Update status to dead_letter
	eventData["processing_status"] = "dead_letter"
	eventData["moved_to_dlq_at"] = time.Now().Format(time.RFC3339Nano)
	eventData["last_updated"] = time.Now().Format(time.RFC3339Nano)

	// Save to dead letter bucket
	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	err = s.db.Update(func(tx *bbolt.Tx) error {
		// Save to dead letter bucket
		dlqBucket := tx.Bucket([]byte(deadLetterEventsBucket))
		if dlqBucket == nil {
			return fmt.Errorf("bucket %s not found", deadLetterEventsBucket)
		}
		if err := dlqBucket.Put([]byte(eventID), data); err != nil {
			return fmt.Errorf("failed to save to dead letter queue: %w", err)
		}

		// Remove from events bucket
		eventsBucket := tx.Bucket([]byte(eventsBucket))
		if eventsBucket == nil {
			return fmt.Errorf("bucket %s not found", eventsBucket)
		}
		return eventsBucket.Delete([]byte(eventID))
	})

	return err
}
