package trainingservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
)

// JobStatus represents the status of a training job
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// TrainingConfig represents training configuration
type TrainingConfig struct {
	Epochs            int     `json:"epochs"`
	BatchSize         int     `json:"batch_size"`
	LearningRate      float64 `json:"learning_rate"`
	ImageSize         int     `json:"image_size"`
	DataAugmentation  bool    `json:"data_augmentation"`
	FreezeBackbone    bool    `json:"freeze_backbone"`
}

// TrainingMetrics represents training metrics
type TrainingMetrics struct {
	TrainLoss     []float64 `json:"train_loss"`
	ValLoss       []float64 `json:"val_loss"`
	Map           []float64 `json:"map"`
	Precision     []float64 `json:"precision"`
	Recall        []float64 `json:"recall"`
	CurrentEpoch  int       `json:"current_epoch"`
	TotalEpochs   int       `json:"total_epochs"`
	CurrentLoss   *float64  `json:"current_loss,omitempty"`
	CurrentValLoss *float64 `json:"current_val_loss,omitempty"`
	CurrentMap    *float64  `json:"current_map,omitempty"`
}

// TrainingJob represents a training job record
type TrainingJob struct {
	JobID            string           `json:"job_id"`
	BaselineModelID  string           `json:"baseline_model_id"`
	DatasetID        string           `json:"dataset_id"`
	CameraID         string           `json:"camera_id"`
	EdgeID           string           `json:"edge_id"`
	Status           JobStatus        `json:"status"`
	TrainedModelID   *string          `json:"trained_model_id,omitempty"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	ErrorMessage     *string          `json:"error_message,omitempty"`
	TrainingConfig   *TrainingConfig  `json:"training_config,omitempty"`
	Metrics          *TrainingMetrics `json:"metrics,omitempty"`
	DeploymentID     *string          `json:"deployment_id,omitempty"`
	DeploymentStatus *string          `json:"deployment_status,omitempty"`
	DeployedAt       *time.Time       `json:"deployed_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// JobStore manages training job storage in SQLite
type JobStore struct {
	db *database.DB
}

// NewJobStore creates a new training job store
func NewJobStore(db *database.DB) (*JobStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}

	return &JobStore{
		db: db,
	}, nil
}

// CreateJob creates a new training job
func (js *JobStore) CreateJob(ctx context.Context, job *TrainingJob) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}

	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	// Serialize training config
	configJSON, err := json.Marshal(job.TrainingConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal training config: %w", err)
	}

	// Serialize metrics (may be nil for new job)
	metricsJSON := []byte("null")
	if job.Metrics != nil {
		metricsJSON, err = json.Marshal(job.Metrics)
		if err != nil {
			return fmt.Errorf("failed to marshal metrics: %w", err)
		}
	}

	// Convert timestamps to Unix seconds (SQLite INTEGER)
	var startedAt, completedAt, deployedAt *int64
	if job.StartedAt != nil {
		ts := job.StartedAt.Unix()
		startedAt = &ts
	}
	if job.CompletedAt != nil {
		ts := job.CompletedAt.Unix()
		completedAt = &ts
	}
	if job.DeployedAt != nil {
		ts := job.DeployedAt.Unix()
		deployedAt = &ts
	}

	var deploymentID, deploymentStatus sql.NullString
	if job.DeploymentID != nil {
		deploymentID = sql.NullString{String: *job.DeploymentID, Valid: true}
	}
	if job.DeploymentStatus != nil {
		deploymentStatus = sql.NullString{String: *job.DeploymentStatus, Valid: true}
	}

	query := `
		INSERT INTO training_jobs (
			job_id, baseline_model_id, dataset_id, camera_id, edge_id,
			status, trained_model_id, started_at, completed_at, error_message,
			training_config, metrics, deployment_id, deployment_status, deployed_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = js.db.ExecContext(ctx, query,
		job.JobID,
		job.BaselineModelID,
		job.DatasetID,
		job.CameraID,
		job.EdgeID,
		string(job.Status),
		job.TrainedModelID,
		startedAt,
		completedAt,
		job.ErrorMessage,
		string(configJSON),
		string(metricsJSON),
		deploymentID,
		deploymentStatus,
		deployedAt,
		job.CreatedAt.Unix(),
		job.UpdatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to create training job: %w", err)
	}

	return nil
}

// UpdateJobStatus updates job status and optionally other fields
func (js *JobStore) UpdateJobStatus(ctx context.Context, jobID string, status JobStatus, updates *JobUpdate) error {
	if jobID == "" {
		return fmt.Errorf("job ID is required")
	}

	now := time.Now()

	// Build update query dynamically
	query := "UPDATE training_jobs SET status = ?, updated_at = ?"
	args := []interface{}{string(status), now.Unix()}

	if updates != nil {
		if updates.TrainedModelID != nil {
			query += ", trained_model_id = ?"
			args = append(args, *updates.TrainedModelID)
		}

		if updates.StartedAt != nil {
			query += ", started_at = ?"
			ts := updates.StartedAt.Unix()
			args = append(args, ts)
		}

		if updates.CompletedAt != nil {
			query += ", completed_at = ?"
			ts := updates.CompletedAt.Unix()
			args = append(args, ts)
		}

		if updates.ErrorMessage != nil {
			query += ", error_message = ?"
			args = append(args, *updates.ErrorMessage)
		}

		if updates.TrainingConfig != nil {
			configJSON, err := json.Marshal(updates.TrainingConfig)
			if err != nil {
				return fmt.Errorf("failed to marshal training config: %w", err)
			}
			query += ", training_config = ?"
			args = append(args, string(configJSON))
		}

		if updates.Metrics != nil {
			metricsJSON, err := json.Marshal(updates.Metrics)
			if err != nil {
				return fmt.Errorf("failed to marshal metrics: %w", err)
			}
			query += ", metrics = ?"
			args = append(args, string(metricsJSON))
		}

		if updates.DeploymentID != nil {
			query += ", deployment_id = ?"
			args = append(args, *updates.DeploymentID)
		}

		if updates.DeploymentStatus != nil {
			query += ", deployment_status = ?"
			args = append(args, *updates.DeploymentStatus)
		}

		if updates.DeployedAt != nil {
			query += ", deployed_at = ?"
			ts := updates.DeployedAt.Unix()
			args = append(args, ts)
		}
	}

	query += " WHERE job_id = ?"
	args = append(args, jobID)

	result, err := js.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update training job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("training job not found: %s", jobID)
	}

	return nil
}

// JobUpdate contains optional fields to update
type JobUpdate struct {
	TrainedModelID   *string          `json:"trained_model_id,omitempty"`
	StartedAt        *time.Time       `json:"started_at,omitempty"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
	ErrorMessage     *string          `json:"error_message,omitempty"`
	TrainingConfig   *TrainingConfig  `json:"training_config,omitempty"`
	Metrics          *TrainingMetrics `json:"metrics,omitempty"`
	DeploymentID     *string          `json:"deployment_id,omitempty"`
	DeploymentStatus *string          `json:"deployment_status,omitempty"`
	DeployedAt       *time.Time       `json:"deployed_at,omitempty"`
}

// GetJob retrieves a training job by ID
func (js *JobStore) GetJob(ctx context.Context, jobID string) (*TrainingJob, error) {
	if jobID == "" {
		return nil, fmt.Errorf("job ID is required")
	}

	query := `
		SELECT job_id, baseline_model_id, dataset_id, camera_id, edge_id,
		       status, trained_model_id, started_at, completed_at, error_message,
		       training_config, metrics, deployment_id, deployment_status, deployed_at,
		       created_at, updated_at
		FROM training_jobs
		WHERE job_id = ?
	`

	var job TrainingJob
	var startedAt, completedAt, deployedAt, createdAt, updatedAt sql.NullInt64
	var trainedModelID, errorMessage, deploymentID, deploymentStatus sql.NullString
	var configJSON, metricsJSON sql.NullString

	err := js.db.QueryRowContext(ctx, query, jobID).Scan(
		&job.JobID,
		&job.BaselineModelID,
		&job.DatasetID,
		&job.CameraID,
		&job.EdgeID,
		&job.Status,
		&trainedModelID,
		&startedAt,
		&completedAt,
		&errorMessage,
		&configJSON,
		&metricsJSON,
		&deploymentID,
		&deploymentStatus,
		&deployedAt,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("training job not found: %s", jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get training job: %w", err)
	}

	// Parse optional fields
	if trainedModelID.Valid {
		job.TrainedModelID = &trainedModelID.String
	}
	if errorMessage.Valid {
		job.ErrorMessage = &errorMessage.String
	}
	if startedAt.Valid {
		t := time.Unix(startedAt.Int64, 0)
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		job.CompletedAt = &t
	}
	if createdAt.Valid {
		job.CreatedAt = time.Unix(createdAt.Int64, 0)
	}
	if updatedAt.Valid {
		job.UpdatedAt = time.Unix(updatedAt.Int64, 0)
	}

	// Parse JSON fields
	if configJSON.Valid && configJSON.String != "null" {
		var config TrainingConfig
		if err := json.Unmarshal([]byte(configJSON.String), &config); err == nil {
			job.TrainingConfig = &config
		}
	}

	if metricsJSON.Valid && metricsJSON.String != "null" {
		var metrics TrainingMetrics
		if err := json.Unmarshal([]byte(metricsJSON.String), &metrics); err == nil {
			job.Metrics = &metrics
		}
	}

	return &job, nil
}

// ListJobsOptions contains options for listing jobs
type ListJobsOptions struct {
	CameraID *string
	EdgeID   *string
	Status   *JobStatus
	Limit    int
	Offset   int
}

// ListJobs lists training jobs with optional filters
func (js *JobStore) ListJobs(ctx context.Context, opts *ListJobsOptions) ([]*TrainingJob, error) {
	if opts == nil {
		opts = &ListJobsOptions{
			Limit:  100,
			Offset: 0,
		}
	}

	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 1000 {
		opts.Limit = 1000
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	query := `
		SELECT job_id, baseline_model_id, dataset_id, camera_id, edge_id,
		       status, trained_model_id, started_at, completed_at, error_message,
		       training_config, metrics, created_at, updated_at
		FROM training_jobs
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.CameraID != nil {
		query += " AND camera_id = ?"
		args = append(args, *opts.CameraID)
	}

	if opts.EdgeID != nil {
		query += " AND edge_id = ?"
		args = append(args, *opts.EdgeID)
	}

	if opts.Status != nil {
		query += " AND status = ?"
		args = append(args, string(*opts.Status))
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := js.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list training jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*TrainingJob
	for rows.Next() {
		job, err := js.scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return jobs, nil
}

// scanJob scans a row into a TrainingJob struct
func (js *JobStore) scanJob(rows *sql.Rows) (*TrainingJob, error) {
	var job TrainingJob
	var startedAt, completedAt, createdAt, updatedAt sql.NullInt64
	var trainedModelID, errorMessage sql.NullString
	var configJSON, metricsJSON sql.NullString

	err := rows.Scan(
		&job.JobID,
		&job.BaselineModelID,
		&job.DatasetID,
		&job.CameraID,
		&job.EdgeID,
		&job.Status,
		&trainedModelID,
		&startedAt,
		&completedAt,
		&errorMessage,
		&configJSON,
		&metricsJSON,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		return nil, err
	}

	// Parse optional fields
	if trainedModelID.Valid {
		job.TrainedModelID = &trainedModelID.String
	}
	if errorMessage.Valid {
		job.ErrorMessage = &errorMessage.String
	}
	if startedAt.Valid {
		t := time.Unix(startedAt.Int64, 0)
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		job.CompletedAt = &t
	}
	if createdAt.Valid {
		job.CreatedAt = time.Unix(createdAt.Int64, 0)
	}
	if updatedAt.Valid {
		job.UpdatedAt = time.Unix(updatedAt.Int64, 0)
	}

	// Parse JSON fields
	if configJSON.Valid && configJSON.String != "null" {
		var config TrainingConfig
		if err := json.Unmarshal([]byte(configJSON.String), &config); err == nil {
			job.TrainingConfig = &config
		}
	}

	if metricsJSON.Valid && metricsJSON.String != "null" {
		var metrics TrainingMetrics
		if err := json.Unmarshal([]byte(metricsJSON.String), &metrics); err == nil {
			job.Metrics = &metrics
		}
	}

	return &job, nil
}

// CountJobs counts training jobs matching the filters
func (js *JobStore) CountJobs(ctx context.Context, opts *ListJobsOptions) (int, error) {
	if opts == nil {
		opts = &ListJobsOptions{}
	}

	query := "SELECT COUNT(*) FROM training_jobs WHERE 1=1"
	args := []interface{}{}

	if opts.CameraID != nil {
		query += " AND camera_id = ?"
		args = append(args, *opts.CameraID)
	}

	if opts.EdgeID != nil {
		query += " AND edge_id = ?"
		args = append(args, *opts.EdgeID)
	}

	if opts.Status != nil {
		query += " AND status = ?"
		args = append(args, string(*opts.Status))
	}

	var count int
	err := js.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count training jobs: %w", err)
	}

	return count, nil
}

