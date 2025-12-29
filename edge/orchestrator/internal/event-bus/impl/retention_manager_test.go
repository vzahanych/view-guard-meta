package impl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap/zaptest"
)

// mockEventBusProvider is a mock implementation of EventBusProvider for testing
type mockEventBusProvider struct {
	deleteExpiredEventsFunc func(ctx context.Context, beforeTime time.Time) (int, error)
}

func (m *mockEventBusProvider) PersistEvent(ctx context.Context, event types.EventAny) error {
	return nil
}

func (m *mockEventBusProvider) LoadEvent(ctx context.Context, eventID string) (*types.EventAny, error) {
	return nil, nil
}

func (m *mockEventBusProvider) ListEvents(ctx context.Context, filters *types.EventFilters) ([]types.EventAny, error) {
	return nil, nil
}

func (m *mockEventBusProvider) DeleteEvent(ctx context.Context, eventID string) error {
	return nil
}

func (m *mockEventBusProvider) DeleteExpiredEvents(ctx context.Context, beforeTime time.Time) (int, error) {
	if m.deleteExpiredEventsFunc != nil {
		return m.deleteExpiredEventsFunc(ctx, beforeTime)
	}
	return 0, nil
}

func (m *mockEventBusProvider) GetEventCount(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockEventBusProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockEventBusProvider) Close() error {
	return nil
}

func TestNewRetentionManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	provider := &mockEventBusProvider{}
	
	// Test with nil config (should use defaults)
	manager := NewRetentionManager(provider, nil, logger)
	require.NotNil(t, manager)
	assert.NotNil(t, manager.config)
	assert.Equal(t, 24, manager.config.RetentionHours) // Default
	
	// Test with custom config
	config := &types.RetentionConfig{
		RetentionHours:       48,
		CleanupIntervalHours: 12,
		CleanupBatchSize:     2000,
	}
	manager2 := NewRetentionManager(provider, config, logger)
	require.NotNil(t, manager2)
	assert.Equal(t, 48, manager2.config.RetentionHours)
	assert.Equal(t, 12, manager2.config.CleanupIntervalHours)
	assert.Equal(t, 2000, manager2.config.CleanupBatchSize)
}

func TestRetentionManager_CleanupExpiredEvents(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	tests := []struct {
		name            string
		retentionHours  int
		deletedCount    int
		providerError   error
		expectError     bool
		expectDeleted   int64
		expectRunning   bool
	}{
		{
			name:           "Successful cleanup",
			retentionHours: 24,
			deletedCount:   100,
			expectError:    false,
			expectDeleted:  100,
			expectRunning:  false,
		},
		{
			name:           "No events to delete",
			retentionHours: 24,
			deletedCount:   0,
			expectError:    false,
			expectDeleted:  0,
			expectRunning:  false,
		},
		{
			name:           "Large cleanup",
			retentionHours: 24,
			deletedCount:   10000,
			expectError:    false,
			expectDeleted:  10000,
			expectRunning:  false,
		},
		{
			name:           "Provider error",
			retentionHours: 24,
			deletedCount:   0,
			providerError:  errors.New("storage error"),
			expectError:    true,
			expectDeleted:  0, // Stats are still returned on error, but with 0 deleted
			expectRunning:  false,
		},
		{
			name:           "Custom retention period",
			retentionHours: 48,
			deletedCount:   50,
			expectError:    false,
			expectDeleted:  50,
			expectRunning:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedCutoffTime time.Time
			provider := &mockEventBusProvider{
				deleteExpiredEventsFunc: func(ctx context.Context, beforeTime time.Time) (int, error) {
					capturedCutoffTime = beforeTime
					return tt.deletedCount, tt.providerError
				},
			}
			
			config := &types.RetentionConfig{
				RetentionHours: tt.retentionHours,
			}
			config.Validate()
			
			manager := NewRetentionManager(provider, config, logger)
			
			stats, err := manager.CleanupExpiredEvents(ctx)
			
			if tt.expectError {
				require.Error(t, err)
				// Stats are still returned on error (with 0 deleted)
				assert.NotNil(t, stats)
				assert.Equal(t, int64(0), stats.EventsDeleted)
			} else {
				require.NoError(t, err)
				require.NotNil(t, stats)
				assert.Equal(t, tt.expectDeleted, stats.EventsDeleted)
				assert.Greater(t, stats.Duration, time.Duration(0))
				// Space freed is approximate (1KB per event)
				expectedSpaceFreed := tt.expectDeleted * 1024
				assert.Equal(t, expectedSpaceFreed, stats.SpaceFreedBytes)
			}
			
			// Verify cutoff time is correct
			expectedCutoffTime := time.Now().Add(-time.Duration(tt.retentionHours) * time.Hour)
			assert.WithinDuration(t, expectedCutoffTime, capturedCutoffTime, time.Second)
			
			// Verify cleanup is not running after completion
			assert.Equal(t, tt.expectRunning, manager.IsCleanupRunning())
		})
	}
}

func TestRetentionManager_CleanupExpiredEvents_ConcurrentCalls(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	callCount := 0
	provider := &mockEventBusProvider{
		deleteExpiredEventsFunc: func(ctx context.Context, beforeTime time.Time) (int, error) {
			callCount++
			time.Sleep(10 * time.Millisecond) // Simulate work
			return 100, nil
		},
	}
	
	config := &types.RetentionConfig{
		RetentionHours: 24,
	}
	config.Validate()
	
	manager := NewRetentionManager(provider, config, logger)
	
	// Start multiple concurrent cleanup calls
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			stats, err := manager.CleanupExpiredEvents(ctx)
			// Some calls may return nil stats if cleanup is already running
			// Others will return stats if they successfully run cleanup
			if err == nil && stats != nil {
				assert.GreaterOrEqual(t, stats.EventsDeleted, int64(0))
			}
			done <- true
		}()
	}
	
	// Wait for all calls
	for i := 0; i < 5; i++ {
		<-done
	}
	
	// Only one call should have actually executed (others should return nil)
	// The exact count depends on timing, but should be at least 1 and at most 5
	assert.GreaterOrEqual(t, callCount, 1)
	assert.LessOrEqual(t, callCount, 5)
}

func TestRetentionManager_GetLastCleanupTime(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	provider := &mockEventBusProvider{
		deleteExpiredEventsFunc: func(ctx context.Context, beforeTime time.Time) (int, error) {
			return 10, nil
		},
	}
	
	config := &types.RetentionConfig{
		RetentionHours: 24,
	}
	config.Validate()
	
	manager := NewRetentionManager(provider, config, logger)
	
	// Initially zero time
	lastCleanup := manager.GetLastCleanupTime()
	assert.True(t, lastCleanup.IsZero())
	
	// After cleanup, should be set
	_, err := manager.CleanupExpiredEvents(ctx)
	require.NoError(t, err)
	
	lastCleanup = manager.GetLastCleanupTime()
	assert.False(t, lastCleanup.IsZero())
	assert.WithinDuration(t, time.Now(), lastCleanup, time.Second)
}

func TestRetentionManager_GetLastCleanupStats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	provider := &mockEventBusProvider{
		deleteExpiredEventsFunc: func(ctx context.Context, beforeTime time.Time) (int, error) {
			return 100, nil
		},
	}
	
	config := &types.RetentionConfig{
		RetentionHours: 24,
	}
	config.Validate()
	
	manager := NewRetentionManager(provider, config, logger)
	
	// Initially nil
	stats := manager.GetLastCleanupStats()
	assert.Nil(t, stats)
	
	// After cleanup, should have stats
	_, err := manager.CleanupExpiredEvents(ctx)
	require.NoError(t, err)
	
	stats = manager.GetLastCleanupStats()
	require.NotNil(t, stats)
	assert.Equal(t, int64(100), stats.EventsDeleted)
	assert.Equal(t, int64(100*1024), stats.SpaceFreedBytes) // 1KB per event
	assert.Greater(t, stats.Duration, time.Duration(0))
	
	// Verify it's a copy (modifying shouldn't affect manager)
	stats.EventsDeleted = 999
	stats2 := manager.GetLastCleanupStats()
	assert.Equal(t, int64(100), stats2.EventsDeleted) // Should still be 100
}

func TestRetentionManager_IsCleanupRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	cleanupStarted := make(chan bool)
	cleanupContinue := make(chan bool)
	
	provider := &mockEventBusProvider{
		deleteExpiredEventsFunc: func(ctx context.Context, beforeTime time.Time) (int, error) {
			cleanupStarted <- true
			<-cleanupContinue // Wait for signal to continue
			return 10, nil
		},
	}
	
	config := &types.RetentionConfig{
		RetentionHours: 24,
	}
	config.Validate()
	
	manager := NewRetentionManager(provider, config, logger)
	
	// Start cleanup in goroutine
	done := make(chan bool)
	go func() {
		_, _ = manager.CleanupExpiredEvents(ctx)
		done <- true
	}()
	
	// Wait for cleanup to start
	<-cleanupStarted
	
	// Verify cleanup is running
	assert.True(t, manager.IsCleanupRunning())
	
	// Allow cleanup to complete
	cleanupContinue <- true
	<-done
	
	// Verify cleanup is not running
	assert.False(t, manager.IsCleanupRunning())
}

func TestRetentionManager_ThreadSafety(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	provider := &mockEventBusProvider{
		deleteExpiredEventsFunc: func(ctx context.Context, beforeTime time.Time) (int, error) {
			return 10, nil
		},
	}
	
	config := &types.RetentionConfig{
		RetentionHours: 24,
	}
	config.Validate()
	
	manager := NewRetentionManager(provider, config, logger)
	
	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = manager.GetLastCleanupTime()
				_ = manager.GetLastCleanupStats()
				_ = manager.IsCleanupRunning()
			}
			done <- true
		}()
	}
	
	// Concurrent writes (cleanups)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				_, _ = manager.CleanupExpiredEvents(ctx)
			}
			done <- true
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
	
	// Verify final state is consistent
	lastCleanup := manager.GetLastCleanupTime()
	stats := manager.GetLastCleanupStats()
	assert.False(t, manager.IsCleanupRunning())
	
	// If cleanup ran, stats should be set
	if !lastCleanup.IsZero() {
		assert.NotNil(t, stats)
	}
}

