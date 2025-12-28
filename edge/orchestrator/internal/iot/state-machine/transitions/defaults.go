package transitions

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// RegisterDefaultDeviceTypeTransitions registers default state transitions for all known device types.
// This function registers transitions for:
//   - Cameras
//   - Sensors (motion, temperature, humidity, door, window, smoke, CO2, generic)
//   - Audio devices (microphones)
//   - Access control devices (door locks, keypads, card readers, biometric)
//
// Returns an error if any registration fails.
func RegisterDefaultDeviceTypeTransitions(factory types.DeviceStateMachineFactory) error {
	// Register camera transitions
	if err := factory.RegisterDeviceTypeTransitions(
		types.DeviceTypeCamera,
		convertTransitionsToRules(CameraDeviceStateTransitions),
	); err != nil {
		return fmt.Errorf("failed to register camera transitions: %w", err)
	}

	// Register sensor transitions
	sensorTypes := []types.DeviceType{
		types.DeviceTypeMotionSensor,
		types.DeviceTypeTemperatureSensor,
		types.DeviceTypeHumiditySensor,
		types.DeviceTypeDoorSensor,
		types.DeviceTypeWindowSensor,
		types.DeviceTypeSmokeDetector,
		types.DeviceTypeCO2Sensor,
		types.DeviceTypeGenericSensor,
	}
	for _, sensorType := range sensorTypes {
		if err := factory.RegisterDeviceTypeTransitions(
			sensorType,
			convertTransitionsToRules(SensorDeviceStateTransitions),
		); err != nil {
			return fmt.Errorf("failed to register sensor transitions for %s: %w", sensorType, err)
		}
	}

	// Register audio device transitions
	if err := factory.RegisterDeviceTypeTransitions(
		types.DeviceTypeMicrophone,
		convertTransitionsToRules(AudioDeviceStateTransitions),
	); err != nil {
		return fmt.Errorf("failed to register audio device transitions: %w", err)
	}

	// Register access control device transitions
	accessControlTypes := []types.DeviceType{
		types.DeviceTypeDoorLock,
		types.DeviceTypeKeypad,
		types.DeviceTypeCardReader,
		types.DeviceTypeBiometric,
	}
	for _, acType := range accessControlTypes {
		if err := factory.RegisterDeviceTypeTransitions(
			acType,
			convertTransitionsToRules(AccessControlDeviceStateTransitions),
		); err != nil {
			return fmt.Errorf("failed to register access control transitions for %s: %w", acType, err)
		}
	}

	return nil
}

// convertTransitionsToRules converts a transitions map to a slice of DeviceStateTransitionRule.
// This helper function is used to convert the transition maps into the format expected by the factory.
func convertTransitionsToRules(transitions map[types.DeviceState][]types.DeviceState) []types.DeviceStateTransitionRule {
	rules := make([]types.DeviceStateTransitionRule, 0, len(transitions))
	for fromState, toStates := range transitions {
		rules = append(rules, types.DeviceStateTransitionRule{
			FromState: fromState,
			ToStates:  toStates,
		})
	}
	return rules
}

