package eventbus

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/impl"
	metastorageimpl "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/impl/metastorage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Re-export errors from types package for convenience
var (
	ErrNotInitialized  = types.ErrNotInitialized
	ErrAlreadyStarted  = types.ErrAlreadyStarted
	ErrStoragePressure = types.ErrStoragePressure
	ErrEventDropped    = types.ErrEventDropped
)

// EventBus defines the interface for an application event bus.
//
// The implementation uses meta-storage for persistence and reliability.
// It follows the vm-gateway pattern with lifecycle management and provider-agnostic design.

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_event_bus.go -package=mocks
type EventBus interface {
	// Lifecycle methods

	// Start starts the event bus service.
	// This method performs the following operations:
	//   1. Initializes the storage provider (meta-storage)
	//   2. Verifies connectivity
	//   3. Starts background tasks:
	//      - Retention cleanup worker (runs every 6 hours)
	//      - Storage pressure monitor (runs every 5 minutes)
	//      - Health check worker (runs every 1 minute)
	//   4. Initializes event drop policy
	//
	// This method should be called after all dependencies are configured.
	// If called multiple times, returns ErrAlreadyStarted.
	//
	// Returns an error if:
	//   - The service is already started (ErrAlreadyStarted)
	//   - Provider initialization fails
	//   - Background task startup fails
	Start(ctx context.Context) error

	// Stop gracefully shuts down the event bus service.
	// This method performs the following operations:
	//   1. Stops background tasks (retention cleanup, storage pressure monitor, health check)
	//   2. Flushes pending operations
	//   3. Closes provider connections
	//
	// This method should be called during service shutdown.
	// It is safe to call multiple times (idempotent).
	//
	// Returns an error if:
	//   - Background task shutdown fails
	//   - Provider shutdown fails
	Stop(ctx context.Context) error

	// Name returns the implementation name (e.g. "metastorage-event-bus").
	Name() string

	// Health monitoring

	// HealthSnapshot returns the current health status of the event bus service.
	// This follows the vm-gateway pattern for health snapshots.
	//
	// The snapshot includes:
	//   - Overall status: healthy, warning, storage_pressure, degraded
	//   - Storage pressure information
	//   - Event statistics (published, dropped, persisted)
	//   - Active subscriber count
	//   - Last cleanup time and statistics
	//   - Provider health status
	//
	// This method is safe to call frequently and does not perform expensive operations.
	//
	// Returns:
	//   - EventBusHealth containing comprehensive health information
	HealthSnapshot() types.EventBusHealth

	// Event publishing and subscription

	// Subscribe subscribes to events of a specific type.
	// The returned channel receives events until Unsubscribe is called or the bus is closed.
	//
	// Returns:
	//   - A channel that receives events of the specified type
	//   - The channel is closed when the bus is closed or Unsubscribe is called
	Subscribe(eventType types.EventType) <-chan types.EventAny

	// SubscribeAll subscribes to all events, regardless of type.
	// The returned channel receives all events until Unsubscribe is called or the bus is closed.
	//
	// Returns:
	//   - A channel that receives all events
	//   - The channel is closed when the bus is closed or Unsubscribe is called
	SubscribeAll() <-chan types.EventAny

	// Publish publishes an event to all matching subscribers and persists it to storage.
	// Implementations should be non-blocking (e.g. use buffered channels and drop on overflow).
	//
	// Event drop policy:
	//   - If storage >90% full and event is droppable (workflow trigger): drop event, log warning
	//   - If storage >90% full and event is NOT droppable (operational/health/critical): attempt to persist
	//   - If storage <90% full: persist all events normally
	//
	// Returns an error if:
	//   - The service is not started (ErrNotInitialized)
	//   - Storage pressure detected and event is droppable (ErrStoragePressure)
	//   - Storage pressure detected and event is NOT droppable but cannot be persisted (ErrEventDropped)
	Publish(event types.EventAny) error

	// Unsubscribe removes a subscription for the given event type and channel.
	// The channel is closed after unsubscribing.
	//
	// Parameters:
	//   - eventType: The event type to unsubscribe from (ignored for SubscribeAll subscriptions)
	//   - ch: The channel to unsubscribe
	Unsubscribe(eventType types.EventType, ch <-chan types.EventAny)

	// Close shuts down the event bus and closes all subscriber channels.
	// After Close, Publish and Subscribe calls should be no-ops.
	//
	// Note: This method is deprecated in favor of Stop(). Use Stop() for proper lifecycle management.
	// This method is kept for backward compatibility during migration.
	//
	// Returns an error if:
	//   - Closing subscriber channels fails
	Close() error
}

// PublishTyped publishes a typed event to the bus.
// This is a convenience function that converts Event[T] to EventAny before publishing.
func PublishTyped[T types.EventData](bus EventBus, event types.Event[T]) error {
	eventAny, err := types.ToEventAny(event)
	if err != nil {
		return err
	}
	return bus.Publish(eventAny)
}

// NewEventBus creates a new event bus instance from configuration.
// This factory function validates the configuration and creates the appropriate provider implementation.
//
// Parameters:
//   - ctx: Context for initialization
//   - config: Event bus configuration (must be validated before calling)
//   - logger: Logger for structured logging
//   - metaStore: Meta-storage service (required for meta-storage provider)
//
// Returns:
//   - An EventBus instance
//   - An error if configuration is invalid or provider creation fails
//
// Example:
//
//	config := &types.EventBusConfig{
//		Provider:   "metastorage",
//		BufferSize: 100,
//	}
//	if err := config.Validate(); err != nil {
//		log.Fatal(err)
//	}
//	bus, err := eventbus.NewEventBus(ctx, config, logger, &metaStore)
//	if err != nil {
//		log.Fatal(err)
//	}
func NewEventBus(ctx context.Context, config *types.EventBusConfig, logger *zap.Logger, metaStore *metastorage.MetaDataStore) (EventBus, error) {
	if config == nil {
		return nil, fmt.Errorf("event bus config is required")
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event bus config: %w", err)
	}

	if config.Provider != "metastorage" {
		return nil, fmt.Errorf("unsupported event bus provider: %s (only 'metastorage' is supported)", config.Provider)
	}

	if metaStore == nil || *metaStore == nil {
		return nil, fmt.Errorf("meta-storage is required for metastorage event bus provider")
	}

	// Create meta-storage provider
	metaStorageProvider, err := metastorageimpl.NewMetaStorageProvider(*metaStore, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create meta-storage provider: %w", err)
	}

	// Create event bus implementation (pass meta-storage for storage pressure monitoring)
	busImpl, err := impl.NewEventBusImpl(metaStorageProvider, config, logger, *metaStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create event bus implementation: %w", err)
	}

	return busImpl, nil
}

// EventBusProvider creates the event bus from config with fx lifecycle management.
// This follows the vm-gateway pattern for dependency injection.
//
// The service lifecycle is managed by Fx:
//   - OnStart: Calls bus.Start(ctx)
//   - OnStop: Calls bus.Stop(ctx)
//
// Parameters:
//   - lc: Fx lifecycle for service management
//   - cfg: Event bus configuration
//   - logger: Logger for structured logging
//   - metaStore: Meta-storage service (required for meta-storage provider)
//
// Returns:
//   - An EventBus instance with lifecycle hooks registered
//   - An error if configuration is invalid or provider creation fails
//
// Example:
//
//	var Module = fx.Module("event-bus",
//		fx.Provide(eventbus.EventBusProvider),
//	)
func EventBusProvider(lc fx.Lifecycle, cfg *types.EventBusConfig, logger *zap.Logger, metaStore *metastorage.MetaDataStore) (EventBus, error) {
	bus, err := NewEventBus(context.Background(), cfg, logger, metaStore)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting event bus service")
			if err := bus.Start(ctx); err != nil {
				logger.Error("Failed to start event bus", zap.Error(err))
				return err
			}
			logger.Info("Event bus service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping event bus service")
			if err := bus.Stop(ctx); err != nil {
				logger.Error("Failed to stop event bus", zap.Error(err))
				return err
			}
			logger.Info("Event bus service stopped")
			return nil
		},
	})

	return bus, nil
}
