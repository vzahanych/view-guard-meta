package impl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

func createTestMetaStorage(t *testing.T) *MetaStorageImpl {
	logger := zap.NewNop()
	provider := newMockProvider()
	impl := NewMetaStorageImpl(provider, logger)
	return impl
}

func TestMetaStorageImpl_SaveDevice(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	device := types.DeviceMetadata{
		ID:         types.DeviceID("device1"),
		Name:       "Test Device",
		DeviceType: types.DeviceTypeCamera,
		Enabled:    true,
		Status:     "online",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := storage.SaveDevice(ctx, device)
	require.NoError(t, err)

	// Verify device was saved
	retrieved, found := storage.GetDevice(ctx, "device1")
	assert.True(t, found)
	assert.Equal(t, device.ID, retrieved.ID)
	assert.Equal(t, device.Name, retrieved.Name)
}

func TestMetaStorageImpl_UpdateDevice(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	device := types.DeviceMetadata{
		ID:         types.DeviceID("device1"),
		Name:       "Test Device",
		DeviceType: types.DeviceTypeCamera,
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := storage.SaveDevice(ctx, device)
	require.NoError(t, err)

	// Update device
	updated, err := storage.UpdateDevice(ctx, "device1", func(d types.DeviceMetadata) types.DeviceMetadata {
		d.Name = "Updated Device"
		d.Enabled = false
		return d
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Device", updated.Name)
	assert.False(t, updated.Enabled)

	// Verify update
	retrieved, found := storage.GetDevice(ctx, "device1")
	assert.True(t, found)
	assert.Equal(t, "Updated Device", retrieved.Name)
}

func TestMetaStorageImpl_SaveDataUnit(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	dataUnit := types.DataUnitMetadata{
		ID:         "data1",
		DeviceID:   types.DeviceID("device1"),
		DeviceType: types.DeviceTypeCamera,
		DataType:   "image",
		Label:      "motion",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := storage.SaveDataUnit(ctx, dataUnit)
	require.NoError(t, err)

	// Verify data unit was saved
	retrieved, found := storage.GetDataUnit(ctx, "data1")
	assert.True(t, found)
	assert.Equal(t, dataUnit.ID, retrieved.ID)
	assert.Equal(t, dataUnit.DeviceID, retrieved.DeviceID)
}

func TestMetaStorageImpl_ListDataUnitsWithFilters(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	// Save multiple data units
	device1 := types.DeviceID("device1")
	device2 := types.DeviceID("device2")
	deviceType := types.DeviceTypeCamera

	for i := 0; i < 5; i++ {
		dataUnit := types.DataUnitMetadata{
			ID:         "data" + string(rune(i)),
			DeviceID:   device1,
			DeviceType: deviceType,
			DataType:   "image",
			Label:      "motion",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		err := storage.SaveDataUnit(ctx, dataUnit)
		require.NoError(t, err)
	}

	// Save data unit for device2
	dataUnit2 := types.DataUnitMetadata{
		ID:         "data_device2",
		DeviceID:   device2,
		DeviceType: deviceType,
		DataType:   "image",
		Label:      "motion",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err := storage.SaveDataUnit(ctx, dataUnit2)
	require.NoError(t, err)

	// Filter by device ID
	filters := &types.DataUnitFilters{
		DeviceID: &device1,
	}
	results, err := storage.ListDataUnits(ctx, filters)
	require.NoError(t, err)
	assert.Equal(t, 5, len(results))
	for _, result := range results {
		assert.Equal(t, device1, result.DeviceID)
	}
}

func TestMetaStorageImpl_SaveMLLifecycleState(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	state := types.MLLifecycleStateInfo{
		DeviceID:    types.DeviceID("device1"),
		State:       types.MLLifecycleStateAssigned,
		LastUpdated: time.Now(),
	}

	err := storage.SaveMLLifecycleState(ctx, "device1", state)
	require.NoError(t, err)

	// Verify state was saved
	retrieved, found := storage.GetMLLifecycleState(ctx, "device1")
	assert.True(t, found)
	assert.Equal(t, state.DeviceID, retrieved.DeviceID)
	assert.Equal(t, state.State, retrieved.State)
}

func TestMetaStorageImpl_UpdateMLLifecycleStateCAS(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	// Save initial state
	initialState := types.MLLifecycleStateInfo{
		DeviceID:    types.DeviceID("device1"),
		State:       types.MLLifecycleStateAssigned,
		LastUpdated: time.Now(),
		Version:     1,
	}
	err := storage.SaveMLLifecycleState(ctx, "device1", initialState)
	require.NoError(t, err)

	// Update with CAS (should succeed)
	updated, err := storage.UpdateMLLifecycleStateCAS(ctx, "device1", 1, func(state types.MLLifecycleStateInfo) types.MLLifecycleStateInfo {
		state.State = types.MLLifecycleStateAwaitingDataset
		state.Version = 2
		return state
	})
	require.NoError(t, err)
	assert.Equal(t, types.MLLifecycleStateAwaitingDataset, updated.State)
	assert.Equal(t, 2, updated.Version)

	// Try CAS with wrong version (should fail)
	_, err = storage.UpdateMLLifecycleStateCAS(ctx, "device1", 1, func(state types.MLLifecycleStateInfo) types.MLLifecycleStateInfo {
		state.State = types.MLLifecycleStateDatasetReadyLocal
		return state
	})
	assert.Error(t, err)
}

func TestMetaStorageImpl_SaveModelDeployment(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

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

	err := storage.SaveModelDeployment(ctx, deployment)
	require.NoError(t, err)

	// Verify deployment was saved
	retrieved, found := storage.GetModelDeployment(ctx, "model1")
	assert.True(t, found)
	assert.Equal(t, deployment.ModelID, retrieved.ModelID)
	assert.Equal(t, deployment.DeviceID, retrieved.DeviceID)
}

func TestMetaStorageImpl_HealthSnapshot(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	// Start storage
	err := storage.Start(ctx)
	require.NoError(t, err)
	defer storage.Stop(ctx)

	// Get health snapshot
	health := storage.HealthSnapshot(ctx)
	assert.NotNil(t, health)
	assert.Equal(t, types.HealthStatusHealthy, health.Status)
}

func TestMetaStorageImpl_Lifecycle(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	// Test Start
	err := storage.Start(ctx)
	require.NoError(t, err)
	// Test Start again (should fail)
	err = storage.Start(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrAlreadyStarted, err)

	// Test Stop
	err = storage.Stop(ctx)
	require.NoError(t, err)
}

func TestMetaStorageImpl_QuotaEnforcement(t *testing.T) {
	ctx := context.Background()
	storage := createTestMetaStorage(t)

	// Set up quota manager with strict limits
	quotaConfig := &types.QuotaConfig{
		MaxSizeMB:               1,
		MaxRecordsPerBucket:     5,
		WarningThresholdPercent: 80,
	}
	quotaManager := NewQuotaManager(storage.provider, quotaConfig, zap.NewNop(), "")
	storage.SetQuotaManager(quotaManager)

	// Fill bucket to limit
	for i := 0; i < 5; i++ {
		dataUnit := types.DataUnitMetadata{
			ID:         "data" + string(rune(i)),
			DeviceID:   types.DeviceID("device1"),
			DeviceType: types.DeviceTypeCamera,
			DataType:   "image",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		err := storage.SaveDataUnit(ctx, dataUnit)
		require.NoError(t, err)
	}

	// Update quota status
	_, err := quotaManager.GetQuotaStatus(ctx)
	require.NoError(t, err)

	// Try to save one more (should fail)
	dataUnit := types.DataUnitMetadata{
		ID:         "data6",
		DeviceID:   types.DeviceID("device1"),
		DeviceType: types.DeviceTypeCamera,
		DataType:   "image",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err = storage.SaveDataUnit(ctx, dataUnit)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "quota")
}
