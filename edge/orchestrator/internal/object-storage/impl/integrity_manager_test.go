package impl

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

func TestIntegrityManager_CalculateHash(t *testing.T) {
	logger := zap.NewNop()
	manager := NewIntegrityManager(nil, logger)

	data := []byte("test data")
	hash := manager.CalculateHash(data)

	// Hash should be SHA-256 hex string (64 characters)
	assert.Equal(t, 64, len(hash))
	assert.NotEmpty(t, hash)
}

func TestIntegrityManager_CalculateHashFromReader(t *testing.T) {
	logger := zap.NewNop()
	manager := NewIntegrityManager(nil, logger)

	data := []byte("test data")
	hash, err := manager.CalculateHashFromReader(bytes.NewReader(data))
	require.NoError(t, err)

	// Should match direct hash calculation
	expectedHash := manager.CalculateHash(data)
	assert.Equal(t, expectedHash, hash)
}

func TestIntegrityManager_VerifyObjectIntegrity(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Store object with hash
	testData := []byte("test data")
	hash := manager.CalculateHash(testData)
	key := "video_clip/camera/cam-001/2025-12-28/test.mp4"
	metadata := map[string]string{
		"hash": hash,
	}
	err := provider.StoreObject(ctx, key, bytes.NewReader(testData), int64(len(testData)), "video/mp4", metadata)
	require.NoError(t, err)

	// Verify integrity (should succeed)
	err = manager.VerifyObjectIntegrity(ctx, key)
	assert.NoError(t, err)

	// Corrupt the object by storing different data (but keep old hash in metadata to simulate corruption)
	corruptedData := []byte("corrupted data")
	// Store with corrupted data but old hash in metadata to simulate corruption
	corruptedMetadata := map[string]string{
		"hash": hash, // Keep old hash to simulate corruption
	}
	err = provider.StoreObject(ctx, key, bytes.NewReader(corruptedData), int64(len(corruptedData)), "video/mp4", corruptedMetadata)
	require.NoError(t, err)

	// Verify integrity (should fail with hash mismatch)
	err = manager.VerifyObjectIntegrity(ctx, key)
	assert.Error(t, err)
	// The error should indicate hash mismatch (which is corruption)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestIntegrityManager_VerifyStorageIntegrity(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Store some objects with hashes
	testData1 := []byte("test data 1")
	hash1 := manager.CalculateHash(testData1)
	key1 := "video_clips/camera/cam-001/2025-12-28/test1.mp4"
	metadata1 := map[string]string{
		"hash": hash1,
	}
	err := provider.StoreObject(ctx, key1, bytes.NewReader(testData1), int64(len(testData1)), "video/mp4", metadata1)
	require.NoError(t, err)

	testData2 := []byte("test data 2")
	hash2 := manager.CalculateHash(testData2)
	key2 := "images/camera/cam-001/2025-12-28/test2.jpg"
	metadata2 := map[string]string{
		"hash": hash2,
	}
	err = provider.StoreObject(ctx, key2, bytes.NewReader(testData2), int64(len(testData2)), "image/jpeg", metadata2)
	require.NoError(t, err)

	// Verify storage integrity
	report, err := manager.VerifyStorageIntegrity(ctx)
	require.NoError(t, err)
	assert.True(t, report.IsHealthy)
	assert.Equal(t, 0, report.ErrorCount)
	assert.Greater(t, report.ObjectsChecked, 0)
}

func TestIntegrityManager_DetectCorruption(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Store object with hash
	testData := []byte("test data")
	hash := manager.CalculateHash(testData)
	key := "video_clip/camera/cam-001/2025-12-28/test.mp4"
	metadata := map[string]string{
		"hash": hash,
	}
	err := provider.StoreObject(ctx, key, bytes.NewReader(testData), int64(len(testData)), "video/mp4", metadata)
	require.NoError(t, err)

	// Corrupt the object by storing different data but keeping old hash
	corruptedData := []byte("corrupted data")
	corruptedMetadata := map[string]string{
		"hash": hash, // Keep old hash to simulate corruption
	}
	err = provider.StoreObject(ctx, key, bytes.NewReader(corruptedData), int64(len(corruptedData)), "video/mp4", corruptedMetadata)
	require.NoError(t, err)

	// Detect corruption
	err = manager.DetectCorruption(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrCorruptionDetected, err)
}

func TestIntegrityManager_GetLastIntegrityReport(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Initially no report
	report := manager.GetLastIntegrityReport()
	assert.Nil(t, report)

	// Run verification
	_, err := manager.VerifyStorageIntegrity(ctx)
	require.NoError(t, err)

	// Now should have a report
	report = manager.GetLastIntegrityReport()
	assert.NotNil(t, report)
}

