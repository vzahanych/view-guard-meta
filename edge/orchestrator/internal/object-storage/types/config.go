package types

// ObjectStorageConfig contains configuration for the object-storage service.
// This is a provider-agnostic configuration structure that supports multiple storage backends.
type ObjectStorageConfig struct {
	// Provider specifies the storage provider to use (e.g., "minio", "s3", "filesystem").
	Provider string `yaml:"provider"`

	// Endpoint is the storage endpoint URL (used for remote storage providers like MinIO/S3).
	// For local providers like filesystem, this is empty.
	Endpoint string `yaml:"endpoint"`

	// AccessKey is the access key for authentication (used for remote storage providers).
	// For local providers like filesystem, this is empty.
	AccessKey string `yaml:"access_key"`

	// SecretKey is the secret key for authentication (used for remote storage providers).
	// For local providers like filesystem, this is empty.
	SecretKey string `yaml:"secret_key"`

	// Region is the storage region (used for remote storage providers).
	// For local providers like filesystem, this is empty.
	Region string `yaml:"region"`

	// QuotaConfig contains quota management configuration.
	QuotaConfig *QuotaConfig `yaml:"quota"`

	// RetentionConfig contains retention policy configuration.
	RetentionConfig *RetentionConfig `yaml:"retention"`

	// EncryptionConfig contains encryption configuration (optional).
	EncryptionConfig *EncryptionConfig `yaml:"encryption"`

	// Provider-specific configurations
	MinIOConfig *MinIOConfig `yaml:"minio"`
	// S3Config *S3Config `yaml:"s3"` // Future
	// FilesystemConfig *FilesystemConfig `yaml:"filesystem"` // Future
}

// QuotaConfig contains configuration for storage quota management.
type QuotaConfig struct {
	// MaxSizeMB is the maximum storage size in megabytes (default: 100,000 MB)
	MaxSizeMB int `yaml:"max_size_mb"`

	// WarningThresholdPercent is the warning threshold percentage (default: 80)
	// When storage usage exceeds this threshold, warnings are emitted
	WarningThresholdPercent int `yaml:"warning_threshold_percent"`

	// FullThresholdPercent is the full threshold percentage (default: 95)
	// When storage usage exceeds this threshold, write operations are rejected
	FullThresholdPercent int `yaml:"full_threshold_percent"`
}

// Validate validates the quota configuration and sets defaults.
func (c *QuotaConfig) Validate() {
	if c.MaxSizeMB <= 0 {
		c.MaxSizeMB = 100000 // Default: 100,000 MB
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
}

// RetentionConfig contains configuration for retention policies.
type RetentionConfig struct {
	// DatasetRetentionDays is the retention period for dataset objects in days (default: 30)
	// Objects are retained for this period after upload completion
	DatasetRetentionDays int `yaml:"dataset_retention_days"`

	// EventRetentionDays is the retention period for event attachments in days (default: 7)
	// Objects are retained for this period after VM acknowledgment
	EventRetentionDays int `yaml:"event_retention_days"`

	// ModelRetentionVersions is the number of model versions to retain per device (default: 2)
	ModelRetentionVersions int `yaml:"model_retention_versions"`

	// ModelRetentionGracePeriodDays is the grace period in days after purge eligibility (default: 7)
	ModelRetentionGracePeriodDays int `yaml:"model_retention_grace_period_days"`

	// UnassignedDeviceDataRetentionDays is the retention period for unassigned device data in days (default: 30)
	// Objects are retained for this period after device unassignment
	UnassignedDeviceDataRetentionDays int `yaml:"unassigned_device_data_retention_days"`

	// CleanupIntervalHours is how often to run cleanup in hours (default: 6)
	CleanupIntervalHours int `yaml:"cleanup_interval_hours"`
}

// Validate validates the retention configuration and sets defaults.
func (c *RetentionConfig) Validate() {
	if c.DatasetRetentionDays <= 0 {
		c.DatasetRetentionDays = 30 // Default: 30 days
	}
	if c.EventRetentionDays <= 0 {
		c.EventRetentionDays = 7 // Default: 7 days
	}
	if c.ModelRetentionVersions <= 0 {
		c.ModelRetentionVersions = 2 // Default: 2 versions
	}
	if c.ModelRetentionGracePeriodDays <= 0 {
		c.ModelRetentionGracePeriodDays = 7 // Default: 7 days
	}
	if c.UnassignedDeviceDataRetentionDays <= 0 {
		c.UnassignedDeviceDataRetentionDays = 30 // Default: 30 days
	}
	if c.CleanupIntervalHours <= 0 {
		c.CleanupIntervalHours = 6 // Default: 6 hours
	}
}

// EncryptionConfig contains encryption configuration.
// Note: Encryption is implementation-dependent (provider must support it).
type EncryptionConfig struct {
	// Enabled indicates whether encryption is enabled (default: false)
	Enabled bool `yaml:"enabled"`

	// Provider specifies the encryption provider (kms, hardware-backed, software)
	Provider string `yaml:"provider"`

	// KeySource specifies the key source (KMS endpoint, hardware module path, key file path)
	KeySource string `yaml:"key_source"`

	// Algorithm specifies the encryption algorithm (default: AES-256-GCM)
	Algorithm string `yaml:"algorithm"`
}

// Validate validates the encryption configuration and sets defaults.
func (c *EncryptionConfig) Validate() {
	if !c.Enabled {
		return // No validation needed if encryption is disabled
	}

	// Set default algorithm if not specified
	if c.Algorithm == "" {
		c.Algorithm = "AES-256-GCM"
	}

	// Set default provider if not specified
	if c.Provider == "" {
		c.Provider = "software"
	}

	// Validate provider
	validProviders := map[string]bool{
		"kms":            true,
		"hardware-backed": true,
		"software":        true,
	}
	if !validProviders[c.Provider] {
		c.Provider = "software" // Default to software if invalid
	}

	// Validate algorithm
	validAlgorithms := map[string]bool{
		"AES-256-GCM": true,
		"AES-128-GCM": true,
		"ChaCha20-Poly1305": true,
	}
	if !validAlgorithms[c.Algorithm] {
		c.Algorithm = "AES-256-GCM" // Default to AES-256-GCM if invalid
	}
}

// MinIOConfig contains MinIO-specific configuration.
type MinIOConfig struct {
	// Endpoint is the MinIO server endpoint
	Endpoint string `yaml:"endpoint"`

	// AccessKey is the MinIO access key
	AccessKey string `yaml:"access_key"`

	// SecretKey is the MinIO secret key
	SecretKey string `yaml:"secret_key"`

	// Region is the MinIO region (default: "us-east-1")
	Region string `yaml:"region"`

	// Bucket is the MinIO bucket name (default: "edge-storage")
	Bucket string `yaml:"bucket"`

	// UseSSL indicates whether to use SSL/TLS
	UseSSL bool `yaml:"use_ssl"`

	// InsecureSkipVerify indicates whether to skip TLS certificate verification (for dev)
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

