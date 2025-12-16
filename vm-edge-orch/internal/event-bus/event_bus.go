package eventbus

import (
	"time"
)

// EventType represents the type of an event.
// Using string keeps it flexible while still allowing typed constants.
type EventType string

// Event represents an application-level event flowing through the bus.
type Event struct {
	Type      EventType
	Source    string                 // Component that emitted the event
	Timestamp time.Time              // When the event was created
	Data      map[string]interface{} // Arbitrary event-specific payload
}

// EventBus defines the interface for an application event bus.
//
// Implementations can be in-memory, NATS-backed, or anything else that
// satisfies this contract.
type EventBus interface {
	// Name returns the implementation name (e.g. "inmemory-event-bus", "nats-event-bus").
	Name() string

	// Subscribe subscribes to events of a specific type.
	// The returned channel receives events until Unsubscribe is called or the bus is closed.
	Subscribe(eventType EventType) <-chan Event

	// SubscribeAll subscribes to all events, regardless of type.
	SubscribeAll() <-chan Event

	// Publish publishes an event to all matching subscribers.
	// Implementations should be non-blocking (e.g. use buffered channels and drop on overflow).
	Publish(event Event)

	// Unsubscribe removes a subscription for the given event type and channel.
	Unsubscribe(eventType EventType, ch <-chan Event)

	// Close shuts down the event bus and closes all subscriber channels.
	// After Close, Publish and Subscribe calls should be no-ops.
	Close() error
}


