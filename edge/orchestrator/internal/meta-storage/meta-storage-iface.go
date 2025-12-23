package metastorage

import (
	"context"
	"fmt"

	bboltimp "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/bbolt-imp"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Re-export types for convenience
type StorageEntryMetadata = types.StorageEntryMetadata
type DeployedModelMetadata = types.DeployedModelMetadata
type StorageStats = types.StorageStats
type ModelFilters = types.ModelFilters
type CameraMetadata = types.CameraMetadata
type CameraFilters = types.CameraFilters
type ScreenshotMetadata = types.ScreenshotMetadata
type ClipMetadata = types.ClipMetadata

// MetaDataStore defines operations for managing metadata about stored objects
type MetaDataStore interface {
	// Storage entry metadata (clips and snapshots)
	SaveStorageEntry(ctx context.Context, entry StorageEntryMetadata) error
	DeleteStorageEntry(ctx context.Context, path string) error
	ListStorageEntries(ctx context.Context, fileType string) ([]StorageEntryMetadata, error)
	GetStorageStats(ctx context.Context) (*StorageStats, error)

	// Deployed model metadata
	SaveDeployedModel(ctx context.Context, model DeployedModelMetadata) error
	UpdateDeployedModel(ctx context.Context, modelID string, updateFn func(DeployedModelMetadata) DeployedModelMetadata) (DeployedModelMetadata, error)
	GetDeployedModel(ctx context.Context, modelID string) (DeployedModelMetadata, bool)
	ListDeployedModels(ctx context.Context, filters *ModelFilters) ([]DeployedModelMetadata, error)
	DeleteDeployedModel(ctx context.Context, modelID string) error

	// Camera metadata
	SaveCamera(ctx context.Context, camera CameraMetadata) error
	UpdateCamera(ctx context.Context, cameraID string, updateFn func(CameraMetadata) CameraMetadata) (CameraMetadata, error)
	GetCamera(ctx context.Context, cameraID string) (CameraMetadata, bool)
	ListCameras(ctx context.Context, filters *CameraFilters) ([]CameraMetadata, error)
	DeleteCamera(ctx context.Context, cameraID string) error

	// Screenshot metadata
	SaveScreenshot(ctx context.Context, screenshot ScreenshotMetadata) error
	UpdateScreenshot(ctx context.Context, screenshotID string, updateFn func(ScreenshotMetadata) ScreenshotMetadata) (ScreenshotMetadata, error)
	GetScreenshot(ctx context.Context, screenshotID string) (ScreenshotMetadata, bool)
	ListScreenshots(ctx context.Context, filters map[string]interface{}) ([]ScreenshotMetadata, error)
	DeleteScreenshot(ctx context.Context, screenshotID string) error

	// Clip metadata
	SaveClip(ctx context.Context, clip ClipMetadata) error
	GetClip(ctx context.Context, clipID string) (ClipMetadata, bool)
	ListClips(ctx context.Context, filters map[string]interface{}) ([]ClipMetadata, error)
	DeleteClip(ctx context.Context, clipID string) error

	// Security event metadata (for state-mng)
	SaveSecurityEvent(ctx context.Context, eventID string, eventData map[string]interface{}) error
	GetSecurityEvent(ctx context.Context, eventID string) (map[string]interface{}, bool)
	ListSecurityEvents(ctx context.Context, filters map[string]interface{}) ([]map[string]interface{}, error)
	DeleteSecurityEvent(ctx context.Context, eventID string) error

	// Pending snapshot request metadata (for state-mng)
	SavePendingSnapshotRequest(ctx context.Context, cameraID string, requestData map[string]interface{}) error
	GetPendingSnapshotRequest(ctx context.Context, cameraID string) (map[string]interface{}, bool)
	ListPendingSnapshotRequests(ctx context.Context) ([]map[string]interface{}, error)
	DeletePendingSnapshotRequest(ctx context.Context, cameraID string) error

	// Edge state metadata (current state and history)
	SaveEdgeState(ctx context.Context, state map[string]interface{}) error
	GetCurrentEdgeState(ctx context.Context) (map[string]interface{}, bool)
	GetEdgeStateHistory(ctx context.Context, limit int) ([]map[string]interface{}, error)

	// Edge capabilities metadata (capabilities sent by VM)
	SaveEdgeCapabilities(ctx context.Context, capabilities map[string]interface{}) error
	GetEdgeCapabilities(ctx context.Context) (map[string]interface{}, bool)

	// Close closes the metadata store and releases all resources (e.g., database connections).
	// After Close, all methods should return errors.
	Close() error
}

func NewMetaDataStore(ctx context.Context, config *types.MetaStorageConfig, logger *zap.Logger) (MetaDataStore, error) {
	switch config.Provider {
	case "bbolt":
		return bboltimp.NewBboltMetaDataStore(ctx, config, logger)
	default:
		return nil, fmt.Errorf("unsupported meta-storage provider: %s", config.Provider)
	}
}

// MetaStorageProvider creates the meta storage service with fx lifecycle management
func MetaStorageProvider(lc fx.Lifecycle, cfg *types.MetaStorageConfig, logger *zap.Logger) (MetaDataStore, error) {
	store, err := NewMetaDataStore(context.Background(), cfg, logger)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Meta storage started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Meta storage stopping")
			if err := store.Close(); err != nil {
				logger.Error("Failed to close meta storage", zap.Error(err))
				return err
			}
			logger.Info("Meta storage stopped")
			return nil
		},
	})

	return store, nil
}
