package camera

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func TestManager_RegisterCamera_RTSP(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	discovered := &DiscoveredCamera{
		ID:            "cam-rtsp-1",
		Manufacturer:  "Test Manufacturer",
		Model:         "Test Model",
		IPAddress:     "192.168.1.100",
		RTSPURLs:      []string{"rtsp://192.168.1.100:554/stream"},
		Capabilities:  CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt:  time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	camera, err := mgr.GetCamera("cam-rtsp-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if camera == nil {
		t.Fatal("Camera should be registered")
	}

	if camera.Type != CameraTypeRTSP {
		t.Errorf("Expected type %s, got %s", CameraTypeRTSP, camera.Type)
	}

	if camera.Manufacturer != "Test Manufacturer" {
		t.Errorf("Expected manufacturer 'Test Manufacturer', got '%s'", camera.Manufacturer)
	}

	if len(camera.RTSPURLs) != 1 {
		t.Errorf("Expected 1 RTSP URL, got %d", len(camera.RTSPURLs))
	}
}

func TestManager_RegisterCamera_ONVIF(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	discovered := &DiscoveredCamera{
		ID:            "cam-onvif-1",
		Manufacturer:  "ONVIF Manufacturer",
		Model:         "ONVIF Model",
		IPAddress:     "192.168.1.101",
		ONVIFEndpoint: "http://192.168.1.101/onvif/device_service",
		RTSPURLs:      []string{"rtsp://192.168.1.101:554/stream"},
		Capabilities:  CameraCapabilities{HasVideoStreams: true, HasPTZ: true},
		DiscoveredAt:  time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	camera, err := mgr.GetCamera("cam-onvif-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if camera.Type != CameraTypeONVIF {
		t.Errorf("Expected type %s, got %s", CameraTypeONVIF, camera.Type)
	}

	if camera.ONVIFEndpoint != "http://192.168.1.101/onvif/device_service" {
		t.Errorf("Expected ONVIF endpoint, got '%s'", camera.ONVIFEndpoint)
	}
}

func TestManager_RegisterCamera_USB(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	discovered := &DiscoveredCamera{
		ID:           "cam-usb-1",
		Manufacturer: "USB Manufacturer",
		Model:        "USB Model",
		IPAddress:    "/dev/video0", // Using IPAddress field for device path in discovery
		RTSPURLs:     []string{"/dev/video0"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	camera, err := mgr.GetCamera("cam-usb-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if camera.Type != CameraTypeUSB {
		t.Errorf("Expected type %s, got %s", CameraTypeUSB, camera.Type)
	}

	if camera.DevicePath != "/dev/video0" {
		t.Errorf("Expected device path '/dev/video0', got '%s'", camera.DevicePath)
	}
}

func TestManager_GetCamera_NotFound(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	_, err := mgr.GetCamera("nonexistent")
	if err == nil {
		t.Error("GetCamera should return error for nonexistent camera")
	}
}

func TestManager_ListCameras(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	// Register multiple cameras
	cameras := []*DiscoveredCamera{
		{
			ID:           "cam-1",
			Manufacturer: "Manufacturer 1",
			Model:        "Model 1",
			RTSPURLs:     []string{"rtsp://192.168.1.100/stream"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			DiscoveredAt: time.Now(),
		},
		{
			ID:           "cam-2",
			Manufacturer: "Manufacturer 2",
			Model:        "Model 2",
			RTSPURLs:     []string{"rtsp://192.168.1.101/stream"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			DiscoveredAt: time.Now(),
		},
		{
			ID:           "cam-3",
			Manufacturer: "Manufacturer 3",
			Model:        "Model 3",
			RTSPURLs:     []string{"/dev/video0"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			DiscoveredAt: time.Now(),
		},
	}

	for _, discovered := range cameras {
		err := mgr.RegisterCamera(ctx, discovered)
		if err != nil {
			t.Fatalf("RegisterCamera failed: %v", err)
		}
	}

	// List all cameras
	allCameras := mgr.ListCameras(false)
	if len(allCameras) != 3 {
		t.Errorf("Expected 3 cameras, got %d", len(allCameras))
	}

	// Disable one camera
	err := mgr.DisableCamera(ctx, "cam-2")
	if err != nil {
		t.Fatalf("DisableCamera failed: %v", err)
	}

	// List only enabled cameras
	enabledCameras := mgr.ListCameras(true)
	if len(enabledCameras) != 2 {
		t.Errorf("Expected 2 enabled cameras, got %d", len(enabledCameras))
	}
}

func TestManager_UpdateCameraConfig(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	// Register camera
	discovered := &DiscoveredCamera{
		ID:           "cam-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	// Update config
	newConfig := CameraConfig{
		RecordingEnabled: false,
		MotionDetection:   false,
		Quality:          "high",
		FrameRate:        30,
		Resolution:       "1920x1080",
	}

	err = mgr.UpdateCameraConfig(ctx, "cam-1", newConfig)
	if err != nil {
		t.Fatalf("UpdateCameraConfig failed: %v", err)
	}

	// Verify config
	camera, err := mgr.GetCamera("cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	if camera.Config.Quality != "high" {
		t.Errorf("Expected quality 'high', got '%s'", camera.Config.Quality)
	}

	if camera.Config.FrameRate != 30 {
		t.Errorf("Expected frame rate 30, got %d", camera.Config.FrameRate)
	}

	if camera.Config.RecordingEnabled {
		t.Error("Expected RecordingEnabled=false")
	}
}

func TestManager_EnableDisableCamera(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	// Register camera
	discovered := &DiscoveredCamera{
		ID:           "cam-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	// Verify enabled by default
	camera, err := mgr.GetCamera("cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if !camera.Enabled {
		t.Error("Camera should be enabled by default")
	}

	// Disable camera
	err = mgr.DisableCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("DisableCamera failed: %v", err)
	}

	camera, err = mgr.GetCamera("cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if camera.Enabled {
		t.Error("Camera should be disabled")
	}

	// Enable camera
	err = mgr.EnableCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("EnableCamera failed: %v", err)
	}

	camera, err = mgr.GetCamera("cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if !camera.Enabled {
		t.Error("Camera should be enabled")
	}
}

func TestManager_DeleteCamera(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	// Register camera
	discovered := &DiscoveredCamera{
		ID:           "cam-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	// Verify it exists
	_, err = mgr.GetCamera("cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}

	// Delete camera
	err = mgr.DeleteCamera(ctx, "cam-1")
	if err != nil {
		t.Fatalf("DeleteCamera failed: %v", err)
	}

	// Verify it's gone
	_, err = mgr.GetCamera("cam-1")
	if err == nil {
		t.Error("GetCamera should fail for deleted camera")
	}
}

func TestManager_GetCameraStatus(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	// Register camera
	discovered := &DiscoveredCamera{
		ID:           "cam-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	status, err := mgr.GetCameraStatus("cam-1")
	if err != nil {
		t.Fatalf("GetCameraStatus failed: %v", err)
	}

	if status == "" {
		t.Error("Status should not be empty")
	}

	// Test with nonexistent camera
	_, err = mgr.GetCameraStatus("nonexistent")
	if err == nil {
		t.Error("GetCameraStatus should return error for nonexistent camera")
	}
}

func TestManager_RecoverCameras(t *testing.T) {
	mgr, stateMgr := setupTestManager(t)
	defer stateMgr.Close()

	ctx := context.Background()

	// Save cameras directly to state
	cameras := []state.CameraState{
		{
			ID:      "cam-1",
			Name:    "Camera 1",
			RTSPURL: "rtsp://192.168.1.100/stream",
			Enabled: true,
		},
		{
			ID:      "cam-2",
			Name:    "Camera 2",
			RTSPURL: "/dev/video0",
			Enabled: true,
		},
	}

	for _, camState := range cameras {
		err := stateMgr.SaveCamera(ctx, camState)
		if err != nil {
			t.Fatalf("SaveCamera failed: %v", err)
		}
	}

	// Recover cameras
	err := mgr.recoverCameras(ctx)
	if err != nil {
		t.Fatalf("recoverCameras failed: %v", err)
	}

	// Verify cameras were recovered
	recovered := mgr.ListCameras(false)
	if len(recovered) != 2 {
		t.Errorf("Expected 2 recovered cameras, got %d", len(recovered))
	}

	// Verify camera types
	cam1, err := mgr.GetCamera("cam-1")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if cam1.Type != CameraTypeRTSP {
		t.Errorf("Expected CameraTypeRTSP, got %s", cam1.Type)
	}

	cam2, err := mgr.GetCamera("cam-2")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if cam2.Type != CameraTypeUSB {
		t.Errorf("Expected CameraTypeUSB, got %s", cam2.Type)
	}
}

func TestManager_UnifiedCameraInterface(t *testing.T) {
	mgr, _ := setupTestManager(t)
	defer mgr.stateMgr.Close()

	ctx := context.Background()

	// Register network camera
	networkCam := &DiscoveredCamera{
		ID:           "network-cam",
		Manufacturer: "Network",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://192.168.1.100/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	// Register USB camera
	usbCam := &DiscoveredCamera{
		ID:           "usb-cam",
		Manufacturer: "USB",
		Model:        "Model",
		RTSPURLs:     []string{"/dev/video0"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, networkCam)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	err = mgr.RegisterCamera(ctx, usbCam)
	if err != nil {
		t.Fatalf("RegisterCamera failed: %v", err)
	}

	// List all cameras (unified interface)
	allCameras := mgr.ListCameras(false)
	if len(allCameras) != 2 {
		t.Errorf("Expected 2 cameras, got %d", len(allCameras))
	}

	// Verify both can be accessed through same interface
	network, err := mgr.GetCamera("network-cam")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if network.Type != CameraTypeRTSP {
		t.Errorf("Expected network camera type RTSP, got %s", network.Type)
	}

	usb, err := mgr.GetCamera("usb-cam")
	if err != nil {
		t.Fatalf("GetCamera failed: %v", err)
	}
	if usb.Type != CameraTypeUSB {
		t.Errorf("Expected USB camera type USB, got %s", usb.Type)
	}

	// Both should have same base fields
	if network.ID == "" || usb.ID == "" {
		t.Error("Both cameras should have ID")
	}
	if network.Name == "" || usb.Name == "" {
		t.Error("Both cameras should have Name")
	}
	if network.Status == "" || usb.Status == "" {
		t.Error("Both cameras should have Status")
	}
}

