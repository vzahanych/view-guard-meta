package iot

import "fmt"

// CameraDeviceStateTransitions defines camera-specific state transitions
// Cameras have additional states beyond generic device states:
// - waiting_for_screenshots, screenshot_set_ready, model_deployed, frame_processing
// These map to generic states: registered -> active -> processing
var CameraDeviceStateTransitions = map[DeviceState][]DeviceState{
	DeviceStateUndiscovered: {
		DeviceStateDiscovered,
		DeviceStateError,
	},
	DeviceStateDiscovered: {
		DeviceStateRegistered, // Maps to "synced" in camera terminology
		DeviceStateDisconnected,
		DeviceStateError,
	},
	DeviceStateRegistered: {
		DeviceStateActive, // Maps to "waiting_for_screenshots" or "screenshot_set_ready"
		DeviceStateProcessing, // Maps to "model_deployed" or "frame_processing"
		DeviceStateDisconnected,
		DeviceStateError,
		DeviceStateDisabled,
	},
	DeviceStateActive: {
		DeviceStateProcessing, // Start processing (model deployed)
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
		DeviceStateActive, // Stop processing, go back to active
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

// SensorDeviceStateTransitions defines sensor-specific state transitions
// Sensors have simpler state flows: discovered -> registered -> active -> processing
var SensorDeviceStateTransitions = map[DeviceState][]DeviceState{
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
		DeviceStateProcessing, // Start reading sensors
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
		DeviceStateDiscovered,
		DeviceStateRegistered,
		DeviceStateDisconnected,
	},
	DeviceStateDisconnected: {
		DeviceStateDiscovered,
		DeviceStateError,
	},
	DeviceStateDisabled: {
		DeviceStateRegistered,
		DeviceStateError,
	},
}

// AudioDeviceStateTransitions defines audio device-specific state transitions
// Audio devices follow similar patterns to sensors
var AudioDeviceStateTransitions = map[DeviceState][]DeviceState{
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
		DeviceStateProcessing, // Start capturing/streaming audio
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
		DeviceStateDiscovered,
		DeviceStateRegistered,
		DeviceStateDisconnected,
	},
	DeviceStateDisconnected: {
		DeviceStateDiscovered,
		DeviceStateError,
	},
	DeviceStateDisabled: {
		DeviceStateRegistered,
		DeviceStateError,
	},
}

// AccessControlDeviceStateTransitions defines access control device-specific state transitions
// Access control devices (locks, keypads) have simpler operational states
var AccessControlDeviceStateTransitions = map[DeviceState][]DeviceState{
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
		DeviceStateActive, // Device is ready to control access
		DeviceStateIdle,
		DeviceStateDisconnected,
		DeviceStateError,
		DeviceStateDisabled,
	},
	DeviceStateActive: {
		DeviceStateIdle,
		DeviceStateDisconnected,
		DeviceStateError,
		DeviceStateDisabled,
	},
	DeviceStateIdle: {
		DeviceStateActive,
		DeviceStateDisconnected,
		DeviceStateError,
		DeviceStateDisabled,
	},
	DeviceStateProcessing: {
		// Access control devices typically don't have a "processing" state
		// But we include it for consistency
		DeviceStateActive,
		DeviceStateIdle,
		DeviceStateDisconnected,
		DeviceStateError,
		DeviceStateDisabled,
	},
	DeviceStateError: {
		DeviceStateDiscovered,
		DeviceStateRegistered,
		DeviceStateDisconnected,
	},
	DeviceStateDisconnected: {
		DeviceStateDiscovered,
		DeviceStateError,
	},
	DeviceStateDisabled: {
		DeviceStateRegistered,
		DeviceStateError,
	},
}

// GetDeviceTypeTransitions returns state transitions for a specific device type
func GetDeviceTypeTransitions(deviceType DeviceType) map[DeviceState][]DeviceState {
	switch deviceType {
	case DeviceTypeCamera:
		return CameraDeviceStateTransitions
	case DeviceTypeMotionSensor, DeviceTypeTemperatureSensor, DeviceTypeHumiditySensor,
		DeviceTypeDoorSensor, DeviceTypeWindowSensor, DeviceTypeSmokeDetector,
		DeviceTypeCO2Sensor, DeviceTypeGenericSensor:
		return SensorDeviceStateTransitions
	case DeviceTypeMicrophone:
		return AudioDeviceStateTransitions
	case DeviceTypeDoorLock, DeviceTypeKeypad, DeviceTypeCardReader, DeviceTypeBiometric:
		return AccessControlDeviceStateTransitions
	default:
		// Use default transitions for unknown device types
		return getDefaultDeviceStateTransitions()
	}
}

// RegisterDefaultDeviceTypeTransitions registers default state transitions for all known device types
func RegisterDefaultDeviceTypeTransitions(factory DeviceStateMachineFactory) error {
	// Register camera transitions
	if err := factory.RegisterDeviceTypeTransitions(DeviceTypeCamera, convertTransitionsToRules(CameraDeviceStateTransitions)); err != nil {
		return fmt.Errorf("failed to register camera transitions: %w", err)
	}

	// Register sensor transitions
	sensorTypes := []DeviceType{
		DeviceTypeMotionSensor, DeviceTypeTemperatureSensor, DeviceTypeHumiditySensor,
		DeviceTypeDoorSensor, DeviceTypeWindowSensor, DeviceTypeSmokeDetector,
		DeviceTypeCO2Sensor, DeviceTypeGenericSensor,
	}
	for _, sensorType := range sensorTypes {
		if err := factory.RegisterDeviceTypeTransitions(sensorType, convertTransitionsToRules(SensorDeviceStateTransitions)); err != nil {
			return fmt.Errorf("failed to register sensor transitions for %s: %w", sensorType, err)
		}
	}

	// Register audio device transitions
	if err := factory.RegisterDeviceTypeTransitions(DeviceTypeMicrophone, convertTransitionsToRules(AudioDeviceStateTransitions)); err != nil {
		return fmt.Errorf("failed to register audio device transitions: %w", err)
	}

	// Register access control device transitions
	accessControlTypes := []DeviceType{
		DeviceTypeDoorLock, DeviceTypeKeypad, DeviceTypeCardReader, DeviceTypeBiometric,
	}
	for _, acType := range accessControlTypes {
		if err := factory.RegisterDeviceTypeTransitions(acType, convertTransitionsToRules(AccessControlDeviceStateTransitions)); err != nil {
			return fmt.Errorf("failed to register access control transitions for %s: %w", acType, err)
		}
	}

	return nil
}

// convertTransitionsToRules converts a transitions map to DeviceStateTransitionRule slice
func convertTransitionsToRules(transitions map[DeviceState][]DeviceState) []DeviceStateTransitionRule {
	rules := make([]DeviceStateTransitionRule, 0, len(transitions))
	for fromState, toStates := range transitions {
		rules = append(rules, DeviceStateTransitionRule{
			FromState: fromState,
			ToStates:  toStates,
		})
	}
	return rules
}

