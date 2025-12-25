package aigateway

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	common "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/common"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	cctv "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	statemng "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// AIGateway defines the interface for the AI gateway service.
// The gateway communicates with the AI service to process frames from CCTV cameras
// and creates events when abnormal frames are detected.
type AIGateway interface {
	common.Service

	// StartFrameProcessing starts processing frames from a camera
	StartFrameProcessing(ctx context.Context, cameraID string) error

	// StopFrameProcessing stops processing frames from a camera
	StopFrameProcessing(ctx context.Context, cameraID string) error

	// IsProcessing returns whether frames are being processed for a camera
	IsProcessing(cameraID string) bool

	// GetProcessingStats returns statistics about frame processing
	GetProcessingStats(cameraID string) (*types.ProcessingStats, error)

	// SetEventCallback sets a callback function that will be called when a security event is detected
	// Security events are security issues/anomalies detected by AI when camera frames differ from trained dataset
	SetEventCallback(callback func(*statemng.SecurityEvent))

	// SetConfidenceThreshold sets the confidence threshold for anomaly detection
	SetConfidenceThreshold(threshold float64)

	// GetConfidenceThreshold returns the current confidence threshold
	GetConfidenceThreshold() float64

	// NotifyModelDeployment notifies the AI service about a deployed model
	// The AI service will load the model from MinIO using the provided metadata
	NotifyModelDeployment(ctx context.Context, metadata *types.ModelMetadata) error

	// ProcessFrame processes a frame that was stored in object storage
	// The AI service will:
	// 1. Load the model from MinIO (if not already loaded) using model metadata
	// 2. Process the frame from object storage
	// 3. Determine if it's similar to training set or has anomalies
	// 4. Delete normal frames or move suspicious ones to security event bucket
	ProcessFrame(ctx context.Context, cameraID string, frameKey string, frameData []byte) error
}

func NewAIGateway(ctx context.Context, config *types.AIGatewayConfig, cctvSvc cctv.CCTVService, logger *zap.Logger) (AIGateway, error) {
	// TODO: When provider field is added to AIConfig, use: config.AI.Provider
	provider := "default" // Default provider until config field is added
	switch provider {
	case "default":
		return impl.NewAIGatewayImpl(config, cctvSvc, logger)
	default:
		return nil, fmt.Errorf("unsupported ai-gateway provider: %s", provider)
	}
}

// AIGatewayProvider creates the AI gateway with fx lifecycle management
// The gateway is optional and will be nil if AIServiceURL is not configured
//
// Dependencies:
//   - cctvService: Required for frame capture
//   - objectStore: Explicit dependency for startup ordering (ensures ObjectStorage starts before AIGateway)
//   - metaStore, eventBus: Explicit dependencies for startup ordering
func AIGatewayProvider(
	lc fx.Lifecycle,
	cfg *types.AIGatewayConfig,
	cctvService cctv.CCTVService,
	objectStore objectstorage.ObjectStorageService,
	metaStore metastorage.MetaDataStore,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (AIGateway, error) {
	if cfg.AIServiceURL == "" {
		logger.Info("AI service URL not configured, AI gateway will not be available")
		return nil, nil
	}

	gateway, err := NewAIGateway(context.Background(), cfg, cctvService, logger)
	if err != nil {
		return nil, err
	}

	// Dependencies are used for startup ordering only
	// metaStore, objectStore, eventBus are available for future use
	_ = metaStore
	_ = objectStore
	_ = eventBus

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if gateway != nil {
				if err := gateway.Start(ctx); err != nil {
					return err
				}
			}
			logger.Info("AI gateway started", zap.String("service_url", cfg.AIServiceURL))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if gateway != nil {
				if err := gateway.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("AI gateway stopped")
			return nil
		},
	})

	return gateway, nil
}
