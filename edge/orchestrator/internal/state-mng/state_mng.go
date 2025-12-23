package statemng

import (
	"context"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	aigateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	cctv "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	webgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// StateManager is the central orchestrator for the edge appliance.
// It:
//   - Listens for events from all components via the event bus
//   - Manages workflow and coordinates between services
//   - Updates edge state based on events and service interactions
//   - Coordinates communication between:
//   - ai-gateway (AI inference and detection)
//   - cctv (camera management and frame capture)
//   - meta-storage (metadata management)
//   - object-storage (object storage operations)
//   - vm-gateway (VM communication)
//   - web-gateway (web UI)
//   - Manages capability syncing to VM (replaces deprecated capabilities package)
type StateManager interface {
	// Start begins processing events from the event bus and managing workflows.
	Start(ctx context.Context) error

	// Stop stops processing events and waits for in-flight tasks to finish.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// SetAIGateway sets the AI gateway service dependency
	SetAIGateway(aiGateway interface{})

	// SetCCTVService sets the CCTV service dependency
	SetCCTVService(cctvService interface{})

	// SetMetaStorage sets the metadata storage service dependency
	SetMetaStorage(metaStorage interface{})

	// SetObjectStorage sets the object storage service dependency
	SetObjectStorage(objectStorage interface{})

	// SetVMGateway sets the VM gateway service dependency
	SetVMGateway(vmGateway interface{})

	// SetWebGateway sets the web gateway service dependency
	SetWebGateway(webGateway interface{})


	// SetConfig sets the configuration
	SetConfig(cfg *config.Config)

	// SetStateManager sets the state manager for security event storage
	SetStateManager(stateManager interface{})

	// SyncCameraCapabilities syncs capabilities for a single camera to the VM
	// This is used when a dataset is uploaded to immediately update the VM with the latest status
	SyncCameraCapabilities(ctx context.Context, cameraID string) error

	// UploadDatasetForCamera packages and uploads all screenshots for a camera to the VM
	// Returns the dataset ID if successful, or an error
	// This replaces the deprecated dataset package functionality
	UploadDatasetForCamera(ctx context.Context, cameraID string, screenshotList []interface{}) (string, error)

	// SetEdgeID sets the edge ID for dataset uploads
	SetEdgeID(edgeID string)

	// ReportStatus reports deployment status to the VM
	// This replaces the deprecated deployment package functionality
	ReportStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error

	// Security Event Management (replaces deprecated @events package)
	// Security events are security issues/anomalies detected by AI when camera frames differ from trained dataset
	// This is NOT related to event-bus events

	// SaveSecurityEvent saves a security event to storage
	SaveSecurityEvent(ctx context.Context, event *types.SecurityEvent) error

	// EnqueueSecurityEvent enqueues a security event for transmission to VM
	EnqueueSecurityEvent(ctx context.Context, event *types.SecurityEvent, priority int) error

	// GetSecurityEvent retrieves a security event by ID
	GetSecurityEvent(ctx context.Context, eventID string) (*types.SecurityEvent, error)

	// ListSecurityEvents retrieves security events with optional filters
	ListSecurityEvents(ctx context.Context, cameraID string, eventType string, startTime, endTime time.Time, limit int) ([]*types.SecurityEvent, error)

	// GetPendingSecurityEvents returns pending (untransmitted) security events
	GetPendingSecurityEvents(ctx context.Context, limit int) ([]*types.SecurityEvent, error)

	// Snapshot Request Management (replaces deprecated @snapshot_request package)
	// Handles VM → Edge snapshot capture requests

	// SavePendingSnapshotRequest saves a pending snapshot request from VM
	SavePendingSnapshotRequest(ctx context.Context, cameraID string, label string, customLabel string, count int32) error

	// GetPendingSnapshotRequest retrieves a pending snapshot request for a camera
	GetPendingSnapshotRequest(ctx context.Context, cameraID string) (*types.PendingSnapshotRequest, error)

	// GetAllPendingSnapshotRequests retrieves all pending snapshot requests
	GetAllPendingSnapshotRequests(ctx context.Context) (map[string]*types.PendingSnapshotRequest, error)

	// ClearPendingSnapshotRequest clears a pending snapshot request for a camera
	ClearPendingSnapshotRequest(ctx context.Context, cameraID string) error
}

// NewStateManager creates a new StateManager instance.
// This factory function should typically not be called directly;
// use StateManagerProvider instead for proper dependency injection.
func NewStateManager(eventBus eventbus.EventBus, logger *zap.Logger) (StateManager, error) {
	return impl.NewStateManagerImpl(eventBus, logger)
}

// StateManagerProvider creates the StateManager with fx lifecycle management and wires all service dependencies.
func StateManagerProvider(
	lc fx.Lifecycle,
	cfg *config.Config,
	edgeID string,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	cctvService cctv.CCTVService,
	aiGateway aigateway.AIGateway,
	vmGateway vmgateway.VMGateway,
	webGateway webgateway.WebGateway,
) (StateManager, error) {
	manager, err := NewStateManager(eventBus, logger)
	if err != nil {
		return nil, err
	}

	// Wire core dependencies into the state manager
	if metaStore != nil {
		manager.SetMetaStorage(metaStore)
	}
	if objectStore != nil {
		manager.SetObjectStorage(objectStore)
	}
	if cctvService != nil {
		manager.SetCCTVService(cctvService)
	}
	if aiGateway != nil {
		manager.SetAIGateway(aiGateway)
	}
	if vmGateway != nil {
		manager.SetVMGateway(vmGateway)
	}
	if webGateway != nil {
		manager.SetWebGateway(webGateway)
	}
	if cfg != nil {
		manager.SetConfig(cfg)
	}
	if edgeID != "" {
		manager.SetEdgeID(edgeID)
	}

	// Setup lifecycle hooks
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if manager != nil {
				if err := manager.Start(ctx); err != nil {
					return err
				}
				logger.Info("State manager started")
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if manager != nil {
				if err := manager.Stop(ctx); err != nil {
					return err
				}
				logger.Info("State manager stopped")
			}
			return nil
		},
	})

	return manager, nil
}
