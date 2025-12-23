package minioimp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// S3Client defines the minimal operations required from an S3/MinIO client.
// This keeps MinIOObjectStorage decoupled from a specific SDK or wrapper.
type S3Client interface {
	UploadFile(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	DownloadFile(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteFile(ctx context.Context, key string) error
}

// minioClient implements S3Client using MinIO Go client
type minioClient struct {
	client   *minio.Client
	bucket   string
	endpoint string
	logger   *zap.Logger
}

// NewMinIOClient creates a new MinIO client from configuration
func NewMinIOClient(ctx context.Context, cfg *types.ObjectStorageConfig, logger *zap.Logger) (S3Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("object storage endpoint is required")
	}
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("object storage access key is required")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("object storage secret key is required")
	}

	// Default bucket name
	bucket := "edge-storage"

	// Initialize MinIO client
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: false, // Set to true for HTTPS
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Test connection by checking if bucket exists, create if it doesn't
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		logger.Info("Created object storage bucket", zap.String("bucket", bucket))
	}

	return &minioClient{
		client:   client,
		bucket:   bucket,
		endpoint: cfg.Endpoint,
		logger:   logger,
	}, nil
}

// UploadFile uploads a file to MinIO
func (c *minioClient) UploadFile(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := c.client.PutObject(ctx, c.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file %s: %w", key, err)
	}
	return nil
}

// DownloadFile downloads a file from MinIO
func (c *minioClient) DownloadFile(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file %s: %w", key, err)
	}
	return obj, nil
}

// DeleteFile deletes a file from MinIO
func (c *minioClient) DeleteFile(ctx context.Context, key string) error {
	if err := c.client.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", key, err)
	}
	return nil
}

// MinIOObjectStorage implements ObjectStorageService using S3/MinIO via S3Client.
type MinIOObjectStorage struct {
	logger   *zap.Logger
	s3Client S3Client
}

// NewMinIOObjectStorage creates a new MinIOObjectStorage from configuration.
func NewMinIOObjectStorage(ctx context.Context, cfg *types.ObjectStorageConfig, logger *zap.Logger) (*MinIOObjectStorage, error) {
	client, err := NewMinIOClient(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &MinIOObjectStorage{
		logger:   logger,
		s3Client: client,
	}, nil
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

// Stop is a no-op; there are no long-lived connections to close.
func (s *MinIOObjectStorage) Stop(ctx context.Context) error {
	s.logger.Info("Stopping MinIO object storage service")
	return nil
}

// GetS3Client returns the underlying S3 client.
func (s *MinIOObjectStorage) GetS3Client() S3Client {
	return s.s3Client
}

// Clips

func (s *MinIOObjectStorage) StoreClip(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if key == "" {
		return fmt.Errorf("clip key is required")
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	return s.s3Client.UploadFile(ctx, key, r, size, contentType)
}

func (s *MinIOObjectStorage) LoadClip(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("clip key is required")
	}
	return s.s3Client.DownloadFile(ctx, key)
}

func (s *MinIOObjectStorage) DeleteClip(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("clip key is required")
	}
	return s.s3Client.DeleteFile(ctx, key)
}

func (s *MinIOObjectStorage) GenerateClipKey(cameraID string) string {
	// Organize by date: YYYY-MM-DD/cameraID_timestamp_uuid.mp4
	dateDir := time.Now().Format("2006-01-02")
	timestamp := time.Now().Format("150405")
	uuid := uuid.New().String()
	filename := fmt.Sprintf("%s_%s_%s.mp4", cameraID, timestamp, uuid)
	return path.Join("clips", dateDir, filename)
}

// Snapshots

func (s *MinIOObjectStorage) StoreSnapshot(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if key == "" {
		return fmt.Errorf("snapshot key is required")
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return s.s3Client.UploadFile(ctx, key, r, size, contentType)
}

func (s *MinIOObjectStorage) LoadSnapshot(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("snapshot key is required")
	}
	return s.s3Client.DownloadFile(ctx, key)
}

func (s *MinIOObjectStorage) DeleteSnapshot(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("snapshot key is required")
	}
	return s.s3Client.DeleteFile(ctx, key)
}

func (s *MinIOObjectStorage) GenerateSnapshotKey(cameraID string, isThumbnail bool) string {
	// Organize by date: YYYY-MM-DD/cameraID_timestamp_uuid.jpg
	dateDir := time.Now().Format("2006-01-02")
	timestamp := time.Now().Format("150405")
	uuid := uuid.New().String()

	var filename string
	if isThumbnail {
		filename = fmt.Sprintf("%s_%s_%s_thumb.jpg", cameraID, timestamp, uuid)
	} else {
		filename = fmt.Sprintf("%s_%s_%s.jpg", cameraID, timestamp, uuid)
	}
	return path.Join("snapshots", dateDir, filename)
}

// Models

func (s *MinIOObjectStorage) StoreModel(ctx context.Context, modelKey string, modelData []byte, metadataKey string, metadataJSON []byte) error {
	if modelKey == "" {
		return fmt.Errorf("model key is required")
	}
	if metadataKey == "" {
		return fmt.Errorf("metadata key is required")
	}

	// Store model file
	modelReader := io.NopCloser(bytes.NewReader(modelData))
	if err := s.s3Client.UploadFile(ctx, modelKey, modelReader, int64(len(modelData)), "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to store model file: %w", err)
	}

	// Store metadata file
	metadataReader := io.NopCloser(bytes.NewReader(metadataJSON))
	if err := s.s3Client.UploadFile(ctx, metadataKey, metadataReader, int64(len(metadataJSON)), "application/json"); err != nil {
		// Try to clean up model file on error
		s.s3Client.DeleteFile(ctx, modelKey)
		return fmt.Errorf("failed to store metadata file: %w", err)
	}

	return nil
}

func (s *MinIOObjectStorage) LoadModel(ctx context.Context, modelKey string) ([]byte, error) {
	if modelKey == "" {
		return nil, fmt.Errorf("model key is required")
	}
	reader, err := s.s3Client.DownloadFile(ctx, modelKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (s *MinIOObjectStorage) LoadModelMetadata(ctx context.Context, metadataKey string) ([]byte, error) {
	if metadataKey == "" {
		return nil, fmt.Errorf("metadata key is required")
	}
	reader, err := s.s3Client.DownloadFile(ctx, metadataKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (s *MinIOObjectStorage) DeleteModel(ctx context.Context, modelKey string, metadataKey string) error {
	if modelKey == "" {
		return fmt.Errorf("model key is required")
	}
	if metadataKey == "" {
		return fmt.Errorf("metadata key is required")
	}

	// Delete both files
	if err := s.s3Client.DeleteFile(ctx, modelKey); err != nil {
		s.logger.Warn("Failed to delete model file", zap.String("key", modelKey), zap.Error(err))
	}
	if err := s.s3Client.DeleteFile(ctx, metadataKey); err != nil {
		s.logger.Warn("Failed to delete metadata file", zap.String("key", metadataKey), zap.Error(err))
	}

	return nil
}
