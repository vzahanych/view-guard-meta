package types

import (
	"context"
	"time"
)

// DeviceState represents the generic state of a device
// Device states are device-type-agnostic and can be extended with device-type-specific states
type DeviceState string

const (
	// Generic device states (applicable to all device types)
	DeviceStateUndiscovered DeviceState = "undiscovered" // Device has not been discovered yet
	DeviceStateDiscovered   DeviceState = "discovered"   // Device has been discovered but not yet registered
	DeviceStateRegistered   DeviceState = "registered"   // Device has been registered with the system
	DeviceStateActive       DeviceState = "active"       // Device is active and operational
	DeviceStateIdle         DeviceState = "idle"         // Device is enabled but not actively processing
	DeviceStateProcessing   DeviceState = "processing"   // Device is actively processing data
	DeviceStateError        DeviceState = "error"        // Device-specific error occurred
	DeviceStateDisconnected DeviceState = "disconnected" // Device connection was lost
	DeviceStateDisabled     DeviceState = "disabled"     // Device is disabled
)

// DeviceStateInfo contains metadata about the device state
type DeviceStateInfo struct {
	DeviceID    string                 `json:"device_id"`
	DeviceType  DeviceType             `json:"device_type"`
	State       DeviceState             `json:"state"`
	LastUpdated time.Time              `json:"last_updated"`
	Error       string                 `json:"error,omitempty"`        // Error message if state is error
	Metadata    map[string]interface{} `json:"metadata,omitempty"`     // Device-type-specific metadata
	IsActive    bool                   `json:"is_active"`             // Whether device is actively processing
}

// DeviceStateTransitionRule defines a rule for state transitions
type DeviceStateTransitionRule struct {
	FromState DeviceState   `json:"from_state"`
	ToStates  []DeviceState `json:"to_states"`
	Condition func(*DeviceStateInfo) bool `json:"-"` // Optional condition function
}

// DeviceStateMachine defines the state machine for per-device states
// This is a generic interface that can be implemented for different device types
//
//go:generate go run go.uber.org/mock/mockgen -destination=../mocks/mock_device_state_machine.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types DeviceStateMachine
type DeviceStateMachine interface {
	// GetDeviceID returns the device ID this state machine is for
	GetDeviceID() string

	// GetDeviceType returns the device type
	GetDeviceType() DeviceType

	// GetState returns the current device state
	GetState() DeviceState

	// GetStateInfo returns detailed device state information
	GetStateInfo() DeviceStateInfo

	// Transition transitions to a new device state
	// Returns error if transition is invalid
	Transition(newState DeviceState, errorMsg string) error

	// CanTransition checks if a transition from current state to new state is valid
	CanTransition(newState DeviceState) bool

	// IsOperational returns true if device is in an operational state
	IsOperational() bool

	// IsReadyForProcessing returns true if device is ready to process data
	IsReadyForProcessing() bool

	// SetMetadata sets device-type-specific metadata
	SetMetadata(key string, value interface{})

	// GetMetadata retrieves device-type-specific metadata
	GetMetadata(key string) (interface{}, bool)
}

// DeviceStateMachineFactory creates device state machines for specific device types
// Different device types may have different state transition rules
type DeviceStateMachineFactory interface {
	// CreateStateMachine creates a state machine for a device
	CreateStateMachine(deviceID string, deviceType DeviceType) (DeviceStateMachine, error)

	// GetValidTransitions returns valid state transitions for a device type
	GetValidTransitions(deviceType DeviceType, fromState DeviceState) []DeviceState

	// RegisterDeviceTypeTransitions registers state transition rules for a device type
	RegisterDeviceTypeTransitions(deviceType DeviceType, rules []DeviceStateTransitionRule) error
}

// DeviceStateMachineRegistry is an interface for managing device state machines
type DeviceStateMachineRegistry interface {
	// GetOrCreateStateMachine gets an existing state machine or creates a new one for a device
	GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType DeviceType) (DeviceStateMachine, error)

	// GetStateMachine retrieves a state machine for a device
	GetStateMachine(deviceID string) (DeviceStateMachine, error)

	// CreateStateMachine creates a new state machine for a device
	CreateStateMachine(deviceID string, deviceType DeviceType) (DeviceStateMachine, error)

	// RemoveStateMachine removes a state machine for a device
	RemoveStateMachine(deviceID string) error

	// GetAllStateMachines returns all state machines
	GetAllStateMachines() []DeviceStateMachine

	// GetStateMachinesByType returns all state machines for a specific device type
	GetStateMachinesByType(deviceType DeviceType) []DeviceStateMachine
}

