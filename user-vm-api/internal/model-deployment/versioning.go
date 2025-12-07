package modeldeployment

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
)

// VersionManager handles model version tracking and rollback (basic for PoC, full implementation deferred)
type VersionManager struct {
	store        *DeploymentStore
	modelCatalog *modelcatalog.ModelCatalog
	logger       *logging.Logger
}

// NewVersionManager creates a new version manager
func NewVersionManager(
	store *DeploymentStore,
	modelCatalog *modelcatalog.ModelCatalog,
	logger *logging.Logger,
) (*VersionManager, error) {
	if store == nil {
		return nil, fmt.Errorf("deployment store is required")
	}
	if modelCatalog == nil {
		return nil, fmt.Errorf("model catalog is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &VersionManager{
		store:        store,
		modelCatalog: modelCatalog,
		logger:       logger,
	}, nil
}

// GetModelVersion extracts version from model metadata
// For PoC: Basic version extraction from model metadata
// Future: Full semantic versioning support
func (vm *VersionManager) GetModelVersion(modelID string) (string, error) {
	modelEntry, err := vm.modelCatalog.GetModel(modelID)
	if err != nil {
		return "", fmt.Errorf("failed to get model: %w", err)
	}

	// Extract version from model metadata
	if modelEntry.Metadata != nil && modelEntry.Metadata.Version != "" {
		return modelEntry.Metadata.Version, nil
	}

	// Fallback to model entry version
	if modelEntry.Version != "" {
		return modelEntry.Version, nil
	}

	// Default version for PoC
	return "1.0", nil
}

// GetDeploymentsByVersion lists deployments for a specific model version
// For PoC: Basic filtering by version
// Future: Full version history tracking and semantic versioning support
func (vm *VersionManager) GetDeploymentsByVersion(ctx context.Context, modelID string, version string) ([]*DeploymentJob, error) {
	// Get all deployments for the model
	filters := &DeploymentFilters{
		ModelID: modelID,
	}

	allDeployments, err := vm.store.ListDeployments(ctx, filters)
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

// GetVersionHistory returns version history for an Edge/camera
// For PoC: Basic version history from deployment records
// Future: Full version tracking with semantic versioning and rollback support
func (vm *VersionManager) GetVersionHistory(ctx context.Context, edgeID string, cameraID *string) ([]*DeploymentJob, error) {
	filters := &DeploymentFilters{
		EdgeID:   edgeID,
		CameraID: "",
		Status:   DeploymentStatusDeployed, // Only deployed models
	}

	if cameraID != nil {
		filters.CameraID = *cameraID
	}

	deployments, err := vm.store.ListDeployments(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	// Sort by deployment completed time (most recent first)
	// For PoC, we return as-is (database already orders by created_at DESC)
	// Future: Full sorting and version comparison

	return deployments, nil
}

// GetActiveModelVersion returns the active (latest deployed) model version for Edge/camera
// For PoC: Returns latest deployed model
// Future: Full active version tracking with rollback support
func (vm *VersionManager) GetActiveModelVersion(ctx context.Context, edgeID string, cameraID *string) (*DeploymentJob, error) {
	history, err := vm.GetVersionHistory(ctx, edgeID, cameraID)
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
func (vm *VersionManager) RollbackToVersion(ctx context.Context, edgeID string, cameraID *string, targetVersion string) error {
	// For PoC, return error indicating this is deferred
	return fmt.Errorf("model rollback is deferred to post-PoC. For PoC, models are deployed directly without rollback support")
}

// RequestRollback requests rollback to previous version from Edge
// NOTE: This is deferred to post-PoC. For PoC, this is a placeholder.
// Future implementation will:
// - Edge tests new model and accepts/rejects
// - If rejected, Edge falls back to previous version
// - VM tracks which version is active on each Edge
func (vm *VersionManager) RequestRollback(ctx context.Context, edgeID string, cameraID *string) error {
	// For PoC, return error indicating this is deferred
	return fmt.Errorf("Edge-side rollback request is deferred to post-PoC. For PoC, Edge-side model management is not implemented")
}

