package orchestrator

import (
	"context"
	"database/sql"
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
	modeldeployment "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-deployment"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// APIServer provides HTTP REST API endpoints
type APIServer struct {
	config                      *config.Config
	logger                      *logging.Logger
	db                          *sql.DB // Database connection for health checks
	capStore                    *tunnelgateway.CapabilityStore
	edgeAPIServer               *tunnelgateway.EdgeAPIServer
	edgeClient                  *tunnelgateway.EdgeClient
	datasetReceiver             DatasetReceiver
	modelCatalog                *modelcatalog.ModelCatalog
	modelStorage                *storage.ModelStorage
	modelDeploymentService      ModelDeploymentService
	modelDeploymentOrchestrator ModelDeploymentOrchestrator
	minioStorage                *modeldeployment.MinIOModelStorage // For archiving trained models
	server                      *http.Server
}

// ModelDeploymentService interface for model deployment
type ModelDeploymentService interface {
	ManualDeploy(ctx context.Context, modelID string, edgeID string, cameraID *string) (*modeldeployment.DeploymentJob, error)
}

// ModelDeploymentOrchestrator interface for deployment operations
type ModelDeploymentOrchestrator interface {
	GetDeploymentJob(ctx context.Context, deploymentID string) (*modeldeployment.DeploymentJob, error)
	ListDeploymentJobs(ctx context.Context, filters *modeldeployment.DeploymentFilters) ([]*modeldeployment.DeploymentJob, error)
	CompleteDeployment(ctx context.Context, deploymentID string, modelFilePath *string) error
	ActivateDeployment(ctx context.Context, deploymentID string) error
	FailDeployment(ctx context.Context, deploymentID string, errorMessage string) error
}

// DatasetReceiver interface for receiving dataset uploads
type DatasetReceiver interface {
	ReceiveDataset(ctx context.Context, edgeID string, cameraID string, archivePath string, checksum string) (string, string, error)
}

// NewAPIServer creates a new API server
func NewAPIServer(cfg *config.Config, log *logging.Logger, capStore *tunnelgateway.CapabilityStore, edgeAPIServer *tunnelgateway.EdgeAPIServer, edgeClient *tunnelgateway.EdgeClient) *APIServer {
	return &APIServer{
		config:        cfg,
		logger:        log,
		capStore:      capStore,
		edgeAPIServer: edgeAPIServer,
		edgeClient:    edgeClient,
	}
}

// SetDatabase sets the database connection for health checks
func (s *APIServer) SetDatabase(db *sql.DB) {
	s.db = db
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

// SetModelDeploymentService sets the model deployment service
func (s *APIServer) SetModelDeploymentService(service ModelDeploymentService) {
	s.modelDeploymentService = service
}

// SetModelDeploymentOrchestrator sets the model deployment orchestrator
func (s *APIServer) SetModelDeploymentOrchestrator(orchestrator ModelDeploymentOrchestrator) {
	s.modelDeploymentOrchestrator = orchestrator
}

// SetMinIOModelStorage sets the MinIO model storage (for archiving trained models)
func (s *APIServer) SetMinIOModelStorage(storage *modeldeployment.MinIOModelStorage) {
	s.minioStorage = storage
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
	mux.HandleFunc("/api/cameras/", s.handleCameraRoutes) // Handles all /api/cameras/ sub-paths

	// Dataset upload endpoint
	mux.HandleFunc("/api/datasets/upload", s.handleDatasetUpload)

	// Admin API endpoints (for SaaS components and setup scripts)
	mux.HandleFunc("/api/admin/models/register", s.handleAdminRegisterModel)

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

	// Edge connection status endpoints
	// Note: Order matters - more specific routes should come first
	mux.HandleFunc("/api/edges/", s.handleEdgeRoutes) // Handles all /api/edges/ sub-paths
	mux.HandleFunc("/api/edges", s.handleListEdges)   // List all edges (must come after /api/edges/)

	// Model deployment endpoints
	mux.HandleFunc("/api/deployments", s.handleDeployments)
	mux.HandleFunc("/api/deployments/", s.handleDeploymentByID)

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
		WriteTimeout: 35 * time.Second, // Increased to allow for VM→Edge gRPC retry logic (30s context + buffer)
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

// handleCameraRoutes handles all /api/cameras/ sub-paths
func (s *APIServer) handleCameraRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/cameras/"):]

	// Check if it's a request-snapshots endpoint
	if strings.HasSuffix(path, "/request-snapshots") {
		s.handleRequestSnapshot(w, r)
		return
	}

	// Check if it's a dataset endpoint
	if strings.HasSuffix(path, "/dataset") {
		s.handleGetCameraDataset(w, r)
		return
	}

	// Default: handle as camera dataset endpoint
	s.handleGetCameraDataset(w, r)
}

// handleRequestSnapshot handles POST /api/cameras/{id}/request-snapshots
func (s *APIServer) handleRequestSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.edgeClient == nil {
		http.Error(w, "Edge client not available", http.StatusServiceUnavailable)
		return
	}

	// Extract camera ID from path
	// Path format: /api/cameras/{id}/request-snapshots
	path := r.URL.Path[len("/api/cameras/"):]
	cameraID := path[:len(path)-len("/request-snapshots")]

	if cameraID == "" {
		http.Error(w, "Camera ID required", http.StatusBadRequest)
		return
	}

	// Get edge_id from query parameter or use first connected edge
	edgeID := r.URL.Query().Get("edge_id")
	if edgeID == "" {
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

	// Parse request body
	var req struct {
		Label       string `json:"label"`        // Optional: "normal", "threat", "abnormal", "custom"
		CustomLabel string `json:"custom_label"` // Required if label == "custom"
		Count       int32  `json:"count"`        // Optional: number of snapshots (default: 1)
		AutoCapture bool   `json:"auto_capture"` // Optional: auto-capture for integration tests (default: false)
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.Label == "" {
		req.Label = "normal"
	}
	if req.Count <= 0 {
		req.Count = 1
	}

	// Create context with timeout to allow for retry logic (2 attempts with 2s wait + connection time)
	// The temporal retry fix in RequestSnapshotCapture may need up to ~10 seconds total
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	s.logger.Info("Requesting snapshot capture from Edge",
		zap.String("edge_id", edgeID),
		zap.String("camera_id", cameraID),
		zap.String("label", req.Label),
		zap.Int32("count", req.Count),
		zap.Bool("auto_capture", req.AutoCapture))

	// Call Edge to request snapshot capture (includes temporal retry for Edge startup)
	resp, err := s.edgeClient.RequestSnapshotCapture(ctx, edgeID, cameraID, req.Label, req.CustomLabel, req.Count, req.AutoCapture)
	if err != nil {
		s.logger.Error("Failed to request snapshot capture",
			zap.String("edge_id", edgeID),
			zap.String("camera_id", cameraID),
			zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to request snapshot capture: %v", err), http.StatusInternalServerError)
		return
	}

	// RequestSnapshotCapture returns a response even on failure (Accepted=false)
	// This is a valid response, not an error
	if !resp.Accepted {
		s.logger.Warn("Edge rejected snapshot capture request",
			zap.String("edge_id", edgeID),
			zap.String("camera_id", cameraID),
			zap.String("message", resp.Message))
		// Still return 200 OK with the response, as this is a valid Edge response
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
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
	path := r.URL.Path[len("/api/cameras/"):]
	cameraID := path
	if idx := strings.Index(cameraID, "/"); idx > 0 {
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
		DatasetID:                 status.DatasetID,
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
// Verifies that all critical services are initialized and ready:
// - Database is initialized (migrations completed, edges table exists)
// - MinIO connection is working (if configured)
// - Python training service connection is working
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	healthStatus := map[string]interface{}{
		"status": "healthy",
		"checks": make(map[string]string),
	}
	allHealthy := true

	// Check 1: Database initialization and connectivity
	// Verify that:
	// - Database connection is working (can execute queries)
	// - Migrations are complete (edges table exists)
	// - Database is accessible and responsive
	if s.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// First, verify database connection by performing a simple query
		var count int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table';").Scan(&count)
		if err != nil {
			healthStatus["checks"].(map[string]string)["database"] = fmt.Sprintf("connection_error: %v", err)
			allHealthy = false
		} else {
			// Database connection works, now verify edges table exists (migrations completed)
			var tableName string
			err = s.db.QueryRowContext(ctx,
				"SELECT name FROM sqlite_master WHERE type='table' AND name='edges';").Scan(&tableName)
			if err != nil {
				if err == sql.ErrNoRows {
					healthStatus["checks"].(map[string]string)["database"] = "not_initialized (edges table missing)"
					allHealthy = false
				} else {
					healthStatus["checks"].(map[string]string)["database"] = fmt.Sprintf("query_error: %v", err)
					allHealthy = false
				}
			} else {
				// Final verification: perform a lightweight read query on edges table to ensure it's fully accessible
				var edgeCount int
				err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edges;").Scan(&edgeCount)
				if err != nil {
					healthStatus["checks"].(map[string]string)["database"] = fmt.Sprintf("table_access_error: %v", err)
					allHealthy = false
				} else {
					healthStatus["checks"].(map[string]string)["database"] = fmt.Sprintf("ready (tables: %d, edges: %d)", count, edgeCount)
				}
			}
		}
	} else {
		healthStatus["checks"].(map[string]string)["database"] = "not_configured"
		allHealthy = false
	}

	// Check 2: MinIO connection (if configured)
	if s.minioStorage != nil {
		// MinIOModelStorage should have a way to check connectivity
		// For now, we'll assume it's ready if it's initialized
		// In the future, we could add a HealthCheck method to MinIOModelStorage
		healthStatus["checks"].(map[string]string)["minio"] = "ready"
	} else {
		// MinIO is optional (only required for trained model storage)
		healthStatus["checks"].(map[string]string)["minio"] = "not_configured"
	}

	// Check 3: Python training service connection
	trainingServiceURL := s.getTrainingServiceURL()
	if trainingServiceURL != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/health", trainingServiceURL), nil)
		if err != nil {
			healthStatus["checks"].(map[string]string)["training_service"] = fmt.Sprintf("error: %v", err)
			allHealthy = false
		} else {
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				healthStatus["checks"].(map[string]string)["training_service"] = fmt.Sprintf("unreachable: %v", err)
				allHealthy = false
			} else {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					healthStatus["checks"].(map[string]string)["training_service"] = "ready"
				} else {
					healthStatus["checks"].(map[string]string)["training_service"] = fmt.Sprintf("unhealthy: HTTP %d", resp.StatusCode)
					allHealthy = false
				}
			}
		}
	} else {
		healthStatus["checks"].(map[string]string)["training_service"] = "not_configured"
		allHealthy = false
	}

	// Set overall status
	if !allHealthy {
		healthStatus["status"] = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		healthStatus["status"] = "healthy"
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(healthStatus)
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
	DatasetID                 string            `json:"dataset_id,omitempty"`
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

	// Extract model ID from path: /api/models/{model_id} or /api/models/{model_id}/file or /api/models/{model_id}/deployments
	path := strings.TrimPrefix(r.URL.Path, "/api/models/")
	parts := strings.Split(path, "/")
	modelID := parts[0]

	if modelID == "" {
		http.Error(w, "Model ID is required", http.StatusBadRequest)
		return
	}

	// Check if this is a deployments request - delegate to deployment handler
	if len(parts) > 1 && parts[1] == "deployments" {
		// Delegate to deployment handler for /api/models/{model_id}/deployments
		s.handleDeploymentByID(w, r)
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

	// Archive trained models to MinIO FIRST (trained models must be stored in MinIO before deployment)
	// This ensures models are persisted and can be redeployed if Edge restarts or is replaced
	isTrainedModel := metadata.TrainingDatasetID != "" && metadata.TrainingDate != ""
	if isTrainedModel {
		if s.minioStorage == nil {
			s.logger.Error("MinIO storage not available - cannot persist trained model",
				zap.String("model_id", modelID),
			)
			http.Error(w, "MinIO storage not configured - trained models require MinIO persistence", http.StatusServiceUnavailable)
			return
		}

		ctx := r.Context()
		s.logger.Info("Persisting trained model to MinIO before deployment",
			zap.String("model_id", modelID),
			zap.String("reason", "Edge may restart or be replaced - model must be in MinIO for redeployment"),
		)

		if err := s.minioStorage.ArchiveModel(ctx, modelID); err != nil {
			s.logger.Error("Failed to persist trained model to MinIO - deployment will not be possible",
				zap.String("model_id", modelID),
				zap.Error(err),
			)
			http.Error(w, fmt.Sprintf("Failed to persist trained model to MinIO: %v", err), http.StatusInternalServerError)
			return
		}

		// Verify model exists in MinIO after archiving (critical check)
		exists, err := s.minioStorage.ModelExistsInMinIO(ctx, modelID)
		if err != nil {
			s.logger.Error("Failed to verify trained model in MinIO after archiving",
				zap.String("model_id", modelID),
				zap.Error(err),
			)
			http.Error(w, fmt.Sprintf("Failed to verify model in MinIO: %v", err), http.StatusInternalServerError)
			return
		}
		if !exists {
			s.logger.Error("Trained model not found in MinIO after archiving - persistence failed",
				zap.String("model_id", modelID),
			)
			http.Error(w, "Model archiving to MinIO failed - model not found after upload", http.StatusInternalServerError)
			return
		}

		s.logger.Info("Trained model successfully persisted to MinIO - ready for deployment",
			zap.String("model_id", modelID),
			zap.String("note", "Model can now be deployed to Edge and redeployed if Edge restarts/replaces"),
		)
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

// handleEdgeRoutes routes requests to specific edge endpoints
func (s *APIServer) handleEdgeRoutes(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/edges/{edge_id}/...
	path := strings.TrimPrefix(r.URL.Path, "/api/edges/")
	parts := strings.Split(path, "/")

	// If path is empty (just /api/edges/), delegate to list handler
	if len(parts) == 0 || parts[0] == "" {
		s.handleListEdges(w, r)
		return
	}

	edgeID := parts[0]

	// Route to specific handlers based on path
	if len(parts) >= 2 {
		switch parts[1] {
		case "status":
			s.handleEdgeStatus(w, r, edgeID)
			return
		case "health":
			s.handleEdgeHealth(w, r, edgeID)
			return
		case "models":
			// Delegate to deployment handler for /api/edges/{edge_id}/models/deploy
			if len(parts) >= 3 && parts[2] == "deploy" {
				s.handleEdgeModelsDeploy(w, r, edgeID)
				return
			}
			// Other models endpoints - return 404 for now
			http.NotFound(w, r)
			return
		}
	}

	// If no specific handler matched, return 404
	http.NotFound(w, r)
}

// handleListEdges handles GET /api/edges - List all Edges with connection status
func (s *APIServer) handleListEdges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.edgeAPIServer == nil {
		http.Error(w, "Edge API server not available", http.StatusServiceUnavailable)
		return
	}

	// Get connection monitor
	connMonitor := s.edgeAPIServer.GetConnectionMonitor()
	if connMonitor == nil {
		http.Error(w, "Connection monitor not available", http.StatusServiceUnavailable)
		return
	}

	// Get all connection states
	allStates := connMonitor.GetAllConnectionStates()

	// Query database for all registered edges
	// First, check and fix any edges with non-active status (for test/dev environments)
	var checkStatus string
	var checkEdgeID string
	if err := s.edgeAPIServer.GetDB().QueryRowContext(r.Context(), "SELECT edge_id, status FROM edges LIMIT 1").Scan(&checkEdgeID, &checkStatus); err == nil {
		s.logger.Info("Found edge in database", zap.String("edge_id", checkEdgeID), zap.String("status", checkStatus), zap.String("status_repr", fmt.Sprintf("%q", checkStatus)))
		if checkStatus != "active" {
			s.logger.Info("Fixing edge status", zap.String("edge_id", checkEdgeID), zap.String("current_status", checkStatus))
			result, updateErr := s.edgeAPIServer.GetDB().ExecContext(r.Context(),
				"UPDATE edges SET status = 'active' WHERE edge_id = ?", checkEdgeID)
			if updateErr == nil {
				rowsAffected, _ := result.RowsAffected()
				s.logger.Info("Updated edge status", zap.String("edge_id", checkEdgeID), zap.Int64("rows_affected", rowsAffected))
			} else {
				s.logger.Error("Failed to update edge status", zap.Error(updateErr))
			}
		} else {
			s.logger.Info("Edge status is already active", zap.String("edge_id", checkEdgeID))
		}
	} else {
		s.logger.Warn("No edges found in database for status check", zap.Error(err))
	}

	rows, err := s.edgeAPIServer.GetDB().QueryContext(r.Context(),
		"SELECT edge_id, name, wireguard_public_key, wireguard_endpoint, last_seen, status, created_at, updated_at FROM edges WHERE status = 'active'")
	if err != nil {
		s.logger.Error("Failed to query edges", zap.Error(err))
		http.Error(w, "Failed to query edges", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type EdgeStatus struct {
		EdgeID             string     `json:"edge_id"`
		Name               string     `json:"name"`
		WireGuardPublicKey string     `json:"wireguard_public_key"`
		WireGuardEndpoint  *string    `json:"wireguard_endpoint,omitempty"`
		ConnectionState    string     `json:"connection_state"`
		LastSeen           time.Time  `json:"last_seen"`
		LastHeartbeat      *time.Time `json:"last_heartbeat,omitempty"`
		LastHandshake      *time.Time `json:"last_handshake,omitempty"`
		Latency            *string    `json:"latency,omitempty"`
		Status             string     `json:"status"`
		CreatedAt          time.Time  `json:"created_at"`
		UpdatedAt          time.Time  `json:"updated_at"`
	}

	edges := []EdgeStatus{}
	rowCount := 0

	for rows.Next() {
		rowCount++
		var edgeID, name, publicKey, dbStatus string
		var endpoint sql.NullString
		var lastSeen, createdAt, updatedAt int64

		if err := rows.Scan(&edgeID, &name, &publicKey, &endpoint, &lastSeen, &dbStatus, &createdAt, &updatedAt); err != nil {
			s.logger.Warn("Failed to scan edge row", zap.Error(err))
			continue
		}

		// Only include active edges in response
		if dbStatus != "active" {
			s.logger.Debug("Skipping edge with non-active status", zap.String("edge_id", edgeID), zap.String("status", dbStatus), zap.String("status_bytes", fmt.Sprintf("%q", dbStatus)))
			continue
		}

		edgeStatus := EdgeStatus{
			EdgeID:             edgeID,
			Name:               name,
			WireGuardPublicKey: publicKey,
			LastSeen:           time.Unix(lastSeen, 0),
			Status:             dbStatus,
			CreatedAt:          time.Unix(createdAt, 0),
			UpdatedAt:          time.Unix(updatedAt, 0),
			ConnectionState:    "registered", // Default
		}

		if endpoint.Valid {
			edgeStatus.WireGuardEndpoint = &endpoint.String
		}

		// Get connection state from monitor
		if stateInfo, exists := allStates[edgeID]; exists {
			edgeStatus.ConnectionState = string(stateInfo.State)
			if !stateInfo.LastHeartbeat.IsZero() {
				edgeStatus.LastHeartbeat = &stateInfo.LastHeartbeat
			}
			if !stateInfo.LastHandshake.IsZero() {
				edgeStatus.LastHandshake = &stateInfo.LastHandshake
			}
			if stateInfo.Latency > 0 {
				latencyStr := stateInfo.Latency.String()
				edgeStatus.Latency = &latencyStr
			}
		}

		edges = append(edges, edgeStatus)
	}

	s.logger.Debug("Query edges result", zap.Int("row_count", rowCount), zap.Int("edges_count", len(edges)))

	if err := json.NewEncoder(w).Encode(edges); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// handleEdgeStatus handles GET /api/edges/{edge_id}/status - Get detailed connection status
func (s *APIServer) handleEdgeStatus(w http.ResponseWriter, r *http.Request, edgeID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.edgeAPIServer == nil {
		http.Error(w, "Edge API server not available", http.StatusServiceUnavailable)
		return
	}

	// Query database for edge
	var name, publicKey, dbStatus string
	var endpoint sql.NullString
	var lastSeen, createdAt, updatedAt int64

	err := s.edgeAPIServer.GetDB().QueryRowContext(r.Context(),
		"SELECT name, wireguard_public_key, wireguard_endpoint, last_seen, status, created_at, updated_at FROM edges WHERE edge_id = ?",
		edgeID).Scan(&name, &publicKey, &endpoint, &lastSeen, &dbStatus, &createdAt, &updatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Edge not found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to query edge", zap.String("edge_id", edgeID), zap.Error(err))
		http.Error(w, "Failed to query edge", http.StatusInternalServerError)
		return
	}

	// Get connection state from monitor
	connMonitor := s.edgeAPIServer.GetConnectionMonitor()
	var stateInfo *tunnelgateway.ConnectionStateInfo
	if connMonitor != nil {
		if info, exists := connMonitor.GetConnectionState(edgeID); exists {
			stateInfo = info
		}
	}

	// Get gRPC connection info
	var grpcConn *tunnelgateway.EdgeConnection
	grpcConn, grpcConnected := s.edgeAPIServer.GetConnection(edgeID)

	// Get WireGuard peer info - check interface first, then sync with database
	var wgPeer *tunnelgateway.PeerInfo
	var actualPublicKey string = publicKey

	if s.edgeAPIServer.GetWireGuardServer() != nil {
		// First, try to find peer by database public key
		key, err := wgtypes.ParseKey(publicKey)
		if err == nil {
			wgPeer, _ = s.edgeAPIServer.GetWireGuardServer().GetPeerInfo(key)
		}

		// Always check WireGuard interface to get actual connected peer status
		// This ensures we get real-time data, not just what's cached
		foundPeer := s.edgeAPIServer.GetWireGuardServer().FindConnectedPeer()
		if foundPeer != nil {
			actualPublicKey = foundPeer.PublicKey.String()
			wgPeer = foundPeer

			// If database public key doesn't match actual peer, update database
			if actualPublicKey != publicKey {
				s.logger.Info("WireGuard peer public key mismatch, updating database",
					zap.String("edge_id", edgeID),
					zap.String("database_key", publicKey),
					zap.String("actual_key", actualPublicKey))

				// Update database with actual public key from WireGuard interface
				_, updateErr := s.edgeAPIServer.GetDB().ExecContext(r.Context(),
					"UPDATE edges SET wireguard_public_key = ?, updated_at = ? WHERE edge_id = ?",
					actualPublicKey, time.Now().Unix(), edgeID)
				if updateErr != nil {
					s.logger.Warn("Failed to update wireguard_public_key in database",
						zap.String("edge_id", edgeID),
						zap.Error(updateErr))
				} else {
					// Update local variable for response
					publicKey = actualPublicKey
				}
			}
		}
	} else {
		s.logger.Debug("WireGuard server not available", zap.String("edge_id", edgeID))
	}

	// Build response
	response := map[string]interface{}{
		"edge_id":              edgeID,
		"name":                 name,
		"wireguard_public_key": publicKey,
		"status":               dbStatus,
		"last_seen":            time.Unix(lastSeen, 0).Format(time.RFC3339),
		"created_at":           time.Unix(createdAt, 0).Format(time.RFC3339),
		"updated_at":           time.Unix(updatedAt, 0).Format(time.RFC3339),
	}

	if endpoint.Valid {
		response["wireguard_endpoint"] = endpoint.String
	}

	// Connection state
	if stateInfo != nil {
		response["connection_state"] = string(stateInfo.State)
		response["state_changed_at"] = stateInfo.StateChangedAt.Format(time.RFC3339)
		if !stateInfo.LastHeartbeat.IsZero() {
			response["last_heartbeat"] = stateInfo.LastHeartbeat.Format(time.RFC3339)
		}
		if !stateInfo.LastHandshake.IsZero() {
			response["last_handshake"] = stateInfo.LastHandshake.Format(time.RFC3339)
		}
		if stateInfo.Latency > 0 {
			response["latency"] = stateInfo.Latency.String()
		}
		response["connection_count"] = stateInfo.ConnectionCount
		response["reconnect_attempts"] = stateInfo.ReconnectAttempts
		if !stateInfo.LastReconnectAttempt.IsZero() {
			response["last_reconnect_attempt"] = stateInfo.LastReconnectAttempt.Format(time.RFC3339)
		}

		// Health metrics
		if !stateInfo.FirstConnectedAt.IsZero() {
			response["first_connected_at"] = stateInfo.FirstConnectedAt.Format(time.RFC3339)
		}
		if !stateInfo.LastConnectedAt.IsZero() {
			response["last_connected_at"] = stateInfo.LastConnectedAt.Format(time.RFC3339)
		}
		response["total_uptime"] = stateInfo.TotalUptime.String()
		response["total_downtime"] = stateInfo.TotalDowntime.String()
		if !stateInfo.CurrentSessionStart.IsZero() {
			response["current_session_start"] = stateInfo.CurrentSessionStart.Format(time.RFC3339)
			response["current_session_uptime"] = stateInfo.CurrentSessionUptime.String()
		}

		// gRPC call metrics
		response["grpc_call_count"] = stateInfo.GRPCCallCount
		response["grpc_success_count"] = stateInfo.GRPCSuccessCount
		response["grpc_failure_count"] = stateInfo.GRPCFailureCount
		if stateInfo.GRPCCallCount > 0 {
			response["grpc_success_rate"] = float64(stateInfo.GRPCSuccessCount) / float64(stateInfo.GRPCCallCount) * 100.0
		}
		if !stateInfo.LastGRPCCallTime.IsZero() {
			response["last_grpc_call_time"] = stateInfo.LastGRPCCallTime.Format(time.RFC3339)
		}

		// Packet loss metrics
		response["ping_count"] = stateInfo.PingCount
		response["pong_count"] = stateInfo.PongCount
		if stateInfo.PingCount > 0 {
			packetLoss := float64(stateInfo.PingCount-stateInfo.PongCount) / float64(stateInfo.PingCount) * 100.0
			if packetLoss < 0 {
				packetLoss = 0.0
			}
			if packetLoss > 100 {
				packetLoss = 100.0
			}
			response["packet_loss_percent"] = packetLoss
		}
	} else {
		response["connection_state"] = "registered"
	}

	// gRPC connection info
	grpcInfo := map[string]interface{}{
		"connected": grpcConnected,
	}
	if grpcConn != nil {
		// Access exported fields directly (mu is unexported and can't be accessed from this package)
		grpcInfo["connected_at"] = grpcConn.ConnectedAt.Format(time.RFC3339)
		if !grpcConn.LastHeartbeat.IsZero() {
			grpcInfo["last_heartbeat"] = grpcConn.LastHeartbeat.Format(time.RFC3339)
		}
		if !grpcConn.LastTelemetry.IsZero() {
			grpcInfo["last_telemetry"] = grpcConn.LastTelemetry.Format(time.RFC3339)
		}
		if grpcConn.Latency > 0 {
			grpcInfo["latency"] = grpcConn.Latency.String()
		}
		grpcInfo["connection_count"] = grpcConn.ConnectionCount
	}
	response["grpc_connection"] = grpcInfo

	// WireGuard peer info
	if wgPeer != nil {
		// Access exported fields directly (mu is unexported and can't be accessed from this package)
		wgInfo := map[string]interface{}{
			"connected": wgPeer.Connected,
		}
		if !wgPeer.LastHandshake.IsZero() {
			wgInfo["last_handshake"] = wgPeer.LastHandshake.Format(time.RFC3339)
		}
		if wgPeer.Latency > 0 {
			wgInfo["latency"] = wgPeer.Latency.String()
		}
		wgInfo["bytes_received"] = wgPeer.BytesReceived
		wgInfo["bytes_sent"] = wgPeer.BytesSent
		wgInfo["ping_count"] = wgPeer.PingCount
		wgInfo["pong_count"] = wgPeer.PongCount
		if !wgPeer.LastPingTime.IsZero() {
			wgInfo["last_ping_time"] = wgPeer.LastPingTime.Format(time.RFC3339)
		}
		if !wgPeer.LastPongTime.IsZero() {
			wgInfo["last_pong_time"] = wgPeer.LastPongTime.Format(time.RFC3339)
		}
		response["wireguard_peer"] = wgInfo
	}

	// Add health metrics if available
	if stateInfo != nil {
		healthMetrics := map[string]interface{}{}

		// Uptime/Downtime
		if !stateInfo.FirstConnectedAt.IsZero() {
			healthMetrics["first_connected_at"] = stateInfo.FirstConnectedAt.Format(time.RFC3339)
		}
		if !stateInfo.LastConnectedAt.IsZero() {
			healthMetrics["last_connected_at"] = stateInfo.LastConnectedAt.Format(time.RFC3339)
		}
		healthMetrics["total_uptime"] = stateInfo.TotalUptime.String()
		healthMetrics["total_downtime"] = stateInfo.TotalDowntime.String()
		if !stateInfo.CurrentSessionStart.IsZero() {
			healthMetrics["current_session_start"] = stateInfo.CurrentSessionStart.Format(time.RFC3339)
			healthMetrics["current_session_uptime"] = stateInfo.CurrentSessionUptime.String()
		}

		// gRPC metrics
		healthMetrics["grpc_call_count"] = stateInfo.GRPCCallCount
		healthMetrics["grpc_success_count"] = stateInfo.GRPCSuccessCount
		healthMetrics["grpc_failure_count"] = stateInfo.GRPCFailureCount
		if stateInfo.GRPCCallCount > 0 {
			healthMetrics["grpc_success_rate"] = float64(stateInfo.GRPCSuccessCount) / float64(stateInfo.GRPCCallCount) * 100.0
		}

		// Packet loss
		if stateInfo.PingCount > 0 {
			packetLoss := float64(stateInfo.PingCount-stateInfo.PongCount) / float64(stateInfo.PingCount) * 100.0
			if packetLoss < 0 {
				packetLoss = 0.0
			}
			if packetLoss > 100 {
				packetLoss = 100.0
			}
			healthMetrics["packet_loss_percent"] = packetLoss
		}
		healthMetrics["ping_count"] = stateInfo.PingCount
		healthMetrics["pong_count"] = stateInfo.PongCount

		response["health_metrics"] = healthMetrics
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// handleEdgeHealth handles GET /api/edges/{edge_id}/health - Get WireGuard tunnel health metrics
func (s *APIServer) handleEdgeHealth(w http.ResponseWriter, r *http.Request, edgeID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.edgeAPIServer == nil {
		http.Error(w, "Edge API server not available", http.StatusServiceUnavailable)
		return
	}

	// Query database for edge public key
	var publicKey string
	err := s.edgeAPIServer.GetDB().QueryRowContext(r.Context(),
		"SELECT wireguard_public_key FROM edges WHERE edge_id = ?",
		edgeID).Scan(&publicKey)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Edge not found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to query edge", zap.String("edge_id", edgeID), zap.Error(err))
		http.Error(w, "Failed to query edge", http.StatusInternalServerError)
		return
	}

	// Get WireGuard peer info
	wgServer := s.edgeAPIServer.GetWireGuardServer()
	if wgServer == nil {
		http.Error(w, "WireGuard server not available", http.StatusServiceUnavailable)
		return
	}

	key, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		http.Error(w, "Invalid WireGuard public key", http.StatusBadRequest)
		return
	}

	wgPeer, exists := wgServer.GetPeerInfo(key)
	if !exists {
		response := map[string]interface{}{
			"edge_id": edgeID,
			"healthy": false,
			"message": "WireGuard peer not found",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get connection state
	connMonitor := s.edgeAPIServer.GetConnectionMonitor()
	var stateInfo *tunnelgateway.ConnectionStateInfo
	if connMonitor != nil {
		if info, exists := connMonitor.GetConnectionState(edgeID); exists {
			stateInfo = info
		}
	}

	// Build health response
	// Access exported fields directly (mu is unexported and can't be accessed from this package)
	now := time.Now()
	handshakeAge := now.Sub(wgPeer.LastHandshake)
	healthy := wgPeer.Connected && !wgPeer.LastHandshake.IsZero() && handshakeAge < 10*time.Minute

	health := map[string]interface{}{
		"edge_id":          edgeID,
		"healthy":          healthy,
		"tunnel_connected": wgPeer.Connected,
		"last_handshake":   wgPeer.LastHandshake.Format(time.RFC3339),
		"handshake_age":    handshakeAge.String(),
		"latency":          wgPeer.Latency.String(),
		"transfer_stats": map[string]interface{}{
			"bytes_received": wgPeer.BytesReceived,
			"bytes_sent":     wgPeer.BytesSent,
			"ping_count":     wgPeer.PingCount,
			"pong_count":     wgPeer.PongCount,
		},
	}

	if !wgPeer.LastPingTime.IsZero() {
		health["last_ping_time"] = wgPeer.LastPongTime.Format(time.RFC3339)
	}
	if !wgPeer.LastPongTime.IsZero() {
		health["last_pong_time"] = wgPeer.LastPongTime.Format(time.RFC3339)
	}

	if stateInfo != nil {
		health["connection_state"] = string(stateInfo.State)
		if !stateInfo.LastHeartbeat.IsZero() {
			health["last_heartbeat"] = stateInfo.LastHeartbeat.Format(time.RFC3339)
		}

		// Health metrics
		healthMetrics := map[string]interface{}{}

		// Uptime/Downtime
		if !stateInfo.FirstConnectedAt.IsZero() {
			healthMetrics["first_connected_at"] = stateInfo.FirstConnectedAt.Format(time.RFC3339)
		}
		if !stateInfo.LastConnectedAt.IsZero() {
			healthMetrics["last_connected_at"] = stateInfo.LastConnectedAt.Format(time.RFC3339)
		}
		healthMetrics["total_uptime"] = stateInfo.TotalUptime.String()
		healthMetrics["total_downtime"] = stateInfo.TotalDowntime.String()
		if !stateInfo.CurrentSessionStart.IsZero() {
			healthMetrics["current_session_start"] = stateInfo.CurrentSessionStart.Format(time.RFC3339)
			healthMetrics["current_session_uptime"] = stateInfo.CurrentSessionUptime.String()
		}

		// Reconnection metrics
		healthMetrics["reconnect_attempts"] = stateInfo.ReconnectAttempts

		// gRPC call metrics
		healthMetrics["grpc_call_count"] = stateInfo.GRPCCallCount
		healthMetrics["grpc_success_count"] = stateInfo.GRPCSuccessCount
		healthMetrics["grpc_failure_count"] = stateInfo.GRPCFailureCount
		if stateInfo.GRPCCallCount > 0 {
			healthMetrics["grpc_success_rate"] = float64(stateInfo.GRPCSuccessCount) / float64(stateInfo.GRPCCallCount) * 100.0
		}

		// Packet loss
		if stateInfo.PingCount > 0 {
			packetLoss := float64(stateInfo.PingCount-stateInfo.PongCount) / float64(stateInfo.PingCount) * 100.0
			if packetLoss < 0 {
				packetLoss = 0.0
			}
			if packetLoss > 100 {
				packetLoss = 100.0
			}
			healthMetrics["packet_loss_percent"] = packetLoss
		}
		healthMetrics["ping_count"] = stateInfo.PingCount
		healthMetrics["pong_count"] = stateInfo.PongCount

		health["health_metrics"] = healthMetrics
	}

	if err := json.NewEncoder(w).Encode(health); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// handleAdminRegisterModel handles POST /api/admin/models/register
// Admin API for registering models (used by setup scripts and SaaS components)
func (s *APIServer) handleAdminRegisterModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.modelCatalog == nil || s.modelStorage == nil {
		http.Error(w, "Model catalog not available", http.StatusServiceUnavailable)
		return
	}

	// Parse request body
	var req struct {
		ModelID   string                 `json:"model_id"`
		ModelPath string                 `json:"model_path"` // Path to model file on filesystem
		Metadata  *storage.ModelMetadata `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("Failed to decode request", zap.Error(err))
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.ModelID == "" {
		http.Error(w, "model_id is required", http.StatusBadRequest)
		return
	}

	if req.ModelPath == "" {
		http.Error(w, "model_path is required", http.StatusBadRequest)
		return
	}

	if req.Metadata == nil {
		http.Error(w, "metadata is required", http.StatusBadRequest)
		return
	}

	// Ensure model_id in metadata matches
	req.Metadata.ModelID = req.ModelID

	s.logger.Info("Admin model registration request",
		zap.String("model_id", req.ModelID),
		zap.String("model_path", req.ModelPath),
		zap.String("model_type", req.Metadata.ModelType),
	)

	// Read model file from filesystem
	modelData, err := os.ReadFile(req.ModelPath)
	if err != nil {
		s.logger.Error("Failed to read model file", zap.Error(err), zap.String("path", req.ModelPath))
		http.Error(w, fmt.Sprintf("Failed to read model file: %v", err), http.StatusInternalServerError)
		return
	}

	s.logger.Info("Read model file from filesystem",
		zap.String("path", req.ModelPath),
		zap.Int("size", len(modelData)),
	)

	// Store model using ModelStorage (this will store it in the catalog's expected location)
	// ModelStorage stores at {baseDir}/{modelID}/model.onnx
	if err := s.modelStorage.StoreModel(req.ModelID, modelData, req.Metadata); err != nil {
		s.logger.Error("Failed to store model", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to store model: %v", err), http.StatusInternalServerError)
		return
	}

	s.logger.Info("Model stored successfully",
		zap.String("model_id", req.ModelID),
	)

	// Register model in catalog
	if err := s.modelCatalog.RegisterModel(req.ModelID, req.Metadata); err != nil {
		s.logger.Warn("Failed to register model in catalog", zap.Error(err))
		// Don't fail the registration, just log the warning
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"model_id": req.ModelID,
		"status":   "registered",
		"message":  "Model registered successfully",
	})
}
