package modelcatalog

import (
	"fmt"
	"strings"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
)

// ModelStatus represents the status of a model
type ModelStatus string

const (
	ModelStatusBaseline   ModelStatus = "baseline"   // Baseline model available for training
	ModelStatusTraining   ModelStatus = "training"   // Model is currently being trained
	ModelStatusReady      ModelStatus = "ready"      // Model is ready for deployment
	ModelStatusDeployed   ModelStatus = "deployed"   // Model is deployed to Edge
	ModelStatusDeprecated ModelStatus = "deprecated" // Model is deprecated (replaced by newer version)
)

// ModelCatalog manages model registration, querying, and status tracking
type ModelCatalog struct {
	modelStorage *storage.ModelStorage
	baseDir      string
}

// NewModelCatalog creates a new model catalog
func NewModelCatalog(modelStorage *storage.ModelStorage, baseDir string) (*ModelCatalog, error) {
	if modelStorage == nil {
		return nil, fmt.Errorf("model storage is required")
	}
	if baseDir == "" {
		return nil, fmt.Errorf("base directory is required")
	}

	return &ModelCatalog{
		modelStorage: modelStorage,
		baseDir:      baseDir,
	}, nil
}

// ModelEntry represents a model entry in the catalog
type ModelEntry struct {
	ModelID           string                 `json:"model_id"`
	Version           string                 `json:"version"`
	CameraID          string                 `json:"camera_id,omitempty"`
	ModelType         string                 `json:"model_type"`
	Status            ModelStatus            `json:"status"`
	Framework         string                 `json:"framework"`
	TrainingDatasetID string                 `json:"training_dataset_id,omitempty"`
	TrainingDate      string                 `json:"training_date,omitempty"`
	Metadata          *storage.ModelMetadata `json:"metadata,omitempty"`
}

// RegisterModel registers a model in the catalog
// This is called after a model is stored via ModelStorage
func (mc *ModelCatalog) RegisterModel(modelID string, metadata *storage.ModelMetadata) error {
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}
	if metadata == nil {
		return fmt.Errorf("metadata is required")
	}

	// Verify model exists in storage
	if !mc.modelStorage.ModelExists(modelID) {
		return fmt.Errorf("model %s does not exist in storage", modelID)
	}

	// Model is already registered if it exists in storage and has metadata
	// The catalog indexes models by scanning the filesystem
	// No additional registration step needed - models are auto-indexed
	return nil
}

// GetModel retrieves a model by ID
func (mc *ModelCatalog) GetModel(modelID string) (*ModelEntry, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID is required")
	}

	// Get model info from storage
	modelInfo, err := mc.modelStorage.GetModelInfo(modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}

	// Determine model status based on metadata and location
	status := mc.determineModelStatus(modelInfo.Metadata, modelID)

	return &ModelEntry{
		ModelID:           modelInfo.Metadata.ModelID,
		Version:           modelInfo.Metadata.Version,
		CameraID:          modelInfo.Metadata.CameraID,
		ModelType:         modelInfo.Metadata.ModelType,
		Status:            status,
		Framework:         modelInfo.Metadata.Framework,
		TrainingDatasetID: modelInfo.Metadata.TrainingDatasetID,
		TrainingDate:      modelInfo.Metadata.TrainingDate,
		Metadata:          modelInfo.Metadata,
	}, nil
}

// ListModels returns all models in the catalog
func (mc *ModelCatalog) ListModels() ([]*ModelEntry, error) {
	models, err := mc.modelStorage.ListModels()
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var entries []*ModelEntry
	for _, modelID := range models {
		entry, err := mc.GetModel(modelID)
		if err != nil {
			// Skip models that can't be read
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetBaselineModels returns all baseline models available for training
func (mc *ModelCatalog) GetBaselineModels() ([]*ModelEntry, error) {
	allModels, err := mc.ListModels()
	if err != nil {
		return nil, err
	}

	var baselineModels []*ModelEntry
	for _, model := range allModels {
		if model.Status == ModelStatusBaseline {
			baselineModels = append(baselineModels, model)
		}
	}

	return baselineModels, nil
}

// GetBaselineModelsByType returns baseline models filtered by model type
func (mc *ModelCatalog) GetBaselineModelsByType(modelType string) ([]*ModelEntry, error) {
	baselineModels, err := mc.GetBaselineModels()
	if err != nil {
		return nil, err
	}

	var filtered []*ModelEntry
	for _, model := range baselineModels {
		if model.ModelType == modelType {
			filtered = append(filtered, model)
		}
	}

	return filtered, nil
}

// GetTrainedModels returns all trained models (non-baseline)
func (mc *ModelCatalog) GetTrainedModels() ([]*ModelEntry, error) {
	allModels, err := mc.ListModels()
	if err != nil {
		return nil, err
	}

	var trainedModels []*ModelEntry
	for _, model := range allModels {
		if model.Status != ModelStatusBaseline {
			trainedModels = append(trainedModels, model)
		}
	}

	return trainedModels, nil
}

// GetModelsByCamera returns models for a specific camera
func (mc *ModelCatalog) GetModelsByCamera(cameraID string) ([]*ModelEntry, error) {
	allModels, err := mc.ListModels()
	if err != nil {
		return nil, err
	}

	var filtered []*ModelEntry
	for _, model := range allModels {
		if model.CameraID == cameraID {
			filtered = append(filtered, model)
		}
	}

	return filtered, nil
}

// GetModelsByDataset returns models trained from a specific dataset
func (mc *ModelCatalog) GetModelsByDataset(datasetID string) ([]*ModelEntry, error) {
	allModels, err := mc.ListModels()
	if err != nil {
		return nil, err
	}

	var filtered []*ModelEntry
	for _, model := range allModels {
		if model.TrainingDatasetID == datasetID {
			filtered = append(filtered, model)
		}
	}

	return filtered, nil
}

// GetModelsByStatus returns models filtered by status
func (mc *ModelCatalog) GetModelsByStatus(status ModelStatus) ([]*ModelEntry, error) {
	allModels, err := mc.ListModels()
	if err != nil {
		return nil, err
	}

	var filtered []*ModelEntry
	for _, model := range allModels {
		if model.Status == status {
			filtered = append(filtered, model)
		}
	}

	return filtered, nil
}

// UpdateModelStatus updates the status of a model
// Note: Status is derived from model location and metadata, but we can update metadata to reflect status changes
func (mc *ModelCatalog) UpdateModelStatus(modelID string, status ModelStatus) error {
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}

	// Verify model exists
	if !mc.modelStorage.ModelExists(modelID) {
		return fmt.Errorf("model %s does not exist", modelID)
	}

	// Status is determined by location and metadata, not stored directly
	// For status tracking, we could add a status field to metadata in the future
	// For now, status is derived from:
	// - baseline: model_id starts with "baseline-" or in baseline/ directory
	// - training: training_dataset_id is set but training_date is empty
	// - ready: training_dataset_id and training_date are set
	// - deployed: (future - would need additional metadata)
	// - deprecated: (future - would need additional metadata)

	// For PoC, we derive status from metadata and location
	// Future enhancement: store status explicitly in metadata
	_ = status // Acknowledge status parameter for future use

	return nil
}

// determineModelStatus determines model status based on metadata and location
func (mc *ModelCatalog) determineModelStatus(metadata *storage.ModelMetadata, modelID string) ModelStatus {
	// Check if it's a baseline model (model_id starts with "baseline-" or in baseline/ directory)
	if strings.HasPrefix(modelID, "baseline-") {
		return ModelStatusBaseline
	}

	// Check model path to determine if it's in baseline/ or trained/ directory
	modelPath := mc.modelStorage.GetModelPath(modelID)
	if strings.Contains(modelPath, "/baseline/") {
		return ModelStatusBaseline
	}

	// Check if model is being trained (has dataset but no training date)
	if metadata.TrainingDatasetID != "" && metadata.TrainingDate == "" {
		return ModelStatusTraining
	}

	// Check if model is trained (has dataset and training date)
	if metadata.TrainingDatasetID != "" && metadata.TrainingDate != "" {
		return ModelStatusReady
	}

	// Default to ready if it's not a baseline model
	return ModelStatusReady
}

// GetDefaultBaselineModel returns the default baseline model for training
// Default: baseline-yolov8n
func (mc *ModelCatalog) GetDefaultBaselineModel() (*ModelEntry, error) {
	defaultModelID := "baseline-yolov8n"
	
	// Check if default model exists
	if !mc.modelStorage.ModelExists(defaultModelID) {
		// Try to find any baseline YOLO model
		baselineModels, err := mc.GetBaselineModelsByType("yolo")
		if err != nil {
			return nil, fmt.Errorf("failed to get baseline models: %w", err)
		}
		
		if len(baselineModels) == 0 {
			return nil, fmt.Errorf("no baseline models available")
		}
		
		// Return first available baseline YOLO model
		return baselineModels[0], nil
	}

	return mc.GetModel(defaultModelID)
}

// ScanModels scans the filesystem and indexes all models
// This is called on catalog initialization to build the index
func (mc *ModelCatalog) ScanModels() error {
	// ModelStorage.ListModels() already scans the filesystem
	// This method is here for future enhancements (caching, indexing, etc.)
	_, err := mc.modelStorage.ListModels()
	return err
}

// ModelExists checks if a model exists in the catalog
func (mc *ModelCatalog) ModelExists(modelID string) bool {
	return mc.modelStorage.ModelExists(modelID)
}

// GetModelLineage returns the lineage of a trained model (baseline → dataset → trained model)
func (mc *ModelCatalog) GetModelLineage(modelID string) (*ModelLineage, error) {
	model, err := mc.GetModel(modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	lineage := &ModelLineage{
		TrainedModel: model,
	}

	// If this is a trained model, try to find the baseline model it was trained from
	if model.TrainingDatasetID != "" {
		// For now, we assume baseline-yolov8n was used
		// Future: Store baseline_model_id in metadata
		baselineModelID := "baseline-yolov8n"
		if mc.modelStorage.ModelExists(baselineModelID) {
			baselineModel, err := mc.GetModel(baselineModelID)
			if err == nil {
				lineage.BaselineModel = baselineModel
			}
		}
	}

	return lineage, nil
}

// ModelLineage represents the lineage of a trained model
type ModelLineage struct {
	BaselineModel *ModelEntry `json:"baseline_model,omitempty"`
	DatasetID     string      `json:"dataset_id,omitempty"`
	TrainedModel  *ModelEntry `json:"trained_model"`
}

