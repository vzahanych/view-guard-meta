package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// createTestImage creates a test JPEG image and returns base64 encoded string
func createTestImageBase64(t *testing.T) string {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// setupTestScreenshotServer creates a test server with screenshot service
func setupTestScreenshotServer(t *testing.T) (*Server, *state.Manager, *screenshots.Service, func()) {
	tmpDir, err := os.MkdirTemp("", "screenshot-handler-test-*")
	require.NoError(t, err)

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
			Storage: config.StorageConfig{
				MaxDiskUsagePercent: 80.0,
			},
		},
	}

	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	stateMgr, err := state.NewManager(cfg, log)
	require.NoError(t, err)

	// Create test camera
	camera := state.CameraState{
		ID:      "test-camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err = stateMgr.SaveCamera(context.Background(), camera)
	require.NoError(t, err)

	// Create screenshot service
	screenshotSvc, err := screenshots.NewService(stateMgr, cfg, log)
	require.NoError(t, err)

	// Create web server
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	server := NewServer(webCfg, log)
	server.SetScreenshotService(screenshotSvc)
	server.setupRoutes()

	cleanup := func() {
		stateMgr.Close()
		os.RemoveAll(tmpDir)
	}

	return server, stateMgr, screenshotSvc, cleanup
}

func TestHandleSaveScreenshot(t *testing.T) {
	server, _, _, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	imageData := createTestImageBase64(t)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "valid request with normal label",
			requestBody: map[string]interface{}{
				"camera_id":  "test-camera-1",
				"label":      "normal",
				"image_data": imageData,
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "valid request with custom label",
			requestBody: map[string]interface{}{
				"camera_id":    "test-camera-1",
				"label":        "custom",
				"custom_label": "custom-label-1",
				"image_data":   imageData,
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "valid request with description and metadata",
			requestBody: map[string]interface{}{
				"camera_id":   "test-camera-1",
				"label":       "normal",
				"description": "Test description",
				"metadata": map[string]interface{}{
					"test_key": "test_value",
				},
				"image_data": imageData,
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "missing camera_id",
			requestBody: map[string]interface{}{
				"label":      "normal",
				"image_data": imageData,
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "missing label",
			requestBody: map[string]interface{}{
				"camera_id":  "test-camera-1",
				"image_data": imageData,
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "missing image_data",
			requestBody: map[string]interface{}{
				"camera_id": "test-camera-1",
				"label":     "normal",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "invalid base64 image data",
			requestBody: map[string]interface{}{
				"camera_id":  "test-camera-1",
				"label":      "normal",
				"image_data": "not-valid-base64",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "invalid camera_id",
			requestBody: map[string]interface{}{
				"camera_id":  "non-existent-camera",
				"label":      "normal",
				"image_data": imageData,
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "id")
				assert.Contains(t, response, "camera_id")
				assert.Contains(t, response, "file_path")
			}
		})
	}
}

func TestHandleListScreenshots(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)

	// Create test screenshots
	screenshots := []*screenshots.Screenshot{
		{CameraID: "test-camera-1", Label: screenshots.LabelNormal, Description: "Normal 1"},
		{CameraID: "test-camera-1", Label: screenshots.LabelThreat, Description: "Threat 1"},
		{CameraID: "test-camera-1", Label: screenshots.LabelNormal, Description: "Normal 2"},
	}

	// Decode image data
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	for _, s := range screenshots {
		err := screenshotSvc.SaveScreenshot(ctx, s, decodedImage)
		require.NoError(t, err)
	}

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "list all screenshots",
			url:            "/api/screenshots",
			expectedStatus: http.StatusOK,
			expectedCount:  3,
		},
		{
			name:           "filter by label normal",
			url:            "/api/screenshots?label=normal",
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "filter by label threat",
			url:            "/api/screenshots?label=threat",
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "filter by camera",
			url:            "/api/screenshots?camera_id=test-camera-1",
			expectedStatus: http.StatusOK,
			expectedCount:  3,
		},
		{
			name:           "filter by description",
			url:            "/api/screenshots?description=Normal",
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "pagination with limit",
			url:            "/api/screenshots?limit=2",
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "pagination with limit and offset",
			url:            "/api/screenshots?limit=2&offset=1",
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "sort by created_at desc",
			url:            "/api/screenshots?sort_by=created_at&sort_order=desc",
			expectedStatus: http.StatusOK,
			expectedCount:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", tt.url, nil)
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Contains(t, response, "screenshots")
			assert.Contains(t, response, "count")

			screenshotsList := response["screenshots"].([]interface{})
			assert.Len(t, screenshotsList, tt.expectedCount)
		})
	}
}

func TestHandleGetScreenshot(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	// Create a test screenshot
	screenshot := &screenshots.Screenshot{
		CameraID:    "test-camera-1",
		Label:       screenshots.LabelNormal,
		Description: "Test screenshot",
	}
	err = screenshotSvc.SaveScreenshot(ctx, screenshot, decodedImage)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		screenshotID   string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "get existing screenshot",
			screenshotID:   screenshot.ID,
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "get non-existent screenshot",
			screenshotID:   "non-existent-id",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/screenshots/"+tt.screenshotID, nil)
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.screenshotID, response["id"])
			}
		})
	}
}

func TestHandleGetScreenshotImage(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	// Create a test screenshot
	screenshot := &screenshots.Screenshot{
		CameraID: "test-camera-1",
		Label:    screenshots.LabelNormal,
	}
	err = screenshotSvc.SaveScreenshot(ctx, screenshot, decodedImage)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		screenshotID   string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "get existing image",
			screenshotID:   screenshot.ID,
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "get non-existent image",
			screenshotID:   "non-existent-id",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/screenshots/"+tt.screenshotID+"/image", nil)
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
				assert.Greater(t, len(w.Body.Bytes()), 0)
			}
		})
	}
}

func TestHandleGetScreenshotThumbnail(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	// Create a test screenshot
	screenshot := &screenshots.Screenshot{
		CameraID: "test-camera-1",
		Label:    screenshots.LabelNormal,
	}
	err = screenshotSvc.SaveScreenshot(ctx, screenshot, decodedImage)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/screenshots/"+screenshot.ID+"/thumbnail", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Greater(t, len(w.Body.Bytes()), 0)
}

func TestHandleUpdateScreenshot(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	// Create a test screenshot
	screenshot := &screenshots.Screenshot{
		CameraID:    "test-camera-1",
		Label:       screenshots.LabelNormal,
		Description: "Original description",
	}
	err = screenshotSvc.SaveScreenshot(ctx, screenshot, decodedImage)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		screenshotID   string
		requestBody    map[string]interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:         "update label",
			screenshotID: screenshot.ID,
			requestBody: map[string]interface{}{
				"label": "threat",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:         "update custom label",
			screenshotID: screenshot.ID,
			requestBody: map[string]interface{}{
				"label":        "custom",
				"custom_label": "new-custom-label",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:         "update description",
			screenshotID: screenshot.ID,
			requestBody: map[string]interface{}{
				"description": "Updated description",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:         "update metadata",
			screenshotID: screenshot.ID,
			requestBody: map[string]interface{}{
				"metadata": map[string]interface{}{
					"new_key": "new_value",
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:         "update non-existent screenshot",
			screenshotID: "non-existent-id",
			requestBody: map[string]interface{}{
				"label": "threat",
			},
			expectedStatus: http.StatusInternalServerError, // Service returns error, handler returns 500
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("PUT", "/api/screenshots/"+tt.screenshotID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.screenshotID, response["id"])
			}
		})
	}
}

func TestHandleDeleteScreenshot(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	// Create a test screenshot
	screenshot := &screenshots.Screenshot{
		CameraID: "test-camera-1",
		Label:    screenshots.LabelNormal,
	}
	err = screenshotSvc.SaveScreenshot(ctx, screenshot, decodedImage)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		screenshotID   string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "delete existing screenshot",
			screenshotID:   screenshot.ID,
			expectedStatus: http.StatusNoContent, // DELETE returns 204 No Content
			expectError:    false,
		},
		{
			name:           "delete non-existent screenshot",
			screenshotID:   "non-existent-id",
			expectedStatus: http.StatusInternalServerError, // Service returns error, handler returns 500
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("DELETE", "/api/screenshots/"+tt.screenshotID, nil)
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				// Verify screenshot is deleted
				_, err := screenshotSvc.GetScreenshot(ctx, tt.screenshotID)
				assert.Error(t, err)
			}
		})
	}
}

func TestHandleExportScreenshots(t *testing.T) {
	server, _, screenshotSvc, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImageBase64(t)
	decodedImage, err := base64.StdEncoding.DecodeString(imageData)
	require.NoError(t, err)

	// Create test screenshots
	screenshots := []*screenshots.Screenshot{
		{CameraID: "test-camera-1", Label: screenshots.LabelNormal},
		{CameraID: "test-camera-1", Label: screenshots.LabelThreat},
	}

	for _, s := range screenshots {
		err := screenshotSvc.SaveScreenshot(ctx, s, decodedImage)
		require.NoError(t, err)
	}

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "export all screenshots",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "export by label",
			requestBody: map[string]interface{}{
				"label": "normal",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "export by camera",
			requestBody: map[string]interface{}{
				"camera_id": "test-camera-1",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "export with metadata",
			requestBody: map[string]interface{}{
				"include_metadata": true,
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/screenshots/export", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
				assert.Greater(t, len(w.Body.Bytes()), 0)
			}
		})
	}
}

func TestDecodeBase64Image(t *testing.T) {
	// Create test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	require.NoError(t, err)

	validBase64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	validDataURL := "data:image/jpeg;base64," + validBase64

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "valid base64 string",
			input:       validBase64,
			expectError: false,
		},
		{
			name:        "valid data URL",
			input:       validDataURL,
			expectError: false,
		},
		{
			name:        "invalid base64",
			input:       "not-valid-base64!!!",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeBase64Image(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestHandleGetDatasetStatus_ReadOnlyEndpoint(t *testing.T) {
	server, _, _, cleanup := setupTestScreenshotServer(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	// Save a single "normal" screenshot via API so that dataset status has data.
	imageData := createTestImageBase64(t)

	saveRequest := map[string]interface{}{
		"camera_id":   "test-camera-1",
		"label":       "normal",
		"description": "Dataset status test screenshot",
		"image_data":  imageData,
	}

	saveBody, _ := json.Marshal(saveRequest)
	req := httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Call the new read-only dataset status endpoint
	req = httptest.NewRequest("GET", "/api/cameras/test-camera-1/dataset", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "test-camera-1", resp["camera_id"])

	ds, ok := resp["dataset_status"].(map[string]interface{})
	require.True(t, ok, "dataset_status should be present in response")

	// RequiredSnapshotCount should default to 50 unless overridden by config
	required, ok := ds["required_snapshot_count"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(50), required)

	// We saved at least one normal screenshot, so labeled_snapshot_count should be >= 1
	labeled, ok := ds["labeled_snapshot_count"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, labeled, float64(1))
}

func TestHandleScreenshotServiceUnavailable(t *testing.T) {
	// Create server without screenshot service
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	server := NewServer(webCfg, log)
	server.setupRoutes()

	gin.SetMode(gin.TestMode)

	endpoints := []string{
		"/api/screenshots",
		"/api/screenshots/test-id",
		"/api/screenshots/test-id/image",
		"/api/screenshots/test-id/thumbnail",
	}

	for _, endpoint := range endpoints {
		t.Run("service unavailable: "+endpoint, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", endpoint, nil)
			server.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		})
	}
}
