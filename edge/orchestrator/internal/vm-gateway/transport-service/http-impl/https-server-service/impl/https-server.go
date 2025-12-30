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
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	objectstoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// HTTPServer implements Edge-side HTTPS server for VM to call Edge
// All traffic routes through tunnel (WireGuard, OpenVPN, etc.) or localhost for development
type HTTPServer struct {
	serverCfg     *httpsservertypes.HTTPServerConfig
	tunnelClient  tunnelclient.TunnelClientService // Tunnel client for checking interface status and connection state
	logger        *zap.Logger
	eventBus      eventbus.EventBus // Optional: for publishing events
	httpServer    *http.Server
	listener      net.Listener
	metaStorage   metastorage.MetaDataStore
	objectStorage objectstorage.ObjectStorageService
	edgeID        string // Edge ID for GetConfig
	rateLimiter   *RateLimiter
	listenAddr    string // Listen address, stored during Start() for use in Stop() events
	mu            sync.RWMutex
}

// NewHTTPServer creates a new HTTPS server for Edge
func NewHTTPServer(
	serverCfg *httpsservertypes.HTTPServerConfig,
	log *zap.Logger,
	edgeID string, // Edge ID for GetConfig
	tunnelClient tunnelclient.TunnelClientService, // Tunnel client for checking interface status and connection state
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
) *HTTPServer {
	// Normalize logger: if nil, use a no-op logger
	if log == nil {
		log = zap.NewNop()
	}
	return &HTTPServer{
		serverCfg:     serverCfg,
		tunnelClient:  tunnelClient,
		logger:        log,
		edgeID:        edgeID,
		metaStorage:   metaStore,
		objectStorage: objectStore,
		eventBus:      eventBus,
	}
}

// deployModel deploys a model using meta-storage and object-storage
// deviceID is required - models are trained on specific device datasets and must be associated with a device
func (s *HTTPServer) deployModel(ctx context.Context, modelID string, deploymentID *string, edgeID string, deviceID string, modelData []byte, metadata httpsservertypes.ModelDeploymentMetadata) (string, error) {
	if s.metaStorage == nil || s.objectStorage == nil {
		return "", fmt.Errorf("meta-storage or object-storage not available")
	}

	// Set defaults for metadata fields
	version := metadata.Version
	if version == "" {
		version = "1.0"
	}
	modelType := metadata.ModelType
	if modelType == "" {
		modelType = "yolo"
	}
	framework := metadata.Framework
	if framework == "" {
		framework = "onnx"
	}

	// Prepare metadata JSON for object storage
	metadataJSON := map[string]interface{}{
		"model_id":    modelID,
		"version":     version,
		"model_type":  modelType,
		"framework":   framework,
		"device_id":   deviceID,
		"deployed_at": time.Now().Format(time.RFC3339),
	}
	if metadata.TrainingDatasetID != "" {
		metadataJSON["training_dataset_id"] = metadata.TrainingDatasetID
	}
	if metadata.TrainingDate != "" {
		metadataJSON["training_date"] = metadata.TrainingDate
	}
	if len(metadata.InputShape) > 0 {
		metadataJSON["input_shape"] = metadata.InputShape
	}
	if len(metadata.Preprocessing) > 0 {
		metadataJSON["preprocessing"] = metadata.Preprocessing
	}

	metadataJSONBytes, err := json.Marshal(metadataJSON)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Convert deviceID to DeviceID type
	deviceIDTyped := objectstoragetypes.DeviceID(deviceID)
	
	// Get device type from device metadata if available, default to camera
	deviceType := metastoragetypes.DeviceTypeCamera
	if s.metaStorage != nil {
		device, found := s.metaStorage.GetDevice(ctx, deviceID)
		if found {
			deviceType = device.DeviceType
		}
	}

	// Store model artifacts in object storage
	artifacts := map[string][]byte{
		"model":    modelData,
		"metadata": metadataJSONBytes,
	}
	if err := s.objectStorage.StoreModelArtifacts(ctx, modelID, deviceIDTyped, nil, artifacts); err != nil {
		return "", fmt.Errorf("failed to store model artifacts in object storage: %w", err)
	}

	// Get model paths from object storage (using GenerateModelKey)
	modelKey := s.objectStorage.GenerateModelKey(modelID, deviceIDTyped, "model")
	metadataKey := s.objectStorage.GenerateModelKey(modelID, deviceIDTyped, "metadata")

	// Save model metadata in meta-storage
	now := time.Now()
	deploymentIDStr := ""
	if deploymentID != nil {
		deploymentIDStr = *deploymentID
	}
	modelMetadata := metastoragetypes.ModelDeploymentMetadata{
		ModelID:      modelID,
		DeploymentID: deploymentIDStr,
		DeviceID:     metastoragetypes.DeviceID(deviceID),
		DeviceType:   deviceType,
		ModelPath:    modelKey,
		MetadataPath: metadataKey,
		DeployedAt:   now,
		Status:       "active",
		EdgeID:       edgeID,
		Version:      version,
		ModelType:    modelType,
		Framework:    framework,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.metaStorage.SaveModelDeployment(ctx, modelMetadata); err != nil {
		// Try to clean up object storage on failure
		_ = s.objectStorage.DeleteModelArtifacts(ctx, modelID, deviceIDTyped)
		return "", fmt.Errorf("failed to save model metadata: %w", err)
	}

	return modelKey, nil
}

// Name returns the service name
func (s *HTTPServer) Name() string {
	return "https-server"
}

// Start starts the HTTPS server
func (s *HTTPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use config values with defaults
	listenAddr := s.serverCfg.ListenAddress
	if listenAddr == "" {
		// Default to tunnel IP for production, localhost for dev
		if s.isTunnelEnabled() {
			listenAddr = "10.0.0.2:8443"
		} else {
			listenAddr = "localhost:8443" // Development mode
		}
	}

	// For development mode (tunnel disabled), allow localhost addresses only
	// Note: 0.0.0.0 is not allowed for security reasons (binds to all interfaces)
	if !s.isTunnelEnabled() {
		host, _, err := net.SplitHostPort(listenAddr)
		if err != nil {
			return fmt.Errorf("invalid listen address: %w", err)
		}
		isLocalhost := host == "localhost" || host == "127.0.0.1" || host == ""
		if !isLocalhost {
			s.logger.Info("HTTPS server disabled in dev mode - must use localhost address when tunnel is disabled",
				zap.String("listen_address", listenAddr))
			return nil
		}
	}

	maxWait := s.serverCfg.TunnelInterfaceWaitTimeout
	if maxWait == 0 {
		maxWait = 30 * time.Second
	}
	waitInterval := s.serverCfg.TunnelInterfaceCheckInterval
	if waitInterval == 0 {
		waitInterval = 500 * time.Millisecond
	}

	s.logger.Info("Starting Edge HTTPS server", zap.String("address", listenAddr))

	// Emit transport.connecting event
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.TransportConnectingEventData]{
			Type:      evtbusstypes.EventTypeNetworkTransportConnecting,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TransportConnectingEventData{
				Service:  "https-server",
				Endpoint: listenAddr,
				Protocol: "https",
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish transport connecting event", zap.Error(err))
		}
	}

	// Wait for tunnel interface only if tunnel is enabled and not localhost
	host, _, _ := net.SplitHostPort(listenAddr)
	if s.isTunnelEnabled() && host != "localhost" && host != "127.0.0.1" {
		waited := time.Duration(0)
		for waited < maxWait {
			if s.isTunnelInterfaceReady(host) {
				s.logger.Info("Tunnel interface is ready", zap.String("address", listenAddr))
				break
			}

			if waited > 0 {
				s.logger.Debug("Waiting for tunnel interface to be ready",
					zap.String("address", listenAddr),
					zap.Duration("waited", waited))
			}

			time.Sleep(waitInterval)
			waited += waitInterval
		}

		if waited >= maxWait {
			s.logger.Info("Tunnel interface not ready after waiting, attempting to bind anyway",
				zap.String("address", listenAddr),
				zap.Duration("waited", waited))
		}
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		// Emit transport.connection_error event
		if s.eventBus != nil {
			event := evtbusstypes.Event[evtbusstypes.TransportConnectionErrorEventData]{
				Type:      evtbusstypes.EventTypeNetworkTransportConnectionError,
				Source:    s.Name(),
				Timestamp: time.Now(),
				Data: evtbusstypes.TransportConnectionErrorEventData{
					Service:   "https-server",
					Endpoint:  listenAddr,
					Protocol:  "https",
					Error:     err.Error(),
					Retryable: true,
				},
			}
			if pubErr := eventbus.PublishTyped(s.eventBus, event); pubErr != nil {
				s.logger.Warn("Failed to publish transport connection error event", zap.Error(pubErr))
			}
		}
		return fmt.Errorf("failed to create listener on %s: %w", listenAddr, err)
	}
	s.listener = listener

	// Load TLS credentials for mTLS (zero-trust security)
	// Certificates are always required - validate before proceeding
	serverCertPath := s.serverCfg.ServerCertPath
	serverKeyPath := s.serverCfg.ServerKeyPath
	caCertPath := s.serverCfg.CACertPath

	// Check if we're in localhost dev mode (tunnel disabled)
	// Note: 0.0.0.0 is not allowed for security reasons (binds to all interfaces)
	isLocalhostMode := !s.isTunnelEnabled()
	if isLocalhostMode && listenAddr != "" {
		host, _, err := net.SplitHostPort(listenAddr)
		if err == nil {
			isLocalhostMode = host == "localhost" || host == "127.0.0.1"
		}
	}

	// Only set defaults if not in localhost mode (production with tunnel)
	if !isLocalhostMode {
		if serverCertPath == "" {
			serverCertPath = "/etc/ssl/certs/edge-server.crt"
		}
		if serverKeyPath == "" {
			serverKeyPath = "/etc/ssl/private/edge-server.key"
		}
		if caCertPath == "" {
			caCertPath = "/etc/ssl/certs/ca.crt"
		}
	}

	// Certificates are required - fail fast if any are missing
	if serverCertPath == "" || serverKeyPath == "" || caCertPath == "" {
		missingFields := []string{}
		if serverCertPath == "" {
			missingFields = append(missingFields, "server_cert_path")
		}
		if serverKeyPath == "" {
			missingFields = append(missingFields, "server_key_path")
		}
		if caCertPath == "" {
			missingFields = append(missingFields, "ca_cert_path")
		}
		// Close listener before returning error
		listener.Close()
		s.listener = nil
		return fmt.Errorf("TLS certificates are required for HTTPS server. Missing required fields: %s. "+
			"Please provide server_cert_path, server_key_path, and ca_cert_path in the HTTPS server configuration",
			strings.Join(missingFields, ", "))
	}

	var tlsConfig *tls.Config

	// Load TLS certificates (all paths are now guaranteed to be non-empty)
	{
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

		// Setup certificate pinning if configured
		// Default to enabled if fingerprint is provided
		pinningConfig := s.serverCfg.CertificatePinning
		// If fingerprint is provided but pinning is not explicitly disabled, enable it
		if pinningConfig.EdgeCAFingerprint != "" {
			if !pinningConfig.PinningEnabled {
				// Default to enabled when fingerprint is present
				pinningConfig.PinningEnabled = true
			}
			SetupServerCertificatePinning(tlsConfig, &pinningConfig, s.logger)
		}

		// Setup certificate revocation checking if configured
		if s.serverCfg.CertificateRevocation.CRLEnabled || s.serverCfg.CertificateRevocation.OCSPEnabled {
			SetupServerCertificateRevocation(tlsConfig, &s.serverCfg.CertificateRevocation, s.logger)
		}

		s.logger.Info("Loaded TLS credentials for HTTPS server (mTLS enabled)",
			zap.String("server_cert", serverCertPath))
	}

	// Create HTTP mux and setup routes
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	// Create rate limiter if enabled
	var handler http.Handler = mux
	if s.serverCfg.RateLimit.Enabled {
		s.rateLimiter = NewRateLimiter(&s.serverCfg.RateLimit, s.logger, s.eventBus)
		handler = RateLimitMiddleware(s.rateLimiter, s.logger)(mux)
		s.logger.Info("Rate limiting enabled",
			zap.Int("default_requests_per_minute", s.serverCfg.RateLimit.GetLimitForEndpoint("")),
			zap.Int("burst_size", s.serverCfg.RateLimit.GetBurstSize()))
	}

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
		Handler:      handler,
		TLSConfig:    tlsConfig,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Store listen address for use in Stop() disconnect events
	s.listenAddr = listenAddr

	// Start server in goroutine
	go func() {
		s.logger.Info("Edge HTTPS server listening and ready to accept connections", zap.String("address", listenAddr))
		// Use ServeTLS since we're providing TLS config
		if err := s.httpServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTPS server error", zap.Error(err))
			// Emit transport.connection_error event if server fails after starting
			if s.eventBus != nil {
				event := evtbusstypes.Event[evtbusstypes.TransportConnectionErrorEventData]{
					Type:      evtbusstypes.EventTypeNetworkTransportConnectionError,
					Source:    s.Name(),
					Timestamp: time.Now(),
					Data: evtbusstypes.TransportConnectionErrorEventData{
						Service:   "https-server",
						Endpoint:  listenAddr,
						Protocol:  "https",
						Error:     err.Error(),
						Retryable: true,
					},
				}
				if pubErr := eventbus.PublishTyped(s.eventBus, event); pubErr != nil {
					s.logger.Warn("Failed to publish transport connection error event", zap.Error(pubErr))
				}
			}
		}
	}()

	// Give the server a moment to start accepting connections
	// The server is started in a goroutine, so we return immediately
	// The caller should use WaitForServerReady() or IsServerReady() to verify readiness
	time.Sleep(100 * time.Millisecond)

	s.logger.Info("Edge HTTPS server started (checking readiness)", zap.String("address", listenAddr))

	// Emit transport.connected event
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.TransportConnectedEventData]{
			Type:      evtbusstypes.EventTypeNetworkTransportConnected,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TransportConnectedEventData{
				Service:  "https-server",
				Endpoint: listenAddr,
				Protocol: "https",
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish transport connected event", zap.Error(err))
		}
	}

	return nil
}

// Stop stops the HTTPS server
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop rate limiter if it exists
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
		s.rateLimiter = nil
	}

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

	// Emit transport.disconnected event
	// Use stored listenAddr (captured during Start()) since httpServer may be nil
	if s.eventBus != nil {
		listenAddr := s.listenAddr // Use stored address from Start()
		event := evtbusstypes.Event[evtbusstypes.TransportDisconnectedEventData]{
			Type:      evtbusstypes.EventTypeNetworkTransportDisconnected,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TransportDisconnectedEventData{
				Service:  "https-server",
				Endpoint: listenAddr,
				Protocol: "https",
				Reason:   "server stopped",
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish transport disconnected event", zap.Error(err))
		}
	}

	// Clear stored listen address
	s.listenAddr = ""

	return nil
}

// isTunnelEnabled returns true if tunnel service is enabled and not "none"
func (s *HTTPServer) isTunnelEnabled() bool {
	return s.tunnelClient != nil && s.tunnelClient.Name() != "none"
}

// isTunnelInterfaceReady checks if the tunnel interface exists and has the specified IP address.
// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
func (s *HTTPServer) isTunnelInterfaceReady(expectedIP string) bool {
	// Method 1: Use tunnel client's interface name if available
	if s.tunnelClient != nil {
		iface := s.tunnelClient.GetInterfaceName()
		if iface != "" {
			// Check if interface exists and has the IP address
			return s.checkInterfaceHasIP(iface, expectedIP)
		}
	}

	// Method 2: Use system commands to check interface status
	// Try 'ip addr show' to check if interface exists and has IP
	// Default to "wg0" for backward compatibility, but this works with any tunnel provider
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

// getVMCommandProcessingTimeout returns the VM command processing timeout with default.
func (s *HTTPServer) getVMCommandProcessingTimeout() time.Duration {
	if s.serverCfg.Timeouts.VMCommandProcessingTimeout > 0 {
		return s.serverCfg.Timeouts.VMCommandProcessingTimeout
	}
	return 10 * time.Second // Default: 10 seconds
}

// setupRoutes configures all REST API endpoints
func (s *HTTPServer) setupRoutes(mux *http.ServeMux) {
	// Health check endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/health/ready", s.handleReadiness)

	// API v1 endpoints
	mux.HandleFunc("/api/v1/config/get", s.handleGetConfig)
	mux.HandleFunc("/api/v1/config/update", s.handleUpdateConfig)
	mux.HandleFunc("/api/v1/snapshots/capture", s.handleRequestDataUnitCapture) // Data unit capture (device-agnostic)
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

// handleReadiness handles readiness check requests
// This endpoint verifies that the HTTPS server is ready to accept connections.
func (s *HTTPServer) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if server is ready
	if !s.IsServerReady() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "not_ready",
			"service": "edge-https-server",
			"message": "HTTPS server is not ready",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ready",
		"service": "edge-https-server",
		"message": "HTTPS server is ready to accept connections",
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

	// Limit request body size (max 1MB for config update requests)
	// MaxBytesReader handles both Content-Length and chunked requests
	LimitRequestBody(r, 1*1024*1024)

	var req struct {
		ConfigJSON string `json:"config_json"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		// Check if error is due to request body too large
		if errStr := err.Error(); strings.Contains(errStr, "request body too large") {
			s.handleValidationError(w, &ValidationError{Field: "request_body", Message: "exceeds maximum size of 1MB"}, s.logger)
			return
		}
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Validate config JSON
	if err := ValidateJSON("config_json", req.ConfigJSON, true); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}

	// Use VM command processing timeout
	ctx := r.Context()
	timeout := s.getVMCommandProcessingTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// TODO: Implement config update logic
	_ = timeoutCtx // Use timeoutCtx when implementing config update
	s.sendSuccessResponse(w, map[string]interface{}{
		"message": "Config update not yet implemented",
	})
}

// handleRequestDataUnitCapture handles data unit capture requests from VM (snapshots, sensor readings, etc.)
// This replaces the deprecated @snapshot_request package functionality
// Supports device-agnostic data capture for any IoT device type
func (s *HTTPServer) handleRequestDataUnitCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DeviceID    string `json:"device_id"` // Device ID (device-agnostic)
		Label       string `json:"label"`     // Suggested label (user will verify via UI)
		CustomLabel string `json:"custom_label"`
		Count       int32  `json:"count"`
	}

	// Limit request body size (max 1MB for data unit capture requests)
	// MaxBytesReader handles both Content-Length and chunked requests
	LimitRequestBody(r, 1*1024*1024)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		// Check if error is due to request body too large
		if errStr := err.Error(); strings.Contains(errStr, "request body too large") {
			s.handleValidationError(w, &ValidationError{Field: "request_body", Message: "exceeds maximum size of 1MB"}, s.logger)
			return
		}
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Validate and sanitize inputs
	if err := ValidateDeviceID(req.DeviceID); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	deviceID := SanitizeString(req.DeviceID)

	if err := ValidateLabel(req.Label); err != nil {
		// If label is empty, set default
		if req.Label == "" {
			req.Label = "normal"
		} else {
			s.handleValidationError(w, err, s.logger)
			return
		}
	}

	if err := ValidateCustomLabel(req.CustomLabel, req.Label == "custom"); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if req.CustomLabel != "" {
		req.CustomLabel = SanitizeString(req.CustomLabel)
	}

	if err := ValidateCount(req.Count, 1, 1000); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}

	// Use VM command processing timeout
	ctx := r.Context()
	timeout := s.getVMCommandProcessingTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

		// Validate device exists in IoT inventory (meta-storage is the source of truth for discovered devices)
	if s.metaStorage != nil {
		_, found := s.metaStorage.GetDevice(timeoutCtx, deviceID)
		if !found {
			s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("device_id '%s' not found in device inventory", deviceID))
			return
		}
	}

	// Set defaults (validation already handled above)
	label := req.Label
	count := req.Count

	s.logger.Info("Received data unit capture request from VM",
		zap.String("device_id", deviceID),
		zap.String("label", label),
		zap.Int32("count", count))

	// Always require user verification for training datasets
	// VM provides a suggested label, but user must verify/correct it via UI before capture
	// This ensures training data quality and prevents incorrect labels from polluting the dataset
	// Publish event for state manager to handle pending data unit capture request
	// State manager will save the request and notify UI
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.SnapshotRequestedEventData]{
			Type:      evtbusstypes.EventTypeDataUnitRequested,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.SnapshotRequestedEventData{
				DeviceID:    deviceID,
				Label:       label,
				CustomLabel: req.CustomLabel,
				Count:       count,
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish data unit capture event", zap.Error(err))
		}
	}

	s.logger.Info("Stored pending data unit capture request for UI",
		zap.String("device_id", deviceID),
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

	// Use VM command processing timeout
	ctx := r.Context()
	timeout := s.getVMCommandProcessingTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Limit request body size (max 100MB for model deployments)
	// Note: For multipart forms, we still need to limit the overall body size,
	// but ParseMultipartForm has its own maxMemory parameter for buffering.
	maxSize := s.serverCfg.MultipartFormMaxMemory
	if maxSize == 0 {
		maxSize = 100 << 20 // Default: 100MB
	}
	// MaxBytesReader handles both Content-Length and chunked requests
	LimitRequestBody(r, maxSize)

	// Parse multipart form using configured max memory (default: 100MB if not configured)
	// Note: This is the memory limit for buffering; larger files will be written to temp files on disk
	// maxMemory should match maxSize for consistency
	maxMemory := maxSize
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		// Check if error is due to request body too large
		if errStr := err.Error(); strings.Contains(errStr, "request body too large") {
			s.handleValidationError(w, &ValidationError{Field: "request_body", Message: fmt.Sprintf("exceeds maximum size of %d bytes", maxSize)}, s.logger)
			return
		}
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("failed to parse multipart form: %v", err))
		return
	}

	// Get metadata
	metadataJSON := r.FormValue("metadata")
	if err := ValidateJSON("metadata", metadataJSON, true); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}

	var metadata httpsservertypes.ModelDeploymentMetadata

	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid metadata JSON: %v", err))
		return
	}

	// Validate metadata fields
	if err := ValidateModelID(metadata.ModelID); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if err := ValidateDeviceID(metadata.DeviceID); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if err := ValidateDeploymentID(metadata.DeploymentID); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if err := ValidateVersion(metadata.Version, false); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if err := ValidateModelType(metadata.ModelType, false); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if err := ValidateFramework(metadata.Framework, false); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	if err := ValidateInputShape(metadata.InputShape); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}

	// Sanitize string fields
	metadata.ModelID = SanitizeString(metadata.ModelID)
	metadata.DeviceID = SanitizeString(metadata.DeviceID)
	if metadata.DeploymentID != "" {
		metadata.DeploymentID = SanitizeString(metadata.DeploymentID)
	}
	if metadata.Version != "" {
		metadata.Version = SanitizeString(metadata.Version)
	}
	if metadata.ModelType != "" {
		metadata.ModelType = SanitizeString(metadata.ModelType)
	}
	if metadata.Framework != "" {
		metadata.Framework = SanitizeString(metadata.Framework)
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

	// Device ID validation already handled above

	var deploymentID *string
	if metadata.DeploymentID != "" {
		deploymentID = &metadata.DeploymentID
	}

	// Deploy model using meta-storage and object-storage
	modelKey, err := s.deployModel(timeoutCtx, metadata.ModelID, deploymentID, s.edgeID, metadata.DeviceID, modelData, metadata)
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
		eventData := evtbusstypes.ModelDeploymentStatusEventData{
			DeploymentID: *deploymentID,
			Status:       "deployed",
			ModelPath:    modelKey,
			ModelID:      metadata.ModelID,
		}
		event := evtbusstypes.Event[evtbusstypes.ModelDeploymentStatusEventData]{
			Type:      evtbusstypes.EventType("model.deployment.status"),
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data:      eventData,
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish model deployment status event", zap.Error(err))
		}
	}

	// Publish model deployment event to event bus
	// State manager will listen for this event and notify AI gateway
	if s.eventBus != nil {
		// Get model metadata from meta-storage to include in event
		eventData := evtbusstypes.ModelDeployedEventData{
			ModelID:      metadata.ModelID,
			DeploymentID: deploymentID,
		}

		if s.metaStorage != nil {
			modelMeta, found := s.metaStorage.GetModelDeployment(timeoutCtx, metadata.ModelID)
			if found {
				eventData.Version = modelMeta.Version
				eventData.ModelType = modelMeta.ModelType
				eventData.Framework = modelMeta.Framework
				eventData.ModelPath = modelMeta.ModelPath
				eventData.MetadataPath = modelMeta.MetadataPath
			}
		}

		eventData.DeviceID = metadata.DeviceID

		event := evtbusstypes.Event[evtbusstypes.ModelDeployedEventData]{
			Type:      evtbusstypes.EventType("model.deployed"),
			Source:    "vm-gateway",
			Timestamp: time.Now(),
			Data:      eventData,
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish model deployed event", zap.Error(err))
		}

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

	// Limit request body size (max 1KB for service restart requests)
	// MaxBytesReader handles both Content-Length and chunked requests
	LimitRequestBody(r, 1*1024)

	var req struct {
		ServiceName string `json:"service_name"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		// Check if error is due to request body too large
		if err.Error() == "http: request body too large" {
			s.handleValidationError(w, &ValidationError{Field: "request_body", Message: "exceeds maximum size of 1KB"}, s.logger)
			return
		}
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Validate service name
	if err := ValidateString("service_name", req.ServiceName, 1, 255, true); err != nil {
		s.handleValidationError(w, err, s.logger)
		return
	}
	req.ServiceName = SanitizeString(req.ServiceName)

	// Use VM command processing timeout
	ctx := r.Context()
	timeout := s.getVMCommandProcessingTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// TODO: Implement service restart logic
	_ = timeoutCtx // Use timeoutCtx when implementing service restart
	s.sendSuccessResponse(w, map[string]interface{}{
		"message": fmt.Sprintf("Service restart not yet implemented for: %s", req.ServiceName),
	})
}

// handleSyncCapabilities handles capability sync requests from VM
// VM sends Edge device capabilities (video devices, sensors, etc.) which Edge stores and processes
func (s *HTTPServer) handleSyncCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size (max 1MB for capabilities sync requests)
	// MaxBytesReader handles both Content-Length and chunked requests
	LimitRequestBody(r, 1*1024*1024)

	var req struct {
		Capabilities map[string]interface{} `json:"capabilities"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		// Check if error is due to request body too large
		if errStr := err.Error(); strings.Contains(errStr, "request body too large") {
			s.handleValidationError(w, &ValidationError{Field: "request_body", Message: "exceeds maximum size of 1MB"}, s.logger)
			return
		}
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if req.Capabilities == nil {
		s.sendErrorResponse(w, http.StatusBadRequest, "capabilities field is required")
		return
	}

	// Validate capabilities map size (prevent DoS)
	if len(req.Capabilities) > 1000 {
		s.handleValidationError(w, &ValidationError{Field: "capabilities", Message: "must contain at most 1000 entries"}, s.logger)
		return
	}

	// Store capabilities in meta-storage
	if s.metaStorage != nil {
		// Use VM command processing timeout
		ctx := r.Context()
		timeout := s.getVMCommandProcessingTimeout()
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := s.metaStorage.SaveEdgeCapabilities(timeoutCtx, req.Capabilities); err != nil {
			s.logger.Error("Failed to save edge capabilities", zap.Error(err))
			s.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to save capabilities: %v", err))
			return
		}
		s.logger.Info("Edge capabilities saved", zap.Any("capabilities", req.Capabilities))
	}

	// Send event to state manager to process capabilities
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.CapabilitiesReceivedEventData]{
			Type:      evtbusstypes.EventType("edge.capabilities_received"),
			Source:    "https-server",
			Timestamp: time.Now(),
			Data: evtbusstypes.CapabilitiesReceivedEventData{
				Capabilities: req.Capabilities,
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish capabilities received event", zap.Error(err))
		}
		s.logger.Info("Published capabilities_received event to state manager")
	}

	s.sendSuccessResponse(w, map[string]interface{}{
		"message": "Capabilities received and stored successfully",
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

// GetCertificateRotationStatus returns the certificate rotation status.
// HTTPServer doesn't implement certificate rotation (that's the client's job).
func (s *HTTPServer) GetCertificateRotationStatus() *vmgatewaytypes.CertificateRotationStatus {
	return nil
}

// GetTimeSyncStatus returns the time synchronization status.
// HTTPServer doesn't implement time sync checking (that's the client's job).
func (s *HTTPServer) GetTimeSyncStatus() *vmgatewaytypes.TimeSyncStatus {
	return nil
}

// GetRateLimitStats returns rate limiting statistics.
// This method is used for health metrics collection.
func (s *HTTPServer) GetRateLimitStats() *vmgatewaytypes.RateLimitStats {
	s.mu.RLock()
	rateLimiter := s.rateLimiter
	s.mu.RUnlock()
	if rateLimiter == nil {
		return nil
	}
	enabled, requestsPerMinute, burstSize, totalViolations, activeBuckets := rateLimiter.GetStats()
	return &vmgatewaytypes.RateLimitStats{
		Enabled:          enabled,
		RequestsPerMinute: requestsPerMinute,
		BurstSize:        burstSize,
		TotalViolations:  totalViolations,
		ActiveBuckets:    activeBuckets,
	}
}
