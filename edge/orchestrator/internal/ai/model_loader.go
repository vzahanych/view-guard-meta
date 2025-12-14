package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
)

// ModelLoader manages model loading and activation for inference
type ModelLoader struct {
	modelStorage *storage.ModelStorage
	aiClient     *Client
	logger       *logger.Logger
	activeModels map[string]*ActiveModel // camera_id -> active model
}

// ActiveModel represents a currently loaded and active model
type ActiveModel struct {
	ModelID      string
	ModelPath    string
	MetadataPath string
	Version      string
	ModelType    string
	Framework    string
	CameraID     *string
	LoadedAt     time.Time
	Ready        bool
	Metadata     *ModelMetadataForInference
}

// ToActiveModelInfo converts ActiveModel to ActiveModelInfo for interface compatibility
func (am *ActiveModel) ToActiveModelInfo() *ActiveModelInfo {
	return &ActiveModelInfo{
		ModelID:      am.ModelID,
		ModelPath:    am.ModelPath,
		MetadataPath: am.MetadataPath,
		Version:      am.Version,
		ModelType:    am.ModelType,
		Framework:    am.Framework,
		CameraID:     am.CameraID,
		LoadedAt:     am.LoadedAt,
		Ready:        am.Ready,
	}
}

// ActiveModelInfo represents a loaded model (for interface compatibility)
type ActiveModelInfo struct {
	ModelID      string
	ModelPath    string
	MetadataPath string
	Version      string
	ModelType    string
	Framework    string
	CameraID     *string
	LoadedAt     time.Time
	Ready        bool
}

// ModelMetadataForInference represents model metadata for inference
// This is a subset of storage.ModelMetadata focused on inference needs
type ModelMetadataForInference struct {
	ModelID       string                 `json:"model_id"`
	Version       string                 `json:"version"`
	ModelType     string                 `json:"model_type"`
	CameraID      *string                `json:"camera_id,omitempty"`
	Framework     string                 `json:"framework"`
	InputShape    []int                  `json:"input_shape,omitempty"`
	Preprocessing map[string]interface{} `json:"preprocessing,omitempty"`
}

// ModelReadinessCheck validates that a model is ready for inference
type ModelReadinessCheck struct {
	ModelExists     bool
	MetadataExists  bool
	ValidFormat     bool
	InputShapeValid bool
	Errors          []string
}

// NewModelLoader creates a new model loader
func NewModelLoader(modelStorage *storage.ModelStorage, aiClient *Client, log *logger.Logger) *ModelLoader {
	return &ModelLoader{
		modelStorage: modelStorage,
		aiClient:     aiClient,
		logger:       log,
		activeModels: make(map[string]*ActiveModel),
	}
}

// LoadModel loads a model from storage and prepares it for inference
// Returns ActiveModelInfo for interface compatibility
func (ml *ModelLoader) LoadModel(ctx context.Context, modelID string, cameraID *string) (*ActiveModelInfo, error) {
	// Get model from storage
	deployedModel, err := ml.modelStorage.GetModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model from storage: %w", err)
	}

	// Check if model files exist
	if _, err := os.Stat(deployedModel.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", deployedModel.ModelPath)
	}
	if _, err := os.Stat(deployedModel.MetadataPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("metadata file not found: %s", deployedModel.MetadataPath)
	}

	// Load metadata
	metadata, err := ml.loadMetadata(deployedModel.MetadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	// Check model readiness
	readiness, err := ml.CheckModelReadiness(ctx, deployedModel)
	if err != nil {
		return nil, fmt.Errorf("failed to check model readiness: %w", err)
	}
	if !readiness.ValidFormat || len(readiness.Errors) > 0 {
		return nil, fmt.Errorf("model not ready: %v", readiness.Errors)
	}

	// Create active model
	activeModel := &ActiveModel{
		ModelID:      modelID,
		ModelPath:    deployedModel.ModelPath,
		MetadataPath: deployedModel.MetadataPath,
		Version:      deployedModel.Version,
		ModelType:    deployedModel.ModelType,
		Framework:    deployedModel.Framework,
		CameraID:     cameraID,
		LoadedAt:     time.Now(),
		Ready:        true,
		Metadata:     metadata,
	}

	// Deactivate previous model for this camera if exists
	if cameraID != nil {
		key := *cameraID
		if oldModel, exists := ml.activeModels[key]; exists {
			ml.logger.Info("Deactivating previous model",
				"old_model_id", oldModel.ModelID,
				"new_model_id", modelID,
				"camera_id", *cameraID,
			)
			// Update old model status in database
			_ = ml.modelStorage.UpdateModelStatus(ctx, oldModel.ModelID, "inactive")
		}
		ml.activeModels[key] = activeModel
	}

	// Update model status to active in database
	if err := ml.modelStorage.UpdateModelStatus(ctx, modelID, "active"); err != nil {
		ml.logger.Warn("Failed to update model status to active",
			"model_id", modelID,
			"error", err,
		)
	}

	ml.logger.Info("Model loaded successfully",
		"model_id", modelID,
		"version", deployedModel.Version,
		"camera_id", cameraID,
		"model_path", deployedModel.ModelPath,
	)

	// Notify AI service about new model (if AI service supports model loading)
	// For PoC, this is a placeholder - actual model loading in AI service will be in future epic
	ml.notifyAIService(ctx, activeModel)

	return activeModel.ToActiveModelInfo(), nil
}

// loadMetadata loads model metadata from JSON file
func (ml *ModelLoader) loadMetadata(metadataPath string) (*ModelMetadataForInference, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	// First unmarshal into storage.ModelMetadata to get all fields
	var storageMetadata storage.ModelMetadata
	if err := json.Unmarshal(data, &storageMetadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata JSON: %w", err)
	}

	// Convert to inference-focused metadata
	metadata := &ModelMetadataForInference{
		ModelID:       storageMetadata.ModelID,
		Version:       storageMetadata.Version,
		ModelType:     storageMetadata.ModelType,
		CameraID:      storageMetadata.CameraID,
		Framework:     storageMetadata.Framework,
		InputShape:    storageMetadata.InputShape,
		Preprocessing: storageMetadata.Preprocessing,
	}

	return metadata, nil
}

// CheckModelReadiness validates that a model is ready for inference
func (ml *ModelLoader) CheckModelReadiness(ctx context.Context, model *storage.DeployedModel) (*ModelReadinessCheck, error) {
	check := &ModelReadinessCheck{
		Errors: make([]string, 0),
	}

	// Check if model file exists
	if _, err := os.Stat(model.ModelPath); err == nil {
		check.ModelExists = true
	} else {
		check.Errors = append(check.Errors, fmt.Sprintf("Model file not found: %s", model.ModelPath))
	}

	// Check if metadata file exists
	if _, err := os.Stat(model.MetadataPath); err == nil {
		check.MetadataExists = true
	} else {
		check.Errors = append(check.Errors, fmt.Sprintf("Metadata file not found: %s", model.MetadataPath))
	}

	// Basic format validation (check file extension)
	if check.ModelExists {
		ext := filepath.Ext(model.ModelPath)
		if ext == ".onnx" {
			check.ValidFormat = true
		} else {
			check.Errors = append(check.Errors, fmt.Sprintf("Unsupported model format: %s (expected .onnx)", ext))
		}
	}

	// Load and validate metadata
	if check.MetadataExists {
		metadata, err := ml.loadMetadata(model.MetadataPath)
		if err == nil {
			// Validate input shape
			if len(metadata.InputShape) > 0 {
				// Basic validation: input shape should have at least 3 dimensions (batch, height, width) or 4 (batch, channels, height, width)
				if len(metadata.InputShape) >= 3 {
					check.InputShapeValid = true
					// Validate dimensions are positive
					for i, dim := range metadata.InputShape {
						if dim <= 0 {
							check.Errors = append(check.Errors, fmt.Sprintf("Invalid input shape dimension %d: %d (must be > 0)", i, dim))
							check.InputShapeValid = false
						}
					}
				} else {
					check.Errors = append(check.Errors, fmt.Sprintf("Input shape must have at least 3 dimensions, got %d", len(metadata.InputShape)))
				}
			} else {
				check.Errors = append(check.Errors, "Input shape not specified in metadata")
			}
		} else {
			check.Errors = append(check.Errors, fmt.Sprintf("Failed to load metadata: %v", err))
		}
	}

	return check, nil
}

// GetActiveModel returns the active model for a camera
// This method implements web.ModelLoaderService interface
func (ml *ModelLoader) GetActiveModel(cameraID string) (*ActiveModelInfo, bool) {
	model, exists := ml.activeModels[cameraID]
	if !exists {
		return nil, false
	}
	return model.ToActiveModelInfo(), true
}

// DeactivateModel deactivates a model for a camera
func (ml *ModelLoader) DeactivateModel(ctx context.Context, cameraID string) error {
	if model, exists := ml.activeModels[cameraID]; exists {
		// Update status in database
		if err := ml.modelStorage.UpdateModelStatus(ctx, model.ModelID, "inactive"); err != nil {
			ml.logger.Warn("Failed to update model status to inactive",
				"model_id", model.ModelID,
				"error", err,
			)
		}

		delete(ml.activeModels, cameraID)

		ml.logger.Info("Model deactivated",
			"model_id", model.ModelID,
			"camera_id", cameraID,
		)
	}

	return nil
}

// ListActiveModels returns all currently active models
func (ml *ModelLoader) ListActiveModels() map[string]*ActiveModel {
	result := make(map[string]*ActiveModel)
	for k, v := range ml.activeModels {
		result[k] = v
	}
	return result
}

// notifyAIService notifies the AI service about a new model
// For PoC, this is a placeholder - actual implementation will be in future epic
func (ml *ModelLoader) notifyAIService(ctx context.Context, model *ActiveModel) {
	// For PoC, we log that the model is ready
	// In future epic, this will:
	// 1. Send HTTP request to AI service to load the model
	// 2. AI service loads ONNX model using OpenVINO runtime
	// 3. AI service validates model compatibility
	// 4. AI service confirms model is ready for inference

	ml.logger.Info("Model ready for AI service",
		"model_id", model.ModelID,
		"model_path", model.ModelPath,
		"camera_id", model.CameraID,
		"version", model.Version,
		"model_type", model.ModelType,
	)

	// TODO: In future epic, implement HTTP call to AI service:
	// POST /api/v1/models/load
	// {
	//   "model_id": "...",
	//   "model_path": "...",
	//   "metadata_path": "...",
	//   "camera_id": "...",
	//   "version": "..."
	// }
}

// SwitchModel switches to a different model for a camera
func (ml *ModelLoader) SwitchModel(ctx context.Context, newModelID string, cameraID *string) (*ActiveModelInfo, error) {
	if cameraID == nil {
		return nil, fmt.Errorf("camera_id is required for model switching")
	}

	// Load new model (this will automatically deactivate the old one)
	return ml.LoadModel(ctx, newModelID, cameraID)
}

// IsModelReady checks if a model is loaded and ready for inference
func (ml *ModelLoader) IsModelReady(cameraID string) bool {
	model, exists := ml.activeModels[cameraID]
	return exists && model != nil && model.Ready
}
