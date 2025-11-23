# Architecture Diagrams

This document provides detailed architecture diagrams for The Private AI Guardian platform, based on the concept outlined in README.md.

## How to Read This Document

This document explains how the SaaS control plane, per-tenant KVM VMs (Private Cloud Nodes), and on-premise Edge Appliances work together to provide AI-powered video security without ever centralizing raw video or encryption keys. The architecture is organized into three distinct layers, each with clear responsibilities and privacy boundaries. All data flows preserve the core guarantee that raw video never leaves customer premises unencrypted.

## Table of Contents

1. [High-Level System Architecture](#1-high-level-system-architecture)
2. [Three-Layer Architecture Detail](#2-three-layer-architecture-detail)
3. [Component Interaction Diagram](#3-component-interaction-diagram)
4. [Data Flow Diagrams](#4-data-flow-diagrams)
5. [Security & Privacy Boundaries](#5-security--privacy-boundaries)
6. [Network Topology](#6-network-topology)
7. [Deployment Architecture](#7-deployment-architecture)

---

## User Journeys

This section illustrates how the architecture supports real-world user experiences, connecting the technical diagrams to practical use cases.

### Journey 1: Motion Detection and Alert

* Home user's camera detects motion → Edge Appliance processes video locally with AI → Event metadata (no video) is sent to KVM VM → KVM VM forwards summary to SaaS → User receives push notification on mobile app → User opens app and sees event in timeline with thumbnail.

### Journey 2: Remote Clip Viewing

* User receives motion alert → Opens app → Sees event metadata in timeline → Taps "View Clip" → SaaS issues time-bound token → KVM VM validates token and requests clip from Edge Appliance → Edge Appliance streams clip over WireGuard tunnel → KVM VM relays stream to user's browser over HTTPS/WebRTC → User views clip without SaaS ever storing the video.

### Journey 3: Archive and Retrieve Evidence

* Important event occurs (e.g., break-in) → Edge Appliance encrypts clip using user-derived key → Encrypted clip sent to KVM VM → KVM VM checks quota and uploads to Filecoin → CID stored in SaaS → Months later, user needs evidence → User requests archived clip → SaaS provides CID → User's browser fetches encrypted blob directly from Filecoin → Browser decrypts locally using user secret → Evidence retrieved without SaaS or KVM VM ever seeing plaintext.

---

## 1. High-Level System Architecture

This section provides a top-level view of the entire system, showing how the multi-tenant SaaS Control Plane connects to per-customer KVM VMs, which in turn connect to distributed Edge Appliances.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SaaS Control Plane                                  │
│                         (Multi-Tenant)                                      │
│                                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │   Web UI     │  │   Mobile     │  │   Billing    │  │ Provisioning │ │
│  │   Dashboard  │  │   App        │  │   Service    │  │   Service    │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
│                                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                     │
│  │   Event      │  │   Auth &     │  │   Health     │                     │
│  │   Inventory  │  │   Identity   │  │   Monitor    │                     │
│  └──────────────┘  └──────────────┘  └──────────────┘                     │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ API / Control Channel
                                    │ (No raw video data)
                                    │
        ┌───────────────────────────┼───────────────────────────┐
        │                           │                           │
        ▼                           ▼                           ▼
┌───────────────┐          ┌───────────────┐          ┌───────────────┐
│  Customer A   │          │  Customer B   │          │  Customer N   │
│  KVM VM       │          │  KVM VM       │          │  KVM VM       │
│               │          │               │          │               │
│ (Private      │          │ (Private      │          │ (Private      │
│  Cloud Node)  │          │  Cloud Node)  │          │  Cloud Node)  │
└───────┬───────┘          └───────┬───────┘          └───────┬───────┘
        │                          │                          │
        │ WireGuard Tunnel         │ WireGuard Tunnel         │ WireGuard Tunnel
        │                          │                          │
        ▼                          ▼                          ▼
┌───────────────┐          ┌───────────────┐          ┌───────────────┐
│  Customer A   │          │  Customer B   │          │  Customer N   │
│  Mini PC      │          │  Mini PC      │          │  Mini PC      │
│  Edge         │          │  Edge         │          │  Edge         │
│  Appliance    │          │  Appliance    │          │  Appliance    │
│               │          │               │          │               │
│  ┌─────────┐  │          │  ┌─────────┐  │          │  ┌─────────┐  │
│  │ Camera  │  │          │  │ Camera  │  │          │  │ Camera  │  │
│  │  1-N    │  │          │  │  1-N    │  │          │  │  1-N    │  │
│  └─────────┘  │          │  └─────────┘  │          │  └─────────┘  │
└───────────────┘          └───────────────┘          └───────────────┘
```

---

## 2. Three-Layer Architecture Detail

This section breaks down each of the three architectural layers in detail, showing responsibilities, components, and data storage policies for each layer.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ LAYER 1: SaaS Control Plane (Multi-Tenant)                                   │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Responsibilities:                                                           │
│  • User accounts, authentication, authorization                              │
│  • Subscription management & billing                                        │
│  • Global event inventory & search                                          │
│  • Unified UI (web & mobile)                                                 │
│  • KVM VM provisioning & lifecycle management                               │
│  • ISO image generation (tenant-specific)                                   │
│  • Health monitoring & alerting                                              │
│  • Archive status & retention policy configuration                           │
│                                                                              │
│  Data Stored:                                                                │
│  • User accounts & credentials (hashed)                                     │
│  • Event metadata (timestamps, labels, camera IDs, CIDs)                    │
│  • Subscription & billing records                                           │
│  • KVM VM assignments & status                                              │
│  • NO raw video, NO decryption keys                                         │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS / API
                                    │ (Event metadata, control commands)
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ LAYER 2: Customer KVM VM (Single-Tenant, "Private Cloud Node")              │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ WireGuard Server Endpoint                                          │    │
│  │ • Terminates tunnel from Edge Appliance                            │    │
│  │ • Per-tenant configuration & secrets                               │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Event Cache & Telemetry Collector                                  │    │
│  │ • Receives event metadata from Edge                                │    │
│  │ • Caches rich metadata locally (bounding boxes, detection scores) │    │
│  │ • Forwards summarized, privacy-minimized metadata to SaaS         │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Filecoin Sync Orchestrator                                         │    │
│  │ • Receives encrypted clips from Edge                               │    │
│  │ • Enforces quota & retention policies                              │    │
│  │ • Uploads to Filecoin/IPFS provider                                │    │
│  │ • Stores CIDs & metadata                                           │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Stream Relay & Control                                             │    │
│  │ • Validates time-bound tokens from SaaS                            │    │
│  │ • Orchestrates on-demand clip streaming from Edge                  │    │
│  │ • Optionally relays encrypted streams to client                     │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Data Stored:                                                                │
│  • No persistent storage of raw or decrypted video                          │
│  • Only short-lived encrypted clip buffers during Filecoin upload            │
│  • Event cache & telemetry                                                  │
│  • CIDs & archive metadata                                                  │
│  • NO decryption keys                                                       │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ WireGuard Tunnel
                                    │ (Encrypted, authenticated)
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ LAYER 3: Edge Appliance (Local/On-Premise, Mini PC)                          │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Video Ingest Engine                                                │    │
│  │ • Connects to RTSP/ONVIF cameras (network)                         │    │
│  │ • Connects to USB cameras (V4L2, direct device access)             │    │
│  │ • Decodes streams using hardware acceleration where available      │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ AI Processing Engine                                               │    │
│  │ • Runs inference (Python/OpenVINO)                                 │    │
│  │ • Detects: people, vehicles, custom models                         │    │
│  │ • Generates event metadata                                         │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Local Storage Manager                                              │    │
│  │ • Stores raw video clips & snapshots on local SSD                  │    │
│  │ • Manages retention policies (e.g., 7-30 days)                     │    │
│  │ • Handles on-demand clip streaming                                 │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Encryption & Archive Client                                        │    │
│  │ • Encrypts clips using user-derived key                            │    │
│  │ • Pushes encrypted blobs to KVM VM for archiving                   │    │
│  │ • Manages archival queue                                           │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ WireGuard Client                                                   │    │
│  │ • Connects to assigned KVM VM                                      │    │
│  │ • Maintains persistent tunnel                                      │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Telemetry & Health Reporter                                        │    │
│  │ • Sends heartbeat & metrics to KVM VM                              │    │
│  │ • Reports: CPU/GPU, disk, camera status, queue length              │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Data Stored:                                                                │
│  • ALL raw video clips & snapshots (local SSD)                              │
│  • Encryption keys (derived from user secret, never transmitted)            │
│  • Camera configurations & AI models                                        │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ RTSP/ONVIF (network)
                                    │ V4L2 (USB)
                                    │ (Local network only)
                                    │
                          ┌─────────┴─────────┐
                          │                   │
                    ┌─────▼─────┐      ┌─────▼─────┐
                    │  Camera 1  │      │  Camera N  │
                    │  (RTSP)    │      │  (ONVIF)   │      │  (USB/V4L2) │
                    └───────────┘      └───────────┘
```

---

## 3. Component Interaction Diagram

This diagram shows the high-level interaction flow between components when a user views a clip, demonstrating how requests flow through the system layers.

```
┌─────────────┐
│   User      │
│  (Browser/  │
│   Mobile)   │
└──────┬──────┘
       │
       │ 1. Login / View Events
       ▼
┌─────────────────────────────────────────────────────────────┐
│              SaaS Control Plane                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Auth       │  │   Event      │  │   Billing    │     │
│  │   Service    │──│   Inventory  │──│   Service    │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
       │
       │ 2. Query Event Metadata
       │    (No video data)
       ▼
┌─────────────────────────────────────────────────────────────┐
│              Customer KVM VM                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Event      │  │   Filecoin   │  │   Stream     │     │
│  │   Cache      │──│   Sync       │──│   Relay      │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
       │
       │ 3. Request Clip Stream (on-demand)
       │    (Time-bound token)
       ▼
┌─────────────────────────────────────────────────────────────┐
│              Edge Appliance                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Video      │  │   AI         │  │   Storage    │     │
│  │   Ingest     │──│   Engine     │──│   Manager    │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
       │
       │ 4a. Stream Clip (encrypted over WireGuard)
       │     Edge → KVM VM
       ▼
┌─────────────────────────────────────────────────────────────┐
│              Customer KVM VM                                │
│  ┌──────────────┐                                           │
│  │   Stream     │                                           │
│  │   Relay      │                                           │
│  └──────┬───────┘                                           │
└─────────┼───────────────────────────────────────────────────┘
          │
          │ 4b. Relay Stream (HTTPS/WebRTC)
          │     KVM VM → User
          ▼
┌─────────────┐
│   User      │
│  (Viewing)  │
└─────────────┘
```

---

## 4. Data Flow Diagrams

This section shows how data flows through the system for detection, live streaming, archiving, and archive retrieval. All flows preserve the core guarantee that raw video never leaves the customer premises unencrypted.

### 4.1 Normal Operation: Detection → Event → Alert

```
┌─────────┐
│ Camera  │
└────┬────┘
     │ RTSP Stream
     ▼
┌─────────────────────────────────────────────────────────────┐
│ Edge Appliance                                               │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐ │
│  │   Video      │─────▶│   AI         │─────▶│  Event   │ │
│  │   Decoder    │      │   Inference  │      │  Detector│ │
│  └──────────────┘      └──────────────┘      └─────┬────┘ │
│                                                     │      │
│  ┌──────────────┐                                  │      │
│  │   Clip       │◀─────────────────────────────────┘      │
│  │   Recorder   │                                          │
│  └──────┬───────┘                                          │
│         │                                                  │
│         │ Store clip locally                               │
│         ▼                                                  │
│  ┌──────────────┐                                          │
│  │   Local SSD  │                                          │
│  └──────────────┘                                          │
│                                                           │
│  ┌──────────────┐                                          │
│  │   Event      │                                          │
│  │   Metadata   │                                          │
│  │   Generator  │                                          │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ Event Metadata (JSON)
          │ (No video)
          │ WireGuard Tunnel
          ▼
┌─────────────────────────────────────────────────────────────┐
│ Customer KVM VM                                              │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐                     │
│  │   Event      │─────▶│   Event      │                     │
│  │   Receiver   │      │   Cache      │                     │
│  └──────────────┘      └──────┬───────┘                     │
│                               │                             │
│                               │ Summarized Metadata         │
│                               ▼                             │
│  ┌──────────────┐                                          │
│  │   SaaS       │                                          │
│  │   Forwarder  │                                          │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ HTTPS/API
          │ (Summarized event metadata)
          ▼
┌─────────────────────────────────────────────────────────────┐
│ SaaS Control Plane                                          │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐ │
│  │   Event      │─────▶│   Timeline   │─────▶│  Alert   │ │
│  │   Inventory  │      │   Service    │      │  Service │ │
│  └──────────────┘      └──────────────┘      └──────────┘ │
└─────────────────────────────────────────────────────────────┘
          │
          │ Push Notification / Email
          ▼
┌─────────┐
│  User   │
└─────────┘
```

### 4.2 Remote Clip Streaming (On-Demand)

```
┌─────────┐
│  User   │
│(Browser)│
└────┬────┘
     │ 1. Click "View Clip" for Event X
     ▼
┌─────────────────────────────────────────────────────────────┐
│ SaaS Control Plane                                          │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐                     │
│  │   Auth       │─────▶│   Token      │                     │
│  │   Check      │      │   Generator  │                     │
│  └──────────────┘      └──────┬───────┘                     │
│                               │                             │
│                               │ Time-bound token            │
│                               │ + Event ID                  │
│                               ▼                             │
│  ┌──────────────┐                                          │
│  │   Stream     │                                          │
│  │   Request    │                                          │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ HTTPS/API
          │ (Token + Event ID)
          ▼
┌─────────────────────────────────────────────────────────────┐
│ Customer KVM VM                                              │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐                     │
│  │   Token      │─────▶│   Stream     │                     │
│  │   Validator  │      │   Orchestrator│                    │
│  └──────────────┘      └──────┬───────┘                     │
└─────────┼─────────────────────┼─────────────────────────────┘
          │                     │
          │                     │ WireGuard
          │                     │ (Token + Event ID)
          │                     ▼
          │            ┌─────────────────────────────────────────────┐
          │            │ Mini PC Edge Appliance                      │
          │            │                                             │
          │            │  ┌──────────────┐      ┌──────────────┐    │
          │            │  │   Token      │─────▶│   Clip       │    │
          │            │  │   Validator  │      │   Reader     │    │
          │            │  └──────────────┘      └──────┬───────┘    │
          │            │                               │            │
          │            │                               │ Read from  │
          │            │                               │ Local SSD  │
          │            │                               ▼            │
          │            │  ┌──────────────┐                          │
          │            │  │   Local SSD   │                          │
          │            │  └──────┬───────┘                          │
          │            │         │                                  │
          │            │         │ Stream clip                      │
          │            │         │ (Encrypted over WireGuard)       │
          │            │         ▼                                  │
          │            │  ┌──────────────┐                          │
          │            │  │   Stream     │                          │
          │            │  │   Sender     │                          │
          │            │  └──────┬───────┘                          │
          │            └─────────┼──────────────────────────────────┘
          │                      │
          │                      │ WireGuard Tunnel
          │                      │ (Encrypted video stream)
          │                      │
          │                      ▼
          │            ┌─────────────────────────────────────────────┐
          │            │ Customer KVM VM                              │
          │            │                                              │
          │            │  ┌──────────────┐                            │
          │            │  │   Stream     │                            │
          │            │  │   Relay      │                            │
          │            │  └──────┬───────┘                            │
          │            └─────────┼────────────────────────────────────┘
          │                      │
          │                      │ HTTPS/WebRTC
          │                      │ (Encrypted stream)
          │                      │
          └──────────────────────┼──────────────────────────────────┘
                                 │
                                 ▼
                          ┌─────────┐
                          │  User   │
                          │(Viewing)│
                          └─────────┘
```

### 4.3 Filecoin Archiving Flow

```
┌─────────────────────────────────────────────────────────────┐
│ Edge Appliance                                               │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐ │
│  │   Event      │─────▶│   Archive    │─────▶│  Clip    │ │
│  │   Selector   │      │   Policy     │      │  Encrypt │ │
│  │   (Policy)   │      │   Check      │      │  Engine  │ │
│  └──────────────┘      └──────────────┘      └─────┬────┘ │
│                                                     │      │
│                                                     │      │
│  ┌──────────────┐                                  │      │
│  │   User       │                                  │      │
│  │   Secret     │──────────────────────────────────┘      │
│  │   (Key Derivation)                                     │
│  └──────────────┘                                          │
│                                                           │
│  ┌──────────────┐                                          │
│  │   Encrypted  │                                          │
│  │   Clip Blob  │                                          │
│  │   + Metadata │                                          │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ WireGuard Tunnel
          │ (Encrypted clip blob)
          ▼
┌─────────────────────────────────────────────────────────────┐
│ Customer KVM VM                                              │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐ │
│  │   Quota      │─────▶│   Filecoin    │─────▶│  CID     │ │
│  │   Checker    │      │   Uploader    │      │  Storage │ │
│  └──────────────┘      └──────────────┘      └─────┬────┘ │
│                                                     │      │
│                                                     │      │
│  ┌──────────────┐                                  │      │
│  │   Archive    │                                  │      │
│  │   Metadata   │◀─────────────────────────────────┘      │
│  │   (CID, size,│                                          │
│  │    retention)│                                         │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ HTTPS/API
          │ (CID + metadata, NO key)
          ▼
┌─────────────────────────────────────────────────────────────┐
│ SaaS Control Plane                                          │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐                     │
│  │   Archive    │─────▶│   Event      │                     │
│  │   Status     │      │   Inventory  │                     │
│  │   Updater    │      │   (Update)   │                     │
│  └──────────────┘      └──────────────┘                     │
└─────────────────────────────────────────────────────────────┘
          │
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│ Filecoin/IPFS Provider                                       │
│                                                              │
│  ┌──────────────┐                                          │
│  │   Encrypted  │                                          │
│  │   Clip Blob  │                                          │
│  │   (Stored)   │                                          │
│  └──────────────┘                                          │
│                                                              │
│  Returns: CID (Content Identifier)                          │
└─────────────────────────────────────────────────────────────┘
```

### 4.4 Archive Retrieval Flow

```
┌─────────┐
│  User   │
│(Browser)│
└────┬────┘
     │ 1. Request "Download from Archive"
     ▼
┌─────────────────────────────────────────────────────────────┐
│ SaaS Control Plane                                          │
│                                                              │
│  ┌──────────────┐      ┌──────────────┐                     │
│  │   Event      │─────▶│   Archive    │                     │
│  │   Lookup     │      │   Metadata   │                     │
│  └──────────────┘      └──────┬───────┘                     │
│                               │                             │
│                               │ CID + Key Identifier        │
│                               │ (NOT the key itself)        │
│                               ▼                             │
│  ┌──────────────┐                                          │
│  │   Client     │                                          │
│  │   Response   │                                          │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ HTTPS/API
          │ (CID + key identifier)
          ▼
┌─────────┐
│  User   │
│(Browser)│
└────┬────┘
     │ 2. Fetch encrypted blob from Filecoin
     │    (Direct to provider, NOT through SaaS)
     ▼
┌─────────────────────────────────────────────────────────────┐
│ Filecoin/IPFS Provider                                       │
│                                                              │
│  ┌──────────────┐                                          │
│  │   Encrypted  │                                          │
│  │   Clip Blob  │                                          │
│  └──────┬───────┘                                          │
└─────────┼──────────────────────────────────────────────────┘
          │
          │ Encrypted blob
          ▼
┌─────────┐
│  User   │
│(Browser)│
└────┬────┘
     │ 3. Decrypt locally using user secret
     │    (SaaS never sees plaintext)
     ▼
┌─────────┐
│  User   │
│(Viewing)│
└─────────┘
```

---

## 5. Security & Privacy Boundaries

This section defines the privacy boundaries, threat model, key management, and security guarantees that ensure customer data remains private even if various components are compromised.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ PRIVACY BOUNDARY 1: Customer Premises                                        │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Edge Appliance                                                      │    │
│  │                                                                     │    │
│  │  ✅ Raw video streams (stays local)                                │    │
│  │  ✅ Full video clips (stays local)                                 │    │
│  │  ✅ Snapshots (stays local)                                        │    │
│  │  ✅ Decryption keys (derived locally, never transmitted)           │    │
│  │                                                                     │    │
│  │  ⚠️  Event metadata (leaves premises)                              │    │
│  │  ⚠️  Encrypted clips (leaves premises for archiving)               │    │
│  │  ⚠️  Telemetry (leaves premises)                                   │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ IP Cameras (RTSP/ONVIF)                                            │    │
│  │  ✅ Never connect to internet directly                             │    │
│  │  ✅ Only accessible on local network                              │    │
│  │                                                                     │    │
│  │ USB Cameras (V4L2)                                                │    │
│  │  ✅ Connected directly to Mini PC via USB                         │    │
│  │  ✅ Accessible via /dev/video* device paths                        │    │
│  │  ✅ Automatic detection via V4L2                                  │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ WireGuard Tunnel
                                    │ (Encrypted, authenticated)
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ PRIVACY BOUNDARY 2: Customer KVM VM (Single-Tenant)                         │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Customer KVM VM                                                    │    │
│  │                                                                     │    │
│  │  ✅ Tenant isolation (dedicated VM)                                 │    │
│  │  ✅ Encrypted tunnel termination                                   │    │
│  │  ✅ Transient encrypted clip buffers (during upload)               │    │
│  │  ✅ Event metadata cache                                           │    │
│  │  ✅ CIDs & archive metadata                                        │    │
│  │                                                                     │    │
│  │  ❌ NO raw video (long-term)                                       │    │
│  │  ❌ NO decryption keys                                             │    │
│  │  ❌ NO plaintext video                                              │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS/API
                                    │ (Event metadata, control)
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ PRIVACY BOUNDARY 3: SaaS Control Plane (Multi-Tenant)                       │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ SaaS Control Plane                                                 │    │
│  │                                                                     │    │
│  │  ✅ User accounts & auth (hashed credentials)                     │    │
│  │  ✅ Event metadata (timestamps, labels, camera IDs)                │    │
│  │  ✅ Subscription & billing                                         │    │
│  │  ✅ CIDs (Content Identifiers)                                      │    │
│  │  ✅ Key identifiers/hashes (NOT keys themselves)                   │    │
│  │                                                                     │    │
│  │  ❌ NO raw video                                                    │    │
│  │  ❌ NO full video clips                                             │    │
│  │  ❌ NO decryption keys                                              │    │
│  │  ❌ NO plaintext video                                              │    │
│  │  ❌ NO biometric templates                                          │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Direct Access
                                    │ (User-initiated)
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ PRIVACY BOUNDARY 4: Filecoin/IPFS (Decentralized Storage)                   │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Filecoin/IPFS Provider                                             │    │
│  │                                                                     │    │
│  │  ✅ Encrypted clip blobs (end-to-end encrypted)                    │    │
│  │  ✅ CIDs (public identifiers)                                       │    │
│  │                                                                     │    │
│  │  ❌ NO decryption keys                                              │    │
│  │  ❌ NO plaintext video                                              │    │
│  │  ❌ NO ability to decrypt (without user secret)                     │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Security Guarantees Summary

| Data Type | Location | Encryption | Access Control |
|-----------|----------|------------|----------------|
| Raw Video Streams | Edge Appliance (local) | N/A (local only) | Local network isolation |
| Video Clips | Edge Appliance (local) | N/A (local only) | Local filesystem |
| Event Metadata | KVM VM, SaaS | In-transit (TLS/mTLS) | Tenant isolation |
| Encrypted Archive Clips | Filecoin/IPFS | End-to-end (user key) | CID-based access |
| Decryption Keys | User device only | N/A (never transmitted) | User custody |
| WireGuard Traffic | Edge ↔ KVM VM | WireGuard encryption | Per-tenant tunnel |

### Threat Model

This subsection explicitly defines the security assumptions and protections against various attack scenarios.

#### Security Assumptions

- **Edge Appliance**: Physically under customer control; customer is responsible for physical security of the device.
- **KVM VM Hosts**: Secured and monitored by the provider; host infrastructure is trusted for isolation and availability.
- **Filecoin/IPFS Providers**: Untrusted storage (honest-but-curious at best); providers cannot decrypt customer data.

#### Attack Scenarios & Protections

| Compromised Component | What Attacker Can Access | What Attacker Cannot Access | Protection Level |
|----------------------|-------------------------|----------------------------|------------------|
| **SaaS Control Plane** | Event metadata (timestamps, labels, camera IDs), CIDs, subscription info | Raw video, full clips, decryption keys, plaintext video | Metadata only; no video content |
| **Filecoin/IPFS Provider** | Encrypted clip blobs, CIDs | Decryption keys, plaintext video | Encrypted blobs only; cannot decrypt |
| **SaaS + Filecoin** | Event metadata, encrypted blobs, CIDs | Decryption keys, plaintext video | Still no keys; end-to-end encryption protects content |
| **KVM VM** | Encrypted clips in transit/at rest (during upload), event metadata cache | Decryption keys, plaintext video, long-term raw video | Encrypted data only; keys never stored |
| **Edge Appliance** (physical compromise) | All local data including raw video and derived encryption keys | N/A | Physical security is customer responsibility; keys can be rotated |

#### Key Protection Guarantees

- Decryption keys are **never transmitted** off the Edge Appliance or user device.
- Keys are derived from user secrets that remain under customer control.
- Compromise of SaaS, KVM VM, or Filecoin providers does not expose decryption keys.
- Even with full access to encrypted archives, attackers cannot decrypt without the user secret.

### Key Management Model

This subsection describes how encryption keys are derived, managed, and used throughout the system lifecycle.

#### Key Derivation

- **User Secret**: Customer-controlled secret (passphrase or hardware-backed key) used as the root for key derivation.
- **Key Derivation**: Encryption keys are derived from the user secret using standard key derivation functions (e.g., PBKDF2, Argon2).
- **Key Storage**: Derived keys are stored only on the Edge Appliance and user devices; never transmitted to SaaS or KVM VM.

#### Key Lifecycle

- **Initial Configuration**: User secret is configured during Edge Appliance setup (via SaaS UI with local pairing code, or USB-based secure transfer).
- **Key Rotation**: Users can rotate their encryption key; existing encrypted archives remain accessible with the old key until re-encrypted (if desired).
- **Multi-Device Scenarios**: 
  - Single user with multiple Edge Appliances: All devices can use the same user secret (shared key derivation).
  - Per-site/per-device keys: Optionally, each Edge Appliance can derive device-specific keys from a master user secret.
- **Key Recovery**: If user loses their secret, archived clips encrypted with that key cannot be recovered (by design, for security). Local clips on Edge Appliance remain accessible until device is reset.

#### Browser-Based Decryption

For archive retrieval, decryption happens in the user's browser using the user secret:
- User enters passphrase or uses hardware-backed key (e.g., WebAuthn).
- Browser derives decryption key locally.
- Encrypted blob is fetched directly from Filecoin/IPFS provider.
- Decryption occurs entirely in the browser; SaaS and KVM VM never see plaintext or the user secret.

#### Key Identifiers

- SaaS and KVM VM may store key identifiers/hashes for mapping clips to encryption metadata.
- These identifiers are cryptographic hashes, not the keys themselves.
- Key identifiers cannot be used to decrypt content.

### Observability & Logging (Privacy-Aware)

This subsection defines what telemetry and logs are collected, and what is explicitly excluded to protect privacy.

#### What Is Logged

- **Resource Metrics**: CPU/GPU utilization, disk usage, memory consumption, network bandwidth.
- **Operational Metrics**: Camera online/offline status, event counts per camera, archival queue lengths.
- **Health Indicators**: Heartbeat status, connectivity state, software version, update status.
- **Structured Metadata**: Event types (e.g., "Person detected"), timestamps, camera identifiers, detection confidence scores.

#### What Is NOT Logged

- **Raw Video Frames**: No video frames or pixel data are logged.
- **Biometric Templates**: No face embeddings, fingerprint data, or other biometric identifiers.
- **Free-Text Descriptions**: No verbose descriptions of individuals or activities.
- **Decryption Keys**: No encryption keys or key material.
- **User Secrets**: No passphrases or authentication secrets.

#### Logging Philosophy

- **Structured, Minimal Metadata**: Only structured, privacy-minimized metadata is logged.
- **Purpose-Limited**: Logs are used solely for operational monitoring, health checks, and support; not for analytics or profiling.
- **Retention Policies**: Logs are retained only as long as necessary for operational purposes.

### Communication Security

This subsection details the security of all communication channels in the system.

#### End-to-End Channel Security

- **Internet Clients → SaaS**: TLS (HTTPS) for all web and mobile app communication.
- **SaaS → KVM VM**: Mutual TLS (mTLS) with per-VM certificates for all API calls and control commands.
- **Edge Appliance → KVM VM**: WireGuard encrypted tunnel with per-tenant authentication.
- **KVM VM → Filecoin/IPFS**: TLS (HTTPS) for archive uploads and retrievals.
- **User → Filecoin/IPFS**: TLS (HTTPS) for direct archive retrieval (bypassing SaaS/KVM).

#### Authentication & Authorization

- **User Authentication**: Standard OAuth2/OIDC or similar for SaaS UI access.
- **Edge Appliance Authentication**: WireGuard key-based authentication to KVM VM; initial bootstrap via secure tokens embedded in ISO.
- **KVM VM Authentication**: mTLS certificates for SaaS communication; WireGuard server keys for Edge connections.

---

## 6. Network Topology

This section illustrates the network architecture, showing how components communicate across the internet and local networks, including ports and protocols used.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Internet / Public Network                                                    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ SaaS Control Plane                                                 │    │
│  │ • Web UI (HTTPS)                                                   │    │
│  │ • Mobile App API (HTTPS)                                           │    │
│  │ • KVM VM Management API (HTTPS)                                    │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Customer KVM VMs (Per-Tenant)                                      │    │
│  │ • WireGuard Server (UDP 51820)                                     │    │
│  │ • Control API (HTTPS)                                              │    │
│  │ • Stream Relay (HTTPS/WebRTC)                                      │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Filecoin/IPFS Providers                                            │    │
│  │ • IPFS Gateway (HTTPS)                                             │    │
│  │ • Filecoin Storage Market                                          │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ WireGuard Tunnel
                                    │ (UDP 51820)
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ Customer Premises (Private Network)                                          │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Edge Appliance                                                      │    │
│  │ • WireGuard Client (connects to KVM VM)                            │    │
│  │ • Video Ingest (RTSP/ONVIF client)                                 │    │
│  │ • Local Storage (SSD)                                               │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Network Cameras (Local Network Only)                              │    │
│  │ • Camera 1 (RTSP: rtsp://192.168.1.100:554/stream)                 │    │
│  │ • Camera 2 (ONVIF: http://192.168.1.101:8080)                      │    │
│  │ • Camera N (RTSP/ONVIF)                                            │    │
│  │                                                                     │    │
│  │  ⚠️  Never directly accessible from internet                        │    │
│  │  ✅ Only accessible on local network                               │    │
│  │                                                                     │    │
│  │ USB Cameras (Direct Connection)                                    │    │
│  │ • Camera 1 (USB: /dev/video0 via V4L2)                             │    │
│  │ • Camera 2 (USB: /dev/video1 via V4L2)                             │    │
│  │ • Camera N (USB/V4L2)                                              │    │
│  │                                                                     │    │
│  │  ✅ Connected directly to Mini PC via USB                         │    │
│  │  ✅ Automatic detection via V4L2                                  │    │
│  │  ✅ Accessible via device path (/dev/video*)                       │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Network Ports & Protocols

| Component | Port | Protocol | Purpose | Direction |
|-----------|------|----------|---------|-----------|
| SaaS Web UI | 443 | HTTPS | User interface | User → SaaS |
| SaaS API | 443 | HTTPS | API calls | User/Mobile → SaaS |
| KVM VM Control | 443 | HTTPS (mTLS) | Management API | SaaS → KVM VM |
| WireGuard | 51820 | UDP | Encrypted tunnel | Edge ↔ KVM VM |
| RTSP | 554 | RTSP | Video stream | Edge → Network Camera |
| ONVIF | 8080 | HTTP | Camera discovery | Edge → Network Camera |
| V4L2 | N/A | Device | USB camera access | Edge → USB Camera (via /dev/video*) |
| Filecoin/IPFS | 443 | HTTPS | Archive upload/retrieval | KVM VM ↔ Provider |

---

## 7. Deployment Architecture

This section shows the physical and logical deployment structure, including cloud infrastructure, KVM host architecture, and distributed Edge Appliances at customer premises.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Cloud Infrastructure (Multi-Tenant SaaS)                                     │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Load Balancer                                                       │    │
│  │ • Routes HTTPS traffic to SaaS services                            │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ SaaS Application Servers (Auto-scaling)                             │    │
│  │ • Web UI service                                                   │    │
│  │ • API service                                                      │    │
│  │ • Auth service                                                     │    │
│  │ • Billing service                                                  │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Database Cluster (Multi-Tenant)                                   │    │
│  │ • User accounts                                                    │    │
│  │ • Event metadata                                                   │    │
│  │ • Subscription records                                             │    │
│  │ • KVM VM assignments                                               │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ KVM Host Infrastructure                                           │    │
│  │                                                                     │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │    │
│  │  │  KVM Host 1  │  │  KVM Host 2  │  │  KVM Host N  │           │    │
│  │  │              │  │              │  │              │           │    │
│  │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │           │    │
│  │  │ │Customer A│ │  │ │Customer C│ │  │ │Customer E│ │           │    │
│  │  │ │  KVM VM  │ │  │ │  KVM VM  │ │  │ │  KVM VM  │ │           │    │
│  │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │           │    │
│  │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │           │    │
│  │  │ │Customer B│ │  │ │Customer D│ │  │ │Customer F│ │           │    │
│  │  │ │  KVM VM  │ │  │ │  KVM VM  │ │  │ │  KVM VM  │ │           │    │
│  │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │           │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘           │    │
│  │                                                                     │    │
│  │  • Each KVM VM is single-tenant                                    │    │
│  │  • Multiple VMs per host (resource isolation)                      │    │
│  │  • Auto-provisioning via SaaS API                                 │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ ISO Generation Service                                            │    │
│  │ • Builds tenant-specific ISO images                                │    │
│  │ • Embeds bootstrap configuration                                   │    │
│  │ • Secure download endpoint                                         │    │
│  └────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Internet
                                    │
┌──────────────────────────────────────────────────────────────────────────────┐
│ Customer Premises (Distributed)                                             │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Customer A: Home/Office                                            │    │
│  │                                                                     │    │
│  │  ┌──────────────┐                                                  │    │
│  │  │  Edge        │                                                  │    │
│  │  │  Appliance   │                                                  │    │
│  │  └──────┬───────┘                                                  │    │
│  │         │                                                          │    │
│  │         │ Local Network                                            │    │
│  │         │                                                          │    │
│  │  ┌──────▼───────┐  ┌──────────┐  ┌──────────┐                    │    │
│  │  │  Camera 1    │  │ Camera 2 │  │ Camera N │                    │    │
│  │  └──────────────┘  └──────────┘  └──────────┘                    │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │ Customer B: Small Business                                         │    │
│  │                                                                     │    │
│  │  ┌──────────────┐                                                  │    │
│  │  │  Edge        │                                                  │    │
│  │  │  Appliance   │                                                  │    │
│  │  └──────┬───────┘                                                  │    │
│  │         │                                                          │    │
│  │         │ Local Network                                            │    │
│  │         │                                                          │    │
│  │  ┌──────▼───────┐  ┌──────────┐  ┌──────────┐                    │    │
│  │  │  Camera 1    │  │ Camera 2 │  │ Camera N │                    │    │
│  │  └──────────────┘  └──────────┘  └──────────┘                    │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ... (N customers, each with isolated infrastructure)                       │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Deployment Characteristics

- **SaaS Control Plane**: Multi-tenant, auto-scaling, cloud-native
- **KVM VMs**: Single-tenant per customer, provisioned on shared hosts
- **Edge Appliances**: Customer-owned hardware, distributed globally
- **Isolation**: Network (WireGuard), compute (KVM), data (encryption)
- **Scalability**: Horizontal scaling for SaaS, per-customer VM scaling for KVM layer

### Reliability, High Availability & Scaling

This subsection describes how the system ensures reliability and handles failures at each layer.

#### SaaS Control Plane

- **Multi-AZ Deployment**: Application servers and databases deployed across multiple availability zones for fault tolerance.
- **Database Backups**: Point-in-time recovery and automated backups for data protection.
- **Auto-Scaling**: Application servers scale horizontally based on load (CPU, memory, request rate).
- **Load Balancing**: Traffic distributed across multiple application instances.

#### KVM VM Layer

- **Host Failure Handling**: If a KVM host fails, customer VMs are automatically restarted on another host (VM migration or reprovisioning).
- **Active Monitoring**: Continuous monitoring of KVM host health with automated alerts and remediation.
- **VM Scaling**: High-load customers can receive:
  - Larger VM flavors (more CPU, memory, storage).
  - Multiple VMs per tenant (e.g., one per site) under the same tenant account.
- **Auto-Reprovisioning**: Failed or unhealthy VMs are automatically reprovisioned with state recovery.

#### Edge Appliances

- **Offline Resilience**: Loss of connectivity to KVM VM does **not** stop local recording; only remote viewing and archiving are affected.
- **Health Monitoring**: Edge Appliances send regular heartbeats and telemetry to KVM VM.
- **Offline Alerts**: SaaS surfaces "Edge Appliance offline" alerts to users when connectivity is lost.
- **Queue Management**: Archival queue persists locally; syncs resume when connectivity is restored.

#### Edge Software & Model Updates

- **Update Delivery**: Edge Appliances receive signed software and AI model updates via the KVM VM over the WireGuard tunnel.
- **Version Management**: Version checks ensure compatibility and enable rollback if needed.
- **Update Windows**: Updates are coordinated during low-usage windows to minimize disruption.
- **Security Patches**: Critical security patches (CVEs) can be pushed with priority scheduling.

---

## Legend

- **Solid lines (──)**: Data flow / communication
- **Dashed lines (- -)**: Control flow / management
- **Boxes (┌─┐)**: Components / services
- **Arrows (→)**: Direction of data/control flow
- **✅**: Allowed / Secure
- **❌**: Prohibited / Not stored
- **⚠️**: Conditional / Policy-dependent

---

*This architecture document is based on the concept outlined in README.md and provides visual representations of the system design, data flows, and security boundaries.*

