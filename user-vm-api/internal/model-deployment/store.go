package modeldeployment

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
)

// DeploymentStatus represents the status of a deployment job
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusDeploying DeploymentStatus = "deploying"
	DeploymentStatusDeployed  DeploymentStatus = "deployed"
	DeploymentStatusFailed    DeploymentStatus = "failed"
)

// DeploymentJob represents a model deployment job record
type DeploymentJob struct {
	DeploymentID         string           `json:"deployment_id"`
	ModelID              string           `json:"model_id"`
	EdgeID               string           `json:"edge_id"`
	CameraID             *string          `json:"camera_id,omitempty"`
	Status               DeploymentStatus  `json:"status"`
	DeploymentStartedAt  *time.Time        `json:"deployment_started_at,omitempty"`
	DeploymentCompletedAt *time.Time       `json:"deployment_completed_at,omitempty"`
	ErrorMessage         *string           `json:"error_message,omitempty"`
	ModelFilePath        *string           `json:"model_file_path,omitempty"`
	DeploymentVersion    *string           `json:"deployment_version,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// DeploymentStore manages deployment job storage in SQLite
type DeploymentStore struct {
	db *database.DB
}

// NewDeploymentStore creates a new deployment store
func NewDeploymentStore(db *database.DB) (*DeploymentStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}

	return &DeploymentStore{
		db: db,
	}, nil
}

// CreateDeployment creates a new deployment job
func (ds *DeploymentStore) CreateDeployment(ctx context.Context, job *DeploymentJob) error {
	if job == nil {
		return fmt.Errorf("deployment job is required")
	}

	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	query := `
		INSERT INTO model_deployments (
			deployment_id, model_id, edge_id, camera_id, status,
			deployment_started_at, deployment_completed_at, error_message,
			model_file_path, deployment_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var cameraID sql.NullString
	if job.CameraID != nil {
		cameraID = sql.NullString{String: *job.CameraID, Valid: true}
	}

	var startedAt sql.NullInt64
	if job.DeploymentStartedAt != nil {
		startedAt = sql.NullInt64{Int64: job.DeploymentStartedAt.Unix(), Valid: true}
	}

	var completedAt sql.NullInt64
	if job.DeploymentCompletedAt != nil {
		completedAt = sql.NullInt64{Int64: job.DeploymentCompletedAt.Unix(), Valid: true}
	}

	var errorMsg sql.NullString
	if job.ErrorMessage != nil {
		errorMsg = sql.NullString{String: *job.ErrorMessage, Valid: true}
	}

	var modelPath sql.NullString
	if job.ModelFilePath != nil {
		modelPath = sql.NullString{String: *job.ModelFilePath, Valid: true}
	}

	var version sql.NullString
	if job.DeploymentVersion != nil {
		version = sql.NullString{String: *job.DeploymentVersion, Valid: true}
	}

	_, err := ds.db.ExecContext(ctx, query,
		job.DeploymentID,
		job.ModelID,
		job.EdgeID,
		cameraID,
		string(job.Status),
		startedAt,
		completedAt,
		errorMsg,
		modelPath,
		version,
		job.CreatedAt.Unix(),
		job.UpdatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to create deployment job: %w", err)
	}

	return nil
}

// GetDeployment retrieves a deployment job by ID
func (ds *DeploymentStore) GetDeployment(ctx context.Context, deploymentID string) (*DeploymentJob, error) {
	if deploymentID == "" {
		return nil, fmt.Errorf("deployment ID is required")
	}

	query := `
		SELECT deployment_id, model_id, edge_id, camera_id, status,
		       deployment_started_at, deployment_completed_at, error_message,
		       model_file_path, deployment_version, created_at, updated_at
		FROM model_deployments
		WHERE deployment_id = ?
	`

	var job DeploymentJob
	var cameraID sql.NullString
	var startedAt sql.NullInt64
	var completedAt sql.NullInt64
	var errorMsg sql.NullString
	var modelPath sql.NullString
	var version sql.NullString
	var status string
	var createdAt, updatedAt int64

	err := ds.db.QueryRowContext(ctx, query, deploymentID).Scan(
		&job.DeploymentID,
		&job.ModelID,
		&job.EdgeID,
		&cameraID,
		&status,
		&startedAt,
		&completedAt,
		&errorMsg,
		&modelPath,
		&version,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("deployment job not found: %s", deploymentID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment job: %w", err)
	}

	job.Status = DeploymentStatus(status)
	job.CreatedAt = time.Unix(createdAt, 0)
	job.UpdatedAt = time.Unix(updatedAt, 0)

	if cameraID.Valid {
		job.CameraID = &cameraID.String
	}
	if startedAt.Valid {
		t := time.Unix(startedAt.Int64, 0)
		job.DeploymentStartedAt = &t
	}
	if completedAt.Valid {
		t := time.Unix(completedAt.Int64, 0)
		job.DeploymentCompletedAt = &t
	}
	if errorMsg.Valid {
		job.ErrorMessage = &errorMsg.String
	}
	if modelPath.Valid {
		job.ModelFilePath = &modelPath.String
	}
	if version.Valid {
		job.DeploymentVersion = &version.String
	}

	return &job, nil
}

// UpdateDeploymentStatus updates the status of a deployment job
func (ds *DeploymentStore) UpdateDeploymentStatus(ctx context.Context, deploymentID string, status DeploymentStatus) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment ID is required")
	}

	now := time.Now()
	query := `
		UPDATE model_deployments
		SET status = ?, updated_at = ?
		WHERE deployment_id = ?
	`

	_, err := ds.db.ExecContext(ctx, query, string(status), now.Unix(), deploymentID)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	return nil
}

// UpdateDeployment updates a deployment job
func (ds *DeploymentStore) UpdateDeployment(ctx context.Context, job *DeploymentJob) error {
	if job == nil {
		return fmt.Errorf("deployment job is required")
	}

	job.UpdatedAt = time.Now()

	query := `
		UPDATE model_deployments
		SET status = ?, deployment_started_at = ?, deployment_completed_at = ?,
		    error_message = ?, model_file_path = ?, deployment_version = ?, updated_at = ?
		WHERE deployment_id = ?
	`

	var startedAt sql.NullInt64
	if job.DeploymentStartedAt != nil {
		startedAt = sql.NullInt64{Int64: job.DeploymentStartedAt.Unix(), Valid: true}
	}

	var completedAt sql.NullInt64
	if job.DeploymentCompletedAt != nil {
		completedAt = sql.NullInt64{Int64: job.DeploymentCompletedAt.Unix(), Valid: true}
	}

	var errorMsg sql.NullString
	if job.ErrorMessage != nil {
		errorMsg = sql.NullString{String: *job.ErrorMessage, Valid: true}
	}

	var modelPath sql.NullString
	if job.ModelFilePath != nil {
		modelPath = sql.NullString{String: *job.ModelFilePath, Valid: true}
	}

	var version sql.NullString
	if job.DeploymentVersion != nil {
		version = sql.NullString{String: *job.DeploymentVersion, Valid: true}
	}

	_, err := ds.db.ExecContext(ctx, query,
		string(job.Status),
		startedAt,
		completedAt,
		errorMsg,
		modelPath,
		version,
		job.UpdatedAt.Unix(),
		job.DeploymentID,
	)

	if err != nil {
		return fmt.Errorf("failed to update deployment job: %w", err)
	}

	return nil
}

// ListDeployments lists deployment jobs with optional filters
func (ds *DeploymentStore) ListDeployments(ctx context.Context, filters *DeploymentFilters) ([]*DeploymentJob, error) {
	query := `
		SELECT deployment_id, model_id, edge_id, camera_id, status,
		       deployment_started_at, deployment_completed_at, error_message,
		       model_file_path, deployment_version, created_at, updated_at
		FROM model_deployments
		WHERE 1=1
	`
	args := []interface{}{}

	if filters != nil {
		if filters.EdgeID != "" {
			query += " AND edge_id = ?"
			args = append(args, filters.EdgeID)
		}
		if filters.ModelID != "" {
			query += " AND model_id = ?"
			args = append(args, filters.ModelID)
		}
		if filters.CameraID != "" {
			query += " AND camera_id = ?"
			args = append(args, filters.CameraID)
		}
		if filters.Status != "" {
			query += " AND status = ?"
			args = append(args, string(filters.Status))
		}
	}

	query += " ORDER BY created_at DESC"

	if filters != nil && filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
		if filters.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filters.Offset)
		}
	}

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer rows.Close()

	var jobs []*DeploymentJob
	for rows.Next() {
		var job DeploymentJob
		var cameraID sql.NullString
		var startedAt sql.NullInt64
		var completedAt sql.NullInt64
		var errorMsg sql.NullString
		var modelPath sql.NullString
		var version sql.NullString
		var status string
		var createdAt, updatedAt int64

		err := rows.Scan(
			&job.DeploymentID,
			&job.ModelID,
			&job.EdgeID,
			&cameraID,
			&status,
			&startedAt,
			&completedAt,
			&errorMsg,
			&modelPath,
			&version,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}

		job.Status = DeploymentStatus(status)
		job.CreatedAt = time.Unix(createdAt, 0)
		job.UpdatedAt = time.Unix(updatedAt, 0)

		if cameraID.Valid {
			job.CameraID = &cameraID.String
		}
		if startedAt.Valid {
			t := time.Unix(startedAt.Int64, 0)
			job.DeploymentStartedAt = &t
		}
		if completedAt.Valid {
			t := time.Unix(completedAt.Int64, 0)
			job.DeploymentCompletedAt = &t
		}
		if errorMsg.Valid {
			job.ErrorMessage = &errorMsg.String
		}
		if modelPath.Valid {
			job.ModelFilePath = &modelPath.String
		}
		if version.Valid {
			job.DeploymentVersion = &version.String
		}

		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate deployments: %w", err)
	}

	return jobs, nil
}

// GetDeploymentsByVersion lists deployments filtered by version
// For PoC: Basic version filtering
// Future: Full semantic versioning support
func (ds *DeploymentStore) GetDeploymentsByVersion(ctx context.Context, modelID string, version string) ([]*DeploymentJob, error) {
	// Get all deployments for the model
	filters := &DeploymentFilters{
		ModelID: modelID,
	}

	allDeployments, err := ds.ListDeployments(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	// Filter by version
	var filtered []*DeploymentJob
	for _, deployment := range allDeployments {
		if deployment.DeploymentVersion != nil && *deployment.DeploymentVersion == version {
			filtered = append(filtered, deployment)
		}
	}

	return filtered, nil
}

// DeploymentFilters represents filters for listing deployments
type DeploymentFilters struct {
	EdgeID   string
	ModelID  string
	CameraID string
	Status   DeploymentStatus
	Limit    int
	Offset   int
}

