# Implementation Plan - PoC

This document provides a detailed, phase-by-phase implementation plan for The Private AI Guardian platform Proof of Concept (PoC), based on the architecture and technical stack defined in ARCHITECTURE.md and TECHNICAL_STACK.md.

## Overview

The PoC implementation follows a **bottom-up approach**, building from the edge inward:
1. **Edge Appliance (Mini PC)** - Go orchestrator and Python AI services
2. **User VM API (Docker Compose)** - Go services running in local Docker Compose environment
3. **MinIO (Docker Compose)** - S3-compatible storage for remote clip archiving
4. **Integration, Testing & Polish** - End-to-end integration and refinement

**Note**: For PoC, SaaS components (Management Server, SaaS Control Plane, SaaS UI) are **not included**. The PoC focuses on Edge Appliance ↔ User VM API communication, with User VM API running as a Docker Compose service in the local development environment.

Each phase builds upon the previous one, allowing for incremental validation and testing.

**Target Timeline**: 8-12 weeks for complete PoC (revised based on actual development velocity)

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
- **Phase 1 (Edge Appliance)**: ~88% complete
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
  - ✅ Telemetry & health reporting
  - ⬜ Encryption & archive client
  - ✅ Edge Web UI (local network accessible - Epic 1.9)
    - **Backend API** (Steps 1.9.1-1.9.6):
      - ✅ Web Server & API Foundation (Step 1.9.1)
      - ✅ Camera Streaming API (Step 1.9.2)
      - ✅ Event API (Step 1.9.3)
      - ✅ Configuration API (Step 1.9.4)
      - ✅ Camera Management API (Step 1.9.5)
      - ✅ Status & Metrics API (Step 1.9.6)
    - **Frontend UI** (Steps 1.9.7-1.9.12):
      - ✅ Frontend Framework & Build Setup (Step 1.9.7)
      - ✅ Camera Viewer UI (Step 1.9.8)
      - ✅ Event Timeline UI (Step 1.9.9)
      - ✅ Configuration UI (Step 1.9.10)
      - ✅ Camera Management UI (Step 1.9.11)
      - ✅ Dashboard UI (Step 1.9.12)
    - **Integration** (Step 1.9.13):
      - ✅ Integration & Testing (Step 1.9.13)
- **Phase 2 (User VM API)**: 0% complete
  - **Note**: User VM API runs in Docker Compose for PoC (no SaaS needed)
- **Phase 3 (SaaS Backend)**: ⏸️ DEFERRED (not needed for PoC)
- **Phase 4 (SaaS UI)**: ⏸️ DEFERRED (not needed for PoC - Edge Web UI is sufficient)
- **Phase 5 (ISO & Deployment)**: ⏸️ DEFERRED (not needed for PoC - Docker Compose is sufficient)
- **Phase 6 (Integration & Testing)**: 0% complete

*Last Updated: 2025-11-23*

**Repository Structure Note:**
- **Public components** (Edge Appliance, User Server, Crypto libraries, Protocol definitions) are developed directly in the meta repository:
  - `edge/` - Edge Appliance software (including Edge Web UI)
  - `user-vm-api/` - User Server (runs on user's VM, handles WireGuard, models, storage)
  - `crypto/` - Encryption libraries
  - `proto/` - Protocol buffer definitions
- **Private components** (Management Server, SaaS backend, SaaS frontend, Infrastructure) are in separate private repositories (git submodules):
  - `management-server/` - Management Server (controls User Servers, talks to SaaS)
  - `saas-backend/` - SaaS backend (private submodule)
  - `saas-frontend/` - SaaS frontend (private submodule)
  - `infra/` - Infrastructure (private submodule)

**Note on Edge Web UI**: The Edge Web UI is part of the Edge Appliance and is **open source** in the main repository. This aligns with the "trust us / verify us" privacy story - customers can audit all code that runs on their hardware, including the local admin interface. The SaaS frontend remains private as it contains multi-tenant SaaS logic and commercial IP.

## Priority Tags

Epics are tagged with priority levels:
- **P0 (Core PoC)**: Must-have for PoC demonstration - essential functionality
- **P1 (Nice-to-have)**: Important but can be simplified or deferred if time is tight
- **P2 (Post-PoC)**: Full implementation deferred until after PoC validation

## Early Milestones

To avoid discovering integration issues late, we include early vertical slices:

- **Milestone 1 (End of Phase 2)**: First full event flow
  - Camera → Edge Appliance → User VM API → Event cache
  - Validates core data flow without streaming
  - **Target**: Week 2-3

- **Milestone 2 (End of Phase 2)**: First clip viewing
  - Edge Web UI "View Clip" → User VM API → Edge Appliance → Stream to UI (HTTP-based)
  - Validates streaming path using simple HTTP progressive download
  - **Target**: Week 3-4

**Note**: For PoC, milestones are simplified - no SaaS components needed. Edge Web UI provides the user interface, and User VM API handles event caching and stream relay.

## PoC Scope Summary

This implementation plan is scoped for a **realistic PoC**, not a full v1 product.

**PoC Deployment Topology**: 1 Edge Appliance → 1 User VM API (in Docker Compose) → MinIO (S3-compatible storage), running locally for development and testing. This is a **simplified PoC without SaaS components**.

**Key PoC Simplifications**:
- **No SaaS components** - Edge Appliance and User VM API communicate directly
- **User VM API runs in Docker Compose** - Part of local development environment as a dedicated service
- **MinIO instead of Filecoin** - Use MinIO (S3-compatible) for remote storage in PoC
- **Direct communication** - Edge → User VM API (no Management Server or SaaS in PoC)

Key simplifications:

### What's Included (P0 - Core PoC)
- Single Edge Appliance with 1-2 cameras
- Basic AI inference (person/vehicle detection)
- Local clip recording and storage
- Event flow: Edge → User VM API (Docker Compose service)
- Basic clip viewing (HTTP relay acceptable)
- **Edge Web UI** (local network accessible for configuration and monitoring)
- **User VM API** (Go service in Docker Compose):
  - WireGuard server for Edge connections
  - Event cache and storage
  - Stream relay for clip viewing
  - MinIO integration (S3-compatible storage)
  - AI model catalog (basic)
  - Secondary event analysis (basic)
- **MinIO** (S3-compatible storage in Docker Compose) for remote clip storage
- Essential testing (critical paths only)

### What's Deferred (P2 - Post-PoC)
- **SaaS Control Plane** (not needed for PoC)
- **Management Server** (not needed for PoC)
- **SaaS UI** (not needed for PoC - Edge Web UI is sufficient)
- Full Stripe billing integration
- Automated VM provisioning
- Tenant-specific ISO generation
- **WebRTC implementation** (HTTP progressive download is P0 for PoC)
- **Full Filecoin integration** (MinIO/S3 is P0 for PoC, Filecoin bridge to be developed later)
- **S3-Filecoin bridge** (to be developed post-PoC to migrate from MinIO to Filecoin)
- Advanced camera configuration (zones, schedules)
- Comprehensive test coverage (>70%)
- Full security audit
- Advanced monitoring and observability
- Update automation

### Architecture Compliance (PoC)
- **Edge Appliance**: All video processing, AI inference, and local storage (Phase 1)
- **User VM API** (Docker Compose): WireGuard server, event cache, stream relay, MinIO integration - NO video processing or AI (Phase 2)
- **MinIO**: S3-compatible storage for remote clip archiving (replaces Filecoin in PoC)
- **SaaS**: Deferred to post-PoC

---

## Table of Contents

This implementation plan has been split into phase-specific files for better maintainability:

1. **[Phase 1: Edge Appliance (Mini PC) - Go & Python Apps](IMPLEMENTATION_PLAN_PHASE1.md)** - Detailed implementation plan for Edge Appliance
2. **[Phase 2: User VM API Services (Docker Compose)](IMPLEMENTATION_PLAN_PHASE2.md)** - Detailed implementation plan for User VM API
3. **[Phase 3: SaaS Control Plane Backend](IMPLEMENTATION_PLAN_PHASE3.md)** - Detailed implementation plan for SaaS Backend (deferred for PoC)
4. **[Phase 4: SaaS UI Frontend](IMPLEMENTATION_PLAN_PHASE4.md)** - Detailed implementation plan for SaaS UI (deferred for PoC)
5. **[Phase 5: ISO Building & Deployment Automation](IMPLEMENTATION_PLAN_PHASE5.md)** - Detailed implementation plan for ISO & Deployment (deferred for PoC)
6. **[Phase 6: Integration, Testing & Polish](IMPLEMENTATION_PLAN_PHASE6.md)** - Detailed implementation plan for Integration & Testing

**Note**: Phases 3-5 (SaaS Backend, SaaS UI, ISO & Deployment) are **deferred for PoC**. The PoC focuses on Edge Appliance ↔ User VM API communication, with User VM API running in Docker Compose.

---

## Phase Overview

This section provides a high-level overview of each phase. For detailed implementation plans, see the phase-specific files linked in the [Table of Contents](#table-of-contents).

### Phase 1: Edge Appliance (Mini PC) - Go & Python Apps

**See**: [IMPLEMENTATION_PLAN_PHASE1.md](IMPLEMENTATION_PLAN_PHASE1.md) for detailed implementation plan.

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

### Phase 2: User VM API Services (Docker Compose)

**See**: [IMPLEMENTATION_PLAN_PHASE2.md](IMPLEMENTATION_PLAN_PHASE2.md) for detailed implementation plan.

**Duration**: 1-2 weeks  
**Goal**: Build User VM API services in Go - WireGuard server, event cache, stream relay, MinIO integration (S3-compatible), AI model catalog, and secondary event analysis

**PoC Scope**: User VM API runs as a **Docker Compose service** in the local development environment. For PoC:
- **No SaaS components** - Edge Appliance and User VM API communicate directly
- **No Management Server** - Direct Edge ↔ User VM API communication
- **MinIO instead of Filecoin** - Use MinIO (S3-compatible) for remote storage
- **Docker Compose integration** - User VM API and MinIO run as services alongside Edge Appliance

**Status**: 0% complete

### Phase 3: SaaS Control Plane Backend

**See**: [IMPLEMENTATION_PLAN_PHASE3.md](IMPLEMENTATION_PLAN_PHASE3.md) for detailed implementation plan.

**Duration**: 2-3 weeks  
**Goal**: Build core SaaS backend services - authentication, event inventory, basic VM management

**Scope**: Simplified for PoC - manual VM provisioning, basic auth, essential event storage

**Status**: ⏸️ DEFERRED (not needed for PoC)

### Phase 4: SaaS UI Frontend

**See**: [IMPLEMENTATION_PLAN_PHASE4.md](IMPLEMENTATION_PLAN_PHASE4.md) for detailed implementation plan.

**Duration**: 2 weeks  
**Goal**: Build core React frontend - authentication, event timeline, basic clip viewing

**Scope**: Simplified UI for PoC - essential features only, no advanced configuration

**Status**: ⏸️ DEFERRED (not needed for PoC - Edge Web UI is sufficient)

### Phase 5: ISO Building & Deployment Automation

**See**: [IMPLEMENTATION_PLAN_PHASE5.md](IMPLEMENTATION_PLAN_PHASE5.md) for detailed implementation plan.

**Duration**: 1-2 weeks  
**Goal**: Basic ISO generation and simple deployment automation

**Scope**: Simplified for PoC - generic ISO or simple build script, manual deployment acceptable

**Status**: ⏸️ DEFERRED (not needed for PoC - Docker Compose is sufficient)

### Phase 6: Integration, Testing & Polish

**See**: [IMPLEMENTATION_PLAN_PHASE6.md](IMPLEMENTATION_PLAN_PHASE6.md) for detailed implementation plan.

**Duration**: 2 weeks  
**Goal**: End-to-end integration, essential testing, basic security, PoC demo preparation

**Scope**: Focus on integration and demo readiness, not full production hardening

**Status**: 0% complete

---
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

### Epic 1.6: WireGuard Communication (PoC: Direct, MVP/Production: libp2p + WireGuard)

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

#### Step 1.6.1: WireGuard Client (PoC - Direct Connection)
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

#### Step 1.6.4: WireGuard Client Service (PoC - Direct, MVP/Production - After Mesh Connection)
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

#### Step 1.6.5: gRPC Communication (Over WireGuard Tunnel)
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

#### Step 1.6.6: Event Transmission (Optional: Via Mesh Pubsub for MVP)
- **Substep 1.6.5.0**: Event pubsub via Mesh (MVP enhancement)
  - **Status**: ⬜ TODO
  - **P1**: **MVP**: Use Mesh `Publish` for async event notifications
  - **P1**: **MVP**: Subscribe to `/guardian/events` topic on User VM
  - **P1**: **MVP**: Keep gRPC for synchronous commands (request/response)
  - **P2**: **Production**: Full pubsub for event fanout, multi-version protocols
  - Location: `internal/events/mesh_publisher.go` (uses Mesh interface)

#### Step 1.6.7: Event Transmission (Current: gRPC)
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
  - **Status**: ✅ DONE
  - **P0**: Test system metrics collection (CPU, memory, disk, network) ✅
  - **P0**: Test application metrics (camera status, event queue length) ✅
  - **P0**: Test health status aggregation ✅
  - **P0**: Test periodic heartbeat transmission ✅
  - **P1**: Test telemetry batching and persistence ✅
  - Location: `internal/telemetry/collector_test.go`, `internal/telemetry/sender_test.go` ✅

### Epic 1.8: Encryption & Archive Client (Basic)

**Priority: P1** (Can be simplified for PoC)

#### Step 1.8.1: Encryption Service
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

### Epic 1.9: Edge Web UI (Local Network Accessible)

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

## Backend API Implementation

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

## Frontend UI Implementation

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

## Integration & Testing

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
  - **P0**: Reuse labeled screenshot capture UI to curate “normal” baseline datasets ✅
  - **P0**: Implement dataset export (ZIP + metadata manifest) for customer VM training ✅ (`/api/screenshots/export`, UI button)
  - **P1**: Support delta exports (only new screenshots since last export) ⬜ TODO
  - Location: `internal/web/frontend/src/pages/Screenshots.tsx`, `internal/web/screenshots/service.go`

- **Substep 1.9.14.2**: On-device inference and anomaly detection
  - **Status**: ✅ COMPLETE
  - **P0**: Load latest customer “normal” dataset and evaluate incoming frames per camera via local anomaly detector ✅
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

## Phase 2: User VM API Services (Docker Compose)

**Duration**: 1-2 weeks  
**Goal**: Build User VM API services in Go - WireGuard server, event cache, stream relay, MinIO integration (S3-compatible), AI model catalog, and secondary event analysis

**Note**: Duration estimate based on actual development velocity (Edge Phase 1 took ~2 days for ~88% completion). Phase 2 may be faster due to similar Go patterns and established architecture.

**PoC Scope**: User VM API runs as a **Docker Compose service** in the local development environment. For PoC:
- **No SaaS components** - Edge Appliance and User VM API communicate directly
- **No Management Server** - Direct Edge ↔ User VM API communication
- **MinIO instead of Filecoin** - Use MinIO (S3-compatible) for remote storage
- **Docker Compose integration** - User VM API and MinIO run as services alongside Edge Appliance

**Scope**: User VM API (open source) that runs in Docker Compose. The User VM API:
- Manages WireGuard tunnel termination for Edge Appliances
- Maintains AI model catalog (base models, customer-trained variants - basic for PoC)
- Receives event snapshots/clips, runs secondary analysis (basic for PoC)
- Persists long-term events/clips in MinIO (S3-compatible storage)
- Stream relay for on-demand clip viewing from Edge
- Event cache and telemetry aggregation

---

## Phase 2 Design: Anomaly Detection Model Training Pipeline

**Critical Design Decision**: Before implementing Phase 2, we must design the anomaly detection model training pipeline. This affects:
- Model architecture selection (what type of models to train)
- Python training service design (how to train models on user snapshots)
- Model packaging and distribution (how models are sent to Edge)
- Edge inference integration (how Edge uses trained models)

### 2.0.1: Model Architecture Design

**Problem**: We need models that can distinguish "normal" vs "unusual" situations from camera snapshots, not just object detection. This requires **anomaly detection** models, not classification models.

**Model Architecture Options**:

1. **Autoencoder-based Anomaly Detection** (Recommended for PoC):
   - **Architecture**: Convolutional Autoencoder (CAE)
   - **Training**: Train on "normal" labeled snapshots only
   - **Inference**: Reconstruct input frame → Calculate reconstruction error → High error = anomaly
   - **Advantages**: 
     - Only needs "normal" examples (no need for "threat" examples during training)
     - Works well for scene-level anomalies (blocked camera, unusual objects, etc.)
     - Can be trained per-camera for camera-specific normal scenes
   - **Output Format**: ONNX or OpenVINO IR (for Edge inference)
   - **Model Size**: ~5-20 MB (suitable for Edge deployment)

2. **Variational Autoencoder (VAE)** (Alternative):
   - Similar to autoencoder but with probabilistic encoding
   - Better uncertainty estimation
   - Slightly more complex

3. **One-Class SVM / Isolation Forest** (Not recommended):
   - Traditional ML, not deep learning
   - Requires feature extraction (less flexible)
   - Harder to deploy on Edge

**Selected Architecture for PoC**: **Convolutional Autoencoder (CAE)**

**Model Specifications**:
- **Input**: RGB image (e.g., 224x224 or 320x240, configurable per camera)
- **Encoder**: 3-4 convolutional layers + pooling (reduces to latent space)
- **Decoder**: 3-4 transposed convolutional layers (reconstructs image)
- **Latent Space**: 128-256 dimensions (compressed representation)
- **Loss Function**: Mean Squared Error (MSE) or Perceptual Loss
- **Output**: Reconstruction error (scalar) + reconstructed image (for visualization)
- **Threshold**: Configurable per-camera (default: 0.05 reconstruction error)

**Training Data Requirements**:
- **Minimum**: 50-100 "normal" labeled snapshots per camera
- **Recommended**: 200-500 "normal" snapshots per camera
- **Format**: JPEG images, organized by label (`normal/`, `threat/`, `abnormal/`)
- **Preprocessing**: Resize to model input size, normalize pixel values

### 2.0.2: Python Training Service Design

**Component**: `user-vm-api/training-service/` (Python service, separate from Go services)

**Architecture**:
```
user-vm-api/
├── training-service/          # Python training service
│   ├── Dockerfile            # Python 3.10+ with PyTorch/ONNX
│   ├── requirements.txt      # PyTorch, torchvision, onnx, opencv-python, etc.
│   ├── main.py              # Training service entry point (HTTP/gRPC server)
│   ├── models/
│   │   ├── autoencoder.py   # CAE model definition
│   │   └── base_models.py   # Pre-trained base models
│   ├── training/
│   │   ├── trainer.py       # Training loop
│   │   ├── dataset.py       # Dataset loader (from dataset storage)
│   │   └── metrics.py       # Training metrics (loss, validation error)
│   ├── export/
│   │   ├── onnx_exporter.py # Export to ONNX format
│   │   └── openvino_exporter.py # Export to OpenVINO IR (optional)
│   └── config/
│       └── training_config.yaml # Training hyperparameters
```

**Training Service API** (HTTP REST or gRPC):
- `POST /train` - Start training job
  - Input: `{dataset_id, camera_id, model_config, hyperparameters}`
  - Output: `{job_id, status}`
- `GET /train/{job_id}` - Get training job status
  - Output: `{status, progress, metrics, model_path}`
- `GET /train/{job_id}/metrics` - Get training metrics
  - Output: `{epoch, loss, validation_error, learning_rate}`

**Training Workflow**:
1. User VM API (Go) receives dataset export from Edge
2. User VM API stores dataset in `datasets/{dataset_id}/{label}/`
3. User VM API creates training job, calls Python training service
4. Python service:
   - Loads "normal" images from `datasets/{dataset_id}/normal/`
   - Trains CAE model on normal images
   - Validates on held-out normal images
   - Exports trained model to ONNX format
   - Saves model to `models/{model_id}/model.onnx`
5. User VM API updates model catalog, distributes to Edge

**Training Hyperparameters** (Configurable):
- Learning rate: 0.001 (Adam optimizer)
- Batch size: 16-32 (depending on GPU memory)
- Epochs: 50-100 (early stopping if validation loss plateaus)
- Image size: 224x224 or 320x240 (per camera)
- Latent dimension: 128-256
- Loss function: MSE or Perceptual Loss

### 2.0.3: Model Packaging & Distribution

**Model Format**: ONNX (Open Neural Network Exchange)
- **Why ONNX**: 
  - Standard format, works with OpenVINO (Edge inference)
  - Can be converted to OpenVINO IR on Edge if needed
  - Smaller file size than PyTorch checkpoints
- **Model Metadata**:
  - Model version, training date, dataset ID
  - Input/output shapes, normalization parameters
  - Anomaly threshold (reconstruction error threshold)
  - Camera ID (if per-camera model)

**Distribution Flow**:
1. Python training service exports model to `models/{model_id}/model.onnx`
2. User VM API (Go) packages model + metadata:
   - Model file: `model.onnx`
   - Metadata: `metadata.json` (version, threshold, camera_id, etc.)
3. User VM API pushes to Edge via WireGuard (gRPC):
   - Stream model file over gRPC
   - Edge stores in `{data_dir}/models/{model_id}/model.onnx`
   - Edge updates model registry
4. Edge AI service loads model for inference

### 2.0.4: Edge Inference Integration

**Edge AI Service** (Python, existing):
- **Current**: Basic brightness-based anomaly detection
- **New**: Load trained CAE models from User VM
- **Inference Flow**:
  1. Receive frame from camera
  2. Preprocess: Resize to model input size, normalize
  3. Run CAE inference: `reconstructed = model.encode_decode(frame)`
  4. Calculate reconstruction error: `error = mse(frame, reconstructed)`
  5. Compare to threshold: `if error > threshold: trigger_event()`
  6. If anomaly: Capture snapshot, record clip, enqueue event

**Model Loading**:
- Edge AI service monitors `{data_dir}/models/` directory
- On new model file: Load ONNX model (using ONNX Runtime or OpenVINO)
- Per-camera model assignment: Each camera can have its own trained model
- Model versioning: Edge keeps previous model until new one is validated

**Inference Performance**:
- Target: <100ms per frame (10 FPS minimum)
- Hardware: CPU (Intel N100) or iGPU (Intel QSV) if available
- Optimization: OpenVINO IR format (faster than ONNX Runtime on Intel)

### 2.0.5: Training Pipeline Integration Points

**User VM API (Go) ↔ Python Training Service**:
- **Communication**: HTTP REST API (simple) or gRPC (more efficient)
- **Job Management**: User VM API tracks training jobs in SQLite
- **File Access**: Python service reads from `datasets/{dataset_id}/` (shared filesystem)
- **Model Storage**: Python service writes to `models/{model_id}/` (shared filesystem)

**Docker Compose Setup**:
```yaml
services:
  user-vm-api:
    # Go services
    ...
  
  training-service:
    build: ./training-service
    volumes:
      - ./data/datasets:/app/datasets  # Shared dataset storage
      - ./data/models:/app/models      # Shared model storage
    environment:
      - TRAINING_DATA_DIR=/app/datasets
      - MODEL_OUTPUT_DIR=/app/models
    ports:
      - "8082:8080"  # Training service API
```

**Training Service Dependencies**:
- Python 3.10+
- PyTorch 2.0+ (or TensorFlow if preferred)
- ONNX / onnxruntime
- OpenCV (image preprocessing)
- NumPy, PIL/Pillow

---

**Next Steps**: This design should be implemented in Epic 2.8 (AI Model Orchestrator & Training Pipeline) with the following structure:
- Step 2.8.1: Model Catalog Management (Go)
- Step 2.8.2: Dataset Ingestion & Training Pipeline (Go + Python service)
- Step 2.8.3: Model Distribution to Edge (Go)
- Step 2.8.4: Python Training Service Implementation (NEW)
- Step 2.8.5: Edge Inference Integration (Python AI service update)

**Note**: Edge still performs first-level capture/recording, but User VM API handles event caching, stream relay, and remote archival to MinIO over the secure WireGuard channel.

**Note**: Milestone 1 (first full event flow) will be achieved at the end of Phase 2, after User VM API is integrated with Edge Appliance.

### Epic 2.1: User VM API Project Setup

**Priority: P0**

**Priority: P0**

#### Step 2.1.1: Project Structure
- **Substep 2.1.1.1**: Create User VM API directory structure
  - **Status**: ⬜ TODO
  - Note: User VM API is public/open source (developed directly in meta repo, secrets in memory only)
  - `user-vm-api/` - Main API services
  - `user-vm-api/cmd/server/` - Main server entry point
  - `user-vm-api/internal/wireguard-server/` - WireGuard server service
  - `user-vm-api/internal/event-cache/` - Event cache service
  - `user-vm-api/internal/stream-relay/` - Stream relay service
  - `user-vm-api/internal/storage-sync/` - Storage sync service (MinIO/S3 for PoC, Filecoin bridge post-PoC)
  - `user-vm-api/internal/ai-orchestrator/` - AI model catalog and training
  - `user-vm-api/internal/event-analyzer/` - Secondary event analysis
  - `user-vm-api/internal/telemetry-aggregator/` - Telemetry aggregation
  - `user-vm-api/internal/orchestrator/` - Main orchestrator service
  - `user-vm-api/internal/shared/` - Shared libraries (config, logging, service base)
  - `user-vm-api/config/` - Configuration files
  - `user-vm-api/scripts/` - Build and deployment scripts
  - Note: gRPC proto definitions are in meta repo `proto/proto/edge/` (Edge ↔ User Server) and `proto/proto/kvm/` (User Server ↔ Management Server), imported as Go modules
- **Substep 2.1.1.2**: Go modules setup
  - **Status**: ✅ DONE
  - Initialize Go modules (`go.mod`, `go.sum`)
  - Import `proto/go` from meta repo as Go module dependency
  - Import `crypto/go` from meta repo (if needed for encryption verification)
  - Dependency management (WireGuard, gRPC, SQLite, etc.)
  - Shared libraries structure
  - Location: `user-vm-api/go.mod`
- **Substep 2.1.1.3**: Set up CI/CD basics
  - **Status**: ✅ DONE
  - GitHub Actions for User VM API (in meta repo)
  - Docker image builds for Go service
  - Linting and basic tests
  - Location: `.github/workflows/user-vm-api.yml`

#### Step 2.1.2: Local Development Environment
- **Substep 2.1.2.1**: Development tooling setup
  - **Status**: ⬜ TODO
  - Install Go 1.25+ (as per TECHNICAL_STACK.md)
  - Set up code formatters (gofmt, goimports)
  - Configure linters (golangci-lint)
  - Set up pre-commit hooks
- **Substep 2.1.2.2**: Local testing environment
  - **Status**: ⬜ TODO
  - Docker Compose for local services (if needed)
  - Mock Edge Appliance setup (for testing WireGuard connections)
  - Local SQLite database setup
  - Test WireGuard tunnel setup
- **Substep 2.1.2.3**: IDE configuration
  - **Status**: ⬜ TODO
  - VS Code / Cursor workspace settings
  - Debugging configurations for Go
  - Code snippets

#### Step 2.1.3: Database & Storage Setup
- **Substep 2.1.3.1**: SQLite schema design
  - **Status**: ⬜ TODO
  - Event cache table (event_id, edge_id, camera_id, timestamp, event_type, metadata, snapshot_path, clip_path, analyzed, severity, created_at, updated_at)
  - Edge Appliance registry (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at)
  - AI model catalog (model_id, name, version, type, base_model, training_dataset_id, model_file_path, status, created_at, updated_at)
  - Training datasets (dataset_id, name, edge_id, dataset_dir_path, label_counts, total_images, status, created_at, updated_at)
  - CID storage table (cid_id, event_id, clip_path, cid, storage_provider, size_bytes, uploaded_at, retention_until)
  - Telemetry buffer table (telemetry_id, edge_id, timestamp, metrics_json, forwarded, created_at)
  - Location: `internal/shared/database/schema.go`
- **Substep 2.1.3.2**: Database migration system
  - **Status**: ⬜ TODO
  - Migration tool setup (golang-migrate or custom)
  - Initial migrations (create all tables)
  - Migration rollback support
  - Migration versioning
  - Location: `internal/shared/database/migrations/`
- **Substep 2.1.3.3**: Database connection management
  - **Status**: ⬜ TODO
  - SQLite connection pool setup
  - Connection health checks
  - Database initialization
  - Location: `internal/shared/database/db.go`
- **Substep 2.1.3.4**: File storage system for training datasets
  - **Status**: ⬜ TODO
  - **P0**: Create dataset storage directory structure:
    - Base directory: `{data_dir}/datasets/`
    - Per-dataset structure: `datasets/{dataset_id}/`
    - Label-based subdirectories: `datasets/{dataset_id}/{label}/` (e.g., `normal/`, `threat/`, `abnormal/`, `custom/`)
    - Image files stored in label subdirectories: `datasets/{dataset_id}/{label}/image_{id}.jpg`
  - **P0**: Dataset storage service:
    - Create dataset directory structure
    - Store received snapshot images from Edge exports
    - Organize images by label
    - Track dataset size and file counts
    - Dataset cleanup and deletion
  - **P0**: Storage quota management:
    - Per-dataset size limits
    - Total storage quota for all datasets
    - Automatic cleanup of old datasets
  - **P0**: Dataset export/import:
    - Export dataset as ZIP archive (for training service)
    - Import dataset from Edge export (ZIP or directory)
    - Validate dataset structure and images
  - **P1**: Dataset versioning:
    - Track dataset versions
    - Support dataset snapshots
  - Location: `internal/shared/storage/dataset_storage.go`
- **Substep 2.1.3.5**: Model file storage
  - **Status**: ⬜ TODO
  - **P0**: Create model storage directory structure:
    - Base directory: `{data_dir}/models/`
    - Per-model structure: `models/{model_id}/`
    - Model files: `models/{model_id}/model.onnx` (or `.pt`, `.tflite`, etc.)
    - Model metadata: `models/{model_id}/metadata.json`
  - **P0**: Model storage service:
    - Store trained model files
    - Track model versions
    - Model file retrieval for distribution to Edge
    - Model cleanup and deletion
  - **P0**: Storage quota management:
    - Per-model size limits
    - Total storage quota for all models
  - Location: `internal/shared/storage/model_storage.go`
- **Substep 2.1.3.6**: Unit tests for User VM API project setup
  - **Status**: ⬜ TODO
  - **P0**: Test database schema initialization
  - **P0**: Test database migration system
  - **P0**: Test database connection management
  - **P0**: Test dataset storage service (directory creation, image storage, organization by label)
  - **P0**: Test dataset export/import
  - **P0**: Test model storage service (model file storage, versioning)
  - **P0**: Test storage quota management
  - **P0**: Test Go module dependencies
  - **P1**: Test CI/CD pipeline
  - Location: `internal/shared/database/db_test.go`, `internal/shared/database/migrations/migrations_test.go`, `internal/shared/storage/dataset_storage_test.go`, `internal/shared/storage/model_storage_test.go`

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
- **Substep 2.3.2.2**: Event storage (PoC - no SaaS)
  - **Status**: ⬜ TODO
  - **P0**: Store events in SQLite event cache (no SaaS forwarding in PoC)
  - **P0**: Event querying and retrieval for Edge Web UI
  - **P2**: SaaS communication (gRPC client, event forwarding - post-PoC)
  - **P2**: Handle forwarding failures and retries (post-PoC)
  - **P2**: Acknowledgment handling (post-PoC)
- **Substep 2.3.2.3**: Unit tests for event cache service
  - **Status**: ⬜ TODO
  - **P0**: Test event reception from Edge (gRPC server)
  - **P0**: Test event validation and storage
  - **P0**: Test event cache management (querying, expiration, cleanup)
  - **P0**: Test event summarization (privacy-minimized metadata)
  - **P0**: Test event storage and retrieval (no SaaS in PoC)
  - **P2**: Test event forwarding to SaaS (gRPC client, retries, acknowledgments - post-PoC)

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
- **Substep 2.4.2.2**: Telemetry storage (PoC - no SaaS)
  - **Status**: ⬜ TODO
  - **P0**: Store telemetry in SQLite buffer (no SaaS forwarding in PoC)
  - **P0**: Telemetry querying for Edge Web UI
  - **P2**: Forward to SaaS (gRPC client, periodic reporting - post-PoC)
  - **P2**: Advanced alert forwarding (post-PoC)
- **Substep 2.4.2.3**: Unit tests for telemetry aggregation service
  - **Status**: ⬜ TODO
  - **P0**: Test telemetry reception and validation
  - **P0**: Test telemetry aggregation (healthy/unhealthy status)
  - **P0**: Test telemetry summarization
  - **P0**: Test telemetry storage and retrieval (no SaaS in PoC)
  - **P2**: Test telemetry forwarding to SaaS (post-PoC)
  - **P1**: Test advanced metrics aggregation

### Epic 2.5: Stream Relay Service

**Priority: P0**

#### Step 2.5.1: Stream Request Handling
- **Substep 2.5.1.1**: Stream request handling (PoC - no SaaS tokens)
  - **Status**: ⬜ TODO
  - **P0**: Receive stream requests directly from Edge Web UI (no SaaS tokens in PoC)
  - **P0**: Validate event ID and basic authorization
  - **P2**: Token validation (receive time-bound tokens from SaaS - post-PoC)
  - **P2**: Validate token signature and expiration (post-PoC)
  - **P2**: Extract event ID and user info from token (post-PoC)
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
  - **P0**: Test stream request handling (no SaaS tokens in PoC)
  - **P2**: Test token validation (post-PoC)
  - **P0**: Test stream orchestration (request clip from Edge)
  - **P0**: Test HTTP-based relay (progressive download)
  - **P1**: Test WebRTC relay (if implemented)

### Epic 2.6: Storage Sync Service (MinIO/S3 for PoC)

**Priority: P0** (MinIO integration for PoC, Filecoin bridge post-PoC)

**Note**: For PoC, we use **MinIO** (S3-compatible storage) running in Docker Compose. The User VM API uses the **AWS Go SDK** to communicate with MinIO. Post-PoC, we'll develop an S3-Filecoin bridge to migrate from MinIO to Filecoin.

**Storage Organization**: Each camera has its own MinIO bucket for organizing event frames and clips:
- Bucket naming: `camera-{camera_id}` (e.g., `camera-rtsp-192.168.1.100`, `camera-usb-usb-3-9`)
- Event frames stored as: `events/{event_id}/snapshot.jpg`
- Clips stored as: `events/{event_id}/clip.mp4`
- Metadata stored as: `events/{event_id}/metadata.json`

#### Step 2.6.1: MinIO Integration (PoC)
- **Substep 2.6.1.1**: AWS Go SDK setup
  - **Status**: ⬜ TODO
  - **P0**: Import AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2`)
  - **P0**: Configure S3 client with MinIO endpoint
  - **P0**: Credential configuration (access key, secret key)
  - **P0**: Endpoint configuration (MinIO URL, disable SSL for PoC)
  - **P0**: Region configuration
  - Location: `internal/storage-sync/s3_client.go`
- **Substep 2.6.1.2**: Bucket management (per-camera buckets)
  - **Status**: ⬜ TODO
  - **P0**: Create bucket for each camera on first event/clip upload
  - **P0**: Bucket naming: `camera-{camera_id}` (sanitized)
  - **P0**: Check bucket existence before operations
  - **P0**: Handle bucket creation errors gracefully
  - **P1**: Bucket lifecycle policies (retention, cleanup)
  - Location: `internal/storage-sync/bucket_manager.go`
- **Substep 2.6.1.3**: Encrypted clip reception and upload
  - **Status**: ⬜ TODO
  - **P0**: Receive encrypted clips from Edge (already encrypted)
  - **P0**: Store temporarily during upload
  - **P0**: Upload encrypted clips to camera-specific MinIO bucket
  - **P0**: Object key format: `events/{event_id}/clip.mp4`
  - **P0**: Automatic cleanup of temporary files after upload
  - Location: `internal/storage-sync/clip_uploader.go`
- **Substep 2.6.1.4**: Event frame/snapshot upload
  - **Status**: ⬜ TODO
  - **P0**: Receive event frames/snapshots from Edge
  - **P0**: Upload to camera-specific MinIO bucket
  - **P0**: Object key format: `events/{event_id}/snapshot.jpg`
  - **P0**: Support multiple snapshots per event
  - Location: `internal/storage-sync/snapshot_uploader.go`
- **Substep 2.6.1.5**: Metadata storage
  - **Status**: ⬜ TODO
  - **P0**: Store event metadata as JSON in MinIO bucket
  - **P0**: Object key format: `events/{event_id}/metadata.json`
  - **P0**: Include event type, timestamp, camera ID, detection details
  - **P0**: Associate metadata with clips and snapshots
  - Location: `internal/storage-sync/metadata_uploader.go`
- **Substep 2.6.1.6**: Object key storage (replacing CID storage)
  - **Status**: ⬜ TODO
  - **P0**: Store MinIO object keys in SQLite (replacing CID storage for PoC)
  - **P0**: Associate object keys with events
  - **P0**: Query events by camera, date range, event type
  - **P2**: CID storage (for Filecoin post-PoC)
  - Location: `internal/storage-sync/metadata.go`

#### Step 2.6.2: Quota Management (Basic)
- **Substep 2.6.2.1**: Simple quota tracking (per-camera)
  - **Status**: ⬜ TODO
  - **P0**: Hard-coded quota limit per camera for PoC
  - **P0**: Track archive size per camera bucket
  - **P0**: Calculate bucket size using AWS SDK (ListObjectsV2, sum sizes)
  - **P0**: Store quota usage in SQLite
  - **P2**: Complex quota policies from SaaS (post-PoC)
- **Substep 2.6.2.2**: Basic quota enforcement
  - **Status**: ⬜ TODO
  - **P0**: Check quota before upload (per camera bucket)
  - **P0**: Reject uploads if camera bucket over quota
  - **P0**: Quota calculation includes clips, snapshots, and metadata
  - **P1**: Quota warnings (e.g., 80% threshold)
  - **P2**: Advanced quota management (post-PoC)

#### Step 2.6.3: Archive Metadata Management
- **Substep 2.6.3.1**: Object key storage (MinIO per-camera buckets)
  - **Status**: ⬜ TODO
  - **P0**: Store MinIO object keys in SQLite (replacing CID storage for PoC)
  - **P0**: Associate object keys with events and camera buckets
  - **P0**: Store bucket name, object key, size, upload timestamp
  - **P0**: Query objects by camera, event ID, date range
  - **P1**: Basic metadata storage
  - **P2**: CID storage (for Filecoin post-PoC)
  - Location: `internal/storage-sync/metadata.go`
- **Substep 2.6.3.2**: Archive status tracking
  - **Status**: ⬜ TODO
  - **P0**: Track archive status locally (no SaaS in PoC)
  - **P0**: Store archive metadata in SQLite (per camera)
  - **P0**: Track upload status (pending, uploading, completed, failed)
  - **P0**: Retry failed uploads
  - **P2**: Archive status updates to SaaS (post-PoC)
  - **P2**: CID transmission to SaaS (post-PoC)
  - Location: `internal/storage-sync/status_tracker.go`
- **Substep 2.6.3.3**: Clip and snapshot retrieval
  - **Status**: ⬜ TODO
  - **P0**: Retrieve clips from MinIO using AWS SDK (GetObject)
  - **P0**: Retrieve snapshots from MinIO using AWS SDK
  - **P0**: Stream clips/snapshots to Edge Web UI or stream relay
  - **P0**: Handle missing objects gracefully
  - **P0**: Support range requests for partial downloads
  - Location: `internal/storage-sync/retriever.go`
- **Substep 2.6.3.4**: Unit tests for storage sync service
  - **Status**: ⬜ TODO
  - **P0**: Test AWS SDK client setup and MinIO connection
  - **P0**: Test bucket creation and management (per-camera buckets)
  - **P0**: Test encrypted clip upload to camera bucket
  - **P0**: Test snapshot upload to camera bucket
  - **P0**: Test metadata upload and retrieval
  - **P0**: Test quota tracking and enforcement (per camera)
  - **P0**: Test object key storage and retrieval
  - **P0**: Test clip/snapshot retrieval from MinIO
  - **P0**: Test archive status tracking
  - Location: `internal/storage-sync/*_test.go`

### Epic 2.7: User VM API Orchestration

**Priority: P0**

#### Step 2.7.1: Orchestrator Service Framework
- **Substep 2.7.1.1**: Main orchestrator service
  - **Status**: ⬜ TODO
  - Service initialization and startup
  - Configuration management (YAML/JSON config)
  - Logging setup (structured JSON logging)
  - Graceful shutdown handling
  - Location: `internal/orchestrator/server.go`
- **Substep 2.7.1.2**: Service manager pattern
  - **Status**: ⬜ TODO
  - Service lifecycle management
  - Service registration and discovery
  - Inter-service communication (channels/events)
  - Service dependency injection
  - Location: `internal/orchestrator/manager.go`
- **Substep 2.7.1.3**: Health check system
  - **Status**: ⬜ TODO
  - Health check endpoints (HTTP/gRPC)
  - Service status reporting
  - Dependency health checks (database, WireGuard, MinIO connection)
  - Location: `internal/orchestrator/health.go`
- **Substep 2.7.1.4**: Unit tests for orchestrator service framework
  - **Status**: ⬜ TODO
  - **P0**: Test service initialization and shutdown
  - **P0**: Test service manager lifecycle
  - **P0**: Test health check system
  - **P0**: Test configuration management
  - **P1**: Test inter-service communication
  - Location: `internal/orchestrator/server_test.go`, `internal/orchestrator/manager_test.go`

#### Step 2.7.2: Management Server Communication
- **Substep 2.7.2.1**: gRPC client to Management Server
  - **Status**: ⬜ TODO
  - **P0**: Connection setup to Management Server
  - **P0**: mTLS configuration
  - **P0**: Connection health monitoring and reconnection
  - **P0**: Connection retry logic
  - Location: `internal/orchestrator/management_client.go`
- **Substep 2.7.2.2**: Command handling from Management Server
  - **Status**: ⬜ TODO
  - **P0**: Receive commands from Management Server (update, restart, config changes)
  - **P0**: Command validation and execution
  - **P0**: Command acknowledgment
  - **P1**: Advanced command orchestration (rollback, staged updates)
  - Location: `internal/orchestrator/command_handler.go`
- **Substep 2.7.2.3**: Status reporting to Management Server
  - **Status**: ⬜ TODO
  - **P0**: Periodic status reports (health, metrics summary)
  - **P0**: Event forwarding coordination
  - **P0**: Telemetry aggregation forwarding
  - Location: `internal/orchestrator/status_reporter.go`
- **Substep 2.7.2.4**: Unit tests for Docker Compose integration
  - **Status**: ⬜ TODO
  - **P0**: Test Docker Compose service startup and health checks
  - **P0**: Test networking between Edge and User VM API
  - **P0**: Test MinIO integration
  - **P0**: Test WireGuard tunnel setup in Docker Compose
  - **P2**: Test Management Server communication (post-PoC)
  - Location: `infra/local/` (integration tests)

### Epic 2.8: AI Model Orchestrator & Training Pipeline

**Priority: P0**

**Note**: This epic implements the anomaly detection model training pipeline designed in Section 2.0. It includes:
- **Model Architecture**: Convolutional Autoencoder (CAE) for anomaly detection (normal vs unusual)
- **Python Training Service**: Separate Python service for model training (PyTorch/ONNX)
- **Model Distribution**: ONNX model format, distributed to Edge via WireGuard
- **Edge Inference**: Edge AI service loads and runs trained CAE models for real-time anomaly detection

**Architecture Overview**:
1. Edge exports labeled snapshots (normal/threat/abnormal) → User VM API
2. User VM API stores dataset in `datasets/{dataset_id}/{label}/`
3. User VM API triggers Python training service to train CAE model on "normal" images
4. Python service trains model, exports to ONNX, saves to `models/{model_id}/model.onnx`
5. User VM API distributes trained model to Edge via WireGuard
6. Edge AI service loads model, runs inference on incoming frames
7. High reconstruction error → anomaly detected → event triggered

#### Step 2.8.1: AI Model Catalog Management
- **Substep 2.8.1.1**: Model registry service
  - **Status**: ⬜ TODO
  - **P0**: Maintain registry of base models (YOLOv8, custom models)
  - **P0**: Store customer-specific fine-tuned models
  - **P0**: Model versioning and metadata storage
  - **P0**: Model status tracking (active, training, archived)
  - Location: `internal/ai-orchestrator/catalog.go`
- **Substep 2.8.1.2**: Model storage management
  - **Status**: ⬜ TODO
  - **P0**: Model file storage (local filesystem or object storage)
  - **P0**: Model metadata database storage
  - **P0**: Model file integrity verification
  - **P1**: Model compression and optimization
  - Location: `internal/ai-orchestrator/storage.go`
- **Substep 2.8.1.3**: Unit tests for model catalog
  - **Status**: ⬜ TODO
  - **P0**: Test model registry operations (add, update, query, delete)
  - **P0**: Test model versioning
  - **P0**: Test model storage management
  - **P1**: Test model integrity verification
  - Location: `internal/ai-orchestrator/catalog_test.go`

#### Step 2.8.2: Dataset Ingestion & Training Pipeline
- **Substep 2.8.2.1**: Dataset reception from Edge
  - **Status**: ⬜ TODO
  - **P0**: Receive labeled screenshot datasets exported from Edge (ZIP archives or directory structure)
  - **P0**: Extract and store dataset images using dataset storage service (`datasets/{dataset_id}/{label}/`)
  - **P0**: Dataset validation (format, labels, structure, image integrity)
  - **P0**: Index dataset in SQLite (metadata: dataset_id, label_counts, total_images, dataset_dir_path)
  - **P0**: Dataset metadata tracking (label counts, size, export date)
  - Location: `internal/ai-orchestrator/dataset.go` (uses `internal/shared/storage/dataset_storage.go`)
- **Substep 2.8.2.2**: Training job queue
  - **Status**: ⬜ TODO
  - **P0**: Training job creation and queuing
  - **P0**: Training job status tracking
  - **P0**: Support incremental training (baseline + customer-labeled data)
  - **P1**: Training job prioritization
  - **P1**: Training job scheduling and resource management
  - Location: `internal/ai-orchestrator/training_queue.go`
- **Substep 2.8.2.3**: Model training execution (Go orchestrator)
  - **Status**: ⬜ TODO
  - **P0**: Integration with Python training service (HTTP REST or gRPC)
  - **P0**: Training job creation: Call Python service `/train` endpoint with `{dataset_id, camera_id, config}`
  - **P0**: Training job execution and monitoring (poll Python service for status)
  - **P0**: Training metrics collection (loss, validation error, epoch progress)
  - **P0**: Model artifact retrieval: Python service saves to `models/{model_id}/model.onnx`
  - **P0**: Update model catalog with trained model metadata
  - **P1**: Training history and metrics tracking per tenant
  - Location: `internal/ai-orchestrator/trainer.go` (Go orchestrator, calls Python service)

#### Step 2.8.4: Python Training Service Implementation (NEW)
- **Substep 2.8.4.1**: Python training service setup
  - **Status**: ⬜ TODO
  - **P0**: Create `user-vm-api/training-service/` directory structure
  - **P0**: Python 3.10+ Dockerfile with PyTorch, ONNX, OpenCV dependencies
  - **P0**: HTTP REST API server (Flask/FastAPI) or gRPC server
  - **P0**: Training service configuration (data dirs, model output dir, hyperparameters)
  - Location: `user-vm-api/training-service/`
- **Substep 2.8.4.2**: CAE model implementation
  - **Status**: ⬜ TODO
  - **P0**: Implement Convolutional Autoencoder (CAE) model in PyTorch
  - **P0**: Encoder: 3-4 conv layers + pooling (224x224 → 128-256 dim latent)
  - **P0**: Decoder: 3-4 transposed conv layers (latent → 224x224 reconstruction)
  - **P0**: Configurable input size (224x224 or 320x240)
  - **P0**: Configurable latent dimension (128-256)
  - Location: `user-vm-api/training-service/models/autoencoder.py`
- **Substep 2.8.4.3**: Training pipeline implementation
  - **Status**: ⬜ TODO
  - **P0**: Dataset loader: Load "normal" images from `datasets/{dataset_id}/normal/`
  - **P0**: Data preprocessing: Resize, normalize, augment (optional)
  - **P0**: Training loop: Train CAE on normal images (MSE loss)
  - **P0**: Validation: Calculate reconstruction error on held-out normal images
  - **P0**: Early stopping: Stop if validation loss plateaus
  - **P0**: Hyperparameters: Learning rate, batch size, epochs (configurable)
  - Location: `user-vm-api/training-service/training/trainer.py`
- **Substep 2.8.4.4**: Model export to ONNX
  - **Status**: ⬜ TODO
  - **P0**: Export trained PyTorch model to ONNX format
  - **P0**: Save model to `models/{model_id}/model.onnx`
  - **P0**: Generate model metadata JSON (version, threshold, camera_id, input_shape)
  - **P0**: Validate exported ONNX model (test inference)
  - Location: `user-vm-api/training-service/export/onnx_exporter.py`
- **Substep 2.8.4.5**: Training service API
  - **Status**: ⬜ TODO
  - **P0**: `POST /train` endpoint: Start training job
    - Input: `{dataset_id, camera_id, model_config, hyperparameters}`
    - Output: `{job_id, status}`
  - **P0**: `GET /train/{job_id}` endpoint: Get training status
    - Output: `{status, progress, metrics, model_path}`
  - **P0**: `GET /train/{job_id}/metrics` endpoint: Get training metrics
    - Output: `{epoch, loss, validation_error, learning_rate}`
  - **P0**: Background training job execution (async)
  - Location: `user-vm-api/training-service/main.py`
- **Substep 2.8.4.6**: Unit tests for Python training service
  - **Status**: ⬜ TODO
  - **P0**: Test CAE model forward pass
  - **P0**: Test training loop (mock dataset)
  - **P0**: Test ONNX export
  - **P0**: Test training API endpoints
  - Location: `user-vm-api/training-service/tests/`
- **Substep 2.8.2.4**: Unit tests for training pipeline
  - **Status**: ⬜ TODO
  - **P0**: Test dataset reception and validation
  - **P0**: Test training job queue operations
  - **P0**: Test training job execution (mock training service)
  - **P1**: Test training metrics collection
  - Location: `internal/ai-orchestrator/dataset_test.go`, `internal/ai-orchestrator/training_queue_test.go`

#### Step 2.8.3: Model Distribution to Edge
- **Substep 2.8.3.1**: Model packaging and versioning
  - **Status**: ⬜ TODO
  - **P0**: Package trained models (ONNX, OpenVINO IR format)
  - **P0**: Model versioning and tagging
  - **P0**: Model metadata generation (version, training date, dataset info)
  - **P0**: Store trained model files using model storage service (`models/{model_id}/model.onnx`)
  - **P0**: Update model catalog in SQLite with model file path
  - **P1**: Model compression for efficient transfer
  - Location: `internal/ai-orchestrator/packager.go` (uses `internal/shared/storage/model_storage.go`)
- **Substep 2.8.3.2**: Model push to Edge via WireGuard
  - **Status**: ⬜ TODO
  - **P0**: Retrieve model file from model storage service (`models/{model_id}/model.onnx`)
  - **P0**: Push model files to Edge AI service via WireGuard tunnel (gRPC)
  - **P0**: Model transfer progress tracking
  - **P0**: Transfer verification and integrity checks
  - **P0**: Model activation on Edge (acknowledgment/rollback)
  - Location: `internal/ai-orchestrator/deployer.go` (uses `internal/shared/storage/model_storage.go`)
- **Substep 2.8.3.3**: Model deployment management
  - **Status**: ⬜ TODO
  - **P0**: Track model deployment status per Edge Appliance
  - **P0**: Rollback support (revert to previous model version)
  - **P1**: Staged/blue-green deployment of models
  - **P1**: A/B testing support (deploy different models to different Edge Appliances)
  - Location: `internal/ai-orchestrator/deployment.go`
- **Substep 2.8.3.4**: Unit tests for model distribution
  - **Status**: ⬜ TODO
  - **P0**: Test model packaging and versioning
  - **P0**: Test model push to Edge (mock Edge service)
  - **P0**: Test model activation and rollback
  - **P1**: Test staged deployment
  - Location: `internal/ai-orchestrator/deployer_test.go`, `internal/ai-orchestrator/deployment_test.go`

### Epic 2.9: Secondary Event Analysis & Alerting

**Priority: P0**

#### Step 2.9.1: Event Reception & Analysis Pipeline
- **Substep 2.9.1.1**: Event snapshot/clip reception
  - **Status**: ⬜ TODO
  - **P0**: Receive event snapshots and clips from Edge via WireGuard/gRPC
  - **P0**: Validate event data structure
  - **P0**: Store event media temporarily for analysis
  - **P0**: Enforce WireGuard-only data plane (reject non-WireGuard connections)
  - Location: `internal/event-analyzer/receiver.go`
- **Substep 2.9.1.2**: Secondary inference service
  - **Status**: ⬜ TODO
  - **P0**: Run higher-accuracy inference on event snapshots/clips
  - **P0**: Use trained models from catalog for analysis
  - **P0**: Generate detection results with confidence scores
  - **P0**: Compare with Edge's initial detection results
  - **P1**: Multi-model ensemble analysis
  - Location: `internal/event-analyzer/inference.go`
- **Substep 2.9.1.3**: Severity classification
  - **Status**: ⬜ TODO
  - **P0**: Classify event severity (critical, warning, normal, false_positive)
  - **P0**: Decision logic based on inference results and thresholds
  - **P0**: Persist VM-side event verdicts in database
  - **P1**: Adaptive threshold adjustment based on feedback
  - Location: `internal/event-analyzer/classifier.go`
- **Substep 2.9.1.4**: Unit tests for event analysis
  - **Status**: ⬜ TODO
  - **P0**: Test event reception and validation
  - **P0**: Test secondary inference (mock inference service)
  - **P0**: Test severity classification logic
  - **P0**: Test event verdict persistence
  - Location: `internal/event-analyzer/receiver_test.go`, `internal/event-analyzer/classifier_test.go`

#### Step 2.9.2: Alert Generation & Forwarding
- **Substep 2.9.2.1**: Alert generation
  - **Status**: ⬜ TODO
  - **P0**: Generate alerts for critical events
  - **P0**: Alert metadata (event_id, severity, timestamp, camera_id)
  - **P0**: Alert deduplication (prevent duplicate alerts for same event)
  - **P1**: Alert aggregation (group related events)
  - Location: `internal/event-analyzer/alert.go`
- **Substep 2.9.2.2**: Alert forwarding to Management Server
  - **Status**: ⬜ TODO
  - **P0**: Forward alerts to Management Server via gRPC
  - **P0**: Alert acknowledgment handling
  - **P0**: Retry logic for failed alert forwarding
  - **P1**: Alert priority queuing
  - Location: `internal/event-analyzer/forwarder.go`
- **Substep 2.9.2.3**: Feedback loop to Edge
  - **Status**: ⬜ TODO
  - **P1**: Send feedback to Edge (e.g., update anomaly thresholds)
  - **P1**: Model performance feedback
  - **P2**: Adaptive threshold adjustment on Edge
  - Location: `internal/event-analyzer/feedback.go`
- **Substep 2.9.2.4**: Unit tests for alerting
  - **Status**: ⬜ TODO
  - **P0**: Test alert generation
  - **P0**: Test alert forwarding to Management Server
  - **P0**: Test alert acknowledgment and retry logic
  - **P1**: Test feedback loop to Edge
  - Location: `internal/event-analyzer/alert_test.go`, `internal/event-analyzer/forwarder_test.go`

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

### Phase 2 Success Criteria (User VM API)

**PoC Must-Have:**
- ✅ WireGuard server running and accepting Edge connections
- ✅ User Server receiving events from Edge Appliances
- ✅ Events cached in SQLite and forwarded to Management Server
- ✅ Basic stream relay working (Edge → User Server → Client via HTTP)
- ✅ Basic telemetry aggregation and forwarding to Management Server
- ✅ AI model catalog management (base models + customer-trained variants)
- ✅ Dataset ingestion from Edge and training job queuing
- ✅ Model distribution to Edge via WireGuard tunnel
- ✅ Secondary event analysis (inference on snapshots/clips, severity classification)
- ✅ Alert generation and forwarding to Management Server
- ✅ Basic remote storage (S3-compatible for PoC, with CID tracking)
- ✅ **Milestone 1**: First full event flow (Camera → Edge → User Server → Management Server → SaaS → Simple UI)

**Stretch Goals:**
- Full WebRTC implementation (HTTP relay acceptable for PoC)
- Full Filecoin/IPFS integration (S3 stub acceptable for PoC)
- Advanced telemetry aggregation and analytics
- Staged/blue-green model deployment
- Multi-model ensemble analysis

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

