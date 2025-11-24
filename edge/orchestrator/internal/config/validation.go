package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Validate validates the configuration with detailed error messages
func (c *Config) Validate() error {
	var errors []string

	// Validate orchestrator settings
	if c.Edge.Orchestrator.DataDir == "" {
		errors = append(errors, "edge.orchestrator.data_dir is required")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLogLevels[strings.ToLower(c.Edge.Orchestrator.LogLevel)] {
		errors = append(errors, fmt.Sprintf("invalid log_level: %s (must be: debug, info, warn, error, fatal)", c.Edge.Orchestrator.LogLevel))
	}

	// Validate log format
	if c.Edge.Orchestrator.LogFormat != "text" && c.Edge.Orchestrator.LogFormat != "json" {
		errors = append(errors, fmt.Sprintf("invalid log_format: %s (must be: text or json)", c.Edge.Orchestrator.LogFormat))
	}

	// Validate storage settings
	if c.Edge.Storage.MaxDiskUsagePercent < 0 || c.Edge.Storage.MaxDiskUsagePercent > 100 {
		errors = append(errors, fmt.Sprintf("max_disk_usage_percent must be between 0 and 100, got: %.2f", c.Edge.Storage.MaxDiskUsagePercent))
	}

	if c.Edge.Storage.RetentionDays < 0 {
		errors = append(errors, fmt.Sprintf("retention_days must be >= 0, got: %d", c.Edge.Storage.RetentionDays))
	}

	// Validate AI settings
	if c.Edge.AI.ConfidenceThreshold < 0 || c.Edge.AI.ConfidenceThreshold > 1 {
		errors = append(errors, fmt.Sprintf("confidence_threshold must be between 0 and 1, got: %.2f", c.Edge.AI.ConfidenceThreshold))
	}

	if c.Edge.AI.ServiceURL == "" {
		errors = append(errors, "ai.service_url is required")
	}

	if c.Edge.AI.AnomalyThreshold < 0 {
		errors = append(errors, fmt.Sprintf("ai.anomaly_threshold must be >= 0, got: %.2f", c.Edge.AI.AnomalyThreshold))
	}

	if c.Edge.AI.ClipDuration < 0 {
		errors = append(errors, fmt.Sprintf("ai.clip_duration must be >= 0, got: %v", c.Edge.AI.ClipDuration))
	}

	if c.Edge.AI.PreEventDuration < 0 {
		errors = append(errors, fmt.Sprintf("ai.pre_event_duration must be >= 0, got: %v", c.Edge.AI.PreEventDuration))
	}

	// Validate events settings
	if c.Edge.Events.QueueSize <= 0 {
		errors = append(errors, fmt.Sprintf("events.queue_size must be > 0, got: %d", c.Edge.Events.QueueSize))
	}

	if c.Edge.Events.BatchSize <= 0 {
		errors = append(errors, fmt.Sprintf("events.batch_size must be > 0, got: %d", c.Edge.Events.BatchSize))
	}

	if c.Edge.Events.BatchSize > c.Edge.Events.QueueSize {
		errors = append(errors, fmt.Sprintf("events.batch_size (%d) cannot be greater than queue_size (%d)", c.Edge.Events.BatchSize, c.Edge.Events.QueueSize))
	}

	// Validate storage directories
	if c.Edge.Storage.ClipsDir != "" {
		if !filepath.IsAbs(c.Edge.Storage.ClipsDir) && !strings.HasPrefix(c.Edge.Storage.ClipsDir, "./") {
			// Relative path - make it relative to data_dir
			c.Edge.Storage.ClipsDir = filepath.Join(c.Edge.Orchestrator.DataDir, c.Edge.Storage.ClipsDir)
		}
	}

	if c.Edge.Storage.SnapshotsDir != "" {
		if !filepath.IsAbs(c.Edge.Storage.SnapshotsDir) && !strings.HasPrefix(c.Edge.Storage.SnapshotsDir, "./") {
			// Relative path - make it relative to data_dir
			c.Edge.Storage.SnapshotsDir = filepath.Join(c.Edge.Orchestrator.DataDir, c.Edge.Storage.SnapshotsDir)
		}
	}

	// Validate WireGuard settings
	if c.Edge.WireGuard.Enabled {
		if c.Edge.WireGuard.ConfigPath == "" && c.Edge.WireGuard.KVMEndpoint == "" {
			errors = append(errors, "wireguard.config_path or wireguard.kvm_endpoint is required when wireguard is enabled")
		}
	}

	// Validate telemetry settings
	if c.Edge.Telemetry.Interval <= 0 {
		errors = append(errors, fmt.Sprintf("telemetry.interval must be > 0, got: %v", c.Edge.Telemetry.Interval))
	}

	// Validate camera settings
	if c.Edge.Cameras.Discovery.Interval <= 0 {
		errors = append(errors, fmt.Sprintf("cameras.discovery.interval must be > 0, got: %v", c.Edge.Cameras.Discovery.Interval))
	}

	if c.Edge.Cameras.RTSP.Timeout <= 0 {
		errors = append(errors, fmt.Sprintf("cameras.rtsp.timeout must be > 0, got: %v", c.Edge.Cameras.RTSP.Timeout))
	}

	if c.Edge.Cameras.RTSP.ReconnectInterval <= 0 {
		errors = append(errors, fmt.Sprintf("cameras.rtsp.reconnect_interval must be > 0, got: %v", c.Edge.Cameras.RTSP.ReconnectInterval))
	}

	// Validate AI inference interval
	if c.Edge.AI.InferenceInterval <= 0 {
		errors = append(errors, fmt.Sprintf("ai.inference_interval must be > 0, got: %v", c.Edge.AI.InferenceInterval))
	}

	// Validate events transmission interval
	if c.Edge.Events.TransmissionInterval <= 0 {
		errors = append(errors, fmt.Sprintf("events.transmission_interval must be > 0, got: %v", c.Edge.Events.TransmissionInterval))
	}

	// Normalize dataset export dir if relative
	if c.Edge.AI.DatasetExportDir != "" {
		if !filepath.IsAbs(c.Edge.AI.DatasetExportDir) && !strings.HasPrefix(c.Edge.AI.DatasetExportDir, "./") {
			c.Edge.AI.DatasetExportDir = filepath.Join(c.Edge.Orchestrator.DataDir, c.Edge.AI.DatasetExportDir)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}
