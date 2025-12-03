package dataset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// createTestScreenshotForService creates a test screenshot with image file
func createTestScreenshotForService(t *testing.T, tmpDir string, id string, label screenshots.Label) *screenshots.Screenshot {
	imagePath := filepath.Join(tmpDir, id+".jpg")
	imageData := []byte("fake-image-data-" + id)
	err := os.WriteFile(imagePath, imageData, 0644)
	require.NoError(t, err)

	return &screenshots.Screenshot{
		ID:          id,
		CameraID:    "test-camera-1",
		FilePath:    imagePath,
		Label:       label,
		Description: "Test screenshot " + id,
		CreatedAt:   time.Now(),
	}
}

// TestService_UploadDatasetForCamera tests the complete service flow
func TestService_UploadDatasetForCamera(t *testing.T) {
	// Create mock VM server
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			// Parse multipart form
			err := r.ParseMultipartForm(100 << 20)
			require.NoError(t, err)

			// Return success response
			response := map[string]interface{}{
				"success":    true,
				"dataset_id": "test-dataset-456",
				"message":    "Dataset uploaded successfully",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockVM.Close()

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	exportsDir := filepath.Join(tmpDir, "exports")
	os.MkdirAll(exportsDir, 0755)

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: dataDir,
			},
			AI: config.AIConfig{
				DatasetExportDir: exportsDir,
			},
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	service := NewService(cfg, log, "test-edge-1")

	// Create test screenshots
	screenshotList := []*screenshots.Screenshot{
		createTestScreenshotForService(t, tmpDir, "screenshot-1", screenshots.LabelNormal),
		createTestScreenshotForService(t, tmpDir, "screenshot-2", screenshots.LabelNormal),
		createTestScreenshotForService(t, tmpDir, "screenshot-3", screenshots.LabelThreat),
	}

	ctx := context.Background()
	datasetID, err := service.UploadDatasetForCamera(ctx, "test-camera-1", screenshotList)
	require.NoError(t, err)
	assert.Equal(t, "test-dataset-456", datasetID)

	// Verify archive was cleaned up
	files, err := os.ReadDir(exportsDir)
	require.NoError(t, err)
	assert.Equal(t, 0, len(files), "Archive should be cleaned up after upload")
}

// TestService_UploadDatasetForCamera_EmptyList tests error handling for empty screenshot list
func TestService_UploadDatasetForCamera_EmptyList(t *testing.T) {
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: t.TempDir(),
			},
		},
	}
	log := logger.NewNopLogger()
	service := NewService(cfg, log, "test-edge-1")

	ctx := context.Background()
	_, err := service.UploadDatasetForCamera(ctx, "test-camera-1", []*screenshots.Screenshot{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no screenshots")
}

// TestService_UploadDatasetForCamera_UploadFailure tests error handling when upload fails
func TestService_UploadDatasetForCamera_UploadFailure(t *testing.T) {
	// Create mock VM server that returns error
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		}
	}))
	defer mockVM.Close()

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	exportsDir := filepath.Join(tmpDir, "exports")
	os.MkdirAll(exportsDir, 0755)

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: dataDir,
			},
			AI: config.AIConfig{
				DatasetExportDir: exportsDir,
			},
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	service := NewService(cfg, log, "test-edge-1")

	// Create test screenshots
	screenshotList := []*screenshots.Screenshot{
		createTestScreenshotForService(t, tmpDir, "screenshot-1", screenshots.LabelNormal),
	}

	ctx := context.Background()
	_, err := service.UploadDatasetForCamera(ctx, "test-camera-1", screenshotList)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}

// TestService_UploadDatasetForCamera_ArchiveCleanup tests that archive is cleaned up even on error
func TestService_UploadDatasetForCamera_ArchiveCleanup(t *testing.T) {
	// Create mock VM server that returns error
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer mockVM.Close()

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	exportsDir := filepath.Join(tmpDir, "exports")
	os.MkdirAll(exportsDir, 0755)

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: dataDir,
			},
			AI: config.AIConfig{
				DatasetExportDir: exportsDir,
			},
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	service := NewService(cfg, log, "test-edge-1")

	// Create test screenshots
	screenshotList := []*screenshots.Screenshot{
		createTestScreenshotForService(t, tmpDir, "screenshot-1", screenshots.LabelNormal),
	}

	ctx := context.Background()
	_, err := service.UploadDatasetForCamera(ctx, "test-camera-1", screenshotList)
	require.Error(t, err)

	// Verify archive was cleaned up even on error
	files, err := os.ReadDir(exportsDir)
	require.NoError(t, err)
	assert.Equal(t, 0, len(files), "Archive should be cleaned up even on upload failure")
}
