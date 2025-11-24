# Edge Appliance Docker Compose Setup

This Docker Compose configuration allows you to test the Edge Appliance with a real USB camera connected to your laptop.

## Prerequisites

- Docker and Docker Compose installed
- USB camera connected to your laptop
- FFmpeg installed on host (for testing, optional)
- Internet connection (for model download on first run)

## Quick Start

### 1. Check USB Camera

First, verify your USB camera is detected:

```bash
# List video devices
ls -la /dev/video*

# Check camera info (if v4l-utils is installed)
v4l2-ctl --list-devices
```

### 2. Start Edge Services

```bash
cd infra/local

# Start Edge Appliance services
docker-compose -f docker-compose.edge.yml up -d

# View logs
docker-compose -f docker-compose.edge.yml logs -f

# Or follow specific service logs
docker-compose -f docker-compose.edge.yml logs -f edge-orchestrator
docker-compose -f docker-compose.edge.yml logs -f edge-ai-service
```

### 3. Check Service Status

```bash
# Check container status
docker-compose -f docker-compose.edge.yml ps

# Check AI service health (mapped to host port 8180)
curl http://localhost:8180/health

# Check orchestrator status (mapped to host port 8181)
curl http://localhost:8181/api/status

# Access Web UI
# Open browser to: http://localhost:8181
```

### 4. View Logs

```bash
# All services
docker-compose -f docker-compose.edge.yml logs -f

# Orchestrator only
docker-compose -f docker-compose.edge.yml logs -f edge-orchestrator

# AI service only
docker-compose -f docker-compose.edge.yml logs -f edge-ai-service
```

### 5. Stop Services

```bash
docker-compose -f docker-compose.edge.yml down

# Remove volumes (clears all data)
docker-compose -f docker-compose.edge.yml down -v
```

## USB Camera Access

The Docker Compose setup passes USB camera devices to the orchestrator container:

- **Devices**: `/dev/video0`, `/dev/video1`, `/dev/video2` are passed to the container
- **Privileged mode**: Required for V4L2 device access
- **Volume mount**: `/dev` is mounted to allow device discovery

### Finding Your Camera Device

```bash
# On host, list video devices
ls -la /dev/video*

# Check device info
v4l2-ctl --device=/dev/video0 --all

# Test camera access
ffmpeg -f v4l2 -i /dev/video0 -frames:v 1 test.jpg
```

### Troubleshooting USB Camera Access

If the camera is not detected:

1. **Check device permissions on host:**
   ```bash
   ls -la /dev/video*
   # Should show: crw-rw----+ 1 root video ...
   ```

2. **Add user to video group (if needed):**
   ```bash
   sudo usermod -aG video $USER
   # Log out and back in
   ```

3. **Verify device in container:**
   ```bash
   docker-compose -f docker-compose.edge.yml exec edge-orchestrator ls -la /dev/video*
   ```

4. **Check container logs for camera discovery:**
   ```bash
   docker-compose -f docker-compose.edge.yml logs edge-orchestrator | grep -i camera
   ```

## AI Model Setup

**Important**: In production, AI models are downloaded from a remote VM. For local Docker Compose testing, the model is automatically downloaded on first container start.

### Automatic Model Download

The AI service container will automatically download a YOLOv8 model (yolov8n by default) on first start if no model is found. The model is stored in the `ai-models` Docker volume and persists across container restarts.

**First start may take 2-3 minutes** while the model downloads and converts to OpenVINO format.

### Manual Model Download (Optional)

If you want to pre-download the model before starting containers:

```bash
cd infra/local
./download-model.sh yolov8n
```

This downloads the model to `infra/local/models/` which you can then mount:

```yaml
volumes:
  - ./models:/app/models:ro
```

### Model Options

Available YOLOv8 models (set via `AI_MODEL_NAME` environment variable):
- `yolov8n` - Nano (smallest, fastest) - **default**
- `yolov8s` - Small
- `yolov8m` - Medium
- `yolov8l` - Large
- `yolov8x` - Extra Large (slowest, most accurate)

**Note**: Larger models require more memory and are slower. For testing, `yolov8n` is recommended.

## Configuration

The configuration is provided via:
- **Environment variables** (see `docker-compose.edge.yml`)
- **Config file** (mounted from `edge-config` volume)

To customize configuration:

1. **Edit environment variables** in `docker-compose.edge.yml`
2. **Or mount a custom config file:**
   ```yaml
   volumes:
     - ./config.docker.yaml:/app/config/config.yaml:ro
   ```

## Data Persistence

Data is stored in Docker volumes:
- `edge-data`: Clips, snapshots, database
- `edge-config`: Configuration files
- `ai-models`: AI model files
- `ai-data`: AI service data

To access data:
```bash
# View clips
docker-compose -f docker-compose.edge.yml exec edge-orchestrator ls -la /app/data/clips

# View database
docker-compose -f docker-compose.edge.yml exec edge-orchestrator ls -la /app/data/db
```

## Network

Services communicate via the `view-guard-edge` bridge network:
- **AI Service**: `http://edge-ai-service:8080`
- **Orchestrator**: Health check on port 8080 (if configured)

## Development Mode

For development with live code reloading, you can mount source code:

```yaml
volumes:
  - ../../edge/orchestrator:/app:ro
  - ../../edge/ai-service:/app:ro
```

Then rebuild containers when code changes.

## Testing

### Test USB Camera Discovery

```bash
# Check if camera is discovered
docker-compose -f docker-compose.edge.yml exec edge-orchestrator \
  /app/orchestrator -config /app/config/config.yaml

# Or use the test binary
docker-compose -f docker-compose.edge.yml exec edge-orchestrator \
  /app/bin/test-usb-discovery
```

### Test Video Processing

Once the orchestrator is running, it should:
1. Discover the USB camera
2. Start capturing frames
3. Send frames to AI service for inference
4. Generate events when detections occur
5. Store clips locally

Check logs to verify:
```bash
docker-compose -f docker-compose.edge.yml logs edge-orchestrator | grep -E "(camera|event|clip)"
```

## Notes

- **Privileged mode**: Required for USB camera access. In production, use specific capabilities instead.
- **Device passthrough**: Only specific devices are passed (`/dev/video0`, `/dev/video1`, `/dev/video2`). Add more if needed.
- **FFmpeg**: Included in the orchestrator container for video processing.
- **OpenVINO**: AI service uses OpenVINO for inference (CPU mode in Docker).
- **Model Download**: Models are automatically downloaded on first start for testing. In production, models are downloaded from a remote VM, not locally.
- **Model Persistence**: Models are stored in the `ai-models` Docker volume and persist across container restarts.

## Troubleshooting

### Container won't start
- Check Docker logs: `docker-compose -f docker-compose.edge.yml logs`
- Verify Docker has access to devices
- Check disk space: `df -h`

### Camera not detected
- Verify camera is connected: `ls /dev/video*`
- Check device permissions
- Verify container has access: `docker-compose exec edge-orchestrator ls /dev/video*`

### AI service not responding
- Check AI service logs: `docker-compose logs edge-ai-service`
- Verify network connectivity: `docker-compose exec edge-orchestrator ping edge-ai-service`
- Check health endpoint: `curl http://localhost:8080/health`

### No events generated
- Check camera is online in logs
- Verify AI service is processing frames
- Check confidence threshold in config
- Review event queue status in logs

