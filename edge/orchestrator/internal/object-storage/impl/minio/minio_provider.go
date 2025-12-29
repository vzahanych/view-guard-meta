package minio

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// MinIOProvider implements the ObjectStorageProvider interface using MinIO.
// This provider is thread-safe and manages a MinIO client connection.
type MinIOProvider struct {
	client *minio.Client
	bucket string
	logger *zap.Logger
}

// NewMinIOProvider creates a new MinIO provider instance.
// The provider initializes the MinIO client and verifies connectivity.
func NewMinIOProvider(ctx context.Context, cfg *types.MinIOConfig, logger *zap.Logger) (*MinIOProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("MinIO configuration is required")
	}

	// Set defaults
	endpoint := cfg.Endpoint
	if endpoint == "" {
		return nil, fmt.Errorf("MinIO endpoint is required")
	}

	accessKey := cfg.AccessKey
	if accessKey == "" {
		return nil, fmt.Errorf("MinIO access key is required")
	}

	secretKey := cfg.SecretKey
	if secretKey == "" {
		return nil, fmt.Errorf("MinIO secret key is required")
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1" // Default region
	}

	bucket := cfg.Bucket
	if bucket == "" {
		bucket = "edge-storage" // Default bucket
	}

	// Initialize MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: cfg.UseSSL,
		Region: region,
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
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		if logger != nil {
			logger.Info("Created MinIO bucket", zap.String("bucket", bucket))
		}
	}

	return &MinIOProvider{
		client: client,
		bucket: bucket,
		logger: logger,
	}, nil
}

// StoreObject stores an object in MinIO storage.
func (p *MinIOProvider) StoreObject(ctx context.Context, key string, r io.Reader, size int64, contentType string, metadata map[string]string) error {
	if key == "" {
		return fmt.Errorf("object key is required")
	}

	// Convert metadata map to MinIO metadata format
	minioMetadata := make(map[string]string)
	for k, v := range metadata {
		minioMetadata[k] = v
	}

	// Store object with metadata
	_, err := p.client.PutObject(ctx, p.bucket, key, r, size, minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: minioMetadata,
	})
	if err != nil {
		return fmt.Errorf("failed to store object %s: %w", key, err)
	}

	return nil
}

// LoadObject retrieves an object from MinIO storage.
func (p *MinIOProvider) LoadObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("object key is required")
	}

	obj, err := p.client.GetObject(ctx, p.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to load object %s: %w", key, err)
	}

	return obj, nil
}

// DeleteObject deletes an object from MinIO storage.
func (p *MinIOProvider) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("object key is required")
	}

	if err := p.client.RemoveObject(ctx, p.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("failed to delete object %s: %w", key, err)
	}

	return nil
}

// ListObjects lists objects in MinIO storage with the given prefix.
func (p *MinIOProvider) ListObjects(ctx context.Context, prefix string) ([]types.ObjectInfo, error) {
	var objects []types.ObjectInfo

	// List objects with prefix
	objectCh := p.client.ListObjects(ctx, p.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range objectCh {
		if obj.Err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", obj.Err)
		}

		objects = append(objects, types.ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified.Unix(),
			ETag:         obj.ETag,
		})
	}

	return objects, nil
}

// GetObjectMetadata retrieves metadata for an object from MinIO storage.
func (p *MinIOProvider) GetObjectMetadata(ctx context.Context, key string) (*types.ObjectMetadata, error) {
	if key == "" {
		return nil, fmt.Errorf("object key is required")
	}

	// Get object info
	objInfo, err := p.client.StatObject(ctx, p.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object metadata for %s: %w", key, err)
	}

	// Extract metadata from MinIO metadata
	metadata := &types.ObjectMetadata{
		Key:         objInfo.Key,
		Size:        objInfo.Size,
		ContentType: objInfo.ContentType,
		Hash:        objInfo.ETag, // Use ETag as hash (MinIO provides MD5 ETag)
		CreatedAt:   objInfo.LastModified, // MinIO doesn't track creation time separately
		Metadata:    make(map[string]string), // Initialize metadata map
	}

	// Extract device ID and device type from user metadata if available
	if objInfo.UserMetadata != nil {
		// Copy all user metadata to metadata map
		for k, v := range objInfo.UserMetadata {
			metadata.Metadata[k] = v
		}

		if deviceID, ok := objInfo.UserMetadata["device_id"]; ok {
			metadata.DeviceID = types.DeviceID(deviceID)
		}
		if deviceType, ok := objInfo.UserMetadata["device_type"]; ok {
			metadata.DeviceType = types.DeviceType(deviceType)
		}
		// Extract hash from metadata if available (stored separately from ETag)
		if hash, ok := objInfo.UserMetadata["hash"]; ok {
			metadata.Hash = hash
		}
		// Extract created_at from metadata if available
		if createdAtStr, ok := objInfo.UserMetadata["created_at"]; ok {
			if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
				metadata.CreatedAt = createdAt
			}
		}
		// Extract upload_completed_at and vm_ack_at for retention
		if uploadCompletedAtStr, ok := objInfo.UserMetadata["upload_completed_at"]; ok {
			if uploadCompletedAt, err := time.Parse(time.RFC3339, uploadCompletedAtStr); err == nil {
				metadata.UploadCompletedAt = &uploadCompletedAt
			}
		}
		if vmAckAtStr, ok := objInfo.UserMetadata["vm_ack_at"]; ok && vmAckAtStr != "" {
			if vmAckAt, err := time.Parse(time.RFC3339, vmAckAtStr); err == nil {
				metadata.VMAckAt = &vmAckAt
			}
		}
	}

	return metadata, nil
}

// HealthCheck performs a health check on the MinIO provider.
// Returns an error if the provider is unhealthy.
func (p *MinIOProvider) HealthCheck(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("MinIO client is not initialized")
	}

	// Check if bucket exists and is accessible
	exists, err := p.client.BucketExists(ctx, p.bucket)
	if err != nil {
		return fmt.Errorf("MinIO health check failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("MinIO bucket %s does not exist", p.bucket)
	}

	return nil
}

// Close closes the MinIO provider and releases resources.
// Note: MinIO client doesn't require explicit closing, but we provide this for interface compliance.
func (p *MinIOProvider) Close() error {
	// MinIO client doesn't have a Close method
	// Connection pooling is handled internally by the MinIO SDK
	return nil
}

