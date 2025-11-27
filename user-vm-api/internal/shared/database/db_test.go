package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test ping
	ctx := context.Background()
	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}
}

func TestHealthCheck(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.HealthCheck(ctx); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

func TestInitializeSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitializeSchema(ctx); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Verify tables were created by querying them
	tables := []string{"edges", "events", "training_datasets", "ai_models", "cid_storage", "telemetry_buffer"}
	for _, table := range tables {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
		if err != nil {
			t.Fatalf("Table %s was not created: %v", table, err)
		}
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Close database
	if err := db.Close(); err != nil {
		t.Fatalf("Failed to close database: %v", err)
	}

	// Try to ping closed database
	ctx := context.Background()
	if err := db.Ping(ctx); err == nil {
		t.Fatal("Expected error when pinging closed database")
	}
}

func TestExecContext(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitializeSchema(ctx); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Insert test edge
	edgeID := "test-edge-1"
	now := time.Now().Unix()
	_, err = db.ExecContext(ctx,
		"INSERT INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		edgeID, "Test Edge", "test-public-key", now, "active", now, now)
	if err != nil {
		t.Fatalf("Failed to insert edge: %v", err)
	}

	// Verify insertion
	var name string
	err = db.QueryRowContext(ctx, "SELECT name FROM edges WHERE edge_id = ?", edgeID).Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query edge: %v", err)
	}
	if name != "Test Edge" {
		t.Fatalf("Expected name 'Test Edge', got '%s'", name)
	}
}

func TestQueryContext(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitializeSchema(ctx); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Insert test data
	now := time.Now().Unix()
	_, err = db.ExecContext(ctx,
		"INSERT INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"edge-1", "Edge 1", "key-1", now, "active", now, now)
	if err != nil {
		t.Fatalf("Failed to insert edge: %v", err)
	}

	_, err = db.ExecContext(ctx,
		"INSERT INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"edge-2", "Edge 2", "key-2", now, "active", now, now)
	if err != nil {
		t.Fatalf("Failed to insert edge: %v", err)
	}

	// Query edges
	rows, err := db.QueryContext(ctx, "SELECT edge_id, name FROM edges ORDER BY edge_id")
	if err != nil {
		t.Fatalf("Failed to query edges: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var edgeID, name string
		if err := rows.Scan(&edgeID, &name); err != nil {
			t.Fatalf("Failed to scan row: %v", err)
		}
		count++
	}

	if count != 2 {
		t.Fatalf("Expected 2 edges, got %d", count)
	}
}

func TestBeginTx(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitializeSchema(ctx); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Insert in transaction
	now := time.Now().Unix()
	_, err = tx.Exec("INSERT INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"tx-edge", "TX Edge", "tx-key", now, "active", now, now)
	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify insertion
	var name string
	err = db.QueryRowContext(ctx, "SELECT name FROM edges WHERE edge_id = ?", "tx-edge").Scan(&name)
	if err != nil {
		t.Fatalf("Failed to query edge: %v", err)
	}
	if name != "TX Edge" {
		t.Fatalf("Expected name 'TX Edge', got '%s'", name)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("/tmp/test.db")
	if cfg.DatabasePath != "/tmp/test.db" {
		t.Fatalf("Expected database path '/tmp/test.db', got '%s'", cfg.DatabasePath)
	}
	if cfg.MaxOpenConns == 0 {
		t.Fatal("MaxOpenConns should be set")
	}
	if cfg.MaxIdleConns == 0 {
		t.Fatal("MaxIdleConns should be set")
	}
}

func TestDatabaseFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig(dbPath)
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("Database file was not created: %v", err)
	}
}

