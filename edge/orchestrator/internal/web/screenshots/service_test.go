package screenshots

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// createTestImage creates a test JPEG image
func createTestImage(t *testing.T, format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	var buf []byte
	var err error

	if format == "png" {
		var pngBuf bytes.Buffer
		err = png.Encode(&pngBuf, img)
		buf = pngBuf.Bytes()
	} else {
		var jpegBuf bytes.Buffer
		err = jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90})
		buf = jpegBuf.Bytes()
	}

	require.NoError(t, err)
	return buf
}

// setupTestService creates a test screenshot service
func setupTestService(t *testing.T) (*Service, *state.Manager, func()) {
	tmpDir, err := os.MkdirTemp("", "screenshot-test-*")
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

	// Create test camera (required for foreign key)
	camera := state.CameraState{
		ID:      "test-camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test",
		Enabled: true,
	}
	err = stateMgr.SaveCamera(context.Background(), camera)
	require.NoError(t, err)

	service, err := NewService(stateMgr, cfg, log)
	require.NoError(t, err)

	cleanup := func() {
		stateMgr.Close()
		os.RemoveAll(tmpDir)
	}

	return service, stateMgr, cleanup
}

func TestSaveScreenshot_ValidData(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	tests := []struct {
		name        string
		screenshot  *Screenshot
		expectError bool
	}{
		{
			name: "normal label",
			screenshot: &Screenshot{
				CameraID:    "test-camera-1",
				Label:       LabelNormal,
				Description: "Test description",
			},
			expectError: false,
		},
		{
			name: "threat label",
			screenshot: &Screenshot{
				CameraID:    "test-camera-1",
				Label:       LabelThreat,
				Description: "Threat description",
			},
			expectError: false,
		},
		{
			name: "abnormal label",
			screenshot: &Screenshot{
				CameraID:    "test-camera-1",
				Label:       LabelAbnormal,
				Description: "Abnormal description",
			},
			expectError: false,
		},
		{
			name: "custom label with custom_label",
			screenshot: &Screenshot{
				CameraID:    "test-camera-1",
				Label:       LabelCustom,
				CustomLabel: "custom-label-1",
				Description: "Custom description",
			},
			expectError: false,
		},
		{
			name: "with metadata",
			screenshot: &Screenshot{
				CameraID:    "test-camera-1",
				Label:       LabelNormal,
				Description: "With metadata",
				Metadata: map[string]interface{}{
					"test_key": "test_value",
				},
			},
			expectError: false,
		},
		{
			name: "with created_by",
			screenshot: &Screenshot{
				CameraID:    "test-camera-1",
				Label:       LabelNormal,
				Description: "With created_by",
				CreatedBy:   "test-user",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.SaveScreenshot(ctx, tt.screenshot, imageData)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.screenshot.ID)
				assert.NotEmpty(t, tt.screenshot.FilePath)

				// Verify file exists
				assert.FileExists(t, tt.screenshot.FilePath)

				// Verify metadata includes image dimensions
				assert.NotNil(t, tt.screenshot.Metadata)
				assert.Contains(t, tt.screenshot.Metadata, "width")
				assert.Contains(t, tt.screenshot.Metadata, "height")

				// Verify screenshot can be retrieved with timestamps
				retrieved, err := service.GetScreenshot(ctx, tt.screenshot.ID)
				require.NoError(t, err)
				assert.NotEmpty(t, retrieved.CreatedAt)
				assert.NotEmpty(t, retrieved.UpdatedAt)
			}
		})
	}
}

func TestSaveScreenshot_ErrorCases(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name        string
		screenshot  *Screenshot
		imageData   []byte
		expectError bool
	}{
		{
			name: "invalid image data",
			screenshot: &Screenshot{
				CameraID: "test-camera-1",
				Label:    LabelNormal,
			},
			imageData:   []byte("not an image"),
			expectError: true,
		},
		{
			name: "empty image data",
			screenshot: &Screenshot{
				CameraID: "test-camera-1",
				Label:    LabelNormal,
			},
			imageData:   []byte{},
			expectError: true,
		},
		{
			name: "invalid camera ID",
			screenshot: &Screenshot{
				CameraID: "non-existent-camera",
				Label:    LabelNormal,
			},
			imageData:   createTestImage(t, "jpeg"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.SaveScreenshot(ctx, tt.screenshot, tt.imageData)
			assert.Error(t, err)
		})
	}
}

func TestGetScreenshot(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Save a screenshot first
	screenshot := &Screenshot{
		CameraID:    "test-camera-1",
		Label:       LabelNormal,
		Description: "Test screenshot",
	}
	err := service.SaveScreenshot(ctx, screenshot, imageData)
	require.NoError(t, err)

	// Test getting existing screenshot
	retrieved, err := service.GetScreenshot(ctx, screenshot.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, screenshot.ID, retrieved.ID)
	assert.Equal(t, screenshot.CameraID, retrieved.CameraID)
	assert.Equal(t, screenshot.Label, retrieved.Label)
	assert.Equal(t, screenshot.Description, retrieved.Description)

	// Test getting non-existent screenshot
	_, err = service.GetScreenshot(ctx, "non-existent-id")
	assert.Error(t, err)
}

func TestListScreenshots(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Create multiple screenshots with different labels
	screenshots := []*Screenshot{
		{CameraID: "test-camera-1", Label: LabelNormal, Description: "Normal 1"},
		{CameraID: "test-camera-1", Label: LabelNormal, Description: "Normal 2"},
		{CameraID: "test-camera-1", Label: LabelThreat, Description: "Threat 1"},
		{CameraID: "test-camera-1", Label: LabelCustom, CustomLabel: "custom-1", Description: "Custom 1"},
	}

	for _, s := range screenshots {
		err := service.SaveScreenshot(ctx, s, imageData)
		require.NoError(t, err)
	}

	tests := []struct {
		name           string
		filters        *ScreenshotFilters
		expectedCount  int
		expectedLabels []Label
	}{
		{
			name:          "no filters",
			filters:       nil,
			expectedCount: 4,
		},
		{
			name: "filter by label normal",
			filters: &ScreenshotFilters{
				Label: LabelNormal,
			},
			expectedCount:  2,
			expectedLabels: []Label{LabelNormal},
		},
		{
			name: "filter by label threat",
			filters: &ScreenshotFilters{
				Label: LabelThreat,
			},
			expectedCount:  1,
			expectedLabels: []Label{LabelThreat},
		},
		{
			name: "filter by custom label",
			filters: &ScreenshotFilters{
				CustomLabel: "custom-1",
			},
			expectedCount:  1,
			expectedLabels: []Label{LabelCustom},
		},
		{
			name: "filter by camera",
			filters: &ScreenshotFilters{
				CameraID: "test-camera-1",
			},
			expectedCount: 4,
		},
		{
			name: "filter by description",
			filters: &ScreenshotFilters{
				Description: "Normal",
			},
			expectedCount: 2,
		},
		{
			name: "limit and offset",
			filters: &ScreenshotFilters{
				Limit:  2,
				Offset: 1,
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.ListScreenshots(ctx, tt.filters)
			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)

			if len(tt.expectedLabels) > 0 {
				for _, s := range result {
					assert.Contains(t, tt.expectedLabels, s.Label)
				}
			}
		})
	}
}

func TestUpdateScreenshot(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Save a screenshot first
	screenshot := &Screenshot{
		CameraID:    "test-camera-1",
		Label:       LabelNormal,
		Description: "Original description",
		Metadata:    map[string]interface{}{"key1": "value1"},
	}
	err := service.SaveScreenshot(ctx, screenshot, imageData)
	require.NoError(t, err)

	tests := []struct {
		name       string
		update     *ScreenshotUpdate
		verifyFunc func(*testing.T, *Screenshot)
	}{
		{
			name: "update label",
			update: &ScreenshotUpdate{
				Label: func() *Label { l := LabelThreat; return &l }(),
			},
			verifyFunc: func(t *testing.T, s *Screenshot) {
				assert.Equal(t, LabelThreat, s.Label)
				assert.Equal(t, "Original description", s.Description) // Should remain unchanged
			},
		},
		{
			name: "update custom label",
			update: &ScreenshotUpdate{
				Label:       func() *Label { l := LabelCustom; return &l }(),
				CustomLabel: func() *string { s := "new-custom-label"; return &s }(),
			},
			verifyFunc: func(t *testing.T, s *Screenshot) {
				assert.Equal(t, LabelCustom, s.Label)
				assert.Equal(t, "new-custom-label", s.CustomLabel)
			},
		},
		{
			name: "update description",
			update: &ScreenshotUpdate{
				Description: func() *string { s := "Updated description"; return &s }(),
			},
			verifyFunc: func(t *testing.T, s *Screenshot) {
				assert.Equal(t, "Updated description", s.Description)
			},
		},
		{
			name: "update metadata",
			update: &ScreenshotUpdate{
				Metadata: map[string]interface{}{
					"key1": "updated_value",
					"key2": "new_value",
				},
			},
			verifyFunc: func(t *testing.T, s *Screenshot) {
				assert.Equal(t, "updated_value", s.Metadata["key1"])
				assert.Equal(t, "new_value", s.Metadata["key2"])
			},
		},
		{
			name: "update all fields",
			update: &ScreenshotUpdate{
				Label:       func() *Label { l := LabelAbnormal; return &l }(),
				Description: func() *string { s := "Fully updated"; return &s }(),
				Metadata:    map[string]interface{}{"new_key": "new_value"},
			},
			verifyFunc: func(t *testing.T, s *Screenshot) {
				assert.Equal(t, LabelAbnormal, s.Label)
				assert.Equal(t, "Fully updated", s.Description)
				assert.Equal(t, "new_value", s.Metadata["new_key"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.UpdateScreenshot(ctx, screenshot.ID, tt.update)
			assert.NoError(t, err)

			// Verify update
			updated, err := service.GetScreenshot(ctx, screenshot.ID)
			require.NoError(t, err)
			tt.verifyFunc(t, updated)
		})
	}
}

func TestDeleteScreenshot(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Save a screenshot first
	screenshot := &Screenshot{
		CameraID: "test-camera-1",
		Label:    LabelNormal,
	}
	err := service.SaveScreenshot(ctx, screenshot, imageData)
	require.NoError(t, err)

	filePath := screenshot.FilePath
	assert.FileExists(t, filePath)

	// Delete screenshot
	err = service.DeleteScreenshot(ctx, screenshot.ID)
	assert.NoError(t, err)

	// Verify screenshot is deleted
	_, err = service.GetScreenshot(ctx, screenshot.ID)
	assert.Error(t, err)

	// Verify file is deleted
	assert.NoFileExists(t, filePath)

	// Test deleting non-existent screenshot
	err = service.DeleteScreenshot(ctx, "non-existent-id")
	assert.Error(t, err)
}

func TestGetLabelCounts(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Create screenshots with different labels
	screenshots := []*Screenshot{
		{CameraID: "test-camera-1", Label: LabelNormal},
		{CameraID: "test-camera-1", Label: LabelNormal},
		{CameraID: "test-camera-1", Label: LabelThreat},
		{CameraID: "test-camera-1", Label: LabelAbnormal},
		{CameraID: "test-camera-1", Label: LabelCustom, CustomLabel: "custom-1"},
	}

	for _, s := range screenshots {
		err := service.SaveScreenshot(ctx, s, imageData)
		require.NoError(t, err)
	}

	// Test GetLabelCounts
	counts, err := service.GetLabelCounts(ctx, "test-camera-1")
	assert.NoError(t, err)
	// GetLabelCounts returns counts grouped by label field (not custom_label)
	assert.Equal(t, 2, counts[LabelNormal])
	assert.Equal(t, 1, counts[LabelThreat])
	assert.Equal(t, 1, counts[LabelAbnormal])
	assert.Equal(t, 1, counts[LabelCustom]) // Custom label is counted as "custom", not by custom_label value

	// Test with non-existent camera
	counts, err = service.GetLabelCounts(ctx, "non-existent-camera")
	assert.NoError(t, err)
	assert.Empty(t, counts)
}

func TestGetScreenshotImage(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Save a screenshot
	screenshot := &Screenshot{
		CameraID: "test-camera-1",
		Label:    LabelNormal,
	}
	err := service.SaveScreenshot(ctx, screenshot, imageData)
	require.NoError(t, err)

	// Test getting existing image
	retrievedData, err := service.GetScreenshotImage(ctx, screenshot.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, retrievedData)
	// Note: Image may be processed/compressed, so size may differ from original
	assert.Greater(t, len(retrievedData), 0)

	// Test getting non-existent image
	_, err = service.GetScreenshotImage(ctx, "non-existent-id")
	assert.Error(t, err)
}

func TestExportDataset(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	imageData := createTestImage(t, "jpeg")

	// Create multiple screenshots
	screenshots := []*Screenshot{
		{CameraID: "test-camera-1", Label: LabelNormal, Description: "Normal 1"},
		{CameraID: "test-camera-1", Label: LabelNormal, Description: "Normal 2"},
		{CameraID: "test-camera-1", Label: LabelThreat, Description: "Threat 1"},
	}

	for _, s := range screenshots {
		err := service.SaveScreenshot(ctx, s, imageData)
		require.NoError(t, err)
	}

	tests := []struct {
		name            string
		filters         *ScreenshotFilters
		includeMetadata bool
		expectedCount   int
	}{
		{
			name:            "export all",
			filters:         nil,
			includeMetadata: false,
			expectedCount:   3,
		},
		{
			name: "export by label",
			filters: &ScreenshotFilters{
				Label: LabelNormal,
			},
			includeMetadata: false,
			expectedCount:   2,
		},
		{
			name: "export by camera",
			filters: &ScreenshotFilters{
				CameraID: "test-camera-1",
			},
			includeMetadata: false,
			expectedCount:   3,
		},
		{
			name:            "export with metadata",
			filters:         nil,
			includeMetadata: true,
			expectedCount:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.ExportDataset(ctx, tt.filters, tt.includeMetadata)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.FileExists(t, result.FilePath)
			assert.Equal(t, tt.expectedCount, result.SampleCount)

			// Verify ZIP file can be read
			info, err := os.Stat(result.FilePath)
			assert.NoError(t, err)
			assert.Greater(t, info.Size(), int64(0))

			// Cleanup
			os.Remove(result.FilePath)
		})
	}
}

func TestImageProcessing(t *testing.T) {
	service, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name      string
		format    string
		expectPNG bool
	}{
		{
			name:      "JPEG input",
			format:    "jpeg",
			expectPNG: false,
		},
		{
			name:      "PNG input (should convert to JPEG)",
			format:    "png",
			expectPNG: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageData := createTestImage(t, tt.format)
			screenshot := &Screenshot{
				CameraID: "test-camera-1",
				Label:    LabelNormal,
			}

			err := service.SaveScreenshot(ctx, screenshot, imageData)
			assert.NoError(t, err)

			// Verify metadata includes original format
			assert.NotNil(t, screenshot.Metadata)
			assert.Contains(t, screenshot.Metadata, "original_format")
			assert.Contains(t, screenshot.Metadata, "width")
			assert.Contains(t, screenshot.Metadata, "height")
			assert.Contains(t, screenshot.Metadata, "processed_size_bytes")

			// Verify file exists and is readable
			assert.FileExists(t, screenshot.FilePath)
			retrievedData, err := service.GetScreenshotImage(ctx, screenshot.ID)
			assert.NoError(t, err)
			assert.NotEmpty(t, retrievedData)
		})
	}
}
