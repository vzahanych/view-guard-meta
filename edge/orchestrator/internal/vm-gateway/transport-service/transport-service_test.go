package transport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

func TestGetTransportFactory(t *testing.T) {
	tests := []struct {
		name     string
		provider types.TransportProvider
		wantNil  bool
	}{
		{
			name:     "HTTP provider returns factory",
			provider: types.TransportProviderHTTP,
			wantNil:  false,
		},
		{
			name:     "None provider returns nil",
			provider: types.TransportProviderNone,
			wantNil:  true,
		},
		{
			name:     "Unknown provider returns nil",
			provider: types.TransportProvider("unknown"),
			wantNil:  true,
		},
		{
			name:     "GRPC provider returns nil (not yet implemented)",
			provider: types.TransportProviderGRPC,
			wantNil:  true,
		},
		{
			name:     "WebSocket provider returns nil (not yet implemented)",
			provider: types.TransportProviderWebSocket,
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := GetTransportFactory(tt.provider)
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

func TestHTTPFactory_Provider(t *testing.T) {
	factory := getHTTPFactory()
	assert.Equal(t, types.TransportProviderHTTP, factory.Provider())
}

func TestNewTransportService(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	tests := []struct {
		name        string
		cfg         *types.VMGatewayConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Nil config returns error",
			cfg:         nil,
			expectError: true,
			errorMsg:    "transport configuration is required",
		},
		{
			name: "None provider returns error",
			cfg: &types.VMGatewayConfig{
				TransportProvider: types.TransportProviderNone,
			},
			expectError: true,
			errorMsg:    "transport provider is required",
		},
		{
			name: "Unknown provider returns error",
			cfg: &types.VMGatewayConfig{
				TransportProvider: types.TransportProvider("unknown"),
			},
			expectError: true,
			errorMsg:    "unsupported transport provider",
		},
		// Note: Testing actual service creation requires valid certificates,
		// which is better suited for integration tests. Factory selection
		// is tested in TestGetTransportFactory.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use nil for dependencies since we're just testing factory selection
			var metaStore metastorage.MetaDataStore
			var objectStore objectstorage.ObjectStorageService
			var eventBus eventbus.EventBus
			var tunnelService types.TunnelClientService

			service, err := NewTransportService(ctx, tt.cfg, tunnelService, metaStore, objectStore, eventBus, logger)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, service)
			} else {
				// If we get here, the test should have expectError=true
				require.Error(t, err, "Expected error for test case")
			}
		})
	}
}

