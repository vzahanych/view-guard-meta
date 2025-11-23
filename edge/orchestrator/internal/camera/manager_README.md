# Camera Management Service

The camera management service provides a unified interface for managing both network cameras (RTSP/ONVIF) and USB cameras (V4L2).

## Features

- **Unified Interface**: Single API for all camera types (RTSP, ONVIF, USB)
- **Automatic Registration**: Automatically registers cameras discovered by ONVIF and USB discovery services
- **State Persistence**: Cameras are persisted to SQLite database
- **Status Monitoring**: Continuous monitoring of camera status (online/offline)
- **Configuration Management**: Per-camera configuration (recording, motion detection, quality)
- **Enable/Disable**: Enable or disable cameras without deleting them

## Usage

### Initialize Camera Manager

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"

// Create camera manager
cameraMgr := camera.NewManager(
    stateMgr,           // State manager for persistence
    onvifDiscovery,     // ONVIF discovery service (can be nil)
    usbDiscovery,       // USB discovery service (can be nil)
    30*time.Second,     // Status monitoring interval
    logger,
)

// Set event bus for inter-service communication
cameraMgr.SetEventBus(eventBus)

// Start the service
cameraMgr.Start(ctx)
defer cameraMgr.Stop(ctx)
```

### Register a Camera

Cameras are automatically registered when discovered by ONVIF or USB discovery services. You can also manually register a discovered camera:

```go
// Get discovered camera from discovery service
discovered := onvifDiscovery.GetCameraByID("onvif-192.168.1.100")

// Register it
err := cameraMgr.RegisterCamera(ctx, discovered)
```

### List Cameras

```go
// List all cameras
cameras := cameraMgr.ListCameras(false)

// List only enabled cameras
enabledCameras := cameraMgr.ListCameras(true)

for _, cam := range cameras {
    fmt.Printf("Camera: %s (%s) - Status: %s\n", 
        cam.Name, cam.Type, cam.Status)
}
```

### Get Camera

```go
camera, err := cameraMgr.GetCamera("camera-1")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Camera: %s\n", camera.Name)
fmt.Printf("Type: %s\n", camera.Type)
fmt.Printf("Status: %s\n", camera.Status)
```

### Update Camera Configuration

```go
config := camera.CameraConfig{
    RecordingEnabled: true,
    MotionDetection:  true,
    Quality:          "high",
    FrameRate:        30,
    Resolution:       "1920x1080",
}

err := cameraMgr.UpdateCameraConfig(ctx, "camera-1", config)
```

### Enable/Disable Camera

```go
// Disable camera
err := cameraMgr.DisableCamera(ctx, "camera-1")

// Enable camera
err := cameraMgr.EnableCamera(ctx, "camera-1")
```

### Get Camera Status

```go
status, err := cameraMgr.GetCameraStatus("camera-1")
if err != nil {
    log.Fatal(err)
}

switch status {
case camera.CameraStatusOnline:
    fmt.Println("Camera is online")
case camera.CameraStatusOffline:
    fmt.Println("Camera is offline")
case camera.CameraStatusError:
    fmt.Println("Camera has an error")
}
```

### Delete Camera

```go
err := cameraMgr.DeleteCamera(ctx, "camera-1")
```

## Camera Types

### Network Cameras (RTSP/ONVIF)

- **Type**: `CameraTypeRTSP` or `CameraTypeONVIF`
- **Access**: Via RTSP URLs
- **Status**: Based on RTSP client connection status

### USB Cameras

- **Type**: `CameraTypeUSB`
- **Access**: Via device path (e.g., `/dev/video0`)
- **Status**: Based on device presence in discovery service

## Camera Status

- **CameraStatusOnline**: Camera is connected and accessible
- **CameraStatusOffline**: Camera is not accessible
- **CameraStatusConnecting**: Camera connection in progress
- **CameraStatusError**: Camera has an error
- **CameraStatusUnknown**: Camera status is unknown

## Events

The camera manager publishes the following events:

- **EventTypeCameraRegistered**: When a camera is registered
- **EventTypeCameraDiscovered**: (from discovery services) When a camera is discovered

## Integration with Discovery Services

The camera manager automatically subscribes to discovery events from:
- ONVIF discovery service
- USB discovery service

When a camera is discovered, it's automatically registered and persisted to the database.

## State Persistence

Cameras are persisted to SQLite via the state manager. On startup, cameras are recovered from the database and their status is monitored.

## Status Monitoring

The camera manager continuously monitors camera status at configurable intervals (default: 30 seconds). Status is updated based on:
- Network cameras: RTSP client connection status
- USB cameras: Device presence in discovery service

