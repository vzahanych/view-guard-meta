package imp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/config"
	edgegateway "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway"
	edgegatewayimpl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/impl"
	eventbus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus"
	inmemorybus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus/inmemory"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
	bboltimp "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage/bbolt-imp"
	saasgateway "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/saas-gateway"
	saasimpl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/saas-gateway/impl"
	statemng "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/state-mng"
	statemngimpl "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/state-mng/impl"
)

// Server is the main orchestrator implementation. It wires together and manages
// the lifecycle of all top-level services defined in internal/ via their
// interfaces (meta-storage, object-storage, edge-gateway, state-mng, event-bus).
type Server struct {
	cfg *config.Config

	logger *zap.Logger

	// Core infrastructure
	eventBus  eventbus.EventBus
	metaStore metastorage.MetaDataStore

	// Edge gateway: encapsulates VM↔Edge networking (WireGuard, HTTPS server/client)
	edgeGateway edgegateway.EdgeGateway

	// SaaS gateway: admin/control-plane HTTP API for external SaaS components.
	saasGateway saasgateway.SaaSGateway

	// Orchestrator-wide state manager (edge lifecycle & tasks)
	stateManager statemng.StateManager
}

// NewServer creates a new orchestrator server instance.
func NewServer() *Server {
	return &Server{}
}

// Init constructs dependencies. Services are created here and started later in Start.
// This method satisfies the orchestrator.Orchestrator interface.
func (s *Server) Init(cfg *config.Config) error {
	s.cfg = cfg

	// Initialize logger (simple development logger for now).
	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	s.logger = logger

	// 1. Event bus (in-memory implementation for now).
	s.eventBus = inmemorybus.NewInMemoryEventBus(100)

	// 2. Meta-data store: BoltDB-based implementation.
	// Use configured DataDir if provided, otherwise default to ./data.
	dataDir := s.cfg.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	edgeStateDir := filepath.Join(dataDir, "edge_state")
	edgeStateCfg := bboltimp.BoltEdgeStateStoreConfig{
		Path: edgeStateDir,
	}

	edgeStateStore, err := bboltimp.NewBoltEdgeStateStore(s.logger, edgeStateCfg)
	if err != nil {
		return fmt.Errorf("failed to create edge state store: %w", err)
	}
	s.metaStore = edgeStateStore

	// 4. Edge gateway: composes WireGuard server, HTTPS server, HTTPS client.
	// Pass meta-storage and event-bus so HTTPS server can handle edge authentication.
	gateway, err := edgegatewayimpl.NewEdgeGateway(cfg, s.logger, nil, s.metaStore, s.eventBus)
	if err != nil {
		return fmt.Errorf("failed to create edge gateway: %w", err)
	}
	s.edgeGateway = gateway

	// 3. State manager: consumes events and updates edge state / schedules tasks.
	// Pass edge gateway so state manager can request IoT device sync from edges.
	stateManager, err := statemngimpl.NewStateManager(s.eventBus, s.metaStore, s.logger, s.edgeGateway)
	if err != nil {
		return fmt.Errorf("failed to create state manager: %w", err)
	}
	s.stateManager = stateManager

	// 5. SaaS gateway: admin/control-plane HTTP API.
	// Pass meta-storage and event-bus so it can manage edges.
	saasCfg := saasimpl.Config{}
	sgw, err := saasimpl.NewSaaSGateway(saasCfg, s.logger, s.metaStore, s.eventBus)
	if err != nil {
		return fmt.Errorf("failed to create saas gateway: %w", err)
	}
	s.saasGateway = sgw

	return nil
}

// Start starts the orchestrator server and all services in proper order.
// Init must be called before Start.
func (s *Server) Start(ctx context.Context) error {
	if s.cfg == nil || s.logger == nil {
		return fmt.Errorf("orchestrator not initialised; call Init first")
	}

	s.logger.Info("Starting VM Edge Orchestrator")

	// Meta-store does not require an explicit Start; it's ready after construction.

	// Start edge gateway (WireGuard + HTTPS server/client).
	if s.edgeGateway != nil {
		if err := s.edgeGateway.Start(ctx); err != nil {
			return fmt.Errorf("failed to start edge gateway: %w", err)
		}
		s.logger.Info("Edge gateway started")
	}

	// Start SaaS gateway (admin/control-plane HTTP API).
	if s.saasGateway != nil {
		if err := s.saasGateway.Start(ctx); err != nil {
			return fmt.Errorf("failed to start saas gateway: %w", err)
		}
		s.logger.Info("SaaS gateway started")
	}

	// Start state manager (subscribes to event bus and reacts to events).
	if s.stateManager != nil {
		if err := s.stateManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start state manager: %w", err)
		}
		s.logger.Info("State manager started")
	}

	s.logger.Info("User VM API orchestrator started successfully")
	return nil
}

// Shutdown stops the orchestrator server and all services.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Stopping User VM API orchestrator")

	var firstErr error

	// Stop state manager first (stops consuming events and launching tasks).
	if s.stateManager != nil {
		if err := s.stateManager.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop state manager: %w", err)
		}
	}

	// Stop SaaS gateway (admin/control-plane HTTP API).
	if s.saasGateway != nil {
		if err := s.saasGateway.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop saas gateway: %w", err)
		}
	}

	// Stop edge gateway (tears down HTTPS services and WireGuard).
	if s.edgeGateway != nil {
		if err := s.edgeGateway.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop edge gateway: %w", err)
		}
	}

	// Shutdown meta-store (flush/close BoltDB) if it exposes a Shutdown method.
	if s.metaStore != nil {
		if ms, ok := s.metaStore.(interface {
			Shutdown(context.Context) error
		}); ok {
			if err := ms.Shutdown(ctx); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("failed to shutdown meta-store: %w", err)
			}
		}
	}

	// Close event bus last.
	if s.eventBus != nil {
		if err := s.eventBus.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close event bus: %w", err)
		}
	}

	if firstErr != nil {
		s.logger.Error("User VM API orchestrator stopped with errors", zap.Error(firstErr))
		return firstErr
	}

	s.logger.Info("User VM API orchestrator stopped")
	return nil
}
