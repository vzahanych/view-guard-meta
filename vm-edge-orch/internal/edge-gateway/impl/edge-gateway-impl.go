package impl

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	edgegateway "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway"
	httpsclient "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/https-client-service"
	httpsclientimpl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/https-client-service/impl"
	httpsserver "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/https-server-service"
	httpsserverimpl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/https-server-service/impl"
	wgserver "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/wg-server-service"
	wgserverimpl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/wg-server-service/impl"
)

// edgeGateway implements the EdgeGateway interface
type edgeGateway struct {
	wgServerService    wgserver.WGServerService
	httpsServerService httpsserver.HTTPSServerService
	httpsClientService httpsclient.HTTPSClientService
	logger             *zap.Logger
	mu                 sync.RWMutex
	started            bool
}

// NewEdgeGateway creates a new EdgeGateway implementation that composes
// WireGuard server, HTTPS server, and HTTPS client services.
// cfg, log, and db are interface{} to avoid dependencies on non-existent packages.
// metaStore and eventBus are optional dependencies for HTTPS server authentication handlers.
func NewEdgeGateway(cfg interface{}, log interface{}, db interface{}, metaStore interface{}, eventBus interface{}) (edgegateway.EdgeGateway, error) {
	// Try to extract logger if it's a zap.Logger
	var logger *zap.Logger
	if zapLogger, ok := log.(*zap.Logger); ok {
		logger = zapLogger
	} else {
		// Create a simple logger if none provided
		logger, _ = zap.NewDevelopment()
	}

	// Create WireGuard server service
	wgServer, err := wgserverimpl.NewWGServerService(cfg, log, db)
	if err != nil {
		return nil, fmt.Errorf("failed to create WireGuard server service: %w", err)
	}

	// Create HTTPS server service (uses config for TLS paths)
	// Pass metaStore and eventBus for authentication handlers
	httpsServer, err := httpsserverimpl.NewHTTPSServerService(cfg, log, metaStore, eventBus)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTPS server service: %w", err)
	}

	// Create HTTPS client service (uses config for TLS paths)
	httpsClient, err := httpsclientimpl.NewHTTPSClientService(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTPS client service: %w", err)
	}

	return &edgeGateway{
		wgServerService:    wgServer,
		httpsServerService: httpsServer,
		httpsClientService: httpsClient,
		logger:             logger,
	}, nil
}

// Name returns the service name
func (g *edgeGateway) Name() string {
	return "edge-gateway"
}

// Start starts all underlying services in the correct order
func (g *edgeGateway) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.started {
		return fmt.Errorf("edge gateway is already started")
	}

	if g.logger != nil {
		g.logger.Info("Starting Edge Gateway (all services)")
	}

	// Step 1: Start WireGuard server first (required for HTTPS services)
	if g.logger != nil {
		g.logger.Info("Starting WireGuard server service...")
	}
	if err := g.wgServerService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start WireGuard server service: %w", err)
	}

	// Step 2: Start HTTPS server (depends on WireGuard)
	if g.logger != nil {
		g.logger.Info("Starting HTTPS server service...")
	}
	if err := g.httpsServerService.Start(ctx); err != nil {
		// Try to stop WireGuard if HTTPS server fails
		_ = g.wgServerService.Stop(ctx)
		return fmt.Errorf("failed to start HTTPS server service: %w", err)
	}

	// Step 3: Start HTTPS client (depends on WireGuard)
	if g.logger != nil {
		g.logger.Info("Starting HTTPS client service...")
	}
	if err := g.httpsClientService.Start(ctx); err != nil {
		// Try to stop already started services
		_ = g.httpsServerService.Stop(ctx)
		_ = g.wgServerService.Stop(ctx)
		return fmt.Errorf("failed to start HTTPS client service: %w", err)
	}

	g.started = true

	if g.logger != nil {
		g.logger.Info("Edge Gateway started successfully (all services running)")
	}

	return nil
}

// Stop stops all underlying services in reverse order
func (g *edgeGateway) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.started {
		return nil // Already stopped
	}

	if g.logger != nil {
		g.logger.Info("Stopping Edge Gateway (all services)")
	}

	var errors []error

	// Stop HTTPS client first
	if g.logger != nil {
		g.logger.Info("Stopping HTTPS client service...")
	}
	if err := g.httpsClientService.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop HTTPS client service: %w", err))
	}

	// Stop HTTPS server
	if g.logger != nil {
		g.logger.Info("Stopping HTTPS server service...")
	}
	if err := g.httpsServerService.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop HTTPS server service: %w", err))
	}

	// Stop WireGuard server last
	if g.logger != nil {
		g.logger.Info("Stopping WireGuard server service...")
	}
	if err := g.wgServerService.Stop(ctx); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop WireGuard server service: %w", err))
	}

	g.started = false

	if len(errors) > 0 {
		if g.logger != nil {
			g.logger.Error("Some services failed to stop", zap.Errors("errors", errors))
		}
		return fmt.Errorf("errors stopping services: %v", errors)
	}

	if g.logger != nil {
		g.logger.Info("Edge Gateway stopped successfully")
	}

	return nil
}

// GetWGServerService returns the underlying WireGuard server service
func (g *edgeGateway) GetWGServerService() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.wgServerService
}

// GetHTTPSServerService returns the underlying HTTPS server service
func (g *edgeGateway) GetHTTPSServerService() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.httpsServerService
}

// GetHTTPSClientService returns the underlying HTTPS client service
func (g *edgeGateway) GetHTTPSClientService() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.httpsClientService
}
