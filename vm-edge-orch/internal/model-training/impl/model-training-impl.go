package impl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	modeltraining "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/model-training"
)

type modelTrainingImpl struct {
	baseURL string
	client  *http.Client
	logger  *zap.Logger
}

// ModelTrainingConfig contains configuration for the model training service client.
type ModelTrainingConfig struct {
	BaseURL string
	Timeout time.Duration
}

// NewModelTraining creates a new model training service client implementation.
func NewModelTraining(cfg ModelTrainingConfig, logger *zap.Logger) (modeltraining.ModelTraining, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("model training service base URL is required")
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &modelTrainingImpl{
		baseURL: cfg.BaseURL,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}, nil
}

// StartTrainingJob starts a new training job with the provided configuration.
func (m *modelTrainingImpl) StartTrainingJob(ctx context.Context, req modeltraining.TrainingJobRequest) (*modeltraining.TrainingJobResponse, error) {
	if req.EdgeID == uuid.Nil || req.CameraID == uuid.Nil || req.DatasetID == uuid.Nil {
		return nil, fmt.Errorf("edge_id, camera_id, and dataset_id are required")
	}

	startReq := trainingStartRequest{
		EdgeID:          req.EdgeID,
		CameraID:        req.CameraID,
		DeviceID:        req.DeviceID,
		DatasetID:       req.DatasetID,
		BaselineModelID: req.BaselineModelID,
		TrainingConfig:  req.TrainingConfig,
	}

	payload, err := json.Marshal(startReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal training request: %w", err)
	}

	url := fmt.Sprintf("%s/api/training/start", m.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create training request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("training service request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read training response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.logger.Warn("Training service returned non-2xx",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return nil, fmt.Errorf("training service returned status %d: %s", resp.StatusCode, string(body))
	}

	var startResp trainingStartResponse
	if err := json.Unmarshal(body, &startResp); err != nil {
		return nil, fmt.Errorf("failed to parse training response: %w", err)
	}

	result := &modeltraining.TrainingJobResponse{
		JobID:               startResp.JobID,
		Status:              startResp.Status,
		BaselineModelID:     startResp.BaselineModelID,
		DatasetID:           startResp.DatasetID,
		StartedAt:           startResp.StartedAt,
		EstimatedCompletion: startResp.EstimatedCompletion,
	}

	m.logger.Info("Training job started",
		zap.String("job_id", result.JobID.String()),
		zap.String("edge_id", req.EdgeID.String()),
		zap.String("camera_id", req.CameraID.String()),
		zap.String("dataset_id", req.DatasetID.String()),
	)

	return result, nil
}

// GetTrainingJobStatus retrieves the current status of a training job.
func (m *modelTrainingImpl) GetTrainingJobStatus(ctx context.Context, jobID uuid.UUID) (*modeltraining.TrainingJobStatus, error) {
	url := fmt.Sprintf("%s/api/training/jobs/%s", m.baseURL, jobID.String())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("training service request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read status response: %w", readErr)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("training job %s not found", jobID)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.logger.Warn("Training service returned non-2xx for status request",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return nil, fmt.Errorf("training service returned status %d: %s", resp.StatusCode, string(body))
	}

	var statusResp trainingJobStatusResponse
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}

	result := &modeltraining.TrainingJobStatus{
		JobID:               statusResp.JobID,
		EdgeID:              statusResp.EdgeID,
		CameraID:            statusResp.CameraID,
		DeviceID:            statusResp.DeviceID,
		DatasetID:           statusResp.DatasetID,
		BaselineModelID:     statusResp.BaselineModelID,
		Status:              statusResp.Status,
		Progress:            statusResp.Progress,
		StartedAt:           statusResp.StartedAt,
		CompletedAt:         statusResp.CompletedAt,
		EstimatedCompletion: statusResp.EstimatedCompletion,
		Error:               statusResp.Error,
	}

	return result, nil
}

// CancelTrainingJob cancels a running training job.
func (m *modelTrainingImpl) CancelTrainingJob(ctx context.Context, jobID uuid.UUID) error {
	url := fmt.Sprintf("%s/api/training/jobs/%s/cancel", m.baseURL, jobID.String())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create cancel request: %w", err)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("training service request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read cancel response: %w", readErr)
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("training job %s not found", jobID)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.logger.Warn("Training service returned non-2xx for cancel request",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return fmt.Errorf("training service returned status %d: %s", resp.StatusCode, string(body))
	}

	m.logger.Info("Training job canceled",
		zap.String("job_id", jobID.String()),
	)

	return nil
}

// ListTrainingJobs lists all training jobs, optionally filtered by edge ID or device ID.
func (m *modelTrainingImpl) ListTrainingJobs(ctx context.Context, filter *modeltraining.TrainingJobFilter) ([]modeltraining.TrainingJobStatus, error) {
	listURL := fmt.Sprintf("%s/api/training/jobs", m.baseURL)
	if filter != nil {
		params := url.Values{}
		if filter.EdgeID != nil {
			params.Add("edge_id", filter.EdgeID.String())
		}
		if filter.DeviceID != nil {
			params.Add("device_id", filter.DeviceID.String())
		}
		if filter.Status != nil {
			params.Add("status", *filter.Status)
		}
		if len(params) > 0 {
			listURL += "?" + params.Encode()
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %w", err)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("training service request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("failed to read list response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.logger.Warn("Training service returned non-2xx for list request",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return nil, fmt.Errorf("training service returned status %d: %s", resp.StatusCode, string(body))
	}

	var listResp trainingJobListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse list response: %w", err)
	}

	result := make([]modeltraining.TrainingJobStatus, 0, len(listResp.Jobs))
	for _, job := range listResp.Jobs {
		result = append(result, modeltraining.TrainingJobStatus{
			JobID:               job.JobID,
			EdgeID:              job.EdgeID,
			CameraID:            job.CameraID,
			DeviceID:            job.DeviceID,
			DatasetID:           job.DatasetID,
			BaselineModelID:     job.BaselineModelID,
			Status:              job.Status,
			Progress:            job.Progress,
			StartedAt:           job.StartedAt,
			CompletedAt:         job.CompletedAt,
			EstimatedCompletion: job.EstimatedCompletion,
			Error:               job.Error,
		})
	}

	return result, nil
}

// Internal request/response types for JSON marshaling
type trainingStartRequest struct {
	EdgeID          uuid.UUID                     `json:"edge_id"`
	CameraID        uuid.UUID                     `json:"camera_id"`
	DeviceID        uuid.UUID                     `json:"device_id"`
	DatasetID       uuid.UUID                     `json:"dataset_id"`
	BaselineModelID uuid.UUID                     `json:"baseline_model_id"`
	TrainingConfig  *modeltraining.TrainingConfig `json:"training_config,omitempty"`
}

type trainingStartResponse struct {
	JobID               uuid.UUID `json:"job_id"`
	Status              string    `json:"status"`
	BaselineModelID     uuid.UUID `json:"baseline_model_id"`
	DatasetID           uuid.UUID `json:"dataset_id"`
	StartedAt           string    `json:"started_at"`
	EstimatedCompletion string    `json:"estimated_completion,omitempty"`
}

type trainingJobStatusResponse struct {
	JobID               uuid.UUID `json:"job_id"`
	EdgeID              uuid.UUID `json:"edge_id"`
	CameraID            uuid.UUID `json:"camera_id"`
	DeviceID            uuid.UUID `json:"device_id"`
	DatasetID           uuid.UUID `json:"dataset_id"`
	BaselineModelID     uuid.UUID `json:"baseline_model_id"`
	Status              string    `json:"status"`
	Progress            float64   `json:"progress,omitempty"`
	StartedAt           string    `json:"started_at"`
	CompletedAt         string    `json:"completed_at,omitempty"`
	EstimatedCompletion string    `json:"estimated_completion,omitempty"`
	Error               string    `json:"error,omitempty"`
}

type trainingJobListResponse struct {
	Jobs []trainingJobStatusResponse `json:"jobs"`
}
