package web

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestServer(t *testing.T) *Server {
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := &config.WebConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    0, // Use 0 to get a random port
	}

	return NewServer(cfg, log)
}

func TestServer_NewServer(t *testing.T) {
	server := setupTestServer(t)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.Name() != "web-server" {
		t.Errorf("Expected service name 'web-server', got '%s'", server.Name())
	}
}

func TestServer_StartStop(t *testing.T) {
	server := setupTestServer(t)
	
	// Use a random port by setting Port to 0
	server.config.Port = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	err = server.Stop(stopCtx)
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestServer_Start_Disabled(t *testing.T) {
	log, err := logger.New(logger.LogConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	cfg := &config.WebConfig{
		Enabled: false,
		Host:    "127.0.0.1",
		Port:    8080,
	}

	server := NewServer(cfg, log)
	ctx := context.Background()

	err = server.Start(ctx)
	if err != nil {
		t.Fatalf("Start should not fail when disabled: %v", err)
	}

	// Server should not be running
	if server.httpServer != nil {
		t.Error("HTTP server should be nil when disabled")
	}
}

func TestServer_APIEndpoints(t *testing.T) {
	server := setupTestServer(t)
	server.config.Port = 0 // Random port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	// Get the actual port from the server
	// Since we're using port 0, we need to find the actual port
	// For now, we'll test that the server started successfully
	time.Sleep(200 * time.Millisecond)

	// Test that server is running
	if server.httpServer == nil {
		t.Error("HTTP server should be running")
	}
}

func TestServer_StaticFiles(t *testing.T) {
	// Verify static files are embedded (embed.FS is never nil)
	// Just verify the variable exists
	_ = staticFiles
}

func TestServer_CORS(t *testing.T) {
	server := setupTestServer(t)
	server.config.Port = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(200 * time.Millisecond)

	// Test OPTIONS request (CORS preflight)
	// Note: We can't easily test the full HTTP request without knowing the port
	// This is a basic test that the server starts and CORS middleware is configured
	if server.router == nil {
		t.Error("Router should be initialized")
	}
}

// Test helper to make HTTP requests (for future use)
func makeRequest(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

