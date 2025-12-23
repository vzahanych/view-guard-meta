package impl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	pb "github.com/vzahanych/view-guard-meta/proto/go/generated/edge_internal"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
)

// AIClient is an HTTP client for the Python AI service using protobuf JSON
type AIClient struct {
	serviceURL            string
	httpClient            *http.Client
	logger                *zap.Logger
	defaultConfidence     float64
	defaultEnabledClasses []string
	maxRetries            int
	retryDelay            time.Duration
}

// AIClientConfig contains configuration for the AI client
type AIClientConfig struct {
	ServiceURL          string
	Timeout             time.Duration
	ConfidenceThreshold float64
	EnabledClasses      []string
	MaxRetries          int           // Maximum number of retries for failed requests
	RetryDelay          time.Duration // Delay between retries
}

// NewAIClient creates a new AI service client
func NewAIClient(config AIClientConfig, log *zap.Logger) (*AIClient, error) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second // Default retry delay
	}

	// Ensure service URL has http:// or https:// prefix
	serviceURL := config.ServiceURL
	if serviceURL == "" {
		return nil, fmt.Errorf("service URL is required")
	}
	if len(serviceURL) < 7 || (serviceURL[:7] != "http://" && serviceURL[:8] != "https://") {
		// Default to http:// if no scheme provided
		serviceURL = "http://" + serviceURL
	}

	return &AIClient{
		serviceURL: serviceURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:                log,
		defaultConfidence:     config.ConfidenceThreshold,
		defaultEnabledClasses: config.EnabledClasses,
		maxRetries:            config.MaxRetries,
		retryDelay:            config.RetryDelay,
	}, nil
}

// Close closes the HTTP client (no-op for HTTP, but kept for interface compatibility)
func (c *AIClient) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}

// Infer performs inference on a single frame
func (c *AIClient) Infer(ctx context.Context, frame *cctvtypes.Frame) (*types.InferenceResponse, error) {
	// Encode frame to base64
	imageBase64 := base64.StdEncoding.EncodeToString(frame.Data)

	// Create proto request
	pbReq := &pb.InferenceRequest{
		Image: imageBase64,
	}

	// Add optional parameters if configured
	if c.defaultConfidence > 0 {
		conf := float32(c.defaultConfidence)
		pbReq.ConfidenceThreshold = &conf
	}
	if len(c.defaultEnabledClasses) > 0 {
		pbReq.EnabledClasses = c.defaultEnabledClasses
	}

	// Marshal protobuf to JSON
	jsonData, err := protojson.Marshal(pbReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request with retry logic
	url := fmt.Sprintf("%s/api/v1/inference", c.serviceURL)
	c.logger.Debug("Sending inference request", zap.String("url", url))
	startTime := time.Now()
	
	var resp *http.Response
	var body []byte
	var requestErr error
	
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Debug("Retrying inference request",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", c.maxRetries),
				zap.Duration("retry_delay", c.retryDelay))
			time.Sleep(c.retryDelay)
		}
		
		// Create HTTP request (recreate for retries since body may have been consumed)
		httpReq, createErr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if createErr != nil {
			return nil, fmt.Errorf("failed to create request: %w", createErr)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		
		resp, requestErr = c.httpClient.Do(httpReq)
		if requestErr != nil {
			if attempt < c.maxRetries {
				c.logger.Warn("Request failed, will retry", zap.Error(requestErr), zap.Int("attempt", attempt+1))
				continue
			}
			return nil, fmt.Errorf("failed to send request after %d attempts: %w", c.maxRetries+1, requestErr)
		}
		defer resp.Body.Close()
		
		// Read response
		body, requestErr = io.ReadAll(resp.Body)
		if requestErr != nil {
			if attempt < c.maxRetries {
				c.logger.Warn("Failed to read response, will retry", zap.Error(requestErr), zap.Int("attempt", attempt+1))
				continue
			}
			return nil, fmt.Errorf("failed to read response: %w", requestErr)
		}
		
		// Check status code
		if resp.StatusCode != http.StatusOK {
			if attempt < c.maxRetries && resp.StatusCode >= 500 {
				// Retry on server errors (5xx)
				c.logger.Warn("AI service returned server error, will retry",
					zap.Int("status", resp.StatusCode),
					zap.Int("attempt", attempt+1))
				continue
			}
			c.logger.Warn(
				"AI service returned error",
				zap.Int("status", resp.StatusCode),
				zap.String("response", string(body)),
			)
			return nil, fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(body))
		}
		
		// Success - break out of retry loop
		break
	}
	
	requestDuration := time.Since(startTime)

	// Unmarshal JSON to protobuf
	pbResp := &pb.InferenceResponse{}
	if err := protojson.Unmarshal(body, pbResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert proto response to internal type
	result := protoToInferenceResponse(pbResp)

	c.logger.Debug(
		"Inference completed",
		zap.Int("detection_count", result.DetectionCount),	
		zap.Float64("inference_time_ms", result.InferenceTimeMs),
		zap.Int64("request_duration_ms", requestDuration.Milliseconds()),
	)

	return result, nil
}

// InferWithOptions performs inference with custom options
func (c *AIClient) InferWithOptions(
	ctx context.Context,
	frame *cctvtypes.Frame,
	confidenceThreshold *float64,
	enabledClasses []string,
) (*types.InferenceResponse, error) {
	// Encode frame to base64
	imageBase64 := base64.StdEncoding.EncodeToString(frame.Data)

	// Create proto request
	pbReq := &pb.InferenceRequest{
		Image: imageBase64,
	}

	// Use provided options or defaults
	if confidenceThreshold != nil {
		conf := float32(*confidenceThreshold)
		pbReq.ConfidenceThreshold = &conf
	} else if c.defaultConfidence > 0 {
		conf := float32(c.defaultConfidence)
		pbReq.ConfidenceThreshold = &conf
	}

	if len(enabledClasses) > 0 {
		pbReq.EnabledClasses = enabledClasses
	} else if len(c.defaultEnabledClasses) > 0 {
		pbReq.EnabledClasses = c.defaultEnabledClasses
	}

	// Marshal protobuf to JSON
	jsonData, err := protojson.Marshal(pbReq)
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

	// Unmarshal JSON to protobuf
	pbResp := &pb.InferenceResponse{}
	if err := protojson.Unmarshal(body, pbResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert proto response to internal type
	return protoToInferenceResponse(pbResp), nil
}

// InferBatch performs batch inference on multiple frames
func (c *AIClient) InferBatch(ctx context.Context, frames []*cctvtypes.Frame) (*types.BatchInferenceResponse, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames provided")
	}

	// Encode all frames to base64
	images := make([]string, len(frames))
	for i, frame := range frames {
		images[i] = base64.StdEncoding.EncodeToString(frame.Data)
	}

	// Create proto batch request
	pbReq := &pb.BatchInferenceRequest{
		Images: images,
	}

	// Add optional parameters if configured
	if c.defaultConfidence > 0 {
		conf := float32(c.defaultConfidence)
		pbReq.ConfidenceThreshold = &conf
	}
	if len(c.defaultEnabledClasses) > 0 {
		pbReq.EnabledClasses = c.defaultEnabledClasses
	}

	// Marshal protobuf to JSON
	jsonData, err := protojson.Marshal(pbReq)
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
	c.logger.Debug("Sending batch inference request", zap.String("url", url), zap.Int("frame_count", len(frames)))
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

	// Unmarshal JSON to protobuf
	pbResp := &pb.BatchInferenceResponse{}
	if err := protojson.Unmarshal(body, pbResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert proto response to internal type
	batchResp := &types.BatchInferenceResponse{
		TotalInferenceTimeMs:   float64(pbResp.TotalInferenceTimeMs),
		AverageInferenceTimeMs: float64(pbResp.AverageInferenceTimeMs),
		Results:                make([]types.InferenceResponse, len(pbResp.Results)),
	}

	for i, result := range pbResp.Results {
		batchResp.Results[i] = *protoToInferenceResponse(result)
	}

	c.logger.Debug(
		"Batch inference completed",
		zap.Int("frame_count", len(frames)),
		zap.Float64("total_time_ms", batchResp.TotalInferenceTimeMs),
		zap.Float64("avg_time_ms", batchResp.AverageInferenceTimeMs),
	)

	return batchResp, nil
}

// protoToInferenceResponse converts a proto InferenceResponse to internal type
func protoToInferenceResponse(pbResp *pb.InferenceResponse) *types.InferenceResponse {
	boxes := make([]types.BoundingBox, len(pbResp.BoundingBoxes))
	for i, pbBox := range pbResp.BoundingBoxes {
		boxes[i] = types.BoundingBox{
			X1:         float64(pbBox.X1),
			Y1:         float64(pbBox.Y1),
			X2:         float64(pbBox.X2),
			Y2:         float64(pbBox.Y2),
			Confidence: float64(pbBox.Confidence),
			ClassID:    int(pbBox.ClassId),
			ClassName:  pbBox.ClassName,
		}
	}

	return &types.InferenceResponse{
		BoundingBoxes:   boxes,
		InferenceTimeMs: float64(pbResp.InferenceTimeMs),
		FrameShape:      convertInt32Slice(pbResp.FrameShape),
		ModelInputShape: convertInt32Slice(pbResp.ModelInputShape),
		DetectionCount:  int(pbResp.DetectionCount),
	}
}

// convertInt32Slice converts []int32 to []int
func convertInt32Slice(s []int32) []int {
	result := make([]int, len(s))
	for i, v := range s {
		result[i] = int(v)
	}
	return result
}

// InferWithRetry performs inference with retry logic
func (c *AIClient) InferWithRetry(
	ctx context.Context,
	frame *cctvtypes.Frame,
	maxRetries int,
	retryDelay time.Duration,
) (*types.InferenceResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Debug(
				"Retrying inference",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
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
			zap.Int("attempt", attempt+1),
			zap.Error(err),
		)
	}

	return nil, fmt.Errorf("inference failed after %d retries: %w", maxRetries, lastErr)
}

// GetStats retrieves inference statistics from the AI service
func (c *AIClient) GetStats(ctx context.Context) (*types.InferenceStats, error) {
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

	// Unmarshal JSON to protobuf
	pbResp := &pb.InferenceStats{}
	if err := protojson.Unmarshal(body, pbResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &types.InferenceStats{
		TotalInferences: int(pbResp.TotalInferences),
		TotalTimeMs:     float64(pbResp.TotalTimeMs),
		AverageTimeMs:   float64(pbResp.AverageTimeMs),
	}, nil
}

// HealthCheck checks if the AI service is healthy
func (c *AIClient) HealthCheck(ctx context.Context) error {
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
func (c *AIClient) SetConfidenceThreshold(threshold float64) {
	c.defaultConfidence = threshold
}

// SetEnabledClasses updates the default enabled classes
func (c *AIClient) SetEnabledClasses(classes []string) {
	c.defaultEnabledClasses = classes
}

// NotifyModelDeployment sends model metadata to AI service for loading from MinIO
func (c *AIClient) NotifyModelDeployment(ctx context.Context, metadata *types.ModelMetadata) error {
	// Marshal metadata to JSON
	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal model metadata: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/models/load", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	c.logger.Info("Notifying AI service about model deployment",
		zap.String("url", url),
		zap.String("model_id", metadata.ModelID),
		zap.String("camera_id", *metadata.CameraID),
	)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn(
			"AI service model deployment notification failed",
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Info("AI service notified about model deployment",
		zap.String("model_id", metadata.ModelID),
		zap.String("response", string(body)),
	)

	return nil
}
