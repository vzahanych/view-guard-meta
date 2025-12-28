package statemachine

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// deviceStateMachineRegistryImpl is the default implementation of DeviceStateMachineRegistry.
// It manages a collection of device state machines and provides thread-safe access.
type deviceStateMachineRegistryImpl struct {
	machines map[string]types.DeviceStateMachine
	factory  types.DeviceStateMachineFactory
	mu       sync.RWMutex
	logger   *zap.Logger
}

// NewDeviceStateMachineRegistry creates a new device state machine registry.
// If logger is nil, a no-op logger will be used.
func NewDeviceStateMachineRegistry(
	factory types.DeviceStateMachineFactory,
	logger *zap.Logger,
) types.DeviceStateMachineRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &deviceStateMachineRegistryImpl{
		machines: make(map[string]types.DeviceStateMachine),
		factory:  factory,
		logger:   logger,
	}
}

// GetOrCreateStateMachine gets an existing state machine or creates a new one for a device.
// This is a convenience method that combines GetStateMachine and CreateStateMachine.
func (r *deviceStateMachineRegistryImpl) GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
	// Try to get existing state machine first
	r.mu.RLock()
	machine, exists := r.machines[deviceID]
	r.mu.RUnlock()

	if exists {
		r.logger.Debug("State machine found",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
		)
		return machine, nil
	}

	// Create new state machine
	r.mu.Lock()
	// Double-check after acquiring write lock
	if machine, exists := r.machines[deviceID]; exists {
		r.mu.Unlock()
		r.logger.Debug("State machine found (double-check)",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
		)
		return machine, nil
	}

	// Create new state machine
	machine, err := r.factory.CreateStateMachine(deviceID, deviceType)
	if err != nil {
		r.mu.Unlock()
		r.logger.Error("Failed to create state machine",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create state machine for device %s: %w", deviceID, err)
	}

	r.machines[deviceID] = machine
	r.mu.Unlock()

	r.logger.Info("State machine created and registered",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(deviceType)),
	)

	return machine, nil
}

// GetStateMachine retrieves a state machine by device ID.
func (r *deviceStateMachineRegistryImpl) GetStateMachine(deviceID string) (types.DeviceStateMachine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	machine, exists := r.machines[deviceID]
	if !exists {
		r.logger.Warn("State machine not found",
			zap.String("device_id", deviceID),
		)
		return nil, fmt.Errorf("%w: device %s", types.ErrStateMachineNotFound, deviceID)
	}

	return machine, nil
}

// CreateStateMachine creates a new state machine for a device.
func (r *deviceStateMachineRegistryImpl) CreateStateMachine(deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.machines[deviceID]; exists {
		r.logger.Warn("State machine already exists",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
		)
		return nil, fmt.Errorf("state machine for device %s already exists", deviceID)
	}

	machine, err := r.factory.CreateStateMachine(deviceID, deviceType)
	if err != nil {
		r.logger.Error("Failed to create state machine",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create state machine for device %s: %w", deviceID, err)
	}

	r.machines[deviceID] = machine
	r.logger.Info("State machine created and registered",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(deviceType)),
	)

	return machine, nil
}

// RemoveStateMachine removes a state machine.
func (r *deviceStateMachineRegistryImpl) RemoveStateMachine(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.machines[deviceID]; !exists {
		r.logger.Warn("State machine not found for removal",
			zap.String("device_id", deviceID),
		)
		return fmt.Errorf("%w: device %s", types.ErrStateMachineNotFound, deviceID)
	}

	delete(r.machines, deviceID)
	r.logger.Info("State machine removed",
		zap.String("device_id", deviceID),
	)

	return nil
}

// GetAllStateMachines returns all registered state machines.
func (r *deviceStateMachineRegistryImpl) GetAllStateMachines() []types.DeviceStateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]types.DeviceStateMachine, 0, len(r.machines))
	for _, machine := range r.machines {
		result = append(result, machine)
	}

	r.logger.Debug("Retrieved all state machines",
		zap.Int("count", len(result)),
	)

	return result
}

// GetStateMachinesByType returns all state machines for a specific device type.
func (r *deviceStateMachineRegistryImpl) GetStateMachinesByType(deviceType types.DeviceType) []types.DeviceStateMachine {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []types.DeviceStateMachine
	for _, machine := range r.machines {
		if machine.GetDeviceType() == deviceType {
			result = append(result, machine)
		}
	}

	r.logger.Debug("Retrieved state machines by type",
		zap.String("device_type", string(deviceType)),
		zap.Int("count", len(result)),
	)

	return result
}

