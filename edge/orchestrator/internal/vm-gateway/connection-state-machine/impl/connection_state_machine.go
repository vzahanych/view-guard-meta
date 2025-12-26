package impl

import (
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// ValidConnectionStateTransitions defines valid state transitions
var ValidConnectionStateTransitions = map[types.ConnectionState][]types.ConnectionState{
	types.ConnectionStateDisconnected: {
		types.ConnectionStateWGConnecting,
		types.ConnectionStateError,
	},
	types.ConnectionStateWGConnecting: {
		types.ConnectionStateWireGuardConnected,
		types.ConnectionStateWGConnectionError,
		types.ConnectionStateDisconnected,
		types.ConnectionStateError,
	},
	types.ConnectionStateWireGuardConnected: {
		types.ConnectionStateHTTPConnecting,
		types.ConnectionStateDisconnected,
		types.ConnectionStateError,
	},
	types.ConnectionStateHTTPConnecting: {
		types.ConnectionStateHTTPSConnected,
		types.ConnectionStateHTTPConnectionError,
		types.ConnectionStateDisconnected,
		types.ConnectionStateError,
	},
	types.ConnectionStateHTTPSConnected: {
		types.ConnectionStateAuthenticated,
		types.ConnectionStateDisconnected,
		types.ConnectionStateError,
	},
	types.ConnectionStateAuthenticated: {
		types.ConnectionStateCapabilitiesReceived,
		types.ConnectionStateDisconnected,
		types.ConnectionStateError,
	},
	types.ConnectionStateCapabilitiesReceived: {
		types.ConnectionStateDisconnected,
		types.ConnectionStateError,
	},
	types.ConnectionStateWGConnectionError: {
		types.ConnectionStateDisconnected,
		types.ConnectionStateWireGuardConnected, // Recovery
		types.ConnectionStateError,
	},
	types.ConnectionStateHTTPConnectionError: {
		types.ConnectionStateDisconnected,
		types.ConnectionStateHTTPSConnected, // Recovery
		types.ConnectionStateError,
	},
	types.ConnectionStateError: {
		types.ConnectionStateDisconnected,
	},
}

// IsValidTransition checks if a transition from fromState to toState is valid
func IsValidTransition(fromState types.ConnectionState, toState types.ConnectionState) bool {
	// Same state is always valid (no-op)
	if fromState == toState {
		return true
	}

	// Check if transition is in valid transitions map
	validTransitions, exists := ValidConnectionStateTransitions[fromState]
	if !exists {
		return false
	}

	for _, validState := range validTransitions {
		if validState == toState {
			return true
		}
	}

	return false
}

// ConnectionStateMachineImpl implements the ConnectionStateMachine interface
type ConnectionStateMachineImpl struct {
	mu        sync.RWMutex
	state     types.ConnectionState
	stateInfo types.ConnectionStateInfo
}

// NewConnectionStateMachine creates a new connection state machine
func NewConnectionStateMachine() *ConnectionStateMachineImpl {
	initialState := types.ConnectionStateDisconnected
	return &ConnectionStateMachineImpl{
		state: initialState,
		stateInfo: types.ConnectionStateInfo{
			State:         initialState,
			LastUpdated:   time.Now(),
			VMReachable:   false,
			NetworkHealth: "unhealthy", // Initial state is disconnected, so unhealthy
		},
	}
}

// GetState returns the current connection state
func (c *ConnectionStateMachineImpl) GetState() types.ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// GetStateInfo returns detailed connection state information
func (c *ConnectionStateMachineImpl) GetStateInfo() types.ConnectionStateInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stateInfo
}

// Transition transitions to a new connection state
// Returns error if transition is invalid
func (c *ConnectionStateMachineImpl) Transition(newState types.ConnectionState, errorMsg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !IsValidTransition(c.state, newState) {
		return fmt.Errorf("invalid state transition from %s to %s", c.state, newState)
	}

	c.state = newState
	c.stateInfo.State = newState
	c.stateInfo.LastUpdated = time.Now()
	c.stateInfo.Error = errorMsg

	// Update VM reachability based on state
	c.stateInfo.VMReachable = c.isVMReachable(newState)

	// Update network health based on state
	c.stateInfo.NetworkHealth = c.getNetworkHealth(newState)

	return nil
}

// CanTransition checks if a transition from current state to new state is valid
func (c *ConnectionStateMachineImpl) CanTransition(newState types.ConnectionState) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return IsValidTransition(c.state, newState)
}

// IsConnected returns true if Edge is connected to VM (wireguard + https + authenticated)
func (c *ConnectionStateMachineImpl) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == types.ConnectionStateAuthenticated ||
		c.state == types.ConnectionStateCapabilitiesReceived
}

// IsAuthenticated returns true if Edge is authenticated with VM
func (c *ConnectionStateMachineImpl) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == types.ConnectionStateAuthenticated ||
		c.state == types.ConnectionStateCapabilitiesReceived
}

// isVMReachable determines if VM is reachable based on connection state
func (c *ConnectionStateMachineImpl) isVMReachable(state types.ConnectionState) bool {
	return state == types.ConnectionStateWireGuardConnected ||
		state == types.ConnectionStateHTTPSConnected ||
		state == types.ConnectionStateAuthenticated ||
		state == types.ConnectionStateCapabilitiesReceived
}

// getNetworkHealth determines network health based on connection state
func (c *ConnectionStateMachineImpl) getNetworkHealth(state types.ConnectionState) string {
	switch state {
	case types.ConnectionStateDisconnected,
		types.ConnectionStateWGConnectionError,
		types.ConnectionStateHTTPConnectionError,
		types.ConnectionStateError:
		return "unhealthy"
	case types.ConnectionStateWGConnecting,
		types.ConnectionStateHTTPConnecting:
		return "degraded"
	case types.ConnectionStateWireGuardConnected,
		types.ConnectionStateHTTPSConnected,
		types.ConnectionStateAuthenticated,
		types.ConnectionStateCapabilitiesReceived:
		return "healthy"
	default:
		return "unknown"
	}
}

