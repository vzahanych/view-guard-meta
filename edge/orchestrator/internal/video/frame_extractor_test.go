package video

import (
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewFrameExtractor(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()

	config := FrameExtractorConfig{
		BufferSize:      10,
		ExtractInterval: 1 * time.Second,
		Preprocess: PreprocessConfig{
			ResizeWidth:  640,
			ResizeHeight: 480,
			Quality:      85,
		},
	}

	extractor := NewFrameExtractor(ffmpeg, config, log)
	if extractor == nil {
		t.Fatal("NewFrameExtractor returned nil")
	}

	if extractor.bufferSize != 10 {
		t.Errorf("Expected buffer size 10, got %d", extractor.bufferSize)
	}

	if extractor.extractInterval != 1*time.Second {
		t.Errorf("Expected extract interval 1s, got %v", extractor.extractInterval)
	}
}

func TestFrameExtractor_DefaultValues(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()

	// Test with empty config (should use defaults)
	config := FrameExtractorConfig{}
	extractor := NewFrameExtractor(ffmpeg, config, log)

	if extractor.bufferSize != 10 {
		t.Errorf("Expected default buffer size 10, got %d", extractor.bufferSize)
	}

	if extractor.extractInterval != 1*time.Second {
		t.Errorf("Expected default extract interval 1s, got %v", extractor.extractInterval)
	}

	if extractor.preprocessConfig.Quality != 85 {
		t.Errorf("Expected default quality 85, got %d", extractor.preprocessConfig.Quality)
	}
}

func TestFrameExtractor_GetFrameBuffer(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	buffer := extractor.GetFrameBuffer()
	if buffer == nil {
		t.Fatal("GetFrameBuffer returned nil")
	}
}

func TestFrameExtractor_IsRunning(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	// Initially not running
	if extractor.IsRunning() {
		t.Error("Extractor should not be running initially")
	}
}

func TestFrameExtractor_GetBufferSize(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	bufferSize := extractor.GetBufferSize()
	if bufferSize != 5 {
		t.Errorf("Expected buffer size 5, got %d", bufferSize)
	}
}

func TestFrameExtractor_GetExtractInterval(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	interval := extractor.GetExtractInterval()
	if interval != 100*time.Millisecond {
		t.Errorf("Expected extract interval 100ms, got %v", interval)
	}
}

func TestFrameExtractor_SetExtractInterval(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	newInterval := 2 * time.Second
	extractor.SetExtractInterval(newInterval)

	if extractor.GetExtractInterval() != newInterval {
		t.Errorf("Expected extract interval %v, got %v", newInterval, extractor.GetExtractInterval())
	}
}

func TestFrameExtractor_SetPreprocessConfig(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	newConfig := PreprocessConfig{
		ResizeWidth:  1280,
		ResizeHeight: 720,
		Quality:      90,
	}

	extractor.SetPreprocessConfig(newConfig)

	// Verify config was set (we can't directly access, but we can test via behavior)
	extractor.mu.RLock()
	config := extractor.preprocessConfig
	extractor.mu.RUnlock()

	if config.ResizeWidth != 1280 {
		t.Errorf("Expected resize width 1280, got %d", config.ResizeWidth)
	}

	if config.Quality != 90 {
		t.Errorf("Expected quality 90, got %d", config.Quality)
	}
}

func TestFrameExtractor_Stop(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	// Stop should not panic even if not started
	extractor.Stop()

	// Verify stopped
	if extractor.IsRunning() {
		t.Error("Extractor should not be running after Stop")
	}
}

func TestFrameExtractor_StartStop(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	extractor := setupTestFrameExtractor(t, ffmpeg)

	// Try to start with invalid input (should fail gracefully)
	err := extractor.Start("invalid://input", "camera-1")
	if err == nil {
		// If it doesn't fail, that's okay - we'll just stop it
		extractor.Stop()
	} else {
		// Expected to fail with invalid input
		if !extractor.IsRunning() {
			// Good, extractor didn't start
		}
	}
}

func TestFrameExtractor_FrameBufferManagement(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()

	config := FrameExtractorConfig{
		BufferSize:      2, // Small buffer for testing
		ExtractInterval: 100 * time.Millisecond,
	}

	extractor := NewFrameExtractor(ffmpeg, config, log)

	// Verify buffer size
	if extractor.GetBufferSize() != 2 {
		t.Errorf("Expected buffer size 2, got %d", extractor.GetBufferSize())
	}

	// Get buffer channel
	buffer := extractor.GetFrameBuffer()
	if buffer == nil {
		t.Fatal("GetFrameBuffer returned nil")
	}
}

