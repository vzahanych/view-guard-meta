package events

import (
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// Event represents a detected event with full context
type Event struct {
	ID           string                 // UUID
	CameraID     string                 // Camera that detected the event
	EventType    string                 // Event type (e.g., "person_detected", "vehicle_detected")
	Timestamp    time.Time              // When the event occurred
	Confidence   float64                // Detection confidence (0.0 to 1.0)
	BoundingBox  *ai.BoundingBox        // Bounding box of detected object (nil if not applicable)
	Metadata     map[string]interface{} // Additional event metadata
	ClipPath     string                 // Path to associated video clip (if any)
	SnapshotPath string                 // Path to associated snapshot (if any)
}

// EventType constants
const (
	EventTypePersonDetected   = "person_detected"
	EventTypeVehicleDetected  = "vehicle_detected"
	EventTypeObjectDetected   = "object_detected"
	EventTypeMotionDetected   = "motion_detected"
	EventTypeCustomDetected   = "custom_detected"
	EventTypeCameraObstructed = "camera_obstructed" // Critical: Camera view is blocked
	EventTypeAnomalyDetected  = "anomaly_detected"  // Adaptive AI anomaly
)

// COCO class IDs for common objects
const (
	COCOClassPerson     = 0
	COCOClassBicycle    = 1
	COCOClassCar        = 2
	COCOClassMotorcycle = 3
	COCOClassAirplane   = 4
	COCOClassBus        = 5
	COCOClassTrain      = 6
	COCOClassTruck      = 7
)

// ClassIDToEventType maps COCO class IDs to event types
var ClassIDToEventType = map[int]string{
	COCOClassPerson:     EventTypePersonDetected,
	COCOClassBicycle:    EventTypeVehicleDetected,
	COCOClassCar:        EventTypeVehicleDetected,
	COCOClassMotorcycle: EventTypeVehicleDetected,
	COCOClassBus:        EventTypeVehicleDetected,
	COCOClassTrain:      EventTypeVehicleDetected,
	COCOClassTruck:      EventTypeVehicleDetected,
}

// NewEvent creates a new event with a generated UUID
func NewEvent() *Event {
	return &Event{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// ToEventState converts an Event to EventState for storage
func (e *Event) ToEventState() state.EventState {
	// Build metadata JSON
	metadata := make(map[string]interface{})

	// Copy existing metadata
	for k, v := range e.Metadata {
		metadata[k] = v
	}

	// Add detection-specific metadata
	metadata["confidence"] = e.Confidence
	if e.BoundingBox != nil {
		metadata["bounding_box"] = map[string]interface{}{
			"x1":         e.BoundingBox.X1,
			"y1":         e.BoundingBox.Y1,
			"x2":         e.BoundingBox.X2,
			"y2":         e.BoundingBox.Y2,
			"class_id":   e.BoundingBox.ClassID,
			"class_name": e.BoundingBox.ClassName,
		}
	}
	metadata["frame_width"] = 0  // Will be set by generator if available
	metadata["frame_height"] = 0 // Will be set by generator if available

	return state.EventState{
		ID:           e.ID,
		CameraID:     e.CameraID,
		EventType:    e.EventType,
		Timestamp:    e.Timestamp,
		Metadata:     metadata,
		ClipPath:     e.ClipPath,
		SnapshotPath: e.SnapshotPath,
		Transmitted:  false,
	}
}

// FromEventState creates an Event from EventState
func FromEventState(es state.EventState) *Event {
	event := &Event{
		ID:           es.ID,
		CameraID:     es.CameraID,
		EventType:    es.EventType,
		Timestamp:    es.Timestamp,
		ClipPath:     es.ClipPath,
		SnapshotPath: es.SnapshotPath,
		Metadata:     make(map[string]interface{}),
	}

	// Extract metadata
	if es.Metadata != nil {
		// Copy metadata
		for k, v := range es.Metadata {
			event.Metadata[k] = v
		}

		// Extract confidence
		if conf, ok := es.Metadata["confidence"].(float64); ok {
			event.Confidence = conf
		}

		// Extract bounding box
		if bboxMap, ok := es.Metadata["bounding_box"].(map[string]interface{}); ok {
			bbox := &ai.BoundingBox{}
			if x1, ok := bboxMap["x1"].(float64); ok {
				bbox.X1 = x1
			}
			if y1, ok := bboxMap["y1"].(float64); ok {
				bbox.Y1 = y1
			}
			if x2, ok := bboxMap["x2"].(float64); ok {
				bbox.X2 = x2
			}
			if y2, ok := bboxMap["y2"].(float64); ok {
				bbox.Y2 = y2
			}
			if classID, ok := bboxMap["class_id"].(float64); ok {
				bbox.ClassID = int(classID)
			}
			if className, ok := bboxMap["class_name"].(string); ok {
				bbox.ClassName = className
			}
			if conf, ok := bboxMap["confidence"].(float64); ok {
				bbox.Confidence = conf
			}
			event.BoundingBox = bbox
		}
	}

	return event
}

// IsSignificant returns true if the event is significant enough to be stored
func (e *Event) IsSignificant(minConfidence float64) bool {
	return e.Confidence >= minConfidence
}

// GetEventTypeFromClassID returns the event type for a given COCO class ID
func GetEventTypeFromClassID(classID int) string {
	if eventType, ok := ClassIDToEventType[classID]; ok {
		return eventType
	}
	return EventTypeObjectDetected
}
