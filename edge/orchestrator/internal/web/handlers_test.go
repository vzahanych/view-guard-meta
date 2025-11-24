package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// mockStorageService implements StorageService for testing
type mockStorageService struct {
	clipsDir     string
	snapshotsDir string
}

func (m *mockStorageService) GetClipsDir() string {
	return m.clipsDir
}

func (m *mockStorageService) GetSnapshotsDir() string {
	return m.snapshotsDir
}

func setupTestEventServer(t *testing.T) (*Server, *state.Manager, *mockStorageService, func()) {
	// Create temp directory for test data
	tmpDir, err := os.MkdirTemp("", "web-test-*")
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

	// Create storage service
	storageSvc := &mockStorageService{
		clipsDir:     filepath.Join(tmpDir, "clips"),
		snapshotsDir: filepath.Join(tmpDir, "snapshots"),
	}

	// Create web server
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	server := NewServer(webCfg, log)
	server.SetEventDependencies(stateMgr, storageSvc)
	// Setup routes manually for testing
	server.setupRoutes()

	// Cleanup function
	cleanup := func() {
		stateMgr.Close()
		os.RemoveAll(tmpDir)
	}

	return server, stateMgr, storageSvc, cleanup
}

func TestHandleListEvents(t *testing.T) {
	server, stateMgr, _, cleanup := setupTestEventServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create test cameras (required for foreign key constraint)
	camera1 := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera 1",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	camera2 := state.CameraState{
		ID:      "camera-2",
		Name:    "Test Camera 2",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera1)
	require.NoError(t, err)
	err = stateMgr.SaveCamera(ctx, camera2)
	require.NoError(t, err)

	// Create test events
	event1 := events.NewEvent()
	event1.CameraID = "camera-1"
	event1.EventType = events.EventTypePersonDetected
	event1.Timestamp = time.Now().Add(-2 * time.Hour)
	event1.Confidence = 0.95
	event1.ClipPath = "clips/event1.mp4"
	event1.SnapshotPath = "snapshots/event1.jpg"

	event2 := events.NewEvent()
	event2.CameraID = "camera-2"
	event2.EventType = events.EventTypeVehicleDetected
	event2.Timestamp = time.Now().Add(-1 * time.Hour)
	event2.Confidence = 0.87
	event2.ClipPath = "clips/event2.mp4"
	event2.SnapshotPath = "snapshots/event2.jpg"

	// Save events
	err = stateMgr.SaveEvent(ctx, event1.ToEventState())
	require.NoError(t, err)
	err = stateMgr.SaveEvent(ctx, event2.ToEventState())
	require.NoError(t, err)

	// Test: List all events
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/events", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, int(response["count"].(float64)))
	assert.Equal(t, 2, int(response["total"].(float64)))
	eventsList := response["events"].([]interface{})
	assert.Len(t, eventsList, 2)

	// Test: Filter by camera
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/events?camera_id=camera-1", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, int(response["count"].(float64)))

	// Test: Filter by event type
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/events?event_type=person_detected", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, int(response["count"].(float64)))

	// Test: Pagination
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/events?limit=1&offset=0", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, int(response["count"].(float64)))
	assert.Equal(t, 2, int(response["total"].(float64)))
}

func TestHandleGetEvent(t *testing.T) {
	server, stateMgr, _, cleanup := setupTestEventServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create test camera (required for foreign key constraint)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera)
	require.NoError(t, err)

	// Create test event
	event := events.NewEvent()
	event.CameraID = "camera-1"
	event.EventType = events.EventTypePersonDetected
	event.Timestamp = time.Now()
	event.Confidence = 0.95
	event.ClipPath = "clips/event1.mp4"
	event.SnapshotPath = "snapshots/event1.jpg"

	// Save event
	err = stateMgr.SaveEvent(ctx, event.ToEventState())
	require.NoError(t, err)

	// Test: Get existing event
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/events/"+event.ID, nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, event.ID, response["id"])
	assert.Equal(t, event.CameraID, response["camera_id"])
	assert.Equal(t, event.EventType, response["event_type"])
	assert.Equal(t, event.Confidence, response["confidence"])

	// Test: Get non-existent event
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/events/non-existent", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleListEvents_NoStateManager(t *testing.T) {
	// Create server without state manager
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)
	server := NewServer(webCfg, log)
	// Setup routes manually since Start() isn't called
	server.setupRoutes()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/events", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleGetEvent_NoStateManager(t *testing.T) {
	// Create server without state manager
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)
	server := NewServer(webCfg, log)
	// Setup routes manually since Start() isn't called
	server.setupRoutes()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/events/test-id", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandlePlayClip(t *testing.T) {
	server, stateMgr, storageSvc, cleanup := setupTestEventServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create test camera (required for foreign key constraint)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera)
	require.NoError(t, err)

	// Create test clip file
	clipDir := storageSvc.GetClipsDir()
	err = os.MkdirAll(clipDir, 0755)
	require.NoError(t, err)

	clipPath := filepath.Join(clipDir, "test-clip.mp4")
	err = os.WriteFile(clipPath, []byte("fake video data"), 0644)
	require.NoError(t, err)

	// Create test event with clip
	event := events.NewEvent()
	event.CameraID = "camera-1"
	event.EventType = events.EventTypePersonDetected
	event.Timestamp = time.Now()
	event.ClipPath = "test-clip.mp4" // Relative path

	// Save event
	err = stateMgr.SaveEvent(ctx, event.ToEventState())
	require.NoError(t, err)

	// Test: Play clip
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/clips/"+event.ID+"/play", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
}

func TestHandleGetSnapshot(t *testing.T) {
	server, stateMgr, storageSvc, cleanup := setupTestEventServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create test camera (required for foreign key constraint)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera)
	require.NoError(t, err)

	// Create test snapshot file
	snapshotDir := storageSvc.GetSnapshotsDir()
	err = os.MkdirAll(snapshotDir, 0755)
	require.NoError(t, err)

	snapshotPath := filepath.Join(snapshotDir, "test-snapshot.jpg")
	err = os.WriteFile(snapshotPath, []byte("fake image data"), 0644)
	require.NoError(t, err)

	// Create test event with snapshot
	event := events.NewEvent()
	event.CameraID = "camera-1"
	event.EventType = events.EventTypePersonDetected
	event.Timestamp = time.Now()
	event.SnapshotPath = "test-snapshot.jpg" // Relative path

	// Save event
	err = stateMgr.SaveEvent(ctx, event.ToEventState())
	require.NoError(t, err)

	// Test: Get snapshot
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/snapshots/"+event.ID, nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
}

func TestHandlePlayClip_NotFound(t *testing.T) {
	server, stateMgr, _, cleanup := setupTestEventServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create test camera (required for foreign key constraint)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera)
	require.NoError(t, err)

	// Create test event without clip
	event := events.NewEvent()
	event.CameraID = "camera-1"
	event.EventType = events.EventTypePersonDetected
	event.Timestamp = time.Now()
	// No clip path

	// Save event
	err = stateMgr.SaveEvent(ctx, event.ToEventState())
	require.NoError(t, err)

	// Test: Play clip (should return 404)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/clips/"+event.ID+"/play", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetSnapshot_NotFound(t *testing.T) {
	server, stateMgr, _, cleanup := setupTestEventServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create test camera (required for foreign key constraint)
	camera := state.CameraState{
		ID:      "camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err := stateMgr.SaveCamera(ctx, camera)
	require.NoError(t, err)

	// Create test event without snapshot
	event := events.NewEvent()
	event.CameraID = "camera-1"
	event.EventType = events.EventTypePersonDetected
	event.Timestamp = time.Now()
	// No snapshot path

	// Save event
	err = stateMgr.SaveEvent(ctx, event.ToEventState())
	require.NoError(t, err)

	// Test: Get snapshot (should return 404)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/snapshots/"+event.ID, nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

