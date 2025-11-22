# The Private AI Guardian

> **Privacy-first video security platform** that runs AI processing on your hardware, archives encrypted evidence to decentralized storage, and never sends raw video to a multi-tenant cloud.

## Trust Us / Verify Us

The Private AI Guardian is built on a **"trust us / verify us"** model:

- ✅ **Open source where it matters**: Edge Appliance software, encryption libraries, and wire protocols are fully open source and auditable
- ✅ **Runs on your hardware**: All video processing and primary storage happens on your Mini PC Edge Appliance
- ✅ **End-to-end encryption**: You control the decryption keys; we never see plaintext video
- ✅ **Transparent architecture**: Public documentation of what's verifiable vs. what's proprietary SaaS

**What you can verify:**
- `edge/` - Edge Appliance software (runs entirely on your hardware)
- `crypto/` - Encryption libraries (client-side encryption and decryption)
- `proto/` - Wire protocols (communication contracts between components)

**What remains proprietary:**
- Multi-tenant SaaS Control Plane (billing, provisioning, UI)
- Production KVM VM agent (Filecoin integration, quotas, operations)

This transparency enables you to **audit the privacy-critical code** while we protect our commercial operations.

## Quick Start

### For Developers

```bash
# Clone the meta repository
git clone git@github.com:vzahanych/view-guard-meta.git
cd view-guard-meta

# Bootstrap development environment
./scripts/bootstrap-dev.sh
```

The bootstrap script will:
- Verify public components are available (`edge/`, `crypto/`, `proto/`)
- Initialize private submodules (if you have access)
- Run basic sanity checks

### For Users

1. **Sign up** in the SaaS Control Plane
2. **Download** your tenant-specific ISO image
3. **Install** on a Mini PC (Intel N100 or equivalent)
4. **Connect** your RTSP/ONVIF cameras
5. **Start monitoring** - AI processing happens locally, events appear in the UI

## Architecture Overview

The platform uses a **three-layer architecture**:

1. **Edge Appliance (Mini PC)** - Local video processing, AI inference, clip storage
2. **KVM VM (Private Cloud Node)** - Per-tenant relay, event cache, Filecoin sync
3. **SaaS Control Plane** - Multi-tenant UI, billing, provisioning

```
┌─────────────────────────────────────┐
│   SaaS Control Plane (Multi-Tenant) │
│   - Web UI, Billing, Provisioning  │
└──────────────┬──────────────────────┘
               │
               │ HTTPS/API (metadata only)
               │
┌──────────────▼──────────────────────┐
│   Customer KVM VM (Single-Tenant)   │
│   - WireGuard Server, Event Cache   │
│   - Filecoin Sync, Stream Relay     │
└──────────────┬──────────────────────┘
               │
               │ WireGuard Tunnel
               │
┌──────────────▼──────────────────────┐
│   Edge Appliance (On-Premise)       │
│   - Video Processing, AI Inference │
│   - Local Storage, Encryption      │
└──────────────┬──────────────────────┘
               │
               │ RTSP/ONVIF
               │
         ┌─────┴─────┐
         │  Cameras  │
         └───────────┘
```

## Repository Structure

### Public Components (in this repository)

All public open-source components are developed directly in this repository:

- **`edge/`** - Edge Appliance software
  - Camera discovery and RTSP/ONVIF client
  - Video processing with FFmpeg
  - AI inference service (OpenVINO/YOLO)
  - Local clip storage and retention
  - WireGuard client
  - License: Apache 2.0

- **`crypto/`** - Encryption libraries
  - AES-256-GCM encryption
  - Argon2id key derivation
  - Browser-based decryption
  - License: Apache 2.0

- **`proto/`** - Protocol definitions
  - gRPC proto definitions for Edge ↔ KVM VM
  - gRPC proto definitions for KVM VM ↔ SaaS
  - Generated language stubs (Go, TypeScript, Python)
  - License: Apache 2.0

### Private Components (git submodules)

Private components are referenced as git submodules (require access):

- **`kvm-agent/`** - Production KVM VM agent
- **`saas-backend/`** - SaaS Control Plane backend
- **`saas-frontend/`** - SaaS Control Plane frontend
- **`infra/`** - Infrastructure as Code

## Privacy Guarantees

| Data Type | Location | Encryption | Access Control |
|-----------|----------|------------|----------------|
| Raw Video Streams | Edge Appliance (local) | N/A (local only) | Local network isolation |
| Video Clips | Edge Appliance (local) | N/A (local only) | Local filesystem |
| Event Metadata | KVM VM, SaaS | In-transit (TLS/mTLS) | Tenant isolation |
| Encrypted Archive Clips | Filecoin/IPFS | End-to-end (user key) | CID-based access |
| Decryption Keys | User device only | N/A (never transmitted) | User custody |

**Core Privacy Principles:**
- Raw video **never leaves** customer premises unencrypted
- Decryption keys are **derived locally** and **never transmitted**
- SaaS Control Plane stores **only metadata** (timestamps, labels, camera IDs)
- Encrypted archives are **end-to-end encrypted** - we cannot decrypt them

## Documentation

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** - Detailed architecture diagrams and data flows
- **[docs/TECHNICAL_STACK.md](docs/TECHNICAL_STACK.md)** - Technology choices and rationale
- **[docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md)** - Phase-by-phase implementation plan
- **[docs/PROJECT_STRUCTURE.md](docs/PROJECT_STRUCTURE.md)** - Repository organization and structure

## Self-Hosting

You can self-host the Edge Appliance and use your own KVM VM or storage backend. See the public components for:

- Edge Appliance installation and configuration (`edge/`)
- Protocol specifications for building custom integrations (`proto/`)
- Encryption library usage for client-side decryption (`crypto/`)

The SaaS Control Plane remains proprietary, but the core privacy guarantees are verifiable through the open-source components.

## Contributing

We welcome contributions to the public components:

- Edge Appliance improvements (`edge/`)
- Encryption library enhancements (`crypto/`)
- Protocol specification refinements (`proto/`)
- Documentation improvements

See individual component directories for contribution guidelines.

## License

- **Public components**: Apache 2.0
- **Private components**: Proprietary

## Support

- **Documentation**: See `docs/` directory
- **Issues**: Open issues in this repository
- **Security**: Report security issues via security@yourorg.com

---

**The Private AI Guardian** - Privacy-first video security that you can verify.

