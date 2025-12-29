// +build integration

package impl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/impl/bbolt"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

func createTestBoltDBProvider(t *testing.T) (*bbolt.BoltDBProvider, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	logger := zap.NewNop()
	cfg := &types.BoltDBConfig{
		DatabaseFile: dbPath,
	}
	
	ctx := context.Background()
	provider, err := bbolt.NewBoltDBProvider(ctx, cfg, logger)
	require.NoError(t, err)
	
	return provider, dbPath
}

func createTestMetaStorageWithBoltDB(t *testing.T) (*MetaStorageImpl, string) {
	provider, dbPath := createTestBoltDBProvider(t)
	logger := zap.NewNop()
	impl := NewMetaStorageImpl(provider, logger)
	return impl, dbPath
}

func TestMetaStorageImpl_Integration_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	storage, _ := createTestMetaStorageWithBoltDB(t)

	// Start storage
	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Save device
	device := types.DeviceMetadata{
		ID:         types.DeviceID("device1"),
		Name:       "Test Camera",
		DeviceType: types.DeviceTypeCamera,
		Enabled:    true,
		Status:     "online",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDevice(ctx, device)
	require.NoError(t, err)

	// Get device
	retrieved, found := storage.GetDevice(ctx, "device1")
	assert.True(t, found)
	assert.Equal(t, device.ID, retrieved.ID)

	// Update device
	updated, err := storage.UpdateDevice(ctx, "device1", func(d types.DeviceMetadata) types.DeviceMetadata {
		d.Name = "Updated Camera"
		return d
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Camera", updated.Name)

	// Delete device
	err = storage.DeleteDevice(ctx, "device1")
	require.NoError(t, err)

	// Verify deleted
	_, found = storage.GetDevice(ctx, "device1")
	assert.False(t, found)
}

func TestMetaStorageImpl_Integration_DataUnitLifecycle(t *testing.T) {
	ctx := context.Background()
	storage, _ := createTestMetaStorageWithBoltDB(t)

	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Save multiple data units
	deviceID := types.DeviceID("device1")
	for i := 0; i < 10; i++ {
		dataUnit := types.DataUnitMetadata{
			ID:         "data" + string(rune(i)),
			DeviceID:   deviceID,
			DeviceType: types.DeviceTypeCamera,
			DataType:   "image",
			Label:      "motion",
			CreatedAt:  time.Now().Add(-time.Duration(i) * time.Hour),
			UpdatedAt:  time.Now(),
		}
		err := storage.SaveDataUnit(ctx, dataUnit)
		require.NoError(t, err)
	}

	// List all data units
	all, err := storage.ListDataUnits(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 10, len(all))

	// Filter by device ID
	filters := &types.DataUnitFilters{
		DeviceID: &deviceID,
	}
	filtered, err := storage.ListDataUnits(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, 10, len(filtered))

	// Delete data unit
	err = storage.DeleteDataUnit(ctx, "data0")
	require.NoError(t, err)

	// Verify deleted
	_, found := storage.GetDataUnit(ctx, "data0")
	assert.False(t, found)
}

func TestMetaStorageImpl_Integration_MLLifecycleState(t *testing.T) {
	ctx := context.Background()
	storage, _ := createTestMetaStorageWithBoltDB(t)

	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Save initial state
	state := types.MLLifecycleStateInfo{
		DeviceID:    types.DeviceID("device1"),
		State:       types.MLLifecycleStateAssigned,
		LastUpdated: time.Now(),
		Version:     1,
	}
	err = storage.SaveMLLifecycleState(ctx, "device1", state)
	require.NoError(t, err)

	// Update state with CAS
	updated, err := storage.UpdateMLLifecycleStateCAS(ctx, "device1", 1, func(s types.MLLifecycleStateInfo) types.MLLifecycleStateInfo {
		s.State = types.MLLifecycleStateAwaitingDataset
		s.Version = 2
		return s
	})
	require.NoError(t, err)
	assert.Equal(t, types.MLLifecycleStateAwaitingDataset, updated.State)
	assert.Equal(t, 2, updated.Version)

	// List all states
	states, err := storage.ListMLLifecycleStates(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(states))
}

func TestMetaStorageImpl_Integration_QuotaAndRetention(t *testing.T) {
	ctx := context.Background()
	storage, dbPath := createTestMetaStorageWithBoltDB(t)

	// Set up quota manager
	quotaConfig := &types.QuotaConfig{
		MaxSizeMB: 10,
		MaxRecordsPerBucket: 100,
		WarningThresholdPercent: 80,
	}
	quotaManager := NewQuotaManager(storage.provider, quotaConfig, zap.NewNop(), dbPath)
	storage.SetQuotaManager(quotaManager)

	// Set up retention manager
	retentionConfig := &types.RetentionConfig{
		DefaultRetentionHours: 1,
		CleanupIntervalHours:  1,
	}
	retentionManager := NewRetentionManager(storage.provider, retentionConfig, zap.NewNop())
	storage.SetRetentionManager(retentionManager)

	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Add old data unit (2 hours ago)
	oldDataUnit := types.DataUnitMetadata{
		ID:         "old_data",
		DeviceID:   types.DeviceID("device1"),
		DeviceType: types.DeviceTypeCamera,
		DataType:   "image",
		CreatedAt:  time.Now().Add(-2 * time.Hour),
		UpdatedAt:  time.Now().Add(-2 * time.Hour),
	}
	err = storage.SaveDataUnit(ctx, oldDataUnit)
	require.NoError(t, err)

	// Add recent data unit
	recentDataUnit := types.DataUnitMetadata{
		ID:         "recent_data",
		DeviceID:   types.DeviceID("device1"),
		DeviceType: types.DeviceTypeCamera,
		DataType:   "image",
		CreatedAt:  time.Now().Add(-30 * time.Minute),
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDataUnit(ctx, recentDataUnit)
	require.NoError(t, err)

	// Run cleanup
	stats, err := retentionManager.CleanupExpiredRecords(ctx)
	require.NoError(t, err)
	assert.Greater(t, stats.RecordsDeleted, int64(0))

	// Verify old record is deleted
	_, found := storage.GetDataUnit(ctx, "old_data")
	assert.False(t, found)

	// Verify recent record still exists
	_, found = storage.GetDataUnit(ctx, "recent_data")
	assert.True(t, found)
}

func TestMetaStorageImpl_Integration_IntegrityVerification(t *testing.T) {
	ctx := context.Background()
	storage, _ := createTestMetaStorageWithBoltDB(t)

	// Set up integrity manager
	integrityManager := NewIntegrityManager(storage.provider, zap.NewNop())
	storage.SetIntegrityManager(integrityManager)

	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Add valid data
	device := types.DeviceMetadata{
		ID:         types.DeviceID("device1"),
		Name:       "Test Device",
		DeviceType: types.DeviceTypeCamera,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDevice(ctx, device)
	require.NoError(t, err)

	// Verify integrity
	report, err := integrityManager.VerifyDatabaseIntegrity(ctx)
	require.NoError(t, err)
	assert.True(t, report.IsHealthy)
	assert.Equal(t, 0, report.ErrorCount)
}

func TestMetaStorageImpl_Integration_HealthSnapshot(t *testing.T) {
	ctx := context.Background()
	storage, dbPath := createTestMetaStorageWithBoltDB(t)

	// Set up managers
	quotaConfig := &types.QuotaConfig{
		MaxSizeMB: 10,
		MaxRecordsPerBucket: 100,
	}
	quotaManager := NewQuotaManager(storage.provider, quotaConfig, zap.NewNop(), dbPath)
	storage.SetQuotaManager(quotaManager)

	integrityManager := NewIntegrityManager(storage.provider, zap.NewNop())
	storage.SetIntegrityManager(integrityManager)

	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Get health snapshot
	health := storage.HealthSnapshot(ctx)
	assert.NotNil(t, health)
	assert.Equal(t, types.HealthStatusHealthy, health.Status)
	assert.NotNil(t, health.Quota)
	assert.NotNil(t, health.ProviderHealth)
}

func TestMetaStorageImpl_Integration_SchemaMigration(t *testing.T) {
	ctx := context.Background()
	storage, _ := createTestMetaStorageWithBoltDB(t)

	// Set up schema migrator
	migrator := NewSchemaMigrator(storage.provider, zap.NewNop())
	storage.SetSchemaMigrator(migrator)

	// Register test migration
	migration := &mockMigration{
		version:     1,
		description: "Create test bucket",
		upFunc: func(ctx context.Context, p types.MetaStorageProvider) error {
			return p.CreateBucket(ctx, "test_bucket")
		},
	}
	err := migrator.RegisterMigration(migration)
	require.NoError(t, err)

	err = storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Run migration
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Verify bucket was created
	exists, err := storage.provider.BucketExists(ctx, "test_bucket")
	require.NoError(t, err)
	assert.True(t, exists)

	// Verify version
	version, err := migrator.GetCurrentVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func TestMetaStorageImpl_Integration_ModelDeployment(t *testing.T) {
	ctx := context.Background()
	storage, _ := createTestMetaStorageWithBoltDB(t)

	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Save model deployment
	deployment := types.ModelDeploymentMetadata{
		ModelID:      "model1",
		DeploymentID: "deployment1",
		DeviceID:     types.DeviceID("device1"),
		DeviceType:   types.DeviceTypeCamera,
		ModelPath:    "/path/to/model",
		DeployedAt:   time.Now(),
		Status:       "active",
		Version:      "1.0",
		ModelType:    "yolo",
		Framework:    "openvino",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	err = storage.SaveModelDeployment(ctx, deployment)
	require.NoError(t, err)

	// List deployments
	filters := &types.ModelFilters{
		DeviceID: stringPtr("device1"),
	}
	deployments, err := storage.ListModelDeployments(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, 1, len(deployments))
	assert.Equal(t, deployment.ModelID, deployments[0].ModelID)
}

func stringPtr(s string) *string {
	return &s
}

