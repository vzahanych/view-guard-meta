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
	UserVMAPI UserVMAPIConfig `yaml:"user_vm_api"`
	Log       LogConfig       `yaml:"log,omitempty"`
}

// UserVMAPIConfig contains User VM API specific configuration
type UserVMAPIConfig struct {
	Orchestrator         OrchestratorConfig         `yaml:"orchestrator"`
	WireGuardServer      WireGuardServerConfig      `yaml:"wireguard_server"`
	EventCache           EventCacheConfig           `yaml:"event_cache"`
	StreamRelay          StreamRelayConfig           `yaml:"stream_relay"`
	StorageSync          StorageSyncConfig          `yaml:"storage_sync"`
	AIOrchestrator       AIOrchestratorConfig       `yaml:"ai_orchestrator"`
	EventAnalyzer        EventAnalyzerConfig        `yaml:"event_analyzer"`
	TelemetryAggregator  TelemetryAggregatorConfig  `yaml:"telemetry_aggregator"`
	ManagementServer     ManagementServerConfig     `yaml:"management_server"`
}

// OrchestratorConfig contains orchestrator service configuration
type OrchestratorConfig struct {
	LogLevel   string `yaml:"log_level"`
	LogFormat  string `yaml:"log_format"`
	DataDir    string `yaml:"data_dir"`
	ConfigFile string `yaml:"config_file"`
}

// WireGuardServerConfig contains WireGuard server configuration
type WireGuardServerConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Interface    string `yaml:"interface"`
	ListenPort   int    `yaml:"listen_port"`
	PrivateKey   string `yaml:"private_key"`   // Path to private key file (not in code)
	PublicKey    string `yaml:"public_key"`    // Path to public key file (not in code)
	ConfigPath   string `yaml:"config_path"`   // Path to WireGuard config file
}

// EventCacheConfig contains event cache configuration
type EventCacheConfig struct {
	Enabled        bool          `yaml:"enabled"`
	DatabasePath   string        `yaml:"database_path"`
	RetentionDays  int           `yaml:"retention_days"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
}

// StreamRelayConfig contains stream relay configuration
type StreamRelayConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ListenAddr  string `yaml:"listen_addr"`
	TokenSecret string `yaml:"token_secret"` // For token validation (not in code)
}

// StorageSyncConfig contains storage sync configuration (MinIO/S3 for PoC, Filecoin post-PoC)
type StorageSyncConfig struct {
	Enabled          bool                   `yaml:"enabled"`
	Provider         string                 `yaml:"provider"` // s3 (MinIO for PoC), ipfs, filecoin (post-PoC)
	ProviderConfig   map[string]interface{} `yaml:"provider_config"`
	QuotaGBPerCamera int                    `yaml:"quota_gb_per_camera"` // Quota per camera bucket
	RetentionDays    int                    `yaml:"retention_days"`
}

// AIOrchestratorConfig contains AI model orchestrator configuration
type AIOrchestratorConfig struct {
	Enabled          bool   `yaml:"enabled"`
	ModelCatalogPath string `yaml:"model_catalog_path"`
	TrainingEnabled  bool   `yaml:"training_enabled"`
	TrainingService  string `yaml:"training_service"` // URL or path to training service
}

// EventAnalyzerConfig contains event analyzer configuration
type EventAnalyzerConfig struct {
	Enabled           bool    `yaml:"enabled"`
	InferenceService  string  `yaml:"inference_service"` // URL or path to inference service
	SeverityThreshold float64 `yaml:"severity_threshold"`
}

// TelemetryAggregatorConfig contains telemetry aggregator configuration
type TelemetryAggregatorConfig struct {
	Enabled         bool          `yaml:"enabled"`
	AggregationInterval time.Duration `yaml:"aggregation_interval"`
	ForwardInterval    time.Duration `yaml:"forward_interval"`
}

// ManagementServerConfig contains Management Server connection configuration
type ManagementServerConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Endpoint   string `yaml:"endpoint"`
	MTLS       bool   `yaml:"mtls"`
	CertPath   string `yaml:"cert_path"`
	KeyPath    string `yaml:"key_path"`
	CAPath     string `yaml:"ca_path"`
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error, fatal
	Format string `yaml:"format"` // json, text
	Output string `yaml:"output"` // stdout, stderr, or file path
}

// Load loads configuration from a YAML file
func Load(configPath string) (*Config, error) {
	// If no path provided, search common locations
	if configPath == "" {
		searchPaths := []string{
			"/app/config/config.yaml", // Docker runtime default
			"config/config.yaml",
			"config/config.dev.yaml",
			"../config/config.yaml",
			"./config.yaml",
		}

		for _, path := range searchPaths {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}

		if configPath == "" {
			return nil, fmt.Errorf("no configuration file found in common locations")
		}
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	setDefaults(&cfg)

	// Validate
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for configuration
func setDefaults(cfg *Config) {
	// Log defaults
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "text"
	}
	if cfg.Log.Output == "" {
		cfg.Log.Output = "stdout"
	}

	// Orchestrator defaults
	if cfg.UserVMAPI.Orchestrator.DataDir == "" {
		cfg.UserVMAPI.Orchestrator.DataDir = "./data"
	}
	if cfg.UserVMAPI.Orchestrator.LogLevel == "" {
		cfg.UserVMAPI.Orchestrator.LogLevel = "info"
	}
	if cfg.UserVMAPI.Orchestrator.LogFormat == "" {
		cfg.UserVMAPI.Orchestrator.LogFormat = "text"
	}

	// WireGuard server defaults
	if cfg.UserVMAPI.WireGuardServer.Interface == "" {
		cfg.UserVMAPI.WireGuardServer.Interface = "wg0"
	}
	if cfg.UserVMAPI.WireGuardServer.ListenPort == 0 {
		cfg.UserVMAPI.WireGuardServer.ListenPort = 51820
	}

	// Event cache defaults
	if cfg.UserVMAPI.EventCache.DatabasePath == "" {
		cfg.UserVMAPI.EventCache.DatabasePath = filepath.Join(cfg.UserVMAPI.Orchestrator.DataDir, "events.db")
	}
	if cfg.UserVMAPI.EventCache.RetentionDays == 0 {
		cfg.UserVMAPI.EventCache.RetentionDays = 30
	}
	if cfg.UserVMAPI.EventCache.CleanupInterval == 0 {
		cfg.UserVMAPI.EventCache.CleanupInterval = 24 * time.Hour
	}

	// Stream relay defaults
	if cfg.UserVMAPI.StreamRelay.ListenAddr == "" {
		cfg.UserVMAPI.StreamRelay.ListenAddr = ":8080"
	}

	// Telemetry aggregator defaults
	if cfg.UserVMAPI.TelemetryAggregator.AggregationInterval == 0 {
		cfg.UserVMAPI.TelemetryAggregator.AggregationInterval = 1 * time.Minute
	}
	if cfg.UserVMAPI.TelemetryAggregator.ForwardInterval == 0 {
		cfg.UserVMAPI.TelemetryAggregator.ForwardInterval = 5 * time.Minute
	}

	// Storage sync defaults
	if cfg.UserVMAPI.StorageSync.QuotaGBPerCamera == 0 {
		cfg.UserVMAPI.StorageSync.QuotaGBPerCamera = 10 // 10 GB per camera bucket
	}
	if cfg.UserVMAPI.StorageSync.RetentionDays == 0 {
		cfg.UserVMAPI.StorageSync.RetentionDays = 90
	}
}

// validate validates the configuration
func validate(cfg *Config) error {
	// Validate data directory exists or can be created
	if cfg.UserVMAPI.Orchestrator.DataDir != "" {
		if err := os.MkdirAll(cfg.UserVMAPI.Orchestrator.DataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
	}

	// Validate WireGuard config if enabled
	if cfg.UserVMAPI.WireGuardServer.Enabled {
		if cfg.UserVMAPI.WireGuardServer.PrivateKey == "" && cfg.UserVMAPI.WireGuardServer.ConfigPath == "" {
			return fmt.Errorf("wireguard_server.private_key or wireguard_server.config_path must be set")
		}
	}

	// Validate Management Server config if enabled
	if cfg.UserVMAPI.ManagementServer.Enabled {
		if cfg.UserVMAPI.ManagementServer.Endpoint == "" {
			return fmt.Errorf("management_server.endpoint must be set")
		}
		if cfg.UserVMAPI.ManagementServer.MTLS {
			if cfg.UserVMAPI.ManagementServer.CertPath == "" || cfg.UserVMAPI.ManagementServer.KeyPath == "" {
				return fmt.Errorf("management_server.cert_path and management_server.key_path must be set when mtls is enabled")
			}
		}
	}

	return nil
}

