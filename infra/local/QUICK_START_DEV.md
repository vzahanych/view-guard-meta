# Quick Start: Development Environment

## 🚀 Start Development Environment

```bash
cd infra/local
./start-dev.sh
```

This will:
1. Generate WireGuard keys and configs
2. Start all services with source code mounted
3. Enable hot-reload (automatic restart on code changes)

## ✏️ Make Code Changes

1. Edit code in your editor (e.g., `user-vm-api/internal/orchestrator/api.go`)
2. Save the file
3. **Hot-reload automatically restarts the service** (check logs to see it rebuild)

## 📊 View Logs

```bash
# All services
docker compose -f docker-compose.dev.yml logs -f

# Specific service
docker compose -f docker-compose.dev.yml logs -f user-vm-api
docker compose -f docker-compose.dev.yml logs -f edge-orchestrator
```

## 🧪 Run Tests

```bash
# Run specific epic
./test-local-e2e-dev.sh --epic 2.4

# Run all tests
./test-local-e2e-dev.sh
```

## 🔄 Restart Service (if hot-reload doesn't work)

```bash
docker compose -f docker-compose.dev.yml restart user-vm-api
docker compose -f docker-compose.dev.yml restart edge-orchestrator
```

## 🛑 Stop Environment

```bash
docker compose -f docker-compose.dev.yml down
```

## 💡 Tips

- **First startup**: Downloads dependencies (~2-3 minutes)
- **Subsequent starts**: Much faster (cached modules)
- **Hot-reload**: Works automatically, no manual restart needed
- **Code changes**: Save file → service auto-restarts → see changes in logs

## 🔍 Verify Hot Reload

1. Start service: `./start-dev.sh user-vm-api`
2. View logs: `docker compose -f docker-compose.dev.yml logs -f user-vm-api`
3. Edit a Go file: `user-vm-api/internal/orchestrator/api.go`
4. Save the file
5. Watch logs - you should see air detecting the change and rebuilding

## 📝 Files Created

- `docker-compose.dev.yml` - Development compose file
- `test-local-e2e-dev.sh` - Test script for dev environment
- `start-dev.sh` - Quick start script
- `user-vm-api/.air.toml` - Hot-reload config for user-vm-api
- `edge/orchestrator/.air.toml` - Hot-reload config for edge-orchestrator
