# State Machine Package Preparation

This document summarizes the findings from reviewing the current state machine implementation and the preparation for moving it to the `state-machine/` subpackage.

## Current Implementation Review

### Files to Review

1. **`device_state_machine.go`** (404 lines)
2. **`device_state_configs.go`** (295 lines)
3. **`camera_state_adapter.go`** (228 lines)
4. **`device_state_adapter.go`** (192 lines)
5. **`device-state-service.go`** (90 lines) - **STAYS IN ROOT**

### Components to Move

#### 1. Core State Machine Implementation (`device_state_machine.go`)

**Struct**: `deviceStateMachineImpl`
- Fields:
  - `deviceID string`
  - `deviceType iottypes.DeviceType`
  - `mu sync.RWMutex`
  - `state iottypes.DeviceState`
  - `stateInfo iottypes.DeviceStateInfo`
  - `transitions map[iottypes.DeviceState][]iottypes.DeviceState`
- **Note**: Currently missing logger field (should be added)

**Constructor**: `NewDeviceStateMachine(deviceID, deviceType, transitions)`
- Currently takes transitions map directly
- Should be updated to accept `*zap.Logger` parameter

**Methods** (10 methods):
1. `GetDeviceID() string`
2. `GetDeviceType() iottypes.DeviceType`
3. `GetState() iottypes.DeviceState`
4. `GetStateInfo() iottypes.DeviceStateInfo`
5. `Transition(newState, errorMsg) error`
6. `CanTransition(newState) bool`
7. `IsOperational() bool`
8. `IsReadyForProcessing() bool`
9. `SetMetadata(key, value)`
10. `GetMetadata(key) (interface{}, bool)`
11. `isValidTransition(from, to) bool` - private helper

#### 2. Factory Implementation (`device_state_machine.go`)

**Struct**: `deviceStateMachineFactoryImpl`
- Fields:
  - `mu sync.RWMutex`
  - `typeTransitions map[iottypes.DeviceType]map[iottypes.DeviceState][]iottypes.DeviceType`
  - `defaultTransitions map[iottypes.DeviceState][]iottypes.DeviceState`

**Constructor**: `NewDeviceStateMachineFactory()`
- **Note**: Should be updated to accept `*zap.Logger` parameter

**Methods** (3 methods):
1. `CreateStateMachine(deviceID, deviceType) (iottypes.DeviceStateMachine, error)`
2. `GetValidTransitions(deviceType, fromState) []iottypes.DeviceState`
3. `RegisterDeviceTypeTransitions(deviceType, rules) error`

**Helper Function**:
- `getDefaultDeviceStateTransitions() map[iottypes.DeviceState][]iottypes.DeviceState`

#### 3. Registry Implementation (`device_state_machine.go`)

**Struct**: `deviceStateMachineRegistryImpl`
- Fields:
  - `factory iottypes.DeviceStateMachineFactory`
  - `machines map[string]iottypes.DeviceStateMachine`
  - `mu sync.RWMutex`
- **Note**: Currently missing logger field (should be added)

**Constructor**: `NewDeviceStateMachineRegistry(factory)`
- **Note**: Should be updated to accept `*zap.Logger` parameter

**Methods** (5 methods):
1. `GetStateMachine(deviceID) (iottypes.DeviceStateMachine, error)`
2. `CreateStateMachine(deviceID, deviceType) (iottypes.DeviceStateMachine, error)`
3. `RemoveStateMachine(deviceID) error`
4. `GetAllStateMachines() []iottypes.DeviceStateMachine`
5. `GetStateMachinesByType(deviceType) []iottypes.DeviceStateMachine`

**Note**: Interface has `GetOrCreateStateMachine` method, but implementation doesn't have it yet. Need to add.

#### 4. Transition Configs (`device_state_configs.go`)

**Variables** (transition maps):
- `CameraDeviceStateTransitions` - Camera-specific transitions
- `SensorDeviceStateTransitions` - Sensor-specific transitions
- `AudioDeviceStateTransitions` - Audio device transitions
- `AccessControlDeviceStateTransitions` - Access control device transitions

**Functions**:
- `GetDeviceTypeTransitions(deviceType) map[DeviceState][]DeviceState`
- `RegisterDefaultDeviceTypeTransitions(factory) error`
- `convertTransitionsToRules(transitions) []DeviceStateTransitionRule`

#### 5. Camera State Adapter (`camera_state_adapter.go`)

**Struct**: `CameraStateAdapter`
- Fields:
  - `deviceSM DeviceStateMachine`

**Type**: `CameraWorkflowState` (string constants)
- `CameraWorkflowStateSynced`
- `CameraWorkflowStateWaitingForScreenshots`
- `CameraWorkflowStateScreenshotSetReady`
- `CameraWorkflowStateModelDeployed`
- `CameraWorkflowStateFrameProcessing`

**Struct**: `CameraStateInfo`
- Camera-specific state information

**Constructor**: `NewCameraStateAdapter(deviceSM)`

**Methods** (11 methods):
1. `GetCameraWorkflowState() CameraWorkflowState`
2. `SetCameraWorkflowState(workflowState)`
3. `TransitionToCameraWorkflowState(workflowState, errorMsg) error`
4. `SetModelID(modelID)`
5. `GetModelID() string`
6. `SetDatasetID(datasetID)`
7. `GetDatasetID() string`
8. `IsOperational() bool`
9. `IsReadyForProcessing() bool`
10. `GetDeviceStateMachine() DeviceStateMachine`
11. `GetCameraStateInfo() CameraStateInfo`

#### 6. Camera State Machine Adapter (`camera_state_adapter.go` - state-mng bridge)

**Struct**: `CameraStateMachineAdapter`
- Fields:
  - `deviceSM DeviceStateMachine`
  - `cameraAdapter *CameraStateAdapter`

**Constructor**: `NewCameraStateMachineAdapter(deviceSM)`

**Methods** (8 methods - implements state-mng CameraStateMachine interface):
1. `GetCameraID() string`
2. `GetState() statemngtypes.CameraState`
3. `GetStateInfo() statemngtypes.CameraStateInfo`
4. `Transition(newState, errorMsg) error`
5. `CanTransition(newState) bool`
6. `IsOperational() bool`
7. `IsReadyForProcessing() bool`
8. `SetModelID(modelID)`
9. `SetDatasetID(datasetID)`

**Helper Functions**:
- `transitionGenericState(newState, errorMsg) error` - private
- `canTransitionGenericState(newState) bool` - private
- `mapWorkflowStateToCameraState(workflowState, genericState) statemngtypes.CameraState`
- `mapCameraStateToWorkflowState(cameraState) CameraWorkflowState`
- `mapGenericStateToCameraState(genericState) statemngtypes.CameraState`
- `isValidCameraWorkflowTransition(from, to) bool`

**External Dependency**: `statemngtypes` package (state-mng bridge)

#### 7. Device State Adapter (`device_state_adapter.go`)

**Struct**: `DeviceStateAdapter`
- Fields:
  - `deviceSM DeviceStateMachine`

**Constructor**: `NewDeviceStateAdapter(deviceSM)`

**Methods** (8 methods):
1. `GetDeviceID() string`
2. `GetDeviceType() iottypes.DeviceType`
3. `GetState() iottypes.DeviceState`
4. `GetStateInfo() iottypes.DeviceStateInfo`
5. `Transition(newState, errorMsg) error`
6. `CanTransition(newState) bool`
7. `IsOperational() bool`
8. `IsReadyForProcessing() bool`

**Note**: This appears to be a simple wrapper/delegator. May not be needed if we use the interface directly.

### Components That Stay in Root

#### 1. DeviceStateService (`device-state-service.go`)

**Interface**: `DeviceStateService` - **STAYS IN ROOT**
- This is the top-level interface for managing device states
- Services like state-mng should use this interface

**Struct**: `deviceStateServiceImpl` - **STAYS IN ROOT**
- Wraps `DeviceStateMachineRegistry`
- Provides high-level operations

**Constructors**: - **STAY IN ROOT**
- `NewDeviceStateService(registry) DeviceStateService`
- `NewDeviceStateServiceWithDefaults() (DeviceStateService, error)`

**Methods** (5 methods - all delegate to registry):
1. `GetOrCreateStateMachine(ctx, deviceID, deviceType) (DeviceStateMachine, error)`
2. `GetStateMachine(deviceID) (DeviceStateMachine, error)`
3. `GetAllStateMachines() map[string]DeviceStateMachine`
4. `RemoveStateMachine(deviceID) error`
5. `GetStateMachinesByType(deviceType) []DeviceStateMachine`

**Note**: `GetOrCreateStateMachine` calls `registry.GetOrCreateStateMachine`, but registry interface may not have this method. Need to verify and add if missing.

### Dependencies

#### Imports from `iot/types` package:
- `iottypes.DeviceType`
- `iottypes.DeviceState`
- `iottypes.DeviceStateInfo`
- `iottypes.DeviceStateTransitionRule`
- `iottypes.DeviceStateMachine`
- `iottypes.DeviceStateMachineFactory`
- `iottypes.DeviceStateMachineRegistry`

#### Standard library imports:
- `context`
- `fmt`
- `sync`
- `time`

#### External dependencies:
- `go.uber.org/zap` (should be added for logging)
- `statemngtypes` (for CameraStateMachineAdapter - state-mng bridge)

### Type References to Update

All type references are already using `iottypes.` prefix, so they just need to be changed to `types.`:
- `iottypes.DeviceType` → `types.DeviceType`
- `iottypes.DeviceState` → `types.DeviceState`
- `iottypes.DeviceStateInfo` → `types.DeviceStateInfo`
- `iottypes.DeviceStateTransitionRule` → `types.DeviceStateTransitionRule`
- `iottypes.DeviceStateMachine` → `types.DeviceStateMachine`
- `iottypes.DeviceStateMachineFactory` → `types.DeviceStateMachineFactory`
- `iottypes.DeviceStateMachineRegistry` → `types.DeviceStateMachineRegistry`

### Test Files

**Status**: No test files found for state machine
- No `*state*test*.go` files in `internal/iot/`
- Tests will need to be created in `state-machine/machine_test.go`

### Registry Interface Issue

**Problem**: `device-state-service.go` calls `registry.GetOrCreateStateMachine()`, but:
- Interface `DeviceStateMachineRegistry` in `types/state.go` may not have this method
- Implementation `deviceStateMachineRegistryImpl` doesn't have this method

**Action**: Need to add `GetOrCreateStateMachine` method to:
1. Interface in `types/state.go`
2. Implementation in `device_state_machine.go` (will move to `state-machine/registry.go`)

### Error Handling

**Current**: Uses `fmt.Errorf` with string messages

**Target**: Use sentinel errors from `types/errors.go`:
- `types.ErrInvalidTransition` - when transition is invalid
- `types.ErrStateMachineNotFound` - when state machine not found

### Logging

**Current**: No structured logging

**Target**: Add structured logging using `zap.Logger`:
- Add `logger *zap.Logger` field to `deviceStateMachineImpl`
- Add `logger *zap.Logger` field to `deviceStateMachineFactoryImpl`
- Add `logger *zap.Logger` field to `deviceStateMachineRegistryImpl`
- Add logging to all methods (Info, Warn, Error, Debug as appropriate)

### Locking Strategy

**Current**: Uses `defer r.mu.Unlock()` pattern (holds lock during method execution)

**Target**: Follow VMGateway pattern - copy references under lock, call outside lock:
- For read operations: copy values under lock, return outside lock
- For write operations: current pattern is acceptable (short critical sections)

**Note**: Current implementation already follows good locking practices for most methods.

### Context Handling

**Current**: Context passed as parameter in some methods, never stored in struct ✅

**Target**: Continue this pattern ✅

## Target Package Structure

```
internal/iot/state-machine/
  machine.go              # deviceStateMachineImpl, factory, registry
  machine_test.go          # Unit tests
  examples_test.go        # Example tests
  transitions/
    configs.go            # Transition tables (CameraDeviceStateTransitions, etc.)
    defaults.go           # Default transitions and helper functions
  adapters/
    camera_workflow.go    # CameraStateAdapter (camera workflow state mapping)
    state_mng_bridge.go   # CameraStateMachineAdapter (state-mng bridge)
    device_adapter.go     # DeviceStateAdapter (if needed)
  PREPARATION.md          # This document
```

## Comparison with VMGateway

### VMGateway Pattern (`connection-state-machine/`)
```
connection-state-machine/
  doc.go
  impl/
    connection_state_machine.go
    connection_state_machine_test.go
    connection_state_machine_examples_test.go
```

### IoT State Machine Pattern (`state-machine/`)
```
state-machine/
  machine.go              # Core implementation
  machine_test.go         # Unit tests
  examples_test.go        # Example tests
  transitions/            # Transition configs (device-type-specific)
  adapters/               # Adapters (camera workflow, state-mng bridge)
```

**Key Differences**:
- IoT has device-type-specific transitions (camera, sensor, etc.) → `transitions/` subdirectory
- IoT has adapters for external services (state-mng) → `adapters/` subdirectory
- VMGateway is simpler (single state machine type)

## Migration Checklist

### Section 4.1: Preparation ✅
- [x] Review current implementation
- [x] Identify all components
- [x] Document dependencies
- [x] Identify what stays vs. moves
- [x] Create directory structure
- [x] Document findings

### Section 4.2: Move Core Implementation (Next)
- [ ] Create machine.go with deviceStateMachineImpl
- [ ] Add logger field and parameter
- [ ] Update all type references to use `types` package
- [ ] Add structured logging
- [ ] Use sentinel errors
- [ ] Create factory.go with factory implementation
- [ ] Create registry.go with registry implementation
- [ ] Add GetOrCreateStateMachine method

### Section 4.3: Move Transition Configs (Future)
- [ ] Create transitions/configs.go
- [ ] Create transitions/defaults.go
- [ ] Move all transition tables
- [ ] Move helper functions

### Section 4.4: Move Adapters (Future)
- [ ] Create adapters/camera_workflow.go
- [ ] Create adapters/state_mng_bridge.go
- [ ] Create adapters/device_adapter.go (if needed)
- [ ] Update imports for state-mng bridge

### Section 4.5: Tests (Future)
- [ ] Create machine_test.go
- [ ] Create examples_test.go
- [ ] Test all methods
- [ ] Test error cases
- [ ] Test concurrent access

### Section 4.6: Integration (Future)
- [ ] Update root package to use state-machine
- [ ] Update device-state-service.go to use state-machine
- [ ] Update iot_impl.go to use state-machine
- [ ] Remove old files from root

## Notes

1. **Logger Addition**: The current implementation doesn't have logging. We should add it following VMGateway patterns.

2. **GetOrCreateStateMachine**: Need to add this method to registry interface and implementation.

3. **DeviceStateAdapter**: This appears to be a simple wrapper. May not be needed if we use the interface directly. Review during migration.

4. **No Tests**: Currently no tests exist. We'll need to create comprehensive tests.

5. **Package Name**: Using `statemachine` as package name (following Go naming conventions for hyphenated directories).

6. **External Dependency**: `CameraStateMachineAdapter` depends on `state-mng/types`. This is acceptable as it's isolated in `adapters/` subdirectory.

7. **DeviceStateService Stays**: As per plan, `DeviceStateService` stays in root as a clean wrapper around the registry.

