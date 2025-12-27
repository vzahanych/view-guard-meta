package inmemory

import (
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
)

// inMemoryEventBus is an in-process implementation of EventBus.
// It is safe for concurrent use.
type InMemoryEventBus struct {
	mu          sync.RWMutex
	subscribers map[types.EventType][]chan types.EventAny
	allSubs     []chan types.EventAny
	bufferSize  int
	closed      bool
}

// NewInMemoryEventBus creates a new in-memory event bus implementation.
// bufferSize controls the size of subscriber channels; non-positive values
// are treated as a default of 100.
func NewInMemoryEventBus(bufferSize int) *InMemoryEventBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &InMemoryEventBus{
		subscribers: make(map[types.EventType][]chan types.EventAny),
		allSubs:     make([]chan types.EventAny, 0),
		bufferSize:  bufferSize,
	}
}

// Name returns the implementation name.
func (b *InMemoryEventBus) Name() string {
	return "inmemory-event-bus"
}

// Subscribe subscribes to events of a specific type.
func (b *InMemoryEventBus) Subscribe(eventType types.EventType) <-chan types.EventAny {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan types.EventAny)
		close(ch)
		return ch
	}

	ch := make(chan types.EventAny, b.bufferSize)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// SubscribeAll subscribes to all events, regardless of type.
func (b *InMemoryEventBus) SubscribeAll() <-chan types.EventAny {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan types.EventAny)
		close(ch)
		return ch
	}

	ch := make(chan types.EventAny, b.bufferSize)
	b.allSubs = append(b.allSubs, ch)
	return ch
}

// Publish publishes an event to all matching subscribers.
func (b *InMemoryEventBus) Publish(event types.EventAny) {
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
func (b *InMemoryEventBus) Unsubscribe(eventType types.EventType, ch <-chan types.EventAny) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If already closed, channels are already closed by Close(), so don't close again
	if b.closed {
		return
	}

	// Remove from specific subscribers and close the channel
	if subs, ok := b.subscribers[eventType]; ok {
		filtered := make([]chan types.EventAny, 0, len(subs))
		for _, sub := range subs {
			if sub != ch {
				filtered = append(filtered, sub)
			} else {
				// Close channel safely (recover from panic if already closed)
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel was already closed, ignore panic
						}
					}()
					close(sub)
				}()
			}
		}
		if len(filtered) == 0 {
			delete(b.subscribers, eventType)
		} else {
			b.subscribers[eventType] = filtered
		}
	}

	// Remove from allSubs if present and close the channel
	if len(b.allSubs) > 0 {
		filteredAll := make([]chan types.EventAny, 0, len(b.allSubs))
		for _, sub := range b.allSubs {
			if sub != ch {
				filteredAll = append(filteredAll, sub)
			} else {
				// Close channel safely (recover from panic if already closed)
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel was already closed, ignore panic
						}
					}()
					close(sub)
				}()
			}
		}
		b.allSubs = filteredAll
	}
}

// Close shuts down the event bus and closes all subscriber channels.
func (b *InMemoryEventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all channels safely (in case Unsubscribe() already closed some)
	closeChannel := func(ch chan types.EventAny) {
		defer func() {
			// Recover from panic if channel is already closed
			if r := recover(); r != nil {
				// Channel was already closed, ignore
			}
		}()
		close(ch)
	}

	for _, subs := range b.subscribers {
		for _, sub := range subs {
			closeChannel(sub)
		}
	}
	for _, sub := range b.allSubs {
		closeChannel(sub)
	}

	b.subscribers = make(map[types.EventType][]chan types.EventAny)
	b.allSubs = make([]chan types.EventAny, 0)

	return nil
}
