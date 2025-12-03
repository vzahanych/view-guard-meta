package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// UploadResult contains the result of a dataset upload
type UploadResult struct {
	Success   bool
	DatasetID string // Returned by VM if successful
	Error     error
}

// Uploader handles uploading datasets to the VM
type Uploader struct {
	config     *config.Config
	logger     *logger.Logger
	httpClient *http.Client
	vmEndpoint string
}

// NewUploader creates a new dataset uploader
func NewUploader(cfg *config.Config, log *logger.Logger) *Uploader {
	// Create HTTP client with reasonable timeouts
	httpClient := &http.Client{
		Timeout: 10 * time.Minute, // Allow up to 10 minutes for large dataset uploads
	}

	// Determine VM endpoint from config
	vmEndpoint := cfg.Edge.WireGuard.KVMEndpoint
	if vmEndpoint == "" {
		// Default to localhost for PoC (when WireGuard is not configured)
		vmEndpoint = "http://localhost:8080"
	}

	// Ensure endpoint has http:// or https:// prefix
	if !hasProtocol(vmEndpoint) {
		vmEndpoint = "http://" + vmEndpoint
	}

	return &Uploader{
		config:     cfg,
		logger:     log,
		httpClient: httpClient,
		vmEndpoint: vmEndpoint,
	}
}

// UploadDataset uploads a dataset archive to the VM
// Returns the dataset ID if successful, or an error
func (u *Uploader) UploadDataset(ctx context.Context, archivePath string, cameraID string, checksum string, edgeID string) (*UploadResult, error) {
	// Verify archive file exists
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return &UploadResult{Success: false, Error: fmt.Errorf("archive file not found: %w", err)}, nil
	}
	fileSize := fileInfo.Size()

	u.logger.Info("Starting dataset upload",
		"archive_path", archivePath,
		"camera_id", cameraID,
		"file_size_bytes", fileSize,
		"checksum", checksum,
		"vm_endpoint", u.vmEndpoint,
	)

	// Execute upload with retry logic
	result, err := u.uploadWithRetry(ctx, archivePath, cameraID, checksum, edgeID, fileSize)
	if err != nil {
		return &UploadResult{Success: false, Error: err}, nil
	}

	return result, nil
}

// createMultipartRequest creates a new multipart form request for dataset upload
func (u *Uploader) createMultipartRequest(ctx context.Context, archivePath string, cameraID string, checksum string, edgeID string) (*http.Request, error) {
	// Open archive file
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add archive file
	part, err := writer.CreateFormFile("dataset", filepath.Base(archivePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	// Copy file to form
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file to form: %w", err)
	}

	// Add metadata fields
	if err := writer.WriteField("camera_id", cameraID); err != nil {
		return nil, fmt.Errorf("failed to write camera_id field: %w", err)
	}

	if err := writer.WriteField("checksum", checksum); err != nil {
		return nil, fmt.Errorf("failed to write checksum field: %w", err)
	}

	// Close multipart writer
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	uploadURL := u.vmEndpoint + "/api/datasets/upload"
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	// Add Edge ID header for authentication
	if edgeID != "" {
		req.Header.Set("X-Edge-ID", edgeID)
	}

	return req, nil
}

// uploadWithRetry attempts to upload with exponential backoff retry
// Retries on network errors and 5xx server errors, but not on 4xx client errors
func (u *Uploader) uploadWithRetry(ctx context.Context, archivePath string, cameraID string, checksum string, edgeID string, fileSize int64) (*UploadResult, error) {
	maxRetries := 3
	baseDelay := 2 * time.Second
	maxDelay := 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Log attempt
		if attempt > 0 {
			delay := u.calculateBackoffDelay(attempt, baseDelay, maxDelay)
			u.logger.Info("Retrying dataset upload",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"delay_seconds", delay.Seconds(),
				"camera_id", cameraID,
			)

			// Wait with exponential backoff
			select {
			case <-ctx.Done():
				return &UploadResult{Success: false, Error: ctx.Err()}, nil
			case <-time.After(delay):
				// Continue with retry
			}
		} else {
			u.logger.Info("Attempting dataset upload",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"camera_id", cameraID,
			)
		}

		// Create new request for this attempt (multipart body needs to be recreated)
		req, err := u.createMultipartRequest(ctx, archivePath, cameraID, checksum, edgeID)
		if err != nil {
			u.logger.Error("Failed to create upload request",
				"attempt", attempt+1,
				"error", err,
			)
			// Don't retry on request creation errors (likely file I/O issues)
			return &UploadResult{Success: false, Error: fmt.Errorf("failed to create request: %w", err)}, nil
		}

		// Execute request
		startTime := time.Now()
		resp, err := u.httpClient.Do(req)
		uploadDuration := time.Since(startTime)

		if err != nil {
			// Network error - retry
			u.logger.Warn("Dataset upload network error",
				"attempt", attempt+1,
				"error", err,
				"duration_seconds", uploadDuration.Seconds(),
			)

			// Check if context was cancelled
			if ctx.Err() != nil {
				return &UploadResult{Success: false, Error: ctx.Err()}, nil
			}

			// If this is the last attempt, return error
			if attempt == maxRetries-1 {
				return &UploadResult{Success: false, Error: fmt.Errorf("upload failed after %d attempts: %w", maxRetries, err)}, nil
			}

			// Continue to retry
			continue
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			u.logger.Warn("Failed to read response body",
				"attempt", attempt+1,
				"error", err,
			)

			if attempt == maxRetries-1 {
				return &UploadResult{Success: false, Error: fmt.Errorf("failed to read response: %w", err)}, nil
			}
			continue
		}

		// Check response status
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))

			// Don't retry on client errors (4xx) - these are likely permanent
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				u.logger.Error("Dataset upload failed (client error, not retrying)",
					"status_code", resp.StatusCode,
					"response_body", string(body),
					"attempt", attempt+1,
				)
				return &UploadResult{Success: false, Error: err}, nil
			}

			// Retry on server errors (5xx) and other errors
			u.logger.Warn("Dataset upload failed (server error, will retry)",
				"status_code", resp.StatusCode,
				"response_body", string(body),
				"attempt", attempt+1,
				"duration_seconds", uploadDuration.Seconds(),
			)

			if attempt == maxRetries-1 {
				return &UploadResult{Success: false, Error: err}, nil
			}
			continue
		}

		// Success! Parse JSON response to extract dataset ID
		var response struct {
			DatasetID string `json:"dataset_id"`
			Message   string `json:"message,omitempty"`
		}
		datasetID := ""
		if err := json.Unmarshal(body, &response); err == nil {
			datasetID = response.DatasetID
		} else {
			u.logger.Warn("Failed to parse dataset_id from response", "error", err, "response_body", string(body))
			// Try to extract from response body as fallback
			if len(body) > 0 {
				// Response might be plain text or different format
				datasetID = string(body)
			}
		}

		u.logger.Info("Dataset upload successful",
			"status_code", resp.StatusCode,
			"dataset_id", datasetID,
			"file_size_bytes", fileSize,
			"attempt", attempt+1,
			"duration_seconds", uploadDuration.Seconds(),
		)

		return &UploadResult{
			Success:   true,
			DatasetID: datasetID,
		}, nil
	}

	return &UploadResult{Success: false, Error: fmt.Errorf("upload failed after %d attempts", maxRetries)}, nil
}

// calculateBackoffDelay calculates exponential backoff delay with jitter
func (u *Uploader) calculateBackoffDelay(attempt int, baseDelay time.Duration, maxDelay time.Duration) time.Duration {
	// Exponential backoff: baseDelay * 2^(attempt-1)
	delay := baseDelay * time.Duration(1<<uint(attempt-1))

	// Cap at maxDelay
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// hasProtocol checks if a URL has a protocol prefix
func hasProtocol(url string) bool {
	return len(url) > 7 && (url[0:7] == "http://" || url[0:8] == "https://")
}
