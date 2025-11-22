# State Management

The state management system provides SQLite-based persistence for system state, cameras, events, and telemetry data.

## Features

- **SQLite Database**: Lightweight, file-based database for state persistence
- **State Recovery**: Automatic recovery of state on restart
- **Camera Management**: Persist camera configurations and status
- **Event Queue**: Persistent event queue for reliable transmission
- **System State**: Key-value storage for system configuration
- **Storage Tracking**: Track stored clips and snapshots
- **Thread-Safe**: All operations are thread-safe

## Database Schema

### Tables

- **`system_state`** - Key-value pairs for system configuration
- **`cameras`** - Camera configurations and status
- **`events`** - Event records with metadata
- **`event_queue`** - Queue of pending events for transmission
- **`telemetry`** - Buffered telemetry data
- **`storage_state`** - Track stored clips and snapshots

## Usage

### Initialize State Manager

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"

// Create state manager
stateMgr, err := state.NewManager(cfg, logger)
if err != nil {
    log.Fatal(err)
}
defer stateMgr.Close()
```

### Recover State on Startup

```go
// Recover state from database
recovered, err := stateMgr.RecoverState(ctx)
if err != nil {
    log.Fatal(err)
}

// Restore cameras
for _, cam := range recovered.Cameras {
    // Restore camera connections
}

// Restore pending events
for _, event := range recovered.Events {
    // Re-queue events for transmission
}
```

### Camera Management

```go
// Save camera
cam := state.CameraState{
    ID:      "camera-1",
    Name:    "Front Door",
    RTSPURL: "rtsp://192.168.1.100:554/stream",
    Enabled: true,
}
err := stateMgr.SaveCamera(ctx, cam)

// Get camera
camera, err := stateMgr.GetCamera(ctx, "camera-1")

// List cameras
cameras, err := stateMgr.ListCameras(ctx, true) // enabled only

// Update last seen
err := stateMgr.UpdateCameraLastSeen(ctx, "camera-1")
```

### Event Management

```go
// Save event
event := state.EventState{
    ID:          "event-123",
    CameraID:    "camera-1",
    EventType:   "person_detected",
    Timestamp:   time.Now(),
    Metadata:    map[string]interface{}{"confidence": 0.95},
    ClipPath:    "/path/to/clip.mp4",
    Transmitted: false,
}
err := stateMgr.SaveEvent(ctx, event)

// Get pending events
pending, err := stateMgr.GetPendingEvents(ctx, 100)

// Mark event as transmitted
err := stateMgr.MarkEventTransmitted(ctx, "event-123")

// Cleanup old events
err := stateMgr.CleanupOldEvents(ctx, 30*24*time.Hour) // 30 days
```

### System State

```go
// Save system state
err := stateMgr.SaveSystemState(ctx, "last_sync", time.Now().Format(time.RFC3339))

// Get system state
value, err := stateMgr.GetSystemState(ctx, "last_sync")
```

## State Recovery

On startup, the state manager automatically recovers:

1. **Cameras**: All enabled cameras are restored
2. **Pending Events**: Events not yet transmitted are restored
3. **System State**: All system state key-value pairs are restored

This ensures continuity across restarts.

## Database Location

The database is stored at:
```
{data_dir}/db/edge.db
```

Where `data_dir` is configured in `edge.orchestrator.data_dir`.

## WAL Mode

The database uses Write-Ahead Logging (WAL) mode for better concurrency and performance.

## Foreign Keys

Foreign key constraints are enabled to maintain referential integrity:
- Events reference cameras
- Storage state references cameras and events
- Event queue references events

## Thread Safety

All state operations are thread-safe using read-write mutexes. Multiple goroutines can safely access the state manager concurrently.

