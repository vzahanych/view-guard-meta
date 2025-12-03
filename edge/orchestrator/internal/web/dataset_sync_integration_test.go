package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/capabilities"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/dataset"
	grpcclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/grpc"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

// setupTestWebServerWithDatasetSync creates a web server with dataset sync capabilities
func setupTestWebServerWithDatasetSync(t *testing.T, vmEndpoint string) (*Server, *TestWebEnvironment, func()) {
	server, env, cleanup := setupTestWebServerWithScreenshots(t)

	// Create WireGuard client (disabled for tests)
	wgClient := wireguard.NewClient(&env.Config.Edge.WireGuard, env.Logger)

	// Create mock gRPC client (for capability sync)
	grpcClient := grpcclient.NewClient(&env.Config.Edge.WireGuard, wgClient, env.Logger)

	// Create capability sync service
	capabilitySync := capabilities.NewSyncService(env.Config, env.CameraMgr, server.screenshotSvc.(*screenshots.Service), grpcClient, env.Logger)
	server.SetCapabilitySyncService(capabilitySync)

	// Create dataset service with test VM endpoint
	cfg := env.Config
	if vmEndpoint != "" {
		cfg.Edge.WireGuard.KVMEndpoint = vmEndpoint
	}
	datasetSvc := dataset.NewService(cfg, env.Logger, "test-edge-1")
	server.SetDatasetService(datasetSvc)

	return server, env, cleanup
}

// TestDatasetSync_CompleteFlow tests the complete dataset sync flow
func TestDatasetSync_CompleteFlow(t *testing.T) {
	// Create a mock HTTP server to simulate VM endpoint
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			// Parse multipart form
			err := r.ParseMultipartForm(100 << 20) // 100MB max
			require.NoError(t, err)

			// Verify form fields
			cameraID := r.FormValue("camera_id")
			checksum := r.FormValue("checksum")
			require.NotEmpty(t, cameraID)
			require.NotEmpty(t, checksum)

			// Verify file was uploaded
			file, header, err := r.FormFile("dataset")
			require.NoError(t, err)
			defer file.Close()
			require.NotNil(t, header)
			assert.Greater(t, header.Size, int64(0))

			// Return success response with dataset ID
			response := map[string]interface{}{
				"success":    true,
				"dataset_id": "test-dataset-123",
				"edge_id":    "test-edge-1",
				"camera_id":  cameraID,
				"message":    "Dataset uploaded and processed successfully",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockVM.Close()

	server, _, cleanup := setupTestWebServerWithDatasetSync(t, mockVM.URL)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Step 1: Create a test camera
	cameraData := map[string]interface{}{
		"id":        "test-camera-sync",
		"name":      "Test Camera for Sync",
		"type":      "rtsp",
		"enabled":   true,
		"rtsp_urls": []string{"rtsp://test/stream"},
	}

	cameraBody, _ := json.Marshal(cameraData)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewBuffer(cameraBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Step 2: Create 50+ labeled screenshots (minimum required for sync)
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	minSnapshots := 50
	for i := 0; i < minSnapshots; i++ {
		saveRequest := map[string]interface{}{
			"camera_id":  "test-camera-sync",
			"label":      "normal",
			"image_data": imageBase64,
		}

		saveBody, _ := json.Marshal(saveRequest)
		req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, "Failed to create screenshot %d", i+1)
	}

	// Step 3: Verify dataset status shows ready
	req = httptest.NewRequest("GET", "/api/cameras/test-camera-sync/dataset/status", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var statusResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &statusResponse))
	datasetStatus, ok := statusResponse["dataset_status"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, datasetStatus["snapshot_required"], "Dataset should be ready for sync")

	// Step 4: Trigger dataset sync
	req = httptest.NewRequest("POST", "/api/cameras/test-camera-sync/dataset/sync", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "Sync should succeed")

	var syncResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &syncResponse))
	assert.Equal(t, true, syncResponse["dataset_synced"])
	assert.NotEmpty(t, syncResponse["dataset_id"])
	assert.Equal(t, "test-camera-sync", syncResponse["camera_id"])
}

// TestDatasetSync_NotReady tests sync when dataset is not ready (< 50 snapshots)
func TestDatasetSync_NotReady(t *testing.T) {
	server, _, cleanup := setupTestWebServerWithDatasetSync(t, "")
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Create a test camera
	cameraData := map[string]interface{}{
		"id":        "test-camera-not-ready",
		"name":      "Test Camera Not Ready",
		"type":      "rtsp",
		"enabled":   true,
		"rtsp_urls": []string{"rtsp://test/stream"},
	}

	cameraBody, _ := json.Marshal(cameraData)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewBuffer(cameraBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Create only 10 screenshots (less than required 50)
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	for i := 0; i < 10; i++ {
		saveRequest := map[string]interface{}{
			"camera_id":  "test-camera-not-ready",
			"label":      "normal",
			"image_data": imageBase64,
		}

		saveBody, _ := json.Marshal(saveRequest)
		req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	}

	// Try to sync - should fail with Conflict status
	req = httptest.NewRequest("POST", "/api/cameras/test-camera-not-ready/dataset/sync", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, "Sync should fail when dataset not ready")

	var errorResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errorResponse))
	assert.Contains(t, errorResponse["error"].(string), "not ready")
}

// TestDatasetSync_UploadFailure tests handling of upload failures
func TestDatasetSync_UploadFailure(t *testing.T) {
	// Create a mock HTTP server that returns error
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			// Return server error
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Internal server error",
			})
		}
	}))
	defer mockVM.Close()

	server, _, cleanup := setupTestWebServerWithDatasetSync(t, mockVM.URL)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Create camera and 50+ screenshots
	cameraData := map[string]interface{}{
		"id":        "test-camera-upload-fail",
		"name":      "Test Camera Upload Fail",
		"type":      "rtsp",
		"enabled":   true,
		"rtsp_urls": []string{"rtsp://test/stream"},
	}

	cameraBody, _ := json.Marshal(cameraData)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewBuffer(cameraBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Create 50 screenshots
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	for i := 0; i < 50; i++ {
		saveRequest := map[string]interface{}{
			"camera_id":  "test-camera-upload-fail",
			"label":      "normal",
			"image_data": imageBase64,
		}

		saveBody, _ := json.Marshal(saveRequest)
		req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	}

	// Try to sync - should fail due to upload error
	req = httptest.NewRequest("POST", "/api/cameras/test-camera-upload-fail/dataset/sync", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "Sync should fail on upload error")

	var errorResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errorResponse))
	assert.Contains(t, errorResponse["error"].(string), "upload")
}

// TestDatasetSync_NetworkError tests handling of network errors
func TestDatasetSync_NetworkError(t *testing.T) {
	// Use invalid endpoint to simulate network error
	invalidEndpoint := "http://localhost:99999"

	server, _, cleanup := setupTestWebServerWithDatasetSync(t, invalidEndpoint)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Create camera and 50+ screenshots
	cameraData := map[string]interface{}{
		"id":        "test-camera-network-error",
		"name":      "Test Camera Network Error",
		"type":      "rtsp",
		"enabled":   true,
		"rtsp_urls": []string{"rtsp://test/stream"},
	}

	cameraBody, _ := json.Marshal(cameraData)
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewBuffer(cameraBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Create 50 screenshots
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	for i := 0; i < 50; i++ {
		saveRequest := map[string]interface{}{
			"camera_id":  "test-camera-network-error",
			"label":      "normal",
			"image_data": imageBase64,
		}

		saveBody, _ := json.Marshal(saveRequest)
		req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	}

	// Try to sync - should fail due to network error
	req = httptest.NewRequest("POST", "/api/cameras/test-camera-network-error/dataset/sync", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// Should fail with internal server error due to network issue
	require.Equal(t, http.StatusInternalServerError, w.Code, "Sync should fail on network error")
}
