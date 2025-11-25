# User VM API Docker Compose Setup (PoC)

This Docker Compose configuration runs the User VM API and MinIO services for local PoC testing.

## Overview

For PoC, the User VM API runs as a Docker Compose service alongside the Edge Appliance:
- **User VM API**: Go service handling WireGuard server, event cache, stream relay, and MinIO integration
- **MinIO**: S3-compatible storage for remote clip archiving (replaces Filecoin in PoC)

**Note**: No SaaS components are needed for PoC. Edge Appliance and User VM API communicate directly.

## Prerequisites

- Docker and Docker Compose installed
- Edge Appliance services running (see `README.EDGE.md`)
- WireGuard tools installed on host (for WireGuard tunnel setup)

## Quick Start

### 1. Start User VM API and MinIO

```bash
cd infra/local

# Start User VM API and MinIO services
docker-compose -f docker-compose.user-vm.yml up -d

# View logs
docker-compose -f docker-compose.user-vm.yml logs -f

# Or follow specific service logs
docker-compose -f docker-compose.user-vm.yml logs -f user-vm-api
docker-compose -f docker-compose.user-vm.yml logs -f minio
```

### 2. Check Service Status

```bash
# Check container status
docker-compose -f docker-compose.user-vm.yml ps

# Check User VM API health (mapped to host port 8280)
curl http://localhost:8280/health

# Check MinIO health (mapped to host port 9000)
curl http://localhost:9000/minio/health/live

# Access MinIO Console UI
# Open browser to: http://localhost:9001
# Login: minioadmin / minioadmin
```

### 3. MinIO Bucket Management

**Note**: Buckets are created automatically by User VM API when needed. Each camera gets its own bucket:
- Bucket naming: `camera-{camera_id}` (e.g., `camera-rtsp-192.168.1.100`, `camera-usb-usb-3-9`)
- Buckets are created on first event/clip upload for that camera
- Event frames: `events/{event_id}/snapshot.jpg`
- Clips: `events/{event_id}/clip.mp4`
- Metadata: `events/{event_id}/metadata.json`

You can verify buckets using MinIO Console UI at http://localhost:9001 or MinIO client:

```bash
# Using MinIO client (mc)
docker-compose -f docker-compose.user-vm.yml exec minio \
  mc alias set local http://localhost:9000 minioadmin minioadmin

# List buckets (will show camera-specific buckets as they are created)
docker-compose -f docker-compose.user-vm.yml exec minio \
  mc ls local/
```

### 4. Stop Services

```bash
docker-compose -f docker-compose.user-vm.yml down

# Remove volumes (clears all data)
docker-compose -f docker-compose.user-vm.yml down -v
```

## Integration with Edge Appliance

To run both Edge Appliance and User VM API together:

```bash
cd infra/local

# Start Edge services
docker-compose -f docker-compose.edge.yml up -d

# Start User VM API services
docker-compose -f docker-compose.user-vm.yml up -d

# View all logs
docker-compose -f docker-compose.edge.yml logs -f &
docker-compose -f docker-compose.user-vm.yml logs -f
```

## WireGuard Configuration

For PoC, WireGuard tunnel setup between Edge and User VM API:

1. **User VM API generates WireGuard keys** on first start
2. **Edge Appliance** connects to User VM API using WireGuard
3. **Tunnel established** over Docker network

**Note**: In production, WireGuard would use public IPs. For PoC, we use Docker network IPs.

## MinIO Configuration

MinIO is configured with:
- **S3 API**: `http://localhost:9000` (host) or `http://minio:9000` (container)
- **Console UI**: `http://localhost:9001`
- **Default credentials**: `minioadmin` / `minioadmin`
- **Bucket organization**: Each camera has its own bucket (`camera-{camera_id}`)
- **AWS Go SDK**: User VM API uses AWS SDK v2 to communicate with MinIO

## Data Persistence

Data is stored in Docker volumes:
- `user-vm-data`: User VM API data (events.db, WireGuard config, models)
- `minio-data`: MinIO storage (archived clips)

To access data:
```bash
# View User VM API data
docker-compose -f docker-compose.user-vm.yml exec user-vm-api ls -la /app/data

# View MinIO data
docker-compose -f docker-compose.user-vm.yml exec minio ls -la /data
```

## Network

Services communicate via the `view-guard-edge` bridge network:
- **User VM API**: `http://user-vm-api:8080` (from Edge)
- **MinIO**: `http://minio:9000` (from User VM API)
- **WireGuard**: UDP port 51820

## Post-PoC: S3-Filecoin Bridge

After PoC, we'll develop an S3-Filecoin bridge to migrate from MinIO to Filecoin:
- Bridge service will sync MinIO objects to Filecoin
- Generate real CIDs for Filecoin storage
- Maintain backward compatibility with S3 API

## Troubleshooting

### User VM API won't start
- Check Docker logs: `docker-compose -f docker-compose.user-vm.yml logs`
- Verify MinIO is healthy: `curl http://localhost:9000/minio/health/live`
- Check disk space: `df -h`

### WireGuard tunnel not working
- Verify WireGuard keys are generated: `docker-compose exec user-vm-api ls -la /app/data/wireguard`
- Check WireGuard interface: `docker-compose exec user-vm-api ip link show wg0`
- Verify network connectivity: `docker-compose exec user-vm-api ping minio`

### MinIO not accessible
- Check MinIO logs: `docker-compose logs minio`
- Verify bucket exists: `docker-compose exec minio mc ls local/`
- Check network connectivity: `docker-compose exec user-vm-api curl http://minio:9000/minio/health/live`

