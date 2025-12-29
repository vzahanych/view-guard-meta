package types

// QuotaConfig contains configuration for storage quota management.
type QuotaConfig struct {
	// MaxSizeMB is the maximum storage size in megabytes (default: 1,000 MB)
	MaxSizeMB int `yaml:"max_size_mb"`

	// WarningThresholdPercent is the warning threshold percentage (default: 80)
	// When storage usage exceeds this threshold, warnings are emitted
	WarningThresholdPercent int `yaml:"warning_threshold_percent"`

	// FullThresholdPercent is the full threshold percentage (default: 95)
	// When storage usage exceeds this threshold, write operations are rejected
	FullThresholdPercent int `yaml:"full_threshold_percent"`

	// MaxRecordsPerBucket is the maximum number of records per bucket (default: 1,000,000)
	// This can be overridden per bucket if needed
	MaxRecordsPerBucket int `yaml:"max_records_per_bucket"`

	// PerBucketLimits allows per-bucket record limits (optional)
	// If not specified, MaxRecordsPerBucket is used for all buckets
	PerBucketLimits map[string]int `yaml:"per_bucket_limits"`
}

// Validate validates the quota configuration and sets defaults.
func (c *QuotaConfig) Validate() {
	if c.MaxSizeMB <= 0 {
		c.MaxSizeMB = 1000 // Default: 1,000 MB
	}
	if c.WarningThresholdPercent <= 0 || c.WarningThresholdPercent >= 100 {
		c.WarningThresholdPercent = 80 // Default: 80%
	}
	if c.FullThresholdPercent <= 0 || c.FullThresholdPercent >= 100 {
		c.FullThresholdPercent = 95 // Default: 95%
	}
	if c.WarningThresholdPercent >= c.FullThresholdPercent {
		// Ensure warning threshold is less than full threshold
		c.WarningThresholdPercent = c.FullThresholdPercent - 10
		if c.WarningThresholdPercent < 50 {
			c.WarningThresholdPercent = 80
		}
	}
	if c.MaxRecordsPerBucket <= 0 {
		c.MaxRecordsPerBucket = 1000000 // Default: 1,000,000 records
	}
}

// GetBucketLimit returns the record limit for a specific bucket.
// If a per-bucket limit is configured, it returns that; otherwise returns MaxRecordsPerBucket.
func (c *QuotaConfig) GetBucketLimit(bucketName string) int {
	if c.PerBucketLimits != nil {
		if limit, ok := c.PerBucketLimits[bucketName]; ok {
			return limit
		}
	}
	return c.MaxRecordsPerBucket
}

// MetaStorageConfig contains configuration for the meta-storage service.
// This is a provider-agnostic configuration structure that supports multiple storage backends.
type MetaStorageConfig struct {
	// Provider specifies the storage provider to use (e.g., "bbolt", "sqlite", "postgres").
	Provider string `yaml:"provider"`

	// Endpoint is the storage endpoint URL (used for remote storage providers like PostgreSQL).
	// For local providers like BoltDB, this is empty.
	Endpoint string `yaml:"endpoint"`

	// AccessKey is the access key for authentication (used for remote storage providers).
	// For local providers like BoltDB, this is empty.
	AccessKey string `yaml:"access_key"`

	// SecretKey is the secret key for authentication (used for remote storage providers).
	// For local providers like BoltDB, this is empty.
	SecretKey string `yaml:"secret_key"`

	// Region is the storage region (used for remote storage providers).
	// For local providers like BoltDB, this is empty.
	Region string `yaml:"region"`

	// DataDir is the data directory for local storage providers (used by BoltDB, SQLite).
	// This is where the database file(s) will be stored.
	DataDir string `yaml:"data_dir"`

	// BoltDB contains BoltDB-specific configuration (used when provider is "bbolt").
	// If not specified, defaults are used.
	BoltDB *BoltDBConfig `yaml:"bbolt"`

	// Quota contains quota configuration for storage management
	Quota *QuotaConfig `yaml:"quota"`

	// Retention contains retention policy configuration
	Retention *RetentionConfig `yaml:"retention"`
}

// RetentionConfig contains configuration for retention policies.
type RetentionConfig struct {
	// EventBusRetentionHours is the retention period for event bus events in hours (default: 24 hours)
	EventBusRetentionHours int `yaml:"event_bus_retention_hours"`

	// DeadLetterRetentionDays is the retention period for dead letter events in days (default: 90 days)
	DeadLetterRetentionDays int `yaml:"dead_letter_retention_days"`

	// EdgeStateHistoryRetentionDays is the retention period for edge state history in days (default: 30 days)
	EdgeStateHistoryRetentionDays int `yaml:"edge_state_history_retention_days"`

	// CleanupIntervalHours is the interval between cleanup runs in hours (default: 6 hours)
	CleanupIntervalHours int `yaml:"cleanup_interval_hours"`

	// PerBucketRetention allows per-bucket retention policies (optional)
	// Format: bucket_name -> retention_hours
	// If not specified, default retention policies are used
	PerBucketRetention map[string]int `yaml:"per_bucket_retention"`
}

// Validate validates the retention configuration and sets defaults.
func (c *RetentionConfig) Validate() {
	if c.EventBusRetentionHours <= 0 {
		c.EventBusRetentionHours = 24 // Default: 24 hours
	}
	if c.DeadLetterRetentionDays <= 0 {
		c.DeadLetterRetentionDays = 90 // Default: 90 days
	}
	if c.EdgeStateHistoryRetentionDays <= 0 {
		c.EdgeStateHistoryRetentionDays = 30 // Default: 30 days
	}
	if c.CleanupIntervalHours <= 0 {
		c.CleanupIntervalHours = 6 // Default: 6 hours
	}
}

// GetBucketRetentionHours returns the retention period in hours for a specific bucket.
// If a per-bucket retention is configured, it returns that; otherwise returns the default for that bucket type.
func (c *RetentionConfig) GetBucketRetentionHours(bucketName string) int {
	// Check per-bucket retention first
	if c.PerBucketRetention != nil {
		if retention, ok := c.PerBucketRetention[bucketName]; ok {
			return retention
		}
	}

	// Return default retention based on bucket type
	switch bucketName {
	case "event_bus", "event_queue":
		return c.EventBusRetentionHours
	case "dead_letter_events":
		return c.DeadLetterRetentionDays * 24 // Convert days to hours
	case "edge_state_history":
		return c.EdgeStateHistoryRetentionDays * 24 // Convert days to hours
	default:
		// No retention for other buckets (infinite retention)
		return 0
	}
}

