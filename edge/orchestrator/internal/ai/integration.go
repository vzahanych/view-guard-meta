package ai

import (
	"context"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// FrameProcessor processes frames through the AI service
type FrameProcessor struct {
	client      *Client
	logger      *logger.Logger
	mu          sync.RWMutex
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc
	onDetection func(*DetectionResult) // Callback for detection results
	lastFrameTime map[string]time.Time  // cameraID -> last frame time
	interval    time.Duration          // Minimum interval between inferences per camera
}

// FrameProcessorConfig contains configuration for frame processor
type FrameProcessorConfig struct {
	Client      *Client
	Interval    time.Duration // Minimum interval between inferences per camera
	OnDetection func(*DetectionResult) // Callback for detection results
}

// NewFrameProcessor creates a new frame processor
func NewFrameProcessor(config FrameProcessorConfig, log *logger.Logger) *FrameProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	interval := config.Interval
	if interval == 0 {
		interval = time.Second // Default: 1 frame per second per camera
	}

	return &FrameProcessor{
		client:        config.Client,
		logger:        log,
		ctx:           ctx,
		cancel:        cancel,
		onDetection:   config.OnDetection,
		lastFrameTime: make(map[string]time.Time),
		interval:      interval,
	}
}

// ProcessFrame processes a frame through the AI service
func (fp *FrameProcessor) ProcessFrame(ctx context.Context, frame *video.Frame) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	// Check if we should skip this frame (rate limiting)
	lastTime, exists := fp.lastFrameTime[frame.CameraID]
	if exists && time.Since(lastTime) < fp.interval {
		// Skip frame - too soon since last inference
		return nil
	}

	// Update last frame time
	fp.lastFrameTime[frame.CameraID] = time.Now()

	// Perform inference in goroutine to avoid blocking
	go func() {
		detectionCtx, cancel := context.WithTimeout(fp.ctx, 30*time.Second)
		defer cancel()

		resp, err := fp.client.Infer(detectionCtx, frame)
		if err != nil {
			fp.logger.Warn(
				"Inference failed",
				"camera_id", frame.CameraID,
				"error", err,
			)
			return
		}

		// Create detection result
		result := &DetectionResult{
			Response:      resp,
			FrameTimestamp: frame.Timestamp,
			CameraID:      frame.CameraID,
			FrameWidth:    frame.Width,
			FrameHeight:   frame.Height,
		}

		// Call callback if set
		if fp.onDetection != nil {
			fp.onDetection(result)
		}
	}()

	return nil
}

// Start starts the frame processor
func (fp *FrameProcessor) Start() error {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	if fp.running {
		return nil
	}

	fp.running = true
	fp.logger.Info("Frame processor started")

	return nil
}

// Stop stops the frame processor
func (fp *FrameProcessor) Stop() {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	if !fp.running {
		return
	}

	fp.cancel()
	fp.running = false
	fp.logger.Info("Frame processor stopped")
}

// IsRunning returns whether the processor is running
func (fp *FrameProcessor) IsRunning() bool {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.running
}

// SetInterval updates the inference interval
func (fp *FrameProcessor) SetInterval(interval time.Duration) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.interval = interval
}

