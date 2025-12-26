package iot

import (
	"context"
	"fmt"
	"sync"
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
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_device_state_machine.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DeviceStateMachine
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

// deviceStateMachineImpl is a generic implementation of DeviceStateMachine
type deviceStateMachineImpl struct {
	deviceID   string
	deviceType DeviceType
	mu         sync.RWMutex
	state      DeviceState
	stateInfo  DeviceStateInfo
	transitions map[DeviceState][]DeviceState // Valid transitions for this device type
}

// NewDeviceStateMachine creates a new device state machine
func NewDeviceStateMachine(deviceID string, deviceType DeviceType, transitions map[DeviceState][]DeviceState) *deviceStateMachineImpl {
	initialState := DeviceStateUndiscovered
	return &deviceStateMachineImpl{
		deviceID:    deviceID,
		deviceType:  deviceType,
		state:       initialState,
		transitions: transitions,
		stateInfo: DeviceStateInfo{
			DeviceID:    deviceID,
			DeviceType:  deviceType,
			State:       initialState,
			LastUpdated: time.Now(),
			IsActive:    false,
			Metadata:    make(map[string]interface{}),
		},
	}
}

// GetDeviceID returns the device ID
func (d *deviceStateMachineImpl) GetDeviceID() string {
	return d.deviceID
}

// GetDeviceType returns the device type
func (d *deviceStateMachineImpl) GetDeviceType() DeviceType {
	return d.deviceType
}

// GetState returns the current device state
func (d *deviceStateMachineImpl) GetState() DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// GetStateInfo returns detailed device state information
func (d *deviceStateMachineImpl) GetStateInfo() DeviceStateInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// Return a copy to prevent external modification
	info := d.stateInfo
	metadataCopy := make(map[string]interface{})
	for k, v := range d.stateInfo.Metadata {
		metadataCopy[k] = v
	}
	info.Metadata = metadataCopy
	return info
}

// Transition transitions to a new device state
func (d *deviceStateMachineImpl) Transition(newState DeviceState, errorMsg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isValidTransition(d.state, newState) {
		return fmt.Errorf("invalid device state transition from %s to %s for device %s (type: %s)",
			d.state, newState, d.deviceID, d.deviceType)
	}

	d.state = newState
	d.stateInfo.State = newState
	d.stateInfo.LastUpdated = time.Now()
	d.stateInfo.Error = errorMsg

	// Update IsActive based on state
	d.stateInfo.IsActive = (newState == DeviceStateActive || newState == DeviceStateProcessing)

	return nil
}

// CanTransition checks if a transition is valid
func (d *deviceStateMachineImpl) CanTransition(newState DeviceState) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isValidTransition(d.state, newState)
}

// isValidTransition checks if a transition is valid (must be called with lock held)
func (d *deviceStateMachineImpl) isValidTransition(fromState DeviceState, toState DeviceState) bool {
	// Same state is always valid (no-op)
	if fromState == toState {
		return true
	}

	// Check if transition is in valid transitions map
	validTransitions, exists := d.transitions[fromState]
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

// IsOperational returns true if device is in an operational state
func (d *deviceStateMachineImpl) IsOperational() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state == DeviceStateActive || d.state == DeviceStateProcessing
}

// IsReadyForProcessing returns true if device is ready to process data
func (d *deviceStateMachineImpl) IsReadyForProcessing() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state == DeviceStateActive || d.state == DeviceStateProcessing
}

// SetMetadata sets device-type-specific metadata
func (d *deviceStateMachineImpl) SetMetadata(key string, value interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stateInfo.Metadata == nil {
		d.stateInfo.Metadata = make(map[string]interface{})
	}
	d.stateInfo.Metadata[key] = value
}

// GetMetadata retrieves device-type-specific metadata
func (d *deviceStateMachineImpl) GetMetadata(key string) (interface{}, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.stateInfo.Metadata == nil {
		return nil, false
	}
	value, exists := d.stateInfo.Metadata[key]
	return value, exists
}

// deviceStateMachineFactoryImpl is the default implementation of DeviceStateMachineFactory
type deviceStateMachineFactoryImpl struct {
	mu                sync.RWMutex
	typeTransitions   map[DeviceType]map[DeviceState][]DeviceState
	defaultTransitions map[DeviceState][]DeviceState
}

// NewDeviceStateMachineFactory creates a new device state machine factory
func NewDeviceStateMachineFactory() DeviceStateMachineFactory {
	factory := &deviceStateMachineFactoryImpl{
		typeTransitions:   make(map[DeviceType]map[DeviceState][]DeviceState),
		defaultTransitions: getDefaultDeviceStateTransitions(),
	}
	return factory
}

// CreateStateMachine creates a state machine for a device
func (f *deviceStateMachineFactoryImpl) CreateStateMachine(deviceID string, deviceType DeviceType) (DeviceStateMachine, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get transitions for this device type, or use default
	transitions, exists := f.typeTransitions[deviceType]
	if !exists {
		// Use default transitions
		transitions = f.defaultTransitions
	}

	// Create a copy of transitions map for the state machine
	transitionsCopy := make(map[DeviceState][]DeviceState)
	for from, toStates := range transitions {
		toStatesCopy := make([]DeviceState, len(toStates))
		copy(toStatesCopy, toStates)
		transitionsCopy[from] = toStatesCopy
	}

	return NewDeviceStateMachine(deviceID, deviceType, transitionsCopy), nil
}

// GetValidTransitions returns valid state transitions for a device type
func (f *deviceStateMachineFactoryImpl) GetValidTransitions(deviceType DeviceType, fromState DeviceState) []DeviceState {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get transitions for this device type, or use default
	transitions, exists := f.typeTransitions[deviceType]
	if !exists {
		transitions = f.defaultTransitions
	}

	validStates, exists := transitions[fromState]
	if !exists {
		return []DeviceState{}
	}

	// Return a copy
	result := make([]DeviceState, len(validStates))
	copy(result, validStates)
	return result
}

// RegisterDeviceTypeTransitions registers state transition rules for a device type
func (f *deviceStateMachineFactoryImpl) RegisterDeviceTypeTransitions(deviceType DeviceType, rules []DeviceStateTransitionRule) error {
	if deviceType == DeviceTypeUnknown {
		return fmt.Errorf("cannot register transitions for unknown device type")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Initialize transitions map for this device type if not exists
	if f.typeTransitions[deviceType] == nil {
		f.typeTransitions[deviceType] = make(map[DeviceState][]DeviceState)
	}

	// Register transitions from rules
	for _, rule := range rules {
		f.typeTransitions[deviceType][rule.FromState] = rule.ToStates
	}

	return nil
}

// getDefaultDeviceStateTransitions returns default state transitions for all device types
func getDefaultDeviceStateTransitions() map[DeviceState][]DeviceState {
	return map[DeviceState][]DeviceState{
		DeviceStateUndiscovered: {
			DeviceStateDiscovered,
			DeviceStateError,
		},
		DeviceStateDiscovered: {
			DeviceStateRegistered,
			DeviceStateDisconnected,
			DeviceStateError,
		},
		DeviceStateRegistered: {
			DeviceStateActive,
			DeviceStateIdle,
			DeviceStateDisconnected,
			DeviceStateError,
			DeviceStateDisabled,
		},
		DeviceStateActive: {
			DeviceStateProcessing,
			DeviceStateIdle,
			DeviceStateDisconnected,
			DeviceStateError,
			DeviceStateDisabled,
		},
		DeviceStateIdle: {
			DeviceStateActive,
			DeviceStateProcessing,
			DeviceStateDisconnected,
			DeviceStateError,
			DeviceStateDisabled,
		},
		DeviceStateProcessing: {
			DeviceStateActive,
			DeviceStateIdle,
			DeviceStateDisconnected,
			DeviceStateError,
			DeviceStateDisabled,
		},
		DeviceStateError: {
			DeviceStateDiscovered,  // Reset and start over
			DeviceStateRegistered,  // Retry from registered state
			DeviceStateDisconnected,
		},
		DeviceStateDisconnected: {
			DeviceStateDiscovered, // Reconnect and rediscover
			DeviceStateError,
		},
		DeviceStateDisabled: {
			DeviceStateRegistered, // Re-enable
			DeviceStateError,
		},
	}
}

// DeviceStateMachineRegistry manages device state machines
type DeviceStateMachineRegistry interface {
	// GetOrCreateStateMachine gets an existing state machine or creates a new one
	GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType DeviceType) (DeviceStateMachine, error)

	// GetStateMachine retrieves a state machine by device ID
	GetStateMachine(deviceID string) (DeviceStateMachine, error)

	// GetAllStateMachines returns all registered state machines
	GetAllStateMachines() map[string]DeviceStateMachine

	// RemoveStateMachine removes a state machine
	RemoveStateMachine(deviceID string) error

	// GetStateMachinesByType returns all state machines for a specific device type
	GetStateMachinesByType(deviceType DeviceType) []DeviceStateMachine
}

// deviceStateMachineRegistryImpl is the default implementation of DeviceStateMachineRegistry
type deviceStateMachineRegistryImpl struct {
	factory   DeviceStateMachineFactory
	machines  map[string]DeviceStateMachine
	mu        sync.RWMutex
}

// NewDeviceStateMachineRegistry creates a new device state machine registry
func NewDeviceStateMachineRegistry(factory DeviceStateMachineFactory) DeviceStateMachineRegistry {
	return &deviceStateMachineRegistryImpl{
		factory:  factory,
		machines: make(map[string]DeviceStateMachine),
	}
}

// GetOrCreateStateMachine gets an existing state machine or creates a new one
func (r *deviceStateMachineRegistryImpl) GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType DeviceType) (DeviceStateMachine, error) {
	r.mu.RLock()
	machine, exists := r.machines[deviceID]
	r.mu.RUnlock()

	if exists {
		return machine, nil
	}

	// Create new state machine
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if machine, exists := r.machines[deviceID]; exists {
		return machine, nil
	}

	machine, err := r.factory.CreateStateMachine(deviceID, deviceType)
	if err != nil {
		return nil, err
	}

	r.machines[deviceID] = machine
	return machine, nil
}

// GetStateMachine retrieves a state machine by device ID
func (r *deviceStateMachineRegistryImpl) GetStateMachine(deviceID string) (DeviceStateMachine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	machine, exists := r.machines[deviceID]
	if !exists {
		return nil, fmt.Errorf("state machine for device %s not found", deviceID)
	}

	return machine, nil
}

// GetAllStateMachines returns all registered state machines
func (r *deviceStateMachineRegistryImpl) GetAllStateMachines() map[string]DeviceStateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]DeviceStateMachine)
	for id, machine := range r.machines {
		result[id] = machine
	}
	return result
}

// RemoveStateMachine removes a state machine
func (r *deviceStateMachineRegistryImpl) RemoveStateMachine(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.machines[deviceID]; !exists {
		return fmt.Errorf("state machine for device %s not found", deviceID)
	}

	delete(r.machines, deviceID)
	return nil
}

// GetStateMachinesByType returns all state machines for a specific device type
func (r *deviceStateMachineRegistryImpl) GetStateMachinesByType(deviceType DeviceType) []DeviceStateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []DeviceStateMachine
	for _, machine := range r.machines {
		if machine.GetDeviceType() == deviceType {
			result = append(result, machine)
		}
	}
	return result
}

