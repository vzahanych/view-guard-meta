package tunnelclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

func TestGetTunnelFactory(t *testing.T) {
	tests := []struct {
		name     string
		provider types.TunnelProvider
		wantNil  bool
	}{
		{
			name:     "WireGuard provider returns factory",
			provider: types.TunnelProviderWireGuard,
			wantNil:  false,
		},
		{
			name:     "None provider returns nil",
			provider: types.TunnelProviderNone,
			wantNil:  true,
		},
		{
			name:     "Unknown provider returns nil",
			provider: types.TunnelProvider("unknown"),
			wantNil:  true,
		},
		{
			name:     "OpenVPN provider returns nil (not yet implemented)",
			provider: types.TunnelProviderOpenVPN,
			wantNil:  true,
		},
		{
			name:     "IPSec provider returns nil (not yet implemented)",
			provider: types.TunnelProviderIPSec,
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := GetTunnelFactory(tt.provider)
			if tt.wantNil {
				assert.Nil(t, factory, "Expected nil factory for provider %s", tt.provider)
			} else {
				assert.NotNil(t, factory, "Expected non-nil factory for provider %s", tt.provider)
				if factory != nil {
					assert.Equal(t, tt.provider, factory.Provider(), "Factory should return correct provider")
				}
			}
		})
	}
}

func TestWireGuardFactory_Provider(t *testing.T) {
	factory := &wireguardFactory{}
	assert.Equal(t, types.TunnelProviderWireGuard, factory.Provider())
}

func TestNewTunnelClientService(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		cfg         *types.TunnelConfig
		expectNil   bool
		expectError bool
		errorMsg    string
	}{
		{
			name: "Nil config returns nil (no error)",
			cfg:  nil,
			expectNil: true,
			expectError: false,
		},
		{
			name: "Disabled tunnel returns nil",
			cfg: &types.TunnelConfig{
				Enabled:  false,
				Provider: types.TunnelProviderWireGuard,
			},
			expectNil: true,
			expectError: false,
		},
		{
			name: "None provider returns nil",
			cfg: &types.TunnelConfig{
				Enabled:  true,
				Provider: types.TunnelProviderNone,
			},
			expectNil: true,
			expectError: false,
		},
		{
			name: "Valid WireGuard config creates service",
			cfg: &types.TunnelConfig{
				Enabled:      true,
				Provider:     types.TunnelProviderWireGuard,
				KVMEndpoint:  "10.0.0.1:51820",
				InterfaceName: "wg0",
				RawConfig: map[string]interface{}{
					"config_path": "/test/wg0.conf",
				},
			},
			expectNil: false,
			expectError: false,
		},
		{
			name: "Unknown provider returns error",
			cfg: &types.TunnelConfig{
				Enabled:  true,
				Provider: types.TunnelProvider("unknown"),
			},
			expectNil: true,
			expectError: true,
			errorMsg:    "unsupported tunnel provider",
		},
		{
			name: "OpenVPN provider returns error (not yet implemented)",
			cfg: &types.TunnelConfig{
				Enabled:  true,
				Provider: types.TunnelProviderOpenVPN,
			},
			expectNil: true,
			expectError: true,
			errorMsg:    "unsupported tunnel provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewTunnelClientService(tt.cfg, logger)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, service)
			} else {
				require.NoError(t, err)
				if tt.expectNil {
					assert.Nil(t, service)
				} else {
					assert.NotNil(t, service)
					assert.Implements(t, (*types.TunnelClientService)(nil), service)
					assert.Equal(t, "wireguard-client", service.Name())
				}
			}
		})
	}
}

func TestWireGuardFactory_CreateService(t *testing.T) {
	factory := &wireguardFactory{}
	logger := zap.NewNop()

	cfg := &types.TunnelConfig{
		Enabled:      true,
		Provider:     types.TunnelProviderWireGuard,
		KVMEndpoint:  "10.0.0.1:51820",
		InterfaceName: "wg0",
		RawConfig: map[string]interface{}{
			"config_path": "/test/wg0.conf",
		},
	}

	service, err := factory.CreateService(cfg, logger)

	require.NoError(t, err)
	require.NotNil(t, service)
	assert.Equal(t, "wireguard-client", service.Name())
	assert.Implements(t, (*types.TunnelClientService)(nil), service)
}

