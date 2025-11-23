# Implementation Plan - PoC

This document provides a detailed, phase-by-phase implementation plan for The Private AI Guardian platform Proof of Concept (PoC), based on the architecture and technical stack defined in ARCHITECTURE.md and TECHNICAL_STACK.md.

## Overview

The PoC implementation follows a **bottom-up approach**, building from the edge inward:
1. **Edge Appliance (Mini PC)** - Go orchestrator and Python AI services
2. **KVM VM Agent (Private Cloud Node)** - Services running on per-tenant VMs
3. **SaaS Control Plane** - Backend services and VM management
4. **SaaS UI** - Frontend web application
5. **ISO Building & Deployment** - Image generation and deployment automation
6. **Integration, Testing & Polish** - End-to-end integration and refinement

Each phase builds upon the previous one, allowing for incremental validation and testing.

**Target Timeline**: 12-16 weeks for complete PoC

## Testing Strategy

**Unit Tests**: Unit test substeps are included in each implementation step. These should be written **during implementation** (not deferred) to enable regression testing as the codebase grows. Each unit test substep specifies what should be tested with priority levels (P0 = critical, P1 = important, P2 = nice-to-have).

**Integration & E2E Tests**: Phase 6 includes comprehensive integration testing, end-to-end testing, and regression test suites that verify all components work together.

**Test-Driven Development**: While not strictly required, writing unit tests alongside implementation helps catch regressions early and ensures code quality throughout development.

## Progress Tracking

**Status Legend:**
- ✅ **DONE** - Completed and tested
- ⬜ **TODO** - Not yet started
- 🚧 **IN_PROGRESS** - Currently being worked on
- ⏸️ **BLOCKED** - Waiting on dependencies or blocked by issues

**Current Progress Summary:**
- **Phase 1 (Edge Appliance)**: ~85% complete
  - ✅ Development environment setup
  - ✅ Core framework (service manager, config, state, health checks)
  - ✅ Camera discovery (RTSP, ONVIF, USB/V4L2)
  - ✅ Video processing (FFmpeg integration, frame extraction, clip recording)
  - ✅ Local storage management (clip storage, retention, snapshots)
  - ✅ Integration tests (service, config/state, storage)
  - ✅ AI inference service (Python)
  - ✅ Event management & queue
  - ✅ WireGuard client & gRPC communication
  - ✅ Event transmission (gRPC integration, clip streaming)
  - 🚧 Telemetry & health reporting (implementation complete, unit tests pending)
  - ⬜ Encryption & archive client
- **Phase 2 (KVM VM Agent)**: 0% complete
- **Phase 3 (SaaS Backend)**: 0% complete
- **Phase 4 (SaaS UI)**: 0% complete
- **Phase 5 (ISO & Deployment)**: 0% complete
- **Phase 6 (Integration & Testing)**: 0% complete

*Last Updated: 2025-11-23*

**Repository Structure Note:**
- **Public components** (Edge Appliance, Crypto libraries, Protocol definitions) are developed directly in the meta repository:
  - `edge/` - Edge Appliance software
  - `crypto/` - Encryption libraries
  - `proto/` - Protocol buffer definitions
- **Private components** (KVM VM agent, SaaS backend, SaaS frontend, Infrastructure) are in separate private repositories (git submodules):
  - `kvm-agent/` - KVM VM agent (private submodule)
  - `saas-backend/` - SaaS backend (private submodule)
  - `saas-frontend/` - SaaS frontend (private submodule)
  - `infra/` - Infrastructure (private submodule)

## Priority Tags

Epics are tagged with priority levels:
- **P0 (Core PoC)**: Must-have for PoC demonstration - essential functionality
- **P1 (Nice-to-have)**: Important but can be simplified or deferred if time is tight
- **P2 (Post-PoC)**: Full implementation deferred until after PoC validation

## Early Milestones

To avoid discovering integration issues late, we include early vertical slices:

- **Milestone 1 (End of Phase 3)**: First full event flow
  - Camera → Edge Appliance → KVM VM → SaaS → Basic event list in UI
  - Validates core data flow without streaming
  - **Target**: Week 8-9

- **Milestone 2 (End of Phase 4)**: First clip viewing
  - UI "View Clip" → SaaS → KVM VM → Edge Appliance → Stream to UI (HTTP-based)
  - Validates streaming path using simple HTTP progressive download
  - **Target**: Week 10-11

**Note**: Milestone 1 requires Phase 3 (SaaS backend + basic UI) to be complete, as it needs the full stack to demonstrate event flow.

## PoC Scope Summary

This implementation plan is scoped for a **realistic 12-16 week PoC**, not a full v1 product.

**PoC Deployment Topology**: 1 Edge Appliance → 1 KVM VM → 1 SaaS cluster, with 1-2 demo tenants. This is a single-instance PoC, not a scaled multi-tenant deployment.

Key simplifications:

### What's Included (P0 - Core PoC)
- Single Edge Appliance with 1-2 cameras
- Basic AI inference (person/vehicle detection)
- Local clip recording and storage
- Event flow: Edge → KVM VM → SaaS → UI
- Basic clip viewing (HTTP relay acceptable)
- Manual VM provisioning (1-2 pre-provisioned VMs)
- Generic ISO or simple build script
- Basic authentication (Auth0)
- Simple event timeline UI
- Essential testing (critical paths only)

### What's Deferred (P2 - Post-PoC)
- Full Stripe billing integration
- Automated Terraform VM provisioning
- Tenant-specific ISO generation
- **WebRTC implementation** (HTTP progressive download is P0 for PoC)
- **Full Filecoin integration** (S3 stub with fake CIDs is P0 for PoC)
- Advanced camera configuration (zones, schedules)
- Comprehensive test coverage (>70%)
- Full security audit
- Advanced monitoring and observability
- Update automation

### Architecture Compliance
- **Edge Appliance**: All video processing, AI inference, and local storage (Phase 1)
- **KVM VM**: Only relay, event cache, and orchestration - NO video processing or AI (Phase 2)
- **SaaS**: Control plane, event inventory, basic VM management - NO raw video (Phase 3)

---

## Table of Contents

1. [Phase 1: Edge Appliance (Mini PC) - Go & Python Apps](#phase-1-edge-appliance-mini-pc---go--python-apps)
2. [Phase 2: KVM VM Agent Services](#phase-2-kvm-vm-agent-services)
3. [Phase 3: SaaS Control Plane Backend](#phase-3-saas-control-plane-backend)
4. [Phase 4: SaaS UI Frontend](#phase-4-saas-ui-frontend)
5. [Phase 5: ISO Building & Deployment Automation](#phase-5-iso-building--deployment-automation)
6. [Phase 6: Integration, Testing & Polish](#phase-6-integration-testing--polish)

---

## Phase 1: Edge Appliance (Mini PC) - Go & Python Apps

**Duration**: 3-4 weeks  
**Goal**: Build core Edge Appliance software - Go orchestrator, Python AI service, video processing, local storage, WireGuard client

**Scope**: Single Mini PC, 1-2 cameras, basic functionality sufficient for PoC demonstration

**Status**: ~85% Complete
- ✅ Epic 1.1: Development Environment (mostly complete, CI/CD deferred)
- ✅ Epic 1.2: Go Orchestrator Core Framework (complete)
- ✅ Epic 1.3: Video Ingest & Processing (complete)
- ✅ Epic 1.4: Python AI Inference Service (complete)
- ✅ Epic 1.5: Event Management & Queue (complete)
- ✅ Epic 1.6: WireGuard Client & Communication (complete)
- 🚧 Epic 1.7: Telemetry & Health Reporting (in progress - unit tests pending)
- ⬜ Epic 1.8: Encryption & Archive Client (not started)

**Test Coverage**: 230 tests (220 unit + 10 integration), all passing ✅

### Epic 1.1: Development Environment & Project Setup

**Priority: P0**

#### Step 1.1.1: Repository & Project Structure
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

#### Step 1.1.2: Local Development Environment
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

### Epic 1.2: Go Orchestrator Service - Core Framework

**Priority: P0**

#### Step 1.2.1: Orchestrator Service Structure
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

#### Step 1.2.2: Configuration & State Management
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

### Epic 1.3: Video Ingest & Processing (Go)

**Priority: P0**

#### Step 1.3.1: Camera Discovery & Connection
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

#### Step 1.3.2: Video Decoding with FFmpeg
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

#### Step 1.3.3: Local Storage Management
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

### Epic 1.4: Python AI Inference Service

**Priority: P0**

#### Step 1.4.1: AI Service Framework
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

#### Step 1.4.2: Model Management
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

#### Step 1.4.3: Inference Pipeline
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

#### Step 1.4.4: Integration Testing
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

### Epic 1.5: Event Management & Queue

**Priority: P0**

**Implementation Location**: `edge/orchestrator/internal/events/` (Go)

**Note**: This epic integrates AI detection results from the Python service with event generation, storage, and queueing in the Go orchestrator. The AI service client (`internal/ai/client.go`) should be implemented first to connect to the Python service.

#### Step 1.5.0: AI Service Client (Prerequisite)
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

#### Step 1.5.1: Event Detection & Generation
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

#### Step 1.5.2: Event Queue Management
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

### Epic 1.6: WireGuard Client & Communication

**Priority: P0**

#### Step 1.6.1: WireGuard Client Service
- **Substep 1.6.1.1**: WireGuard client implementation
  - **Status**: ✅ DONE
  - Go WireGuard client using `wg-quick` command (PoC approach) ✅
  - Connection to KVM VM ✅
  - Configuration management ✅
  - Key management (config file template generation) ✅
  - Location: `internal/wireguard/client.go` ✅
- **Substep 1.6.1.2**: Tunnel management
  - **Status**: ✅ DONE
  - Tunnel health monitoring ✅
  - Automatic reconnection logic ✅
  - Connection state management ✅
  - Latency tracking ✅

#### Step 1.6.2: gRPC Communication
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

#### Step 1.6.3: Event Transmission
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

### Epic 1.7: Telemetry & Health Reporting

**Priority: P0** (Basic telemetry only for PoC)

#### Step 1.7.1: Telemetry Collection
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

#### Step 1.7.2: Health Reporting
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
  - **Status**: ⬜ TODO
  - **P0**: Test system metrics collection (CPU, memory, disk, network)
  - **P0**: Test application metrics (camera status, event queue length)
  - **P0**: Test health status aggregation
  - **P0**: Test periodic heartbeat transmission
  - **P1**: Test telemetry batching and persistence

### Epic 1.8: Encryption & Archive Client (Basic)

**Priority: P1** (Can be simplified for PoC)

#### Step 1.8.1: Encryption Service
- **Substep 1.8.1.1**: Clip encryption implementation
  - **Status**: ⬜ TODO
  - **P0**: Use encryption library from meta repo `crypto/go/`
  - **P0**: AES-256-GCM encryption (via crypto library)
  - **P0**: Argon2id key derivation from user secret (via crypto library)
  - **P1**: Encryption metadata generation
- **Substep 1.8.1.2**: Key management
  - **Status**: ⬜ TODO
  - **P0**: User secret handling (never transmitted)
  - **P0**: Key derivation logic (via crypto library)
  - **P0**: Key storage (local only)
  - Import `crypto/go` as Go module dependency
- **Substep 1.8.1.3**: Archive queue (basic)
  - **Status**: ⬜ TODO
  - **P1**: Encrypted clip queue
  - **P1**: Basic transmission to KVM VM
  - **P2**: Advanced queue management
- **Substep 1.8.1.4**: Unit tests for encryption and archive client
  - **Status**: ⬜ TODO
  - **P0**: Test clip encryption using crypto library
  - **P0**: Test key derivation (Argon2id)
  - **P0**: Test key management (local storage, never transmitted)
  - **P1**: Test encrypted clip queue
  - **P1**: Test basic transmission to KVM VM

---

## Phase 2: KVM VM Agent Services (Private Cloud Node)

**Duration**: 2-3 weeks  
**Goal**: Build KVM VM agent services - WireGuard server, event cache, stream relay, basic Filecoin sync

**Scope**: Single-tenant VM agent, no video processing, no AI - strictly relay and orchestration layer

**Note**: This phase contains **only** KVM VM responsibilities. Video ingest, AI inference, and clip recording are Edge Appliance responsibilities (Phase 1). The KVM VM handles **encrypted clip blobs in transit**, **event metadata**, and **telemetry** - never raw video.

**Note**: Milestone 1 (first full event flow) will be achieved at the end of Phase 3, after SaaS backend and basic UI are complete.

### Epic 2.1: KVM VM Agent Project Setup

**Priority: P0**

#### Step 2.1.1: Project Structure
- **Substep 2.1.1.1**: Create KVM VM agent directory structure
  - **Status**: ⬜ TODO
  - Note: KVM VM agent is a private repository (git submodule in meta repo)
  - `kvm-agent/` - Main agent services
  - `kvm-agent/wireguard-server/` - WireGuard server service
  - `kvm-agent/event-cache/` - Event cache service
  - `kvm-agent/stream-relay/` - Stream relay service
  - `kvm-agent/filecoin-sync/` - Filecoin sync service
  - Note: gRPC proto definitions are in meta repo `proto/proto/kvm/` (imported as Go module)
- **Substep 2.1.1.2**: Go modules setup
  - **Status**: ⬜ TODO
  - Initialize Go modules
  - Dependency management (imports `proto/go` from meta repo)
  - Shared libraries

#### Step 2.1.2: Database & Storage Setup
- **Substep 2.1.2.1**: SQLite schema design
  - **Status**: ⬜ TODO
  - Event cache table
  - CID storage table
  - Telemetry buffer table
  - Edge Appliance registry
- **Substep 2.1.2.2**: Database migration system
  - **Status**: ⬜ TODO
  - Migration tool setup
  - Initial migrations
  - Migration rollback
- **Substep 2.1.2.3**: Unit tests for KVM VM agent project setup
  - **Status**: ⬜ TODO
  - **P0**: Test database schema initialization
  - **P0**: Test database migration system
  - **P1**: Test Go module dependencies

### Epic 2.2: WireGuard Server Service

**Priority: P0**

#### Step 2.2.1: WireGuard Server Implementation
- **Substep 2.2.1.1**: WireGuard server service
  - **Status**: ⬜ TODO
  - Go service using `golang.zx2c4.com/wireguard`
  - Server configuration management
  - Server key management
- **Substep 2.2.1.2**: Client management
  - **Status**: ⬜ TODO
  - Client key generation
  - Client configuration generation
  - Client registration and storage
- **Substep 2.2.1.3**: Bootstrap process
  - **Status**: ⬜ TODO
  - Bootstrap token validation
  - Initial client registration
  - Long-lived credential issuance

#### Step 2.2.2: Tunnel Management
- **Substep 2.2.2.1**: Connection monitoring
  - **Status**: ⬜ TODO
  - Track connected Edge Appliances
  - Connection state management
  - Disconnection detection and handling
- **Substep 2.2.2.2**: Tunnel health monitoring
  - **Status**: ⬜ TODO
  - Ping/pong mechanism
  - Latency tracking
  - Bandwidth monitoring
  - Tunnel statistics collection
- **Substep 2.2.2.3**: Unit tests for WireGuard server service
  - **Status**: ⬜ TODO
  - **P0**: Test WireGuard server initialization and configuration
  - **P0**: Test client key generation and management
  - **P0**: Test bootstrap process and token validation
  - **P0**: Test connection monitoring and state management
  - **P1**: Test tunnel health monitoring (ping/pong, latency, bandwidth)

### Epic 2.3: Event Cache Service

**Priority: P0**

#### Step 2.3.1: Event Reception & Storage
- **Substep 2.3.1.1**: Event reception from Edge
  - **Status**: ⬜ TODO
  - gRPC server for Edge connections
  - Receive events over WireGuard tunnel
  - Validate event structure
  - Store in SQLite cache with rich metadata
- **Substep 2.3.1.2**: Event cache management
  - **Status**: ⬜ TODO
  - Rich metadata storage (bounding boxes, detection scores)
  - Event querying and retrieval
  - Cache expiration policies
  - Cache cleanup

#### Step 2.3.2: Event Forwarding to SaaS
- **Substep 2.3.2.1**: Event summarization
  - **Status**: ⬜ TODO
  - Privacy-minimized metadata extraction
  - Remove sensitive details (bounding boxes, raw scores)
  - Create summarized event record
- **Substep 2.3.2.2**: SaaS communication
  - **Status**: ⬜ TODO
  - gRPC client to SaaS
  - Forward summarized events
  - Handle forwarding failures and retries
  - Acknowledgment handling
- **Substep 2.3.2.3**: Unit tests for event cache service
  - **Status**: ⬜ TODO
  - **P0**: Test event reception from Edge (gRPC server)
  - **P0**: Test event validation and storage
  - **P0**: Test event cache management (querying, expiration, cleanup)
  - **P0**: Test event summarization (privacy-minimized metadata)
  - **P0**: Test event forwarding to SaaS (gRPC client, retries, acknowledgments)

### Epic 2.4: Telemetry Aggregation Service

**Priority: P0**

#### Step 2.4.1: Telemetry Collection
- **Substep 2.4.1.1**: Telemetry reception
  - **Status**: ⬜ TODO
  - **P0**: Receive telemetry from Edge Appliances
  - **P0**: Validate telemetry data
  - **P0**: Store raw-ish telemetry records in SQLite buffer
- **Substep 2.4.1.2**: Telemetry aggregation
  - **Status**: ⬜ TODO
  - **P0**: Simple "healthy/unhealthy" status calculation
  - **P1**: Aggregate per-tenant metrics (averages, totals)
  - **P1**: Advanced health status calculation

#### Step 2.4.2: Telemetry Forwarding
- **Substep 2.4.2.1**: Telemetry summarization
  - **Status**: ⬜ TODO
  - **P0**: Forward simple health status to SaaS
  - **P1**: Summarize telemetry (remove detailed metrics, create summaries)
- **Substep 2.4.2.2**: Forward to SaaS
  - **Status**: ⬜ TODO
  - **P0**: Send basic health status to SaaS
  - **P0**: Periodic reporting
  - **P1**: Advanced alert forwarding
- **Substep 2.4.2.3**: Unit tests for telemetry aggregation service
  - **Status**: ⬜ TODO
  - **P0**: Test telemetry reception and validation
  - **P0**: Test telemetry aggregation (healthy/unhealthy status)
  - **P0**: Test telemetry summarization
  - **P0**: Test telemetry forwarding to SaaS
  - **P1**: Test advanced metrics aggregation

### Epic 2.5: Stream Relay Service

**Priority: P0**

#### Step 2.5.1: Stream Request Handling
- **Substep 2.5.1.1**: Token validation
  - **Status**: ⬜ TODO
  - Receive time-bound tokens from SaaS
  - Validate token signature and expiration
  - Extract event ID and user info from token
- **Substep 2.5.1.2**: Stream orchestration
  - **Status**: ⬜ TODO
  - Request clip from Edge Appliance via gRPC
  - Handle Edge Appliance response
  - Stream setup coordination

#### Step 2.5.2: Stream Relay Implementation
- **Substep 2.5.2.1**: HTTP-based relay (P0 for PoC)
  - **Status**: ⬜ TODO
  - **P0**: Simple HTTP progressive download relay from Edge via KVM to client
  - **P0**: Request clip from Edge over WireGuard/gRPC
  - **P0**: Stream clip data via HTTP(S) to client
  - **P0**: Basic error handling and stream interruptions
  - **P1**: WebRTC relay using Pion (ICE, STUN/TURN, SDP exchange)
  - **P2**: Advanced WebRTC features (transcoding, reconnection logic)
- **Substep 2.5.2.2**: Unit tests for stream relay service
  - **Status**: ⬜ TODO
  - **P0**: Test token validation
  - **P0**: Test stream orchestration (request clip from Edge)
  - **P0**: Test HTTP-based relay (progressive download)
  - **P1**: Test WebRTC relay (if implemented)

### Epic 2.6: Filecoin Sync Service (Basic/Stub)

**Priority: P1** (Can be stubbed for PoC)

#### Step 2.6.1: Basic Archive Upload (PoC)
- **Substep 2.6.1.1**: Encrypted clip reception
  - **Status**: ⬜ TODO
  - **P0**: Receive encrypted clips from Edge (already encrypted)
  - **P0**: Store temporarily during upload
  - **P0**: Automatic cleanup after upload
- **Substep 2.6.1.2**: Upload implementation (basic)
  - **Status**: ⬜ TODO
  - **P0 Option**: Stub with S3 + fake CIDs for PoC demo
  - **P1 Option**: Basic IPFS gateway upload
  - **P2**: Full Filecoin integration
  - CID retrieval and storage

#### Step 2.6.2: Quota Management (Basic)
- **Substep 2.6.2.1**: Simple quota tracking
  - **Status**: ⬜ TODO
  - **P0**: Hard-coded quota limit for PoC
  - **P0**: Track archive size per tenant
  - **P2**: Complex quota policies from SaaS
- **Substep 2.6.2.2**: Basic quota enforcement
  - **Status**: ⬜ TODO
  - **P0**: Check quota before upload
  - **P0**: Reject uploads if over quota
  - **P2**: Advanced quota management

#### Step 2.6.3: Archive Metadata Management
- **Substep 2.6.3.1**: CID storage
  - **Status**: ⬜ TODO
  - **P0**: Store CIDs in SQLite
  - **P0**: Associate CIDs with events
  - **P1**: Basic metadata storage
- **Substep 2.6.3.2**: Archive status updates
  - **Status**: ⬜ TODO
  - **P0**: Update SaaS with archive status
  - **P0**: CID transmission to SaaS
  - **P2**: Advanced notification system
- **Substep 2.6.3.3**: Unit tests for Filecoin sync service
  - **Status**: ⬜ TODO
  - **P0**: Test encrypted clip reception
  - **P0**: Test upload implementation (S3 stub or IPFS gateway)
  - **P0**: Test quota tracking and enforcement
  - **P0**: Test CID storage and retrieval
  - **P0**: Test archive status updates

### Epic 2.7: KVM VM Agent Orchestration

**Priority: P0**

#### Step 2.7.1: Agent Service Manager
- **Substep 2.7.1.1**: Main agent service
  - **Status**: ⬜ TODO
  - Service initialization
  - Service lifecycle management
  - Configuration management
- **Substep 2.7.1.2**: Service coordination
  - **Status**: ⬜ TODO
  - Inter-service communication
  - Service health monitoring
  - Graceful shutdown

#### Step 2.7.2: SaaS Communication
- **Substep 2.7.2.1**: gRPC client to SaaS
  - **Status**: ⬜ TODO
  - **P0**: Connection setup
  - **P0**: mTLS configuration
  - **P0**: Connection health monitoring
- **Substep 2.7.2.2**: Command handling (basic)
  - **Status**: ⬜ TODO
  - **P1**: Basic command handling from SaaS
  - **P2**: Advanced command orchestration
- **Substep 2.7.2.3**: Unit tests for KVM VM agent orchestration
  - **Status**: ⬜ TODO
  - **P0**: Test agent service manager initialization
  - **P0**: Test service coordination and lifecycle
  - **P0**: Test gRPC client to SaaS (connection, mTLS)
  - **P1**: Test command handling from SaaS

---

## Phase 3: SaaS Control Plane Backend

**Duration**: 2-3 weeks  
**Goal**: Build core SaaS backend services - authentication, event inventory, basic VM management

**Scope**: Simplified for PoC - manual VM provisioning, basic auth, essential event storage

---

### Epic 3.1: SaaS Backend Project Setup

**Priority: P0**

#### Step 3.1.1: Project Structure
- **Substep 3.1.1.1**: Create SaaS backend directory structure
  - **Status**: ⬜ TODO
  - Note: SaaS backend is a private repository (git submodule in meta repo)
  - `saas/api/` - REST API service
  - `saas/auth/` - Authentication service
  - `saas/events/` - Event inventory service
  - `saas/provisioning/` - VM provisioning service
  - `saas/billing/` - Billing service
  - `saas/shared/` - Shared libraries
  - Note: gRPC proto definitions for KVM VM ↔ SaaS are in meta repo `proto/proto/kvm/` (imported as Go module)
- **Substep 3.1.1.2**: Go modules and dependencies
  - **Status**: ⬜ TODO
  - Initialize Go modules
  - Import `proto/go` from meta repo as Go module dependency
  - Database drivers (PostgreSQL, Redis)
  - External service clients

#### Step 3.1.2: Database Setup
- **Substep 3.1.2.1**: PostgreSQL schema design
  - **Status**: ⬜ TODO
  - Users table
  - Tenants table
  - KVM VM assignments table
  - Event metadata table
  - Subscriptions table
  - Billing records table
- **Substep 3.1.2.2**: Database migration system
  - **Status**: ⬜ TODO
  - Migration tool setup (golang-migrate)
  - Initial migrations
  - Migration rollback capability
- **Substep 3.1.2.3**: Unit tests for SaaS backend project setup
  - **Status**: ⬜ TODO
  - **P0**: Test PostgreSQL schema initialization
  - **P0**: Test database migration system
  - **P1**: Test Go module dependencies

### Epic 3.2: Authentication & User Management

**Priority: P0**

#### Step 3.2.1: Auth0 Integration
- **Substep 3.2.1.1**: Auth0 application setup
  - **Status**: ⬜ TODO
  - **P0**: Create Auth0 application
  - **P0**: Configure OAuth2/OIDC settings
  - **P0**: Set up callback URLs
  - **P1**: Configure user roles (single "tenant admin" role is P0)
- **Substep 3.2.1.2**: Backend authentication service
  - **Status**: ⬜ TODO
  - **P0**: JWT token validation middleware
  - **P0**: User session management
  - **P0**: Simple tenant mapping (single role: "tenant admin")
  - **P1**: Full RBAC implementation with multiple roles
  - **P0**: Token refresh handling
- **Substep 3.2.1.3**: User service
  - **Status**: ⬜ TODO
  - User CRUD operations
  - User profile management
  - User preferences storage
  - User-tenant association

#### Step 3.2.2: Tenant Management
- **Substep 3.2.2.1**: Tenant service
  - **Status**: ⬜ TODO
  - Tenant creation and management
  - Tenant settings
  - Tenant-KVM VM assignment
  - Tenant subscription association
- **Substep 3.2.2.2**: Multi-tenancy isolation
  - **Status**: ⬜ TODO
  - Tenant context middleware
  - Data isolation enforcement
  - Cross-tenant access prevention
- **Substep 3.2.2.3**: Unit tests for authentication and user management
  - **Status**: ⬜ TODO
  - **P0**: Test JWT token validation middleware
  - **P0**: Test user session management
  - **P0**: Test tenant mapping and isolation
  - **P0**: Test user CRUD operations
  - **P0**: Test tenant service (creation, VM assignment)
  - **P1**: Test RBAC implementation (if implemented)

### Epic 3.3: Event Inventory Service

**Priority: P0**

#### Step 3.3.1: Event Storage & Querying
- **Substep 3.3.1.1**: Event service implementation
  - **Status**: ⬜ TODO
  - **P0**: Store summarized event metadata in PostgreSQL
  - **P0**: Basic event querying API
  - **P0**: Basic filtering (camera, type, date range)
  - **P2**: Advanced indexing and full-text search
- **Substep 3.3.1.2**: Event search functionality
  - **Status**: ⬜ TODO
  - **P1**: Basic search by metadata fields
  - **P2**: Full-text search (PostgreSQL pg_trgm)
- **Substep 3.3.1.3**: Event retention policies
  - **Status**: ⬜ TODO
  - **P1**: Basic retention (simple cleanup)
  - **P2**: Configurable retention periods, archive status tracking

#### Step 3.3.2: Real-time Event Updates
- **Substep 3.3.2.1**: Event updates mechanism
  - **Status**: ⬜ TODO
  - **P0**: Basic polling (`/events` endpoint with periodic refresh)
  - **P1**: Server-Sent Events (SSE) for live updates
  - **P2**: Advanced SSE reconnection handling
- **Substep 3.3.2.2**: Event aggregation
  - **Status**: ⬜ TODO
  - **P1**: Basic event counts
  - **P2**: Advanced statistics and dashboard data
- **Substep 3.3.2.3**: Unit tests for event inventory service
  - **Status**: ⬜ TODO
  - **P0**: Test event storage and retrieval
  - **P0**: Test event querying and filtering
  - **P0**: Test event updates mechanism (polling)
  - **P1**: Test event search functionality
  - **P1**: Test Server-Sent Events (if implemented)
  - **P1**: Test event aggregation

### Epic 3.4: KVM VM Management Service (Basic)

**Priority: P0** (Simplified for PoC)

#### Step 3.4.1: Basic VM Assignment
- **Substep 3.4.1.1**: Manual VM provisioning (PoC)
  - **Status**: ⬜ TODO
  - **P0**: Pre-provision 1-2 KVM VMs manually
  - Simple CLI script or manual setup
  - Store VM connection details in database
  - **P2**: Full Terraform automation (post-PoC)
- **Substep 3.4.1.2**: VM assignment service (basic)
  - **Status**: ⬜ TODO
  - Assign pre-provisioned VM to tenant on signup
  - Store tenant-VM mapping in database
  - Basic VM status tracking
  - **P2**: VM lifecycle management (start/stop/delete, scaling)

#### Step 3.4.2: VM Communication
- **Substep 3.4.2.1**: gRPC server for VM agents
  - **Status**: ⬜ TODO
  - gRPC server setup
  - mTLS configuration
  - Command handling
- **Substep 3.4.2.2**: VM agent management
  - **Status**: ⬜ TODO
  - Agent registration
  - Agent health monitoring
  - Agent command execution
  - Agent configuration updates
- **Substep 3.4.2.3**: Unit tests for KVM VM management service
  - **Status**: ⬜ TODO
  - **P0**: Test VM assignment service
  - **P0**: Test tenant-VM mapping storage
  - **P0**: Test gRPC server for VM agents (mTLS, command handling)
  - **P0**: Test agent registration and health monitoring
  - **P1**: Test VM lifecycle management (if implemented)

### Epic 3.5: ISO Generation Service (Basic)

**Priority: P1** (Can use generic ISO for early PoC)

#### Step 3.5.1: Basic ISO Setup (PoC)
- **Substep 3.5.1.1**: Generic ISO preparation
  - **Status**: ⬜ TODO
  - **P0**: Single generic ISO with hard-coded config or manual bootstrap script
  - Base Ubuntu 24.04 LTS ISO
  - Manual configuration editing for PoC
  - **P2**: Full Packer pipeline with tenant-specific generation
- **Substep 3.5.1.2**: Basic bootstrap (PoC)
  - **Status**: ⬜ TODO
  - **P0**: Manual bootstrap token generation and configuration
  - Simple script-based configuration injection
  - **P2**: Automated tenant-specific ISO generation
- **Substep 3.5.1.3**: ISO download (basic)
  - **Status**: ⬜ TODO
  - **P0**: Simple download endpoint or manual distribution
  - **P2**: Secure download API with CDN integration
- **Substep 3.5.1.4**: Unit tests for ISO generation service
  - **Status**: ⬜ TODO
  - **P1**: Test generic ISO preparation
  - **P1**: Test bootstrap token generation
  - **P1**: Test ISO download endpoint

### Epic 3.6: Billing & Subscription Service (Basic)

**Priority: P2** (Defer to post-PoC, use free plan for PoC)

#### Step 3.6.1: Basic Plan Management (PoC)
- **Substep 3.6.1.1**: Simple plan model
  - **Status**: ⬜ TODO
  - **P0**: Hard-coded "free plan" for PoC
  - Basic plan assignment to tenants
  - **P2**: Full Stripe integration with webhooks
- **Substep 3.6.1.2**: Quota management (basic)
  - **Status**: ⬜ TODO
  - **P0**: Hard-coded quota limits for PoC
  - Basic quota tracking
  - **P2**: Full quota service with plan-based limits
- **Substep 3.6.1.3**: Unit tests for billing and subscription service
  - **Status**: ⬜ TODO
  - **P2**: Test plan management (if implemented)
  - **P2**: Test quota management (if implemented)

### Epic 3.7: REST API Service

**Priority: P0**

#### Step 3.7.1: API Framework
- **Substep 3.7.1.1**: Gin framework setup
  - **Status**: ⬜ TODO
  - Router configuration
  - Middleware setup (auth, logging, CORS)
  - Error handling
- **Substep 3.7.1.2**: API endpoints
  - **Status**: ⬜ TODO
  - User endpoints
  - Event endpoints
  - Camera endpoints
  - Subscription endpoints
  - VM management endpoints
- **Substep 3.7.1.3**: API documentation
  - **Status**: ⬜ TODO
  - OpenAPI/Swagger specification
  - API endpoint documentation
  - Request/response examples

#### Step 3.7.2: API Features (Basic)
- **Substep 3.7.2.1**: Rate limiting
  - **Status**: ⬜ TODO
  - **P1**: Basic rate limiting middleware
  - **P2**: Advanced per-user rate limits
- **Substep 3.7.2.2**: Caching
  - **Status**: ⬜ TODO
  - **P1**: Basic Redis caching for critical data
  - **P2**: Advanced caching strategies
- **Substep 3.7.2.3**: Unit tests for REST API service
  - **Status**: ⬜ TODO
  - **P0**: Test API endpoints (user, event, camera, subscription, VM management)
  - **P0**: Test authentication middleware
  - **P0**: Test error handling
  - **P1**: Test rate limiting middleware
  - **P1**: Test Redis caching (if implemented)

---

## Phase 4: SaaS UI Frontend

**Duration**: 2 weeks  
**Goal**: Build core React frontend - authentication, event timeline, basic clip viewing

**Scope**: Simplified UI for PoC - essential features only, no advanced configuration

**Milestone 2 Target**: End of this phase - first clip viewing (UI → SaaS → KVM VM → Edge → Stream)

### Epic 4.1: Frontend Project Setup

**Priority: P0**

**Note**: SaaS frontend is a private repository (git submodule in meta repo).

#### Step 4.1.1: React Project Structure
- **Substep 4.1.1.1**: Initialize React + TypeScript project
  - **Status**: ⬜ TODO
  - Create React app with Vite or Create React App
  - TypeScript configuration
  - Tailwind CSS setup
- **Substep 4.1.1.2**: Project structure
  - **Status**: ⬜ TODO
  - `src/components/` - React components
  - `src/pages/` - Page components
  - `src/hooks/` - Custom hooks
  - `src/services/` - API services
  - `src/store/` - State management (Zustand)
  - `src/utils/` - Utility functions
- **Substep 4.1.1.3**: Development tooling
  - **Status**: ⬜ TODO
  - ESLint configuration
  - Prettier configuration
  - Testing setup (Vitest/Jest)

#### Step 4.1.2: API Client Setup
- **Substep 4.1.2.1**: API client implementation
  - **Status**: ⬜ TODO
  - Axios or fetch wrapper
  - Request/response interceptors
  - Error handling
- **Substep 4.1.2.2**: API service layer
  - **Status**: ⬜ TODO
  - Event API service
  - User API service
  - Camera API service
  - Subscription API service
- **Substep 4.1.2.3**: Unit tests for frontend project setup
  - **Status**: ⬜ TODO
  - **P0**: Test API client implementation (request/response interceptors, error handling)
  - **P0**: Test API service layer (event, user, camera, subscription services)
  - **P1**: Test React component structure and utilities

### Epic 4.2: Authentication UI

**Priority: P0**

#### Step 4.2.1: Auth0 Integration
- **Substep 4.2.1.1**: Auth0 React SDK setup
  - **Status**: ⬜ TODO
  - Install and configure Auth0 React SDK
  - Auth0Provider setup
  - Configuration
- **Substep 4.2.1.2**: Authentication flows
  - **Status**: ⬜ TODO
  - Login page
  - Logout functionality
  - Protected route wrapper
  - Token management

#### Step 4.2.2: User Profile UI
- **Substep 4.2.2.1**: User profile page
  - **Status**: ⬜ TODO
  - Profile information display
  - Profile editing
  - User preferences
- **Substep 4.2.2.2**: User settings
  - **Status**: ⬜ TODO
  - Settings page
  - Notification preferences
  - Account management
- **Substep 4.2.2.3**: Unit tests for authentication UI
  - **Status**: ⬜ TODO
  - **P0**: Test Auth0 React SDK integration
  - **P0**: Test login/logout flows
  - **P0**: Test protected route wrapper
  - **P0**: Test token management
  - **P1**: Test user profile and settings components

### Epic 4.3: Dashboard & Navigation

**Priority: P0**

#### Step 4.3.1: Main Layout
- **Substep 4.3.1.1**: Layout component
  - **Status**: ⬜ TODO
  - Header with user info
  - Navigation sidebar
  - Main content area
  - Responsive design
- **Substep 4.3.1.2**: Navigation
  - **Status**: ⬜ TODO
  - Route configuration (React Router)
  - Navigation menu
  - Active route highlighting
  - Mobile navigation

#### Step 4.3.2: Dashboard Page (Basic)
- **Substep 4.3.2.1**: Basic dashboard
  - **Status**: ⬜ TODO
  - **P0**: Simple "Events" nav item
  - **P0**: Basic camera status label (e.g., "Cameras: 2 online")
  - **P1**: Dashboard widgets (camera overview, recent events, health indicators)
- **Substep 4.3.2.2**: Updates
  - **Status**: ⬜ TODO
  - **P0**: Basic polling refresh
  - **P1**: SSE connection for live updates
- **Substep 4.3.2.3**: Unit tests for dashboard and navigation
  - **Status**: ⬜ TODO
  - **P0**: Test layout component (header, sidebar, responsive)
  - **P0**: Test navigation and routing
  - **P0**: Test dashboard page (basic polling, camera status)
  - **P1**: Test SSE connection (if implemented)

### Epic 4.4: Event Timeline UI

**Priority: P0**

#### Step 4.4.1: Timeline Component
- **Substep 4.4.1.1**: Timeline layout
  - **Status**: ⬜ TODO
  - **P0**: Simple table/list of events
  - **P0**: Event card rendering
  - **P0**: Basic pagination
  - **P1**: Date grouping
  - **P1**: Infinite scroll
- **Substep 4.4.1.2**: Event cards
  - **Status**: ⬜ TODO
  - **P0**: Event metadata display
  - **P0**: Event type indicators
  - **P0**: Timestamp formatting
  - **P1**: Event thumbnail display

#### Step 4.4.2: Event Filtering & Search
- **Substep 4.4.2.1**: Filter UI
  - **Status**: ⬜ TODO
  - **P0**: Basic filters (camera, type, date range)
  - **P0**: Simple filter state management
  - **P1**: Advanced date range picker
- **Substep 4.4.2.2**: Search functionality
  - **Status**: ⬜ TODO
  - **P1**: Search input and basic search
  - **P1**: Search API integration
  - **P2**: Search history

#### Step 4.4.3: Event Details View
- **Substep 4.4.3.1**: Event detail modal/page
  - **Status**: ⬜ TODO
  - Event metadata display
  - Thumbnail/snapshot display
  - Detection details (bounding boxes if available)
  - Camera information
- **Substep 4.4.3.2**: Event actions
  - **Status**: ⬜ TODO
  - "View Clip" button
  - "Download" button
  - "Archive" button (if applicable)
  - Event deletion (if allowed)
- **Substep 4.4.3.3**: Unit tests for event timeline UI
  - **Status**: ⬜ TODO
  - **P0**: Test timeline component (event list, cards, pagination)
  - **P0**: Test event filtering (camera, type, date range)
  - **P0**: Test event details view and actions
  - **P1**: Test event search functionality
  - **P1**: Test date grouping and infinite scroll

### Epic 4.5: Clip Viewing UI

**Priority: P0**

#### Step 4.5.1: Video Player Component
- **Substep 4.5.1.1**: Video player integration
  - **Status**: ⬜ TODO
  - **P0**: Standard HTML5 `<video>` element with HTTP URL
  - **P0**: React video player component using HTTP progressive download
  - **P0**: Basic playback controls (play/pause, seek)
  - **P1**: WebRTC stream handling (if WebRTC implemented)
  - **P1**: Fullscreen support
- **Substep 4.5.1.2**: Stream request flow
  - **Status**: ⬜ TODO
  - **P0**: "View Clip" button requests HTTP URL from SaaS
  - **P0**: Loading states and error handling
  - **P1**: WebRTC connection management (if WebRTC implemented)

#### Step 4.5.2: Clip Player Features
- **Substep 4.5.2.1**: Playback features
  - **Status**: ⬜ TODO
  - Play/pause
  - Seek
  - Volume control
  - Playback speed
- **Substep 4.5.2.2**: Clip information
  - **Status**: ⬜ TODO
  - Clip metadata display
  - Camera information
  - Timestamp display
  - Download option
- **Substep 4.5.2.3**: Unit tests for clip viewing UI
  - **Status**: ⬜ TODO
  - **P0**: Test video player component (HTTP progressive download)
  - **P0**: Test stream request flow (loading states, error handling)
  - **P0**: Test playback features (play/pause, seek, volume)
  - **P1**: Test WebRTC stream handling (if implemented)

### Epic 4.6: Camera Management UI (Basic)

**Priority: P0** (Simplified for PoC)

#### Step 4.6.1: Basic Camera List
- **Substep 4.6.1.1**: Camera list page (simple)
  - **Status**: ⬜ TODO
  - **P0**: Display discovered cameras from Edge
  - Camera status indicators (online/offline)
  - Basic camera information
  - **P2**: Camera thumbnail/preview, advanced actions
- **Substep 4.6.1.2**: Basic camera configuration
  - **Status**: ⬜ TODO
  - **P0**: Camera naming and labeling
  - **P2**: Detection zones, schedules, advanced settings
- **Substep 4.6.1.3**: Unit tests for camera management UI
  - **Status**: ⬜ TODO
  - **P0**: Test camera list page (display, status indicators)
  - **P0**: Test camera configuration (naming, labeling)
  - **P2**: Test advanced camera settings (if implemented)

### Epic 4.7: Subscription & Billing UI

**Priority: P2** (Defer to post-PoC)

#### Step 4.7.1: Basic Plan Display (PoC)
- **Substep 4.7.1.1**: Simple plan indicator
  - **Status**: ⬜ TODO
  - **P0**: Display "Free Plan" or plan name (hard-coded for PoC)
  - **P2**: Full subscription management UI, plan comparison, upgrade/downgrade
- **Substep 4.7.1.2**: Billing UI
  - **Status**: ⬜ TODO
  - **P2**: Payment method management, billing history, Stripe integration

### Epic 4.8: Onboarding & ISO Download

**Priority: P1** (Can be simplified for PoC)

#### Step 4.8.1: Onboarding Flow
- **Substep 4.8.1.1**: Onboarding wizard
  - **Status**: ⬜ TODO
  - Welcome screen
  - Plan selection
  - ISO download instructions
  - Setup guide
- **Substep 4.8.1.2**: ISO download page
  - **Status**: ⬜ TODO
  - ISO download button
  - Download instructions
  - Installation guide
  - Troubleshooting tips
- **Substep 4.8.1.3**: Unit tests for onboarding and ISO download
  - **Status**: ⬜ TODO
  - **P1**: Test onboarding flow components
  - **P1**: Test ISO download page

---

## Phase 5: ISO Building & Deployment Automation

**Duration**: 1-2 weeks  
**Goal**: Basic ISO generation and simple deployment automation

**Scope**: Simplified for PoC - generic ISO or simple build script, manual deployment acceptable

---

### Epic 5.1: ISO Build Pipeline (Basic)

**Priority: P1** (Can use generic ISO for early PoC)

**Note**: As of November 2025, Ubuntu 24.04 LTS is the current LTS (supported until 2029). Ubuntu 22.04 LTS remains supported until 2027 and is also acceptable for PoC.

#### Step 5.1.1: Basic ISO Setup
- **Substep 5.1.1.1**: Generic ISO preparation
  - **Status**: ⬜ TODO
  - **P0**: Single generic Ubuntu 24.04 LTS Server ISO
  - Manual configuration or simple bootstrap script
  - Basic auto-install configuration
  - **P2**: Full Packer automation with tenant-specific generation
- **Substep 5.1.1.2**: Software pre-installation (basic)
  - **Status**: ⬜ TODO
  - **P0**: Manual installation of Edge Appliance software
  - Or simple installation script
  - **P2**: Automated packaging and pre-installation

#### Step 5.1.2: Basic Configuration
- **Substep 5.1.2.1**: Bootstrap configuration (simple)
  - **Status**: ⬜ TODO
  - **P0**: Manual bootstrap token generation
  - Manual KVM VM connection details configuration
  - Simple first-boot script
  - **P2**: Automated tenant-specific configuration injection
- **Substep 5.1.2.2**: Build automation (basic)
  - **Status**: ⬜ TODO
  - **P0**: Simple build script on developer machine
  - **P2**: Full CI/CD pipeline with Packer
- **Substep 5.1.2.3**: Unit tests for ISO build pipeline
  - **Status**: ⬜ TODO
  - **P1**: Test ISO preparation scripts
  - **P1**: Test bootstrap configuration generation
  - **P1**: Test build automation scripts
  - **P2**: Test Packer automation (if implemented)

### Epic 5.2: Deployment Automation (Basic)

**Priority: P1** (Manual deployment acceptable for PoC)

#### Step 5.2.1: Basic Deployment
- **Substep 5.2.1.1**: Manual deployment (PoC)
  - **Status**: ⬜ TODO
  - **P0**: Manual KVM VM setup (1-2 VMs)
  - Manual agent installation and configuration
  - Manual SaaS deployment (Docker Compose or simple K8s)
  - **P2**: Full Terraform automation
- **Substep 5.2.1.2**: Basic automation scripts
  - **Status**: ⬜ TODO
  - **P1**: Simple deployment scripts
  - Basic configuration management
  - **P2**: Full Infrastructure as Code

#### Step 5.2.2: SaaS Deployment (Basic)
- **Substep 5.2.2.1**: Simple deployment
  - **Status**: ⬜ TODO
  - **P0**: Docker Compose for local PoC or simple K8s deployment
  - Basic service configuration
  - **P2**: Full EKS setup with advanced configuration
- **Substep 5.2.2.2**: Database setup
  - **Status**: ⬜ TODO
  - **P0**: Manual PostgreSQL setup or managed database
  - Basic migration execution
  - **P2**: Automated database deployment and backups
- **Substep 5.2.2.3**: Unit tests for deployment automation
  - **Status**: ⬜ TODO
  - **P1**: Test deployment scripts
  - **P1**: Test configuration management
  - **P2**: Test Infrastructure as Code (if implemented)

### Epic 5.3: Update & Maintenance Automation

**Priority: P2** (Defer to post-PoC)

#### Step 5.3.1: Update Mechanisms
- **Substep 5.3.1.1**: Manual updates for PoC
  - **Status**: ⬜ TODO
  - **P0**: Manual update process for PoC
  - **P2**: Automated update delivery, signed packages, rollback mechanisms

---

## Phase 6: Integration, Testing & Polish

**Duration**: 2 weeks  
**Goal**: End-to-end integration, essential testing, basic security, PoC demo preparation

**Scope**: Focus on integration and demo readiness, not full production hardening

### Epic 6.1: End-to-End Integration

**Priority: P0**

#### Step 6.1.1: Complete Data Flow Integration
- **Substep 6.1.1.1**: Event flow end-to-end
  - **Status**: ⬜ TODO
  - Camera → Edge → KVM VM → SaaS → UI
  - Verify data integrity at each step
  - Test error handling and recovery
- **Substep 6.1.1.2**: Stream flow end-to-end
  - **Status**: ⬜ TODO
  - UI request → SaaS → KVM VM → Edge → Stream → UI
  - **P0**: Test HTTP clip streaming (progressive download)
  - **P0**: Test stream interruptions and basic error handling
  - **P1/P2**: Test WebRTC stream quality (if WebRTC implemented)
- **Substep 6.1.1.3**: Telemetry flow end-to-end
  - **Status**: ⬜ TODO
  - Edge → KVM VM → SaaS → Dashboard
  - Verify telemetry accuracy
  - Test aggregation and reporting

#### Step 6.1.2: Multi-Tenant Isolation Testing
- **Substep 6.1.2.1**: Tenant isolation verification
  - **Status**: ⬜ TODO
  - Create multiple test tenants
  - Verify data isolation in SaaS
  - Test cross-tenant access prevention
- **Substep 6.1.2.2**: KVM VM isolation
  - **Status**: ⬜ TODO
  - Verify WireGuard tunnel isolation
  - Test VM resource isolation
  - Verify network isolation
  - Test VM-to-VM isolation

#### Step 6.1.3: Archive Integration (Basic)
- **Substep 6.1.3.1**: Archive flow end-to-end (P0: S3 stub)
  - **Status**: ⬜ TODO
  - **P0**: Edge encryption → KVM VM → S3 stub → fake CID storage
  - **P0**: Verify encryption throughout
  - **P0**: Test basic quota enforcement
  - **P2**: Full Filecoin integration with real CIDs
- **Substep 6.1.3.2**: Archive retrieval flow
  - **Status**: ⬜ TODO
  - **P0**: UI request → SaaS → fake CID → S3 stub (basic verification)
  - **P2**: Browser-based decryption using Filecoin blob (full implementation)

### Epic 6.2: Essential Testing

**Priority: P0** (Focus on critical paths for PoC)

#### Step 6.2.1: Critical Path Unit Tests (Regression Suite)
- **Substep 6.2.1.1**: Essential unit tests (regression verification)
  - **Status**: ⬜ TODO
  - **P0**: Verify all unit tests from previous phases pass (regression check)
  - **P0**: Edge event generation & queueing (comprehensive coverage)
  - **P0**: Edge↔KVM VM gRPC contracts (contract testing)
  - **P0**: KVM VM↔SaaS event/telemetry contracts (contract testing)
  - **P0**: Basic auth flows (security-critical paths)
  - **P2**: Full test coverage > 70% (comprehensive coverage audit)
- **Substep 6.2.1.2**: Python AI service tests (regression verification)
  - **Status**: ⬜ TODO
  - **P0**: Verify all Python unit tests pass (regression check)
  - **P0**: Model loading and basic inference (comprehensive coverage)
  - **P2**: Comprehensive edge case testing

#### Step 6.2.2: Integration Testing (Essential)
- **Substep 6.2.2.1**: Critical integration tests
  - **Status**: 🚧 IN_PROGRESS
  - **P0**: Edge ↔ KVM VM event flow (blocked on Phase 2)
  - **P0**: KVM VM ↔ SaaS event forwarding (blocked on Phase 2-3)
  - **P0**: Full stack event flow (blocked on Phase 2-3)
  - **P0**: Service manager integration ✅
  - **P0**: Configuration and state integration ✅
  - **P0**: Storage management integration ✅
  - **P0**: Database integration with storage state ✅
  - **P2**: Comprehensive integration test suite
- **Substep 6.2.2.2**: Database tests (basic)
  - **Status**: ✅ DONE
  - **P0**: Data persistence verification ✅
  - **P0**: State recovery tests ✅
  - **P0**: Camera state persistence ✅
  - **P2**: Transaction and performance tests

#### Step 6.2.3: End-to-End Testing (Key Scenarios)
- **Substep 6.2.3.1**: Critical E2E scenarios
  - **Status**: ⬜ TODO
  - **P0**: Event detection and display in UI
  - **P0**: Clip viewing flow
  - **P1**: Basic archive flow (if Filecoin implemented)
  - **P2**: Full E2E test automation with Playwright/Cypress
- **Substep 6.2.3.2**: Manual testing
  - **Status**: ⬜ TODO
  - **P0**: Manual test scenarios for PoC demo
  - **P2**: Automated E2E test suite

#### Step 6.2.4: Performance Testing (Basic)
- **Substep 6.2.4.1**: Basic performance verification
  - **Status**: ⬜ TODO
  - **P0**: Single camera, single user performance
  - **P1**: Basic load testing (2-3 concurrent users)
  - **P2**: Comprehensive load and performance testing
  - Network throughput

### Epic 6.3: Error Handling & Resilience

**Priority: P0** (Basic error handling for PoC)

#### Step 6.3.1: Error Handling Implementation
- **Substep 6.3.1.1**: Error handling patterns
  - **Status**: ⬜ TODO
  - **P0**: Standardized error types
  - **P0**: Error propagation
  - **P0**: Error logging
  - **P0**: Basic user-friendly error messages
- **Substep 6.3.1.2**: Retry mechanisms
  - **Status**: ⬜ TODO
  - **P0**: Network operation retries with exponential backoff
  - **P0**: Database operation retries
  - **P1**: Circuit breakers
- **Substep 6.3.1.3**: Resilience testing
  - **Status**: ⬜ TODO
  - Network failure scenarios
  - Service crash scenarios
  - Database failure scenarios
  - Recovery testing

### Epic 6.4: Security Hardening

**Priority: P0** (Basic security for PoC)

#### Step 6.4.1: Basic Security Review
- **Substep 6.4.1.1**: Essential security checks
  - **Status**: ⬜ TODO
  - **P0**: Dependency vulnerability scanning
  - **P0**: Basic security best practices review
  - **P2**: Full static analysis (CodeQL, SonarQube)
- **Substep 6.4.1.2**: Basic security testing
  - **Status**: ⬜ TODO
  - **P0**: API authentication/authorization testing
  - **P0**: Input validation testing
  - **P2**: Full penetration testing
- **Substep 6.4.1.3**: Security fixes
  - **Status**: ⬜ TODO
  - **P0**: Address critical vulnerabilities
  - **P2**: Comprehensive security hardening

#### Step 6.4.2: Essential Security Enhancements
- **Substep 6.4.2.1**: Input validation
  - **Status**: ⬜ TODO
  - **P0**: Sanitize all user inputs
  - **P0**: Validate API parameters
  - **P0**: SQL injection prevention
  - **P1**: XSS prevention
- **Substep 6.4.2.2**: Basic rate limiting
  - **Status**: ⬜ TODO
  - **P1**: Basic API rate limiting
  - **P2**: Advanced rate limiting and DDoS protection
- **Substep 6.4.2.3**: Security headers
  - **Status**: ⬜ TODO
  - **P0**: HTTPS enforcement
  - **P1**: Basic security headers
  - **P2**: Comprehensive security headers (CSP, HSTS, etc.)

### Epic 6.5: Performance Optimization

**Priority: P1** (Basic optimization for PoC)

#### Step 6.5.1: Essential Backend Optimization
- **Substep 6.5.1.1**: Basic database optimization
  - **Status**: ⬜ TODO
  - **P0**: Essential index creation
  - **P1**: Query optimization for critical paths
  - **P2**: Advanced optimization and caching
- **Substep 6.5.1.2**: Service optimization (basic)
  - **Status**: ⬜ TODO
  - **P1**: Profile and fix obvious performance issues
  - **P2**: Comprehensive optimization

#### Step 6.5.2: Basic Frontend Optimization
- **Substep 6.5.2.1**: Essential bundle optimization
  - **Status**: ⬜ TODO
  - **P1**: Basic code splitting
  - **P2**: Advanced optimization (tree shaking, lazy loading)
- **Substep 6.5.2.2**: Performance optimization (basic)
  - **Status**: ⬜ TODO
  - **P1**: Fix obvious React performance issues
  - **P2**: Comprehensive optimization

### Epic 6.6: Monitoring & Observability

**Priority: P1** (Basic monitoring for PoC)

#### Step 6.6.1: Basic Metrics Implementation
- **Substep 6.6.1.1**: Essential metrics
  - **Status**: ⬜ TODO
  - **P0**: Basic service health metrics
  - **P1**: Prometheus setup with basic metrics
  - **P2**: Comprehensive metrics (business, system)
- **Substep 6.6.1.2**: Basic dashboards
  - **Status**: ⬜ TODO
  - **P1**: Simple Grafana dashboard for health
  - **P2**: Comprehensive dashboards
- **Substep 6.6.1.3**: Basic alerting
  - **Status**: ⬜ TODO
  - **P1**: Essential alerts (service down, critical errors)
  - **P2**: Comprehensive alerting setup

#### Step 6.6.2: Basic Logging Implementation
- **Substep 6.6.2.1**: Structured logging
  - **Status**: ⬜ TODO
  - **P0**: JSON log format
  - **P0**: Basic log levels
  - **P2**: Advanced contextual logging
- **Substep 6.6.2.2**: Log aggregation (basic)
  - **Status**: ⬜ TODO
  - **P1**: Basic log collection (Loki or simple file aggregation)
  - **P2**: Full log aggregation and analysis
- **Substep 6.6.2.3**: Privacy-aware logging
  - **Status**: ⬜ TODO
  - **P0**: Ensure no PII in logs (critical for PoC)
  - **P0**: Basic log sanitization
  - **P2**: Comprehensive sensitive data filtering

### Epic 6.7: Documentation

**Priority: P1** (Essential documentation for PoC)

#### Step 6.7.1: Technical Documentation
- **Substep 6.7.1.1**: API documentation
  - **Status**: ⬜ TODO
  - OpenAPI/Swagger specs
  - API endpoint documentation
  - Request/response examples
- **Substep 6.7.1.2**: Code documentation
  - **Status**: ⬜ TODO
  - GoDoc comments
  - Python docstrings
  - Architecture decision records (ADRs)
- **Substep 6.7.1.3**: Deployment documentation
  - **Status**: ⬜ TODO
  - Infrastructure setup guide
  - Service deployment procedures
  - Configuration reference

#### Step 6.7.2: User Documentation
- **Substep 6.7.2.1**: User guide
  - **Status**: ⬜ TODO
  - Getting started guide
  - Feature documentation
  - Troubleshooting guide
- **Substep 6.7.2.2**: Developer guide
  - **Status**: ⬜ TODO
  - Development environment setup
  - Contribution guidelines
  - Testing guidelines

### Epic 6.8: PoC Demo Preparation

**Priority: P0**

#### Step 6.8.1: Demo Environment Setup
- **Substep 6.8.1.1**: Clean demo environment
  - **Status**: ⬜ TODO
  - Fresh deployment
  - Sample data setup
  - Test cameras configuration
- **Substep 6.8.1.2**: Demo script preparation
  - **Status**: ⬜ TODO
  - Demo flow outline
  - Key features to showcase
  - Backup scenarios

#### Step 6.8.2: Demo Materials
- **Substep 6.8.2.1**: Presentation materials
  - **Status**: ⬜ TODO
  - Architecture overview slides
  - Key features slides
  - Demo video recording
- **Substep 6.8.2.2**: Demo data
  - **Status**: ⬜ TODO
  - Sample events
  - Sample clips
  - Test scenarios

---

## Success Criteria

### Phase 1 Success Criteria (Edge Appliance)

**PoC Must-Have:**
- ✅ Go orchestrator service running and managing all components
- ✅ Python AI service running and performing inference
- ✅ Edge Appliance can discover and connect to 1-2 cameras (RTSP/ONVIF/USB)
- ✅ Video clips recorded and stored locally
- ✅ AI inference detecting objects (people, vehicles)
- ✅ Events generated and queued for transmission
- ✅ WireGuard client connecting to KVM VM
- 🚧 Basic telemetry being collected and reported (implementation complete, unit tests pending)

**Completed Components:**
- ✅ Core orchestrator framework (service manager, config, state, health checks)
- ✅ Camera discovery and management (RTSP, ONVIF, USB/V4L2)
- ✅ Video processing (FFmpeg integration, frame extraction, clip recording)
- ✅ Local storage management (file organization, retention, snapshots)
- ✅ Comprehensive unit tests (161 tests passing)
- ✅ Integration tests (10 tests passing)

**Stretch Goals:**
- Advanced detection zones, schedules
- Complex retention policies
- Full archive client implementation

### Phase 2 Success Criteria (KVM VM Agent)

**PoC Must-Have:**
- ✅ WireGuard server running and accepting Edge connections
- ✅ KVM VM receiving events from Edge Appliances
- ✅ Events cached in SQLite and forwarded to SaaS
- ✅ Basic stream relay working (Edge → KVM VM → Client via HTTP or WebRTC)
- ✅ Basic telemetry aggregation and forwarding to SaaS
- ✅ **Milestone 1**: First full event flow (Camera → Edge → KVM VM → SaaS → Simple UI)

**Stretch Goals:**
- Full WebRTC implementation (HTTP relay acceptable for PoC)
- Full Filecoin integration (S3 stub acceptable for PoC)
- Advanced telemetry aggregation

### Phase 3 Success Criteria (SaaS Backend)

**PoC Must-Have:**
- ✅ Authentication service working (Auth0 integration)
- ✅ Users can sign up and authenticate
- ✅ Event inventory service storing and querying events
- ✅ Basic filtering (date, camera, event type)
- ✅ REST API functional for core endpoints
- ✅ Manual KVM VM assignment working

**Stretch Goals:**
- Automated VM provisioning (Terraform)
- Full Stripe billing integration
- Advanced VM lifecycle management

### Phase 4 Success Criteria (SaaS UI)

**PoC Must-Have:**
- ✅ Users can log in via Auth0
- ✅ Event timeline displaying events in UI
- ✅ Basic filtering and search
- ✅ Users can view clips on-demand
- ✅ **Milestone 2**: First clip viewing flow working
- ✅ Basic camera list/status page

**Stretch Goals:**
- Subscription management UI
- Advanced camera configuration UI
- Rich dashboard with statistics

### Phase 5 Success Criteria (ISO & Deployment)

**PoC Must-Have:**
- ✅ Generic ISO or simple build script working
- ✅ ISO can be installed on Mini PC
- ✅ Edge Appliance boots and connects to KVM VM
- ✅ Basic deployment scripts or manual deployment working

**Stretch Goals:**
- Tenant-specific ISO generation
- Full Packer automation
- Automated deployment pipeline

### Phase 6 Success Criteria (Integration & Polish)

**PoC Must-Have:**
- ✅ End-to-end event flow working (Camera → Edge → KVM VM → SaaS → UI)
- ✅ End-to-end HTTP clip streaming working (progressive download)
- ✅ Basic security review (no critical vulnerabilities)
- ✅ Essential tests passing (critical paths)
- ✅ Basic monitoring working
- ✅ PoC demo ready with key scenarios

**Stretch Goals:**
- WebRTC streaming implementation
- End-to-end Filecoin archiving working (S3 stub acceptable for PoC)
- Test coverage > 70%
- Full security audit
- Comprehensive performance testing
- Full documentation

---

## Risk Mitigation

### Technical Risks

**Risk**: Hardware acceleration not working on target Mini PCs  
**Mitigation**: Implement software fallback early, test on multiple hardware configurations

**Risk**: OpenVINO performance not meeting requirements  
**Mitigation**: Benchmark early, have ONNX Runtime as fallback, consider model quantization

**Risk**: WireGuard tunnel stability issues  
**Mitigation**: Implement robust reconnection logic, test under various network conditions

**Risk**: Filecoin integration complexity  
**Mitigation**: Use S3 stub with fake CIDs for PoC, add Filecoin integration post-PoC

### Timeline Risks

**Risk**: Phase delays cascading  
**Mitigation**: Build in buffer time, prioritize critical path features, be ready to descope non-essential features

**Risk**: Integration issues discovered late  
**Mitigation**: Integrate early and often, continuous testing throughout

### Resource Risks

**Risk**: Key dependencies unavailable  
**Mitigation**: Identify critical dependencies early, have alternatives ready

---

## Dependencies & Prerequisites

### External Dependencies
- Auth0 account and API keys
- Stripe account and API keys
- AWS account (or alternative cloud)
- Hetzner account (or alternative KVM hosting)
- Filecoin/IPFS provider access
- Test cameras (RTSP/ONVIF compatible)

### Internal Dependencies
- Development team with Go, Python, TypeScript experience
- DevOps expertise for infrastructure setup
- Access to test Mini PC hardware

---

## Next Steps After PoC

1. **User Testing**: Deploy PoC to beta users, gather feedback
2. **Performance Scaling**: Optimize for higher load, more cameras
3. **Feature Expansion**: Add advanced AI models, additional camera types
4. **Production Hardening**: Enhanced security, compliance, SLAs
5. **Commercialization**: Pricing optimization, sales materials, go-to-market

---

*This implementation plan is a living document and should be updated as the project progresses and requirements evolve.*

