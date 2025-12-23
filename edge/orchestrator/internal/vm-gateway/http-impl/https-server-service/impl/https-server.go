package impl

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

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/types"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"
	"go.uber.org/zap"
)

// HTTPServer implements Edge-side HTTPS server for VM to call Edge
// All traffic routes through WireGuard tunnel (or localhost for development)
type HTTPServer struct {
	serverCfg *httpsservertypes.HTTPServerConfig
	wgCfg     *wgclienttypes.WGClientConfig
	wgClient  interface { // WireGuard client for checking interface status
		GetInterfaceName() string
	}
	logger        *zap.Logger
	eventBus      eventbus.EventBus // Optional: for publishing events
	httpServer    *http.Server
	listener      net.Listener
	cctvService   cctv.CCTVService // Optional: for snapshot capture (replaces snapshot_request.Service)
	metaStorage   metastorage.MetaDataStore
	objectStorage objectstorage.ObjectStorageService
	edgeID        string // Edge ID for GetConfig
	mu            sync.RWMutex
}

// NewHTTPServer creates a new HTTPS server for Edge
func NewHTTPServer(
	serverCfg *httpsservertypes.HTTPServerConfig,
	wgCfg *wgclienttypes.WGClientConfig,
	log *zap.Logger,
	edgeID string, // Edge ID for GetConfig
	wgClient interface { // WireGuard client for checking interface status
		GetInterfaceName() string
	},
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
) *HTTPServer {
	return &HTTPServer{
		serverCfg:     serverCfg,
		wgCfg:         wgCfg,
		wgClient:      wgClient,
		logger:        log,
		edgeID:        edgeID,
		metaStorage:   metaStore,
		objectStorage: objectStore,
		eventBus:      eventBus,
	}
}

// deployModel deploys a model using meta-storage and object-storage
func (s *HTTPServer) deployModel(ctx context.Context, modelID string, deploymentID *string, edgeID string, cameraID *string, modelData []byte, metadata map[string]interface{}) (string, error) {
	if s.metaStorage == nil || s.objectStorage == nil {
		return "", fmt.Errorf("meta-storage or object-storage not available")
	}

	// Extract metadata fields
	version, _ := metadata["version"].(string)
	if version == "" {
		version = "1.0"
	}
	modelType, _ := metadata["model_type"].(string)
	if modelType == "" {
		modelType = "yolo"
	}
	framework, _ := metadata["framework"].(string)
	if framework == "" {
		framework = "onnx"
	}

	// Generate object storage keys
	modelKey := fmt.Sprintf("models/%s/model.onnx", modelID)
	metadataKey := fmt.Sprintf("models/%s/metadata.json", modelID)

	// Prepare metadata JSON for object storage
	metadataJSON := map[string]interface{}{
		"model_id":    modelID,
		"version":     version,
		"model_type":  modelType,
		"framework":   framework,
		"camera_id":   cameraID,
		"deployed_at": time.Now().Format(time.RFC3339),
	}
	if trainingDatasetID, ok := metadata["training_dataset_id"].(string); ok {
		metadataJSON["training_dataset_id"] = trainingDatasetID
	}
	if trainingDate, ok := metadata["training_date"].(string); ok {
		metadataJSON["training_date"] = trainingDate
	}
	if inputShape, ok := metadata["input_shape"].([]interface{}); ok {
		metadataJSON["input_shape"] = inputShape
	}
	if preprocessing, ok := metadata["preprocessing"].(map[string]interface{}); ok {
		metadataJSON["preprocessing"] = preprocessing
	}

	metadataJSONBytes, err := json.Marshal(metadataJSON)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Store model file and metadata in object storage
	if err := s.objectStorage.StoreModel(ctx, modelKey, modelData, metadataKey, metadataJSONBytes); err != nil {
		return "", fmt.Errorf("failed to store model in object storage: %w", err)
	}

	// Save model metadata in meta-storage
	now := time.Now()
	modelMetadata := metastorage.DeployedModelMetadata{
		ModelID:      modelID,
		DeploymentID: deploymentID,
		ModelPath:    modelKey,    // Object storage key
		MetadataPath: metadataKey, // Object storage key
		DeployedAt:   now,
		Status:       "active",
		EdgeID:       edgeID,
		CameraID:     cameraID,
		Version:      version,
		ModelType:    modelType,
		Framework:    framework,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.metaStorage.SaveDeployedModel(ctx, modelMetadata); err != nil {
		// Try to clean up object storage on failure
		_ = s.objectStorage.DeleteModel(ctx, modelKey, metadataKey)
		return "", fmt.Errorf("failed to save model metadata: %w", err)
	}

	return modelKey, nil
}

// SetCCTVService sets the CCTV service for snapshot capture (replaces snapshot_request.Service)
func (s *HTTPServer) SetCCTVService(cctvSvc cctv.CCTVService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cctvService = cctvSvc
}

// Name returns the service name
func (s *HTTPServer) Name() string {
	return "https-server"
}

// Start starts the HTTPS server
func (s *HTTPServer) Start(ctx context.Context) error {
	// Check if WireGuard is enabled (for production) or if we're in localhost dev mode
	if s.wgCfg != nil && !s.wgCfg.Enabled && s.serverCfg.ListenAddress == "" {
		s.logger.Info("HTTPS server disabled (WireGuard disabled and no listen address configured)")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Use config values with defaults
	listenAddr := s.serverCfg.ListenAddress
	if listenAddr == "" {
		// Default to WireGuard IP for production, localhost for dev
		if s.wgCfg != nil && s.wgCfg.Enabled {
			listenAddr = "10.0.0.2:8443"
		} else {
			listenAddr = "localhost:8443" // Development mode
		}
	}

	maxWait := s.serverCfg.WireGuardInterfaceWaitTimeout
	if maxWait == 0 {
		maxWait = 30 * time.Second
	}
	waitInterval := s.serverCfg.WireGuardInterfaceCheckInterval
	if waitInterval == 0 {
		waitInterval = 500 * time.Millisecond
	}

	s.logger.Info("Starting Edge HTTPS server", zap.String("address", listenAddr))

	// Wait for WireGuard interface only if WireGuard is enabled and not localhost
	host, _, _ := net.SplitHostPort(listenAddr)
	if s.wgCfg != nil && s.wgCfg.Enabled && host != "localhost" && host != "127.0.0.1" {
		waited := time.Duration(0)
		for waited < maxWait {
			if s.isWireGuardInterfaceReady(host) {
				s.logger.Info("WireGuard interface is ready", zap.String("address", listenAddr))
				break
			}

			if waited > 0 {
				s.logger.Debug("Waiting for WireGuard interface to be ready",
					zap.String("address", listenAddr),
					zap.Duration("waited", waited))
			}

			time.Sleep(waitInterval)
			waited += waitInterval
		}

		if waited >= maxWait {
			s.logger.Info("WireGuard interface not ready after waiting, attempting to bind anyway",
				zap.String("address", listenAddr),
				zap.Duration("waited", waited))
		}
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to create listener on %s: %w", listenAddr, err)
	}
	s.listener = listener

	// Load TLS credentials for mTLS (zero-trust security)
	serverCertPath := s.serverCfg.ServerCertPath
	serverKeyPath := s.serverCfg.ServerKeyPath
	caCertPath := s.serverCfg.CACertPath

	// Defaults for production
	if serverCertPath == "" {
		serverCertPath = "/etc/ssl/certs/edge-server.crt"
	}
	if serverKeyPath == "" {
		serverKeyPath = "/etc/ssl/private/edge-server.key"
	}
	if caCertPath == "" {
		caCertPath = "/etc/ssl/certs/ca.crt"
	}

	var tlsConfig *tls.Config

	// Only load TLS if cert paths are provided (allows localhost dev without certs)
	if serverCertPath != "" && serverKeyPath != "" && caCertPath != "" {
		cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			return fmt.Errorf("failed to load server certificate: %w", err)
		}

		// Load CA certificate for client certificate verification
		caCert, err := loadCACertificate(caCertPath)
		if err != nil {
			return fmt.Errorf("failed to load CA certificate: %w", err)
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS: require client certificate
			ClientCAs:    caCert,                         // x509.CertPool for client certificate verification
			MinVersion:   tls.VersionTLS12,
		}

		s.logger.Info("Loaded TLS credentials for HTTPS server (mTLS enabled)",
			zap.String("server_cert", serverCertPath))
	} else {
		// For localhost dev without certs, use basic TLS without mTLS (development only)
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		s.logger.Warn("HTTPS server running without TLS certificates (development mode only)",
			zap.String("address", listenAddr))
	}

	// Create HTTP mux and setup routes
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	// Use config timeouts with defaults
	readTimeout := s.serverCfg.ReadTimeout
	if readTimeout == 0 {
		readTimeout = 30 * time.Second
	}
	writeTimeout := s.serverCfg.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 30 * time.Second
	}
	idleTimeout := s.serverCfg.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = 120 * time.Second
	}

	// Create HTTPS server
	s.httpServer = &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Start server in goroutine
	go func() {
		s.logger.Info("Edge HTTPS server listening and ready to accept connections", zap.String("address", listenAddr))
		// Use ServeTLS since we're providing TLS config
		if err := s.httpServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTPS server error", zap.Error(err))
		}
	}()

	// Give the server a moment to start accepting connections
	time.Sleep(100 * time.Millisecond)

	s.logger.Info("Edge HTTPS server started", zap.String("address", listenAddr))

	return nil
}

// Stop stops the HTTPS server
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Error shutting down HTTPS server", zap.Error(err))
		}
		s.httpServer = nil
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}

	s.logger.Info("Edge HTTPS server stopped")

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
		s.logger.Error("Failed to marshal config", zap.Error(err))
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

// handleRequestSnapshotCapture handles snapshot capture requests from VM
// This replaces the deprecated @snapshot_request package functionality
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

	if req.CameraID == "" {
		s.sendErrorResponse(w, http.StatusBadRequest, "camera_id is required")
		return
	}

	ctx := r.Context()

	// Validate camera exists (if CCTV service is available)
	if s.cctvService != nil {
		_, err := s.cctvService.GetCamera(ctx, req.CameraID)
		if err != nil {
			s.sendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("camera %s not found", req.CameraID))
			return
		}
	}

	// Set defaults
	label := req.Label
	if label == "" {
		label = "normal"
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}

	// Validate label
	validLabels := map[string]bool{
		"normal":   true,
		"threat":   true,
		"abnormal": true,
		"custom":   true,
	}
	if !validLabels[label] {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid label: %s (must be normal, threat, abnormal, or custom)", label))
		return
	}

	if label == "custom" && req.CustomLabel == "" {
		s.sendErrorResponse(w, http.StatusBadRequest, "custom_label is required when label is 'custom'")
		return
	}

	s.logger.Info("Received snapshot capture request from VM",
		zap.String("camera_id", req.CameraID),
		zap.String("label", label),
		zap.Int32("count", count),
		zap.Bool("auto_capture", req.AutoCapture))

	// If auto_capture is true, capture snapshots immediately
	if req.AutoCapture {
		if s.cctvService == nil {
			s.sendErrorResponse(w, http.StatusServiceUnavailable, "CCTV service not available for auto-capture")
			return
		}

		var snapshotIDs []string
		for i := int32(0); i < count; i++ {
			description := fmt.Sprintf("Auto-captured snapshot %d/%d (VM request)", i+1, count)
			screenshotID, err := s.cctvService.CaptureScreenshotWithLabel(ctx, req.CameraID, label, req.CustomLabel, description)
			if err != nil {
				s.logger.Error("Auto-capture failed", zap.Error(err), zap.String("camera_id", req.CameraID), zap.String("label", label), zap.Int32("index", i+1))
				s.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("auto-capture failed at snapshot %d/%d: %v", i+1, count, err))
				return
			}
			snapshotIDs = append(snapshotIDs, screenshotID)

			// Small delay between captures
			if i < count-1 {
				time.Sleep(500 * time.Millisecond)
			}
		}

		s.logger.Info("Auto-captured snapshots successfully",
			zap.String("camera_id", req.CameraID),
			zap.Int("count", len(snapshotIDs)),
			zap.Strings("snapshot_ids", snapshotIDs))

		s.sendSuccessResponse(w, map[string]interface{}{
			"accepted":     true,
			"message":      fmt.Sprintf("Captured %d snapshots", len(snapshotIDs)),
			"snapshot_ids": snapshotIDs,
		})
		return
	}

	// Publish event for state manager to handle pending snapshot request
	// State manager will save the request and notify UI
	if s.eventBus != nil {
		s.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("snapshot.requested"),
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id":    req.CameraID,
				"label":        label,
				"custom_label": req.CustomLabel,
				"count":        count,
			},
		})
	}

	s.logger.Info("Stored pending snapshot request for UI",
		zap.String("camera_id", req.CameraID),
		zap.String("label", label),
		zap.Int32("count", count))

	s.sendSuccessResponse(w, map[string]interface{}{
		"accepted": true,
		"message":  "Snapshot request stored, user will be notified in UI",
	})
}

// handleDeployModel handles model deployment requests (multipart upload)
func (s *HTTPServer) handleDeployModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.metaStorage == nil || s.objectStorage == nil {
		s.sendErrorResponse(w, http.StatusServiceUnavailable, "meta-storage or object-storage not available")
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

	// Convert metadata struct to map for deployModel
	metadataMap := map[string]interface{}{
		"version":             metadata.Version,
		"model_type":          metadata.ModelType,
		"framework":           metadata.Framework,
		"training_dataset_id": metadata.TrainingDatasetID,
		"training_date":       metadata.TrainingDate,
		"input_shape":         metadata.InputShape,
		"preprocessing":       metadata.Preprocessing,
	}

	// Deploy model using meta-storage and object-storage
	modelKey, err := s.deployModel(ctx, metadata.ModelID, deploymentID, s.edgeID, cameraID, modelData, metadataMap)
	if err != nil {
		s.logger.Error("Failed to deploy model", zap.Error(err), zap.String("model_id", metadata.ModelID))
		s.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to deploy model: %v", err))
		return
	}

	s.logger.Info("Model deployed successfully",
		zap.String("deployment_id", metadata.DeploymentID),
		zap.String("model_id", metadata.ModelID),
		zap.String("model_key", modelKey))

	// Publish event for state manager to report deployment status to VM
	// State manager will handle the status reporting through HTTPS client
	// Report "deployed" status after model is received and stored
	// The AI service will load the model from MinIO and report "active" status separately
	if s.eventBus != nil && deploymentID != nil {
		s.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("model.deployment.status"),
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"deployment_id": *deploymentID,
				"status":        "deployed",
				"model_path":    modelKey,
				"model_id":      metadata.ModelID,
			},
		})
	}

	// Publish model deployment event to event bus
	// State manager will listen for this event and notify AI gateway
	if s.eventBus != nil {
		ctx := r.Context()
		// Get model metadata from meta-storage to include in event
		eventData := map[string]interface{}{
			"model_id":      metadata.ModelID,
			"deployment_id": deploymentID,
		}

		if s.metaStorage != nil {
			modelMeta, found := s.metaStorage.GetDeployedModel(ctx, metadata.ModelID)
			if found {
				eventData["version"] = modelMeta.Version
				eventData["model_type"] = modelMeta.ModelType
				eventData["framework"] = modelMeta.Framework
				eventData["model_path"] = modelMeta.ModelPath
				eventData["metadata_path"] = modelMeta.MetadataPath
			}
		}

		if cameraID != nil && *cameraID != "" {
			eventData["camera_id"] = *cameraID
		}

		s.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("model.deployed"),
			Source:    "vm-gateway",
			Timestamp: time.Now(),
			Data:      eventData,
		})

		deploymentIDStr := ""
		if deploymentID != nil {
			deploymentIDStr = *deploymentID
		}
		s.logger.Info("Model deployment event published",
			zap.String("model_id", metadata.ModelID),
			zap.String("deployment_id", deploymentIDStr),
		)
	}

	s.sendSuccessResponse(w, map[string]interface{}{
		"model_file_path": modelKey,
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
