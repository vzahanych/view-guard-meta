package impl

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name      string
		fromState types.ConnectionState
		toState   types.ConnectionState
		wantValid bool
	}{
		// Same state is always valid
		{
			name:      "Same state is valid",
			fromState: types.ConnectionStateDisconnected,
			toState:   types.ConnectionStateDisconnected,
			wantValid: true,
		},
		// Valid transitions from disconnected
		{
			name:      "Disconnected to tunnel_connecting is valid",
			fromState: types.ConnectionStateDisconnected,
			toState:   types.ConnectionStateTunnelConnecting,
			wantValid: true,
		},
		{
			name:      "Disconnected to error is valid",
			fromState: types.ConnectionStateDisconnected,
			toState:   types.ConnectionStateError,
			wantValid: true,
		},
		// Valid transitions from tunnel_connecting
		{
			name:      "Tunnel_connecting to tunnel_connected is valid",
			fromState: types.ConnectionStateTunnelConnecting,
			toState:   types.ConnectionStateTunnelConnected,
			wantValid: true,
		},
		{
			name:      "Tunnel_connecting to tunnel_connection_error is valid",
			fromState: types.ConnectionStateTunnelConnecting,
			toState:   types.ConnectionStateTunnelConnectionError,
			wantValid: true,
		},
		{
			name:      "Tunnel_connecting to disconnected is valid",
			fromState: types.ConnectionStateTunnelConnecting,
			toState:   types.ConnectionStateDisconnected,
			wantValid: true,
		},
		// Valid transitions from tunnel_connected
		{
			name:      "Tunnel_connected to transport_connecting is valid",
			fromState: types.ConnectionStateTunnelConnected,
			toState:   types.ConnectionStateTransportConnecting,
			wantValid: true,
		},
		// Valid transitions from transport_connecting
		{
			name:      "Transport_connecting to transport_connected is valid",
			fromState: types.ConnectionStateTransportConnecting,
			toState:   types.ConnectionStateTransportConnected,
			wantValid: true,
		},
		// Valid transitions from transport_connected
		{
			name:      "Transport_connected to authenticated is valid",
			fromState: types.ConnectionStateTransportConnected,
			toState:   types.ConnectionStateAuthenticated,
			wantValid: true,
		},
		// Valid transitions from authenticated
		{
			name:      "Authenticated to capabilities_received is valid",
			fromState: types.ConnectionStateAuthenticated,
			toState:   types.ConnectionStateCapabilitiesReceived,
			wantValid: true,
		},
		// Recovery transitions
		{
			name:      "Tunnel_connection_error to tunnel_connected (recovery) is valid",
			fromState: types.ConnectionStateTunnelConnectionError,
			toState:   types.ConnectionStateTunnelConnected,
			wantValid: true,
		},
		{
			name:      "Transport_connection_error to transport_connected (recovery) is valid",
			fromState: types.ConnectionStateTransportConnectionError,
			toState:   types.ConnectionStateTransportConnected,
			wantValid: true,
		},
		// Invalid transitions
		{
			name:      "Disconnected to authenticated is invalid",
			fromState: types.ConnectionStateDisconnected,
			toState:   types.ConnectionStateAuthenticated,
			wantValid: false,
		},
		{
			name:      "Tunnel_connected to authenticated is invalid",
			fromState: types.ConnectionStateTunnelConnected,
			toState:   types.ConnectionStateAuthenticated,
			wantValid: false,
		},
		{
			name:      "Transport_connected to tunnel_connecting is invalid",
			fromState: types.ConnectionStateTransportConnected,
			toState:   types.ConnectionStateTunnelConnecting,
			wantValid: false,
		},
		{
			name:      "Capabilities_received to authenticated is invalid",
			fromState: types.ConnectionStateCapabilitiesReceived,
			toState:   types.ConnectionStateAuthenticated,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := IsValidTransition(tt.fromState, tt.toState)
			assert.Equal(t, tt.wantValid, valid, "Transition from %s to %s should be %v", tt.fromState, tt.toState, tt.wantValid)
		})
	}
}

func TestConnectionStateMachine_Transition(t *testing.T) {
	tests := []struct {
		name        string
		initialState types.ConnectionState
		newState     types.ConnectionState
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid transition succeeds",
			initialState: types.ConnectionStateDisconnected,
			newState:    types.ConnectionStateTunnelConnecting,
			expectError: false,
		},
		{
			name:        "Invalid transition fails",
			initialState: types.ConnectionStateDisconnected,
			newState:    types.ConnectionStateAuthenticated,
			expectError: true,
			errorMsg:    "invalid state transition",
		},
		{
			name:        "Same state transition succeeds",
			initialState: types.ConnectionStateDisconnected,
			newState:    types.ConnectionStateDisconnected,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := NewConnectionStateMachine()
			
			// Set initial state if not disconnected
			if tt.initialState != types.ConnectionStateDisconnected {
				err := machine.Transition(tt.initialState, "")
				require.NoError(t, err)
			}

			err := machine.Transition(tt.newState, "test error message")

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.newState, machine.GetState())
				stateInfo := machine.GetStateInfo()
				assert.Equal(t, tt.newState, stateInfo.State)
				assert.NotZero(t, stateInfo.LastUpdated)
			}
		})
	}
}

func TestConnectionStateMachine_CanTransition(t *testing.T) {
	machine := NewConnectionStateMachine()

	// Valid transition
	assert.True(t, machine.CanTransition(types.ConnectionStateTunnelConnecting))

	// Invalid transition
	assert.False(t, machine.CanTransition(types.ConnectionStateAuthenticated))

	// Same state is valid
	assert.True(t, machine.CanTransition(types.ConnectionStateDisconnected))
}

func TestConnectionStateMachine_GetState(t *testing.T) {
	machine := NewConnectionStateMachine()
	
	// Initial state should be disconnected
	assert.Equal(t, types.ConnectionStateDisconnected, machine.GetState())

	// Transition and verify state
	err := machine.Transition(types.ConnectionStateTunnelConnecting, "")
	require.NoError(t, err)
	assert.Equal(t, types.ConnectionStateTunnelConnecting, machine.GetState())
}

func TestConnectionStateMachine_GetStateInfo(t *testing.T) {
	machine := NewConnectionStateMachine()
	
	stateInfo := machine.GetStateInfo()
	assert.Equal(t, types.ConnectionStateDisconnected, stateInfo.State)
	assert.NotZero(t, stateInfo.LastUpdated)
	assert.False(t, stateInfo.VMReachable)
	assert.Equal(t, "unhealthy", stateInfo.NetworkHealth)

	// Transition and verify state info updates
	err := machine.Transition(types.ConnectionStateTunnelConnecting, "test error")
	require.NoError(t, err)
	
	stateInfo = machine.GetStateInfo()
	assert.Equal(t, types.ConnectionStateTunnelConnecting, stateInfo.State)
	assert.Equal(t, "test error", stateInfo.Error)
	assert.True(t, stateInfo.LastUpdated.After(time.Now().Add(-time.Second)))
}

func TestConnectionStateMachine_StateMachineCompleteness(t *testing.T) {
	// Test that all states have at least one valid transition (no dangling states)
	allStates := []types.ConnectionState{
		types.ConnectionStateDisconnected,
		types.ConnectionStateTunnelConnecting,
		types.ConnectionStateTunnelConnected,
		types.ConnectionStateTransportConnecting,
		types.ConnectionStateTransportConnected,
		types.ConnectionStateAuthenticated,
		types.ConnectionStateCapabilitiesReceived,
		types.ConnectionStateTunnelConnectionError,
		types.ConnectionStateTransportConnectionError,
		types.ConnectionStateError,
	}

	for _, state := range allStates {
		t.Run(string(state)+"_has_valid_transitions", func(t *testing.T) {
			validTransitions, exists := ValidConnectionStateTransitions[state]
			require.True(t, exists, "State %s should have valid transitions defined", state)
			assert.NotEmpty(t, validTransitions, "State %s should have at least one valid transition", state)
		})
	}
}

func TestConnectionStateMachine_ConcurrentAccess(t *testing.T) {
	machine := NewConnectionStateMachine()
	
	// Test concurrent reads and writes
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = machine.GetState()
				_ = machine.GetStateInfo()
				_ = machine.CanTransition(types.ConnectionStateTunnelConnecting)
			}
		}()
	}

	// Concurrent writes (sequential transitions)
	wg.Add(1)
	go func() {
		defer wg.Done()
		states := []types.ConnectionState{
			types.ConnectionStateTunnelConnecting,
			types.ConnectionStateTunnelConnected,
			types.ConnectionStateTransportConnecting,
			types.ConnectionStateTransportConnected,
		}
		for j := 0; j < numOperations; j++ {
			for _, state := range states {
				_ = machine.Transition(state, "")
			}
			// Reset to disconnected
			_ = machine.Transition(types.ConnectionStateDisconnected, "")
		}
	}()

	wg.Wait()
	// If we get here without race detector errors, the test passes
}

func TestConnectionStateMachine_RecoveryPaths(t *testing.T) {
	tests := []struct {
		name        string
		errorState  types.ConnectionState
		recoveryState types.ConnectionState
	}{
		{
			name:        "Tunnel error can recover to tunnel_connected",
			errorState:  types.ConnectionStateTunnelConnectionError,
			recoveryState: types.ConnectionStateTunnelConnected,
		},
		{
			name:        "Transport error can recover to transport_connected",
			errorState:  types.ConnectionStateTransportConnectionError,
			recoveryState: types.ConnectionStateTransportConnected,
		},
		{
			name:        "Error can recover to disconnected",
			errorState:  types.ConnectionStateError,
			recoveryState: types.ConnectionStateDisconnected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := NewConnectionStateMachine()
			
			// First transition to error state
			// We need to go through valid transitions first
			if tt.errorState == types.ConnectionStateTunnelConnectionError {
				_ = machine.Transition(types.ConnectionStateTunnelConnecting, "")
			} else if tt.errorState == types.ConnectionStateTransportConnectionError {
				_ = machine.Transition(types.ConnectionStateTunnelConnecting, "")
				_ = machine.Transition(types.ConnectionStateTunnelConnected, "")
				_ = machine.Transition(types.ConnectionStateTransportConnecting, "")
			}
			
			err := machine.Transition(tt.errorState, "test error")
			require.NoError(t, err)

			// Test recovery transition
			err = machine.Transition(tt.recoveryState, "")
			require.NoError(t, err, "Recovery from %s to %s should succeed", tt.errorState, tt.recoveryState)
			assert.Equal(t, tt.recoveryState, machine.GetState())
		})
	}
}

func TestConnectionStateMachine_VMReachability(t *testing.T) {
	machine := NewConnectionStateMachine()

	// Disconnected - VM not reachable
	assert.False(t, machine.GetStateInfo().VMReachable)

	// Tunnel connecting - VM not reachable yet
	err := machine.Transition(types.ConnectionStateTunnelConnecting, "")
	require.NoError(t, err)
	assert.False(t, machine.GetStateInfo().VMReachable)

	// Tunnel connected - VM reachable
	err = machine.Transition(types.ConnectionStateTunnelConnected, "")
	require.NoError(t, err)
	assert.True(t, machine.GetStateInfo().VMReachable)

	// Transport connected - VM reachable
	err = machine.Transition(types.ConnectionStateTransportConnecting, "")
	require.NoError(t, err)
	err = machine.Transition(types.ConnectionStateTransportConnected, "")
	require.NoError(t, err)
	assert.True(t, machine.GetStateInfo().VMReachable)

	// Authenticated - VM reachable
	err = machine.Transition(types.ConnectionStateAuthenticated, "")
	require.NoError(t, err)
	assert.True(t, machine.GetStateInfo().VMReachable)
}

func TestConnectionStateMachine_NetworkHealth(t *testing.T) {
	machine := NewConnectionStateMachine()

	// Disconnected - unhealthy
	assert.Equal(t, "unhealthy", machine.GetStateInfo().NetworkHealth)

	// Tunnel connecting - degraded
	err := machine.Transition(types.ConnectionStateTunnelConnecting, "")
	require.NoError(t, err)
	assert.Equal(t, "degraded", machine.GetStateInfo().NetworkHealth)

	// Tunnel connected - healthy
	err = machine.Transition(types.ConnectionStateTunnelConnected, "")
	require.NoError(t, err)
	assert.Equal(t, "healthy", machine.GetStateInfo().NetworkHealth)

	// Transport connected - healthy
	err = machine.Transition(types.ConnectionStateTransportConnecting, "")
	require.NoError(t, err)
	err = machine.Transition(types.ConnectionStateTransportConnected, "")
	require.NoError(t, err)
	assert.Equal(t, "healthy", machine.GetStateInfo().NetworkHealth)
}

