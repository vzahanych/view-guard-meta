# Event Management

This package provides event detection, generation, storage, queueing, and transmission functionality for the Edge Appliance.

## Overview

The event management system:
- Generates events from AI detection results
- Stores events in SQLite via `state.Manager`
- Manages an event queue with priority and size limits
- Transmits events with retry logic
- Provides event querying and filtering
- Handles event deduplication
- Manages event lifecycle (creation, storage, transmission)

## Components

### Event (`event.go`)

Core event structure with:
- UUID-based event IDs
- Camera ID and timestamp
- Event type (person_detected, vehicle_detected, etc.)
- Detection confidence and bounding boxes
- Metadata (frame dimensions, inference time, etc.)
- Associated clip and snapshot paths

### Generator (`generator.go`)

Event generation from AI detections:
- Converts AI detection results to events
- Applies confidence threshold filtering
- Applies enabled classes filtering
- Deduplication (prevents duplicate events within time window)
- Associates clips and snapshots with events

### Storage (`storage.go`)

Event storage and retrieval:
- Saves events to SQLite via `state.Manager`
- Queries events with filters (camera, type, time range)
- Marks events as transmitted
- Cleans up old events

### Queue (`queue.go`)

Event queue management:
- Enqueue/dequeue operations
- Priority-based ordering
- Queue size limits
- Retry count tracking
- Queue statistics

### Transmitter (`transmitter.go`)

Event transmission service:
- Continuous queue processing
- Batch transmission
- Retry logic with exponential backoff
- Max retry limits
- Queue recovery on startup
- Transmission callback (will be gRPC client in Epic 1.6)

## Usage

### Generate Events from AI Detection

```go
// Create generator
generator := events.NewGenerator(events.GeneratorConfig{
    StateManager:        stateMgr,
    ConfidenceThreshold: 0.5,
    EnabledClasses:      []string{"person", "car"},
    DeduplicationWindow: 5 * time.Second,
}, logger)

// Generate events from detection result
detection := &ai.DetectionResult{
    Response: &ai.InferenceResponse{
        BoundingBoxes: []ai.BoundingBox{...},
    },
    CameraID: "camera-1",
    FrameTimestamp: time.Now(),
}

events, err := generator.GenerateEventsFromDetection(ctx, detection)
if err != nil {
    log.Error("Failed to generate events", err)
    return
}

// Save events to queue
queue := events.NewQueue(events.QueueConfig{
    StateManager: stateMgr,
    MaxSize:      1000,
}, logger)

storage := events.NewStorage(stateMgr, logger)
for _, event := range events {
    // Associate clip/snapshot if available
    if clipPath != "" {
        generator.AssociateClip(event, clipPath)
    }
    if snapshotPath != "" {
        generator.AssociateSnapshot(event, snapshotPath)
    }
    
    // Enqueue event
    err := queue.Enqueue(ctx, event, 0) // priority 0
    if err != nil {
        log.Error("Failed to enqueue event", err)
    }
}
```

### Process Queue with Transmitter

```go
// Create transmitter
transmitter := events.NewTransmitter(events.TransmitterConfig{
    Queue:               queue,
    Storage:             storage,
    BatchSize:           10,
    TransmissionInterval: 5 * time.Second,
    MaxRetries:          3,
    RetryDelay:          1 * time.Second,
    OnTransmit: func(ctx context.Context, events []*events.Event) error {
        // This will be the gRPC client in Epic 1.6
        // For now, just log
        log.Info("Transmitting events", "count", len(events))
        return nil
    },
}, logger)

// Start transmitter
err := transmitter.Start(ctx)
if err != nil {
    log.Fatal("Failed to start transmitter", err)
}
defer transmitter.Stop(ctx)

// Recover queue on startup
err = transmitter.RecoverQueue(ctx)
if err != nil {
    log.Error("Failed to recover queue", err)
}
```

### Query Events

```go
storage := events.NewStorage(stateMgr, logger)

// Get events by camera
events, err := storage.GetEventsByCamera(ctx, "camera-1", 100)

// Get events by type
events, err := storage.GetEventsByType(ctx, events.EventTypePersonDetected, 100)

// Get events in time range
startTime := time.Now().Add(-24 * time.Hour)
endTime := time.Now()
events, err := storage.GetEventsByTimeRange(ctx, startTime, endTime, 100)

// Get pending (untransmitted) events
pending, err := storage.GetPendingEvents(ctx, 100)
```

### Queue Operations

```go
queue := events.NewQueue(events.QueueConfig{
    StateManager: stateMgr,
    MaxSize:      1000,
}, logger)

// Enqueue with priority
err := queue.Enqueue(ctx, event, 10) // high priority

// Dequeue next event
event, err := queue.Dequeue(ctx)

// Batch dequeue
events, err := queue.BatchDequeue(ctx, 10)

// Get queue size
size, err := queue.Size(ctx)

// Get queue statistics
stats, err := queue.GetQueueStats(ctx)
```

## Event Types

- `EventTypePersonDetected` - Person detected (COCO class 0)
- `EventTypeVehicleDetected` - Vehicle detected (car, truck, bus, etc.)
- `EventTypeObjectDetected` - Other object detected
- `EventTypeMotionDetected` - Motion detected (future)
- `EventTypeCustomDetected` - Custom detection class

## Deduplication

The generator prevents duplicate events within a configurable time window (default: 5 seconds). This prevents the same detection from generating multiple events if the AI service detects the same object in consecutive frames.

Deduplication key: `cameraID:classID`

## Queue Priority

Events can be enqueued with a priority (integer). Higher priority events are dequeued first. Default priority is 0.

## Retry Logic

The transmitter implements retry logic:
- Increments retry count on transmission failure
- Configurable max retries (default: 3)
- After max retries, event is marked as transmitted to prevent infinite retries
- Retry delay is configurable (default: 1 second)

## Configuration

Configure event management in `config.yaml`:

```yaml
edge:
  events:
    queue_size: 1000
    batch_size: 10
    transmission_interval: 5s
    max_retries: 3
    retry_delay: 1s
  ai:
    confidence_threshold: 0.5
    enabled_classes: ["person", "car"]
```

## Integration Points

### Current Integration
- **AI Service Client**: `FrameProcessor.OnDetection` callback can call `generator.GenerateEventsFromDetection()`
- **Clip Recorder**: Can associate clips with events using `generator.AssociateClip()`
- **Snapshot Generator**: Can associate snapshots with events using `generator.AssociateSnapshot()`
- **State Manager**: Events are automatically queued when saved (via `state.Manager.SaveEvent()`)

### Future Integration (Epic 1.6)
- **gRPC Client**: `Transmitter.OnTransmit` callback will be replaced with gRPC client to KVM VM
- **WireGuard Tunnel**: Events will be transmitted over WireGuard tunnel to KVM VM

## Next Steps

The event queue and transmission system is ready for:
- **Epic 1.6**: WireGuard Client & Communication (gRPC client integration)
- **Clip Recorder**: Associate clips with events when recording
- **Snapshot Generator**: Associate snapshots with events
