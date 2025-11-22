# Configuration Service

The configuration service provides comprehensive configuration management with file loading, environment variable support, validation, and hot reloading.

## Features

- **YAML Configuration Files**: Load configuration from YAML files
- **Environment Variable Overrides**: Override any setting via environment variables
- **Configuration Validation**: Comprehensive validation with detailed error messages
- **Hot Reloading**: Reload configuration at runtime without restart
- **Change Notifications**: Watch for configuration changes
- **Thread-Safe**: Safe for concurrent access

## Usage

### Basic Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"

// Load configuration
cfg, err := config.Load("config.yaml")
if err != nil {
    log.Fatal(err)
}
```

### Using Configuration Service

```go
// Create configuration service
cfgSvc, err := config.NewService("config.yaml", logger)
if err != nil {
    log.Fatal(err)
}

// Get current configuration (thread-safe)
currentCfg := cfgSvc.Get()

// Watch for configuration changes
cfgSvc.Watch(func(ctx context.Context, oldCfg, newCfg *config.Config) error {
    logger.Info("Configuration changed")
    // Handle configuration change
    return nil
})

// Reload configuration
if err := cfgSvc.Reload(ctx); err != nil {
    log.Error("Failed to reload configuration", err)
}
```

## Environment Variables

All configuration values can be overridden using environment variables:

### Orchestrator Settings
- `EDGE_LOG_LEVEL` - Log level (debug, info, warn, error, fatal)
- `EDGE_LOG_FORMAT` - Log format (text, json)
- `EDGE_DATA_DIR` - Data directory path

### WireGuard Settings
- `EDGE_WIREGUARD_ENABLED` - Enable WireGuard (true/false)
- `EDGE_WIREGUARD_CONFIG_PATH` - WireGuard config file path
- `EDGE_WIREGUARD_KVM_ENDPOINT` - KVM VM endpoint

### AI Service Settings
- `EDGE_AI_SERVICE_URL` - AI service URL
- `EDGE_AI_CONFIDENCE_THRESHOLD` - Confidence threshold (0.0-1.0)

### Storage Settings
- `EDGE_STORAGE_CLIPS_DIR` - Clips directory
- `EDGE_STORAGE_SNAPSHOTS_DIR` - Snapshots directory
- `EDGE_STORAGE_RETENTION_DAYS` - Retention days
- `EDGE_STORAGE_MAX_DISK_USAGE_PERCENT` - Max disk usage (0-100)

### Events Settings
- `EDGE_EVENTS_QUEUE_SIZE` - Event queue size
- `EDGE_EVENTS_BATCH_SIZE` - Event batch size

### Telemetry Settings
- `EDGE_TELEMETRY_ENABLED` - Enable telemetry (true/false)
- `EDGE_TELEMETRY_INTERVAL` - Telemetry interval (duration)

### Log Settings
- `LOG_LEVEL` - Log level
- `LOG_FORMAT` - Log format
- `LOG_OUTPUT` - Log output (stdout, file path)

## Configuration Validation

The configuration service validates all settings:

- **Required fields**: Data directory, AI service URL
- **Range validation**: Percentages (0-100), thresholds (0-1)
- **Format validation**: Log levels, log formats
- **Relationship validation**: Batch size <= queue size
- **Path normalization**: Relative paths are resolved

## Helper Functions

```go
// Get environment variable with default
val := config.GetEnvWithDefault("EDGE_DATA_DIR", "./data")

// Get boolean environment variable
enabled := config.GetEnvBool("EDGE_WIREGUARD_ENABLED", false)

// Get integer environment variable
size := config.GetEnvInt("EDGE_EVENTS_QUEUE_SIZE", 1000)

// Get duration environment variable
interval := config.GetEnvDuration("EDGE_TELEMETRY_INTERVAL", 60*time.Second)

// Get float64 environment variable
threshold := config.GetEnvFloat64("EDGE_AI_CONFIDENCE_THRESHOLD", 0.5)
```

## Configuration File Structure

See `../../config/config.dev.yaml` for a complete example configuration file.

## Error Handling

All configuration operations return detailed error messages:

```go
cfg, err := config.Load("config.yaml")
if err != nil {
    // Error includes all validation failures
    log.Fatal(err)
}
```

Example error output:
```
configuration validation failed:
  - edge.orchestrator.data_dir is required
  - max_disk_usage_percent must be between 0 and 100, got: 150.00
  - confidence_threshold must be between 0 and 1, got: 1.50
```

