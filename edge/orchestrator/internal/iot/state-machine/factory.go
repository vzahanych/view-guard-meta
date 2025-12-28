package statemachine

import (
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// deviceStateMachineFactoryImpl is the default implementation of DeviceStateMachineFactory.
// It manages device-type-specific state transition rules and creates state machines.
type deviceStateMachineFactoryImpl struct {
	mu                sync.RWMutex
	typeTransitions   map[types.DeviceType]map[types.DeviceState][]types.DeviceState
	defaultTransitions map[types.DeviceState][]types.DeviceState
	logger            *zap.Logger
}

// NewDeviceStateMachineFactory creates a new device state machine factory.
// If logger is nil, a no-op logger will be used.
func NewDeviceStateMachineFactory(logger *zap.Logger) types.DeviceStateMachineFactory {
	if logger == nil {
		logger = zap.NewNop()
	}
	factory := &deviceStateMachineFactoryImpl{
		typeTransitions:   make(map[types.DeviceType]map[types.DeviceState][]types.DeviceState),
		defaultTransitions: getDefaultDeviceStateTransitions(),
		logger:            logger,
	}
	return factory
}

// CreateStateMachine creates a state machine for a device.
func (f *deviceStateMachineFactoryImpl) CreateStateMachine(deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get transitions for this device type, or use default
	transitions, exists := f.typeTransitions[deviceType]
	if !exists {
		// Use default transitions
		transitions = f.defaultTransitions
		f.logger.Debug("Using default transitions for device type",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
		)
	} else {
		f.logger.Debug("Using device-type-specific transitions",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
		)
	}

	// Create a copy of transitions map for the state machine
	transitionsCopy := make(map[types.DeviceState][]types.DeviceState)
	for from, toStates := range transitions {
		toStatesCopy := make([]types.DeviceState, len(toStates))
		copy(toStatesCopy, toStates)
		transitionsCopy[from] = toStatesCopy
	}

	machine := NewDeviceStateMachine(deviceID, deviceType, transitionsCopy, f.logger)
	f.logger.Info("State machine created",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(deviceType)),
	)
	return machine, nil
}

// GetValidTransitions returns valid state transitions for a device type.
func (f *deviceStateMachineFactoryImpl) GetValidTransitions(deviceType types.DeviceType, fromState types.DeviceState) []types.DeviceState {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Get transitions for this device type, or use default
	transitions, exists := f.typeTransitions[deviceType]
	if !exists {
		transitions = f.defaultTransitions
	}

	validStates, exists := transitions[fromState]
	if !exists {
		return []types.DeviceState{}
	}

	// Return a copy
	result := make([]types.DeviceState, len(validStates))
	copy(result, validStates)
	return result
}

// RegisterDeviceTypeTransitions registers state transition rules for a device type.
func (f *deviceStateMachineFactoryImpl) RegisterDeviceTypeTransitions(deviceType types.DeviceType, rules []types.DeviceStateTransitionRule) error {
	if deviceType == types.DeviceTypeUnknown {
		f.logger.Error("Cannot register transitions for unknown device type")
		return fmt.Errorf("cannot register transitions for unknown device type")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Initialize transitions map for this device type if not exists
	if f.typeTransitions[deviceType] == nil {
		f.typeTransitions[deviceType] = make(map[types.DeviceState][]types.DeviceState)
	}

	// Register transitions from rules
	ruleCount := 0
	for _, rule := range rules {
		f.typeTransitions[deviceType][rule.FromState] = rule.ToStates
		ruleCount++
	}

	f.logger.Info("Device type transitions registered",
		zap.String("device_type", string(deviceType)),
		zap.Int("rule_count", ruleCount),
	)

	return nil
}

// getDefaultDeviceStateTransitions returns default state transitions for all device types.
// This is used as a fallback when no device-type-specific transitions are registered.
func getDefaultDeviceStateTransitions() map[types.DeviceState][]types.DeviceState {
	return map[types.DeviceState][]types.DeviceState{
		types.DeviceStateUndiscovered: {
			types.DeviceStateDiscovered,
			types.DeviceStateError,
		},
		types.DeviceStateDiscovered: {
			types.DeviceStateRegistered,
			types.DeviceStateDisconnected,
			types.DeviceStateError,
		},
		types.DeviceStateRegistered: {
			types.DeviceStateActive,
			types.DeviceStateIdle,
			types.DeviceStateDisconnected,
			types.DeviceStateError,
			types.DeviceStateDisabled,
		},
		types.DeviceStateActive: {
			types.DeviceStateProcessing,
			types.DeviceStateIdle,
			types.DeviceStateDisconnected,
			types.DeviceStateError,
			types.DeviceStateDisabled,
		},
		types.DeviceStateIdle: {
			types.DeviceStateActive,
			types.DeviceStateProcessing,
			types.DeviceStateDisconnected,
			types.DeviceStateError,
			types.DeviceStateDisabled,
		},
		types.DeviceStateProcessing: {
			types.DeviceStateActive,
			types.DeviceStateIdle,
			types.DeviceStateDisconnected,
			types.DeviceStateError,
			types.DeviceStateDisabled,
		},
		types.DeviceStateError: {
			types.DeviceStateDiscovered,  // Reset and start over
			types.DeviceStateRegistered,  // Retry from registered state
			types.DeviceStateDisconnected,
		},
		types.DeviceStateDisconnected: {
			types.DeviceStateDiscovered, // Reconnect and rediscover
			types.DeviceStateError,
		},
		types.DeviceStateDisabled: {
			types.DeviceStateRegistered, // Re-enable
			types.DeviceStateError,
		},
	}
}

