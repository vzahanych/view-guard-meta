package impl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/config"
	httpsserver "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/https-server-service"
)

// httpsServerService implements the HTTPSServerService interface
type httpsServerService struct {
	httpServer *http.Server
	listener   net.Listener
	mu         sync.RWMutex
	logger     *zap.Logger // Simple logger, can be nil
	ctx        context.Context
	cancel     context.CancelFunc
	tlsConfig  config.TLSConfig
}

// NewHTTPSServerService creates a new HTTPS server service implementation.
// cfg is interface{} to avoid tight coupling to the config package.
// For now, we use simple defaults: listen on 10.0.0.1:8443.
func NewHTTPSServerService(cfg interface{}, log interface{}) (httpsserver.HTTPSServerService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Try to extract logger if it's a zap.Logger
	var logger *zap.Logger
	if zapLogger, ok := log.(*zap.Logger); ok {
		logger = zapLogger
	} else {
		// Create a simple logger if none provided
		logger, _ = zap.NewDevelopment()
	}

	// Extract TLS configuration from application config if available.
	var tlsCfg config.TLSConfig
	if appCfg, ok := cfg.(*config.Config); ok && appCfg != nil {
		tlsCfg = appCfg.TLS
	}

	server := &httpsServerService{
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		tlsConfig: tlsCfg,
	}

	return server, nil
}

// Name returns the service name
func (s *httpsServerService) Name() string {
	return "https-server"
}

// Start starts the HTTPS server
func (s *httpsServerService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Listen on WireGuard interface IP only (Edge will connect to VM's WireGuard IP)
	// Port 8443 for HTTPS (replaces gRPC port 50051)
	// Binding to 10.0.0.1 ensures server is only accessible through WireGuard tunnel
	listenAddr := "10.0.0.1:8443"

	if s.logger != nil {
		s.logger.Info("Starting HTTPS server", zap.String("address", listenAddr))
	}

	// Optionally verify WireGuard interface is ready.
	if !s.isWireGuardInterfaceReady("10.0.0.1") && s.logger != nil {
		s.logger.Warn("WireGuard interface may not be ready, attempting to bind anyway",
			zap.String("address", listenAddr))
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to create listener on %s (WireGuard interface may not be ready): %w", listenAddr, err)
	}
	s.listener = listener

	// Load TLS credentials for mTLS (zero-trust security).
	// Use configured paths if provided, otherwise fall back to system defaults.
	serverCertPath := s.tlsConfig.ServerCert
	serverKeyPath := s.tlsConfig.ServerKey
	caCertPath := s.tlsConfig.CACert

	if serverCertPath == "" {
		serverCertPath = "/etc/ssl/certs/vm-server.crt"
	}
	if serverKeyPath == "" {
		serverKeyPath = "/etc/ssl/private/vm-server.key"
	}
	if caCertPath == "" {
		caCertPath = "/etc/ssl/certs/ca.crt"
	}

	cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Load CA certificate for client certificate verification
	caCertPool, err := loadCACertificate(caCertPath)
	if err != nil {
		return fmt.Errorf("failed to load CA certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS: require client certificate
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	if s.logger != nil {
		s.logger.Info("Loaded TLS credentials for HTTPS server (mTLS enabled)",
			zap.String("server_cert", serverCertPath))
	}

	// Create HTTP mux and setup routes
	mux := http.NewServeMux()
	s.setupRoutes(mux)

	// Create HTTPS server
	s.httpServer = &http.Server{
		Addr:         listenAddr,
		Handler:      s.authMiddleware(mux),
		TLSConfig:    tlsConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if s.logger != nil {
			s.logger.Info("HTTPS server listening and ready to accept connections", zap.String("address", listenAddr))
		}
		// Use ServeTLS since we're providing TLS config
		if err := s.httpServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			if s.logger != nil {
				s.logger.Error("HTTPS server error", zap.Error(err))
			}
		}
	}()

	// Give the server a moment to start accepting connections
	time.Sleep(100 * time.Millisecond)

	if s.logger != nil {
		s.logger.Info("HTTPS server started", zap.String("address", listenAddr))
	}

	return nil
}

// Stop stops the HTTPS server
func (s *httpsServerService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			if s.logger != nil {
				s.logger.Error("Error shutting down HTTPS server", zap.Error(err))
			}
		}
		s.httpServer = nil
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}

	if s.logger != nil {
		s.logger.Info("HTTPS server stopped")
	}

	return nil
}

// isWireGuardInterfaceReady checks if the WireGuard interface exists and has the specified IP address
func (s *httpsServerService) isWireGuardInterfaceReady(expectedIP string) bool {
	// Use system commands to check interface status
	return s.checkInterfaceHasIP("wg0", expectedIP)
}

// checkInterfaceHasIP checks if a network interface exists and has the specified IP address
func (s *httpsServerService) checkInterfaceHasIP(iface, expectedIP string) bool {
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
		// Expected IP is 10.0.0.1, check for 10.0.0.1/24 or similar
		if strings.Contains(outputStr, "10.0.0.1") {
			return true
		}
	}

	return false
}

// setupRoutes configures all REST API endpoints
func (s *httpsServerService) setupRoutes(mux *http.ServeMux) {
	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// API v1 endpoints (simplified for refactoring)
	mux.HandleFunc("/api/v1/auth/authenticate", s.handleAuthenticate)
	mux.HandleFunc("/api/v1/telemetry/heartbeat", s.handleHeartbeat)
	// Additional endpoints can be added as needed during refactoring
}

// authMiddleware extracts Edge identity from client certificate and adds to context
func (s *httpsServerService) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client certificate from TLS connection
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			s.sendErrorResponse(w, http.StatusUnauthorized, "client certificate required")
			return
		}

		clientCert := r.TLS.PeerCertificates[0]

		// Extract Edge identity from certificate (CN or SAN)
		edgeID := clientCert.Subject.CommonName
		if edgeID == "" && len(clientCert.DNSNames) > 0 {
			edgeID = clientCert.DNSNames[0]
		}

		// Get WireGuard peer from connection
		peerAddr := r.RemoteAddr
		// Extract IP from peer address
		host, _, err := net.SplitHostPort(peerAddr)
		if err != nil {
			s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid peer address: %v", err))
			return
		}

		// Add edgeID to context
		ctx := context.WithValue(r.Context(), "edge_id", edgeID)
		ctx = context.WithValue(ctx, "peer_addr", host)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleHealth handles health check requests
func (s *httpsServerService) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "https-server",
	})
}

// handleAuthenticate handles Edge authentication/registration requests
func (s *httpsServerService) handleAuthenticate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		EdgeID string `json:"edge_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Extract edge ID from certificate CN (format: edge-client-{edge-id})
	edgeID := req.EdgeID
	if edgeID == "" {
		if ctxEdgeID, ok := r.Context().Value("edge_id").(string); ok {
			edgeID = ctxEdgeID
			// Certificate CN format is "edge-client-{edge-id}", extract edge ID
			if strings.HasPrefix(edgeID, "edge-client-") {
				edgeID = strings.TrimPrefix(edgeID, "edge-client-")
			}
		}
	}

	if edgeID == "" {
		s.sendErrorResponse(w, http.StatusBadRequest, "edge_id is required (from certificate CN or request body)")
		return
	}

	// Simplified authentication - full implementation will be added during refactoring
	if s.logger != nil {
		s.logger.Info("Edge authentication request",
			zap.String("edge_id", edgeID))
	}

	s.sendSuccessResponse(w, map[string]interface{}{
		"success": true,
		"edge_id": edgeID,
		"message": "Edge authenticated successfully",
	})
}

// handleHeartbeat handles heartbeat requests from Edge
func (s *httpsServerService) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		EdgeID    string `json:"edge_id"`
		Timestamp int64  `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Extract edge ID from certificate CN
	edgeID := req.EdgeID
	if edgeID == "" {
		if ctxEdgeID, ok := r.Context().Value("edge_id").(string); ok {
			edgeID = ctxEdgeID
			if strings.HasPrefix(edgeID, "edge-client-") {
				edgeID = strings.TrimPrefix(edgeID, "edge-client-")
			}
		}
	}

	if edgeID == "" {
		s.sendErrorResponse(w, http.StatusBadRequest, "edge_id is required (from certificate CN or request body)")
		return
	}

	s.sendSuccessResponse(w, map[string]interface{}{
		"success":          true,
		"server_timestamp": time.Now().UnixNano(),
	})
}

// Helper functions

func (s *httpsServerService) sendSuccessResponse(w http.ResponseWriter, data map[string]interface{}) {
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

func (s *httpsServerService) sendErrorResponse(w http.ResponseWriter, statusCode int, errorMessage string) {
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
