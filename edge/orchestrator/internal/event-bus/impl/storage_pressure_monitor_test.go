package impl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metastoragemocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/mocks"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
)

func TestNewStoragePressureMonitor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Test with nil meta-storage
	monitor := NewStoragePressureMonitor(nil, 90, logger)
	require.NotNil(t, monitor)
	assert.Equal(t, 90, monitor.GetThreshold())
	
	// Test with default threshold (0 or negative)
	monitor2 := NewStoragePressureMonitor(nil, 0, logger)
	assert.Equal(t, 90, monitor2.GetThreshold())
	
	monitor3 := NewStoragePressureMonitor(nil, -10, logger)
	assert.Equal(t, 90, monitor3.GetThreshold())
	
	// Test with custom threshold
	monitor4 := NewStoragePressureMonitor(nil, 85, logger)
	assert.Equal(t, 85, monitor4.GetThreshold())
}

func TestStoragePressureMonitor_IsStoragePressure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	monitor := NewStoragePressureMonitor(nil, 90, logger)
	
	// Initially no pressure (no check performed yet)
	assert.False(t, monitor.IsStoragePressure())
}

func TestStoragePressureMonitor_GetStorageUsagePercent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	monitor := NewStoragePressureMonitor(nil, 90, logger)
	
	// Initially 0% (no check performed yet)
	assert.Equal(t, 0.0, monitor.GetStorageUsagePercent())
}

func TestStoragePressureMonitor_CheckStoragePressure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	tests := []struct {
		name           string
		quota          *metastoragetypes.StorageQuota
		threshold      int
		expectedPressure bool
		expectedPercent  float64
	}{
		{
			name: "No pressure - below threshold",
			quota: &metastoragetypes.StorageQuota{
				Used:  8000, // 80%
				Limit: 10000,
			},
			threshold:        90,
			expectedPressure: false,
			expectedPercent:  80.0,
		},
		{
			name: "No pressure - at threshold boundary (below)",
			quota: &metastoragetypes.StorageQuota{
				Used:  8999, // 89.99%
				Limit: 10000,
			},
			threshold:        90,
			expectedPressure: false,
			expectedPercent:  89.99,
		},
		{
			name: "Pressure - at threshold",
			quota: &metastoragetypes.StorageQuota{
				Used:  9000, // 90%
				Limit: 10000,
			},
			threshold:        90,
			expectedPressure: true,
			expectedPercent:  90.0,
		},
		{
			name: "Pressure - above threshold",
			quota: &metastoragetypes.StorageQuota{
				Used:  9500, // 95%
				Limit: 10000,
			},
			threshold:        90,
			expectedPressure: true,
			expectedPercent:  95.0,
		},
		{
			name: "Pressure - full",
			quota: &metastoragetypes.StorageQuota{
				Used:  10000, // 100%
				Limit: 10000,
			},
			threshold:        90,
			expectedPressure: true,
			expectedPercent:  100.0,
		},
		{
			name: "No quota - no pressure",
			quota: nil,
			threshold:        90,
			expectedPressure: false,
			expectedPercent:  0.0,
		},
		{
			name: "Zero limit - no pressure",
			quota: &metastoragetypes.StorageQuota{
				Used:  1000,
				Limit: 0,
			},
			threshold:        90,
			expectedPressure: false,
			expectedPercent:  0.0,
		},
		{
			name: "Custom threshold - 85%",
			quota: &metastoragetypes.StorageQuota{
				Used:  8600, // 86%
				Limit: 10000,
			},
			threshold:        85,
			expectedPressure: true,
			expectedPercent:  86.0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			
			metaStore := metastoragemocks.NewMockMetaDataStore(ctrl)
			metaStore.EXPECT().HealthSnapshot(ctx).Return(metastoragetypes.StorageHealth{
				Quota: tt.quota,
			}).AnyTimes()
			
			monitor := NewStoragePressureMonitor(metaStore, tt.threshold, logger)
			
			hasPressure, err := monitor.CheckStoragePressure(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedPressure, hasPressure)
			
			// Verify cached values
			assert.Equal(t, tt.expectedPressure, monitor.IsStoragePressure())
			assert.InDelta(t, tt.expectedPercent, monitor.GetStorageUsagePercent(), 0.01)
		})
	}
}

func TestStoragePressureMonitor_CheckStoragePressure_NoMetaStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	monitor := NewStoragePressureMonitor(nil, 90, logger)
	
	hasPressure, err := monitor.CheckStoragePressure(ctx)
	require.NoError(t, err)
	assert.False(t, hasPressure)
	assert.Equal(t, 0.0, monitor.GetStorageUsagePercent())
}

func TestStoragePressureMonitor_GetLastCheckTime(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	metaStore := metastoragemocks.NewMockMetaDataStore(ctrl)
	metaStore.EXPECT().HealthSnapshot(ctx).Return(metastoragetypes.StorageHealth{
		Quota: &metastoragetypes.StorageQuota{
			Used:  5000,
			Limit: 10000,
		},
	}).AnyTimes()
	
	monitor := NewStoragePressureMonitor(metaStore, 90, logger)
	
	// Initially zero time
	lastCheck := monitor.GetLastCheckTime()
	assert.True(t, lastCheck.IsZero())
	
	// After check, should be set
	_, err := monitor.CheckStoragePressure(ctx)
	require.NoError(t, err)
	
	lastCheck = monitor.GetLastCheckTime()
	assert.False(t, lastCheck.IsZero())
	assert.WithinDuration(t, time.Now(), lastCheck, time.Second)
}

func TestStoragePressureMonitor_SetThreshold(t *testing.T) {
	logger := zaptest.NewLogger(t)
	monitor := NewStoragePressureMonitor(nil, 90, logger)
	
	// Set valid threshold
	monitor.SetThreshold(85)
	assert.Equal(t, 85, monitor.GetThreshold())
	
	// Set invalid threshold (should default to 90)
	monitor.SetThreshold(0)
	assert.Equal(t, 90, monitor.GetThreshold())
	
	monitor.SetThreshold(-10)
	assert.Equal(t, 90, monitor.GetThreshold())
}

func TestStoragePressureMonitor_GetThreshold(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	monitor := NewStoragePressureMonitor(nil, 90, logger)
	assert.Equal(t, 90, monitor.GetThreshold())
	
	monitor2 := NewStoragePressureMonitor(nil, 85, logger)
	assert.Equal(t, 85, monitor2.GetThreshold())
}

func TestStoragePressureMonitor_ThreadSafety(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	metaStore := metastoragemocks.NewMockMetaDataStore(ctrl)
	// Allow multiple calls to HealthSnapshot for concurrent tests
	metaStore.EXPECT().HealthSnapshot(ctx).Return(metastoragetypes.StorageHealth{
		Quota: &metastoragetypes.StorageQuota{
			Used:  9500, // 95% - above threshold
			Limit: 10000,
		},
	}).AnyTimes()
	
	monitor := NewStoragePressureMonitor(metaStore, 90, logger)
	
	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = monitor.IsStoragePressure()
				_ = monitor.GetStorageUsagePercent()
				_ = monitor.GetThreshold()
			}
			done <- true
		}()
	}
	
	// Concurrent writes (checks)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				_, _ = monitor.CheckStoragePressure(ctx)
			}
			done <- true
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
	
	// Verify final state
	assert.True(t, monitor.IsStoragePressure())
	assert.InDelta(t, 95.0, monitor.GetStorageUsagePercent(), 0.01)
}

func TestStoragePressureMonitor_PressureStateChange(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()
	
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	// Start with no pressure
	metaStore := metastoragemocks.NewMockMetaDataStore(ctrl)
	metaStore.EXPECT().HealthSnapshot(ctx).Return(metastoragetypes.StorageHealth{
		Quota: &metastoragetypes.StorageQuota{
			Used:  8000, // 80%
			Limit: 10000,
		},
	}).Times(1)
	
	monitor := NewStoragePressureMonitor(metaStore, 90, logger)
	
	hasPressure, err := monitor.CheckStoragePressure(ctx)
	require.NoError(t, err)
	assert.False(t, hasPressure)
	
	// Change to pressure
	metaStore.EXPECT().HealthSnapshot(ctx).Return(metastoragetypes.StorageHealth{
		Quota: &metastoragetypes.StorageQuota{
			Used:  9500, // 95%
			Limit: 10000,
		},
	}).Times(1)
	hasPressure, err = monitor.CheckStoragePressure(ctx)
	require.NoError(t, err)
	assert.True(t, hasPressure)
	
	// Change back to no pressure
	metaStore.EXPECT().HealthSnapshot(ctx).Return(metastoragetypes.StorageHealth{
		Quota: &metastoragetypes.StorageQuota{
			Used:  8000, // 80%
			Limit: 10000,
		},
	}).Times(1)
	hasPressure, err = monitor.CheckStoragePressure(ctx)
	require.NoError(t, err)
	assert.False(t, hasPressure)
}

