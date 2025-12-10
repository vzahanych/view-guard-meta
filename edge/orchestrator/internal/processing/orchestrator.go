package processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// EventProcessingOrchestrator orchestrates the complete event processing pipeline
type EventProcessingOrchestrator struct {
	*service.ServiceBase
	logger            *logger.Logger
	// Services
	frameProcessor    *FrameProcessor
	inferenceService  *InferenceService
	eventDetector     *EventDetector
	clipRecorder      *ClipRecorderService
	snapshotCapture   *SnapshotCaptureService
	eventRegistrar    *EventRegistrarService
	eventQueueManager *EventQueueManagerService
	// Configuration
	config            *OrchestratorConfig
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	// Camera stream inputs (for clip recording)
	cameraStreamInputs map[string]string // cameraID -> stream input (RTSP URL or device path)
}

// OrchestratorConfig contains orchestrator configuration
type OrchestratorConfig struct {
	// Frame processing
	InferenceInterval time.Duration // Interval between frames sent to inference
	PreBufferDuration time.Duration // Duration of pre-event buffer
	
	// Inference
	ConfidenceThreshold float64
	EnabledClasses      []string
	
	// Event detection
	MinEventDuration    time.Duration // Minimum event duration for debouncing
	
	// Clip recording
	PostEventDuration   time.Duration // Duration to record after event
	MaxClipLength       time.Duration // Maximum clip length
	
	// Snapshot capture
	JPEGQuality         int // JPEG quality for snapshots (1-100)
	
	// Retry configuration
	MaxRetries          int
	RetryBaseDelay      time.Duration
	RetryMaxDelay       time.Duration
}

// NewEventProcessingOrchestrator creates a new event processing orchestrator
func NewEventProcessingOrchestrator(
	frameProcessor *FrameProcessor,
	inferenceService *InferenceService,
	eventDetector *EventDetector,
	clipRecorder *ClipRecorderService,
	snapshotCapture *SnapshotCaptureService,
	eventRegistrar *EventRegistrarService,
	eventQueueManager *EventQueueManagerService,
	config *OrchestratorConfig,
	log *logger.Logger,
) *EventProcessingOrchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	return &EventProcessingOrchestrator{
		ServiceBase:       service.NewServiceBase("event-processing-orchestrator", log),
		logger:            log,
		frameProcessor:    frameProcessor,
		inferenceService:  inferenceService,
		eventDetector:     eventDetector,
		clipRecorder:      clipRecorder,
		snapshotCapture:   snapshotCapture,
		eventRegistrar:    eventRegistrar,
		eventQueueManager: eventQueueManager,
		config:            config,
		ctx:               ctx,
		cancel:            cancel,
		cameraStreamInputs: make(map[string]string),
	}
}

// Start starts the event processing orchestrator
func (epo *EventProcessingOrchestrator) Start(ctx context.Context) error {
	// Wire up the processing pipeline
	
	// 1. Frame Processor → Inference Service
	epo.frameProcessor.onInferenceFrame = func(frame *video.Frame) {
		// Process frame for inference
		if err := epo.inferenceService.ProcessFrame(ctx, frame); err != nil {
			epo.logger.Warn("Failed to process frame for inference",
				"camera_id", frame.CameraID,
				"error", err,
			)
		}
	}

	// 2. Inference Service → Event Detector
	epo.inferenceService.onDetection = func(result *InferenceResult) {
		// Process inference result for event detection
		if err := epo.eventDetector.ProcessInferenceResult(ctx, result); err != nil {
			epo.logger.Warn("Failed to process inference result",
				"camera_id", result.CameraID,
				"error", err,
			)
		}
	}

	// 3. Event Detector → Clip Recording, Snapshot Capture, Event Registration
	epo.eventDetector.onEventDetected = func(event *events.Event) {
		epo.handleEventDetected(ctx, event)
	}

	// Start all services
	if err := epo.frameProcessor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start frame processor: %w", err)
	}

	if err := epo.inferenceService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start inference service: %w", err)
	}

	if err := epo.eventDetector.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event detector: %w", err)
	}

	if err := epo.clipRecorder.Start(ctx); err != nil {
		return fmt.Errorf("failed to start clip recorder: %w", err)
	}

	if err := epo.snapshotCapture.Start(ctx); err != nil {
		return fmt.Errorf("failed to start snapshot capture: %w", err)
	}

	if err := epo.eventRegistrar.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event registrar: %w", err)
	}

	if err := epo.eventQueueManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event queue manager: %w", err)
	}

	epo.LogInfo("Event processing orchestrator started",
		"inference_interval", epo.config.InferenceInterval,
		"confidence_threshold", epo.config.ConfidenceThreshold,
		"min_event_duration", epo.config.MinEventDuration,
	)

	return nil
}

// Stop stops the event processing orchestrator
func (epo *EventProcessingOrchestrator) Stop() error {
	epo.cancel()

	// Stop all services in reverse order
	var errors []error

	if err := epo.eventQueueManager.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop event queue manager: %w", err))
	}

	if err := epo.eventRegistrar.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop event registrar: %w", err))
	}

	if err := epo.snapshotCapture.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop snapshot capture: %w", err))
	}

	if err := epo.clipRecorder.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop clip recorder: %w", err))
	}

	if err := epo.eventDetector.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop event detector: %w", err))
	}

	if err := epo.inferenceService.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop inference service: %w", err))
	}

	if err := epo.frameProcessor.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop frame processor: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping services: %v", errors)
	}

	epo.LogInfo("Event processing orchestrator stopped")
	return nil
}

// handleEventDetected handles a detected event
func (epo *EventProcessingOrchestrator) handleEventDetected(ctx context.Context, event *events.Event) {
	epo.logger.Info("Event detected, starting processing pipeline",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"event_type", event.EventType,
	)

	// Get triggering frame from frame buffer
	var triggeringFrame *video.Frame
	if buffer, ok := epo.frameProcessor.GetBuffer(event.CameraID); ok {
		triggeringFrame = buffer.GetLatestFrame()
	}

	// 1. Capture snapshot (triggering frame)
	if triggeringFrame != nil {
		_, err := epo.snapshotCapture.CaptureEventSnapshot(ctx, event, triggeringFrame)
		if err != nil {
			epo.logger.Warn("Failed to capture snapshot",
				"event_id", event.ID,
				"error", err,
			)
			// Continue processing even if snapshot fails
		}
	}

	// 2. Start clip recording
	cameraStreamInput := epo.getCameraStreamInput(event.CameraID)
	if cameraStreamInput != "" {
		err := epo.clipRecorder.RecordEventClip(ctx, event, cameraStreamInput)
		if err != nil {
			epo.logger.Warn("Failed to start clip recording",
				"event_id", event.ID,
				"error", err,
			)
			// Continue processing even if clip recording fails
		}
	} else {
		epo.logger.Warn("No camera stream input available for clip recording",
			"event_id", event.ID,
			"camera_id", event.CameraID,
		)
	}

	// 3. Register event in database
	err := epo.eventRegistrar.RegisterEvent(ctx, event)
	if err != nil {
		epo.logger.Error("Failed to register event",
			"event_id", event.ID,
			"error", err,
		)
		// This is critical - log error but continue
		return
	}

	// 4. Update event with clip and snapshot paths (if available)
	// Note: Clip path will be updated when recording completes
	if event.ClipPath != "" || event.SnapshotPath != "" {
		err = epo.eventRegistrar.UpdateEventPaths(ctx, event.ID, event.ClipPath, event.SnapshotPath)
		if err != nil {
			epo.logger.Warn("Failed to update event paths",
				"event_id", event.ID,
				"error", err,
			)
		}
	}

	// 5. Enqueue event for VM transmission
	err = epo.eventQueueManager.EnqueueEvent(ctx, event, 0)
	if err != nil {
		epo.logger.Warn("Failed to enqueue event",
			"event_id", event.ID,
			"error", err,
		)
		// Event is already registered, so this is non-critical
	}

	epo.logger.Info("Event processing pipeline completed",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"event_type", event.EventType,
		"clip_path", event.ClipPath,
		"snapshot_path", event.SnapshotPath,
	)
}

// getCameraStreamInput returns the camera stream input for a camera
func (epo *EventProcessingOrchestrator) getCameraStreamInput(cameraID string) string {
	epo.mu.RLock()
	defer epo.mu.RUnlock()
	return epo.cameraStreamInputs[cameraID]
}

// SetCameraStreamInput sets the camera stream input for a camera
func (epo *EventProcessingOrchestrator) SetCameraStreamInput(cameraID string, streamInput string) {
	epo.mu.Lock()
	defer epo.mu.Unlock()
	epo.cameraStreamInputs[cameraID] = streamInput
	epo.logger.Debug("Camera stream input set",
		"camera_id", cameraID,
		"stream_input", streamInput,
	)
}

// RemoveCameraStreamInput removes the camera stream input for a camera
func (epo *EventProcessingOrchestrator) RemoveCameraStreamInput(cameraID string) {
	epo.mu.Lock()
	defer epo.mu.Unlock()
	delete(epo.cameraStreamInputs, cameraID)
	epo.logger.Debug("Camera stream input removed", "camera_id", cameraID)
}

// UpdateConfig updates the orchestrator configuration
func (epo *EventProcessingOrchestrator) UpdateConfig(config *OrchestratorConfig) {
	epo.mu.Lock()
	defer epo.mu.Unlock()
	epo.config = config

	// Update service configurations
	if epo.frameProcessor != nil {
		epo.frameProcessor.SetInferenceInterval(config.InferenceInterval)
	}

	if epo.inferenceService != nil {
		epo.inferenceService.SetConfidenceThreshold(config.ConfidenceThreshold)
		epo.inferenceService.SetEnabledClasses(config.EnabledClasses)
	}

	if epo.eventDetector != nil {
		epo.eventDetector.SetConfidenceThreshold(config.ConfidenceThreshold)
		epo.eventDetector.SetMinEventDuration(config.MinEventDuration)
	}

	if epo.clipRecorder != nil {
		epo.clipRecorder.SetPreEventDuration(config.PreBufferDuration)
		epo.clipRecorder.SetPostEventDuration(config.PostEventDuration)
		epo.clipRecorder.SetMaxClipLength(config.MaxClipLength)
	}

	if epo.snapshotCapture != nil {
		epo.snapshotCapture.SetJPEGQuality(config.JPEGQuality)
	}

	if epo.eventQueueManager != nil {
		epo.eventQueueManager.SetMaxRetries(config.MaxRetries)
		epo.eventQueueManager.SetRetryDelays(config.RetryBaseDelay, config.RetryMaxDelay)
	}

	epo.logger.Info("Orchestrator configuration updated")
}

// GetConfig returns the current orchestrator configuration
func (epo *EventProcessingOrchestrator) GetConfig() *OrchestratorConfig {
	epo.mu.RLock()
	defer epo.mu.RUnlock()
	return epo.config
}

// GetServiceStatus returns the status of all services
func (epo *EventProcessingOrchestrator) GetServiceStatus() map[string]interface{} {
	status := make(map[string]interface{})

	status["frame_processor"] = map[string]interface{}{
		"running": epo.frameProcessor != nil,
	}

	status["inference_service"] = map[string]interface{}{
		"running": epo.inferenceService != nil,
	}

	status["event_detector"] = map[string]interface{}{
		"running": epo.eventDetector != nil,
	}

	status["clip_recorder"] = map[string]interface{}{
		"running": epo.clipRecorder != nil,
	}

	status["snapshot_capture"] = map[string]interface{}{
		"running": epo.snapshotCapture != nil,
	}

	status["event_registrar"] = map[string]interface{}{
		"running": epo.eventRegistrar != nil,
	}

	status["event_queue_manager"] = map[string]interface{}{
		"running": epo.eventQueueManager != nil,
	}

	return status
}

