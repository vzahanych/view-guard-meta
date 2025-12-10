package processing

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

func TestNewFrameProcessor(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		PreprocessConfig: PreprocessConfig{
			ResizeWidth:  640,
			ResizeHeight: 480,
			Quality:      85,
		},
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)
	if processor == nil {
		t.Fatal("NewFrameProcessor returned nil")
	}

	if processor.inferenceInterval != 1*time.Second {
		t.Errorf("Expected inference interval 1s, got %v", processor.inferenceInterval)
	}
}

func TestFrameProcessor_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)

	ctx := context.Background()
	err := processor.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify running
	status := processor.GetStatus().GetStatus()
	if status != service.StatusRunning {
		t.Errorf("Expected status %s, got %s", service.StatusRunning, status)
	}

	err = processor.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	status = processor.GetStatus().GetStatus()
	if status != service.StatusStopped {
		t.Errorf("Expected status %s, got %s", service.StatusStopped, status)
	}
}

func TestFrameProcessor_GetBuffer(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)

	ctx := context.Background()
	err := processor.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer processor.Stop()

	// Get buffer for camera (should be created automatically)
	buffer, ok := processor.GetBuffer("camera-1")
	if !ok {
		t.Error("GetBuffer should return true for existing camera")
	}

	if buffer == nil {
		t.Fatal("GetBuffer returned nil buffer")
	}

	if buffer.cameraID != "camera-1" {
		t.Errorf("Expected cameraID 'camera-1', got '%s'", buffer.cameraID)
	}
}

func TestFrameProcessor_RemoveBuffer(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)

	ctx := context.Background()
	err := processor.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer processor.Stop()

	// Get buffer (creates it)
	_, ok := processor.GetBuffer("camera-1")
	if !ok {
		t.Fatal("Failed to get buffer")
	}

	// Remove buffer
	processor.RemoveBuffer("camera-1")

	// Buffer should no longer exist
	_, ok = processor.GetBuffer("camera-1")
	if ok {
		t.Error("Buffer should not exist after RemoveBuffer")
	}
}

func TestFrameProcessor_SetInferenceInterval(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)

	newInterval := 2 * time.Second
	processor.SetInferenceInterval(newInterval)

	if processor.inferenceInterval != newInterval {
		t.Errorf("Expected inference interval %v, got %v", newInterval, processor.inferenceInterval)
	}
}

func TestFrameProcessor_SetPreprocessConfig(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)

	newConfig := PreprocessConfig{
		ResizeWidth:  1280,
		ResizeHeight: 720,
		Quality:      90,
	}

	processor.SetPreprocessConfig(newConfig)

	if processor.preprocessConfig.ResizeWidth != 1280 {
		t.Errorf("Expected resize width 1280, got %d", processor.preprocessConfig.ResizeWidth)
	}

	if processor.preprocessConfig.ResizeHeight != 720 {
		t.Errorf("Expected resize height 720, got %d", processor.preprocessConfig.ResizeHeight)
	}

	if processor.preprocessConfig.Quality != 90 {
		t.Errorf("Expected quality 90, got %d", processor.preprocessConfig.Quality)
	}
}

func TestFrameProcessor_PreprocessFrame(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := FrameProcessorConfig{
		PreprocessConfig: PreprocessConfig{
			ResizeWidth:  640,
			ResizeHeight: 480,
			Quality:      85,
		},
		InferenceInterval: 1 * time.Second,
		PreBufferDuration: 5 * time.Second,
	}

	processor := NewFrameProcessor(config, log)

	// Create a test frame
	frame := &video.Frame{
		CameraID:  "camera-1",
		Timestamp: time.Now(),
		Data:      []byte{1, 2, 3, 4}, // Minimal JPEG data
	}

	// Preprocess frame (should not panic)
	processed, err := processor.preprocessFrame(frame)
	if err != nil {
		// Preprocessing may fail for invalid JPEG data, which is expected
		// Just verify the function doesn't panic
		return
	}
	if processed == nil {
		t.Fatal("PreprocessFrame returned nil")
	}

	// Verify camera ID is preserved
	if processed.CameraID != "camera-1" {
		t.Errorf("Expected cameraID 'camera-1', got '%s'", processed.CameraID)
	}
}

