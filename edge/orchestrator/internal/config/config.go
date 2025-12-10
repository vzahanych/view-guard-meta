package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Edge EdgeConfig `yaml:"edge"`
	Log  LogConfig  `yaml:"log,omitempty"`
}

// EdgeConfig contains Edge Appliance specific configuration
type EdgeConfig struct {
	Orchestrator OrchestratorConfig `yaml:"orchestrator"`
	WireGuard    WireGuardConfig    `yaml:"wireguard"`
	Cameras      CamerasConfig      `yaml:"cameras"`
	Storage      StorageConfig      `yaml:"storage"`
	AI           AIConfig           `yaml:"ai"`
	Events       EventsConfig       `yaml:"events"`
	Telemetry    TelemetryConfig    `yaml:"telemetry"`
	Encryption   EncryptionConfig   `yaml:"encryption"`
	Web          WebConfig          `yaml:"web"`
	Processing   ProcessingConfig   `yaml:"processing"`
}

// OrchestratorConfig contains orchestrator service configuration
type OrchestratorConfig struct {
	LogLevel   string `yaml:"log_level"`
	LogFormat  string `yaml:"log_format"`
	DataDir    string `yaml:"data_dir"`
	ConfigFile string `yaml:"config_file"`
}

// WireGuardConfig contains WireGuard client configuration
type WireGuardConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ConfigPath  string `yaml:"config_path"`
	KVMEndpoint string `yaml:"kvm_endpoint"`
}

// CamerasConfig contains camera discovery and connection configuration
type CamerasConfig struct {
	Discovery DiscoveryConfig `yaml:"discovery"`
	RTSP      RTSPConfig      `yaml:"rtsp"`
}

// DiscoveryConfig contains camera discovery configuration
type DiscoveryConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
}

// RTSPConfig contains RTSP client configuration
type RTSPConfig struct {
	Timeout           time.Duration `yaml:"timeout"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval"`
}

// StorageConfig contains local storage configuration
type StorageConfig struct {
	ClipsDir            string  `yaml:"clips_dir"`
	SnapshotsDir        string  `yaml:"snapshots_dir"`
	RetentionDays       int     `yaml:"retention_days"`
	MaxDiskUsagePercent float64 `yaml:"max_disk_usage_percent"`
	// Screenshot storage configuration (Substep 2.2.2.7.1)
	ScreenshotRetentionDays  int `yaml:"screenshot_retention_days"`    // Retention for labeled screenshots (0 = no automatic deletion)
	ScreenshotMaxSizeMB      int `yaml:"screenshot_max_size_mb"`       // Maximum size per screenshot in MB (0 = no limit)
	ScreenshotMaxTotalSizeGB int `yaml:"screenshot_max_total_size_gb"` // Maximum total size for all screenshots in GB (0 = no limit)
}

// AIConfig contains AI service configuration
type AIConfig struct {
	ServiceURL            string        `yaml:"service_url"`
	InferenceInterval     time.Duration `yaml:"inference_interval"`
	ConfidenceThreshold   float64       `yaml:"confidence_threshold"`
	EnabledClasses        []string      `yaml:"enabled_classes"` // Optional: filter by class names
	LocalInferenceEnabled bool          `yaml:"local_inference_enabled"`
	BaselineLabel         string        `yaml:"baseline_label"`
	AnomalyThreshold      float64       `yaml:"anomaly_threshold"`
	LocalModelPath        string        `yaml:"local_model_path"`
	ClipDuration          time.Duration `yaml:"clip_duration"`
	PreEventDuration      time.Duration `yaml:"pre_event_duration"`
	DatasetExportDir      string        `yaml:"dataset_export_dir"`
	MinNormalSnapshots    int           `yaml:"min_normal_snapshots"`
}

// EventsConfig contains event management configuration
type EventsConfig struct {
	QueueSize            int           `yaml:"queue_size"`
	BatchSize            int           `yaml:"batch_size"`
	TransmissionInterval time.Duration `yaml:"transmission_interval"`
}

// TelemetryConfig contains telemetry collection configuration
type TelemetryConfig struct {
	Interval time.Duration `yaml:"interval"`
	Enabled  bool          `yaml:"enabled"`
}

// EncryptionConfig contains encryption service configuration
type EncryptionConfig struct {
	Enabled    bool   `yaml:"enabled"`
	UserSecret string `yaml:"user_secret"` // User secret for key derivation (never logged)
	Salt       string `yaml:"salt"`        // Salt for key derivation (hex encoded, optional - will be generated if not provided)
	SaltPath   string `yaml:"salt_path"`   // Path to file where salt is stored
}

// WebConfig contains web server configuration
type WebConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	// TODO: Add authentication configuration (Step 1.9.1)
	// AuthToken string `yaml:"auth_token"` // Simple token-based auth for PoC
}

// ProcessingConfig contains event processing configuration
type ProcessingConfig struct {
	// Frame processing
	InferenceInterval time.Duration `yaml:"inference_interval"` // Interval between frames sent to inference (e.g., 1s = 1 FPS)
	PreBufferDuration time.Duration `yaml:"pre_buffer_duration"` // Duration of pre-event buffer (e.g., 5s)
	
	// Inference
	ConfidenceThreshold float64  `yaml:"confidence_threshold"` // Global confidence threshold (0.0-1.0)
	PerCameraThresholds map[string]float64 `yaml:"per_camera_thresholds,omitempty"` // Per-camera thresholds (camera_id -> threshold)
	EnabledClasses      []string `yaml:"enabled_classes,omitempty"` // Optional: filter by class names
	
	// Event detection
	MinEventDuration    time.Duration `yaml:"min_event_duration"` // Minimum event duration for debouncing (e.g., 2s)
	
	// Clip recording
	PostEventDuration   time.Duration `yaml:"post_event_duration"` // Duration to record after event (e.g., 10s)
	MaxClipLength       time.Duration `yaml:"max_clip_length"` // Maximum clip length (0 = no limit)
	
	// Snapshot capture
	JPEGQuality         int `yaml:"jpeg_quality"` // JPEG quality for snapshots (1-100, default 85)
	
	// Retry configuration
	MaxRetries          int           `yaml:"max_retries"` // Maximum retry attempts for transmission (0 = unlimited)
	RetryBaseDelay      time.Duration `yaml:"retry_base_delay"` // Base delay for exponential backoff
	RetryMaxDelay       time.Duration `yaml:"retry_max_delay"` // Maximum delay for exponential backoff
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// Load reads and parses the configuration file
func Load(configPath string) (*Config, error) {
	// Default config path if not provided
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	return &cfg, nil
}

// getDefaultConfigPath returns the default configuration file path
func getDefaultConfigPath() string {
	// Try common locations
	paths := []string{
		"./config/config.dev.yaml",
		"./config/config.yaml",
		"../config/config.dev.yaml",
		"../config/config.yaml",
		"/etc/view-guard-edge/config.yaml",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Return the first default if none found (will error later)
	return paths[0]
}

// setDefaults sets default values for configuration
func (c *Config) setDefaults() {
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "text"
	}
	if c.Log.Output == "" {
		c.Log.Output = "stdout"
	}

	if c.Edge.Orchestrator.LogLevel == "" {
		c.Edge.Orchestrator.LogLevel = "info"
	}
	if c.Edge.Orchestrator.LogFormat == "" {
		c.Edge.Orchestrator.LogFormat = "text"
	}
	if c.Edge.Orchestrator.DataDir == "" {
		c.Edge.Orchestrator.DataDir = "./data"
	}

	if c.Edge.Storage.ClipsDir == "" {
		c.Edge.Storage.ClipsDir = filepath.Join(c.Edge.Orchestrator.DataDir, "clips")
	}
	if c.Edge.Storage.SnapshotsDir == "" {
		c.Edge.Storage.SnapshotsDir = filepath.Join(c.Edge.Orchestrator.DataDir, "snapshots")
	}
	if c.Edge.Storage.RetentionDays == 0 {
		c.Edge.Storage.RetentionDays = 7
	}
	if c.Edge.Storage.MaxDiskUsagePercent == 0 {
		c.Edge.Storage.MaxDiskUsagePercent = 80
	}

	// Processing configuration defaults
	if c.Edge.Processing.InferenceInterval == 0 {
		c.Edge.Processing.InferenceInterval = 1 * time.Second // Default: 1 FPS
	}
	if c.Edge.Processing.PreBufferDuration == 0 {
		c.Edge.Processing.PreBufferDuration = 5 * time.Second // Default: 5 seconds
	}
	if c.Edge.Processing.ConfidenceThreshold == 0 {
		c.Edge.Processing.ConfidenceThreshold = 0.5 // Default: 50% confidence
	}
	if c.Edge.Processing.MinEventDuration == 0 {
		c.Edge.Processing.MinEventDuration = 2 * time.Second // Default: 2 seconds
	}
	if c.Edge.Processing.PostEventDuration == 0 {
		c.Edge.Processing.PostEventDuration = 10 * time.Second // Default: 10 seconds
	}
	if c.Edge.Processing.JPEGQuality == 0 {
		c.Edge.Processing.JPEGQuality = 85 // Default: 85% quality
	}
	if c.Edge.Processing.MaxRetries == 0 {
		c.Edge.Processing.MaxRetries = 10 // Default: 10 retries
	}
	if c.Edge.Processing.RetryBaseDelay == 0 {
		c.Edge.Processing.RetryBaseDelay = 1 * time.Second // Default: 1 second
	}
	if c.Edge.Processing.RetryMaxDelay == 0 {
		c.Edge.Processing.RetryMaxDelay = 5 * time.Minute // Default: 5 minutes
	}

	if c.Edge.AI.ServiceURL == "" {
		c.Edge.AI.ServiceURL = "http://localhost:8080"
	}
	if c.Edge.AI.InferenceInterval == 0 {
		c.Edge.AI.InferenceInterval = time.Second
	}
	if c.Edge.AI.ConfidenceThreshold == 0 {
		c.Edge.AI.ConfidenceThreshold = 0.5
	}
	if c.Edge.AI.BaselineLabel == "" {
		c.Edge.AI.BaselineLabel = "normal"
	}
	if c.Edge.AI.AnomalyThreshold == 0 {
		c.Edge.AI.AnomalyThreshold = 12.0
	}
	if c.Edge.AI.ClipDuration == 0 {
		c.Edge.AI.ClipDuration = 10 * time.Second
	}
	if c.Edge.AI.PreEventDuration == 0 {
		c.Edge.AI.PreEventDuration = 2 * time.Second
	}
	if c.Edge.AI.DatasetExportDir == "" {
		c.Edge.AI.DatasetExportDir = filepath.Join(c.Edge.Orchestrator.DataDir, "exports")
	}
	// Set default for MinNormalSnapshots (Substep 2.2.2.7.1)
	// Default: 50 normal snapshots required per camera
	// Only set default if not explicitly configured (0 or negative means not set)
	if c.Edge.AI.MinNormalSnapshots <= 0 {
		c.Edge.AI.MinNormalSnapshots = 50
	}

	if c.Edge.Events.QueueSize == 0 {
		c.Edge.Events.QueueSize = 1000
	}
	if c.Edge.Events.BatchSize == 0 {
		c.Edge.Events.BatchSize = 10
	}
	if c.Edge.Events.TransmissionInterval == 0 {
		c.Edge.Events.TransmissionInterval = 5 * time.Second
	}

	if c.Edge.Telemetry.Interval == 0 {
		c.Edge.Telemetry.Interval = 60 * time.Second
	}
	if !c.Edge.Cameras.Discovery.Enabled {
		c.Edge.Cameras.Discovery.Enabled = true
	}
	if c.Edge.Cameras.Discovery.Interval == 0 {
		c.Edge.Cameras.Discovery.Interval = 300 * time.Second
	}
	if c.Edge.Cameras.RTSP.Timeout == 0 {
		c.Edge.Cameras.RTSP.Timeout = 30 * time.Second
	}
	if c.Edge.Cameras.RTSP.ReconnectInterval == 0 {
		c.Edge.Cameras.RTSP.ReconnectInterval = 10 * time.Second
	}
}
