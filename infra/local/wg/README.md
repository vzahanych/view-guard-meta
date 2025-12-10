# WireGuard Key and Config Management

## Overview

This directory contains scripts and templates for generating WireGuard keys and configuration files for the local development environment.

## Key Persistence Behavior

### Keys are Preserved Across Service Restarts
- **Keys are stored** in `./wg/keys/` directory on the host filesystem
- **Keys persist** across `docker compose restart` or `docker compose down` (without `-v`)
- **Keys are NOT regenerated** when services restart - they are preserved

### Keys are Regenerated on Clean Start
- **Keys are removed** when running `test-phase2.sh` with cleanup (`cleanup_old_data`)
- **Keys are regenerated** when `wg-setup` service runs and finds no existing keys
- This ensures fresh keys on a clean test run

## Files

- `generate-keys.sh` - Generates WireGuard keys (server, edge, preshared key)
  - **Skips generation if keys already exist** (preserves keys across restarts)
  - Generates new keys only if `wg/keys/` directory is empty
  
- `generate-configs.sh` - Generates WireGuard config files from keys
  - **Always regenerates configs** from current keys
  - Ensures configs match the keys in `wg/keys/`
  
- `server-wg0.conf` - Template for server WireGuard config
- `edge-wg0.conf` - Template for edge WireGuard config

## Usage

### Normal Operation (Keys Preserved)
```bash
# Start services - keys are preserved if they exist
docker compose up -d

# Restart services - keys remain unchanged
docker compose restart

# Stop services - keys remain unchanged
docker compose down
```

### Clean Start (Keys Regenerated)
```bash
# Run test with cleanup - keys will be regenerated
./test-phase2.sh --epic 2.2

# Or manually clean and regenerate
rm -rf wg/keys/* wg/config/*.conf
docker compose run --rm wg-setup
```

## Troubleshooting

### Keys Don't Match Between Services
If you see WireGuard connection issues:
1. Check that keys match: `cat wg/keys/server.public` and `cat wg/keys/edge.public`
2. Regenerate configs: `docker compose run --rm wg-setup`
3. Restart services: `docker compose restart edge-orchestrator user-vm-api`

### Force Key Regeneration
To force new keys (e.g., after a key mismatch):
```bash
rm -rf wg/keys/* wg/config/*.conf
docker compose run --rm wg-setup
docker compose restart edge-orchestrator user-vm-api
```

