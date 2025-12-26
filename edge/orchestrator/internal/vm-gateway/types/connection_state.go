package types

import "time"

// ConnectionState represents the connection-level state of the Edge appliance.
// This is a global state that applies to the entire Edge appliance, not individual devices.
// Connection states track the Edge's connectivity and authentication status with the VM.
type ConnectionState string

const (
	// ConnectionStateDisconnected indicates the Edge is not connected to the VM
	ConnectionStateDisconnected ConnectionState = "disconnected"

	// ConnectionStateWGConnecting indicates WireGuard connection is being established
	ConnectionStateWGConnecting ConnectionState = "wg_connecting"

	// ConnectionStateWireGuardConnected indicates WireGuard tunnel is established
	ConnectionStateWireGuardConnected ConnectionState = "wireguard_connected"

	// ConnectionStateHTTPConnecting indicates HTTPS connection is being established
	ConnectionStateHTTPConnecting ConnectionState = "http_connecting"

	// ConnectionStateHTTPSConnected indicates HTTPS connection is established
	ConnectionStateHTTPSConnected ConnectionState = "https_connected"

	// ConnectionStateAuthenticated indicates Edge is authenticated with the VM
	ConnectionStateAuthenticated ConnectionState = "authenticated"

	// ConnectionStateCapabilitiesReceived indicates Edge has received capabilities from VM
	ConnectionStateCapabilitiesReceived ConnectionState = "capabilities_received"

	// ConnectionStateError indicates a connection-level error occurred
	ConnectionStateError ConnectionState = "error"

	// ConnectionStateWGConnectionError indicates WireGuard connection error
	ConnectionStateWGConnectionError ConnectionState = "wg_connection_error"

	// ConnectionStateHTTPConnectionError indicates HTTPS connection error
	ConnectionStateHTTPConnectionError ConnectionState = "http_connection_error"
)

// ConnectionStateInfo contains metadata about the connection state
type ConnectionStateInfo struct {
	State         ConnectionState `json:"state"`
	LastUpdated   time.Time       `json:"last_updated"`
	Error         string          `json:"error,omitempty"`          // Error message if state is error
	NetworkHealth string          `json:"network_health,omitempty"` // "healthy", "degraded", "unhealthy"
	VMReachable   bool            `json:"vm_reachable"`             // Whether VM is reachable
}

// ConnectionStateMachine defines the state machine for connection-level states
// Valid state transitions:
//
//	disconnected -> wg_connecting -> wireguard_connected -> http_connecting -> https_connected -> authenticated -> capabilities_received
//	                                                                              |
//	                                                                              v
//	                                                                         error (on failure)
//
//	Any state -> disconnected (on connection loss)
//	Any state -> error (on critical error)
//
// Error states can transition to:
//   - wg_connection_error -> disconnected (retry) or wireguard_connected (recovered)
//   - http_connection_error -> disconnected (retry) or https_connected (recovered)
//   - error -> disconnected (retry)
type ConnectionStateMachine interface {
	// GetState returns the current connection state
	GetState() ConnectionState

	// GetStateInfo returns detailed connection state information
	GetStateInfo() ConnectionStateInfo

	// Transition transitions to a new connection state
	// Returns error if transition is invalid
	Transition(newState ConnectionState, errorMsg string) error

	// CanTransition checks if a transition from current state to new state is valid
	CanTransition(newState ConnectionState) bool

	// IsConnected returns true if Edge is connected to VM (wireguard + https + authenticated)
	IsConnected() bool

	// IsAuthenticated returns true if Edge is authenticated with VM
	IsAuthenticated() bool
}

