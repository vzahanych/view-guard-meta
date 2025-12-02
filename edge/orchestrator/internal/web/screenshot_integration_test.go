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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// createTestImageBytes creates a test JPEG image and returns the bytes
func createTestImageBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	require.NoError(t, err)
	return buf.Bytes()
}

// setupTestWebServerWithScreenshots creates a web server with screenshot service
func setupTestWebServerWithScreenshots(t *testing.T) (*Server, *TestWebEnvironment, func()) {
	server, env, cleanup := setupTestWebServer(t)

	// Create screenshot service
	screenshotSvc, err := screenshots.NewService(env.StateMgr, env.Config, env.Logger)
	require.NoError(t, err)

	// Set screenshot service on server
	server.SetScreenshotService(screenshotSvc)

	return server, env, cleanup
}

// TestScreenshotWorkflow_CompleteFlow tests the complete screenshot workflow
func TestScreenshotWorkflow_CompleteFlow(t *testing.T) {
	server, env, cleanup := setupTestWebServerWithScreenshots(t)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Step 1: Create a test camera
	cameraData := map[string]interface{}{
		"id":        "test-camera-1",
		"name":      "Test Camera",
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

	// Step 2: Capture and save a screenshot
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	saveRequest := map[string]interface{}{
		"camera_id":   "test-camera-1",
		"label":       "normal",
		"description": "Test screenshot for integration test",
		"image_data":  imageBase64,
	}

	saveBody, _ := json.Marshal(saveRequest)
	req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var saveResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saveResponse))
	screenshotID, ok := saveResponse["id"].(string)
	require.True(t, ok, "Response should contain screenshot ID")
	require.NotEmpty(t, screenshotID)

	// Step 3: List screenshots
	req = httptest.NewRequest("GET", "/api/screenshots", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResponse))
	screenshotsList, ok := listResponse["screenshots"].([]interface{})
	require.True(t, ok)
	require.Greater(t, len(screenshotsList), 0)

	// Verify our screenshot is in the list
	found := false
	for _, s := range screenshotsList {
		if sMap, ok := s.(map[string]interface{}); ok {
			if sMap["id"] == screenshotID {
				found = true
				assert.Equal(t, "normal", sMap["label"])
				assert.Equal(t, "Test screenshot for integration test", sMap["description"])
				break
			}
		}
	}
	require.True(t, found, "Saved screenshot should be in the list")

	// Step 4: View screenshot details
	req = httptest.NewRequest("GET", "/api/screenshots/"+screenshotID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var getResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResponse))
	assert.Equal(t, screenshotID, getResponse["id"])
	assert.Equal(t, "test-camera-1", getResponse["camera_id"])
	assert.Equal(t, "normal", getResponse["label"])

	// Step 5: Get screenshot image
	req = httptest.NewRequest("GET", "/api/screenshots/"+screenshotID+"/image", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Greater(t, len(w.Body.Bytes()), 0)

	// Step 6: Edit screenshot
	updateRequest := map[string]interface{}{
		"label":       "threat",
		"description": "Updated description",
	}

	updateBody, _ := json.Marshal(updateRequest)
	req = httptest.NewRequest("PUT", "/api/screenshots/"+screenshotID, bytes.NewBuffer(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updateResponse))
	assert.Equal(t, "threat", updateResponse["label"])
	assert.Equal(t, "Updated description", updateResponse["description"])

	// Verify update by getting screenshot again
	req = httptest.NewRequest("GET", "/api/screenshots/"+screenshotID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResponse))
	assert.Equal(t, "threat", getResponse["label"])
	assert.Equal(t, "Updated description", getResponse["description"])

	// Step 7: Delete screenshot
	req = httptest.NewRequest("DELETE", "/api/screenshots/"+screenshotID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	// Verify deletion by trying to get the screenshot
	req = httptest.NewRequest("GET", "/api/screenshots/"+screenshotID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestScreenshotWorkflow_DatasetStatusUpdates tests dataset status updates after save/delete
func TestScreenshotWorkflow_DatasetStatusUpdates(t *testing.T) {
	server, env, cleanup := setupTestWebServerWithScreenshots(t)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Create a test camera
	cameraData := map[string]interface{}{
		"id":        "test-camera-1",
		"name":      "Test Camera",
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

	// Get initial camera status
	req = httptest.NewRequest("GET", "/api/cameras/test-camera-1", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var initialCamera map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &initialCamera))

	// Save a screenshot with "normal" label
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	saveRequest := map[string]interface{}{
		"camera_id":  "test-camera-1",
		"label":      "normal",
		"image_data": imageBase64,
	}

	saveBody, _ := json.Marshal(saveRequest)
	req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var saveResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saveResponse))

	// Check if dataset status is included in response
	if datasetStatus, ok := saveResponse["dataset_status"].(map[string]interface{}); ok {
		assert.GreaterOrEqual(t, datasetStatus["labeled_snapshot_count"], float64(1))
		assert.Equal(t, float64(50), datasetStatus["required_snapshot_count"]) // Default min snapshots
	}

	// Get updated camera status
	req = httptest.NewRequest("GET", "/api/cameras/test-camera-1", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updatedCamera map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updatedCamera))

	// Verify dataset status was updated
	if datasetStatus, ok := updatedCamera["dataset_status"].(map[string]interface{}); ok {
		labeledCount, _ := datasetStatus["labeled_snapshot_count"].(float64)
		assert.GreaterOrEqual(t, labeledCount, float64(1))
	}

	// Delete the screenshot
	screenshotID, _ := saveResponse["id"].(string)
	req = httptest.NewRequest("DELETE", "/api/screenshots/"+screenshotID, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// Get camera status after deletion
	req = httptest.NewRequest("GET", "/api/cameras/test-camera-1", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var afterDeleteCamera map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &afterDeleteCamera))

	// Verify dataset status was updated after deletion
	if datasetStatus, ok := afterDeleteCamera["dataset_status"].(map[string]interface{}); ok {
		labeledCount, _ := datasetStatus["labeled_snapshot_count"].(float64)
		assert.Equal(t, float64(0), labeledCount)
	}
}

// TestScreenshotWorkflow_MultipleCameras tests multiple cameras with different snapshot counts
func TestScreenshotWorkflow_MultipleCameras(t *testing.T) {
	server, env, cleanup := setupTestWebServerWithScreenshots(t)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Create two cameras
	cameras := []map[string]interface{}{
		{
			"id":        "camera-1",
			"name":      "Camera 1",
			"type":      "rtsp",
			"enabled":   true,
			"rtsp_urls": []string{"rtsp://test1/stream"},
		},
		{
			"id":        "camera-2",
			"name":      "Camera 2",
			"type":      "rtsp",
			"enabled":   true,
			"rtsp_urls": []string{"rtsp://test2/stream"},
		},
	}

	for _, cameraData := range cameras {
		cameraBody, _ := json.Marshal(cameraData)
		req := httptest.NewRequest("POST", "/api/cameras", bytes.NewBuffer(cameraBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// Save 3 screenshots for camera-1
	var camera1ScreenshotIDs []string
	for i := 0; i < 3; i++ {
		saveRequest := map[string]interface{}{
			"camera_id":  "camera-1",
			"label":      "normal",
			"image_data": imageBase64,
		}

		saveBody, _ := json.Marshal(saveRequest)
		req := httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		screenshotID, _ := response["id"].(string)
		camera1ScreenshotIDs = append(camera1ScreenshotIDs, screenshotID)
	}

	// Save 1 screenshot for camera-2
	saveRequest := map[string]interface{}{
		"camera_id":  "camera-2",
		"label":      "normal",
		"image_data": imageBase64,
	}

	saveBody, _ := json.Marshal(saveRequest)
	req := httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Verify camera-1 has 3 screenshots
	req = httptest.NewRequest("GET", "/api/screenshots?camera_id=camera-1", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResponse))
	screenshotsList, _ := listResponse["screenshots"].([]interface{})
	assert.Equal(t, 3, len(screenshotsList))

	// Verify camera-2 has 1 screenshot
	req = httptest.NewRequest("GET", "/api/screenshots?camera_id=camera-2", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResponse))
	screenshotsList, _ = listResponse["screenshots"].([]interface{})
	assert.Equal(t, 1, len(screenshotsList))

	// Verify dataset status for each camera
	req = httptest.NewRequest("GET", "/api/cameras/camera-1", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var camera1 map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &camera1))
	if datasetStatus, ok := camera1["dataset_status"].(map[string]interface{}); ok {
		labeledCount, _ := datasetStatus["labeled_snapshot_count"].(float64)
		assert.Equal(t, float64(3), labeledCount)
	}

	req = httptest.NewRequest("GET", "/api/cameras/camera-2", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var camera2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &camera2))
	if datasetStatus, ok := camera2["dataset_status"].(map[string]interface{}); ok {
		labeledCount, _ := datasetStatus["labeled_snapshot_count"].(float64)
		assert.Equal(t, float64(1), labeledCount)
	}
}

// TestScreenshotWorkflow_ExportFunctionality tests export functionality with real data
func TestScreenshotWorkflow_ExportFunctionality(t *testing.T) {
	server, env, cleanup := setupTestWebServerWithScreenshots(t)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Create a test camera
	cameraData := map[string]interface{}{
		"id":        "test-camera-1",
		"name":      "Test Camera",
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

	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// Save multiple screenshots with different labels
	labels := []string{"normal", "normal", "threat", "abnormal"}
	var screenshotIDs []string

	for _, label := range labels {
		saveRequest := map[string]interface{}{
			"camera_id":  "test-camera-1",
			"label":      label,
			"image_data": imageBase64,
		}

		saveBody, _ := json.Marshal(saveRequest)
		req := httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		screenshotID, _ := response["id"].(string)
		screenshotIDs = append(screenshotIDs, screenshotID)
	}

	// Export all screenshots
	exportRequest := map[string]interface{}{
		"include_metadata": true,
	}

	exportBody, _ := json.Marshal(exportRequest)
	req = httptest.NewRequest("POST", "/api/screenshots/export", bytes.NewBuffer(exportBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Greater(t, len(w.Body.Bytes()), 0)

	// Export by label
	exportRequest = map[string]interface{}{
		"label": "normal",
	}

	exportBody, _ = json.Marshal(exportRequest)
	req = httptest.NewRequest("POST", "/api/screenshots/export", bytes.NewBuffer(exportBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))

	// Export by camera
	exportRequest = map[string]interface{}{
		"camera_id": "test-camera-1",
	}

	exportBody, _ = json.Marshal(exportRequest)
	req = httptest.NewRequest("POST", "/api/screenshots/export", bytes.NewBuffer(exportBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
}

// TestScreenshotWorkflow_ErrorRecovery tests error recovery scenarios
func TestScreenshotWorkflow_ErrorRecovery(t *testing.T) {
	server, env, cleanup := setupTestWebServerWithScreenshots(t)
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Test: Try to save screenshot with invalid base64
	invalidRequest := map[string]interface{}{
		"camera_id":  "test-camera-1",
		"label":      "normal",
		"image_data": "invalid-base64!!!",
	}

	invalidBody, _ := json.Marshal(invalidRequest)
	req := httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(invalidBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	// Test: Try to save screenshot with missing camera
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	missingCameraRequest := map[string]interface{}{
		"camera_id":  "non-existent-camera",
		"label":      "normal",
		"image_data": imageBase64,
	}

	missingCameraBody, _ := json.Marshal(missingCameraRequest)
	req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(missingCameraBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	// Test: Try to get non-existent screenshot
	req = httptest.NewRequest("GET", "/api/screenshots/non-existent-id", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	// Test: Try to update non-existent screenshot
	updateRequest := map[string]interface{}{
		"label": "threat",
	}

	updateBody, _ := json.Marshal(updateRequest)
	req = httptest.NewRequest("PUT", "/api/screenshots/non-existent-id", bytes.NewBuffer(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	// Test: Try to delete non-existent screenshot
	req = httptest.NewRequest("DELETE", "/api/screenshots/non-existent-id", nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestScreenshotWorkflow_ServiceUnavailable tests behavior when screenshot service is unavailable
func TestScreenshotWorkflow_ServiceUnavailable(t *testing.T) {
	server, _, cleanup := setupTestWebServer(t) // Note: No screenshot service set
	defer cleanup()

	ctx := context.Background()

	// Start server
	require.NoError(t, server.Start(ctx))
	defer server.Stop(ctx)

	server.GetStatus().SetStatus(service.StatusRunning)
	time.Sleep(100 * time.Millisecond)

	// Test: Try to list screenshots without service
	req := httptest.NewRequest("GET", "/api/screenshots", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Test: Try to save screenshot without service
	imageData := createTestImageBytes(t)
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	saveRequest := map[string]interface{}{
		"camera_id":  "test-camera-1",
		"label":      "normal",
		"image_data": imageBase64,
	}

	saveBody, _ := json.Marshal(saveRequest)
	req = httptest.NewRequest("POST", "/api/screenshots", bytes.NewBuffer(saveBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
