package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	Up          func(*sql.Tx) error
	Down        func(*sql.Tx) error
}

// Migrator handles database migrations
type Migrator struct {
	db        *database.DB
	migrations []Migration
}

// NewMigrator creates a new migrator
func NewMigrator(db *database.DB) *Migrator {
	return &Migrator{
		db:        db,
		migrations: getMigrations(),
	}
}

// getMigrations returns all migrations in order
func getMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Description: "Initial schema - create all tables",
			Up: func(tx *sql.Tx) error {
				tables := database.AllTables()
				for _, tableSQL := range tables {
					if _, err := tx.Exec(tableSQL); err != nil {
						return fmt.Errorf("failed to create table: %w", err)
					}
				}
				return nil
			},
			Down: func(tx *sql.Tx) error {
				// Drop tables in reverse order (respecting foreign keys)
				tables := []string{
					"DROP TABLE IF EXISTS telemetry_buffer",
					"DROP TABLE IF EXISTS cid_storage",
					"DROP TABLE IF EXISTS ai_models",
					"DROP TABLE IF EXISTS training_datasets",
					"DROP TABLE IF EXISTS events",
					"DROP TABLE IF EXISTS edges",
				}
				for _, dropSQL := range tables {
					if _, err := tx.Exec(dropSQL); err != nil {
						return fmt.Errorf("failed to drop table: %w", err)
					}
				}
				return nil
			},
		},
		{
			Version:     2,
			Description: "Add edge camera status table",
			Up: func(tx *sql.Tx) error {
				if _, err := tx.Exec(database.CreateEdgeCameraStatusTable); err != nil {
					return fmt.Errorf("failed to create edge_camera_status table: %w", err)
				}
				return nil
			},
			Down: func(tx *sql.Tx) error {
				if _, err := tx.Exec("DROP TABLE IF EXISTS edge_camera_status"); err != nil {
					return fmt.Errorf("failed to drop edge_camera_status table: %w", err)
				}
				return nil
			},
		},
	}
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist
func (m *Migrator) ensureMigrationsTable(ctx context.Context) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at INTEGER NOT NULL
	);
	`
	_, err := m.db.ExecContext(ctx, createTableSQL)
	return err
}

// GetCurrentVersion returns the current migration version
func (m *Migrator) GetCurrentVersion(ctx context.Context) (int, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return 0, fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	var version sql.NullInt64
	err := m.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get current version: %w", err)
	}

	if !version.Valid {
		return 0, nil
	}

	return int(version.Int64), nil
}

// Up applies all pending migrations
func (m *Migrator) Up(ctx context.Context) error {
	currentVersion, err := m.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	for _, migration := range m.migrations {
		if migration.Version <= currentVersion {
			continue
		}

		// Begin transaction
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		// Apply migration
		if err := migration.Up(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		// Record migration
		now := time.Now().Unix()
		_, err = tx.Exec(
			"INSERT INTO schema_migrations (version, description, applied_at) VALUES (?, ?, ?)",
			migration.Version,
			migration.Description,
			now,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

// Down rolls back the last migration
func (m *Migrator) Down(ctx context.Context) error {
	currentVersion, err := m.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	// Find the last migration
	var lastMigration *Migration
	for i := len(m.migrations) - 1; i >= 0; i-- {
		if m.migrations[i].Version == currentVersion {
			lastMigration = &m.migrations[i]
			break
		}
	}

	if lastMigration == nil {
		return fmt.Errorf("migration version %d not found", currentVersion)
	}

	// Begin transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Rollback migration
	if err := lastMigration.Down(tx); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to rollback migration %d: %w", lastMigration.Version, err)
	}

	// Remove migration record
	_, err = tx.Exec("DELETE FROM schema_migrations WHERE version = ?", currentVersion)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to remove migration record %d: %w", currentVersion, err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback: %w", err)
	}

	return nil
}

// Status returns migration status information
func (m *Migrator) Status(ctx context.Context) (currentVersion int, pendingCount int, err error) {
	currentVersion, err = m.GetCurrentVersion(ctx)
	if err != nil {
		return 0, 0, err
	}

	pendingCount = 0
	for _, migration := range m.migrations {
		if migration.Version > currentVersion {
			pendingCount++
		}
	}

	return currentVersion, pendingCount, nil
}

