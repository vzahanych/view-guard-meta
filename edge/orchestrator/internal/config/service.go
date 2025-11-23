package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// Service provides configuration management with environment variable support
type Service struct {
	config     *Config
	configPath string
	logger     *logger.Logger
	mu         sync.RWMutex
	watchers   []ConfigWatcher
}

// ConfigWatcher is called when configuration changes
type ConfigWatcher func(ctx context.Context, oldConfig, newConfig *Config) error

// NewService creates a new configuration service
func NewService(configPath string, log *logger.Logger) (*Service, error) {
	cfg, err := Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load initial configuration: %w", err)
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &Service{
		config:     cfg,
		configPath: configPath,
		logger:     log,
		watchers:   make([]ConfigWatcher, 0),
	}, nil
}

// Get returns the current configuration (thread-safe)
func (s *Service) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Reload reloads the configuration from file
func (s *Service) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldConfig := s.config

	// Load new configuration
	newConfig, err := Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to reload configuration: %w", err)
	}

	// Apply environment variable overrides
	applyEnvOverrides(newConfig)

	// Validate new configuration
	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("invalid reloaded configuration: %w", err)
	}

	// Update configuration
	s.config = newConfig

	// Notify watchers
	for _, watcher := range s.watchers {
		if err := watcher(ctx, oldConfig, newConfig); err != nil {
			s.logger.Error("Config watcher error", "error", err)
		}
	}

	s.logger.Info("Configuration reloaded", "path", s.configPath)
	return nil
}

// Watch registers a configuration change watcher
func (s *Service) Watch(watcher ConfigWatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchers = append(s.watchers, watcher)
}

// applyEnvOverrides applies environment variable overrides to configuration
func applyEnvOverrides(cfg *Config) {
	// Edge Orchestrator settings
	if val := os.Getenv("EDGE_LOG_LEVEL"); val != "" {
		cfg.Edge.Orchestrator.LogLevel = val
	}
	if val := os.Getenv("EDGE_LOG_FORMAT"); val != "" {
		cfg.Edge.Orchestrator.LogFormat = val
	}
	if val := os.Getenv("EDGE_DATA_DIR"); val != "" {
		cfg.Edge.Orchestrator.DataDir = val
	}

	// WireGuard settings
	if val := os.Getenv("EDGE_WIREGUARD_ENABLED"); val != "" {
		cfg.Edge.WireGuard.Enabled = (val == "true" || val == "1")
	}
	if val := os.Getenv("EDGE_WIREGUARD_CONFIG_PATH"); val != "" {
		cfg.Edge.WireGuard.ConfigPath = val
	}
	if val := os.Getenv("EDGE_WIREGUARD_KVM_ENDPOINT"); val != "" {
		cfg.Edge.WireGuard.KVMEndpoint = val
	}

	// AI Service settings
	if val := os.Getenv("EDGE_AI_SERVICE_URL"); val != "" {
		cfg.Edge.AI.ServiceURL = val
	}
	if val := os.Getenv("EDGE_AI_CONFIDENCE_THRESHOLD"); val != "" {
		if threshold, err := parseFloat64(val); err == nil {
			cfg.Edge.AI.ConfidenceThreshold = threshold
		}
	}
	if val := os.Getenv("EDGE_AI_INFERENCE_INTERVAL"); val != "" {
		if interval, err := time.ParseDuration(val); err == nil {
			cfg.Edge.AI.InferenceInterval = interval
		}
	}
	if val := os.Getenv("EDGE_AI_ENABLED_CLASSES"); val != "" {
		// Parse comma-separated class names
		classes := strings.Split(val, ",")
		for i := range classes {
			classes[i] = strings.TrimSpace(classes[i])
		}
		cfg.Edge.AI.EnabledClasses = classes
	}

	// Storage settings
	if val := os.Getenv("EDGE_STORAGE_CLIPS_DIR"); val != "" {
		cfg.Edge.Storage.ClipsDir = val
	}
	if val := os.Getenv("EDGE_STORAGE_SNAPSHOTS_DIR"); val != "" {
		cfg.Edge.Storage.SnapshotsDir = val
	}
	if val := os.Getenv("EDGE_STORAGE_RETENTION_DAYS"); val != "" {
		if days, err := parseInt(val); err == nil {
			cfg.Edge.Storage.RetentionDays = days
		}
	}
	if val := os.Getenv("EDGE_STORAGE_MAX_DISK_USAGE_PERCENT"); val != "" {
		if percent, err := parseFloat64(val); err == nil {
			cfg.Edge.Storage.MaxDiskUsagePercent = percent
		}
	}

	// Events settings
	if val := os.Getenv("EDGE_EVENTS_QUEUE_SIZE"); val != "" {
		if size, err := parseInt(val); err == nil {
			cfg.Edge.Events.QueueSize = size
		}
	}
	if val := os.Getenv("EDGE_EVENTS_BATCH_SIZE"); val != "" {
		if size, err := parseInt(val); err == nil {
			cfg.Edge.Events.BatchSize = size
		}
	}

	// Telemetry settings
	if val := os.Getenv("EDGE_TELEMETRY_ENABLED"); val != "" {
		cfg.Edge.Telemetry.Enabled = (val == "true" || val == "1")
	}
	if val := os.Getenv("EDGE_TELEMETRY_INTERVAL"); val != "" {
		if interval, err := time.ParseDuration(val); err == nil {
			cfg.Edge.Telemetry.Interval = interval
		}
	}

	// Log settings
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		cfg.Log.Level = val
	}
	if val := os.Getenv("LOG_FORMAT"); val != "" {
		cfg.Log.Format = val
	}
	if val := os.Getenv("LOG_OUTPUT"); val != "" {
		cfg.Log.Output = val
	}
}

// Helper functions for parsing environment variables
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func parseFloat64(s string) (float64, error) {
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}

// GetEnvWithDefault gets an environment variable with a default value
func GetEnvWithDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// GetEnvBool gets a boolean environment variable
func GetEnvBool(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	val = strings.ToLower(val)
	return val == "true" || val == "1" || val == "yes" || val == "on"
}

// GetEnvInt gets an integer environment variable
func GetEnvInt(key string, defaultValue int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(val, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// GetEnvDuration gets a duration environment variable
func GetEnvDuration(key string, defaultValue time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	if duration, err := time.ParseDuration(val); err == nil {
		return duration
	}
	return defaultValue
}

// GetEnvFloat64 gets a float64 environment variable
func GetEnvFloat64(key string, defaultValue float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	var result float64
	if _, err := fmt.Sscanf(val, "%f", &result); err != nil {
		return defaultValue
	}
	return result
}

