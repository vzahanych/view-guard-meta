package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

const (
	// MaxModelSizeBytes is the maximum size for a model file (50MB)
	MaxModelSizeBytes = 50 * 1024 * 1024
	// ModelsDirName is the directory name for storing models
	ModelsDirName = "models"
)

// ModelMetadata represents model metadata from VM
type ModelMetadata struct {
	ModelID            string                 `json:"model_id"`
	Version            string                 `json:"version"`
	ModelType          string                 `json:"model_type"`
	CameraID           *string                `json:"camera_id,omitempty"`
	Framework          string                 `json:"framework"`
	TrainingDatasetID  *string                `json:"training_dataset_id,omitempty"`
	TrainingDate       *string                `json:"training_date,omitempty"`
	InputShape         []int                  `json:"input_shape,omitempty"`
	Preprocessing      map[string]interface{} `json:"preprocessing,omitempty"`
	AdditionalMetadata map[string]interface{} `json:"-"` // For any extra fields
}

// DeployedModel represents a deployed model in the database
type DeployedModel struct {
	ModelID      string
	DeploymentID *string
	ModelPath    string
	MetadataPath string
	DeployedAt   time.Time
	Status       string // 'active', 'inactive', 'failed'
	EdgeID       string
	CameraID     *string
	Version      string
	ModelType    string
	Framework    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ModelStorage manages model files and database records
type ModelStorage struct {
	modelsDir string
	db        *sql.DB
	logger    *logger.Logger
}

// NewModelStorage creates a new model storage service
func NewModelStorage(cfg *config.Config, stateMgr *state.Manager, log *logger.Logger) (*ModelStorage, error) {
	// Determine models directory
	modelsDir := filepath.Join(cfg.Edge.Orchestrator.DataDir, ModelsDirName)
	
	// Ensure directory exists
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}

	return &ModelStorage{
		modelsDir: modelsDir,
		db:        stateMgr.GetDB(),
		logger:    log,
	}, nil
}

// GetModelsDir returns the models directory path
func (ms *ModelStorage) GetModelsDir() string {
	return ms.modelsDir
}

// StoreModel stores a model file and metadata, and creates a database record
func (ms *ModelStorage) StoreModel(
	ctx context.Context,
	modelID string,
	deploymentID *string,
	edgeID string,
	cameraID *string,
	modelData []byte,
	metadata *ModelMetadata,
) (*DeployedModel, error) {
	// Validate model size
	if len(modelData) > MaxModelSizeBytes {
		return nil, fmt.Errorf("model size %d bytes exceeds maximum allowed size of %d bytes", len(modelData), MaxModelSizeBytes)
	}

	// Validate model format (basic check - should be ONNX)
	// For PoC, we'll just check the file extension or magic bytes
	// In production, this would use a proper ONNX parser
	if len(modelData) < 4 {
		return nil, fmt.Errorf("model file too small to be valid")
	}

	// Create model directory
	modelDir := filepath.Join(ms.modelsDir, modelID)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create model directory: %w", err)
	}

	// Write model file
	modelPath := filepath.Join(modelDir, "model.onnx")
	if err := os.WriteFile(modelPath, modelData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write model file: %w", err)
	}

	// Write metadata file
	metadataPath := filepath.Join(modelDir, "metadata.json")
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		// Clean up model file on error
		os.Remove(modelPath)
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil {
		// Clean up model file on error
		os.Remove(modelPath)
		return nil, fmt.Errorf("failed to write metadata file: %w", err)
	}

	// Create database record
	now := time.Now()
	deployedModel := &DeployedModel{
		ModelID:      modelID,
		DeploymentID: deploymentID,
		ModelPath:    modelPath,
		MetadataPath: metadataPath,
		DeployedAt:   now,
		Status:       "active",
		EdgeID:       edgeID,
		CameraID:     cameraID,
		Version:      metadata.Version,
		ModelType:    metadata.ModelType,
		Framework:    metadata.Framework,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := ms.saveModelRecord(ctx, deployedModel); err != nil {
		// Clean up files on error
		os.Remove(modelPath)
		os.Remove(metadataPath)
		return nil, fmt.Errorf("failed to save model record: %w", err)
	}

	ms.logger.Info("Model stored successfully",
		"model_id", modelID,
		"deployment_id", deploymentID,
		"edge_id", edgeID,
		"camera_id", cameraID,
		"model_path", modelPath,
	)

	return deployedModel, nil
}

// saveModelRecord saves a model record to the database
func (ms *ModelStorage) saveModelRecord(ctx context.Context, model *DeployedModel) error {
	query := `
		INSERT INTO deployed_models (
			model_id, deployment_id, model_path, metadata_path,
			deployed_at, status, edge_id, camera_id,
			version, model_type, framework,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_id) DO UPDATE SET
			deployment_id = excluded.deployment_id,
			model_path = excluded.model_path,
			metadata_path = excluded.metadata_path,
			deployed_at = excluded.deployed_at,
			status = excluded.status,
			camera_id = excluded.camera_id,
			version = excluded.version,
			model_type = excluded.model_type,
			framework = excluded.framework,
			updated_at = excluded.updated_at
	`

	var cameraID interface{}
	if model.CameraID != nil {
		cameraID = *model.CameraID
	}

	var deploymentID interface{}
	if model.DeploymentID != nil {
		deploymentID = *model.DeploymentID
	}

	_, err := ms.db.ExecContext(ctx, query,
		model.ModelID,
		deploymentID,
		model.ModelPath,
		model.MetadataPath,
		model.DeployedAt.Unix(),
		model.Status,
		model.EdgeID,
		cameraID,
		model.Version,
		model.ModelType,
		model.Framework,
		model.CreatedAt.Unix(),
		model.UpdatedAt.Unix(),
	)

	return err
}

// GetModel retrieves a deployed model by ID
func (ms *ModelStorage) GetModel(ctx context.Context, modelID string) (*DeployedModel, error) {
	query := `
		SELECT model_id, deployment_id, model_path, metadata_path,
		       deployed_at, status, edge_id, camera_id,
		       version, model_type, framework,
		       created_at, updated_at
		FROM deployed_models
		WHERE model_id = ?
	`

	var model DeployedModel
	var deploymentID, cameraID sql.NullString
	var deployedAt, createdAt, updatedAt int64

	err := ms.db.QueryRowContext(ctx, query, modelID).Scan(
		&model.ModelID,
		&deploymentID,
		&model.ModelPath,
		&model.MetadataPath,
		&deployedAt,
		&model.Status,
		&model.EdgeID,
		&cameraID,
		&model.Version,
		&model.ModelType,
		&model.Framework,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query model: %w", err)
	}

	if deploymentID.Valid {
		model.DeploymentID = &deploymentID.String
	}
	if cameraID.Valid {
		model.CameraID = &cameraID.String
	}
	model.DeployedAt = time.Unix(deployedAt, 0)
	model.CreatedAt = time.Unix(createdAt, 0)
	model.UpdatedAt = time.Unix(updatedAt, 0)

	return &model, nil
}

// ListModels lists deployed models with optional filters
func (ms *ModelStorage) ListModels(ctx context.Context, edgeID string, cameraID *string, status *string) ([]*DeployedModel, error) {
	query := `
		SELECT model_id, deployment_id, model_path, metadata_path,
		       deployed_at, status, edge_id, camera_id,
		       version, model_type, framework,
		       created_at, updated_at
		FROM deployed_models
		WHERE edge_id = ?
	`
	args := []interface{}{edgeID}

	if cameraID != nil {
		query += " AND camera_id = ?"
		args = append(args, *cameraID)
	}
	if status != nil {
		query += " AND status = ?"
		args = append(args, *status)
	}

	query += " ORDER BY deployed_at DESC"

	rows, err := ms.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query models: %w", err)
	}
	defer rows.Close()

	var models []*DeployedModel
	for rows.Next() {
		var model DeployedModel
		var deploymentID, cameraID sql.NullString
		var deployedAt, createdAt, updatedAt int64

		if err := rows.Scan(
			&model.ModelID,
			&deploymentID,
			&model.ModelPath,
			&model.MetadataPath,
			&deployedAt,
			&model.Status,
			&model.EdgeID,
			&cameraID,
			&model.Version,
			&model.ModelType,
			&model.Framework,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}

		if deploymentID.Valid {
			model.DeploymentID = &deploymentID.String
		}
		if cameraID.Valid {
			model.CameraID = &cameraID.String
		}
		model.DeployedAt = time.Unix(deployedAt, 0)
		model.CreatedAt = time.Unix(createdAt, 0)
		model.UpdatedAt = time.Unix(updatedAt, 0)

		models = append(models, &model)
	}

	return models, nil
}

// GetModelByVersion retrieves a model by model ID and version
func (ms *ModelStorage) GetModelByVersion(ctx context.Context, modelID string, version string) (*DeployedModel, error) {
	query := `
		SELECT model_id, deployment_id, model_path, metadata_path,
		       deployed_at, status, edge_id, camera_id,
		       version, model_type, framework,
		       created_at, updated_at
		FROM deployed_models
		WHERE model_id = ? AND version = ?
		ORDER BY deployed_at DESC
		LIMIT 1
	`

	var model DeployedModel
	var deploymentID, cameraID sql.NullString
	var deployedAt, createdAt, updatedAt int64

	err := ms.db.QueryRowContext(ctx, query, modelID, version).Scan(
		&model.ModelID,
		&deploymentID,
		&model.ModelPath,
		&model.MetadataPath,
		&deployedAt,
		&model.Status,
		&model.EdgeID,
		&cameraID,
		&model.Version,
		&model.ModelType,
		&model.Framework,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("model not found: %s version %s", modelID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query model: %w", err)
	}

	if deploymentID.Valid {
		model.DeploymentID = &deploymentID.String
	}
	if cameraID.Valid {
		model.CameraID = &cameraID.String
	}
	model.DeployedAt = time.Unix(deployedAt, 0)
	model.CreatedAt = time.Unix(createdAt, 0)
	model.UpdatedAt = time.Unix(updatedAt, 0)

	return &model, nil
}

// GetVersionHistory returns all versions of a model
func (ms *ModelStorage) GetVersionHistory(ctx context.Context, modelID string) ([]*DeployedModel, error) {
	query := `
		SELECT model_id, deployment_id, model_path, metadata_path,
		       deployed_at, status, edge_id, camera_id,
		       version, model_type, framework,
		       created_at, updated_at
		FROM deployed_models
		WHERE model_id = ?
		ORDER BY deployed_at DESC
	`

	rows, err := ms.db.QueryContext(ctx, query, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to query model versions: %w", err)
	}
	defer rows.Close()

	var models []*DeployedModel
	for rows.Next() {
		var model DeployedModel
		var deploymentID, cameraID sql.NullString
		var deployedAt, createdAt, updatedAt int64

		if err := rows.Scan(
			&model.ModelID,
			&deploymentID,
			&model.ModelPath,
			&model.MetadataPath,
			&deployedAt,
			&model.Status,
			&model.EdgeID,
			&cameraID,
			&model.Version,
			&model.ModelType,
			&model.Framework,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}

		if deploymentID.Valid {
			model.DeploymentID = &deploymentID.String
		}
		if cameraID.Valid {
			model.CameraID = &cameraID.String
		}
		model.DeployedAt = time.Unix(deployedAt, 0)
		model.CreatedAt = time.Unix(createdAt, 0)
		model.UpdatedAt = time.Unix(updatedAt, 0)

		models = append(models, &model)
	}

	return models, nil
}

// UpdateModelStatus updates the status of a deployed model
func (ms *ModelStorage) UpdateModelStatus(ctx context.Context, modelID string, status string) error {
	if status != "active" && status != "inactive" && status != "failed" {
		return fmt.Errorf("invalid status: %s (must be active, inactive, or failed)", status)
	}

	query := `
		UPDATE deployed_models
		SET status = ?, updated_at = ?
		WHERE model_id = ?
	`

	_, err := ms.db.ExecContext(ctx, query, status, time.Now().Unix(), modelID)
	if err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	ms.logger.Info("Model status updated",
		"model_id", modelID,
		"status", status,
	)

	return nil
}

// DeleteModel deletes a model and its files
func (ms *ModelStorage) DeleteModel(ctx context.Context, modelID string) error {
	// Get model to find file paths
	model, err := ms.GetModel(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to get model for deletion: %w", err)
	}

	// Delete files
	if err := os.Remove(model.ModelPath); err != nil && !os.IsNotExist(err) {
		ms.logger.Warn("Failed to delete model file", "path", model.ModelPath, "error", err)
	}
	if err := os.Remove(model.MetadataPath); err != nil && !os.IsNotExist(err) {
		ms.logger.Warn("Failed to delete metadata file", "path", model.MetadataPath, "error", err)
	}

	// Try to remove model directory if empty
	modelDir := filepath.Dir(model.ModelPath)
	if err := os.Remove(modelDir); err != nil && !os.IsNotExist(err) {
		// Directory not empty or other error - ignore
		ms.logger.Debug("Could not remove model directory (may not be empty)", "dir", modelDir, "error", err)
	}

	// Delete database record
	query := `DELETE FROM deployed_models WHERE model_id = ?`
	_, err = ms.db.ExecContext(ctx, query, modelID)
	if err != nil {
		return fmt.Errorf("failed to delete model record: %w", err)
	}

	ms.logger.Info("Model deleted",
		"model_id", modelID,
		"model_path", model.ModelPath,
	)

	return nil
}

// CleanupOptions controls model cleanup behavior
type CleanupOptions struct {
	// RemoveInactive removes models with status 'inactive'
	RemoveInactive bool
	// RemoveFailed removes models with status 'failed'
	RemoveFailed bool
	// KeepActiveVersions keeps the N most recent active models per camera
	KeepActiveVersions int
	// MaxAgeDays removes models older than this many days
	MaxAgeDays int
	// FreeSpaceTargetMB target free space in MB (cleanup until this is reached)
	FreeSpaceTargetMB int64
}

// CleanupResult describes the outcome of a cleanup operation
type CleanupResult struct {
	DeletedModels   int
	FreedSpaceBytes int64
	Errors          []string
}

// CleanupModels removes old or inactive models to free disk space
func (ms *ModelStorage) CleanupModels(ctx context.Context, opts CleanupOptions) (*CleanupResult, error) {
	result := &CleanupResult{
		Errors: make([]string, 0),
	}

	// Get all models
	query := `
		SELECT model_id, deployment_id, model_path, metadata_path,
		       deployed_at, status, edge_id, camera_id,
		       version, model_type, framework,
		       created_at, updated_at
		FROM deployed_models
		ORDER BY deployed_at ASC
	`

	rows, err := ms.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query models for cleanup: %w", err)
	}
	defer rows.Close()

	var modelsToDelete []*DeployedModel
	modelsByCamera := make(map[string][]*DeployedModel) // camera_id -> models

	for rows.Next() {
		var model DeployedModel
		var deploymentID, cameraID sql.NullString
		var deployedAt, createdAt, updatedAt int64

		if err := rows.Scan(
			&model.ModelID,
			&deploymentID,
			&model.ModelPath,
			&model.MetadataPath,
			&deployedAt,
			&model.Status,
			&model.EdgeID,
			&cameraID,
			&model.Version,
			&model.ModelType,
			&model.Framework,
			&createdAt,
			&updatedAt,
		); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to scan model: %v", err))
			continue
		}

		if deploymentID.Valid {
			model.DeploymentID = &deploymentID.String
		}
		if cameraID.Valid {
			model.CameraID = &cameraID.String
		}
		model.DeployedAt = time.Unix(deployedAt, 0)
		model.CreatedAt = time.Unix(createdAt, 0)
		model.UpdatedAt = time.Unix(updatedAt, 0)

		// Check if model should be deleted based on status
		if opts.RemoveInactive && model.Status == "inactive" {
			modelsToDelete = append(modelsToDelete, &model)
			continue
		}
		if opts.RemoveFailed && model.Status == "failed" {
			modelsToDelete = append(modelsToDelete, &model)
			continue
		}

		// Check if model is too old
		if opts.MaxAgeDays > 0 {
			age := time.Since(model.DeployedAt)
			if age > time.Duration(opts.MaxAgeDays)*24*time.Hour {
				modelsToDelete = append(modelsToDelete, &model)
				continue
			}
		}

		// Group active models by camera for version management
		if model.Status == "active" && model.CameraID != nil {
			camID := *model.CameraID
			modelsByCamera[camID] = append(modelsByCamera[camID], &model)
		}
	}

	// If KeepActiveVersions is set, mark older active models for deletion
	if opts.KeepActiveVersions > 0 {
		for camID, cameraModels := range modelsByCamera {
			if len(cameraModels) > opts.KeepActiveVersions {
				// Sort by deployed_at descending (newest first)
				// Keep the N newest, mark the rest for deletion
				sort.Slice(cameraModels, func(i, j int) bool {
					return cameraModels[i].DeployedAt.After(cameraModels[j].DeployedAt)
				})
				// Mark older models for deletion
				for i := opts.KeepActiveVersions; i < len(cameraModels); i++ {
					modelsToDelete = append(modelsToDelete, cameraModels[i])
				}
				ms.logger.Debug("Marking old active models for deletion",
					"camera_id", camID,
					"total_models", len(cameraModels),
					"keeping", opts.KeepActiveVersions,
					"deleting", len(cameraModels)-opts.KeepActiveVersions,
				)
			}
		}
	}

	// Delete marked models
	for _, model := range modelsToDelete {
		// Get file sizes before deletion
		var modelSize, metadataSize int64
		if info, err := os.Stat(model.ModelPath); err == nil {
			modelSize = info.Size()
		}
		if info, err := os.Stat(model.MetadataPath); err == nil {
			metadataSize = info.Size()
		}

		if err := ms.DeleteModel(ctx, model.ModelID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to delete model %s: %v", model.ModelID, err))
			continue
		}

		result.DeletedModels++
		result.FreedSpaceBytes += modelSize + metadataSize

		// Check if we've freed enough space
		if opts.FreeSpaceTargetMB > 0 {
			freedMB := result.FreedSpaceBytes / (1024 * 1024)
			if freedMB >= opts.FreeSpaceTargetMB {
				ms.logger.Info("Cleanup target reached",
					"freed_mb", freedMB,
					"target_mb", opts.FreeSpaceTargetMB,
				)
				break
			}
		}
	}

	ms.logger.Info("Model cleanup completed",
		"deleted_models", result.DeletedModels,
		"freed_space_mb", result.FreedSpaceBytes/(1024*1024),
		"errors", len(result.Errors),
	)

	return result, nil
}

// GetStorageStats returns storage statistics for deployed models
func (ms *ModelStorage) GetStorageStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count models by status
	query := `
		SELECT status, COUNT(*) as count
		FROM deployed_models
		GROUP BY status
	`

	rows, err := ms.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query model stats: %w", err)
	}
	defer rows.Close()

	statusCounts := make(map[string]int)
	totalModels := 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		statusCounts[status] = count
		totalModels += count
	}

	stats["total_models"] = totalModels
	stats["by_status"] = statusCounts

	// Calculate total storage used
	var totalSize int64
	models, err := ms.ListModels(ctx, "", nil, nil) // Get all models
	if err == nil {
		for _, model := range models {
			if info, err := os.Stat(model.ModelPath); err == nil {
				totalSize += info.Size()
			}
			if info, err := os.Stat(model.MetadataPath); err == nil {
				totalSize += info.Size()
			}
		}
	}

	stats["total_size_bytes"] = totalSize
	stats["total_size_mb"] = float64(totalSize) / (1024 * 1024)

	return stats, nil
}

