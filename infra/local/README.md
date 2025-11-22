# Local Development Guide

This guide helps you set up a local development environment for The Private AI Guardian platform.

## Prerequisites

- **Go 1.25+** - [Download](https://go.dev/dl/)
- **Python 3.12+** - [Download](https://www.python.org/downloads/)
- **Docker & Docker Compose** - [Download](https://www.docker.com/products/docker-desktop/)
- **FFmpeg** (optional, for testing RTSP streams) - [Download](https://ffmpeg.org/download.html)

## Quick Start

### 1. Clone and Bootstrap

```bash
git clone git@github.com:vzahanych/view-guard-meta.git
cd view-guard-meta
./scripts/bootstrap-dev.sh
```

### 2. Set Up Edge Appliance Development Environment

```bash
cd edge
./scripts/setup-dev.sh
```

This will:
- Verify Go and Python installations
- Set up Go module dependencies
- Create Python virtual environment
- Create local data directories
- Check for development tools

### 3. Start Local Services

```bash
# Start Docker Compose services (RTSP simulator)
cd infra/local
docker-compose up -d

# Or from repository root:
docker-compose -f infra/local/docker-compose.yml up -d

# Verify RTSP test stream
./edge/scripts/test-rtsp.sh rtsp://localhost:8554/test
```

### 4. Install Development Tools

```bash
# Go linter
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Python formatter and linter
pip install black pylint
```

## Local Services

This directory contains Docker Compose services for local development:

- **`docker-compose.yml`** - Local development services (RTSP simulator, test streams)
- **`rtsp-test/`** - RTSP server configuration for testing

### Services

- **RTSP Simulator** (`rtsp-simulator`)
  - RTSP server on port `8554`
  - Test stream available at `rtsp://localhost:8554/test`

- **RTSP Test Stream** (`rtsp-test-stream`)
  - Generates test pattern video stream
  - Publishes to RTSP server automatically

### Testing RTSP Stream

```bash
# Using the test script
./edge/scripts/test-rtsp.sh rtsp://localhost:8554/test

# Using ffplay
ffplay rtsp://localhost:8554/test

# Using VLC
vlc rtsp://localhost:8554/test
```

### Stop Services

```bash
cd infra/local
docker-compose down
```

Or from anywhere:

```bash
docker-compose -f infra/local/docker-compose.yml down
```

## Development Workflow

### Working on Edge Appliance

1. **Start services:**
   ```bash
   cd infra/local
   docker-compose up -d
   ```

2. **Run Edge orchestrator:**
   ```bash
   cd edge/orchestrator
   go run main.go -config ../config/config.dev.yaml
   ```

3. **Run AI service:**
   ```bash
   cd edge/ai-service
   source venv/bin/activate
   python main.py
   ```

### Code Formatting

**Go:**
```bash
# Format code
gofmt -w ./edge

# Lint code
cd edge
golangci-lint run ./...
```

**Python:**
```bash
# Format code
cd edge/ai-service
black .

# Lint code
pylint .
```

### Running Tests

**Go:**
```bash
cd edge
go test ./... -v
```

**Python:**
```bash
cd edge/ai-service
source venv/bin/activate
pytest -v
```

## IDE Setup

### VS Code / Cursor

The repository includes workspace settings in `.vscode/`:
- **Recommended extensions** are listed in `.vscode/extensions.json`
- **Debugging configurations** are in `.vscode/launch.json`
- **Tasks** for formatting, linting, and testing are in `.vscode/tasks.json`

Install recommended extensions:
```bash
code --install-extension golang.go
code --install-extension ms-python.python
code --install-extension ms-python.black-formatter
```

## Local Testing Environment

### Database

**Note**: Edge Appliance uses **SQLite** for local metadata storage (no PostgreSQL needed).

- **SQLite**: Local database file in `edge/data/db/` (created automatically)

PostgreSQL and Redis are only needed for SaaS backend development (private component). They are commented out in `docker-compose.yml` by default. Uncomment them if working on SaaS backend components.

### Configuration

Development configuration is in `edge/config/config.dev.yaml`. This uses:
- Local data directories (`./data/`)
- Text logging (easier to read during development)
- Shorter intervals for faster testing
- WireGuard disabled for local development

## Project Structure

```
view-guard-meta/
├── edge/                    # Edge Appliance (public component)
│   ├── orchestrator/        # Go orchestrator service
│   ├── ai-service/          # Python AI inference service
│   ├── shared/              # Shared Go libraries
│   ├── config/              # Configuration files
│   └── scripts/             # Build and deployment scripts
├── crypto/                  # Encryption libraries (public component)
│   ├── go/                  # Go encryption library
│   ├── typescript/          # Browser/Node.js library
│   └── python/              # Python library
├── proto/                   # Protocol definitions (public component)
│   ├── proto/               # Protocol buffer definitions
│   └── go/                  # Generated Go stubs
└── infra/local/             # Local development infrastructure
    ├── docker-compose.yml   # Local development services
    └── rtsp-test/            # RTSP test configuration
```

## Troubleshooting

### Go Module Issues

If you encounter module resolution issues:
```bash
cd edge
go mod tidy
go mod download
```

### Python Virtual Environment

If the virtual environment is not working:
```bash
cd edge/ai-service
rm -rf venv
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```

### Docker Services

If Docker services fail to start:
```bash
cd infra/local
docker-compose down
docker-compose up -d
docker-compose logs
```

### RTSP Stream Not Working

1. Check if containers are running: `cd infra/local && docker-compose ps`
2. Check RTSP server logs: `cd infra/local && docker-compose logs rtsp-simulator`
3. Verify port 8554 is not in use: `netstat -an | grep 8554`

## Notes

- These services are for **local development only**
- Edge Appliance uses SQLite (no PostgreSQL/Redis needed for Edge development)
- PostgreSQL and Redis are commented out - uncomment if working on SaaS backend components

## Next Steps

- See `docs/IMPLEMENTATION_PLAN.md` for detailed implementation steps
- See `docs/TECHNICAL_STACK.md` for technology choices
- See `docs/ARCHITECTURE.md` for system architecture
