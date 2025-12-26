package iot

import (
	"context"
	"fmt"
)

// DeviceStateService provides access to device state machines.
// This is the top interface for managing device states in the IoT layer.
// Services like state-mng should use this interface to query and manage device states,
// rather than implementing their own state machines.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_device_state_service.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DeviceStateService
type DeviceStateService interface {
	// GetOrCreateStateMachine gets an existing state machine or creates a new one for a device
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

// NewDeviceStateService creates a new device state service.
// This wraps the DeviceStateMachineRegistry and provides the top-level interface
// for accessing device state machines.
func NewDeviceStateService(registry DeviceStateMachineRegistry) DeviceStateService {
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
func NewDeviceStateServiceWithDefaults() (DeviceStateService, error) {
	factory := NewDeviceStateMachineFactory()
	
	// Register default transitions for all device types
	if err := RegisterDefaultDeviceTypeTransitions(factory); err != nil {
		return nil, fmt.Errorf("failed to register default device type transitions: %w", err)
	}
	
	registry := NewDeviceStateMachineRegistry(factory)
	service := NewDeviceStateService(registry)
	
	return service, nil
}

// deviceStateServiceImpl implements DeviceStateService by delegating to DeviceStateMachineRegistry
type deviceStateServiceImpl struct {
	registry DeviceStateMachineRegistry
}

// GetOrCreateStateMachine gets an existing state machine or creates a new one for a device
func (s *deviceStateServiceImpl) GetOrCreateStateMachine(ctx context.Context, deviceID string, deviceType DeviceType) (DeviceStateMachine, error) {
	return s.registry.GetOrCreateStateMachine(ctx, deviceID, deviceType)
}

// GetStateMachine retrieves a state machine by device ID
func (s *deviceStateServiceImpl) GetStateMachine(deviceID string) (DeviceStateMachine, error) {
	return s.registry.GetStateMachine(deviceID)
}

// GetAllStateMachines returns all registered state machines
func (s *deviceStateServiceImpl) GetAllStateMachines() map[string]DeviceStateMachine {
	return s.registry.GetAllStateMachines()
}

// RemoveStateMachine removes a state machine
func (s *deviceStateServiceImpl) RemoveStateMachine(deviceID string) error {
	return s.registry.RemoveStateMachine(deviceID)
}

// GetStateMachinesByType returns all state machines for a specific device type
func (s *deviceStateServiceImpl) GetStateMachinesByType(deviceType DeviceType) []DeviceStateMachine {
	return s.registry.GetStateMachinesByType(deviceType)
}

