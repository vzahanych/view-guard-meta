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
	pendingSnapshotRequestsBucket = "pending_snapshot_requests"
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
			pendingSnapshotRequestsBucket,
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
func (s *BboltMetaStorage) ListCameras(ctx context.Context, enabledOnly bool) ([]types.CameraMetadata, error) {
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

			if enabledOnly && !camera.Enabled {
				return nil
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
				if label, ok := filters["label"].(string); ok && screenshot.Label != label {
					return nil
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

// Close closes the database and releases all resources
func (s *BboltMetaStorage) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
