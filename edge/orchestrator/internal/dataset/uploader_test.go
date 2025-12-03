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
)

// createTestArchive creates a test archive file for upload testing
func createTestArchive(t *testing.T) string {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test-dataset.tar.gz")

	// Create a simple archive file
	data := []byte("fake-archive-data")
	err := os.WriteFile(archivePath, data, 0644)
	require.NoError(t, err)

	return archivePath
}

// TestUploader_UploadDataset_Success tests successful upload
func TestUploader_UploadDataset_Success(t *testing.T) {
	// Create mock VM server
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			// Parse multipart form
			err := r.ParseMultipartForm(100 << 20)
			require.NoError(t, err)

			// Verify form fields
			cameraID := r.FormValue("camera_id")
			checksum := r.FormValue("checksum")
			assert.Equal(t, "test-camera-1", cameraID)
			assert.NotEmpty(t, checksum)

			// Verify file was uploaded
			file, header, err := r.FormFile("dataset")
			require.NoError(t, err)
			defer file.Close()
			assert.NotNil(t, header)
			assert.Greater(t, header.Size, int64(0))

			// Return success response
			response := map[string]interface{}{
				"success":    true,
				"dataset_id": "test-dataset-123",
				"message":    "Dataset uploaded successfully",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockVM.Close()

	// Create uploader with mock endpoint
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	archivePath := createTestArchive(t)
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	ctx := context.Background()
	result, err := uploader.UploadDataset(ctx, archivePath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "test-dataset-123", result.DatasetID)
}

// TestUploader_UploadDataset_RetryLogic tests retry logic on network errors
func TestUploader_UploadDataset_RetryLogic(t *testing.T) {
	attemptCount := 0
	// Create mock VM server that fails first 2 times, succeeds on 3rd
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			attemptCount++
			if attemptCount < 3 {
				// Return server error (should retry)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
			} else {
				// Success on 3rd attempt
				response := map[string]interface{}{
					"success":    true,
					"dataset_id": "test-dataset-123",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			}
		}
	}))
	defer mockVM.Close()

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	archivePath := createTestArchive(t)
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	ctx := context.Background()
	result, err := uploader.UploadDataset(ctx, archivePath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 3, attemptCount, "Should have retried 2 times before succeeding")
}

// TestUploader_UploadDataset_ClientError tests that client errors (4xx) are not retried
func TestUploader_UploadDataset_ClientError(t *testing.T) {
	attemptCount := 0
	// Create mock VM server that returns client error
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			attemptCount++
			// Return client error (should not retry)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Bad request"})
		}
	}))
	defer mockVM.Close()

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	archivePath := createTestArchive(t)
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	ctx := context.Background()
	result, err := uploader.UploadDataset(ctx, archivePath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, attemptCount, "Should not retry on client error")
}

// TestUploader_UploadDataset_MaxRetries tests that max retries are respected
func TestUploader_UploadDataset_MaxRetries(t *testing.T) {
	attemptCount := 0
	// Create mock VM server that always fails
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/datasets/upload" && r.Method == "POST" {
			attemptCount++
			// Always return server error
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		}
	}))
	defer mockVM.Close()

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	archivePath := createTestArchive(t)
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	ctx := context.Background()
	result, err := uploader.UploadDataset(ctx, archivePath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	// Should retry 3 times (initial + 2 retries)
	assert.Equal(t, 3, attemptCount, "Should retry up to 3 times")
}

// TestUploader_UploadDataset_FileNotFound tests handling of missing archive file
func TestUploader_UploadDataset_FileNotFound(t *testing.T) {
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: "http://localhost:8080",
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	nonExistentPath := "/non/existent/path.tar.gz"
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	ctx := context.Background()
	result, err := uploader.UploadDataset(ctx, nonExistentPath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "not found")
}

// TestUploader_UploadDataset_NetworkError tests handling of network errors
func TestUploader_UploadDataset_NetworkError(t *testing.T) {
	// Use invalid endpoint to simulate network error
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: "http://localhost:99999",
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	archivePath := createTestArchive(t)
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := uploader.UploadDataset(ctx, archivePath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

// TestUploader_UploadDataset_ContextCancellation tests handling of context cancellation
func TestUploader_UploadDataset_ContextCancellation(t *testing.T) {
	// Create slow mock server
	mockVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockVM.Close()

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			WireGuard: config.WireGuardConfig{
				KVMEndpoint: mockVM.URL,
			},
		},
	}
	log := logger.NewNopLogger()
	uploader := NewUploader(cfg, log)

	archivePath := createTestArchive(t)
	cameraID := "test-camera-1"
	checksum := "test-checksum-123"

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := uploader.UploadDataset(ctx, archivePath, cameraID, checksum)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Should fail due to context timeout
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}
