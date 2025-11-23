# Local Storage Management

The storage package provides local storage management for video clips and snapshots, including file organization, retention policies, disk space monitoring, and snapshot generation.

## Features

- **File Organization**: Date-based directory structure (YYYY-MM-DD/cameraID_timestamp.ext)
- **Disk Space Monitoring**: Real-time disk usage tracking with caching
- **Retention Policies**: Automatic deletion of old files based on retention period and disk usage
- **Snapshot Generation**: JPEG snapshot and thumbnail generation from video frames
- **Storage State Tracking**: Integration with state manager for tracking stored files

## Components

### StorageService

Main service for managing clip and snapshot storage.

```go
config := StorageConfig{
    ClipsDir:            "/data/clips",
    SnapshotsDir:        "/data/snapshots",
    RetentionDays:       7,
    MaxDiskUsagePercent: 80.0,
    StateManager:        stateManager,
}

storage, err := NewStorageService(config, logger)
```

**Key Methods:**
- `GenerateClipPath(cameraID)` - Generate organized clip path
- `GenerateSnapshotPath(cameraID, isThumbnail)` - Generate snapshot path
- `SaveClip(ctx, path, cameraID, eventID, size)` - Save clip entry
- `SaveSnapshot(ctx, path, cameraID, eventID, size)` - Save snapshot entry
- `GetDiskUsage(ctx)` - Get current disk usage
- `CheckDiskSpace(ctx)` - Check if disk has space
- `EnforceRetention(ctx)` - Enforce retention policy
- `DeleteClip(ctx, path)` - Delete clip file and entry
- `DeleteSnapshot(ctx, path)` - Delete snapshot file and entry

### DiskMonitor

Monitors disk space usage with caching.

```go
monitor, err := NewDiskMonitor("/data/clips", 80.0, logger)
usage, err := monitor.GetUsage(ctx)
```

**Features:**
- Caches disk usage for 30 seconds
- Provides total, used, and available bytes
- Calculates usage percentage

### RetentionPolicy

Enforces storage retention policies.

```go
policy, err := NewRetentionPolicy(7, 80.0, stateManager, logger)
err := policy.Enforce(ctx)
```

**Features:**
- Deletes files older than retention period
- Deletes oldest files when disk usage exceeds threshold
- Can pause recording when disk is full

### SnapshotGenerator

Generates JPEG snapshots and thumbnails from video frames.

```go
generator := NewSnapshotGenerator(storage, SnapshotConfig{
    Quality:       85,
    ThumbnailSize: 320,
}, logger)

snapshotPath, err := generator.GenerateSnapshot(ctx, frameData, cameraID, eventID)
thumbnailPath, err := generator.GenerateThumbnail(ctx, frameData, cameraID, eventID)
```

**Features:**
- JPEG encoding with configurable quality
- Thumbnail generation with automatic resizing
- Aspect ratio preservation
- Automatic storage entry creation

## File Organization

### Clips
```
/data/clips/
  YYYY-MM-DD/
    camera-1_HHMMSS.mp4
    camera-2_HHMMSS.mp4
```

### Snapshots
```
/data/snapshots/
  YYYY-MM-DD/
    camera-1_HHMMSS.jpg
    camera-1_HHMMSS_thumb.jpg
```

## Retention Policy

The retention policy enforces two rules:

1. **Time-based retention**: Files older than `retentionDays` are automatically deleted
2. **Disk space retention**: When disk usage exceeds `maxDiskUsagePercent`, oldest files are deleted until usage is below threshold

## Disk Space Monitoring

Disk monitoring provides:
- Total disk space
- Used space
- Available space
- Usage percentage

Monitoring is cached for 30 seconds to reduce system calls.

## Integration with State Manager

The storage service can integrate with the state manager to track stored files in the database:

```go
storageStateManager := NewStorageStateManager(db, logger)
config.StateManager = storageStateManager
```

This enables:
- Tracking file metadata (size, camera, event)
- Querying stored files
- Automatic cleanup of database entries when files are deleted

## Usage Example

```go
// Initialize storage service
config := StorageConfig{
    ClipsDir:            "/data/clips",
    SnapshotsDir:        "/data/snapshots",
    RetentionDays:       7,
    MaxDiskUsagePercent: 80.0,
    StateManager:        storageStateManager,
}
storage, _ := NewStorageService(config, logger)

// Generate clip path
clipPath := storage.GenerateClipPath("camera-1")

// After recording, save clip entry
fileInfo, _ := os.Stat(clipPath)
storage.SaveClip(ctx, clipPath, "camera-1", "event-123", fileInfo.Size())

// Check disk space
hasSpace, _ := storage.CheckDiskSpace(ctx)
if !hasSpace {
    // Pause recording or enforce retention
    storage.EnforceRetention(ctx)
}

// Generate snapshot from frame
generator := NewSnapshotGenerator(storage, SnapshotConfig{}, logger)
snapshotPath, _ := generator.GenerateSnapshot(ctx, frameData, "camera-1", "event-123")
```

## Testing

All components have comprehensive unit tests:

- `storage_test.go` - Storage service tests
- `disk_monitor_test.go` - Disk monitoring tests
- `retention_test.go` - Retention policy tests

Run tests with:
```bash
go test ./internal/storage/...
```

