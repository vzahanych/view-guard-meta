# The Private AI Guardian – SaaS Concept & Architecture (Comprehensive Detail)

> **Short version:**  
> This project is a SaaS platform that provisions a dedicated **User VM (Private Cloud Node)** per customer. Users download a **tenant-specific installation package** (ISO/EXE) containing embedded certificates, keys, and User VM FQDN. The **Mini PC Edge Appliance** automatically establishes a **WireGuard tunnel** to the User VM upon installation, performs all **AI video processing and storage locally**, and **securely archives encrypted event clips to remote storage** (MinIO/S3 for PoC, Filecoin post-PoC). The SaaS Control Plane manages global state, billing, and the unified UI, but **never handles plaintext raw video or customer decryption keys**.
>
> **PoC Note**: For the Proof of Concept, **no SaaS components are needed**. The PoC focuses on Edge Appliance ↔ User VM API communication, with User VM API running as a Docker Compose service and using MinIO (S3-compatible) for remote storage instead of Filecoin. Post-PoC, an S3-Filecoin bridge will be developed to migrate from MinIO to Filecoin.

---

## Table of Contents

1. [Vision & Problem Statement](#1-vision--problem-statement)  
2. [Target Users & Positioning](#2-target-users--positioning)  
3. [High-Level Product Concept](#3-high-level-product-concept)  
4. [Three-Layer Architecture](#4-three-layer-architecture)  
5. [Division of Responsibilities (Detailed)](#5-division-of-responsibilities-detailed)  
6. [Core Workflows (Detailed)](#6-core-workflows-detailed)  
7. [Data, Security, and Privacy Model](#7-data-security-and-privacy-model)  
8. [Business Model & Pricing Concept](#8-business-model--pricing-concept)  
9. [Next Steps for Implementation Planning](#9-next-steps-for-implementation-planning)

---

## 1. Vision & Problem Statement

### Vision

Provide **Ring/Arlo-level convenience** and **"cloud-like" reliability** while keeping:

- **All video processing and primary storage on the customer's own hardware**.
- **Long-term, verifiable evidence** in decentralized storage (Filecoin).
- **Hard privacy guarantees** and **predictable, low monthly pricing**.

### Problem

Customers currently must choose between:

- **Vendor cameras (Cloud Lock-in / Privacy Issues)**  
  - Raw video streams go to the vendor's cloud.  
  - Opaque privacy, per-GB or per-camera pricing, long-term lock-in.

- **DIY NVRs (High Ops Burden / No Managed Access)**  
  - Full local control and privacy.  
  - But no managed remote access, no easy mobile UX, no managed backups, and a lot of operational burden.

Our platform aims to combine the **control of DIY** with the **experience of managed SaaS**, plus **decentralized archiving**.

---

## 2. Target Users & Positioning

### Primary Target

**Privacy-focused prosumers & power users** who:

- Already own IP cameras (RTSP/ONVIF) or USB cameras.
- Are willing to run a Mini PC / NUC at home.
- Want remote access and intelligence, but **not** cloud lock-in.

### Secondary Target

**Small businesses (SMBs)** that:

- Have 4–10 cameras (shops, offices, small warehouses, parking lots).
- Need **reliable, long-term, offsite evidence** for legal/insurance purposes.
- Don't want to maintain their own IT or video servers.

### Positioning Statement

> A **Managed Edge AI Security Platform** that gives each customer their own **private cloud node** and a **local AI appliance**, featuring **Filecoin-backed evidence archiving** – **without** sending raw video to a multi-tenant cloud.

---

## 3. High-Level Product Concept

### Core Product

- A **SaaS Control Plane** for onboarding, billing, event inventory, and the unified UI.
- A **Management Server** (private infrastructure) that:
  - Controls and manages per-customer User Servers.
  - Communicates with the SaaS Control Plane.
  - Handles VM provisioning, lifecycle management, and orchestration.
- A **per-customer User Server** (open source, runs on customer's dedicated VM) that:
  - **Isolates** tenant traffic on the customer's VM.
  - Terminates the WireGuard tunnel from the Mini PC.
  - Hosts the **AI model catalog** for that tenant (base + custom variants).
  - Retrains models using user-labeled screenshots and pushes approved builds down to the Edge.
  - Performs **secondary event analysis** on incoming snapshots/clips before escalating alarms.
  - Acts as the dedicated control / relay / archiving node with remote storage integrations.
- A **Mini PC Edge Appliance** that:
  - Installs from a **customer-specific ISO image**.
  - Handles all **local AI processing and video storage**.
  - Connects **only** to its assigned KVM VM using WireGuard.
  - Encrypts event clips and syncs them to Filecoin via the KVM VM.

### Key Features (Conceptual)

- **Zero-touch provisioning** via a **customer-specific, auto-installing ISO** for the Mini PC.
- **Local AI detection** (people, vehicles, custom models) on the Mini PC.
- **Local clip + snapshot storage** on the Mini PC's SSD.
- **Per-customer isolation** via the dedicated KVM VM ("Private Cloud Node").
- **On-demand remote clip streaming** over WireGuard, coordinated by the User Server.
- **Filecoin-based archival** of selected event clips, encrypted end-to-end.

---

## 4. Three-Layer Architecture

We use a **three-layer "Managed Edge–KVM" architecture** for isolation, resilience, and clear responsibility boundaries.

1. **SaaS Control Plane (Multi-Tenant)** – the high-level management layer.
2. **Customer KVM VM (Single-Tenant)** – the dedicated "Private Cloud Node".
3. **Mini PC Edge Appliance (Local / On-Premise)** – the execution and storage layer.

Conceptual diagram (not implementation-specific):

```text
+------------------------ SaaS Control Plane (Multi-Tenant) ------------------------+
| - User accounts, auth, billing, subscriptions                                    |
| - UI: event timeline, configuration, health dashboard                            |
| - Provisioning: per-customer KVM VM, ISO generation                              |
| - Global event inventory & search                                                |
+----------------------------------+-----------------------------------------------+
                                   |
                        (SaaS provisions per-customer VM)
                                   |
                     +-------------v------------------+
                     |   Management Server (Private)  |
                     |                                |
                     | - Controls User Servers        |
                     | - Talks to SaaS               |
                     | - VM provisioning & lifecycle  |
                     +-------------+------------------+
                                   |
                          (Control Channel)
                                   |
                     +-------------v------------------+
                     |   User Server (Public/Open)    |
                     |   (Customer's VM)              |
                     |                                |
                     | - WireGuard server endpoint    |
                     | - Per-tenant config & secrets  |
                     | - Event cache & telemetry      |
                     | - Filecoin sync / client role  |
                     | - AI model catalog             |
                     +-------------+------------------+
                                   |
                           (WireGuard tunnel)
                                   |
                     +-------------v------------------+
                     |     Mini PC Edge Appliance     |
                     |          (On-Prem)             |
                     |                                |
                     | - Connects to VM via libp2p+WG │
                     |   (handles NAT automatically)  │
                     | - Ingests camera streams       |
                     | - Runs AI (local)              |
                     | - Stores clips & snapshots     |
                     | - Encrypts clips               |
                     | - Sends encrypted clips        |
                     |   to User Server for Filecoin  |
                     |   archive                      |
                     +--------------------------------+
```

---

## 5. Division of Responsibilities (Detailed)

This table formalizes the division of labor across the four conceptual layers, including security, quota enforcement, and hardware management. It is **conceptual only** and does not define concrete APIs or data formats.

| Responsibility Area | SaaS Control Plane (Multi-Tenant) | Management Server (Private) | User Server (Single-Tenant, Open Source) | Mini PC Edge Appliance (Local/Edge) |
| :--- | :--- | :--- | :--- |
| **System Identity** | Manages user accounts, subscriptions, and billing. Knows which Edge and KVM belong to which tenant. | N/A for connectivity; only coordinates User Server lifecycle. Does not handle WireGuard. | Acts as **WireGuard server endpoint** for its Edge Appliances. Authenticates Edge devices and manages WireGuard tunnel configuration. |
| **Provisioning** | **Triggers KVM creation** & generates a unique **autoinstall ISO** image embedding KVM config and bootstrap tokens. | Hosts initial configuration/bootstrapping endpoint. Issues long-lived WireGuard config to the Mini PC at first connection. | Boots from ISO, runs **autoinstall**, configures OS + containers, connects to KVM VM automatically. |
| **AI Processing** | Manages entitlements to **AI model packs**. Tracks which tenants can access premium detectors. | Coordinates model distribution to User Servers. Manages model versioning across User Servers. | **Owns the tenant's model catalog**: retrains models with user-labeled screenshots, versions them, performs secondary inference on incoming snapshots/clips, and pushes approved models down to Edge over WireGuard. | **Primary real-time AI processor.** Decodes streams (Go orchestrator using iGPU/Intel QSV), runs on-box inference (Python/OpenVINO), and generates initial event metadata. |
| **Video Data Storage** | **Does not store raw video** or full clips. Stores only metadata about events and high-level clip status. | N/A (does not handle video data) | May hold small, **transient encrypted clip buffers** during Filecoin uploads. Does not keep long-term raw video. | **Stores all raw video clips and snapshots** on local SSD, subject to local retention policies (e.g., 7–30 days). |
| **Clip Archiving (Filecoin)** | Displays Filecoin archive status (CID) and retention info in UI. Manages **global retention policy configuration**. | Monitors User Server archive operations. Aggregates archive status for SaaS. | **Filecoin/remote-storage orchestrator.** Enforces archive **quota/retention limits**, stores long-term copies of events/clips, and keeps metadata index for SaaS queries. | **Encrypts clips locally** (using user-derived or device-specific key). Pushes encrypted blobs + metadata to User Server for archiving. |
| **Archival Policy Enforcement** | **Defines** maximum storage quota (GB / retention days) based on subscription tier. | **Enforces** quota by tracking size and count of archived clips (via CIDs and stored metadata). Manages retention expiration. | Executes archival policy (e.g., "archive only Person events"), but **defers quota enforcement** to the KVM VM. |
| **Encryption / Keys** | Stores only key *identifiers/hashes* for mapping; never plaintext keys. | May see key identifiers for quota and mapping; never plaintext keys. | Receives *already encrypted* blobs plus key identifiers; does not hold plaintext keys or secrets. | **Derives and holds encryption keys** from a customer secret, performs all encryption locally, never transmits keys. |
| **Event Indexing** | Maintains **global multi-tenant event inventory** (timeline/dashboard, search). | Aggregates event data from User Servers. Forwards to SaaS. | Maintains **per-tenant event cache**, runs secondary analysis (e.g., anomaly confirmation) on snapshots/clips, and forwards verdicts + summarized metadata to Management Server/SaaS. | **Generates raw event metadata** for each AI detection (camera, time, category, severity, clip references). |
| **Remote Viewing** | Provides the **UI layer** and issues **time-bound request tokens** for stream authorization. | Routes stream requests to appropriate User Server. Coordinates stream relay. | **Control and optional stream relay.** Validates token and orchestrates clip streaming from Edge; can relay encrypted stream back to client. | Reads clip from local SSD and streams it **on-demand** over the WireGuard tunnel (established via libp2p) to the User Server / client. |
| **Health Monitoring** | **Global health dashboard.** Raises automated alerts if User Server or Edge goes offline or reports risk telemetry. | **Aggregates health data** from User Servers. Monitors User Server VM health and resource usage. | **Detailed telemetry collector.** Receives heartbeats and metrics (CPU/GPU load, unsynced queue length) from Edge. Forwards aggregated telemetry to Management Server. | Sends detailed **heartbeat and telemetry** (resource usage, camera status, storage capacity, archival backlog) to User Server regularly. |
| **Updates & Maintenance** | Defines release channels and schedules for Edge and User Server software. | Coordinates User Server updates. Manages update rollout across User Servers. | Applies per-tenant update policies and can instruct Edge to upgrade when safe. Receives update instructions from Management Server. | Applies updates to local containers/OS based on instructions from User Server, ideally during low-usage windows. |
| **Hardware Driver Support** | N/A | Receives hardware / driver version telemetry from Edge. | Ensures **iGPU driver support** (e.g., Intel Quick Sync) is functional via the custom ISO installer on supported Mini PCs. |

---

## 6. Core Workflows (Detailed)

All workflows below are **conceptual**, describing behavior, not concrete protocols or API designs.

### 6.1 Customer Onboarding & Appliance Install

1. **Sign-up in SaaS UI**  
   User creates an account and selects a subscription plan (Base / Pro / etc.). SaaS creates a tenant record.

2. **User VM Provisioning**  
   SaaS provisions a dedicated User VM (Private Cloud Node) for the tenant via the Management Server. The Management Server:
   - Creates and configures the User VM instance.
   - Generates WireGuard server keys and configuration on the User VM.
   - Assigns a unique FQDN (Fully Qualified Domain Name) for the User VM endpoint.
   - Generates client certificates and keys for the Edge Appliance.
   - Stores bootstrap configuration and credentials securely.

3. **Installation Package Generation**  
   SaaS builds a **tenant-specific installation package** (ISO image, EXE installer, or other format - format to be determined) that contains:
   - OS image and Edge agent stack (for ISO) or Edge application installer (for EXE).
   - **Embedded certificates and keys** for authenticating to the User VM.
   - **FQDN of the User VM** endpoint (e.g., `user-abc123.example.com`).
   - Bootstrap configuration to establish WireGuard tunnel to the assigned User VM.
   - All necessary configuration to connect to that tenant's User VM via WireGuard.

4. **Download & Install (Mini PC)**  
   User downloads the installation package from the SaaS UI and installs it on their Mini PC:
   - **For ISO**: User writes ISO to USB drive, boots Mini PC from USB, and runs automated installation.
   - **For EXE/Installer**: User runs the installer on their Mini PC (Windows/Linux).
   - After installation, the Mini PC reboots (if applicable) and runs as an **Edge Appliance**.

5. **Automatic WireGuard Tunnel Establishment (Edge → User VM)**  
   Upon first boot/startup, the Edge Go application automatically:
   - Reads the embedded certificates, keys, and FQDN from the installation package.
   - Configures WireGuard client with the provided credentials.
   - Establishes a WireGuard tunnel to the User VM using the provided FQDN.
   - Authenticates using the embedded certificates and keys.
   - The User VM validates the Edge Appliance credentials and **promotes** it to a fully registered device.
   - The User VM issues stable, long-lived WireGuard configuration (if needed for rotation).

6. **Camera Discovery & Configuration**  
   Once the WireGuard tunnel is established:
   - Edge scans LAN for RTSP/ONVIF-compatible network cameras.
   - Edge detects USB cameras connected directly to the Mini PC (via V4L2).
   - Edge reports discovered cameras to the User VM, which forwards metadata to SaaS.
   - SaaS UI displays discovered cameras (both network and USB).
   - User labels/configures cameras (e.g., "Front Door", "Parking Lot", schedules, zones) via SaaS UI.
   - Configuration is pushed from SaaS → User VM → Edge Appliance.

**Result**: The tenant has a functioning **local AI appliance** connected to their **private cloud node** (User VM) via a secure WireGuard tunnel, visible and configurable through the SaaS UI.

**Key Security Properties**:
- Each Edge Appliance is cryptographically bound to its specific User VM via embedded certificates.
- The FQDN ensures Edge Appliances connect to the correct User VM endpoint.
- WireGuard tunnel provides encrypted, authenticated communication.
- No manual configuration required on the Edge Appliance - everything is embedded in the installation package.

---

### 6.2 Normal Operation: Detection → Event → Alert

1. **Continuous Video Ingest (Edge)**  
   - Edge connects to network cameras via RTSP/ONVIF feeds.
   - Edge accesses USB cameras directly via device paths (e.g., `/dev/video0`).
   - Video is decoded locally; AI is run at configured intervals (e.g., every N frames).

2. **AI Detection (Edge)**  
   - When AI detects a relevant event (e.g., person, vehicle, custom label):
     - Edge marks a time window (e.g., 2 seconds before and 5 seconds after).
     - Edge records a short **event clip** and snapshots to local SSD.

3. **Event Metadata Generation (Edge)**  
   - Edge creates a conceptual **event record** including:
     - Tenant, Edge, and camera identity.
     - Timestamps.
     - Detection category and severity.
     - Local identifiers for clip and snapshots.
   - No raw video is included.

4. **Event Forwarding (Edge → KVM VM → SaaS)**  
   - Edge sends event metadata to the KVM VM over WireGuard.
   - KVM VM may enrich or cache the event.
   - KVM VM forwards summarized metadata to the SaaS Control Plane.

5. **Timeline & Alert (SaaS)**  
   - SaaS stores event metadata in the global event inventory.
   - SaaS triggers push/email notifications based on user preferences.
   - Event appears in the SaaS UI timeline.

At this stage, **only metadata** (and optionally small thumbnails) have left the premises. The full clip remains on the Edge.

---

### 6.3 Remote Clip Streaming (On-Demand)

Triggered when a user clicks **"View Clip"** in the SaaS UI or mobile app.

1. **User Request (Client → SaaS)**  
   - Client requests playback for a specific event.
   - SaaS validates:
     - User authentication / authorization.
     - Subscription plan (e.g., Pro).
     - Device and KVM VM online status.

2. **Token & Command (SaaS → KVM VM)**  
   - SaaS issues a **short-lived, time-bound token** associated with that event and user.
   - SaaS sends a conceptual "play clip for event X using token T" command to the KVM VM.

3. **Stream Orchestration (KVM VM → Edge)**  
   - KVM VM relays the request and token to the Edge over WireGuard.
   - Edge validates token and locates the associated clip on local SSD.

4. **Streaming (Edge → KVM VM → Client)**  
   - Edge streams the clip over the WireGuard tunnel.
   - KVM VM either:
     - Relays the stream to the client, or
     - Acts purely as the control plane while stream flows end-to-end through the tunnel.

5. **Playback (Client)**  
   - Client plays the clip as if from a cloud video service, but the data is actually coming from the user's own Mini PC, over an encrypted tunnel.

No permanent copy of the full clip is stored in the multi-tenant SaaS; streaming is transient.

---

### 6.4 Remote Storage Archiving & Retrieval (MinIO/S3 for PoC, Filecoin post-PoC)

Archiving is **event-based**, not continuous. Only selected event clips are archived.

**PoC Note**: For PoC, we use **MinIO** (S3-compatible) with **per-camera buckets**. User VM API uses **AWS Go SDK v2** to manage storage. Post-PoC, an S3-Filecoin bridge will migrate data to Filecoin.

#### 6.4.1 Archiving Flow

1. **Policy & Eligibility**  
   - Tenant's plan and settings define which events are eligible:
     - For example, all "Person" events during business hours.
     - Or only events manually marked "Important" in the UI.

2. **Encryption (Edge)**  
   - Edge encrypts the clip (and optionally snapshots) using:
     - A key derived from a user secret and/or device-specific secret.
   - Result: an encrypted blob ready for offsite storage.

3. **Policy & Quota Check (User VM API)**  
   - Edge sends the encrypted blob plus metadata to the User VM API.
   - User VM API checks:
     - Per-camera quota (GB per camera bucket).
     - Current usage vs. quota for that camera's bucket.
   - If over quota, User VM API can reject archiving for that event or request user action.

4. **Upload (User VM API → MinIO/S3 for PoC)**  
   - User VM API uses **AWS Go SDK v2** to upload to MinIO.
   - **Per-camera bucket organization**: Each camera has its own bucket (`camera-{camera_id}`).
   - Upload structure:
     - Clips: `events/{event_id}/clip.mp4` in camera bucket
     - Snapshots: `events/{event_id}/snapshot.jpg` in camera bucket
     - Metadata: `events/{event_id}/metadata.json` in camera bucket
   - User VM API stores object keys in SQLite (replacing CID storage for PoC):
     - Object key, bucket name, size, timestamp, retention target.
     - A hash/identifier of the encryption metadata (not the key itself).

5. **Status Update (User VM API)**  
   - User VM API tracks archive status locally in SQLite (no SaaS in PoC).
   - Post-PoC: User VM API notifies SaaS that event X is archived with object key Y.
   - Post-PoC: SaaS updates the event record to show archive status and retention.

#### 6.4.2 Retrieval Flow

1. **User Requests Archived Clip**  
   - In Edge Web UI (PoC) or SaaS UI (post-PoC), user opens an older event and selects "Download from archive" or equivalent.

2. **Archive Info (User VM API → Client)**  
   - User VM API provides the client with:
     - MinIO object key and bucket name (PoC) or CID (post-PoC).
     - Encryption metadata hash/identifier.
   - Optionally, User VM API may provide information on which key slot to use.

3. **Download & Decrypt (Client)**  
   - **PoC**: Client fetches the encrypted blob from MinIO using AWS SDK (via User VM API or directly).
   - **Post-PoC**: Client fetches the encrypted blob directly from the Filecoin/IPFS provider.
   - Client uses the user-derived secret to reconstruct the decryption key and decrypt locally.
   - Clip is then viewable on the client, without SaaS or User VM API ever seeing plaintext.

---

### 6.5 Health Monitoring & Maintenance

1. **Edge Telemetry**  
   - Edge periodically sends:
     - CPU/GPU utilization.
     - Disk usage and remaining space.
     - Per-camera status (online/offline).
     - Length of unsynced archival queue.

2. **KVM VM Telemetry**  
   - KVM VM receives Edge telemetry and:
     - Performs per-tenant health checks.
     - Aggregates or compresses data for the SaaS Control Plane.

3. **SaaS Health Dashboard**  
   - SaaS displays high-level status to tenant (and internal support):
     - "Mini PC offline for 20 minutes."
     - "Disk nearing capacity."
     - "Archival backlog high."
   - SaaS may send proactive warnings/notifications.

4. **Updates**  
   - SaaS defines available versions/releases for Edge and KVM components.
   - KVM VM coordinates with Edge to find a safe update window.
   - Edge pulls updates and applies them with minimal user intervention.

---

## 7. Data, Security, and Privacy Model

| Data Type | Leaves Edge? | Destination | Security / Privacy Model |
| :--- | :--- | :--- | :--- |
| **Raw Video Streams** | **NO** | Stays on local Mini PC SSD. | 100% Zero-Cloud Storage Guarantee for continuous raw streams and main recordings. |
| **Event Metadata** | **YES** | KVM VM and SaaS Control Plane. | Sent over a secure channel (WireGuard from Edge→KVM, plus secure transport KVM→SaaS). Small JSON-like packets (tenant, device, timestamps, AI labels, etc.). **No raw frames; no biometric templates (e.g., face embeddings) stored in the multi-tenant SaaS.** |
| **Filecoin Archive (Encrypted Clips)** | **YES** | Filecoin/IPFS via KVM VM relay. | **Encrypted end-to-end.** Edge encrypts using a key derived from user secret; client decrypts. SaaS and KVM VM only see CID + key identifiers, never plaintext video or keys. |
| **Encryption Keys** | **NO** (plaintext) | Derived on Edge and Client only. | The customer is the **sole custodian** of the secret needed to derive decryption keys. SaaS and KVM VM may store hashes/identifiers, but never plaintext keys. |
| **Telemetry** | **YES** | KVM VM and SaaS Control Plane. | Sent over secure channels. Contains resource metrics and health indicators, not video content. Used for automated health monitoring and support. |
| **Isolation** | N/A | N/A | All Edge communication passes through a dedicated, cryptographically secure **WireGuard tunnel** terminating at the tenant's single-tenant KVM VM, providing strong tenant isolation. |

---

## 8. Business Model & Pricing Concept

> These are conceptual pricing targets and levers, not finalized numbers.

### 8.1 Subscription Justification

The recurring subscription covers:

- **Managed Isolation**  
  - Cost of running a **dedicated KVM VM** per tenant (compute, storage, networking).

- **Zero-Maintenance Security & Connectivity**  
  - Managed WireGuard, automatic patching, firmware/OS updates, and secure remote access.

- **Verified Archiving**  
  - Filecoin/IPFS archival costs and orchestration logic (upload, retention, retrieval).

- **Support & Monitoring**  
  - Health dashboards, proactive alerts, and human support when needed.

This differentiates clearly from "one-time license NVR software" by emphasizing the ongoing service, not just software bits.

### 8.2 Base Tier (Example: $5/month)

Conceptually:

- Up to ~6 cameras.
- Local AI detection and local clip storage on Mini PC.
- Managed WireGuard connectivity via the tenant's KVM VM.
- Event metadata timeline & push notifications.
- **Limited or no** Filecoin archival quota (e.g., only a small GB or days limit).
- Community or standard support.

**Goal:**  
High-margin, low-support entry product that introduces users to the platform and its UX.

### 8.3 Pro Tier (Example: $10/month)

Conceptually:

- Higher camera and event limits (e.g., up to 10 cameras).
- **On-demand remote clip streaming** for recent events (configurable window).
- **Filecoin archival** of selected events with a simple, generous quota (e.g., 10 GB of archived clips with 90 days retention).
- Access to **advanced proprietary AI packs** (e.g., "Package Detection", "Animal Monitoring", "Parking Violations").
- Priority support and faster response SLAs.

**Goal:**  
Capture users who value decentralized evidence storage, stronger guarantees, and more advanced AI.

### 8.4 Future Business / Enterprise Tier (Concept)

Potential future tier, for SMBs and multi-site deployments:

- Larger archival limits (more GB, longer retention, e.g., 1 year).
- Multiple sites and multiple Edge devices per tenant.
- SLA-backed uptime, audit logs, user roles/permissions.
- Possibly dedicated physical hardware in the data center.

---

## 9. Next Steps for Implementation Planning

This document is a **conceptual foundation**, not a technical specification. Implementation-planning models should take this and produce more detailed design/roadmap documents.

Recommended next steps:

1. **Define Data Contracts (Concept → Design)**  
   - Specify concrete event and telemetry data models (e.g., JSON/Protobuf schemas).  
   - Define APIs between:
     - Edge ↔ KVM VM  
     - KVM VM ↔ SaaS Control Plane

2. **Design Key Management & Security Flows**  
   - Plan the lifecycle of encryption keys and user secrets:
     - How keys are derived, rotated, and invalidated.
     - How recovery works if user loses a device or secret.
   - Decide if any external KMS is needed (and where).

3. **Detail Filecoin Integration**  
   - Choose specific Filecoin/IPFS provider(s).  
   - Design the upload and retrieval pipeline, including:
     - Handling of CIDs and associated metadata.
     - Retention and deletion policies.
     - Handling provider outages.

4. **Detail Provisioning & ISO Pipeline**  
   - KVM VM lifecycle: creation, scaling, deletion.  
   - ISO builder:
     - How template images are maintained.
     - How tenant-specific config is injected.
     - How downloads are authenticated and rate-limited.

5. **Plan Hardware & Driver Support**  
   - Choose an initial list of officially supported Mini PCs (e.g., Intel N100/N300-class).  
   - Ensure the custom ISO consistently delivers:
     - Proper iGPU drivers.
     - Stable OpenVINO / Quick Sync support.

6. **Operational & Cost Model**  
   - Estimate KVM density (tenants per physical host).  
   - Model bandwidth and storage usage under realistic assumptions.  
   - Define monitoring/alerting stack and on-call/support responsibilities.

7. **UX and Product Story**  
   - Refine how to explain:
     - "Private cloud node" (KVM VM) in simple terms.
     - "Local AI" vs. traditional cloud cameras.
     - Filecoin-based evidence archive as a concrete benefit (e.g., "insurance-grade, tamper-resistant proofs").

This document should live in the `docs/` directory as the **single source of truth for the concept**, from which all detailed design and implementation plans can be derived.

---

## Repository Structure Note

The public open-source components (Edge Appliance software, User Server, encryption libraries, and protocol definitions) are developed directly in this meta repository:

- **`edge/`** - Edge Appliance software (runs on customer hardware)
- **`user-vm-api/`** - User Server (runs on per-tenant VMs, user's private cloud node)
- **`crypto/`** - Encryption libraries (client-side encryption/decryption)
- **`proto/`** - Protocol definitions (communication contracts)

These components are fully auditable and open source (Apache 2.0), supporting the "trust us / verify us" privacy story. The User Server is open source because it handles user data and AI models on their dedicated VM - only secrets (WireGuard keys, encryption key identifiers) are kept in memory at runtime. Private components (SaaS Control Plane, Management Server, infrastructure automation) remain in separate private repositories.
