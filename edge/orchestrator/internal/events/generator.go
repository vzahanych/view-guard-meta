package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// Generator generates events from AI detection results
type Generator struct {
	stateManager    *state.Manager
	logger          *logger.Logger
	confidenceThreshold float64
	enabledClasses  []string
	dedupWindow     time.Duration // Deduplication time window
	recentEvents    map[string]time.Time // cameraID+classID -> timestamp for deduplication
	mu              sync.RWMutex
}

// GeneratorConfig contains configuration for the event generator
type GeneratorConfig struct {
	StateManager        *state.Manager
	ConfidenceThreshold float64
	EnabledClasses      []string
	DeduplicationWindow time.Duration // Time window for deduplication (e.g., 5 seconds)
}

// NewGenerator creates a new event generator
func NewGenerator(config GeneratorConfig, log *logger.Logger) *Generator {
	dedupWindow := config.DeduplicationWindow
	if dedupWindow == 0 {
		dedupWindow = 5 * time.Second // Default: 5 seconds
	}

	return &Generator{
		stateManager:        config.StateManager,
		logger:              log,
		confidenceThreshold: config.ConfidenceThreshold,
		enabledClasses:      config.EnabledClasses,
		dedupWindow:         dedupWindow,
		recentEvents:        make(map[string]time.Time),
	}
}

// GenerateEventsFromDetection generates events from a detection result
func (g *Generator) GenerateEventsFromDetection(
	ctx context.Context,
	detection *ai.DetectionResult,
) ([]*Event, error) {
	if detection == nil || detection.Response == nil {
		return nil, fmt.Errorf("invalid detection result")
	}

	var events []*Event
	now := time.Now()

	// Process each bounding box as a potential event
	for _, bbox := range detection.Response.BoundingBoxes {
		// Check confidence threshold
		if bbox.Confidence < g.confidenceThreshold {
			g.logger.Debug(
				"Skipping detection below confidence threshold",
				"confidence", bbox.Confidence,
				"threshold", g.confidenceThreshold,
			)
			continue
		}

		// Check enabled classes filter
		if len(g.enabledClasses) > 0 {
			classEnabled := false
			for _, enabledClass := range g.enabledClasses {
				if bbox.ClassName == enabledClass {
					classEnabled = true
					break
				}
			}
			if !classEnabled {
				g.logger.Debug(
					"Skipping detection for disabled class",
					"class", bbox.ClassName,
				)
				continue
			}
		}

		// Check deduplication
		dedupKey := fmt.Sprintf("%s:%d", detection.CameraID, bbox.ClassID)
		if g.isDuplicate(dedupKey, now) {
			g.logger.Debug(
				"Skipping duplicate detection",
				"camera_id", detection.CameraID,
				"class_id", bbox.ClassID,
			)
			continue
		}

		// Create event
		event := NewEvent()
		event.CameraID = detection.CameraID
		event.Timestamp = detection.FrameTimestamp
		event.Confidence = bbox.Confidence
		event.BoundingBox = &bbox
		event.EventType = GetEventTypeFromClassID(bbox.ClassID)

		// Add frame metadata
		event.Metadata["frame_width"] = detection.FrameWidth
		event.Metadata["frame_height"] = detection.FrameHeight
		event.Metadata["inference_time_ms"] = detection.Response.InferenceTimeMs
		event.Metadata["model_input_shape"] = detection.Response.ModelInputShape

		events = append(events, event)

		// Mark as seen for deduplication
		g.markSeen(dedupKey, now)
	}

	return events, nil
}

// GenerateEventFromBoundingBox generates a single event from a bounding box
func (g *Generator) GenerateEventFromBoundingBox(
	ctx context.Context,
	cameraID string,
	timestamp time.Time,
	bbox ai.BoundingBox,
	frameWidth, frameHeight int,
) (*Event, error) {
	// Check confidence threshold
	if bbox.Confidence < g.confidenceThreshold {
		return nil, fmt.Errorf("confidence below threshold: %f < %f", bbox.Confidence, g.confidenceThreshold)
	}

	// Check enabled classes filter
	if len(g.enabledClasses) > 0 {
		classEnabled := false
		for _, enabledClass := range g.enabledClasses {
			if bbox.ClassName == enabledClass {
				classEnabled = true
				break
			}
		}
		if !classEnabled {
			return nil, fmt.Errorf("class not enabled: %s", bbox.ClassName)
		}
	}

	// Check deduplication
	dedupKey := fmt.Sprintf("%s:%d", cameraID, bbox.ClassID)
	now := time.Now()
	if g.isDuplicate(dedupKey, now) {
		return nil, fmt.Errorf("duplicate detection within deduplication window")
	}

	// Create event
	event := NewEvent()
	event.CameraID = cameraID
	event.Timestamp = timestamp
	event.Confidence = bbox.Confidence
	event.BoundingBox = &bbox
	event.EventType = GetEventTypeFromClassID(bbox.ClassID)

	// Add frame metadata
	event.Metadata["frame_width"] = frameWidth
	event.Metadata["frame_height"] = frameHeight

	// Mark as seen for deduplication
	g.markSeen(dedupKey, now)

	return event, nil
}

// AssociateClip associates a clip path with an event
func (g *Generator) AssociateClip(event *Event, clipPath string) {
	event.ClipPath = clipPath
	event.Metadata["clip_path"] = clipPath
}

// AssociateSnapshot associates a snapshot path with an event
func (g *Generator) AssociateSnapshot(event *Event, snapshotPath string) {
	event.SnapshotPath = snapshotPath
	event.Metadata["snapshot_path"] = snapshotPath
}

// isDuplicate checks if a detection is a duplicate within the deduplication window
func (g *Generator) isDuplicate(key string, now time.Time) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	lastSeen, exists := g.recentEvents[key]
	if !exists {
		return false
	}

	// Check if within deduplication window
	return now.Sub(lastSeen) < g.dedupWindow
}

// markSeen marks a detection as seen for deduplication
func (g *Generator) markSeen(key string, timestamp time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.recentEvents[key] = timestamp
}

// CleanupOldDedupEntries removes old deduplication entries
func (g *Generator) CleanupOldDedupEntries() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for key, timestamp := range g.recentEvents {
		if now.Sub(timestamp) > g.dedupWindow*2 {
			delete(g.recentEvents, key)
		}
	}
}

// SetConfidenceThreshold updates the confidence threshold
func (g *Generator) SetConfidenceThreshold(threshold float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.confidenceThreshold = threshold
}

// SetEnabledClasses updates the enabled classes filter
func (g *Generator) SetEnabledClasses(classes []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enabledClasses = classes
}

// SetDeduplicationWindow updates the deduplication window
func (g *Generator) SetDeduplicationWindow(window time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dedupWindow = window
}

