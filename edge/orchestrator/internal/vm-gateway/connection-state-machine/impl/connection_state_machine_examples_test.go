package impl

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// ExampleNewConnectionStateMachine demonstrates how to create a new connection state machine.
func ExampleNewConnectionStateMachine() {
	machine := NewConnectionStateMachine()

	// Initial state is disconnected
	state := machine.GetState()
	fmt.Printf("Initial state: %s\n", state)

	// Output:
	// Initial state: disconnected
}

// ExampleConnectionStateMachine_Transition demonstrates valid state transitions.
func ExampleConnectionStateMachine_Transition() {
	machine := NewConnectionStateMachine()

	// Transition from disconnected to tunnel_connecting
	if err := machine.Transition(types.ConnectionStateTunnelConnecting, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("State: %s\n", machine.GetState())

	// Transition to tunnel_connected
	if err := machine.Transition(types.ConnectionStateTunnelConnected, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("State: %s\n", machine.GetState())

	// Output:
	// State: tunnel_connecting
	// State: tunnel_connected
}

// ExampleConnectionStateMachine_CanTransition demonstrates how to check if a transition is valid.
func ExampleConnectionStateMachine_CanTransition() {
	machine := NewConnectionStateMachine()

	// Check if we can transition to tunnel_connecting (valid)
	canTransition := machine.CanTransition(types.ConnectionStateTunnelConnecting)
	fmt.Printf("Can transition to tunnel_connecting: %v\n", canTransition)

	// Check if we can transition to authenticated (invalid - must go through intermediate states)
	canTransition = machine.CanTransition(types.ConnectionStateAuthenticated)
	fmt.Printf("Can transition to authenticated: %v\n", canTransition)

	// Output:
	// Can transition to tunnel_connecting: true
	// Can transition to authenticated: false
}

// ExampleConnectionStateMachine_GetStateInfo demonstrates how to get detailed state information.
func ExampleConnectionStateMachine_GetStateInfo() {
	machine := NewConnectionStateMachine()

	// Get state info
	stateInfo := machine.GetStateInfo()
	fmt.Printf("State: %s\n", stateInfo.State)
	fmt.Printf("VM reachable: %v\n", stateInfo.VMReachable)
	fmt.Printf("Network health: %s\n", stateInfo.NetworkHealth)

	// Transition through states to tunnel_connected
	_ = machine.Transition(types.ConnectionStateTunnelConnecting, "")
	_ = machine.Transition(types.ConnectionStateTunnelConnected, "")

	// Get updated state info
	stateInfo = machine.GetStateInfo()
	fmt.Printf("After transition - State: %s\n", stateInfo.State)
	fmt.Printf("After transition - VM reachable: %v\n", stateInfo.VMReachable)
	fmt.Printf("After transition - Network health: %s\n", stateInfo.NetworkHealth)

	// Output:
	// State: disconnected
	// VM reachable: false
	// Network health: unhealthy
	// After transition - State: tunnel_connected
	// After transition - VM reachable: true
	// After transition - Network health: healthy
}

// ExampleIsValidTransition demonstrates the transition validation function.
func ExampleIsValidTransition() {
	// Valid transition: disconnected → tunnel_connecting
	valid := IsValidTransition(types.ConnectionStateDisconnected, types.ConnectionStateTunnelConnecting)
	fmt.Printf("disconnected → tunnel_connecting: %v\n", valid)

	// Valid transition: tunnel_connected → transport_connecting
	valid = IsValidTransition(types.ConnectionStateTunnelConnected, types.ConnectionStateTransportConnecting)
	fmt.Printf("tunnel_connected → transport_connecting: %v\n", valid)

	// Invalid transition: disconnected → authenticated (must go through intermediate states)
	valid = IsValidTransition(types.ConnectionStateDisconnected, types.ConnectionStateAuthenticated)
	fmt.Printf("disconnected → authenticated: %v\n", valid)

	// Output:
	// disconnected → tunnel_connecting: true
	// tunnel_connected → transport_connecting: true
	// disconnected → authenticated: false
}

// Example_recovery demonstrates error recovery paths.
func Example_recovery() {
	machine := NewConnectionStateMachine()

	// Go through normal flow to tunnel_connecting
	_ = machine.Transition(types.ConnectionStateTunnelConnecting, "")

	// Simulate tunnel connection error
	_ = machine.Transition(types.ConnectionStateTunnelConnectionError, "tunnel connection failed")
	fmt.Printf("Error state: %s\n", machine.GetState())

	// Recover to tunnel_connected
	if err := machine.Transition(types.ConnectionStateTunnelConnected, ""); err != nil {
		fmt.Printf("Recovery error: %v\n", err)
		return
	}
	fmt.Printf("Recovered state: %s\n", machine.GetState())

	// Output:
	// Error state: tunnel_connection_error
	// Recovered state: tunnel_connected
}

