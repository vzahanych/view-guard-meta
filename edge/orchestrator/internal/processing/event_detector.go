package processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// EventDetector handles event detection logic based on inference results
type EventDetector struct {
	*service.ServiceBase
	logger                *logger.Logger
	onEventDetected       func(*events.Event) // Callback for detected events
	mu                    sync.RWMutex
	ctx                   context.Context
	cancel                context.CancelFunc
	// Configuration
	globalConfidenceThreshold float64
	perCameraThresholds       map[string]float64 // cameraID -> threshold
	minEventDuration          time.Duration      // Minimum duration for event (debouncing)
	// State tracking for debouncing
	activeEvents map[string]*EventState // cameraID -> current event state
}

// EventState tracks the state of a potential event for debouncing
type EventState struct {
	StartTime      time.Time
	LastDetection  time.Time
	DetectionCount int
	MaxConfidence  float64
	Detections     []Detection
	Frame          *video.Frame
}

// EventDetectorConfig contains event detector configuration
type EventDetectorConfig struct {
	GlobalConfidenceThreshold float64
	PerCameraThresholds       map[string]float64 // Optional per-camera thresholds
	MinEventDuration          time.Duration      // Minimum event duration for debouncing
	OnEventDetected           func(*events.Event) // Callback for detected events
}

// NewEventDetector creates a new event detector
func NewEventDetector(config EventDetectorConfig, log *logger.Logger) *EventDetector {
	ctx, cancel := context.WithCancel(context.Background())

	// Default confidence threshold
	confidenceThreshold := config.GlobalConfidenceThreshold
	if confidenceThreshold == 0 {
		confidenceThreshold = 0.5 // Default: 50% confidence
	}

	// Default minimum event duration (debouncing)
	minEventDuration := config.MinEventDuration
	if minEventDuration == 0 {
		minEventDuration = 2 * time.Second // Default: 2 seconds
	}

	return &EventDetector{
		ServiceBase:              service.NewServiceBase("event-detector", log),
		logger:                   log,
		onEventDetected:          config.OnEventDetected,
		ctx:                      ctx,
		cancel:                   cancel,
		globalConfidenceThreshold: confidenceThreshold,
		perCameraThresholds:       config.PerCameraThresholds,
		minEventDuration:         minEventDuration,
		activeEvents:             make(map[string]*EventState),
	}
}

// Start starts the event detector
func (ed *EventDetector) Start(ctx context.Context) error {
	// Start cleanup goroutine for expired events
	go ed.cleanupExpiredEvents(ctx)

	ed.LogInfo("Event detector started",
		"global_confidence_threshold", ed.globalConfidenceThreshold,
		"min_event_duration", ed.minEventDuration,
	)
	return nil
}

// Stop stops the event detector
func (ed *EventDetector) Stop() error {
	ed.cancel()
	ed.LogInfo("Event detector stopped")
	return nil
}

// ProcessInferenceResult processes an inference result and determines if an event should be triggered
func (ed *EventDetector) ProcessInferenceResult(ctx context.Context, result *InferenceResult) error {
	if result == nil || result.Error != nil {
		// Skip processing if inference failed
		return nil
	}

	cameraID := result.CameraID

	// Get confidence threshold for this camera (or use global)
	threshold := ed.getConfidenceThreshold(cameraID)

	// Filter detections by confidence threshold
	eventDetections := ed.filterDetectionsByThreshold(result.Detections, threshold)
	if len(eventDetections) == 0 {
		// No detections above threshold - clear any active event state
		ed.clearEventState(cameraID)
		return nil
	}

	// Update or create event state
	ed.mu.Lock()
	eventState, exists := ed.activeEvents[cameraID]
	if !exists {
		// Create new event state
		eventState = &EventState{
			StartTime:      time.Now(),
			LastDetection:  time.Now(),
			DetectionCount: len(eventDetections),
			MaxConfidence:  ed.getMaxConfidence(eventDetections),
			Detections:     eventDetections,
			Frame:          result.Frame,
		}
		ed.activeEvents[cameraID] = eventState
		ed.mu.Unlock()

		ed.logger.Debug("Event state created",
			"camera_id", cameraID,
			"detection_count", len(eventDetections),
			"max_confidence", eventState.MaxConfidence,
		)
		return nil
	}

	// Update existing event state
	eventState.LastDetection = time.Now()
	eventState.DetectionCount += len(eventDetections)
	if newMax := ed.getMaxConfidence(eventDetections); newMax > eventState.MaxConfidence {
		eventState.MaxConfidence = newMax
	}
	// Update detections (keep most recent)
	eventState.Detections = append(eventState.Detections, eventDetections...)
	// Keep only recent detections (last 10)
	if len(eventState.Detections) > 10 {
		eventState.Detections = eventState.Detections[len(eventState.Detections)-10:]
	}
	eventState.Frame = result.Frame // Update to most recent frame
	ed.mu.Unlock()

	// Check if event duration exceeds minimum (debouncing)
	eventDuration := time.Since(eventState.StartTime)
	if eventDuration >= ed.minEventDuration {
		// Event duration exceeded - trigger event registration
		return ed.triggerEvent(ctx, cameraID, eventState)
	}

	return nil
}

// filterDetectionsByThreshold filters detections by confidence threshold
func (ed *EventDetector) filterDetectionsByThreshold(detections []Detection, threshold float64) []Detection {
	filtered := make([]Detection, 0)
	for _, det := range detections {
		if det.Confidence >= threshold && det.EventType == "event" {
			filtered = append(filtered, det)
		}
	}
	return filtered
}

// getMaxConfidence returns the maximum confidence from detections
func (ed *EventDetector) getMaxConfidence(detections []Detection) float64 {
	max := 0.0
	for _, det := range detections {
		if det.Confidence > max {
			max = det.Confidence
		}
	}
	return max
}

// getConfidenceThreshold returns the confidence threshold for a camera
func (ed *EventDetector) getConfidenceThreshold(cameraID string) float64 {
	ed.mu.RLock()
	defer ed.mu.RUnlock()

	if threshold, ok := ed.perCameraThresholds[cameraID]; ok {
		return threshold
	}
	return ed.globalConfidenceThreshold
}

// clearEventState clears the event state for a camera
func (ed *EventDetector) clearEventState(cameraID string) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	delete(ed.activeEvents, cameraID)
}

// triggerEvent triggers event registration
func (ed *EventDetector) triggerEvent(ctx context.Context, cameraID string, eventState *EventState) error {
	// Determine event type from detections
	eventType := ed.determineEventType(eventState.Detections)

	// Get primary detection (highest confidence)
	primaryDetection := ed.getPrimaryDetection(eventState.Detections)

	// Create event
	event := events.NewEvent()
	event.CameraID = cameraID
	event.EventType = eventType
	event.Timestamp = eventState.StartTime // Use event start time
	event.Confidence = eventState.MaxConfidence

	// Add bounding box if available
	if primaryDetection != nil {
		event.BoundingBox = &ai.BoundingBox{
			X1:         primaryDetection.BoundingBox.X1,
			Y1:         primaryDetection.BoundingBox.Y1,
			X2:         primaryDetection.BoundingBox.X2,
			Y2:         primaryDetection.BoundingBox.Y2,
			Confidence: primaryDetection.Confidence,
			ClassID:    primaryDetection.ClassID,
			ClassName:  primaryDetection.ClassName,
		}
	}

	// Generate event metadata
	event.Metadata = ed.generateEventMetadata(eventState, primaryDetection)

	ed.logger.Info("Event detected",
		"event_id", event.ID,
		"camera_id", cameraID,
		"event_type", eventType,
		"confidence", event.Confidence,
		"detection_count", eventState.DetectionCount,
		"duration_ms", time.Since(eventState.StartTime).Milliseconds(),
	)

	// Call callback
	if ed.onEventDetected != nil {
		ed.onEventDetected(event)
	}

	// Clear event state after triggering
	ed.clearEventState(cameraID)

	return nil
}

// determineEventType determines the event type from detections
func (ed *EventDetector) determineEventType(detections []Detection) string {
	if len(detections) == 0 {
		return events.EventTypeObjectDetected
	}

	// Count detections by class
	classCounts := make(map[string]int)
	for _, det := range detections {
		classCounts[det.ClassName]++
	}

	// Find most common class
	maxCount := 0
	mostCommonClass := ""
	for className, count := range classCounts {
		if count > maxCount {
			maxCount = count
			mostCommonClass = className
		}
	}

	// Map class name to event type
	eventType := ed.mapClassNameToEventType(mostCommonClass)
	if eventType == "" {
		// Default to object detected
		eventType = events.EventTypeObjectDetected
	}

	return eventType
}

// mapClassNameToEventType maps a class name to an event type
func (ed *EventDetector) mapClassNameToEventType(className string) string {
	// Map common class names to event types
	classNameMap := map[string]string{
		"person":   events.EventTypePersonDetected,
		"car":      events.EventTypeVehicleDetected,
		"truck":    events.EventTypeVehicleDetected,
		"bus":      events.EventTypeVehicleDetected,
		"motorcycle": events.EventTypeVehicleDetected,
		"bicycle":  events.EventTypeVehicleDetected,
		"train":    events.EventTypeVehicleDetected,
		"airplane": events.EventTypeVehicleDetected,
	}

	if eventType, ok := classNameMap[className]; ok {
		return eventType
	}

	// Check COCO class ID mapping
	for classID, eventType := range events.ClassIDToEventType {
		// This is a simplified mapping - in production, would use actual class IDs
		if className == fmt.Sprintf("class_%d", classID) {
			return eventType
		}
	}

	return ""
}

// getPrimaryDetection returns the detection with highest confidence
func (ed *EventDetector) getPrimaryDetection(detections []Detection) *Detection {
	if len(detections) == 0 {
		return nil
	}

	maxConfidence := 0.0
	var primary *Detection
	for i := range detections {
		if detections[i].Confidence > maxConfidence {
			maxConfidence = detections[i].Confidence
			primary = &detections[i]
		}
	}
	return primary
}

// generateEventMetadata generates event metadata from event state
func (ed *EventDetector) generateEventMetadata(eventState *EventState, primaryDetection *Detection) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Detection statistics
	metadata["detection_count"] = eventState.DetectionCount
	metadata["max_confidence"] = eventState.MaxConfidence
	metadata["event_duration_ms"] = time.Since(eventState.StartTime).Milliseconds()

	// Frame information
	if eventState.Frame != nil {
		metadata["frame_width"] = eventState.Frame.Width
		metadata["frame_height"] = eventState.Frame.Height
		metadata["frame_timestamp"] = eventState.Frame.Timestamp
	}

	// Primary detection information
	if primaryDetection != nil {
		metadata["primary_class_id"] = primaryDetection.ClassID
		metadata["primary_class_name"] = primaryDetection.ClassName
		metadata["primary_confidence"] = primaryDetection.Confidence
		metadata["bounding_box"] = map[string]interface{}{
			"x1": primaryDetection.BoundingBox.X1,
			"y1": primaryDetection.BoundingBox.Y1,
			"x2": primaryDetection.BoundingBox.X2,
			"y2": primaryDetection.BoundingBox.Y2,
		}
	}

	// All detections summary
	detectionSummary := make([]map[string]interface{}, 0, len(eventState.Detections))
	for _, det := range eventState.Detections {
		detectionSummary = append(detectionSummary, map[string]interface{}{
			"class_id":   det.ClassID,
			"class_name": det.ClassName,
			"confidence": det.Confidence,
		})
	}
	metadata["all_detections"] = detectionSummary

	return metadata
}

// cleanupExpiredEvents periodically cleans up expired event states
func (ed *EventDetector) cleanupExpiredEvents(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ed.ctx.Done():
			return
		case <-ticker.C:
			ed.mu.Lock()
			now := time.Now()
			expiredCameras := make([]string, 0)

			for cameraID, eventState := range ed.activeEvents {
				// If no detection for 30 seconds, clear the state
				if now.Sub(eventState.LastDetection) > 30*time.Second {
					expiredCameras = append(expiredCameras, cameraID)
				}
			}

			for _, cameraID := range expiredCameras {
				delete(ed.activeEvents, cameraID)
				ed.logger.Debug("Cleared expired event state", "camera_id", cameraID)
			}
			ed.mu.Unlock()
		}
	}
}

// SetConfidenceThreshold sets the global confidence threshold
func (ed *EventDetector) SetConfidenceThreshold(threshold float64) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.globalConfidenceThreshold = threshold
}

// SetCameraConfidenceThreshold sets the confidence threshold for a specific camera
func (ed *EventDetector) SetCameraConfidenceThreshold(cameraID string, threshold float64) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	if ed.perCameraThresholds == nil {
		ed.perCameraThresholds = make(map[string]float64)
	}
	ed.perCameraThresholds[cameraID] = threshold
}

// SetMinEventDuration sets the minimum event duration for debouncing
func (ed *EventDetector) SetMinEventDuration(duration time.Duration) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.minEventDuration = duration
}

// GetConfidenceThreshold returns the global confidence threshold
func (ed *EventDetector) GetConfidenceThreshold() float64 {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.globalConfidenceThreshold
}

// GetCameraConfidenceThreshold returns the confidence threshold for a camera
func (ed *EventDetector) GetCameraConfidenceThreshold(cameraID string) float64 {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	if threshold, ok := ed.perCameraThresholds[cameraID]; ok {
		return threshold
	}
	return ed.globalConfidenceThreshold
}

// GetMinEventDuration returns the minimum event duration
func (ed *EventDetector) GetMinEventDuration() time.Duration {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.minEventDuration
}

