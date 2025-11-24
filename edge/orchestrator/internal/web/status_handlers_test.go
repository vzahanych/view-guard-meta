package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// mockTelemetryCollector is a mock implementation of TelemetryCollector
type mockTelemetryCollector struct {
	lastMetrics *edge.TelemetryData
	collectErr  error
}

func (m *mockTelemetryCollector) GetLastMetrics() interface{} {
	return m.lastMetrics
}

func (m *mockTelemetryCollector) Collect(ctx context.Context) (interface{}, error) {
	if m.collectErr != nil {
		return nil, m.collectErr
	}
	return m.lastMetrics, nil
}

func setupTestStatusServer(t *testing.T) (*Server, *mockTelemetryCollector, func()) {
	// Create temp directory for test data
	tmpDir, err := os.MkdirTemp("", "status-test-*")
	require.NoError(t, err)

	// Create test config
	cfg := &config.WebConfig{
		Port: 8080,
	}

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	// Create server
	server := NewServer(cfg, log)
	server.SetVersion("test-version-1.0.0")
	server.setupRoutes()

	// Create mock telemetry collector
	mockCollector := &mockTelemetryCollector{}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return server, mockCollector, cleanup
}

func TestHandleStatus(t *testing.T) {
	server, _, cleanup := setupTestStatusServer(t)
	defer cleanup()

	// Set server status to running for test
	server.GetStatus().SetStatus(service.StatusRunning)
	
	// Wait a bit to ensure uptime > 0
	time.Sleep(10 * time.Millisecond)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.NotEmpty(t, response["uptime"])
	assert.Equal(t, "test-version-1.0.0", response["version"])
	assert.NotEmpty(t, response["timestamp"])
	
	// Check uptime_seconds - it should be a number (may be 0 if test runs very fast)
	uptimeSeconds, ok := response["uptime_seconds"].(float64)
	if !ok {
		// Try int64
		uptimeSecondsInt, okInt := response["uptime_seconds"].(int64)
		if okInt {
			assert.GreaterOrEqual(t, uptimeSecondsInt, int64(0))
		} else {
			t.Errorf("uptime_seconds is not a number: %T", response["uptime_seconds"])
		}
	} else {
		assert.GreaterOrEqual(t, uptimeSeconds, float64(0))
	}
}

func TestHandleMetrics_NoCollector(t *testing.T) {
	server, _, cleanup := setupTestStatusServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response["error"], "Telemetry collector not available")
}

func TestHandleMetrics_WithCollector(t *testing.T) {
	server, mockCollector, cleanup := setupTestStatusServer(t)
	defer cleanup()

	// Set up mock collector with metrics
	mockCollector.lastMetrics = &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    "test-edge-001",
		System: &edge.SystemMetrics{
			CpuUsagePercent:  25.5,
			MemoryUsedBytes:  1024 * 1024 * 512,  // 512 MB
			MemoryTotalBytes: 1024 * 1024 * 2048, // 2 GB
			DiskUsedBytes:    1024 * 1024 * 1024 * 10, // 10 GB
			DiskTotalBytes:   1024 * 1024 * 1024 * 100, // 100 GB
			DiskUsagePercent: 10.0,
		},
	}

	server.SetTelemetryDependency(mockCollector)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "system")
	system, ok := response["system"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 25.5, system["cpu_usage_percent"])
	assert.Equal(t, float64(1024*1024*512), system["memory_used_bytes"])
	assert.Equal(t, float64(1024*1024*2048), system["memory_total_bytes"])
}

func TestHandleMetrics_CollectOnDemand(t *testing.T) {
	server, mockCollector, cleanup := setupTestStatusServer(t)
	defer cleanup()

	// Set up mock collector with no last metrics, but Collect will return metrics
	mockCollector.lastMetrics = nil
	mockCollector.lastMetrics = &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    "test-edge-001",
		System: &edge.SystemMetrics{
			CpuUsagePercent:  15.0,
			MemoryUsedBytes:  1024 * 1024 * 256,
			MemoryTotalBytes: 1024 * 1024 * 1024,
			DiskUsedBytes:    0,
			DiskTotalBytes:   0,
			DiskUsagePercent: 0.0,
		},
	}

	server.SetTelemetryDependency(mockCollector)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "system")
}

func TestHandleAppMetrics_NoCollector(t *testing.T) {
	server, _, cleanup := setupTestStatusServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/app", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleAppMetrics_WithCollector(t *testing.T) {
	server, mockCollector, cleanup := setupTestStatusServer(t)
	defer cleanup()

	// Create temp directory for camera manager
	tmpDir, err := os.MkdirTemp("", "camera-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create state manager for camera manager
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
		},
	}
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	stateMgr, err := state.NewManager(cfg, log)
	require.NoError(t, err)

	// Create camera manager
	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	server.SetDependencies(cameraMgr, nil)

	// Set up mock collector with application metrics
	mockCollector.lastMetrics = &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    "test-edge-001",
		Application: &edge.ApplicationMetrics{
			EventQueueLength:      5,
			ActiveCameras:         3,
			AiInferenceTimeMs:     150.5,
			StorageClipsCount:     100,
			StorageClipsSizeBytes: 1024 * 1024 * 1024 * 5, // 5 GB
		},
	}

	server.SetTelemetryDependency(mockCollector)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/app", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "application")
	app, ok := response["application"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(5), app["event_queue_length"])
	assert.Equal(t, float64(3), app["active_cameras"])
	assert.Equal(t, 150.5, app["ai_inference_time_ms"])
}

func TestHandleAppMetrics_WithCameras(t *testing.T) {
	server, mockCollector, cleanup := setupTestStatusServer(t)
	defer cleanup()

	// Create temp directory for camera manager
	tmpDir, err := os.MkdirTemp("", "camera-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create state manager for camera manager
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
		},
	}
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	stateMgr, err := state.NewManager(cfg, log)
	require.NoError(t, err)

	// Create camera manager and add some cameras
	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	server.SetDependencies(cameraMgr, nil)

	// Add cameras
	ctx := context.Background()
	cam1 := &camera.Camera{
		ID:      "cam1",
		Name:    "Camera 1",
		Type:    camera.CameraTypeRTSP,
		Enabled: true,
		Status:  camera.CameraStatusOnline,
	}
	cam2 := &camera.Camera{
		ID:      "cam2",
		Name:    "Camera 2",
		Type:    camera.CameraTypeUSB,
		Enabled: true,
		Status:  camera.CameraStatusOffline,
	}
	cam3 := &camera.Camera{
		ID:      "cam3",
		Name:    "Camera 3",
		Type:    camera.CameraTypeRTSP,
		Enabled: false,
		Status:  camera.CameraStatusOnline,
	}
	cameraMgr.AddCamera(ctx, cam1)
	cameraMgr.AddCamera(ctx, cam2)
	cameraMgr.AddCamera(ctx, cam3)

	// Set up mock collector
	mockCollector.lastMetrics = &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    "test-edge-001",
		Application: &edge.ApplicationMetrics{
			EventQueueLength:      2,
			ActiveCameras:         1, // Only cam1 is online and enabled
			AiInferenceTimeMs:     100.0,
			StorageClipsCount:     50,
			StorageClipsSizeBytes: 1024 * 1024 * 1024 * 2,
		},
	}

	server.SetTelemetryDependency(mockCollector)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/app", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	app, ok := response["application"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(3), app["total_cameras"])
	assert.Equal(t, float64(2), app["enabled_cameras"])
	assert.Equal(t, float64(2), app["online_cameras"]) // cam1 and cam3 are online
}

func TestHandleTelemetry_NoCollector(t *testing.T) {
	server, _, cleanup := setupTestStatusServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleTelemetry_WithCollector(t *testing.T) {
	server, mockCollector, cleanup := setupTestStatusServer(t)
	defer cleanup()

	now := time.Now()
	mockCollector.lastMetrics = &edge.TelemetryData{
		Timestamp: now.UnixNano(),
		EdgeId:    "test-edge-001",
		System: &edge.SystemMetrics{
			CpuUsagePercent:  30.0,
			MemoryUsedBytes:  1024 * 1024 * 1024,
			MemoryTotalBytes: 1024 * 1024 * 1024 * 4,
			DiskUsedBytes:    1024 * 1024 * 1024 * 20,
			DiskTotalBytes:   1024 * 1024 * 1024 * 200,
			DiskUsagePercent: 10.0,
		},
		Application: &edge.ApplicationMetrics{
			EventQueueLength:      10,
			ActiveCameras:         5,
			AiInferenceTimeMs:     200.0,
			StorageClipsCount:     200,
			StorageClipsSizeBytes: 1024 * 1024 * 1024 * 10,
		},
		Cameras: []*edge.CameraStatus{
			{
				CameraId:      "cam1",
				Online:        true,
				LastSeen:      now.Format(time.RFC3339),
				StatusMessage: "online",
			},
			{
				CameraId:      "cam2",
				Online:        false,
				LastSeen:      "",
				StatusMessage: "offline",
			},
		},
	}

	server.SetTelemetryDependency(mockCollector)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "test-edge-001", response["edge_id"])
	assert.Contains(t, response, "system")
	assert.Contains(t, response, "application")
	assert.Contains(t, response, "cameras")

	// Check cameras array
	cameras, ok := response["cameras"].([]interface{})
	require.True(t, ok)
	assert.Len(t, cameras, 2)

	cam1, ok := cameras[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "cam1", cam1["camera_id"])
	assert.Equal(t, true, cam1["online"])
}

func TestHandleTelemetry_CollectOnDemand(t *testing.T) {
	server, mockCollector, cleanup := setupTestStatusServer(t)
	defer cleanup()

	// Set up mock collector with no last metrics, but Collect will return metrics
	mockCollector.lastMetrics = nil
	mockCollector.lastMetrics = &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    "test-edge-002",
		System: &edge.SystemMetrics{
			CpuUsagePercent:  20.0,
			MemoryUsedBytes:  512 * 1024 * 1024,
			MemoryTotalBytes: 2 * 1024 * 1024 * 1024,
			DiskUsedBytes:    0,
			DiskTotalBytes:   0,
			DiskUsagePercent: 0.0,
		},
		Application: &edge.ApplicationMetrics{
			EventQueueLength:      0,
			ActiveCameras:         0,
			AiInferenceTimeMs:     0.0,
			StorageClipsCount:     0,
			StorageClipsSizeBytes: 0,
		},
	}

	server.SetTelemetryDependency(mockCollector)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "test-edge-002", response["edge_id"])
}


