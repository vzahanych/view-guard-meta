package modeldeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	modelcatalog "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultEdgeGRPCPort is the default Edge gRPC server port
	DefaultEdgeGRPCPort = 50052
	// DefaultTransferTimeout is the default timeout for model transfer (10 minutes for large models)
	DefaultTransferTimeout = 10 * time.Minute
	// MaxRetries is the maximum number of retry attempts
	MaxRetries = 3
	// BaseRetryDelay is the base delay between retries
	BaseRetryDelay = 2 * time.Second
	// MaxRetryDelay is the maximum delay between retries
	MaxRetryDelay = 30 * time.Second
	// ChunkSize is the size of each chunk for gRPC streaming (1MB chunks)
	ChunkSize = 1024 * 1024 // 1MB
)

// ModelTransferService handles transferring models to Edge over WireGuard tunnel via gRPC
type ModelTransferService struct {
	modelStorage  *storage.ModelStorage // For baseline models (on disk)
	minioStorage  *MinIOModelStorage    // For trained models (in MinIO only)
	modelCatalog  *modelcatalog.ModelCatalog
	tunnelGateway *tunnelgateway.EdgeAPIServer
	edgeClient    *tunnelgateway.EdgeClient // gRPC client for VM → Edge calls
	logger        *logging.Logger
}

// NewModelTransferService creates a new model transfer service
func NewModelTransferService(
	modelStorage *storage.ModelStorage, // For baseline models (on disk)
	minioStorage *MinIOModelStorage, // For trained models (in MinIO only) - can be nil
	modelCatalog *modelcatalog.ModelCatalog,
	tunnelGateway *tunnelgateway.EdgeAPIServer,
	edgeClient *tunnelgateway.EdgeClient, // gRPC client for VM → Edge calls
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
	if edgeClient == nil {
		return nil, fmt.Errorf("edge client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &ModelTransferService{
		modelStorage:  modelStorage,
		minioStorage:  minioStorage, // Can be nil if MinIO is not configured
		modelCatalog:  modelCatalog,
		tunnelGateway: tunnelGateway,
		edgeClient:    edgeClient,
		logger:        logger,
	}, nil
}

// TransferResult contains the result of a model transfer
type TransferResult struct {
	Success       bool
	ModelFilePath *string // Path to model file on Edge (returned by Edge)
	ErrorMessage  string
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

	// Step 2: Verify bidirectional gRPC connection is healthy and ready
	// This is critical for security applications - connection health must be monitored
	// The connection should already exist from earlier operations (Epic 2.4: RequestSnapshotCapture)
	// We verify it's still alive and ready before using it for model deployment
	mts.logger.Info("Verifying bidirectional gRPC connection health before model deployment",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("note", "Connection should already exist from earlier operations - verifying it's still alive"),
	)

	// Verify connection health using EdgeClient
	// Use a longer timeout to allow connection establishment if it doesn't exist yet
	// (though it should exist from Epic 2.4)
	healthCtx, healthCancel := context.WithTimeout(ctx, 30*time.Second)
	defer healthCancel()
	if err := mts.edgeClient.VerifyConnectionHealth(healthCtx, edgeID); err != nil {
		mts.logger.Error("Bidirectional gRPC connection not healthy - deployment cannot proceed",
			zap.String("edge_id", edgeID),
			zap.String("model_id", modelID),
			zap.Error(err),
			zap.String("note", "Connection health failure is a security concern - connection should be established and monitored continuously"),
		)
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("gRPC connection not healthy: %v (connection health is critical for security - connection should be established and monitored at each test step)", err),
		}, nil
	}

	mts.logger.Info("Bidirectional gRPC connection verified healthy - ready for model deployment",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("note", "Using existing bidirectional connection - extending with DeployModel action"),
	)

	// Step 4: Determine if this is a trained model or baseline model
	// Trained models are stored in MinIO only (not on disk)
	// Baseline models are stored on disk (can optionally be archived to MinIO for backup)
	modelEntry, err := mts.modelCatalog.GetModel(modelID)
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to get model entry: %v", err),
		}, nil
	}

	isTrainedModel := modelEntry.TrainingDatasetID != "" || (modelEntry.Metadata != nil && modelEntry.Metadata.TrainingDatasetID != "")

	var modelPath string
	var modelSize int64

	if isTrainedModel {
		// Trained models: Read from MinIO only
		if mts.minioStorage == nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: "MinIO storage not configured, cannot read trained model",
			}, nil
		}

		// Check if model exists in MinIO
		// Use background context with timeout for MinIO check to avoid cancellation
		minioCtx, minioCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer minioCancel()
		exists, err := mts.minioStorage.ModelExistsInMinIO(minioCtx, modelID)
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to check model in MinIO: %v", err),
			}, nil
		}
		if !exists {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Trained model %s not found in MinIO (trained models must be stored in MinIO)", modelID),
			}, nil
		}

		// Get model size from MinIO (use same context as existence check)
		modelSize, err = mts.minioStorage.GetModelSizeFromMinIO(minioCtx, modelID)
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to get model size from MinIO: %v", err),
			}, nil
		}

		// For trained models, modelPath will be set to temp file path in transferModel
		modelPath = "" // Will be set to temp file path in transferModel
		mts.logger.Info("Trained model found in MinIO, will download during gRPC transfer",
			zap.String("model_id", modelID),
			zap.Int64("size", modelSize),
		)
	} else {
		// Baseline models: Read from filesystem
		modelPath = mts.modelStorage.GetModelFilePath(modelID)
		if !mts.modelStorage.ModelExists(modelID) {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Baseline model file not found: %s", modelPath),
			}, nil
		}

		// Get file size before opening
		fileInfo, err := os.Stat(modelPath)
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to get model file info: %v", err),
			}, nil
		}
		modelSize = fileInfo.Size()

		// For baseline models, file will be opened in transferModel for each attempt
		mts.logger.Info("Baseline model found on filesystem, will read during gRPC transfer",
			zap.String("model_id", modelID),
			zap.String("path", modelPath),
			zap.Int64("size", modelSize),
		)
	}

	// Step 4: Get model metadata from catalog (metadata is stored in catalog, not separately)
	// Note: For trained models, we don't read metadata.json from MinIO - metadata is in catalog
	// For baseline models, metadata is also in catalog (not from filesystem metadata.json)

	// Step 5: Get model info (for metadata like input_shape, preprocessing)
	// Try to get from model storage first (for baseline models), fall back to catalog metadata
	var modelInfo *storage.ModelInfo
	if !isTrainedModel {
		// Baseline models: Try to get from model storage
		info, err := mts.modelStorage.GetModelInfo(modelID)
		if err == nil {
			modelInfo = info
		}
	}

	// If modelInfo is nil, construct from catalog entry metadata
	if modelInfo == nil {
		if modelEntry.Metadata != nil {
			// Construct ModelInfo from catalog entry
			modelInfo = &storage.ModelInfo{
				ModelID:   modelID,
				ModelPath: modelPath,
				SizeBytes: modelSize,
				Metadata: &storage.ModelMetadata{
					ModelID:   modelID,
					Version:   modelEntry.Version,
					ModelType: modelEntry.ModelType,
					CameraID:  modelEntry.CameraID,
					Framework: modelEntry.Framework,
					InputShape: func() []int {
						if modelEntry.Metadata != nil && len(modelEntry.Metadata.InputShape) > 0 {
							return modelEntry.Metadata.InputShape
						}
						return []int{}
					}(),
					Preprocessing: func() map[string]interface{} {
						if modelEntry.Metadata != nil && modelEntry.Metadata.Preprocessing != nil {
							return modelEntry.Metadata.Preprocessing
						}
						return make(map[string]interface{})
					}(),
					TrainingDatasetID: modelEntry.TrainingDatasetID,
					TrainingDate:      modelEntry.TrainingDate,
				},
			}
		} else {
			return &TransferResult{
				Success:      false,
				ErrorMessage: "Model metadata not available in catalog",
			}, nil
		}
	}

	// Step 7: Transfer model with retry logic via gRPC streaming
	result, err := mts.transferWithRetry(ctx, edgeID, deploymentID, modelID, modelPath, isTrainedModel, modelSize, modelInfo, modelEntry)
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

// transferWithRetry attempts to transfer model with exponential backoff retry via gRPC streaming
func (mts *ModelTransferService) transferWithRetry(
	ctx context.Context,
	edgeID string,
	deploymentID string,
	modelID string,
	modelPath string,
	isTrainedModel bool, // Whether this is a trained model (from MinIO) or baseline (from filesystem)
	modelSize int64, // Model file size
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
				zap.String("model_id", modelID),
				zap.Int("attempt", attempt+1),
			)
			// Wait for tunnel reconnection (handled by WireGuard service)
			// Wait up to 30 seconds for reconnection
			if mts.waitForConnection(edgeID, 30*time.Second) {
				mts.logger.Info("Edge reconnected, proceeding with retry",
					zap.String("edge_id", edgeID),
					zap.String("model_id", modelID),
					zap.Int("attempt", attempt+1),
				)
			} else {
				mts.logger.Error("Edge reconnection timeout, cannot retry",
					zap.String("edge_id", edgeID),
					zap.String("model_id", modelID),
					zap.Int("attempt", attempt+1),
				)
				return &TransferResult{
					Success:      false,
					ErrorMessage: fmt.Sprintf("Edge %s is not connected and reconnection timed out", edgeID),
				}, fmt.Errorf("Edge %s is not connected", edgeID)
			}
		}

		// Attempt transfer via gRPC streaming (transferModel will reopen file for each attempt)
		result, err := mts.transferModel(ctx, edgeID, deploymentID, modelID, modelPath, isTrainedModel, modelSize, modelInfo, modelEntry)
		if err == nil && result.Success {
			return result, nil
		}

		// Log error with detailed information
		if err != nil {
			// Check if it's a gRPC error for better logging
			grpcErr, isGRPC := status.FromError(err)
			if isGRPC {
				mts.logger.Warn("Model transfer attempt failed (gRPC error)",
					zap.String("edge_id", edgeID),
					zap.String("model_id", modelID),
					zap.Int("attempt", attempt+1),
					zap.String("grpc_code", grpcErr.Code().String()),
					zap.String("grpc_message", grpcErr.Message()),
					zap.Bool("retryable", isRetryableError(err)),
					zap.Error(err),
				)
			} else {
				mts.logger.Warn("Model transfer attempt failed",
					zap.String("edge_id", edgeID),
					zap.String("model_id", modelID),
					zap.Int("attempt", attempt+1),
					zap.Bool("retryable", isRetryableError(err)),
					zap.Error(err),
				)
			}
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
			mts.logger.Error("Non-retryable error encountered, stopping retries",
				zap.String("edge_id", edgeID),
				zap.String("model_id", modelID),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			return result, err
		}
	}

	return &TransferResult{
		Success:      false,
		ErrorMessage: "Transfer failed after maximum retries",
	}, nil
}

// transferModel performs a single model transfer attempt via gRPC streaming
func (mts *ModelTransferService) transferModel(
	ctx context.Context,
	edgeID string,
	deploymentID string,
	modelID string,
	modelPath string,
	isTrainedModel bool, // Whether this is a trained model (from MinIO) or baseline (from filesystem)
	modelSize int64, // Model file size
	modelInfo *storage.ModelInfo,
	modelEntry *modelcatalog.ModelEntry,
) (*TransferResult, error) {
	// Reuse existing gRPC connection via EdgeClient (bidirectional connection)
	// This extends the existing connection with a new remote action instead of creating a new connection
	mts.logger.Info("Using existing bidirectional gRPC connection for model deployment",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("note", "Reusing connection - extending with DeployModel action"),
	)

	// Use background context with timeout for connection (not tied to request context)
	// This ensures connection can be established even if the original context is canceled
	connCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use EdgeClient's connection pool to get or create connection
	// This reuses the same connection used by RequestSnapshotCapture and other VM → Edge calls
	conn, err := mts.edgeClient.GetOrCreateConnection(connCtx, edgeID)
	if err != nil {
		mts.logger.Error("Failed to get Edge gRPC connection for model deployment",
			zap.String("edge_id", edgeID),
			zap.String("model_id", modelID),
			zap.Error(err),
			zap.String("note", "Connection is reused from connection pool"),
		)
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to get Edge gRPC connection: %v", err),
		}, nil
	}
	// Note: Don't close connection - we reuse it from the connection pool

	// Create ControlService client
	client := edge.NewControlServiceClient(conn)

	// Create streaming context with timeout for large transfers
	streamCtx, streamCancel := context.WithTimeout(ctx, DefaultTransferTimeout)
	defer streamCancel()

	// Create gRPC stream for DeployModel (server-side streaming: VM streams to Edge)
	stream, err := client.DeployModel(streamCtx)
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to create DeployModel stream: %v", err),
		}, nil
	}

	// Prepare metadata for header
	preprocessingMap := make(map[string]string)
	if modelInfo.Metadata.Preprocessing != nil {
		for k, v := range modelInfo.Metadata.Preprocessing {
			// Convert value to string (JSON-encoded)
			valBytes, err := json.Marshal(v)
			if err == nil {
				preprocessingMap[k] = string(valBytes)
			}
		}
	}

	// Prepare full metadata JSON
	metadataJSON, err := json.Marshal(map[string]interface{}{
		"model_id":            modelID,
		"version":             modelEntry.Version,
		"model_type":          modelEntry.ModelType,
		"camera_id":           modelEntry.CameraID,
		"framework":           modelEntry.Framework,
		"training_dataset_id": modelEntry.TrainingDatasetID,
		"training_date":       modelEntry.TrainingDate,
		"input_shape":         modelInfo.Metadata.InputShape,
		"preprocessing":       modelInfo.Metadata.Preprocessing,
	})
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal metadata: %v", err),
		}, nil
	}

	// Send header chunk with metadata
	header := &edge.DeployModelChunk{
		Payload: &edge.DeployModelChunk_Header{
			Header: &edge.DeployModelHeader{
				DeploymentId: deploymentID,
				ModelId:      modelID,
				Version:      modelEntry.Version,
				ModelType:    modelEntry.ModelType,
				CameraId: func() string {
					if modelEntry.CameraID != "" {
						return modelEntry.CameraID
					}
					return ""
				}(),
				Framework:         modelEntry.Framework,
				TrainingDatasetId: modelEntry.TrainingDatasetID,
				TrainingDate:      modelEntry.TrainingDate,
				InputShape: func() []int32 {
					result := make([]int32, len(modelInfo.Metadata.InputShape))
					for i, v := range modelInfo.Metadata.InputShape {
						result[i] = int32(v)
					}
					return result
				}(),
				Preprocessing: preprocessingMap,
				MetadataJson:  string(metadataJSON),
				TotalSize:     uint64(modelSize),
			},
		},
		Offset: 0,
		Eof:    false,
	}

	if err := stream.Send(header); err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to send header: %v", err),
		}, nil
	}

	// Open model file for streaming
	var modelFile *os.File
	if isTrainedModel {
		// Trained model: Download from MinIO to temp file
		if mts.minioStorage == nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: "MinIO storage not configured, cannot read trained model",
			}, nil
		}

		tempFile, err := os.CreateTemp("", fmt.Sprintf("model-%s-*.onnx", modelID))
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to create temp file: %v", err),
			}, nil
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()

		reader, err := mts.minioStorage.GetModelFromMinIO(ctx, modelID)
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to download model from MinIO: %v", err),
			}, nil
		}
		defer reader.Close()

		_, err = io.Copy(tempFile, reader)
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to copy model from MinIO: %v", err),
			}, nil
		}

		modelFile, err = os.Open(tempFile.Name())
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to open temp model file: %v", err),
			}, nil
		}
		defer modelFile.Close()
	} else {
		// Baseline model: Open from filesystem
		if modelPath == "" {
			return &TransferResult{
				Success:      false,
				ErrorMessage: "Model path is empty for baseline model",
			}, nil
		}
		modelFile, err = os.Open(modelPath)
		if err != nil {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to open model file: %v", err),
			}, nil
		}
		defer modelFile.Close()
	}

	// Stream model file in chunks with progress logging
	buffer := make([]byte, ChunkSize)
	var offset int64 = 0
	var totalSent int64 = 0
	lastProgressLog := time.Now()
	progressLogInterval := 5 * time.Second // Log progress every 5 seconds

	for {
		n, err := modelFile.Read(buffer)
		if err != nil && err != io.EOF {
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to read model file: %v", err),
			}, nil
		}

		if n == 0 {
			break
		}

		// Check tunnel health during transfer (non-blocking check)
		if time.Since(lastProgressLog) >= progressLogInterval {
			if !mts.isEdgeConnected(edgeID) {
				mts.logger.Warn("Edge disconnected during transfer, will retry",
					zap.String("edge_id", edgeID),
					zap.String("model_id", modelID),
					zap.Int64("bytes_sent", totalSent),
					zap.Int64("total_size", modelSize),
				)
				return &TransferResult{
					Success:      false,
					ErrorMessage: "Edge disconnected during transfer",
				}, fmt.Errorf("Edge disconnected during transfer")
			}
			lastProgressLog = time.Now()
		}

		// Send data chunk
		chunk := &edge.DeployModelChunk{
			Payload: &edge.DeployModelChunk_Data{
				Data: buffer[:n],
			},
			Offset: offset,
			Eof:    err == io.EOF,
		}

		if err := stream.Send(chunk); err != nil {
			// Check if this is a gRPC error that might be retryable
			grpcErr, ok := status.FromError(err)
			if ok {
				mts.logger.Warn("gRPC error during chunk send",
					zap.String("edge_id", edgeID),
					zap.String("model_id", modelID),
					zap.String("code", grpcErr.Code().String()),
					zap.String("message", grpcErr.Message()),
					zap.Int64("bytes_sent", totalSent),
					zap.Int64("total_size", modelSize),
				)
			}
			return &TransferResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to send chunk at offset %d: %v", offset, err),
			}, err
		}

		offset += int64(n)
		totalSent += int64(n)

		// Log progress periodically
		if time.Since(lastProgressLog) >= progressLogInterval {
			progressPercent := float64(totalSent) / float64(modelSize) * 100
			mts.logger.Info("Model transfer progress",
				zap.String("edge_id", edgeID),
				zap.String("model_id", modelID),
				zap.Int64("bytes_sent", totalSent),
				zap.Int64("total_size", modelSize),
				zap.Float64("progress_percent", progressPercent),
			)
			lastProgressLog = time.Now()
		}

		if err == io.EOF {
			break
		}
	}

	mts.logger.Info("Model file streamed completely",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.Int64("total_bytes", totalSent),
	)

	// Close send stream and receive response
	resp, err := stream.CloseAndRecv()
	if err != nil {
		return &TransferResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to receive response: %v", err),
		}, nil
	}

	if !resp.Success {
		return &TransferResult{
			Success:      false,
			ErrorMessage: resp.ErrorMessage,
		}, nil
	}

	mts.logger.Info("Model transferred successfully via gRPC",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("model_file_path", resp.ModelFilePath),
	)

	return &TransferResult{
		Success:       true,
		ModelFilePath: &resp.ModelFilePath,
	}, nil
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	// Context cancellation is not retryable
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Check for gRPC status errors
	if grpcErr, ok := status.FromError(err); ok {
		code := grpcErr.Code()
		switch code {
		// Retryable gRPC errors
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted,
			codes.Internal, codes.Aborted, codes.Unknown:
			return true
		// Non-retryable gRPC errors (client errors)
		case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
			codes.PermissionDenied, codes.FailedPrecondition, codes.OutOfRange,
			codes.Unimplemented, codes.Unauthenticated:
			return false
		// Default: retry on other errors
		default:
			return true
		}
	}

	// Check for network errors (connection refused, timeout, etc.)
	// These are typically retryable
	// If error string contains common non-retryable patterns, don't retry
	errStr := strings.ToLower(err.Error())
	nonRetryablePatterns := []string{
		"invalid argument",
		"not found",
		"permission denied",
		"authentication failed",
		"unauthorized",
		"already exists",
		"failed precondition",
	}
	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// Default: retry on unknown errors (network issues, etc.)
	return true
}
