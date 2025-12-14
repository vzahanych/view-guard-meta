# Development Docker Compose Setup

This directory contains a development docker-compose configuration that mounts source code directly into containers, allowing for rapid iteration without rebuilding images.

## Quick Start

```bash
# Start development environment (all services)
./start-dev.sh

# Or start specific service
./start-dev.sh user-vm-api
./start-dev.sh edge-orchestrator

# Run tests against dev environment
./test-local-e2e-dev.sh --epic 2.4

# View logs
docker compose -f docker-compose.dev.yml logs -f user-vm-api edge-orchestrator

# Stop and cleanup
docker compose -f docker-compose.dev.yml down -v
```

## Differences from Production

### Development (`docker-compose.dev.yml`)
- **Source code mounted**: Code is mounted as volumes, changes are immediately available
- **Go services run with `go run`**: No binary compilation needed
- **Faster iteration**: Make code changes, restart container (or use `go run` auto-reload)
- **Frontend**: Built on container startup (can be optimized with volume mounts for hot-reload)

### Production (`docker-compose.yml`)
- **Pre-built images**: Uses Dockerfile to build optimized binaries
- **Slower iteration**: Requires `docker compose build` after code changes
- **Better for CI/CD**: Reproducible builds with versioned images

## Development Workflow

1. **Start dev environment**:
   ```bash
   docker compose -f docker-compose.dev.yml up -d
   ```

2. **Make code changes** in your editor

3. **Restart affected service**:
   ```bash
   docker compose -f docker-compose.dev.yml restart user-vm-api
   # or
   docker compose -f docker-compose.dev.yml restart edge-orchestrator
   ```

4. **View logs** to see changes:
   ```bash
   docker compose -f docker-compose.dev.yml logs -f user-vm-api
   ```

5. **Run tests**:
   ```bash
   ./test-local-e2e-dev.sh --epic 2.4
   ```

## Hot Reload

The development setup includes **hot-reload** using `air` for Go services. Code changes are automatically detected and the service restarts:

- **Automatic**: No manual restart needed after code changes
- **Fast**: Only rebuilds when Go files change
- **Smart**: Excludes test files, node_modules, and other non-source files

### How It Works

1. Services automatically install `air` on startup
2. `air` watches for file changes in Go source files
3. On change, it rebuilds and restarts the service
4. Falls back to `go run` if `air` is unavailable

### Configuration

Air configuration files:
- `user-vm-api/.air.toml` - Configuration for user-vm-api service
- `edge/orchestrator/.air.toml` - Configuration for edge-orchestrator service

You can customize these files to adjust:
- Which files to watch
- Build commands
- Exclude patterns
- Delay between changes and rebuild

### Disabling Hot Reload

To disable hot-reload and use `go run` directly, modify the command in `docker-compose.dev.yml`:

```yaml
command: >
  sh -c "
    # ... install dependencies ...
    go run ./cmd/server -config /app/config/config.yaml
  "
```

## Tips

- **First startup is slower**: Dependencies are downloaded on first run
- **Go modules cached**: Subsequent starts are faster
- **Frontend changes**: Currently requires container restart (can be optimized with dev server)
- **Database/data**: Persisted in named volumes, survives container restarts

## Troubleshooting

### Service won't start
- Check logs: `docker compose -f docker-compose.dev.yml logs <service-name>`
- Verify source code is mounted: `docker compose -f docker-compose.dev.yml exec user-vm-api ls -la /workspace`

### Code changes not reflected
- Restart the service: `docker compose -f docker-compose.dev.yml restart <service-name>`
- Check if file is actually mounted: `docker compose -f docker-compose.dev.yml exec user-vm-api cat /workspace/user-vm-api/cmd/server/main.go`

### Port conflicts
- Change port mappings in `docker-compose.dev.yml` if ports are already in use
