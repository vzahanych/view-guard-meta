package iot

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
	statemachine "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine/transitions"
)

// DeviceStateService provides access to device state machines.
// This is the top interface for managing device states in the IoT layer.
// Services like state-mng should use this interface to query and manage device states,
// rather than implementing their own state machines.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_device_state_service.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DeviceStateService
type DeviceStateService interface {
	// GetOrCreateStateMachine gets an existing state machine or creates a new one for a device
	GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error)

	// GetStateMachine retrieves a state machine by device ID
	GetStateMachine(deviceID string) (types.DeviceStateMachine, error)

	// GetAllStateMachines returns all registered state machines
	GetAllStateMachines() map[string]types.DeviceStateMachine

	// RemoveStateMachine removes a state machine
	RemoveStateMachine(deviceID string) error

	// GetStateMachinesByType returns all state machines for a specific device type
	GetStateMachinesByType(deviceType types.DeviceType) []types.DeviceStateMachine
}

// NewDeviceStateService creates a new device state service.
// This wraps the DeviceStateMachineRegistry and provides the top-level interface
// for accessing device state machines.
func NewDeviceStateService(registry types.DeviceStateMachineRegistry) DeviceStateService {
	return &deviceStateServiceImpl{
		registry: registry,
	}
}

// NewDeviceStateServiceWithDefaults creates a new device state service with default configuration.
// This is a convenience function that:
// 1. Creates a device state machine factory
// 2. Registers default state transitions for all device types
// 3. Creates a registry with the factory
// 4. Creates and returns the service
//
// If logger is nil, a no-op logger will be used.
func NewDeviceStateServiceWithDefaults(logger *zap.Logger) (DeviceStateService, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	factory := statemachine.NewDeviceStateMachineFactory(logger)

	// Register default transitions for all device types
	if err := transitions.RegisterDefaultDeviceTypeTransitions(factory); err != nil {
		return nil, fmt.Errorf("failed to register default device type transitions: %w", err)
	}

	registry := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	service := NewDeviceStateService(registry)

	return service, nil
}

// deviceStateServiceImpl implements DeviceStateService by delegating to DeviceStateMachineRegistry
type deviceStateServiceImpl struct {
	registry types.DeviceStateMachineRegistry
}

// GetOrCreateStateMachine gets an existing state machine or creates a new one for a device
func (s *deviceStateServiceImpl) GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType types.DeviceType) (types.DeviceStateMachine, error) {
	return s.registry.GetOrCreateStateMachine(ctx, deviceID, deviceType)
}

// GetStateMachine retrieves a state machine by device ID
func (s *deviceStateServiceImpl) GetStateMachine(deviceID string) (types.DeviceStateMachine, error) {
	return s.registry.GetStateMachine(deviceID)
}

// GetAllStateMachines returns all registered state machines
func (s *deviceStateServiceImpl) GetAllStateMachines() map[string]types.DeviceStateMachine {
	machines := s.registry.GetAllStateMachines()
	// Convert slice to map for API compatibility
	result := make(map[string]types.DeviceStateMachine, len(machines))
	for _, machine := range machines {
		result[machine.GetDeviceID()] = machine
	}
	return result
}

// RemoveStateMachine removes a state machine
func (s *deviceStateServiceImpl) RemoveStateMachine(deviceID string) error {
	return s.registry.RemoveStateMachine(deviceID)
}

// GetStateMachinesByType returns all state machines for a specific device type
func (s *deviceStateServiceImpl) GetStateMachinesByType(deviceType types.DeviceType) []types.DeviceStateMachine {
	return s.registry.GetStateMachinesByType(deviceType)
}

