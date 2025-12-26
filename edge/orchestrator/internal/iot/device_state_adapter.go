package iot

import (
	"fmt"
	"time"
)

// CameraStateAdapter adapts camera-specific states to generic device states
// This allows cameras to use the generic DeviceStateMachine while maintaining
// camera-specific workflow states in metadata
type CameraStateAdapter struct {
	deviceSM DeviceStateMachine
}

// NewCameraStateAdapter creates a new camera state adapter
func NewCameraStateAdapter(deviceSM DeviceStateMachine) *CameraStateAdapter {
	return &CameraStateAdapter{
		deviceSM: deviceSM,
	}
}

// CameraWorkflowState represents camera-specific workflow states
// These are stored in device state metadata, not as primary states
type CameraWorkflowState string

const (
	CameraWorkflowStateSynced              CameraWorkflowState = "synced"
	CameraWorkflowStateWaitingForScreenshots CameraWorkflowState = "waiting_for_screenshots"
	CameraWorkflowStateScreenshotSetReady   CameraWorkflowState = "screenshot_set_ready"
	CameraWorkflowStateModelDeployed        CameraWorkflowState = "model_deployed"
	CameraWorkflowStateFrameProcessing      CameraWorkflowState = "frame_processing"
)

// GetCameraWorkflowState returns the camera-specific workflow state from metadata
func (a *CameraStateAdapter) GetCameraWorkflowState() CameraWorkflowState {
	workflowState, exists := a.deviceSM.GetMetadata("camera_workflow_state")
	if !exists || workflowState == nil {
		// Map generic state to camera workflow state
		genericState := a.deviceSM.GetState()
		switch genericState {
		case DeviceStateRegistered:
			return CameraWorkflowStateSynced
		case DeviceStateActive:
			// Check if we have more specific workflow state in metadata
			if ws, ok := a.deviceSM.GetMetadata("camera_workflow_state"); ok && ws != nil {
				if wsStr, ok := ws.(string); ok {
					return CameraWorkflowState(wsStr)
				}
			}
			return CameraWorkflowStateWaitingForScreenshots
		case DeviceStateProcessing:
			// Check metadata for specific processing state
			if ws, ok := a.deviceSM.GetMetadata("camera_workflow_state"); ok && ws != nil {
				if wsStr, ok := ws.(string); ok {
					return CameraWorkflowState(wsStr)
				}
			}
			return CameraWorkflowStateFrameProcessing
		default:
			// For disconnected, error, undiscovered, discovered states, don't return a workflow state
			return ""
		}
	}

	if wsStr, ok := workflowState.(string); ok && wsStr != "" {
		return CameraWorkflowState(wsStr)
	}
	// If workflow state is nil or empty, fall back to generic state mapping
	genericState := a.deviceSM.GetState()
	switch genericState {
	case DeviceStateDisconnected, DeviceStateError, DeviceStateUndiscovered, DeviceStateDiscovered:
		return "" // No workflow state for these generic states
	default:
		return CameraWorkflowStateSynced
	}
}

// SetCameraWorkflowState sets the camera-specific workflow state in metadata
func (a *CameraStateAdapter) SetCameraWorkflowState(workflowState CameraWorkflowState) {
	a.deviceSM.SetMetadata("camera_workflow_state", string(workflowState))

	// Also update generic state based on workflow state
	switch workflowState {
	case CameraWorkflowStateSynced:
		_ = a.deviceSM.Transition(DeviceStateRegistered, "")
	case CameraWorkflowStateWaitingForScreenshots, CameraWorkflowStateScreenshotSetReady:
		_ = a.deviceSM.Transition(DeviceStateActive, "")
	case CameraWorkflowStateModelDeployed, CameraWorkflowStateFrameProcessing:
		_ = a.deviceSM.Transition(DeviceStateProcessing, "")
	}
}

// TransitionToCameraWorkflowState transitions to a camera-specific workflow state
func (a *CameraStateAdapter) TransitionToCameraWorkflowState(workflowState CameraWorkflowState, errorMsg string) error {
	// Set workflow state in metadata
	a.SetCameraWorkflowState(workflowState)

	// Transition generic state if needed
	var targetGenericState DeviceState
	switch workflowState {
	case CameraWorkflowStateSynced:
		targetGenericState = DeviceStateRegistered
	case CameraWorkflowStateWaitingForScreenshots, CameraWorkflowStateScreenshotSetReady:
		targetGenericState = DeviceStateActive
	case CameraWorkflowStateModelDeployed, CameraWorkflowStateFrameProcessing:
		targetGenericState = DeviceStateProcessing
	default:
		return fmt.Errorf("unknown camera workflow state: %s", workflowState)
	}

	return a.deviceSM.Transition(targetGenericState, errorMsg)
}

// SetModelID sets the model ID in metadata
func (a *CameraStateAdapter) SetModelID(modelID string) {
	a.deviceSM.SetMetadata("model_id", modelID)
}

// GetModelID retrieves the model ID from metadata
func (a *CameraStateAdapter) GetModelID() string {
	if modelID, exists := a.deviceSM.GetMetadata("model_id"); exists {
		if modelIDStr, ok := modelID.(string); ok {
			return modelIDStr
		}
	}
	return ""
}

// SetDatasetID sets the dataset ID in metadata
func (a *CameraStateAdapter) SetDatasetID(datasetID string) {
	a.deviceSM.SetMetadata("dataset_id", datasetID)
}

// GetDatasetID retrieves the dataset ID from metadata
func (a *CameraStateAdapter) GetDatasetID() string {
	if datasetID, exists := a.deviceSM.GetMetadata("dataset_id"); exists {
		if datasetIDStr, ok := datasetID.(string); ok {
			return datasetIDStr
		}
	}
	return ""
}

// IsOperational returns true if camera is in an operational state
func (a *CameraStateAdapter) IsOperational() bool {
	workflowState := a.GetCameraWorkflowState()
	return workflowState == CameraWorkflowStateModelDeployed ||
		workflowState == CameraWorkflowStateFrameProcessing
}

// IsReadyForProcessing returns true if camera is ready to process frames
func (a *CameraStateAdapter) IsReadyForProcessing() bool {
	workflowState := a.GetCameraWorkflowState()
	return workflowState == CameraWorkflowStateModelDeployed ||
		workflowState == CameraWorkflowStateFrameProcessing
}

// GetDeviceStateMachine returns the underlying device state machine
func (a *CameraStateAdapter) GetDeviceStateMachine() DeviceStateMachine {
	return a.deviceSM
}

// CameraStateInfo provides camera-specific state information
type CameraStateInfo struct {
	CameraID     string              `json:"camera_id"`
	State        CameraWorkflowState `json:"state"` // Camera-specific workflow state
	GenericState DeviceState         `json:"generic_state"` // Generic device state
	LastUpdated  time.Time           `json:"last_updated"`
	Error        string              `json:"error,omitempty"`
	ModelID      string              `json:"model_id,omitempty"`
	DatasetID    string              `json:"dataset_id,omitempty"`
	IsProcessing bool                `json:"is_processing"`
}

// GetCameraStateInfo returns camera-specific state information
func (a *CameraStateAdapter) GetCameraStateInfo() CameraStateInfo {
	deviceInfo := a.deviceSM.GetStateInfo()
	workflowState := a.GetCameraWorkflowState()

	return CameraStateInfo{
		CameraID:     deviceInfo.DeviceID,
		State:        workflowState,
		GenericState: deviceInfo.State,
		LastUpdated:  deviceInfo.LastUpdated,
		Error:        deviceInfo.Error,
		ModelID:      a.GetModelID(),
		DatasetID:    a.GetDatasetID(),
		IsProcessing: workflowState == CameraWorkflowStateFrameProcessing,
	}
}

