package types

import "time"

// MLLifecycleState represents the ML lifecycle state of a device.
// This enum defines all possible states in the ML lifecycle workflow.
type MLLifecycleState string

const (
	// MLLifecycleStateUnassigned indicates the device exists locally but VM has not assigned it to this Edge
	MLLifecycleStateUnassigned MLLifecycleState = "Unassigned"

	// MLLifecycleStateAssigned indicates VM assigned device to Edge; Edge must fulfill dataset/model workflow
	MLLifecycleStateAssigned MLLifecycleState = "Assigned"

	// MLLifecycleStateAwaitingDataset indicates VM requested dataset; Edge is collecting labeled units
	MLLifecycleStateAwaitingDataset MLLifecycleState = "AwaitingDataset"

	// MLLifecycleStateDatasetReadyLocal indicates dataset exists locally and passed validation
	MLLifecycleStateDatasetReadyLocal MLLifecycleState = "DatasetReadyLocal"

	// MLLifecycleStateDatasetUploadInProgress indicates dataset upload is in progress
	MLLifecycleStateDatasetUploadInProgress MLLifecycleState = "DatasetUploadInProgress"

	// MLLifecycleStateDatasetUploaded indicates dataset has been uploaded to VM
	MLLifecycleStateDatasetUploaded MLLifecycleState = "DatasetUploaded"

	// MLLifecycleStateTrainingPending indicates VM acknowledged dataset and queued training (optional if VM is async)
	MLLifecycleStateTrainingPending MLLifecycleState = "TrainingPending"

	// MLLifecycleStateModelAvailable indicates VM produced a model and signaled availability; Edge may need to fetch
	MLLifecycleStateModelAvailable MLLifecycleState = "ModelAvailable"

	// MLLifecycleStateModelStored indicates model is stored in object/meta storage and verified
	MLLifecycleStateModelStored MLLifecycleState = "ModelStored"

	// MLLifecycleStateInferenceActive indicates inference loop is running for this device
	MLLifecycleStateInferenceActive MLLifecycleState = "InferenceActive"

	// MLLifecycleStateDegradedNoModel indicates device is active but no valid model is available (Edge cannot detect)
	MLLifecycleStateDegradedNoModel MLLifecycleState = "DegradedNoModel"

	// MLLifecycleStateRecoveryRequired indicates local storage integrity prevents safe operation; VM-assisted resync required
	MLLifecycleStateRecoveryRequired MLLifecycleState = "RecoveryRequired"
)

// String returns the string representation of MLLifecycleState
func (s MLLifecycleState) String() string {
	return string(s)
}

// IsValid checks if the state is a valid ML lifecycle state
func (s MLLifecycleState) IsValid() bool {
	switch s {
	case MLLifecycleStateUnassigned,
		MLLifecycleStateAssigned,
		MLLifecycleStateAwaitingDataset,
		MLLifecycleStateDatasetReadyLocal,
		MLLifecycleStateDatasetUploadInProgress,
		MLLifecycleStateDatasetUploaded,
		MLLifecycleStateTrainingPending,
		MLLifecycleStateModelAvailable,
		MLLifecycleStateModelStored,
		MLLifecycleStateInferenceActive,
		MLLifecycleStateDegradedNoModel,
		MLLifecycleStateRecoveryRequired:
		return true
	default:
		return false
	}
}

// MLLifecycleStateInfo represents the complete ML lifecycle state information for a device.
// This struct is persisted in meta storage and includes all state metadata required for
// workflow orchestration, recovery, and CAS (Compare-And-Swap) operations.
type MLLifecycleStateInfo struct {
	// DeviceID is the device identifier
	DeviceID DeviceID

	// State is the current ML lifecycle state
	State MLLifecycleState

	// LastUpdated is when the state was last updated
	LastUpdated time.Time

	// Error contains any error message associated with the current state
	Error string

	// ModelID is the identifier of the model associated with this device (if any)
	ModelID string

	// ModelVersion is the version of the model (if any)
	ModelVersion string

	// DatasetID is the identifier of the dataset associated with this device (if any)
	DatasetID string

	// OfflineInferenceAllowed is a policy flag indicating whether offline inference is allowed
	// This is set by VM in assignment response and controls whether inference continues
	// when VM connection is lost
	OfflineInferenceAllowed bool

	// LastKnownGoodState is the last known good state before entering error/recovery states
	// This is used for recovery and rollback operations
	LastKnownGoodState MLLifecycleState

	// Version is the version number for CAS (Compare-And-Swap) operations
	// This must be incremented on every update to enable atomic state transitions
	Version int

	// CreatedAt is when this ML lifecycle state record was created
	CreatedAt time.Time
}

// MLLifecycleFilters contains filters for querying ML lifecycle states
type MLLifecycleFilters struct {
	// DeviceID filters by specific device ID
	DeviceID *DeviceID

	// State filters by specific ML lifecycle state
	State *MLLifecycleState

	// States filters by multiple ML lifecycle states (OR condition)
	States []MLLifecycleState

	// HasModel filters for devices that have a model (ModelID != "")
	HasModel *bool

	// HasDataset filters for devices that have a dataset (DatasetID != "")
	HasDataset *bool

	// OfflineInferenceAllowed filters by offline inference policy
	OfflineInferenceAllowed *bool
}

// PendingModelDeployment represents a pending model deployment request.
// This is stored temporarily in the pending_model_deployments bucket and cleaned up after TTL expires.
type PendingModelDeployment struct {
	// DeviceID is the device identifier for this deployment
	DeviceID DeviceID

	// ModelID is the model identifier to deploy
	ModelID string

	// EventData contains the deployment event data (from VM)
	EventData map[string]interface{}

	// ReceivedAt is when the deployment request was received
	ReceivedAt time.Time

	// TTL is the time-to-live for this deployment request (default: 24 hours)
	TTL time.Duration
}

