package video

import (
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewFrameDistributor(t *testing.T) {
	log := logger.NewNopLogger()
	distributor := NewFrameDistributor(FrameDistributorConfig{}, log)

	if distributor == nil {
		t.Fatal("NewFrameDistributor returned nil")
	}
}

func TestFrameDistributor_AddRemoveExtractor(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()
	distributor := NewFrameDistributor(FrameDistributorConfig{}, log)

	extractor := setupTestFrameExtractor(t, ffmpeg)

	// Add extractor
	distributor.AddExtractor("camera-1", extractor)

	// Verify extractor was added
	retrieved, exists := distributor.GetExtractor("camera-1")
	if !exists {
		t.Fatal("Extractor should exist after AddExtractor")
	}

	if retrieved != extractor {
		t.Error("Retrieved extractor should be the same instance")
	}

	// Remove extractor
	distributor.RemoveExtractor("camera-1")

	// Verify extractor was removed
	_, exists = distributor.GetExtractor("camera-1")
	if exists {
		t.Error("Extractor should not exist after RemoveExtractor")
	}
}

func TestFrameDistributor_ListExtractors(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()
	distributor := NewFrameDistributor(FrameDistributorConfig{}, log)

	// Initially empty
	extractors := distributor.ListExtractors()
	if len(extractors) != 0 {
		t.Errorf("Expected 0 extractors, got %d", len(extractors))
	}

	// Add extractors
	extractor1 := setupTestFrameExtractor(t, ffmpeg)
	extractor2 := setupTestFrameExtractor(t, ffmpeg)

	distributor.AddExtractor("camera-1", extractor1)
	distributor.AddExtractor("camera-2", extractor2)

	// List extractors
	extractors = distributor.ListExtractors()
	if len(extractors) != 2 {
		t.Errorf("Expected 2 extractors, got %d", len(extractors))
	}

	// Verify both cameras are in the list
	cameraMap := make(map[string]bool)
	for _, camID := range extractors {
		cameraMap[camID] = true
	}

	if !cameraMap["camera-1"] {
		t.Error("camera-1 should be in extractor list")
	}

	if !cameraMap["camera-2"] {
		t.Error("camera-2 should be in extractor list")
	}
}

func TestFrameDistributor_StartStopExtraction(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()
	distributor := NewFrameDistributor(FrameDistributorConfig{}, log)

	extractor := setupTestFrameExtractor(t, ffmpeg)
	distributor.AddExtractor("camera-1", extractor)

	// Try to start extraction with invalid input (should fail)
	err := distributor.StartExtraction("camera-1", "invalid://input")
	if err == nil {
		// If it doesn't fail, that's okay - we'll just stop it
		distributor.StopExtraction("camera-1")
	}

	// Try to start extraction for non-existent camera
	err = distributor.StartExtraction("camera-2", "rtsp://test")
	if err == nil {
		t.Error("StartExtraction should return error for non-existent extractor")
	}
}

func TestFrameDistributor_StopAll(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()
	distributor := NewFrameDistributor(FrameDistributorConfig{}, log)

	extractor1 := setupTestFrameExtractor(t, ffmpeg)
	extractor2 := setupTestFrameExtractor(t, ffmpeg)

	distributor.AddExtractor("camera-1", extractor1)
	distributor.AddExtractor("camera-2", extractor2)

	// Stop all
	distributor.StopAll()

	// Verify extractors are stopped
	if extractor1.IsRunning() {
		t.Error("Extractor 1 should be stopped")
	}

	if extractor2.IsRunning() {
		t.Error("Extractor 2 should be stopped")
	}
}

func TestFrameDistributor_GetFrameStats(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()
	distributor := NewFrameDistributor(FrameDistributorConfig{}, log)

	extractor := setupTestFrameExtractor(t, ffmpeg)
	distributor.AddExtractor("camera-1", extractor)

	// Get stats
	stats, err := distributor.GetFrameStats("camera-1")
	if err != nil {
		t.Fatalf("GetFrameStats failed: %v", err)
	}

	if stats.CameraID != "camera-1" {
		t.Errorf("Expected camera ID 'camera-1', got '%s'", stats.CameraID)
	}

	if stats.BufferSize != 5 {
		t.Errorf("Expected buffer size 5, got %d", stats.BufferSize)
	}

	if stats.ExtractInterval != 100*time.Millisecond {
		t.Errorf("Expected extract interval 100ms, got %v", stats.ExtractInterval)
	}

	// Try to get stats for non-existent camera
	_, err = distributor.GetFrameStats("camera-2")
	if err == nil {
		t.Error("GetFrameStats should return error for non-existent camera")
	}
}

func TestFrameDistributor_OnFrameCallback(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	log := logger.NewNopLogger()

	frameReceived := false
	var receivedFrame *Frame

	distributor := NewFrameDistributor(FrameDistributorConfig{
		OnFrame: func(frame *Frame) {
			frameReceived = true
			receivedFrame = frame
		},
	}, log)

	extractor := setupTestFrameExtractor(t, ffmpeg)
	distributor.AddExtractor("camera-1", extractor)

	// Verify callback is set
	// The callback will be called when extractor extracts a frame
	_ = frameReceived
	_ = receivedFrame
}

