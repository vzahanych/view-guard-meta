# Camera Management

Camera management package for RTSP/ONVIF camera discovery, connection, and stream handling.

## Features

- **RTSP Client**: Full RTSP client implementation using gortsplib
- **ONVIF Discovery**: Automatic ONVIF camera discovery using WS-Discovery
- **Stream Connection**: Automatic connection and reconnection
- **Health Monitoring**: Stream health monitoring and frame tracking
- **Error Handling**: Robust error handling for network issues
- **Manual Configuration**: Support for manual RTSP URL configuration

## RTSP Client

### Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"

// Create RTSP client
rtspClient := camera.NewRTSPClient(camera.RTSPClientConfig{
    URL:               "rtsp://192.168.1.100:554/stream",
    Username:          "admin",
    Password:          "password",
    Timeout:           30 * time.Second,
    ReconnectInterval: 5 * time.Second,
    OnFrameCallback: func(frameData []byte, timestamp time.Time) {
        // Process frame data
    },
}, logger)

// Start client
rtspClient.Start(ctx)

// Check connection status
if rtspClient.IsConnected() {
    // Client is connected
}

// Get health status
status := rtspClient.GetHealthStatus() // "connected", "disconnected", "degraded", "error"
```

### Features

- **Automatic Reconnection**: Automatically reconnects on connection loss
- **Health Monitoring**: Monitors stream health and frame reception
- **Frame Callbacks**: Callback function for received frames
- **Event Publishing**: Publishes connection/disconnection events
- **Thread-Safe**: All operations are thread-safe

### Configuration

- **URL**: RTSP stream URL (e.g., `rtsp://192.168.1.100:554/stream`)
- **Username/Password**: Optional authentication credentials
- **Timeout**: Connection timeout
- **ReconnectInterval**: Time to wait before reconnecting
- **OnFrameCallback**: Callback function called for each received frame

### Health Status

- **connected**: Stream is connected and receiving frames
- **disconnected**: Stream is not connected
- **degraded**: Stream is connected but not receiving frames
- **error**: Connection error occurred

### Events

The RTSP client publishes events via the event bus:
- `EventTypeCameraConnected` - When camera connects
- `EventTypeCameraDisconnected` - When camera disconnects

## Manual RTSP URL Configuration

For PoC, cameras can be configured manually via RTSP URLs:

```yaml
cameras:
  - id: "camera-1"
    name: "Front Door"
    rtsp_url: "rtsp://192.168.1.100:554/stream"
    username: "admin"
    password: "password"
    enabled: true
```

## ONVIF Discovery

### Testing Discovery on Home Network

To test ONVIF discovery on your home network from your development laptop:

**Option 1: Using the test binary (recommended)**
```bash
cd edge/orchestrator
go build -o bin/test-onvif-discovery ./cmd/test-onvif-discovery
./bin/test-onvif-discovery
```

**Option 2: Using the test script**
```bash
cd edge/orchestrator
./scripts/test-onvif-discovery.sh
```

**Option 3: Using Go tests**
```bash
cd edge/orchestrator
ONVIF_TEST_NETWORK=1 go test -v -run TestONVIFDiscoveryHomeNetwork -timeout 30s ./internal/camera
```

The test will:
- Scan your local network for ONVIF cameras
- Display discovered cameras with their details
- Show RTSP URLs that were detected
- Provide helpful diagnostics if no cameras are found

**Requirements:**
- Your development laptop must be on the same WiFi network as the cameras
- Cameras must support ONVIF and WS-Discovery
- Network must allow multicast traffic (most home routers do by default)

### Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"

// Create ONVIF discovery service
discovery := camera.NewONVIFDiscoveryService(5*time.Minute, logger)

// Start discovery
discovery.Start(ctx)

// Get discovered cameras
cameras := discovery.GetDiscoveredCameras()
for _, cam := range cameras {
    fmt.Printf("Found camera: %s (%s) at %s\n", cam.Model, cam.Manufacturer, cam.IPAddress)
    fmt.Printf("  RTSP URLs: %v\n", cam.RTSPURLs)
}

// Get specific camera
camera := discovery.GetCameraByID("onvif-192.168.1.100")

// Trigger immediate discovery
discovery.TriggerDiscovery()
```

### Features

- **WS-Discovery**: Uses WS-Discovery protocol for network discovery
- **Home WiFi Network Support**: Optimized for home WiFi networks where MiniPC and cameras are on the same subnet
- **Network Interface Selection**: Automatically selects the correct network interface (WiFi/Ethernet) for discovery
- **Automatic Discovery**: Periodic discovery at configurable intervals
- **Camera Information**: Extracts manufacturer, model, IP address
- **RTSP URL Detection**: Attempts to detect RTSP stream URLs
- **Capability Detection**: Detects camera capabilities (PTZ, snapshots, etc.)
- **Event Publishing**: Publishes discovery events via event bus

### Home WiFi Network Setup

The ONVIF discovery is designed to work on typical home WiFi networks:

- **Same Subnet**: MiniPC (Edge Appliance) and WiFi cameras must be on the same network/subnet
- **Multicast Support**: Home WiFi routers typically support multicast on the local network
- **Automatic Interface Detection**: The service automatically finds and uses the correct network interface
- **Private Network Detection**: Prefers private network interfaces (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
- **VPN Filtering**: Automatically skips VPN interfaces (tun, tap, wg) to use the local network interface

### Discovery Process

1. **WS-Discovery Probe**: Sends multicast probe message to network
2. **Response Collection**: Collects responses from ONVIF devices
3. **Device Probing**: Probes each device for detailed information
4. **RTSP URL Extraction**: Attempts to extract RTSP stream URLs
5. **Capability Detection**: Detects camera capabilities

### Discovered Camera Information

- **ID**: Unique camera identifier
- **Manufacturer**: Camera manufacturer
- **Model**: Camera model
- **IP Address**: Camera IP address
- **ONVIF Endpoint**: ONVIF service endpoint
- **RTSP URLs**: Detected RTSP stream URLs
- **Capabilities**: Camera capabilities (PTZ, snapshots, etc.)
- **Last Seen**: Last time camera was seen
- **Discovered At**: When camera was first discovered

### Limitations (PoC)

For PoC, the ONVIF discovery is simplified:
- Basic WS-Discovery implementation
- RTSP URL guessing (common patterns)
- Limited capability detection
- No full ONVIF API integration

For production, consider:
- Full ONVIF API client library
- Proper GetStreamUri calls
- Complete capability detection
- Authentication support

## USB Camera Discovery

### Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"

// Create USB discovery service
usbDiscovery := camera.NewUSBDiscoveryService(5*time.Minute, "/dev", logger)

// Start discovery
usbDiscovery.Start(ctx)

// Get discovered cameras
cameras := usbDiscovery.GetDiscoveredCameras()
for _, cam := range cameras {
    fmt.Printf("Found USB camera: %s at %s\n", cam.Model, cam.IPAddress)
    // Device path is in cam.IPAddress (e.g., /dev/video0)
    // Use with FFmpeg: ffmpeg -i /dev/video0 ...
}

// Get specific camera
camera := usbDiscovery.GetCameraByID("usb-video0")

// Trigger immediate discovery
usbDiscovery.TriggerDiscovery()
```

### Features

- **V4L2 Detection**: Uses Video4Linux2 to detect USB cameras
- **Device Scanning**: Scans /dev/video* devices automatically
- **Device Information**: Attempts to get manufacturer and model via v4l2-ctl or sysfs
- **Capability Detection**: Detects video stream and snapshot capabilities
- **Hotplug Support**: Detects cameras when plugged/unplugged
- **Event Publishing**: Publishes discovery events via event bus

### USB Camera Access

USB cameras are accessed directly via device path (not RTSP):
- Device path: `/dev/video0`, `/dev/video1`, etc.
- Use with FFmpeg: `ffmpeg -i /dev/video0 ...`
- Use with OpenCV: `cv2.VideoCapture(0)` or device path
- Use with GStreamer: `v4l2src device=/dev/video0`

### Requirements

- Linux system with V4L2 support
- USB camera connected and recognized by kernel
- Optional: `v4l2-ctl` for detailed device information

### Testing USB Discovery

```bash
cd edge/orchestrator
go build -o bin/test-usb-discovery ./cmd/test-usb-discovery
./bin/test-usb-discovery
```

## Dependencies

- `github.com/bluenviron/gortsplib/v4` - RTSP client library
- `github.com/pion/rtp` - RTP packet handling

