package bbolt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// BoltDBProvider implements the MetaStorageProvider interface using BoltDB.
// This provider is thread-safe and manages a single BoltDB database connection.
type BoltDBProvider struct {
	db       *bbolt.DB
	dbPath   string // Store database path for size calculation
	logger   *zap.Logger
}

// NewBoltDBProvider creates a new BoltDB provider instance.
// The provider opens the database file and initializes it.
func NewBoltDBProvider(ctx context.Context, cfg *types.BoltDBConfig, logger *zap.Logger) (*BoltDBProvider, error) {
	// Set defaults
	dbFile := cfg.DatabaseFile
	if dbFile == "" {
		dbFile = "meta.db"
	}
	dbPath := filepath.Join(cfg.DataDir, "db", dbFile)

	// Ensure parent directories exist (bbolt.Open does not create them)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}

	// Set file mode default
	fileMode := os.FileMode(cfg.FileMode)
	if fileMode == 0 {
		fileMode = 0600
	}

	// Set timeout default
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 1 * time.Second
	}

	opts := &bbolt.Options{
		Timeout: timeout,
		NoSync:  cfg.NoSync,
	}

	db, err := bbolt.Open(dbPath, fileMode, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB database: %w", err)
	}

	return &BoltDBProvider{
		db:     db,
		dbPath: dbPath,
		logger: logger,
	}, nil
}

// GetDatabasePath returns the path to the database file.
// This is used for health monitoring to calculate database size.
func (p *BoltDBProvider) GetDatabasePath() string {
	return p.dbPath
}

// CreateBucket creates a new bucket/namespace in storage.
// If the bucket already exists, this is a no-op (idempotent).
func (p *BoltDBProvider) CreateBucket(ctx context.Context, name string) error {
	return p.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", name, err)
		}
		return nil
	})
}

// DeleteBucket deletes a bucket/namespace from storage.
// If the bucket does not exist, this returns an error.
func (p *BoltDBProvider) DeleteBucket(ctx context.Context, name string) error {
	return p.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		if bucket == nil {
			return fmt.Errorf("bucket %s does not exist", name)
		}
		return tx.DeleteBucket([]byte(name))
	})
}

// BucketExists checks if a bucket/namespace exists in storage.
func (p *BoltDBProvider) BucketExists(ctx context.Context, name string) bool {
	var exists bool
	_ = p.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		exists = bucket != nil
		return nil
	})
	return exists
}

// Put stores a key-value pair in the specified bucket.
// If the key already exists, it is overwritten.
func (p *BoltDBProvider) Put(ctx context.Context, bucket string, key []byte, value []byte) error {
	return p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucket)
		}
		return b.Put(key, value)
	})
}

// Get retrieves a value by key from the specified bucket.
// Returns an error if the key does not exist.
func (p *BoltDBProvider) Get(ctx context.Context, bucket string, key []byte) ([]byte, error) {
	var value []byte
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucket)
		}
		v := b.Get(key)
		if v == nil {
			return fmt.Errorf("key not found in bucket %s", bucket)
		}
		// Copy the value since BoltDB returns a slice that may be reused
		value = make([]byte, len(v))
		copy(value, v)
		return nil
	})
	return value, err
}

// Delete removes a key-value pair from the specified bucket.
// If the key does not exist, this is a no-op (idempotent).
func (p *BoltDBProvider) Delete(ctx context.Context, bucket string, key []byte) error {
	return p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucket)
		}
		return b.Delete(key)
	})
}

// List lists all key-value pairs in the specified bucket with the given prefix.
// If prefix is empty, all keys are returned.
// The results are returned in key order (lexicographically sorted).
func (p *BoltDBProvider) List(ctx context.Context, bucket string, prefix []byte) ([]types.KeyValue, error) {
	var results []types.KeyValue
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s does not exist", bucket)
		}

		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && (len(prefix) == 0 || len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix)); k, v = c.Next() {
			// Copy key and value since BoltDB returns slices that may be reused
			keyCopy := make([]byte, len(k))
			copy(keyCopy, k)
			valueCopy := make([]byte, len(v))
			copy(valueCopy, v)
			results = append(results, types.KeyValue{
				Key:   keyCopy,
				Value: valueCopy,
			})
		}
		return nil
	})
	return results, err
}

// HealthCheck performs a health check on the storage provider.
// Returns an error if the provider is unhealthy.
func (p *BoltDBProvider) HealthCheck(ctx context.Context) error {
	// Check if database is accessible
	if p.db == nil {
		return fmt.Errorf("database is not initialized")
	}

	// Perform a simple read operation to verify database is accessible
	err := p.db.View(func(tx *bbolt.Tx) error {
		// Try to access a system bucket or perform a simple operation
		// Just verify the transaction can be created
		return nil
	})
	if err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	return nil
}

// Close closes the database connection and releases all resources.
func (p *BoltDBProvider) Close() error {
	if p.db == nil {
		return nil
	}
	return p.db.Close()
}

