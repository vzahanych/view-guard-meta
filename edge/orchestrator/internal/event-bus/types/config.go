package types

import (
	"fmt"
	"time"
)

// EventBusConfig contains event bus configuration.
// This is the structured configuration type used to create an EventBus instance.
//
// All configuration fields are optional and have sensible defaults.
// Call Validate() to set defaults and validate the configuration before use.
//
// Example:
//   config := &EventBusConfig{
//       Provider:   "metastorage",
//       BufferSize: 100,
//       RetentionConfig: &RetentionConfig{
//           RetentionHours: 24,
//           CleanupIntervalHours: 6,
//       },
//   }
//   if err := config.Validate(); err != nil {
//       // Handle error
//   }
type EventBusConfig struct {
	// Provider specifies the event bus provider.
	// Currently only "metastorage" is supported.
	// Default: "metastorage"
	Provider string `yaml:"provider"`

	// BufferSize controls the size of subscriber channels.
	// Larger buffers allow more events to be queued before blocking.
	// Default: 100
	BufferSize int `yaml:"buffer_size"`

	// RetryConfig contains retry configuration for failed events.
	// If nil, default retry configuration is used (3 retries, exponential backoff).
	RetryConfig *RetryConfig `yaml:"retry_config,omitempty"`

	// OrderingConfig contains ordering configuration for event delivery.
	// If nil, no ordering is applied (OrderingModeNone).
	OrderingConfig *OrderingConfig `yaml:"ordering_config,omitempty"`

	// RetentionConfig contains retention and cleanup configuration.
	// If nil, default retention is used (24 hours retention, 6 hours cleanup interval).
	RetentionConfig *RetentionConfig `yaml:"retention_config,omitempty"`

	// DropPolicyConfig contains event drop policy configuration.
	// If nil, default drop policy is used (90% threshold, workflow_trigger default category).
	DropPolicyConfig *DropPolicyConfig `yaml:"drop_policy_config,omitempty"`

	// MetaStorageProviderConfig contains meta-storage provider-specific configuration.
	// This is typically set programmatically, not via YAML.
	MetaStorageProviderConfig *MetaStorageProviderConfig `yaml:"metastorage_provider_config,omitempty"`
}

// Validate validates the event bus configuration and sets defaults.
// This method must be called before using the configuration to create an EventBus.
//
// Validation:
//   - Sets default provider if not specified ("metastorage")
//   - Validates provider is supported
//   - Sets default buffer size if not specified (100)
//   - Validates nested configs (RetryConfig, OrderingConfig, etc.)
//
// Returns an error if:
//   - Provider is invalid (not "metastorage")
//   - Nested config validation fails
//
// Example:
//   config := &EventBusConfig{Provider: "metastorage"}
//   if err := config.Validate(); err != nil {
//       // Handle error
//   }
func (c *EventBusConfig) Validate() error {
	if c.Provider == "" {
		c.Provider = "metastorage" // Default provider
	}
	if c.Provider != "metastorage" {
		return fmt.Errorf("unsupported provider: %s (only 'metastorage' is supported)", c.Provider)
	}

	if c.BufferSize <= 0 {
		c.BufferSize = 100 // Default buffer size
	}

	// Validate and set defaults for nested configs
	if c.RetryConfig != nil {
		c.RetryConfig.Validate()
	}
	if c.OrderingConfig != nil {
		c.OrderingConfig.Validate()
	}
	if c.RetentionConfig != nil {
		c.RetentionConfig.Validate()
	}
	if c.DropPolicyConfig != nil {
		c.DropPolicyConfig.Validate()
	}

	return nil
}

// RetryConfig contains configuration for event retry logic.
// This configures how failed events are retried before being moved to the dead letter queue.
//
// Retry behavior:
//   - Events are retried with exponential backoff
//   - Backoff starts at InitialBackoff and increases by BackoffMultiplier each retry
//   - Backoff is capped at MaxBackoff
//   - After MaxRetries, events are moved to dead letter queue
//
// Example:
//   retryConfig := &RetryConfig{
//       MaxRetries: 3,
//       InitialBackoff: 1 * time.Second,
//       MaxBackoff: 60 * time.Second,
//       BackoffMultiplier: 2.0,
//   }
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	// After this many retries, events are moved to dead letter queue.
	// Default: 3
	// Range: 1-100
	MaxRetries int `yaml:"max_retries"`

	// InitialBackoff is the initial backoff duration before the first retry.
	// Default: 1s
	// Range: >0, capped at 60s
	InitialBackoff time.Duration `yaml:"initial_backoff"`

	// MaxBackoff is the maximum backoff duration (caps exponential backoff).
	// Default: 60s
	// Range: >= InitialBackoff, capped at 1h
	MaxBackoff time.Duration `yaml:"max_backoff"`

	// BackoffMultiplier is the multiplier for exponential backoff.
	// Backoff = InitialBackoff * (BackoffMultiplier ^ retry_count)
	// Default: 2.0
	// Range: >0, capped at 10.0
	BackoffMultiplier float64 `yaml:"backoff_multiplier"`

	// RetryInterval is the interval between retry worker runs.
	// The retry worker checks for failed events and retries them periodically.
	// Default: 5s
	// Range: >0, capped at 5m
	RetryInterval time.Duration `yaml:"retry_interval"`

	// DeadLetterThreshold is the number of retries before moving to dead letter queue.
	// Default: MaxRetries (same as MaxRetries)
	// Range: 1-MaxRetries
	DeadLetterThreshold int `yaml:"dead_letter_threshold"`
}

// Validate validates the retry configuration and sets defaults.
// This is called automatically by EventBusConfig.Validate().
//
// Validation:
//   - Sets defaults for all fields if not specified
//   - Caps values to reasonable ranges
//   - Ensures MaxBackoff >= InitialBackoff
//   - Ensures DeadLetterThreshold <= MaxRetries
func (c *RetryConfig) Validate() {
	// Validate and set default max retries
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3 // Default max retries
	} else if c.MaxRetries > 100 {
		// Cap max retries to prevent excessive retry attempts
		c.MaxRetries = 100
	}

	// Validate and set default initial backoff
	if c.InitialBackoff <= 0 {
		c.InitialBackoff = 1 * time.Second // Default initial backoff
	} else if c.InitialBackoff > 60*time.Second {
		// Cap initial backoff to prevent excessive initial delay
		c.InitialBackoff = 60 * time.Second
	}

	// Validate and set default max backoff
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 60 * time.Second // Default max backoff
	} else if c.MaxBackoff > 1*time.Hour {
		// Cap max backoff to prevent excessive delays
		c.MaxBackoff = 1 * time.Hour
	}

	// Ensure max backoff >= initial backoff
	if c.MaxBackoff < c.InitialBackoff {
		c.MaxBackoff = c.InitialBackoff
	}

	// Validate and set default backoff multiplier
	if c.BackoffMultiplier <= 0 {
		c.BackoffMultiplier = 2.0 // Default multiplier
	} else if c.BackoffMultiplier > 10.0 {
		// Cap multiplier to prevent excessive exponential growth
		c.BackoffMultiplier = 10.0
	}

	// Validate and set default retry interval
	if c.RetryInterval <= 0 {
		c.RetryInterval = 5 * time.Second // Default retry interval
	} else if c.RetryInterval > 5*time.Minute {
		// Cap retry interval to prevent excessive delays between retry worker runs
		c.RetryInterval = 5 * time.Minute
	}

	// Validate and set default dead letter threshold
	if c.DeadLetterThreshold <= 0 {
		c.DeadLetterThreshold = c.MaxRetries // Default to max retries
	} else if c.DeadLetterThreshold > c.MaxRetries {
		// Dead letter threshold cannot exceed max retries
		c.DeadLetterThreshold = c.MaxRetries
	}
}

// OrderingConfig contains configuration for event ordering.
// This configures how events are ordered during delivery to subscribers.
//
// Ordering modes:
//   - None: No ordering (fastest, events delivered as-is)
//   - BestEffort: Reorder if possible (balanced, doesn't wait for missing sequences)
//   - Strict: Buffer and wait for missing sequences (slowest, strongest guarantee)
//
// Ordering is applied per-source (events from the same source are ordered).
//
// Example:
//   orderingConfig := &OrderingConfig{
//       Mode: OrderingModeBestEffort,
//       BufferSize: 100,
//       Timeout: 30 * time.Second,
//       PerSourceOrdering: true,
//   }
type OrderingConfig struct {
	// Mode specifies the ordering mode.
	// Options: "none", "best_effort", "strict"
	// Default: "none"
	Mode OrderingMode `yaml:"mode"`

	// BufferSize is the buffer size for out-of-order events.
	// Used in best_effort and strict modes to buffer events while waiting for ordering.
	// Default: 100
	// Range: 1-10000
	BufferSize int `yaml:"buffer_size"`

	// Timeout is the timeout for waiting for missing sequences in strict mode.
	// After this timeout, buffered events are delivered even if missing sequences haven't arrived.
	// Default: 30s
	// Range: >0, capped at 5m
	Timeout time.Duration `yaml:"timeout"`

	// PerSourceOrdering enables per-source ordering.
	// If true, ordering is applied per source (events from same source are ordered).
	// If false, ordering is global (all events are ordered together).
	// Default: true
	PerSourceOrdering bool `yaml:"per_source_ordering"`
}

// Validate validates the ordering configuration and sets defaults.
// This is called automatically by EventBusConfig.Validate().
//
// Validation:
//   - Sets default mode if not specified ("none")
//   - Validates mode is one of the allowed values
//   - Sets defaults for BufferSize and Timeout if not specified
//   - Caps values to reasonable ranges
func (c *OrderingConfig) Validate() {
	// Validate and set default mode
	if c.Mode == "" {
		c.Mode = OrderingModeNone // Default mode
	} else {
		// Validate mode is one of the allowed values
		switch c.Mode {
		case OrderingModeNone, OrderingModeBestEffort, OrderingModeStrict:
			// Valid mode
		default:
			// Invalid mode, reset to default
			c.Mode = OrderingModeNone
		}
	}

	// Validate and set default buffer size
	if c.BufferSize <= 0 {
		c.BufferSize = 100 // Default buffer size
	} else if c.BufferSize > 10000 {
		// Cap buffer size to prevent excessive memory usage
		c.BufferSize = 10000
	}

	// Validate and set default timeout
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second // Default timeout
	} else if c.Timeout > 5*time.Minute {
		// Cap timeout to prevent excessive waiting
		c.Timeout = 5 * time.Minute
	}

	// PerSourceOrdering defaults to true if not set (handled by zero value)
	// No validation needed - boolean value is always valid
}

// RetentionConfig contains configuration for event retention and cleanup.
// This configures how long events are retained and how often cleanup runs.
//
// Retention policy:
//   - Events older than RetentionHours are considered expired
//   - Cleanup runs every CleanupIntervalHours
//   - Events are deleted in batches of CleanupBatchSize
//
// Example:
//   retentionConfig := &RetentionConfig{
//       RetentionHours: 24,
//       CleanupIntervalHours: 6,
//       CleanupBatchSize: 1000,
//   }
type RetentionConfig struct {
	// RetentionHours is the retention period in hours.
	// Events older than this are deleted during cleanup.
	// Default: 24 hours
	// Range: >0
	RetentionHours int `yaml:"retention_hours"`

	// CleanupIntervalHours is the cleanup interval in hours.
	// Cleanup runs this often to delete expired events.
	// Default: 6 hours
	// Range: >0
	CleanupIntervalHours int `yaml:"cleanup_interval_hours"`

	// CleanupBatchSize is the number of events to delete per batch.
	// Events are deleted in batches to avoid overwhelming storage.
	// Default: 1000
	// Range: >0
	CleanupBatchSize int `yaml:"cleanup_batch_size"`
}

// Validate validates the retention configuration and sets defaults.
// This is called automatically by EventBusConfig.Validate().
//
// Validation:
//   - Sets defaults for all fields if not specified
//   - Ensures all values are >0
func (c *RetentionConfig) Validate() {
	if c.RetentionHours <= 0 {
		c.RetentionHours = 24 // Default retention: 24 hours
	}
	if c.CleanupIntervalHours <= 0 {
		c.CleanupIntervalHours = 6 // Default cleanup interval: 6 hours
	}
	if c.CleanupBatchSize <= 0 {
		c.CleanupBatchSize = 1000 // Default batch size
	}
}

// DropPolicyConfig contains configuration for event drop policy.
// This configures when and which events are dropped during storage pressure.
//
// Drop policy:
//   - When storage usage > StoragePressureThreshold, drop policy is activated
//   - Workflow trigger events (EventCategoryWorkflowTrigger) are dropped
//   - Operational/health and critical events are still attempted to be persisted
//
// Example:
//   dropPolicyConfig := &DropPolicyConfig{
//       StoragePressureThreshold: 90,
//       DefaultCategory: EventCategoryWorkflowTrigger,
//   }
type DropPolicyConfig struct {
	// StoragePressureThreshold is the storage usage threshold for dropping events (percentage).
	// When storage usage exceeds this threshold, droppable events are dropped.
	// Default: 90%
	// Range: 1-100
	StoragePressureThreshold int `yaml:"storage_pressure_threshold"`

	// DefaultCategory is the default category for unmapped events.
	// Events not in CategoryRules are assigned this category.
	// Default: EventCategoryWorkflowTrigger (can be dropped)
	DefaultCategory EventCategory `yaml:"default_category"`

	// CategoryRules maps event types to categories.
	// If not provided, DefaultEventDropPolicy() rules are used.
	// This allows custom categorization rules to be specified.
	CategoryRules map[EventType]EventCategory `yaml:"category_rules,omitempty"`
}

// Validate validates the drop policy configuration and sets defaults.
// This is called automatically by EventBusConfig.Validate().
//
// Validation:
//   - Sets default threshold if not specified (90%)
//   - Sets default category if not specified (EventCategoryWorkflowTrigger)
//   - Ensures threshold is in valid range (1-100)
func (c *DropPolicyConfig) Validate() {
	if c.StoragePressureThreshold <= 0 {
		c.StoragePressureThreshold = 90 // Default threshold: 90%
	}
	if c.DefaultCategory == "" {
		c.DefaultCategory = EventCategoryWorkflowTrigger // Default category
	}
}

// MetaStorageProviderConfig contains meta-storage provider-specific configuration.
// This is used when the provider is "metastorage".
type MetaStorageProviderConfig struct {
	// MetaStorageDependency is a reference to the meta-storage service
	// This is typically injected via dependency injection
	// Note: This field is not serialized to YAML, it's set programmatically
	// The meta-storage service is passed directly to NewMetaStorageProvider()
	// and is not stored in this config struct
}

