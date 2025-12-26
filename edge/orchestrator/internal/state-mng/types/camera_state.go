package types

import "time"

// CameraState represents the state of an individual camera.
// Each camera has its own independent state machine, allowing cameras to operate
// independently of each other and the connection state.
type CameraState string

const (
	// CameraStateUndiscovered indicates the camera has not been discovered yet
	CameraStateUndiscovered CameraState = "undiscovered"

	// CameraStateDiscovered indicates the camera has been discovered but not yet synced with VM
	CameraStateDiscovered CameraState = "discovered"

	// CameraStateSynced indicates the camera has been synced with the VM
	CameraStateSynced CameraState = "synced"

	// CameraStateWaitingForScreenshots indicates waiting for user to provide labeled screenshots
	CameraStateWaitingForScreenshots CameraState = "waiting_for_screenshots"

	// CameraStateScreenshotSetReady indicates labeled screenshots are ready for training
	CameraStateScreenshotSetReady CameraState = "screenshot_set_ready"

	// CameraStateModelDeployed indicates AI model has been deployed for this camera
	CameraStateModelDeployed CameraState = "model_deployed"

	// CameraStateFrameProcessing indicates camera is actively processing frames
	CameraStateFrameProcessing CameraState = "frame_processing"

	// CameraStateError indicates a camera-specific error occurred
	CameraStateError CameraState = "error"

	// CameraStateDisconnected indicates camera connection was lost
	CameraStateDisconnected CameraState = "disconnected"
)

// CameraStateInfo contains metadata about the camera state
type CameraStateInfo struct {
	CameraID     string      `json:"camera_id"`
	State        CameraState `json:"state"`
	LastUpdated  time.Time   `json:"last_updated"`
	Error        string      `json:"error,omitempty"`      // Error message if state is error
	ModelID      string      `json:"model_id,omitempty"`   // Deployed model ID (if model_deployed or frame_processing)
	DatasetID    string      `json:"dataset_id,omitempty"` // Dataset ID (if screenshot_set_ready or later)
	IsProcessing bool        `json:"is_processing"`        // Whether camera is actively processing frames
}

// CameraStateMachine defines the state machine for per-camera states
// Valid state transitions:
//
//	undiscovered -> discovered -> synced -> waiting_for_screenshots -> screenshot_set_ready -> model_deployed -> frame_processing
//	                                                                                                    |
//	                                                                                                    v
//	                                                                                              error (on failure)
//
//	Any state -> disconnected (on camera disconnection)
//	Any state -> error (on camera-specific error)
//
// Error states can transition to:
//   - error -> synced (retry) or discovered (reset)
//   - disconnected -> discovered (reconnect)
type CameraStateMachine interface {
	// GetCameraID returns the camera ID this state machine is for
	GetCameraID() string

	// GetState returns the current camera state
	GetState() CameraState

	// GetStateInfo returns detailed camera state information
	GetStateInfo() CameraStateInfo

	// Transition transitions to a new camera state
	// Returns error if transition is invalid
	Transition(newState CameraState, errorMsg string) error

	// CanTransition checks if a transition from current state to new state is valid
	CanTransition(newState CameraState) bool

	// IsOperational returns true if camera is in an operational state (model_deployed or frame_processing)
	IsOperational() bool

	// IsReadyForProcessing returns true if camera is ready to process frames (model_deployed or frame_processing)
	IsReadyForProcessing() bool
}

// CameraStateMachineWithMetadata extends CameraStateMachine with metadata setters
// This interface is implemented by iot.CameraStateMachineAdapter
type CameraStateMachineWithMetadata interface {
	CameraStateMachine
	
	// SetModelID sets the model ID for the camera (used when model is deployed)
	SetModelID(modelID string)
	
	// SetDatasetID sets the dataset ID for the camera (used when dataset is ready)
	SetDatasetID(datasetID string)
}
