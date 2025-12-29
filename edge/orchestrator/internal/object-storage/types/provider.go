package types

import (
	"context"
	"io"
)

// ObjectInfo represents information about an object in storage.
type ObjectInfo struct {
	// Key is the object key/path
	Key string

	// Size is the object size in bytes
	Size int64

	// ContentType is the MIME type
	ContentType string

	// LastModified is when the object was last modified
	LastModified int64

	// ETag is the object ETag (for integrity verification)
	ETag string
}

// ObjectStorageProvider defines the provider-agnostic interface for object storage operations.
// This interface abstracts away the details of specific storage backends (MinIO, S3, filesystem, etc.)
// and allows the storage service to work with any provider implementation.
//
// Provider implementations should be stateless and thread-safe.
// The storage service manages provider lifecycle and connection pooling.
//
// All methods must be safe for concurrent use. Provider implementations should use
// appropriate synchronization mechanisms to ensure thread safety.
type ObjectStorageProvider interface {
	// StoreObject stores an object in storage.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object key/path
	//   - r: Reader containing the object data
	//   - size: The size of the object in bytes
	//   - contentType: The MIME type of the object
	//   - metadata: Optional metadata map (provider-specific)
	//
	// Returns an error if:
	//   - The key is invalid
	//   - The provider operation fails
	StoreObject(ctx context.Context, key string, r io.Reader, size int64, contentType string, metadata map[string]string) error

	// LoadObject retrieves an object from storage.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object key/path
	//
	// Returns:
	//   - io.ReadCloser containing the object data (caller must close)
	//   - An error if the object is not found or the operation fails
	LoadObject(ctx context.Context, key string) (io.ReadCloser, error)

	// DeleteObject deletes an object from storage.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object key/path
	//
	// Returns an error if:
	//   - The object does not exist (should be idempotent)
	//   - The provider operation fails
	DeleteObject(ctx context.Context, key string) error

	// ListObjects lists objects in storage with the given prefix.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - prefix: The key prefix to filter by (empty string for all objects)
	//
	// Returns:
	//   - Slice of ObjectInfo for matching objects
	//   - An error if the operation fails
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// GetObjectMetadata retrieves metadata for an object.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - key: The object key/path
	//
	// Returns:
	//   - ObjectMetadata containing object metadata
	//   - An error if the object is not found or the operation fails
	GetObjectMetadata(ctx context.Context, key string) (*ObjectMetadata, error)

	// HealthCheck performs a health check on the storage provider.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns:
	//   - An error if the provider is unhealthy
	//   - nil if the provider is healthy
	HealthCheck(ctx context.Context) error

	// Close closes the provider and releases resources.
	// This should be called when the provider is no longer needed.
	Close() error
}

