package processing

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

func TestNewEventDetector(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.6,
		PerCameraThresholds: map[string]float64{
			"camera-1": 0.7,
		},
		MinEventDuration: 3 * time.Second,
	}

	detector := NewEventDetector(config, log)
	if detector == nil {
		t.Fatal("NewEventDetector returned nil")
	}

	if detector.globalConfidenceThreshold != 0.6 {
		t.Errorf("Expected global threshold 0.6, got %f", detector.globalConfidenceThreshold)
	}

	if detector.minEventDuration != 3*time.Second {
		t.Errorf("Expected min event duration 3s, got %v", detector.minEventDuration)
	}
}

func TestEventDetector_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.5,
		MinEventDuration:         2 * time.Second,
	}

	detector := NewEventDetector(config, log)

	ctx := context.Background()
	err := detector.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	status := detector.GetStatus().GetStatus()
	if status != service.StatusRunning {
		t.Errorf("Expected status %s, got %s", service.StatusRunning, status)
	}

	err = detector.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status = detector.GetStatus().GetStatus()
	if status != service.StatusStopped {
		t.Errorf("Expected status %s, got %s", service.StatusStopped, status)
	}
}

func TestEventDetector_ProcessInferenceResult_NoDetections(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.5,
		MinEventDuration:         2 * time.Second,
	}

	detector := NewEventDetector(config, log)

	ctx := context.Background()
	err := detector.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer detector.Stop()

	result := &InferenceResult{
		CameraID:  "camera-1",
		Timestamp: time.Now(),
		Detections: []Detection{}, // No detections
	}

	// Should not error, just skip event detection
	err = detector.ProcessInferenceResult(ctx, result)
	if err != nil {
		t.Errorf("ProcessInferenceResult should not error with no detections, got: %v", err)
	}
}

func TestEventDetector_ProcessInferenceResult_LowConfidence(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.7,
		MinEventDuration:         2 * time.Second,
	}

	detector := NewEventDetector(config, log)

	ctx := context.Background()
	err := detector.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer detector.Stop()

	result := &InferenceResult{
		CameraID:  "camera-1",
		Timestamp: time.Now(),
		Detections: []Detection{
			{
				ClassID:    0,
				ClassName:  "person",
				Confidence: 0.5, // Below threshold
			},
		},
	}

	// Should not trigger event (low confidence)
	err = detector.ProcessInferenceResult(ctx, result)
	if err != nil {
		t.Errorf("ProcessInferenceResult should not error, got: %v", err)
	}
}

func TestEventDetector_ProcessInferenceResult_HighConfidence_NoDebounce(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	var detectedEvent *events.Event
	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.5,
		MinEventDuration:         100 * time.Millisecond, // Short duration for testing
		OnEventDetected: func(event *events.Event) {
			detectedEvent = event
		},
	}

	detector := NewEventDetector(config, log)

	ctx := context.Background()
	err := detector.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer detector.Stop()

	result := &InferenceResult{
		CameraID:  "camera-1",
		Timestamp: time.Now(),
		Frame: &video.Frame{
			CameraID:  "camera-1",
			Timestamp: time.Now(),
			Data:      []byte{1, 2, 3},
		},
		Detections: []Detection{
			{
				ClassID:    0,
				ClassName:  "person",
				Confidence: 0.8, // Above threshold
			},
		},
	}

	// Process result
	err = detector.ProcessInferenceResult(ctx, result)
	if err != nil {
		t.Fatalf("ProcessInferenceResult failed: %v", err)
	}

	// Wait for debounce period
	time.Sleep(150 * time.Millisecond)

	// Process again to trigger event
	err = detector.ProcessInferenceResult(ctx, result)
	if err != nil {
		t.Fatalf("ProcessInferenceResult failed: %v", err)
	}

	// Wait for event to be triggered
	time.Sleep(50 * time.Millisecond)

	if detectedEvent == nil {
		t.Fatal("OnEventDetected callback was not called")
	}

	if detectedEvent.CameraID != "camera-1" {
		t.Errorf("Expected cameraID 'camera-1', got '%s'", detectedEvent.CameraID)
	}

	if detectedEvent.Confidence < 0.8 {
		t.Errorf("Expected confidence >= 0.8, got %f", detectedEvent.Confidence)
	}
}

func TestEventDetector_SetConfidenceThreshold(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.5,
		MinEventDuration:         2 * time.Second,
	}

	detector := NewEventDetector(config, log)

	newThreshold := 0.7
	detector.SetConfidenceThreshold(newThreshold)

	if detector.globalConfidenceThreshold != newThreshold {
		t.Errorf("Expected confidence threshold %f, got %f", newThreshold, detector.globalConfidenceThreshold)
	}
}

func TestEventDetector_SetMinEventDuration(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := EventDetectorConfig{
		GlobalConfidenceThreshold: 0.5,
		MinEventDuration:         2 * time.Second,
	}

	detector := NewEventDetector(config, log)

	newDuration := 5 * time.Second
	detector.SetMinEventDuration(newDuration)

	if detector.minEventDuration != newDuration {
		t.Errorf("Expected min event duration %v, got %v", newDuration, detector.minEventDuration)
	}
}

// Note: EventDetector doesn't have SetEnabledClasses method
// Class filtering is done in InferenceService before passing to EventDetector

