package service

import (
	"context"
	"sync"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	// System events
	EventTypeServiceStarted EventType = "service.started"
	EventTypeServiceStopped EventType = "service.stopped"
	EventTypeServiceError   EventType = "service.error"

	// Camera events
	EventTypeCameraDiscovered EventType = "camera.discovered"
	EventTypeCameraConnected   EventType = "camera.connected"
	EventTypeCameraDisconnected EventType = "camera.disconnected"

	// Video events
	EventTypeFrameReceived EventType = "video.frame_received"
	EventTypeClipRecorded  EventType = "video.clip_recorded"

	// AI events
	EventTypeDetection EventType = "ai.detection"
	EventTypeInference EventType = "ai.inference"

	// Storage events
	EventTypeStorageFull    EventType = "storage.full"
	EventTypeStorageWarning EventType = "storage.warning"

	// Network events
	EventTypeWireGuardConnected    EventType = "network.wireguard.connected"
	EventTypeWireGuardDisconnected EventType = "network.wireguard.disconnected"
)

// Event represents an event in the system
type Event struct {
	Type      EventType
	Source    string                 // Service that emitted the event
	Timestamp time.Time
	Data      map[string]interface{} // Event-specific data
}

// EventBus provides inter-service communication via events
type EventBus struct {
	subscribers map[EventType][]chan Event
	mu          sync.RWMutex
	bufferSize  int
}

// NewEventBus creates a new event bus
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	return &EventBus{
		subscribers: make(map[EventType][]chan Event),
		bufferSize:  bufferSize,
	}
}

// Subscribe subscribes to events of a specific type
func (eb *EventBus) Subscribe(eventType EventType) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, eb.bufferSize)
	eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
	return ch
}

// SubscribeAll subscribes to all events
func (eb *EventBus) SubscribeAll() <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, eb.bufferSize)
	// Subscribe to all existing event types
	for eventType := range eb.subscribers {
		eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
	}
	return ch
}

// Publish publishes an event to all subscribers
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Send to specific subscribers
	if subs, ok := eb.subscribers[event.Type]; ok {
		for _, sub := range subs {
			select {
			case sub <- event:
			default:
				// Channel full, skip (non-blocking)
			}
		}
	}
}

// Unsubscribe removes a subscription
func (eb *EventBus) Unsubscribe(eventType EventType, ch <-chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	subs := eb.subscribers[eventType]
	for i, sub := range subs {
		if sub == ch {
			eb.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			close(sub)
			break
		}
	}
}

// Close closes all subscriptions and cleans up
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType, subs := range eb.subscribers {
		for _, sub := range subs {
			close(sub)
		}
		delete(eb.subscribers, eventType)
	}
}

// EventHandler is a function that handles events
type EventHandler func(ctx context.Context, event Event) error

// SubscribeWithHandler subscribes to events and handles them with a function
func (eb *EventBus) SubscribeWithHandler(ctx context.Context, eventType EventType, handler EventHandler) {
	ch := eb.Subscribe(eventType)
	go func() {
		defer eb.Unsubscribe(eventType, ch)
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				if err := handler(ctx, event); err != nil {
					// Handler error - could log or handle differently
					_ = err
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

