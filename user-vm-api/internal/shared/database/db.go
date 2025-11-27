package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

// DB wraps a SQLite database connection with connection pool management
type DB struct {
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
}

// Config contains database configuration
type Config struct {
	DatabasePath string
	MaxOpenConns int           // Maximum open connections
	MaxIdleConns int           // Maximum idle connections
	ConnMaxLifetime time.Duration // Connection max lifetime
	ConnMaxIdleTime time.Duration // Connection max idle time
}

// DefaultConfig returns a default database configuration
func DefaultConfig(databasePath string) *Config {
	return &Config{
		DatabasePath:    databasePath,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 10 * time.Minute,
	}
}

// New creates a new database connection
func New(cfg *Config) (*DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("database config is required")
	}

	// Open database connection
	db, err := sql.Open("sqlite", cfg.DatabasePath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{
		db:     db,
		closed: false,
	}, nil
}

// GetDB returns the underlying *sql.DB connection
// Use with caution - prefer using methods on DB struct
func (d *DB) GetDB() *sql.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db
}

// Close closes the database connection
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}

	d.closed = true
	return d.db.Close()
}

// Ping checks if the database connection is alive
func (d *DB) Ping(ctx context.Context) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return fmt.Errorf("database is closed")
	}

	return d.db.PingContext(ctx)
}

// HealthCheck performs a health check on the database
func (d *DB) HealthCheck(ctx context.Context) error {
	if err := d.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Perform a simple query to verify database is responsive
	var result int
	err := d.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: %d", result)
	}

	return nil
}

// InitializeSchema creates all tables if they don't exist
func (d *DB) InitializeSchema(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return fmt.Errorf("database is closed")
	}

	tables := AllTables()
	for _, tableSQL := range tables {
		if _, err := d.db.ExecContext(ctx, tableSQL); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// ExecContext executes a query without returning rows
func (d *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return nil, fmt.Errorf("database is closed")
	}

	return d.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows
func (d *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return nil, fmt.Errorf("database is closed")
	}

	return d.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns a single row
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		// Return a row that will error on Scan
		return d.db.QueryRowContext(ctx, "SELECT 1 WHERE 1=0")
	}

	return d.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a new transaction
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.closed {
		return nil, fmt.Errorf("database is closed")
	}

	return d.db.BeginTx(ctx, opts)
}

