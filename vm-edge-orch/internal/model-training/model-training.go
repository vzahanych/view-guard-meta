package modeltraining

import (
	"context"

	"github.com/google/uuid"
)

// ModelTraining provides a unified interface for managing model training operations.
// It serves as an API client to communicate with the Python model training microservice.
//
// This service handles:
//   - Starting training jobs
//   - Querying training job status
//   - Canceling training jobs
//   - Listing training jobs
type ModelTraining interface {
	// StartTrainingJob starts a new training job with the provided configuration.
	StartTrainingJob(ctx context.Context, req TrainingJobRequest) (*TrainingJobResponse, error)

	// GetTrainingJobStatus retrieves the current status of a training job.
	GetTrainingJobStatus(ctx context.Context, jobID uuid.UUID) (*TrainingJobStatus, error)

	// CancelTrainingJob cancels a running training job.
	CancelTrainingJob(ctx context.Context, jobID uuid.UUID) error

	// ListTrainingJobs lists all training jobs, optionally filtered by edge ID or device ID.
	ListTrainingJobs(ctx context.Context, filter *TrainingJobFilter) ([]TrainingJobStatus, error)
}

// TrainingJobRequest holds the context needed to start a training job.
type TrainingJobRequest struct {
	EdgeID          uuid.UUID       `json:"edge_id"`
	CameraID        uuid.UUID       `json:"camera_id"`
	DeviceID        uuid.UUID       `json:"device_id"`
	DatasetID       uuid.UUID       `json:"dataset_id"`
	BaselineModelID uuid.UUID       `json:"baseline_model_id"`
	TrainingConfig  *TrainingConfig `json:"training_config,omitempty"`
}

// TrainingConfig represents optional hyperparameters for a training run.
type TrainingConfig struct {
	Epochs           int     `json:"epochs,omitempty"`
	BatchSize        int     `json:"batch_size,omitempty"`
	LearningRate     float64 `json:"learning_rate,omitempty"`
	ImageSize        int     `json:"image_size,omitempty"`
	DataAugmentation bool    `json:"data_augmentation,omitempty"`
	FreezeBackbone   bool    `json:"freeze_backbone,omitempty"`
}

// TrainingJobResponse represents the response from starting a training job.
type TrainingJobResponse struct {
	JobID               uuid.UUID `json:"job_id"`
	Status              string    `json:"status"`
	BaselineModelID     uuid.UUID `json:"baseline_model_id"`
	DatasetID           uuid.UUID `json:"dataset_id"`
	StartedAt           string    `json:"started_at"`
	EstimatedCompletion string    `json:"estimated_completion,omitempty"`
}

// TrainingJobStatus represents the current status of a training job.
type TrainingJobStatus struct {
	JobID               uuid.UUID `json:"job_id"`
	EdgeID              uuid.UUID `json:"edge_id"`
	CameraID            uuid.UUID `json:"camera_id"`
	DeviceID            uuid.UUID `json:"device_id"`
	DatasetID           uuid.UUID `json:"dataset_id"`
	BaselineModelID     uuid.UUID `json:"baseline_model_id"`
	Status              string    `json:"status"`
	Progress            float64   `json:"progress,omitempty"`
	StartedAt           string    `json:"started_at"`
	CompletedAt         string    `json:"completed_at,omitempty"`
	EstimatedCompletion string    `json:"estimated_completion,omitempty"`
	Error               string    `json:"error,omitempty"`
}

// TrainingJobFilter is used to filter training jobs when listing.
type TrainingJobFilter struct {
	EdgeID   *uuid.UUID `json:"edge_id,omitempty"`
	DeviceID *uuid.UUID `json:"device_id,omitempty"`
	Status   *string    `json:"status,omitempty"`
}

