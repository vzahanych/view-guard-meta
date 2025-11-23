package ai

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

func TestFrameProcessor_ProcessFrame(t *testing.T) {
	// Create mock client with proper httpClient
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	client := NewClient(ClientConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    5 * time.Second,
	}, log)

	detectionCount := 0
	onDetection := func(result *DetectionResult) {
		detectionCount++
	}

	processor := NewFrameProcessor(FrameProcessorConfig{
		Client:      client,
		Interval:    100 * time.Millisecond,
		OnDetection: onDetection,
	}, client.logger)

	frame := &video.Frame{
		Data:      []byte("test frame"),
		Timestamp: time.Now(),
		CameraID:  "camera-1",
		Width:     640,
		Height:    480,
	}

	ctx := context.Background()
	err := processor.ProcessFrame(ctx, frame)

	if err != nil {
		t.Fatalf("ProcessFrame failed: %v", err)
	}

	// Note: Actual inference will fail without a real server,
	// but the frame should be processed
}

func TestFrameProcessor_RateLimiting(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	client := NewClient(ClientConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    5 * time.Second,
	}, log)

	processor := NewFrameProcessor(FrameProcessorConfig{
		Client:   client,
		Interval: 200 * time.Millisecond,
	}, client.logger)

	frame := &video.Frame{
		Data:      []byte("test"),
		Timestamp: time.Now(),
		CameraID:  "camera-1",
		Width:     640,
		Height:    480,
	}

	ctx := context.Background()

	// First frame should be processed
	err := processor.ProcessFrame(ctx, frame)
	if err != nil {
		t.Fatalf("First frame processing failed: %v", err)
	}

	// Second frame immediately after should be skipped (rate limiting)
	err = processor.ProcessFrame(ctx, frame)
	if err != nil {
		t.Fatalf("Second frame processing failed: %v", err)
	}

	// Wait for interval
	time.Sleep(250 * time.Millisecond)

	// Third frame after interval should be processed
	err = processor.ProcessFrame(ctx, frame)
	if err != nil {
		t.Fatalf("Third frame processing failed: %v", err)
	}
}

func TestFrameProcessor_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	client := NewClient(ClientConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    5 * time.Second,
	}, log)

	processor := NewFrameProcessor(FrameProcessorConfig{
		Client: client,
	}, client.logger)

	// Start
	err := processor.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !processor.IsRunning() {
		t.Error("Processor should be running")
	}

	// Stop
	processor.Stop()

	if processor.IsRunning() {
		t.Error("Processor should not be running")
	}
}

