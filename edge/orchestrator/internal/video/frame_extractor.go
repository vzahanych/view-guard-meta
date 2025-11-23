package video

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// Frame represents a single video frame
type Frame struct {
	Data      []byte    // JPEG-encoded frame data
	Timestamp time.Time // Frame timestamp
	Width     int       // Frame width
	Height    int       // Frame height
	CameraID  string    // Camera ID this frame came from
}

// FrameExtractor extracts frames from video streams
type FrameExtractor struct {
	logger        *logger.Logger
	ffmpeg        *FFmpegWrapper
	frameBuffer   chan *Frame
	bufferSize    int
	extractInterval time.Duration
	preprocessConfig PreprocessConfig
	onFrame       func(*Frame) // Callback for extracted frames
	mu            sync.RWMutex
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
}

// PreprocessConfig contains frame preprocessing settings
type PreprocessConfig struct {
	ResizeWidth   int  // Target width (0 = no resize)
	ResizeHeight  int  // Target height (0 = no resize)
	Normalize     bool // Normalize pixel values (for AI)
	Quality       int  // JPEG quality (1-100, default 85)
}

// FrameExtractorConfig contains frame extractor configuration
type FrameExtractorConfig struct {
	BufferSize      int              // Frame buffer size
	ExtractInterval time.Duration    // Interval between frame extractions
	Preprocess      PreprocessConfig // Preprocessing configuration
	OnFrame         func(*Frame)     // Callback for extracted frames
}

// NewFrameExtractor creates a new frame extractor
func NewFrameExtractor(
	ffmpeg *FFmpegWrapper,
	config FrameExtractorConfig,
	log *logger.Logger,
) *FrameExtractor {
	ctx, cancel := context.WithCancel(context.Background())

	// Default buffer size
	bufferSize := config.BufferSize
	if bufferSize == 0 {
		bufferSize = 10 // Default: 10 frames
	}

	// Default extract interval
	extractInterval := config.ExtractInterval
	if extractInterval == 0 {
		extractInterval = 1 * time.Second // Default: 1 frame per second
	}

	// Default preprocessing
	preprocess := config.Preprocess
	if preprocess.Quality == 0 {
		preprocess.Quality = 85 // Default JPEG quality
	}

	return &FrameExtractor{
		logger:          log,
		ffmpeg:          ffmpeg,
		frameBuffer:     make(chan *Frame, bufferSize),
		bufferSize:      bufferSize,
		extractInterval: extractInterval,
		preprocessConfig: preprocess,
		onFrame:         config.OnFrame,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts frame extraction from an input source
func (e *FrameExtractor) Start(input string, cameraID string) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("frame extractor already running")
	}
	e.running = true
	e.mu.Unlock()

	// Validate input
	if err := e.ffmpeg.ValidateInput(input); err != nil {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		return fmt.Errorf("invalid input: %w", err)
	}

	// Start extraction goroutine
	go e.extractFrames(input, cameraID)

	e.logger.Info("Frame extractor started",
		"input", input,
		"camera_id", cameraID,
		"interval", e.extractInterval,
		"buffer_size", e.bufferSize,
	)

	return nil
}

// Stop stops frame extraction
func (e *FrameExtractor) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	e.mu.Unlock()

	e.cancel()
	close(e.frameBuffer)

	e.logger.Info("Frame extractor stopped")
}

// extractFrames extracts frames from the input source
func (e *FrameExtractor) extractFrames(input string, cameraID string) {
	ticker := time.NewTicker(e.extractInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			frame, err := e.extractSingleFrame(input, cameraID)
			if err != nil {
				e.logger.Warn("Failed to extract frame", "error", err, "camera_id", cameraID)
				continue
			}

			// Preprocess frame
			processedFrame, err := e.preprocessFrame(frame)
			if err != nil {
				e.logger.Warn("Failed to preprocess frame", "error", err, "camera_id", cameraID)
				continue
			}

			// Add to buffer (non-blocking)
			select {
			case e.frameBuffer <- processedFrame:
				// Frame added to buffer
			default:
				// Buffer full, drop oldest frame
				select {
				case <-e.frameBuffer:
					// Removed oldest frame
					e.frameBuffer <- processedFrame
				default:
					// Buffer was already empty (shouldn't happen)
				}
				e.logger.Debug("Frame buffer full, dropped oldest frame", "camera_id", cameraID)
			}

			// Call callback if set
			if e.onFrame != nil {
				e.onFrame(processedFrame)
			}
		}
	}
}

// extractSingleFrame extracts a single frame from the input
func (e *FrameExtractor) extractSingleFrame(input string, cameraID string) (*Frame, error) {
	// Build FFmpeg command to extract a single frame
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", input,
		"-frames:v", "1", // Extract only 1 frame
		"-f", "image2pipe", // Output as image stream
		"-vcodec", "mjpeg", // Use MJPEG codec
		"-q:v", fmt.Sprintf("%d", e.preprocessConfig.Quality),
		"-", // Output to stdout
	}

	cmd := e.ffmpeg.BuildCommand(e.ctx, args)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{} // Suppress stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	frameData := stdout.Bytes()
	if len(frameData) == 0 {
		return nil, fmt.Errorf("no frame data extracted")
	}

	// Decode JPEG to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(frameData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode JPEG: %w", err)
	}

	bounds := img.Bounds()
	frame := &Frame{
		Data:      frameData,
		Timestamp: time.Now(),
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		CameraID:  cameraID,
	}

	return frame, nil
}

// preprocessFrame preprocesses a frame (resize, normalize)
func (e *FrameExtractor) preprocessFrame(frame *Frame) (*Frame, error) {
	// If no preprocessing needed, return as-is
	if e.preprocessConfig.ResizeWidth == 0 && e.preprocessConfig.ResizeHeight == 0 && !e.preprocessConfig.Normalize {
		return frame, nil
	}

	// Decode JPEG
	img, err := jpeg.Decode(bytes.NewReader(frame.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame: %w", err)
	}

	// Resize if needed
	if e.preprocessConfig.ResizeWidth > 0 || e.preprocessConfig.ResizeHeight > 0 {
		img = e.resizeImage(img, e.preprocessConfig.ResizeWidth, e.preprocessConfig.ResizeHeight)
	}

	// Normalize if needed (for AI inference)
	// Note: Normalization is typically done in the AI service, but we can prepare the image here
	// For now, we'll just resize and re-encode

	// Re-encode as JPEG
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: e.preprocessConfig.Quality})
	if err != nil {
		return nil, fmt.Errorf("failed to encode frame: %w", err)
	}

	// Update frame dimensions
	bounds := img.Bounds()
	frame.Data = buf.Bytes()
	frame.Width = bounds.Dx()
	frame.Height = bounds.Dy()

	return frame, nil
}

// resizeImage resizes an image
func (e *FrameExtractor) resizeImage(img image.Image, width, height int) image.Image {
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

	// Simple nearest-neighbor resize (for now)
	// In production, use a better resampling algorithm
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

// GetFrameBuffer returns the frame buffer channel
func (e *FrameExtractor) GetFrameBuffer() <-chan *Frame {
	return e.frameBuffer
}

// IsRunning returns whether the extractor is running
func (e *FrameExtractor) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// GetBufferSize returns the current buffer size
func (e *FrameExtractor) GetBufferSize() int {
	return e.bufferSize
}

// GetExtractInterval returns the extract interval
func (e *FrameExtractor) GetExtractInterval() time.Duration {
	return e.extractInterval
}

// SetExtractInterval sets the extract interval
func (e *FrameExtractor) SetExtractInterval(interval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.extractInterval = interval
}

// SetPreprocessConfig sets the preprocessing configuration
func (e *FrameExtractor) SetPreprocessConfig(config PreprocessConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.preprocessConfig = config
}

// ExtractFrameNow extracts a frame immediately (synchronous)
func (e *FrameExtractor) ExtractFrameNow(input string, cameraID string) (*Frame, error) {
	frame, err := e.extractSingleFrame(input, cameraID)
	if err != nil {
		return nil, err
	}

	// Preprocess
	processedFrame, err := e.preprocessFrame(frame)
	if err != nil {
		return nil, err
	}

	return processedFrame, nil
}

