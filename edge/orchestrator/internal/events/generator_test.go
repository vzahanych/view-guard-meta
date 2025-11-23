package events

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestGenerator(t *testing.T) (*Generator, *state.Manager) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	// Create test state manager using helper
	stateMgr := setupTestManager(t)

	generator := NewGenerator(GeneratorConfig{
		StateManager:        stateMgr,
		ConfidenceThreshold: 0.5,
		EnabledClasses:      []string{"person", "car"},
		DeduplicationWindow: 5 * time.Second,
	}, log)

	return generator, stateMgr
}

func TestGenerator_GenerateEventsFromDetection(t *testing.T) {
	generator, _ := setupTestGenerator(t)

	detection := &ai.DetectionResult{
		Response: &ai.InferenceResponse{
			BoundingBoxes: []ai.BoundingBox{
				{
					X1:         100,
					Y1:         200,
					X2:         300,
					Y2:         400,
					Confidence: 0.85,
					ClassID:    COCOClassPerson,
					ClassName:  "person",
				},
				{
					X1:         500,
					Y1:         100,
					X2:         700,
					Y2:         300,
					Confidence: 0.3, // Below threshold
					ClassID:    COCOClassCar,
					ClassName:  "car",
				},
			},
			InferenceTimeMs: 45.2,
			FrameShape:      []int{480, 640},
			ModelInputShape: []int{640, 640},
			DetectionCount:  2,
		},
		FrameTimestamp: time.Now(),
		CameraID:       "camera-1",
		FrameWidth:     640,
		FrameHeight:    480,
	}

	ctx := context.Background()
	events, err := generator.GenerateEventsFromDetection(ctx, detection)

	if err != nil {
		t.Fatalf("Failed to generate events: %v", err)
	}

	// Should only generate 1 event (the person, car is below threshold)
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != EventTypePersonDetected {
		t.Errorf("Expected event type %s, got %s", EventTypePersonDetected, events[0].EventType)
	}

	if events[0].Confidence != 0.85 {
		t.Errorf("Expected confidence 0.85, got %f", events[0].Confidence)
	}
}

func TestGenerator_Deduplication(t *testing.T) {
	generator, _ := setupTestGenerator(t)

	detection := &ai.DetectionResult{
		Response: &ai.InferenceResponse{
			BoundingBoxes: []ai.BoundingBox{
				{
					X1:         100,
					Y1:         200,
					X2:         300,
					Y2:         400,
					Confidence: 0.85,
					ClassID:    COCOClassPerson,
					ClassName:  "person",
				},
			},
			InferenceTimeMs: 45.2,
			FrameShape:      []int{480, 640},
			ModelInputShape: []int{640, 640},
			DetectionCount:  1,
		},
		FrameTimestamp: time.Now(),
		CameraID:       "camera-1",
		FrameWidth:     640,
		FrameHeight:    480,
	}

	ctx := context.Background()

	// First detection should generate an event
	events1, err := generator.GenerateEventsFromDetection(ctx, detection)
	if err != nil {
		t.Fatalf("Failed to generate events: %v", err)
	}
	if len(events1) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events1))
	}

	// Second detection immediately after should be deduplicated
	events2, err := generator.GenerateEventsFromDetection(ctx, detection)
	if err != nil {
		t.Fatalf("Failed to generate events: %v", err)
	}
	if len(events2) != 0 {
		t.Errorf("Expected 0 events (deduplicated), got %d", len(events2))
	}

	// Wait for deduplication window to expire
	time.Sleep(6 * time.Second)

	// Third detection after window should generate an event
	events3, err := generator.GenerateEventsFromDetection(ctx, detection)
	if err != nil {
		t.Fatalf("Failed to generate events: %v", err)
	}
	if len(events3) != 1 {
		t.Errorf("Expected 1 event after deduplication window, got %d", len(events3))
	}
}

func TestGenerator_EnabledClassesFilter(t *testing.T) {
	generator, _ := setupTestGenerator(t)
	generator.SetEnabledClasses([]string{"person"}) // Only person enabled

	detection := &ai.DetectionResult{
		Response: &ai.InferenceResponse{
			BoundingBoxes: []ai.BoundingBox{
				{
					X1:         100,
					Y1:         200,
					X2:         300,
					Y2:         400,
					Confidence: 0.85,
					ClassID:    COCOClassPerson,
					ClassName:  "person",
				},
				{
					X1:         500,
					Y1:         100,
					X2:         700,
					Y2:         300,
					Confidence: 0.9,
					ClassID:    COCOClassCar,
					ClassName:  "car",
				},
			},
			InferenceTimeMs: 45.2,
			FrameShape:      []int{480, 640},
			ModelInputShape: []int{640, 640},
			DetectionCount:  2,
		},
		FrameTimestamp: time.Now(),
		CameraID:       "camera-1",
		FrameWidth:     640,
		FrameHeight:    480,
	}

	ctx := context.Background()
	events, err := generator.GenerateEventsFromDetection(ctx, detection)

	if err != nil {
		t.Fatalf("Failed to generate events: %v", err)
	}

	// Should only generate 1 event (person, car is filtered out)
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != EventTypePersonDetected {
		t.Errorf("Expected event type %s, got %s", EventTypePersonDetected, events[0].EventType)
	}
}

func TestGenerator_AssociateClipAndSnapshot(t *testing.T) {
	generator, _ := setupTestGenerator(t)

	event := NewEvent()
	event.CameraID = "camera-1"
	event.EventType = EventTypePersonDetected

	generator.AssociateClip(event, "/path/to/clip.mp4")
	generator.AssociateSnapshot(event, "/path/to/snapshot.jpg")

	if event.ClipPath != "/path/to/clip.mp4" {
		t.Errorf("Expected clip path /path/to/clip.mp4, got %s", event.ClipPath)
	}

	if event.SnapshotPath != "/path/to/snapshot.jpg" {
		t.Errorf("Expected snapshot path /path/to/snapshot.jpg, got %s", event.SnapshotPath)
	}

	if clipPath, ok := event.Metadata["clip_path"].(string); !ok || clipPath != "/path/to/clip.mp4" {
		t.Error("Expected clip_path in metadata")
	}

	if snapshotPath, ok := event.Metadata["snapshot_path"].(string); !ok || snapshotPath != "/path/to/snapshot.jpg" {
		t.Error("Expected snapshot_path in metadata")
	}
}

