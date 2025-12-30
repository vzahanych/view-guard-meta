package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVMGatewayConfig_Validate(t *testing.T) {
	validConfig := func() *VMGatewayConfig {
		return &VMGatewayConfig{
			TransportProvider: TransportProviderHTTP,
			EdgeID:            "test-edge",
			HTTPServerConfig: HTTPServerConfig{
				ListenAddress:  "localhost:8443",
				ServerCertPath: "/test/server.crt",
				ServerKeyPath:  "/test/server.key",
				CACertPath:     "/test/ca.crt",
			},
			HTTPSClientConfig: HTTPSClientConfig{
				VMEndpoint:     "localhost:8443",
				ClientCertPath: "/test/client.crt",
				ClientKeyPath:  "/test/client.key",
				CACertPath:     "/test/ca.crt",
			},
		}
	}

	tests := []struct {
		name        string
		config      func() *VMGatewayConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid config with tunnel disabled (localhost mode)",
			config:      validConfig,
			expectError: false,
		},
		{
			name: "Valid config with tunnel enabled",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ListenAddress = "10.0.0.2:8443"
				cfg.HTTPSClientConfig.VMEndpoint = "10.0.0.1:8443"
				cfg.Tunnel = TunnelConfig{
					Enabled:     true,
					Provider:    TunnelProviderWireGuard,
					KVMEndpoint: "10.0.0.1:51820",
					InterfaceName: "wg0",
					RawConfig: map[string]interface{}{
						"config_path": "/test/wg0.conf",
					},
				}
				return cfg
			},
			expectError: false,
		},
		{
			name: "Missing transport provider",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.TransportProvider = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "transport_provider is required",
		},
		{
			name: "Unsupported transport provider",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.TransportProvider = TransportProviderGRPC
				return cfg
			},
			expectError: true,
			errorMsg:    "unsupported transport provider",
		},
		{
			name: "Missing edge ID",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.EdgeID = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "edge_id is required",
		},
		{
			name: "Tunnel disabled but non-localhost server address",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ListenAddress = "10.0.0.2:8443"
				return cfg
			},
			expectError: true,
			errorMsg:    "tunnel is disabled but configuration is not in localhost mode",
		},
		{
			name: "Tunnel disabled but non-localhost client endpoint",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.VMEndpoint = "10.0.0.1:8443"
				return cfg
			},
			expectError: true,
			errorMsg:    "tunnel is disabled but configuration is not in localhost mode",
		},
		{
			name: "Tunnel enabled but missing kvm_endpoint",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ListenAddress = "10.0.0.2:8443"
				cfg.HTTPSClientConfig.VMEndpoint = "10.0.0.1:8443"
				cfg.Tunnel = TunnelConfig{
					Enabled:  true,
					Provider: TunnelProviderWireGuard,
					// Missing KVMEndpoint
				}
				return cfg
			},
			expectError: true,
			errorMsg:    "tunnel.kvm_endpoint is required when tunnel is enabled",
		},
		{
			name: "Missing server cert path",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ServerCertPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_server_config.server_cert_path is required",
		},
		{
			name: "Missing server key path",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ServerKeyPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_server_config.server_key_path is required",
		},
		{
			name: "Missing server CA cert path",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.CACertPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_server_config.ca_cert_path is required",
		},
		{
			name: "Missing client endpoint",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.VMEndpoint = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_client_config.vm_endpoint is required",
		},
		{
			name: "Missing client cert path",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.ClientCertPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_client_config.client_cert_path is required",
		},
		{
			name: "Missing client key path",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.ClientKeyPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_client_config.client_key_path is required",
		},
		{
			name: "Missing client CA cert path",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.CACertPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_client_config.ca_cert_path is required",
		},
		{
			name: "Empty server address defaults to localhost (valid)",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ListenAddress = ""
				return cfg
			},
			expectError: false,
		},
		{
			name: "Empty client endpoint is invalid (required field)",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.VMEndpoint = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "https_client_config.vm_endpoint is required",
		},
		{
			name: "127.0.0.1 server address is valid for localhost mode",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPServerConfig.ListenAddress = "127.0.0.1:8443"
				return cfg
			},
			expectError: false,
		},
		{
			name: "127.0.0.1 client endpoint is valid for localhost mode",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.VMEndpoint = "127.0.0.1:8443"
				return cfg
			},
			expectError: false,
		},
		{
			name: "Valid config with AllowInsecureLocalhost=true and localhost endpoint (certificates optional)",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.AllowInsecureLocalhost = true
				cfg.HTTPSClientConfig.VMEndpoint = "localhost:8443"
				cfg.HTTPSClientConfig.ClientCertPath = "" // Optional when AllowInsecureLocalhost=true
				cfg.HTTPSClientConfig.ClientKeyPath = ""
				cfg.HTTPSClientConfig.CACertPath = ""
				return cfg
			},
			expectError: false,
		},
		{
			name: "Invalid: AllowInsecureLocalhost=true but non-localhost endpoint",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.AllowInsecureLocalhost = true
				cfg.HTTPSClientConfig.VMEndpoint = "10.0.0.1:8443" // Non-localhost
				// Enable tunnel to allow non-localhost endpoint (otherwise earlier validation fails)
				cfg.Tunnel = TunnelConfig{
					Enabled:     true,
					Provider:    TunnelProviderWireGuard,
					KVMEndpoint: "10.0.0.1:51820",
					InterfaceName: "wg0",
					RawConfig: map[string]interface{}{
						"config_path": "/test/wg0.conf",
					},
				}
				cfg.HTTPServerConfig.ListenAddress = "10.0.0.2:8443"
				return cfg
			},
			expectError: true,
			errorMsg:    "allow_insecure_localhost can only be enabled for localhost endpoints",
		},
		{
			name: "Invalid: AllowInsecureLocalhost=false (default) and missing certificates",
			config: func() *VMGatewayConfig {
				cfg := validConfig()
				cfg.HTTPSClientConfig.AllowInsecureLocalhost = false // Explicitly false (default)
				cfg.HTTPSClientConfig.ClientCertPath = ""
				return cfg
			},
			expectError: true,
			errorMsg:    "client_cert_path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config()
			err := cfg.Validate()

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVMGatewayConfig_GetTunnelConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *VMGatewayConfig
		wantNil  bool
		wantType string
	}{
		{
			name: "No tunnel config returns nil",
			config: &VMGatewayConfig{
				Tunnel: TunnelConfig{},
			},
			wantNil: true,
		},
		{
			name: "Tunnel with provider returns config",
			config: &VMGatewayConfig{
				Tunnel: TunnelConfig{
					Provider: TunnelProviderWireGuard,
				},
			},
			wantNil: false,
		},
		{
			name: "Tunnel with enabled returns config",
			config: &VMGatewayConfig{
				Tunnel: TunnelConfig{
					Enabled: true,
				},
			},
			wantNil: false,
		},
		{
			name: "Tunnel with provider and enabled returns config",
			config: &VMGatewayConfig{
				Tunnel: TunnelConfig{
					Provider: TunnelProviderWireGuard,
					Enabled:  true,
				},
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config.GetTunnelConfig()
			if tt.wantNil {
				assert.Nil(t, cfg)
			} else {
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestVMGatewayConfig_GetTransportProvider(t *testing.T) {
	tests := []struct {
		name     string
		config   *VMGatewayConfig
		expected TransportProvider
	}{
		{
			name: "HTTP provider returns HTTP",
			config: &VMGatewayConfig{
				TransportProvider: TransportProviderHTTP,
			},
			expected: TransportProviderHTTP,
		},
		{
			name: "Empty provider returns None",
			config: &VMGatewayConfig{
				TransportProvider: "",
			},
			expected: TransportProviderNone,
		},
		{
			name: "GRPC provider returns GRPC",
			config: &VMGatewayConfig{
				TransportProvider: TransportProviderGRPC,
			},
			expected: TransportProviderGRPC,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.config.GetTransportProvider()
			assert.Equal(t, tt.expected, actual)
		})
	}
}

