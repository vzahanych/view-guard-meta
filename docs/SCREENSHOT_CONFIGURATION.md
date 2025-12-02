# Screenshot Configuration Guide

This guide explains how to configure screenshot management settings in the View Guard Edge Orchestrator.

## Related Documentation

- [Screenshot User Guide](./SCREENSHOT_USER_GUIDE.md) - How to use screenshot management features
- [Screenshot API Documentation](./SCREENSHOT_API.md) - API endpoint reference
- [Screenshot Testing Guide](./TESTING_SCREENSHOTS.md) - Testing procedures

## Configuration Options

### Minimum Normal Snapshots (`min_normal_snapshots`)

**Description**: The minimum number of "normal" labeled snapshots required per camera before the dataset is considered ready for training.

**Default Value**: `50`

**Configuration File**:
```yaml
edge:
  ai:
    min_normal_snapshots: 50
```

**Environment Variable**:
```bash
export EDGE_AI_MIN_NORMAL_SNAPSHOTS=50
```

**Validation**: Must be greater than 0. If set to 0 or not specified, defaults to 50.

**Usage**: This value is used to calculate dataset progress. When a camera has collected at least this many "normal" labeled snapshots, the `snapshot_required` flag is set to `false` and the dataset is considered ready for training.

**Example**: If you want to require 100 normal snapshots per camera:
```yaml
edge:
  ai:
    min_normal_snapshots: 100
```

### Screenshot Storage Configuration

#### Screenshot Retention Days (`screenshot_retention_days`)

**Description**: Number of days to retain labeled screenshots before automatic deletion.

**Default Value**: `0` (no automatic deletion)

**Configuration File**:
```yaml
edge:
  storage:
    screenshot_retention_days: 0  # 0 = no automatic deletion
```

**Environment Variable**:
```bash
export EDGE_STORAGE_SCREENSHOT_RETENTION_DAYS=30
```

**Validation**: Must be >= 0. A value of 0 means screenshots are never automatically deleted.

**Example**: To automatically delete screenshots older than 30 days:
```yaml
edge:
  storage:
    screenshot_retention_days: 30
```

#### Maximum Screenshot Size (`screenshot_max_size_mb`)

**Description**: Maximum size allowed per screenshot in megabytes.

**Default Value**: `0` (no limit)

**Configuration File**:
```yaml
edge:
  storage:
    screenshot_max_size_mb: 0  # 0 = no limit
```

**Environment Variable**:
```bash
export EDGE_STORAGE_SCREENSHOT_MAX_SIZE_MB=10
```

**Validation**: Must be >= 0. A value of 0 means no size limit per screenshot.

**Example**: To limit each screenshot to 10 MB:
```yaml
edge:
  storage:
    screenshot_max_size_mb: 10
```

#### Maximum Total Screenshot Storage (`screenshot_max_total_size_gb`)

**Description**: Maximum total size for all screenshots combined in gigabytes.

**Default Value**: `0` (no limit)

**Configuration File**:
```yaml
edge:
  storage:
    screenshot_max_total_size_gb: 0  # 0 = no limit
```

**Environment Variable**:
```bash
export EDGE_STORAGE_SCREENSHOT_MAX_TOTAL_SIZE_GB=100
```

**Validation**: Must be >= 0. A value of 0 means no total size limit.

**Example**: To limit total screenshot storage to 100 GB:
```yaml
edge:
  storage:
    screenshot_max_total_size_gb: 100
```

## Complete Configuration Example

```yaml
edge:
  ai:
    min_normal_snapshots: 50  # Require 50 normal snapshots per camera
    
  storage:
    # Screenshot storage limits
    screenshot_retention_days: 0        # No automatic deletion
    screenshot_max_size_mb: 0           # No per-screenshot size limit
    screenshot_max_total_size_gb: 0    # No total size limit
    
    # General storage settings (also apply to screenshots)
    max_disk_usage_percent: 80          # Warn when disk usage exceeds 80%
```

## Per-Camera Configuration

Currently, the minimum snapshot requirement is global for all cameras. If you need different requirements per camera, you can:

1. **Use the default for most cameras**: Set `min_normal_snapshots` to the most common requirement
2. **Adjust per camera manually**: Monitor individual camera progress in the UI and adjust collection targets manually

**Future Enhancement**: Per-camera minimum snapshot requirements could be added as a camera configuration option if needed.

## Environment Variable Priority

Environment variables override configuration file values. The priority order is:

1. Environment variables (highest priority)
2. Configuration file values
3. Default values (lowest priority)

## Validation

All configuration values are validated on startup:

- `min_normal_snapshots`: Must be > 0 (defaults to 50 if 0 or not set)
- `screenshot_retention_days`: Must be >= 0
- `screenshot_max_size_mb`: Must be >= 0
- `screenshot_max_total_size_gb`: Must be >= 0

If validation fails, the application will not start and will display detailed error messages.

## Configuration Reload

Configuration can be reloaded at runtime using the configuration service API:

```bash
# Reload configuration (if hot reload is enabled)
curl -X PUT http://localhost:8080/api/config
```

Note: Some configuration changes may require a service restart to take full effect.

## Troubleshooting

### Configuration Not Applied

1. **Check configuration file path**: Ensure the config file is in the expected location
2. **Check environment variables**: Environment variables override config file values
3. **Check logs**: Configuration loading errors are logged at startup
4. **Validate configuration**: Use the validation endpoint or check startup logs

### Invalid Configuration Values

If you see validation errors:

1. Check the error message for the specific field and value
2. Ensure numeric values are within valid ranges
3. Ensure required fields are set
4. Check that environment variables are valid (integers for counts, floats for percentages)

### Default Values Not Applied

If defaults aren't being applied:

1. Ensure the configuration file is being loaded correctly
2. Check that `setDefaults()` is being called (it's called automatically in `config.Load()`)
3. Verify that environment variables aren't overriding with invalid values

## Related Documentation

- [Configuration Service README](../../edge/orchestrator/internal/config/README.md)
- [Screenshot Management Testing Guide](../TESTING_SCREENSHOTS.md)
- [Implementation Plan Phase 2](../IMPLEMENTATION_PLAN_PHASE2.md)
