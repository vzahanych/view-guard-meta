# Edge Orchestrator Web UI

The Edge Orchestrator Web UI provides a local network-accessible web interface for managing and monitoring the Edge Appliance.

## Overview

The web UI is a React-based single-page application (SPA) that communicates with the orchestrator via a RESTful API. The frontend is built with:
- **React 18+** with TypeScript
- **Vite** for fast development and optimized builds
- **Tailwind CSS** for styling
- **Axios** for HTTP requests
- **Recharts** for data visualization

The backend API is built with:
- **Gin** web framework
- **Go** standard library

## Access

### Default Configuration

- **Host**: `0.0.0.0` (accessible on all network interfaces)
- **Port**: `8080` (configurable)
- **URL**: `http://<edge-appliance-ip>:8080`

### Local Network Access

The web UI is designed to be accessible from any device on your local network:

1. **Find the Edge Appliance IP address**:
   ```bash
   # On the Edge Appliance
   ip addr show | grep "inet "
   ```

2. **Access from any device on the network**:
   - Open a web browser
   - Navigate to: `http://<edge-appliance-ip>:8080`
   - Example: `http://192.168.1.100:8080`

### Configuration

The web server can be configured in the orchestrator configuration file:

```yaml
edge:
  web:
    enabled: true
    host: "0.0.0.0"  # Listen on all interfaces
    port: 8080       # Port number
```

## Features

### Dashboard

- **System Status**: Health status, uptime, version
- **System Metrics**: CPU, memory, disk usage with real-time charts
- **Application Metrics**: Event queue length, camera counts
- **Auto-refresh**: Metrics update automatically every 5 seconds

### Camera Viewer

- **Live Streaming**: View MJPEG streams from cameras
- **Multi-camera Grid**: View multiple cameras simultaneously
- **Camera Selection**: Switch between cameras
- **Play/Pause Controls**: Control stream playback
- **Fullscreen Mode**: View streams in fullscreen

### Event Timeline

- **Event Listing**: Paginated list of all events
- **Event Filtering**: Filter by camera, event type, date range
- **Event Details**: View detailed event information
- **Snapshot Viewer**: View event snapshots
- **Clip Viewer**: Playback event video clips

### Configuration

- **Camera Settings**: Configure camera parameters
- **AI Settings**: Configure AI service and detection parameters
- **Storage Settings**: Configure storage paths and retention
- **WireGuard Settings**: Configure VPN connection
- **Telemetry Settings**: Configure telemetry collection
- **Encryption Settings**: Configure encryption parameters

### Camera Management

- **Camera List**: View all registered cameras with status
- **Add Camera**: Add RTSP, ONVIF, or USB cameras
- **Edit Camera**: Update camera configuration
- **Delete Camera**: Remove cameras
- **Camera Discovery**: Discover ONVIF and USB cameras on the network
- **Test Connection**: Test camera connectivity

## API Endpoints

### Status & Health

- `GET /api/status` - System status and health
- `GET /api/metrics` - System metrics (CPU, memory, disk)
- `GET /api/metrics/app` - Application metrics (events, cameras)
- `GET /api/telemetry` - Full telemetry data

### Cameras

- `GET /api/cameras` - List all cameras
- `GET /api/cameras/:id` - Get camera details
- `POST /api/cameras` - Add a new camera
- `PUT /api/cameras/:id` - Update camera
- `DELETE /api/cameras/:id` - Delete camera
- `POST /api/cameras/discover` - Discover cameras
- `POST /api/cameras/:id/test` - Test camera connection

### Events

- `GET /api/events` - List events (with pagination and filtering)
- `GET /api/events/:id` - Get event details
- `GET /api/events/:id/clip` - Get event video clip
- `GET /api/events/:id/snapshot` - Get event snapshot

### Configuration

- `GET /api/config` - Get current configuration
- `PUT /api/config` - Update configuration
- `GET /api/config/sections/:section` - Get configuration section
- `PUT /api/config/sections/:section` - Update configuration section

### Streaming

- `GET /api/streams/:camera_id/mjpeg` - MJPEG stream endpoint

## Request/Response Formats

### Camera Object

```json
{
  "id": "camera-1",
  "name": "Front Door",
  "type": "rtsp",
  "manufacturer": "Hikvision",
  "model": "DS-2CD2342WD-I",
  "enabled": true,
  "status": "online",
  "ip_address": "192.168.1.100",
  "rtsp_url": "rtsp://192.168.1.100/stream",
  "config": {
    "recording_enabled": true,
    "motion_detection": true,
    "quality": "high",
    "frame_rate": 30,
    "resolution": "1920x1080"
  }
}
```

### Event Object

```json
{
  "id": "event-1",
  "camera_id": "camera-1",
  "event_type": "motion",
  "timestamp": "2024-01-15T10:30:00Z",
  "confidence": 0.95,
  "metadata": {
    "bounding_boxes": [...],
    "object_count": 1
  },
  "snapshot_path": "/snapshots/event-1.jpg",
  "clip_path": "/clips/event-1.mp4"
}
```

### Status Response

```json
{
  "health": "healthy",
  "uptime_seconds": 3600,
  "version": "1.0.0"
}
```

### Metrics Response

```json
{
  "cpu": {
    "usage_percent": 45.2,
    "cores": 4
  },
  "memory": {
    "used_bytes": 2147483648,
    "total_bytes": 8589934592,
    "usage_percent": 25.0
  },
  "disk": {
    "used_bytes": 107374182400,
    "total_bytes": 1073741824000,
    "usage_percent": 10.0
  }
}
```

## Development

### Frontend Development

1. **Navigate to frontend directory**:
   ```bash
   cd edge/orchestrator/internal/web/frontend
   ```

2. **Install dependencies**:
   ```bash
   npm install
   ```

3. **Start development server**:
   ```bash
   npm run dev
   ```

   The frontend will be available at `http://localhost:5173` (Vite default port).

4. **Build for production**:
   ```bash
   npm run build
   ```

   The built files will be in `dist/` directory, which should be copied to `static/` for embedding.

### Backend Development

The web server is part of the orchestrator service. To test API endpoints:

1. **Start the orchestrator**:
   ```bash
   cd edge/orchestrator
   go run main.go -config ../config/config.dev.yaml
   ```

2. **Test API endpoints**:
   ```bash
   # Status
   curl http://localhost:8080/api/status

   # List cameras
   curl http://localhost:8080/api/cameras

   # Add camera
   curl -X POST http://localhost:8080/api/cameras \
     -H "Content-Type: application/json" \
     -d '{"name":"Test Camera","type":"rtsp","rtsp_url":"rtsp://example.com/stream"}'
   ```

### Integration Testing

Run integration tests:

```bash
cd edge/orchestrator
go test -v ./internal/web -run TestWebServer
```

## Architecture

### Frontend Structure

```
frontend/
├── src/
│   ├── components/     # Reusable UI components
│   ├── pages/         # Page-level components
│   ├── utils/         # Utility functions (API client)
│   ├── styles/        # CSS styles
│   └── main.tsx       # Entry point
├── index.html         # HTML template
└── package.json       # Dependencies
```

### Backend Structure

```
internal/web/
├── server.go          # Web server service
├── handlers.go        # HTTP handlers
├── streaming/         # MJPEG streaming service
└── static/            # Embedded frontend assets
```

### Dependency Injection

The web server receives dependencies via setter methods:

- `SetDependencies(cameraMgr, ffmpeg)` - Camera manager and FFmpeg wrapper
- `SetEventDependencies(stateMgr, storageSvc)` - State manager and storage service
- `SetConfigDependency(configSvc)` - Configuration service
- `SetTelemetryDependency(collector)` - Telemetry collector

These are wired up in `main.go` during orchestrator initialization.

## Troubleshooting

### Web UI Not Accessible

1. **Check if web server is enabled**:
   ```yaml
   edge:
     web:
       enabled: true
   ```

2. **Check firewall settings**:
   ```bash
   # Allow port 8080
   sudo ufw allow 8080
   ```

3. **Check server logs**:
   ```bash
   # Look for web server startup messages
   journalctl -u edge-orchestrator | grep "web"
   ```

### API Endpoints Return 404

- Ensure the web server is started and dependencies are injected
- Check that routes are registered in `server.go`
- Verify the API path matches the frontend API client configuration

### MJPEG Streams Not Loading

- Ensure FFmpeg is installed and available
- Check camera connection status
- Verify camera RTSP URL is correct
- Check streaming service logs

### Frontend Build Errors

- Ensure Node.js 18+ is installed
- Run `npm install` to install dependencies
- Check for TypeScript errors: `npm run type-check`
- Verify Vite configuration

## Security Considerations

⚠️ **Important**: The web UI is designed for local network access only. It does not include authentication by default.

For production deployments:
- Use a reverse proxy (nginx, Traefik) with authentication
- Enable HTTPS/TLS
- Restrict access via firewall rules
- Consider adding authentication middleware to the web server

## Future Enhancements

- [ ] User authentication and authorization
- [ ] Real-time event notifications (WebSocket)
- [ ] Camera PTZ controls
- [ ] Event search and advanced filtering
- [ ] Export events and clips
- [ ] System logs viewer
- [ ] Backup and restore configuration

