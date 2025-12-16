package inmemory

import (
	"sync"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus"
)

// inMemoryEventBus is an in-process implementation of EventBus.
// It is safe for concurrent use.
type inMemoryEventBus struct {
	mu         sync.RWMutex
	subscribers map[eventbus.EventType][]chan eventbus.Event
	allSubs     []chan eventbus.Event
	bufferSize  int
	closed      bool
}

// NewInMemoryEventBus creates a new in-memory event bus implementation.
// bufferSize controls the size of subscriber channels; non-positive values
// are treated as a default of 100.
func NewInMemoryEventBus(bufferSize int) eventbus.EventBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &inMemoryEventBus{
		subscribers: make(map[eventbus.EventType][]chan eventbus.Event),
		allSubs:     make([]chan eventbus.Event, 0),
		bufferSize:  bufferSize,
	}
}

// Name returns the implementation name.
func (b *inMemoryEventBus) Name() string {
	return "inmemory-event-bus"
}

// Subscribe subscribes to events of a specific type.
func (b *inMemoryEventBus) Subscribe(eventType eventbus.EventType) <-chan eventbus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan eventbus.Event)
		close(ch)
		return ch
	}

	ch := make(chan eventbus.Event, b.bufferSize)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// SubscribeAll subscribes to all events, regardless of type.
func (b *inMemoryEventBus) SubscribeAll() <-chan eventbus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan eventbus.Event)
		close(ch)
		return ch
	}

	ch := make(chan eventbus.Event, b.bufferSize)
	b.allSubs = append(b.allSubs, ch)
	return ch
}

// Publish publishes an event to all matching subscribers.
func (b *inMemoryEventBus) Publish(event eventbus.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Send to specific subscribers
	if subs, ok := b.subscribers[event.Type]; ok {
		for _, sub := range subs {
			select {
			case sub <- event:
			default:
				// Channel full, drop event (non-blocking)
			}
		}
	}

	// Send to "all" subscribers
	for _, sub := range b.allSubs {
		select {
		case sub <- event:
		default:
			// Channel full, drop event
		}
	}
}

// Unsubscribe removes a subscription for the given event type and channel.
func (b *inMemoryEventBus) Unsubscribe(eventType eventbus.EventType, ch <-chan eventbus.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from specific subscribers
	if subs, ok := b.subscribers[eventType]; ok {
		filtered := make([]chan eventbus.Event, 0, len(subs))
		for _, sub := range subs {
			if sub != ch {
				filtered = append(filtered, sub)
			} else {
				close(sub)
			}
		}
		if len(filtered) == 0 {
			delete(b.subscribers, eventType)
		} else {
			b.subscribers[eventType] = filtered
		}
	}

	// Remove from allSubs if present
	if len(b.allSubs) > 0 {
		filteredAll := make([]chan eventbus.Event, 0, len(b.allSubs))
		for _, sub := range b.allSubs {
			if sub != ch {
				filteredAll = append(filteredAll, sub)
			} else {
				close(sub)
			}
		}
		b.allSubs = filteredAll
	}
}

// Close shuts down the event bus and closes all subscriber channels.
func (b *inMemoryEventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	for _, subs := range b.subscribers {
		for _, sub := range subs {
			close(sub)
		}
	}
	for _, sub := range b.allSubs {
		close(sub)
	}

	b.subscribers = make(map[eventbus.EventType][]chan eventbus.Event)
	b.allSubs = make([]chan eventbus.Event, 0)

	return nil
}


