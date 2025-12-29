package impl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// mockMigration is a test implementation of SchemaMigration
type mockMigration struct {
	version     int
	description string
	upFunc      func(ctx context.Context) error
	downFunc    func(ctx context.Context) error
}

func (m *mockMigration) Version() int {
	return m.version
}

func (m *mockMigration) Description() string {
	return m.description
}

func (m *mockMigration) Up(ctx context.Context) error {
	if m.upFunc != nil {
		return m.upFunc(ctx)
	}
	return nil
}

func (m *mockMigration) Down(ctx context.Context) error {
	if m.downFunc != nil {
		return m.downFunc(ctx)
	}
	return nil
}

func TestSchemaMigrator_RegisterMigration(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()

	migrator := NewSchemaMigrator(provider, logger)

	// Register first migration
	migration1 := &mockMigration{
		version:     1,
		description: "Test migration 1",
	}
	err := migrator.RegisterMigration(migration1)
	require.NoError(t, err)

	// Register second migration (should be sorted)
	migration2 := &mockMigration{
		version:     2,
		description: "Test migration 2",
	}
	err = migrator.RegisterMigration(migration2)
	require.NoError(t, err)

	// Try to register duplicate version
	err = migrator.RegisterMigration(migration1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestSchemaMigrator_GetCurrentVersion(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	migrator := NewSchemaMigrator(provider, logger)

	// Initially should be version 0
	version, err := migrator.GetCurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, version)

	// Set version manually
	schemaVersion := types.SchemaVersion{
		Version:     1,
		AppliedAt:   time.Now(),
		Description: "Test",
	}
	data, err := json.Marshal(schemaVersion)
	require.NoError(t, err)
	err = provider.CreateBucket(ctx, BucketMeta)
	require.NoError(t, err)
	err = provider.Put(ctx, BucketMeta, []byte(SchemaVersionKey), data)
	require.NoError(t, err)

	// Should now return version 1
	version, err = migrator.GetCurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestSchemaMigrator_Migrate(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	migrator := NewSchemaMigrator(provider, logger)

	// Register migrations that actually create buckets
	migration1 := &mockMigration{
		version:     1,
		description: "Create test bucket",
		upFunc: func(ctx context.Context) error {
			return provider.CreateBucket(ctx, "test_bucket")
		},
	}
	err := migrator.RegisterMigration(migration1)
	require.NoError(t, err)

	migration2 := &mockMigration{
		version:     2,
		description: "Create another bucket",
		upFunc: func(ctx context.Context) error {
			return provider.CreateBucket(ctx, "test_bucket2")
		},
	}
	err = migrator.RegisterMigration(migration2)
	require.NoError(t, err)

	// Run migrations
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify buckets were created
	exists := provider.BucketExists(ctx, "test_bucket")
	assert.True(t, exists)

	exists = provider.BucketExists(ctx, "test_bucket2")
	assert.True(t, exists)

	// Verify version was updated
	version, err := migrator.GetCurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, version)
}

func TestSchemaMigrator_MigrateOnlyNewMigrations(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	migrator := NewSchemaMigrator(provider, logger)

	// Set current version to 1
	schemaVersion := types.SchemaVersion{
		Version:     1,
		AppliedAt:   time.Now(),
		Description: "Initial",
	}
	data, err := json.Marshal(schemaVersion)
	require.NoError(t, err)
	err = provider.CreateBucket(ctx, BucketMeta)
	require.NoError(t, err)
	err = provider.Put(ctx, BucketMeta, []byte(SchemaVersionKey), data)
	require.NoError(t, err)

	// Register migrations 1 and 2
	migration1 := &mockMigration{
		version:     1,
		description: "Migration 1",
	}
	err = migrator.RegisterMigration(migration1)
	require.NoError(t, err)

	migration2 := &mockMigration{
		version:     2,
		description: "Migration 2",
		upFunc: func(ctx context.Context) error {
			return provider.CreateBucket(ctx, "test_bucket")
		},
	}
	err = migrator.RegisterMigration(migration2)
	require.NoError(t, err)

	// Run migrations (should only run migration 2)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify bucket was created
	exists := provider.BucketExists(ctx, "test_bucket")
	assert.True(t, exists)

	// Verify version is 2
	version, err := migrator.GetCurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, version)
}

