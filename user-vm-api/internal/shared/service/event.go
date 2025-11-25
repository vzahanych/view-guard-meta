package service

import (
	"sync"
)

// EventType represents the type of event
type EventType string

const (
	// System events
	EventTypeServiceStarted EventType = "service_started"
	EventTypeServiceStopped EventType = "service_stopped"
	EventTypeServiceError   EventType = "service_error"

	// WireGuard events
	EventTypeWireGuardClientConnected    EventType = "wireguard_client_connected"
	EventTypeWireGuardClientDisconnected EventType = "wireguard_client_disconnected"

	// Event cache events
	EventTypeEventReceived EventType = "event_received"
	EventTypeEventForwarded EventType = "event_forwarded"

	// AI orchestrator events
	EventTypeModelTrained    EventType = "model_trained"
	EventTypeModelDeployed   EventType = "model_deployed"
	EventTypeTrainingStarted EventType = "training_started"
	EventTypeTrainingFailed  EventType = "training_failed"

	// Event analyzer events
	EventTypeEventAnalyzed EventType = "event_analyzed"
	EventTypeAlertGenerated EventType = "alert_generated"

	// Storage events
	EventTypeClipArchived EventType = "clip_archived"
	EventTypeArchiveFailed EventType = "archive_failed"
)

// Event represents an event in the system
type Event struct {
	Type      EventType              `json:"type"`
	Source    string                 `json:"source"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// EventBus provides pub/sub event communication between services
type EventBus struct {
	subscribers map[EventType][]chan Event
	mu          sync.RWMutex
	bufferSize  int
}

// NewEventBus creates a new event bus
func NewEventBus(bufferSize int) *EventBus {
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
	// Subscribe to all known event types
	for eventType := range eb.subscribers {
		eb.subscribers[eventType] = append(eb.subscribers[eventType], ch)
	}

	return ch
}

// Publish publishes an event
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	subscribers := eb.subscribers[event.Type]
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip (non-blocking)
		}
	}
}

// Close closes all subscriber channels
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for _, subscribers := range eb.subscribers {
		for _, ch := range subscribers {
			close(ch)
		}
	}
	eb.subscribers = make(map[EventType][]chan Event)
}

