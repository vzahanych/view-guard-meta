package processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// InferenceService handles model inference for event detection
type InferenceService struct {
	*service.ServiceBase
	logger            *logger.Logger
	modelLoader        ModelLoaderService
	aiClient          *ai.Client
	onDetection       func(*InferenceResult) // Callback for inference results
	mu                sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	confidenceThreshold float64
	enabledClasses     []string
}

// ModelLoaderService interface for getting active models
type ModelLoaderService interface {
	GetActiveModel(cameraID string) (*ai.ActiveModelInfo, error)
	IsModelReady(cameraID string) bool
}

// InferenceServiceConfig contains inference service configuration
type InferenceServiceConfig struct {
	ModelLoader         ModelLoaderService
	AIClient           *ai.Client
	ConfidenceThreshold float64
	EnabledClasses     []string
	OnDetection        func(*InferenceResult) // Callback for inference results
}

// InferenceResult contains the result of an inference operation
type InferenceResult struct {
	CameraID          string
	Frame             *video.Frame
	ModelID           string
	Detections        []Detection
	InferenceTimeMs   int64
	RequestDurationMs int64
	Timestamp         time.Time
	Error             error
}

// Detection represents a single detection from inference
type Detection struct {
	ClassID     int        // Class ID
	ClassName   string     // Class name
	Confidence  float64    // Confidence score (0.0-1.0)
	BoundingBox BoundingBox
	EventType   string     // Event type classification (normal, anomaly, etc.)
}

// BoundingBox represents a bounding box for a detection
type BoundingBox struct {
	X1         float64 // Left coordinate
	Y1         float64 // Top coordinate
	X2         float64 // Right coordinate
	Y2         float64 // Bottom coordinate
	Confidence float64 // Detection confidence (0.0 to 1.0)
	ClassID    int     // COCO class ID
	ClassName  string  // Human-readable class name
}

// NewInferenceService creates a new inference service
func NewInferenceService(config InferenceServiceConfig, log *logger.Logger) *InferenceService {
	ctx, cancel := context.WithCancel(context.Background())

	// Default confidence threshold
	confidenceThreshold := config.ConfidenceThreshold
	if confidenceThreshold == 0 {
		confidenceThreshold = 0.5 // Default: 50% confidence
	}

	return &InferenceService{
		ServiceBase:        service.NewServiceBase("inference-service", log),
		logger:             log,
		modelLoader:        config.ModelLoader,
		aiClient:           config.AIClient,
		onDetection:        config.OnDetection,
		ctx:                ctx,
		cancel:             cancel,
		confidenceThreshold: confidenceThreshold,
		enabledClasses:     config.EnabledClasses,
	}
}

// Start starts the inference service
func (is *InferenceService) Start(ctx context.Context) error {
	is.LogInfo("Inference service started",
		"confidence_threshold", is.confidenceThreshold,
		"enabled_classes", is.enabledClasses,
	)
	return nil
}

// Stop stops the inference service
func (is *InferenceService) Stop() error {
	is.cancel()
	is.LogInfo("Inference service stopped")
	return nil
}

// ProcessFrame processes a frame for inference
func (is *InferenceService) ProcessFrame(ctx context.Context, frame *video.Frame) error {
	// Get active model for this camera
	activeModel, err := is.modelLoader.GetActiveModel(frame.CameraID)
	if err != nil {
		// No active model available - skip inference or use baseline
		is.logger.Debug("No active model available for camera",
			"camera_id", frame.CameraID,
			"error", err,
		)
		
		// For PoC, skip inference if no model is available
		// In production, could fall back to baseline detection
		return fmt.Errorf("no active model for camera %s: %w", frame.CameraID, err)
	}

	// Check if model is ready
	if !is.modelLoader.IsModelReady(frame.CameraID) {
		is.logger.Warn("Model not ready for camera",
			"camera_id", frame.CameraID,
			"model_id", activeModel.ModelID,
		)
		return fmt.Errorf("model not ready for camera %s", frame.CameraID)
	}

	// Perform inference
	startTime := time.Now()
	inferenceResp, err := is.performInference(ctx, frame, activeModel)
	requestDuration := time.Since(startTime)

	// Create inference result
	result := &InferenceResult{
		CameraID:          frame.CameraID,
		Frame:             frame,
		ModelID:           activeModel.ModelID,
		InferenceTimeMs:   0,
		RequestDurationMs: requestDuration.Milliseconds(),
		Timestamp:         time.Now(),
		Error:             err,
	}

	if err != nil {
		is.logger.Warn("Inference failed",
			"camera_id", frame.CameraID,
			"model_id", activeModel.ModelID,
			"error", err,
		)
		// Still call callback with error result
		if is.onDetection != nil {
			is.onDetection(result)
		}
		return err
	}

	// Parse inference response
	result.InferenceTimeMs = int64(inferenceResp.InferenceTimeMs)
	result.Detections = is.parseDetections(inferenceResp)

	is.logger.Debug("Inference completed",
		"camera_id", frame.CameraID,
		"model_id", activeModel.ModelID,
		"detection_count", len(result.Detections),
		"inference_time_ms", result.InferenceTimeMs,
		"request_duration_ms", result.RequestDurationMs,
	)

	// Call callback with results
	if is.onDetection != nil {
		is.onDetection(result)
	}

	return nil
}

// performInference performs the actual inference request
func (is *InferenceService) performInference(ctx context.Context, frame *video.Frame, activeModel *ai.ActiveModelInfo) (*ai.InferenceResponse, error) {
	// Use AI client to perform inference
	// Frame encoding (JPEG → base64) is handled by the AI client
	var inferenceResp *ai.InferenceResponse
	var err error

	// Use confidence threshold and enabled classes from service config
	// or from model metadata if available
	confidenceThreshold := &is.confidenceThreshold
	enabledClasses := is.enabledClasses

	// Perform inference with options
	inferenceResp, err = is.aiClient.InferWithOptions(
		ctx,
		frame,
		confidenceThreshold,
		enabledClasses,
	)

	if err != nil {
		return nil, fmt.Errorf("AI service inference failed: %w", err)
	}

	return inferenceResp, nil
}

// parseDetections parses inference response into Detection objects
func (is *InferenceService) parseDetections(resp *ai.InferenceResponse) []Detection {
	if resp == nil || resp.BoundingBoxes == nil {
		return []Detection{}
	}

	detections := make([]Detection, 0, len(resp.BoundingBoxes))
	for _, bbox := range resp.BoundingBoxes {
		// Classify event type based on detection
		// For PoC, we'll use a simple classification:
		// - If confidence is high and class is not "normal", it's an event
		// - In production, this would be more sophisticated
		eventType := is.classifyEventType(bbox)

		detection := Detection{
			ClassID:    bbox.ClassID,
			ClassName:  bbox.ClassName,
			Confidence: bbox.Confidence,
			BoundingBox: BoundingBox{
				X1:         bbox.X1,
				Y1:         bbox.Y1,
				X2:         bbox.X2,
				Y2:         bbox.Y2,
				Confidence: bbox.Confidence,
				ClassID:    bbox.ClassID,
				ClassName:  bbox.ClassName,
			},
			EventType: eventType,
		}

		detections = append(detections, detection)
	}

	return detections
}

// classifyEventType classifies the event type based on detection
func (is *InferenceService) classifyEventType(bbox ai.BoundingBox) string {
	// Simple classification for PoC
	// In production, this would use model metadata, class mappings, etc.
	if bbox.Confidence >= is.confidenceThreshold {
		// High confidence detection - classify as event
		// Check if class name indicates anomaly or event
		if bbox.ClassName == "normal" || bbox.ClassName == "background" {
			return "normal"
		}
		return "event"
	}
	return "normal"
}

// SetConfidenceThreshold updates the confidence threshold
func (is *InferenceService) SetConfidenceThreshold(threshold float64) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.confidenceThreshold = threshold
	if is.aiClient != nil {
		is.aiClient.SetConfidenceThreshold(threshold)
	}
}

// SetEnabledClasses updates the enabled classes filter
func (is *InferenceService) SetEnabledClasses(classes []string) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.enabledClasses = classes
	if is.aiClient != nil {
		is.aiClient.SetEnabledClasses(classes)
	}
}

// GetConfidenceThreshold returns the current confidence threshold
func (is *InferenceService) GetConfidenceThreshold() float64 {
	is.mu.RLock()
	defer is.mu.RUnlock()
	return is.confidenceThreshold
}

// GetEnabledClasses returns the current enabled classes
func (is *InferenceService) GetEnabledClasses() []string {
	is.mu.RLock()
	defer is.mu.RUnlock()
	return is.enabledClasses
}

