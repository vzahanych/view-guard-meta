package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database manages the SQLite database for state persistence
type Database struct {
	db     *sql.DB
	dbPath string
}

// NewDatabase creates a new database connection
func NewDatabase(dbPath string) (*Database, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes well
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	database := &Database{
		db:     db,
		dbPath: dbPath,
	}

	// Initialize schema
	if err := database.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return database, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// GetDB returns the underlying database connection
func (d *Database) GetDB() *sql.DB {
	return d.db
}

// initSchema initializes the database schema
func (d *Database) initSchema() error {
	schema := `
	-- System state table
	CREATE TABLE IF NOT EXISTS system_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Cameras table
	CREATE TABLE IF NOT EXISTS cameras (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		rtsp_url TEXT NOT NULL,
		enabled BOOLEAN DEFAULT 1,
		last_seen TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Events table (for local event queue and history)
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		camera_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		timestamp TIMESTAMP NOT NULL,
		metadata TEXT, -- JSON metadata
		clip_path TEXT,
		snapshot_path TEXT,
		transmitted BOOLEAN DEFAULT 0,
		transmitted_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
	);

	-- Event queue table (for pending transmission)
	CREATE TABLE IF NOT EXISTS event_queue (
		id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		priority INTEGER DEFAULT 0,
		retry_count INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
	);

	-- Telemetry table (for buffering telemetry data)
	CREATE TABLE IF NOT EXISTS telemetry (
		id TEXT PRIMARY KEY,
		metric_name TEXT NOT NULL,
		metric_value REAL,
		metric_data TEXT, -- JSON for complex metrics
		timestamp TIMESTAMP NOT NULL,
		transmitted BOOLEAN DEFAULT 0,
		transmitted_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Storage state table
	CREATE TABLE IF NOT EXISTS storage_state (
		path TEXT PRIMARY KEY,
		file_type TEXT NOT NULL, -- 'clip' or 'snapshot'
		size_bytes INTEGER NOT NULL,
		camera_id TEXT,
		event_id TEXT,
		created_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP,
		FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE SET NULL,
		FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE SET NULL
	);

	-- Labeled screenshots table (for training data)
	CREATE TABLE IF NOT EXISTS labeled_screenshots (
		id TEXT PRIMARY KEY,
		camera_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		label TEXT NOT NULL, -- 'normal', 'threat', 'abnormal', 'custom'
		custom_label TEXT, -- For custom user-defined labels
		description TEXT, -- User description/notes
		metadata TEXT, -- JSON metadata (e.g., bounding boxes, confidence scores)
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		created_by TEXT, -- User/system identifier
		FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_events_camera_timestamp ON events(camera_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_events_transmitted ON events(transmitted, timestamp);
	CREATE INDEX IF NOT EXISTS idx_event_queue_priority ON event_queue(priority DESC, created_at);
	CREATE INDEX IF NOT EXISTS idx_telemetry_transmitted ON telemetry(transmitted, timestamp);
	CREATE INDEX IF NOT EXISTS idx_storage_expires ON storage_state(expires_at);
	CREATE INDEX IF NOT EXISTS idx_storage_camera ON storage_state(camera_id);
	CREATE INDEX IF NOT EXISTS idx_labeled_screenshots_camera ON labeled_screenshots(camera_id);
	CREATE INDEX IF NOT EXISTS idx_labeled_screenshots_label ON labeled_screenshots(label);
	CREATE INDEX IF NOT EXISTS idx_labeled_screenshots_created ON labeled_screenshots(created_at);
	`

	if _, err := d.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// ensureDir ensures a directory exists
func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}
