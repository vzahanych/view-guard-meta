package ai

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

