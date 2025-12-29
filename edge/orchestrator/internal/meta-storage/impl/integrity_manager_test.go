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

func TestIntegrityManager_VerifyDatabaseIntegrity(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Create a standard bucket and add valid record
	err := provider.CreateBucket(ctx, BucketDataUnits)
	require.NoError(t, err)

	validRecord := map[string]interface{}{
		"id":        "valid_record",
		"created_at": time.Now().Format(time.RFC3339),
	}
	validData, err := json.Marshal(validRecord)
	require.NoError(t, err)
	err = provider.Put(ctx, BucketDataUnits, []byte("valid_record"), validData)
	require.NoError(t, err)

	// Verify integrity
	report, err := manager.VerifyDatabaseIntegrity(ctx)
	require.NoError(t, err)
	assert.True(t, report.IsHealthy)
	assert.Equal(t, 0, report.ErrorCount)
	assert.Greater(t, report.RecordsChecked, 0)
}

func TestIntegrityManager_DetectCorruption(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Create a standard bucket with corrupted record (invalid JSON)
	err := provider.CreateBucket(ctx, BucketDataUnits)
	require.NoError(t, err)
	err = provider.Put(ctx, BucketDataUnits, []byte("corrupted"), []byte("invalid json {"))
	require.NoError(t, err)

	// Verify integrity should detect corruption
	report, err := manager.VerifyDatabaseIntegrity(ctx)
	require.NoError(t, err)
	assert.False(t, report.IsHealthy)
	assert.Greater(t, report.ErrorCount, 0)
}

func TestIntegrityManager_DetectCorruptionMethod(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Create a standard bucket with corrupted record
	err := provider.CreateBucket(ctx, BucketDataUnits)
	require.NoError(t, err)
	err = provider.Put(ctx, BucketDataUnits, []byte("corrupted"), []byte("invalid json"))
	require.NoError(t, err)

	// Detect corruption
	err = manager.DetectCorruption(ctx)
	assert.Error(t, err)
	assert.Equal(t, types.ErrCorruptionDetected, err)
}

func TestIntegrityManager_GetLastIntegrityReport(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Initially no report
	report := manager.GetLastIntegrityReport()
	assert.Nil(t, report)

	// Run verification
	_, err := manager.VerifyDatabaseIntegrity(ctx)
	require.NoError(t, err)

	// Now should have a report
	report = manager.GetLastIntegrityReport()
	assert.NotNil(t, report)
}

func TestIntegrityManager_PeriodicChecks(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := NewIntegrityManager(provider, logger)

	// Start periodic checks
	manager.StartPeriodicIntegrityChecks(ctx, 50*time.Millisecond)

	// Wait for first check to run
	time.Sleep(100 * time.Millisecond)

	// Verify that a check has been performed (by checking last check time)
	lastCheck := manager.GetLastCheckTime()
	assert.False(t, lastCheck.IsZero(), "Should have performed at least one check")

	// Cancel context to stop periodic checks
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestIntegrityManager_GetCorruptionRecoverySuggestions(t *testing.T) {
	logger := zap.NewNop()
	provider := newMockProvider()
	ctx := context.Background()

	manager := NewIntegrityManager(provider, logger)

	// Create a standard bucket with corrupted record
	err := provider.CreateBucket(ctx, BucketDataUnits)
	require.NoError(t, err)
	err = provider.Put(ctx, BucketDataUnits, []byte("corrupted"), []byte("invalid json"))
	require.NoError(t, err)

	// Get recovery suggestions
	suggestions, err := manager.GetCorruptionRecoverySuggestions(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, suggestions)
	// Check that suggestions contain recovery information
	assert.Greater(t, len(suggestions), 0)
	// Check that at least one suggestion mentions corruption or resync
	hasRelevantSuggestion := false
	for _, suggestion := range suggestions {
		if contains(suggestion, "corrupted") || contains(suggestion, "resync") || contains(suggestion, "integrity") {
			hasRelevantSuggestion = true
			break
		}
	}
	assert.True(t, hasRelevantSuggestion, "Should have at least one relevant recovery suggestion")
}

// Helper function for string contains check
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

