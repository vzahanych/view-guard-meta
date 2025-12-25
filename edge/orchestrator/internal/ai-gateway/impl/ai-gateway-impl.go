package impl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	statemng "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	"go.uber.org/zap"
)

// aiGatewayImpl implements the AIGateway interface
type aiGatewayImpl struct {
	config        *types.AIGatewayConfig
	logger        *zap.Logger
	aiClient      *AIClient
	cctvService   cctv.CCTVService
	eventCallback func(*statemng.SecurityEvent)

	// Frame processing state
	processing map[string]*cameraProcessor // cameraID -> processor
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc

	// Configuration
	confidenceThreshold float64
	inferenceInterval   time.Duration
}

// cameraProcessor handles frame processing for a single camera
type cameraProcessor struct {
	cameraID        string
	onFrameCallback func([]byte, time.Time)
	stopCapture     context.CancelFunc
	stats           *types.ProcessingStats
	mu              sync.RWMutex
}

// NewAIGateway creates a new AI gateway implementation
func NewAIGatewayImpl(
	cfg *types.AIGatewayConfig,
	cctvSvc cctv.CCTVService,
	log *zap.Logger,
) (*aiGatewayImpl, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create AI client with config values
	requestTimeout := cfg.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = 30 * time.Second // Default if not set
	}
	aiClientConfig := AIClientConfig{
		ServiceURL:          cfg.AIServiceURL,
		Timeout:             requestTimeout,
		ConfidenceThreshold: cfg.ConfidenceThreshold,
		EnabledClasses:      cfg.EnabledClasses,
		MaxRetries:          cfg.MaxRetries,
		RetryDelay:          cfg.RetryDelay,
	}
	aiClient, err := NewAIClient(aiClientConfig, log)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create AI client: %w", err)
	}

	gateway := &aiGatewayImpl{
		config:              cfg,
		logger:              log,
		aiClient:            aiClient,
		cctvService:         cctvSvc,
		processing:          make(map[string]*cameraProcessor),
		ctx:                 ctx,
		cancel:              cancel,
		confidenceThreshold: cfg.ConfidenceThreshold,
		inferenceInterval:   cfg.InferenceInterval,
	}

	return gateway, nil
}

// Name returns the service name
func (g *aiGatewayImpl) Name() string {
	return "ai-gateway"
}

// Start starts the AI gateway service
func (g *aiGatewayImpl) Start(ctx context.Context) error {
	g.logger.Info("Starting AI gateway")

	// Health check AI service
	if err := g.aiClient.HealthCheck(ctx); err != nil {
		g.logger.Warn("AI service health check failed, gateway will continue", zap.Error(err))
	} else {
		g.logger.Info("AI service is healthy")
	}

	g.logger.Info("AI gateway started")
	return nil
}

// Stop stops the AI gateway service
func (g *aiGatewayImpl) Stop(ctx context.Context) error {
	g.logger.Info("Stopping AI gateway")

	// Stop all frame processing
	g.mu.Lock()
	for cameraID := range g.processing {
		g.stopFrameProcessingLocked(ctx, cameraID)
	}
	g.mu.Unlock()

	// Close HTTP client (no-op, but kept for consistency)
	if g.aiClient != nil {
		_ = g.aiClient.Close()
	}

	g.cancel()
	g.logger.Info("AI gateway stopped")
	return nil
}

// StartFrameProcessing starts processing frames from a camera
func (g *aiGatewayImpl) StartFrameProcessing(ctx context.Context, cameraID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.processing[cameraID]; exists {
		return fmt.Errorf("frame processing already started for camera %s", cameraID)
	}

	// Create processor
	processor := &cameraProcessor{
		cameraID: cameraID,
		stats: &types.ProcessingStats{
			CameraID:     cameraID,
			IsProcessing: true,
		},
	}

	// Create context for frame capture
	captureCtx, cancel := context.WithCancel(g.ctx)
	processor.stopCapture = cancel

	// Set up frame callback
	processor.onFrameCallback = func(frameData []byte, timestamp time.Time) {
		g.processFrame(captureCtx, cameraID, frameData, timestamp)
	}

	// Adapter to convert from *cctvtypes.Frame to the expected callback signature
	frameCallback := func(frame *cctvtypes.Frame) {
		processor.onFrameCallback(frame.Data, frame.Timestamp)
	}

	// Start frame capture from CCTV service
	// Note: StartFrameCapture may return "not implemented" if the feature is not yet implemented
	if err := g.cctvService.StartFrameCapture(captureCtx, cameraID, frameCallback); err != nil {
		cancel()
		// Check if it's a "not implemented" error
		if err.Error() == "not implemented" {
			g.logger.Warn("Frame capture not yet implemented in CCTV service, frame processing will not start",
				zap.String("camera_id", cameraID))
			return fmt.Errorf("frame capture not implemented: %w", err)
		}
		return fmt.Errorf("failed to start frame capture: %w", err)
	}

	g.processing[cameraID] = processor
	g.logger.Info("Started frame processing", zap.String("camera_id", cameraID))
	return nil
}

// StopFrameProcessing stops processing frames from a camera
func (g *aiGatewayImpl) StopFrameProcessing(ctx context.Context, cameraID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stopFrameProcessingLocked(ctx, cameraID)
}

// stopFrameProcessingLocked stops frame processing (must be called with lock held)
func (g *aiGatewayImpl) stopFrameProcessingLocked(ctx context.Context, cameraID string) error {
	processor, exists := g.processing[cameraID]
	if !exists {
		return fmt.Errorf("frame processing not started for camera %s", cameraID)
	}

	// Stop frame capture
	if processor.stopCapture != nil {
		processor.stopCapture()
	}

	// Stop frame capture in CCTV service
	if err := g.cctvService.StopFrameCapture(ctx, cameraID); err != nil {
		g.logger.Warn("Failed to stop frame capture", zap.Error(err), zap.String("camera_id", cameraID))
	}

	// Update stats
	processor.mu.Lock()
	processor.stats.IsProcessing = false
	processor.mu.Unlock()

	delete(g.processing, cameraID)
	g.logger.Info("Stopped frame processing", zap.String("camera_id", cameraID))
	return nil
}

// IsProcessing returns whether frames are being processed for a camera
func (g *aiGatewayImpl) IsProcessing(cameraID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.processing[cameraID]
	return exists
}

// GetProcessingStats returns statistics about frame processing
func (g *aiGatewayImpl) GetProcessingStats(cameraID string) (*types.ProcessingStats, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	processor, exists := g.processing[cameraID]
	if !exists {
		return nil, fmt.Errorf("frame processing not started for camera %s", cameraID)
	}

	processor.mu.RLock()
	defer processor.mu.RUnlock()

	// Return a copy of stats
	stats := *processor.stats
	return &stats, nil
}

// SetEventCallback sets a callback function that will be called when a security event is detected
func (g *aiGatewayImpl) SetEventCallback(callback func(*statemng.SecurityEvent)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.eventCallback = callback
}

// SetConfidenceThreshold sets the confidence threshold for anomaly detection
func (g *aiGatewayImpl) SetConfidenceThreshold(threshold float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.confidenceThreshold = threshold
	g.aiClient.SetConfidenceThreshold(threshold)
}

// GetConfidenceThreshold returns the current confidence threshold
func (g *aiGatewayImpl) GetConfidenceThreshold() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.confidenceThreshold
}

// processFrame processes a single frame through the AI service
func (g *aiGatewayImpl) processFrame(ctx context.Context, cameraID string, frameData []byte, timestamp time.Time) {
	processor := g.getProcessor(cameraID)
	if processor == nil {
		return
	}

	startTime := time.Now()

	// Create cctvtypes.Frame from frame data
	frame := &cctvtypes.Frame{
		CameraID:  cameraID,
		Data:      frameData,
		Timestamp: timestamp,
		Width:     0, // Will be determined by AI service
		Height:    0, // Will be determined by AI service
	}

	// Perform inference with timeout from config
	requestTimeout := g.config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = 30 * time.Second // Default if not set
	}
	inferenceCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := g.aiClient.Infer(inferenceCtx, frame)
	processingTime := time.Since(startTime)

	processor.mu.Lock()
	processor.stats.FramesProcessed++
	processor.stats.LastProcessedAt = time.Now()
	// Update average processing time (simple moving average)
	if processor.stats.AverageProcessingTime == 0 {
		processor.stats.AverageProcessingTime = processingTime
	} else {
		processor.stats.AverageProcessingTime = (processor.stats.AverageProcessingTime + processingTime) / 2
	}
	processor.mu.Unlock()

	if err != nil {
		g.logger.Warn("Inference failed", zap.Error(err), zap.String("camera_id", cameraID))
		processor.mu.Lock()
		processor.stats.ErrorCount++
		processor.mu.Unlock()
		return
	}

	// Check if any detections exceed the confidence threshold (abnormal frame)
	if resp.DetectionCount > 0 {
		// Check if any detection has high confidence (indicating abnormality)
		hasAbnormalDetection := false
		for _, box := range resp.BoundingBoxes {
			if box.Confidence >= g.confidenceThreshold {
				hasAbnormalDetection = true
				break
			}
		}

		if hasAbnormalDetection {
			// Create event for abnormal frame
			event := g.createEventFromDetection(cameraID, resp, timestamp)

			processor.mu.Lock()
			processor.stats.EventsDetected++
			processor.mu.Unlock()

			// Call event callback if set
			if g.eventCallback != nil {
				g.eventCallback(event)
			}

			g.logger.Info("Abnormal frame detected",
				zap.String("camera_id", cameraID),
				zap.String("event_id", event.ID),
				zap.Int("detection_count", resp.DetectionCount),
				zap.Float64("confidence", resp.BoundingBoxes[0].Confidence),
			)
		}
	}
}

// createEventFromDetection creates a SecurityEvent from an inference response
func (g *aiGatewayImpl) createEventFromDetection(
	cameraID string,
	resp *types.InferenceResponse,
	timestamp time.Time,
) *statemng.SecurityEvent {
	event := statemng.NewSecurityEvent()
	event.CameraID = cameraID
	event.Timestamp = timestamp
	event.EventType = statemng.SecurityEventTypeAnomalyDetected

	// Use the highest confidence detection
	if len(resp.BoundingBoxes) > 0 {
		bestBox := resp.BoundingBoxes[0]
		for _, box := range resp.BoundingBoxes {
			if box.Confidence > bestBox.Confidence {
				bestBox = box
			}
		}

		event.Confidence = bestBox.Confidence
		event.BoundingBox = &types.BoundingBox{
			X1:         bestBox.X1,
			Y1:         bestBox.Y1,
			X2:         bestBox.X2,
			Y2:         bestBox.Y2,
			Confidence: bestBox.Confidence,
			ClassID:    bestBox.ClassID,
			ClassName:  bestBox.ClassName,
		}

		// Map class ID to security event type if applicable
		if eventType, ok := statemng.ClassIDToSecurityEventType[bestBox.ClassID]; ok {
			event.EventType = eventType
		}
	}

	// Add metadata
	event.Metadata = map[string]interface{}{
		"detection_count":   resp.DetectionCount,
		"inference_time_ms": resp.InferenceTimeMs,
		"frame_shape":       resp.FrameShape,
		"model_input_shape": resp.ModelInputShape,
		"all_detections":    resp.BoundingBoxes,
	}

	return event
}

// getProcessor gets the processor for a camera (must be called with lock held or in safe context)
func (g *aiGatewayImpl) getProcessor(cameraID string) *cameraProcessor {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.processing[cameraID]
}

// NotifyModelDeployment notifies the AI service about a deployed model
// ProcessFrame processes a frame that was stored in object storage
// The AI service will:
// 1. Load the model from MinIO (if not already loaded) using model metadata
// 2. Process the frame from object storage
// 3. Determine if it's similar to training set or has anomalies
// 4. Delete normal frames or move suspicious ones to security event bucket
func (g *aiGatewayImpl) ProcessFrame(ctx context.Context, cameraID string, frameKey string, frameData []byte) error {
	if frameKey == "" {
		return fmt.Errorf("frame key is required")
	}
	if len(frameData) == 0 {
		return fmt.Errorf("frame data is required")
	}

	// Process frame using existing processFrame logic
	// This will:
	// 1. Send frame to AI service for inference
	// 2. Check if detections exceed confidence threshold
	// 3. Create security events for abnormal frames
	timestamp := time.Now()
	g.processFrame(ctx, cameraID, frameData, timestamp)

	// Note: Frame deletion/moving to security event bucket should be handled by AI service
	// or by the state manager based on the inference results
	// For now, we rely on the AI service to handle frame lifecycle

	return nil
}

func (g *aiGatewayImpl) NotifyModelDeployment(ctx context.Context, metadata *types.ModelMetadata) error {
	return g.aiClient.NotifyModelDeployment(ctx, metadata)
}
