package modeldeployment

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
)

// ModelDeploymentService monitors model catalog and triggers deployments
type ModelDeploymentService struct {
	orchestrator *ModelDeploymentOrchestrator
	modelCatalog *modelcatalog.ModelCatalog
	eventBus     *service.EventBus
	logger       *logging.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	running      bool
	mu           sync.RWMutex
}

// NewModelDeploymentService creates a new model deployment service
func NewModelDeploymentService(
	orchestrator *ModelDeploymentOrchestrator,
	modelCatalog *modelcatalog.ModelCatalog,
	eventBus *service.EventBus,
	logger *logging.Logger,
) (*ModelDeploymentService, error) {
	if orchestrator == nil {
		return nil, fmt.Errorf("orchestrator is required")
	}
	if modelCatalog == nil {
		return nil, fmt.Errorf("model catalog is required")
	}
	if eventBus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Note: ModelConverter is already initialized in orchestrator

	ctx, cancel := context.WithCancel(context.Background())

	return &ModelDeploymentService{
		orchestrator: orchestrator,
		modelCatalog: modelCatalog,
		eventBus:     eventBus,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start starts the deployment service
func (s *ModelDeploymentService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("deployment service is already running")
	}

	s.running = true
	s.logger.Info("Starting model deployment service")

	// Subscribe to model.registered events
	eventChan := s.eventBus.Subscribe(service.EventTypeModelTrained)

	// Start event listener
	s.wg.Add(1)
	go s.listenForModelEvents(eventChan)

	// Start periodic catalog scan (fallback if events are missed)
	s.wg.Add(1)
	go s.periodicCatalogScan()

	return nil
}

// Stop stops the deployment service
func (s *ModelDeploymentService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("deployment service is not running")
	}

	s.logger.Info("Stopping model deployment service")
	s.cancel()
	s.wg.Wait()
	s.running = false

	return nil
}

// listenForModelEvents listens for model.registered events and triggers deployment
func (s *ModelDeploymentService) listenForModelEvents(eventChan <-chan service.Event) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Stopping model event listener")
			return
		case event := <-eventChan:
			if event.Type == service.EventTypeModelTrained {
				s.handleModelTrainedEvent(event)
			}
		}
	}
}

// handleModelTrainedEvent handles a model.trained event
func (s *ModelDeploymentService) handleModelTrainedEvent(event service.Event) {
	modelID, ok := event.Data["model_id"].(string)
	if !ok || modelID == "" {
		s.logger.Warn("Model trained event missing model_id", zap.Any("event", event))
		return
	}

	s.logger.Info("Received model trained event", zap.String("model_id", modelID))

	// Trigger deployment
	err := s.TriggerDeployment(s.ctx, modelID)
	if err != nil {
		s.logger.Error("Failed to trigger deployment for model",
			zap.String("model_id", modelID),
			zap.Error(err),
		)
	}
}

// periodicCatalogScan periodically scans the model catalog for newly registered models
func (s *ModelDeploymentService) periodicCatalogScan() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Scan every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Stopping periodic catalog scan")
			return
		case <-ticker.C:
			s.scanCatalogForNewModels()
		}
	}
}

// scanCatalogForNewModels scans the catalog for models ready for deployment
func (s *ModelDeploymentService) scanCatalogForNewModels() {
	// Get all models with status "ready" (trained and ready for deployment)
	models, err := s.modelCatalog.GetModelsByStatus(modelcatalog.ModelStatusReady)
	if err != nil {
		s.logger.Error("Failed to scan catalog for new models", zap.Error(err))
		return
	}

	for _, model := range models {
		// Check if deployment already exists for this model
		deployments, err := s.orchestrator.ListDeploymentJobs(s.ctx, &DeploymentFilters{
			ModelID: model.ModelID,
		})
		if err != nil {
			s.logger.Error("Failed to check existing deployments",
				zap.String("model_id", model.ModelID),
				zap.Error(err),
			)
			continue
		}

		// If no deployments exist, trigger deployment
		if len(deployments) == 0 {
			s.logger.Info("Found new model ready for deployment",
				zap.String("model_id", model.ModelID),
			)
			err := s.TriggerDeployment(s.ctx, model.ModelID)
			if err != nil {
				s.logger.Error("Failed to trigger deployment",
					zap.String("model_id", model.ModelID),
					zap.Error(err),
				)
			}
		}
	}
}

// TriggerDeployment triggers deployment for a model
func (s *ModelDeploymentService) TriggerDeployment(ctx context.Context, modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}

	// Determine deployment targets
	targets, err := s.orchestrator.DetermineDeploymentTargets(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to determine deployment targets: %w", err)
	}

	// Create deployment jobs for each target
	for _, target := range targets {
		job, err := s.orchestrator.CreateDeploymentJob(ctx, modelID, target.EdgeID, target.CameraID)
		if err != nil {
			s.logger.Error("Failed to create deployment job",
				zap.String("model_id", modelID),
				zap.String("edge_id", target.EdgeID),
				zap.Error(err),
			)
			continue
		}

		s.logger.Info("Created deployment job",
			zap.String("deployment_id", job.DeploymentID),
			zap.String("model_id", modelID),
			zap.String("edge_id", target.EdgeID),
		)

		// Start deployment (async)
		go func(jobID string) {
			err := s.orchestrator.StartDeployment(ctx, jobID)
			if err != nil {
				s.logger.Error("Failed to start deployment",
					zap.String("deployment_id", jobID),
					zap.Error(err),
				)
			}
		}(job.DeploymentID)
	}

	return nil
}

// ManualDeploy triggers manual deployment for a model to a specific Edge
func (s *ModelDeploymentService) ManualDeploy(ctx context.Context, modelID string, edgeID string, cameraID *string) (*DeploymentJob, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID is required")
	}
	if edgeID == "" {
		return nil, fmt.Errorf("edge ID is required")
	}

	// Create deployment job
	job, err := s.orchestrator.CreateDeploymentJob(ctx, modelID, edgeID, cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment job: %w", err)
	}

	// Start deployment
	err = s.orchestrator.StartDeployment(ctx, job.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to start deployment: %w", err)
	}

	return job, nil
}

