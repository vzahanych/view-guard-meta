package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/snapshot_request"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Server implements Edge-side gRPC server for VM to call Edge
type Server struct {
	*service.ServiceBase
	config          *config.WireGuardConfig
	logger          *logger.Logger
	grpcServer      *grpc.Server
	listener        net.Listener
	snapshotService *snapshot_request.Service
	modelStorage    ModelStorageService // Optional: for model deployment
	mu              sync.RWMutex
}

// NewServer creates a new gRPC server for Edge
func NewServer(
	cfg *config.WireGuardConfig,
	log *logger.Logger,
	snapshotService *snapshot_request.Service,
) *Server {
	return &Server{
		ServiceBase:     service.NewServiceBase("grpc-server", log),
		config:          cfg,
		logger:          log,
		snapshotService: snapshotService,
	}
}

// SetModelStorage sets the model storage service for model deployment
func (s *Server) SetModelStorage(storage ModelStorageService) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update the controlServiceServer with model storage
	if s.grpcServer != nil {
		// Server already started, need to recreate with new service
		s.logger.Warn("Cannot set model storage after server started")
		return
	}

	// Will be set when server starts
	s.modelStorage = storage
}

// Start starts the gRPC server
func (s *Server) Start(ctx context.Context) error {
	if !s.config.Enabled {
		s.LogInfo("gRPC server disabled (WireGuard disabled)")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.GetStatus().SetStatus(service.StatusStarting)

	// Listen on WireGuard interface (VM will connect to Edge's WireGuard IP)
	// Default port: 50052 (different from VM's 50051 to avoid conflicts)
	listenAddr := ":50052"

	s.LogInfo("Starting Edge gRPC server", "address", listenAddr)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		s.GetStatus().SetError(err)
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	// Load TLS credentials for mTLS (zero-trust security)
	var creds credentials.TransportCredentials
	serverCertPath := "/etc/ssl/certs/edge-server.crt"
	serverKeyPath := "/etc/ssl/private/edge-server.key"
	caCertPath := "/etc/ssl/certs/ca.crt"

	// Try to load TLS credentials (certificates should be mounted from Epic 2.0)
	tlsCreds, err := LoadServerCredentials(serverCertPath, serverKeyPath, caCertPath)
	if err != nil {
		s.LogError("Failed to load TLS credentials for gRPC server, using insecure (not recommended for production)", err,
			"server_cert", serverCertPath,
			"server_key", serverKeyPath,
			"ca_cert", caCertPath,
			"note", "TLS certificates should be generated in Epic 2.0 and mounted to containers")
		// Fall back to insecure for development/testing
		creds = nil
	} else {
		s.LogInfo("Loaded TLS credentials for gRPC server (mTLS enabled)",
			"server_cert", serverCertPath)
		creds = tlsCreds
	}

	// Create gRPC server with TLS
	opts := []grpc.ServerOption{
		// Add interceptors for auth/logging if needed
	}

	// Add TLS credentials if available
	if creds != nil {
		opts = append(opts, grpc.Creds(creds))
	}

	s.grpcServer = grpc.NewServer(opts...)

	// Register ControlService (for RequestSnapshotCapture and DeployModel)
	s.mu.RLock()
	modelStorage := s.modelStorage
	s.mu.RUnlock()

	edge.RegisterControlServiceServer(s.grpcServer, &controlServiceServer{
		snapshotService: s.snapshotService,
		modelStorage:    modelStorage,
		logger:          s.logger,
	})

	// Start server in goroutine
	go func() {
		// Log that server is about to start accepting connections
		s.LogInfo("Edge gRPC server accepting connections", "address", listenAddr)
		if err := s.grpcServer.Serve(listener); err != nil {
			s.LogError("gRPC server error", err)
		}
	}()

	// Give the server a moment to start accepting connections
	// This ensures the server is ready before we mark it as running
	time.Sleep(100 * time.Millisecond)

	s.GetStatus().SetStatus(service.StatusRunning)
	s.LogInfo("Edge gRPC server started", "address", listenAddr)

	return nil
}

// Stop stops the gRPC server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.GetStatus().SetStatus(service.StatusStopping)

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.grpcServer = nil
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}

	s.GetStatus().SetStatus(service.StatusStopped)
	s.LogInfo("Edge gRPC server stopped")

	return nil
}

// controlServiceServer implements ControlService for Edge
type controlServiceServer struct {
	edge.UnimplementedControlServiceServer
	snapshotService *snapshot_request.Service
	modelStorage    ModelStorageService // Optional: for model deployment
	logger          *logger.Logger
}

// ModelStorageService interface for model storage (matches web.ModelStorageService)
type ModelStorageService interface {
	StoreModel(ctx context.Context, modelID string, deploymentID *string, edgeID string, cameraID *string, modelData []byte, metadata *storage.ModelMetadata) (*storage.DeployedModel, error)
}

// RequestSnapshotCapture handles snapshot capture requests from VM
func (s *controlServiceServer) RequestSnapshotCapture(ctx context.Context, req *edge.RequestSnapshotCaptureRequest) (*edge.RequestSnapshotCaptureResponse, error) {
	if s.snapshotService == nil {
		return &edge.RequestSnapshotCaptureResponse{
			Accepted: false,
			Message:  "snapshot service not available",
		}, nil
	}

	return s.snapshotService.RequestSnapshotCapture(ctx, req)
}

// DeployModel handles model deployment requests from VM (server-side streaming: VM streams model to Edge)
func (s *controlServiceServer) DeployModel(stream edge.ControlService_DeployModelServer) error {
	if s.modelStorage == nil {
		return stream.SendAndClose(&edge.DeployModelResponse{
			Success:      false,
			ErrorMessage: "model storage service not available",
		})
	}

	ctx := stream.Context()
	var header *edge.DeployModelHeader
	var modelData []byte
	var totalReceived int64

	// Receive stream chunks
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stream.SendAndClose(&edge.DeployModelResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("failed to receive chunk: %v", err),
			})
		}

		// Handle header chunk
		if chunk.GetHeader() != nil {
			header = chunk.GetHeader()
			modelData = make([]byte, 0, header.TotalSize)
			s.logger.Info("Received model deployment header",
				"deployment_id", header.DeploymentId,
				"model_id", header.ModelId,
				"camera_id", header.CameraId,
				"total_size", header.TotalSize,
			)
			continue
		}

		// Handle data chunk
		if chunk.GetData() != nil {
			data := chunk.GetData()
			modelData = append(modelData, data...)
			totalReceived += int64(len(data))
		}

		if chunk.Eof {
			break
		}
	}

	// Validate we received header and complete model
	if header == nil {
		return stream.SendAndClose(&edge.DeployModelResponse{
			Success:      false,
			ErrorMessage: "model header not received",
		})
	}

	if int64(len(modelData)) != int64(header.TotalSize) {
		return stream.SendAndClose(&edge.DeployModelResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("model size mismatch: expected %d, received %d", header.TotalSize, len(modelData)),
		})
	}

	// Parse metadata from header
	var cameraID *string
	if header.CameraId != "" {
		cameraID = &header.CameraId
	}

	var deploymentID *string
	if header.DeploymentId != "" {
		deploymentID = &header.DeploymentId
	}

	// Parse metadata JSON
	var metadataMap map[string]interface{}
	if header.MetadataJson != "" {
		if err := json.Unmarshal([]byte(header.MetadataJson), &metadataMap); err != nil {
			s.logger.Warn("Failed to parse metadata JSON, using header fields",
				"error", err,
			)
		}
	}

	// Create ModelMetadata from header
	var trainingDatasetID *string
	if header.TrainingDatasetId != "" {
		trainingDatasetID = &header.TrainingDatasetId
	}

	var trainingDate *string
	if header.TrainingDate != "" {
		trainingDate = &header.TrainingDate
	}

	inputShape := make([]int, len(header.InputShape))
	for i, v := range header.InputShape {
		inputShape[i] = int(v)
	}

	preprocessing := make(map[string]interface{})
	for k, v := range header.Preprocessing {
		// Parse JSON-encoded values
		var val interface{}
		if err := json.Unmarshal([]byte(v), &val); err == nil {
			preprocessing[k] = val
		} else {
			preprocessing[k] = v // Fallback to string
		}
	}

	modelMetadata := &storage.ModelMetadata{
		ModelID:           header.ModelId,
		Version:           header.Version,
		ModelType:         header.ModelType,
		CameraID:          cameraID,
		Framework:         header.Framework,
		TrainingDatasetID: trainingDatasetID,
		TrainingDate:      trainingDate,
		InputShape:        inputShape,
		Preprocessing:     preprocessing,
	}

	// Store model using model storage service
	deployedModel, err := s.modelStorage.StoreModel(ctx, header.ModelId, deploymentID, "", cameraID, modelData, modelMetadata)
	if err != nil {
		s.logger.Error("Failed to store model",
			"error", err,
			"model_id", header.ModelId,
			"deployment_id", header.DeploymentId,
		)
		return stream.SendAndClose(&edge.DeployModelResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to store model: %v", err),
		})
	}

	// Extract model file path from deployed model
	modelFilePath := deployedModel.ModelPath

	s.logger.Info("Model deployed successfully via gRPC",
		"model_id", header.ModelId,
		"deployment_id", header.DeploymentId,
		"camera_id", cameraID,
		"model_path", modelFilePath,
	)

	return stream.SendAndClose(&edge.DeployModelResponse{
		Success:       true,
		ModelFilePath: modelFilePath,
		Message:       "Model deployed successfully",
	})
}
