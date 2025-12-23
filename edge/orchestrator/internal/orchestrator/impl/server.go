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



// Start starts the orchestrator server.
// Note: Individual services (eventBus, metaStorage, objectStorage, edgeStateMgr,
// cctvService, aiGateway, vmGateway, webGateway) are already started by their
// respective fx lifecycle hooks. This method only performs orchestrator-level
// initialization if needed.
func (s *Server) Start(ctx context.Context) error {
	if s.cfg == nil || s.logger == nil {
		return fmt.Errorf("orchestrator not initialised; call Init first")
	}

	s.logger.Info("Edge Orchestrator ready - all services started via fx lifecycle")
	return nil
}

// Shutdown stops the orchestrator server.
// Note: Individual services (eventBus, metaStorage, objectStorage, edgeStateMgr,
// cctvService, aiGateway, vmGateway, webGateway) are stopped by their respective
// fx lifecycle hooks. This method only performs orchestrator-level cleanup if needed.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Edge Orchestrator shutting down - all services will be stopped via fx lifecycle")
	
	// Only shutdown telemetry provider if it's not managed by fx lifecycle
	// (check if it has its own lifecycle hook)
	if s.telemetryProvider != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.telemetryProvider.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Failed to shutdown telemetry provider", zap.Error(err))
			return err
		}
	}

	return nil
}
