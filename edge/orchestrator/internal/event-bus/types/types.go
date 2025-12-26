package types

import "time"

// EventType represents the type of an event.
// Using string keeps it flexible while still allowing typed constants.
type EventType string

// Event represents an application-level event flowing through the bus.
type Event struct {
	Type           EventType
	Source         string                 // Component that emitted the event
	Timestamp      time.Time              // When the event was created
	Data           map[string]interface{} // Arbitrary event-specific payload
	SequenceNumber int64                  // Sequence number per source (0 if not set)
}

// EventProcessingStatus represents the processing status of an event
type EventProcessingStatus string

const (
	EventStatusPending    EventProcessingStatus = "pending"     // Event is pending processing
	EventStatusProcessing  EventProcessingStatus = "processing"  // Event is currently being processed
	EventStatusSucceeded  EventProcessingStatus = "succeeded"   // Event was successfully processed
	EventStatusFailed     EventProcessingStatus = "failed"      // Event processing failed (will be retried)
	EventStatusDeadLetter EventProcessingStatus = "dead_letter" // Event moved to dead letter queue after max retries
)

// OrderingMode defines how events are ordered
type OrderingMode string

const (
	OrderingModeNone        OrderingMode = "none"         // No ordering guarantees
	OrderingModeBestEffort  OrderingMode = "best_effort"  // Best-effort ordering (reorder if possible)
	OrderingModeStrict      OrderingMode = "strict"       // Strict ordering (buffer and wait for missing sequences)
)

// EventBusConfig contains event bus configuration
type EventBusConfig struct {
	Provider        string        `yaml:"provider"`          // Event bus provider (e.g., "inmemory", "bbolt", "nats")
	BufferSize      int           `yaml:"buffer_size"`       // Buffer size for event channels
	DataDir         string        `yaml:"data_dir"`          // Data directory for persistent storage (used by bbolt)
	MaxRetries      int           `yaml:"max_retries"`       // Maximum number of retry attempts for failed events
	InitialBackoff  time.Duration `yaml:"initial_backoff"`   // Initial backoff duration for retries
	MaxBackoff      time.Duration `yaml:"max_backoff"`       // Maximum backoff duration (caps exponential backoff)
	BackoffMultiplier float64     `yaml:"backoff_multiplier"` // Multiplier for exponential backoff (e.g., 2.0)
	RetryInterval   time.Duration `yaml:"retry_interval"`    // Interval between retry worker runs
	OrderingMode    string        `yaml:"ordering_mode"`     // Event ordering mode: "none", "best_effort", "strict"
	OrderingBufferSize int        `yaml:"ordering_buffer_size"` // Buffer size for out-of-order event buffering
	OrderingTimeout  time.Duration `yaml:"ordering_timeout"`   // Timeout for waiting for missing sequences in strict mode
}
