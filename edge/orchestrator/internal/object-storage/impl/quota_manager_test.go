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

// mockObjectProvider is defined in mock_provider.go

func TestQuotaManager_GetQuotaStatus(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	config := &types.QuotaConfig{
		MaxSizeMB:               10, // 10 MB limit
		WarningThresholdPercent: 80,
		FullThresholdPercent:    95,
	}
	config.Validate()

	manager := NewQuotaManager(provider, config, logger)

	// Test empty storage
	status, err := manager.GetQuotaStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, int64(0), status.Used)
	assert.Equal(t, int64(10*1024*1024), status.Limit) // 10 MB in bytes

	// Add some objects
	testData := make([]byte, 1024*1024) // 1 MB
	key1 := "video_clips/camera/cam-001/2025-12-28/test1.mp4"
	err = provider.StoreObject(ctx, key1, bytes.NewReader(testData), int64(len(testData)), "video/mp4", nil)
	require.NoError(t, err)

	key2 := "images/camera/cam-001/2025-12-28/test2.jpg"
	err = provider.StoreObject(ctx, key2, bytes.NewReader(testData), int64(len(testData)), "image/jpeg", nil)
	require.NoError(t, err)

	// Get quota status
	status, err = manager.GetQuotaStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2*1024*1024), status.Used) // 2 MB used

	// Verify object counts
	counts := manager.GetObjectCounts()
	assert.Equal(t, int64(1), counts["video_clips"])
	assert.Equal(t, int64(1), counts["images"])
}

func TestQuotaManager_CheckQuotaBeforeWrite(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	config := &types.QuotaConfig{
		MaxSizeMB:               1, // 1 MB limit
		WarningThresholdPercent: 80,
		FullThresholdPercent:    95,
	}
	config.Validate()

	manager := NewQuotaManager(provider, config, logger)

	// Fill storage to 90% (900 KB)
	testData := make([]byte, 900*1024)
	key := "video_clips/camera/cam-001/2025-12-28/test.mp4"
	err := provider.StoreObject(ctx, key, bytes.NewReader(testData), int64(len(testData)), "video/mp4", nil)
	require.NoError(t, err)

	// Update quota status
	_, err = manager.GetQuotaStatus(ctx)
	require.NoError(t, err)

	// Try to write 40 KB (should succeed - 900 KB + 40 KB = 940 KB = 94% < 95%)
	err = manager.CheckQuotaBeforeWrite(ctx, 40*1024)
	assert.NoError(t, err) // Should allow (below 95% threshold)

	// Fill to 96% (960 KB total)
	key2 := "video_clips/camera/cam-001/2025-12-28/test2.mp4"
	err = provider.StoreObject(ctx, key2, bytes.NewReader(make([]byte, 60*1024)), 60*1024, "video/mp4", nil)
	require.NoError(t, err)

	// Update quota status
	_, err = manager.GetQuotaStatus(ctx)
	require.NoError(t, err)

	// Try to write 50 KB (should fail - exceeds 95% threshold: 960 KB + 50 KB = 1010 KB > 95% of 1 MB)
	err = manager.CheckQuotaBeforeWrite(ctx, 50*1024)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")
}

func TestQuotaManager_WarningThreshold(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	config := &types.QuotaConfig{
		MaxSizeMB:               1, // 1 MB limit
		WarningThresholdPercent: 80,
		FullThresholdPercent:    95,
	}
	config.Validate()

	manager := NewQuotaManager(provider, config, logger)

	// Fill to 85% (850 KB)
	testData := make([]byte, 850*1024)
	key := "video_clips/camera/cam-001/2025-12-28/test.mp4"
	err := provider.StoreObject(ctx, key, bytes.NewReader(testData), int64(len(testData)), "video/mp4", nil)
	require.NoError(t, err)

	status, err := manager.GetQuotaStatus(ctx)
	require.NoError(t, err)

	usagePercent := float64(status.Used) / float64(status.Limit) * 100
	assert.GreaterOrEqual(t, usagePercent, 80.0)
}

func TestQuotaManager_GetCachedQuotaStatus(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()
	ctx := context.Background()

	config := &types.QuotaConfig{
		MaxSizeMB:               10,
		WarningThresholdPercent: 80,
		FullThresholdPercent:    95,
	}
	config.Validate()

	manager := NewQuotaManager(provider, config, logger)

	// Initially no cached status
	cached := manager.GetCachedQuotaStatus()
	assert.Nil(t, cached)

	// Get quota status (will cache it)
	_, err := manager.GetQuotaStatus(ctx)
	require.NoError(t, err)

	// Now should have cached status
	cached = manager.GetCachedQuotaStatus()
	assert.NotNil(t, cached)
}

func TestQuotaManager_DefaultConfig(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockObjectProvider()

	// Test with nil config (should use defaults)
	manager := NewQuotaManager(provider, nil, logger)
	assert.NotNil(t, manager.config)
	assert.Greater(t, manager.config.MaxSizeMB, 0)
}

