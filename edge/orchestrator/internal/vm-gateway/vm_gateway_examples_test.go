package vmgateway

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// ExampleVMGatewayProvider demonstrates how to create a VM Gateway using dependency injection.
//
// This example shows the typical usage pattern with Fx lifecycle management.
// The gateway will automatically start and stop sub-services in the correct order.
func ExampleVMGatewayProvider() {
	// In a real application, the gateway would be created via Fx:
	//
	//   gateway, err := VMGatewayProvider(
	//       lc,        // fx.Lifecycle
	//       cfg,       // *types.VMGatewayConfig
	//       metaStore, // metastorage.MetaDataStore
	//       objectStore, // objectstorage.ObjectStorageService
	//       eventBus,  // eventbus.EventBus
	//       logger,    // *zap.Logger
	//   )

	fmt.Println("Gateway created via VMGatewayProvider with Fx lifecycle")
	// Output:
	// Gateway created via VMGatewayProvider with Fx lifecycle
}

// ExampleVMGateway_Start demonstrates how to start the gateway and its sub-services.
func ExampleVMGateway_Start() {
	// In real usage, the gateway would be created via Fx and started automatically.
	// This example shows the pattern:

	// gateway would be provided by Fx
	// err := gateway.Start(ctx)

	fmt.Println("Gateway start pattern: gateway.Start(ctx)")
	// Output:
	// Gateway start pattern: gateway.Start(ctx)
}

// ExampleVMGateway_TransitionConnectionState demonstrates how to transition connection states.
func ExampleVMGateway_TransitionConnectionState() {
	// In real usage, the gateway would be provided by Fx.
	// This example shows the pattern:

	// gateway would be provided by Fx
	// currentState := gateway.GetConnectionState()
	// err := gateway.TransitionConnectionState(types.ConnectionStateTunnelConnecting, "")

	fmt.Println("State transition pattern: gateway.TransitionConnectionState(newState, errorMsg)")
	// Output:
	// State transition pattern: gateway.TransitionConnectionState(newState, errorMsg)
}

// ExampleVMGateway_Authenticate demonstrates how to authenticate with the VM.
func ExampleVMGateway_Authenticate() {
	// In real usage, the gateway would be provided by Fx and started.
	// This example shows the pattern:

	edgeID := "edge-123"
	// gateway would be provided by Fx
	// err := gateway.Authenticate(ctx, edgeID)

	fmt.Printf("Authentication pattern: gateway.Authenticate(ctx, %s)\n", edgeID)
	// Output:
	// Authentication pattern: gateway.Authenticate(ctx, edge-123)
}

// ExampleVMGateway_HealthSnapshot demonstrates how to get a health snapshot of the gateway.
func ExampleVMGateway_HealthSnapshot() {
	// In real usage, the gateway would be provided by Fx.
	// This example shows the pattern:

	// gateway would be provided by Fx
	// status := gateway.HealthSnapshot()
	// fmt.Printf("State: %s, Tunnel: %v, Transport: %v\n",
	//     status.ConnectionState.State,
	//     status.TunnelStatus.Enabled,
	//     status.TransportStatus.Connected)

	fmt.Println("Health snapshot pattern: status := gateway.HealthSnapshot()")
	// Output:
	// Health snapshot pattern: status := gateway.HealthSnapshot()
}

// ExampleTunnelConfig demonstrates the new tunnel configuration format.
func ExampleTunnelConfig() {
	// Example: WireGuard tunnel configuration
	cfg := &types.VMGatewayConfig{
		TransportProvider: types.TransportProviderHTTP,
		EdgeID:            "edge-123",
		Tunnel: types.TunnelConfig{
			Provider:     types.TunnelProviderWireGuard,
			Enabled:      true,
			KVMEndpoint:  "10.0.0.1:51820",
			InterfaceName: "wg0",
			RawConfig: map[string]interface{}{
				"config_path": "/etc/wireguard/wg0.conf",
			},
		},
	}

	// Get tunnel config
	tunnelCfg := cfg.GetTunnelConfig()
	if tunnelCfg != nil {
		fmt.Printf("Tunnel provider: %s\n", tunnelCfg.Provider)
		fmt.Printf("Tunnel enabled: %v\n", tunnelCfg.Enabled)
		fmt.Printf("KVM endpoint: %s\n", tunnelCfg.KVMEndpoint)
	}

	// Output:
	// Tunnel provider: wireguard
	// Tunnel enabled: true
	// KVM endpoint: 10.0.0.1:51820
}

// Example_localhostMode demonstrates localhost development mode configuration.
func Example_localhostMode() {
	// Example: Localhost development mode (tunnel disabled)
	cfg := &types.VMGatewayConfig{
		TransportProvider: types.TransportProviderHTTP,
		EdgeID:            "edge-123",
		// Tunnel is disabled (not configured)
		HTTPServerConfig: types.HTTPServerConfig{
			ListenAddress:  "localhost:8443", // Must be localhost
			ServerCertPath: "/test/server.crt",
			ServerKeyPath:  "/test/server.key",
			CACertPath:     "/test/ca.crt",
		},
		HTTPSClientConfig: types.HTTPSClientConfig{
			VMEndpoint:     "localhost:8443", // Must be localhost
			ClientCertPath: "/test/client.crt",
			ClientKeyPath:  "/test/client.key",
			CACertPath:     "/test/ca.crt",
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Printf("Validation error: %v\n", err)
		return
	}

	tunnelCfg := cfg.GetTunnelConfig()
	fmt.Printf("Tunnel configured: %v\n", tunnelCfg != nil)
	fmt.Printf("Localhost mode: %v\n", tunnelCfg == nil)

	// Output:
	// Tunnel configured: false
	// Localhost mode: true
}
