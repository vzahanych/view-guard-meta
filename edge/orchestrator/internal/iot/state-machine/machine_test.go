package statemachine_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// TestNewDeviceStateMachine tests creating a new state machine
func TestNewDeviceStateMachine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	assert.NotNil(t, sm)
	assert.Equal(t, "device-1", sm.GetDeviceID())
	assert.Equal(t, types.DeviceTypeCamera, sm.GetDeviceType())
	assert.Equal(t, types.DeviceStateUndiscovered, sm.GetState())
}

// TestDeviceStateMachine_GetState tests getting the current state
func TestDeviceStateMachine_GetState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	state := sm.GetState()
	assert.Equal(t, types.DeviceStateUndiscovered, state)
}

// TestDeviceStateMachine_GetStateInfo tests getting detailed state information
func TestDeviceStateMachine_GetStateInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	stateInfo := sm.GetStateInfo()
	assert.Equal(t, "device-1", stateInfo.DeviceID)
	assert.Equal(t, types.DeviceTypeCamera, stateInfo.DeviceType)
	assert.Equal(t, types.DeviceStateUndiscovered, stateInfo.State)
	assert.False(t, stateInfo.IsActive)
	assert.NotNil(t, stateInfo.Metadata)
}

// TestDeviceStateMachine_Transition_Valid tests valid state transitions
func TestDeviceStateMachine_Transition_Valid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Transition from undiscovered to discovered
	err := sm.Transition(types.DeviceStateDiscovered, "")
	require.NoError(t, err)
	assert.Equal(t, types.DeviceStateDiscovered, sm.GetState())

	// Transition from discovered to registered
	err = sm.Transition(types.DeviceStateRegistered, "")
	require.NoError(t, err)
	assert.Equal(t, types.DeviceStateRegistered, sm.GetState())
}

// TestDeviceStateMachine_Transition_Invalid tests invalid state transitions
func TestDeviceStateMachine_Transition_Invalid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Try to transition to an invalid state (registered without going through discovered)
	err := sm.Transition(types.DeviceStateRegistered, "")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidTransition))
	assert.Equal(t, types.DeviceStateUndiscovered, sm.GetState())
}

// TestDeviceStateMachine_Transition_SameState tests transitioning to the same state (no-op)
func TestDeviceStateMachine_Transition_SameState(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Transition to the same state (should be valid)
	err := sm.Transition(types.DeviceStateUndiscovered, "")
	assert.NoError(t, err)
	assert.Equal(t, types.DeviceStateUndiscovered, sm.GetState())
}

// TestDeviceStateMachine_Transition_WithError tests transitioning with an error message
func TestDeviceStateMachine_Transition_WithError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateError},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Transition to error state with error message
	err := sm.Transition(types.DeviceStateError, "Connection failed")
	require.NoError(t, err)
	assert.Equal(t, types.DeviceStateError, sm.GetState())

	stateInfo := sm.GetStateInfo()
	assert.Equal(t, "Connection failed", stateInfo.Error)
}

// TestDeviceStateMachine_CanTransition tests checking if a transition is valid
func TestDeviceStateMachine_CanTransition(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Check valid transition
	assert.True(t, sm.CanTransition(types.DeviceStateDiscovered))

	// Check invalid transition
	assert.False(t, sm.CanTransition(types.DeviceStateRegistered))

	// Transition to discovered
	_ = sm.Transition(types.DeviceStateDiscovered, "")

	// Now registered should be valid
	assert.True(t, sm.CanTransition(types.DeviceStateRegistered))
}

// TestDeviceStateMachine_IsOperational tests checking if device is operational
func TestDeviceStateMachine_IsOperational(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
		types.DeviceStateRegistered:    {types.DeviceStateActive, types.DeviceStateProcessing},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Initially not operational
	assert.False(t, sm.IsOperational())

	// Transition to active
	_ = sm.Transition(types.DeviceStateDiscovered, "")
	_ = sm.Transition(types.DeviceStateRegistered, "")
	_ = sm.Transition(types.DeviceStateActive, "")
	assert.True(t, sm.IsOperational())

	// Transition to processing
	_ = sm.Transition(types.DeviceStateProcessing, "")
	assert.True(t, sm.IsOperational())
}

// TestDeviceStateMachine_IsReadyForProcessing tests checking if device is ready for processing
func TestDeviceStateMachine_IsReadyForProcessing(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
		types.DeviceStateRegistered:    {types.DeviceStateActive, types.DeviceStateProcessing},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Initially not ready
	assert.False(t, sm.IsReadyForProcessing())

	// Transition to active
	_ = sm.Transition(types.DeviceStateDiscovered, "")
	_ = sm.Transition(types.DeviceStateRegistered, "")
	_ = sm.Transition(types.DeviceStateActive, "")
	assert.True(t, sm.IsReadyForProcessing())

	// Transition to processing
	_ = sm.Transition(types.DeviceStateProcessing, "")
	assert.True(t, sm.IsReadyForProcessing())
}

// TestDeviceStateMachine_SetMetadata tests setting metadata
func TestDeviceStateMachine_SetMetadata(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Set metadata
	sm.SetMetadata("key1", "value1")
	sm.SetMetadata("key2", 42)

	// Get metadata
	value1, exists := sm.GetMetadata("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value1)

	value2, exists := sm.GetMetadata("key2")
	assert.True(t, exists)
	assert.Equal(t, 42, value2)

	// Get non-existent metadata
	_, exists = sm.GetMetadata("key3")
	assert.False(t, exists)
}

// TestDeviceStateMachine_GetMetadata tests getting metadata
func TestDeviceStateMachine_GetMetadata(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Get metadata that doesn't exist
	_, exists := sm.GetMetadata("nonexistent")
	assert.False(t, exists)

	// Set and get metadata
	sm.SetMetadata("test", "value")
	value, exists := sm.GetMetadata("test")
	assert.True(t, exists)
	assert.Equal(t, "value", value)
}

// TestDeviceStateMachine_ConcurrentAccess tests concurrent access to state machine
func TestDeviceStateMachine_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.GetState()
			_ = sm.GetStateInfo()
			_ = sm.CanTransition(types.DeviceStateDiscovered)
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.Transition(types.DeviceStateDiscovered, "")
			_ = sm.Transition(types.DeviceStateUndiscovered, "")
		}()
	}

	wg.Wait()
	// Should not panic or deadlock
}

// TestDeviceStateMachine_StateInfoCopy tests that GetStateInfo returns a copy
func TestDeviceStateMachine_StateInfoCopy(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	stateInfo1 := sm.GetStateInfo()
	stateInfo2 := sm.GetStateInfo()

	// Modify metadata in first copy
	stateInfo1.Metadata["test"] = "value"

	// Second copy should not be affected
	_, exists := stateInfo2.Metadata["test"]
	assert.False(t, exists)
}

// TestNewDeviceStateMachineFactory tests creating a new factory
func TestNewDeviceStateMachineFactory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	assert.NotNil(t, factory)
}

// TestDeviceStateMachineFactory_CreateStateMachine tests creating a state machine via factory
func TestDeviceStateMachineFactory_CreateStateMachine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	sm, err := factory.CreateStateMachine("device-1", types.DeviceTypeCamera)
	require.NoError(t, err)
	assert.NotNil(t, sm)
	assert.Equal(t, "device-1", sm.GetDeviceID())
	assert.Equal(t, types.DeviceTypeCamera, sm.GetDeviceType())
}

// TestDeviceStateMachineFactory_GetValidTransitions tests getting valid transitions
func TestDeviceStateMachineFactory_GetValidTransitions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	// Get valid transitions for undiscovered state (should use defaults)
	transitions := factory.GetValidTransitions(types.DeviceTypeCamera, types.DeviceStateUndiscovered)
	assert.NotEmpty(t, transitions)
}

// TestDeviceStateMachineFactory_RegisterDeviceTypeTransitions tests registering device type transitions
func TestDeviceStateMachineFactory_RegisterDeviceTypeTransitions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	rules := []types.DeviceStateTransitionRule{
		{
			FromState: types.DeviceStateUndiscovered,
			ToStates:  []types.DeviceState{types.DeviceStateDiscovered},
		},
	}

	err := factory.RegisterDeviceTypeTransitions(types.DeviceTypeCamera, rules)
	require.NoError(t, err)

	// Verify transitions are registered
	transitions := factory.GetValidTransitions(types.DeviceTypeCamera, types.DeviceStateUndiscovered)
	assert.Contains(t, transitions, types.DeviceStateDiscovered)
}

// TestNewDeviceStateMachineRegistry tests creating a new registry
func TestNewDeviceStateMachineRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	assert.NotNil(t, registry)
}

// TestDeviceStateMachineRegistry_GetOrCreateStateMachine tests getting or creating a state machine
func TestDeviceStateMachineRegistry_GetOrCreateStateMachine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Create a new state machine
	sm1, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	require.NoError(t, err)
	assert.NotNil(t, sm1)
	assert.Equal(t, "device-1", sm1.GetDeviceID())

	// Get the same state machine (should return existing)
	sm2, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	require.NoError(t, err)
	assert.Equal(t, sm1, sm2)
}

// TestDeviceStateMachineRegistry_GetStateMachine tests getting an existing state machine
func TestDeviceStateMachineRegistry_GetStateMachine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Create a state machine
	_, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	require.NoError(t, err)

	// Get the state machine
	sm, err := registry.GetStateMachine("device-1")
	require.NoError(t, err)
	assert.NotNil(t, sm)
	assert.Equal(t, "device-1", sm.GetDeviceID())

	// Try to get non-existent state machine
	_, err = registry.GetStateMachine("device-2")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrStateMachineNotFound))
}

// TestDeviceStateMachineRegistry_CreateStateMachine tests creating a new state machine
func TestDeviceStateMachineRegistry_CreateStateMachine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	// Create a new state machine
	sm, err := registry.CreateStateMachine("device-1", types.DeviceTypeCamera)
	require.NoError(t, err)
	assert.NotNil(t, sm)
	assert.Equal(t, "device-1", sm.GetDeviceID())

	// Try to create duplicate state machine
	_, err = registry.CreateStateMachine("device-1", types.DeviceTypeCamera)
	assert.Error(t, err)
}

// TestDeviceStateMachineRegistry_RemoveStateMachine tests removing a state machine
func TestDeviceStateMachineRegistry_RemoveStateMachine(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Create a state machine
	_, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	require.NoError(t, err)

	// Remove the state machine
	err = registry.RemoveStateMachine("device-1")
	require.NoError(t, err)

	// Try to get removed state machine
	_, err = registry.GetStateMachine("device-1")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrStateMachineNotFound))
}

// TestDeviceStateMachineRegistry_GetAllStateMachines tests getting all state machines
func TestDeviceStateMachineRegistry_GetAllStateMachines(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Create multiple state machines
	_, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	require.NoError(t, err)
	_, err = registry.GetOrCreateStateMachine(ctx, "device-2", types.DeviceTypeGenericSensor)
	require.NoError(t, err)

	// Get all state machines
	machines := registry.GetAllStateMachines()
	assert.Len(t, machines, 2)
}

// TestDeviceStateMachineRegistry_GetStateMachinesByType tests getting state machines by type
func TestDeviceStateMachineRegistry_GetStateMachinesByType(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Create state machines of different types
	_, err := registry.GetOrCreateStateMachine(ctx, "camera-1", types.DeviceTypeCamera)
	require.NoError(t, err)
	_, err = registry.GetOrCreateStateMachine(ctx, "camera-2", types.DeviceTypeCamera)
	require.NoError(t, err)
	_, err = registry.GetOrCreateStateMachine(ctx, "sensor-1", types.DeviceTypeGenericSensor)
	require.NoError(t, err)

	// Get cameras only
	cameras := registry.GetStateMachinesByType(types.DeviceTypeCamera)
	assert.Len(t, cameras, 2)

	// Get sensors only
	sensors := registry.GetStateMachinesByType(types.DeviceTypeGenericSensor)
	assert.Len(t, sensors, 1)
}

// TestDeviceStateMachineRegistry_ConcurrentAccess tests concurrent access to registry
func TestDeviceStateMachineRegistry_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Concurrent creates
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deviceID := fmt.Sprintf("device-%d", id)
			_, err := registry.GetOrCreateStateMachine(ctx, deviceID, types.DeviceTypeCamera)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all machines were created
	machines := registry.GetAllStateMachines()
	assert.Len(t, machines, 100)
}

// TestDeviceStateMachine_StateInfoLastUpdated tests that LastUpdated is updated on transitions
func TestDeviceStateMachine_StateInfoLastUpdated(t *testing.T) {
	logger := zaptest.NewLogger(t)
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	stateInfo1 := sm.GetStateInfo()
	time.Sleep(10 * time.Millisecond)

	// Transition to new state
	_ = sm.Transition(types.DeviceStateDiscovered, "")

	stateInfo2 := sm.GetStateInfo()
	assert.True(t, stateInfo2.LastUpdated.After(stateInfo1.LastUpdated))
}

