package types

import (
	"context"
)

// KeyValue represents a key-value pair in storage.
type KeyValue struct {
	Key   []byte
	Value []byte
}

// MetaStorageProvider defines the provider-agnostic interface for storage operations.
// This interface abstracts away the details of specific storage backends (BoltDB, SQLite, PostgreSQL, etc.)
// and allows the storage service to work with any provider implementation.
//
// Provider implementations should be stateless and thread-safe.
// The storage service manages provider lifecycle and connection pooling.
//
// All methods must be safe for concurrent use. Provider implementations should use
// appropriate synchronization mechanisms (transactions, locks, etc.) to ensure thread safety.
type MetaStorageProvider interface {
	// CreateBucket creates a new bucket/namespace in storage.
	// If the bucket already exists, this should be a no-op (idempotent).
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - name: The name of the bucket to create
	//
	// Returns an error if:
	//   - The bucket name is invalid
	//   - The provider operation fails
	CreateBucket(ctx context.Context, name string) error

	// DeleteBucket deletes a bucket/namespace from storage.
	// If the bucket does not exist, this should return an error.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - name: The name of the bucket to delete
	//
	// Returns an error if:
	//   - The bucket does not exist
	//   - The provider operation fails
	DeleteBucket(ctx context.Context, name string) error

	// BucketExists checks if a bucket/namespace exists in storage.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - name: The name of the bucket to check
	//
	// Returns:
	//   - true if the bucket exists
	//   - false if the bucket does not exist
	BucketExists(ctx context.Context, name string) bool

	// Put stores a key-value pair in the specified bucket.
	// If the key already exists, it should be overwritten.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - bucket: The name of the bucket
	//   - key: The key to store (must not be nil or empty)
	//   - value: The value to store (can be nil or empty)
	//
	// Returns an error if:
	//   - The bucket does not exist
	//   - The key is nil or empty
	//   - The provider operation fails
	Put(ctx context.Context, bucket string, key []byte, value []byte) error

	// Get retrieves a value by key from the specified bucket.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - bucket: The name of the bucket
	//   - key: The key to retrieve (must not be nil or empty)
	//
	// Returns:
	//   - The value as a byte slice (copied, safe to modify)
	//   - An error if:
	//     - The bucket does not exist
	//     - The key does not exist
	//     - The key is nil or empty
	//     - The provider operation fails
	//
	// Note: The returned value is a copy and can be safely modified by the caller.
	Get(ctx context.Context, bucket string, key []byte) ([]byte, error)

	// Delete removes a key-value pair from the specified bucket.
	// If the key does not exist, this should be a no-op (idempotent).
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - bucket: The name of the bucket
	//   - key: The key to delete (must not be nil or empty)
	//
	// Returns an error if:
	//   - The bucket does not exist
	//   - The provider operation fails
	Delete(ctx context.Context, bucket string, key []byte) error

	// List lists all key-value pairs in the specified bucket with the given prefix.
	// If prefix is empty, all keys are returned.
	// The results are returned in key order (provider-specific, typically lexicographic).
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - bucket: The name of the bucket
	//   - prefix: The key prefix to filter by (nil or empty for all keys)
	//
	// Returns:
	//   - A slice of KeyValue pairs matching the prefix (keys and values are copied, safe to modify)
	//   - An error if:
	//     - The bucket does not exist
	//     - The provider operation fails
	//
	// Note: The returned keys and values are copies and can be safely modified by the caller.
	List(ctx context.Context, bucket string, prefix []byte) ([]KeyValue, error)

	// HealthCheck performs a health check on the storage provider.
	// This should verify connectivity, accessibility, and basic operations.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns an error if:
	//   - The provider is not initialized
	//   - The provider cannot be accessed
	//   - A basic operation fails (e.g., cannot create a transaction)
	//
	// This method should be lightweight and fast, suitable for frequent health checks.
	HealthCheck(ctx context.Context) error
}

// BoltDBConfig contains BoltDB-specific configuration.
// This is used when the provider is "bbolt".
type BoltDBConfig struct {
	// DataDir is the directory where the database file will be stored
	DataDir string

	// DatabaseFile is the name of the database file (default: "meta.db")
	DatabaseFile string

	// FileMode is the file mode for the database file (default: 0600)
	FileMode uint32

	// Timeout is the timeout for database operations (default: 1 second)
	Timeout int64 // in seconds

	// NoSync disables fsync after each write (for performance, default: false)
	NoSync bool
}

// SQLiteConfig contains SQLite-specific configuration.
// This is used when the provider is "sqlite" (future).
type SQLiteConfig struct {
	// DataDir is the directory where the database file will be stored
	DataDir string

	// DatabaseFile is the name of the database file (default: "meta.db")
	DatabaseFile string

	// FileMode is the file mode for the database file (default: 0600)
	FileMode uint32

	// Timeout is the timeout for database operations (default: 1 second)
	Timeout int64 // in seconds
}

// PostgreSQLConfig contains PostgreSQL-specific configuration.
// This is used when the provider is "postgres" (future).
type PostgreSQLConfig struct {
	// Endpoint is the PostgreSQL connection string (e.g., "postgresql://localhost:5432/viewguard")
	Endpoint string

	// Username is the database username
	Username string

	// Password is the database password
	Password string

	// Database is the database name
	Database string

	// MaxConnections is the maximum number of connections in the pool (default: 10)
	MaxConnections int

	// Timeout is the timeout for database operations (default: 5 seconds)
	Timeout int64 // in seconds
}

// StorageEventEmitter is an interface for emitting storage-related events.
// This interface is implemented by the event-bus package to avoid import cycles.
// The event-bus package depends on meta-storage, so meta-storage cannot import event-bus directly.
type StorageEventEmitter interface {
	// EmitStorageEvent emits a storage event with the given type and data.
	// The eventType should be one of the storage event types (e.g., "storage.warning", "storage.full").
	// The data should be a JSON-marshalable struct containing event-specific information.
	EmitStorageEvent(eventType string, data interface{})
}

