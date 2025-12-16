package httpsclient

import (
	"context"
)

// HTTPSClientService provides HTTPS/HTTP2 client functionality for the VM
// to make requests to Edge devices over WireGuard connections.
//
// The service provides:
//   - HTTPS/HTTP2 client for VM → Edge communication
//   - mTLS support for secure Edge authentication
//   - Request routing over WireGuard tunnel
type HTTPSClientService interface {
	// Start starts the HTTPS client service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the HTTPS client service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string
}

