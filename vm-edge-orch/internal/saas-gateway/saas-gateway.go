package saasgateway

import (
	"context"
)

// SaaSGateway provides HTTP REST API endpoints for SaaS components to interact with the VM application.
// This service acts as an admin interface gateway, handling requests from SaaS control plane components.
//
// The service provides:
//   - HTTP server for SaaS component requests
//   - Admin interface endpoints for VM management
//   - Health check and status endpoints
type SaaSGateway interface {
	// Start starts the HTTP server and begins serving requests.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the HTTP server.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string
}
