package web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/snapshot_request"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// HTTPServer implements Edge-side HTTPS server for VM to call Edge
// All traffic routes through WireGuard tunnel (listens on 10.0.0.2:8443)
type HTTPServer struct {
	*service.ServiceBase
	config          *config.WireGuardConfig
	wgClient        interface { // WireGuard client for checking interface status
		GetInterfaceName() string
	}
	logger          *logger.Logger
	httpServer      *http.Server
	listener        net.Listener
	snapshotService *snapshot_request.Service
	modelStorage    ModelStorageService // Optional: for model deployment
	edgeID          string              // Edge ID for GetConfig
	mu              sync.RWMutex
}

// NewHTTPServer creates a new HTTPS server for Edge
func NewHTTPServer(
	cfg *config.WireGuardConfig,
	log *logger.Logger,
	snapshotService *snapshot_request.Service,
	edgeID string, // Edge ID for GetConfig
	wgClient interface { // WireGuard client for checking interface status
		GetInterfaceName() string
	},
) *HTTPServer {
	return &HTTPServer{
		ServiceBase:     service.NewServiceBase("https-server", log),
		config:          cfg,
		wgClient:        wgClient,
		logger:          log,
		snapshotService: snapshotService,
		edgeID:          edgeID,
	}
}

// SetModelStorage sets the model storage service for model deployment
func (s *HTTPServer) SetModelStorage(storage ModelStorageService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelStorage = storage
}

// Start starts the HTTPS server
func (s *HTTPServer) Start(ctx context.Context) error {
	if !s.config.Enabled {
		s.LogInfo("HTTPS server disabled (WireGuard disabled)")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.GetStatus().SetStatus(service.StatusStarting)

	// Listen on WireGuard interface IP only (VM will connect to Edge's WireGuard IP)
	// Port 8443 for HTTPS (replaces gRPC port 50052)
	// Binding to 10.0.0.2 ensures server is only accessible through WireGuard tunnel
	// Wait for WireGuard interface to be ready (interface must exist before binding)
	listenAddr := "10.0.0.2:8443"

	s.LogInfo("Starting Edge HTTPS server", "address", listenAddr)

	// Wait for WireGuard interface (10.0.0.2) to be available
	// Check WireGuard interface status using system commands
	maxWait := 30 * time.Second
	waitInterval := 500 * time.Millisecond
	waited := time.Duration(0)

	for waited < maxWait {
		// Check if WireGuard interface exists and has IP address assigned
		if s.isWireGuardInterfaceReady("10.0.0.2") {
			s.LogInfo("WireGuard interface is ready", "address", listenAddr)
			break
		}

		if waited > 0 {
			s.LogDebug("Waiting for WireGuard interface to be ready",
				"address", listenAddr,
				"waited", waited)
		}

		time.Sleep(waitInterval)
		waited += waitInterval
	}

	if waited >= maxWait {
		s.LogInfo("WireGuard interface not ready after waiting, attempting to bind anyway",
			"address", listenAddr,
			"waited", waited)
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		s.GetStatus().SetError(err)
		return fmt.Errorf("failed to create listener on %s (WireGuard interface may not be ready): %w", listenAddr, err)
	}
	s.listener = listener

	// Load TLS credentials for mTLS (zero-trust security)
	serverCertPath := "/etc/ssl/certs/edge-server.crt"
	serverKeyPath := "/etc/ssl/private/edge-server.key"
	caCertPath := "/etc/ssl/certs/ca.crt"

	cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		s.GetStatus().SetError(err)
		return fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Load CA certificate for client certificate verification
	caCert, err := loadCACertificate(caCertPath)
	if err != nil {
		s.GetStatus().SetError(err)
		return fmt.Errorf("failed to load CA certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS: require client certificate
		ClientCAs:    caCert,                         // x509.CertPool for client certificate verification
		MinVersion:   tls.VersionTLS12,
	}

	s.LogInfo("Loaded TLS credentials for HTTPS server (mTLS enabled)",
		"server_cert", serverCertPath)

	// Create HTTP mux and setup routes
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	// Create HTTPS server
	s.httpServer = &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		s.LogInfo("Edge HTTPS server listening and ready to accept connections", "address", listenAddr)
		// Use ServeTLS since we're providing TLS config
		if err := s.httpServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			s.LogError("HTTPS server error", err)
		}
	}()

	// Give the server a moment to start accepting connections
	time.Sleep(100 * time.Millisecond)

	s.GetStatus().SetStatus(service.StatusRunning)
	s.LogInfo("Edge HTTPS server started", "address", listenAddr)

	return nil
}

// Stop stops the HTTPS server
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.GetStatus().SetStatus(service.StatusStopping)

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.LogError("Error shutting down HTTPS server", err)
		}
		s.httpServer = nil
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}

	s.GetStatus().SetStatus(service.StatusStopped)
	s.LogInfo("Edge HTTPS server stopped")

	return nil
}

// isWireGuardInterfaceReady checks if the WireGuard interface exists and has the specified IP address
func (s *HTTPServer) isWireGuardInterfaceReady(expectedIP string) bool {
	// Method 1: Use WireGuard client's interface name if available
	if s.wgClient != nil {
		iface := s.wgClient.GetInterfaceName()
		if iface != "" {
			// Check if interface exists and has the IP address
			return s.checkInterfaceHasIP(iface, expectedIP)
		}
	}

	// Method 2: Use system commands to check interface status
	// Try 'ip addr show' to check if interface exists and has IP
	return s.checkInterfaceHasIP("wg0", expectedIP)
}

// checkInterfaceHasIP checks if a network interface exists and has the specified IP address
func (s *HTTPServer) checkInterfaceHasIP(iface, expectedIP string) bool {
	// Try 'ip addr show <interface>' command
	cmd := exec.Command("ip", "addr", "show", iface)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Interface doesn't exist or command failed
		return false
	}

	// Check if output contains the expected IP address
	outputStr := string(output)
	if strings.Contains(outputStr, expectedIP) {
		return true
	}

	// Also check if interface is UP
	if strings.Contains(outputStr, "state UP") {
		// Interface is up, check if IP is in the output (might be in CIDR format)
		// Expected IP is 10.0.0.2, check for 10.0.0.2/24 or similar
		if strings.Contains(outputStr, "10.0.0.2") {
			return true
		}
	}

	return false
}

// setupRoutes configures all REST API endpoints
func (s *HTTPServer) setupRoutes(mux *http.ServeMux) {
	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// API v1 endpoints
	mux.HandleFunc("/api/v1/config/get", s.handleGetConfig)
	mux.HandleFunc("/api/v1/config/update", s.handleUpdateConfig)
	mux.HandleFunc("/api/v1/snapshots/capture", s.handleRequestSnapshotCapture)
	mux.HandleFunc("/api/v1/models/deploy", s.handleDeployModel)
	mux.HandleFunc("/api/v1/services/restart", s.handleRestartService)
	mux.HandleFunc("/api/v1/capabilities/sync", s.handleSyncCapabilities)
}

// handleHealth handles health check requests
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "edge-https-server",
	})
}

// handleGetConfig handles GetConfig requests
func (s *HTTPServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Build configuration JSON with Edge state information
	config := map[string]interface{}{
		"edge_id": s.edgeID,
		"services": map[string]interface{}{
			"edge_ai_service": map[string]interface{}{
				"status": "healthy", // TODO: Query actual health from edge-ai-service
			},
			"edge_orchestrator": map[string]interface{}{
				"status": "running",
			},
		},
		"wireguard": map[string]interface{}{
			"enabled":   true,
			"interface": "wg0",
		},
		"timestamp": time.Now().Unix(),
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		s.logger.Error("Failed to marshal config", "error", err)
		s.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal config: %v", err))
		return
	}

	s.sendSuccessResponse(w, map[string]interface{}{
		"config_json": string(configJSON),
	})
}

// handleUpdateConfig handles UpdateConfig requests
func (s *HTTPServer) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ConfigJSON string `json:"config_json"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// TODO: Implement config update logic
	s.sendSuccessResponse(w, map[string]interface{}{
		"message": "Config update not yet implemented",
	})
}

// handleRequestSnapshotCapture handles snapshot capture requests
func (s *HTTPServer) handleRequestSnapshotCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CameraID    string `json:"camera_id"`
		Label       string `json:"label"`
		CustomLabel string `json:"custom_label"`
		Count       int32  `json:"count"`
		AutoCapture bool   `json:"auto_capture"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if s.snapshotService == nil {
		s.sendErrorResponse(w, http.StatusServiceUnavailable, "snapshot service not available")
		return
	}

	// Convert to proto request format
	ctx := r.Context()
	protoReq := &edge.RequestSnapshotCaptureRequest{
		CameraId:    req.CameraID,
		Label:       req.Label,
		CustomLabel: req.CustomLabel,
		Count:       req.Count,
		AutoCapture: req.AutoCapture,
	}

	protoResp, err := s.snapshotService.RequestSnapshotCapture(ctx, protoReq)
	if err != nil {
		s.logger.Error("Snapshot capture failed", "error", err)
		s.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("snapshot capture failed: %v", err))
		return
	}

	if protoResp.Accepted {
		s.sendSuccessResponse(w, map[string]interface{}{
			"accepted":     true,
			"message":      protoResp.Message,
			"snapshot_ids": protoResp.SnapshotIds,
		})
	} else {
		s.sendErrorResponse(w, http.StatusBadRequest, protoResp.Message)
	}
}

// handleDeployModel handles model deployment requests (multipart upload)
func (s *HTTPServer) handleDeployModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.modelStorage == nil {
		s.sendErrorResponse(w, http.StatusServiceUnavailable, "model storage service not available")
		return
	}

	ctx := r.Context()

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("failed to parse multipart form: %v", err))
		return
	}

	// Get metadata
	metadataJSON := r.FormValue("metadata")
	if metadataJSON == "" {
		s.sendErrorResponse(w, http.StatusBadRequest, "metadata field is required")
		return
	}

	var metadata struct {
		DeploymentID      string                 `json:"deployment_id"`
		ModelID           string                 `json:"model_id"`
		Version           string                 `json:"version"`
		ModelType         string                 `json:"model_type"`
		CameraID          string                 `json:"camera_id"`
		Framework         string                 `json:"framework"`
		TrainingDatasetID string                 `json:"training_dataset_id"`
		TrainingDate      string                 `json:"training_date"`
		InputShape        []int                  `json:"input_shape"`
		Preprocessing     map[string]interface{} `json:"preprocessing"`
		TotalSize         uint64                 `json:"total_size"`
	}

	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid metadata JSON: %v", err))
		return
	}

	// Get model file
	file, _, err := r.FormFile("model")
	if err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("model file is required: %v", err))
		return
	}
	defer file.Close()

	// Read model data
	modelData, err := io.ReadAll(file)
	if err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("failed to read model file: %v", err))
		return
	}

	// Validate size
	if uint64(len(modelData)) != metadata.TotalSize {
		s.sendErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("model size mismatch: expected %d, received %d", metadata.TotalSize, len(modelData)))
		return
	}

	// Parse camera ID
	var cameraID *string
	if metadata.CameraID != "" {
		cameraID = &metadata.CameraID
	}

	var deploymentID *string
	if metadata.DeploymentID != "" {
		deploymentID = &metadata.DeploymentID
	}

	var trainingDatasetID *string
	if metadata.TrainingDatasetID != "" {
		trainingDatasetID = &metadata.TrainingDatasetID
	}

	var trainingDate *string
	if metadata.TrainingDate != "" {
		trainingDate = &metadata.TrainingDate
	}

	// Create ModelMetadata
	modelMetadata := &storage.ModelMetadata{
		ModelID:           metadata.ModelID,
		Version:           metadata.Version,
		ModelType:         metadata.ModelType,
		CameraID:          cameraID,
		Framework:         metadata.Framework,
		TrainingDatasetID: trainingDatasetID,
		TrainingDate:      trainingDate,
		InputShape:        metadata.InputShape,
		Preprocessing:     metadata.Preprocessing,
	}

	// Store model
	deployedModel, err := s.modelStorage.StoreModel(ctx, metadata.ModelID, deploymentID, s.edgeID, cameraID, modelData, modelMetadata)
	if err != nil {
		s.logger.Error("Failed to store model", "error", err)
		s.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to store model: %v", err))
		return
	}

	s.logger.Info("Model deployed successfully",
		"deployment_id", metadata.DeploymentID,
		"model_id", metadata.ModelID,
		"model_path", deployedModel.ModelPath)

	s.sendSuccessResponse(w, map[string]interface{}{
		"model_file_path": deployedModel.ModelPath,
		"message":         "Model deployed successfully",
	})
}

// handleRestartService handles service restart requests
func (s *HTTPServer) handleRestartService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ServiceName string `json:"service_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// TODO: Implement service restart logic
	s.sendSuccessResponse(w, map[string]interface{}{
		"message": fmt.Sprintf("Service restart not yet implemented for: %s", req.ServiceName),
	})
}

// handleSyncCapabilities handles capability sync requests
func (s *HTTPServer) handleSyncCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement capability sync
	s.sendSuccessResponse(w, map[string]interface{}{
		"message": "Capability sync not yet implemented",
	})
}

// Helper functions

func (s *HTTPServer) sendSuccessResponse(w http.ResponseWriter, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"success": true,
	}
	for k, v := range data {
		response[k] = v
	}
	json.NewEncoder(w).Encode(response)
}

func (s *HTTPServer) sendErrorResponse(w http.ResponseWriter, statusCode int, errorMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       false,
		"error_message": errorMessage,
	})
}

// loadCACertificate loads CA certificate for client certificate verification
func loadCACertificate(caCertPath string) (*x509.CertPool, error) {
	caCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}
	return caCertPool, nil
}
