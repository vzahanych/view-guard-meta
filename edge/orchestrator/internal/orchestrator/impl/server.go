package impl

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	aigateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	cctv "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	statemng "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng"
	telemetryotel "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/telemetry-otel"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	webgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway"
	"go.uber.org/zap"
)


// Server is the main orchestrator implementation. It wires together and manages
// the lifecycle of all top-level services defined in internal/ via their
// interfaces (meta-storage, object-storage, ai-gateway, state-mng, event-bus, etc.).
type Server struct {
	// Edge ID
	edgeID string

	cfg *config.Config

	logger *zap.Logger

	// Core infrastructure
	eventBus      eventbus.EventBus
	metaStorage   metastorage.MetaDataStore
	objectStorage objectstorage.ObjectStorageService

	// Edge state manager (manages edge lifecycle and connection state)
	edgeStateMgr statemng.StateManager

	// CCTV service
	cctvService cctv.CCTVService

	// AI gateway
	aiGateway aigateway.AIGateway

	// VM gateway components
	vmGateway vmgateway.VMGateway

	// Web gateway
	webGateway webgateway.WebGateway

	// Telemetry provider
	telemetryProvider *telemetryotel.Provider
}

// NewServer creates a new orchestrator server instance.
func NewServer(
	edgeID string,
	cfg *config.Config,
	log *zap.Logger,
	eventBus eventbus.EventBus,
	metaStorage metastorage.MetaDataStore,
	objectStorage objectstorage.ObjectStorageService,
	edgeStateMgr statemng.StateManager,
	cctvService cctv.CCTVService,
	aiGateway aigateway.AIGateway,
	vmGateway vmgateway.VMGateway,
	webGateway webgateway.WebGateway,
	telemetryProvider *telemetryotel.Provider,
) *Server {
	return &Server{
		edgeID:            edgeID,
		cfg:               cfg,
		logger:            log,
		eventBus:          eventBus,
		metaStorage:       metaStorage,
		objectStorage:     objectStorage,
		edgeStateMgr:      edgeStateMgr,
		cctvService:       cctvService,
		aiGateway:         aiGateway,
		vmGateway:         vmGateway,
		webGateway:        webGateway,
		telemetryProvider: telemetryProvider,
	}
}



// Start starts the orchestrator server and all services in proper order.
// Init must be called before Start.
func (s *Server) Start(ctx context.Context) error {
	if s.cfg == nil || s.logger == nil {
		return fmt.Errorf("orchestrator not initialised; call Init first")
	}

	s.logger.Info("Starting Edge Orchestrator")

	// Start edge state manager
	if s.edgeStateMgr != nil {
		if err := s.edgeStateMgr.Start(ctx); err != nil {
			return fmt.Errorf("failed to start edge state manager: %w", err)
		}
		s.logger.Info("Edge state manager started")
	}

	// Start object storage
	if s.objectStorage != nil {
		if err := s.objectStorage.Start(ctx); err != nil {
			return fmt.Errorf("failed to start object storage: %w", err)
		}
		s.logger.Info("Object storage started")
	}

	// Start CCTV service
	if s.cctvService != nil {
		if err := s.cctvService.Start(ctx); err != nil {
			return fmt.Errorf("failed to start CCTV service: %w", err)
		}
		s.logger.Info("CCTV service started")
	}

	// Start VM gateway (WireGuard, HTTPS client, HTTPS server)
	if s.vmGateway != nil {
		if err := s.vmGateway.Start(ctx); err != nil {
			return fmt.Errorf("failed to start VM gateway: %w", err)
		}
		s.logger.Info("VM gateway started")
	}

	// Start AI gateway
	if s.aiGateway != nil {
		if err := s.aiGateway.Start(ctx); err != nil {
			return fmt.Errorf("failed to start AI gateway: %w", err)
		}
		s.logger.Info("AI gateway started")
	}

	// Start web gateway
	if s.webGateway != nil {
		if err := s.webGateway.Start(ctx); err != nil {
			return fmt.Errorf("failed to start web gateway: %w", err)
		}
		s.logger.Info("Web gateway started")
	}

	s.logger.Info("Edge Orchestrator started successfully")
	return nil
}

// Shutdown stops the orchestrator server and all services.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Stopping Edge Orchestrator")

	var firstErr error

	// Stop services in reverse order of startup

	// Stop web gateway
	if s.webGateway != nil {
		if err := s.webGateway.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop web gateway: %w", err)
		}
	}

	// Stop AI gateway
	if s.aiGateway != nil {
		if err := s.aiGateway.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop AI gateway: %w", err)
		}
	}

	// Stop VM gateway (stops WireGuard, HTTPS client, HTTPS server)
	if s.vmGateway != nil {
		if err := s.vmGateway.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop VM gateway: %w", err)
		}
	}

	// Stop CCTV service
	if s.cctvService != nil {
		if err := s.cctvService.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop CCTV service: %w", err)
		}
	}

	// Stop object storage
	if s.objectStorage != nil {
		if err := s.objectStorage.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop object storage: %w", err)
		}
	}

	// Stop edge state manager
	if s.edgeStateMgr != nil {
		if err := s.edgeStateMgr.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop edge state manager: %w", err)
		}
	}

	// Shutdown telemetry provider
	if s.telemetryProvider != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.telemetryProvider.Shutdown(shutdownCtx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to shutdown telemetry provider: %w", err)
		}
	}


	// Close event bus
	if s.eventBus != nil {
		if err := s.eventBus.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close event bus: %w", err)
		}
	}

	if firstErr != nil {
		s.logger.Error("Edge Orchestrator stopped with errors", zap.Error(firstErr))
		return firstErr
	}

	s.logger.Info("Edge Orchestrator stopped")
	return nil
}
