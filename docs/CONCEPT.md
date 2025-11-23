# The Private AI Guardian – SaaS Concept & Architecture (Comprehensive Detail)

> **Short version:**  
> This project is a SaaS platform that provisions a dedicated **KVM VM (Private Cloud Node)** per customer. The **Mini PC Edge Appliance** performs all **AI video processing and storage locally**, connects to the VM via a unique **WireGuard** tunnel, and **securely archives encrypted event clips to Filecoin**. The SaaS Control Plane manages global state, billing, and the unified UI, but **never handles plaintext raw video or customer decryption keys**.

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
- A **per-customer KVM VM** ("Private Cloud Node") that:
  - **Isolates** tenant traffic.
  - Terminates the WireGuard tunnel from the Mini PC.
  - Acts as the dedicated control / relay / archiving node.
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
- **On-demand remote clip streaming** over WireGuard, coordinated by the KVM VM.
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
                     |      Customer KVM VM          |
                     |      ("Private Cloud Node")   |
                     |                                |
                     | - WireGuard server endpoint    |
                     | - Per-tenant config & secrets  |
                     | - Event cache & telemetry      |
                     | - Filecoin sync / client role  |
                     +-------------+------------------+
                                   |
                           (WireGuard tunnel)
                                   |
                     +-------------v------------------+
                     |     Mini PC Edge Appliance     |
                     |          (On-Prem)             |
                     |                                |
                     | - Connects to KVM via WG       |
                     | - Ingests camera streams       |
                     | - Runs AI (local)              |
                     | - Stores clips & snapshots     |
                     | - Encrypts clips               |
                     | - Sends encrypted clips        |
                     |   to KVM for Filecoin archive  |
                     +--------------------------------+
```

---

## 5. Division of Responsibilities (Detailed)

This table formalizes the division of labor across the three conceptual layers, including security, quota enforcement, and hardware management. It is **conceptual only** and does not define concrete APIs or data formats.

| Responsibility Area | SaaS Control Plane (Multi-Tenant) | Customer KVM VM (Single-Tenant) | Mini PC Edge Appliance (Local/Edge) |
| :--- | :--- | :--- | :--- |
| **System Identity** | Manages user accounts, subscriptions, and billing. Knows which Edge and KVM belong to which tenant. | Acts as **WireGuard server** for its Edge devices. Holds per-tenant config & secrets. | Acts as **WireGuard client**. Authenticates *only* to its assigned KVM VM. |
| **Provisioning** | **Triggers KVM creation** & generates a unique **autoinstall ISO** image embedding KVM config and bootstrap tokens. | Hosts initial configuration/bootstrapping endpoint. Issues long-lived WireGuard config to the Mini PC at first connection. | Boots from ISO, runs **autoinstall**, configures OS + containers, connects to KVM VM automatically. |
| **AI Processing** | Manages entitlements to **AI model packs**. Distributes encrypted model artifacts conceptually. | May cache and propagate model artifacts and configuration. **Does no video AI processing itself.** | **Primary AI processor.** Decodes streams (Go orchestrator using iGPU/Intel QSV), runs inference (Python/OpenVINO), and generates event metadata. |
| **Video Data Storage** | **Does not store raw video** or full clips. Stores only metadata about events and high-level clip status. | May hold small, **transient encrypted clip buffers** during Filecoin uploads. Does not keep long-term raw video. | **Stores all raw video clips and snapshots** on local SSD, subject to local retention policies (e.g., 7–30 days). |
| **Clip Archiving (Filecoin)** | Displays Filecoin archive status (CID) and retention info in UI. Manages **global retention policy configuration**. | **Filecoin sync orchestrator.** Enforces archive **quota/retention limits** based on subscription tier. Uploads encrypted clips to provider and stores CIDs. | **Encrypts clips locally** (using user-derived or device-specific key). Pushes encrypted blobs + metadata to KVM VM for archiving. |
| **Archival Policy Enforcement** | **Defines** maximum storage quota (GB / retention days) based on subscription tier. | **Enforces** quota by tracking size and count of archived clips (via CIDs and stored metadata). Manages retention expiration. | Executes archival policy (e.g., "archive only Person events"), but **defers quota enforcement** to the KVM VM. |
| **Encryption / Keys** | Knows a hash/identifier of the user's secret/key. **Never stores the plaintext decryption key.** | Stores key hash/identifier as needed for mapping clips to keys. **Never stores the plaintext decryption key.** | **Derives the encryption key** from a customer-controlled secret and uses it for local clip encryption. |
| **Event Indexing** | Maintains **global multi-tenant event inventory** (timeline/dashboard, search). | Maintains **per-tenant event cache** with richer/raw metadata from Edge. Forwards summarized metadata to SaaS. | **Generates event metadata** for each AI detection (camera, time, category, severity, clip references). |
| **Remote Viewing** | Provides the **UI layer** and issues **time-bound request tokens** for stream authorization. | **Control and optional stream relay.** Validates token and orchestrates clip streaming from Edge; can relay encrypted stream back to client. | Reads clip from local SSD and streams it **on-demand** over the WireGuard tunnel to the KVM VM / client. |
| **Health Monitoring** | **Global health dashboard.** Raises automated alerts if KVM VM or Edge goes offline or reports risk telemetry. | **Detailed telemetry collector.** Receives heartbeats and metrics (CPU/GPU load, unsynced queue length) from Edge. | Sends detailed **heartbeat and telemetry** (resource usage, camera status, storage capacity, archival backlog) to KVM VM regularly. |
| **Updates & Maintenance** | Defines release channels and schedules for Edge and KVM software. | Applies per-tenant update policies and can instruct Edge to upgrade when safe. | Applies updates to local containers/OS based on instructions from KVM VM, ideally during low-usage windows. |
| **Hardware Driver Support** | N/A | Receives hardware / driver version telemetry from Edge. | Ensures **iGPU driver support** (e.g., Intel Quick Sync) is functional via the custom ISO installer on supported Mini PCs. |

---

## 6. Core Workflows (Detailed)

All workflows below are **conceptual**, describing behavior, not concrete protocols or API designs.

### 6.1 Customer Onboarding & Appliance Install

1. **Sign-up in SaaS UI**  
   User creates an account and selects a subscription plan (Base / Pro / etc.). SaaS creates a tenant record.

2. **KVM VM Provisioning**  
   SaaS provisions a dedicated KVM VM for the tenant and generates:
   - WireGuard server keys and configuration.
   - Initial bootstrap identifiers for the Edge.

3. **ISO Generation**  
   SaaS builds a **tenant-specific ISO image** for the Mini PC that contains:
   - OS image and Edge agent stack.
   - Bootstrap configuration to connect to that tenant's KVM VM via WireGuard.

4. **Download & Install (Mini PC)**  
   User downloads the ISO and installs it on their Mini PC (e.g., via USB). After installation, the Mini PC reboots as an **Edge Appliance**.

5. **First Connection (Edge → KVM VM)**  
   - Edge boots, brings up WireGuard, and connects to the KVM VM using embedded bootstrap credentials.
   - KVM VM validates and **promotes** the Edge to a fully registered device.
   - KVM VM issues stable credentials and long-lived WireGuard configuration.

6. **Camera Discovery & Configuration**  
   - Edge scans LAN for RTSP/ONVIF-compatible network cameras.
   - Edge detects USB cameras connected directly to the Mini PC (via V4L2).
   - SaaS UI displays discovered cameras (both network and USB).
   - User labels/configures cameras (e.g., "Front Door", "Parking Lot", schedules, zones).

Result: the tenant has a functioning **local AI appliance** connected to their **private cloud node**, visible and configurable through the SaaS UI.

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

### 6.4 Filecoin Archiving & Retrieval (Pro Concept)

Archiving is **event-based**, not continuous. Only selected event clips are archived.

#### 6.4.1 Archiving Flow

1. **Policy & Eligibility**  
   - Tenant's plan and settings define which events are eligible:
     - For example, all "Person" events during business hours.
     - Or only events manually marked "Important" in the UI.

2. **Encryption (Edge)**  
   - Edge encrypts the clip (and optionally snapshots) using:
     - A key derived from a user secret and/or device-specific secret.
   - Result: an encrypted blob ready for offsite storage.

3. **Policy & Quota Check (KVM VM)**  
   - Edge sends the encrypted blob plus metadata to the KVM VM.
   - KVM VM checks:
     - Subscription tier.
     - Current usage vs. quota (GB / #clips / retention).
   - If over quota, KVM VM can reject archiving for that event or request user action.

4. **Upload (KVM VM → Filecoin/IPFS)**  
   - KVM VM uploads the encrypted blob to a Filecoin/IPFS-backed provider.
   - KVM VM receives a **Content Identifier (CID)** and stores:
     - CID.
     - Clip metadata (size, timestamp, retention target).
     - A hash/identifier of the encryption metadata (not the key itself).

5. **Status Update (KVM VM → SaaS)**  
   - KVM VM notifies SaaS that event X is archived with CID Y.
   - SaaS updates the event record to show archive status and retention.

#### 6.4.2 Retrieval Flow

1. **User Requests Archived Clip**  
   - In SaaS UI, user opens an older event and selects "Download from archive" or equivalent.

2. **Archive Info (SaaS → Client)**  
   - SaaS provides the client with:
     - CID.
     - Encryption metadata hash/identifier.
   - Optionally, SaaS may provide information on which key slot to use.

3. **Download & Decrypt (Client)**  
   - Client fetches the encrypted blob directly from the Filecoin/IPFS provider.
   - Client uses the user-derived secret to reconstruct the decryption key and decrypt locally.
   - Clip is then viewable on the client, without SaaS or KVM VM ever seeing plaintext.

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

The public open-source components (Edge Appliance software, encryption libraries, and protocol definitions) are developed directly in this meta repository:

- **`edge/`** - Edge Appliance software (runs on customer hardware)
- **`crypto/`** - Encryption libraries (client-side encryption/decryption)
- **`proto/`** - Protocol definitions (communication contracts)

These components are fully auditable and open source (Apache 2.0), supporting the "trust us / verify us" privacy story. Private components (SaaS Control Plane, production KVM VM agent, infrastructure) remain in separate private repositories.
