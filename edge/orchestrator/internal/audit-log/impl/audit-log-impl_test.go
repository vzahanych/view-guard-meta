package impl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	objectstoragemocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/mocks"
	vmgatewaymocks "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/mocks"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
)

func TestAuditLogService_LogDataAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	entry := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{
			Result: "success",
		},
		ResourceType: "screenshot",
		ResourceID:   "screenshot-123",
		Action:       "read",
	}

	// Expect StoreSnapshot to be called
	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		DoAndReturn(func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
			// Verify the key format
			assert.Contains(t, key, "audit-logs/")
			assert.Contains(t, key, ".json")

			// Read and verify the content
			data, err := io.ReadAll(r)
			require.NoError(t, err)

			var storedEntry types.DataAccessEntry
			err = json.Unmarshal(data, &storedEntry)
			require.NoError(t, err)

			// Verify entry fields
			assert.Equal(t, types.EntryTypeDataAccess, storedEntry.Type)
			assert.Equal(t, "screenshot", storedEntry.ResourceType)
			assert.Equal(t, "screenshot-123", storedEntry.ResourceID)
			assert.Equal(t, "read", storedEntry.Action)
			assert.Equal(t, "success", storedEntry.Result)
			assert.NotEmpty(t, storedEntry.ID)
			assert.NotEmpty(t, storedEntry.Hash)
			assert.Equal(t, "test-edge-1", storedEntry.EdgeID)

			return nil
		}).
		Times(1)

	err := service.LogDataAccess(ctx, entry)
	require.NoError(t, err)
}

func TestAuditLogService_LogAuthentication(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	entry := types.AuthenticationEntry{
		AuditEntry: types.AuditEntry{
			Result:   "success",
			UserID:   "user-123",
			IPAddress: "192.168.1.1",
		},
		Method:   "api_key",
		Identity: "user-123",
	}

	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		DoAndReturn(func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
			data, err := io.ReadAll(r)
			require.NoError(t, err)

			var storedEntry types.AuthenticationEntry
			err = json.Unmarshal(data, &storedEntry)
			require.NoError(t, err)

			assert.Equal(t, types.EntryTypeAuthentication, storedEntry.Type)
			assert.Equal(t, "api_key", storedEntry.Method)
			assert.Equal(t, "user-123", storedEntry.Identity)
			assert.Equal(t, "success", storedEntry.Result)
			assert.NotEmpty(t, storedEntry.Hash)

			return nil
		}).
		Times(1)

	err := service.LogAuthentication(ctx, entry)
	require.NoError(t, err)
}

func TestAuditLogService_TamperProofHashChain(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	var firstHash, secondHash string

	// First entry
	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		DoAndReturn(func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
			data, err := io.ReadAll(r)
			require.NoError(t, err)

			var entry types.DataAccessEntry
			json.Unmarshal(data, &entry)

			// First entry should have empty previous hash
			assert.Empty(t, entry.PreviousHash)
			assert.NotEmpty(t, entry.Hash)
			firstHash = entry.Hash

			return nil
		}).
		Times(1)

	entry1 := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{Result: "success"},
		Action:     "read",
	}
	err := service.LogDataAccess(ctx, entry1)
	require.NoError(t, err)

	// Second entry should reference first entry's hash
	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		DoAndReturn(func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
			data, err := io.ReadAll(r)
			require.NoError(t, err)

			var entry types.DataAccessEntry
			json.Unmarshal(data, &entry)

			// Second entry should have first entry's hash as previous hash
			assert.Equal(t, firstHash, entry.PreviousHash)
			assert.NotEmpty(t, entry.Hash)
			secondHash = entry.Hash
			assert.NotEqual(t, firstHash, secondHash)

			return nil
		}).
		Times(1)

	entry2 := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{Result: "success"},
		Action:     "write",
	}
	err = service.LogDataAccess(ctx, entry2)
	require.NoError(t, err)

	// Third entry should reference second entry's hash
	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		DoAndReturn(func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
			data, err := io.ReadAll(r)
			require.NoError(t, err)

			var entry types.DataAccessEntry
			json.Unmarshal(data, &entry)

			// Third entry should have second entry's hash as previous hash
			assert.Equal(t, secondHash, entry.PreviousHash)
			assert.NotEmpty(t, entry.Hash)

			return nil
		}).
		Times(1)

	entry3 := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{Result: "success"},
		Action:     "delete",
	}
	err = service.LogDataAccess(ctx, entry3)
	require.NoError(t, err)
}

func TestAuditLogService_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       false, // Disabled
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	// When disabled, no storage should be called
	entry := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{Result: "success"},
		Action:     "read",
	}

	err := service.LogDataAccess(ctx, entry)
	require.NoError(t, err)

	// Verify no calls to object storage
	// (gomock will fail if any unexpected calls are made)
}

func TestAuditLogService_SyncToVM(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	// First, log some entries
	entry1 := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{
			Result:    "success",
			Timestamp: time.Now().Add(-30 * time.Minute),
		},
		Action: "read",
	}

	entry2 := types.AuthenticationEntry{
		AuditEntry: types.AuditEntry{
			Result:    "success",
			Timestamp: time.Now().Add(-15 * time.Minute),
		},
		Method: "api_key",
	}

	// Store entries
	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		Return(nil).
		Times(2)

	err := service.LogDataAccess(ctx, entry1)
	require.NoError(t, err)

	err = service.LogAuthentication(ctx, entry2)
	require.NoError(t, err)

	// Note: QueryAuditLogsFromStorage currently returns empty results (placeholder)
	// So SyncToVM will find no entries to sync
	// This test verifies the sync logic doesn't crash when there are no entries

	err = service.SyncToVM(ctx)
	require.NoError(t, err)
}

func TestAuditLogService_SyncToVM_WithEntries(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	// Mock QueryAuditLogs to return some entries
	// Since QueryAuditLogsFromStorage is a placeholder, we'll test the sync logic
	// by directly testing the syncBatchToVM method behavior

	// For now, test that sync handles empty results gracefully
	err := service.SyncToVM(ctx)
	require.NoError(t, err)

	// Test with VM gateway not available (nil check)
	serviceNoVM := NewAuditLogService(config, mockObjectStorage, nil, logger, "test-edge-1")
	err = serviceNoVM.SyncToVM(ctx)
	require.NoError(t, err) // Should return nil when VM gateway is nil
}

func TestAuditLogService_QueryAuditLogs(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	// QueryAuditLogsFromStorage is currently a placeholder
	// Test that it returns empty results without crashing
	filters := types.QueryFilters{
		StartTime: timePtr(time.Now().Add(-24 * time.Hour)),
		EndTime:   timePtr(time.Now()),
		Limit:     10,
	}

	entries, err := service.QueryAuditLogs(ctx, filters)
	require.NoError(t, err)
	assert.NotNil(t, entries)
	// Currently returns empty due to placeholder implementation
}

func TestAuditLogService_GetAuditLogEntry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	entryID := "test-entry-123"

	// Test entry not found - service searches through retention days
	// It will try to load from multiple dates (today, yesterday, etc.)
	mockObjectStorage.EXPECT().
		LoadSnapshot(gomock.Any(), gomock.Any()).
		Return(nil, io.EOF).
		Times(7) // RetentionDays = 7, so it tries 7 times

	entry, err := service.GetAuditLogEntry(ctx, entryID)
	assert.Error(t, err)
	assert.Nil(t, entry)
	assert.Contains(t, err.Error(), "not found")

	// Test entry found - reset controller for new test
	ctrl2 := gomock.NewController(t)
	defer ctrl2.Finish()

	mockObjectStorage2 := objectstoragemocks.NewMockObjectStorageService(ctrl2)
	service2 := NewAuditLogService(config, mockObjectStorage2, mockVMGateway, logger, "test-edge-1")

	entryData := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{
			ID:        entryID,
			Type:      types.EntryTypeDataAccess,
			Timestamp: time.Now(),
			Result:    "success",
		},
		Action: "read",
	}

	jsonData, _ := json.Marshal(entryData)

	// Entry found on first try (today's date)
	mockObjectStorage2.EXPECT().
		LoadSnapshot(gomock.Any(), gomock.Any()).
		Return(io.NopCloser(bytes.NewReader(jsonData)), nil).
		Times(1)

	entry, err = service2.GetAuditLogEntry(ctx, entryID)
	require.NoError(t, err)
	assert.NotNil(t, entry)
}

func TestAuditLogService_CleanupOldLogs(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	// CleanupOldLogs is currently a placeholder
	// Test that it doesn't crash
	err := service.CleanupOldLogs(ctx)
	require.NoError(t, err)
}

func TestAuditLogService_Lifecycle(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  100 * time.Millisecond, // Short interval for testing
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start service
	err := service.Start(ctx)
	require.NoError(t, err)

	// Give it a moment to start background goroutines
	time.Sleep(50 * time.Millisecond)

	// Stop service
	err = service.Stop(ctx)
	require.NoError(t, err)

	// Verify service name
	assert.Equal(t, "audit-log-service", service.Name())
}

func TestAuditLogService_AllEntryTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()

	// Test all entry types
	testCases := []struct {
		name  string
		entry interface{}
		logFn func(context.Context, interface{}) error
	}{
		{
			name: "DataAccess",
			entry: types.DataAccessEntry{
				AuditEntry: types.AuditEntry{Result: "success"},
				Action:     "read",
			},
			logFn: func(ctx context.Context, e interface{}) error {
				return service.LogDataAccess(ctx, e.(types.DataAccessEntry))
			},
		},
		{
			name: "Authentication",
			entry: types.AuthenticationEntry{
				AuditEntry: types.AuditEntry{Result: "success"},
				Method:     "api_key",
			},
			logFn: func(ctx context.Context, e interface{}) error {
				return service.LogAuthentication(ctx, e.(types.AuthenticationEntry))
			},
		},
		{
			name: "Authorization",
			entry: types.AuthorizationEntry{
				AuditEntry: types.AuditEntry{Result: "success"},
				Granted:    true,
			},
			logFn: func(ctx context.Context, e interface{}) error {
				return service.LogAuthorization(ctx, e.(types.AuthorizationEntry))
			},
		},
		{
			name: "ConfigurationChange",
			entry: types.ConfigurationChangeEntry{
				AuditEntry: types.AuditEntry{Result: "success"},
				Field:      "retention_days",
			},
			logFn: func(ctx context.Context, e interface{}) error {
				return service.LogConfigurationChange(ctx, e.(types.ConfigurationChangeEntry))
			},
		},
		{
			name: "ModelDeployment",
			entry: types.ModelDeploymentEntry{
				AuditEntry: types.AuditEntry{Result: "success"},
				Action:     "deploy",
			},
			logFn: func(ctx context.Context, e interface{}) error {
				return service.LogModelDeployment(ctx, e.(types.ModelDeploymentEntry))
			},
		},
		{
			name: "SecurityEvent",
			entry: types.SecurityEventEntry{
				AuditEntry: types.AuditEntry{Result: "success"},
				Severity:   "high",
			},
			logFn: func(ctx context.Context, e interface{}) error {
				return service.LogSecurityEvent(ctx, e.(types.SecurityEventEntry))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockObjectStorage.EXPECT().
				StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
				Return(nil).
				Times(1)

			err := tc.logFn(ctx, tc.entry)
			require.NoError(t, err)
		})
	}
}

func TestAuditLogService_DefaultConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	// Test with zero values - should use defaults
	config := &types.AuditLogConfig{
		Enabled: true,
		// RetentionDays: 0 (should default to 7)
		// SyncInterval: 0 (should default to 1 hour)
	}

	_ = NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	assert.Equal(t, 7, config.RetentionDays)
	assert.Equal(t, 1*time.Hour, config.SyncInterval)
}

func TestAuditLogService_StorageError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       true,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	entry := types.DataAccessEntry{
		AuditEntry: types.AuditEntry{Result: "success"},
		Action:     "read",
	}

	// Test storage error
	mockObjectStorage.EXPECT().
		StoreSnapshot(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "application/json").
		Return(io.EOF).
		Times(1)

	err := service.LogDataAccess(ctx, entry)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to store audit log entry")
}

func TestAuditLogService_GetAuditLogEntry_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       false,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	entry, err := service.GetAuditLogEntry(ctx, "test-entry")
	assert.Error(t, err)
	assert.Nil(t, entry)
	assert.Contains(t, err.Error(), "disabled")
}

func TestAuditLogService_QueryAuditLogs_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       false,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	filters := types.QueryFilters{
		Limit: 10,
	}

	entries, err := service.QueryAuditLogs(ctx, filters)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestAuditLogService_CleanupOldLogs_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       false,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	err := service.CleanupOldLogs(ctx)
	require.NoError(t, err)
}

func TestAuditLogService_SyncToVM_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjectStorage := objectstoragemocks.NewMockObjectStorageService(ctrl)
	mockVMGateway := vmgatewaymocks.NewMockVMGateway(ctrl)

	config := &types.AuditLogConfig{
		Enabled:       false,
		RetentionDays: 7,
		SyncInterval:  1 * time.Hour,
	}

	service := NewAuditLogService(config, mockObjectStorage, mockVMGateway, logger, "test-edge-1")

	ctx := context.Background()
	err := service.SyncToVM(ctx)
	require.NoError(t, err)
}

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}

