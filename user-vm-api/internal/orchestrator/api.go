package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// APIServer provides HTTP REST API endpoints
type APIServer struct {
	config      *config.Config
	logger      *logging.Logger
	capStore    *tunnelgateway.CapabilityStore
	edgeAPIServer *tunnelgateway.EdgeAPIServer
	server      *http.Server
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
			ID:                      status.CameraID,
			Name:                    status.Name,
			Type:                    status.Type,
			Status:                  status.Status,
			Enabled:                 status.Enabled,
			LabeledSnapshotCount:    status.LabeledSnapshotCount,
			RequiredSnapshotCount:   status.RequiredSnapshotCount,
			SnapshotRequired:        status.SnapshotRequired,
			TrainingEligibilityStatus: string(status.TrainingEligibilityStatus),
			SyncedAt:                status.SyncedAt,
			UpdatedAt:               status.UpdatedAt,
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
	LabeledSnapshotCount      uint32   `json:"labeled_snapshot_count"`
	RequiredSnapshotCount      uint32   `json:"required_snapshot_count"`
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

