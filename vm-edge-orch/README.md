# User VM API

The User VM API is the open-source server that runs on each customer's dedicated VM (Private Cloud Node). It handles WireGuard tunnel termination, event caching, AI model orchestration, secondary event analysis, and remote storage.

## Overview

The User VM API manages:
- WireGuard server for Edge Appliance connections
- Event cache and forwarding to Management Server (post-PoC)
- AI model catalog and training pipeline
- Secondary event analysis and alerting
- Stream relay for on-demand clip viewing
- Remote storage (MinIO/S3 for PoC, Filecoin/IPFS post-PoC) integration
  - Uses AWS Go SDK v2 to communicate with MinIO
  - Each camera has its own bucket (`camera-{camera_id}`) for organizing event frames and clips
- Telemetry aggregation

## Structure

```
user-vm-api/
├── cmd/server/          # Main server entry point
├── internal/
│   ├── wireguard-server/    # WireGuard server service
│   ├── event-cache/          # Event cache service
│   ├── stream-relay/         # Stream relay service
│   ├── storage-sync/         # Storage sync service (MinIO/S3 for PoC, Filecoin post-PoC)
│   ├── ai-orchestrator/      # AI model catalog and training
│   ├── event-analyzer/       # Secondary event analysis
│   ├── telemetry-aggregator/ # Telemetry aggregation
│   ├── orchestrator/         # Main orchestrator service
│   └── shared/               # Shared libraries
│       ├── config/           # Configuration management
│       ├── logging/          # Structured logging
│       ├── service/          # Service base and manager
│       └── database/         # Database utilities
├── config/               # Configuration files
├── scripts/              # Build and deployment scripts
└── go.mod                # Go module dependencies
```

## Building

```bash
cd user-vm-api
go build -o user-vm-api ./cmd/server
```

## Running

```bash
# With default config (searches common locations)
./user-vm-api

# With custom config
./user-vm-api -config config/config.yaml
```

## Configuration

The User VM API reads YAML configuration files. See `config/config.yaml.example` for an example.

Key configuration sections:
- `user_vm_api.orchestrator` - Orchestrator settings
- `user_vm_api.wireguard_server` - WireGuard server configuration
- `user_vm_api.event_cache` - Event cache settings
- `user_vm_api.stream_relay` - Stream relay configuration
- `user_vm_api.storage_sync` - Storage sync configuration (MinIO/S3 for PoC, Filecoin post-PoC)
- `user_vm_api.ai_orchestrator` - AI model catalog and training settings
- `user_vm_api.event_analyzer` - Secondary event analysis settings
- `user_vm_api.telemetry_aggregator` - Telemetry aggregation settings
- `user_vm_api.management_server` - Management Server connection settings (disabled for PoC)
- `log` - Logging configuration

## Dependencies

- **Proto definitions**: Imported from `proto/go` (Edge ↔ User Server and User Server ↔ Management Server)
- **Crypto library**: Imported from `crypto/go` (if needed for encryption verification)
- **WireGuard**: `golang.zx2c4.com/wireguard/wgctrl` for WireGuard server management
- **gRPC**: For communication with Edge Appliances (and Management Server post-PoC)
- **SQLite**: For local event cache and metadata storage
- **AWS Go SDK v2**: `github.com/aws/aws-sdk-go-v2` for MinIO/S3-compatible storage (PoC)
  - S3 client for uploading/downloading clips, snapshots, and metadata
  - Each camera has its own MinIO bucket for organizing event data
  - Bucket naming: `camera-{camera_id}` (e.g., `camera-rtsp-192.168.1.100`)
- **Filecoin**: Filecoin client libraries (post-PoC, via S3-Filecoin bridge)

## Privacy & Security

- **Open Source**: This component is fully open source (Apache 2.0) for auditability
- **Secrets in Memory**: WireGuard keys, encryption key identifiers, and other secrets are kept in memory only at runtime
- **No Secrets in Code**: Secrets are not stored in the codebase or committed to version control
- **User's VM**: Runs on customer's dedicated VM, providing tenant isolation

## PoC Deployment

For PoC, the User VM API runs as a **Docker Compose service** in the local development environment:
- **No SaaS components** - Edge Appliance and User VM API communicate directly
- **MinIO instead of Filecoin** - Use MinIO (S3-compatible) for remote storage
- **AWS Go SDK** - User VM API uses AWS SDK v2 to communicate with MinIO
- **Per-camera buckets** - Each camera has its own MinIO bucket (`camera-{camera_id}`) for organizing event frames and clips
- **Docker Compose integration** - See `infra/local/docker-compose.user-vm.yml` and `infra/local/README.USER_VM.md`

**Storage Organization**:
- Event frames/snapshots: `events/{event_id}/snapshot.jpg`
- Clips: `events/{event_id}/clip.mp4`
- Metadata: `events/{event_id}/metadata.json`
- All stored in camera-specific buckets for easy organization and quota management

Post-PoC, an S3-Filecoin bridge will be developed to migrate from MinIO to Filecoin.

