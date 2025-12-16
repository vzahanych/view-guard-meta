package edgegateway

import (
	"context"
)

// EdgeGateway provides a unified interface for VM↔Edge bidirectional secure communication.
// It combines three core services:
//   - WireGuard server service (tunnel management)
//   - HTTPS/HTTP2 server service (Edge → VM communication)
//   - HTTPS/HTTP2 client service (VM → Edge communication)
//
// The gateway provides:
//   - Unified lifecycle management for all three services
//   - Coordinated startup and shutdown
//   - Access to underlying services when needed
type EdgeGateway interface {
	// Start starts all underlying services (WireGuard server, HTTPS server, HTTPS client).
	// Services are started in the correct order: WireGuard first, then HTTPS services.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down all underlying services.
	// Services are stopped in reverse order: HTTPS services first, then WireGuard.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// GetWGServerService returns the underlying WireGuard server service.
	GetWGServerService() interface{}

	// GetHTTPSServerService returns the underlying HTTPS server service.
	GetHTTPSServerService() interface{}

	// GetHTTPSClientService returns the underlying HTTPS client service.
	GetHTTPSClientService() interface{}
}
