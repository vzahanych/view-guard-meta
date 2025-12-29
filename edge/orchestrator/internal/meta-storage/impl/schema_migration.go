package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

const (
	// SchemaVersionKey is the key used to store the current schema version in the _meta bucket
	SchemaVersionKey = "schema_version"
)

// SchemaMigrator manages schema migrations for the storage system.
// It tracks registered migrations, applies them in order, and maintains the current schema version.
type SchemaMigrator struct {
	provider     types.MetaStorageProvider
	logger       *zap.Logger
	migrations   []types.SchemaMigration
	eventEmitter types.StorageEventEmitter // Optional event emitter for emitting migration events
}

// NewSchemaMigrator creates a new schema migrator.
func NewSchemaMigrator(provider types.MetaStorageProvider, logger *zap.Logger) *SchemaMigrator {
	return &SchemaMigrator{
		provider:   provider,
		logger:     logger,
		migrations: make([]types.SchemaMigration, 0),
	}
}

// SetEventEmitter sets the event emitter for this schema migrator.
// This is optional - if not set, events will not be emitted.
func (m *SchemaMigrator) SetEventEmitter(eventEmitter types.StorageEventEmitter) {
	m.eventEmitter = eventEmitter
}

// RegisterMigration registers a migration with the migrator.
// Migrations should be registered in order (from lowest version to highest).
// Duplicate versions are not allowed.
func (m *SchemaMigrator) RegisterMigration(migration types.SchemaMigration) error {
	// Check for duplicate versions
	for _, existing := range m.migrations {
		if existing.Version() == migration.Version() {
			return fmt.Errorf("duplicate migration version: %d", migration.Version())
		}
	}

	m.migrations = append(m.migrations, migration)

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version() < m.migrations[j].Version()
	})

	if m.logger != nil {
		m.logger.Info("Registered schema migration",
			zap.Int("version", migration.Version()),
			zap.String("description", migration.Description()))
	}

	return nil
}

// GetCurrentVersion returns the current schema version.
// Returns 0 if no schema version has been set (initial state).
func (m *SchemaMigrator) GetCurrentVersion(ctx context.Context) (int, error) {
	data, err := m.provider.Get(ctx, BucketMeta, []byte(SchemaVersionKey))
	if err != nil {
		// If key doesn't exist, return version 0 (initial state)
		return 0, nil
	}

	var schemaVersion types.SchemaVersion
	if err := json.Unmarshal(data, &schemaVersion); err != nil {
		return 0, fmt.Errorf("failed to unmarshal schema version: %w", err)
	}

	return schemaVersion.Version, nil
}

// setCurrentVersion sets the current schema version.
func (m *SchemaMigrator) setCurrentVersion(ctx context.Context, version int, description string) error {
	schemaVersion := types.SchemaVersion{
		Version:     version,
		AppliedAt:   time.Now(),
		Description: description,
	}

	data, err := json.Marshal(schemaVersion)
	if err != nil {
		return fmt.Errorf("failed to marshal schema version: %w", err)
	}

	return m.provider.Put(ctx, BucketMeta, []byte(SchemaVersionKey), data)
}

// Migrate applies all pending migrations in order.
// This operation is idempotent - it only applies migrations that haven't been applied yet.
//
// Migration process:
// 1. Load current schema version
// 2. Find all migrations with version > current version
// 3. Apply each migration in order
// 4. Update schema version after each successful migration
// 5. Return error if any migration fails (partial migrations are not committed)
func (m *SchemaMigrator) Migrate(ctx context.Context) error {
	currentVersion, err := m.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	if m.logger != nil {
		m.logger.Info("Starting schema migration",
			zap.Int("current_version", currentVersion),
			zap.Int("registered_migrations", len(m.migrations)))
	}

	// Find pending migrations
	pendingMigrations := make([]types.SchemaMigration, 0)
	for _, migration := range m.migrations {
		if migration.Version() > currentVersion {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}

	if len(pendingMigrations) == 0 {
		if m.logger != nil {
			m.logger.Info("No pending migrations, schema is up to date",
				zap.Int("current_version", currentVersion))
		}
		return nil
	}

	if m.logger != nil {
		m.logger.Info("Found pending migrations",
			zap.Int("current_version", currentVersion),
			zap.Int("pending_count", len(pendingMigrations)),
			zap.Int("target_version", pendingMigrations[len(pendingMigrations)-1].Version()))
	}

	// Apply each migration in order
	for _, migration := range pendingMigrations {
		if m.logger != nil {
			m.logger.Info("Applying migration",
				zap.Int("version", migration.Version()),
				zap.String("description", migration.Description()))
		}

		// Emit migration started event
		if m.eventEmitter != nil {
			m.eventEmitter.EmitStorageEvent("storage.schema_migration_started", map[string]interface{}{
				"schema_version": migration.Version(),
				"description":    migration.Description(),
			})
		}

		// Apply migration
		if err := migration.Up(ctx); err != nil {
			return fmt.Errorf("failed to apply migration v%d (%s): %w",
				migration.Version(), migration.Description(), err)
		}

		// Update schema version after successful migration
		if err := m.setCurrentVersion(ctx, migration.Version(), migration.Description()); err != nil {
			return fmt.Errorf("failed to update schema version after migration v%d: %w",
				migration.Version(), err)
		}

		// Emit migration completed event
		if m.eventEmitter != nil {
			m.eventEmitter.EmitStorageEvent("storage.schema_migration_completed", map[string]interface{}{
				"schema_version": migration.Version(),
				"description":    migration.Description(),
			})
		}

		if m.logger != nil {
			m.logger.Info("Migration applied successfully",
				zap.Int("version", migration.Version()),
				zap.String("description", migration.Description()))
		}
	}

	if m.logger != nil {
		m.logger.Info("Schema migration completed",
			zap.Int("final_version", pendingMigrations[len(pendingMigrations)-1].Version()))
	}

	return nil
}

// Rollback rolls back the last applied migration.
// This is useful for undoing a migration if it causes issues.
// WARNING: Rollback should be used with caution and only when necessary.
func (m *SchemaMigrator) Rollback(ctx context.Context) error {
	currentVersion, err := m.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to rollback (schema version is 0)")
	}

	// Find the migration for the current version
	var currentMigration types.SchemaMigration
	for _, migration := range m.migrations {
		if migration.Version() == currentVersion {
			currentMigration = migration
			break
		}
	}

	if currentMigration == nil {
		return fmt.Errorf("migration for version %d not found", currentVersion)
	}

	if m.logger != nil {
		m.logger.Info("Rolling back migration",
			zap.Int("version", currentVersion),
			zap.String("description", currentMigration.Description()))
	}

	// Rollback migration
	if err := currentMigration.Down(ctx); err != nil {
		return fmt.Errorf("failed to rollback migration v%d (%s): %w",
			currentVersion, currentMigration.Description(), err)
	}

	// Find previous version
	previousVersion := 0
	for _, migration := range m.migrations {
		if migration.Version() < currentVersion && migration.Version() > previousVersion {
			previousVersion = migration.Version()
		}
	}

	// Update schema version to previous version
	description := "Rolled back from version " + fmt.Sprintf("%d", currentVersion)
	if previousVersion > 0 {
		// Find description of previous migration
		for _, migration := range m.migrations {
			if migration.Version() == previousVersion {
				description = migration.Description()
				break
			}
		}
	}

	if err := m.setCurrentVersion(ctx, previousVersion, description); err != nil {
		return fmt.Errorf("failed to update schema version after rollback: %w", err)
	}

	if m.logger != nil {
		m.logger.Info("Migration rolled back successfully",
			zap.Int("previous_version", previousVersion))
	}

	return nil
}

// GetMigrationHistory returns the history of applied migrations.
// This is useful for debugging and auditing.
func (m *SchemaMigrator) GetMigrationHistory(ctx context.Context) ([]types.SchemaVersion, error) {
	// For now, we only track the current version
	// In the future, we could maintain a history of all applied migrations
	currentVersion, err := m.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current schema version: %w", err)
	}

	if currentVersion == 0 {
		return []types.SchemaVersion{}, nil
	}

	// Get current schema version details
	data, err := m.provider.Get(ctx, BucketMeta, []byte(SchemaVersionKey))
	if err != nil {
		return nil, fmt.Errorf("failed to get schema version: %w", err)
	}

	var schemaVersion types.SchemaVersion
	if err := json.Unmarshal(data, &schemaVersion); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema version: %w", err)
	}

	return []types.SchemaVersion{schemaVersion}, nil
}

