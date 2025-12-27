/*
Package vmgateway provides a unified, provider-agnostic interface for Edge↔VM bidirectional secure communication.

The VM Gateway is the core service that manages communication between Edge appliances and the VM (Virtual Machine).
It abstracts away the details of tunnel and transport implementations, allowing the system to work with any
combination of tunnel providers (WireGuard, OpenVPN, IPSec, etc.) and transport providers (HTTP, gRPC, WebSocket, etc.).

# Architecture

The gateway is composed of two core abstractions:

	┌─────────────────────────────────────────────────────────┐
	│                    VMGateway Interface                    │
	│  (Unified API for Edge↔VM Communication)                │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│ Tunnel Client      │         │ Transport Service     │
	│ Service            │         │                      │
	│                    │         │                      │
	│ - WireGuard        │         │ - HTTP/HTTPS         │
	│ - OpenVPN (future) │         │ - gRPC (future)      │
	│ - IPSec (future)   │         │ - WebSocket (future) │
	└────────────────────┘         └──────────────────────┘

The gateway coordinates the lifecycle of both services:
  - Tunnel service is started first (establishes secure network connection)
  - Transport service is started second (uses tunnel for communication)
  - Services are stopped in reverse order

# Provider Agnosticism

The gateway is designed to be provider-agnostic:

  - Tunnel providers: WireGuard (current), OpenVPN, IPSec (future)
  - Transport providers: HTTP (current), gRPC, WebSocket (future)
  - Device types: Cameras, sensors, audio devices, and other IoT devices

This allows the system to:
  - Switch tunnel/transport providers without changing application code
  - Support different deployment scenarios (production with tunnel, dev without tunnel)
  - Add new providers by implementing the appropriate interfaces

# Configuration

The gateway uses a provider-agnostic configuration structure:

	tunnel:
	  provider: wireguard
	  enabled: true
	  kvm_endpoint: "10.0.0.1:51820"
	  interface_name: "wg0"
	  raw_config:
	    config_path: "/etc/wireguard/wg0.conf"

	transport_provider: http

	https_server_config:
	  listen_address: "10.0.0.2:8443"
	  server_cert_path: "/etc/ssl/certs/edge-server.crt"
	  server_key_path: "/etc/ssl/private/edge-server.key"
	  ca_cert_path: "/etc/ssl/certs/ca.crt"

	https_client_config:
	  vm_endpoint: "10.0.0.1:8443"
	  client_cert_path: "/etc/ssl/certs/edge-client.crt"
	  client_key_path: "/etc/ssl/private/edge-client.key"
	  ca_cert_path: "/etc/ssl/certs/ca.crt"

For localhost development (tunnel disabled):

	tunnel:
	  enabled: false

	https_server_config:
	  listen_address: "localhost:8443"
	  # ... cert paths ...

	https_client_config:
	  vm_endpoint: "localhost:8443"
	  # ... cert paths ...

# Connection State Machine

The gateway tracks connection state through a state machine:

	disconnected → tunnel_connecting → tunnel_connected → transport_connecting →
	transport_connected → authenticated → capabilities_received

Error states can recover:
  - tunnel_connection_error → tunnel_connected (recovery)
  - transport_connection_error → transport_connected (recovery)
  - error → disconnected (recovery)

# Usage

The gateway is typically created using dependency injection (Fx):

	gateway, err := VMGatewayProvider(
	    lc,           // fx.Lifecycle
	    cfg,          // *types.VMGatewayConfig
	    metaStore,    // metastorage.MetaDataStore
	    objectStore,  // objectstorage.ObjectStorageService
	    eventBus,     // eventbus.EventBus
	    logger,       // *zap.Logger
	)

The gateway manages its own lifecycle and starts/stops sub-services automatically.

# Recent Refactoring

This package has been refactored to be fully provider-agnostic:

  - Types consolidated: All VM API types are in vm-gateway/types
  - Tunnel config promoted: TunnelConfig is now top-level in VMGatewayConfig
  - State machine generalized: States use generic tunnel/transport terminology
  - Interfaces moved: TransportService and TunnelClientService are in types package
  - Method names generalized: IsTransportConnected(), GetTunnelInterfaceName(), etc.
  - Error handling standardized: Sentinel errors and consistent error wrapping
  - Context handling fixed: Contexts flow from callers, not created in constructors
  - Observability enhanced: HealthSnapshot() method for debugging
  - Comments updated: All comments use provider-agnostic terminology


# Examples

See the Example* functions in the test files for usage examples.
*/
package vmgateway
