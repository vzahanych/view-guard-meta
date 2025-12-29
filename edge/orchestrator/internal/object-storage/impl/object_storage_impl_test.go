package impl

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

func TestObjectStorageImpl_StartStop(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
		QuotaConfig: &types.QuotaConfig{
			MaxSizeMB:               100,
			WarningThresholdPercent: 80,
			FullThresholdPercent:    95,
		},
		RetentionConfig: &types.RetentionConfig{
			DatasetRetentionDays: 30,
			EventRetentionDays:   7,
			CleanupIntervalHours: 1,
		},
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Test start
	err := impl.Start(ctx)
	require.NoError(t, err)

	// Test that service is started (check via Name method)
	assert.Equal(t, "object-storage", impl.Name())

	// Test stop
	err = impl.Stop(ctx)
	require.NoError(t, err)
}

func TestObjectStorageImpl_StoreLoadDeleteDataUnit(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Start service
	err := impl.Start(ctx)
	require.NoError(t, err)
	defer impl.Stop(ctx)

	// Store data unit
	deviceID := types.DeviceID("camera-001")
	deviceType := types.DeviceTypeCamera
	dataType := types.DataTypeImage
	key := impl.GenerateDataUnitKey(deviceID, deviceType, dataType, false)
	testData := []byte("test image data")
	
	err = impl.StoreDataUnit(ctx, deviceID, deviceType, dataType, key, bytes.NewReader(testData), int64(len(testData)), "image/jpeg")
	require.NoError(t, err)

	// Load data unit
	rc, err := impl.LoadDataUnit(ctx, key)
	require.NoError(t, err)
	defer rc.Close()

	loadedData, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, testData, loadedData)

	// Delete data unit
	err = impl.DeleteDataUnit(ctx, key)
	require.NoError(t, err)

	// Verify deleted
	_, err = impl.LoadDataUnit(ctx, key)
	assert.Error(t, err)
	assert.Equal(t, types.ErrObjectNotFound, err)
}

func TestObjectStorageImpl_QuotaEnforcement(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
		QuotaConfig: &types.QuotaConfig{
			MaxSizeMB:               1, // 1 MB limit
			WarningThresholdPercent: 80,
			FullThresholdPercent:    95,
		},
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Start service
	err := impl.Start(ctx)
	require.NoError(t, err)
	defer impl.Stop(ctx)

	// Fill to 96% (960 KB)
	deviceID := types.DeviceID("camera-001")
	deviceType := types.DeviceTypeCamera
	dataType := types.DataTypeVideoClip
	
	for i := 0; i < 10; i++ {
		key := impl.GenerateDataUnitKey(deviceID, deviceType, dataType, false)
		testData := make([]byte, 96*1024) // 96 KB per object
		err = impl.StoreDataUnit(ctx, deviceID, deviceType, dataType, key, bytes.NewReader(testData), int64(len(testData)), "video/mp4")
		require.NoError(t, err)
	}

	// Try to store another object (should fail - exceeds 95% threshold)
	key := impl.GenerateDataUnitKey(deviceID, deviceType, dataType, false)
	testData := make([]byte, 50*1024) // 50 KB
	err = impl.StoreDataUnit(ctx, deviceID, deviceType, dataType, key, bytes.NewReader(testData), int64(len(testData)), "video/mp4")
	assert.Error(t, err)
	assert.Equal(t, types.ErrQuotaExceeded, err)
}

func TestObjectStorageImpl_HealthSnapshot(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
		QuotaConfig: &types.QuotaConfig{
			MaxSizeMB:               100,
			WarningThresholdPercent: 80,
			FullThresholdPercent:    95,
		},
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Start service
	err := impl.Start(ctx)
	require.NoError(t, err)
	defer impl.Stop(ctx)

	// Get health snapshot
	health := impl.HealthSnapshot()
	assert.NotNil(t, health)
	assert.Equal(t, types.HealthStatusHealthy, health.Status)
	assert.NotNil(t, health.Quota)
}

func TestObjectStorageImpl_StoreLoadModelArtifacts(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Start service
	err := impl.Start(ctx)
	require.NoError(t, err)
	defer impl.Stop(ctx)

	// Store model artifacts
	modelID := "yolov8-detection-v1"
	deviceID := types.DeviceID("camera-001")
	manifest := &types.ModelManifest{
		ModelID:        modelID,
		DeviceID:       deviceID,
		Version:        "v1.0.0",
		TargetRuntime:  "OpenVINO",
		ProtocolVersion: "1.0",
		SchemaVersion:   "1.0",
		ArtifactHashes: make(map[string]string),
		CreatedAt:      time.Now(),
		Metadata:       make(map[string]interface{}),
	}
	artifacts := map[string][]byte{
		"model":    []byte("model binary data"),
		"metadata": []byte(`{"version": "v1.0.0"}`),
		"manifest": []byte(`{"model_id": "yolov8-detection-v1"}`),
	}

	err = impl.StoreModelArtifacts(ctx, modelID, deviceID, manifest, artifacts)
	require.NoError(t, err)

	// Load model artifacts
	loaded, err := impl.LoadModelArtifacts(ctx, modelID, deviceID)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, modelID, loaded.ModelID)
	assert.Equal(t, deviceID, loaded.DeviceID)
	assert.Equal(t, "v1.0.0", loaded.Version)
	assert.Equal(t, artifacts["model"], loaded.Model)
	assert.Equal(t, artifacts["metadata"], loaded.Metadata)
	assert.Equal(t, artifacts["manifest"], loaded.Manifest)
	assert.NotEmpty(t, loaded.Hashes)
}

func TestObjectStorageImpl_StoreLoadSecurityEventAttachment(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Start service
	err := impl.Start(ctx)
	require.NoError(t, err)
	defer impl.Stop(ctx)

	// Store attachment
	eventID := "event-123"
	deviceID := types.DeviceID("camera-001")
	dataType := types.DataTypeImage
	attachmentData := []byte("image attachment data")

	key, err := impl.StoreSecurityEventAttachment(ctx, eventID, deviceID, dataType, attachmentData)
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.True(t, strings.HasPrefix(key, "security-events/"))

	// Load attachment
	rc, err := impl.LoadSecurityEventAttachment(ctx, key)
	require.NoError(t, err)
	defer rc.Close()

	loadedData, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, attachmentData, loadedData)
}

func TestObjectStorageImpl_NotInitialized(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	config := &types.ObjectStorageConfig{
		Provider: "mock",
	}

	impl := NewObjectStorageImpl(provider, config, logger)
	ctx := context.Background()

	// Try to use service before starting (should fail)
	deviceID := types.DeviceID("camera-001")
	deviceType := types.DeviceTypeCamera
	dataType := types.DataTypeImage
	key := impl.GenerateDataUnitKey(deviceID, deviceType, dataType, false)
	
	err := impl.StoreDataUnit(ctx, deviceID, deviceType, dataType, key, bytes.NewReader([]byte("data")), 4, "image/jpeg")
	assert.Error(t, err)
	assert.Equal(t, types.ErrNotInitialized, err)
}

