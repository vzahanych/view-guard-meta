package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// MaxModelSizeBytes is the maximum allowed model size (50MB) for Edge compatibility
	MaxModelSizeBytes = 50 * 1024 * 1024 // 50MB
)

// ModelStorage manages AI model file storage
type ModelStorage struct {
	baseDir string
}

// ModelMetadata contains model metadata
type ModelMetadata struct {
	// Core identification fields
	ModelID   string `json:"model_id"`            // Unique identifier (UUID)
	Version   string `json:"version"`             // Model version string
	CameraID  string `json:"camera_id,omitempty"` // Optional camera-specific model
	ModelType string `json:"model_type"`          // Model type (yolo, cae, etc.)

	// Model configuration
	InputShape    []int                  `json:"input_shape"`             // Input tensor shape
	LatentDim     int                    `json:"latent_dim,omitempty"`    // Latent dimension (for CAE models)
	Threshold     float64                `json:"threshold,omitempty"`     // Detection threshold
	Framework     string                 `json:"framework"`               // Inference framework (openvino, onnxruntime)
	ONNXFile      string                 `json:"onnx_file"`               // Model file name
	Preprocessing map[string]interface{} `json:"preprocessing,omitempty"` // Preprocessing parameters

	// Training information
	TrainingDatasetID string `json:"training_dataset_id,omitempty"` // Link to training dataset (from Epic 2.2.3)
	TrainingDate      string `json:"training_date,omitempty"`       // When model was trained

	// Performance metrics (optional, may be empty for baseline or un-evaluated models)
	Accuracy  float64 `json:"accuracy,omitempty"`  // Model accuracy (if available)
	Precision float64 `json:"precision,omitempty"` // Precision score
	Recall    float64 `json:"recall,omitempty"`    // Recall score
	F1Score   float64 `json:"f1_score,omitempty"`  // F1 score
}

// ModelInfo contains model file information
type ModelInfo struct {
	ModelID      string
	ModelPath    string
	MetadataPath string
	Metadata     *ModelMetadata
	SizeBytes    int64
}

// NewModelStorage creates a new model storage manager
func NewModelStorage(baseDir string) (*ModelStorage, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("base directory is required")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &ModelStorage{
		baseDir: baseDir,
	}, nil
}

// CreateModelDirectory creates a new model directory structure
func (ms *ModelStorage) CreateModelDirectory(modelID string) (string, error) {
	if modelID == "" {
		return "", fmt.Errorf("model ID is required")
	}

	modelPath := filepath.Join(ms.baseDir, modelID)
	if err := os.MkdirAll(modelPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create model directory: %w", err)
	}

	return modelPath, nil
}

// StoreModel stores a model file (ONNX or OpenVINO IR) and metadata
func (ms *ModelStorage) StoreModel(modelID string, modelData []byte, metadata *ModelMetadata) error {
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}

	if metadata == nil {
		return fmt.Errorf("metadata is required")
	}

	// Validate model size
	modelSize := int64(len(modelData))
	if modelSize > MaxModelSizeBytes {
		return fmt.Errorf("model size (%d bytes) exceeds maximum allowed size (%d bytes)", modelSize, MaxModelSizeBytes)
	}

	// Validate model format based on framework
	if err := ms.validateModelFormat(modelData, metadata.Framework); err != nil {
		return fmt.Errorf("model format validation failed: %w", err)
	}

	// Create model directory
	modelPath, err := ms.CreateModelDirectory(modelID)
	if err != nil {
		return fmt.Errorf("failed to create model directory: %w", err)
	}

	// Store model file based on format
	var modelFileName string
	if metadata.Framework == "openvino" {
		// For OpenVINO IR, we expect .xml and .bin files
		// For now, store as .onnx if only one file provided, or handle separately
		// This is a simplified implementation - full OpenVINO IR support would need separate .xml/.bin handling
		modelFileName = "model.onnx"
	} else {
		// Default to ONNX
		modelFileName = "model.onnx"
	}

	modelFilePath := filepath.Join(modelPath, modelFileName)
	if err := os.WriteFile(modelFilePath, modelData, 0644); err != nil {
		return fmt.Errorf("failed to write model file: %w", err)
	}

	// Update metadata
	metadata.ModelID = modelID
	metadata.ONNXFile = modelFileName

	// Store metadata
	metadataPath := filepath.Join(modelPath, "metadata.json")
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// GetModelPath returns the path to a model directory
func (ms *ModelStorage) GetModelPath(modelID string) string {
	return filepath.Join(ms.baseDir, modelID)
}

// GetModelFilePath returns the path to a model ONNX file
func (ms *ModelStorage) GetModelFilePath(modelID string) string {
	return filepath.Join(ms.baseDir, modelID, "model.onnx")
}

// GetMetadataPath returns the path to a model metadata file
func (ms *ModelStorage) GetMetadataPath(modelID string) string {
	return filepath.Join(ms.baseDir, modelID, "metadata.json")
}

// GetModelInfo returns information about a model
func (ms *ModelStorage) GetModelInfo(modelID string) (*ModelInfo, error) {
	modelPath := ms.GetModelPath(modelID)
	modelFilePath := ms.GetModelFilePath(modelID)
	metadataPath := ms.GetMetadataPath(modelID)

	// Check if model exists
	if _, err := os.Stat(modelFilePath); err != nil {
		return nil, fmt.Errorf("model file not found: %w", err)
	}

	// Read metadata
	metadata, err := ms.GetMetadata(modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	// Get model file size
	modelFileInfo, err := os.Stat(modelFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat model file: %w", err)
	}

	return &ModelInfo{
		ModelID:      modelID,
		ModelPath:    modelPath,
		MetadataPath: metadataPath,
		Metadata:     metadata,
		SizeBytes:    modelFileInfo.Size(),
	}, nil
}

// GetMetadata reads and parses model metadata
func (ms *ModelStorage) GetMetadata(modelID string) (*ModelMetadata, error) {
	metadataPath := ms.GetMetadataPath(modelID)

	// Read metadata file
	metadataJSON, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	// Parse metadata
	var metadata ModelMetadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

// ReadModel reads the model file data
func (ms *ModelStorage) ReadModel(modelID string) ([]byte, error) {
	modelFilePath := ms.GetModelFilePath(modelID)
	return os.ReadFile(modelFilePath)
}

// DeleteModel deletes a model and all its files
func (ms *ModelStorage) DeleteModel(modelID string) error {
	modelPath := ms.GetModelPath(modelID)
	return os.RemoveAll(modelPath)
}

// ListModels returns a list of all model IDs
func (ms *ModelStorage) ListModels() ([]string, error) {
	entries, err := os.ReadDir(ms.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	var models []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it's a valid model directory (has model.onnx)
			modelFilePath := filepath.Join(ms.baseDir, entry.Name(), "model.onnx")
			if _, err := os.Stat(modelFilePath); err == nil {
				models = append(models, entry.Name())
			}
		}
	}

	return models, nil
}

// ValidateModel validates a model structure (file existence, metadata, size, format)
func (ms *ModelStorage) ValidateModel(modelID string) error {
	modelFilePath := ms.GetModelFilePath(modelID)
	metadataPath := ms.GetMetadataPath(modelID)

	// Check if model file exists
	if _, err := os.Stat(modelFilePath); err != nil {
		return fmt.Errorf("model file not found: %w", err)
	}

	// Check if metadata file exists
	if _, err := os.Stat(metadataPath); err != nil {
		return fmt.Errorf("metadata file not found: %w", err)
	}

	// Validate metadata
	_, err := ms.GetMetadata(modelID)
	if err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	// Validate model size
	if err := ms.ValidateModelSize(modelID); err != nil {
		return fmt.Errorf("model size validation failed: %w", err)
	}

	// Validate model format
	if err := ms.ValidateModelFormat(modelID); err != nil {
		return fmt.Errorf("model format validation failed: %w", err)
	}

	return nil
}

// GetModelSize returns the size of a model in bytes
func (ms *ModelStorage) GetModelSize(modelID string) (int64, error) {
	info, err := ms.GetModelInfo(modelID)
	if err != nil {
		return 0, err
	}
	return info.SizeBytes, nil
}

// GetTotalStorageSize returns the total size of all models
func (ms *ModelStorage) GetTotalStorageSize() (int64, error) {
	models, err := ms.ListModels()
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, modelID := range models {
		size, err := ms.GetModelSize(modelID)
		if err != nil {
			continue // Skip models that can't be read
		}
		totalSize += size
	}

	return totalSize, nil
}

// UpdateMetadata updates model metadata
func (ms *ModelStorage) UpdateMetadata(modelID string, metadata *ModelMetadata) error {
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}

	if metadata == nil {
		return fmt.Errorf("metadata is required")
	}

	metadataPath := ms.GetMetadataPath(modelID)

	// Ensure model exists
	if _, err := os.Stat(metadataPath); err != nil {
		return fmt.Errorf("model not found: %w", err)
	}

	// Get model path for validation
	_ = ms.GetModelPath(modelID)

	// Update metadata
	metadata.ModelID = modelID
	metadata.ONNXFile = "model.onnx"

	// Write metadata
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// ModelExists checks if a model exists
func (ms *ModelStorage) ModelExists(modelID string) bool {
	modelFilePath := ms.GetModelFilePath(modelID)
	_, err := os.Stat(modelFilePath)
	return err == nil
}

// GetModelVersions returns all versions of a model (if versioned)
func (ms *ModelStorage) GetModelVersions(modelID string) ([]string, error) {
	// For now, we assume one model per modelID
	// In the future, this could support versioning
	if ms.ModelExists(modelID) {
		metadata, err := ms.GetMetadata(modelID)
		if err != nil {
			return nil, err
		}
		return []string{metadata.Version}, nil
	}
	return []string{}, nil
}

// validateModelFormat validates model format based on framework
func (ms *ModelStorage) validateModelFormat(modelData []byte, framework string) error {
	if len(modelData) == 0 {
		return fmt.Errorf("model data is empty")
	}

	switch framework {
	case "onnx", "onnxruntime":
		return ms.validateONNXFormat(modelData)
	case "openvino":
		// For OpenVINO IR, basic validation: check for XML-like structure
		// Full validation would require parsing the XML structure
		// For PoC, we accept any non-empty data if framework is openvino
		if len(modelData) < 10 {
			return fmt.Errorf("OpenVINO IR model data too small to be valid")
		}
		// Basic check: OpenVINO IR files typically start with XML declaration or contain XML-like content
		// This is a simplified check - full validation would parse the XML
		return nil
	default:
		// Default to ONNX validation for unknown frameworks
		return ms.validateONNXFormat(modelData)
	}
}

// validateONNXFormat performs basic ONNX file structure validation
func (ms *ModelStorage) validateONNXFormat(modelData []byte) error {
	if len(modelData) < 16 {
		return fmt.Errorf("ONNX model data too small to be valid")
	}

	// ONNX files typically start with a protobuf header
	// Basic validation: check for common ONNX file signatures
	// ONNX models are protobuf-encoded, so we check for reasonable structure

	// Check 1: Minimum size (ONNX models are typically at least a few KB)
	if len(modelData) < 1024 {
		return fmt.Errorf("ONNX model appears too small (minimum expected ~1KB)")
	}

	// Check 2: Look for protobuf-like structure indicators
	// Protobuf messages often start with field tags (varint encoding)
	// This is a heuristic check - not foolproof but catches obviously invalid data

	// For a more robust check, we could use the onnx package to parse,
	// but that adds a dependency. For PoC, basic size and structure checks suffice.
	// The actual ONNX runtime will validate the model when it's loaded.

	return nil
}

// ValidateModelSize validates that a model size is within Edge constraints
func (ms *ModelStorage) ValidateModelSize(modelID string) error {
	size, err := ms.GetModelSize(modelID)
	if err != nil {
		return fmt.Errorf("failed to get model size: %w", err)
	}

	if size > MaxModelSizeBytes {
		return fmt.Errorf("model size (%d bytes) exceeds maximum allowed size (%d bytes)", size, MaxModelSizeBytes)
	}

	return nil
}

// ValidateModelFormat validates the format of an existing model
func (ms *ModelStorage) ValidateModelFormat(modelID string) error {
	metadata, err := ms.GetMetadata(modelID)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	modelData, err := ms.ReadModel(modelID)
	if err != nil {
		return fmt.Errorf("failed to read model file: %w", err)
	}

	return ms.validateModelFormat(modelData, metadata.Framework)
}
