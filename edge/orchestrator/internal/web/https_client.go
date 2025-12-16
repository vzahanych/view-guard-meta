package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// HTTPSClient manages HTTPS connections to KVM VM over WireGuard tunnel
// Replaces gRPC client for Edge → VM communication
type HTTPSClient struct {
	*service.ServiceBase
	config     *config.WireGuardConfig
	wgClient   *wireguard.Client
	logger     *logger.Logger
	httpClient *http.Client
	mu         sync.RWMutex
	vmEndpoint string // VM HTTPS endpoint (10.0.0.1:8443)
	edgeID     string // Edge ID for authentication
}

// NewHTTPSClient creates a new HTTPS client for Edge → VM calls
func NewHTTPSClient(cfg *config.WireGuardConfig, wgClient *wireguard.Client, edgeID string, log *logger.Logger) (*HTTPSClient, error) {
	// Load TLS client certificate for mTLS
	clientCertPath := "/etc/ssl/certs/edge-client.crt"
	clientKeyPath := "/etc/ssl/private/edge-client.key"
	caCertPath := "/etc/ssl/certs/ca.crt"
	// When connecting via IP address (10.0.0.1) through WireGuard tunnel,
	// Go's TLS will verify against IP SANs in the certificate.
	// The VM server certificate includes IP.2 = 10.0.0.1 in SANs.
	// Setting ServerName to empty allows IP-based verification.
	serverName := "" // Empty for IP-based verification (certificate has IP.2 = 10.0.0.1 in SANs)

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

	// Configure TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName, // Empty for IP-based verification (10.0.0.1 matches IP.2 in certificate SANs)
	}

	// Create HTTP client with TLS config
	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &HTTPSClient{
		ServiceBase: service.NewServiceBase("https-client", log),
		config:      cfg,
		wgClient:    wgClient,
		logger:      log,
		httpClient:  httpClient,
		vmEndpoint:  "10.0.0.1:8443", // VM WireGuard IP and HTTPS port
		edgeID:      edgeID,
	}, nil
}

// Start starts the HTTPS client service
func (c *HTTPSClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.config.Enabled {
		c.LogInfo("HTTPS client disabled (WireGuard disabled)")
		return nil
	}

	c.GetStatus().SetStatus(service.StatusStarting)

	// Wait for WireGuard to be connected
	if !c.wgClient.IsConnected() {
		c.LogInfo("Waiting for WireGuard connection...")
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
					c.LogInfo("WireGuard connected, HTTPS client ready")
					c.GetStatus().SetStatus(service.StatusRunning)
					return nil
				}
			}
		}
	}

	c.GetStatus().SetStatus(service.StatusRunning)
	c.LogInfo("HTTPS client started", "vm_endpoint", c.vmEndpoint)

	// Authenticate with VM when HTTPS client starts (after WireGuard is connected)
	// This sets the connection state to "connected" in both VM and Edge
	go func() {
		// Wait a moment for everything to be ready
		time.Sleep(2 * time.Second)

		// Send authentication request using stored edge ID
		if err := c.Authenticate(ctx, c.edgeID); err != nil {
			c.LogError("Failed to authenticate with VM", err)
			// Don't fail startup - will retry on next heartbeat or can be retried
		} else {
			c.LogInfo("Successfully authenticated with VM", "edge_id", c.edgeID)
		}
	}()

	return nil
}

// Stop stops the HTTPS client service
func (c *HTTPSClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetStatus().SetStatus(service.StatusStopped)
	c.LogInfo("HTTPS client stopped")

	return nil
}

// IsConnected returns true if WireGuard is connected (HTTPS client is always ready when WireGuard is up)
func (c *HTTPSClient) IsConnected() bool {
	return c.wgClient != nil && c.wgClient.IsConnected()
}

// Heartbeat sends a heartbeat to the VM
func (c *HTTPSClient) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error {
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
func (c *HTTPSClient) SendTelemetry(ctx context.Context, data *edge.TelemetryData) error {
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
func (c *HTTPSClient) SendEvents(ctx context.Context, events []*edge.Event) error {
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
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
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
		return fmt.Errorf("authentication not successful: %s", result.Message)
	}

	return nil
}

// GetConfig retrieves VM configuration
func (c *HTTPSClient) GetConfig(ctx context.Context) (*edge.GetConfigResponse, error) {
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
		return &edge.GetConfigResponse{
			Success:      false,
			ErrorMessage: result.ErrorMessage,
		}, nil
	}

	return &edge.GetConfigResponse{
		Success:    true,
		ConfigJson: result.ConfigJSON,
	}, nil
}

// SyncCapabilities syncs camera capabilities to the VM
func (c *HTTPSClient) SyncCapabilities(ctx context.Context, req *edge.SyncCapabilitiesRequest) (*edge.SyncCapabilitiesResponse, error) {
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

	return &edge.SyncCapabilitiesResponse{
		Success:      result.Success,
		ErrorMessage: result.ErrorMessage,
	}, nil
}
