# Phase 1: Edge Appliance (Mini PC) - Go & Python Apps

**Duration**: 3-4 weeks  
**Goal**: Build core Edge Appliance software - Go orchestrator, Python AI service, video processing, local storage, WireGuard client

**Scope**: Single Mini PC, 1-2 cameras, basic functionality sufficient for PoC demonstration

**Status**: ~88% Complete (Epic 1.9 complete, remaining: encryption & archive client)
- ✅ Epic 1.1: Development Environment (mostly complete, CI/CD deferred)
- ✅ Epic 1.2: Go Orchestrator Core Framework (complete)
- ✅ Epic 1.3: Video Ingest & Processing (complete)
- ✅ Epic 1.4: Python AI Inference Service (complete)
- ✅ Epic 1.5: Event Management & Queue (complete)
- ✅ Epic 1.6: WireGuard Client & Communication (complete)
- ✅ Epic 1.7: Telemetry & Health Reporting (complete)
- ⬜ Epic 1.8: Encryption & Archive Client (not started)
- ✅ Epic 1.9: Edge Web UI (COMPLETE - All backend APIs, frontend UI components, and integration tests complete)

**Test Coverage**: 230 tests (220 unit + 10 integration), all passing ✅

**See [Main Implementation Plan](IMPLEMENTATION_PLAN.md) for overview, progress tracking, and success criteria.**

---

## Epic 1.1: Development Environment & Project Setup

**Priority: P0**

### Step 1.1.1: Repository & Project Structure
- **Substep 1.1.1.1**: Verify meta repository structure
  - **Status**: ✅ DONE
  - Public components are developed directly in the meta repository
  - Edge Appliance code lives in `edge/` directory
  - Crypto libraries live in `crypto/` directory
  - Protocol definitions live in `proto/` directory
  - Set up `.gitignore` files if needed
- **Substep 1.1.1.2**: Create Edge Appliance directory structure
  - **Status**: ✅ DONE
  - `edge/orchestrator/` - Go main orchestrator service
  - `edge/ai-service/` - Python AI inference service
  - `edge/shared/` - Shared Go libraries
  - `edge/config/` - Configuration files
  - `edge/scripts/` - Build and deployment scripts
  - Note: gRPC proto definitions are in `proto/proto/edge/` (not in edge/)
- **Substep 1.1.1.3**: Set up CI/CD basics
  - **Status**: ⬜ TODO
  - GitHub Actions for Edge services (in meta repo)
  - Docker image builds for Go and Python services
  - Linting and basic tests

### Step 1.1.2: Local Development Environment
- **Substep 1.1.2.1**: Development tooling setup
  - **Status**: ✅ DONE
  - Install Go 1.25+, Python 3.12+ (as per TECHNICAL_STACK.md)
  - Set up code formatters (gofmt, black)
  - Configure linters (golangci-lint, pylint)
- **Substep 1.1.2.2**: Local testing environment
  - **Status**: ✅ DONE
  - Docker Compose for local services (if needed)
  - Mock camera setup (RTSP test stream)
  - Local SQLite database setup
- **Substep 1.1.2.3**: IDE configuration
  - **Status**: ✅ DONE
  - VS Code / Cursor workspace settings
  - Debugging configurations for Go and Python
  - Code snippets

---

## Epic 1.2: Go Orchestrator Service - Core Framework

**Priority: P0**

### Step 1.2.1: Orchestrator Service Structure
- **Substep 1.2.1.1**: Main service framework
  - **Status**: ✅ DONE
  - Service initialization
  - Configuration management (YAML/JSON config)
  - Logging setup (structured JSON logging)
  - Graceful shutdown handling
- **Substep 1.2.1.2**: Service architecture
  - **Status**: ✅ DONE
  - Service manager pattern
  - Service lifecycle management
  - Inter-service communication (channels/events)
- **Substep 1.2.1.3**: Health check system
  - **Status**: ✅ DONE
  - Health check endpoints
  - Service status reporting
  - Dependency health checks
- **Substep 1.2.1.4**: Unit tests for orchestrator service structure
  - **Status**: ✅ DONE
  - **P0**: Test service initialization and shutdown
  - **P0**: Test service manager registration and lifecycle
  - **P0**: Test event bus integration
  - **P0**: Test health check endpoints and responses
  - **P1**: Test service status tracking and reporting

### Step 1.2.2: Configuration & State Management
- **Substep 1.2.2.1**: Configuration service
  - **Status**: ✅ DONE
  - Config file loading
  - Environment variable support
  - Config validation
- **Substep 1.2.2.2**: State management
  - **Status**: ✅ DONE
  - System state persistence (SQLite)
  - State recovery on restart
  - State synchronization
- **Substep 1.2.2.3**: Unit tests for configuration and state management
  - **Status**: ✅ DONE
  - **P0**: Test config file loading and validation ✅
  - **P0**: Test environment variable overrides ✅
  - **P0**: Test state persistence and recovery ✅
  - **P0**: Test camera state CRUD operations ✅
  - **P0**: Test event state storage and retrieval ✅
  - **P1**: Test config hot reload functionality ✅

---

## Epic 1.3: Video Ingest & Processing (Go)

**Priority: P0**

### Step 1.3.1: Camera Discovery & Connection
- **Substep 1.3.1.1**: RTSP client implementation
  - **Status**: ✅ DONE
  - **P0**: Go RTSP client using `gortsplib`
  - **P0**: Stream connection and reconnection logic
  - **P0**: Error handling for network issues
  - **P0**: Stream health monitoring
  - **P0**: Manual RTSP URL configuration (for PoC)
- **Substep 1.3.1.2**: ONVIF camera discovery
  - **Status**: ✅ DONE
  - **P1**: ONVIF device discovery (WS-Discovery)
  - **P1**: Camera capability detection
  - **P1**: Stream URL extraction
  - **P2**: Camera configuration retrieval
- **Substep 1.3.1.3**: USB camera discovery (V4L2)
  - **Status**: ✅ DONE
  - **P0**: USB camera detection via V4L2 (Video4Linux2)
  - **P0**: Scan `/dev/video*` devices automatically
  - **P0**: Device information extraction (manufacturer, model via `v4l2-ctl` or sysfs)
  - **P0**: Hotplug support (detect cameras when plugged/unplugged)
  - **P0**: Device path access for FFmpeg integration
  - **P1**: Capability detection (video streams, snapshot support)
- **Substep 1.3.1.4**: Camera management service
  - **Status**: ✅ DONE
  - **P0**: Camera registration and storage (SQLite)
  - **P0**: Unified camera interface for both network (RTSP/ONVIF) and USB cameras
  - **P0**: Basic camera configuration management
  - **P0**: Support for 1-2 cameras (PoC scope)
  - **P0**: Basic camera status monitoring
- **Substep 1.3.1.5**: Unit tests for camera discovery and management
  - **Status**: ✅ DONE
  - **P0**: Test RTSP client connection and reconnection ✅
  - **P0**: Test ONVIF discovery (mock WS-Discovery responses) ✅
  - **P0**: Test USB camera detection (mock V4L2 devices) ✅
  - **P0**: Test camera registration and storage ✅
  - **P0**: Test camera status monitoring ✅
  - **P0**: Test unified camera interface (network and USB) ✅
  - **P1**: Test camera configuration updates ✅
  - **P1**: Test camera enable/disable operations ✅

### Step 1.3.2: Video Decoding with FFmpeg
- **Substep 1.3.2.1**: FFmpeg integration
  - **Status**: ✅ DONE
  - Go wrapper for FFmpeg (exec-based, can be replaced with CGO bindings later) ✅
  - Hardware acceleration detection (Intel QSV via VAAPI) ✅
  - Software fallback implementation ✅
  - Codec detection and selection ✅
- **Substep 1.3.2.2**: Frame extraction pipeline
  - **Status**: ✅ DONE
  - Extract frames at configurable intervals ✅
  - Frame buffer management ✅
  - Frame preprocessing (resize, normalize) ✅
  - Frame distribution to AI service ✅
- **Substep 1.3.2.3**: Video clip recording
  - **Status**: ✅ DONE
  - Start/stop recording on events ✅
  - MP4 encoding with H.264 ✅
  - Clip metadata generation (duration, size, camera) ✅
  - Concurrent recording for multiple cameras ✅
- **Substep 1.3.2.4**: Unit tests for video decoding and recording
  - **Status**: ✅ DONE
  - **P0**: Test FFmpeg wrapper initialization ✅
  - **P0**: Test frame extraction pipeline ✅
  - **P0**: Test video clip recording start/stop ✅
  - **P0**: Test clip metadata generation ✅
  - **P1**: Test hardware acceleration detection ✅
  - **P1**: Test concurrent recording for multiple cameras ✅
  - **P2**: Test codec detection and selection ✅

### Step 1.3.3: Local Storage Management
- **Substep 1.3.3.1**: Clip storage service
  - **Status**: ✅ DONE
  - **P0**: File system organization (date/camera structure) ✅
  - **P0**: Clip naming convention ✅
  - **P0**: Basic disk space monitoring ✅
  - **P1**: Advanced storage quota management
- **Substep 1.3.3.2**: Retention policy enforcement
  - **Status**: ✅ DONE
  - **P0**: Simple "delete oldest when disk > X% full" rule ✅
  - **P0**: Basic retention (e.g., 7 days default) ✅
  - **P1**: Configurable retention periods and thresholds ✅
  - **P1**: Advanced backpressure handling (pause recording when disk full) ✅
- **Substep 1.3.3.3**: Snapshot generation
  - **Status**: ✅ DONE
  - **P1**: JPEG snapshot capture on events ✅
  - **P1**: Thumbnail generation ✅
  - **P1**: Snapshot storage management ✅
  - **P2**: Snapshot cleanup automation
- **Substep 1.3.3.4**: Unit tests for local storage management
  - **Status**: ✅ DONE
  - **P0**: Test clip storage service (file organization, naming) ✅
  - **P0**: Test retention policy enforcement ✅
  - **P0**: Test disk space monitoring ✅
  - **P1**: Test snapshot generation and storage
  - **P1**: Test storage quota management
  - **P2**: Test snapshot cleanup automation

---

## Epic 1.4: Python AI Inference Service

**Priority: P0**

### Step 1.4.1: AI Service Framework
- **Substep 1.4.1.1**: Python service structure
  - **Status**: ✅ DONE
  - FastAPI service for HTTP/gRPC ✅
  - Service initialization ✅
  - Health check endpoints ✅
  - Logging setup ✅
- **Substep 1.4.1.2**: OpenVINO installation and setup
  - **Status**: ✅ DONE
  - Install OpenVINO toolkit ✅
  - Hardware detection (CPU/iGPU) ✅
  - Model conversion tools setup ✅
  - OpenVINO runtime configuration ✅
- **Substep 1.4.1.3**: Unit tests for AI service framework
  - **Status**: ✅ DONE
  - **P0**: Test FastAPI service initialization and startup
  - **P0**: Test health check endpoints (liveness, readiness, detailed)
  - **P0**: Test logging setup (JSON and text formats)
  - **P0**: Test configuration loading and validation
  - **P0**: Test OpenVINO runtime initialization and hardware detection
  - **P0**: Test model conversion utilities (ONNX to IR)
  - **P1**: Test graceful shutdown handling
  - **P1**: Test error handling when OpenVINO is not available

### Step 1.4.2: Model Management
- **Substep 1.4.2.1**: Model loader service
  - **Status**: ✅ DONE
  - Model loading from filesystem ✅
  - Model versioning ✅
  - Model hot-reload capability ✅
  - Model validation ✅
- **Substep 1.4.2.2**: YOLOv8 model integration
  - **Status**: ✅ DONE
  - Download pre-trained YOLOv8 model ✅
  - Convert to ONNX format ✅
  - Convert to OpenVINO IR ✅
  - Model optimization for target hardware ✅
- **Substep 1.4.2.3**: Unit tests for model management
  - **Status**: ✅ DONE
  - **P0**: Test model loading from filesystem
  - **P0**: Test model validation (file existence, format, compatibility)
  - **P0**: Test model versioning and version tracking
  - **P0**: Test model hot-reload capability
  - **P0**: Test YOLOv8 model integration (ONNX conversion, IR conversion)
  - **P1**: Test model optimization for different hardware targets
  - **P1**: Test error handling for invalid or missing models

### Step 1.4.3: Inference Pipeline
- **Substep 1.4.3.1**: Inference service implementation
  - **Status**: ✅ DONE
  - Frame preprocessing for YOLO (resize, normalize) ✅
  - Inference execution with OpenVINO ✅
  - Post-processing (NMS, confidence filtering) ✅
  - Bounding box extraction ✅
- **Substep 1.4.3.2**: Detection logic
  - **Status**: ✅ DONE
  - Person detection ✅
  - Vehicle detection ✅
  - Custom detection classes ✅
  - Detection threshold configuration ✅
- **Substep 1.4.3.3**: gRPC/HTTP API for inference
  - **Status**: ✅ DONE
  - Inference request handling ✅
  - Response formatting ✅
  - Error handling ✅
  - Performance metrics ✅
- **Substep 1.4.3.4**: Unit tests for AI inference service
  - **Status**: ✅ DONE
  - **P0**: Test inference pipeline (preprocessing, inference, post-processing)
  - **P0**: Test frame preprocessing for YOLO (resize, normalize)
  - **P0**: Test inference execution with OpenVINO (mock model)
  - **P0**: Test post-processing (NMS, confidence filtering, bounding box extraction)
  - **P0**: Test detection logic (person, vehicle detection)
  - **P0**: Test detection threshold configuration
  - **P0**: Test gRPC/HTTP API endpoints (request handling, response formatting)
  - **P0**: Test error handling (invalid inputs, model errors, timeout)
  - **P1**: Test inference performance metrics
  - **P1**: Test batch inference processing
  - **P2**: Test custom detection classes

### Step 1.4.4: Integration Testing
- **Substep 1.4.4.1**: Integration tests for AI service
  - **Status**: ✅ DONE
  - **P0**: Test end-to-end inference flow (frame input → detection output)
  - **P0**: Test AI service integration with Edge Orchestrator (HTTP/gRPC)
  - **P0**: Test model loading and inference with real OpenVINO runtime
  - **P0**: Test hardware acceleration (CPU and GPU if available)
  - **P0**: Test concurrent inference requests
  - **P0**: Test service health and readiness with loaded model
  - **P1**: Test model hot-reload during service operation
  - **P1**: Test error recovery (model reload after failure)
  - **P2**: Test performance under load (multiple concurrent requests)

---

## Epic 1.5: Event Management & Queue

**Priority: P0**

**Implementation Location**: `edge/orchestrator/internal/events/` (Go)

**Note**: This epic integrates AI detection results from the Python service with event generation, storage, and queueing in the Go orchestrator. The AI service client (`internal/ai/client.go`) should be implemented first to connect to the Python service.

### Step 1.5.0: AI Service Client (Prerequisite)
- **Substep 1.5.0.1**: AI service HTTP client
  - **Status**: ✅ DONE
  - HTTP client for Python AI service (`internal/ai/client.go`) ✅
  - Request/response types matching Python API ✅
  - Frame encoding (JPEG → base64) ✅
  - Error handling and retries ✅
  - Integration with frame distributor ✅
  - Frame processor for rate limiting ✅
- **Substep 1.5.0.2**: AI service configuration
  - **Status**: ✅ DONE
  - AI service URL configuration ✅
  - Inference interval configuration ✅
  - Confidence threshold configuration ✅
  - Enabled classes configuration ✅
  - Environment variable support ✅

### Step 1.5.1: Event Detection & Generation
- **Substep 1.5.1.1**: Event structure definition
  - **Status**: ✅ DONE
  - Event schema (timestamp, camera, type, confidence, bounding boxes) ✅
  - Event ID generation (UUID) ✅
  - Event state management ✅
  - Location: `internal/events/event.go` ✅
- **Substep 1.5.1.2**: Event creation service
  - **Status**: ✅ DONE
  - Trigger on AI detection results ✅
  - Convert AI detections to events ✅
  - Associate clips and snapshots with events ✅
  - Generate event metadata JSON ✅
  - Event deduplication logic ✅
  - Location: `internal/events/generator.go` ✅
- **Substep 1.5.1.3**: Event storage
  - **Status**: ✅ DONE
  - Store events in SQLite (use existing `state.Manager`) ✅
  - Event querying ✅
  - Event expiration ✅
  - Location: `internal/events/storage.go` ✅

### Step 1.5.2: Event Queue Management
- **Substep 1.5.2.1**: Local event queue
  - **Status**: ✅ DONE
  - Queue implementation (in-memory + SQLite persistence via `state.Manager`) ✅
  - Queue priority handling ✅
  - Queue size limits ✅
  - Location: `internal/events/queue.go` ✅
- **Substep 1.5.2.2**: Transmission logic
  - **Status**: ✅ DONE
  - Queue processing service ✅
  - Retry logic for failed transmissions ✅
  - Queue persistence on restart (uses existing `state.Manager.GetPendingEvents`) ✅
  - Queue recovery ✅
  - Location: `internal/events/transmitter.go` (will integrate with Epic 1.6 gRPC client) ✅
- **Substep 1.5.2.3**: Unit tests for event management and queue
  - **Status**: ✅ DONE
  - **P0**: Test event structure and ID generation ✅
  - **P0**: Test event creation and storage ✅
  - **P0**: Test event queue operations (enqueue, dequeue, priority) ✅
  - **P0**: Test queue persistence and recovery ✅
  - **P0**: Test retry logic for failed transmissions ✅
  - **P1**: Test event deduplication logic ✅

---

## Epic 1.6: WireGuard Communication (PoC: Direct, MVP/Production: libp2p + WireGuard)

**Priority: P0**

**Note**: 
- **PoC**: libp2p is **NOT implemented for PoC**. Edge connects to User VM's WireGuard endpoint directly (port-forward or local network). This simplifies PoC and focuses on core functionality.
- **MVP/Production**: This epic combines **libp2p** (for peer discovery and NAT traversal) with **WireGuard** (for efficient data transfer). libp2p handles NAT traversal automatically, allowing Edge Appliances behind WiFi router NAT to connect to User VMs.

**Architecture (MVP/Production)**:
1. **Mesh Abstraction Layer**: Abstract libp2p behind a `Mesh` interface to keep domain logic decoupled
2. Edge uses libp2p (via Mesh) to discover available User VMs
3. libp2p establishes connection (handles NAT traversal automatically)
4. Edge and User VM exchange WireGuard keys over libp2p secure channel
5. WireGuard tunnel is established for efficient data transfer
6. libp2p connection maintained for health checks and re-establishment

**Implementation Progression**:
- **PoC (Minimal)**: Direct WireGuard connection (no libp2p). Edge connects to User VM's WireGuard server endpoint directly (port-forward or local network). Mesh interface may be stubbed for future use, but no implementation.
- **MVP (Basic)**: Static bootstrap (User VM address hard-coded), simple protocol `/guardian/control/1.0.0`, no DHT/relays/pubsub. Mesh interface implemented with libp2p.
- **Production (Full)**: Static peer list (multiple VMs), one pubsub topic `/guardian/events`, DHT-based discovery, multiple protocols, relays, multi-version support

**High Availability**: 
- **Production**: Edge discovers multiple User VMs via DHT, supports load balancing/failover
- **MVP**: Static list of multiple User VMs (HA support)
- **PoC**: Single User VM, direct WireGuard connection (no libp2p)

### Step 1.6.1: WireGuard Client (PoC - Direct Connection)
- **Substep 1.6.1.1**: Direct WireGuard connection (PoC)
  - **Status**: ✅ DONE (needs update for PoC simplification)
  - **P0**: **PoC**: Connect directly to User VM's WireGuard server endpoint (hard-coded in config)
  - **P0**: **PoC**: No libp2p, no Mesh interface - just direct WireGuard connection
  - **P0**: WireGuard client configuration from config file
  - **P0**: Connection to User VM WireGuard server (port-forward or local network)
  - **P0**: Tunnel health monitoring ✅
  - **P0**: Automatic reconnection logic ✅
  - **P1**: **MVP**: Mesh interface stub (for future libp2p integration)
  - Location: `internal/wireguard/client.go` (existing, needs PoC simplification note)
- **Substep 1.6.1.2**: User VM connection (PoC - Direct WireGuard)
  - **Status**: ✅ DONE (needs update for PoC simplification)
  - **P0**: **PoC**: Connect directly to User VM's WireGuard server (hard-coded endpoint in config)
  - **P0**: **PoC**: No discovery needed - just direct connection
  - **P0**: Connection health monitoring ✅
  - **P0**: Automatic reconnection logic ✅
  - **P1**: **MVP**: Use Mesh interface for discovery (deferred)
  - **P1**: **MVP**: Connect to static list of multiple User VMs (HA support)
  - **P1**: **MVP**: Load balancing between multiple discovered VMs
  - **P1**: **MVP**: Failover to backup VM if primary fails
  - **P2**: **Production**: Discover User VMs via libp2p DHT
  - Location: `internal/wireguard/client.go` (existing, PoC uses direct connection)
- **Substep 1.6.1.3**: WireGuard key exchange (PoC - Direct)
  - **Status**: ✅ DONE (needs update for PoC simplification)
  - **P0**: **PoC**: WireGuard keys configured in config file (no key exchange needed)
  - **P0**: **PoC**: Direct WireGuard connection - keys pre-configured
  - **P0**: Validate connection (tunnel health monitoring) ✅
  - **P1**: **MVP**: Use Mesh `OpenStream` to establish secure channel to User VM
  - **P1**: **MVP**: libp2p implementation uses built-in encryption (TLS or Noise)
  - **P1**: **MVP**: Exchange WireGuard keys over secure stream (via Mesh interface)
  - **P1**: **MVP**: Simple protocol `/guardian/control/1.0.0` for key exchange
  - **P1**: Certificate pinning for additional security
  - Location: `internal/wireguard/client.go` (PoC uses pre-configured keys)

### Step 1.6.4: WireGuard Client Service (PoC - Direct, MVP/Production - After Mesh Connection)
- **Substep 1.6.2.1**: WireGuard client implementation
  - **Status**: ✅ DONE (needs update for libp2p integration)
  - Go WireGuard client using `wg-quick` command (PoC approach) ✅
  - **P0**: **PoC**: WireGuard keys pre-configured in config file (no key exchange)
  - **P1**: **MVP**: Receive WireGuard keys from User VM over Mesh secure channel
  - **P0**: Configure WireGuard client with exchanged keys
  - **P0**: **PoC**: Connection to User VM WireGuard server (direct connection) ✅
  - **P1**: **MVP**: Connection to User VM (after Mesh connection established) ⬜ TODO
  - Configuration management ✅
  - **P0**: **PoC**: Key management (pre-configured in config file) ✅
  - **P1**: **MVP**: Key management (received via Mesh, not from config file) ⬜ TODO
  - Location: `internal/wireguard/client.go` (needs update)
- **Substep 1.6.4.2**: Tunnel management
  - **Status**: ✅ DONE
  - Tunnel health monitoring ✅
  - Automatic reconnection logic ✅
  - Connection state management ✅
  - Latency tracking ✅

### Step 1.6.5: gRPC Communication (Over WireGuard Tunnel)
- **Substep 1.6.2.1**: Proto definitions
  - **Status**: ✅ DONE
  - Proto definitions created in `proto/proto/edge/` directory ✅
  - Edge ↔ KVM VM proto files (events, control, telemetry, streaming) ✅
  - Makefile for generating Go stubs ✅
  - Import proto stubs from `proto/go` as Go module dependency ✅
  - Note: Requires `protoc` to generate stubs (documented in proto/README.md) ✅
- **Substep 1.6.2.2**: gRPC client implementation
  - **Status**: ✅ DONE
  - gRPC client setup using proto stubs from `proto/go` ✅
  - Event transmission over WireGuard tunnel ✅
  - Acknowledge receipt handling ✅
  - Error handling and retries ✅
  - Event sender for converting internal events to proto ✅
  - Fully functional with generated proto stubs ✅
  - Location: `internal/grpc/client.go`, `internal/grpc/event_sender.go` ✅

### Step 1.6.6: Event Transmission (Optional: Via Mesh Pubsub for MVP)
- **Substep 1.6.5.0**: Event pubsub via Mesh (MVP enhancement)
  - **Status**: ⬜ TODO
  - **P1**: **MVP**: Use Mesh `Publish` for async event notifications
  - **P1**: **MVP**: Subscribe to `/guardian/events` topic on User VM
  - **P1**: **MVP**: Keep gRPC for synchronous commands (request/response)
  - **P2**: **Production**: Full pubsub for event fanout, multi-version protocols
  - Location: `internal/events/mesh_publisher.go` (uses Mesh interface)

### Step 1.6.7: Event Transmission (Current: gRPC)
- **Substep 1.6.3.1**: Event sender service
  - **Status**: ✅ DONE
  - Event transmitter integrated with gRPC client ✅
  - Send event metadata over WireGuard/gRPC ✅
  - Handle transmission failures (retryable error detection) ✅
  - Transmission status tracking ✅
  - Location: `internal/grpc/integration.go` ✅
- **Substep 1.6.3.2**: Clip streaming (on-demand)
  - **Status**: ✅ DONE
  - Stream clip on request from KVM VM ✅
  - Handle stream interruptions ✅
  - Stream metadata transmission ✅
  - Location: `internal/grpc/streaming.go` ✅
- **Substep 1.6.3.3**: Unit tests for WireGuard client and communication
  - **Status**: ✅ DONE
  - **P0**: Test WireGuard client connection and configuration ✅
  - **P0**: Test tunnel health monitoring and reconnection ✅
  - **P0**: Test gRPC client setup and proto stub usage ✅
  - **P0**: Test event transmission over WireGuard/gRPC ✅
  - **P1**: Test clip streaming (on-demand) ✅
  - **P1**: Test error handling and retries ✅
  - Location: `internal/grpc/client_test.go`, `internal/grpc/integration_test.go`, `internal/grpc/streaming_test.go` ✅

---

## Epic 1.7: Telemetry & Health Reporting

**Priority: P0** (Basic telemetry only for PoC)

### Step 1.7.1: Telemetry Collection
- **Substep 1.7.1.1**: System metrics collection
  - **Status**: ✅ DONE
  - CPU utilization monitoring ✅
  - Memory usage tracking ✅
  - Disk usage monitoring ✅
  - Network statistics (deferred - basic implementation complete)
  - Location: `internal/telemetry/collector.go` ✅
- **Substep 1.7.1.2**: Application metrics
  - **Status**: ✅ DONE
  - Camera status (online/offline) ✅
  - Event queue length ✅
  - AI inference performance (placeholder) ✅
  - Storage usage per camera ✅
  - Location: `internal/telemetry/collector.go` ✅
- **Substep 1.7.1.3**: Health status aggregation
  - **Status**: ✅ DONE
  - Heartbeat generation with timestamp and edge ID ✅
  - Basic health status (healthy/warning/critical) ✅
  - Location: `internal/telemetry/sender.go` (heartbeat loop) ✅

### Step 1.7.2: Health Reporting
- **Substep 1.7.2.1**: Periodic heartbeat
  - **Status**: ✅ DONE
  - Send heartbeat to KVM VM via gRPC ✅
  - Heartbeat interval configuration ✅
  - Heartbeat failure handling ✅
  - Location: `internal/telemetry/sender.go`, `internal/grpc/telemetry_sender.go` ✅
- **Substep 1.7.2.2**: Telemetry transmission
  - **Status**: ✅ DONE
  - Send telemetry data to KVM VM via gRPC ✅
  - Telemetry collection (system and application metrics) ✅
  - Telemetry batching (configurable interval) ✅
  - Location: `internal/telemetry/sender.go`, `internal/telemetry/collector.go`, `internal/grpc/telemetry_sender.go` ✅
- **Substep 1.7.2.3**: Unit tests for telemetry and health reporting
  - **Status**: ✅ DONE
  - **P0**: Test system metrics collection (CPU, memory, disk, network) ✅
  - **P0**: Test application metrics (camera status, event queue length) ✅
  - **P0**: Test health status aggregation ✅
  - **P0**: Test periodic heartbeat transmission ✅
  - **P1**: Test telemetry batching and persistence ✅
  - Location: `internal/telemetry/collector_test.go`, `internal/telemetry/sender_test.go` ✅

---

## Epic 1.8: Encryption & Archive Client (Basic)

**Priority: P1** (Can be simplified for PoC)

### Step 1.8.1: Encryption Service
- **Substep 1.8.1.1**: Clip encryption implementation
  - **Status**: ✅ DONE
  - **P0**: Use encryption library from meta repo `crypto/go/` ✅
  - **P0**: AES-256-GCM encryption (via crypto library) ✅
  - **P0**: Argon2id key derivation from user secret (via crypto library) ✅
  - **P1**: Encryption metadata generation ✅
  - Location: `crypto/go/encryption/encryption.go`, `crypto/go/keyderivation/keyderivation.go`, `internal/encryption/service.go` ✅
- **Substep 1.8.1.2**: Key management
  - **Status**: ✅ DONE
  - **P0**: User secret handling (never transmitted) ✅
  - **P0**: Key derivation logic (via crypto library) ✅
  - **P0**: Key storage (local only) ✅
  - Import `crypto/go` as Go module dependency ✅
  - Location: `internal/encryption/service.go` ✅
- **Substep 1.8.1.3**: Archive queue (basic)
  - **Status**: ⬜ TODO
  - **P1**: Encrypted clip queue
  - **P1**: Basic transmission to KVM VM
  - **P2**: Advanced queue management
- **Substep 1.8.1.4**: Unit tests for encryption and archive client
  - **Status**: ✅ DONE
  - **P0**: Test clip encryption using crypto library ✅
  - **P0**: Test key derivation (Argon2id) ✅
  - **P0**: Test key management (local storage, never transmitted) ✅
  - **P1**: Test encrypted clip queue (deferred - archive queue not yet implemented)
  - **P1**: Test basic transmission to KVM VM (deferred - archive queue not yet implemented)
  - Location: `internal/encryption/service_test.go` ✅

---

## Epic 1.9: Edge Web UI (Local Network Accessible)

**Priority: P0** (Essential for local management and monitoring)

**Overview**: A web-based user interface accessible on the local home network (similar to router admin UI) that allows users to monitor, configure, and manage the Edge Appliance directly without requiring the SaaS control plane. This UI runs locally on the Edge Appliance and is accessible via HTTP on the local network.

**Key Features**:
- Live camera feed viewing (MJPEG/JPEG streaming)
- Event timeline and viewer
- System configuration (camera settings, AI thresholds, storage, WireGuard)
- Camera management (add/remove, status, discovery)
- System status dashboard (health, metrics, telemetry)
- Clip and snapshot viewing/download
- Settings management (encryption, storage retention, logging)

**Structure**: This epic is organized into **Backend API** steps (1.9.1-1.9.6) and **Frontend UI** steps (1.9.7-1.9.12), followed by integration and testing (1.9.13).

---

### Backend API Implementation

#### Step 1.9.1: Web Server & API Foundation
- **Substep 1.9.1.1**: HTTP server setup
  - **Status**: ✅ DONE
  - **P0**: Embedded HTTP server (using Go `net/http` or `gin`/`echo`) ✅
  - **P0**: Serve static frontend assets (HTML, CSS, JS) ✅
  - **P0**: REST API endpoints for backend communication ✅
  - **P0**: CORS configuration (if needed for local network access) ✅
  - **P0**: Basic authentication (simple password or token-based) ⬜ TODO (deferred - not needed for PoC)
  - **P1**: HTTPS support (self-signed cert for local network) ⬜ TODO (deferred - not needed for PoC)
  - Location: `internal/web/server.go`, `internal/web/handlers.go` ✅
- **Substep 1.9.1.2**: API endpoints structure
  - **Status**: ✅ DONE
  - **P0**: Health check endpoint (`/api/health`) ✅
  - **P0**: System status endpoint (`/api/status`) ✅
  - **P0**: Camera endpoints (`/api/cameras`, `/api/cameras/:id`) ✅ (placeholder - will be implemented in Step 1.9.5)
  - **P0**: Event endpoints (`/api/events`, `/api/events/:id`) ✅ (placeholder - will be implemented in Step 1.9.3)
  - **P0**: Configuration endpoints (`/api/config`, `/api/config/:section`) ✅ (placeholder - will be implemented in Step 1.9.4)
  - **P1**: Metrics/telemetry endpoint (`/api/metrics`) ✅ (placeholder - will be implemented in Step 1.9.6)
  - Location: `internal/web/handlers.go` ✅
- **Substep 1.9.1.3**: Unit tests for web server
  - **Status**: ✅ DONE
  - **P0**: Test HTTP server startup and shutdown ✅
  - **P0**: Test API endpoint routing ✅
  - **P0**: Test static file serving ✅
  - **P1**: Test authentication middleware ⬜ TODO (deferred - authentication not yet implemented)
  - Location: `internal/web/server_test.go` ✅

#### Step 1.9.2: Camera Streaming API
- **Substep 1.9.2.1**: MJPEG/JPEG streaming endpoints
  - **Status**: ✅ DONE
  - **P0**: MJPEG stream endpoint (`/api/cameras/:id/stream`) ✅
  - **P0**: Single frame JPEG endpoint (`/api/cameras/:id/frame`) ✅
  - **P0**: Frame extraction from camera feed (using FFmpeg directly) ✅
  - **P0**: Stream management (start/stop, connection handling) ✅
  - **P1**: Multi-camera stream support (grid view) ⬜ TODO (deferred - can be added later)
  - **P1**: Stream quality/bitrate configuration ⬜ TODO (deferred - using default quality for now)
  - Location: `internal/web/handlers.go`, `internal/web/streaming/service.go` ✅
- **Substep 1.9.2.2**: Unit tests for streaming API
  - **Status**: ✅ DONE
  - **P0**: Test MJPEG stream generation ✅
  - **P0**: Test frame extraction and serving ✅
  - **P0**: Test stream connection handling ✅
  - **P1**: Test multi-camera streaming ⬜ TODO (deferred - not yet implemented)
  - Location: `internal/web/streaming/service_test.go` ✅

#### Step 1.9.3: Event API
- **Substep 1.9.3.1**: Event API endpoints
  - **Status**: ✅ DONE
  - **P0**: List events endpoint (`/api/events` with pagination, filtering) ✅
  - **P0**: Get event details endpoint (`/api/events/:id`) ✅
  - **P0**: Event filtering (camera, type, date range) ✅
  - **P0**: Event metadata (detection classes, confidence, timestamps) ✅
  - **P1**: Event search functionality ⬜ TODO (deferred - can be added later)
  - Location: `internal/web/handlers.go` ✅
- **Substep 1.9.3.2**: Clip and snapshot API endpoints
  - **Status**: ✅ DONE
  - **P0**: Clip playback endpoint (`/api/clips/:id/play`) ✅
  - **P0**: Snapshot viewing endpoint (`/api/snapshots/:id`) ✅
  - **P0**: Clip download endpoint (`/api/clips/:id/download`) ✅
  - **P1**: Clip timeline scrubbing support ⬜ TODO (deferred - not yet implemented)
  - Location: `internal/web/handlers.go` ✅
- **Substep 1.9.3.3**: Unit tests for event API
  - **Status**: ✅ DONE
  - **P0**: Test event listing and pagination ✅
  - **P0**: Test event filtering ✅
  - **P0**: Test event detail retrieval ✅
  - **P0**: Test clip/snapshot serving ✅
  - **P1**: Test event search ⬜ TODO (deferred - search not yet implemented)
  - Location: `internal/web/handlers_test.go` ✅

#### Step 1.9.4: Configuration API
- **Substep 1.9.4.1**: Configuration API endpoints
  - **Status**: ✅ DONE
  - **P0**: Get configuration endpoint (`/api/config`) ✅
  - **P0**: Update configuration endpoint (`/api/config`, `PUT`) ✅
  - **P0**: Configuration validation ✅
  - **P0**: Configuration sections (camera, AI, storage, WireGuard, telemetry, encryption) ✅
  - **P1**: Configuration export/import ⬜ TODO (deferred - can be added later)
  - Location: `internal/web/handlers.go` ✅
- **Substep 1.9.4.2**: Unit tests for configuration API
  - **Status**: ✅ DONE
  - **P0**: Test configuration retrieval ✅
  - **P0**: Test configuration updates ✅
  - **P0**: Test configuration validation ✅
  - **P1**: Test configuration export/import ⬜ TODO (deferred - export/import not yet implemented)
  - Location: `internal/web/config_handlers_test.go` ✅

#### Step 1.9.5: Camera Management API
- **Substep 1.9.5.1**: Camera management API endpoints
  - **Status**: ✅ DONE
  - **P0**: List cameras endpoint (`/api/cameras`) ✅
  - **P0**: Get camera details endpoint (`/api/cameras/:id`) ✅
  - **P0**: Add camera endpoint (`/api/cameras`, `POST`) ✅
  - **P0**: Update camera endpoint (`/api/cameras/:id`, `PUT`) ✅
  - **P0**: Remove camera endpoint (`/api/cameras/:id`, `DELETE`) ✅
  - **P0**: Camera discovery endpoint (`/api/cameras/discover`) ✅
  - **P1**: Camera test connection endpoint (`/api/cameras/:id/test`) ✅
  - Location: `internal/web/handlers.go` ✅
- **Substep 1.9.5.2**: Unit tests for camera management API
  - **Status**: ✅ DONE
  - **P0**: Test camera listing ✅
  - **P0**: Test camera add/update/remove ✅
  - **P0**: Test camera discovery ✅
  - **P1**: Test camera connection testing ✅
  - Location: `internal/web/camera_handlers_test.go` ✅

#### Step 1.9.6: Status & Metrics API
- **Substep 1.9.6.1**: Status and metrics API endpoints
  - **Status**: ✅ COMPLETE
  - **P0**: System status endpoint (`/api/status`) - health, uptime, version ✅
  - **P0**: System metrics endpoint (`/api/metrics`) - CPU, memory, disk, network ✅
  - **P0**: Application metrics endpoint (`/api/metrics/app`) - camera count, event queue length, AI inference stats ✅
  - **P0**: Telemetry data endpoint (`/api/telemetry`) - recent telemetry snapshots ✅
  - **P1**: Historical metrics endpoint (`/api/metrics/history`) - time-series data ⬜ TODO (deferred)
  - Location: `internal/web/handlers.go`, `internal/web/server.go`
- **Substep 1.9.6.2**: Unit tests for status API
  - **Status**: ✅ COMPLETE
  - **P0**: Test system status retrieval ✅
  - **P0**: Test metrics retrieval ✅
  - **P0**: Test telemetry data retrieval ✅
  - **P1**: Test historical metrics ⬜ TODO (deferred)
  - Location: `internal/web/status_handlers_test.go`

---

### Frontend UI Implementation

#### Step 1.9.7: Frontend Framework & Build Setup

**Technology Stack Recommendation:**
- **Frontend Framework**: React 18+ with TypeScript (matches SaaS stack for consistency)
- **Build Tool**: Vite (fast, lightweight, excellent DX)
- **Styling**: Tailwind CSS (matches SaaS stack, utility-first, small bundle size)
- **State Management**: React Context API + hooks (simple, no external deps needed for PoC)
- **HTTP Client**: Fetch API or `axios` (lightweight)
- **Charts**: Chart.js or Recharts (for metrics visualization)
- **Icons**: Heroicons or Lucide React (lightweight SVG icons)
- **Embedding**: Go `embed` package (built-in, no external tools)

**Alternative (Lighter Option):**
- **Alpine.js** + **Tailwind CSS** (no build step, very lightweight, good for simple admin UIs)
- Consider this if React feels like overkill for a local admin UI

- **Substep 1.9.7.1**: Frontend build setup
  - **Status**: ✅ COMPLETE
  - **P0**: React + Vite + TypeScript project setup ✅
  - **P0**: Build configuration for production (minified, optimized) ✅
  - **P0**: Embedded static files in Go binary (using Go `embed` package) ✅
  - **P0**: Development workflow (dev server for local development, build for production) ✅
  - **P1**: Hot module replacement (HMR) for faster development ✅
  - Location: `edge/orchestrator/internal/web/frontend/` (source), `internal/web/static/` (built assets)
- **Substep 1.9.7.2**: UI components and styling
  - **Status**: ✅ COMPLETE
  - **P0**: Tailwind CSS setup and configuration ✅
  - **P0**: Basic responsive layout (mobile-friendly) ✅
  - **P0**: Navigation sidebar/header component ✅
  - **P0**: Form components (inputs, selects, buttons) ✅
  - **P0**: Card components (event cards, camera cards, metric cards) ✅
  - **P0**: Icon library integration (Lucide React) ✅
  - **P1**: Chart components (Recharts for metrics) ✅ (Recharts installed, ready for use)
  - **P1**: Loading states and error boundaries ✅
  - Location: `edge/orchestrator/internal/web/frontend/src/components/`, `edge/orchestrator/internal/web/frontend/src/styles/`

#### Step 1.9.8: Camera Viewer UI
- **Substep 1.9.8.1**: Camera viewer component
  - **Status**: ✅ COMPLETE
  - **P0**: HTML5 `<img>` tag with MJPEG stream URL ✅
  - **P0**: Camera selection dropdown/list ✅
  - **P0**: Play/pause controls ✅
  - **P1**: Multi-camera grid layout ✅
  - **P1**: Fullscreen mode ✅
  - **P2**: WebRTC streaming (post-PoC) ⬜ TODO (deferred)
  - Location: `edge/orchestrator/internal/web/frontend/src/components/CameraViewer.tsx`, `edge/orchestrator/internal/web/frontend/src/components/CameraGrid.tsx`, `edge/orchestrator/internal/web/frontend/src/pages/Cameras.tsx`

#### Step 1.9.9: Event Timeline UI
- **Substep 1.9.9.1**: Event timeline component
  - **Status**: ✅ COMPLETE
  - **P0**: Event list/timeline view ✅
  - **P0**: Event cards with metadata (timestamp, camera, detection type) ✅
  - **P0**: Event detail modal/page ✅
  - **P0**: Pagination or infinite scroll ✅
  - **P1**: Date grouping and filtering UI ✅
  - **P1**: Event thumbnail display (snapshots) ✅
  - Location: `edge/orchestrator/internal/web/frontend/src/components/EventCard.tsx`, `edge/orchestrator/internal/web/frontend/src/components/EventTimeline.tsx`, `edge/orchestrator/internal/web/frontend/src/components/EventDetailModal.tsx`
- **Substep 1.9.9.2**: Clip and snapshot viewer
  - **Status**: ✅ COMPLETE
  - **P0**: Frontend video player (HTML5 `<video>` tag) ✅
  - **P0**: Snapshot gallery view ✅
  - **P1**: Clip timeline scrubbing ⬜ TODO (deferred - HTML5 video controls provide basic scrubbing)
  - Location: `edge/orchestrator/internal/web/frontend/src/components/ClipViewer.tsx`, `edge/orchestrator/internal/web/frontend/src/components/EventDetailModal.tsx`

#### Step 1.9.10: Configuration UI
- **Substep 1.9.10.1**: Configuration forms
  - **Status**: ✅ COMPLETE
  - **P0**: Camera configuration form (discovery, RTSP settings) ✅
  - **P0**: AI configuration form (service URL, confidence thresholds, detection classes) ✅
  - **P0**: Storage configuration form (retention policies, clip storage paths) ✅
  - **P0**: WireGuard configuration form (enabled/disabled, endpoint, config path) ✅
  - **P0**: Telemetry configuration form (enabled/disabled, interval) ✅
  - **P1**: Encryption configuration form (enabled, salt, salt path) ✅ (Note: User secret cannot be updated via API for security)
  - **P1**: Configuration validation and error display ✅
  - Location: `edge/orchestrator/internal/web/frontend/src/components/ConfigForm.tsx`, `edge/orchestrator/internal/web/frontend/src/pages/Configuration.tsx`

#### Step 1.9.11: Camera Management UI
- **Substep 1.9.11.1**: Camera management interface
  - **Status**: ✅ COMPLETE
  - **P0**: Camera list view with status indicators ✅
  - **P0**: Add camera form (RTSP URL, ONVIF settings, USB device selection) ✅
  - **P0**: Camera edit form ✅
  - **P0**: Camera discovery UI (scan for RTSP/ONVIF/USB cameras) ✅
  - **P0**: Camera status display (online/offline, last seen) ✅
  - **P1**: Camera preview/test connection ✅
  - Location: `edge/orchestrator/internal/web/frontend/src/components/CameraList.tsx`, `edge/orchestrator/internal/web/frontend/src/components/CameraForm.tsx`, `edge/orchestrator/internal/web/frontend/src/components/CameraDiscovery.tsx`, `edge/orchestrator/internal/web/frontend/src/pages/CameraManagement.tsx`

#### Step 1.9.12: Dashboard UI
- **Substep 1.9.12.1**: System status dashboard
  - **Status**: ✅ COMPLETE
  - **P0**: Dashboard layout (header, sidebar navigation, main content) ✅
  - **P0**: System health overview (status indicators, uptime, version) ✅
  - **P0**: System metrics display (CPU, memory, disk usage - simple text/gauge) ✅
  - **P0**: Application metrics display (camera count, event queue, AI stats) ✅
  - **P0**: Navigation menu (Dashboard, Cameras, Events, Configuration, Settings) ✅
  - **P1**: Metric charts (simple line/bar charts using Recharts) ✅
  - **P1**: Real-time updates (polling every 30 seconds with manual refresh) ✅
  - Location: `edge/orchestrator/internal/web/frontend/src/pages/Dashboard.tsx`, `edge/orchestrator/internal/web/frontend/src/components/MetricCard.tsx`, `edge/orchestrator/internal/web/frontend/src/components/MetricChart.tsx`

---

### Integration & Testing

#### Step 1.9.13: Integration & Testing
- **Substep 1.9.13.1**: Service integration
  - **Status**: ✅ COMPLETE
  - **P0**: Register web service with orchestrator service manager ✅ (already done in Step 1.9.1)
  - **P0**: Web server startup/shutdown with orchestrator lifecycle ✅ (already done in Step 1.9.1)
  - **P0**: Configuration integration (web server port, authentication) ✅ (already done in Step 1.9.1)
  - **P0**: Dependency injection (camera manager, event queue, storage, telemetry) ✅ (wired in main.go)
  - Location: `internal/web/server.go`, `main.go`
- **Substep 1.9.13.2**: End-to-end testing
  - **Status**: ✅ COMPLETE
  - **P0**: Test web UI accessible on local network ✅ (integration tests created)
  - **P0**: Test camera feed viewing ✅ (integration tests created)
  - **P0**: Test event timeline ✅ (integration tests created)
  - **P0**: Test configuration updates ✅ (integration tests created)
  - **P0**: Test camera management ✅ (integration tests created)
  - **P1**: Test system status dashboard ✅ (integration tests created)
  - Location: `internal/web/integration_test.go` or manual testing
- **Substep 1.9.13.3**: Documentation
  - **Status**: ✅ COMPLETE
  - **P0**: Web UI access instructions (default port, local network URL) ✅
  - **P0**: API documentation (endpoint list, request/response formats) ✅
  - **P1**: User guide for web UI features ✅
  - Location: `internal/web/README.md`, `docs/EDGE_UI.md`

#### Step 1.9.14: Adaptive AI & Event Recording Pipeline
- **Substep 1.9.14.1**: Custom model training dataset workflow
  - **Status**: ✅ COMPLETE
  - **P0**: Reuse labeled screenshot capture UI to curate "normal" baseline datasets ✅
  - **P0**: Implement dataset export (ZIP + metadata manifest) for customer VM training ✅ (`/api/screenshots/export`, UI button)
  - **P1**: Support delta exports (only new screenshots since last export) ⬜ TODO
  - Location: `internal/web/frontend/src/pages/Screenshots.tsx`, `internal/web/screenshots/service.go`
- **Substep 1.9.14.2**: On-device inference and anomaly detection
  - **Status**: ✅ COMPLETE
  - **P0**: Load latest customer "normal" dataset and evaluate incoming frames per camera via local anomaly detector ✅
  - **P0**: Classify frames as `normal` vs `event` using adaptive brightness baseline ✅ (`internal/ai/local_detector.go`)
  - **P1**: Allow per-camera sensitivity/threshold overrides via configuration API/UI ⬜ TODO
  - Location: `edge/orchestrator/internal/ai`, `internal/config`, `internal/web/handlers.go`
- **Substep 1.9.14.3**: Event capture, clip recording, and forwarding
  - **Status**: ✅ COMPLETE
  - **P0**: When an event is detected, persist the triggering frame as a snapshot ✅
  - **P0**: Record a short rolling clip (pre/post buffer) to local disk via storage service ✅
  - **P0**: Enqueue clip + metadata for secure transfer to the customer VM for alerting ✅ (stored + queued via event queue)
  - **P1**: Provide retry/backoff + delivery confirmation to VM ⬜ TODO
  - Location: `internal/events`, `internal/storage`, `internal/ai/local_detector.go`, VM sync pipeline (`Phase 2`)

---

**See [Main Implementation Plan](IMPLEMENTATION_PLAN.md) for success criteria and next phases.**

