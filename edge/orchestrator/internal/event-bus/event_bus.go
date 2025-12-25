package eventbus

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/inmemory"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
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
	Subscribe(eventType types.EventType) <-chan types.Event

	// SubscribeAll subscribes to all events, regardless of type.
	SubscribeAll() <-chan types.Event

	// Publish publishes an event to all matching subscribers.
	// Implementations should be non-blocking (e.g. use buffered channels and drop on overflow).
	Publish(event types.Event)

	// Unsubscribe removes a subscription for the given event type and channel.
	Unsubscribe(eventType types.EventType, ch <-chan types.Event)

	// Close shuts down the event bus and closes all subscriber channels.
	// After Close, Publish and Subscribe calls should be no-ops.
	Close() error
}

func NewEventBus(ctx context.Context, config *types.EventBusConfig, logger *zap.Logger) (EventBus, error) {
	switch config.Provider {
	case "inmemory":
		return inmemory.NewInMemoryEventBus(config.BufferSize), nil
	default:
		return nil, fmt.Errorf("unsupported event bus provider: %s", config.Provider)
	}
}

// EventBusProvider creates the event bus from config with fx lifecycle management
func EventBusProvider(lc fx.Lifecycle, cfg *types.EventBusConfig, logger *zap.Logger) (EventBus, error) {
	bus, err := NewEventBus(context.Background(), cfg, logger)
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
