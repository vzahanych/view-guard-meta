package statemachine_test

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine/transitions"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// ExampleNewDeviceStateMachine demonstrates how to create a new device state machine.
func ExampleNewDeviceStateMachine() {
	logger := zap.NewNop()
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Initial state is undiscovered
	state := sm.GetState()
	fmt.Printf("Initial state: %s\n", state)
	fmt.Printf("Device ID: %s\n", sm.GetDeviceID())
	fmt.Printf("Device type: %s\n", sm.GetDeviceType())

	// Output:
	// Initial state: undiscovered
	// Device ID: device-1
	// Device type: camera
}

// ExampleDeviceStateMachine_Transition demonstrates valid state transitions.
func ExampleDeviceStateMachine_Transition() {
	logger := zap.NewNop()
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
		types.DeviceStateRegistered:    {types.DeviceStateActive},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Transition from undiscovered to discovered
	if err := sm.Transition(types.DeviceStateDiscovered, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("State: %s\n", sm.GetState())

	// Transition to registered
	if err := sm.Transition(types.DeviceStateRegistered, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("State: %s\n", sm.GetState())

	// Transition to active
	if err := sm.Transition(types.DeviceStateActive, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("State: %s\n", sm.GetState())

	// Output:
	// State: discovered
	// State: registered
	// State: active
}

// ExampleDeviceStateMachine_CanTransition demonstrates how to check if a transition is valid.
func ExampleDeviceStateMachine_CanTransition() {
	logger := zap.NewNop()
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
		types.DeviceStateRegistered:    {types.DeviceStateActive},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Check if we can transition to discovered (valid)
	canTransition := sm.CanTransition(types.DeviceStateDiscovered)
	fmt.Printf("Can transition to discovered: %v\n", canTransition)

	// Check if we can transition to active (invalid - must go through discovered and registered)
	canTransition = sm.CanTransition(types.DeviceStateActive)
	fmt.Printf("Can transition to active: %v\n", canTransition)

	// Transition to discovered
	_ = sm.Transition(types.DeviceStateDiscovered, "")

	// Now check if we can transition to registered (valid)
	canTransition = sm.CanTransition(types.DeviceStateRegistered)
	fmt.Printf("After transition to discovered, can transition to registered: %v\n", canTransition)

	// Output:
	// Can transition to discovered: true
	// Can transition to active: false
	// After transition to discovered, can transition to registered: true
}

// ExampleDeviceStateMachine_GetStateInfo demonstrates how to get detailed state information.
func ExampleDeviceStateMachine_GetStateInfo() {
	logger := zap.NewNop()
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
		types.DeviceStateDiscovered:   {types.DeviceStateRegistered},
		types.DeviceStateRegistered:    {types.DeviceStateActive},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Get state info
	stateInfo := sm.GetStateInfo()
	fmt.Printf("State: %s\n", stateInfo.State)
	fmt.Printf("Device ID: %s\n", stateInfo.DeviceID)
	fmt.Printf("Device type: %s\n", stateInfo.DeviceType)
	fmt.Printf("Is active: %v\n", stateInfo.IsActive)

	// Transition through states to active
	_ = sm.Transition(types.DeviceStateDiscovered, "")
	_ = sm.Transition(types.DeviceStateRegistered, "")
	_ = sm.Transition(types.DeviceStateActive, "")

	// Get updated state info
	stateInfo = sm.GetStateInfo()
	fmt.Printf("After transition - State: %s\n", stateInfo.State)
	fmt.Printf("After transition - Is active: %v\n", stateInfo.IsActive)

	// Output:
	// State: undiscovered
	// Device ID: device-1
	// Device type: camera
	// Is active: false
	// After transition - State: active
	// After transition - Is active: true
}

// ExampleDeviceStateMachine_SetMetadata demonstrates how to use metadata.
func ExampleDeviceStateMachine_SetMetadata() {
	logger := zap.NewNop()
	transitions := map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {types.DeviceStateDiscovered},
	}

	sm := statemachine.NewDeviceStateMachine("device-1", types.DeviceTypeCamera, transitions, logger)

	// Set metadata
	sm.SetMetadata("model_id", "model-123")
	sm.SetMetadata("dataset_id", "dataset-456")
	sm.SetMetadata("version", 1)

	// Get metadata
	modelID, exists := sm.GetMetadata("model_id")
	if exists {
		fmt.Printf("Model ID: %s\n", modelID)
	}

	datasetID, exists := sm.GetMetadata("dataset_id")
	if exists {
		fmt.Printf("Dataset ID: %s\n", datasetID)
	}

	version, exists := sm.GetMetadata("version")
	if exists {
		fmt.Printf("Version: %d\n", version)
	}

	// Output:
	// Model ID: model-123
	// Dataset ID: dataset-456
	// Version: 1
}

// ExampleNewDeviceStateMachineFactory demonstrates how to create a factory.
func ExampleNewDeviceStateMachineFactory() {
	logger := zap.NewNop()
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	// Create a state machine using the factory
	sm, err := factory.CreateStateMachine("device-1", types.DeviceTypeCamera)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("State machine created: %s\n", sm.GetDeviceID())
	fmt.Printf("Device type: %s\n", sm.GetDeviceType())
	fmt.Printf("Initial state: %s\n", sm.GetState())

	// Output:
	// State machine created: device-1
	// Device type: camera
	// Initial state: undiscovered
}

// ExampleDeviceStateMachineFactory_RegisterDeviceTypeTransitions demonstrates how to register device type transitions.
func ExampleDeviceStateMachineFactory_RegisterDeviceTypeTransitions() {
	logger := zap.NewNop()
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	// Register custom transitions for camera device type
	rules := []types.DeviceStateTransitionRule{
		{
			FromState: types.DeviceStateUndiscovered,
			ToStates:  []types.DeviceState{types.DeviceStateDiscovered},
		},
		{
			FromState: types.DeviceStateDiscovered,
			ToStates:  []types.DeviceState{types.DeviceStateRegistered},
		},
	}

	err := factory.RegisterDeviceTypeTransitions(types.DeviceTypeCamera, rules)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Create a state machine - it will use the registered transitions
	_, err = factory.CreateStateMachine("camera-1", types.DeviceTypeCamera)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Check valid transitions
	transitions := factory.GetValidTransitions(types.DeviceTypeCamera, types.DeviceStateUndiscovered)
	fmt.Printf("Valid transitions from undiscovered: %v\n", len(transitions) > 0)

	// Output:
	// Valid transitions from undiscovered: true
}

// ExampleNewDeviceStateMachineRegistry demonstrates how to create a registry.
func ExampleNewDeviceStateMachineRegistry() {
	logger := zap.NewNop()
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Get or create a state machine
	sm, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("State machine created: %s\n", sm.GetDeviceID())

	// Get the same state machine again (returns existing)
	sm2, err := registry.GetOrCreateStateMachine(ctx, "device-1", types.DeviceTypeCamera)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Same state machine returned: %v\n", sm == sm2)

	// Output:
	// State machine created: device-1
	// Same state machine returned: true
}

// ExampleDeviceStateMachineRegistry_GetStateMachinesByType demonstrates how to get state machines by type.
func ExampleDeviceStateMachineRegistry_GetStateMachinesByType() {
	logger := zap.NewNop()
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)

	ctx := context.Background()

	// Create state machines of different types
	_, _ = registry.GetOrCreateStateMachine(ctx, "camera-1", types.DeviceTypeCamera)
	_, _ = registry.GetOrCreateStateMachine(ctx, "camera-2", types.DeviceTypeCamera)
	_, _ = registry.GetOrCreateStateMachine(ctx, "sensor-1", types.DeviceTypeGenericSensor)

	// Get all cameras
	cameras := registry.GetStateMachinesByType(types.DeviceTypeCamera)
	fmt.Printf("Number of cameras: %d\n", len(cameras))

	// Get all sensors
	sensors := registry.GetStateMachinesByType(types.DeviceTypeGenericSensor)
	fmt.Printf("Number of sensors: %d\n", len(sensors))

	// Output:
	// Number of cameras: 2
	// Number of sensors: 1
}

// ExampleRegisterDefaultDeviceTypeTransitions demonstrates how to register default transitions.
func ExampleRegisterDefaultDeviceTypeTransitions() {
	logger := zap.NewNop()
	factory := statemachine.NewDeviceStateMachineFactory(logger)

	// Register default transitions for all device types
	err := transitions.RegisterDefaultDeviceTypeTransitions(factory)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Create a state machine - it will use the default transitions
	_, err = factory.CreateStateMachine("camera-1", types.DeviceTypeCamera)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Check valid transitions (should include defaults)
	validTransitions := factory.GetValidTransitions(types.DeviceTypeCamera, types.DeviceStateUndiscovered)
	fmt.Printf("Valid transitions from undiscovered: %d\n", len(validTransitions))

	// Output:
	// Valid transitions from undiscovered: 2
}

