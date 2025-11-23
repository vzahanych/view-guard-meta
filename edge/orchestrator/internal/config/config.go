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
	Edge      EdgeConfig      `yaml:"edge"`
	Log       LogConfig       `yaml:"log,omitempty"`
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
	Timeout          time.Duration `yaml:"timeout"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval"`
}

// StorageConfig contains local storage configuration
type StorageConfig struct {
	ClipsDir            string  `yaml:"clips_dir"`
	SnapshotsDir        string  `yaml:"snapshots_dir"`
	RetentionDays       int     `yaml:"retention_days"`
	MaxDiskUsagePercent float64 `yaml:"max_disk_usage_percent"`
}

// AIConfig contains AI service configuration
type AIConfig struct {
	ServiceURL          string        `yaml:"service_url"`
	InferenceInterval  time.Duration `yaml:"inference_interval"`
	ConfidenceThreshold float64      `yaml:"confidence_threshold"`
	EnabledClasses     []string      `yaml:"enabled_classes"` // Optional: filter by class names
}

// EventsConfig contains event management configuration
type EventsConfig struct {
	QueueSize           int           `yaml:"queue_size"`
	BatchSize           int           `yaml:"batch_size"`
	TransmissionInterval time.Duration `yaml:"transmission_interval"`
}

// TelemetryConfig contains telemetry collection configuration
type TelemetryConfig struct {
	Interval time.Duration `yaml:"interval"`
	Enabled  bool          `yaml:"enabled"`
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

	if c.Edge.AI.ServiceURL == "" {
		c.Edge.AI.ServiceURL = "http://localhost:8080"
	}
	if c.Edge.AI.InferenceInterval == 0 {
		c.Edge.AI.InferenceInterval = time.Second
	}
	if c.Edge.AI.ConfidenceThreshold == 0 {
		c.Edge.AI.ConfidenceThreshold = 0.5
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


