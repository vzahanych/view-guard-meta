# Transition Configurations

This document summarizes the transition configuration files moved to the `transitions/` subpackage.

## Files Created

### 1. `configs.go` (248 lines)
**Purpose**: Device-type-specific state transition tables

**Components**:
- **4 Transition Maps**:
  - `CameraDeviceStateTransitions` - Camera-specific state transitions (9 states)
  - `SensorDeviceStateTransitions` - Sensor-specific state transitions (9 states)
  - `AudioDeviceStateTransitions` - Audio device state transitions (9 states)
  - `AccessControlDeviceStateTransitions` - Access control device transitions (9 states)

- **1 Helper Function**:
  - `GetDeviceTypeTransitions(deviceType)` - Returns appropriate transition map for device type
    - Supports: Camera, 8 sensor types, Microphone, 4 access control types
    - Returns `nil` for unknown device types (caller should use defaults)

**Key Features**:
- All type references use `types` package
- All transition maps exported for use by factory
- Helper function exported for external use
- Clear documentation comments

### 2. `defaults.go` (85 lines)
**Purpose**: Default transition registration helpers

**Components**:
- **1 Main Function**:
  - `RegisterDefaultDeviceTypeTransitions(factory)` - Registers all default transitions:
    - Camera transitions
    - Sensor transitions (8 types: motion, temperature, humidity, door, window, smoke, CO2, generic)
    - Audio device transitions (microphone)
    - Access control transitions (4 types: door lock, keypad, card reader, biometric)

- **1 Helper Function**:
  - `convertTransitionsToRules(transitions)` - Converts transition map to rules slice
    - Used internally by `RegisterDefaultDeviceTypeTransitions`

**Key Features**:
- Comprehensive error handling with context
- Well-documented functions
- Proper error wrapping with `fmt.Errorf` and `%w`
- All type references use `types` package

## Device Type Coverage

### Cameras
- **Device Type**: `types.DeviceTypeCamera`
- **Transitions**: Camera-specific workflow states mapped to generic states

### Sensors (8 types)
- **Device Types**:
  - `types.DeviceTypeMotionSensor`
  - `types.DeviceTypeTemperatureSensor`
  - `types.DeviceTypeHumiditySensor`
  - `types.DeviceTypeDoorSensor`
  - `types.DeviceTypeWindowSensor`
  - `types.DeviceTypeSmokeDetector`
  - `types.DeviceTypeCO2Sensor`
  - `types.DeviceTypeGenericSensor`
- **Transitions**: Simple sensor state flow (discovered -> registered -> active -> processing)

### Audio Devices
- **Device Type**: `types.DeviceTypeMicrophone`
- **Transitions**: Similar to sensors (discovered -> registered -> active -> processing)

### Access Control Devices (4 types)
- **Device Types**:
  - `types.DeviceTypeDoorLock`
  - `types.DeviceTypeKeypad`
  - `types.DeviceTypeCardReader`
  - `types.DeviceTypeBiometric`
- **Transitions**: Simpler operational states (no processing state typically)

## State Transition Patterns

All device types follow a common pattern with 9 generic states:
1. **Undiscovered** → Discovered, Error
2. **Discovered** → Registered, Disconnected, Error
3. **Registered** → Active, Idle, Disconnected, Error, Disabled
4. **Active** → Processing, Idle, Disconnected, Error, Disabled
5. **Idle** → Active, Processing, Disconnected, Error, Disabled
6. **Processing** → Active, Idle, Disconnected, Error, Disabled
7. **Error** → Discovered, Registered, Disconnected
8. **Disconnected** → Discovered, Error
9. **Disabled** → Registered, Error

Device-type-specific transitions customize this pattern based on device capabilities.

## Usage

### Registering Default Transitions
```go
factory := statemachine.NewDeviceStateMachineFactory(logger)
if err := transitions.RegisterDefaultDeviceTypeTransitions(factory); err != nil {
    return fmt.Errorf("failed to register transitions: %w", err)
}
```

### Getting Transitions for a Device Type
```go
transitions := transitions.GetDeviceTypeTransitions(deviceType)
if transitions == nil {
    // Use default transitions
    transitions = getDefaultDeviceStateTransitions()
}
```

## Statistics

- **Total lines**: 333 lines (configs.go: 248, defaults.go: 85)
- **Transition maps**: 4 maps
- **Functions**: 2 exported functions
- **Device types covered**: 14 device types (1 camera + 8 sensors + 1 audio + 4 access control)
- **States per map**: 9 states each

## Compilation Status

✅ **All files compile successfully**
- No compilation errors
- No linter errors
- Package structure correct

## Next Steps

1. **Section 4.4**: Move adapters to `adapters/` subdirectory
2. **Section 4.5**: Create tests
3. **Section 4.6**: Update root package to use state-machine package

