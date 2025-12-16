package metastorage

import (
	"time"

	"github.com/google/uuid"
)

// EdgeState represents VM-side view of an edge node.
type EdgeState struct {
	UUID        uuid.UUID         `json:"uuid"`
	WGPublicKey string            `json:"wg_public_key"`
	Status      EdgeStatus        `json:"status"`
	Devices     []IoTDevice       `json:"devices,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// EdgeStatus defines allowed lifecycle states for an edge from VM perspective.
type EdgeStatus string

const (
	EdgeStatusRegistered            EdgeStatus = "registered"               // Admin has added edge UUID + WG key
	EdgeStatusConnected             EdgeStatus = "wireguard_connected"      // WG up
	EdgeStatusHTTPSConnected        EdgeStatus = "https_connected"          // HTTPS up
	EdgeStatusAuthenticated         EdgeStatus = "authenticated"            // Authenticated
	EdgeStatusIOTSynced             EdgeStatus = "iot_synced"               // IoT inventory/capabilities synced
	EdgeStatusIOTTrainDataRequested EdgeStatus = "iot_train_data_requested" // Ready for work (tasks can be assigned)
	EdgeStatusIOTTrainDataSynced    EdgeStatus = "iot_train_data_synced"    // Ready for work (tasks can be assigned)
	EdgeStatusIOTTrainModelTrained  EdgeStatus = "iot_train_model_trained"  // Model trained
	EdgeStatusIOTTrainModelDeployed EdgeStatus = "iot_train_model_deployed" // Model deployed
	EdgeStatusError                 EdgeStatus = "error"                    // Error state; requires attention
)

// IoTType represents supported IoT device types.
type IoTType string

const (
	IoTTypeCCTV IoTType = "cctv"
)

// IoTDevice describes an IoT endpoint attached to an edge.
type IoTDevice struct {
	UUID     uuid.UUID         `json:"uuid"`
	Type     IoTType           `json:"type"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RawModelMetadata describes metadata for a raw (baseline) model that is not bound to a specific Edge.
type RawModelMetadata struct {
	ID        uuid.UUID         `json:"id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// TrainedModelMetadata describes metadata for a trained model produced for a specific IoT device on an Edge.
type TrainedModelMetadata struct {
	ID            uuid.UUID         `json:"id"`
	EdgeID        uuid.UUID         `json:"edge_id"`
	DeviceID      uuid.UUID         `json:"device_id"`
	SourceModelID uuid.UUID         `json:"source_model_id"` // Raw/baseline model this was trained from
	DatasetID     uuid.UUID         `json:"dataset_id"`      // Training dataset used
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// EdgeEventMetadata describes metadata for an event reported by an Edge.
type EdgeEventMetadata struct {
	ID        uuid.UUID         `json:"id"`
	EdgeID    uuid.UUID         `json:"edge_id"`
	DeviceID  uuid.UUID         `json:"device_id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// ClipMetadata describes metadata for a video clip or snapshot stored for an event.
type ClipMetadata struct {
	ID        uuid.UUID         `json:"id"`
	EdgeID    uuid.UUID         `json:"edge_id"`
	EventID   uuid.UUID         `json:"event_id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// TrainingDatasetMetadata describes metadata for IoT device training datasets.
type TrainingDatasetMetadata struct {
	ID        uuid.UUID         `json:"id"`
	EdgeID    uuid.UUID         `json:"edge_id"`
	DeviceID  uuid.UUID         `json:"device_id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// MetaDataStore defines KV-style operations for edge states and related metadata.
type MetaDataStore interface {
	// Edge lifecycle and metadata
	RegisterEdge(id uuid.UUID, state EdgeState) error
	UnregisterEdge(id uuid.UUID) error
	GetEdge(id uuid.UUID) (EdgeState, bool)
	UpdateEdge(id uuid.UUID, updateFn func(EdgeState) EdgeState) (EdgeState, error)

	// Raw model metadata
	RegisterRawModel(id uuid.UUID, meta RawModelMetadata) error
	UnregisterRawModel(id uuid.UUID) error
	GetRawModel(id uuid.UUID) (RawModelMetadata, bool)
	UpdateRawModel(id uuid.UUID, updateFn func(RawModelMetadata) RawModelMetadata) (RawModelMetadata, error)

	// Trained model metadata
	RegisterTrainedModel(id uuid.UUID, meta TrainedModelMetadata) error
	UnregisterTrainedModel(id uuid.UUID) error
	GetTrainedModel(id uuid.UUID) (TrainedModelMetadata, bool)
	UpdateTrainedModel(id uuid.UUID, updateFn func(TrainedModelMetadata) TrainedModelMetadata) (TrainedModelMetadata, error)

	// Edge events metadata
	RegisterEdgeEvent(id uuid.UUID, meta EdgeEventMetadata) error
	UnregisterEdgeEvent(id uuid.UUID) error
	GetEdgeEvent(id uuid.UUID) (EdgeEventMetadata, bool)
	UpdateEdgeEvent(id uuid.UUID, updateFn func(EdgeEventMetadata) EdgeEventMetadata) (EdgeEventMetadata, error)

	// Clips metadata
	RegisterClip(id uuid.UUID, meta ClipMetadata) error
	UnregisterClip(id uuid.UUID) error
	GetClip(id uuid.UUID) (ClipMetadata, bool)
	UpdateClip(id uuid.UUID, updateFn func(ClipMetadata) ClipMetadata) (ClipMetadata, error)

	// IoT device training datasets metadata
	RegisterTrainingDataset(id uuid.UUID, meta TrainingDatasetMetadata) error
	UnregisterTrainingDataset(id uuid.UUID) error
	GetTrainingDataset(id uuid.UUID) (TrainingDatasetMetadata, bool)
	UpdateTrainingDataset(id uuid.UUID, updateFn func(TrainingDatasetMetadata) TrainingDatasetMetadata) (TrainingDatasetMetadata, error)


}
