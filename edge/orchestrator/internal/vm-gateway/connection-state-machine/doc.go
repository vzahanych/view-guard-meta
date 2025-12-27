/*
Package impl implements the connection state machine for the VM Gateway.

The connection state machine tracks the lifecycle of Edge↔VM connectivity,
from initial connection through authentication and capability synchronization.

# State Machine Overview

The state machine manages the following states:

  - disconnected: Edge is not connected to VM
  - tunnel_connecting: Tunnel connection is being established
  - tunnel_connected: Tunnel is established
  - transport_connecting: Transport connection is being established
  - transport_connected: Transport connection is established
  - authenticated: Edge is authenticated with VM
  - capabilities_received: Edge has received capabilities from VM
  - tunnel_connection_error: Tunnel connection error occurred
  - transport_connection_error: Transport connection error occurred
  - error: General connection error

# Valid State Transitions

The state machine enforces valid transitions:

  disconnected → tunnel_connecting → tunnel_connected → transport_connecting →
  transport_connected → authenticated → capabilities_received

Error recovery paths:
  - tunnel_connection_error → tunnel_connected (recovery)
  - tunnel_connection_error → disconnected (retry)
  - transport_connection_error → transport_connected (recovery)
  - transport_connection_error → disconnected (retry)
  - error → disconnected (retry)

Any state can transition to:
  - disconnected (on connection loss)
  - error (on critical error)

# VM Reachability

The state machine tracks VM reachability:
  - true: When tunnel_connected or any subsequent state
  - false: When disconnected or tunnel_connecting

# Network Health

The state machine tracks network health:
  - "healthy": When tunnel_connected or any subsequent state
  - "degraded": When tunnel_connecting
  - "unhealthy": When disconnected or error states

# Thread Safety

The state machine is thread-safe and can be accessed concurrently.
All state transitions are protected by mutex locks.

# Usage

Create a new state machine:

  machine := NewConnectionStateMachine()

Check current state:

  state := machine.GetState()
  stateInfo := machine.GetStateInfo()

Transition to a new state:

  err := machine.Transition(newState, "optional error message")

Check if a transition is valid:

  canTransition := machine.CanTransition(newState)
*/
package impl

