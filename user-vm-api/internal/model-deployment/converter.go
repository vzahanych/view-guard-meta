package modeldeployment

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	"go.uber.org/zap"
)

const (
	// EdgeMaxModelSizeBytes is the maximum allowed model size for Edge deployment (50MB)
	EdgeMaxModelSizeBytes = 50 * 1024 * 1024 // 50MB
)

// ModelConverter handles model format validation and conversion for Edge deployment
type ModelConverter struct {
	modelStorage *storage.ModelStorage
	modelCatalog *modelcatalog.ModelCatalog
	logger       *logging.Logger
}

// NewModelConverter creates a new model converter for Edge deployment
func NewModelConverter(
	modelStorage *storage.ModelStorage,
	modelCatalog *modelcatalog.ModelCatalog,
	logger *logging.Logger,
) (*ModelConverter, error) {
	if modelStorage == nil {
		return nil, fmt.Errorf("model storage is required")
	}
	if modelCatalog == nil {
		return nil, fmt.Errorf("model catalog is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &ModelConverter{
		modelStorage: modelStorage,
		modelCatalog: modelCatalog,
		logger:       logger,
	}, nil
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid      bool     `json:"valid"`
	Errors     []string `json:"errors,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
	ModelSize  int64    `json:"model_size,omitempty"`
	ModelFormat string  `json:"model_format,omitempty"`
}

// PrepareModelForDeployment validates and prepares a model for Edge deployment
// For PoC: Validates ONNX format and size, returns model as-is (no conversion)
// Future: Will convert ONNX to OpenVINO IR format for better Edge performance
func (mc *ModelConverter) PrepareModelForDeployment(modelID string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Step 1: Verify model exists
	if !mc.modelStorage.ModelExists(modelID) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("model %s does not exist", modelID))
		return result, nil
	}

	// Step 2: Get model info
	modelInfo, err := mc.modelStorage.GetModelInfo(modelID)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get model info: %v", err))
		return result, nil
	}

	result.ModelSize = modelInfo.SizeBytes
	result.ModelFormat = modelInfo.Metadata.Framework

	// Step 3: Validate model format (must be ONNX for PoC)
	if err := mc.validateModelFormat(modelInfo, result); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
		return result, nil
	}

	// Step 4: Validate model size (must be ≤50MB for Edge)
	if err := mc.validateModelSize(modelInfo, result); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
		return result, nil
	}

	// Step 5: Validate model metadata
	if err := mc.validateModelMetadata(modelInfo, result); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
		return result, nil
	}

	// For PoC: No conversion needed, ONNX models are deployed directly
	// Edge AI Service (OpenVINO) can load ONNX models
	mc.logger.Info("Model validated for Edge deployment",
		zap.String("model_id", modelID),
		zap.String("format", result.ModelFormat),
		zap.Int64("size_bytes", result.ModelSize),
	)

	return result, nil
}

// validateModelFormat validates that the model is in ONNX format
func (mc *ModelConverter) validateModelFormat(modelInfo *storage.ModelInfo, result *ValidationResult) error {
	metadata := modelInfo.Metadata

	// For PoC, we expect ONNX format (training pipeline outputs ONNX)
	if metadata.Framework != "onnx" && metadata.Framework != "onnxruntime" {
		return fmt.Errorf("model format must be ONNX for Edge deployment (current: %s)", metadata.Framework)
	}

	// Verify ONNX file exists
	if metadata.ONNXFile == "" {
		return fmt.Errorf("ONNX file not specified in model metadata")
	}

	// Check if ONNX file exists
	modelPath := mc.modelStorage.GetModelPath(modelInfo.ModelID)
	onnxPath := fmt.Sprintf("%s/%s", modelPath, metadata.ONNXFile)
	if !mc.modelStorage.ModelExists(modelInfo.ModelID) {
		return fmt.Errorf("ONNX file not found: %s", onnxPath)
	}

	return nil
}

// validateModelSize validates that the model size is within Edge constraints
func (mc *ModelConverter) validateModelSize(modelInfo *storage.ModelInfo, result *ValidationResult) error {
	size := modelInfo.SizeBytes

	// Edge constraint: Model size must be ≤50MB
	if size > EdgeMaxModelSizeBytes {
		return fmt.Errorf("model size (%d bytes) exceeds Edge maximum (%d bytes)", size, EdgeMaxModelSizeBytes)
	}

	// Warning if approaching limit (80% of max)
	if size > EdgeMaxModelSizeBytes*8/10 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("model size (%d bytes) is approaching Edge maximum limit (%d bytes)", size, EdgeMaxModelSizeBytes))
	}

	return nil
}

// validateModelMetadata validates model metadata for Edge compatibility
func (mc *ModelConverter) validateModelMetadata(modelInfo *storage.ModelInfo, result *ValidationResult) error {
	metadata := modelInfo.Metadata

	// Validate input shape (required for Edge preprocessing)
	if len(metadata.InputShape) == 0 {
		return fmt.Errorf("model metadata missing input_shape")
	}

	// For YOLOv8 models, validate input shape is [1, 3, 640, 640]
	if metadata.ModelType == "yolov8n" || metadata.ModelType == "yolo" {
		expectedShape := []int{1, 3, 640, 640}
		if len(metadata.InputShape) != len(expectedShape) {
			return fmt.Errorf("invalid input shape length for YOLOv8 model: expected %d, got %d",
				len(expectedShape), len(metadata.InputShape))
		}
		for i, dim := range expectedShape {
			if metadata.InputShape[i] != dim {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("input shape dimension %d: expected %d, got %d (may cause preprocessing issues)",
						i, dim, metadata.InputShape[i]))
			}
		}
	}

	// Validate preprocessing configuration (if present)
	if metadata.Preprocessing != nil {
		// Check for image_size in preprocessing
		if imageSize, ok := metadata.Preprocessing["image_size"].(float64); ok {
			// For YOLOv8, image_size should be 640
			if metadata.ModelType == "yolov8n" || metadata.ModelType == "yolo" {
				if int(imageSize) != 640 {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("preprocessing image_size (%v) does not match expected YOLOv8 size (640)", imageSize))
				}
			}
		}
	}

	// Validate model type is supported
	supportedTypes := []string{"yolov8n", "yolo", "cae"}
	isSupported := false
	for _, t := range supportedTypes {
		if metadata.ModelType == t {
			isSupported = true
			break
		}
	}
	if !isSupported {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("model type %s may not be fully supported on Edge (supported: %v)", metadata.ModelType, supportedTypes))
	}

	return nil
}

// ConvertONNXToOpenVINO converts an ONNX model to OpenVINO IR format
// NOTE: This is deferred to post-PoC. For PoC, models are deployed as ONNX.
// Future implementation will:
// - Use OpenVINO Model Optimizer (mo) to convert ONNX to IR (.xml + .bin)
// - Optimize for target Edge hardware (CPU, iGPU)
// - Store converted model alongside ONNX model
func (mc *ModelConverter) ConvertONNXToOpenVINO(modelID string, targetDevice string) error {
	// For PoC, return error indicating conversion is not implemented
	return fmt.Errorf("ONNX to OpenVINO IR conversion is deferred to post-PoC. For PoC, models are deployed as ONNX format")
}

// OptimizeModel applies optimization techniques to reduce model size
// NOTE: This is deferred to post-PoC. For PoC, models are deployed as-is.
// Future optimization techniques:
// - FP16 quantization (reduce model size, maintain accuracy)
// - Model pruning (remove unnecessary weights)
// - ONNX graph optimization (fuse operations, remove unused nodes)
func (mc *ModelConverter) OptimizeModel(modelID string, optimizationType string) error {
	// For PoC, return error indicating optimization is not implemented
	return fmt.Errorf("model optimization is deferred to post-PoC. For PoC, models are deployed as-is")
}

// QuantizeModel applies quantization to reduce model size
// NOTE: Deferred to post-PoC
// Future implementation will:
// - Apply FP16 quantization (half precision)
// - Validate accuracy after quantization
// - Store quantized model alongside original
func (mc *ModelConverter) QuantizeModel(modelID string, precision string) error {
	// For PoC, return error indicating quantization is not implemented
	return fmt.Errorf("model quantization is deferred to post-PoC")
}

