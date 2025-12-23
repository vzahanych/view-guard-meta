package types

import "time"

// InferenceRequest represents a request to the AI service
type InferenceRequest struct {
	Image              string   `json:"image"`                // Base64-encoded JPEG image
	ConfidenceThreshold *float64 `json:"confidence_threshold,omitempty"` // Optional override
	EnabledClasses     []string `json:"enabled_classes,omitempty"`       // Optional filter
}

// BoundingBox represents a detected object's bounding box
type BoundingBox struct {
	X1         float64 `json:"x1"`          // Left coordinate
	Y1         float64 `json:"y1"`          // Top coordinate
	X2         float64 `json:"x2"`          // Right coordinate
	Y2         float64 `json:"y2"`          // Bottom coordinate
	Confidence float64 `json:"confidence"` // Detection confidence (0.0 to 1.0)
	ClassID    int     `json:"class_id"`   // COCO class ID
	ClassName  string  `json:"class_name"`  // Human-readable class name
}

// InferenceResponse represents the response from the AI service
type InferenceResponse struct {
	BoundingBoxes   []BoundingBox `json:"bounding_boxes"`    // Detected objects
	InferenceTimeMs float64       `json:"inference_time_ms"` // Inference duration
	FrameShape      []int         `json:"frame_shape"`      // [height, width]
	ModelInputShape []int         `json:"model_input_shape"` // [height, width]
	DetectionCount  int           `json:"detection_count"`  // Number of detections
}

// BatchInferenceRequest represents a batch inference request
type BatchInferenceRequest struct {
	Images             []string  `json:"images"`
	ConfidenceThreshold *float64  `json:"confidence_threshold,omitempty"`
	EnabledClasses     []string  `json:"enabled_classes,omitempty"`
}

// BatchInferenceResponse represents a batch inference response
type BatchInferenceResponse struct {
	Results                  []InferenceResponse `json:"results"`
	TotalInferenceTimeMs     float64            `json:"total_inference_time_ms"`
	AverageInferenceTimeMs   float64            `json:"average_inference_time_ms"`
}

// InferenceStats represents inference statistics
type InferenceStats struct {
	TotalInferences   int     `json:"total_inferences"`
	TotalTimeMs       float64 `json:"total_time_ms"`
	AverageTimeMs     float64 `json:"average_time_ms"`
}

// DetectionResult represents a single detection result with frame context
type DetectionResult struct {
	Response      *InferenceResponse
	FrameTimestamp time.Time
	CameraID      string
	FrameWidth    int
	FrameHeight   int
}

// ModelMetadata represents model metadata for AI service
type ModelMetadata struct {
	ModelID       string                 `json:"model_id"`
	Version       string                 `json:"version"`
	ModelType     string                 `json:"model_type"`
	CameraID      *string                `json:"camera_id,omitempty"`
	Framework     string                 `json:"framework"`
	InputShape    []int                  `json:"input_shape,omitempty"`
	Preprocessing map[string]interface{} `json:"preprocessing,omitempty"`
	ModelPath     string                 `json:"model_path"`     // Path in MinIO (bucket/object)
	MetadataPath  string                 `json:"metadata_path"` // Path to metadata JSON in MinIO
}

// ProcessingStats contains statistics about frame processing for a camera
type ProcessingStats struct {
	CameraID              string        `json:"camera_id"`
	FramesProcessed       int64         `json:"frames_processed"`
	EventsDetected        int64         `json:"events_detected"`
	LastProcessedAt       time.Time     `json:"last_processed_at"`
	AverageProcessingTime  time.Duration `json:"average_processing_time_ms"`
	ErrorCount            int64         `json:"error_count"`
	IsProcessing          bool          `json:"is_processing"`
}

// AIGatewayConfig contains configuration for the AI gateway
type AIGatewayConfig struct {
	AIServiceURL          string        // URL of the AI service
	InferenceInterval     time.Duration // Minimum interval between inferences per camera
	ConfidenceThreshold   float64       // Confidence threshold for anomaly detection (0.0 to 1.0)
	EnabledClasses        []string      // Optional: filter by class names
	RequestTimeout        time.Duration // Timeout for AI service requests
	MaxRetries            int           // Maximum number of retries for failed requests
	RetryDelay            time.Duration // Delay between retries
}

