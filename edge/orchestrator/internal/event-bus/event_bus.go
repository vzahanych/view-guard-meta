package eventbus

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/bboltebus"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/inmemory"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/metastoragebus"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// EventBus defines the interface for an application event bus.
//
// Implementations can be in-memory, NATS-backed, or anything else that
// satisfies this contract.

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_event_bus.go -package=mocks
type EventBus interface {
	// Name returns the implementation name (e.g. "inmemory-event-bus", "nats-event-bus").
	Name() string

	// Subscribe subscribes to events of a specific type.
	// The returned channel receives events until Unsubscribe is called or the bus is closed.
	Subscribe(eventType types.EventType) <-chan types.EventAny

	// SubscribeAll subscribes to all events, regardless of type.
	SubscribeAll() <-chan types.EventAny

	// Publish publishes an event to all matching subscribers.
	// Implementations should be non-blocking (e.g. use buffered channels and drop on overflow).
	Publish(event types.EventAny)

	// Unsubscribe removes a subscription for the given event type and channel.
	Unsubscribe(eventType types.EventType, ch <-chan types.EventAny)

	// Close shuts down the event bus and closes all subscriber channels.
	// After Close, Publish and Subscribe calls should be no-ops.
	Close() error
}

// PublishTyped publishes a typed event to the bus.
// This is a convenience function that converts Event[T] to EventAny before publishing.
func PublishTyped[T types.EventData](bus EventBus, event types.Event[T]) error {
	eventAny, err := types.ToEventAny(event)
	if err != nil {
		return err
	}
	bus.Publish(eventAny)
	return nil
}

func NewEventBus(ctx context.Context, config *types.EventBusConfig, logger *zap.Logger, metaStore *metastorage.MetaDataStore) (EventBus, error) {
	switch config.Provider {
	case "inmemory":
		return inmemory.NewInMemoryEventBus(config.BufferSize), nil
	case "bbolt":
		if config.DataDir == "" {
			return nil, fmt.Errorf("data_dir is required for bbolt event bus provider")
		}
		dbPath := filepath.Join(config.DataDir, "db", "event-bus.db")
		return bboltebus.NewBboltEventBus(dbPath, config.BufferSize, logger)
	case "metastorage":
		if metaStore == nil || *metaStore == nil {
			return nil, fmt.Errorf("meta-storage is required for metastorage event bus provider")
		}
		// Create retry config if retry is enabled
		var retryConfig *metastoragebus.RetryConfig
		if config.MaxRetries > 0 {
			retryConfig = &metastoragebus.RetryConfig{
				MaxRetries:       config.MaxRetries,
				InitialBackoff:   config.InitialBackoff,
				MaxBackoff:       config.MaxBackoff,
				BackoffMultiplier: config.BackoffMultiplier,
				RetryInterval:   config.RetryInterval,
			}
		}
		// Create ordering config if ordering is enabled
		var orderingConfig *metastoragebus.OrderingConfig
		if config.OrderingMode != "" && config.OrderingMode != string(types.OrderingModeNone) {
			orderingMode := types.OrderingMode(config.OrderingMode)
			if orderingMode == types.OrderingModeBestEffort || orderingMode == types.OrderingModeStrict {
				bufferSize := config.OrderingBufferSize
				if bufferSize <= 0 {
					bufferSize = 100 // Default buffer size
				}
				timeout := config.OrderingTimeout
				if timeout <= 0 {
					timeout = 30 * time.Second // Default timeout
				}
				orderingConfig = &metastoragebus.OrderingConfig{
					Mode:       orderingMode,
					BufferSize: bufferSize,
					Timeout:    timeout,
				}
			}
		}
		return metastoragebus.NewMetaStorageEventBus(*metaStore, config.BufferSize, logger, retryConfig, orderingConfig)
	default:
		return nil, fmt.Errorf("unsupported event bus provider: %s", config.Provider)
	}
}

// EventBusProvider creates the event bus from config with fx lifecycle management
// metaStore is optional and only required when provider is "metastorage"
// Note: If using "metastorage" provider, meta-storage must be provided before event-bus
func EventBusProvider(lc fx.Lifecycle, cfg *types.EventBusConfig, logger *zap.Logger, metaStore *metastorage.MetaDataStore) (EventBus, error) {
	bus, err := NewEventBus(context.Background(), cfg, logger, metaStore)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Event bus started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Event bus stopping")
			if err := bus.Close(); err != nil {
				logger.Error("Failed to close event bus", zap.Error(err))
				return err
			}
			logger.Info("Event bus stopped")
			return nil
		},
	})

	return bus, nil
}
