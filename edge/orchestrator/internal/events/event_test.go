package events

import (
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func TestNewEvent(t *testing.T) {
	event := NewEvent()

	if event.ID == "" {
		t.Error("Event ID should not be empty")
	}

	if event.Metadata == nil {
		t.Error("Event metadata should be initialized")
	}

	if event.Timestamp.IsZero() {
		t.Error("Event timestamp should be set")
	}
}

func TestEvent_ToEventState(t *testing.T) {
	event := NewEvent()
	event.CameraID = "camera-1"
	event.EventType = EventTypePersonDetected
	event.Timestamp = time.Now()
	event.Confidence = 0.85
	event.BoundingBox = &ai.BoundingBox{
		X1:         100,
		Y1:         200,
		X2:         300,
		Y2:         400,
		Confidence: 0.85,
		ClassID:    COCOClassPerson,
		ClassName:  "person",
	}

	eventState := event.ToEventState()

	if eventState.ID != event.ID {
		t.Errorf("Expected ID %s, got %s", event.ID, eventState.ID)
	}

	if eventState.CameraID != event.CameraID {
		t.Errorf("Expected CameraID %s, got %s", event.CameraID, eventState.CameraID)
	}

	if eventState.EventType != event.EventType {
		t.Errorf("Expected EventType %s, got %s", event.EventType, eventState.EventType)
	}

	// Check metadata
	if conf, ok := eventState.Metadata["confidence"].(float64); !ok || conf != 0.85 {
		t.Errorf("Expected confidence in metadata to be 0.85, got %v", conf)
	}

	if bbox, ok := eventState.Metadata["bounding_box"].(map[string]interface{}); !ok {
		t.Error("Expected bounding_box in metadata")
	} else {
		if x1, ok := bbox["x1"].(float64); !ok || x1 != 100 {
			t.Errorf("Expected x1 to be 100, got %v", x1)
		}
	}
}

func TestFromEventState(t *testing.T) {
	eventState := state.EventState{
		ID:        "test-event-id",
		CameraID:  "camera-1",
		EventType: EventTypePersonDetected,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"confidence": 0.85,
			"bounding_box": map[string]interface{}{
				"x1":         100.0,
				"y1":         200.0,
				"x2":         300.0,
				"y2":         400.0,
				"class_id":   float64(COCOClassPerson),
				"class_name": "person",
				"confidence": 0.85,
			},
		},
	}

	event := FromEventState(eventState)

	if event.ID != eventState.ID {
		t.Errorf("Expected ID %s, got %s", eventState.ID, event.ID)
	}

	if event.Confidence != 0.85 {
		t.Errorf("Expected confidence 0.85, got %f", event.Confidence)
	}

	if event.BoundingBox == nil {
		t.Error("Expected bounding box to be set")
	} else {
		if event.BoundingBox.X1 != 100 {
			t.Errorf("Expected X1 100, got %f", event.BoundingBox.X1)
		}
		if event.BoundingBox.ClassID != COCOClassPerson {
			t.Errorf("Expected ClassID %d, got %d", COCOClassPerson, event.BoundingBox.ClassID)
		}
	}
}

func TestEvent_IsSignificant(t *testing.T) {
	tests := []struct {
		name          string
		confidence    float64
		minConfidence float64
		expected      bool
	}{
		{
			name:          "above threshold",
			confidence:    0.9,
			minConfidence: 0.5,
			expected:      true,
		},
		{
			name:          "below threshold",
			confidence:    0.3,
			minConfidence: 0.5,
			expected:      false,
		},
		{
			name:          "at threshold",
			confidence:    0.5,
			minConfidence: 0.5,
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewEvent()
			event.Confidence = tt.confidence

			result := event.IsSignificant(tt.minConfidence)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetEventTypeFromClassID(t *testing.T) {
	tests := []struct {
		classID   int
		eventType string
	}{
		{COCOClassPerson, EventTypePersonDetected},
		{COCOClassCar, EventTypeVehicleDetected},
		{COCOClassTruck, EventTypeVehicleDetected},
		{99, EventTypeObjectDetected}, // Unknown class
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			result := GetEventTypeFromClassID(tt.classID)
			if result != tt.eventType {
				t.Errorf("Expected %s, got %s", tt.eventType, result)
			}
		})
	}
}

