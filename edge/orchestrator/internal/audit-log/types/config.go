package types

import "time"

// AuditLogConfig contains configuration for audit logging.
// This follows the provider-agnostic configuration pattern from meta-storage/object-storage.
type AuditLogConfig struct {
	// Provider specifies the storage provider to use (e.g., "metastorage", "objectstorage").
	// Default: "metastorage" (preferred provider).
	Provider string `yaml:"provider"`

	// RetentionDays is the retention period for audit logs in edge storage (before syncing to VM).
	// Default: 90 days (updated from 7 days for production requirements).
	RetentionDays int `yaml:"retention_days"`

	// SyncInterval is the interval for syncing audit logs to VM.
	// Default: 5 minutes (updated from 1 hour for production requirements).
	SyncInterval time.Duration `yaml:"sync_interval"`

	// SyncBatchSize is the number of records to sync per batch.
	// Default: 1000 records.
	SyncBatchSize int `yaml:"sync_batch_size"`

	// SyncTriggerMode specifies the sync trigger mode.
	// Values: "time_based", "count_based", "hybrid" (default: "hybrid").
	// Hybrid mode triggers sync when either 5 minutes pass OR 1000 records are queued.
	SyncTriggerMode string `yaml:"sync_trigger_mode"`

	// SyncQueueConfig contains configuration for the sync queue.
	SyncQueueConfig *SyncQueueConfig `yaml:"sync_queue_config"`

	// CleanupInterval is the interval for running cleanup of expired entries.
	// Default: 24 hours.
	CleanupInterval time.Duration `yaml:"cleanup_interval"`

	// CleanupBatchSize is the number of entries to delete per cleanup batch.
	// Default: 1000 entries.
	CleanupBatchSize int `yaml:"cleanup_batch_size"`

	// Enable audit logging.
	// Default: true.
	Enabled bool `yaml:"enabled"`
}

// Validate validates the audit log configuration and sets defaults.
func (c *AuditLogConfig) Validate() {
	if c.Provider == "" {
		c.Provider = "metastorage" // Default: meta-storage (preferred provider)
	}
	if c.RetentionDays == 0 {
		c.RetentionDays = 90 // Default: 90 days (updated from 7 days)
	}
	if c.SyncInterval == 0 {
		c.SyncInterval = 5 * time.Minute // Default: 5 minutes (updated from 1 hour)
	}
	if c.SyncBatchSize == 0 {
		c.SyncBatchSize = 1000 // Default: 1000 records
	}
	if c.SyncTriggerMode == "" {
		c.SyncTriggerMode = "hybrid" // Default: hybrid mode
	}
	if c.CleanupInterval == 0 {
		c.CleanupInterval = 24 * time.Hour // Default: 24 hours
	}
	if c.CleanupBatchSize == 0 {
		c.CleanupBatchSize = 1000 // Default: 1000 entries
	}
	if c.SyncQueueConfig == nil {
		c.SyncQueueConfig = &SyncQueueConfig{}
	}
	c.SyncQueueConfig.Validate()
}

// SyncQueueConfig contains configuration for the sync queue.
type SyncQueueConfig struct {
	// MaxQueueSize is the maximum number of entries in the sync queue.
	// Default: 100,000 records.
	MaxQueueSize int `yaml:"max_queue_size"`

	// RetryBackoff is the initial retry backoff duration for failed syncs.
	// Default: 1 second (exponential backoff).
	RetryBackoff time.Duration `yaml:"retry_backoff"`

	// MaxRetries is the maximum number of retry attempts for failed syncs.
	// Default: 10.
	MaxRetries int `yaml:"max_retries"`
}

// Validate validates the sync queue configuration and sets defaults.
func (c *SyncQueueConfig) Validate() {
	if c.MaxQueueSize == 0 {
		c.MaxQueueSize = 100000 // Default: 100,000 records
	}
	if c.RetryBackoff == 0 {
		c.RetryBackoff = 1 * time.Second // Default: 1 second
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 10 // Default: 10 retries
	}
}

