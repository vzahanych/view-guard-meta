package impl

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// mockProvider is a mock implementation of MetaStorageProvider for testing
type mockProvider struct {
	buckets map[string]map[string][]byte
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		buckets: make(map[string]map[string][]byte),
	}
}

func (m *mockProvider) CreateBucket(ctx context.Context, bucketName string) error {
	if m.buckets[bucketName] == nil {
		m.buckets[bucketName] = make(map[string][]byte)
	}
	return nil
}

func (m *mockProvider) DeleteBucket(ctx context.Context, bucketName string) error {
	delete(m.buckets, bucketName)
	return nil
}

func (m *mockProvider) BucketExists(ctx context.Context, bucketName string) bool {
	_, exists := m.buckets[bucketName]
	return exists
}

func (m *mockProvider) Put(ctx context.Context, bucketName string, key []byte, value []byte) error {
	if m.buckets[bucketName] == nil {
		m.buckets[bucketName] = make(map[string][]byte)
	}
	m.buckets[bucketName][string(key)] = value
	return nil
}

func (m *mockProvider) Get(ctx context.Context, bucketName string, key []byte) ([]byte, error) {
	bucket, exists := m.buckets[bucketName]
	if !exists {
		return nil, types.ErrRecordNotFound
	}
	value, exists := bucket[string(key)]
	if !exists {
		return nil, types.ErrRecordNotFound
	}
	// Return a copy
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

func (m *mockProvider) Delete(ctx context.Context, bucketName string, key []byte) error {
	bucket, exists := m.buckets[bucketName]
	if !exists {
		return nil
	}
	delete(bucket, string(key))
	return nil
}

func (m *mockProvider) List(ctx context.Context, bucketName string, prefix []byte) ([]types.KeyValue, error) {
	bucket, exists := m.buckets[bucketName]
	if !exists {
		return []types.KeyValue{}, nil
	}
	prefixStr := string(prefix)
	results := make([]types.KeyValue, 0)
	for k, v := range bucket {
		if prefixStr == "" || len(k) >= len(prefixStr) && k[:len(prefixStr)] == prefixStr {
			// Return copies
			keyCopy := make([]byte, len(k))
			copy(keyCopy, []byte(k))
			valueCopy := make([]byte, len(v))
			copy(valueCopy, v)
			results = append(results, types.KeyValue{
				Key:   keyCopy,
				Value: valueCopy,
			})
		}
	}
	return results, nil
}

func (m *mockProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockProvider) Start(ctx context.Context) error {
	return nil
}

func (m *mockProvider) Stop(ctx context.Context) error {
	return nil
}

func (m *mockProvider) Close() error {
	return nil
}

func TestQuotaManager_GetQuotaStatus(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	// Create test database file
	tmpFile := t.TempDir() + "/test.db"
	config := &types.QuotaConfig{
		MaxSizeMB:               10,
		MaxRecordsPerBucket:     100,
		WarningThresholdPercent: 80,
	}

	manager := NewQuotaManager(provider, config, logger, tmpFile)

	// Test empty database
	status, err := manager.GetQuotaStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)
	// Limit should be calculated from MaxSizeMB (10 MB = 10 * 1024 * 1024 bytes)
	expectedLimit := int64(10 * 1024 * 1024)
	assert.Equal(t, expectedLimit, status.Limit)
	// Used should be 0 for empty database
	assert.Equal(t, int64(0), status.Used)

	// Add some records to a standard bucket
	err = provider.CreateBucket(ctx, BucketDataUnits)
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		err = provider.Put(ctx, BucketDataUnits, []byte("key"+string(rune(i))), []byte("value"))
		require.NoError(t, err)
	}

	status, err = manager.GetQuotaStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)
	// Verify bucket counts via GetBucketCounts
	bucketCounts := manager.GetBucketCounts()
	assert.Equal(t, int64(50), bucketCounts[BucketDataUnits])
}

func TestQuotaManager_CheckQuotaBeforeWrite(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	config := &types.QuotaConfig{
		MaxSizeMB:               1, // 1 MB limit
		MaxRecordsPerBucket:     10,
		WarningThresholdPercent: 80,
	}

	tmpFile := t.TempDir() + "/test.db"
	manager := NewQuotaManager(provider, config, logger, tmpFile)

	// Test quota check with empty database
	err := manager.CheckQuotaBeforeWrite(ctx, BucketDataUnits, 100)
	assert.NoError(t, err)

	// Fill bucket to limit using a standard bucket
	err = provider.CreateBucket(ctx, BucketDataUnits)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		err = provider.Put(ctx, BucketDataUnits, []byte("key"+string(rune(i))), []byte("value"))
		require.NoError(t, err)
	}

	// Update quota status to refresh bucket counts
	_, err = manager.GetQuotaStatus(ctx)
	require.NoError(t, err)

	// Try to write when at limit (11th record should fail)
	err = manager.CheckQuotaBeforeWrite(ctx, BucketDataUnits, 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "record limit")
}

func TestQuotaManager_WarningThreshold(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	config := &types.QuotaConfig{
		MaxSizeMB:               1, // Small limit to test threshold
		MaxRecordsPerBucket:     100,
		WarningThresholdPercent: 80,
		FullThresholdPercent:    95,
	}

	tmpFile := t.TempDir() + "/test.db"
	// Create a file with some size to simulate usage
	err := os.WriteFile(tmpFile, make([]byte, 900*1024), 0644) // 900 KB = 90% of 1 MB
	require.NoError(t, err)

	manager := NewQuotaManager(provider, config, logger, tmpFile)

	status, err := manager.GetQuotaStatus(ctx)
	require.NoError(t, err)
	assert.NotNil(t, status)
	// Verify that usage is above warning threshold (900 KB / 1 MB = 90%)
	usagePercent := float64(status.Used) / float64(status.Limit) * 100
	assert.GreaterOrEqual(t, usagePercent, 80.0)
}

func TestQuotaManager_DefaultConfig(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()

	// Test with nil config (should use defaults)
	manager := NewQuotaManager(provider, nil, logger, "")
	assert.NotNil(t, manager.config)
	assert.Greater(t, manager.config.MaxSizeMB, 0)
}
