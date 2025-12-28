package adapters

import (
	"fmt"

	"go.uber.org/zap"

	statemngtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// CameraStateMachineAdapter adapts a DeviceStateMachine to the CameraStateMachine interface.
// This allows state-mng to use the iot device state machine while maintaining
// backward compatibility with the CameraStateMachine interface.
//
// NOTE: This adapter creates a bridge to the state-mng service. The external dependency
// (state-mng/types) is isolated to this adapters package to prevent coupling the root
// iot package to state-mng.
type CameraStateMachineAdapter struct {
	deviceSM      types.DeviceStateMachine
	cameraAdapter *CameraStateAdapter
	logger        *zap.Logger
}

// NewCameraStateMachineAdapter creates a new adapter that wraps a DeviceStateMachine
// and implements the CameraStateMachine interface.
// If logger is nil, a no-op logger will be used.
func NewCameraStateMachineAdapter(deviceSM types.DeviceStateMachine, logger *zap.Logger) *CameraStateMachineAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CameraStateMachineAdapter{
		deviceSM:      deviceSM,
		cameraAdapter: NewCameraStateAdapter(deviceSM, logger),
		logger:        logger,
	}
}

// GetCameraID returns the camera ID this state machine is for.
func (a *CameraStateMachineAdapter) GetCameraID() string {
	return a.deviceSM.GetDeviceID()
}

// GetState returns the current camera state.
func (a *CameraStateMachineAdapter) GetState() statemngtypes.CameraState {
	workflowState := a.cameraAdapter.GetCameraWorkflowState()
	genericState := a.deviceSM.GetState()
	return mapWorkflowStateToCameraState(workflowState, genericState)
}

// GetStateInfo returns detailed camera state information.
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

// Transition transitions to a new camera state.
func (a *CameraStateMachineAdapter) Transition(newState statemngtypes.CameraState, errorMsg string) error {
	workflowState := mapCameraStateToWorkflowState(newState)
	if workflowState == "" {
		// For states that don't map to workflow states, use generic device state transitions
		return a.transitionGenericState(newState, errorMsg)
	}

	if err := a.cameraAdapter.TransitionToCameraWorkflowState(workflowState, errorMsg); err != nil {
		a.logger.Error("Failed to transition camera workflow state",
			zap.String("device_id", a.deviceSM.GetDeviceID()),
			zap.String("camera_state", string(newState)),
			zap.String("workflow_state", string(workflowState)),
			zap.Error(err),
		)
		return fmt.Errorf("failed to transition to workflow state %s: %w", workflowState, err)
	}

	a.logger.Info("Camera state transitioned",
		zap.String("device_id", a.deviceSM.GetDeviceID()),
		zap.String("camera_state", string(newState)),
		zap.String("workflow_state", string(workflowState)),
	)

	return nil
}

// CanTransition checks if a transition from current state to new state is valid.
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

// IsOperational returns true if camera is in an operational state.
func (a *CameraStateMachineAdapter) IsOperational() bool {
	return a.cameraAdapter.IsOperational()
}

// IsReadyForProcessing returns true if camera is ready to process frames.
func (a *CameraStateMachineAdapter) IsReadyForProcessing() bool {
	return a.cameraAdapter.IsReadyForProcessing()
}

// SetModelID sets the model ID (delegates to camera adapter).
func (a *CameraStateMachineAdapter) SetModelID(modelID string) {
	a.cameraAdapter.SetModelID(modelID)
}

// SetDatasetID sets the dataset ID (delegates to camera adapter).
func (a *CameraStateMachineAdapter) SetDatasetID(datasetID string) {
	a.cameraAdapter.SetDatasetID(datasetID)
}

// transitionGenericState handles transitions for states that map to generic device states.
func (a *CameraStateMachineAdapter) transitionGenericState(newState statemngtypes.CameraState, errorMsg string) error {
	var targetDeviceState types.DeviceState
	switch newState {
	case statemngtypes.CameraStateUndiscovered:
		targetDeviceState = types.DeviceStateUndiscovered
	case statemngtypes.CameraStateDiscovered:
		targetDeviceState = types.DeviceStateDiscovered
	case statemngtypes.CameraStateDisconnected:
		targetDeviceState = types.DeviceStateDisconnected
		// Clear workflow state metadata when transitioning to disconnected
		a.deviceSM.SetMetadata("camera_workflow_state", nil)
	case statemngtypes.CameraStateError:
		targetDeviceState = types.DeviceStateError
		// Clear workflow state metadata when transitioning to error
		a.deviceSM.SetMetadata("camera_workflow_state", nil)
	default:
		a.logger.Warn("Cannot transition to camera state directly",
			zap.String("device_id", a.deviceSM.GetDeviceID()),
			zap.String("camera_state", string(newState)),
		)
		return fmt.Errorf("cannot transition to camera state %s directly (use workflow states)", newState)
	}

	if err := a.deviceSM.Transition(targetDeviceState, errorMsg); err != nil {
		a.logger.Error("Failed to transition generic state",
			zap.String("device_id", a.deviceSM.GetDeviceID()),
			zap.String("camera_state", string(newState)),
			zap.String("target_device_state", string(targetDeviceState)),
			zap.Error(err),
		)
		return fmt.Errorf("failed to transition to device state %s: %w", targetDeviceState, err)
	}

	return nil
}

// canTransitionGenericState checks if a generic state transition is valid.
func (a *CameraStateMachineAdapter) canTransitionGenericState(newState statemngtypes.CameraState) bool {
	var targetDeviceState types.DeviceState
	switch newState {
	case statemngtypes.CameraStateUndiscovered:
		targetDeviceState = types.DeviceStateUndiscovered
	case statemngtypes.CameraStateDiscovered:
		targetDeviceState = types.DeviceStateDiscovered
	case statemngtypes.CameraStateDisconnected:
		targetDeviceState = types.DeviceStateDisconnected
	case statemngtypes.CameraStateError:
		targetDeviceState = types.DeviceStateError
	default:
		return false
	}
	return a.deviceSM.CanTransition(targetDeviceState)
}

// mapWorkflowStateToCameraState maps camera workflow state to camera state.
func mapWorkflowStateToCameraState(workflowState CameraWorkflowState, genericState types.DeviceState) statemngtypes.CameraState {
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

// mapCameraStateToWorkflowState maps camera state to workflow state.
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

// mapGenericStateToCameraState maps generic device state to camera state.
func mapGenericStateToCameraState(genericState types.DeviceState) statemngtypes.CameraState {
	switch genericState {
	case types.DeviceStateUndiscovered:
		return statemngtypes.CameraStateUndiscovered
	case types.DeviceStateDiscovered:
		return statemngtypes.CameraStateDiscovered
	case types.DeviceStateDisconnected:
		return statemngtypes.CameraStateDisconnected
	case types.DeviceStateError:
		return statemngtypes.CameraStateError
	default:
		return statemngtypes.CameraStateUndiscovered
	}
}

// isValidCameraWorkflowTransition checks if a workflow state transition is valid.
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

