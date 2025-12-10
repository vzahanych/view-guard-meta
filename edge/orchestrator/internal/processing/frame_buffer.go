package processing

import (
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// FrameBufferConfig contains frame buffer configuration
type FrameBufferConfig struct {
	Duration  time.Duration // Maximum duration of frames to keep (e.g., 5-10 seconds)
	MaxFrames int           // Maximum number of frames to keep (0 = no limit, only duration-based)
}

// FrameBuffer maintains a circular buffer of frames for pre-event recording
type FrameBuffer struct {
	cameraID string
	config   FrameBufferConfig
	frames   []*video.Frame
	mu       sync.RWMutex
	logger   *logger.Logger
	closed   bool
	// Statistics
	totalFramesAdded int64
	framesDropped    int64
}

// NewFrameBuffer creates a new frame buffer
func NewFrameBuffer(cameraID string, config FrameBufferConfig, log *logger.Logger) *FrameBuffer {
	return &FrameBuffer{
		cameraID: cameraID,
		config:   config,
		frames:   make([]*video.Frame, 0),
		logger:   log,
		closed:   false,
	}
}

// NewFrameBufferWithDuration creates a new frame buffer with duration only (backward compatibility)
func NewFrameBufferWithDuration(cameraID string, duration time.Duration, log *logger.Logger) *FrameBuffer {
	return NewFrameBuffer(cameraID, FrameBufferConfig{
		Duration:  duration,
		MaxFrames: 0, // No frame limit, only duration-based
	}, log)
}

// AddFrame adds a frame to the buffer
func (fb *FrameBuffer) AddFrame(frame *video.Frame) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.closed {
		return
	}

	fb.totalFramesAdded++

	// Add frame
	fb.frames = append(fb.frames, frame)

	// Remove frames based on duration limit
	if fb.config.Duration > 0 {
		now := time.Now()
		cutoffTime := now.Add(-fb.config.Duration)

		// Find first frame that's still within duration
		startIdx := 0
		for i, f := range fb.frames {
			if f.Timestamp.After(cutoffTime) {
				startIdx = i
				break
			}
		}

		// Remove old frames
		if startIdx > 0 {
			fb.framesDropped += int64(startIdx)
			fb.frames = fb.frames[startIdx:]
		}
	}

	// Remove frames based on max frames limit
	if fb.config.MaxFrames > 0 && len(fb.frames) > fb.config.MaxFrames {
		// Remove oldest frames (keep only the most recent MaxFrames)
		dropCount := len(fb.frames) - fb.config.MaxFrames
		fb.framesDropped += int64(dropCount)
		fb.frames = fb.frames[dropCount:]
	}
}

// GetLatestFrame returns the most recent frame
func (fb *FrameBuffer) GetLatestFrame() *video.Frame {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	if len(fb.frames) == 0 {
		return nil
	}

	return fb.frames[len(fb.frames)-1]
}

// GetFrames returns all frames in the buffer
func (fb *FrameBuffer) GetFrames() []*video.Frame {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	// Return a copy to avoid race conditions
	frames := make([]*video.Frame, len(fb.frames))
	copy(frames, fb.frames)
	return frames
}

// GetFramesInRange returns frames within a time range
func (fb *FrameBuffer) GetFramesInRange(startTime, endTime time.Time) []*video.Frame {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	var result []*video.Frame
	for _, frame := range fb.frames {
		if (frame.Timestamp.After(startTime) || frame.Timestamp.Equal(startTime)) &&
			(frame.Timestamp.Before(endTime) || frame.Timestamp.Equal(endTime)) {
			result = append(result, frame)
		}
	}
	return result
}

// GetFramesBefore returns all frames before a given time (for pre-event recording)
func (fb *FrameBuffer) GetFramesBefore(beforeTime time.Time) []*video.Frame {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	var result []*video.Frame
	for _, frame := range fb.frames {
		if frame.Timestamp.Before(beforeTime) {
			result = append(result, frame)
		}
	}
	return result
}

// Clear removes all frames from the buffer
func (fb *FrameBuffer) Clear() {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.frames = fb.frames[:0]
}

// Size returns the number of frames in the buffer
func (fb *FrameBuffer) Size() int {
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	return len(fb.frames)
}

// Close closes the buffer
func (fb *FrameBuffer) Close() {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.closed = true
	fb.frames = nil
}

// IsClosed returns whether the buffer is closed
func (fb *FrameBuffer) IsClosed() bool {
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	return fb.closed
}

// GetConfig returns the buffer configuration
func (fb *FrameBuffer) GetConfig() FrameBufferConfig {
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	return fb.config
}

// SetConfig updates the buffer configuration
func (fb *FrameBuffer) SetConfig(config FrameBufferConfig) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.closed {
		return
	}

	fb.config = config

	// Apply new limits immediately
	if fb.config.Duration > 0 {
		now := time.Now()
		cutoffTime := now.Add(-fb.config.Duration)

		// Find first frame that's still within duration
		startIdx := 0
		for i, f := range fb.frames {
			if f.Timestamp.After(cutoffTime) {
				startIdx = i
				break
			}
		}

		// Remove old frames
		if startIdx > 0 {
			fb.framesDropped += int64(startIdx)
			fb.frames = fb.frames[startIdx:]
		}
	}

	if fb.config.MaxFrames > 0 && len(fb.frames) > fb.config.MaxFrames {
		dropCount := len(fb.frames) - fb.config.MaxFrames
		fb.framesDropped += int64(dropCount)
		fb.frames = fb.frames[dropCount:]
	}
}

// GetStats returns buffer statistics
func (fb *FrameBuffer) GetStats() FrameBufferStats {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	oldestTime := time.Time{}
	newestTime := time.Time{}
	if len(fb.frames) > 0 {
		oldestTime = fb.frames[0].Timestamp
		newestTime = fb.frames[len(fb.frames)-1].Timestamp
	}

	return FrameBufferStats{
		CameraID:         fb.cameraID,
		CurrentSize:      len(fb.frames),
		TotalFramesAdded: fb.totalFramesAdded,
		FramesDropped:    fb.framesDropped,
		OldestFrameTime:  oldestTime,
		NewestFrameTime:  newestTime,
		Duration:         fb.config.Duration,
		MaxFrames:       fb.config.MaxFrames,
		Closed:           fb.closed,
	}
}

// FrameBufferStats contains statistics about the frame buffer
type FrameBufferStats struct {
	CameraID         string
	CurrentSize      int
	TotalFramesAdded int64
	FramesDropped    int64
	OldestFrameTime  time.Time
	NewestFrameTime  time.Time
	Duration         time.Duration
	MaxFrames        int
	Closed           bool
}

