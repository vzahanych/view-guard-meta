package impl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service/types"
	"go.uber.org/zap"
)


// TestHTTPServer_Start_RequiresCertificates verifies that Start() fails with a clear error
// when certificate paths are missing.
func TestHTTPServer_Start_RequiresCertificates(t *testing.T) {
	logger := zap.NewNop()
	tunnelClient := &mockTunnelClient{} // Use existing mock from https-server_test.go

	tests := []struct {
		name          string
		serverCertPath string
		serverKeyPath  string
		caCertPath     string
		expectedErrorContains []string
	}{
		{
			name:          "all certificate paths missing",
			serverCertPath: "",
			serverKeyPath:  "",
			caCertPath:     "",
			expectedErrorContains: []string{"server_cert_path", "server_key_path", "ca_cert_path"},
		},
		{
			name:          "server cert path missing",
			serverCertPath: "",
			serverKeyPath:  "/test/server.key",
			caCertPath:     "/test/ca.crt",
			expectedErrorContains: []string{"server_cert_path"},
		},
		{
			name:          "server key path missing",
			serverCertPath: "/test/server.crt",
			serverKeyPath:  "",
			caCertPath:     "/test/ca.crt",
			expectedErrorContains: []string{"server_key_path"},
		},
		{
			name:          "CA cert path missing",
			serverCertPath: "/test/server.crt",
			serverKeyPath:  "/test/server.key",
			caCertPath:     "",
			expectedErrorContains: []string{"ca_cert_path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &httpsservertypes.HTTPServerConfig{
				ListenAddress:  "localhost:8443",
				ServerCertPath: tt.serverCertPath,
				ServerKeyPath:  tt.serverKeyPath,
				CACertPath:     tt.caCertPath,
			}

			server := NewHTTPServer(cfg, logger, "test-edge-id", tunnelClient, nil, nil, nil)
			ctx := context.Background()

			err := server.Start(ctx)

			require.Error(t, err, "Start() should fail when certificates are missing")
			assert.Contains(t, err.Error(), "TLS certificates are required", "Error should mention TLS certificates requirement")
			
			// Verify that all missing fields are mentioned in the error
			for _, field := range tt.expectedErrorContains {
				assert.Contains(t, err.Error(), field, "Error should mention missing field: %s", field)
			}
		})
	}
}

// TestHTTPServer_Start_RequiresCertificates_ErrorIsActionable verifies that the error message
// is actionable and provides clear guidance.
func TestHTTPServer_Start_RequiresCertificates_ErrorIsActionable(t *testing.T) {
	logger := zap.NewNop()
	tunnelClient := &mockTunnelClient{} // Use existing mock from https-server_test.go

	cfg := &httpsservertypes.HTTPServerConfig{
		ListenAddress:  "localhost:8443",
		ServerCertPath: "", // Missing
		ServerKeyPath:  "/test/server.key",
		CACertPath:     "/test/ca.crt",
	}

	server := NewHTTPServer(cfg, logger, "test-edge-id", tunnelClient, nil, nil, nil)
	ctx := context.Background()

	err := server.Start(ctx)

	require.Error(t, err, "Start() should fail")
	
	// Verify error message is actionable
	errMsg := err.Error()
	assert.Contains(t, errMsg, "TLS certificates are required", "Error should state requirement clearly")
	assert.Contains(t, errMsg, "server_cert_path", "Error should mention the missing field")
	assert.Contains(t, errMsg, "HTTPS server configuration", "Error should mention where to configure")
}

