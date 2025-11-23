package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// Client is an HTTP client for the Python AI service
type Client struct {
	serviceURL          string
	httpClient          *http.Client
	logger              *logger.Logger
	defaultConfidence   float64
	defaultEnabledClasses []string
}

// ClientConfig contains configuration for the AI client
type ClientConfig struct {
	ServiceURL          string
	Timeout            time.Duration
	ConfidenceThreshold float64
	EnabledClasses     []string
}

// NewClient creates a new AI service client
func NewClient(config ClientConfig, log *logger.Logger) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		serviceURL:          config.ServiceURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:              log,
		defaultConfidence:   config.ConfidenceThreshold,
		defaultEnabledClasses: config.EnabledClasses,
	}
}

// Infer performs inference on a single frame
func (c *Client) Infer(ctx context.Context, frame *video.Frame) (*InferenceResponse, error) {
	// Encode frame to base64
	imageBase64 := base64.StdEncoding.EncodeToString(frame.Data)

	// Create request
	req := InferenceRequest{
		Image: imageBase64,
	}

	// Add optional parameters if configured
	if c.defaultConfidence > 0 {
		req.ConfidenceThreshold = &c.defaultConfidence
	}
	if len(c.defaultEnabledClasses) > 0 {
		req.EnabledClasses = c.defaultEnabledClasses
	}

	return c.inferRequest(ctx, req)
}

// InferWithOptions performs inference with custom options
func (c *Client) InferWithOptions(
	ctx context.Context,
	frame *video.Frame,
	confidenceThreshold *float64,
	enabledClasses []string,
) (*InferenceResponse, error) {
	// Encode frame to base64
	imageBase64 := base64.StdEncoding.EncodeToString(frame.Data)

	// Create request
	req := InferenceRequest{
		Image: imageBase64,
	}

	// Use provided options or defaults
	if confidenceThreshold != nil {
		req.ConfidenceThreshold = confidenceThreshold
	} else if c.defaultConfidence > 0 {
		req.ConfidenceThreshold = &c.defaultConfidence
	}

	if len(enabledClasses) > 0 {
		req.EnabledClasses = enabledClasses
	} else if len(c.defaultEnabledClasses) > 0 {
		req.EnabledClasses = c.defaultEnabledClasses
	}

	return c.inferRequest(ctx, req)
}

// InferBatch performs batch inference on multiple frames
func (c *Client) InferBatch(ctx context.Context, frames []*video.Frame) (*BatchInferenceResponse, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames provided")
	}

	// Encode all frames to base64
	images := make([]string, len(frames))
	for i, frame := range frames {
		images[i] = base64.StdEncoding.EncodeToString(frame.Data)
	}

	// Create batch request
	req := BatchInferenceRequest{
		Images: images,
	}

	// Add optional parameters if configured
	if c.defaultConfidence > 0 {
		req.ConfidenceThreshold = &c.defaultConfidence
	}
	if len(c.defaultEnabledClasses) > 0 {
		req.EnabledClasses = c.defaultEnabledClasses
	}

	// Marshal request
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/inference/batch", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	c.logger.Debug("Sending batch inference request", "url", url, "frame_count", len(frames))
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var batchResp BatchInferenceResponse
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debug(
		"Batch inference completed",
		"frame_count", len(frames),
		"total_time_ms", batchResp.TotalInferenceTimeMs,
		"avg_time_ms", batchResp.AverageInferenceTimeMs,
	)

	return &batchResp, nil
}

// inferRequest performs a single inference request
func (c *Client) inferRequest(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	// Marshal request
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/inference", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	c.logger.Debug("Sending inference request", "url", url)
	startTime := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	requestDuration := time.Since(startTime)

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn(
			"AI service returned error",
			"status", resp.StatusCode,
			"response", string(body),
		)
		return nil, fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var inferenceResp InferenceResponse
	if err := json.Unmarshal(body, &inferenceResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debug(
		"Inference completed",
		"detection_count", inferenceResp.DetectionCount,
		"inference_time_ms", inferenceResp.InferenceTimeMs,
		"request_duration_ms", requestDuration.Milliseconds(),
	)

	return &inferenceResp, nil
}

// InferWithRetry performs inference with retry logic
func (c *Client) InferWithRetry(
	ctx context.Context,
	frame *video.Frame,
	maxRetries int,
	retryDelay time.Duration,
) (*InferenceResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Debug(
				"Retrying inference",
				"attempt", attempt,
				"max_retries", maxRetries,
			)

			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		resp, err := c.Infer(ctx, frame)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		c.logger.Warn(
			"Inference attempt failed",
			"attempt", attempt+1,
			"error", err,
		)
	}

	return nil, fmt.Errorf("inference failed after %d retries: %w", maxRetries, lastErr)
}

// GetStats retrieves inference statistics from the AI service
func (c *Client) GetStats(ctx context.Context) (*InferenceStats, error) {
	url := fmt.Sprintf("%s/api/v1/inference/stats", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var stats InferenceStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &stats, nil
}

// HealthCheck checks if the AI service is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health/ready", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AI service health check failed: status %d", resp.StatusCode)
	}

	return nil
}

// SetConfidenceThreshold updates the default confidence threshold
func (c *Client) SetConfidenceThreshold(threshold float64) {
	c.defaultConfidence = threshold
}

// SetEnabledClasses updates the default enabled classes
func (c *Client) SetEnabledClasses(classes []string) {
	c.defaultEnabledClasses = classes
}

