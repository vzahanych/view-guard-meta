package modeldeployment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	modelcatalog "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// ModelDeploymentOrchestrator coordinates model deployment workflow
type ModelDeploymentOrchestrator struct {
	store           *DeploymentStore
	modelCatalog    *modelcatalog.ModelCatalog
	modelStorage    *storage.ModelStorage
	modelConverter  *ModelConverter
	transferService *ModelTransferService
	minioStorage    *MinIOModelStorage // MinIO storage for model archiving
	tunnelGateway   *tunnelgateway.EdgeAPIServer
	logger          *logging.Logger
	jobs            map[string]*DeploymentJob
	mu              sync.RWMutex
}

// NewModelDeploymentOrchestrator creates a new deployment orchestrator
func NewModelDeploymentOrchestrator(
	store *DeploymentStore,
	modelCatalog *modelcatalog.ModelCatalog,
	modelStorage *storage.ModelStorage,
	modelConverter *ModelConverter,
	transferService *ModelTransferService,
	minioStorage *MinIOModelStorage, // Optional: nil if MinIO is not configured
	tunnelGateway *tunnelgateway.EdgeAPIServer,
	logger *logging.Logger,
) (*ModelDeploymentOrchestrator, error) {
	if store == nil {
		return nil, fmt.Errorf("deployment store is required")
	}
	if modelCatalog == nil {
		return nil, fmt.Errorf("model catalog is required")
	}
	if modelStorage == nil {
		return nil, fmt.Errorf("model storage is required")
	}
	if modelConverter == nil {
		return nil, fmt.Errorf("model converter is required")
	}
	if transferService == nil {
		return nil, fmt.Errorf("transfer service is required")
	}
	if tunnelGateway == nil {
		return nil, fmt.Errorf("tunnel gateway is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &ModelDeploymentOrchestrator{
		store:           store,
		modelCatalog:    modelCatalog,
		modelStorage:    modelStorage,
		modelConverter:  modelConverter,
		transferService: transferService,
		minioStorage:    minioStorage, // Can be nil if MinIO is not configured
		tunnelGateway:   tunnelGateway,
		logger:          logger,
		jobs:            make(map[string]*DeploymentJob),
	}, nil
}

// CreateDeploymentJob creates a new deployment job
func (o *ModelDeploymentOrchestrator) CreateDeploymentJob(
	ctx context.Context,
	modelID string,
	edgeID string,
	cameraID *string,
) (*DeploymentJob, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID is required")
	}
	if edgeID == "" {
		return nil, fmt.Errorf("edge ID is required")
	}

	// Verify model exists
	modelEntry, err := o.modelCatalog.GetModel(modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	// Get model metadata to extract version
	var deploymentVersion *string
	if modelEntry.Metadata != nil && modelEntry.Metadata.Version != "" {
		v := modelEntry.Metadata.Version
		deploymentVersion = &v
	}

	// Create deployment job
	job := &DeploymentJob{
		DeploymentID:      uuid.New().String(),
		ModelID:           modelID,
		EdgeID:            edgeID,
		CameraID:          cameraID,
		Status:            DeploymentStatusPending,
		DeploymentVersion: deploymentVersion,
	}

	// Save to database
	err = o.store.CreateDeployment(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment job: %w", err)
	}

	// Store in memory
	o.mu.Lock()
	o.jobs[job.DeploymentID] = job
	o.mu.Unlock()

	o.logger.Info("Created deployment job",
		zap.String("deployment_id", job.DeploymentID),
		zap.String("model_id", modelID),
		zap.String("edge_id", edgeID),
		zap.Any("camera_id", cameraID),
	)

	return job, nil
}

// GetDeploymentJob retrieves a deployment job by ID
func (o *ModelDeploymentOrchestrator) GetDeploymentJob(ctx context.Context, deploymentID string) (*DeploymentJob, error) {
	if deploymentID == "" {
		return nil, fmt.Errorf("deployment ID is required")
	}

	// Try memory first
	o.mu.RLock()
	job, exists := o.jobs[deploymentID]
	o.mu.RUnlock()

	if exists {
		return job, nil
	}

	// Fall back to database
	job, err := o.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	// Cache in memory
	o.mu.Lock()
	o.jobs[deploymentID] = job
	o.mu.Unlock()

	return job, nil
}

// ListDeploymentJobs lists deployment jobs with optional filters
func (o *ModelDeploymentOrchestrator) ListDeploymentJobs(ctx context.Context, filters *DeploymentFilters) ([]*DeploymentJob, error) {
	return o.store.ListDeployments(ctx, filters)
}

// StartDeployment starts the deployment workflow for a job
func (o *ModelDeploymentOrchestrator) StartDeployment(ctx context.Context, deploymentID string) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment ID is required")
	}

	// Get deployment job
	job, err := o.GetDeploymentJob(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment job: %w", err)
	}

	// Check if already started
	if job.Status != DeploymentStatusPending {
		return fmt.Errorf("deployment job %s is not in pending status (current: %s)", deploymentID, job.Status)
	}

	// Validate and prepare model for deployment
	validationResult, err := o.modelConverter.PrepareModelForDeployment(job.ModelID)
	if err != nil {
		return fmt.Errorf("failed to prepare model for deployment: %w", err)
	}

	if !validationResult.Valid {
		errorMsg := fmt.Sprintf("model validation failed: %v", validationResult.Errors)
		_ = o.FailDeployment(ctx, deploymentID, errorMsg)
		return fmt.Errorf("model validation failed: %v", validationResult.Errors)
	}

	// Log warnings if any
	if len(validationResult.Warnings) > 0 {
		o.logger.Warn("Model validation warnings",
			zap.String("deployment_id", deploymentID),
			zap.String("model_id", job.ModelID),
			zap.Strings("warnings", validationResult.Warnings),
		)
	}

	// For trained models: Ensure they are stored in MinIO (trained models are stored in MinIO only, not on disk)
	// For baseline models: Archive to MinIO for backup (baseline models remain on disk)
	modelEntry, err := o.modelCatalog.GetModel(job.ModelID)
	if err == nil {
		isTrainedModel := modelEntry.TrainingDatasetID != "" || (modelEntry.Metadata != nil && modelEntry.Metadata.TrainingDatasetID != "")

		if isTrainedModel {
			// CRITICAL: Trained models MUST be persisted in MinIO before deployment
			// This ensures models can be redeployed if Edge restarts or is replaced
			// without requiring retraining for the same camera
			if o.minioStorage == nil {
				return fmt.Errorf("MinIO storage not configured, cannot deploy trained model %s (trained models must be stored in MinIO for persistence)", job.ModelID)
			}

			o.logger.Info("Verifying trained model is persisted in MinIO before deployment",
				zap.String("deployment_id", deploymentID),
				zap.String("model_id", job.ModelID),
				zap.String("reason", "Model must be in MinIO for redeployment if Edge restarts/replaces"),
			)

			// Use background context with timeout for MinIO check to avoid cancellation from HTTP request context
			// This ensures the check completes even if the HTTP handler returns
			minioCtx, minioCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer minioCancel()
			exists, err := o.minioStorage.ModelExistsInMinIO(minioCtx, job.ModelID)
			if err != nil {
				o.logger.Error("Failed to verify trained model in MinIO - deployment cannot proceed",
					zap.String("deployment_id", deploymentID),
					zap.String("model_id", job.ModelID),
					zap.Error(err),
				)
				return fmt.Errorf("failed to verify trained model in MinIO (required for persistence): %w", err)
			}
			if !exists {
				o.logger.Error("Trained model not found in MinIO - model must be persisted before deployment",
					zap.String("deployment_id", deploymentID),
					zap.String("model_id", job.ModelID),
					zap.String("reason", "Model must be in MinIO to allow redeployment without retraining"),
				)
				return fmt.Errorf("trained model %s not found in MinIO - model must be persisted to MinIO before deployment (allows redeployment if Edge restarts/replaces)", job.ModelID)
			}

			o.logger.Info("Confirmed trained model is persisted in MinIO - safe to deploy",
				zap.String("deployment_id", deploymentID),
				zap.String("model_id", job.ModelID),
				zap.String("note", "Model can be redeployed from MinIO if Edge restarts or is replaced"),
			)
		} else {
			// Baseline models: Archive to MinIO for backup (baseline models remain on disk)
			if o.minioStorage != nil {
				err = o.minioStorage.ArchiveModel(ctx, job.ModelID)
				if err != nil {
					// Log warning but continue with deployment - MinIO is for backup for baseline models
					o.logger.Warn("Failed to archive baseline model to MinIO, continuing with deployment",
						zap.String("deployment_id", deploymentID),
						zap.String("model_id", job.ModelID),
						zap.Error(err),
					)
				} else {
					o.logger.Info("Archived baseline model to MinIO",
						zap.String("deployment_id", deploymentID),
						zap.String("model_id", job.ModelID),
					)
				}
			}
		}
	}

	// Update status to deploying
	now := time.Now()
	job.Status = DeploymentStatusDeploying
	job.DeploymentStartedAt = &now

	err = o.store.UpdateDeployment(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	o.logger.Info("Started deployment",
		zap.String("deployment_id", deploymentID),
		zap.String("model_id", job.ModelID),
		zap.String("edge_id", job.EdgeID),
		zap.String("model_format", validationResult.ModelFormat),
		zap.Int64("model_size_bytes", validationResult.ModelSize),
	)

	// Transfer model to Edge (async)
	// Use background context with timeout for async transfer to avoid cancellation when StartDeployment returns
	// This ensures the transfer can complete even if the original request context is canceled
	transferCtx, transferCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer transferCancel() // Cancel if goroutine exits early
	go func() {
		defer transferCancel() // Ensure context is canceled when goroutine exits
		transferResult, err := o.transferService.TransferModel(transferCtx, deploymentID, job.ModelID, job.EdgeID)
		if err != nil {
			o.logger.Error("Model transfer failed",
				zap.String("deployment_id", deploymentID),
				zap.String("model_id", job.ModelID),
				zap.String("edge_id", job.EdgeID),
				zap.Error(err),
			)
			_ = o.FailDeployment(ctx, deploymentID, fmt.Sprintf("Transfer failed: %v", err))
			return
		}

		if !transferResult.Success {
			o.logger.Error("Model transfer failed",
				zap.String("deployment_id", deploymentID),
				zap.String("model_id", job.ModelID),
				zap.String("edge_id", job.EdgeID),
				zap.String("error", transferResult.ErrorMessage),
			)
			_ = o.FailDeployment(ctx, deploymentID, transferResult.ErrorMessage)
			return
		}

		// Transfer successful, mark deployment as completed
		_ = o.CompleteDeployment(ctx, deploymentID, transferResult.ModelFilePath)
	}()

	return nil
}

// CompleteDeployment marks a deployment as completed (deployed to Edge)
func (o *ModelDeploymentOrchestrator) CompleteDeployment(ctx context.Context, deploymentID string, modelFilePath *string) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment ID is required")
	}

	job, err := o.GetDeploymentJob(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment job: %w", err)
	}

	now := time.Now()
	job.Status = DeploymentStatusDeployed
	job.DeploymentCompletedAt = &now
	if modelFilePath != nil {
		job.ModelFilePath = modelFilePath
	}

	err = o.store.UpdateDeployment(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	o.logger.Info("Completed deployment",
		zap.String("deployment_id", deploymentID),
		zap.String("model_id", job.ModelID),
		zap.String("edge_id", job.EdgeID),
	)

	return nil
}

// ActivateDeployment marks a deployment as active (model loaded and active on Edge for inference)
// This is called when Edge confirms the model is loaded and ready for inference
func (o *ModelDeploymentOrchestrator) ActivateDeployment(ctx context.Context, deploymentID string) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment ID is required")
	}

	job, err := o.GetDeploymentJob(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment job: %w", err)
	}

	// Update status to active
	job.Status = DeploymentStatusActive

	err = o.store.UpdateDeployment(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	o.logger.Info("Activated deployment",
		zap.String("deployment_id", deploymentID),
		zap.String("model_id", job.ModelID),
		zap.String("edge_id", job.EdgeID),
		zap.String("camera_id", func() string {
			if job.CameraID != nil {
				return *job.CameraID
			}
			return ""
		}()),
	)

	// Note: Marking previous deployments as superseded is deferred to future enhancement
	// For PoC, we track active model via GetActiveModelVersion() which returns latest deployed model

	return nil
}

// FailDeployment marks a deployment as failed
func (o *ModelDeploymentOrchestrator) FailDeployment(ctx context.Context, deploymentID string, errorMessage string) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment ID is required")
	}

	job, err := o.GetDeploymentJob(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment job: %w", err)
	}

	now := time.Now()
	job.Status = DeploymentStatusFailed
	job.DeploymentCompletedAt = &now
	job.ErrorMessage = &errorMessage

	err = o.store.UpdateDeployment(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	o.logger.Error("Deployment failed",
		zap.String("deployment_id", deploymentID),
		zap.String("model_id", job.ModelID),
		zap.String("edge_id", job.EdgeID),
		zap.String("error", errorMessage),
	)

	return nil
}

// DetermineDeploymentTargets determines target Edge appliances for a model
func (o *ModelDeploymentOrchestrator) DetermineDeploymentTargets(ctx context.Context, modelID string) ([]DeploymentTarget, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID is required")
	}

	// Get model entry
	modelEntry, err := o.modelCatalog.GetModel(modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	var targets []DeploymentTarget

	// Extract edge_id and camera_id from model metadata
	if modelEntry.Metadata != nil {
		// For trained models, deploy to the Edge that provided the training dataset
		if modelEntry.Metadata.TrainingDatasetID != "" {
			// Get edge_id from training dataset (dataset has edge_id)
			// For PoC, we need to query the training_datasets table to get edge_id
			// For now, we'll use camera_id to infer - this is a limitation
			// TODO: Query training_datasets table to get edge_id from dataset_id
			// For now, we'll need to add edge_id to model metadata during training
			// or query the dataset storage service

			// Check if we can get edge_id from preprocessing metadata (stored by training service)
			var edgeID string
			if modelEntry.Metadata.Preprocessing != nil {
				if edgeIDVal, ok := modelEntry.Metadata.Preprocessing["edge_id"].(string); ok {
					edgeID = edgeIDVal
				}
			}

			if edgeID == "" {
				// Fallback: This is a PoC limitation
				// In production, edge_id should be stored in model metadata or queried from dataset
				o.logger.Warn("Edge ID not found in model metadata, cannot determine deployment target",
					zap.String("model_id", modelID),
					zap.String("camera_id", modelEntry.CameraID),
					zap.String("dataset_id", modelEntry.Metadata.TrainingDatasetID),
				)
				return nil, fmt.Errorf("edge_id not found in model metadata for model %s (dataset_id: %s)", modelID, modelEntry.Metadata.TrainingDatasetID)
			}

			var cameraID *string
			if modelEntry.CameraID != "" {
				cameraID = &modelEntry.CameraID
			}
			targets = append(targets, DeploymentTarget{
				EdgeID:   edgeID,
				CameraID: cameraID,
			})
		}
	}

	// If no targets found, return error
	if len(targets) == 0 {
		return nil, fmt.Errorf("no deployment targets found for model %s (model may not have edge_id or camera_id in metadata)", modelID)
	}

	return targets, nil
}

// GetVersionHistory returns version history for an Edge/camera
// Returns deployments with status "deployed" or "active" (models that were successfully deployed)
// For PoC: Basic version history from deployment records
// Future: Full version tracking with semantic versioning
func (o *ModelDeploymentOrchestrator) GetVersionHistory(ctx context.Context, edgeID string, cameraID *string) ([]*DeploymentJob, error) {
	// Get all deployments for Edge/camera (we'll filter by status in application code)
	filters := &DeploymentFilters{
		EdgeID:   edgeID,
		CameraID: "",
	}

	if cameraID != nil {
		filters.CameraID = *cameraID
	}

	deployments, err := o.store.ListDeployments(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	// Filter to only deployed/active models (exclude pending, deploying, failed)
	var successfulDeployments []*DeploymentJob
	for _, dep := range deployments {
		if dep.Status == DeploymentStatusDeployed || dep.Status == DeploymentStatusActive {
			successfulDeployments = append(successfulDeployments, dep)
		}
	}

	return successfulDeployments, nil
}

// GetActiveModelVersion returns the active (latest deployed/active) model version for Edge/camera
// For PoC: Returns latest deployed or active model
// Future: Full active version tracking with rollback support
func (o *ModelDeploymentOrchestrator) GetActiveModelVersion(ctx context.Context, edgeID string, cameraID *string) (*DeploymentJob, error) {
	history, err := o.GetVersionHistory(ctx, edgeID, cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to get version history: %w", err)
	}

	if len(history) == 0 {
		cameraStr := ""
		if cameraID != nil {
			cameraStr = fmt.Sprintf(" and camera %s", *cameraID)
		}
		return nil, fmt.Errorf("no deployed models found for Edge %s%s", edgeID, cameraStr)
	}

	// Prefer active models over deployed models
	// If multiple active models exist, return the most recent one
	var activeModel *DeploymentJob
	for _, dep := range history {
		if dep.Status == DeploymentStatusActive {
			if activeModel == nil || (dep.CreatedAt.After(activeModel.CreatedAt)) {
				activeModel = dep
			}
		}
	}

	if activeModel != nil {
		return activeModel, nil
	}

	// No active model found, return the most recent deployed model
	// Return the most recent deployment (first in list, as it's sorted by created_at DESC)
	return history[0], nil
}

// RollbackToVersion rolls back to a previous model version
// NOTE: This is deferred to post-PoC. For PoC, this is a placeholder.
// Future implementation will:
// - Verify previous model version exists
// - Verify Edge is connected
// - Redeploy previous model version
// - Handle Edge-side testing and acceptance
func (o *ModelDeploymentOrchestrator) RollbackToVersion(ctx context.Context, edgeID string, cameraID *string, targetVersion string) error {
	// For PoC, return error indicating this is deferred
	return fmt.Errorf("model rollback is deferred to post-PoC. For PoC, models are deployed directly without rollback support")
}

// DeploymentTarget represents a target Edge appliance for deployment
type DeploymentTarget struct {
	EdgeID   string
	CameraID *string
}
