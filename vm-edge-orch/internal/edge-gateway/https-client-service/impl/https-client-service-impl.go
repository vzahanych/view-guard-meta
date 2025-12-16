package impl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/https-client-service"
)

// httpsClientService implements the HTTPSClientService interface
type httpsClientService struct {
	wgServer interface{} // WireGuard server (optional, for getting Edge IPs)
	client   *http.Client
	mu       sync.RWMutex
	logger   *zap.Logger // Simple logger, can be nil
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewHTTPSClientService creates a new HTTPS client service implementation.
// wgServer is interface{} to avoid dependencies on non-existent packages.
// The client is used for VM → Edge communication over WireGuard.
func NewHTTPSClientService(wgServer interface{}, log interface{}) (httpsclient.HTTPSClientService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Try to extract logger if it's a zap.Logger
	var logger *zap.Logger
	if zapLogger, ok := log.(*zap.Logger); ok {
		logger = zapLogger
	} else {
		// Create a simple logger if none provided
		logger, _ = zap.NewDevelopment()
	}

	// Load client certificate for mTLS
	clientCertPath := "/etc/ssl/certs/vm-client.crt"
	clientKeyPath := "/etc/ssl/private/vm-client.key"
	caCertPath := "/etc/ssl/certs/ca.crt"

	// Load client certificate
	cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		// If certificates don't exist, create a client without mTLS (for development)
		if logger != nil {
			logger.Warn("Failed to load client certificate, creating client without mTLS",
				zap.String("cert_path", clientCertPath),
				zap.Error(err))
		}
	}

	// Load CA certificate for server verification
	var caCertPool *x509.CertPool
	if caCertPath != "" {
		caCertPool, err = loadCACertificate(caCertPath)
		if err != nil && logger != nil {
			logger.Warn("Failed to load CA certificate", zap.Error(err))
		}
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if cert.Certificate != nil && len(cert.Certificate) > 0 {
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if caCertPool != nil {
		tlsConfig.RootCAs = caCertPool
	}

	// Create HTTP client with custom transport for WireGuard routing
	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	service := &httpsClientService{
		wgServer: wgServer,
		client:   client,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
	}

	return service, nil
}

// Name returns the service name
func (c *httpsClientService) Name() string {
	return "https-client"
}

// Start starts the HTTPS client service
func (c *httpsClientService) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.logger != nil {
		c.logger.Info("Starting HTTPS client service")
	}

	// Client is ready to use immediately
	// No background goroutines needed for a client

	if c.logger != nil {
		c.logger.Info("HTTPS client service started")
	}

	return nil
}

// Stop stops the HTTPS client service
func (c *httpsClientService) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.logger != nil {
		c.logger.Info("Stopping HTTPS client service")
	}

	c.cancel()

	// Close idle connections
	if c.client != nil && c.client.Transport != nil {
		if transport, ok := c.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	if c.logger != nil {
		c.logger.Info("HTTPS client service stopped")
	}

	return nil
}

// GetClient returns the underlying HTTP client for making requests.
// This is a helper method for making requests to Edge devices.
func (c *httpsClientService) GetClient() *http.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// RequestEdge makes an HTTPS request to a specific Edge device.
// edgeIP should be the WireGuard IP of the Edge (e.g., "10.0.0.2").
// This is a helper method for convenience.
func (c *httpsClientService) RequestEdge(ctx context.Context, edgeIP string, method, path string, body []byte) (*http.Response, error) {
	client := c.GetClient()
	if client == nil {
		return nil, fmt.Errorf("HTTP client not available")
	}

	// Construct URL using Edge's WireGuard IP
	url := fmt.Sprintf("https://%s:8443%s", edgeIP, path)

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Body = http.NoBody // Simplified - body handling can be added later
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	return resp, nil
}

// loadCACertificate loads CA certificate for server certificate verification
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

