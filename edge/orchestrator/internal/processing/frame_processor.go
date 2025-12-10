package processing

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// FrameProcessor processes camera frames for event detection
type FrameProcessor struct {
	*service.ServiceBase
	logger            *logger.Logger
	buffers           map[string]*FrameBuffer // cameraID -> buffer
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	preprocessConfig  PreprocessConfig
	inferenceInterval time.Duration
	onInferenceFrame  func(*video.Frame) // Callback for frames to send to inference
}

// PreprocessConfig contains frame preprocessing settings
type PreprocessConfig struct {
	ResizeWidth   int  // Target width (0 = no resize)
	ResizeHeight  int  // Target height (0 = no resize)
	Normalize     bool // Normalize pixel values (for AI) - placeholder for future
	Quality       int  // JPEG quality (1-100, default 85)
}

// FrameProcessorConfig contains frame processor configuration
type FrameProcessorConfig struct {
	PreprocessConfig   PreprocessConfig
	InferenceInterval  time.Duration // Interval between frames sent to inference (e.g., 1 FPS)
	PreBufferDuration  time.Duration // Duration of pre-event buffer (e.g., 5 seconds)
	OnInferenceFrame   func(*video.Frame) // Callback for frames to send to inference service
}

// NewFrameProcessor creates a new frame processor
func NewFrameProcessor(config FrameProcessorConfig, log *logger.Logger) *FrameProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	// Default inference interval (1 FPS)
	inferenceInterval := config.InferenceInterval
	if inferenceInterval == 0 {
		inferenceInterval = 1 * time.Second
	}

	// Default pre-buffer duration (5 seconds)
	preBufferDuration := config.PreBufferDuration
	if preBufferDuration == 0 {
		preBufferDuration = 5 * time.Second
	}

	// Default preprocessing
	preprocess := config.PreprocessConfig
	if preprocess.Quality == 0 {
		preprocess.Quality = 85 // Default JPEG quality
	}

	return &FrameProcessor{
		ServiceBase:       service.NewServiceBase("frame-processor", log),
		logger:            log,
		buffers:           make(map[string]*FrameBuffer),
		ctx:               ctx,
		cancel:            cancel,
		preprocessConfig:  preprocess,
		inferenceInterval: inferenceInterval,
		onInferenceFrame:  config.OnInferenceFrame,
	}
}

// Start starts the frame processor
func (fp *FrameProcessor) Start(ctx context.Context) error {
	if fp.GetEventBus() == nil {
		return fmt.Errorf("event bus not configured")
	}

	// Subscribe to frame received events
	eventCh := fp.GetEventBus().Subscribe(service.EventTypeFrameReceived)

	// Start event handler goroutine
	go fp.handleFrameEvents(ctx, eventCh)

	// Start inference frame distributor
	go fp.distributeInferenceFrames(ctx)

	fp.LogInfo("Frame processor started",
		"inference_interval", fp.inferenceInterval,
		"preprocess", fmt.Sprintf("%dx%d", fp.preprocessConfig.ResizeWidth, fp.preprocessConfig.ResizeHeight),
	)

	return nil
}

// Stop stops the frame processor
func (fp *FrameProcessor) Stop() error {
	fp.cancel()

	fp.mu.Lock()
	defer fp.mu.Unlock()

	// Clean up all buffers
	for cameraID, buffer := range fp.buffers {
		buffer.Close()
		delete(fp.buffers, cameraID)
	}

	fp.LogInfo("Frame processor stopped")
	return nil
}

// handleFrameEvents handles frame received events from the event bus
func (fp *FrameProcessor) handleFrameEvents(ctx context.Context, eventCh <-chan service.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-fp.ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}

			// Extract frame data from event
			cameraID, ok := event.Data["camera_id"].(string)
			if !ok {
				fp.logger.Warn("Frame event missing camera_id", "event", event.Type)
				continue
			}

			frameData, ok := event.Data["frame_data"].([]byte)
			if !ok {
				fp.logger.Warn("Frame event missing frame_data", "camera_id", cameraID)
				continue
			}

			timestamp, ok := event.Data["timestamp"].(time.Time)
			if !ok {
				// Try parsing from string if needed
				if tsStr, ok := event.Data["timestamp"].(string); ok {
					var err error
					timestamp, err = time.Parse(time.RFC3339, tsStr)
					if err != nil {
						timestamp = time.Now()
					}
				} else {
					timestamp = time.Now()
				}
			}

			// Process frame
			if err := fp.processFrame(cameraID, frameData, timestamp); err != nil {
				fp.LogError("Failed to process frame", err, "camera_id", cameraID)
			}
		}
	}
}

// processFrame processes a single frame
func (fp *FrameProcessor) processFrame(cameraID string, frameData []byte, timestamp time.Time) error {
	// Decode frame to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(frameData))
	if err != nil {
		return fmt.Errorf("failed to decode frame: %w", err)
	}

	bounds := img.Bounds()
	originalFrame := &video.Frame{
		Data:      frameData,
		Timestamp: timestamp,
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		CameraID:  cameraID,
	}

	// Preprocess frame for model input
	processedFrame, err := fp.preprocessFrame(originalFrame)
	if err != nil {
		return fmt.Errorf("failed to preprocess frame: %w", err)
	}

	// Get or create buffer for this camera
	buffer := fp.getOrCreateBuffer(cameraID)

	// Add frame to buffer (for pre-event recording)
	// Buffer automatically handles overflow (oldest frames discarded)
	buffer.AddFrame(processedFrame)

	return nil
}

// preprocessFrame preprocesses a frame for model input
func (fp *FrameProcessor) preprocessFrame(frame *video.Frame) (*video.Frame, error) {
	// If no preprocessing needed, return as-is
	if fp.preprocessConfig.ResizeWidth == 0 && fp.preprocessConfig.ResizeHeight == 0 && !fp.preprocessConfig.Normalize {
		return frame, nil
	}

	// Decode JPEG
	img, err := jpeg.Decode(bytes.NewReader(frame.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame: %w", err)
	}

	// Resize if needed
	if fp.preprocessConfig.ResizeWidth > 0 || fp.preprocessConfig.ResizeHeight > 0 {
		img = fp.resizeImage(img, fp.preprocessConfig.ResizeWidth, fp.preprocessConfig.ResizeHeight)
	}

	// Normalize if needed (for AI inference)
	// Note: Normalization is typically done in the AI service, but we can prepare the image here
	// For now, we'll just resize and re-encode

	// Re-encode as JPEG
	var buf []byte
	buf, err = fp.encodeJPEG(img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode frame: %w", err)
	}

	// Update frame dimensions
	bounds := img.Bounds()
	frame.Data = buf
	frame.Width = bounds.Dx()
	frame.Height = bounds.Dy()

	return frame, nil
}

// resizeImage resizes an image maintaining aspect ratio
func (fp *FrameProcessor) resizeImage(img image.Image, width, height int) image.Image {
	// If dimensions not specified, return original
	if width == 0 && height == 0 {
		return img
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// Calculate dimensions maintaining aspect ratio
	if width == 0 {
		width = (origWidth * height) / origHeight
	}
	if height == 0 {
		height = (origHeight * width) / origWidth
	}

	// Simple nearest-neighbor resize (for PoC)
	// In production, use a better resampling algorithm (e.g., Lanczos)
	resized := image.NewRGBA(image.Rect(0, 0, width, height))

	// Simple nearest-neighbor scaling
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := (x * origWidth) / width
			srcY := (y * origHeight) / height
			resized.Set(x, y, img.At(srcX, srcY))
		}
	}

	return resized
}

// encodeJPEG encodes an image as JPEG
func (fp *FrameProcessor) encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: fp.preprocessConfig.Quality})
	if err != nil {
		return nil, fmt.Errorf("failed to encode JPEG: %w", err)
	}
	return buf.Bytes(), nil
}

// getOrCreateBuffer gets or creates a frame buffer for a camera
func (fp *FrameProcessor) getOrCreateBuffer(cameraID string) *FrameBuffer {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	buffer, ok := fp.buffers[cameraID]
	if !ok {
		// Create new buffer with default pre-buffer duration (5 seconds)
		// This will be configurable in the future
		buffer = NewFrameBuffer(cameraID, FrameBufferConfig{
			Duration:  5 * time.Second, // Default: keep last 5 seconds of frames
			MaxFrames: 0,               // No frame limit, only duration-based
		}, fp.logger)
		fp.buffers[cameraID] = buffer
	}

	return buffer
}

// distributeInferenceFrames distributes frames to inference service at configured interval
func (fp *FrameProcessor) distributeInferenceFrames(ctx context.Context) {
	ticker := time.NewTicker(fp.inferenceInterval)
	defer ticker.Stop()

	lastInferenceTime := make(map[string]time.Time) // cameraID -> last inference time

	for {
		select {
		case <-ctx.Done():
			return
		case <-fp.ctx.Done():
			return
		case <-ticker.C:
			// Get all active buffers
			fp.mu.RLock()
			buffers := make(map[string]*FrameBuffer)
			for cameraID, buffer := range fp.buffers {
				buffers[cameraID] = buffer
			}
			fp.mu.RUnlock()

			// Get latest frame from each buffer and send to inference
			for cameraID, buffer := range buffers {
				// Check if enough time has passed since last inference for this camera
				if lastTime, ok := lastInferenceTime[cameraID]; ok {
					if time.Since(lastTime) < fp.inferenceInterval {
						continue
					}
				}

				// Get latest frame from buffer
				latestFrame := buffer.GetLatestFrame()
				if latestFrame != nil && fp.onInferenceFrame != nil {
					fp.onInferenceFrame(latestFrame)
					lastInferenceTime[cameraID] = time.Now()
				}
			}
		}
	}
}

// GetBuffer returns the frame buffer for a camera
func (fp *FrameProcessor) GetBuffer(cameraID string) (*FrameBuffer, bool) {
	fp.mu.RLock()
	defer fp.mu.RUnlock()

	buffer, ok := fp.buffers[cameraID]
	return buffer, ok
}

// RemoveBuffer removes the frame buffer for a camera (e.g., on camera disconnect)
func (fp *FrameProcessor) RemoveBuffer(cameraID string) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	if buffer, ok := fp.buffers[cameraID]; ok {
		// Get stats before closing for logging
		stats := buffer.GetStats()
		buffer.Close()
		delete(fp.buffers, cameraID)
		fp.LogInfo("Frame buffer removed",
			"camera_id", cameraID,
			"total_frames", stats.TotalFramesAdded,
			"frames_dropped", stats.FramesDropped,
		)
	}
}

// SetInferenceInterval sets the inference interval
func (fp *FrameProcessor) SetInferenceInterval(interval time.Duration) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.inferenceInterval = interval
}

// SetPreprocessConfig sets the preprocessing configuration
func (fp *FrameProcessor) SetPreprocessConfig(config PreprocessConfig) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.preprocessConfig = config
}

