package impl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/types"
	"go.uber.org/zap"
)

// mockTunnelClientForInsecure is a mock tunnel client that always returns "none" (disabled)
type mockTunnelClientForInsecure struct{}

func (m *mockTunnelClientForInsecure) Start(ctx context.Context) error {
	return nil
}

func (m *mockTunnelClientForInsecure) Stop(ctx context.Context) error {
	return nil
}

func (m *mockTunnelClientForInsecure) Name() string {
	return "none"
}

func (m *mockTunnelClientForInsecure) IsConnected() bool {
	return false
}

func (m *mockTunnelClientForInsecure) GetInterfaceName() string {
	return ""
}

func (m *mockTunnelClientForInsecure) GetEndpoint() string {
	return ""
}

// TestNewHTTPSClient_InsecureNotAllowed_RequiresCertificates verifies that the constructor
// fails when insecure is not allowed and certificates are missing.
func TestNewHTTPSClient_InsecureNotAllowed_RequiresCertificates(t *testing.T) {
	logger := zap.NewNop()
	tunnelClient := &mockTunnelClientForInsecure{}

	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            "localhost:8443",
		AllowInsecureLocalhost: false, // Explicitly false (default)
		ClientCertPath:        "",      // Missing
		ClientKeyPath:         "",
		CACertPath:            "",
	}

	client, err := NewHTTPSClient(cfg, tunnelClient, "test-edge-id", nil, logger)

	require.Error(t, err, "Constructor should fail when certificates are missing and insecure is not allowed")
	assert.Nil(t, client, "Client should be nil on error")
	assert.Contains(t, err.Error(), "TLS certificates are required", "Error should mention TLS certificates requirement")
	assert.Contains(t, err.Error(), "client_cert_path", "Error should mention missing field")
}

// TestNewHTTPSClient_InsecureAllowed_LocalhostEndpoint_Succeeds verifies that the constructor
// succeeds when insecure is allowed and endpoint is localhost, even without certificates.
func TestNewHTTPSClient_InsecureAllowed_LocalhostEndpoint_Succeeds(t *testing.T) {
	logger := zap.NewNop()
	tunnelClient := &mockTunnelClientForInsecure{}

	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            "localhost:8443",
		AllowInsecureLocalhost: true,
		ClientCertPath:        "", // Optional when insecure allowed
		ClientKeyPath:         "",
		CACertPath:            "",
	}

	client, err := NewHTTPSClient(cfg, tunnelClient, "test-edge-id", nil, logger)

	require.NoError(t, err, "Constructor should succeed when insecure is allowed for localhost")
	assert.NotNil(t, client, "Client should be created successfully")
}

// TestNewHTTPSClient_InsecureAllowed_NonLocalhostEndpoint_Fails verifies that the constructor
// fails when insecure is allowed but endpoint is not localhost.
func TestNewHTTPSClient_InsecureAllowed_NonLocalhostEndpoint_Fails(t *testing.T) {
	logger := zap.NewNop()
	tunnelClient := &mockTunnelClientForInsecure{}

	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            "10.0.0.1:8443", // Non-localhost
		AllowInsecureLocalhost: true,
		ClientCertPath:        "",
		ClientKeyPath:         "",
		CACertPath:            "",
	}

	client, err := NewHTTPSClient(cfg, tunnelClient, "test-edge-id", nil, logger)

	require.Error(t, err, "Constructor should fail when insecure is allowed but endpoint is not localhost")
	assert.Nil(t, client, "Client should be nil on error")
	assert.Contains(t, err.Error(), "allow_insecure_localhost can only be enabled for localhost endpoints", "Error should mention localhost requirement")
	assert.Contains(t, err.Error(), "10.0.0.1:8443", "Error should mention the invalid endpoint")
}

// TestNewHTTPSClient_InsecureAllowed_127_0_0_1_Succeeds verifies that the constructor
// succeeds when insecure is allowed and endpoint is 127.0.0.1.
func TestNewHTTPSClient_InsecureAllowed_127_0_0_1_Succeeds(t *testing.T) {
	logger := zap.NewNop()
	tunnelClient := &mockTunnelClientForInsecure{}

	cfg := &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            "127.0.0.1:8443",
		AllowInsecureLocalhost: true,
		ClientCertPath:        "", // Optional when insecure allowed
		ClientKeyPath:         "",
		CACertPath:            "",
	}

	client, err := NewHTTPSClient(cfg, tunnelClient, "test-edge-id", nil, logger)

	require.NoError(t, err, "Constructor should succeed when insecure is allowed for 127.0.0.1")
	assert.NotNil(t, client, "Client should be created successfully")
}

