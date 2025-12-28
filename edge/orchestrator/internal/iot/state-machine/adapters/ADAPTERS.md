# State Machine Adapters

This document summarizes the adapter files moved to the `adapters/` subpackage.

## Files Created

### 1. `camera_workflow.go` (240 lines)
**Purpose**: Camera-specific workflow state adapter

**Components**:
- **Types**:
  - `CameraWorkflowState` - Camera-specific workflow state type
  - `CameraStateInfo` - Camera-specific state information struct

- **Constants** (5 workflow states):
  - `CameraWorkflowStateSynced`
  - `CameraWorkflowStateWaitingForScreenshots`
  - `CameraWorkflowStateScreenshotSetReady`
  - `CameraWorkflowStateModelDeployed`
  - `CameraWorkflowStateFrameProcessing`

- **Struct**: `CameraStateAdapter`
  - Fields: `deviceSM types.DeviceStateMachine`, `logger *zap.Logger`

- **Methods** (11 methods):
  - `NewCameraStateAdapter(deviceSM, logger)` - Constructor
  - `GetCameraWorkflowState()` - Get workflow state from metadata
  - `SetCameraWorkflowState(workflowState)` - Set workflow state
  - `TransitionToCameraWorkflowState(workflowState, errorMsg)` - Transition to workflow state
  - `SetModelID(modelID)` / `GetModelID()` - Model ID management
  - `SetDatasetID(datasetID)` / `GetDatasetID()` - Dataset ID management
  - `IsOperational()` - Check if operational
  - `IsReadyForProcessing()` - Check if ready for processing
  - `GetDeviceStateMachine()` - Get underlying state machine
  - `GetCameraStateInfo()` - Get camera-specific state info

**Key Features**:
- Maps camera workflow states to generic device states
- Stores workflow states in device state metadata
- All type references use `types` package
- Structured logging throughout (Info, Debug, Error)
- Proper error handling with error wrapping

### 2. `state_mng_bridge.go` (273 lines)
**Purpose**: Bridge adapter to state-mng service (external dependency)

**Components**:
- **Struct**: `CameraStateMachineAdapter`
  - Fields: `deviceSM types.DeviceStateMachine`, `cameraAdapter *CameraStateAdapter`, `logger *zap.Logger`
  - **Implements**: `statemngtypes.CameraStateMachine` interface

- **Methods** (9 methods):
  - `NewCameraStateMachineAdapter(deviceSM, logger)` - Constructor
  - `GetCameraID()` - Get camera ID
  - `GetState()` - Returns `statemngtypes.CameraState`
  - `GetStateInfo()` - Returns `statemngtypes.CameraStateInfo`
  - `Transition(newState, errorMsg)` - Transition to camera state
  - `CanTransition(newState)` - Check if transition is valid
  - `IsOperational()` - Check if operational
  - `IsReadyForProcessing()` - Check if ready for processing
  - `SetModelID(modelID)` / `SetDatasetID(datasetID)` - Delegate to camera adapter

- **Helper Functions** (4 functions):
  - `transitionGenericState(newState, errorMsg)` - Handle generic state transitions
  - `canTransitionGenericState(newState)` - Check generic state transitions
  - `mapWorkflowStateToCameraState(workflowState, genericState)` - Map workflow to camera state
  - `mapCameraStateToWorkflowState(cameraState)` - Map camera to workflow state
  - `mapGenericStateToCameraState(genericState)` - Map generic to camera state
  - `isValidCameraWorkflowTransition(from, to)` - Validate workflow transitions

**Key Features**:
- **External Dependency Isolation**: `statemngtypes` import isolated to adapters package
- **Bridge Pattern**: Adapts `types.DeviceStateMachine` to `statemngtypes.CameraStateMachine`
- **Structured Logging**: All methods include appropriate logging
- **Error Handling**: Proper error wrapping with context
- **Documentation**: Package-level comment explains the bridge purpose

## External Dependency Isolation

The `state_mng_bridge.go` file isolates the external dependency on `state-mng/types`:
- **Before**: Root `iot` package imported `state-mng/types` directly
- **After**: Only `adapters` package imports `state-mng/types`
- **Benefit**: Root `iot` package is no longer coupled to `state-mng` service

## Usage

### Camera Workflow Adapter
```go
import "github.com/.../internal/iot/state-machine/adapters"

adapter := adapters.NewCameraStateAdapter(deviceSM, logger)
workflowState := adapter.GetCameraWorkflowState()
adapter.TransitionToCameraWorkflowState(adapters.CameraWorkflowStateModelDeployed, "")
```

### State-Mng Bridge Adapter
```go
import "github.com/.../internal/iot/state-machine/adapters"

bridge := adapters.NewCameraStateMachineAdapter(deviceSM, logger)
cameraState := bridge.GetState() // Returns statemngtypes.CameraState
bridge.Transition(statemngtypes.CameraStateSynced, "")
```

## Statistics

- **Total lines**: 513 lines (camera_workflow.go: 240, state_mng_bridge.go: 273)
- **Adapters**: 2 adapters
- **Methods**: 20 methods total
- **Helper functions**: 4 helper functions
- **Types**: 2 types (CameraWorkflowState, CameraStateInfo)
- **Constants**: 5 workflow state constants

## Compilation Status

✅ **All files compile successfully**
- No compilation errors
- No linter errors
- Package structure correct
- External dependency properly isolated

## Next Steps

1. **Section 4.5**: Update DeviceStateService wrapper to use state-machine package
2. **Section 4.6**: Move and update tests
3. **Section 4.7**: Delete old files and verify

