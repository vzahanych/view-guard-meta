package nats

import (
	"fmt"

	eventbus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus"
)

// natsEventBus is a placeholder implementation of EventBus backed by NATS.
// For now, it simply returns an error on Publish/Subscribe and reports its name.
// This allows the application to compile while reserving the extension point.
type natsEventBus struct{}

// NewNATSEventBus creates a new NATS-backed event bus placeholder.
// A real implementation can be added later without changing callers.
func NewNATSEventBus() eventbus.EventBus {
	return &natsEventBus{}
}

// Name returns the implementation name.
func (b *natsEventBus) Name() string {
	return "nats-event-bus"
}

// Subscribe currently panics to signal that NATS support is not implemented yet.
func (b *natsEventBus) Subscribe(eventType eventbus.EventType) <-chan eventbus.Event {
	panic("nats-event-bus: Subscribe not implemented yet")
}

// SubscribeAll currently panics to signal that NATS support is not implemented yet.
func (b *natsEventBus) SubscribeAll() <-chan eventbus.Event {
	panic("nats-event-bus: SubscribeAll not implemented yet")
}

// Publish currently returns an error to signal that NATS support is not implemented yet.
func (b *natsEventBus) Publish(event eventbus.Event) {
	panic(fmt.Sprintf("nats-event-bus: Publish not implemented yet (event type=%s)", event.Type))
}

// Unsubscribe currently panics to signal that NATS support is not implemented yet.
func (b *natsEventBus) Unsubscribe(eventType eventbus.EventType, ch <-chan eventbus.Event) {
	panic("nats-event-bus: Unsubscribe not implemented yet")
}

// Close currently does nothing and returns nil.
func (b *natsEventBus) Close() error {
	return nil
}


