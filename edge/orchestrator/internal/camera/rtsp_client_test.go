package camera

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

func TestNewRTSPClient(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	eventBus := service.NewEventBus(100)

	config := RTSPClientConfig{
		URL:               "rtsp://test:554/stream",
		Username:          "user",
		Password:          "pass",
		Timeout:           10 * time.Second,
		ReconnectInterval: 5 * time.Second,
	}

	client := NewRTSPClient(config, log)
	client.SetEventBus(eventBus)

	if client == nil {
		t.Fatal("NewRTSPClient returned nil")
	}

	if client.url != "rtsp://test:554/stream" {
		t.Errorf("Expected URL 'rtsp://test:554/stream', got '%s'", client.url)
	}

	if client.username != "user" {
		t.Errorf("Expected username 'user', got '%s'", client.username)
	}

	if client.password != "pass" {
		t.Errorf("Expected password 'pass', got '%s'", client.password)
	}
}

func TestRTSPClient_IsConnected(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := RTSPClientConfig{
		URL: "rtsp://test:554/stream",
	}

	client := NewRTSPClient(config, log)

	// Initially not connected
	if client.IsConnected() {
		t.Error("Client should not be connected initially")
	}

	// Manually set connected state for testing
	client.mu.Lock()
	client.connected = true
	client.mu.Unlock()

	if !client.IsConnected() {
		t.Error("Client should be connected after setting state")
	}
}

func TestRTSPClient_GetHealthStatus(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := RTSPClientConfig{
		URL: "rtsp://test:554/stream",
	}

	client := NewRTSPClient(config, log)

	status := client.GetHealthStatus()
	if status != "disconnected" {
		t.Errorf("Expected initial status 'disconnected', got '%s'", status)
	}

	// Set health status
	client.mu.Lock()
	client.healthStatus = "connected"
	client.mu.Unlock()

	status = client.GetHealthStatus()
	if status != "connected" {
		t.Errorf("Expected status 'connected', got '%s'", status)
	}
}

func TestRTSPClient_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := RTSPClientConfig{
		URL: "rtsp://test:554/stream",
	}

	client := NewRTSPClient(config, log)

	ctx := context.Background()

	// Start client (will fail to connect but should not error on Start)
	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("Start should not fail even if connection fails: %v", err)
	}

	// Give it a moment to attempt connection
	time.Sleep(100 * time.Millisecond)

	// Stop client
	err = client.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	if client.GetStatus().GetStatus() != service.StatusStopped {
		t.Errorf("Expected status %s, got %s", service.StatusStopped, client.GetStatus().GetStatus())
	}
}

func TestRTSPClient_Reconnection(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	eventBus := service.NewEventBus(100)

	config := RTSPClientConfig{
		URL:               "rtsp://invalid:554/stream",
		ReconnectInterval: 100 * time.Millisecond,
	}

	client := NewRTSPClient(config, log)
	client.SetEventBus(eventBus)

	ctx := context.Background()

	// Start client - will attempt to connect and fail
	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for reconnection attempt
	time.Sleep(200 * time.Millisecond)

	// Verify it's attempting to reconnect (not connected but running)
	status := client.GetStatus().GetStatus()
	if status != service.StatusRunning && status != service.StatusStarting {
		t.Errorf("Expected status Running or Starting, got %s", status)
	}

	// Stop client
	client.Stop(ctx)
}

func TestRTSPClient_OnFrameCallback(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	frameReceived := false
	var receivedFrame []byte
	var receivedTime time.Time

	config := RTSPClientConfig{
		URL: "rtsp://test:554/stream",
		OnFrameCallback: func(frame []byte, timestamp time.Time) {
			frameReceived = true
			receivedFrame = frame
			receivedTime = timestamp
		},
	}

	client := NewRTSPClient(config, log)

	if client.onFrame == nil {
		t.Error("OnFrame callback should be set")
	}

	// Simulate frame callback
	testFrame := []byte{0x00, 0x00, 0x00, 0x01, 0x67} // H.264 NAL unit
	testTime := time.Now()
	client.onFrame(testFrame, testTime)

	if !frameReceived {
		t.Error("Frame callback should have been called")
	}

	if len(receivedFrame) != len(testFrame) {
		t.Errorf("Expected frame length %d, got %d", len(testFrame), len(receivedFrame))
	}

	if receivedTime.IsZero() {
		t.Error("Received time should be set")
	}
}

func TestRTSPClient_GetLastFrameTime(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	config := RTSPClientConfig{
		URL: "rtsp://test:554/stream",
	}

	client := NewRTSPClient(config, log)

	// Initially zero
	lastFrame := client.GetLastFrameTime()
	if !lastFrame.IsZero() {
		t.Error("Last frame time should be zero initially")
	}

	// Set last frame time
	testTime := time.Now()
	client.mu.Lock()
	client.lastFrame = testTime
	client.mu.Unlock()

	lastFrame = client.GetLastFrameTime()
	if lastFrame.IsZero() {
		t.Error("Last frame time should be set")
	}

	if lastFrame.Before(testTime.Add(-time.Millisecond)) || lastFrame.After(testTime.Add(time.Millisecond)) {
		t.Error("Last frame time should match set time")
	}
}

