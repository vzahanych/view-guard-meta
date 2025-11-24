# Technical Stack

This document defines the technical stack for The Private AI Guardian platform, organized by architectural layer and functional area.

## Table of Contents

1. [Overview](#1-overview)
2. [SaaS Control Plane Stack](#2-saas-control-plane-stack)
3. [KVM VM Stack](#3-kvm-vm-stack)
4. [Edge Appliance Stack](#4-edge-appliance-stack)
5. [AI/ML Stack](#5-aiml-stack)
6. [Video Processing Stack](#6-video-processing-stack)
7. [Infrastructure &amp; DevOps](#7-infrastructure--devops)
8. [Security &amp; Networking](#8-security--networking)
9. [Monitoring &amp; Observability](#9-monitoring--observability)
10. [Development Tools](#10-development-tools)

---

## 1. Overview

The technical stack is organized into three main layers, each with distinct requirements:

- **SaaS Control Plane**: Multi-tenant, cloud-native, high-scale web services
- **KVM VM (Private Cloud Node)**: Single-tenant, lightweight services for relay and orchestration
- **Edge Appliance (Mini PC)**: Resource-constrained, real-time video processing and AI inference

**Note:** This document marks **Baseline** (default choices for MVP) and **Alternatives** (future options). For minimum versions and hardware targets, see [Version Constraints &amp; Compatibility](#version-constraints--compatibility).

---

## MVP Baseline Stack

This section defines the minimal, production-ready stack for initial implementation. All other options in this document are marked as alternatives or future considerations.

### SaaS Control Plane

- **Language**: Go 1.23+, Gin framework
- **Internal Communication**: gRPC
- **Frontend**: React + TypeScript, responsive web (PWA-first; native mobile later)
- **Database**: PostgreSQL 16+, Redis for cache/sessions
- **Message Queue**: RabbitMQ
- **Auth**: Auth0 (hosted OIDC)
- **Object Storage**: AWS S3 (or S3-compatible) + CloudFront/Cloudflare CDN
- **Infrastructure**: AWS + EKS, cert-manager for TLS (no service mesh initially)

### KVM VM (Private Cloud Node)

- **OS**: Ubuntu Server 24.04 LTS (or 22.04 LTS)
- **Runtime**: Docker + systemd (no Kubernetes)
- **Database**: SQLite per tenant
- **Services**: Go daemons for WireGuard server, event cache, Filecoin client, Pion WebRTC relay
- **Communication**: gRPC + mTLS (certificates bootstrapped during VM provisioning)

### Edge Appliance

- **OS**: Ubuntu 24.04 LTS (or 22.04 LTS)
- **Runtimes**: Go (orchestrator + crypto + storage) + Python 3.12+ (AI inference only)
- **Video**: FFmpeg (hardware-accelerated where possible), minimal OpenCV for preprocessing
- **AI**: OpenVINO (primary), ONNX Runtime (fallback)
- **Encryption**: AES-256-GCM, Argon2id for key derivation
- **Storage**: ext4 + SQLite for metadata, local filesystem for clips

### Observability

- **Metrics**: Prometheus + Grafana
- **Logs**: Loki (structured, privacy-aware)
- **Tracing**: OpenTelemetry + Jaeger (start with SaaS + KVM; Edge tracing later)

---

## 2. SaaS Control Plane Stack

### 2.1 Backend Services

**Baseline:**

- **Go** (Golang 1.23+)
  - High performance, excellent concurrency
  - Strong standard library for networking
  - Good fit for API services and microservices
  - Framework: **Gin** for HTTP APIs
  - gRPC for internal service-to-service communication
- **Primary language for all backend services**

**Alternatives:**

- **Python** (FastAPI) - Limited to internal analytics/ML tooling services only
  - Explicitly not for core business logic
  - Keeps stack from drifting into 50/50 Go/Python microservice soup
- **Echo** - Alternative Go framework if team preference

**API Design:**

- **Baseline**: RESTful APIs for external clients, gRPC for internal service-to-service communication
- **Future**: GraphQL only if UI querying patterns demand it (not for MVP)
- OpenAPI/Swagger for API documentation

### 2.2 Frontend

**Baseline:**

- **TypeScript** + **React**
  - Component library: **Tailwind CSS**
  - State management: **Zustand** (simpler than Redux for MVP)
  - Routing: React Router
  - Real-time updates: **Server-Sent Events (SSE)** for MVP
- **Responsive web + PWA first** (no native mobile for MVP)

**Alternatives:**

- Material-UI - Alternative component library
- Redux Toolkit - If state management complexity grows
- WebSocket - For more complex real-time needs
- **React Native** - For native mobile apps (phase 2, not MVP)
- Native: Swift (iOS) + Kotlin (Android) - Only if React Native doesn't meet requirements

### 2.3 Database

**Baseline:**

- **PostgreSQL 16+**
  - ACID compliance for critical data
  - JSONB for flexible event metadata
  - Strong consistency for user accounts, billing
  - Extensions: pg_trgm (text search)
  - **Start with vanilla PostgreSQL** (partitioned tables for telemetry if needed)
- **Redis**
  - Session storage
  - Event metadata caching
  - Rate limiting
  - Pub/Sub for real-time notifications

**Alternatives:**

- PostGIS - Only if location data becomes a requirement
- **TimescaleDB** - Only if telemetry volume demands specialized time-series (not for MVP)
- **InfluxDB** - Alternative time-series option (future consideration)

### 2.4 Message Queue & Event Streaming

**Baseline:**

- **RabbitMQ**
  - Simpler mental model, easier to operate
  - Good for task queues, pub/sub, alerts, async tasks
  - Used for: Event processing pipeline, async tasks, notifications

**Alternatives:**

- **Apache Kafka** - Only if throughput demands it (future consideration)

### 2.5 Authentication & Authorization

**Baseline:**

- **Auth0** (hosted OIDC provider)
  - Fastest to implement, pragmatic choice
  - OAuth2/OIDC for authentication
  - JWT tokens for API authentication
  - RBAC (Role-Based Access Control) for multi-user scenarios
  - Session management via Redis

**Alternatives:**

- **Keycloak** - Self-hosted option if more control or cost optimization needed (long-term)
- Custom implementation - Only if specific requirements not met by hosted solutions

### 2.6 File Storage

**Baseline:**

- **AWS S3** (or S3-compatible)
  - For: ISO images, software artifacts, static assets
- **CloudFlare** or **AWS CloudFront** (CDN)
  - For: ISO downloads, static web assets

**Alternatives:**

- Google Cloud Storage - If using GCP as primary cloud
- MinIO - Self-hosted option (future consideration)

### 2.7 Billing & Payments

**Baseline:**

- **Stripe** for subscription management
  - Webhook handling for payment events
  - Custom billing logic in Go services

**Alternatives:**

- **Paddle** - Alternative payment processor

---

## 3. KVM VM Stack

### 3.1 Runtime Environment

**Baseline:**

- **Docker** + **systemd**
  - Lightweight containerization
  - Easy deployment and updates
  - Services run as systemd units with baked-in configs
  - **No Kubernetes** (keeps complexity low for single-tenant VMs)

**Alternatives:**

- containerd - Alternative container runtime
- **K3s** - Only if orchestration complexity demands it (future consideration, not MVP)
- Docker Compose - For local dev & testing, not required for production

### 3.2 Core Services

**Baseline:**

- **Go** (primary language)
  - WireGuard server management
  - Event relay and caching
  - Filecoin sync orchestration
  - Stream relay service (using **Pion WebRTC**)

**Key Services:**

- WireGuard server (Go libraries or native `wg` command)
- Event cache service (Go)
- Filecoin/IPFS client (Go libraries: `go-ipfs`, `go-fil-markets`)
- Stream relay service (Go + Pion WebRTC)

### 3.3 Data Storage

**Baseline:**

- **SQLite** per tenant
  - Event cache
  - CID storage
  - Telemetry buffer
  - Simple, zero extra service overhead
  - Fits "lightweight appliance-like VM" design

**Alternatives:**

- **PostgreSQL** - Only for high-volume tenants (migration path, not default)
- Local filesystem for transient encrypted clip buffers (automatic cleanup after Filecoin upload)

### 3.4 Communication

**Baseline:**

- **gRPC** for SaaS ↔ KVM VM communication
- **mTLS** for secure API calls
  - Certificates bootstrapped during VM provisioning
  - Rotated periodically via SaaS control plane
- **WireGuard** server for Edge connections

---

## 4. Edge Appliance Stack

### 4.1 Core Runtime

**Baseline:**

- **Go** (Golang 1.23+)
  - Primary orchestrator service
  - Camera management, video processing coordination
  - Event queue and transmission
  - WireGuard client
  - gRPC client for KVM VM communication
  - Local storage management
  - **Web UI server** (embedded HTTP server)

- **Python 3.12+**
  - AI inference service only (FastAPI)
  - OpenVINO/ONNX Runtime integration
  - Minimal dependencies (OpenCV, NumPy, OpenVINO/ONNX)

### 4.2 Web UI (Local Network Accessible)

**Baseline:**

- **React 18+** + **TypeScript**
  - Matches SaaS Control Plane stack for consistency
  - Component-based architecture
  - Type safety for maintainability
- **Vite** (build tool)
  - Fast development and production builds
  - Small bundle size (important for resource-constrained Edge)
  - Excellent developer experience
- **Tailwind CSS**
  - Utility-first CSS framework
  - Matches SaaS Control Plane stack
  - Small runtime footprint
  - Responsive design out of the box
- **State Management**: React Context API + hooks
  - Simple state management (no external library needed for PoC)
  - Sufficient for local admin UI complexity
- **HTTP Client**: Fetch API (native, no dependencies)
- **Charts**: Chart.js or Recharts (for metrics visualization)
- **Icons**: Heroicons or Lucide React (lightweight SVG icons)
- **Embedding**: Go `embed` package (built-in, no external tools)
  - Static files embedded in Go binary at build time
  - Single binary deployment

**Alternatives:**

- **Alpine.js** + **Tailwind CSS** (lighter option)
  - No build step required
  - Very lightweight (~15KB)
  - Good for simple admin UIs
  - Consider if React feels like overkill
- **Vanilla JS** + **Tailwind CSS** (simplest option)
  - No framework overhead
  - Fastest to implement for PoC
  - Less maintainable for complex UIs

**Note**: The Edge Web UI is a **local admin interface** accessible on the home network (similar to router admin UI). It does not require the same scale or complexity as the SaaS Control Plane UI, but using React + TypeScript provides consistency and maintainability.

### 4.3 Video Processing

### 4.1 Operating System

**Baseline:**

- **Ubuntu Server 24.04 LTS** (or 22.04 LTS)
  - Custom ISO with pre-installed dependencies
  - Stable, well-supported

**Alternatives:**

- Debian - Minimal, stable alternative

**Container Runtime:**

- **Baseline**: Docker + systemd for services
  - Compose for local dev & testing, not required for production
  - Services run as systemd units with baked-in configs
- **Alternative**: Podman - If Docker not preferred

### 4.2 Core Services

**Language Split:**

- **Go** (primary orchestrator)
  - Main orchestrator service ("conductor")
  - Camera discovery and management
  - Service coordination
  - WireGuard client management
  - Storage management (local clip storage, retention policies, disk space)
  - Encryption (clip encryption using AES-256-GCM)
- **Python 3.12+** (AI inference only)
  - Strictly inference-only over clean API (gRPC/HTTP)
  - No business logic in Python
  - Keeps two-runtime complexity manageable

**Video Processing:**

- **Baseline**: **FFmpeg**
  - Hardware-accelerated decoding (Intel QSV via VAAPI, NVIDIA NVENC if available)
  - Software fallback for unsupported hardware
  - Frame extraction and preprocessing
  - Clip encoding (H.264/MP4)
- **Camera Access:**
  - **Network Cameras**: RTSP client (`gortsplib`) for IP cameras
  - **USB Cameras**: V4L2 (Video4Linux2) for direct USB camera access
    - Automatic detection via `/dev/video*` devices
    - Device information via `v4l2-ctl` or sysfs
    - Direct device path access (e.g., `/dev/video0`) for FFmpeg
  - Video decoding, encoding, transcoding
  - RTSP/ONVIF stream handling
  - Hardware acceleration support (Intel QSV, NVIDIA NVENC)
  - Minimal **OpenCV** for preprocessing

**Alternatives:**

- **GStreamer** - Only for advanced pipelines that can't be expressed cleanly with FFmpeg

**AI Inference:**

- **Baseline**: **OpenVINO** (Intel)
  - Primary AI inference engine
  - Optimized for Intel CPUs/iGPUs
- **Fallback**: **ONNX Runtime** (if OpenVINO not supported / future ARM SKU)

**Encryption:**

- **Baseline**: **Argon2id** for key derivation
  - AES-256-GCM for clip encryption
- **Alternative**: PBKDF2 - Only if constrained environment demands it

### 4.3 Video Libraries

- **FFmpeg** (libavcodec, libavformat)
- **GStreamer** (optional, for more complex pipelines)
- **OpenCV** (Python/C++)
  - Frame extraction
  - Image preprocessing for AI

### 4.4 Networking

- **WireGuard** (Go client or `wg` command)
  - Persistent tunnel to KVM VM
- **Network Camera Access:**
  - **RTSP/ONVIF** clients for IP cameras
    - Camera discovery and streaming
    - WS-Discovery for ONVIF cameras
- **USB Camera Access:**
  - **V4L2 (Video4Linux2)** for USB cameras
    - Automatic device detection (`/dev/video*`)
    - Direct device path access for FFmpeg
    - Hotplug support (detect cameras when plugged/unplugged)

### 4.5 Local Storage

**Baseline:**

- **Local filesystem** (ext4)
  - Raw video clips
  - Snapshots
  - SQLite for local metadata
- **Backpressure behavior**: Oldest clips deleted when disk full, recording pauses if necessary

---

## 5. AI/ML Stack

### 5.1 Inference Framework

**Baseline:**

- **OpenVINO** (Intel)
  - Optimized for Intel CPUs and iGPUs (target hardware)
  - Model conversion from ONNX/TensorFlow
  - High performance on edge devices
  - **IR (Intermediate Representation)** + **ONNX** models

**Fallback:**

- **ONNX Runtime**
  - Cross-platform (Intel, ARM, NVIDIA)
  - Good fallback if OpenVINO unavailable

**Future Considerations:**

- TensorFlow Lite - For ARM devices (not MVP)
- PyTorch Mobile - If needed (not MVP)

### 5.2 Model Formats

**Baseline:**

- **ONNX** (primary format)
  - Interoperable format
  - Convert from TensorFlow/PyTorch
  - **All production models exported to ONNX as common format before optimization**
- **OpenVINO IR** (Intermediate Representation)
  - Optimized for Intel hardware

### 5.3 Model Management

**Baseline:**

- **Object storage** (S3) + **Git** versioning (or simple DB table)
  - Artifacts in object storage
  - Versioning in Git or database
  - Distribution: Encrypted model artifacts via SaaS → KVM VM → Edge

**Alternatives:**

- **MLflow** - Only if ML lifecycle complexity demands it (future consideration)

### 5.4 Pre-trained Models

**Baseline:**

- **YOLOv8** (object detection)
  - Primary detection model
- **MobileNet** variants (efficient edge inference)
  - For resource-constrained scenarios

**Model Sources:**

- Ultralytics (YOLO)
- TensorFlow Model Zoo
- Custom training pipeline (separate infrastructure)

**Alternatives:**

- YOLOv5 - Alternative to YOLOv8
- Custom fine-tuned models - For specific use cases

### 5.5 Model Serving

**Baseline:**

- **Python service** with OpenVINO/ONNX Runtime
- **gRPC** or **HTTP** API for inference requests
- **Batching is optional** - Edge use cases are often latency-sensitive; micro-batching only if proven beneficial on hardware

---

## 6. Video Processing Stack

### 6.1 Decoding & Encoding

**Primary Tool:**

- **FFmpeg**
  - Hardware-accelerated decoding (Intel QSV, NVIDIA NVENC)
  - Format conversion
  - Stream handling

**Libraries:**

- **libavcodec** (FFmpeg core)
- **libavformat** (container formats)
- **libavfilter** (video filters)

### 6.2 Hardware Acceleration

**Intel:**

- **Intel Quick Sync Video (QSV)**
  - Hardware-accelerated decode/encode
  - VAAPI (Video Acceleration API)

**NVIDIA (if supported):**

- **NVENC/NVDEC**
  - Hardware acceleration on NVIDIA GPUs

**Software Fallback:**

- Software decoding via FFmpeg
- Slower but universal

### 6.3 Stream Protocols

- **RTSP** (Real-Time Streaming Protocol)
  - Client libraries: `gortsplib` (Go), `live555` (C++)
- **ONVIF** (camera discovery and control)
  - Libraries: `onvif` (Python), `go-onvif` (Go)
- **WebRTC** (for remote viewing)
  - Libraries: `Pion WebRTC` (Go), `aiortc` (Python)

### 6.4 Video Storage Formats

- **MP4** (H.264/H.265)
  - Standard format for clips
  - Good compression
- **JPEG/PNG** for snapshots
- **Container**: MP4, MKV

---

## 7. Infrastructure & DevOps

### 7.1 Cloud Infrastructure

**Baseline:**

- **AWS** (primary cloud)
  - Multi-AZ deployment
  - Auto-scaling groups
  - Load balancers
  - EKS for Kubernetes
- **Hetzner** (KVM hosting)
  - Dedicated servers with KVM hypervisor
  - Cost-effective for per-tenant VMs

**Alternatives:**

- Google Cloud Platform - If using GCP as primary cloud
- Azure - Alternative cloud option
- OVH - Alternative KVM hosting provider
- Cloud VMs (AWS EC2, GCP Compute Engine) - Alternative to dedicated servers

### 7.2 Container Orchestration

**SaaS Layer:**

- **Baseline**: **Kubernetes (EKS)**
  - Auto-scaling
  - Ingress controllers: **NGINX**
  - **Plain Kubernetes with cert-manager for TLS** (no service mesh initially)
- **Phase 2**: Service mesh (Istio or Linkerd) for fine-grained mTLS and observability

**KVM VM Layer:**

- **Docker + systemd** (no orchestration)
  - Simpler, single-tenant per VM

**Edge Appliance:**

- **Docker + systemd services**
  - Simple, reliable, low overhead

### 7.3 Infrastructure as Code

- **Terraform**
  - Cloud infrastructure provisioning
  - KVM VM provisioning automation
- **Ansible** or **Pulumi**
  - Configuration management
  - Service deployment

### 7.4 CI/CD

- **GitHub Actions**, **GitLab CI**, or **Jenkins**
  - Automated testing
  - Container builds
  - Deployment pipelines
- **Container Registry**: Docker Hub, AWS ECR, GCR
- **Artifact Storage**: For ISO images, software packages

### 7.5 ISO Generation

- **Custom build pipeline**
  - Base: Ubuntu/Debian ISO
  - **Packer** for image building
  - **cloud-init** for first-boot configuration
  - Tenant-specific configuration injection
  - Secure download endpoint

---

## 8. Security & Networking

### 8.1 VPN & Tunneling

- **WireGuard**
  - Edge ↔ KVM VM encrypted tunnel
  - Go libraries: `golang.zx2c4.com/wireguard`
  - Native `wg` command-line tool

### 8.2 TLS/mTLS

- **TLS 1.3** for all HTTPS communication
- **mTLS** for SaaS ↔ KVM VM
  - Certificate management: **cert-manager** (Kubernetes) or custom
  - CA: Internal CA or Let's Encrypt

### 8.3 Encryption

- **AES-256-GCM** for clip encryption
- **Key derivation**: PBKDF2 or Argon2
- **Go crypto libraries**: `crypto/aes`, `golang.org/x/crypto`

### 8.4 Secrets Management

**Baseline:**

- **AWS Secrets Manager** (or SSM Parameter Store)
  - For SaaS secrets
  - Simpler than introducing Vault on top of AWS
- **Edge Appliance**: Local key derivation from user secret
  - Never stored in central systems

**Alternatives:**

- **HashiCorp Vault** - If outgrowing cloud-native tooling or multi-cloud (future consideration)

### 8.5 Network Security

- **Firewall rules**: iptables, firewalld
- **Network policies**: Kubernetes NetworkPolicy
- **DDoS protection**: CloudFlare, AWS Shield

---

## 9. Monitoring & Observability

### 9.1 Metrics

**Baseline:**

- **Prometheus**
  - Metrics collection
  - Time-series database
- **Grafana**
  - Dashboards and visualization
- **Custom exporters**: For Edge Appliance metrics

### 9.2 Logging (Privacy-Aware)

**Baseline:**

- **Loki** (Grafana)
  - Centralized logging
  - Structured logging: JSON format
  - Log aggregation: From Edge → KVM VM → SaaS

**Privacy Constraints:**

- Logs from Edge **must not** contain:
  - Raw video frames
  - PII-heavy data (full free-text face descriptions, etc.)
- Edge logs should be:
  - Structured, event-level: camera ID, event ID, status codes, errors, resource metrics
- **Sensitive logs can remain local to Edge** unless explicitly needed for support and explicitly opted-in by user

**Alternatives:**

- **ELK Stack** (Elasticsearch, Logstash, Kibana) - Alternative centralized logging solution

### 9.3 Tracing

**Baseline:**

- **OpenTelemetry**
  - Distributed tracing
  - Instrumentation for Go, Python services
- **Jaeger**
  - Trace storage and visualization
  - Start with SaaS + KVM; add Edge tracing later

**Alternatives:**

- **Tempo** (Grafana) - Alternative trace storage

### 9.4 Alerting

**Baseline:**

- **Alertmanager** (Prometheus)
  - Alert routing and notification
- **Slack** integration (or PagerDuty/Opsgenie)
- **Health checks**: Custom endpoints for each service

### 9.5 Uptime Monitoring

- **Synthetic monitoring**: Custom health check services
- **Edge Appliance heartbeat**: Regular telemetry to KVM VM

---

## 10. Development Tools

### 10.1 Version Control

- **Git** (GitHub, GitLab, or self-hosted)
- **Git LFS** for large files (models, ISO images)

### 10.2 Code Quality

- **Linters**: `golangci-lint` (Go), `pylint`/`black` (Python), `eslint` (TypeScript)
- **Formatters**: `gofmt`, `black`, `prettier`
- **Static Analysis**: SonarQube, CodeQL

### 10.3 Testing

- **Unit Testing**:
  - Go: `testing` package, `testify`
  - Python: `pytest`
  - TypeScript: `Jest`, `Vitest`
- **Integration Testing**: Docker Compose test environments
- **E2E Testing**: Playwright, Cypress (for web UI)

### 10.4 Documentation

- **API Documentation**: OpenAPI/Swagger
- **Code Documentation**: GoDoc, Sphinx (Python), JSDoc
- **Architecture Diagrams**: Mermaid, PlantUML, or draw.io

### 10.5 Local Development

- **Docker Compose** for local service stack
- **Minikube** or **Kind** for Kubernetes testing
- **Mock services** for external dependencies

---

## Technology Decision Rationale

### Why Go for Backend Services?

- **Performance**: Excellent for concurrent, I/O-bound services
- **Simplicity**: Easy to learn and maintain
- **Deployment**: Single binary, easy containerization
- **Ecosystem**: Strong libraries for networking, crypto, WireGuard
- **Consistency**: Single language across SaaS, KVM VM, and Edge orchestrator

### Why OpenVINO for AI Inference?

- **Hardware Optimization**: Best performance on Intel hardware (target hardware for Edge Appliances)
- **Edge-Friendly**: Designed for resource-constrained devices
- **Model Compatibility**: Supports ONNX, TensorFlow, PyTorch models
- **Primary Choice**: Matches our Intel Mini PC target hardware

### Why PostgreSQL?

- **ACID Compliance**: Critical for user accounts, billing
- **Flexibility**: JSONB for flexible event metadata
- **Maturity**: Battle-tested, excellent tooling
- **Start Simple**: Vanilla PostgreSQL first; add TimescaleDB only if telemetry volume demands it

### Why SQLite for KVM VMs?

- **Simplicity**: Zero extra service overhead
- **Lightweight**: Fits "appliance-like VM" design
- **Adequate**: Sufficient for per-tenant event cache and CID storage
- **Migration Path**: Can upgrade to PostgreSQL for high-volume tenants if needed

### Why RabbitMQ over Kafka?

- **Simplicity**: Easier mental model, simpler to operate
- **Adequate**: Sufficient for MVP use cases (alerts, async tasks, pub/sub)
- **Migration Path**: Can evolve to Kafka if throughput demands it

### Why WireGuard?

- **Performance**: Faster than OpenVPN, IPsec
- **Simplicity**: Modern, clean codebase
- **Security**: State-of-the-art cryptography
- **Resource Usage**: Low overhead, perfect for edge devices

### Why Docker/Containers?

- **Isolation**: Clean separation of services
- **Portability**: Same image runs on SaaS, KVM VM, Edge
- **Updates**: Easy rollback and versioning
- **Resource Management**: Better than full VMs for edge devices

### Why No Kubernetes on KVM VMs?

- **Complexity**: Running K8s inside every per-tenant VM adds significant overhead
- **Unnecessary**: Single-tenant VMs don't need orchestration complexity
- **Simplicity**: Docker + systemd is sufficient and easier to maintain

---

## Version Constraints & Compatibility

### Minimum Versions

- **Go**: 1.25+
- **Python**: 3.12+
- **Node.js**: 22+ LTS (for frontend)
- **PostgreSQL**: 16+
- **Docker**: 27.0+
- **Ubuntu**: 24.04 LTS (Edge Appliance) or 22.04 LTS (supported until 2027)

### Hardware Requirements

**Edge Appliance (Minimum):**

- CPU: Intel N100 or equivalent (4 cores)
- RAM: 8GB
- Storage: 256GB SSD
- Network: Gigabit Ethernet
- iGPU: Intel UHD Graphics (for hardware acceleration)

**KVM VM (Per Tenant):**

- CPU: 2 vCPUs
- RAM: 4GB
- Storage: 50GB

---

## Future Considerations

These technologies are **not part of the MVP baseline** but may be considered as the platform evolves.

### Potential Additions

- **WebAssembly (WASM)**: For browser-based video processing
- **Rust**: For performance-critical components (video codecs, crypto)
- **eBPF**: For advanced network monitoring
- **WebGPU**: For browser-based AI inference
- **GraphQL**: If UI querying patterns demand it
- **React Native**: For native mobile apps (phase 2)
- **Service Mesh (Istio/Linkerd)**: For fine-grained mTLS and observability (phase 2)
- **TimescaleDB/InfluxDB**: If telemetry volume demands specialized time-series
- **Kafka**: If message queue throughput demands it
- **K3s on KVM VMs**: Only if orchestration complexity demands it
- **MLflow**: If ML lifecycle complexity demands it

### Scalability Considerations

- **Message Queue**: Scale from RabbitMQ to Kafka if volume grows significantly
- **Database**: May need read replicas, sharding for very large scale
- **CDN**: Essential for ISO downloads at scale
- **Edge Compute**: May add edge compute nodes for lower latency
- **Multi-Cloud**: Consider only if specific requirements demand it (not from day 1)

---

*This technical stack document should be reviewed and updated as the project evolves and new requirements emerge.*
