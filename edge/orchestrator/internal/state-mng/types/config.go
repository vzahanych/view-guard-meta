package types

import (
	"fmt"
	"time"
)

// StateManagerConfig holds configuration for the state manager
type StateManagerConfig struct {
	// FrameProcessingInterval is the interval between frame captures for each camera
	// Default: 30 seconds
	// Minimum: 1 second
	// Maximum: 300 seconds (5 minutes)
	FrameProcessingInterval time.Duration `yaml:"frame_processing_interval"`

	// CapabilitySyncInterval is the interval for periodic capability sync to VM
	// Default: 5 minutes
	// Minimum: 30 seconds
	CapabilitySyncInterval time.Duration `yaml:"capability_sync_interval"`

	// MaxConcurrentWorkflows limits the number of concurrent workflow executions
	// Default: 10
	// Minimum: 1
	// Maximum: 100
	MaxConcurrentWorkflows int `yaml:"max_concurrent_workflows"`

	// FrameCaptureErrorThreshold is the number of consecutive frame capture failures
	// before transitioning camera to error state
	// Default: 5
	// Minimum: 1
	FrameCaptureErrorThreshold int `yaml:"frame_capture_error_threshold"`

	// StatePersistenceTimeout is the timeout for state persistence operations
	// Default: 5 seconds
	// Minimum: 1 second
	StatePersistenceTimeout time.Duration `yaml:"state_persistence_timeout"`

	// StatePersistenceMaxRetries is the maximum number of retry attempts for failed persistence operations
	// Default: 3
	// Minimum: 0 (no retries)
	// Maximum: 10
	StatePersistenceMaxRetries int `yaml:"state_persistence_max_retries"`

	// StatePersistenceRetryBackoff is the initial backoff duration for retry attempts
	// Default: 1 second
	// Minimum: 100 milliseconds
	StatePersistenceRetryBackoff time.Duration `yaml:"state_persistence_retry_backoff"`

	// SerializeWorkflows enables serialized workflow execution per event source
	// When enabled, workflows execute sequentially per source, respecting event ordering
	// This leverages event bus ordering guarantees (Subsection 2.1.5) to prevent ordering hazards
	// Default: true (enabled)
	SerializeWorkflows bool `yaml:"serialize_workflows"`
}

// Validate validates the state manager configuration
func (c *StateManagerConfig) Validate() error {
	// Validate frame processing interval
	if c.FrameProcessingInterval < 1*time.Second {
		return &ConfigValidationError{
			Field:   "frame_processing_interval",
			Message: "must be at least 1 second",
		}
	}
	if c.FrameProcessingInterval > 5*time.Minute {
		return &ConfigValidationError{
			Field:   "frame_processing_interval",
			Message: "must be at most 5 minutes",
		}
	}

	// Validate capability sync interval
	if c.CapabilitySyncInterval < 30*time.Second {
		return &ConfigValidationError{
			Field:   "capability_sync_interval",
			Message: "must be at least 30 seconds",
		}
	}

	// Validate max concurrent workflows
	if c.MaxConcurrentWorkflows < 1 {
		return &ConfigValidationError{
			Field:   "max_concurrent_workflows",
			Message: "must be at least 1",
		}
	}
	if c.MaxConcurrentWorkflows > 100 {
		return &ConfigValidationError{
			Field:   "max_concurrent_workflows",
			Message: "must be at most 100",
		}
	}

	// Validate frame capture error threshold
	if c.FrameCaptureErrorThreshold < 1 {
		return &ConfigValidationError{
			Field:   "frame_capture_error_threshold",
			Message: "must be at least 1",
		}
	}

	// Validate state persistence timeout
	if c.StatePersistenceTimeout < 1*time.Second {
		return &ConfigValidationError{
			Field:   "state_persistence_timeout",
			Message: "must be at least 1 second",
		}
	}

	// Validate state persistence max retries
	if c.StatePersistenceMaxRetries < 0 {
		return &ConfigValidationError{
			Field:   "state_persistence_max_retries",
			Message: "must be at least 0",
		}
	}
	if c.StatePersistenceMaxRetries > 10 {
		return &ConfigValidationError{
			Field:   "state_persistence_max_retries",
			Message: "must be at most 10",
		}
	}

	// Validate state persistence retry backoff
	if c.StatePersistenceRetryBackoff < 100*time.Millisecond {
		return &ConfigValidationError{
			Field:   "state_persistence_retry_backoff",
			Message: "must be at least 100 milliseconds",
		}
	}

	return nil
}

// ApplyDefaults applies default values to unset configuration fields
func (c *StateManagerConfig) ApplyDefaults() {
	if c.FrameProcessingInterval == 0 {
		c.FrameProcessingInterval = 30 * time.Second
	}
	if c.CapabilitySyncInterval == 0 {
		c.CapabilitySyncInterval = 5 * time.Minute
	}
	if c.MaxConcurrentWorkflows == 0 {
		c.MaxConcurrentWorkflows = 10
	}
	if c.FrameCaptureErrorThreshold == 0 {
		c.FrameCaptureErrorThreshold = 5
	}
	if c.StatePersistenceTimeout == 0 {
		c.StatePersistenceTimeout = 5 * time.Second
	}
	if c.StatePersistenceMaxRetries == 0 {
		c.StatePersistenceMaxRetries = 3
	}
	if c.StatePersistenceRetryBackoff == 0 {
		c.StatePersistenceRetryBackoff = 1 * time.Second
	}
	// SerializeWorkflows defaults to true (enabled)
	// No need to set default as bool defaults to false, but we want true
	// This is handled in SetConfig
}

// ConfigValidationError represents a configuration validation error
type ConfigValidationError struct {
	Field   string
	Message string
}

func (e *ConfigValidationError) Error() string {
	return fmt.Sprintf("invalid configuration: %s %s", e.Field, e.Message)
}
