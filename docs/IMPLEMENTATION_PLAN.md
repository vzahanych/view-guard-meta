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

### Epic 1.1: Development Environment & Project Setup

**Priority: P0**

#### Step 1.1.1: Repository & Project Structure
- **Substep 1.1.1.1**: Initialize Git repository
  - Create monorepo structure
  - Set up `.gitignore` files
  - Initialize Go modules for Edge services
- **Substep 1.1.1.2**: Create Edge Appliance directory structure
  - `edge/orchestrator/` - Go main orchestrator service
  - `edge/ai-service/` - Python AI inference service
  - `edge/shared/` - Shared Go libraries
  - `edge/proto/` - gRPC proto definitions
  - `edge/config/` - Configuration files
  - `edge/scripts/` - Build and deployment scripts
- **Substep 1.1.1.3**: Set up CI/CD basics
  - GitHub Actions for Edge services
  - Docker image builds for Go and Python services
  - Linting and basic tests

#### Step 1.1.2: Local Development Environment
- **Substep 1.1.2.1**: Development tooling setup
  - Install Go 1.25+, Python 3.12+ (as per TECHNICAL_STACK.md)
  - Set up code formatters (gofmt, black)
  - Configure linters (golangci-lint, pylint)
- **Substep 1.1.2.2**: Local testing environment
  - Docker Compose for local services (if needed)
  - Mock camera setup (RTSP test stream)
  - Local SQLite database setup
- **Substep 1.1.2.3**: IDE configuration
  - VS Code / Cursor workspace settings
  - Debugging configurations for Go and Python
  - Code snippets

### Epic 1.2: Go Orchestrator Service - Core Framework

**Priority: P0**

#### Step 1.2.1: Orchestrator Service Structure
- **Substep 1.2.1.1**: Main service framework
  - Service initialization
  - Configuration management (YAML/JSON config)
  - Logging setup (structured JSON logging)
  - Graceful shutdown handling
- **Substep 1.2.1.2**: Service architecture
  - Service manager pattern
  - Service lifecycle management
  - Inter-service communication (channels/events)
- **Substep 1.2.1.3**: Health check system
  - Health check endpoints
  - Service status reporting
  - Dependency health checks

#### Step 1.2.2: Configuration & State Management
- **Substep 1.2.2.1**: Configuration service
  - Config file loading
  - Environment variable support
  - Config validation
- **Substep 1.2.2.2**: State management
  - System state persistence (SQLite)
  - State recovery on restart
  - State synchronization

### Epic 1.3: Video Ingest & Processing (Go)

**Priority: P0**

#### Step 1.3.1: Camera Discovery & Connection
- **Substep 1.3.1.1**: RTSP client implementation
  - **P0**: Go RTSP client using `gortsplib`
  - **P0**: Stream connection and reconnection logic
  - **P0**: Error handling for network issues
  - **P0**: Stream health monitoring
  - **P0**: Manual RTSP URL configuration (for PoC)
- **Substep 1.3.1.2**: ONVIF camera discovery
  - **P1**: ONVIF device discovery (WS-Discovery)
  - **P1**: Camera capability detection
  - **P1**: Stream URL extraction
  - **P2**: Camera configuration retrieval
- **Substep 1.3.1.3**: Camera management service
  - **P0**: Camera registration and storage (SQLite)
  - **P0**: Basic camera configuration management
  - **P0**: Support for 1-2 cameras (PoC scope)
  - **P0**: Basic camera status monitoring

#### Step 1.3.2: Video Decoding with FFmpeg
- **Substep 1.3.2.1**: FFmpeg integration
  - Go wrapper for FFmpeg (CGO bindings or exec)
  - Hardware acceleration detection (Intel QSV via VAAPI)
  - Software fallback implementation
  - Codec detection and selection
- **Substep 1.3.2.2**: Frame extraction pipeline
  - Extract frames at configurable intervals
  - Frame buffer management
  - Frame preprocessing (resize, normalize)
  - Frame distribution to AI service
- **Substep 1.3.2.3**: Video clip recording
  - Start/stop recording on events
  - MP4 encoding with H.264
  - Clip metadata generation (duration, size, camera)
  - Concurrent recording for multiple cameras

#### Step 1.3.3: Local Storage Management
- **Substep 1.3.3.1**: Clip storage service
  - **P0**: File system organization (date/camera structure)
  - **P0**: Clip naming convention
  - **P0**: Basic disk space monitoring
  - **P1**: Advanced storage quota management
- **Substep 1.3.3.2**: Retention policy enforcement
  - **P0**: Simple "delete oldest when disk > X% full" rule
  - **P0**: Basic retention (e.g., 7 days default)
  - **P1**: Configurable retention periods and thresholds
  - **P1**: Advanced backpressure handling (pause recording when disk full)
- **Substep 1.3.3.3**: Snapshot generation
  - **P1**: JPEG snapshot capture on events
  - **P1**: Thumbnail generation
  - **P1**: Snapshot storage management
  - **P2**: Snapshot cleanup automation

### Epic 1.4: Python AI Inference Service

**Priority: P0**

#### Step 1.4.1: AI Service Framework
- **Substep 1.4.1.1**: Python service structure
  - FastAPI or Flask service for HTTP/gRPC
  - Service initialization
  - Health check endpoints
  - Logging setup
- **Substep 1.4.1.2**: OpenVINO installation and setup
  - Install OpenVINO toolkit
  - Hardware detection (CPU/iGPU)
  - Model conversion tools setup
  - OpenVINO runtime configuration

#### Step 1.4.2: Model Management
- **Substep 1.4.2.1**: Model loader service
  - Model loading from filesystem
  - Model versioning
  - Model hot-reload capability
  - Model validation
- **Substep 1.4.2.2**: YOLOv8 model integration
  - Download pre-trained YOLOv8 model
  - Convert to ONNX format
  - Convert to OpenVINO IR
  - Model optimization for target hardware

#### Step 1.4.3: Inference Pipeline
- **Substep 1.4.3.1**: Inference service implementation
  - Frame preprocessing for YOLO (resize, normalize)
  - Inference execution with OpenVINO
  - Post-processing (NMS, confidence filtering)
  - Bounding box extraction
- **Substep 1.4.3.2**: Detection logic
  - Person detection
  - Vehicle detection
  - Custom detection classes
  - Detection threshold configuration
- **Substep 1.4.3.3**: gRPC/HTTP API for inference
  - Inference request handling
  - Response formatting
  - Error handling
  - Performance metrics

### Epic 1.5: Event Management & Queue

**Priority: P0**

#### Step 1.5.1: Event Detection & Generation
- **Substep 1.5.1.1**: Event structure definition
  - Event schema (timestamp, camera, type, confidence, bounding boxes)
  - Event ID generation (UUID)
  - Event state management
- **Substep 1.5.1.2**: Event creation service
  - Trigger on AI detection
  - Associate clips and snapshots with events
  - Generate event metadata JSON
  - Event deduplication logic
- **Substep 1.5.1.3**: Event storage
  - Store events in SQLite
  - Event querying
  - Event expiration

#### Step 1.5.2: Event Queue Management
- **Substep 1.5.2.1**: Local event queue
  - Queue implementation (in-memory + SQLite persistence)
  - Queue priority handling
  - Queue size limits
- **Substep 1.5.2.2**: Transmission logic
  - Queue processing service
  - Retry logic for failed transmissions
  - Queue persistence on restart
  - Queue recovery

### Epic 1.6: WireGuard Client & Communication

**Priority: P0**

#### Step 1.6.1: WireGuard Client Service
- **Substep 1.6.1.1**: WireGuard client implementation
  - Go WireGuard client using `golang.zx2c4.com/wireguard`
  - Connection to KVM VM
  - Configuration management
  - Key management
- **Substep 1.6.1.2**: Tunnel management
  - Tunnel health monitoring
  - Automatic reconnection logic
  - Connection state management
  - Latency tracking

#### Step 1.6.2: gRPC Communication
- **Substep 1.6.2.1**: Proto definitions
  - Define Edge ↔ KVM VM proto files
  - Event transmission proto
  - Control commands proto
  - Telemetry proto
- **Substep 1.6.2.2**: gRPC client implementation
  - gRPC client setup
  - Event transmission over WireGuard tunnel
  - Acknowledge receipt handling
  - Error handling and retries

#### Step 1.6.3: Event Transmission
- **Substep 1.6.3.1**: Event sender service
  - Send event metadata over WireGuard/gRPC
  - Handle transmission failures
  - Transmission status tracking
- **Substep 1.6.3.2**: Clip streaming (on-demand)
  - Stream clip on request from KVM VM
  - Handle stream interruptions
  - Stream metadata transmission

### Epic 1.7: Telemetry & Health Reporting

**Priority: P0** (Basic telemetry only for PoC)

#### Step 1.7.1: Telemetry Collection
- **Substep 1.7.1.1**: System metrics collection
  - CPU utilization monitoring
  - Memory usage tracking
  - Disk usage monitoring
  - Network statistics
- **Substep 1.7.1.2**: Application metrics
  - Camera status (online/offline)
  - Event queue length
  - AI inference performance
  - Storage usage per camera
- **Substep 1.7.1.3**: Health status aggregation
  - Overall system health calculation
  - Component health status
  - Alert conditions

#### Step 1.7.2: Health Reporting
- **Substep 1.7.2.1**: Periodic heartbeat
  - Send heartbeat to KVM VM
  - Heartbeat interval configuration
  - Heartbeat failure handling
- **Substep 1.7.2.2**: Telemetry transmission
  - Send telemetry data to KVM VM
  - Telemetry batching
  - Telemetry persistence

### Epic 1.8: Encryption & Archive Client (Basic)

**Priority: P1** (Can be simplified for PoC)

#### Step 1.8.1: Encryption Service
- **Substep 1.8.1.1**: Clip encryption implementation
  - **P0**: AES-256-GCM encryption
  - **P0**: Argon2id key derivation from user secret
  - **P1**: Encryption metadata generation
- **Substep 1.8.1.2**: Key management
  - **P0**: User secret handling (never transmitted)
  - **P0**: Key derivation logic
  - **P0**: Key storage (local only)
- **Substep 1.8.1.3**: Archive queue (basic)
  - **P1**: Encrypted clip queue
  - **P1**: Basic transmission to KVM VM
  - **P2**: Advanced queue management

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
  - `kvm-agent/` - Main agent services
  - `kvm-agent/wireguard-server/` - WireGuard server service
  - `kvm-agent/event-cache/` - Event cache service
  - `kvm-agent/stream-relay/` - Stream relay service
  - `kvm-agent/filecoin-sync/` - Filecoin sync service
  - `kvm-agent/proto/` - gRPC proto definitions
- **Substep 2.1.1.2**: Go modules setup
  - Initialize Go modules
  - Dependency management
  - Shared libraries

#### Step 2.1.2: Database & Storage Setup
- **Substep 2.1.2.1**: SQLite schema design
  - Event cache table
  - CID storage table
  - Telemetry buffer table
  - Edge Appliance registry
- **Substep 2.1.2.2**: Database migration system
  - Migration tool setup
  - Initial migrations
  - Migration rollback

### Epic 2.2: WireGuard Server Service

**Priority: P0**

#### Step 2.2.1: WireGuard Server Implementation
- **Substep 2.2.1.1**: WireGuard server service
  - Go service using `golang.zx2c4.com/wireguard`
  - Server configuration management
  - Server key management
- **Substep 2.2.1.2**: Client management
  - Client key generation
  - Client configuration generation
  - Client registration and storage
- **Substep 2.2.1.3**: Bootstrap process
  - Bootstrap token validation
  - Initial client registration
  - Long-lived credential issuance

#### Step 2.2.2: Tunnel Management
- **Substep 2.2.2.1**: Connection monitoring
  - Track connected Edge Appliances
  - Connection state management
  - Disconnection detection and handling
- **Substep 2.2.2.2**: Tunnel health monitoring
  - Ping/pong mechanism
  - Latency tracking
  - Bandwidth monitoring
  - Tunnel statistics collection

### Epic 2.3: Event Cache Service

**Priority: P0**

#### Step 2.3.1: Event Reception & Storage
- **Substep 2.3.1.1**: Event reception from Edge
  - gRPC server for Edge connections
  - Receive events over WireGuard tunnel
  - Validate event structure
  - Store in SQLite cache with rich metadata
- **Substep 2.3.1.2**: Event cache management
  - Rich metadata storage (bounding boxes, detection scores)
  - Event querying and retrieval
  - Cache expiration policies
  - Cache cleanup

#### Step 2.3.2: Event Forwarding to SaaS
- **Substep 2.3.2.1**: Event summarization
  - Privacy-minimized metadata extraction
  - Remove sensitive details (bounding boxes, raw scores)
  - Create summarized event record
- **Substep 2.3.2.2**: SaaS communication
  - gRPC client to SaaS
  - Forward summarized events
  - Handle forwarding failures and retries
  - Acknowledgment handling

### Epic 2.4: Telemetry Aggregation Service

**Priority: P0**

#### Step 2.4.1: Telemetry Collection
- **Substep 2.4.1.1**: Telemetry reception
  - **P0**: Receive telemetry from Edge Appliances
  - **P0**: Validate telemetry data
  - **P0**: Store raw-ish telemetry records in SQLite buffer
- **Substep 2.4.1.2**: Telemetry aggregation
  - **P0**: Simple "healthy/unhealthy" status calculation
  - **P1**: Aggregate per-tenant metrics (averages, totals)
  - **P1**: Advanced health status calculation

#### Step 2.4.2: Telemetry Forwarding
- **Substep 2.4.2.1**: Telemetry summarization
  - **P0**: Forward simple health status to SaaS
  - **P1**: Summarize telemetry (remove detailed metrics, create summaries)
- **Substep 2.4.2.2**: Forward to SaaS
  - **P0**: Send basic health status to SaaS
  - **P0**: Periodic reporting
  - **P1**: Advanced alert forwarding

### Epic 2.5: Stream Relay Service

**Priority: P0**

#### Step 2.5.1: Stream Request Handling
- **Substep 2.5.1.1**: Token validation
  - Receive time-bound tokens from SaaS
  - Validate token signature and expiration
  - Extract event ID and user info from token
- **Substep 2.5.1.2**: Stream orchestration
  - Request clip from Edge Appliance via gRPC
  - Handle Edge Appliance response
  - Stream setup coordination

#### Step 2.5.2: Stream Relay Implementation
- **Substep 2.5.2.1**: HTTP-based relay (P0 for PoC)
  - **P0**: Simple HTTP progressive download relay from Edge via KVM to client
  - **P0**: Request clip from Edge over WireGuard/gRPC
  - **P0**: Stream clip data via HTTP(S) to client
  - **P0**: Basic error handling and stream interruptions
  - **P1**: WebRTC relay using Pion (ICE, STUN/TURN, SDP exchange)
  - **P2**: Advanced WebRTC features (transcoding, reconnection logic)

### Epic 2.6: Filecoin Sync Service (Basic/Stub)

**Priority: P1** (Can be stubbed for PoC)

#### Step 2.6.1: Basic Archive Upload (PoC)
- **Substep 2.6.1.1**: Encrypted clip reception
  - **P0**: Receive encrypted clips from Edge (already encrypted)
  - **P0**: Store temporarily during upload
  - **P0**: Automatic cleanup after upload
- **Substep 2.6.1.2**: Upload implementation (basic)
  - **P0 Option**: Stub with S3 + fake CIDs for PoC demo
  - **P1 Option**: Basic IPFS gateway upload
  - **P2**: Full Filecoin integration
  - CID retrieval and storage

#### Step 2.6.2: Quota Management (Basic)
- **Substep 2.6.2.1**: Simple quota tracking
  - **P0**: Hard-coded quota limit for PoC
  - **P0**: Track archive size per tenant
  - **P2**: Complex quota policies from SaaS
- **Substep 2.6.2.2**: Basic quota enforcement
  - **P0**: Check quota before upload
  - **P0**: Reject uploads if over quota
  - **P2**: Advanced quota management

#### Step 2.6.3: Archive Metadata Management
- **Substep 2.6.3.1**: CID storage
  - **P0**: Store CIDs in SQLite
  - **P0**: Associate CIDs with events
  - **P1**: Basic metadata storage
- **Substep 2.6.3.2**: Archive status updates
  - **P0**: Update SaaS with archive status
  - **P0**: CID transmission to SaaS
  - **P2**: Advanced notification system

### Epic 2.7: KVM VM Agent Orchestration

**Priority: P0**

#### Step 2.7.1: Agent Service Manager
- **Substep 2.7.1.1**: Main agent service
  - Service initialization
  - Service lifecycle management
  - Configuration management
- **Substep 2.7.1.2**: Service coordination
  - Inter-service communication
  - Service health monitoring
  - Graceful shutdown

#### Step 2.7.2: SaaS Communication
- **Substep 2.7.2.1**: gRPC client to SaaS
  - **P0**: Connection setup
  - **P0**: mTLS configuration
  - **P0**: Connection health monitoring
- **Substep 2.7.2.2**: Command handling (basic)
  - **P1**: Basic command handling from SaaS
  - **P2**: Advanced command orchestration

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
  - `saas/api/` - REST API service
  - `saas/auth/` - Authentication service
  - `saas/events/` - Event inventory service
  - `saas/provisioning/` - VM provisioning service
  - `saas/billing/` - Billing service
  - `saas/shared/` - Shared libraries
- **Substep 3.1.1.2**: Go modules and dependencies
  - Initialize Go modules
  - Database drivers (PostgreSQL, Redis)
  - External service clients

#### Step 3.1.2: Database Setup
- **Substep 3.1.2.1**: PostgreSQL schema design
  - Users table
  - Tenants table
  - KVM VM assignments table
  - Event metadata table
  - Subscriptions table
  - Billing records table
- **Substep 3.1.2.2**: Database migration system
  - Migration tool setup (golang-migrate)
  - Initial migrations
  - Migration rollback capability

### Epic 3.2: Authentication & User Management

**Priority: P0**

#### Step 3.2.1: Auth0 Integration
- **Substep 3.2.1.1**: Auth0 application setup
  - **P0**: Create Auth0 application
  - **P0**: Configure OAuth2/OIDC settings
  - **P0**: Set up callback URLs
  - **P1**: Configure user roles (single "tenant admin" role is P0)
- **Substep 3.2.1.2**: Backend authentication service
  - **P0**: JWT token validation middleware
  - **P0**: User session management
  - **P0**: Simple tenant mapping (single role: "tenant admin")
  - **P1**: Full RBAC implementation with multiple roles
  - **P0**: Token refresh handling
- **Substep 3.2.1.3**: User service
  - User CRUD operations
  - User profile management
  - User preferences storage
  - User-tenant association

#### Step 3.2.2: Tenant Management
- **Substep 3.2.2.1**: Tenant service
  - Tenant creation and management
  - Tenant settings
  - Tenant-KVM VM assignment
  - Tenant subscription association
- **Substep 3.2.2.2**: Multi-tenancy isolation
  - Tenant context middleware
  - Data isolation enforcement
  - Cross-tenant access prevention

### Epic 3.3: Event Inventory Service

**Priority: P0**

#### Step 3.3.1: Event Storage & Querying
- **Substep 3.3.1.1**: Event service implementation
  - **P0**: Store summarized event metadata in PostgreSQL
  - **P0**: Basic event querying API
  - **P0**: Basic filtering (camera, type, date range)
  - **P2**: Advanced indexing and full-text search
- **Substep 3.3.1.2**: Event search functionality
  - **P1**: Basic search by metadata fields
  - **P2**: Full-text search (PostgreSQL pg_trgm)
- **Substep 3.3.1.3**: Event retention policies
  - **P1**: Basic retention (simple cleanup)
  - **P2**: Configurable retention periods, archive status tracking

#### Step 3.3.2: Real-time Event Updates
- **Substep 3.3.2.1**: Event updates mechanism
  - **P0**: Basic polling (`/events` endpoint with periodic refresh)
  - **P1**: Server-Sent Events (SSE) for live updates
  - **P2**: Advanced SSE reconnection handling
- **Substep 3.3.2.2**: Event aggregation
  - **P1**: Basic event counts
  - **P2**: Advanced statistics and dashboard data

### Epic 3.4: KVM VM Management Service (Basic)

**Priority: P0** (Simplified for PoC)

#### Step 3.4.1: Basic VM Assignment
- **Substep 3.4.1.1**: Manual VM provisioning (PoC)
  - **P0**: Pre-provision 1-2 KVM VMs manually
  - Simple CLI script or manual setup
  - Store VM connection details in database
  - **P2**: Full Terraform automation (post-PoC)
- **Substep 3.4.1.2**: VM assignment service (basic)
  - Assign pre-provisioned VM to tenant on signup
  - Store tenant-VM mapping in database
  - Basic VM status tracking
  - **P2**: VM lifecycle management (start/stop/delete, scaling)

#### Step 3.4.2: VM Communication
- **Substep 3.4.2.1**: gRPC server for VM agents
  - gRPC server setup
  - mTLS configuration
  - Command handling
- **Substep 3.4.2.2**: VM agent management
  - Agent registration
  - Agent health monitoring
  - Agent command execution
  - Agent configuration updates

### Epic 3.5: ISO Generation Service (Basic)

**Priority: P1** (Can use generic ISO for early PoC)

#### Step 3.5.1: Basic ISO Setup (PoC)
- **Substep 3.5.1.1**: Generic ISO preparation
  - **P0**: Single generic ISO with hard-coded config or manual bootstrap script
  - Base Ubuntu 24.04 LTS ISO
  - Manual configuration editing for PoC
  - **P2**: Full Packer pipeline with tenant-specific generation
- **Substep 3.5.1.2**: Basic bootstrap (PoC)
  - **P0**: Manual bootstrap token generation and configuration
  - Simple script-based configuration injection
  - **P2**: Automated tenant-specific ISO generation
- **Substep 3.5.1.3**: ISO download (basic)
  - **P0**: Simple download endpoint or manual distribution
  - **P2**: Secure download API with CDN integration

### Epic 3.6: Billing & Subscription Service (Basic)

**Priority: P2** (Defer to post-PoC, use free plan for PoC)

#### Step 3.6.1: Basic Plan Management (PoC)
- **Substep 3.6.1.1**: Simple plan model
  - **P0**: Hard-coded "free plan" for PoC
  - Basic plan assignment to tenants
  - **P2**: Full Stripe integration with webhooks
- **Substep 3.6.1.2**: Quota management (basic)
  - **P0**: Hard-coded quota limits for PoC
  - Basic quota tracking
  - **P2**: Full quota service with plan-based limits

### Epic 3.7: REST API Service

**Priority: P0**

#### Step 3.7.1: API Framework
- **Substep 3.7.1.1**: Gin framework setup
  - Router configuration
  - Middleware setup (auth, logging, CORS)
  - Error handling
- **Substep 3.7.1.2**: API endpoints
  - User endpoints
  - Event endpoints
  - Camera endpoints
  - Subscription endpoints
  - VM management endpoints
- **Substep 3.7.1.3**: API documentation
  - OpenAPI/Swagger specification
  - API endpoint documentation
  - Request/response examples

#### Step 3.7.2: API Features (Basic)
- **Substep 3.7.2.1**: Rate limiting
  - **P1**: Basic rate limiting middleware
  - **P2**: Advanced per-user rate limits
- **Substep 3.7.2.2**: Caching
  - **P1**: Basic Redis caching for critical data
  - **P2**: Advanced caching strategies

---

## Phase 4: SaaS UI Frontend

**Duration**: 2 weeks  
**Goal**: Build core React frontend - authentication, event timeline, basic clip viewing

**Scope**: Simplified UI for PoC - essential features only, no advanced configuration

**Milestone 2 Target**: End of this phase - first clip viewing (UI → SaaS → KVM VM → Edge → Stream)

### Epic 4.1: Frontend Project Setup

**Priority: P0**

#### Step 4.1.1: React Project Structure
- **Substep 4.1.1.1**: Initialize React + TypeScript project
  - Create React app with Vite or Create React App
  - TypeScript configuration
  - Tailwind CSS setup
- **Substep 4.1.1.2**: Project structure
  - `src/components/` - React components
  - `src/pages/` - Page components
  - `src/hooks/` - Custom hooks
  - `src/services/` - API services
  - `src/store/` - State management (Zustand)
  - `src/utils/` - Utility functions
- **Substep 4.1.1.3**: Development tooling
  - ESLint configuration
  - Prettier configuration
  - Testing setup (Vitest/Jest)

#### Step 4.1.2: API Client Setup
- **Substep 4.1.2.1**: API client implementation
  - Axios or fetch wrapper
  - Request/response interceptors
  - Error handling
- **Substep 4.1.2.2**: API service layer
  - Event API service
  - User API service
  - Camera API service
  - Subscription API service

### Epic 4.2: Authentication UI

**Priority: P0**

#### Step 4.2.1: Auth0 Integration
- **Substep 4.2.1.1**: Auth0 React SDK setup
  - Install and configure Auth0 React SDK
  - Auth0Provider setup
  - Configuration
- **Substep 4.2.1.2**: Authentication flows
  - Login page
  - Logout functionality
  - Protected route wrapper
  - Token management

#### Step 4.2.2: User Profile UI
- **Substep 4.2.2.1**: User profile page
  - Profile information display
  - Profile editing
  - User preferences
- **Substep 4.2.2.2**: User settings
  - Settings page
  - Notification preferences
  - Account management

### Epic 4.3: Dashboard & Navigation

**Priority: P0**

#### Step 4.3.1: Main Layout
- **Substep 4.3.1.1**: Layout component
  - Header with user info
  - Navigation sidebar
  - Main content area
  - Responsive design
- **Substep 4.3.1.2**: Navigation
  - Route configuration (React Router)
  - Navigation menu
  - Active route highlighting
  - Mobile navigation

#### Step 4.3.2: Dashboard Page (Basic)
- **Substep 4.3.2.1**: Basic dashboard
  - **P0**: Simple "Events" nav item
  - **P0**: Basic camera status label (e.g., "Cameras: 2 online")
  - **P1**: Dashboard widgets (camera overview, recent events, health indicators)
- **Substep 4.3.2.2**: Updates
  - **P0**: Basic polling refresh
  - **P1**: SSE connection for live updates

### Epic 4.4: Event Timeline UI

**Priority: P0**

#### Step 4.4.1: Timeline Component
- **Substep 4.4.1.1**: Timeline layout
  - **P0**: Simple table/list of events
  - **P0**: Event card rendering
  - **P0**: Basic pagination
  - **P1**: Date grouping
  - **P1**: Infinite scroll
- **Substep 4.4.1.2**: Event cards
  - **P0**: Event metadata display
  - **P0**: Event type indicators
  - **P0**: Timestamp formatting
  - **P1**: Event thumbnail display

#### Step 4.4.2: Event Filtering & Search
- **Substep 4.4.2.1**: Filter UI
  - **P0**: Basic filters (camera, type, date range)
  - **P0**: Simple filter state management
  - **P1**: Advanced date range picker
- **Substep 4.4.2.2**: Search functionality
  - **P1**: Search input and basic search
  - **P1**: Search API integration
  - **P2**: Search history

#### Step 4.4.3: Event Details View
- **Substep 4.4.3.1**: Event detail modal/page
  - Event metadata display
  - Thumbnail/snapshot display
  - Detection details (bounding boxes if available)
  - Camera information
- **Substep 4.4.3.2**: Event actions
  - "View Clip" button
  - "Download" button
  - "Archive" button (if applicable)
  - Event deletion (if allowed)

### Epic 4.5: Clip Viewing UI

**Priority: P0**

#### Step 4.5.1: Video Player Component
- **Substep 4.5.1.1**: Video player integration
  - **P0**: Standard HTML5 `<video>` element with HTTP URL
  - **P0**: React video player component using HTTP progressive download
  - **P0**: Basic playback controls (play/pause, seek)
  - **P1**: WebRTC stream handling (if WebRTC implemented)
  - **P1**: Fullscreen support
- **Substep 4.5.1.2**: Stream request flow
  - **P0**: "View Clip" button requests HTTP URL from SaaS
  - **P0**: Loading states and error handling
  - **P1**: WebRTC connection management (if WebRTC implemented)

#### Step 4.5.2: Clip Player Features
- **Substep 4.5.2.1**: Playback features
  - Play/pause
  - Seek
  - Volume control
  - Playback speed
- **Substep 4.5.2.2**: Clip information
  - Clip metadata display
  - Camera information
  - Timestamp display
  - Download option

### Epic 4.6: Camera Management UI (Basic)

**Priority: P0** (Simplified for PoC)

#### Step 4.6.1: Basic Camera List
- **Substep 4.6.1.1**: Camera list page (simple)
  - **P0**: Display discovered cameras from Edge
  - Camera status indicators (online/offline)
  - Basic camera information
  - **P2**: Camera thumbnail/preview, advanced actions
- **Substep 4.6.1.2**: Basic camera configuration
  - **P0**: Camera naming and labeling
  - **P2**: Detection zones, schedules, advanced settings

### Epic 4.7: Subscription & Billing UI

**Priority: P2** (Defer to post-PoC)

#### Step 4.7.1: Basic Plan Display (PoC)
- **Substep 4.7.1.1**: Simple plan indicator
  - **P0**: Display "Free Plan" or plan name (hard-coded for PoC)
  - **P2**: Full subscription management UI, plan comparison, upgrade/downgrade
- **Substep 4.7.1.2**: Billing UI
  - **P2**: Payment method management, billing history, Stripe integration

### Epic 4.8: Onboarding & ISO Download

**Priority: P1** (Can be simplified for PoC)

#### Step 4.8.1: Onboarding Flow
- **Substep 4.8.1.1**: Onboarding wizard
  - Welcome screen
  - Plan selection
  - ISO download instructions
  - Setup guide
- **Substep 4.8.1.2**: ISO download page
  - ISO download button
  - Download instructions
  - Installation guide
  - Troubleshooting tips

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
  - **P0**: Single generic Ubuntu 24.04 LTS Server ISO
  - Manual configuration or simple bootstrap script
  - Basic auto-install configuration
  - **P2**: Full Packer automation with tenant-specific generation
- **Substep 5.1.1.2**: Software pre-installation (basic)
  - **P0**: Manual installation of Edge Appliance software
  - Or simple installation script
  - **P2**: Automated packaging and pre-installation

#### Step 5.1.2: Basic Configuration
- **Substep 5.1.2.1**: Bootstrap configuration (simple)
  - **P0**: Manual bootstrap token generation
  - Manual KVM VM connection details configuration
  - Simple first-boot script
  - **P2**: Automated tenant-specific configuration injection
- **Substep 5.1.2.2**: Build automation (basic)
  - **P0**: Simple build script on developer machine
  - **P2**: Full CI/CD pipeline with Packer

### Epic 5.2: Deployment Automation (Basic)

**Priority: P1** (Manual deployment acceptable for PoC)

#### Step 5.2.1: Basic Deployment
- **Substep 5.2.1.1**: Manual deployment (PoC)
  - **P0**: Manual KVM VM setup (1-2 VMs)
  - Manual agent installation and configuration
  - Manual SaaS deployment (Docker Compose or simple K8s)
  - **P2**: Full Terraform automation
- **Substep 5.2.1.2**: Basic automation scripts
  - **P1**: Simple deployment scripts
  - Basic configuration management
  - **P2**: Full Infrastructure as Code

#### Step 5.2.2: SaaS Deployment (Basic)
- **Substep 5.2.2.1**: Simple deployment
  - **P0**: Docker Compose for local PoC or simple K8s deployment
  - Basic service configuration
  - **P2**: Full EKS setup with advanced configuration
- **Substep 5.2.2.2**: Database setup
  - **P0**: Manual PostgreSQL setup or managed database
  - Basic migration execution
  - **P2**: Automated database deployment and backups

### Epic 5.3: Update & Maintenance Automation

**Priority: P2** (Defer to post-PoC)

#### Step 5.3.1: Update Mechanisms
- **Substep 5.3.1.1**: Manual updates for PoC
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
  - Camera → Edge → KVM VM → SaaS → UI
  - Verify data integrity at each step
  - Test error handling and recovery
- **Substep 6.1.1.2**: Stream flow end-to-end
  - UI request → SaaS → KVM VM → Edge → Stream → UI
  - **P0**: Test HTTP clip streaming (progressive download)
  - **P0**: Test stream interruptions and basic error handling
  - **P1/P2**: Test WebRTC stream quality (if WebRTC implemented)
- **Substep 6.1.1.3**: Telemetry flow end-to-end
  - Edge → KVM VM → SaaS → Dashboard
  - Verify telemetry accuracy
  - Test aggregation and reporting

#### Step 6.1.2: Multi-Tenant Isolation Testing
- **Substep 6.1.2.1**: Tenant isolation verification
  - Create multiple test tenants
  - Verify data isolation in SaaS
  - Test cross-tenant access prevention
- **Substep 6.1.2.2**: KVM VM isolation
  - Verify WireGuard tunnel isolation
  - Test VM resource isolation
  - Verify network isolation
  - Test VM-to-VM isolation

#### Step 6.1.3: Archive Integration (Basic)
- **Substep 6.1.3.1**: Archive flow end-to-end (P0: S3 stub)
  - **P0**: Edge encryption → KVM VM → S3 stub → fake CID storage
  - **P0**: Verify encryption throughout
  - **P0**: Test basic quota enforcement
  - **P2**: Full Filecoin integration with real CIDs
- **Substep 6.1.3.2**: Archive retrieval flow
  - **P0**: UI request → SaaS → fake CID → S3 stub (basic verification)
  - **P2**: Browser-based decryption using Filecoin blob (full implementation)

### Epic 6.2: Essential Testing

**Priority: P0** (Focus on critical paths for PoC)

#### Step 6.2.1: Critical Path Unit Tests
- **Substep 6.2.1.1**: Essential unit tests
  - **P0**: Edge event generation & queueing
  - **P0**: Edge↔KVM VM gRPC contracts
  - **P0**: KVM VM↔SaaS event/telemetry contracts
  - **P0**: Basic auth flows
  - **P2**: Full test coverage > 70%
- **Substep 6.2.1.2**: Python AI service tests
  - **P0**: Model loading and basic inference
  - **P2**: Comprehensive edge case testing

#### Step 6.2.2: Integration Testing (Essential)
- **Substep 6.2.2.1**: Critical integration tests
  - **P0**: Edge ↔ KVM VM event flow
  - **P0**: KVM VM ↔ SaaS event forwarding
  - **P0**: Full stack event flow
  - **P2**: Comprehensive integration test suite
- **Substep 6.2.2.2**: Database tests (basic)
  - **P0**: Data persistence verification
  - **P2**: Transaction and performance tests

#### Step 6.2.3: End-to-End Testing (Key Scenarios)
- **Substep 6.2.3.1**: Critical E2E scenarios
  - **P0**: Event detection and display in UI
  - **P0**: Clip viewing flow
  - **P1**: Basic archive flow (if Filecoin implemented)
  - **P2**: Full E2E test automation with Playwright/Cypress
- **Substep 6.2.3.2**: Manual testing
  - **P0**: Manual test scenarios for PoC demo
  - **P2**: Automated E2E test suite

#### Step 6.2.4: Performance Testing (Basic)
- **Substep 6.2.4.1**: Basic performance verification
  - **P0**: Single camera, single user performance
  - **P1**: Basic load testing (2-3 concurrent users)
  - **P2**: Comprehensive load and performance testing
  - Network throughput

### Epic 6.3: Error Handling & Resilience

**Priority: P0** (Basic error handling for PoC)

#### Step 6.3.1: Error Handling Implementation
- **Substep 6.3.1.1**: Error handling patterns
  - **P0**: Standardized error types
  - **P0**: Error propagation
  - **P0**: Error logging
  - **P0**: Basic user-friendly error messages
- **Substep 6.3.1.2**: Retry mechanisms
  - **P0**: Network operation retries with exponential backoff
  - **P0**: Database operation retries
  - **P1**: Circuit breakers
- **Substep 6.3.1.3**: Resilience testing
  - Network failure scenarios
  - Service crash scenarios
  - Database failure scenarios
  - Recovery testing

### Epic 6.4: Security Hardening

**Priority: P0** (Basic security for PoC)

#### Step 6.4.1: Basic Security Review
- **Substep 6.4.1.1**: Essential security checks
  - **P0**: Dependency vulnerability scanning
  - **P0**: Basic security best practices review
  - **P2**: Full static analysis (CodeQL, SonarQube)
- **Substep 6.4.1.2**: Basic security testing
  - **P0**: API authentication/authorization testing
  - **P0**: Input validation testing
  - **P2**: Full penetration testing
- **Substep 6.4.1.3**: Security fixes
  - **P0**: Address critical vulnerabilities
  - **P2**: Comprehensive security hardening

#### Step 6.4.2: Essential Security Enhancements
- **Substep 6.4.2.1**: Input validation
  - **P0**: Sanitize all user inputs
  - **P0**: Validate API parameters
  - **P0**: SQL injection prevention
  - **P1**: XSS prevention
- **Substep 6.4.2.2**: Basic rate limiting
  - **P1**: Basic API rate limiting
  - **P2**: Advanced rate limiting and DDoS protection
- **Substep 6.4.2.3**: Security headers
  - **P0**: HTTPS enforcement
  - **P1**: Basic security headers
  - **P2**: Comprehensive security headers (CSP, HSTS, etc.)

### Epic 6.5: Performance Optimization

**Priority: P1** (Basic optimization for PoC)

#### Step 6.5.1: Essential Backend Optimization
- **Substep 6.5.1.1**: Basic database optimization
  - **P0**: Essential index creation
  - **P1**: Query optimization for critical paths
  - **P2**: Advanced optimization and caching
- **Substep 6.5.1.2**: Service optimization (basic)
  - **P1**: Profile and fix obvious performance issues
  - **P2**: Comprehensive optimization

#### Step 6.5.2: Basic Frontend Optimization
- **Substep 6.5.2.1**: Essential bundle optimization
  - **P1**: Basic code splitting
  - **P2**: Advanced optimization (tree shaking, lazy loading)
- **Substep 6.5.2.2**: Performance optimization (basic)
  - **P1**: Fix obvious React performance issues
  - **P2**: Comprehensive optimization

### Epic 6.6: Monitoring & Observability

**Priority: P1** (Basic monitoring for PoC)

#### Step 6.6.1: Basic Metrics Implementation
- **Substep 6.6.1.1**: Essential metrics
  - **P0**: Basic service health metrics
  - **P1**: Prometheus setup with basic metrics
  - **P2**: Comprehensive metrics (business, system)
- **Substep 6.6.1.2**: Basic dashboards
  - **P1**: Simple Grafana dashboard for health
  - **P2**: Comprehensive dashboards
- **Substep 6.6.1.3**: Basic alerting
  - **P1**: Essential alerts (service down, critical errors)
  - **P2**: Comprehensive alerting setup

#### Step 6.6.2: Basic Logging Implementation
- **Substep 6.6.2.1**: Structured logging
  - **P0**: JSON log format
  - **P0**: Basic log levels
  - **P2**: Advanced contextual logging
- **Substep 6.6.2.2**: Log aggregation (basic)
  - **P1**: Basic log collection (Loki or simple file aggregation)
  - **P2**: Full log aggregation and analysis
- **Substep 6.6.2.3**: Privacy-aware logging
  - **P0**: Ensure no PII in logs (critical for PoC)
  - **P0**: Basic log sanitization
  - **P2**: Comprehensive sensitive data filtering

### Epic 6.7: Documentation

**Priority: P1** (Essential documentation for PoC)

#### Step 6.7.1: Technical Documentation
- **Substep 6.7.1.1**: API documentation
  - OpenAPI/Swagger specs
  - API endpoint documentation
  - Request/response examples
- **Substep 6.7.1.2**: Code documentation
  - GoDoc comments
  - Python docstrings
  - Architecture decision records (ADRs)
- **Substep 6.7.1.3**: Deployment documentation
  - Infrastructure setup guide
  - Service deployment procedures
  - Configuration reference

#### Step 6.7.2: User Documentation
- **Substep 6.7.2.1**: User guide
  - Getting started guide
  - Feature documentation
  - Troubleshooting guide
- **Substep 6.7.2.2**: Developer guide
  - Development environment setup
  - Contribution guidelines
  - Testing guidelines

### Epic 6.8: PoC Demo Preparation

**Priority: P0**

#### Step 6.8.1: Demo Environment Setup
- **Substep 6.8.1.1**: Clean demo environment
  - Fresh deployment
  - Sample data setup
  - Test cameras configuration
- **Substep 6.8.1.2**: Demo script preparation
  - Demo flow outline
  - Key features to showcase
  - Backup scenarios

#### Step 6.8.2: Demo Materials
- **Substep 6.8.2.1**: Presentation materials
  - Architecture overview slides
  - Key features slides
  - Demo video recording
- **Substep 6.8.2.2**: Demo data
  - Sample events
  - Sample clips
  - Test scenarios

---

## Success Criteria

### Phase 1 Success Criteria (Edge Appliance)

**PoC Must-Have:**
- ✅ Go orchestrator service running and managing all components
- ✅ Python AI service running and performing inference
- ✅ Edge Appliance can discover and connect to 1-2 cameras (RTSP/ONVIF)
- ✅ Video clips recorded and stored locally
- ✅ AI inference detecting objects (people, vehicles)
- ✅ Events generated and queued for transmission
- ✅ WireGuard client connecting to KVM VM
- ✅ Basic telemetry being collected and reported

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

