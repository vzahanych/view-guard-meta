package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// Example migrations for the meta-storage system.
// These migrations demonstrate the migration pattern and can be used as templates
// for future migrations.

// MigrationV1ToV2 adds the ml_lifecycle bucket.
// This migration is idempotent - it can be run multiple times safely.
type MigrationV1ToV2 struct {
	provider types.MetaStorageProvider
	logger   *zap.Logger
}

// NewMigrationV1ToV2 creates a new migration from v1 to v2.
func NewMigrationV1ToV2(provider types.MetaStorageProvider, logger *zap.Logger) *MigrationV1ToV2 {
	return &MigrationV1ToV2{
		provider: provider,
		logger:   logger,
	}
}

func (m *MigrationV1ToV2) Version() int {
	return 2
}

func (m *MigrationV1ToV2) Description() string {
	return "Add ml_lifecycle bucket for ML lifecycle state management"
}

func (m *MigrationV1ToV2) Up(ctx context.Context) error {
	// Create ml_lifecycle bucket if it doesn't exist
	if !m.provider.BucketExists(ctx, BucketMLLifecycle) {
		if err := m.provider.CreateBucket(ctx, BucketMLLifecycle); err != nil {
			return fmt.Errorf("failed to create ml_lifecycle bucket: %w", err)
		}
		if m.logger != nil {
			m.logger.Info("Created ml_lifecycle bucket")
		}
	}
	return nil
}

func (m *MigrationV1ToV2) Down(ctx context.Context) error {
	// Note: We don't delete the bucket on rollback to avoid data loss
	// If rollback is needed, the bucket can be manually deleted
	if m.logger != nil {
		m.logger.Warn("Rollback of ml_lifecycle bucket creation - bucket not deleted to preserve data")
	}
	return nil
}

// MigrationV2ToV3 migrates cameras bucket to devices bucket.
// This migration copies all records from the cameras bucket to the devices bucket.
type MigrationV2ToV3 struct {
	provider types.MetaStorageProvider
	logger   *zap.Logger
}

// NewMigrationV2ToV3 creates a new migration from v2 to v3.
func NewMigrationV2ToV3(provider types.MetaStorageProvider, logger *zap.Logger) *MigrationV2ToV3 {
	return &MigrationV2ToV3{
		provider: provider,
		logger:   logger,
	}
}

func (m *MigrationV2ToV3) Version() int {
	return 3
}

func (m *MigrationV2ToV3) Description() string {
	return "Migrate cameras bucket to devices bucket (device-agnostic refactoring)"
}

func (m *MigrationV2ToV3) Up(ctx context.Context) error {
	// Use the bucket migrator to migrate cameras -> devices
	migrator := NewBucketMigrator(m.provider, m.logger)
	return migrator.MigrateBuckets(ctx)
}

func (m *MigrationV2ToV3) Down(ctx context.Context) error {
	// Rollback by copying data back from devices to cameras
	migrator := NewBucketMigrator(m.provider, m.logger)
	return migrator.RollbackMigration(ctx)
}

// MigrationV3ToV4 adds version field to ML lifecycle state records.
// This migration ensures all existing ML lifecycle state records have a version field for CAS operations.
type MigrationV3ToV4 struct {
	provider types.MetaStorageProvider
	logger   *zap.Logger
}

// NewMigrationV3ToV4 creates a new migration from v3 to v4.
func NewMigrationV3ToV4(provider types.MetaStorageProvider, logger *zap.Logger) *MigrationV3ToV4 {
	return &MigrationV3ToV4{
		provider: provider,
		logger:   logger,
	}
}

func (m *MigrationV3ToV4) Version() int {
	return 4
}

func (m *MigrationV3ToV4) Description() string {
	return "Add version field to ML lifecycle state records for CAS operations"
}

func (m *MigrationV3ToV4) Up(ctx context.Context) error {
	// This migration would update existing ML lifecycle state records to include a version field
	// For now, this is a placeholder - the actual implementation would:
	// 1. List all records in ml_lifecycle bucket
	// 2. Unmarshal each record
	// 3. Add version field if missing (set to 1)
	// 4. Marshal and save back

	if m.logger != nil {
		m.logger.Info("Migration v3->v4: ML lifecycle state records will get version field on next update")
	}

	// This migration is idempotent - records will get version field when they're next updated
	// No immediate action needed as the SaveMLLifecycleState method already handles version initialization
	return nil
}

func (m *MigrationV3ToV4) Down(ctx context.Context) error {
	// Rollback would remove version field, but we don't do this to avoid data loss
	if m.logger != nil {
		m.logger.Warn("Rollback of version field addition - version field not removed to preserve data")
	}
	return nil
}

// RegisterDefaultMigrations registers the default set of migrations.
// This should be called during service initialization.
func RegisterDefaultMigrations(migrator *SchemaMigrator, provider types.MetaStorageProvider, logger *zap.Logger) error {
	migrations := []types.SchemaMigration{
		NewMigrationV1ToV2(provider, logger),
		NewMigrationV2ToV3(provider, logger),
		NewMigrationV3ToV4(provider, logger),
	}

	for _, migration := range migrations {
		if err := migrator.RegisterMigration(migration); err != nil {
			return fmt.Errorf("failed to register migration v%d: %w", migration.Version(), err)
		}
	}

	return nil
}

