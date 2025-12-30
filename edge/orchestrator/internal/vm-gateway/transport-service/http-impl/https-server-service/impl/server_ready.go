package impl

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// IsServerReady checks if the HTTPS server is ready by attempting to connect to it.
// Returns true if the server is listening and accepting connections.
func (s *HTTPServer) IsServerReady() bool {
	s.mu.RLock()
	listener := s.listener
	httpServer := s.httpServer
	s.mu.RUnlock()

	if listener == nil || httpServer == nil {
		return false
	}

	// Get the listen address
	listenAddr := httpServer.Addr
	if listenAddr == "" {
		return false
	}

	// Try to connect to the server
	conn, err := net.DialTimeout("tcp", listenAddr, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// WaitForServerReady waits for the HTTPS server to become ready.
// Returns an error if the server doesn't become ready within the timeout.
func (s *HTTPServer) WaitForServerReady(ctx context.Context, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 10 * time.Second // Default: 10 seconds
	}

	deadline := time.Now().Add(timeout)
	checkInterval := 100 * time.Millisecond

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if server is ready
		if s.IsServerReady() {
			s.logger.Info("HTTPS server is ready", zap.String("address", s.httpServer.Addr))
			return nil
		}

		// Check timeout
		if time.Now().After(deadline) {
			return fmt.Errorf("HTTPS server did not become ready within %v", timeout)
		}

		// Wait before next check
		time.Sleep(checkInterval)
	}
}

// CheckServerReadiness performs a readiness check by making an HTTP request to the readiness endpoint.
// This is more reliable than just checking if the port is open.
// Note: This method is available for future use but currently not called.
func (s *HTTPServer) CheckServerReadiness(ctx context.Context) error {
	s.mu.RLock()
	httpServer := s.httpServer
	s.mu.RUnlock()

	if httpServer == nil {
		return fmt.Errorf("HTTPS server is not initialized")
	}

	// Get the listen address
	listenAddr := httpServer.Addr
	if listenAddr == "" {
		return fmt.Errorf("HTTPS server address is not set")
	}

	// Create a simple HTTP client with timeout
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // For readiness check only
			},
		},
	}

	// Try to connect to the readiness endpoint
	url := fmt.Sprintf("https://%s/api/health/ready", listenAddr)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create readiness request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("readiness check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness check returned status %d", resp.StatusCode)
	}

	return nil
}

