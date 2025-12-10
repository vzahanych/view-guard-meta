package modeldeployment

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

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

const (
	// DefaultEdgeModelDeployEndpoint is the default Edge endpoint for model deployment
	DefaultEdgeModelDeployEndpoint = "/api/models/deploy"
	// DefaultTransferTimeout is the default timeout for model transfer (10 minutes for large models)
	DefaultTransferTimeout = 10 * time.Minute
	// MaxRetries is the maximum number of retry attempts
	MaxRetries = 3
	// BaseRetryDelay is the base delay between retries
	BaseRetryDelay = 2 * time.Second
	// MaxRetryDelay is the maximum delay between retries
	MaxRetryDelay = 30 * time.Second
)

// ModelTransferService handles transferring models to Edge over WireGuard tunnel
type ModelTransferService struct {
	modelStorage  *storage.ModelStorage
	modelCatalog  *modelcatalog.ModelCatalog
	tunnelGateway *tunnelgateway.EdgeAPIServer
	logger        *logging.Logger
	httpClient    *http.Client
}

// NewModelTransferService creates a new model transfer service
func NewModelTransferService(
	modelStorage *storage.ModelStorage,
	modelCatalog *modelcatalog.ModelCatalog,
	tunnelGateway *tunnelgateway.EdgeAPIServer,
	logger *logging.Logger,
) (*ModelTransferService, error) {
	if modelStorage == nil {
		return nil, fmt.Errorf("model storage is required")
	}
	if modelCatalog == nil {
		return nil, fmt.Errorf("model catalog is required")
	}
	if tunnelGateway == nil {
		return nil, fmt.Errorf("tunnel gateway is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create HTTP client with reasonable timeout for large model transfers
	httpClient := &http.Client{
		Timeout: DefaultTransferTimeout,
	}

	return &ModelTransferService{
		modelStorage:  modelStorage,
		modelCatalog:  modelCatalog,
		tunnelGateway: tunnelGateway,
		logger:        logger,
		httpClient:    httpClient,
	}, nil
}

// TransferResult contains the result of a model transfer
type TransferResult struct {
	Success      bool
	ModelFilePath *string // Path to model file on Edge (returned by Edge)
	ErrorMessage string
}

// TransferModel transfers a model to Edge over WireGuard tunnel
func (mts *ModelTransferService) TransferModel(ctx context.Context, deploymentID string, modelID string, edgeID string) (*TransferResult, error) {
	// Step 1: Check Edge connection status
	isConnected := mts.isEdgeConnected(edgeID)
	
	// If not connected, check if Edge is reconnecting and wait
	if !isConnected {
		connMonitor := mts.tunnelGateway.GetConnectionMonitor()
		if connMonitor != nil {
			stateInfo, exists := connMonitor.GetConnectionState(edgeID)
			if exists && stateInfo != nil && stateInfo.State == tunnelgateway.StateReconnecting {
				mts.logger.Info("Edge is reconnecting, waiting for connection",
					zap.String("edge_id", edgeID),
				)
				// Wait up to 30 seconds for connection
				if mts.waitForConnection(edgeID, 30*time.Second) {
					isConnected = true
				} else {
					mts.logger.Warn("Edge connection not available after waiting",
						zap.String("edge_id", edgeID),
					)
					return &TransferResult{
						Success:      false,
						ErrorMessage: "Edge is not connected and reconnection timed out",
					}, nil
				}
			} else {
				// Edge is disconnected, not reconnecting
				mts.logger.Warn("Edge is disconnected, deployment cannot proceed",
					zap.String("edge_id", edgeID),
				)
				return &TransferResult{
					Success:      false,
					ErrorMessage: "Edge is disconnected, cannot deploy model",
				}, nil
			}
		}
	}
	
	// Step 2: Get Edge's WireGuard IP address
	// Try WireGuard IP first, fall back to direct hostname if not available
	var edgeIP string
	var err error
	edgeIP, err = mts.getEdgeWireGuardIP(edgeID)
	if err != nil {
		// WireGuard IP not available - use direct hostname for docker network
		// In docker-compose, Edge orchestrator is accessible at edge-orchestrator:8081
		edgeIP = "edge-orchestrator"
		mts.logger.Info("Using Edge direct hostname (WireGuard IP not available)",
			zap.String("edge_id", edgeID),
			zap.String("edge_hostname", edgeIP),
			zap.Error(err),
		)
	}

	// Step 3: Get model file path
	modelPath := mts.modelStorage.GetModelFilePath(modelID)
	if !mts.modelStorage.ModelExists(modelID) {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Model file not found: %s", modelPath),
		}, nil
	}

	// Step 4: Get model metadata
	modelInfo, err := mts.modelStorage.GetModelInfo(modelID)
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to get model info: %v", err),
		}, nil
	}

	// Step 5: Get model entry from catalog
	modelEntry, err := mts.modelCatalog.GetModel(modelID)
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to get model entry: %v", err),
		}, nil
	}

	// Step 6: Transfer model with retry logic
	result, err := mts.transferWithRetry(ctx, edgeIP, edgeID, deploymentID, modelID, modelPath, modelInfo, modelEntry)
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Transfer failed: %v", err),
		}, nil
	}

	return result, nil
}

// isEdgeConnected checks if Edge is connected via WireGuard tunnel and gRPC
// Uses ConnectionMonitor for comprehensive state checking
func (mts *ModelTransferService) isEdgeConnected(edgeID string) bool {
	// Check ConnectionMonitor for comprehensive state
	connMonitor := mts.tunnelGateway.GetConnectionMonitor()
	if connMonitor != nil {
		stateInfo, exists := connMonitor.GetConnectionState(edgeID)
		if exists && stateInfo != nil {
			state := stateInfo.State
			// Connected - ready for deployment
			if state == tunnelgateway.StateConnected {
				return true
			}
			// Reconnecting - connection in progress, might succeed soon
			if state == tunnelgateway.StateReconnecting {
				mts.logger.Info("Edge is reconnecting, connection may be available soon",
					zap.String("edge_id", edgeID),
					zap.String("state", string(state)),
				)
				// Allow deployment to proceed - connection might be ready by the time transfer starts
				return true
			}
			// Stale - connection exists but not recent, still allow
			if state == tunnelgateway.StateStale {
				mts.logger.Info("Edge connection is stale, but allowing deployment",
					zap.String("edge_id", edgeID),
					zap.String("state", string(state)),
				)
				return true
			}
			// Disconnected - not ready for deployment
			if state == tunnelgateway.StateDisconnected {
				mts.logger.Warn("Edge is disconnected, deployment may fail",
					zap.String("edge_id", edgeID),
					zap.String("state", string(state)),
				)
				return false
			}
		}
	}

	// Fallback to legacy connection check for backward compatibility
	conn, exists := mts.tunnelGateway.GetConnection(edgeID)
	if exists && conn != nil {
		// Check if connection is recent (within last 5 minutes)
		if time.Since(conn.LastHeartbeat) <= 5*time.Minute {
			return true
		}
		// Connection exists but stale - still allow (Edge might reconnect)
		mts.logger.Info("Edge connection exists but stale, allowing deployment",
			zap.String("edge_id", edgeID),
		)
		return true
	}

	// No active connection - check if Edge is registered
	// For PoC mode, allow deployment even without active connection
	// In production, this should return false
	mts.logger.Info("Edge not in active connections, but allowing deployment (PoC mode - will connect on request)",
		zap.String("edge_id", edgeID),
	)
	return true // Allow deployment even without active connection (PoC mode)
}

// waitForConnection waits for Edge to become connected, with timeout
func (mts *ModelTransferService) waitForConnection(edgeID string, timeout time.Duration) bool {
	connMonitor := mts.tunnelGateway.GetConnectionMonitor()
	if connMonitor == nil {
		return false
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		stateInfo, exists := connMonitor.GetConnectionState(edgeID)
		if exists && stateInfo != nil {
			state := stateInfo.State
			if state == tunnelgateway.StateConnected || state == tunnelgateway.StateStale {
				mts.logger.Info("Edge connection available after waiting",
					zap.String("edge_id", edgeID),
					zap.String("state", string(state)),
				)
				return true
			}
		}

		select {
		case <-ticker.C:
			// Continue waiting
		case <-time.After(time.Until(deadline)):
			return false
		}
	}

	return false
}

// getEdgeWireGuardIP gets the WireGuard IP address for an Edge
func (mts *ModelTransferService) getEdgeWireGuardIP(edgeID string) (string, error) {
	// Get Edge connection from EdgeAPIServer
	_, exists := mts.tunnelGateway.GetConnection(edgeID)
	if !exists {
		return "", fmt.Errorf("Edge %s is not connected", edgeID)
	}

	// For PoC, we'll use a simple approach:
	// - Query the database for Edge's WireGuard public key
	// - Use WireGuard server to find peer by public key
	// - Extract allowed IPs from peer info
	// - Use the first allowed IP as the Edge's WireGuard IP
	
	// For now, we'll use a default WireGuard subnet pattern
	// In production, this would query the WireGuard server for the actual peer IP
	// The WireGuard server has peer information with AllowedIPs
	
	// TODO: Implement proper IP retrieval via WireGuardServer.GetPeer or similar
	// For PoC, we can use a configurable approach or WireGuard subnet pattern
	
	// For PoC, use a default WireGuard subnet (10.0.0.0/24)
	// The actual peer IP should be retrieved from WireGuard server
	// For now, we'll use a placeholder that indicates this needs WireGuard server integration
	// In production, this would:
	// 1. Get Edge's public key from connection or database
	// 2. Query WireGuard server for peer by public key
	// 3. Extract allowed IPs (e.g., 10.0.0.2/32)
	// 4. Return the IP address (10.0.0.2)
	
	// For PoC, return a placeholder IP that will be resolved by WireGuard routing
	// The actual implementation should query WireGuardServer for peer IP
	return "", fmt.Errorf("Edge WireGuard IP retrieval requires WireGuardServer.GetPeer implementation - placeholder for PoC")
}

// transferWithRetry attempts to transfer model with exponential backoff retry
func (mts *ModelTransferService) transferWithRetry(
	ctx context.Context,
	edgeIP string,
	edgeID string,
	deploymentID string,
	modelID string,
	modelPath string,
	modelInfo *storage.ModelInfo,
	modelEntry *modelcatalog.ModelEntry,
) (*TransferResult, error) {
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := BaseRetryDelay * time.Duration(1<<uint(attempt-1))
			if delay > MaxRetryDelay {
				delay = MaxRetryDelay
			}

			mts.logger.Info("Retrying model transfer",
				zap.String("edge_id", edgeID),
				zap.String("model_id", modelID),
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay),
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Verify Edge is still connected before retry
		if !mts.isEdgeConnected(edgeID) {
			mts.logger.Warn("Edge disconnected during transfer, waiting for reconnection",
				zap.String("edge_id", edgeID),
			)
			// Wait a bit for reconnection
			time.Sleep(5 * time.Second)
			if !mts.isEdgeConnected(edgeID) {
				return nil, fmt.Errorf("Edge %s is not connected", edgeID)
			}
		}

		// Attempt transfer
		result, err := mts.transferModel(ctx, edgeIP, edgeID, deploymentID, modelID, modelPath, modelInfo, modelEntry)
		if err == nil && result.Success {
			return result, nil
		}

		// Log error
		if err != nil {
			mts.logger.Warn("Model transfer attempt failed",
				zap.String("edge_id", edgeID),
				zap.String("model_id", modelID),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
		} else if !result.Success {
			mts.logger.Warn("Model transfer attempt failed",
				zap.String("edge_id", edgeID),
				zap.String("model_id", modelID),
				zap.Int("attempt", attempt+1),
				zap.String("error", result.ErrorMessage),
			)
		}

		// Check if error is retryable
		if err != nil && !isRetryableError(err) {
			return result, err
		}
	}

	return &TransferResult{
		Success:      false,
		ErrorMessage: "Transfer failed after maximum retries",
	}, nil
}

// transferModel performs a single model transfer attempt
func (mts *ModelTransferService) transferModel(
	ctx context.Context,
	edgeIP string,
	edgeID string,
	deploymentID string,
	modelID string,
	modelPath string,
	modelInfo *storage.ModelInfo,
	modelEntry *modelcatalog.ModelEntry,
) (*TransferResult, error) {
	// Create multipart form request
	req, err := mts.createMultipartRequest(ctx, edgeIP, edgeID, deploymentID, modelID, modelPath, modelInfo, modelEntry)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	mts.logger.Info("Transferring model to Edge",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("edge_ip", edgeIP),
		zap.String("model_path", modelPath),
	)

	// Send request
	resp, err := mts.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Edge returned status %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	// Parse response (Edge should return model file path)
	var response struct {
		Success      bool    `json:"success"`
		ModelFilePath *string `json:"model_file_path,omitempty"`
		Message      string  `json:"message,omitempty"`
		Error        string  `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		// If response is not JSON, assume success if status is OK
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
			return &TransferResult{
				Success: true,
			}, nil
		}
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to parse response: %v", err),
		}, nil
	}

	if !response.Success {
		return &TransferResult{
			Success:      false,
			ErrorMessage: response.Error,
		}, nil
	}

	mts.logger.Info("Model transferred successfully",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("model_file_path", *response.ModelFilePath),
	)

	return &TransferResult{
		Success:       true,
		ModelFilePath: response.ModelFilePath,
	}, nil
}

// createMultipartRequest creates a multipart form request for model transfer
func (mts *ModelTransferService) createMultipartRequest(
	ctx context.Context,
	edgeIP string,
	edgeID string,
	deploymentID string,
	modelID string,
	modelPath string,
	modelInfo *storage.ModelInfo,
	modelEntry *modelcatalog.ModelEntry,
) (*http.Request, error) {
	// Open model file
	file, err := os.Open(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open model file: %w", err)
	}
	defer file.Close()

	// File info not needed for multipart upload, but we could use it for logging
	// fileInfo, err := file.Stat()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get file info: %w", err)
	// }

	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add model file
	part, err := writer.CreateFormFile("model", filepath.Base(modelPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	// Copy file to form
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file to form: %w", err)
	}

	// Add metadata fields
	metadata := map[string]interface{}{
		"model_id":              modelID,
		"version":               modelEntry.Version,
		"model_type":            modelEntry.ModelType,
		"camera_id":             modelEntry.CameraID,
		"framework":             modelEntry.Framework,
		"training_dataset_id":   modelEntry.TrainingDatasetID,
		"training_date":         modelEntry.TrainingDate,
		"input_shape":           modelInfo.Metadata.InputShape,
		"preprocessing":         modelInfo.Metadata.Preprocessing,
	}

	// Add metadata JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := writer.WriteField("metadata", string(metadataJSON)); err != nil {
		return nil, fmt.Errorf("failed to write metadata field: %w", err)
	}

	// Add individual fields for convenience
	if err := writer.WriteField("model_id", modelID); err != nil {
		return nil, fmt.Errorf("failed to write model_id field: %w", err)
	}

	if err := writer.WriteField("version", modelEntry.Version); err != nil {
		return nil, fmt.Errorf("failed to write version field: %w", err)
	}

	if err := writer.WriteField("model_type", modelEntry.ModelType); err != nil {
		return nil, fmt.Errorf("failed to write model_type field: %w", err)
	}

	// Add deployment_id for tracking (Edge uses this to report status back)
	if deploymentID != "" {
		if err := writer.WriteField("deployment_id", deploymentID); err != nil {
			return nil, fmt.Errorf("failed to write deployment_id field: %w", err)
		}
	}

	// Close multipart writer
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	// Use Edge's WireGuard IP to send request over tunnel
	// For PoC, we'll construct the endpoint URL using the WireGuard subnet
	// In production, this would use the actual WireGuard peer IP from WireGuardServer
	// For now, we'll use a placeholder that will be resolved by WireGuard routing
	// The actual endpoint should be: http://<wireguard-peer-ip>:<edge-port>/api/models/deploy
	// For PoC, we can use the WireGuard interface and let routing handle it
	endpointURL := fmt.Sprintf("http://%s%s", edgeIP, DefaultEdgeModelDeployEndpoint)
	
	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Edge-ID", edgeID)

	return req, nil
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	// Network errors are retryable
	// 5xx server errors are retryable
	// 4xx client errors are not retryable
	// Context cancellation is not retryable
	
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Check for network errors (connection refused, timeout, etc.)
	// These are typically retryable
	return true
}

