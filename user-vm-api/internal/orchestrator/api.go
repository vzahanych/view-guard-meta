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
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
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
