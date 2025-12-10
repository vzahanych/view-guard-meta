package processing

import (
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

func TestNewFrameBuffer(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  5 * time.Second,
		MaxFrames: 10,
	}

	buffer := NewFrameBuffer("camera-1", config, log)
	if buffer == nil {
		t.Fatal("NewFrameBuffer returned nil")
	}

	if buffer.cameraID != "camera-1" {
		t.Errorf("Expected cameraID 'camera-1', got '%s'", buffer.cameraID)
	}

	if buffer.config.Duration != 5*time.Second {
		t.Errorf("Expected duration 5s, got %v", buffer.config.Duration)
	}

	if buffer.config.MaxFrames != 10 {
		t.Errorf("Expected max frames 10, got %d", buffer.config.MaxFrames)
	}
}

func TestFrameBuffer_AddFrame(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  5 * time.Second,
		MaxFrames: 3,
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	// Add frames
	for i := 0; i < 5; i++ {
		frame := &video.Frame{
			CameraID:  "camera-1",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Data:      []byte{1, 2, 3, byte(i)},
		}
		buffer.AddFrame(frame)
	}

	// Should only have 3 frames (max limit)
	if buffer.Size() != 3 {
		t.Errorf("Expected 3 frames, got %d", buffer.Size())
	}

	// Check that oldest frames were dropped
	frames := buffer.GetFrames()
	if len(frames) != 3 {
		t.Fatalf("Expected 3 frames, got %d", len(frames))
	}

	// Last frame should be the most recent
	if frames[2].Data[3] != 4 {
		t.Errorf("Expected last frame data[3] to be 4, got %d", frames[2].Data[3])
	}
}

func TestFrameBuffer_DurationBasedEviction(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  2 * time.Second,
		MaxFrames: 0, // No frame limit
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	now := time.Now()

	// Add old frame (3 seconds ago)
	oldFrame := &video.Frame{
		CameraID:  "camera-1",
		Timestamp: now.Add(-3 * time.Second),
		Data:      []byte{1},
	}
	buffer.AddFrame(oldFrame)

	// Add recent frame (1 second ago)
	recentFrame := &video.Frame{
		CameraID:  "camera-1",
		Timestamp: now.Add(-1 * time.Second),
		Data:      []byte{2},
	}
	buffer.AddFrame(recentFrame)

	// Add current frame
	currentFrame := &video.Frame{
		CameraID:  "camera-1",
		Timestamp: now,
		Data:      []byte{3},
	}
	buffer.AddFrame(currentFrame)

	// Old frame should be evicted
	frames := buffer.GetFrames()
	if len(frames) != 2 {
		t.Errorf("Expected 2 frames after eviction, got %d", len(frames))
	}

	// Check that old frame is not present
	for _, frame := range frames {
		if len(frame.Data) > 0 && frame.Data[0] == 1 {
			t.Error("Old frame should have been evicted")
		}
	}
}

func TestFrameBuffer_GetLatestFrame(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  5 * time.Second,
		MaxFrames: 10,
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	// Add multiple frames
	for i := 0; i < 5; i++ {
		frame := &video.Frame{
			CameraID:  "camera-1",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Data:      []byte{byte(i)},
		}
		buffer.AddFrame(frame)
	}

	latest := buffer.GetLatestFrame()
	if latest == nil {
		t.Fatal("GetLatestFrame returned nil")
	}

	if latest.Data[0] != 4 {
		t.Errorf("Expected latest frame data[0] to be 4, got %d", latest.Data[0])
	}
}

func TestFrameBuffer_GetFramesInRange(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  10 * time.Second,
		MaxFrames: 0,
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	now := time.Now()

	// Add frames at different times
	for i := 0; i < 5; i++ {
		frame := &video.Frame{
			CameraID:  "camera-1",
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Data:      []byte{byte(i)},
		}
		buffer.AddFrame(frame)
	}

	// Get frames in range (1-3 seconds)
	startTime := now.Add(1 * time.Second)
	endTime := now.Add(3 * time.Second)
	frames := buffer.GetFramesInRange(startTime, endTime)

	if len(frames) != 3 {
		t.Errorf("Expected 3 frames in range, got %d", len(frames))
	}

	// Verify frame data
	expected := []byte{1, 2, 3}
	for i, frame := range frames {
		if frame.Data[0] != expected[i] {
			t.Errorf("Expected frame data[0] to be %d, got %d", expected[i], frame.Data[0])
		}
	}
}

func TestFrameBuffer_Clear(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  5 * time.Second,
		MaxFrames: 10,
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	// Add frames
	for i := 0; i < 5; i++ {
		frame := &video.Frame{
			CameraID:  "camera-1",
			Timestamp: time.Now(),
			Data:      []byte{byte(i)},
		}
		buffer.AddFrame(frame)
	}

	if buffer.Size() != 5 {
		t.Errorf("Expected 5 frames, got %d", buffer.Size())
	}

	// Clear buffer
	buffer.Clear()

	if buffer.Size() != 0 {
		t.Errorf("Expected 0 frames after clear, got %d", buffer.Size())
	}
}

func TestFrameBuffer_Close(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  5 * time.Second,
		MaxFrames: 10,
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	if buffer.IsClosed() {
		t.Error("Buffer should not be closed initially")
	}

	buffer.Close()

	if !buffer.IsClosed() {
		t.Error("Buffer should be closed after Close()")
	}
}

func TestFrameBuffer_GetStats(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameBufferConfig{
		Duration:  5 * time.Second,
		MaxFrames: 3,
	}

	buffer := NewFrameBuffer("camera-1", config, log)

	// Add more frames than max
	for i := 0; i < 5; i++ {
		frame := &video.Frame{
			CameraID:  "camera-1",
			Timestamp: time.Now(),
			Data:      []byte{byte(i)},
		}
		buffer.AddFrame(frame)
	}

	stats := buffer.GetStats()
	if stats.TotalFramesAdded != 5 {
		t.Errorf("Expected 5 total frames added, got %d", stats.TotalFramesAdded)
	}

	if stats.FramesDropped != 2 {
		t.Errorf("Expected 2 frames dropped, got %d", stats.FramesDropped)
	}
}

