# Edge Web UI Docker Integration

The Edge Web UI has been integrated into the Docker Compose test environment.

## Changes Made

### 1. Dockerfile Updates
- Added frontend build stage using Node.js 20
- Frontend is built before the Go binary
- Built frontend assets are embedded in the Go binary via `embed` package
- Exposed container port 8081 for the web UI (mapped to host 8181 in local compose)

### 2. Docker Compose Updates
- Added port mapping `8181:8081` for web UI access
- Added environment variables for web UI configuration:
  - `EDGE_WEB_ENABLED=true`
  - `EDGE_WEB_HOST=0.0.0.0`
  - `EDGE_WEB_PORT=8081`

### 3. Configuration Updates
- Updated `config.docker.yaml` to enable web UI:
  ```yaml
  web:
    enabled: true
    host: "0.0.0.0"
    port: 8081
  ```

## Building and Running

### Build the Orchestrator

```bash
cd infra/local

# Build the orchestrator (includes frontend build)
docker compose -f docker-compose.edge.yml build edge-orchestrator
```

**Note**: The first build may take several minutes as it:
1. Builds the frontend (Node.js dependencies, TypeScript compilation, Vite build)
2. Builds the Go binary with embedded frontend assets

### Start Services

```bash
# Start all edge services
docker compose -f docker-compose.edge.yml up -d

# View logs
docker compose -f docker-compose.edge.yml logs -f edge-orchestrator
```

### Access the Web UI

Once services are running, open your browser to:

**http://localhost:8181**

The web UI provides:
- **Dashboard**: System status, metrics, and health
- **Cameras**: Live camera viewer with MJPEG streaming
- **Events**: Event timeline with snapshots and clips
- **Configuration**: System configuration management
- **Camera Management**: Add, edit, and manage cameras

### Verify Web UI is Running

```bash
# Check orchestrator logs for web server startup
docker compose -f docker-compose.edge.yml logs edge-orchestrator | grep -i "web"

# Test web UI endpoint
curl http://localhost:8181/api/status

# Test web UI in browser
open http://localhost:8181
# or
xdg-open http://localhost:8181
```

## Testing the Integration

### 1. Check Service Status

```bash
# Check container status
docker compose -f docker-compose.edge.yml ps

# Check orchestrator health
curl http://localhost:8180/health

# Check web UI status
curl http://localhost:8181/api/status
```

### 2. Test Camera Management

1. Open http://localhost:8181 in your browser
2. Navigate to "Camera Management"
3. Click "Discover Cameras" to find RTSP cameras
4. Add a test RTSP camera:
   - Name: "Test Camera"
   - Type: RTSP
   - RTSP URL: `rtsp://rtsp-simulator:8554/test` (from docker-compose.yml RTSP simulator)

### 3. Test Camera Viewer

1. Navigate to "Cameras" in the web UI
2. Select a camera from the dropdown
3. Verify the MJPEG stream loads

### 4. Test Dashboard

1. Navigate to "Dashboard"
2. Verify system metrics are displayed:
   - CPU usage
   - Memory usage
   - Disk usage
   - Application metrics (camera counts, event queue)

### 5. Test Event Timeline

1. Navigate to "Events"
2. Verify events are listed (if any have been generated)
3. Click on an event to view details

## Troubleshooting

### Web UI Not Accessible

1. **Check if web UI is enabled in config:**
   ```bash
   docker compose -f docker-compose.edge.yml exec edge-orchestrator cat /app/config/config.yaml | grep -A 3 web
   ```

2. **Check orchestrator logs:**
   ```bash
   docker compose -f docker-compose.edge.yml logs edge-orchestrator | grep -i "web\|error"
   ```

3. **Verify port is exposed:**
   ```bash
   docker compose -f docker-compose.edge.yml ps edge-orchestrator
  # Should show: 0.0.0.0:8181->8081/tcp
   ```

4. **Check if frontend was built:**
   ```bash
   docker compose -f docker-compose.edge.yml exec edge-orchestrator ls -la /app
   # The binary should include embedded frontend assets
   ```

### Frontend Not Loading

1. **Rebuild the container:**
   ```bash
   docker compose -f docker-compose.edge.yml build --no-cache edge-orchestrator
   docker compose -f docker-compose.edge.yml up -d edge-orchestrator
   ```

2. **Check if static files are embedded:**
   ```bash
   # The Go binary should include embedded static files
   # Check orchestrator logs for "Web server registered"
   docker compose -f docker-compose.edge.yml logs edge-orchestrator | grep "Web server"
   ```

### API Endpoints Not Working

1. **Check API endpoint directly:**
   ```bash
curl http://localhost:8181/api/status
curl http://localhost:8181/api/cameras
   ```

2. **Check CORS settings** (if accessing from different origin)

3. **Verify dependencies are injected:**
   ```bash
   docker compose -f docker-compose.edge.yml logs edge-orchestrator | grep -E "(Camera manager|State manager|Telemetry)"
   ```

## Development Workflow

### Rebuilding After Frontend Changes

If you make changes to the frontend:

```bash
# Rebuild the orchestrator (includes frontend rebuild)
docker compose -f docker-compose.edge.yml build edge-orchestrator

# Restart the service
docker compose -f docker-compose.edge.yml up -d edge-orchestrator
```

### Local Development (Without Docker)

For faster frontend development:

```bash
# In one terminal: Start frontend dev server
cd edge/orchestrator/internal/web/frontend
npm run dev
# Frontend available at http://localhost:5173

# In another terminal: Start orchestrator
cd edge/orchestrator
go run main.go -config ../config/config.dev.yaml
# API available at http://localhost:8080
```

The frontend dev server proxies API requests to `http://localhost:8080` automatically.

## Port Configuration

- **8180 (host) → 8080 (container)**: AI service health endpoint
- **8181 (host) → 8081 (container)**: Web UI/API endpoint

If you need to change the web UI port, update:
1. `infra/local/config.docker.yaml` - `web.port`
2. `infra/local/docker-compose.edge.yml` - port mapping and `EDGE_WEB_PORT` env var
3. `edge/orchestrator/Dockerfile` - `EXPOSE` directive

## Next Steps

1. **Test all UI features** in the Docker environment
2. **Add test cameras** using the RTSP simulator
3. **Verify metrics** are displayed correctly
4. **Test event generation** and viewing
5. **Test configuration updates** via the UI

