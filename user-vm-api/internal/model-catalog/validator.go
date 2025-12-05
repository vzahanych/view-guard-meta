package modelcatalog

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
)

// Edge model constraints for trained models that will be deployed to Edge
const (
	// EdgeMaxModelSizeBytes is the maximum model size (50MB) for Edge miniPC with limited storage
	EdgeMaxModelSizeBytes = 50 * 1024 * 1024 // 50MB
	// EdgeSupportedInputHeight is the standard input height for YOLOv8 models (640 pixels)
	EdgeSupportedInputHeight = 640
	// EdgeSupportedInputWidth is the standard input width for YOLOv8 models (640 pixels)
	EdgeSupportedInputWidth = 640
	// EdgeSupportedChannels is the number of input channels (RGB = 3)
	EdgeSupportedChannels = 3
	// EdgeSupportedBatchSize is the batch size for inference (1 for single frame processing)
	EdgeSupportedBatchSize = 1
)

// EdgeSupportedFormats lists supported model formats for Edge deployment
var EdgeSupportedFormats = []string{
	"onnx",     // ONNX format (for training output, will be converted to OpenVINO IR during deployment)
	"openvino", // OpenVINO IR format (for Edge deployment, converted from ONNX)
}

// EdgeSupportedFrameworks lists supported inference frameworks on Edge
var EdgeSupportedFrameworks = []string{
	"openvino",    // Primary: OpenVINO (optimized for Intel CPUs/iGPUs)
	"onnxruntime", // Fallback: ONNX Runtime (if OpenVINO not supported / future ARM SKU)
}

// ModelValidator validates models for Edge compatibility and correctness
type ModelValidator struct {
	modelStorage *storage.ModelStorage
}

// NewModelValidator creates a new model validator
func NewModelValidator(modelStorage *storage.ModelStorage) *ModelValidator {
	return &ModelValidator{
		modelStorage: modelStorage,
	}
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ValidateModel validates a model for Edge compatibility and correctness
// This performs comprehensive validation including:
// - ONNX model structure (file exists, readable, valid ONNX format)
// - Model size (within Edge constraints)
// - Input/output shapes (compatible with Edge preprocessing)
// - Metadata completeness
func (mv *ModelValidator) ValidateModel(modelID string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check if model exists
	if !mv.modelStorage.ModelExists(modelID) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model %s does not exist", modelID))
		return result, nil
	}

	// Get model info
	modelInfo, err := mv.modelStorage.GetModelInfo(modelID)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get model info: %v", err))
		return result, nil
	}

	// Validate model size (within Edge constraints)
	if err := mv.modelStorage.ValidateModelSize(modelID); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model size validation failed: %v", err))
	}

	// Validate model format (ONNX structure)
	if err := mv.modelStorage.ValidateModelFormat(modelID); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model format validation failed: %v", err))
	}

	// Validate metadata completeness
	if err := mv.validateMetadataCompleteness(modelInfo.Metadata); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("metadata validation failed: %v", err))
	}

	// Validate input/output shapes (compatible with Edge preprocessing)
	if err := mv.validateInputShapes(modelInfo.Metadata); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("input shape validation failed: %v", err))
	}

	// Check for warnings (non-critical issues)
	if modelInfo.SizeBytes > storage.MaxModelSizeBytes*8/10 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("model size (%d bytes) is approaching the maximum limit (%d bytes)", modelInfo.SizeBytes, storage.MaxModelSizeBytes))
	}

	return result, nil
}

// ValidateModelData validates model data before storage
// This is used to validate models before they are stored
func (mv *ModelValidator) ValidateModelData(modelData []byte, metadata *storage.ModelMetadata) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Validate model size
	modelSize := int64(len(modelData))
	if modelSize > storage.MaxModelSizeBytes {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model size (%d bytes) exceeds maximum allowed size (%d bytes)", modelSize, storage.MaxModelSizeBytes))
	}

	// Validate model format based on framework
	if metadata != nil {
		// Create a temporary ModelStorage to use its validation methods
		// For format validation, we'll do basic checks here
		if len(modelData) == 0 {
			result.Valid = false
			result.Errors = append(result.Errors, "model data is empty")
		}

		// Basic ONNX format check (for onnx/onnxruntime frameworks)
		if metadata.Framework == "onnx" || metadata.Framework == "onnxruntime" {
			if len(modelData) < 1024 {
				result.Valid = false
				result.Errors = append(result.Errors, "ONNX model appears too small (minimum expected ~1KB)")
			}
		}

		// Validate metadata completeness
		if err := mv.validateMetadataCompleteness(metadata); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("metadata validation failed: %v", err))
		}

		// Validate input shapes
		if err := mv.validateInputShapes(metadata); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("input shape validation failed: %v", err))
		}
	}

	// Check for warnings
	if modelSize > storage.MaxModelSizeBytes*8/10 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("model size (%d bytes) is approaching the maximum limit (%d bytes)", modelSize, storage.MaxModelSizeBytes))
	}

	return result, nil
}

// validateMetadataCompleteness validates that required metadata fields are present
func (mv *ModelValidator) validateMetadataCompleteness(metadata *storage.ModelMetadata) error {
	if metadata == nil {
		return fmt.Errorf("metadata is nil")
	}

	// Required fields
	if metadata.ModelID == "" {
		return fmt.Errorf("model_id is required")
	}
	if metadata.Version == "" {
		return fmt.Errorf("version is required")
	}
	if metadata.ModelType == "" {
		return fmt.Errorf("model_type is required")
	}
	if len(metadata.InputShape) == 0 {
		return fmt.Errorf("input_shape is required")
	}
	if metadata.Framework == "" {
		return fmt.Errorf("framework is required")
	}
	if metadata.ONNXFile == "" {
		return fmt.Errorf("onnx_file is required")
	}

	// Validate framework value
	validFrameworks := map[string]bool{
		"onnx":        true,
		"onnxruntime": true,
		"openvino":    true,
	}
	if !validFrameworks[metadata.Framework] {
		return fmt.Errorf("invalid framework: %s (must be one of: onnx, onnxruntime, openvino)", metadata.Framework)
	}

	// Validate model type
	validModelTypes := map[string]bool{
		"yolo": true,
		"cae":  true,
	}
	if !validModelTypes[metadata.ModelType] {
		return fmt.Errorf("invalid model_type: %s (must be one of: yolo, cae)", metadata.ModelType)
	}

	return nil
}

// validateInputShapes validates that input shapes are compatible with Edge preprocessing
// Edge expects YOLOv8 standard input shape: [1, 3, 640, 640]
func (mv *ModelValidator) validateInputShapes(metadata *storage.ModelMetadata) error {
	if metadata == nil {
		return fmt.Errorf("metadata is nil")
	}

	if len(metadata.InputShape) == 0 {
		return fmt.Errorf("input_shape is required")
	}

	// For YOLO models, validate standard input shape
	if metadata.ModelType == "yolo" {
		expectedShape := []int{1, 3, 640, 640}
		if len(metadata.InputShape) != len(expectedShape) {
			return fmt.Errorf("invalid input_shape length for YOLO model: expected %d dimensions, got %d", len(expectedShape), len(metadata.InputShape))
		}

		// Check batch dimension (should be 1 for inference)
		if metadata.InputShape[0] != 1 {
			return fmt.Errorf("invalid batch size for YOLO model: expected 1, got %d", metadata.InputShape[0])
		}

		// Check channels (should be 3 for RGB)
		if metadata.InputShape[1] != 3 {
			return fmt.Errorf("invalid channel count for YOLO model: expected 3, got %d", metadata.InputShape[1])
		}

		// Check height and width (should be 640x640 for YOLOv8)
		if metadata.InputShape[2] != 640 || metadata.InputShape[3] != 640 {
			return fmt.Errorf("invalid image dimensions for YOLO model: expected 640x640, got %dx%d", metadata.InputShape[2], metadata.InputShape[3])
		}
	}

	// For CAE models, input shape validation is more flexible
	// We just check that it's a valid shape (non-empty, positive values)
	if metadata.ModelType == "cae" {
		for i, dim := range metadata.InputShape {
			if dim <= 0 {
				return fmt.Errorf("invalid input_shape dimension %d for CAE model: must be positive, got %d", i, dim)
			}
		}
	}

	return nil
}

// ValidateEdgeCompatibility validates that a model is compatible with Edge deployment constraints
// This performs specific checks for Edge hardware limitations:
// - Model size < 50MB (Edge miniPC storage constraint)
// - Model format is ONNX (training output, will be converted to OpenVINO IR during deployment)
// - Input shape matches Edge preprocessing (640x640 for YOLOv8)
// - Framework is supported (OpenVINO primary, ONNX Runtime fallback)
// - Logs warnings for models approaching size limits
func (mv *ModelValidator) ValidateEdgeCompatibility(modelID string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check if model exists
	if !mv.modelStorage.ModelExists(modelID) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model %s does not exist", modelID))
		return result, nil
	}

	// Get model info
	modelInfo, err := mv.modelStorage.GetModelInfo(modelID)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get model info: %v", err))
		return result, nil
	}

	metadata := modelInfo.Metadata

	// Edge Constraint 1: Model size < 50MB
	if modelInfo.SizeBytes > EdgeMaxModelSizeBytes {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model size (%d bytes) exceeds Edge maximum (%d bytes)", modelInfo.SizeBytes, EdgeMaxModelSizeBytes))
	} else if modelInfo.SizeBytes > EdgeMaxModelSizeBytes*8/10 {
		// Warning if approaching limit (80% of max)
		result.Warnings = append(result.Warnings, fmt.Sprintf("model size (%d bytes) is approaching Edge maximum limit (%d bytes)", modelInfo.SizeBytes, EdgeMaxModelSizeBytes))
	}

	// Edge Constraint 2: Model format must be ONNX (training output)
	// Note: OpenVINO IR conversion happens during Edge deployment (future epic)
	if metadata.Framework != "onnx" && metadata.Framework != "onnxruntime" {
		// For training output, we expect ONNX format
		// OpenVINO IR is created during deployment, not during training
		result.Warnings = append(result.Warnings, fmt.Sprintf("model framework is %s, but Edge deployment expects ONNX format (will be converted to OpenVINO IR during deployment)", metadata.Framework))
	}

	// Edge Constraint 3: Input shape must match Edge preprocessing (640x640 for YOLOv8)
	if metadata.ModelType == "yolo" {
		if len(metadata.InputShape) != 4 {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("YOLO model input_shape must have 4 dimensions [batch, channels, height, width], got %d dimensions", len(metadata.InputShape)))
		} else {
			if metadata.InputShape[0] != EdgeSupportedBatchSize {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("YOLO model batch size must be %d for Edge inference, got %d", EdgeSupportedBatchSize, metadata.InputShape[0]))
			}
			if metadata.InputShape[1] != EdgeSupportedChannels {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("YOLO model channel count must be %d (RGB), got %d", EdgeSupportedChannels, metadata.InputShape[1]))
			}
			if metadata.InputShape[2] != EdgeSupportedInputHeight || metadata.InputShape[3] != EdgeSupportedInputWidth {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("YOLO model input dimensions must be %dx%d for Edge preprocessing, got %dx%d", EdgeSupportedInputHeight, EdgeSupportedInputWidth, metadata.InputShape[2], metadata.InputShape[3]))
			}
		}
	}

	// Edge Constraint 4: Framework must be supported (OpenVINO primary, ONNX Runtime fallback)
	frameworkSupported := false
	for _, fw := range EdgeSupportedFrameworks {
		if metadata.Framework == fw {
			frameworkSupported = true
			break
		}
	}
	if !frameworkSupported {
		// Note: ONNX models are converted to OpenVINO IR during deployment
		// So we accept "onnx" as a valid format for training output
		if metadata.Framework != "onnx" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("model framework %s is not in Edge supported frameworks list (%v). Model should be ONNX format for training output.", metadata.Framework, EdgeSupportedFrameworks))
		}
	}

	return result, nil
}

// ValidateEdgeCompatibilityData validates model data for Edge compatibility before storage
// This is used to validate trained models immediately after training, before they are stored
func (mv *ModelValidator) ValidateEdgeCompatibilityData(modelData []byte, metadata *storage.ModelMetadata) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	if metadata == nil {
		result.Valid = false
		result.Errors = append(result.Errors, "metadata is required for Edge compatibility validation")
		return result, nil
	}

	// Edge Constraint 1: Model size < 50MB
	modelSize := int64(len(modelData))
	if modelSize > EdgeMaxModelSizeBytes {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model size (%d bytes) exceeds Edge maximum (%d bytes)", modelSize, EdgeMaxModelSizeBytes))
	} else if modelSize > EdgeMaxModelSizeBytes*8/10 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("model size (%d bytes) is approaching Edge maximum limit (%d bytes)", modelSize, EdgeMaxModelSizeBytes))
	}

	// Edge Constraint 2: Model format must be ONNX (training output)
	if metadata.Framework != "onnx" && metadata.Framework != "onnxruntime" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("model framework is %s, but Edge deployment expects ONNX format (will be converted to OpenVINO IR during deployment)", metadata.Framework))
	}

	// Edge Constraint 3: Input shape must match Edge preprocessing (640x640 for YOLOv8)
	if metadata.ModelType == "yolo" {
		if len(metadata.InputShape) != 4 {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("YOLO model input_shape must have 4 dimensions [batch, channels, height, width], got %d dimensions", len(metadata.InputShape)))
		} else {
			if metadata.InputShape[0] != EdgeSupportedBatchSize {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("YOLO model batch size must be %d for Edge inference, got %d", EdgeSupportedBatchSize, metadata.InputShape[0]))
			}
			if metadata.InputShape[1] != EdgeSupportedChannels {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("YOLO model channel count must be %d (RGB), got %d", EdgeSupportedChannels, metadata.InputShape[1]))
			}
			if metadata.InputShape[2] != EdgeSupportedInputHeight || metadata.InputShape[3] != EdgeSupportedInputWidth {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("YOLO model input dimensions must be %dx%d for Edge preprocessing, got %dx%d", EdgeSupportedInputHeight, EdgeSupportedInputWidth, metadata.InputShape[2], metadata.InputShape[3]))
			}
		}
	}

	// Edge Constraint 4: Framework validation (already handled above)

	return result, nil
}
