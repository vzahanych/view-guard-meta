package minioimp

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/google/uuid"
	"go.uber.org/zap"

	objectstorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/object-storage"
)

// S3Client defines the minimal operations required from an S3/MinIO client.
// This keeps MinIOObjectStorage decoupled from a specific SDK or wrapper.
type S3Client interface {
	UploadFile(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	DownloadFile(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteFile(ctx context.Context, key string) error
}

// MinIOObjectStorage implements ObjectStorageService using S3/MinIO via S3Client.
type MinIOObjectStorage struct {
	logger   *zap.Logger
	s3Client S3Client
}

// NewMinIOObjectStorage creates a new MinIOObjectStorage.
func NewMinIOObjectStorage(logger *zap.Logger, client S3Client) *MinIOObjectStorage {
	return &MinIOObjectStorage{
		logger:   logger,
		s3Client: client,
	}
}

// Name returns the service name.
func (s *MinIOObjectStorage) Name() string {
	return "minio-object-storage"
}

// Start is a no-op for now; connectivity is validated when S3Client is created.
func (s *MinIOObjectStorage) Start(ctx context.Context) error {
	s.logger.Info("Starting MinIO object storage service")
	return nil
}

// Stop is a no-op; there are no long‑lived connections to close.
func (s *MinIOObjectStorage) Stop(ctx context.Context) error {
	s.logger.Info("Stopping MinIO object storage service")
	return nil
}

// GetS3Client returns the underlying S3 client.
func (s *MinIOObjectStorage) GetS3Client() S3Client {
	return s.s3Client
}

// object keys
func rawModelKey(id uuid.UUID) string {
	return path.Join("models", "raw", id.String())
}

func trainedModelKey(id uuid.UUID) string {
	return path.Join("models", "trained", id.String())
}

func datasetKey(id uuid.UUID) string {
	return path.Join("datasets", id.String(), "dataset.tar.gz")
}

func clipKey(id uuid.UUID) string {
	return path.Join("clips", id.String())
}

// Raw models

func (s *MinIOObjectStorage) StoreRawModel(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error {
	if id == uuid.Nil {
		return fmt.Errorf("raw model id is required")
	}
	key := rawModelKey(id)
	return s.s3Client.UploadFile(ctx, key, r, size, contentType)
}

func (s *MinIOObjectStorage) LoadRawModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("raw model id is required")
	}
	key := rawModelKey(id)
	return s.s3Client.DownloadFile(ctx, key)
}

func (s *MinIOObjectStorage) DeleteRawModel(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("raw model id is required")
	}
	key := rawModelKey(id)
	return s.s3Client.DeleteFile(ctx, key)
}

// Trained models

func (s *MinIOObjectStorage) StoreTrainedModel(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error {
	if id == uuid.Nil {
		return fmt.Errorf("trained model id is required")
	}
	key := trainedModelKey(id)
	return s.s3Client.UploadFile(ctx, key, r, size, contentType)
}

func (s *MinIOObjectStorage) LoadTrainedModel(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("trained model id is required")
	}
	key := trainedModelKey(id)
	return s.s3Client.DownloadFile(ctx, key)
}

func (s *MinIOObjectStorage) DeleteTrainedModel(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("trained model id is required")
	}
	key := trainedModelKey(id)
	return s.s3Client.DeleteFile(ctx, key)
}

// Training datasets

func (s *MinIOObjectStorage) StoreTrainingDataset(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error {
	if id == uuid.Nil {
		return fmt.Errorf("training dataset id is required")
	}
	key := datasetKey(id)
	return s.s3Client.UploadFile(ctx, key, r, size, contentType)
}

func (s *MinIOObjectStorage) LoadTrainingDataset(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("training dataset id is required")
	}
	key := datasetKey(id)
	return s.s3Client.DownloadFile(ctx, key)
}

func (s *MinIOObjectStorage) DeleteTrainingDataset(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("training dataset id is required")
	}
	key := datasetKey(id)
	return s.s3Client.DeleteFile(ctx, key)
}

// Clips / snapshots

func (s *MinIOObjectStorage) StoreClip(ctx context.Context, id uuid.UUID, r io.Reader, size int64, contentType string) error {
	if id == uuid.Nil {
		return fmt.Errorf("clip id is required")
	}
	key := clipKey(id)
	return s.s3Client.UploadFile(ctx, key, r, size, contentType)
}

func (s *MinIOObjectStorage) LoadClip(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("clip id is required")
	}
	key := clipKey(id)
	return s.s3Client.DownloadFile(ctx, key)
}

func (s *MinIOObjectStorage) DeleteClip(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("clip id is required")
	}
	key := clipKey(id)
	return s.s3Client.DeleteFile(ctx, key)
}

// Ensure MinIOObjectStorage implements ObjectStorageService.
var _ objectstorage.ObjectStorageService = (*MinIOObjectStorage)(nil)
