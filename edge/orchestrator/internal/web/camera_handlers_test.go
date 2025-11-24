package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestCameraServer(t *testing.T) (*Server, *camera.Manager, *state.Manager, func()) {
	// Create temp directory for test data
	tmpDir, err := os.MkdirTemp("", "camera-test-*")
	require.NoError(t, err)

	// Create test config
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
		},
	}

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	// Create state manager
	stateMgr, err := state.NewManager(cfg, log)
	require.NoError(t, err)

	// Create discovery services (nil for now - we can add them if needed)
	var onvifDiscovery *camera.ONVIFDiscoveryService
	var usbDiscovery *camera.USBDiscoveryService

	// Create camera manager
	cameraMgr := camera.NewManager(
		stateMgr,
		onvifDiscovery,
		usbDiscovery,
		30*time.Second,
		log,
	)

	// Start camera manager
	ctx := context.Background()
	err = cameraMgr.Start(ctx)
	require.NoError(t, err)

	// Create web server
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	server := NewServer(webCfg, log)
	server.SetDependencies(cameraMgr, nil)
	server.setupRoutes()

	// Cleanup function
	cleanup := func() {
		cameraMgr.Stop(ctx)
		stateMgr.Close()
		os.RemoveAll(tmpDir)
	}

	return server, cameraMgr, stateMgr, cleanup
}

func TestHandleListCameras(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add test cameras
	cam1 := &camera.Camera{
		ID:      "camera-1",
		Name:    "Test Camera 1",
		Type:    camera.CameraTypeRTSP,
		Enabled: true,
		RTSPURLs: []string{"rtsp://test1"},
		Config: camera.CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  true,
		},
		DiscoveredAt: time.Now(),
	}
	err := cameraMgr.AddCamera(ctx, cam1)
	require.NoError(t, err)

	cam2 := &camera.Camera{
		ID:      "camera-2",
		Name:    "Test Camera 2",
		Type:    camera.CameraTypeUSB,
		Enabled: false,
		DevicePath: "/dev/video0",
		Config: camera.CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  true,
		},
		DiscoveredAt: time.Now(),
	}
	err = cameraMgr.AddCamera(ctx, cam2)
	require.NoError(t, err)

	// Test listing all cameras
	req := httptest.NewRequest("GET", "/api/cameras", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	cameras, ok := response["cameras"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(cameras))
	assert.Equal(t, float64(2), response["count"])

	// Test listing enabled cameras only
	req = httptest.NewRequest("GET", "/api/cameras?enabled=true", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	cameras, ok = response["cameras"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, len(cameras))
	assert.Equal(t, float64(1), response["count"])
}

func TestHandleListCameras_NoCameraManager(t *testing.T) {
	// Create server without camera manager
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	server := NewServer(webCfg, log)
	server.setupRoutes()

	req := httptest.NewRequest("GET", "/api/cameras", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleGetCamera(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add test camera
	cam := &camera.Camera{
		ID:      "camera-1",
		Name:    "Test Camera",
		Type:    camera.CameraTypeRTSP,
		Enabled: true,
		RTSPURLs: []string{"rtsp://test"},
		Manufacturer: "Test Manufacturer",
		Model: "Test Model",
		Config: camera.CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  true,
		},
		DiscoveredAt: time.Now(),
	}
	err := cameraMgr.AddCamera(ctx, cam)
	require.NoError(t, err)

	// Test getting camera
	req := httptest.NewRequest("GET", "/api/cameras/camera-1", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "camera-1", response["id"])
	assert.Equal(t, "Test Camera", response["name"])
	assert.Equal(t, "rtsp", response["type"])
	assert.Equal(t, true, response["enabled"])
}

func TestHandleGetCamera_NotFound(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/cameras/nonexistent", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleAddCamera(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	// Test adding RTSP camera
	reqBody := map[string]interface{}{
		"id":      "camera-rtsp",
		"name":    "RTSP Camera",
		"type":    "rtsp",
		"rtsp_urls": []string{"rtsp://192.168.1.100:554/stream"},
		"enabled": true,
		"config": map[string]interface{}{
			"recording_enabled": true,
			"motion_detection":  true,
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "camera-rtsp", response["id"])
	assert.Equal(t, "RTSP Camera", response["name"])

	// Verify camera was added
	cam, err := cameraMgr.GetCamera("camera-rtsp")
	require.NoError(t, err)
	assert.NotNil(t, cam)
}

func TestHandleAddCamera_USB(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	// Test adding USB camera
	reqBody := map[string]interface{}{
		"id":          "camera-usb",
		"name":        "USB Camera",
		"type":        "usb",
		"device_path": "/dev/video0",
		"enabled":     true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify camera was added
	cam, err := cameraMgr.GetCamera("camera-usb")
	require.NoError(t, err)
	assert.NotNil(t, cam)
	assert.Equal(t, camera.CameraTypeUSB, cam.Type)
	assert.Equal(t, "/dev/video0", cam.DevicePath)
}

func TestHandleAddCamera_InvalidRequest(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	// Test with missing required fields
	reqBody := map[string]interface{}{
		"name": "Test Camera",
		// Missing id and type
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAddCamera_MissingRTSPURLs(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	// Test RTSP camera without RTSP URLs
	reqBody := map[string]interface{}{
		"id":   "camera-rtsp",
		"name": "RTSP Camera",
		"type": "rtsp",
		// Missing rtsp_urls
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUpdateCamera(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add test camera
	cam := &camera.Camera{
		ID:      "camera-1",
		Name:    "Original Name",
		Type:    camera.CameraTypeRTSP,
		Enabled: true,
		RTSPURLs: []string{"rtsp://original"},
		Config: camera.CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  false,
			FrameRate:        15,
		},
		DiscoveredAt: time.Now(),
	}
	err := cameraMgr.AddCamera(ctx, cam)
	require.NoError(t, err)

	// Update camera
	reqBody := map[string]interface{}{
		"name": "Updated Name",
		"rtsp_urls": []string{"rtsp://updated"},
		"enabled": false,
		"config": map[string]interface{}{
			"motion_detection": true,
			"frame_rate":       30,
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/cameras/camera-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify camera was updated
	updated, err := cameraMgr.GetCamera("camera-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, false, updated.Enabled)
	assert.Equal(t, []string{"rtsp://updated"}, updated.RTSPURLs)
	assert.Equal(t, true, updated.Config.MotionDetection)
	assert.Equal(t, 30, updated.Config.FrameRate)
}

func TestHandleUpdateCamera_NotFound(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	reqBody := map[string]interface{}{
		"name": "Updated Name",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/cameras/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleDeleteCamera(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add test camera
	cam := &camera.Camera{
		ID:      "camera-1",
		Name:    "Test Camera",
		Type:    camera.CameraTypeRTSP,
		Enabled: true,
		RTSPURLs: []string{"rtsp://test"},
		Config: camera.CameraConfig{
			RecordingEnabled: true,
		},
		DiscoveredAt: time.Now(),
	}
	err := cameraMgr.AddCamera(ctx, cam)
	require.NoError(t, err)

	// Delete camera
	req := httptest.NewRequest("DELETE", "/api/cameras/camera-1", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify camera was deleted
	_, err = cameraMgr.GetCamera("camera-1")
	assert.Error(t, err)
}

func TestHandleDeleteCamera_NotFound(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	req := httptest.NewRequest("DELETE", "/api/cameras/nonexistent", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleDiscoverCameras(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/cameras/discover", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Should return empty list if no discovery services are configured
	discovered, ok := response["discovered"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, len(discovered))
}

func TestHandleTestCamera(t *testing.T) {
	server, cameraMgr, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	ctx := context.Background()

	// Add test camera
	cam := &camera.Camera{
		ID:      "camera-1",
		Name:    "Test Camera",
		Type:    camera.CameraTypeRTSP,
		Enabled: true,
		RTSPURLs: []string{"rtsp://test"},
		Status:  camera.CameraStatusOffline,
		Config: camera.CameraConfig{
			RecordingEnabled: true,
		},
		DiscoveredAt: time.Now(),
	}
	err := cameraMgr.AddCamera(ctx, cam)
	require.NoError(t, err)

	// Test camera connection
	req := httptest.NewRequest("POST", "/api/cameras/camera-1/test", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "camera-1", response["camera_id"])
	assert.Contains(t, response, "success")
	assert.Contains(t, response, "message")
}

func TestHandleTestCamera_NotFound(t *testing.T) {
	server, _, _, cleanup := setupTestCameraServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/cameras/nonexistent/test", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

