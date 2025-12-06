package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	modelcatalog "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// APIServer provides HTTP REST API endpoints
type APIServer struct {
	config          *config.Config
	logger          *logging.Logger
	capStore        *tunnelgateway.CapabilityStore
	edgeAPIServer   *tunnelgateway.EdgeAPIServer
	datasetReceiver DatasetReceiver
	modelCatalog    *modelcatalog.ModelCatalog
	modelStorage    *storage.ModelStorage
	server          *http.Server
}

// DatasetReceiver interface for receiving dataset uploads
type DatasetReceiver interface {
	ReceiveDataset(ctx context.Context, edgeID string, cameraID string, archivePath string, checksum string) (string, string, error)
}

// NewAPIServer creates a new API server
func NewAPIServer(cfg *config.Config, log *logging.Logger, capStore *tunnelgateway.CapabilityStore, edgeAPIServer *tunnelgateway.EdgeAPIServer) *APIServer {
	return &APIServer{
		config:        cfg,
		logger:        log,
		capStore:      capStore,
		edgeAPIServer: edgeAPIServer,
	}
}

// SetDatasetReceiver sets the dataset receiver service
func (s *APIServer) SetDatasetReceiver(receiver DatasetReceiver) {
	s.datasetReceiver = receiver
}

// SetModelCatalog sets the model catalog service
func (s *APIServer) SetModelCatalog(catalog *modelcatalog.ModelCatalog) {
	s.modelCatalog = catalog
}

// SetModelStorage sets the model storage service
func (s *APIServer) SetModelStorage(modelStorage *storage.ModelStorage) {
	s.modelStorage = modelStorage
}

// Name returns the service name
func (s *APIServer) Name() string {
	return "api-gateway"
}

// Start starts the HTTP API server
func (s *APIServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Camera endpoints
	mux.HandleFunc("/api/cameras", s.handleListCameras)
	mux.HandleFunc("/api/cameras/", s.handleGetCameraDataset)

	// Dataset upload endpoint
	mux.HandleFunc("/api/datasets/upload", s.handleDatasetUpload)

	// Model management endpoints
	// Register more specific routes first to avoid path matching issues
	mux.HandleFunc("/api/models/baseline", s.handleBaselineModels)
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/models/", s.handleModelByID)

	// Training service proxy endpoints
	// Register more specific routes first to avoid path matching issues
	mux.HandleFunc("/api/training/camera/", s.handleTrainingCamera)
	mux.HandleFunc("/api/training/start", s.handleTrainingStart)
	mux.HandleFunc("/api/training/", s.handleTrainingByID)
	mux.HandleFunc("/api/training", s.handleTraining)

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	port := 8080
	if s.config.UserVMAPI.APIGateway.Port > 0 {
		port = s.config.UserVMAPI.APIGateway.Port
	}

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.logger.Info("Starting API server", zap.Int("port", port))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the HTTP API server
func (s *APIServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping API server")
	return s.server.Shutdown(ctx)
}

// handleListCameras handles GET /api/cameras
func (s *APIServer) handleListCameras(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get edge_id from query parameter (for now, we'll get the first connected edge)
	// TODO: Support multiple edges and proper edge selection
	edgeID := r.URL.Query().Get("edge_id")
	if edgeID == "" {
		// Get first connected edge if available
		if s.edgeAPIServer != nil {
			connectedEdges := s.edgeAPIServer.GetConnectedEdges()
			if len(connectedEdges) > 0 {
				edgeID = connectedEdges[0]
			}
		}
		if edgeID == "" {
			http.Error(w, "No edge_id provided and no connected edges", http.StatusBadRequest)
			return
		}
	}

	statuses, err := s.capStore.ListCameraStatuses(r.Context(), edgeID)
	if err != nil {
		s.logger.Error("Failed to list camera statuses", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to list cameras: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	cameras := make([]CameraResponse, len(statuses))
	for i, status := range statuses {
		cameras[i] = CameraResponse{
			ID:                        status.CameraID,
			Name:                      status.Name,
			Type:                      status.Type,
			Status:                    status.Status,
			Enabled:                   status.Enabled,
			LabeledSnapshotCount:      status.LabeledSnapshotCount,
			RequiredSnapshotCount:     status.RequiredSnapshotCount,
			SnapshotRequired:          status.SnapshotRequired,
			TrainingEligibilityStatus: string(status.TrainingEligibilityStatus),
			SyncedAt:                  status.SyncedAt,
			UpdatedAt:                 status.UpdatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"cameras": cameras,
		"edge_id": edgeID,
	}); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// handleGetCameraDataset handles GET /api/cameras/{id}/dataset
func (s *APIServer) handleGetCameraDataset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract camera ID from path
	// Path format: /api/cameras/{id}/dataset
	cameraID := r.URL.Path[len("/api/cameras/"):]
	if idx := len(cameraID) - len("/dataset"); idx > 0 {
		cameraID = cameraID[:idx]
	}

	if cameraID == "" {
		http.Error(w, "Camera ID required", http.StatusBadRequest)
		return
	}

	// Get edge_id from query parameter
	edgeID := r.URL.Query().Get("edge_id")
	if edgeID == "" {
		// Get first connected edge if available
		if s.edgeAPIServer != nil {
			connectedEdges := s.edgeAPIServer.GetConnectedEdges()
			if len(connectedEdges) > 0 {
				edgeID = connectedEdges[0]
			}
		}
		if edgeID == "" {
			http.Error(w, "No edge_id provided and no connected edges", http.StatusBadRequest)
			return
		}
	}

	status, err := s.capStore.GetCameraStatus(r.Context(), edgeID, cameraID)
	if err != nil {
		s.logger.Error("Failed to get camera status", zap.String("camera_id", cameraID), zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get camera dataset: %v", err), http.StatusNotFound)
		return
	}

	// Convert to API response format
	datasetResponse := DatasetResponse{
		CameraID:                  status.CameraID,
		CameraName:                status.Name,
		LabeledSnapshotCount:      status.LabeledSnapshotCount,
		RequiredSnapshotCount:     status.RequiredSnapshotCount,
		SnapshotRequired:          status.SnapshotRequired,
		TrainingEligibilityStatus: string(status.TrainingEligibilityStatus),
		LabelCounts:               status.LabelCounts,
		SyncedAt:                  status.SyncedAt,
		UpdatedAt:                 status.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(datasetResponse); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// handleDatasetUpload handles POST /api/datasets/upload
func (s *APIServer) handleDatasetUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate Edge from request (extract edge_id from WireGuard peer IP)
	edgeID, err := s.authenticateEdgeFromRequest(r)
	if err != nil {
		s.logger.Warn("Dataset upload authentication failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Check if dataset receiver is available
	if s.datasetReceiver == nil {
		s.logger.Error("Dataset receiver not configured")
		http.Error(w, "Dataset receiver not available", http.StatusServiceUnavailable)
		return
	}

	// Parse multipart form (max 500MB for dataset archives)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		s.logger.Error("Failed to parse multipart form", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Get camera_id from form
	cameraID := r.FormValue("camera_id")
	if cameraID == "" {
		http.Error(w, "camera_id is required", http.StatusBadRequest)
		return
	}

	// Get checksum from form
	checksum := r.FormValue("checksum")

	// Get dataset file from form
	file, header, err := r.FormFile("dataset")
	if err != nil {
		s.logger.Error("Failed to get dataset file from form", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get dataset file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create temporary file for archive
	tmpDir := "/tmp"
	if s.config.UserVMAPI.Orchestrator.DataDir != "" {
		tmpDir = filepath.Join(s.config.UserVMAPI.Orchestrator.DataDir, "tmp")
		os.MkdirAll(tmpDir, 0755)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "dataset_upload_*.tar.gz")
	if err != nil {
		s.logger.Error("Failed to create temporary file", zap.Error(err))
		http.Error(w, "Failed to create temporary file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // Clean up temp file
	}()

	// Copy uploaded file to temporary location
	if _, err := io.Copy(tmpFile, file); err != nil {
		s.logger.Error("Failed to save uploaded file", zap.Error(err))
		http.Error(w, "Failed to save uploaded file", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	s.logger.Info("Dataset upload received",
		zap.String("edge_id", edgeID),
		zap.String("camera_id", cameraID),
		zap.String("filename", header.Filename),
		zap.Int64("size", header.Size),
		zap.String("checksum", checksum),
	)

	// Receive and process dataset
	datasetID, _, err := s.datasetReceiver.ReceiveDataset(r.Context(), edgeID, cameraID, tmpPath, checksum)
	if err != nil {
		s.logger.Error("Failed to receive dataset", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to receive dataset: %v", err), http.StatusInternalServerError)
		return
	}

	// Update training eligibility status
	if err := s.updateTrainingEligibility(r.Context(), edgeID, cameraID, datasetID); err != nil {
		s.logger.Error("Failed to update training eligibility", zap.Error(err))
		// Don't fail the upload, just log the error
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":    true,
		"dataset_id": datasetID,
		"edge_id":    edgeID,
		"camera_id":  cameraID,
		"message":    "Dataset uploaded and processed successfully",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// authenticateEdgeFromRequest authenticates an Edge from HTTP request
// For PoC, we extract edge_id from source IP (WireGuard peer) or header
func (s *APIServer) authenticateEdgeFromRequest(r *http.Request) (string, error) {
	// Try to get edge_id from X-Edge-ID header first (if Edge sends it)
	if edgeID := r.Header.Get("X-Edge-ID"); edgeID != "" {
		// Validate edge_id exists in database
		if s.edgeAPIServer != nil {
			connectedEdges := s.edgeAPIServer.GetConnectedEdges()
			for _, eid := range connectedEdges {
				if eid == edgeID {
					return edgeID, nil
				}
			}
		}
	}

	// Fallback: Extract from source IP (WireGuard peer)
	// This requires EdgeAPIServer to have access to WireGuard server
	if s.edgeAPIServer != nil {
		// Get source IP
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}

		// Use EdgeAPIServer's authentication logic (similar to gRPC)
		// For now, get first connected edge as fallback
		// TODO: Implement proper IP-based authentication similar to gRPC authInterceptor
		connectedEdges := s.edgeAPIServer.GetConnectedEdges()
		if len(connectedEdges) > 0 {
			s.logger.Info("Using first connected edge for dataset upload authentication",
				zap.String("edge_id", connectedEdges[0]),
				zap.String("remote_addr", r.RemoteAddr),
			)
			return connectedEdges[0], nil
		}
	}

	// PoC Fallback: For development/testing, use a default edge ID
	// This allows dataset uploads to work before Edge is fully registered via gRPC
	// TODO: Remove this fallback in production - require proper authentication
	defaultEdgeID := "poc-edge-1"
	s.logger.Info("Using default edge ID for PoC (no gRPC authentication yet)",
		zap.String("edge_id", defaultEdgeID),
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("user_agent", r.Header.Get("User-Agent")),
	)
	return defaultEdgeID, nil
}

// updateTrainingEligibility updates training eligibility status after dataset upload
func (s *APIServer) updateTrainingEligibility(ctx context.Context, edgeID string, cameraID string, datasetID string) error {
	if s.capStore == nil {
		return fmt.Errorf("capability store not available")
	}

	// Update training eligibility status to ready_for_training and link dataset_id
	return s.capStore.UpdateTrainingEligibility(ctx, edgeID, cameraID, datasetID, tunnelgateway.TrainingEligibilityReadyForTraining)
}

// handleHealth handles GET /health
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// CameraResponse represents a camera in the API response
type CameraResponse struct {
	ID                        string    `json:"id"`
	Name                      string    `json:"name"`
	Type                      string    `json:"type"`
	Status                    string    `json:"status"`
	Enabled                   bool      `json:"enabled"`
	LabeledSnapshotCount      uint32    `json:"labeled_snapshot_count"`
	RequiredSnapshotCount     uint32    `json:"required_snapshot_count"`
	SnapshotRequired          bool      `json:"snapshot_required"`
	TrainingEligibilityStatus string    `json:"training_eligibility_status"`
	SyncedAt                  time.Time `json:"synced_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// DatasetResponse represents dataset status for a camera
type DatasetResponse struct {
	CameraID                  string            `json:"camera_id"`
	CameraName                string            `json:"camera_name"`
	LabeledSnapshotCount      uint32            `json:"labeled_snapshot_count"`
	RequiredSnapshotCount     uint32            `json:"required_snapshot_count"`
	SnapshotRequired          bool              `json:"snapshot_required"`
	TrainingEligibilityStatus string            `json:"training_eligibility_status"`
	LabelCounts               map[string]uint32 `json:"label_counts"`
	SyncedAt                  time.Time         `json:"synced_at"`
	UpdatedAt                 time.Time         `json:"updated_at"`
}

// handleModels handles model management endpoints
// GET /api/models - List all models (with optional query parameters)
// POST /api/models - Upload new model
func (s *APIServer) handleModels(w http.ResponseWriter, r *http.Request) {
	if s.modelCatalog == nil || s.modelStorage == nil {
		http.Error(w, "Model catalog not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleListModels(w, r)
	case http.MethodPost:
		s.handleUploadModel(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListModels handles GET /api/models (with optional query parameters)
func (s *APIServer) handleListModels(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	cameraID := r.URL.Query().Get("camera_id")
	datasetID := r.URL.Query().Get("dataset_id")
	status := r.URL.Query().Get("status")

	var entries []*modelcatalog.ModelEntry
	var err error

	// Filter by query parameters
	if cameraID != "" {
		entries, err = s.modelCatalog.GetModelsByCamera(cameraID)
	} else if datasetID != "" {
		entries, err = s.modelCatalog.GetModelsByDataset(datasetID)
	} else if status != "" {
		entries, err = s.modelCatalog.GetModelsByStatus(modelcatalog.ModelStatus(status))
	} else {
		// List all models
		entries, err = s.modelCatalog.ListModels()
	}

	if err != nil {
		s.logger.Error("Failed to list models", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to list models: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(entries); err != nil {
		s.logger.Error("Failed to encode models response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// handleBaselineModels handles GET /api/models/baseline
func (s *APIServer) handleBaselineModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.modelCatalog == nil {
		http.Error(w, "Model catalog not available", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameter for model type
	modelType := r.URL.Query().Get("model_type")

	var entries []*modelcatalog.ModelEntry
	var err error

	if modelType != "" {
		entries, err = s.modelCatalog.GetBaselineModelsByType(modelType)
	} else {
		entries, err = s.modelCatalog.GetBaselineModels()
	}

	if err != nil {
		s.logger.Error("Failed to get baseline models", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get baseline models: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(entries); err != nil {
		s.logger.Error("Failed to encode baseline models response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// handleModelByID handles model operations by ID
// GET /api/models/{model_id} - Get model metadata
// GET /api/models/{model_id}/file - Download model file
// PUT /api/models/{model_id} - Update model metadata
// DELETE /api/models/{model_id} - Delete model
func (s *APIServer) handleModelByID(w http.ResponseWriter, r *http.Request) {
	if s.modelCatalog == nil || s.modelStorage == nil {
		http.Error(w, "Model catalog not available", http.StatusServiceUnavailable)
		return
	}

	// Extract model ID from path: /api/models/{model_id} or /api/models/{model_id}/file
	path := strings.TrimPrefix(r.URL.Path, "/api/models/")
	parts := strings.Split(path, "/")
	modelID := parts[0]

	if modelID == "" {
		http.Error(w, "Model ID is required", http.StatusBadRequest)
		return
	}

	// Check if this is a file download request
	if len(parts) > 1 && parts[1] == "file" {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleDownloadModelFile(w, r, modelID)
		return
	}

	// Handle other operations by method
	switch r.Method {
	case http.MethodGet:
		s.handleGetModel(w, r, modelID)
	case http.MethodPut:
		s.handleUpdateModel(w, r, modelID)
	case http.MethodDelete:
		s.handleDeleteModel(w, r, modelID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetModel handles GET /api/models/{model_id}
func (s *APIServer) handleGetModel(w http.ResponseWriter, r *http.Request, modelID string) {
	entry, err := s.modelCatalog.GetModel(modelID)
	if err != nil {
		s.logger.Error("Failed to get model", zap.String("model_id", modelID), zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get model: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(entry); err != nil {
		s.logger.Error("Failed to encode model response", zap.Error(err))
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// handleDownloadModelFile handles GET /api/models/{model_id}/file
func (s *APIServer) handleDownloadModelFile(w http.ResponseWriter, r *http.Request, modelID string) {
	// Read model file
	modelData, err := s.modelStorage.ReadModel(modelID)
	if err != nil {
		s.logger.Error("Failed to read model file", zap.String("model_id", modelID), zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to read model file: %v", err), http.StatusNotFound)
		return
	}

	// Get model metadata to determine file name
	metadata, err := s.modelStorage.GetMetadata(modelID)
	if err != nil {
		s.logger.Error("Failed to get model metadata", zap.String("model_id", modelID), zap.Error(err))
		http.Error(w, "Failed to get model metadata", http.StatusInternalServerError)
		return
	}

	// Set content type and headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", metadata.ONNXFile))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(modelData)))

	// Write model file
	if _, err := w.Write(modelData); err != nil {
		s.logger.Error("Failed to write model file", zap.Error(err))
		return
	}
}

// handleUploadModel handles POST /api/models (upload new model from training pipeline)
func (s *APIServer) handleUploadModel(w http.ResponseWriter, r *http.Request) {
	// Authenticate Edge from request (same as dataset upload)
	edgeID, err := s.authenticateEdgeFromRequest(r)
	if err != nil {
		s.logger.Warn("Model upload authentication failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Parse multipart form (max 50MB for model files)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.logger.Error("Failed to parse multipart form", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Get model_id from form (required)
	modelID := r.FormValue("model_id")
	if modelID == "" {
		http.Error(w, "model_id is required", http.StatusBadRequest)
		return
	}

	// Get metadata JSON from form
	metadataJSON := r.FormValue("metadata")
	if metadataJSON == "" {
		http.Error(w, "metadata is required", http.StatusBadRequest)
		return
	}

	// Parse metadata
	var metadata storage.ModelMetadata
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		s.logger.Error("Failed to parse metadata", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to parse metadata: %v", err), http.StatusBadRequest)
		return
	}

	// Get model file from form
	file, _, err := r.FormFile("model")
	if err != nil {
		s.logger.Error("Failed to get model file from form", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get model file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read model data
	modelData, err := io.ReadAll(file)
	if err != nil {
		s.logger.Error("Failed to read model file", zap.Error(err))
		http.Error(w, "Failed to read model file", http.StatusInternalServerError)
		return
	}

	s.logger.Info("Model upload received",
		zap.String("edge_id", edgeID),
		zap.String("model_id", modelID),
		zap.String("model_type", metadata.ModelType),
		zap.Int("size", len(modelData)),
	)

	// Store model
	if err := s.modelStorage.StoreModel(modelID, modelData, &metadata); err != nil {
		s.logger.Error("Failed to store model", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to store model: %v", err), http.StatusInternalServerError)
		return
	}

	// Register model in catalog
	if err := s.modelCatalog.RegisterModel(modelID, &metadata); err != nil {
		s.logger.Warn("Failed to register model in catalog", zap.Error(err))
		// Don't fail the upload, just log the warning
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model_id": modelID,
		"status":   "success",
		"message":  "Model uploaded successfully",
	})
}

// handleUpdateModel handles PUT /api/models/{model_id}
func (s *APIServer) handleUpdateModel(w http.ResponseWriter, r *http.Request, modelID string) {
	// Authenticate Edge from request
	_, err := s.authenticateEdgeFromRequest(r)
	if err != nil {
		s.logger.Warn("Model update authentication failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Parse request body
	var metadata storage.ModelMetadata
	if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
		s.logger.Error("Failed to decode metadata", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to decode metadata: %v", err), http.StatusBadRequest)
		return
	}

	// Update metadata
	if err := s.modelStorage.UpdateMetadata(modelID, &metadata); err != nil {
		s.logger.Error("Failed to update model metadata", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to update model metadata: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model_id": modelID,
		"status":   "success",
		"message":  "Model metadata updated successfully",
	})
}

// handleDeleteModel handles DELETE /api/models/{model_id}
func (s *APIServer) handleDeleteModel(w http.ResponseWriter, r *http.Request, modelID string) {
	// Authenticate Edge from request
	_, err := s.authenticateEdgeFromRequest(r)
	if err != nil {
		s.logger.Warn("Model delete authentication failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Delete model
	if err := s.modelStorage.DeleteModel(modelID); err != nil {
		s.logger.Error("Failed to delete model", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to delete model: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model_id": modelID,
		"status":   "success",
		"message":  "Model deleted successfully",
	})
}

// getTrainingServiceURL returns the training service URL with fallback logic
func (s *APIServer) getTrainingServiceURL() string {
	trainingServiceURL := s.config.UserVMAPI.APIGateway.TrainingServiceURL
	if trainingServiceURL == "" {
		// Check environment variable
		if envURL := os.Getenv("TRAINING_SERVICE_URL"); envURL != "" {
			trainingServiceURL = envURL
		} else {
			trainingServiceURL = "http://python-ai-service:8000"
		}
	}
	return trainingServiceURL
}

// checkTrainingServiceHealth checks if training service is available
func (s *APIServer) checkTrainingServiceHealth(ctx context.Context) error {
	trainingServiceURL := s.getTrainingServiceURL()

	healthURL := fmt.Sprintf("%s/health", trainingServiceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		s.logger.Error("Failed to create training service health check request",
			zap.String("url", healthURL),
			zap.Error(err))
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		// Check for specific error types
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				s.logger.Warn("Training service health check timeout",
					zap.String("url", healthURL),
					zap.Error(err))
				return fmt.Errorf("training service health check timeout: %w", err)
			}
			if netErr.Temporary() {
				s.logger.Warn("Training service health check temporary error",
					zap.String("url", healthURL),
					zap.Error(err))
				return fmt.Errorf("training service temporarily unavailable: %w", err)
			}
		}
		// Connection refused, DNS errors, etc.
		s.logger.Warn("Training service health check connection failed",
			zap.String("url", healthURL),
			zap.Error(err))
		return fmt.Errorf("training service connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Warn("Training service health check returned non-OK status",
			zap.String("url", healthURL),
			zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("training service health check returned status %d", resp.StatusCode)
	}

	return nil
}

// proxyTrainingRequest proxies a request to the training service
func (s *APIServer) proxyTrainingRequest(w http.ResponseWriter, r *http.Request, path string) {
	// Check training service health
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.checkTrainingServiceHealth(ctx); err != nil {
		s.logger.Warn("Training service health check failed",
			zap.String("training_service_url", s.getTrainingServiceURL()),
			zap.Error(err))
		http.Error(w, fmt.Sprintf("Training service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Authenticate request (same as other API endpoints)
	_, err := s.authenticateEdgeFromRequest(r)
	if err != nil {
		s.logger.Warn("Training request authentication failed", zap.Error(err))
		http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Build target URL
	trainingServiceURL := s.getTrainingServiceURL()
	targetURL := fmt.Sprintf("%s%s", trainingServiceURL, path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create request to training service
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		s.logger.Error("Failed to create proxy request", zap.Error(err))
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers (except Host)
	for key, values := range r.Header {
		if key != "Host" {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}

	// Set content type if body exists
	if r.Body != nil {
		req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	}

	// Make request to training service
	client := &http.Client{
		Timeout: 300 * time.Second, // Long timeout for training operations
	}

	resp, err := client.Do(req)
	if err != nil {
		// Check for specific error types
		var statusCode int
		var errorMsg string
		
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				s.logger.Error("Training service request timeout",
					zap.String("url", targetURL),
					zap.String("method", r.Method),
					zap.Error(err))
				statusCode = http.StatusGatewayTimeout
				errorMsg = "Training service request timeout"
			} else if netErr.Temporary() {
				s.logger.Error("Training service temporary error",
					zap.String("url", targetURL),
					zap.String("method", r.Method),
					zap.Error(err))
				statusCode = http.StatusServiceUnavailable
				errorMsg = "Training service temporarily unavailable"
			} else {
				// Connection refused, DNS errors, etc.
				s.logger.Error("Training service connection error",
					zap.String("url", targetURL),
					zap.String("method", r.Method),
					zap.String("training_service_url", trainingServiceURL),
					zap.Error(err))
				statusCode = http.StatusBadGateway
				errorMsg = "Training service connection failed"
			}
		} else {
			s.logger.Error("Failed to proxy request to training service",
				zap.String("url", targetURL),
				zap.String("method", r.Method),
				zap.Error(err))
			statusCode = http.StatusBadGateway
			errorMsg = "Training service error"
		}
		
		http.Error(w, fmt.Sprintf("%s: %v", errorMsg, err), statusCode)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		s.logger.Error("Failed to copy response body", zap.Error(err))
	}
}

// handleTraining handles /api/training (list endpoint)
func (s *APIServer) handleTraining(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path
	s.proxyTrainingRequest(w, r, path)
}

// handleTrainingStart handles POST /api/training/start
func (s *APIServer) handleTrainingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Path
	s.proxyTrainingRequest(w, r, path)
}

// handleTrainingByID handles /api/training/{job_id}
func (s *APIServer) handleTrainingByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	s.proxyTrainingRequest(w, r, path)
}

// handleTrainingCamera handles /api/training/camera/{camera_id}
func (s *APIServer) handleTrainingCamera(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	s.proxyTrainingRequest(w, r, path)
}
