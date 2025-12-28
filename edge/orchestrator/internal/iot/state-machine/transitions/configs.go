package transitions

import (
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// CameraDeviceStateTransitions defines camera-specific state transitions.
// Cameras have additional states beyond generic device states:
// - waiting_for_screenshots, screenshot_set_ready, model_deployed, frame_processing
// These map to generic states: registered -> active -> processing
var CameraDeviceStateTransitions = map[types.DeviceState][]types.DeviceState{
	types.DeviceStateUndiscovered: {
		types.DeviceStateDiscovered,
		types.DeviceStateError,
	},
	types.DeviceStateDiscovered: {
		types.DeviceStateRegistered, // Maps to "synced" in camera terminology
		types.DeviceStateDisconnected,
		types.DeviceStateError,
	},
	types.DeviceStateRegistered: {
		types.DeviceStateActive, // Maps to "waiting_for_screenshots" or "screenshot_set_ready"
		types.DeviceStateProcessing, // Maps to "model_deployed" or "frame_processing"
		types.DeviceStateDisconnected,
		types.DeviceStateError,
		types.DeviceStateDisabled,
	},
	types.DeviceStateActive: {
		types.DeviceStateProcessing, // Start processing (model deployed)
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
		types.DeviceStateActive, // Stop processing, go back to active
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

// SensorDeviceStateTransitions defines sensor-specific state transitions.
// Sensors have simpler state flows: discovered -> registered -> active -> processing
var SensorDeviceStateTransitions = map[types.DeviceState][]types.DeviceState{
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
		types.DeviceStateProcessing, // Start reading sensors
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
		types.DeviceStateDiscovered,
		types.DeviceStateRegistered,
		types.DeviceStateDisconnected,
	},
	types.DeviceStateDisconnected: {
		types.DeviceStateDiscovered,
		types.DeviceStateError,
	},
	types.DeviceStateDisabled: {
		types.DeviceStateRegistered,
		types.DeviceStateError,
	},
}

// AudioDeviceStateTransitions defines audio device-specific state transitions.
// Audio devices follow similar patterns to sensors
var AudioDeviceStateTransitions = map[types.DeviceState][]types.DeviceState{
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
		types.DeviceStateProcessing, // Start capturing/streaming audio
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
		types.DeviceStateDiscovered,
		types.DeviceStateRegistered,
		types.DeviceStateDisconnected,
	},
	types.DeviceStateDisconnected: {
		types.DeviceStateDiscovered,
		types.DeviceStateError,
	},
	types.DeviceStateDisabled: {
		types.DeviceStateRegistered,
		types.DeviceStateError,
	},
}

// AccessControlDeviceStateTransitions defines access control device-specific state transitions.
// Access control devices (locks, keypads) have simpler operational states
var AccessControlDeviceStateTransitions = map[types.DeviceState][]types.DeviceState{
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
		types.DeviceStateActive, // Device is ready to control access
		types.DeviceStateIdle,
		types.DeviceStateDisconnected,
		types.DeviceStateError,
		types.DeviceStateDisabled,
	},
	types.DeviceStateActive: {
		types.DeviceStateIdle,
		types.DeviceStateDisconnected,
		types.DeviceStateError,
		types.DeviceStateDisabled,
	},
	types.DeviceStateIdle: {
		types.DeviceStateActive,
		types.DeviceStateDisconnected,
		types.DeviceStateError,
		types.DeviceStateDisabled,
	},
	types.DeviceStateProcessing: {
		// Access control devices typically don't have a "processing" state
		// But we include it for consistency
		types.DeviceStateActive,
		types.DeviceStateIdle,
		types.DeviceStateDisconnected,
		types.DeviceStateError,
		types.DeviceStateDisabled,
	},
	types.DeviceStateError: {
		types.DeviceStateDiscovered,
		types.DeviceStateRegistered,
		types.DeviceStateDisconnected,
	},
	types.DeviceStateDisconnected: {
		types.DeviceStateDiscovered,
		types.DeviceStateError,
	},
	types.DeviceStateDisabled: {
		types.DeviceStateRegistered,
		types.DeviceStateError,
	},
}

// GetDeviceTypeTransitions returns state transitions for a specific device type.
// Returns the appropriate transition map based on the device type, or nil if no specific transitions are defined.
func GetDeviceTypeTransitions(deviceType types.DeviceType) map[types.DeviceState][]types.DeviceState {
	switch deviceType {
	case types.DeviceTypeCamera:
		return CameraDeviceStateTransitions
	case types.DeviceTypeMotionSensor, types.DeviceTypeTemperatureSensor, types.DeviceTypeHumiditySensor,
		types.DeviceTypeDoorSensor, types.DeviceTypeWindowSensor, types.DeviceTypeSmokeDetector,
		types.DeviceTypeCO2Sensor, types.DeviceTypeGenericSensor:
		return SensorDeviceStateTransitions
	case types.DeviceTypeMicrophone:
		return AudioDeviceStateTransitions
	case types.DeviceTypeDoorLock, types.DeviceTypeKeypad, types.DeviceTypeCardReader, types.DeviceTypeBiometric:
		return AccessControlDeviceStateTransitions
	default:
		// Return nil for unknown device types - caller should use default transitions
		return nil
	}
}

