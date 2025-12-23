package impl

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	wgclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"
	"go.uber.org/zap"
)

// TelemetryClient interface for sending telemetry data to the VM.
// HTTPSClient implements this interface directly.
type TelemetryClient interface {
	IsConnected() bool
	SendTelemetry(ctx context.Context, data *wgclienttypes.TelemetryData) error
	Heartbeat(ctx context.Context, req *wgclienttypes.HeartbeatRequest) error
}

// HTTPSClient manages HTTPS connections to KVM VM over WireGuard tunnel
// Replaces gRPC client for Edge → VM communication
// HTTPSClient implements TelemetryClient interface directly
type HTTPSClient struct {
	clientCfg     *httpsclienttypes.HTTPSClientConfig
	wgCfg         *wgclienttypes.WGClientConfig
	wgClient      wgclient.WGClientService
	logger        *zap.Logger
	httpClient    *http.Client
	mu            sync.RWMutex
	vmEndpoint    string // VM HTTPS endpoint
	edgeID        string // Edge ID for authentication
	authenticated bool   // Track if authentication with VM has succeeded
	lastAuthError error  // Track last authentication error
}

// NewHTTPSClient creates a new HTTPS client for Edge → VM calls
func NewHTTPSClient(clientCfg *httpsclienttypes.HTTPSClientConfig, wgCfg *wgclienttypes.WGClientConfig, wgClient wgclient.WGClientService, edgeID string, log *zap.Logger) (*HTTPSClient, error) {
	// Use config values, with defaults for development (localhost mode)
	clientCertPath := clientCfg.ClientCertPath
	clientKeyPath := clientCfg.ClientKeyPath
	caCertPath := clientCfg.CACertPath
	vmEndpoint := clientCfg.VMEndpoint
	timeout := clientCfg.Timeout

	// Defaults for localhost development mode
	if vmEndpoint == "" {
		vmEndpoint = "localhost:8443"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Detect localhost mode (WireGuard disabled and localhost endpoint)
	isLocalhostMode := wgCfg != nil && !wgCfg.Enabled && vmEndpoint != ""
	host, _, err := net.SplitHostPort(vmEndpoint)
	if err == nil {
		isLocalhostMode = isLocalhostMode && (host == "localhost" || host == "127.0.0.1")
	}

	// Only set default cert paths if not in localhost mode (for production with WireGuard)
	if !isLocalhostMode {
		if clientCertPath == "" {
			clientCertPath = "/etc/ssl/certs/edge-client.crt"
		}
		if clientKeyPath == "" {
			clientKeyPath = "/etc/ssl/private/edge-client.key"
		}
		if caCertPath == "" {
			caCertPath = "/etc/ssl/certs/ca.crt"
		}
	}

	// When connecting via IP address through WireGuard tunnel,
	// Go's TLS will verify against IP SANs in the certificate.
	// For localhost development, use localhost as ServerName
	serverName := ""
	if vmEndpoint != "" {
		host, _, err := net.SplitHostPort(vmEndpoint)
		if err == nil && host == "localhost" {
			serverName = "localhost"
		}
	}

	var tlsConfig *tls.Config
	var httpClient *http.Client

	// Only load TLS if cert paths are provided (allows localhost dev without certs)
	if clientCertPath != "" && clientKeyPath != "" && caCertPath != "" {
		log.Info("Loading TLS credentials for HTTPS client (mTLS enabled)",
			zap.String("client_cert", clientCertPath),
			zap.String("client_key", clientKeyPath),
			zap.String("ca_cert", caCertPath))
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}

		// Load CA certificate for server verification
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		// Configure TLS with proper certificate verification
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
			MinVersion:   tls.VersionTLS12,
			ServerName:   serverName,
		}
	} else {
		// For localhost dev without certs, use InsecureSkipVerify (development only)
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true, // Only for localhost development
			MinVersion:         tls.VersionTLS12,
		}
	}

	// Create HTTP client with TLS config
	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient = &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return &HTTPSClient{
		clientCfg:  clientCfg,
		wgCfg:      wgCfg,
		wgClient:   wgClient,
		logger:     log,
		httpClient: httpClient,
		vmEndpoint: vmEndpoint,
		edgeID:     edgeID,
	}, nil
}

// Name returns the service name
func (c *HTTPSClient) Name() string {
	return "https-client"
}

// Start starts the HTTPS client service
func (c *HTTPSClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Allow localhost mode even when WireGuard is disabled
	isLocalhostMode := false
	if c.wgCfg != nil && !c.wgCfg.Enabled {
		host, _, _ := net.SplitHostPort(c.vmEndpoint)
		if host != "localhost" && host != "127.0.0.1" {
			c.logger.Info("HTTPS client disabled (WireGuard disabled and not localhost mode)")
			return nil
		}
		isLocalhostMode = true
		c.logger.Info("HTTPS client starting in localhost development mode")
	}

	// Wait for WireGuard to be connected (skip in localhost mode)
	if !isLocalhostMode && c.wgClient != nil && !c.wgClient.IsConnected() {
		c.logger.Info("Waiting for WireGuard connection...")
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timeout:
				return fmt.Errorf("WireGuard not connected after 30 seconds")
			case <-ticker.C:
				if c.wgClient.IsConnected() {
					c.logger.Info("WireGuard connected, HTTPS client ready")
					return nil
				}
			}
		}
	}

	c.logger.Info("HTTPS client started", zap.String("vm_endpoint", c.vmEndpoint))

	// Authenticate with VM when HTTPS client starts (after WireGuard is connected)
	// This sets the connection state to "connected" in both VM and Edge
	go func() {
		// Wait a moment for everything to be ready
		time.Sleep(2 * time.Second)

		// Send authentication request using stored edge ID
		if err := c.Authenticate(ctx, c.edgeID); err != nil {
			c.mu.Lock()
			c.authenticated = false
			c.lastAuthError = err
			c.mu.Unlock()
			c.logger.Error("Failed to authenticate with VM", zap.Error(err))
			// Don't fail startup - will retry on next heartbeat or can be retried
		} else {
			c.mu.Lock()
			c.authenticated = true
			c.lastAuthError = nil
			c.mu.Unlock()
			c.logger.Info("Successfully authenticated with VM", zap.String("edge_id", c.edgeID))
		}
	}()

	return nil
}

// Stop stops the HTTPS client service
func (c *HTTPSClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("HTTPS client stopped")

	return nil
}

// IsConnected returns true if the connection to VM is established and authenticated
// In localhost mode, it checks if authentication has succeeded
// In production mode, it checks if WireGuard is connected
func (c *HTTPSClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.wgCfg != nil && !c.wgCfg.Enabled {
		// Localhost mode - check if authentication has succeeded
		return c.authenticated
	}
	// Production mode - check if WireGuard is connected
	return c.wgClient != nil && c.wgClient.IsConnected()
}

// Heartbeat sends a heartbeat to the VM
func (c *HTTPSClient) Heartbeat(ctx context.Context, req *wgclienttypes.HeartbeatRequest) error {
	url := fmt.Sprintf("https://%s/api/v1/telemetry/heartbeat", c.vmEndpoint)

	reqBody := map[string]interface{}{
		"edge_id":   req.EdgeId,
		"timestamp": req.Timestamp,
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("heartbeat failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success         bool  `json:"success"`
		ServerTimestamp int64 `json:"server_timestamp"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("heartbeat not successful")
	}

	return nil
}

// SendTelemetry sends telemetry data to the VM
func (c *HTTPSClient) SendTelemetry(ctx context.Context, data *wgclienttypes.TelemetryData) error {
	url := fmt.Sprintf("https://%s/api/v1/telemetry/telemetry", c.vmEndpoint)

	// Convert TelemetryData to JSON
	reqBody := map[string]interface{}{
		"timestamp": data.Timestamp,
		"edge_id":   data.EdgeId,
		// Add other telemetry fields as needed
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telemetry failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendEvents sends events to the VM
func (c *HTTPSClient) SendEvents(ctx context.Context, events []*wgclienttypes.Event) error {
	url := fmt.Sprintf("https://%s/api/v1/telemetry/events", c.vmEndpoint)

	reqBody := map[string]interface{}{
		"events": events,
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send events failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success       bool     `json:"success"`
		EventIDs      []string `json:"event_ids"`
		ReceivedCount int32    `json:"received_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("send events not successful")
	}

	return nil
}

// Authenticate authenticates Edge with VM and sets connection state to "connected"
// This should be called when Edge orchestrator starts (after WireGuard is connected)
func (c *HTTPSClient) Authenticate(ctx context.Context, edgeID string) error {
	url := fmt.Sprintf("https://%s/api/v1/auth/authenticate", c.vmEndpoint)

	reqBody := map[string]interface{}{
		"edge_id": edgeID,
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send authentication request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
		c.mu.Lock()
		c.authenticated = false
		c.lastAuthError = err
		c.mu.Unlock()
		return err
	}

	var result struct {
		Success bool   `json:"success"`
		EdgeID  string `json:"edge_id"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		err := fmt.Errorf("authentication not successful: %s", result.Message)
		c.mu.Lock()
		c.authenticated = false
		c.lastAuthError = err
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	c.authenticated = true
	c.lastAuthError = nil
	c.mu.Unlock()
	c.logger.Info("Edge authenticated with VM", zap.String("edge_id", result.EdgeID))
	return nil
}

// GetConfig retrieves VM configuration
func (c *HTTPSClient) GetConfig(ctx context.Context) (*httpsclienttypes.GetConfigResponse, error) {
	url := fmt.Sprintf("https://%s/api/v1/config/get", c.vmEndpoint)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call GetConfig: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GetConfig failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success      bool   `json:"success"`
		ConfigJSON   string `json:"config_json"`
		ErrorMessage string `json:"error_message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return &httpsclienttypes.GetConfigResponse{
			Success:      false,
			ErrorMessage: result.ErrorMessage,
		}, nil
	}

	return &httpsclienttypes.GetConfigResponse{
		Success:    true,
		ConfigJSON: result.ConfigJSON,
	}, nil
}

// SyncCapabilities syncs camera capabilities to the VM.
// This method uses local DTO types defined in the HTTPS client types package
// to avoid a direct dependency on protobuf-generated messages.
func (c *HTTPSClient) SyncCapabilities(ctx context.Context, req *httpsclienttypes.SyncCapabilitiesRequest) (*httpsclienttypes.SyncCapabilitiesResponse, error) {
	url := fmt.Sprintf("https://%s/api/v1/capabilities/sync", c.vmEndpoint)

	reqBody := map[string]interface{}{
		"cameras":   req.Cameras,
		"synced_at": req.SyncedAt,
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to sync capabilities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sync capabilities failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success      bool   `json:"success"`
		ErrorMessage string `json:"error_message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &httpsclienttypes.SyncCapabilitiesResponse{
		Success:      result.Success,
		ErrorMessage: result.ErrorMessage,
	}, nil
}

// ReportDeploymentStatus reports deployment status to the VM
func (c *HTTPSClient) ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	url := fmt.Sprintf("https://%s/api/deployments/%s/status", c.vmEndpoint, deploymentID)

	requestBody := map[string]interface{}{
		"status":     status,
		"timestamp":  time.Now().Format(time.RFC3339),
		"model_path": modelPath,
		"error":      errorMessage,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Edge-ID", c.edgeID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("VM returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
