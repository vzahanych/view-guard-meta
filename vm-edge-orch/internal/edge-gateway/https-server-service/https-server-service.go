package httpsserver

import (
	"context"
)

// HTTPSServerService provides HTTPS/HTTP2 server functionality for Edge clients
// to connect to the VM. The server is exposed only on WireGuard connections.
//
// The service provides:
//   - HTTPS/HTTP2 server listening on WireGuard interface
//   - Edge authentication and connection management
//   - REST API endpoints for Edge → VM communication
type HTTPSServerService interface {
	// Start starts the HTTPS/HTTP2 server.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the HTTPS/HTTP2 server.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string
}
