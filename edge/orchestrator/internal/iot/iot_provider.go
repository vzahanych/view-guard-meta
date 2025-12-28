package iot

import (
	"context"
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// IoTServiceProvider provides an IoTService instance with fx lifecycle management.
//
// The IoT service is a complex service with multiple internal components:
//   - Plugin registry (device type plugin management)
//   - Device registry (device discovery and registration)
//   - State registry (device state machine management)
//   - Processing service (data processing pipeline)
//   - Hook registry (lifecycle hook management)
//
// All dependencies are injected via fx at construction time.
//
// Architecture decision: Service-owned lifecycle.
//   - Fx manages only the IoT service lifecycle.
//   - IoT Service Start/Stop is the single place that starts/stops sub-services.
//   - Sub-services do not register their own fx.Lifecycle hooks.
//
// Fail-fast behavior: This provider will return an error (not nil) if:
//   - Configuration is invalid or unsupported
//   - Service creation fails
//   - Required dependencies are missing
//
// The application will not start if IoT service creation fails, ensuring production reliability.
func IoTServiceProvider(
	lc fx.Lifecycle,
	config *types.IoTServiceConfig,
	logger *zap.Logger,
) (IoTService, error) {
	// Validate configuration before creating service
	if config != nil {
		if err := config.Validate(); err != nil {
			logger.Error("Invalid IoT service configuration", zap.Error(err))
			return nil, fmt.Errorf("invalid IoT service configuration: %w", err)
		}
	}

	// Create subcomponents in dependency order:
	// 1. Plugin registry (no dependencies)
	pluginRegistry := pluginregistry.NewDevicePluginRegistry(logger)

	// 2. State machine factory (no dependencies)
	stateFactory := statemachine.NewDeviceStateMachineFactory(logger)

	// 3. State machine registry (depends on factory)
	stateRegistry := statemachine.NewDeviceStateMachineRegistry(stateFactory, logger)

	// 4. Processing registry (no dependencies)
	processingRegistry := processing.NewDataProcessorRegistry(logger)

	// 5. Processing service (depends on registry)
	processingService := processing.NewDataProcessingService(processingRegistry, logger)

	// 6. Hook registry (no dependencies)
	hookRegistry := hooks.NewLifecycleHookRegistry(logger)

	// 7. Device registry (depends on plugin registry, state registry, hook registry)
	deviceRegistry := deviceregistry.NewDeviceRegistry(
		pluginRegistry,
		stateRegistry,
		hookRegistry,
		logger,
	)

	// Create the service with all subcomponents
	service := NewIoTService(
		pluginRegistry,
		deviceRegistry,
		stateRegistry,
		processingService,
		hookRegistry,
		config,
		logger,
	)

	// Setup lifecycle hooks - service owns the lifecycle of all sub-services
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting IoT Service (all sub-services)...")
			return service.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping IoT Service...")
			return service.Stop(ctx)
		},
	})

	return service, nil
}

