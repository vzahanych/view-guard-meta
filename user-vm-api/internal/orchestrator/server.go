package orchestrator

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database/migrations"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// Server is the main orchestrator server
type Server struct {
	config  *config.Config
	logger  *logging.Logger
	manager *service.Manager
	db      *database.DB
}

// NewServer creates a new orchestrator server
func NewServer(cfg *config.Config, log *logging.Logger) *Server {
	return &Server{
		config:  cfg,
		logger:  log,
		manager: service.NewManager(log),
	}
}

// Start starts the orchestrator server and all services
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting User VM API orchestrator")

	// Initialize database (single SQLite DB for all services)
	dbCfg := database.DefaultConfig(s.config.UserVMAPI.EventCache.DatabasePath)
	db, err := database.New(dbCfg)
	if err != nil {
		s.logger.Error("Failed to initialize database", zap.Error(err))
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.db = db

	// Run database migrations (create all tables)
	migrator := migrations.NewMigrator(db)
	if err := migrator.Up(ctx); err != nil {
		s.logger.Error("Failed to run database migrations", zap.Error(err))
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Get event bus from manager (already created)
	eventBus := s.manager.GetEventBus()

	var edgeAPIServer *tunnelgateway.EdgeAPIServer
	var capStore *tunnelgateway.CapabilityStore

	// Register Tunnel Gateway / WireGuard services when enabled
	if s.config.UserVMAPI.WireGuardServer.Enabled {
		// WireGuard server (acts as KVM-side tunnel endpoint)
		wgServer, err := tunnelgateway.NewWireGuardServer(s.config, s.logger, db)
		if err != nil {
			s.logger.Error("Failed to create WireGuard server", zap.Error(err))
			return fmt.Errorf("failed to create WireGuard server: %w", err)
		}
		wgServer.SetEventBus(eventBus)
		s.manager.Register(wgServer)

		// Edge authentication/registration manager
		edgeAuth := tunnelgateway.NewEdgeAuth(s.config, s.logger, db, wgServer)

		// Edge-facing gRPC API (over WireGuard tunnel)
		edgeAPIServer, err = tunnelgateway.NewEdgeAPIServer(s.config, s.logger, db, wgServer, edgeAuth)
		if err != nil {
			s.logger.Error("Failed to create Edge API server", zap.Error(err))
			return fmt.Errorf("failed to create Edge API server: %w", err)
		}
		edgeAPIServer.SetEventBus(eventBus)
		capStore = edgeAPIServer.GetCapabilityStore()
		s.manager.Register(edgeAPIServer)
	}

	// Register API Gateway when enabled
	if s.config.UserVMAPI.APIGateway.Enabled {
		apiServer := NewAPIServer(s.config, s.logger, capStore, edgeAPIServer)
		s.manager.Register(apiServer)
		s.logger.Info("API Gateway registered")
	}

	// Register services here as they are implemented
	// Example:
	// wireguardSvc := wireguardserver.New(s.config, s.logger)
	// s.manager.Register(wireguardSvc)

	// Start all services
	if err := s.manager.Start(ctx, s.config); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	s.logger.Info("User VM API orchestrator started successfully")
	return nil
}

// Stop stops the orchestrator server and all services
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping User VM API orchestrator")

	if err := s.manager.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			s.logger.Error("Failed to close database", zap.Error(err))
		}
	}

	s.logger.Info("User VM API orchestrator stopped")
	return nil
}

// GetStatus returns the status of the orchestrator
func (s *Server) GetStatus() *service.ServiceStatus {
	return s.manager.GetStatus()
}

