package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
)

func TestMigrator(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.DefaultConfig(dbPath)
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create migrator
	migrator := NewMigrator(db)

	// Test initial version
	version, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 0 {
		t.Fatalf("Expected version 0, got %d", version)
	}

	// Apply migrations
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Verify version
	version, err = migrator.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 1 {
		t.Fatalf("Expected version 1, got %d", version)
	}

	// Verify tables were created
	tables := []string{"edges", "events", "training_datasets", "ai_models", "cid_storage", "telemetry_buffer"}
	for _, table := range tables {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
		if err != nil {
			t.Fatalf("Table %s was not created: %v", table, err)
		}
	}
}

func TestMigratorStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.DefaultConfig(dbPath)
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Check status before migration
	currentVersion, pendingCount, err := migrator.Status(ctx)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}
	if currentVersion != 0 {
		t.Fatalf("Expected current version 0, got %d", currentVersion)
	}
	if pendingCount == 0 {
		t.Fatal("Expected pending migrations")
	}

	// Apply migrations
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Check status after migration
	currentVersion, pendingCount, err = migrator.Status(ctx)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}
	if currentVersion != 1 {
		t.Fatalf("Expected current version 1, got %d", currentVersion)
	}
	if pendingCount != 0 {
		t.Fatalf("Expected no pending migrations, got %d", pendingCount)
	}
}

func TestMigratorDown(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.DefaultConfig(dbPath)
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Apply migrations
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Verify version
	version, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 1 {
		t.Fatalf("Expected version 1, got %d", version)
	}

	// Rollback migration
	if err := migrator.Down(ctx); err != nil {
		t.Fatalf("Failed to rollback migration: %v", err)
	}

	// Verify version
	version, err = migrator.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 0 {
		t.Fatalf("Expected version 0 after rollback, got %d", version)
	}
}

func TestMigratorDownNoMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.DefaultConfig(dbPath)
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Try to rollback when no migrations applied
	err = migrator.Down(ctx)
	if err == nil {
		t.Fatal("Expected error when rolling back with no migrations")
	}
}

func TestMultipleMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.DefaultConfig(dbPath)
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Apply migrations multiple times (should be idempotent)
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	version, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 1 {
		t.Fatalf("Expected version 1, got %d", version)
	}

	// Apply again (should not change)
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	version, err = migrator.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if version != 1 {
		t.Fatalf("Expected version 1, got %d", version)
	}
}

