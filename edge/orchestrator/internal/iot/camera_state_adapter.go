package iot

import (
	"fmt"

	statemngtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
)

// CameraStateMachineAdapter adapts a DeviceStateMachine to the CameraStateMachine interface.
// This allows state-mng to use the iot device state machine while maintaining
// backward compatibility with the CameraStateMachine interface.
type CameraStateMachineAdapter struct {
	deviceSM  DeviceStateMachine
	cameraAdapter *CameraStateAdapter
}

// NewCameraStateMachineAdapter creates a new adapter that wraps a DeviceStateMachine
// and implements the CameraStateMachine interface.
func NewCameraStateMachineAdapter(deviceSM DeviceStateMachine) *CameraStateMachineAdapter {
	return &CameraStateMachineAdapter{
		deviceSM:      deviceSM,
		cameraAdapter: NewCameraStateAdapter(deviceSM),
	}
}

// GetCameraID returns the camera ID this state machine is for
func (a *CameraStateMachineAdapter) GetCameraID() string {
	return a.deviceSM.GetDeviceID()
}

// GetState returns the current camera state
func (a *CameraStateMachineAdapter) GetState() statemngtypes.CameraState {
	workflowState := a.cameraAdapter.GetCameraWorkflowState()
	return mapWorkflowStateToCameraState(workflowState, a.deviceSM.GetState())
}

// GetStateInfo returns detailed camera state information
func (a *CameraStateMachineAdapter) GetStateInfo() statemngtypes.CameraStateInfo {
	cameraInfo := a.cameraAdapter.GetCameraStateInfo()
	return statemngtypes.CameraStateInfo{
		CameraID:     cameraInfo.CameraID,
		State:        mapWorkflowStateToCameraState(cameraInfo.State, cameraInfo.GenericState),
		LastUpdated:  cameraInfo.LastUpdated,
		Error:        cameraInfo.Error,
		ModelID:      cameraInfo.ModelID,
		DatasetID:    cameraInfo.DatasetID,
		IsProcessing: cameraInfo.IsProcessing,
	}
}

// Transition transitions to a new camera state
func (a *CameraStateMachineAdapter) Transition(newState statemngtypes.CameraState, errorMsg string) error {
	workflowState := mapCameraStateToWorkflowState(newState)
	if workflowState == "" {
		// For states that don't map to workflow states, use generic device state transitions
		return a.transitionGenericState(newState, errorMsg)
	}
	return a.cameraAdapter.TransitionToCameraWorkflowState(workflowState, errorMsg)
}

// CanTransition checks if a transition from current state to new state is valid
func (a *CameraStateMachineAdapter) CanTransition(newState statemngtypes.CameraState) bool {
	workflowState := mapCameraStateToWorkflowState(newState)
	if workflowState == "" {
		// For generic states, check device state machine
		return a.canTransitionGenericState(newState)
	}
	
	// Check if we can transition to the workflow state
	currentWorkflowState := a.cameraAdapter.GetCameraWorkflowState()
	return isValidCameraWorkflowTransition(currentWorkflowState, workflowState)
}

// IsOperational returns true if camera is in an operational state
func (a *CameraStateMachineAdapter) IsOperational() bool {
	return a.cameraAdapter.IsOperational()
}

// IsReadyForProcessing returns true if camera is ready to process frames
func (a *CameraStateMachineAdapter) IsReadyForProcessing() bool {
	return a.cameraAdapter.IsReadyForProcessing()
}

// SetModelID sets the model ID (delegates to camera adapter)
func (a *CameraStateMachineAdapter) SetModelID(modelID string) {
	a.cameraAdapter.SetModelID(modelID)
}

// SetDatasetID sets the dataset ID (delegates to camera adapter)
func (a *CameraStateMachineAdapter) SetDatasetID(datasetID string) {
	a.cameraAdapter.SetDatasetID(datasetID)
}

// transitionGenericState handles transitions for states that map to generic device states
func (a *CameraStateMachineAdapter) transitionGenericState(newState statemngtypes.CameraState, errorMsg string) error {
	var targetDeviceState DeviceState
	switch newState {
	case statemngtypes.CameraStateUndiscovered:
		targetDeviceState = DeviceStateUndiscovered
	case statemngtypes.CameraStateDiscovered:
		targetDeviceState = DeviceStateDiscovered
	case statemngtypes.CameraStateDisconnected:
		targetDeviceState = DeviceStateDisconnected
		// Clear workflow state metadata when transitioning to disconnected
		a.deviceSM.SetMetadata("camera_workflow_state", nil)
	case statemngtypes.CameraStateError:
		targetDeviceState = DeviceStateError
		// Clear workflow state metadata when transitioning to error
		a.deviceSM.SetMetadata("camera_workflow_state", nil)
	default:
		return fmt.Errorf("cannot transition to camera state %s directly (use workflow states)", newState)
	}
	return a.deviceSM.Transition(targetDeviceState, errorMsg)
}

// canTransitionGenericState checks if a generic state transition is valid
func (a *CameraStateMachineAdapter) canTransitionGenericState(newState statemngtypes.CameraState) bool {
	var targetDeviceState DeviceState
	switch newState {
	case statemngtypes.CameraStateUndiscovered:
		targetDeviceState = DeviceStateUndiscovered
	case statemngtypes.CameraStateDiscovered:
		targetDeviceState = DeviceStateDiscovered
	case statemngtypes.CameraStateDisconnected:
		targetDeviceState = DeviceStateDisconnected
	case statemngtypes.CameraStateError:
		targetDeviceState = DeviceStateError
	default:
		return false
	}
	return a.deviceSM.CanTransition(targetDeviceState)
}

// mapWorkflowStateToCameraState maps camera workflow state to camera state
func mapWorkflowStateToCameraState(workflowState CameraWorkflowState, genericState DeviceState) statemngtypes.CameraState {
	switch workflowState {
	case CameraWorkflowStateSynced:
		return statemngtypes.CameraStateSynced
	case CameraWorkflowStateWaitingForScreenshots:
		return statemngtypes.CameraStateWaitingForScreenshots
	case CameraWorkflowStateScreenshotSetReady:
		return statemngtypes.CameraStateScreenshotSetReady
	case CameraWorkflowStateModelDeployed:
		return statemngtypes.CameraStateModelDeployed
	case CameraWorkflowStateFrameProcessing:
		return statemngtypes.CameraStateFrameProcessing
	default:
		// Fall back to generic state mapping
		return mapGenericStateToCameraState(genericState)
	}
}

// mapCameraStateToWorkflowState maps camera state to workflow state
func mapCameraStateToWorkflowState(cameraState statemngtypes.CameraState) CameraWorkflowState {
	switch cameraState {
	case statemngtypes.CameraStateSynced:
		return CameraWorkflowStateSynced
	case statemngtypes.CameraStateWaitingForScreenshots:
		return CameraWorkflowStateWaitingForScreenshots
	case statemngtypes.CameraStateScreenshotSetReady:
		return CameraWorkflowStateScreenshotSetReady
	case statemngtypes.CameraStateModelDeployed:
		return CameraWorkflowStateModelDeployed
	case statemngtypes.CameraStateFrameProcessing:
		return CameraWorkflowStateFrameProcessing
	default:
		return "" // No workflow state mapping
	}
}

// mapGenericStateToCameraState maps generic device state to camera state
func mapGenericStateToCameraState(genericState DeviceState) statemngtypes.CameraState {
	switch genericState {
	case DeviceStateUndiscovered:
		return statemngtypes.CameraStateUndiscovered
	case DeviceStateDiscovered:
		return statemngtypes.CameraStateDiscovered
	case DeviceStateDisconnected:
		return statemngtypes.CameraStateDisconnected
	case DeviceStateError:
		return statemngtypes.CameraStateError
	default:
		return statemngtypes.CameraStateUndiscovered
	}
}

// isValidCameraWorkflowTransition checks if a workflow state transition is valid
func isValidCameraWorkflowTransition(from, to CameraWorkflowState) bool {
	// Define valid workflow transitions
	validTransitions := map[CameraWorkflowState][]CameraWorkflowState{
		CameraWorkflowStateSynced: {
			CameraWorkflowStateWaitingForScreenshots,
			CameraWorkflowStateModelDeployed, // Can skip screenshots if model exists
		},
		CameraWorkflowStateWaitingForScreenshots: {
			CameraWorkflowStateScreenshotSetReady,
		},
		CameraWorkflowStateScreenshotSetReady: {
			CameraWorkflowStateModelDeployed,
		},
		CameraWorkflowStateModelDeployed: {
			CameraWorkflowStateFrameProcessing,
		},
		CameraWorkflowStateFrameProcessing: {
			CameraWorkflowStateModelDeployed, // Can stop processing
		},
	}

	// Same state is always valid
	if from == to {
		return true
	}

	validStates, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, validState := range validStates {
		if validState == to {
			return true
		}
	}

	return false
}

