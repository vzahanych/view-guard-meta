package types

import "errors"

var (
	// Service lifecycle errors
	ErrNotInitialized = errors.New("iot: service not initialized")
	ErrAlreadyStarted = errors.New("iot: service already started")
	ErrNotStarted     = errors.New("iot: service not started")

	// Device errors
	ErrDeviceNotFound = errors.New("iot: device not found")
	ErrDeviceExists   = errors.New("iot: device already exists")
	ErrInvalidDevice  = errors.New("iot: invalid device")

	// Plugin errors
	ErrPluginNotFound  = errors.New("iot: plugin not found")
	ErrPluginExists    = errors.New("iot: plugin already registered")
	ErrNoPluginForType = errors.New("iot: no plugin for device type")

	// State errors
	ErrInvalidTransition    = errors.New("iot: invalid state transition")
	ErrStateMachineNotFound = errors.New("iot: state machine not found")

	// Processing errors
	ErrProcessorNotFound = errors.New("iot: processor not found")
	ErrProcessorExists  = errors.New("iot: processor already registered")

	// Config errors
	ErrInvalidConfig = errors.New("iot: invalid configuration")
)

