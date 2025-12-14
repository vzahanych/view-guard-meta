package storagesync

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

// S3Client wraps MinIO client for S3-compatible storage operations
type S3Client struct {
	client     *minio.Client
	bucketName string
	logger     *zap.Logger
	endpoint   string
	useSSL     bool
}

// S3Config contains S3/MinIO configuration
type S3Config struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	BucketName string
	UseSSL     bool
	Region     string // Optional, defaults to "us-east-1" for MinIO
	CACertPath string // Optional, path to CA certificate for TLS verification
}

// NewS3Client creates a new S3/MinIO client
func NewS3Client(cfg S3Config, logger *zap.Logger) (*S3Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("S3 endpoint is required")
	}
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("S3 access key is required")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("S3 secret key is required")
	}
	if cfg.BucketName == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	// Configure TLS if using SSL
	var transport *http.Transport
	if cfg.UseSSL {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		// Load custom CA certificate if provided (for self-signed certificates)
		if cfg.CACertPath != "" {
			caCert, err := os.ReadFile(cfg.CACertPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA certificate: %w", err)
			}

			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}

			tlsConfig.RootCAs = caCertPool
		}

		// Create custom HTTP transport with TLS config
		transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	// Initialize MinIO client
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: func() string {
			if cfg.Region != "" {
				return cfg.Region
			}
			return "us-east-1" // Default region for MinIO
		}(),
	}

	// Set custom transport if TLS is enabled
	if transport != nil {
		opts.Transport = transport
	}

	client, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	s3Client := &S3Client{
		client:     client,
		bucketName: cfg.BucketName,
		logger:     logger,
		endpoint:   cfg.Endpoint,
		useSSL:     cfg.UseSSL,
	}

	// Ensure bucket exists
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, cfg.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, cfg.BucketName, minio.MakeBucketOptions{
			Region: func() string {
				if cfg.Region != "" {
					return cfg.Region
				}
				return "us-east-1"
			}(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		logger.Info("Created S3 bucket", zap.String("bucket", cfg.BucketName))
	}

	return s3Client, nil
}

// UploadFile uploads a file to S3/MinIO
func (c *S3Client) UploadFile(ctx context.Context, objectKey string, file io.Reader, size int64, contentType string) error {
	if objectKey == "" {
		return fmt.Errorf("object key is required")
	}

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	_, err := c.client.PutObject(ctx, c.bucketName, objectKey, file, size, opts)
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	c.logger.Info("Uploaded file to S3",
		zap.String("bucket", c.bucketName),
		zap.String("object_key", objectKey),
		zap.Int64("size", size),
	)

	return nil
}

// DownloadFile downloads a file from S3/MinIO
func (c *S3Client) DownloadFile(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if objectKey == "" {
		return nil, fmt.Errorf("object key is required")
	}

	object, err := c.client.GetObject(ctx, c.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file from S3: %w", err)
	}

	return object, nil
}

// FileExists checks if a file exists in S3/MinIO
func (c *S3Client) FileExists(ctx context.Context, objectKey string) (bool, error) {
	if objectKey == "" {
		return false, fmt.Errorf("object key is required")
	}

	_, err := c.client.StatObject(ctx, c.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// DeleteFile deletes a file from S3/MinIO
func (c *S3Client) DeleteFile(ctx context.Context, objectKey string) error {
	if objectKey == "" {
		return fmt.Errorf("object key is required")
	}

	err := c.client.RemoveObject(ctx, c.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete file from S3: %w", err)
	}

	c.logger.Info("Deleted file from S3",
		zap.String("bucket", c.bucketName),
		zap.String("object_key", objectKey),
	)

	return nil
}

// GetFileSize gets the size of a file in S3/MinIO
func (c *S3Client) GetFileSize(ctx context.Context, objectKey string) (int64, error) {
	if objectKey == "" {
		return 0, fmt.Errorf("object key is required")
	}

	info, err := c.client.StatObject(ctx, c.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get file size: %w", err)
	}

	return info.Size, nil
}
