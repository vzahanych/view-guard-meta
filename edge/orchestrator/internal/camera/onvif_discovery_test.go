package camera

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	svc "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

func TestNewONVIFDiscoveryService(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	service := NewONVIFDiscoveryService(30*time.Second, log)
	if service == nil {
		t.Fatal("NewONVIFDiscoveryService returned nil")
	}

	if service.discoveryInterval != 30*time.Second {
		t.Errorf("Expected discovery interval 30s, got %v", service.discoveryInterval)
	}
}

func TestONVIFDiscoveryService_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	service := NewONVIFDiscoveryService(30*time.Second, log)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment
	time.Sleep(50 * time.Millisecond)

	// Verify running
	status := service.GetStatus().GetStatus()
	expectedRunning := svc.StatusRunning
	if status != expectedRunning {
		t.Errorf("Expected status %s, got %s", expectedRunning, status)
	}

	err = service.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	status = service.GetStatus().GetStatus()
	expectedStopped := svc.StatusStopped
	if status != expectedStopped {
		t.Errorf("Expected status %s, got %s", expectedStopped, status)
	}
}

func TestONVIFDiscoveryService_GetCameraByID(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	service := NewONVIFDiscoveryService(30*time.Second, log)

	// Initially no cameras
	camera := service.GetCameraByID("test-id")
	if camera != nil {
		t.Error("GetCameraByID should return nil for nonexistent camera")
	}

	// Add a discovered camera
	discovered := &DiscoveredCamera{
		ID:            "test-id",
		Manufacturer:  "Test",
		Model:         "Model",
		IPAddress:     "192.168.1.100",
		ONVIFEndpoint: "http://192.168.1.100/onvif",
		RTSPURLs:      []string{"rtsp://192.168.1.100/stream"},
		Capabilities:  CameraCapabilities{HasVideoStreams: true},
		LastSeen:      time.Now(),
		DiscoveredAt:  time.Now(),
	}

	service.mu.Lock()
	service.discoveredCameras["test-id"] = discovered
	service.mu.Unlock()

	// Retrieve camera
	retrieved := service.GetCameraByID("test-id")
	if retrieved == nil {
		t.Fatal("GetCameraByID should return camera")
	}

	if retrieved.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", retrieved.ID)
	}

	if retrieved.Manufacturer != "Test" {
		t.Errorf("Expected manufacturer 'Test', got '%s'", retrieved.Manufacturer)
	}
}

func TestONVIFDiscoveryService_ListDiscoveredCameras(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	service := NewONVIFDiscoveryService(30*time.Second, log)

	// Add multiple cameras
	cameras := []*DiscoveredCamera{
		{
			ID:           "cam-1",
			Manufacturer: "Manufacturer 1",
			Model:        "Model 1",
			IPAddress:    "192.168.1.100",
			RTSPURLs:     []string{"rtsp://192.168.1.100/stream"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			LastSeen:     time.Now(),
			DiscoveredAt: time.Now(),
		},
		{
			ID:           "cam-2",
			Manufacturer: "Manufacturer 2",
			Model:        "Model 2",
			IPAddress:    "192.168.1.101",
			RTSPURLs:     []string{"rtsp://192.168.1.101/stream"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			LastSeen:     time.Now(),
			DiscoveredAt: time.Now(),
		},
	}

	service.mu.Lock()
	for _, cam := range cameras {
		service.discoveredCameras[cam.ID] = cam
	}
	service.mu.Unlock()

	// List cameras
	list := service.GetDiscoveredCameras()
	if len(list) != 2 {
		t.Errorf("Expected 2 cameras, got %d", len(list))
	}
}

func TestONVIFDiscoveryService_EventPublishing(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	eventBus := svc.NewEventBus(100)

	service := NewONVIFDiscoveryService(30*time.Second, log)
	service.SetEventBus(eventBus)

	// Subscribe to discovery events
	eventType := svc.EventTypeCameraDiscovered
	ch := eventBus.Subscribe(eventType)

	// Manually add a camera (simulating discovery)
	discovered := &DiscoveredCamera{
		ID:           "test-cam",
		Manufacturer: "Test",
		Model:        "Model",
		IPAddress:    "192.168.1.100",
		RTSPURLs:     []string{"rtsp://192.168.1.100/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	service.mu.Lock()
	service.discoveredCameras[discovered.ID] = discovered
	service.mu.Unlock()

	// Publish event manually (normally done in discoverCameras)
	service.PublishEvent(eventType, map[string]interface{}{
		"camera_id":    discovered.ID,
		"manufacturer": discovered.Manufacturer,
		"model":        discovered.Model,
		"ip_address":   discovered.IPAddress,
	})

	// Wait for event
	select {
	case event := <-ch:
		if event.Type != eventType {
			t.Errorf("Expected event type %s, got %s", eventType, event.Type)
		}
		if event.Data["camera_id"] != "test-cam" {
			t.Errorf("Expected camera_id 'test-cam', got %v", event.Data["camera_id"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received within timeout")
	}
}
