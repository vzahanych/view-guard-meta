package impl

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/types"
	tunnelclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/tunnel-client-service"
	"go.uber.org/zap"
)

// mockTunnelClient is a mock implementation of TunnelClientService for testing
type mockTunnelClient struct {
	connected     bool
	interfaceName string
	name          string
}

func (m *mockTunnelClient) Start(ctx context.Context) error {
	return nil
}

func (m *mockTunnelClient) Stop(ctx context.Context) error {
	return nil
}

func (m *mockTunnelClient) Name() string {
	if m.name == "" {
		return "none"
	}
	return m.name
}

func (m *mockTunnelClient) IsConnected() bool {
	return m.connected
}

func (m *mockTunnelClient) GetInterfaceName() string {
	return m.interfaceName
}

func (m *mockTunnelClient) GetEndpoint() string {
	return ""
}

// newTestHTTPSClient creates an HTTPSClient configured for localhost testing
// serverURL should be in format "localhost:port" or "127.0.0.1:port"
func newTestHTTPSClient(t *testing.T, serverURL string, tunnelClient tunnelclient.TunnelClientService) *HTTPSClient {
	logger := zap.NewNop()
	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            serverURL,
		AllowInsecureLocalhost: true, // Enable insecure mode for localhost testing
		ClientCertPath:        "",    // Empty for localhost dev mode
		ClientKeyPath:         "",
		CACertPath:            "",
		Timeout:               5 * time.Second,
	}

	client, err := NewHTTPSClient(cfg, tunnelClient, "test-edge-id", nil, logger)
	require.NoError(t, err)
	require.NotNil(t, client)

	return client
}

// TestHTTPSClient_Start_NoDeadlockOnAuthFailure verifies that Start() does not deadlock
// when authentication fails. This test specifically addresses the deadlock bug where
// Start() held a lock while calling Authenticate(), which then tried to lock again.
func TestHTTPSClient_Start_NoDeadlockOnAuthFailure(t *testing.T) {
	// Create a test HTTPS server that returns authentication failure
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/authenticate" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "authentication failed",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// Use HTTPS server with TLS config that allows InsecureSkipVerify (for testing)
	server := httptest.NewTLSServer(authHandler)
	defer server.Close()
	server.TLS.InsecureSkipVerify = true // Allow self-signed cert for testing

	// Extract port from server URL
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	serverURL := "localhost:" + port

	// Create client with nil tunnel (localhost mode)
	client := newTestHTTPSClient(t, serverURL, nil)

	// Set a short timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// This should not deadlock - the fix removed the lock around Authenticate()
	// If it deadlocks, the test will timeout
	done := make(chan error, 1)
	go func() {
		done <- client.Start(ctx)
	}()

	select {
	case err := <-done:
		// Start() should return without error (authentication failure is logged but doesn't fail startup)
		assert.NoError(t, err, "Start() should not return error even if auth fails")
		// Verify that authentication state was set correctly (in localhost mode, IsConnected checks authenticated)
		assert.False(t, client.IsConnected(), "Should not be connected after auth failure")
	case <-time.After(5 * time.Second):
		t.Fatal("Start() deadlocked or took too long - this indicates the deadlock bug still exists")
	}
}

// TestHTTPSClient_Start_NoDeadlockOnAuthSuccess verifies that Start() does not deadlock
// when authentication succeeds.
func TestHTTPSClient_Start_NoDeadlockOnAuthSuccess(t *testing.T) {
	// Create a test HTTPS server that returns authentication success
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/authenticate" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"edge_id": "test-edge-id",
				"message": "authenticated",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// Use HTTPS server with TLS config that allows InsecureSkipVerify (for testing)
	server := httptest.NewTLSServer(authHandler)
	defer server.Close()

	// Extract port from server URL
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	serverURL := "localhost:" + port

	// Create client with nil tunnel (localhost mode)
	client := newTestHTTPSClient(t, serverURL, nil)

	ctx := context.Background()

	// This should not deadlock
	done := make(chan error, 1)
	go func() {
		done <- client.Start(ctx)
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "Start() should succeed when authentication succeeds")
		// Verify that authentication state was set correctly (in localhost mode, IsConnected checks authenticated)
		assert.True(t, client.IsConnected(), "Should be connected after successful auth")
	case <-time.After(5 * time.Second):
		t.Fatal("Start() deadlocked or took too long - this indicates the deadlock bug still exists")
	}
}

// TestHTTPSClient_Start_ConcurrentCalls verifies that concurrent calls to Start()
// don't cause issues. This tests the lock-free approach doesn't introduce race conditions.
func TestHTTPSClient_Start_ConcurrentCalls(t *testing.T) {
	// Create a test HTTPS server
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/authenticate" && r.Method == http.MethodPost {
			time.Sleep(10 * time.Millisecond) // Small delay to increase chance of race
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"edge_id": "test-edge-id",
				"message": "authenticated",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewTLSServer(authHandler)
	defer server.Close()

	// Extract port from server URL
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	serverURL := "localhost:" + port

	client := newTestHTTPSClient(t, serverURL, nil)
	ctx := context.Background()

	// Call Start() concurrently multiple times
	const numCalls = 5
	errors := make(chan error, numCalls)

	for i := 0; i < numCalls; i++ {
		go func() {
			errors <- client.Start(ctx)
		}()
	}

	// All calls should complete without deadlock
	timeout := time.After(5 * time.Second)
	for i := 0; i < numCalls; i++ {
		select {
		case err := <-errors:
			assert.NoError(t, err, "Concurrent Start() call should not error")
		case <-timeout:
			t.Fatal("Concurrent Start() calls deadlocked or timed out")
		}
	}
}

// TestHTTPSClient_Start_WithTunnelClient verifies Start() works correctly when
// a tunnel client is provided. However, since we're using localhost endpoint,
// the client will still run in localhost mode (tunnel is ignored).
// Note: This test is skipped because it requires proper certificate setup for tunnel mode.
func TestHTTPSClient_Start_WithTunnelClient(t *testing.T) {
	t.Skip("Skipping test that requires tunnel certificates - localhost endpoint already tested in other tests")
	// Create mock tunnel client that is connected
	tunnelClient := &mockTunnelClient{
		connected:     true,
		interfaceName: "wg0",
		name:          "wireguard-client",
	}

	// Create a test HTTPS server
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/authenticate" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"edge_id": "test-edge-id",
				"message": "authenticated",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewTLSServer(authHandler)
	defer server.Close()

	// Extract port from server URL
	u, parseErr := url.Parse(server.URL)
	require.NoError(t, parseErr)
	_, port, splitErr := net.SplitHostPort(u.Host)
	require.NoError(t, splitErr)
	serverURL := "localhost:" + port

	// Even with tunnel client, localhost endpoint means localhost mode
	client := newTestHTTPSClient(t, serverURL, tunnelClient)
	ctx := context.Background()

	err := client.Start(ctx)
	require.NoError(t, err, "Start() should succeed with localhost endpoint")
	// In localhost mode, IsConnected checks authenticated flag
	assert.True(t, client.IsConnected(), "Should be connected after successful auth")
}

// TestHTTPSClient_Start_TunnelNotConnected tests the scenario where tunnel is not connected.
// In this case, Start() should wait for tunnel connection or timeout.
// Note: This requires a non-localhost endpoint to trigger tunnel mode.
func TestHTTPSClient_Start_TunnelNotConnected(t *testing.T) {
	t.Skip("Skipping test that requires tunnel certificates - would need proper cert setup")
	
	// Create mock tunnel client that is NOT connected
	tunnelClient := &mockTunnelClient{
		connected:     false,
		interfaceName: "wg0",
		name:          "wireguard-client",
	}

	logger := zap.NewNop()
	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:    "10.0.0.1:8443", // Non-localhost endpoint
		ClientCertPath: "",
		ClientKeyPath:  "",
		CACertPath:     "",
		Timeout:        5 * time.Second,
	}

	client, err := NewHTTPSClient(cfg, tunnelClient, "test-edge-id", nil, logger)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start() should timeout waiting for tunnel connection
	err = client.Start(ctx)
	assert.Error(t, err, "Start() should error when tunnel is not connected")
	assert.Contains(t, err.Error(), "tunnel not connected", "Error should mention tunnel not connected")
}

// TestHTTPSClient_Start_LocalhostMode verifies that Start() works correctly
// in localhost mode (no tunnel required).
func TestHTTPSClient_Start_LocalhostMode(t *testing.T) {
	// Create a test HTTPS server
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/authenticate" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"edge_id": "test-edge-id",
				"message": "authenticated",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewTLSServer(authHandler)
	defer server.Close()

	// Extract port from server URL
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	serverURL := "localhost:" + port

	// Create client with nil tunnel (localhost mode) and localhost endpoint
	logger := zap.NewNop()
	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            serverURL,
		AllowInsecureLocalhost: true, // Enable insecure mode for localhost testing
		ClientCertPath:        "",
		ClientKeyPath:         "",
		CACertPath:            "",
		Timeout:               5 * time.Second,
	}

	client, err := NewHTTPSClient(cfg, nil, "test-edge-id", nil, logger)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx := context.Background()

	err = client.Start(ctx)
	require.NoError(t, err, "Start() should succeed in localhost mode")
	// In localhost mode, IsConnected checks authenticated flag
	assert.True(t, client.IsConnected(), "Should be connected after successful auth")
}

// TestHTTPSClient_Authenticate_UpdatesState verifies that Authenticate() correctly
// updates the authenticated state and lastAuthError, and that these updates are
// thread-safe (handled with proper locking).
func TestHTTPSClient_Authenticate_UpdatesState(t *testing.T) {
	// Test authentication success
	t.Run("success updates authenticated state", func(t *testing.T) {
		authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"edge_id": "test-edge-id",
				"message": "authenticated",
			})
		})

		server := httptest.NewTLSServer(authHandler)
		defer server.Close()

		u, parseErr := url.Parse(server.URL)
		require.NoError(t, parseErr)
		_, port, splitErr := net.SplitHostPort(u.Host)
		require.NoError(t, splitErr)
		serverURL := "localhost:" + port

		client := newTestHTTPSClient(t, serverURL, nil)
		ctx := context.Background()

		err := client.Authenticate(ctx, "test-edge-id")
		require.NoError(t, err)
		// In localhost mode, IsConnected checks authenticated flag
		assert.True(t, client.IsConnected(), "Should be connected after successful auth")
	})

	// Test authentication failure
	t.Run("failure updates authenticated state", func(t *testing.T) {
		authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "authentication failed",
			})
		})

		server := httptest.NewTLSServer(authHandler)
		defer server.Close()

		u, parseErr := url.Parse(server.URL)
		require.NoError(t, parseErr)
		_, port, splitErr := net.SplitHostPort(u.Host)
		require.NoError(t, splitErr)
		serverURL := "localhost:" + port

		client := newTestHTTPSClient(t, serverURL, nil)
		ctx := context.Background()

		err := client.Authenticate(ctx, "test-edge-id")
		require.Error(t, err, "Authentication should fail")
		// In localhost mode, IsConnected checks authenticated flag
		assert.False(t, client.IsConnected(), "Should not be connected after auth failure")
	})
}

