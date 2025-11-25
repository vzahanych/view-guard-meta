# Project Structure & Repository Organization

This document defines the repository structure, open source strategy, and organization for The Private AI Guardian platform.

**Note on Naming**: Code repositories use the `view-guard-*` prefix. The commercial product name ("Private AI Guardian") may evolve, but repository names should be stable once finalized to avoid breaking Go module paths and integrations.

## Overview

The project is organized into **public open-source repositories** (for privacy-critical components that run on customer hardware) and **private repositories** (for multi-tenant SaaS, billing, and infrastructure). This structure supports our **"trust us / verify us" privacy story** while protecting commercial IP.

**Key Structure Decision**: The meta repository (`view-guard-meta`) is **public** and serves as the **developer landing page** for the entire ecosystem. It:
- Contains all public open-source components (edge, crypto, protocols) developed directly in the repo
- Documents the architecture, threat model, and what is verifiable
- Explains how customers can audit and self-host
- Makes wire protocols discoverable as potential standards
- **Does NOT contain** internal roadmaps, production infrastructure, or private code

All private code lives in **separate private repositories**, which the meta repo **may reference as submodules** for internal developers. This provides:
- Clear separation between verifiable components and SaaS glue
- Transparency about project structure without exposing private code
- Ability for public contributors to understand the full project organization
- Support for the "trust us / verify us" narrative

**The Trust Story**: The public meta repo makes it clear which components customers can audit (edge, crypto, protocols) versus which are proprietary SaaS (billing, multi-tenant operations). This transparency is core to the privacy-first positioning.

## Repository Strategy

### Public Repositories (Open Source)

These components are open source to enable:
- **Auditability**: Customers can verify privacy guarantees
- **Trust**: Security-sensitive code (crypto, AI inference) is transparent
- **Community**: Encourages integrations and third-party tooling
- **Standards**: Wire protocols become open standards

### Private Repositories

These components remain private to protect:
- **Commercial moat**: Multi-tenant SaaS, billing, pricing models
- **Operational IP**: VM provisioning, ISO generation, infrastructure automation
- **Product differentiation**: UI/UX, feature flags, enterprise integrations

### Meta Repository (Public - Developer Landing Page)

The meta repository (`view-guard-meta`) is **public** and serves as the **entry point for developers and customers**. It is the "landing page" for the entire ecosystem.

**Purpose:**
- ✅ **Trust Story**: Shows the map of what is verifiable vs what is SaaS glue
- ✅ **Developer Onboarding**: One place to understand all components and how they fit together
- ✅ **Architecture Documentation**: Public architecture, threat model, and protocols
- ✅ **Component Discovery**: Links to all public repositories (edge, crypto, protocols, SDKs)
- ✅ **Standards Surface**: Makes wire protocols discoverable and forkable
- ✅ **Self-Hosting Guide**: Explains what customers can audit and self-host

**What's in the Public Meta Repo:**
- Architecture documentation (ARCHITECTURE.md, TECHNICAL_STACK.md)
- Implementation plan (public version)
- Links to all public repositories
- Threat model and security guarantees
- Protocol specifications
- Examples and reference deployments
- Developer guides

**What's NOT in the Public Meta Repo:**
- ❌ Internal roadmaps or pricing experiments
- ❌ Production infrastructure automation (Terraform, K8s manifests)
- ❌ Secrets handling or internal runbooks
- ❌ Sales playbooks or investor decks
- ❌ Direct references to private repos that break for public users

**Why Public:**
- **Trust**: Supports "trust us / verify us" narrative - customers can see what's verifiable
- **Standards**: If protocols become de facto standards, they must be publicly linkable
- **Community**: Makes it easy for developers to discover and contribute
- **Transparency**: Shows clear separation between verifiable components and SaaS glue

**Note**: Internal-only documentation (roadmaps, ops, billing details) lives in separate private repositories, not in the public meta repo.

---

## Repository Structure

### Main Meta Repository (Public - Developer Landing Page)

**Purpose**: The public meta repository is the **entry point for developers and customers**. It documents the architecture, threat model, and links to all open-source components. This is where customers and integrators understand what is verifiable and how to self-host.

```
view-guard-meta/                    (PUBLIC - Developer landing page)
├── README.md                       (Main project overview, "trust us / verify us" story)
├── LICENSE                         (Apache 2.0)
├── VERSIONS.md                     (Component version tracking)
├── .gitmodules                     (Git submodule definitions - private repos only)
│
├── docs/                           (Public documentation)
│   ├── ARCHITECTURE.md             (High-level architecture, threat model)
│   ├── TECHNICAL_STACK.md          (Technical stack - public)
│   ├── IMPLEMENTATION_PLAN.md      (Implementation plan - public version)
│   ├── PROJECT_STRUCTURE.md        (This document)
│   └── SELF_HOSTING.md             (Guide for self-hosting)
│
├── examples/                       (Reference deployments, sample apps)
│   ├── minimal-edge-setup/
│   └── self-host-example/
│
├── scripts/
│   └── bootstrap-dev.sh            (Bootstrap script for developers)
│
├── Public Components (developed directly in meta repo):
│   ├── edge/                       (Edge Appliance software)
│   │   ├── orchestrator/         (Go orchestrator service)
│   │   ├── ai-service/           (Python AI inference service)
│   │   ├── shared/                (Shared Go libraries)
│   │   └── go.mod                 (Go module)
│   │
│   ├── user-vm-api/                (User Server - user's private cloud node)
│   │   ├── wireguard-server/     (WireGuard server, client management)
│   │   ├── event-cache/           (Event cache with SQLite)
│   │   ├── stream-relay/          (HTTP/WebRTC stream relay)
│   │   ├── filecoin-sync/         (Filecoin/IPFS integration)
│   │   ├── ai-orchestrator/       (AI model catalog, retraining, distribution)
│   │   ├── event-analyzer/        (Secondary event analysis)
│   │   ├── agent-orchestrator/    (Main agent service)
│   │   └── go.mod                 (Go module)
│   │
│   ├── crypto/                     (Encryption libraries)
│   │   ├── go/                    (Go encryption library)
│   │   ├── typescript/             (Browser/Node.js library)
│   │   └── python/                 (Python library)
│   │
│   └── proto/                      (Protocol buffer definitions)
│       ├── proto/                  (Protocol definitions)
│       ├── go/                     (Generated Go stubs)
│       ├── typescript/             (Generated TypeScript stubs)
│       └── python/                 (Generated Python stubs)
│
└── Private Submodules (optional, require access):
    ├── saas-backend/               (git submodule → view-guard-saas-backend)
    ├── saas-frontend/              (git submodule → view-guard-saas-frontend)
    └── infra/                      (git submodule → view-guard-infra)
```

**Note**: Everything in this repository is customer-facing documentation, examples, and coordination code. Internal-only material (roadmaps, ops, GTM, production infrastructure) lives in private repositories such as `view-guard-internal` and `view-guard-infra`. The public README should clearly state that some components are private (SaaS, billing) and that customers don't need them to self-host or verify privacy guarantees.

### Internal Meta Repository (Optional - Private)

For internal-only documentation, consider a separate private repository:

```
view-guard-internal/                (PRIVATE - Internal docs only)
├── README.md                       (Internal overview)
├── roadmap/                        (Product roadmap, experiments)
├── ops/                            (Internal runbooks, procedures)
├── gtm/                            (Go-to-market materials)
└── infra-internal/                 (Production infrastructure details)
```

**Why Separate:**
- Keeps internal-only content out of public meta repo
- Allows public meta repo to be clean and customer-facing
- Internal team has one place for private documentation

---

## Public Component Details

Public components are developed directly in the meta repository as directories, not as separate repositories or submodules.

### 1. `edge/` (Public - Apache 2.0)

**Location**: `view-guard-meta/edge/`

**Description**: Edge Appliance software for The Private AI Guardian - local video processing, AI inference, and privacy-first security

**Contents:**

```
edge/
├── README.md                       (Edge Appliance overview, privacy guarantees)
├── orchestrator/                   (Go orchestrator service)
│   ├── camera/                     (RTSP/ONVIF client, discovery)
│   ├── video/                      (FFmpeg integration, decoding)
│   ├── storage/                    (Local clip storage, retention)
│   ├── events/                     (Event generation, queueing)
│   ├── wireguard/                  (WireGuard client)
│   ├── telemetry/                  (Telemetry collection)
│   └── encryption/                 (Uses ../crypto/go as dependency)
│
├── ai-service/                     (Python AI inference service)
│   ├── inference/                  (OpenVINO/ONNX Runtime)
│   ├── models/                     (Model loading, management)
│   ├── detection/                  (YOLO detection logic)
│   └── api/                        (gRPC/HTTP inference API)
│
├── shared/                         (Shared Go libraries)
│   ├── config/
│   ├── logging/
│   └── utils/
│
├── go.mod                          (Go module dependencies)
│   ├── github.com/yourorg/view-guard-meta/crypto/go  (Crypto library)
│   └── github.com/yourorg/view-guard-meta/proto/go  (Proto stubs)
│
├── config/                         (Configuration examples)
├── scripts/                        (Build and deployment scripts)
└── docs/                           (Edge-specific documentation)
    ├── INSTALLATION.md
    ├── CONFIGURATION.md
    └── PRIVACY.md
```

**What's Public:**
- ✅ Camera discovery and RTSP/ONVIF client
- ✅ FFmpeg integration and video decoding
- ✅ Local clip storage and retention logic
- ✅ AI inference service (OpenVINO/YOLO pipeline)
- ✅ Event generation and queueing
- ✅ WireGuard client implementation
- ✅ Telemetry collection (local metrics)
- ✅ Uses `crypto/go` library for encryption (no duplication)

**Dependencies:**
- Imports `crypto/go` from the same meta repo for all encryption operations
- Imports `proto/go` from the same meta repo for gRPC proto stubs
- **Fully buildable** as part of the meta repository

**What's Private (or split):**
- ❌ License checks or feature flags
- ❌ Proprietary/custom AI models (if/when developed)
- ❌ Enterprise-only integrations

**Note**: Pre-compiled models can be shipped alongside open code, or the runtime can be open while keeping some models private.

---

### 2. `crypto/` (Public - Apache 2.0)

**Location**: `view-guard-meta/crypto/`

**Description**: End-to-end encryption libraries for The Private AI Guardian - client-side decryption tools for privacy-first video archiving

**Contents:**

```
crypto/
├── README.md                       (Encryption model, key derivation)
├── go/                             (Go encryption library)
│   ├── encryption/                 (AES-256-GCM encryption)
│   ├── keyderivation/              (Argon2id key derivation)
│   └── archive/                    (Archive encryption client)
│
├── typescript/                     (Browser/Node.js library)
│   ├── encryption/
│   ├── keyderivation/
│   └── browser/                    (Browser-specific decryption)
│
├── python/                         (Python library)
│   ├── encryption/
│   └── keyderivation/
│
├── cli/                            (Optional CLI tool)
│   └── decrypt/                    (Command-line decryption)
│
└── docs/
    ├── ENCRYPTION_MODEL.md
    ├── KEY_DERIVATION.md
    └── USAGE.md
```

**What's Public:**
- ✅ **Single source of truth** for all encryption primitives
- ✅ Edge encryption client (AES-256-GCM, Argon2id) - used by `edge/`
- ✅ Browser-based decryption library (TypeScript)
- ✅ CLI decryption tool (optional)
- ✅ Key derivation implementation (Argon2id)
- ✅ Encryption metadata handling

**Why Public:**
- This is the **heart of the end-to-end encryption guarantee**
- **No duplication** - `edge/` imports this library, ensuring one canonical implementation
- Enables auditors and power users to verify "no backdoors"
- Critical for trust in the privacy model

**Note**: `edge/` uses this library as a Go module dependency from the same meta repo. There is no crypto code duplicated in the edge directory.

---

### 3. `proto/` (Public - Apache 2.0)

**Location**: `view-guard-meta/proto/`

**Description**: Protocol buffer definitions and SDKs for The Private AI Guardian platform APIs

**Contents:**

```
proto/
├── README.md                       (API overview, protocol documentation)
├── proto/                          (Protocol buffer definitions - SINGLE SOURCE OF TRUTH)
│   ├── edge/                       (Edge ↔ KVM VM)
│   │   ├── events.proto
│   │   ├── telemetry.proto
│   │   ├── control.proto
│   │   └── streaming.proto
│   │
│   └── kvm/                        (KVM VM ↔ SaaS)
│       ├── events.proto
│       ├── telemetry.proto
│       └── commands.proto
│
├── go/                             (Generated Go stubs)
│   └── generated/
│
├── typescript/                     (Generated TypeScript stubs)
│   └── generated/
│
├── python/                         (Generated Python stubs)
│   └── generated/
│
├── go.mod                          (Go module for proto stubs)
└── docs/
    ├── API_REFERENCE.md
    └── PROTOCOL.md
```

**What's Public:**
- ✅ **Single source of truth** for all `.proto` definitions
- ✅ All `.proto` definitions for Edge ↔ KVM VM communication
- ✅ All `.proto` definitions for KVM VM ↔ SaaS communication
- ✅ Generated language stubs (Go, TypeScript, Python)
- ✅ Protocol documentation

**What's NOT Included:**
- ❌ SaaS internal service-to-service protos (kept in private `saas-backend` repo)
- ❌ HTTP REST API definitions (OpenAPI/Swagger in private `saas-backend` repo)

**Usage:**
- `edge/` imports this as Go module dependency from the same meta repo
- `user-vm-api` (public component) imports this as Go module dependency
- **No copying, no symlinks** - all components depend on this single source

**Why Public:**
- Encourages integrations and third-party tooling
- Locks in wire protocol as an open standard
- Enables community contributions and alternative implementations
- **Fully buildable** as part of the meta repository

---

## Public Component Details (continued)

### 4. `user-vm-api/` (Public - Apache 2.0)

**Location**: `view-guard-meta/user-vm-api/`

**Description**: User VM API for The Private AI Guardian - WireGuard server, event cache, stream relay, Filecoin integration, AI model orchestration, and secondary event analysis

**Why Open Source:**
- Runs on user's dedicated VM (their private cloud node)
- Handles user data and AI models
- Secrets (WireGuard keys, encryption key identifiers) are kept in memory only at runtime
- Supports the "trust us / verify us" privacy story
- Enables full auditability of user data processing

**Contents:**

```
user-vm-api/
├── README.md                       (User VM API overview, privacy guarantees)
├── wireguard-server/               (WireGuard server, client management)
├── event-cache/                    (Event cache with SQLite)
├── stream-relay/                   (HTTP/WebRTC stream relay)
├── filecoin-sync/                  (Filecoin/IPFS integration)
│   ├── uploader/                   (Filecoin upload logic)
│   ├── quota/                      (Quota enforcement)
│   └── cid-storage/                (CID management)
├── ai-orchestrator/                (AI model catalog, retraining, distribution)
├── event-analyzer/                 (Secondary event analysis for alerting)
├── telemetry-aggregator/           (Telemetry aggregation logic)
├── agent-orchestrator/             (Main agent service)
└── go.mod                          (Go module dependencies)
    └── view-guard-proto/go         (Proto stubs dependency)
```

**Note on Secrets:**
- WireGuard keys, encryption key identifiers, and other secrets are loaded into memory at runtime
- Secrets are not stored in the codebase or committed to version control
- Secrets are provided via environment variables, configuration files (excluded from git), or secure key management systems

---

## Private Repository Details

### 5. `view-guard-saas-backend` (Private)

**GitHub Description**: "SaaS Control Plane backend for The Private AI Guardian - authentication, event inventory, VM provisioning, billing"

**Contents:**

```
view-guard-saas-backend/
├── README.md                       (Internal documentation)
├── api/                            (REST API service - Gin)
├── auth/                           (Auth0 integration, JWT validation)
├── events/                         (Event inventory service)
├── provisioning/                   (VM provisioning, Terraform)
├── billing/                        (Stripe integration, subscriptions)
├── iso-generation/                 (ISO build pipeline, Packer)
├── health/                         (Health monitoring service)
├── internal-proto/                 (Internal service-to-service protos)
├── shared/                         (Shared libraries)
└── go.mod                          (Go module dependencies)
    └── view-guard-proto/go         (Proto stubs dependency)
```

**What's Private:**
- All SaaS backend services
- Stripe/Auth0 integrations
- VM provisioning logic (Terraform, Hetzner/AWS specifics)
- ISO generation pipeline
- Billing and subscription management
- Multi-tenant data isolation logic
- Internal service protos (not in `view-guard-proto`)
- HTTP REST API definitions (OpenAPI/Swagger)

---

### 6. `view-guard-saas-frontend` (Private)

**GitHub Description**: "SaaS Control Plane frontend for The Private AI Guardian - React web application"

**Contents:**

```
view-guard-saas-frontend/
├── README.md                       (Internal documentation)
├── src/
│   ├── components/                 (React components)
│   ├── pages/                      (Page components)
│   ├── services/                    (API services)
│   ├── hooks/                       (React hooks)
│   └── utils/                       (Utilities)
├── public/                          (Static assets)
└── package.json                    (Node.js dependencies)
```

**What's Private:**
- Entire React web application
- Subscription management UI
- Multi-tenant dashboards
- Admin tooling
- Billing UI
- Product-specific UX/UI

---

### 7. `view-guard-infra` (Private)

**GitHub Description**: "Infrastructure as Code for The Private AI Guardian - Terraform, Kubernetes, and deployment automation"

**Contents:**

```
view-guard-infra/
├── README.md                       (Internal documentation)
├── terraform/                      (Infrastructure as Code)
│   ├── saas/                       (Production SaaS infrastructure)
│   │   ├── aws/                    (AWS resources)
│   │   ├── kvm-hosts/              (KVM host provisioning)
│   │   └── networking/             (Network configuration)
│   └── demo/                       (Demo/dev environments - smaller stack)
│       └── local/                  (Local development setup)
├── kubernetes/                     (K8s manifests)
│   ├── saas/                       (SaaS service deployments)
│   ├── demo/                       (Demo environment)
│   └── monitoring/                 (Prometheus, Grafana)
├── ansible/                        (Configuration management)
└── scripts/                        (Deployment automation)
```

**What's Private:**
- All infrastructure automation
- Terraform configurations
- Kubernetes manifests
- Deployment scripts
- Secrets management
- Operational runbooks

**Subfolder Convention:**
- `saas/`: Production infrastructure
- `demo/` or `dev/`: Smaller stack for demo environments and local development
- This allows easy separation of production vs development infrastructure without separate repos

---

## Development Workflow

### Initial Setup

```bash
# Clone the main meta repository (public)
git clone git@github.com:yourorg/view-guard-meta.git
cd view-guard-meta

# Use bootstrap script (recommended)
./scripts/bootstrap-dev.sh

# Or manually initialize private submodules (if you have access)
# Note: Private submodules require access to private repos
git submodule update --init --recursive
```

**Access Requirements:**
- **Public components**: Available immediately - they're part of the meta repo
  - `edge/` - Edge Appliance software
  - `user-vm-api/` - User Server (runs on user's VM)
  - `crypto/` - Encryption libraries
  - `proto/` - Protocol definitions
- **Private submodules**: Require access to private repositories
  - `saas-backend/` → `view-guard-saas-backend`
  - `saas-frontend/` → `view-guard-saas-frontend`
  - `infra/` → `view-guard-infra`

### Bootstrap Script

The meta repository includes `scripts/bootstrap-dev.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Public components are already in the repo, no initialization needed
echo "Public components (edge/, crypto/, proto/) are already available."

# Try to init private submodules, but don't fail if they are inaccessible
echo "Attempting to initialize private submodules (if you have access)..."
if ! git submodule update --init management-server saas-backend saas-frontend infra 2>/dev/null; then
  echo "Warning: Some private submodules could not be initialized. This is expected if you are an external contributor."
fi

echo "Running sanity checks on public components..."
pushd edge && go test ./... && popd
pushd proto && go test ./... || true && popd

echo "Bootstrap complete! Public components are ready."
```

**Important**: The bootstrap script **never fails** just because you don't have access to private repos. It will still set up and test the public components, making it friendly for external contributors.

This gives new developers a single command to get started. Public contributors can work with public components even without access to private repos.

### Working with Private Submodules

```bash
# Update private submodules to latest commits
git submodule update --remote

# Update to specific tagged version (recommended)
# user-vm-api is now public, developed directly in meta repo
# No submodule management needed

# Update all private submodules to latest
git submodule update --remote --merge
```

### Development Workflow

1. **Working on public components**: Make changes directly in `edge/`, `crypto/`, or `proto/` directories
2. **Committing changes**: Commit directly to the meta repository
3. **Tagging releases**: Tag the meta repo (v0.1.0, v0.2.0) when releasing public components
4. **Working on private components**: Make changes in private submodule directories, commit to submodule repo, then commit submodule reference in meta repo

### Versioning Strategy

**Semantic Versioning (SemVer)** for all public repositories:

- **Major version** (v1.0.0): Breaking changes
- **Minor version** (v0.1.0): New features, backward compatible
- **Patch version** (v0.0.1): Bug fixes, backward compatible

**Special considerations:**
- `view-guard-proto`: Breaking changes in `.proto` files should bump major version
- `view-guard-crypto`: Breaking changes in encryption API should bump major version
- `view-guard-edge`: Follows standard SemVer

**Version tracking in meta repo:**

Maintain `VERSIONS.md` in the meta repository:

```markdown
# Component Versions

view-guard-edge: v0.3.1
view-guard-crypto: v0.2.0
view-guard-proto: v0.4.0
```

**Best practice**: Pin submodules to **tags**, not arbitrary commits, whenever possible.

### Buildability

**Public components are built as part of the meta repository:**

- `edge/`: Built as part of the meta repo
  - Imports `crypto/go` as Go module from the same repo
  - Imports `proto/go` as Go module from the same repo
  - Can be built independently within the repo structure

- `crypto/`: Built as part of the meta repo
  - No external dependencies (except standard libraries)
  - Can be built independently within the repo structure

- `proto/`: Built as part of the meta repo
  - Generates stubs without external dependencies
  - Can be built independently within the repo structure

**Benefits:**
- Single repository for all public components simplifies development
- Unified CI/CD pipeline for all public components
- Easier cross-component changes and refactoring
- Single source of truth for versioning

### CI/CD Considerations

- **Public components**: Built and tested together in the meta repo CI/CD pipeline
  - Tests run for all public components
  - Releases are tagged on the meta repo
- **Private submodules**: CI/CD pulls in private submodules at specific versions
  - Uses `git submodule update --init --recursive` in CI scripts
  - Integration tests run against pinned versions

---

## Licensing Strategy

### Public Repositories

**License: Apache 2.0**

- **Edge Appliance** (`view-guard-edge`): Apache 2.0
- **Crypto Libraries** (`view-guard-crypto`): Apache 2.0
- **Protocol Definitions** (`view-guard-proto`): Apache 2.0

**Rationale:**
- Permissive license encourages enterprise adoption
- Good for security-sensitive code (allows commercial use)
- Clear patent grant
- Compatible with most other licenses

### Private Repositories

**License: Proprietary / All Rights Reserved**

- All private repositories remain proprietary
- No open source license required

### Alternative: AGPL-3.0 for Server Components (Optional)

If you later open a "community KVM agent" reference implementation:

- **AGPL-3.0** discourages SaaS competitors from cloning without contributing back
- Main SaaS remains closed source
- Edge code stays Apache 2.0 (runs on customer hardware)

---

## Marketing & Positioning

### Public Repositories

**Key Messages:**

1. **`view-guard-edge`**:
   - "Open-source Edge Appliance software - verify our privacy guarantees"
   - "Runs entirely on your hardware - audit the code yourself"
   - "Local AI inference, no cloud dependency"

2. **`view-guard-crypto`**:
   - "End-to-end encryption libraries - open source for transparency"
   - "Client-side decryption - we never see your keys"
   - "Audit our encryption implementation"

3. **`view-guard-proto`**:
   - "Open protocol definitions - build your own integrations"
   - "Standardized APIs for Edge, KVM VM, and SaaS communication"
   - "Community-driven protocol evolution"

### Private Components

**Key Messages:**

- "Hosted multi-tenant control plane and UI are proprietary SaaS"
- "Managed infrastructure, billing, and operations remain private"
- "Open source where it matters (privacy), proprietary where it matters (operations)"

---

## Important Design Decisions

### Crypto: Single Source of Truth

**No duplication**: All encryption primitives live in `view-guard-crypto` only.

- `view-guard-edge` imports `view-guard-crypto/go` as a Go module dependency
- Any audit of `view-guard-crypto` covers edge behavior as well
- One canonical implementation ensures consistency

### Protocol Definitions: Single Source of Truth

**No copying, no symlinks**: `view-guard-proto` is the only source of `.proto` files.

- `view-guard-edge` imports `view-guard-proto/go` as a Go module dependency
- `user-vm-api` imports `view-guard-proto/go` as a Go module dependency
- All repos depend on this single source - no manual sync needed

### SaaS APIs: Proto vs HTTP

**Clarification:**

- **`view-guard-proto/proto/kvm/`**: Contains only **KVM VM ↔ SaaS** gRPC contracts
  - These are cross-boundary contracts that need to be versioned
  - Public because they define the interface between KVM VM and SaaS

- **SaaS internal service-to-service protos**: Kept in private `saas-backend/` repo
  - Internal gRPC contracts between SaaS services
  - Not needed for third-party integrations

- **SaaS HTTP REST API**: Defined in private `saas-backend/` repo
  - OpenAPI/Swagger specifications
  - Used by the React frontend
  - Not gRPC-based (REST API)

**Note for Third-Party Integrators**: Third-party integrators primarily target the **KVM VM ↔ SaaS gRPC protocol** (`view-guard-proto/proto/kvm/`) or future **public HTTP APIs** exposed by the SaaS backend. They do not need internal SaaS service-to-service contracts, which remain private.

### Naming Consistency

**Repository names vs submodule directory names:**

- **Repository**: `view-guard-edge` (GitHub repo name)
- **Submodule directory in meta repo**: `edge-oss/` (directory name)
- **Consistent usage**: Always refer to the repository as `view-guard-edge` in documentation
- **Submodule reference**: Use `edge-oss/` when referring to the directory in the meta repo

**Examples:**
- Repository: Public component in `view-guard-meta` → Directory: `user-vm-api/`
- Repository: `view-guard-saas-backend` → Directory: `saas-backend/`
- Repository: `view-guard-edge` → Directory: `edge-oss/`

**Future naming**: Once product name is finalized (ViewGuard/CoreSight/etc.), update repository names early. Renames later are annoying for Go module paths.

## Directory Mapping Reference

### From Implementation Plan to Repositories

| Implementation Plan Location | Meta Repo Location | Public/Private | Notes |
|-----------------------------|-------------------|----------------|-------|
| `edge/orchestrator/` | `edge/orchestrator/` | Public | Imports crypto & proto as modules |
| `edge/ai-service/` | `edge/ai-service/` | Public | |
| `edge/shared/` | `edge/shared/` | Public | |
| `edge/proto/` | `proto/proto/edge/` | Public | Imported as Go module, not copied |
| `edge/encryption/` | `crypto/go/` | Public | Imported as Go module, not duplicated |
| `user-vm-api/wireguard-server/` | `view-guard-meta/user-vm-api/wireguard-server/` | Public | Direct in meta repo |
| `user-vm-api/event-cache/` | `view-guard-meta/user-vm-api/event-cache/` | Public | Direct in meta repo |
| `user-vm-api/stream-relay/` | `view-guard-meta/user-vm-api/stream-relay/` | Public | Direct in meta repo |
| `user-vm-api/filecoin-sync/` | `view-guard-meta/user-vm-api/filecoin-sync/` | Public | Direct in meta repo |
| `user-vm-api/ai-orchestrator/` | `view-guard-meta/user-vm-api/ai-orchestrator/` | Public | Direct in meta repo |
| `user-vm-api/event-analyzer/` | `view-guard-meta/user-vm-api/event-analyzer/` | Public | Direct in meta repo |
| `user-vm-api/proto/` | `proto/proto/kvm/` | Public | Imported as Go module |
| `saas/api/` | `view-guard-saas-backend/api/` | Private | HTTP REST API (OpenAPI), separate private repo |
| `saas/auth/` | `view-guard-saas-backend/auth/` | Private | Separate private repo |
| `saas/events/` | `view-guard-saas-backend/events/` | Private | Separate private repo |
| `saas/provisioning/` | `view-guard-saas-backend/provisioning/` | Private | Separate private repo |
| `saas/billing/` | `view-guard-saas-backend/billing/` | Private | Separate private repo |
| `saas/iso-generation/` | `view-guard-saas-backend/iso-generation/` | Private | Separate private repo |
| `saas/internal-proto/` | `view-guard-saas-backend/internal-proto/` | Private | Internal service protos, separate private repo |
| `saas-frontend/src/` | `view-guard-saas-frontend/src/` | Private | Separate private repo |
| `infra/terraform/` | `view-guard-infra/terraform/saas/` | Private | Production infra, separate private repo |
| `infra/terraform/` | `view-guard-infra/terraform/demo/` | Private | Demo/dev infra, separate private repo |
| `infra/kubernetes/` | `view-guard-infra/kubernetes/` | Private | Separate private repo |

---

## Next Steps

1. **Set up public components in meta repository**:
   - Create `edge/` directory with basic structure
   - Create `crypto/` directory with encryption libraries
   - Create `proto/` directory with proto definitions

2. **Create private repositories**:
   - `user-vm-api/` (public, in meta repo)
   - `view-guard-saas-backend` (SaaS Control Plane backend)
   - `view-guard-saas-frontend` (SaaS Control Plane frontend)
   - `view-guard-infra` (Infrastructure as Code)

3. **Set up public meta repository**:
   - Create public `view-guard-meta` repository (developer landing page)
   - Add public components directly in the repo (`edge/`, `crypto/`, `proto/`)
   - Add submodules pointing to private repos (optional, for internal developers)
   - Add public documentation (architecture, technical stack, threat model)
   - Add examples and reference deployments
   - Add bootstrap script
   - **Ensure README clearly explains "trust us / verify us" story**

4. **Set up internal meta repository** (optional, private):
   - Create private `view-guard-internal` repository (if needed)
   - Add internal-only documentation (roadmaps, ops, GTM materials)
   - Keep separate from public meta to maintain clean public-facing repo

5. **Add licenses**:
   - Apache 2.0 LICENSE file in the meta repo root
   - README files in each public component directory explaining the open source strategy

6. **Documentation**:
   - Public components: Installation, configuration, privacy guarantees (in component directories)
   - Private repos: Internal documentation, operational runbooks
   - Public meta repo: Project overview, architecture, "trust us / verify us" story
   - Internal meta repo (if created): Roadmaps, ops, GTM materials

7. **CI/CD setup**:
   - Unified CI/CD pipeline for all public components in the meta repo
   - Meta repo CI/CD that pulls private submodules at specific versions
   - Public CI/CD visible to community

---

## Benefits of This Structure

### For Customers
- ✅ **Transparency**: Can audit privacy-critical code
- ✅ **Trust**: Open source encryption and AI inference
- ✅ **Verification**: No "black box" on customer hardware
- ✅ **Clear Map**: Public meta repo shows what's verifiable vs SaaS glue
- ✅ **Self-Hosting**: Can understand how to self-host without SaaS

### For Business
- ✅ **IP Protection**: SaaS, billing, infrastructure remain proprietary
- ✅ **Competitive Moat**: Multi-tenant operations stay private
- ✅ **Flexibility**: Can iterate pricing and features without public scrutiny
- ✅ **Trust Story**: Public meta repo supports "trust us / verify us" narrative
- ✅ **Standards Potential**: Public protocols can become de facto standards

### For Community
- ✅ **Integration**: Open protocols enable third-party tooling
- ✅ **Contributions**: Community can improve edge software
- ✅ **Standards**: Protocols become open standards
- ✅ **Discovery**: Public meta repo makes it easy to find all components
- ✅ **Onboarding**: One landing page explains the entire ecosystem

## Why Public Meta Repo is Essential

The public meta repository is **not** where the commercial moat lives - it's where the **trust story** and **standards surface** live. These absolutely benefit from being public:

- **Trust Story**: Supports "trust us / verify us" narrative by showing what's verifiable vs SaaS glue
- **Developer Onboarding**: Single landing page explains the entire ecosystem and enables component discovery
- **Standards Potential**: Makes wire protocols publicly linkable, discoverable, and forkable
- **Marketing**: Demonstrates commitment to transparency and clear separation of concerns

---

## Summary of Key Design Principles

### 1. Public Meta Repo = Trust Story
- **Public meta repo** is the developer landing page and trust story surface
- Shows what's verifiable (edge, crypto, protocols) vs what's SaaS glue
- Makes protocols discoverable as potential standards
- **NOT** where commercial moat lives - that's in private repos

### 2. Single Source of Truth
- **Crypto**: All encryption primitives in `crypto/` only - no duplication
- **Protocols**: All `.proto` files in `proto/` only - no copying or symlinks
- **Dependencies**: Components import as Go modules from the same repo, ensuring consistency

### 3. Unified Development
- All public components developed in the same repository
- Easier cross-component changes and refactoring
- Unified CI/CD pipeline for all public components
- Single source of truth for versioning

### 4. Clear Boundaries
- **Public**: Privacy-critical code (edge, crypto, protocols) that runs on customer hardware
- **Private**: Commercial moat (SaaS, billing, infrastructure, Management Server)
- **Public Meta**: Coordination, documentation, trust story - NOT internal roadmaps or ops
- **Future flexibility**: Structure allows for community User VM API without reorganization

### 5. Versioning & Maintenance
- Semantic versioning for public components
- Tag-based releases on the meta repo
- Clear version tracking in `VERSIONS.md`

### 6. Developer Experience
- Bootstrap script for easy setup
- Clear documentation for each repository
- Consistent naming conventions
- Public meta repo as single entry point for ecosystem discovery

---

*This structure balances transparency (where it builds trust) with privacy (where it protects business value). The open source components support the "Private AI Guardian" narrative, while proprietary components protect commercial operations. The single-source-of-truth approach for crypto and protocols ensures consistency and auditability.*
