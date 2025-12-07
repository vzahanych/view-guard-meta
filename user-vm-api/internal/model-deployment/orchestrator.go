package modeldeployment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
)

// ModelDeploymentOrchestrator coordinates model deployment workflow
type ModelDeploymentOrchestrator struct {
	store          *DeploymentStore
	modelCatalog   *modelcatalog.ModelCatalog
	modelStorage   *storage.ModelStorage
	modelConverter *ModelConverter
	transferService *ModelTransferService
	tunnelGateway  *tunnelgateway.EdgeAPIServer
	logger         *logging.Logger
	jobs           map[string]*DeploymentJob
	mu             sync.RWMutex
}

// NewModelDeploymentOrchestrator creates a new deployment orchestrator
func NewModelDeploymentOrchestrator(
	store *DeploymentStore,
	modelCatalog *modelcatalog.ModelCatalog,
	modelStorage *storage.ModelStorage,
	modelConverter *ModelConverter,
	transferService *ModelTransferService,
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
	go func() {
		transferResult, err := o.transferService.TransferModel(ctx, deploymentID, job.ModelID, job.EdgeID)
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

// CompleteDeployment marks a deployment as completed
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
// For PoC: Basic version history from deployment records
// Future: Full version tracking with semantic versioning
func (o *ModelDeploymentOrchestrator) GetVersionHistory(ctx context.Context, edgeID string, cameraID *string) ([]*DeploymentJob, error) {
	filters := &DeploymentFilters{
		EdgeID:   edgeID,
		CameraID: "",
		Status:   DeploymentStatusDeployed, // Only deployed models
	}

	if cameraID != nil {
		filters.CameraID = *cameraID
	}

	deployments, err := o.store.ListDeployments(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	return deployments, nil
}

// GetActiveModelVersion returns the active (latest deployed) model version for Edge/camera
// For PoC: Returns latest deployed model
// Future: Full active version tracking with rollback support
func (o *ModelDeploymentOrchestrator) GetActiveModelVersion(ctx context.Context, edgeID string, cameraID *string) (*DeploymentJob, error) {
	history, err := o.GetVersionHistory(ctx, edgeID, cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to get version history: %w", err)
	}

	if len(history) == 0 {
		return nil, fmt.Errorf("no deployed models found for Edge %s", edgeID)
	}

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

