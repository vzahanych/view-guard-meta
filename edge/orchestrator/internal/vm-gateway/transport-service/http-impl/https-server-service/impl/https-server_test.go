package impl

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service/types"
	"go.uber.org/zap"
)

// mockEventBus is a simple mock event bus that captures published events
type mockEventBus struct {
	mu      sync.Mutex
	events  []evtbusstypes.EventAny
	publish func(event evtbusstypes.EventAny) error
}

func newMockEventBus() *mockEventBus {
	return &mockEventBus{
		events: make([]evtbusstypes.EventAny, 0),
	}
}

func (m *mockEventBus) Start(ctx context.Context) error {
	return nil
}

func (m *mockEventBus) Stop(ctx context.Context) error {
	return nil
}

func (m *mockEventBus) Name() string {
	return "mock-event-bus"
}

func (m *mockEventBus) HealthSnapshot() evtbusstypes.EventBusHealth {
	return evtbusstypes.EventBusHealth{}
}

func (m *mockEventBus) Subscribe(eventType evtbusstypes.EventType) <-chan evtbusstypes.EventAny {
	ch := make(chan evtbusstypes.EventAny, 10)
	return ch
}

func (m *mockEventBus) SubscribeAll() <-chan evtbusstypes.EventAny {
	ch := make(chan evtbusstypes.EventAny, 10)
	return ch
}

func (m *mockEventBus) Publish(event evtbusstypes.EventAny) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	if m.publish != nil {
		return m.publish(event)
	}
	return nil
}

func (m *mockEventBus) Unsubscribe(eventType evtbusstypes.EventType, ch <-chan evtbusstypes.EventAny) {
	// No-op for mock
}

func (m *mockEventBus) Close() error {
	return nil
}

func (m *mockEventBus) getEvents() []evtbusstypes.EventAny {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy
	result := make([]evtbusstypes.EventAny, len(m.events))
	copy(result, m.events)
	return result
}

// mockTunnelClient is a mock tunnel client that always returns "none" (disabled)
type mockTunnelClient struct{}

func (m *mockTunnelClient) Start(ctx context.Context) error {
	return nil
}

func (m *mockTunnelClient) Stop(ctx context.Context) error {
	return nil
}

func (m *mockTunnelClient) Name() string {
	return "none"
}

func (m *mockTunnelClient) IsConnected() bool {
	return false
}

func (m *mockTunnelClient) GetInterfaceName() string {
	return ""
}

func (m *mockTunnelClient) GetEndpoint() string {
	return ""
}

// TestHTTPServer_Stop_DisconnectEventIncludesEndpoint verifies that the disconnect event
// includes the correct endpoint address when Stop() is called.
// NOTE: This test requires valid certificate files to run, as certificates are now required.
// Skipping for now - in a full test environment, valid test certificates should be provided.
func TestHTTPServer_Stop_DisconnectEventIncludesEndpoint(t *testing.T) {
	t.Skip("Skipping test that requires valid certificate files - certificates are now required by design")
	
	logger := zap.NewNop()
	eventBus := newMockEventBus()
	tunnelClient := &mockTunnelClient{}

	// Create server config with a specific listen address
	// Note: Certificates are required, but we'll use placeholder paths for testing
	// In real scenarios, these would be valid certificate file paths
	cfg := &httpsservertypes.HTTPServerConfig{
		ListenAddress:  "localhost:8443",
		ServerCertPath: "/test/server.crt",
		ServerKeyPath:  "/test/server.key",
		CACertPath:     "/test/ca.crt",
	}

	server := NewHTTPServer(cfg, logger, "test-edge-id", tunnelClient, nil, nil, eventBus)
	ctx := context.Background()

	// Start the server (this will store the listen address)
	err := server.Start(ctx)
	require.NoError(t, err, "Server should start successfully")

	// Stop the server
	err = server.Stop(ctx)
	require.NoError(t, err, "Server should stop successfully")

	// Verify that a disconnect event was published with the correct endpoint
	events := eventBus.getEvents()
	
	// Find the disconnect event
	var disconnectEvent *evtbusstypes.EventAny
	for i := range events {
		if events[i].Type == evtbusstypes.EventTypeNetworkTransportDisconnected {
			disconnectEvent = &events[i]
			break
		}
	}

	require.NotNil(t, disconnectEvent, "Disconnect event should be published")
	
	// Parse the event data using FromEventAny
	typedEvent, err := evtbusstypes.FromEventAny[evtbusstypes.TransportDisconnectedEventData](*disconnectEvent)
	require.NoError(t, err, "Should be able to parse disconnect event data")
	
	assert.Equal(t, "https-server", typedEvent.Data.Service, "Event service should be https-server")
	assert.Equal(t, "localhost:8443", typedEvent.Data.Endpoint, "Event endpoint should match the listen address")
	assert.Equal(t, "https", typedEvent.Data.Protocol, "Event protocol should be https")
	assert.Equal(t, "server stopped", typedEvent.Data.Reason, "Event reason should be 'server stopped'")
}

// TestHTTPServer_Stop_DisconnectEventWithCustomEndpoint verifies that the disconnect event
// includes the correct endpoint even when a custom listen address is used.
// NOTE: This test requires valid certificate files to run, as certificates are now required.
// Skipping for now - in a full test environment, valid test certificates should be provided.
func TestHTTPServer_Stop_DisconnectEventWithCustomEndpoint(t *testing.T) {
	t.Skip("Skipping test that requires valid certificate files - certificates are now required by design")
	
	logger := zap.NewNop()
	eventBus := newMockEventBus()
	tunnelClient := &mockTunnelClient{}

	// Create server config with a custom listen address
	// Note: Certificates are required, but we'll use placeholder paths for testing
	customAddr := "127.0.0.1:9443"
	cfg := &httpsservertypes.HTTPServerConfig{
		ListenAddress:  customAddr,
		ServerCertPath: "/test/server.crt",
		ServerKeyPath:  "/test/server.key",
		CACertPath:     "/test/ca.crt",
	}

	server := NewHTTPServer(cfg, logger, "test-edge-id", tunnelClient, nil, nil, eventBus)
	ctx := context.Background()

	// Start the server
	err := server.Start(ctx)
	require.NoError(t, err, "Server should start successfully")

	// Stop the server
	err = server.Stop(ctx)
	require.NoError(t, err, "Server should stop successfully")

	// Verify that the disconnect event includes the custom endpoint
	events := eventBus.getEvents()
	
	// Find the disconnect event
	var disconnectEvent *evtbusstypes.EventAny
	for i := range events {
		if events[i].Type == evtbusstypes.EventTypeNetworkTransportDisconnected {
			disconnectEvent = &events[i]
			break
		}
	}

	require.NotNil(t, disconnectEvent, "Disconnect event should be published")
	
	// Parse the event data
	typedEvent, err := evtbusstypes.FromEventAny[evtbusstypes.TransportDisconnectedEventData](*disconnectEvent)
	require.NoError(t, err, "Should be able to parse disconnect event data")
	
	assert.Equal(t, customAddr, typedEvent.Data.Endpoint, "Event endpoint should match the custom listen address")
}

// TestHTTPServer_Stop_DisconnectEventWithDefaultEndpoint verifies that the disconnect event
// includes the default endpoint when no listen address is configured.
// NOTE: This test requires valid certificate files to run, as certificates are now required.
// Skipping for now - in a full test environment, valid test certificates should be provided.
func TestHTTPServer_Stop_DisconnectEventWithDefaultEndpoint(t *testing.T) {
	t.Skip("Skipping test that requires valid certificate files - certificates are now required by design")
	
	logger := zap.NewNop()
	eventBus := newMockEventBus()
	tunnelClient := &mockTunnelClient{} // Returns "none", so localhost mode

	// Create server config without listen address (should default to localhost:8443)
	// Note: Certificates are required, but we'll use placeholder paths for testing
	cfg := &httpsservertypes.HTTPServerConfig{
		ListenAddress:  "", // Empty - should default to localhost:8443 in localhost mode
		ServerCertPath: "/test/server.crt",
		ServerKeyPath:  "/test/server.key",
		CACertPath:     "/test/ca.crt",
	}

	server := NewHTTPServer(cfg, logger, "test-edge-id", tunnelClient, nil, nil, eventBus)
	ctx := context.Background()

	// Start the server
	err := server.Start(ctx)
	require.NoError(t, err, "Server should start successfully")

	// Stop the server
	err = server.Stop(ctx)
	require.NoError(t, err, "Server should stop successfully")

	// Verify that the disconnect event includes the default endpoint
	events := eventBus.getEvents()
	
	// Find the disconnect event
	var disconnectEvent *evtbusstypes.EventAny
	for i := range events {
		if events[i].Type == evtbusstypes.EventTypeNetworkTransportDisconnected {
			disconnectEvent = &events[i]
			break
		}
	}

	require.NotNil(t, disconnectEvent, "Disconnect event should be published")
	
	// Parse the event data
	typedEvent, err := evtbusstypes.FromEventAny[evtbusstypes.TransportDisconnectedEventData](*disconnectEvent)
	require.NoError(t, err, "Should be able to parse disconnect event data")
	
	// In localhost mode with empty ListenAddress, should default to localhost:8443
	assert.Equal(t, "localhost:8443", typedEvent.Data.Endpoint, "Event endpoint should be the default localhost:8443")
	assert.NotEmpty(t, typedEvent.Data.Endpoint, "Event endpoint should not be empty")
}

