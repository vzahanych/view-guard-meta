package statemachine

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// deviceStateMachineImpl is a generic implementation of DeviceStateMachine.
// It provides thread-safe state management with configurable state transitions.
type deviceStateMachineImpl struct {
	deviceID    string
	deviceType  types.DeviceType
	mu          sync.RWMutex
	state       types.DeviceState
	stateInfo   types.DeviceStateInfo
	transitions map[types.DeviceState][]types.DeviceState // Valid transitions for this device type
	logger      *zap.Logger
}

// NewDeviceStateMachine creates a new device state machine.
// If logger is nil, a no-op logger will be used.
func NewDeviceStateMachine(
	deviceID string,
	deviceType types.DeviceType,
	transitions map[types.DeviceState][]types.DeviceState,
	logger *zap.Logger,
) types.DeviceStateMachine {
	if logger == nil {
		logger = zap.NewNop()
	}
	initialState := types.DeviceStateUndiscovered
	return &deviceStateMachineImpl{
		deviceID:    deviceID,
		deviceType:  deviceType,
		state:       initialState,
		transitions: transitions,
		logger:      logger,
		stateInfo: types.DeviceStateInfo{
			DeviceID:    deviceID,
			DeviceType:  deviceType,
			State:       initialState,
			LastUpdated: time.Now(),
			IsActive:    false,
			Metadata:    make(map[string]interface{}),
		},
	}
}

// GetDeviceID returns the device ID.
func (d *deviceStateMachineImpl) GetDeviceID() string {
	return d.deviceID
}

// GetDeviceType returns the device type.
func (d *deviceStateMachineImpl) GetDeviceType() types.DeviceType {
	return d.deviceType
}

// GetState returns the current device state.
func (d *deviceStateMachineImpl) GetState() types.DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// GetStateInfo returns detailed device state information.
// Returns a copy to prevent external modification.
func (d *deviceStateMachineImpl) GetStateInfo() types.DeviceStateInfo {
	d.mu.RLock()
	// Copy state info under lock
	info := d.stateInfo
	metadataCopy := make(map[string]interface{})
	for k, v := range d.stateInfo.Metadata {
		metadataCopy[k] = v
	}
	d.mu.RUnlock()

	// Set metadata copy outside lock
	info.Metadata = metadataCopy
	return info
}

// Transition transitions to a new device state.
// Returns error if transition is invalid.
func (d *deviceStateMachineImpl) Transition(newState types.DeviceState, errorMsg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isValidTransition(d.state, newState) {
		d.logger.Warn("Invalid state transition attempted",
			zap.String("device_id", d.deviceID),
			zap.String("device_type", string(d.deviceType)),
			zap.String("from_state", string(d.state)),
			zap.String("to_state", string(newState)),
		)
		return fmt.Errorf("%w: from %s to %s for device %s (type: %s)",
			types.ErrInvalidTransition, d.state, newState, d.deviceID, d.deviceType)
	}

	oldState := d.state
	d.state = newState
	d.stateInfo.State = newState
	d.stateInfo.LastUpdated = time.Now()
	d.stateInfo.Error = errorMsg

	// Update IsActive based on state
	d.stateInfo.IsActive = (newState == types.DeviceStateActive || newState == types.DeviceStateProcessing)

	d.logger.Info("Device state transitioned",
		zap.String("device_id", d.deviceID),
		zap.String("device_type", string(d.deviceType)),
		zap.String("from_state", string(oldState)),
		zap.String("to_state", string(newState)),
		zap.Bool("is_active", d.stateInfo.IsActive),
	)

	return nil
}

// CanTransition checks if a transition is valid.
func (d *deviceStateMachineImpl) CanTransition(newState types.DeviceState) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isValidTransition(d.state, newState)
}

// isValidTransition checks if a transition is valid (must be called with lock held).
func (d *deviceStateMachineImpl) isValidTransition(fromState types.DeviceState, toState types.DeviceState) bool {
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

// IsOperational returns true if device is in an operational state.
func (d *deviceStateMachineImpl) IsOperational() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state == types.DeviceStateActive || d.state == types.DeviceStateProcessing
}

// IsReadyForProcessing returns true if device is ready to process data.
func (d *deviceStateMachineImpl) IsReadyForProcessing() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state == types.DeviceStateActive || d.state == types.DeviceStateProcessing
}

// SetMetadata sets device-type-specific metadata.
func (d *deviceStateMachineImpl) SetMetadata(key string, value interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stateInfo.Metadata == nil {
		d.stateInfo.Metadata = make(map[string]interface{})
	}
	d.stateInfo.Metadata[key] = value
	d.logger.Debug("Device metadata updated",
		zap.String("device_id", d.deviceID),
		zap.String("key", key),
	)
}

// GetMetadata retrieves device-type-specific metadata.
func (d *deviceStateMachineImpl) GetMetadata(key string) (interface{}, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.stateInfo.Metadata == nil {
		return nil, false
	}
	value, exists := d.stateInfo.Metadata[key]
	return value, exists
}

