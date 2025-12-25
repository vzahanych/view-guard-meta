package objectstorage

import (
	"context"
	"fmt"
	"io"

	minioimp "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/minio-imp"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ObjectStorageService provides operations for storing and retrieving binary objects
// (clips, snapshots, models) using MinIO/S3-compatible object storage.
//
// This service handles the actual object storage operations, while metadata about these objects
// is managed by MetaDataStore in the meta-storage package.

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_object_storage.go -package=mocks
type ObjectStorageService interface {
	// Lifecycle hooks
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string

	// Clips (video files)
	// key is the object key (e.g., "clips/cameraID/timestamp.mp4")
	StoreClip(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	LoadClip(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteClip(ctx context.Context, key string) error
	GenerateClipKey(cameraID string) string // Generates a unique key for a clip

	// Snapshots (image files)
	// key is the object key (e.g., "snapshots/cameraID/timestamp.jpg")
	StoreSnapshot(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	LoadSnapshot(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteSnapshot(ctx context.Context, key string) error
	GenerateSnapshotKey(cameraID string, isThumbnail bool) string // Generates a unique key for a snapshot

	// Frames (temporary image files for AI processing)
	// key is the object key (e.g., "frames/cameraID/date/frameID.jpg")
	// frameData is the raw image bytes
	StoreFrame(ctx context.Context, key string, frameData []byte) error
	LoadFrame(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteFrame(ctx context.Context, key string) error
	MoveFrameToSecurityEvent(ctx context.Context, sourceKey string, cameraID string) (string, error) // Moves frame to security-events bucket

	// Models (model files)
	// modelKey is the object key for the model file (e.g., "models/modelID/model.onnx")
	// metadataKey is the object key for the metadata file (e.g., "models/modelID/metadata.json")
	StoreModel(ctx context.Context, modelKey string, modelData []byte, metadataKey string, metadataJSON []byte) error
	LoadModel(ctx context.Context, modelKey string) ([]byte, error)
	LoadModelMetadata(ctx context.Context, metadataKey string) ([]byte, error)
	DeleteModel(ctx context.Context, modelKey string, metadataKey string) error
}

func NewObjectStorageService(ctx context.Context, config *types.ObjectStorageConfig, logger *zap.Logger) (ObjectStorageService, error) {
	switch config.Provider {
	case "minio":
		return minioimp.NewMinIOObjectStorage(ctx, config, logger)
	default:
		return nil, fmt.Errorf("unsupported object-storage provider: %s", config.Provider)
	}
}

// ObjectStorageProvider creates the object storage service with fx lifecycle management
func ObjectStorageProvider(lc fx.Lifecycle, cfg *types.ObjectStorageConfig, logger *zap.Logger) (ObjectStorageService, error) {
	store, err := NewObjectStorageService(context.Background(), cfg, logger)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if store != nil {
				if err := store.Start(ctx); err != nil {
					return err
				}
			}
			logger.Info("Object storage started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if store != nil {
				if err := store.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("Object storage stopped")
			return nil
		},
	})

	return store, nil
}
