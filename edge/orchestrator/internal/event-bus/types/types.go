package types

import "time"

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

// EventBusConfig contains event bus configuration
type EventBusConfig struct {
	Provider   string `yaml:"provider"`    // Event bus provider (e.g., "inmemory", "nats")
	BufferSize int    `yaml:"buffer_size"` // Buffer size for event channels
}
