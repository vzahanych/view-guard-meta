package adapters

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// CameraWorkflowState represents camera-specific workflow states.
// These are stored in device state metadata, not as primary states.
type CameraWorkflowState string

const (
	// CameraWorkflowStateSynced indicates the camera is synced with the system
	CameraWorkflowStateSynced CameraWorkflowState = "synced"
	// CameraWorkflowStateWaitingForScreenshots indicates waiting for screenshots to be captured
	CameraWorkflowStateWaitingForScreenshots CameraWorkflowState = "waiting_for_screenshots"
	// CameraWorkflowStateScreenshotSetReady indicates screenshots are ready
	CameraWorkflowStateScreenshotSetReady CameraWorkflowState = "screenshot_set_ready"
	// CameraWorkflowStateModelDeployed indicates the ML model is deployed
	CameraWorkflowStateModelDeployed CameraWorkflowState = "model_deployed"
	// CameraWorkflowStateFrameProcessing indicates frames are being processed
	CameraWorkflowStateFrameProcessing CameraWorkflowState = "frame_processing"
)

// CameraStateAdapter adapts camera-specific states to generic device states.
// This allows cameras to use the generic DeviceStateMachine while maintaining
// camera-specific workflow states in metadata.
type CameraStateAdapter struct {
	deviceSM types.DeviceStateMachine
	logger   *zap.Logger
}

// NewCameraStateAdapter creates a new camera state adapter.
// If logger is nil, a no-op logger will be used.
func NewCameraStateAdapter(deviceSM types.DeviceStateMachine, logger *zap.Logger) *CameraStateAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CameraStateAdapter{
		deviceSM: deviceSM,
		logger:   logger,
	}
}

// GetCameraWorkflowState returns the camera-specific workflow state from metadata.
func (a *CameraStateAdapter) GetCameraWorkflowState() CameraWorkflowState {
	workflowState, exists := a.deviceSM.GetMetadata("camera_workflow_state")
	if !exists || workflowState == nil {
		// Map generic state to camera workflow state
		genericState := a.deviceSM.GetState()
		switch genericState {
		case types.DeviceStateRegistered:
			return CameraWorkflowStateSynced
		case types.DeviceStateActive:
			// Check if we have more specific workflow state in metadata
			if ws, ok := a.deviceSM.GetMetadata("camera_workflow_state"); ok && ws != nil {
				if wsStr, ok := ws.(string); ok {
					return CameraWorkflowState(wsStr)
				}
			}
			return CameraWorkflowStateWaitingForScreenshots
		case types.DeviceStateProcessing:
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
	case types.DeviceStateDisconnected, types.DeviceStateError, types.DeviceStateUndiscovered, types.DeviceStateDiscovered:
		return "" // No workflow state for these generic states
	default:
		return CameraWorkflowStateSynced
	}
}

// SetCameraWorkflowState sets the camera-specific workflow state in metadata.
func (a *CameraStateAdapter) SetCameraWorkflowState(workflowState CameraWorkflowState) {
	a.deviceSM.SetMetadata("camera_workflow_state", string(workflowState))

	// Also update generic state based on workflow state
	switch workflowState {
	case CameraWorkflowStateSynced:
		_ = a.deviceSM.Transition(types.DeviceStateRegistered, "")
	case CameraWorkflowStateWaitingForScreenshots, CameraWorkflowStateScreenshotSetReady:
		_ = a.deviceSM.Transition(types.DeviceStateActive, "")
	case CameraWorkflowStateModelDeployed, CameraWorkflowStateFrameProcessing:
		_ = a.deviceSM.Transition(types.DeviceStateProcessing, "")
	}

	a.logger.Debug("Camera workflow state updated",
		zap.String("device_id", a.deviceSM.GetDeviceID()),
		zap.String("workflow_state", string(workflowState)),
		zap.String("generic_state", string(a.deviceSM.GetState())),
	)
}

// TransitionToCameraWorkflowState transitions to a camera-specific workflow state.
func (a *CameraStateAdapter) TransitionToCameraWorkflowState(workflowState CameraWorkflowState, errorMsg string) error {
	// Set workflow state in metadata
	a.SetCameraWorkflowState(workflowState)

	// Transition generic state if needed
	var targetGenericState types.DeviceState
	switch workflowState {
	case CameraWorkflowStateSynced:
		targetGenericState = types.DeviceStateRegistered
	case CameraWorkflowStateWaitingForScreenshots, CameraWorkflowStateScreenshotSetReady:
		targetGenericState = types.DeviceStateActive
	case CameraWorkflowStateModelDeployed, CameraWorkflowStateFrameProcessing:
		targetGenericState = types.DeviceStateProcessing
	default:
		a.logger.Error("Unknown camera workflow state",
			zap.String("device_id", a.deviceSM.GetDeviceID()),
			zap.String("workflow_state", string(workflowState)),
		)
		return fmt.Errorf("unknown camera workflow state: %s", workflowState)
	}

	if err := a.deviceSM.Transition(targetGenericState, errorMsg); err != nil {
		a.logger.Error("Failed to transition generic state",
			zap.String("device_id", a.deviceSM.GetDeviceID()),
			zap.String("workflow_state", string(workflowState)),
			zap.String("target_generic_state", string(targetGenericState)),
			zap.Error(err),
		)
		return fmt.Errorf("failed to transition to generic state %s: %w", targetGenericState, err)
	}

	a.logger.Info("Camera workflow state transitioned",
		zap.String("device_id", a.deviceSM.GetDeviceID()),
		zap.String("workflow_state", string(workflowState)),
		zap.String("generic_state", string(targetGenericState)),
	)

	return nil
}

// SetModelID sets the model ID in metadata.
func (a *CameraStateAdapter) SetModelID(modelID string) {
	a.deviceSM.SetMetadata("model_id", modelID)
	a.logger.Debug("Model ID set",
		zap.String("device_id", a.deviceSM.GetDeviceID()),
		zap.String("model_id", modelID),
	)
}

// GetModelID retrieves the model ID from metadata.
func (a *CameraStateAdapter) GetModelID() string {
	if modelID, exists := a.deviceSM.GetMetadata("model_id"); exists {
		if modelIDStr, ok := modelID.(string); ok {
			return modelIDStr
		}
	}
	return ""
}

// SetDatasetID sets the dataset ID in metadata.
func (a *CameraStateAdapter) SetDatasetID(datasetID string) {
	a.deviceSM.SetMetadata("dataset_id", datasetID)
	a.logger.Debug("Dataset ID set",
		zap.String("device_id", a.deviceSM.GetDeviceID()),
		zap.String("dataset_id", datasetID),
	)
}

// GetDatasetID retrieves the dataset ID from metadata.
func (a *CameraStateAdapter) GetDatasetID() string {
	if datasetID, exists := a.deviceSM.GetMetadata("dataset_id"); exists {
		if datasetIDStr, ok := datasetID.(string); ok {
			return datasetIDStr
		}
	}
	return ""
}

// IsOperational returns true if camera is in an operational state.
func (a *CameraStateAdapter) IsOperational() bool {
	workflowState := a.GetCameraWorkflowState()
	return workflowState == CameraWorkflowStateModelDeployed ||
		workflowState == CameraWorkflowStateFrameProcessing
}

// IsReadyForProcessing returns true if camera is ready to process frames.
func (a *CameraStateAdapter) IsReadyForProcessing() bool {
	workflowState := a.GetCameraWorkflowState()
	return workflowState == CameraWorkflowStateModelDeployed ||
		workflowState == CameraWorkflowStateFrameProcessing
}

// GetDeviceStateMachine returns the underlying device state machine.
func (a *CameraStateAdapter) GetDeviceStateMachine() types.DeviceStateMachine {
	return a.deviceSM
}

// CameraStateInfo provides camera-specific state information.
type CameraStateInfo struct {
	CameraID     string              `json:"camera_id"`
	State        CameraWorkflowState `json:"state"`         // Camera-specific workflow state
	GenericState types.DeviceState   `json:"generic_state"` // Generic device state
	LastUpdated  time.Time           `json:"last_updated"`
	Error        string              `json:"error,omitempty"`
	ModelID      string              `json:"model_id,omitempty"`
	DatasetID    string              `json:"dataset_id,omitempty"`
	IsProcessing bool               `json:"is_processing"`
}

// GetCameraStateInfo returns camera-specific state information.
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

