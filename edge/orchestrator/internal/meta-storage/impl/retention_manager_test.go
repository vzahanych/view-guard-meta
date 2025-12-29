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

func TestRetentionManager_CleanupExpiredRecords(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	config := &types.RetentionConfig{
		EventBusRetentionHours: 1, // 1 hour retention for event_bus
		CleanupIntervalHours:   1,
	}

	manager := NewRetentionManager(provider, config, logger)

	// Use a standard bucket that has retention (event_bus)
	bucketName := BucketEventBus
	err := provider.CreateBucket(ctx, bucketName)
	require.NoError(t, err)

	// Add old record (2 hours ago)
	oldTime := time.Now().Add(-2 * time.Hour)
	oldRecord := map[string]interface{}{
		"id":         "old_record",
		"created_at": oldTime.Format(time.RFC3339),
	}
	oldData, err := json.Marshal(oldRecord)
	require.NoError(t, err)
	err = provider.Put(ctx, bucketName, []byte("old_record"), oldData)
	require.NoError(t, err)

	// Add recent record (30 minutes ago)
	recentTime := time.Now().Add(-30 * time.Minute)
	recentRecord := map[string]interface{}{
		"id":         "recent_record",
		"created_at": recentTime.Format(time.RFC3339),
	}
	recentData, err := json.Marshal(recentRecord)
	require.NoError(t, err)
	err = provider.Put(ctx, bucketName, []byte("recent_record"), recentData)
	require.NoError(t, err)

	// Run cleanup
	stats, err := manager.CleanupExpiredRecords(ctx)
	require.NoError(t, err)
	assert.Greater(t, stats.RecordsDeleted, int64(0))
	assert.Equal(t, 1, stats.BucketsProcessed)

	// Verify old record is deleted
	_, err = provider.Get(ctx, bucketName, []byte("old_record"))
	assert.Error(t, err)
	assert.Equal(t, types.ErrRecordNotFound, err)

	// Verify recent record still exists
	_, err = provider.Get(ctx, bucketName, []byte("recent_record"))
	assert.NoError(t, err)
}

func TestRetentionManager_CleanupWithTimestampField(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	config := &types.RetentionConfig{
		EventBusRetentionHours: 1, // 1 hour retention
		CleanupIntervalHours:   1,
	}

	manager := NewRetentionManager(provider, config, logger)

	// Use a standard bucket that has retention (event_bus)
	bucketName := BucketEventBus
	err := provider.CreateBucket(ctx, bucketName)
	require.NoError(t, err)

	// Add record with timestamp field (2 hours ago)
	oldTime := time.Now().Add(-2 * time.Hour)
	oldRecord := map[string]interface{}{
		"id":        "old_record",
		"timestamp": oldTime.Format(time.RFC3339),
	}
	oldData, err := json.Marshal(oldRecord)
	require.NoError(t, err)
	err = provider.Put(ctx, bucketName, []byte("old_record"), oldData)
	require.NoError(t, err)

	// Run cleanup
	stats, err := manager.CleanupExpiredRecords(ctx)
	require.NoError(t, err)
	assert.Greater(t, stats.RecordsDeleted, int64(0))

	// Verify old record is deleted
	_, err = provider.Get(ctx, bucketName, []byte("old_record"))
	assert.Error(t, err)
}

func TestRetentionManager_DefaultConfig(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()

	// Test with nil config (should use defaults)
	manager := NewRetentionManager(provider, nil, logger)
	assert.NotNil(t, manager.config)
	assert.Greater(t, manager.config.EventBusRetentionHours, 0)
}

func TestRetentionManager_CleanupRunning(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	config := &types.RetentionConfig{
		EventBusRetentionHours: 24,
		CleanupIntervalHours:   1,
	}

	manager := NewRetentionManager(provider, config, logger)

	// Test that cleanup can run successfully
	stats, err := manager.CleanupExpiredRecords(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats)

	// Test that we can run cleanup again after the first one completes
	stats2, err2 := manager.CleanupExpiredRecords(ctx)
	require.NoError(t, err2)
	assert.NotNil(t, stats2)
}
