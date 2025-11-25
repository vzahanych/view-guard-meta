package orchestrator

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
)

// Server is the main orchestrator server
type Server struct {
	config  *config.Config
	logger  *logging.Logger
	manager *service.Manager
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

	s.logger.Info("User VM API orchestrator stopped")
	return nil
}

// GetStatus returns the status of the orchestrator
func (s *Server) GetStatus() *service.ServiceStatus {
	return s.manager.GetStatus()
}

