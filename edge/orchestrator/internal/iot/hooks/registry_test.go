package hooks_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// mockDevice is a minimal test implementation of Device
type mockDevice struct {
	id       string
	deviceType types.DeviceType
	metadata types.DeviceMetadata
}

func (m *mockDevice) GetID() string {
	return m.id
}

func (m *mockDevice) GetMetadata() types.DeviceMetadata {
	return m.metadata
}

func (m *mockDevice) Start(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Stop(ctx context.Context) error {
	return nil
}

func (m *mockDevice) UpdateMetadata(ctx context.Context, updates *types.DeviceMetadataUpdate) error {
	return nil
}

func (m *mockDevice) Enable(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Disable(ctx context.Context) error {
	return nil
}

func (m *mockDevice) IsEnabled() bool {
	return true
}

func (m *mockDevice) GetStatus() types.DeviceStatus {
	return types.DeviceStatusOnline
}

func (m *mockDevice) GetAvailableCommands(ctx context.Context) ([]types.DeviceCommand, error) {
	return []types.DeviceCommand{}, nil
}

func (m *mockDevice) HasCapability(capability types.DeviceCapability) bool {
	return false
}

func (m *mockDevice) CaptureData(ctx context.Context) (*types.DeviceData, error) {
	return nil, nil
}

func (m *mockDevice) StartDataStream(ctx context.Context) (<-chan *types.DeviceData, error) {
	return nil, nil
}

func (m *mockDevice) StopDataStream(ctx context.Context) error {
	return nil
}

func (m *mockDevice) ReadSensor(ctx context.Context, sensorType string) (*types.SensorReading, error) {
	return nil, nil
}

func (m *mockDevice) ReadAllSensors(ctx context.Context) (map[string]*types.SensorReading, error) {
	return nil, nil
}

func (m *mockDevice) ExecuteCommand(ctx context.Context, command types.DeviceCommand) error {
	return nil
}

func (m *mockDevice) GetCapabilities() types.DeviceCapabilities {
	return types.DeviceCapabilities{}
}

// TestNewLifecycleHookRegistry tests creating a new hook registry
func TestNewLifecycleHookRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	assert.NotNil(t, registry)
	
	// Test with nil logger
	registry2 := hooks.NewLifecycleHookRegistry(nil)
	assert.NotNil(t, registry2)
}

// TestLifecycleHookRegistry_RegisterHook_Valid tests registering valid hooks
func TestLifecycleHookRegistry_RegisterHook_Valid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Test discovery hook
	hook1 := &types.LifecycleHook{
		ID:   "discovery-hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Test Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
		Priority: 1,
	}

	err := registry.RegisterHook(hook1)
	require.NoError(t, err)

	// Test registration hook
	hook2 := &types.LifecycleHook{
		ID:   "registration-hook-1",
		Type: types.HookTypeRegistration,
		Name: "Test Registration Hook",
		RegistrationHook: func(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
			return nil
		},
		Priority: 2,
	}

	err = registry.RegisterHook(hook2)
	require.NoError(t, err)

	// Test data collection hook
	hook3 := &types.LifecycleHook{
		ID:   "data-collection-hook-1",
		Type: types.HookTypeDataCollection,
		Name: "Test Data Collection Hook",
		DataCollectionHook: func(ctx context.Context, hookCtx *types.DataCollectionHookContext) error {
			return nil
		},
		Priority: 3,
	}

	err = registry.RegisterHook(hook3)
	require.NoError(t, err)

	// Test teardown hook
	hook4 := &types.LifecycleHook{
		ID:   "teardown-hook-1",
		Type: types.HookTypeTeardown,
		Name: "Test Teardown Hook",
		TeardownHook: func(ctx context.Context, hookCtx *types.TeardownHookContext) error {
			return nil
		},
		Priority: 4,
	}

	err = registry.RegisterHook(hook4)
	require.NoError(t, err)
}

// TestLifecycleHookRegistry_RegisterHook_Invalid tests registering invalid hooks
func TestLifecycleHookRegistry_RegisterHook_Invalid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Test nil hook
	err := registry.RegisterHook(nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))

	// Test hook with empty ID
	hook1 := &types.LifecycleHook{
		ID:   "",
		Type: types.HookTypeDiscovery,
		Name: "Test Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}
	err = registry.RegisterHook(hook1)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))

	// Test hook with empty name
	hook2 := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}
	err = registry.RegisterHook(hook2)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))

	// Test discovery hook with nil function
	hook3 := &types.LifecycleHook{
		ID:   "hook-2",
		Type: types.HookTypeDiscovery,
		Name: "Test Hook",
		DiscoveryHook: nil,
	}
	err = registry.RegisterHook(hook3)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))

	// Test duplicate hook ID
	hook4 := &types.LifecycleHook{
		ID:   "hook-3",
		Type: types.HookTypeDiscovery,
		Name: "Test Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}
	err = registry.RegisterHook(hook4)
	require.NoError(t, err)

	// Try to register duplicate
	err = registry.RegisterHook(hook4)
	assert.Error(t, err)
}

// TestLifecycleHookRegistry_RegisterHook_DefaultPriority tests default priority assignment
func TestLifecycleHookRegistry_RegisterHook_DefaultPriority(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	hook := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Test Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
		Priority: 0, // Should default to 100
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	// Priority should be set to default (100)
	retrieved, err := registry.GetHook("hook-1")
	require.NoError(t, err)
	assert.Equal(t, 100, retrieved.Priority)
}

// TestLifecycleHookRegistry_UnregisterHook tests unregistering hooks
func TestLifecycleHookRegistry_UnregisterHook(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	hook := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Test Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	// Unregister
	err = registry.UnregisterHook("hook-1")
	require.NoError(t, err)

	// Try to get unregistered hook
	_, err = registry.GetHook("hook-1")
	assert.Error(t, err)

	// Try to unregister non-existent hook
	err = registry.UnregisterHook("non-existent")
	assert.Error(t, err)
}

// TestLifecycleHookRegistry_GetHook tests retrieving hooks
func TestLifecycleHookRegistry_GetHook(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	hook := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Test Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
		Priority: 50,
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	// Get hook
	retrieved, err := registry.GetHook("hook-1")
	require.NoError(t, err)
	assert.Equal(t, "hook-1", retrieved.ID)
	assert.Equal(t, "Test Hook", retrieved.Name)
	assert.Equal(t, types.HookTypeDiscovery, retrieved.Type)
	assert.Equal(t, 50, retrieved.Priority)

	// Get non-existent hook
	_, err = registry.GetHook("non-existent")
	assert.Error(t, err)
}

// TestLifecycleHookRegistry_ListHooks tests listing hooks
func TestLifecycleHookRegistry_ListHooks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Register multiple hooks of different types
	hook1 := &types.LifecycleHook{
		ID:   "discovery-hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Discovery Hook 1",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}

	hook2 := &types.LifecycleHook{
		ID:   "discovery-hook-2",
		Type: types.HookTypeDiscovery,
		Name: "Discovery Hook 2",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}

	hook3 := &types.LifecycleHook{
		ID:   "registration-hook-1",
		Type: types.HookTypeRegistration,
		Name: "Registration Hook 1",
		RegistrationHook: func(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
			return nil
		},
	}

	err := registry.RegisterHook(hook1)
	require.NoError(t, err)
	err = registry.RegisterHook(hook2)
	require.NoError(t, err)
	err = registry.RegisterHook(hook3)
	require.NoError(t, err)

	// List all hooks
	allHooks := registry.ListHooks(nil)
	assert.Len(t, allHooks, 3)

	// List discovery hooks only
	discoveryType := types.HookTypeDiscovery
	discoveryHooks := registry.ListHooks(&discoveryType)
	assert.Len(t, discoveryHooks, 2)

	// List registration hooks only
	registrationType := types.HookTypeRegistration
	registrationHooks := registry.ListHooks(&registrationType)
	assert.Len(t, registrationHooks, 1)
}

// TestLifecycleHookRegistry_ExecuteDiscoveryHooks tests executing discovery hooks
func TestLifecycleHookRegistry_ExecuteDiscoveryHooks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executed := false
	hook := &types.LifecycleHook{
		ID:   "discovery-hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Test Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executed = true
			return nil
		},
		Enabled: true,
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	hookCtx := &types.DiscoveryHookContext{
		DeviceType: types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	err = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.True(t, executed)
}

// TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Filter tests hook filtering
func TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Filter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executed1 := false
	executed2 := false

	// Hook with device type filter (camera only)
	deviceType := types.DeviceTypeCamera
	hook1 := &types.LifecycleHook{
		ID:   "discovery-hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Camera Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executed1 = true
			return nil
		},
		DeviceTypeFilter: &deviceType,
		Enabled: true,
	}

	// Hook with no filter (all device types)
	hook2 := &types.LifecycleHook{
		ID:   "discovery-hook-2",
		Type: types.HookTypeDiscovery,
		Name: "Generic Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executed2 = true
			return nil
		},
		Enabled: true,
	}

	err := registry.RegisterHook(hook1)
	require.NoError(t, err)
	err = registry.RegisterHook(hook2)
	require.NoError(t, err)

	// Execute with camera device type - both should execute
	hookCtx := &types.DiscoveryHookContext{
		DeviceType: types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	err = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.True(t, executed1)
	assert.True(t, executed2)

	// Reset
	executed1 = false
	executed2 = false

	// Execute with sensor device type - only hook2 should execute
	hookCtx.DeviceType = types.DeviceTypeGenericSensor
	err = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.False(t, executed1) // Filtered out
	assert.True(t, executed2)    // No filter
}

// TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Disabled tests disabled hooks
func TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executed := false
	hook := &types.LifecycleHook{
		ID:   "discovery-hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Disabled Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executed = true
			return nil
		},
		Enabled: true, // Register as enabled first
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	// Disable the hook after registration
	hook.Enabled = false

	hookCtx := &types.DiscoveryHookContext{
		DeviceType: types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	err = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.False(t, executed) // Should not execute because hook is disabled
}

// TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Priority tests hook priority ordering
func TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Priority(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executionOrder := make([]string, 0)

	// Register hooks with different priorities
	hook1 := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Hook 1 (Priority 100)",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executionOrder = append(executionOrder, "hook-1")
			return nil
		},
		Priority: 100,
		Enabled: true,
	}

	hook2 := &types.LifecycleHook{
		ID:   "hook-2",
		Type: types.HookTypeDiscovery,
		Name: "Hook 2 (Priority 10)",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executionOrder = append(executionOrder, "hook-2")
			return nil
		},
		Priority: 10, // Lower priority = earlier execution
		Enabled: true,
	}

	hook3 := &types.LifecycleHook{
		ID:   "hook-3",
		Type: types.HookTypeDiscovery,
		Name: "Hook 3 (Priority 50)",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executionOrder = append(executionOrder, "hook-3")
			return nil
		},
		Priority: 50,
		Enabled: true,
	}

	// Register in non-priority order
	err := registry.RegisterHook(hook1)
	require.NoError(t, err)
	err = registry.RegisterHook(hook2)
	require.NoError(t, err)
	err = registry.RegisterHook(hook3)
	require.NoError(t, err)

	hookCtx := &types.DiscoveryHookContext{
		DeviceType: types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	err = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	require.NoError(t, err)

	// Should execute in priority order: hook-2 (10), hook-3 (50), hook-1 (100)
	assert.Len(t, executionOrder, 3)
	assert.Equal(t, "hook-2", executionOrder[0])
	assert.Equal(t, "hook-3", executionOrder[1])
	assert.Equal(t, "hook-1", executionOrder[2])
}

// TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Error tests error handling
func TestLifecycleHookRegistry_ExecuteDiscoveryHooks_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	testErr := errors.New("hook execution error")

	hook1 := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Error Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return testErr
		},
		Enabled: true,
	}

	executed2 := false
	hook2 := &types.LifecycleHook{
		ID:   "hook-2",
		Type: types.HookTypeDiscovery,
		Name: "Success Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			executed2 = true
			return nil
		},
		Enabled: true,
	}

	err := registry.RegisterHook(hook1)
	require.NoError(t, err)
	err = registry.RegisterHook(hook2)
	require.NoError(t, err)

	hookCtx := &types.DiscoveryHookContext{
		DeviceType: types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	// Should return first error but continue executing other hooks
	err = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.True(t, executed2) // Second hook should still execute
}

// TestLifecycleHookRegistry_ConcurrentAccess tests concurrent access
func TestLifecycleHookRegistry_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	var wg sync.WaitGroup
	numGoroutines := 10
	numHooks := 10

	// Concurrent registration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numHooks; j++ {
				hook := &types.LifecycleHook{
					ID:   fmt.Sprintf("hook-%d-%d", id, j),
					Type: types.HookTypeDiscovery,
					Name: fmt.Sprintf("Hook %d-%d", id, j),
					DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
						return nil
					},
				}
				_ = registry.RegisterHook(hook)
			}
		}(i)
	}

	wg.Wait()

	// Verify all hooks registered
	allHooks := registry.ListHooks(nil)
	assert.GreaterOrEqual(t, len(allHooks), numGoroutines*numHooks)
}

// TestLifecycleHookRegistry_ExecuteRegistrationHooks tests registration hooks
func TestLifecycleHookRegistry_ExecuteRegistrationHooks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executed := false
	hook := &types.LifecycleHook{
		ID:   "registration-hook-1",
		Type: types.HookTypeRegistration,
		Name: "Test Registration Hook",
		RegistrationHook: func(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
			executed = true
			return nil
		},
		Enabled: true,
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	device := &mockDevice{
		id: "device-1",
		deviceType: types.DeviceTypeCamera,
		metadata: types.DeviceMetadata{
			ID:   "device-1",
			Type: types.DeviceTypeCamera,
		},
	}

	hookCtx := &types.RegistrationHookContext{
		Device: device,
		Metadata: device.GetMetadata(),
		Capabilities: types.DeviceCapabilities{},
	}

	err = registry.ExecuteRegistrationHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.True(t, executed)
}

// TestLifecycleHookRegistry_ExecuteDataCollectionHooks tests data collection hooks
func TestLifecycleHookRegistry_ExecuteDataCollectionHooks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executed := false
	hook := &types.LifecycleHook{
		ID:   "data-collection-hook-1",
		Type: types.HookTypeDataCollection,
		Name: "Test Data Collection Hook",
		DataCollectionHook: func(ctx context.Context, hookCtx *types.DataCollectionHookContext) error {
			executed = true
			return nil
		},
		Enabled: true,
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	device := &mockDevice{
		id: "device-1",
		deviceType: types.DeviceTypeCamera,
		metadata: types.DeviceMetadata{
			ID:   "device-1",
			Type: types.DeviceTypeCamera,
		},
	}

	hookCtx := &types.DataCollectionHookContext{
		Device:   device,
		DataType: types.DeviceDataTypeVideoFrame,
		Operation: "capture",
	}

	err = registry.ExecuteDataCollectionHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.True(t, executed)
}

// TestLifecycleHookRegistry_ExecuteTeardownHooks tests teardown hooks
func TestLifecycleHookRegistry_ExecuteTeardownHooks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := hooks.NewLifecycleHookRegistry(logger)

	executed := false
	hook := &types.LifecycleHook{
		ID:   "teardown-hook-1",
		Type: types.HookTypeTeardown,
		Name: "Test Teardown Hook",
		TeardownHook: func(ctx context.Context, hookCtx *types.TeardownHookContext) error {
			executed = true
			return nil
		},
		Enabled: true,
	}

	err := registry.RegisterHook(hook)
	require.NoError(t, err)

	device := &mockDevice{
		id: "device-1",
		deviceType: types.DeviceTypeCamera,
		metadata: types.DeviceMetadata{
			ID:   "device-1",
			Type: types.DeviceTypeCamera,
		},
	}

	hookCtx := &types.TeardownHookContext{
		Device: device,
		Reason: "shutdown",
	}

	err = registry.ExecuteTeardownHooks(context.Background(), hookCtx)
	require.NoError(t, err)
	assert.True(t, executed)
}

