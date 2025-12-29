# State Manager: production workflow & business logic (Edge ↔ VM, IoT, training, inference)

Date: 2025-12-28  
Scope: `edge/orchestrator/internal/state-mng/*`  
Related docs:
- `edge/orchestrator/internal/iot/doc.go` (device-agnostic IoT service + patterns)
- `edge/orchestrator/internal/vm-gateway/doc.go` (provider-agnostic secure Edge↔VM connectivity + connection state machine)
- `edge/orchestrator/internal/ai-gateway/AI_GATEWAY_DEVICE_AGNOSTIC_REVIEW.md` (device-agnostic target for inference integration)

This document defines the **exact production workflow** the Edge appliance must follow to:
- discover IoT devices, synchronize them with VM, and accept VM “device assignment”
- collect and upload per-device training datasets
- receive, verify, store, and activate **one trained model per device**
- continuously detect security events from raw device data, including **offline** (VM disconnect) operation
- recover from reboots and storage corruption using **reconciliation** and **VM-assisted resync**

It is intentionally **business-logic and reliability oriented**. It is not a PoC/MVP spec.

---

## Edge↔VM bidirectional architecture (critical for production)

Edge communicates with VM in **two directions**:

### Edge → VM (outbound, via HTTPS client)
- **Tunnel**: WireGuard client (Edge initiates connection to VM endpoint)
- **Transport**: HTTPS client (Edge makes authenticated requests to VM)
- **Operations**:
  - Authentication (`Authenticate`)
  - Device sync (`SyncDevices`) - Edge reports discovered devices
  - Data unit sync (`SyncDataUnits`) - Edge sends labeled training data units to VM
  - Security event delivery (`SendEvents`) - Edge sends detections
  - Model deployment status (`ReportModelStatus`) - Edge acknowledges deployments

### VM → Edge (inbound, via Edge HTTPS server)
- **Server**: Edge runs HTTPS server (listens on configured port, e.g., 8443)
- **Authentication**: mTLS (VM presents client certificate; Edge validates)
- **Operations**:
  - Capabilities sync (`POST /api/v1/capabilities/sync`) - VM tells Edge which capabilities to enable
  - Data unit capture requests (`POST /api/v1/snapshots/capture`) - VM requests training data collection (images from cameras, sensor readings, audio samples, etc.)
  - Model deployment (`POST /api/v1/models/deploy`) - VM pushes trained models

**Production requirements**:
- Edge HTTPS server **must be ready before authentication completes** (VM may send commands immediately)
- Both directions require mTLS with certificate pinning
- Edge must handle VM commands even if local device discovery is incomplete (queue commands)
- All VM→Edge commands must be idempotent (safe to retry/duplicate)

---

## Core invariants (must always hold)

- **Per-device model isolation**: each IoT device has **its own** model lifecycle (dataset → training → model → deployment → inference). No shared implicit global “model”.
- **Safety-first behavior**:
  - no unverified model artifacts are ever executed
  - when model/device/storage integrity is uncertain, the system must fail into a **safe degraded mode** and request remediation from VM
- **Offline tolerance**:
  - temporary VM connectivity loss must not stop inference for devices that already have an active model and device connectivity
  - events are queued and delivered later (at-least-once, idempotent at receiver)
- **Reboot resilience**:
  - after a physical restart, Edge must restart monitoring automatically for each eligible device (device present + model present + policy allows)
- **Reconciliation over “hope”**: correctness must not rely on event ordering or uninterrupted connectivity; instead it relies on **idempotent operations** and periodic reconciliation loops.

---

## Roles & responsibilities

### Edge responsibilities (authoritative on Edge)

- **Device management** (via `iot`): discovery, registration, device state tracking, capability reporting.
- **Dataset capture + labeling orchestration** (via `web-gateway` + IoT device data hooks):
  - trigger data unit capture requests (images, sensor readings, audio samples, etc.)
  - record labeling results and dataset readiness
- **Dataset packaging + upload** to VM (via `vm-gateway` transport):
  - deterministic dataset identity
  - encryption, integrity, resumable upload, and audit trail
- **Model intake + activation**:
  - verify model signatures + compatibility
  - store in object storage; store metadata + deployment state in meta storage
  - activate inference for that device
- **Security event detection** from raw device data using trained models:
  - persist events locally (meta/object storage)
  - queue and send to VM with backpressure and retry

### VM responsibilities (authoritative on VM)

- Decide **which devices** this Edge should serve (assignment policy).
- Request training datasets per device (what labels, counts, quality constraints).
- Train and version models per device and **deploy** models back to Edge.
- Act as **recovery authority** to resync devices/models when Edge storage is corrupted.

### State Manager responsibilities (this package)

State Manager is the **workflow orchestrator**:
- it does **not** own the VM connection state machine (owned by `vm-gateway`)
- it does **not** own device state machines (owned by `iot`’s `DeviceStateService`)
- it **does** own the cross-service workflows and the **ML lifecycle coordination**

---

## Edge component contracts (what each service owns)

This section makes the boundaries explicit so the upcoming `state-mng` and `event-bus` refactors have a single shared truth.

### `vm-gateway` (secure Edge↔VM connectivity + VM API transport)

- **Owns**:
  - tunnel lifecycle (provider-agnostic; e.g., WireGuard) - **Edge → VM outbound**
  - transport lifecycle (provider-agnostic; e.g., HTTPS) - **bidirectional**
    - **HTTPS client** (Edge → VM): authentication, device sync, dataset upload, event delivery
    - **HTTPS server** (VM → Edge): receives capabilities, data unit capture requests, model deployments
  - connection state machine: `disconnected → tunnel_connecting → tunnel_connected → transport_connecting → transport_connected → authenticated → capabilities_received`
  - edge authentication and identity presentation to VM (mTLS identity on transport)
- **Emits** (via event bus):
  - connection and auth transitions (e.g., tunnel/transport connected/disconnected, authenticated)
  - `edge.capabilities_received` when VM pushes capabilities to Edge
- **Consumes**:
  - configuration for providers + credentials (certs/keys) and endpoints
  - VM commands via HTTPS server endpoints (capabilities sync, data unit capture requests, model deployments)
- **Must guarantee**:
  - contexts flow from callers (no long-lived stored ctx)
  - retry/backoff for transient failures without thrashing
  - health snapshot observability
  - **HTTPS server must be ready before authentication completes** (VM may send commands immediately)
  - **Bidirectional mTLS**: Edge validates VM client cert on server endpoints; VM validates Edge client cert on client calls
- **Security**:
  - mTLS required for both directions; certificate rotation/revocation strategy required
  - VM identity pinning (trust root) required for VM→Edge commands
  - Edge identity pinning (trust root) required for Edge→VM calls

### `iot` (device-agnostic device lifecycle + data pipeline)

- **Owns**:
  - device discovery, registration, and persistence (device registry)
  - device state machines (per device)
  - device capability model
  - data acquisition interfaces (capture/stream)
  - processing pipeline (registered `DataProcessor`s)
- **Emits** (via event bus):
  - device lifecycle events (discovered/registered/connected/disconnected)
  - raw device data availability events (frame received / data unit recorded) *if used*
- **Consumes**:
  - device plugins (CCTV is one plugin; more types in future)
- **Must guarantee**:
  - device IDs are stable and usable as VM correlation keys
  - capability checks gate operations (e.g., video streaming/capture)
  - health snapshot and robust shutdown semantics

### `ai-gateway` (inference façade + model-aware AI service integration)

**Production responsibilities (what `ai-gateway` must own):**
- **Inference execution**:
  - invoke AI service for inference (bounded by timeouts, retries, and circuit breaking)
  - apply model selection per device (no “global default” model for multiple devices)
  - enforce confidence/class filters
- **Model notification boundary**:
  - accept “model deployed” notifications with metadata sufficient for AI service to load the model
  - *must not* activate unverified models (verification belongs to State Manager; see “Model intake”)
- **Event detection boundary**:
  - convert inference output into a normalized detection/anomaly result
  - signal detections back to the orchestrator (`state-mng`) via a callback or event bus

**Device-agnostic implementation (required by `AI_GATEWAY_DEVICE_AGNOSTIC_REVIEW.md`):**
- `ai-gateway` must be implemented as:
  - **Option A (preferred)**: an IoT `DataProcessor` for `DeviceDataTypeVideoFrame` (and other relevant types), where `iot` owns ingestion and `ai-gateway` owns inference; State Manager binds models and handles event forwarding, OR
  - **Option B**: a service that consumes `iot/types.DeviceData` and is addressed by `DeviceID`

**Failure semantics**:
- inference failures must be non-fatal and must not block device management
- repeated failures should trigger a degraded mode with operator visibility (health + audit)

**Security**:
- never log raw frames or datasets
- treat AI service as untrusted input surface: strict response validation and bounded resource usage

### `state-mng` (workflow orchestration + ML lifecycle + policy)

- **Owns**:
  - cross-service workflows and sequencing
  - per-device ML lifecycle records (persisted)
  - reconciliation loops (startup, periodic, reconnection)
  - offline queueing and eventual delivery contracts (datasets, security events, audit logs)
  - policy decisions that are Edge-local (e.g., whether offline inference is allowed)
- **Consumes**:
  - VM connection state from `vm-gateway`
  - device state and data availability from `iot`
  - user/UI actions via `web-gateway` APIs
  - inference results via `ai-gateway` (or via IoT processing pipeline)
- **Emits**:
  - internal workflow events via `event-bus`
  - VM-facing API calls via `vm-gateway` transport
- **Must guarantee**:
  - per-device serialization of workflows (no concurrent conflicting operations for same device)
  - idempotency for all VM-facing operations using stable keys
  - crash-safe checkpoints in meta storage

### `event-bus` (internal workflow event distribution; not the "SecurityEvent store")

`event-bus` is the **internal orchestration bus** for component coordination and workflow triggers. It is not the authoritative store for security detections or datasets.

- **Owns**:
  - publish/subscribe distribution within the Edge process (and optionally across processes if implemented)
  - persistence/reliability implementation (current provider is meta-storage backed)
  - optional ordering mode
- **Guarantees and non-guarantees** (from the interface contract):
  - `Publish` should be **non-blocking** and may drop on overflow; producers must not assume delivery
  - **Which events can be dropped**: workflow trigger events (device.discovered, model.deployment_received) can be dropped because reconciliation will catch them
  - **Which events must NOT be dropped**: operational/health events (storage.full, health.degraded) must be reliably delivered
  - subscribers receive events until unsubscribed or bus closed
  - ordering may be "best-effort" or "strict" depending on configuration; workflows must still be reconciliation-driven
- **How State Manager must use it**:
  - as a trigger/notification channel only
  - never as the sole source of truth (reconciliation is mandatory)
  - every event handler must be idempotent (duplicate delivery is always possible)
- **Event bus persistence and cleanup**:
  - persisted events retained for 24 hours (configurable)
  - cleanup runs every 6 hours
  - if persistence storage >90% full: drop droppable events only; emit storage pressure alert

### `web-gateway` (Edge UI/API façade)

`web-gateway` hosts the UI and exposes REST APIs (cameras/devices, datasets/snapshots, events, metrics).

- **Owns**:
  - web server lifecycle (HTTP/HTTPS)
  - request authentication/authorization for UI access (Edge-local)
  - API surface for:
    - listing devices and their states
    - managing data unit/dataset collection and labeling workflows
    - viewing security events and attachments (subject to policy)
    - operational health/metrics dashboards
- **Consumes**:
  - meta/object storage for serving metadata and attachments
  - `vm-gateway` for proxying or presenting VM-related state where required
  - `event-bus` for live updates (optional) and State Manager for commands
- **Security**:
  - strict authZ for dataset/event access (principle of least privilege)
  - safe logging (no sensitive payloads)
  - rate limiting and input validation to prevent resource exhaustion

### `audit-log` (tamper-oriented security audit logging)

`audit-log` is the compliance/security audit layer. It is separate from operational logs and separate from security detections.

- **Owns**:
  - durable, queryable audit records for security-sensitive actions
  - periodic sync to VM for long-term retention (default: every 5 minutes or 1000 records, whichever comes first)
  - cleanup/retention enforcement on Edge (keep last 90 days locally, configurable)
- **Must be called by**:
  - State Manager on key lifecycle actions:
    - dataset creation/labeling completion/upload
    - model deployment receipt/verification/activation
    - security event creation and transmission attempts
    - recovery actions (resync requests, storage corruption detection)
  - `web-gateway` on:
    - UI authentication and authorization decisions
    - dataset/event access ("who accessed what")
- **Security properties**:
  - audit records must be tamper-evident (hash-chaining or append-only store recommended)
  - syncing is at-least-once; VM must deduplicate by audit entry idempotency key
- **Audit sync failure handling**:
  - if sync to VM fails: queue audit records locally (max queue: 100,000 records, configurable)
  - if queue is full: emit critical alert; **do NOT drop audit records**; pause sensitive operations until sync resumes
  - operator must investigate and resolve (extend queue, fix VM connectivity, or manually export audit logs)

---

## Terms & identifiers

### Edge identity

- **EdgeID**: stable, globally unique identifier for this Edge appliance.
  - Must be bound to the Edge's mTLS certificate (certificate SAN or CN contains EdgeID).
  - VM uses EdgeID to correlate all operations (device assignments, event delivery, audit logs).
  - EdgeID must survive Edge software upgrades (stored in persistent config or derived from hardware ID).

### Device identity

- **DeviceID**: stable identifier for an IoT device on Edge and VM (device-agnostic).
  - Generated by Edge during device discovery and registration.
  - Must be stable across device reconnections and Edge restarts.
  - Format: `{device_type}-{unique_suffix}` (e.g., `camera-192.168.1.100`, `sensor-temp-001`).

### Data unit

- **DataUnit**: device-agnostic term for a labeled training data sample. Can be:
  - Images (JPEG, PNG) from video devices (cameras)
  - Sensor readings (JSON) from sensors
  - Audio samples (WAV, MP3) from audio devices
  - Any other labeled data format
- **DataUnitID**: unique identifier for a single data unit.

### Dataset

- **Dataset**: a set of labeled data units for a single device (device-agnostic).
- **DatasetID**: deterministic identifier derived from canonical hash of dataset manifest.
  - Guarantees idempotency: same data units + labels → same DatasetID.
  - Hash includes: EdgeID, DeviceID, sorted list of (DataUnitID, label) pairs, schema version.
  - Collision handling: if VM reports DatasetID collision (astronomically unlikely), Edge must regenerate with added salt/timestamp.

### Model

- **ModelArtifact**: the trained model binary + runtime artifacts (e.g., OpenVINO IR, labels, preprocessing config).
- **ModelID**: immutable identifier for a specific trained model version.
- **ModelDeployment**: binding between a DeviceID and a ModelID plus rollout metadata (status, timestamps, reason).

### Security event

- **SecurityEvent**: detection/anomaly raised by inference on device data.
- **EventID**: immutable identifier (UUID); attachments stored in object storage and referenced from metadata.
- **Required fields**: EventID, DeviceID, DeviceType, event_type, timestamp, confidence, ModelID, model_version.
- **Optional fields**: bounding_box, attachment_refs (object storage keys), metadata (device-specific context).

---

## State model

### Connection state (owned by `vm-gateway`)

`vm-gateway` provides the authoritative connection state machine:

`disconnected → tunnel_connecting → tunnel_connected → transport_connecting → transport_connected → authenticated → capabilities_received`

State Manager consumes these transitions via event bus and via VMGateway query APIs.

### Device operational state (owned by `iot`)

IoT provides per-device state machines (device-agnostic). State Manager must treat IoT as the source of truth for:
- discovery/registration
- connectivity and capability readiness
- data streaming availability (a stream is "available" when device is connected AND streaming capability is ready AND no device-level errors exist)

### ML lifecycle state (owned by State Manager; persisted)

State Manager maintains **per-device** ML lifecycle state (persisted in meta storage) to make workflows restart-safe:

- **Unassigned**: device exists locally but VM has not assigned it to this Edge.
- **Assigned**: VM assigned device to Edge; Edge must fulfill dataset/model workflow.
- **AwaitingDataset**: VM requested dataset; Edge is collecting labeled units.
- **DatasetReadyLocal**: dataset exists locally and passed validation.
- **DatasetUploadInProgress / DatasetUploaded**: upload flow state with resumable cursor.
- **TrainingPending**: VM acknowledged dataset and queued training (optional if VM is async).
- **ModelAvailable**: VM produced a model and signaled availability; Edge may need to fetch.
- **ModelStored**: model is stored in object/meta storage and verified.
- **InferenceActive**: inference loop is running for this device.
- **DegradedNoModel**: device is active but no valid model is available (Edge cannot detect).
- **RecoveryRequired**: local storage integrity prevents safe operation; VM-assisted resync required.

**Rule**: transitions must be **idempotent** and driven by reconciliation (not only events).

---

## End-to-end workflow (happy path)

### 1) Edge startup (cold start or restart)

On process start:
- Start `iot` service (device discovery/registry/state machines).
- Start `vm-gateway` (tunnel + transport + auth + HTTPS server for VM→Edge commands).
- Start State Manager orchestrator:
  - subscribe to event bus
  - load persisted ML lifecycle state per device
  - run **Startup Reconciliation Loop** (see below)

**Critical**: Edge runs an **HTTPS server** (via `vm-gateway`) that receives commands FROM VM. This is separate from the Edge→VM HTTPS client used for authentication and data uploads.

### 2) Establish secure Edge↔VM connection (bidirectional)

The connection establishment follows this **exact sequence**:

**Phase 2.1: Tunnel establishment (Edge → VM)**
- Edge initiates WireGuard tunnel to VM endpoint (configured at startup)
- Tunnel state: `disconnected → tunnel_connecting → tunnel_connected`
- If tunnel fails: retry with exponential backoff; do not proceed to transport

**Phase 2.2: Transport establishment (Edge → VM)**
- After tunnel is connected, Edge initiates HTTPS transport connection
- Transport state: `transport_connecting → transport_connected`
- Uses mTLS: Edge presents client certificate; VM validates Edge identity

**Phase 2.3: Edge authentication (Edge → VM)**
- Edge calls `VMGateway.Authenticate(ctx, edgeID)` to authenticate with VM
- VM validates Edge certificate and EdgeID
- Connection state: `transport_connected → authenticated`
- **Production requirement**: authentication must succeed within 30s timeout; retry on failure

**Phase 2.4: VM sends capabilities to Edge (VM → Edge push)**
- **VM initiates** capability sync by POSTing to Edge's HTTPS server: `POST /api/v1/capabilities/sync`
- Edge receives capabilities (e.g., `{"capabilities": {"cctv_camera": true}}`)
- Edge stores capabilities in meta-storage
- Edge publishes `edge.capabilities_received` event to event bus
- Connection state: `authenticated → capabilities_received`
- **Production requirement**: Edge must be ready to receive VM commands before authentication completes

**Phase 2.5: Device discovery and sync (Edge → VM)**
- IoT service discovers devices locally (ONVIF, RTSP, USB, etc.)
- State Manager consumes device discovery events
- When connection reaches `capabilities_received`, State Manager:
  - **Edge → VM**: calls `VMGateway.SyncDevices(ctx, devices)` to report discovered devices
  - **VM → Edge**: VM responds with device assignments (which devices Edge should serve)
  - For each assigned device, State Manager transitions ML lifecycle to **Assigned**

**Production reliability notes**:
- Tunnel/transport/auth failures must not block local device discovery (IoT continues independently)
- Edge HTTPS server must be ready **before** authentication completes (VM may send capabilities immediately after auth)
- All VM→Edge commands require mTLS (VM presents client cert; Edge validates VM identity)
- Edge must handle out-of-order VM commands gracefully (e.g., capabilities before devices are discovered)
- **On reconnection** (transition from `disconnected` → `authenticated` → `capabilities_received`):
  - **Immediately trigger security event sync**: send all unsynced security events to VM (do not wait for next reconciliation)
  - This is a high-priority operation to ensure no security events are lost during disconnection

### 3) VM assigns devices to Edge

VM assignment is returned **in response to Edge's device sync** (`SyncDevices` response):
- VM's `SyncDevicesResponse` contains `Assignments` map: `DeviceID → {assigned: bool, policy: string}`
- For each assigned DeviceID, Edge transitions ML lifecycle to **Assigned**
- For each non-assigned device, Edge must not start training/inference (unless policy explicitly allows local-only)
- **Production requirement**: Edge must handle assignment changes gracefully (device unassigned → stop inference; device newly assigned → start workflow)

### 4) VM requests per-device training dataset

**VM initiates** dataset request by POSTing to Edge's HTTPS server (VM → Edge push):
```
POST https://{edge_https_server}/api/v1/snapshots/capture
Headers: Authorization: mTLS (VM client cert)
{
  "device_id": "device-001",
  "label": "normal",
  "custom_label": "",
  "count": 5,
  "auto_capture": false
}
```

**Note**: The endpoint path `/api/v1/snapshots/capture` uses camera-specific naming for historical reasons, but the operation is device-agnostic and handles data unit capture requests for all device types (cameras, sensors, audio devices, etc.).

Edge receives request and:
- validates VM client certificate (mTLS)
- stores pending data unit capture request in meta-storage
- transitions device ML lifecycle to **AwaitingDataset**
- publishes `data_unit.requested` event to event bus

State Manager orchestrates:
- creation of "pending data unit capture request(s)" for that device
- capture/collection of raw data units through IoT device capabilities (images from cameras, sensor readings, audio samples, etc.)
- labeling workflow exposed through web UI (`web-gateway`)

**Production requirement**: Edge must handle multiple concurrent data unit capture requests per device gracefully:
- **Strategy**: queue requests; process one at a time per device.
- If a request arrives while another is active for the same device: enqueue it and respond with HTTP 202 Accepted.
- If queue is full (configurable, default 10 per device): reject with HTTP 429 Too Many Requests.

### 5) User collects and labels dataset via web

User actions are mediated through web/web-gateway, but State Manager is the workflow owner:
- **Validate labels** and enforce constraints:
  - no missing or empty labels
  - label must match VM-requested label or be from approved label taxonomy
  - minimum examples per label: configurable (default: 5)
  - maximum data unit size: configurable (default: 10 MB per image, 1 MB per sensor reading)
- store dataset metadata in meta storage, and raw units in object storage
- once requirements are satisfied, transition to **DatasetReadyLocal**

### 6) Edge uploads dataset to VM

State Manager packages the dataset, uploads it, and obtains a VM acknowledgement:
- transition to **DatasetUploadInProgress**
- on success, persist `DatasetID`, VM ack, and transition to **DatasetUploaded**
- optionally transition to **TrainingPending** if VM confirms training queueing

### 7) VM trains and deploys per-device model

VM produces `ModelID` for the device and initiates deployment:
- Edge receives deployment notification (push) or discovers it during reconciliation (pull)
- **Concurrent deployment handling**: if a model deployment arrives while inference is active:
  - complete current inference batch (max 10s wait)
  - gracefully stop inference loop
  - proceed with model verification and activation
  - if deployment arrives while another deployment is in progress: reject with error (one deployment per device at a time)
- Edge **verifies** the model and stores it:
  - object storage for artifacts
  - meta storage for deployment metadata, compatibility, and integrity evidence
- transition to **ModelStored**

### 8) Edge starts inference and emits security events

For each device with `ModelStored` and an active IoT data stream:
- State Manager transitions to **InferenceActive**
- Inference consumes **raw device data**, applies the device’s model, produces:
  - security events (metadata + attachments)
  - operational stats/health

**Device-agnostic integration target** (from `AI_GATEWAY_DEVICE_AGNOSTIC_REVIEW.md`):
- inference should be implemented as an IoT `DataProcessor` for `DeviceDataTypeVideoFrame` (or equivalent),
  with State Manager coordinating model binding and event forwarding.

---

## Startup reconciliation loop (required for production)

Events are not enough. On startup and periodically (e.g., every N minutes), State Manager runs:

### A) Reconcile VM connectivity
- Read current VM connection state from `vm-gateway`.
- Verify tunnel, transport, and authentication state are consistent.
- If not authenticated:
  - remain in offline-capable mode; do not block local inference
  - retry authentication if tunnel/transport are connected but auth failed (exponential backoff: 10s, 20s, 40s, max 5min)
  - do not attempt device sync or dataset upload until authenticated
- If authenticated but not `capabilities_received`:
  - wait for VM to push capabilities with timeout (max 30s after authentication)
  - if timeout: log error, retry authentication
  - do not proceed with device sync until capabilities received
- **Capability validation**: if Edge receives capabilities it cannot support:
  - log warning with unsupported capability names
  - continue with supported capabilities only
  - report unsupported capabilities back to VM in next device sync

### B) Reconcile device inventory
- Query IoT registry for all registered devices + connectivity/capabilities.
- For each device, ensure a persisted ML lifecycle record exists (create if missing, initial state: `Unassigned`).
- If VM connection is `capabilities_received`:
  - sync discovered devices to VM (call `VMGateway.SyncDevices`)
  - process VM's assignment response and update ML lifecycle states
  - handle assignment changes:
    - **newly assigned** → transition to `Assigned`, start dataset workflow
    - **unassigned** (was assigned, now not) → transition to `Unassigned`, stop inference, **keep local data** (datasets, models, events) for 30 days (configurable) in case of reassignment; mark for eventual cleanup

### C) Reconcile model availability and integrity
For each device:
- if ML lifecycle indicates a model should exist:
  - verify object storage presence
  - verify integrity (hash) and authenticity (signature)
  - verify runtime compatibility (target runtime version, device capability)
  - if verification fails: transition to **RecoveryRequired** (or **DegradedNoModel** if policy allows detection to stop but device can remain managed)

### D) Reconcile inference execution
For each device:
- if `ModelStored` and device data stream is available: ensure inference loop is running
- if inference should not run (unassigned, policy disabled, device disconnected): ensure inference loop is stopped

### E) Reconcile pending outgoing queues
- **Security events pending upload**: 
  - query meta storage for all events with status `pending_delivery` or `delivery_failed`
  - if VM connected: send immediately (no backoff delay on reconciliation)
  - prioritize oldest events first (FIFO)
  - batch up to 100 events per request (configurable)
- dataset uploads partially completed: resume or restart from last verified checkpoint

**Rule**: reconciliation actions must be **idempotent**, safe to run concurrently with event-driven triggers, and must serialize per device to avoid races.

**Reconciliation timeout and health**:
- Entire reconciliation pass must complete within **5 minutes** (configurable).
- If reconciliation exceeds timeout: log error, abort current pass, schedule next pass immediately.
- Track reconciliation health: consecutive failures >3 → emit `reconciliation.unhealthy` operational event and alert operator.

---

## Offline behavior (VM disconnection tolerance)

When VM connection transitions away from authenticated/capabilities_received:

- **Do not stop inference** for devices where:
  - device is connected locally, and
  - a verified model is available, and
  - policy allows offline detection (policy stored in per-device ML lifecycle record; default: true; set by VM in assignment response field `offline_inference_allowed`)

- **Do stop / pause VM-dependent workflows**:
  - device sync to VM
  - dataset upload
  - model fetch (unless previously cached)
  - event delivery to VM (but **continue local event generation + persistence** - events are stored locally and queued for delivery)

### Security event storage and delivery (production-critical)

**Security events are detected by Edge/ai-service and must be handled as follows:**

#### 1) Immediate local persistence (always, regardless of VM connectivity)

When a security event is detected:
- **Persist immediately to Edge object storage**:
  - event metadata (EventID, DeviceID, event_type, timestamp, confidence, ModelID) → meta storage
  - event attachments (images, sensor readings, audio samples) → object storage
  - store with status: `pending_delivery`
- **Do NOT wait for VM connectivity** before persisting
- **Do NOT block inference** while persisting (async write)

#### 2) Immediate delivery to VM (when connected)

When VM is connected (`authenticated` + `capabilities_received`):
- **Send events ASAP** (as soon as possible):
  - no batching delay (send immediately, or batch up to 100 events if multiple pending)
  - no backoff on first attempt
  - retry with exponential backoff only on failure (1s, 2s, 4s, 8s, max 60s)
- **Delivery priority**: oldest events first (FIFO)
- **Batch size**: max 100 events per request (configurable)
- **After VM acknowledgment**: update event status to `delivered` in meta storage

#### 3) Immediate sync on VM reconnection

When VM connection transitions from `disconnected` → `authenticated` → `capabilities_received`:
- **Trigger immediate security event sync** (do not wait for next reconciliation):
  - query meta storage for all events with status `pending_delivery` or `delivery_failed`
  - send all unsynced events immediately (no delay)
  - process in batches of 100 events
  - continue until all pending events are sent or connection fails again
- **This is a high-priority operation**: security events must reach VM as soon as connectivity is restored

#### 4) Local queuing guarantees (offline operation)

When VM is disconnected:
- **Continue generating and storing events locally**
- **Queue events in meta storage** (status: `pending_delivery`)
- **Queue limit**: max 10,000 events (configurable)
- **If queue full**: 
  - drop oldest events (FIFO) with operator alert
  - OR: stop new event generation (if policy requires all events to be delivered)
  - policy configurable: `drop_oldest_on_overflow` (default: true)

#### 5) At-least-once delivery guarantee

Security event delivery is **at-least-once**:
- Edge persists events locally **before** attempting delivery
- Edge retries until VM ack is received
- VM must deduplicate by `(EdgeID, EventID)` tuple
- If VM receives duplicate event: respond with `"duplicate": ["evt-001"]` in response

#### 6) Backpressure handling

If queue/storage limits are approached:
- **80-90% full**: emit warning, continue normal operation
- **90-95% full**: throttle event attachments (smaller images, reduced data samples)
- **>95% full**: 
  - stop new event attachments (metadata-only events)
  - enforce retention policies (oldest-first) **only if policy permits**
  - emit "storage pressure" critical alert

---

## Storage integrity and corruption recovery (VM-assisted restart)

### Integrity evidence (what must be stored)

For each dataset and model artifact stored locally, persist:
- content hash (e.g., SHA-256) of each object
- signed manifest reference (see below)
- creation time, source VM identity, and audit record

### Detecting corruption

Corruption is detected via:
- missing objects referenced by meta storage
- hash mismatches
- signature validation failures
- meta/object storage internal health checks returning “unhealthy”

### RecoveryRequired behavior

When corruption is detected for a device:
- transition ML lifecycle to **RecoveryRequired**
- stop inference for that device (cannot safely execute)
- keep device management active (discovery/registration continues)
- if VM is reachable:
  - request **resync** for that device (Edge → VM API call):
    ```
    POST https://{vm_endpoint}/api/v1/devices/{deviceID}/resync
    {
      "edge_id": "edge-001",
      "device_id": "device-001",
      "recovery_reason": "storage_corruption",
      "corrupted_resources": ["model", "dataset"],
      "last_known_good_state": {
        "model_id": "model-xyz789",
        "dataset_id": "ds-abc123"
      }
    }
    ```
  - VM responds with:
    - latest assignment + policy
    - model artifact download URL or pushes model directly
    - (optional) dataset reference for re-collection
- if VM is not reachable:
  - remain in RecoveryRequired and expose degraded status via health snapshot + UI

### Full-storage corruption scenario

If meta storage is corrupted globally (cannot load ML lifecycle records):
- Edge enters a **global RecoveryRequired** mode:
  - inference is disabled until trust is restored
  - Edge requests a full resync from VM:
    - device list assigned to this Edge
    - model inventory per device + manifests
  - Edge rebuilds local metadata from VM responses

**Production expectation**: VM-assisted resync is a first-class protocol, not a manual "wipe and reinstall".

**Resync audit trail**: all resync requests must be logged to audit-log with:
- EdgeID, DeviceID, timestamp
- recovery reason (storage_corruption, integrity_failure, operator_initiated)
- corrupted resources list
- VM response status

---

## Dataset packaging & upload (production requirements)

### Dataset manifest (must be deterministic)

Dataset upload (via `VMGateway.SyncDataUnits`) must include data units with:
- DeviceID, EdgeID
- DataUnitID (unique per data unit)
- object_key (path in object storage)
- raw_data_format (e.g., "jpeg", "png", "json", "wav")
- label, custom_label, description
- metadata (timestamps, source device properties, device type)
- created_at timestamp

**DatasetID** should be derived from the canonical data unit collection hash to guarantee idempotency:
- repeated uploads of the same dataset produce the same DatasetID

**Note**: `SyncDataUnits` is device-agnostic and supports:
- Images (JPEG, PNG) from video devices (cameras)
- Sensor readings (JSON) from sensors
- Audio samples (WAV, MP3) from audio devices
- Any other labeled data unit format

### Transport security

- Upload must occur over the secure channel established by `vm-gateway` (tunnel + mTLS transport).
- Implement resumable upload semantics (chunked, with per-chunk hashes).
- VM acknowledgement must include:
  - DatasetID
  - received manifest hash
  - server-side integrity verification result

---

## Model intake, verification, and activation (production requirements)

### Model artifact envelope

Every model deployment must include:
- ModelID, DeviceID, target runtime (e.g., OpenVINO version), preprocessing config
- signed model manifest:
  - artifact list + hashes
  - compatibility constraints
  - training provenance (dataset references, VM job id)
  - signature by VM model-signing identity (offline root / HSM-backed recommended)

### Verification gates (must all pass)

Before activation:
- **authenticity**: verify VM signature chain (pinned root)
- **integrity**: verify hashes for all artifacts
- **compatibility**: verify runtime versions and device capabilities match
- **policy**: verify device is assigned to this Edge; verify rollout policy (allow/deny)

If any gate fails: do not activate; transition to **RecoveryRequired** (or a policy-defined quarantine state).

### Atomic activation

Activation must be atomic per device:
- store artifacts first
- store metadata and mark `ModelStored`
- only then start inference loop and mark `InferenceActive`

If activation fails mid-way, system must roll back to a consistent state and retry safely.

---

## Inference and event generation (device-agnostic target)

### Device-agnostic ingestion

Per `AI_GATEWAY_DEVICE_AGNOSTIC_REVIEW.md`, inference should not be hard-coupled to CCTV:
- consume `iot` device data (e.g., video frames) via IoT streaming APIs
- use capabilities to validate eligibility (video streaming/capture)

### Event persistence contract

For each security event detected by Edge/ai-service:
- **Immediate persistence** (always, regardless of VM connectivity):
  - persist metadata (EventID, DeviceID, DeviceType, event_type, confidence, ModelID, model_version, timestamps) → meta storage
  - persist attachments (data units, e.g., images, sensor readings, audio samples) → object storage
  - store with status: `pending_delivery`
- **Enqueue for VM delivery** with idempotency key (EdgeID, EventID)

### Event delivery contract

- **Immediate delivery when VM connected**: send events ASAP (no batching delay)
- **Immediate sync on reconnection**: when VM reconnects, send all unsynced events immediately
- **At-least-once delivery** from Edge to VM
- VM must deduplicate by `(EdgeID, EventID)` tuple and support ordered processing per device when needed
- Edge must record VM ack and mark event as `delivered` in meta storage; then apply retention policy
- **Retention after delivery**: keep events locally for 7 days after VM ack (configurable) for audit/recovery

---

## Reliability patterns (mandatory)

- **Idempotency**:
  - upserts for device sync, dataset upload, and model storage
  - all VM-facing requests include idempotency keys (EdgeID + stable operation id)
  - VM→Edge commands (capabilities, data unit capture requests, model deployments) must be idempotent (duplicate delivery safe)
- **Serialization per device**:
  - only one workflow for a given device executes at a time
  - allow cross-device parallelism with bounded concurrency
- **Backoff + jitter** for retries; circuit breaking on repeated failures
- **Out-of-order handling**:
  - model deployment may arrive before device is ready; must be stored as pending and reconciled later
  - capabilities may arrive before devices are discovered; store and process when devices are ready
  - data unit capture requests may arrive before device is synced; queue and process when device becomes available
- **Timeouts**: all external calls bounded; no indefinite blocking
  - tunnel establishment: 30s
  - transport establishment: 30s
  - authentication: 30s
  - VM→Edge command processing: 10s (Edge must respond quickly to VM commands)
- **Crash consistency**:
  - persistent state updated in a way that always recovers to a coherent workflow checkpoint
- **Bidirectional connection health**:
  - Edge must monitor both tunnel (Edge→VM) and HTTPS server (VM→Edge) health
  - If HTTPS server fails, Edge cannot receive VM commands; must alert operator
  - Tunnel failures must trigger transport reconnection; transport failures must trigger auth retry

---

## Security requirements (production)

### Identity, authentication, authorization

- Edge↔VM communication must use **mTLS** with:
  - certificate pinning to the VM CA root (Edge trusts single VM CA; VM CA cert fingerprint in config)
  - **certificate rotation strategy**:
    - VM signals upcoming rotation via capabilities sync (field: `cert_rotation_scheduled_at`)
    - Edge downloads new CA cert from VM before rotation (via authenticated endpoint)
    - Edge validates new CA cert (must be signed by current CA or root CA)
    - Edge updates trust store atomically (old CA remains trusted for 7 days grace period)
    - during grace period: Edge accepts either old or new VM cert
    - after grace period: Edge removes old CA from trust store
  - **certificate revocation**: Edge checks CRL or OCSP (configurable) on every auth; cache for 1 hour
- Every VM request must be authorized:
  - VM can only assign/request datasets/models for devices within its tenant/policy scope
  - Edge must only accept assignments from the authenticated VM identity
- **Time synchronization requirement**:
  - Edge clock must be synchronized with VM within ±5 minutes tolerance (configurable)
  - if clock drift detected (via mTLS handshake time comparison): emit warning, continue operation
  - if drift >30 minutes: fail authentication; emit critical alert; operator must fix NTP config

### Model supply-chain security

- Models must be signed by a VM-controlled signing identity (HSM-backed recommended).
- Edge must verify signature before execution.
- Store model manifests and verification results for audit.

### Data confidentiality & privacy

- Datasets and event attachments may contain sensitive imagery/audio:
  - encrypt at rest in object storage (per-object keys; KMS or hardware-backed keys preferred)
  - enforce least-privilege access controls for web UI and internal services
  - implement retention classes and secure deletion policy

### Tamper detection and auditability

- Persist audit logs for:
  - dataset creation, labeling, upload
  - model deployment, verification, activation
  - security event generation and delivery
- Audit records must be tamper-evident (append-only store or hash-chained logs recommended).

### Operational security

- Rate limit inbound VM commands to prevent resource exhaustion.
- Strictly validate all inputs (manifests, metadata fields).
- Do not log sensitive payloads (images/labels) in plaintext logs.

---

## Observability & operations

State Manager must expose a health snapshot (directly or via aggregated system health) including:
- VM connection state (from `vm-gateway`)
- device counts by state (from `iot`)
- ML lifecycle counts by state (assigned, dataset ready, model stored, inference active, recovery required)
- queue depth and oldest pending event age
- storage health (healthy/warning/full) and integrity error counters
- last successful sync timestamps (devices, datasets, models)

Additionally:
- metrics for SLOs: inference latency, event rate, upload throughput, retry counts
- structured logs with correlation ids per workflow and per device

---

## Concurrency and locking strategy (production critical)

### Per-device workflow serialization

State Manager must enforce that **only one workflow operation executes per device at any time**:
- implement a per-device mutex/lock map or a per-device work queue
- cross-device operations (e.g., sync all devices to VM) can execute in parallel with bounded concurrency (e.g., semaphore with N=10)

**Rationale**: prevents race conditions like "upload dataset while model is activating" or "reconcile model integrity while processing inference failure".

### Event handler idempotency

Every event handler must be **fully idempotent**:
- event bus may deliver duplicates (at-least-once semantics)
- reconciliation may run concurrently with event-driven triggers
- implementation: check current state before acting; use CAS (compare-and-swap) or versioned updates in meta storage

### Reconciliation concurrency

Reconciliation loops (startup, periodic, reconnection) must:
- serialize with respect to each other globally (only one reconciliation pass at a time)
- serialize per device with event-driven workflows (acquire device lock before acting):
  - lock acquisition timeout: 30s (configurable)
  - if lock cannot be acquired: skip this device in current pass, retry in next reconciliation
  - log warning if device lock acquisition fails >3 consecutive times
- use a "reconciliation generation ID" (incrementing counter or UUID) in logs for debugging interleaved operations

### Timeout and cancellation

All operations must have **explicit timeouts**:
- VM API calls: 30s default, configurable (per-request timeout)
- AI inference: 10s default, configurable (per-inference-request timeout, not per-frame if batched)
- storage operations: 5s default (read/write single object)
- data unit capture: 60s default (device-specific; cameras may take longer than sensors)
- model verification: 120s default (includes signature and hash checks for large models)
- use context.WithTimeout; propagate cancellation correctly
- **Graceful shutdown**: on SIGTERM, State Manager has 60s to:
  - stop accepting new workflows
  - complete in-flight inference batches (or abort after 10s)
  - flush security event queue to storage (at-least-once guarantee)
  - persist all ML lifecycle state
  - close all service connections

---

## Resource limits and backpressure (prevent runaway consumption)

### Bounded queues

- Security event queue: max 10,000 events (configurable); drop oldest on overflow with operator alert
- Dataset upload queue: max 100 concurrent uploads; block new dataset completion until slot available
- Inference loop concurrency: max N devices processing simultaneously (N = CPU cores or config)

### Storage quotas

- Object storage retention:
  - raw datasets: 30 days default after upload (configurable)
  - security event attachments: 7 days default after VM ack (configurable)
  - models: keep last 2 versions per device; purge older after grace period (7 days default)
    - **model version tracking**: ModelID includes version number; VM decides when to bump version
    - **model rollback**: if new model fails verification or causes inference errors >50% for 1 hour, State Manager may request VM to re-send previous model version (if still within retention period)
  - unassigned device data: 30 days after device unassignment
- Meta storage: enforce max records per bucket with cleanup/archival
- **Gradual backpressure between warning and full thresholds**:
  - 80-90%: emit warnings; continue normal operation
  - 90-95%: slow down data collection (reduce capture rate by 50%)
  - >95%: stop new captures; metadata-only events; trigger aggressive cleanup

### Rate limiting

- VM API calls: max 100 req/min per endpoint class (device sync, dataset upload, event delivery)
- Inference requests to AI service: max 1 req/sec per device (configurable)
- Web UI API: max 1000 req/min per client IP (configurable)

**Failure behavior**: when limits are hit, return backpressure errors with retry-after hints; log and alert on sustained rate limit hits.

---

## Detailed failure scenarios and recovery procedures

### Scenario 1: Model verification fails (bad signature)

**Detection**: Edge receives model deployment; signature validation fails.

**Automated response**:
- do **not** activate model
- transition device to **RecoveryRequired**
- log audit event with failure details (signature mismatch, invalid cert chain)
- send alert to operator dashboard

**Operator action**:
- investigate VM model-signing process (compromised key? bad deploy?)
- if VM issue: VM re-signs and re-deploys
- if Edge issue (corrupted CA cert): operator updates Edge trust root and retries

### Scenario 2: Device disconnects mid-inference

**Detection**: IoT device state transitions to `disconnected` while inference is `InferenceActive`.

**Automated response**:
- gracefully stop inference loop for that device
- transition ML lifecycle to `ModelStored` (model is still valid, device is just offline)
- do **not** delete model or mark as corrupted
- when device reconnects: reconciliation re-starts inference automatically

### Scenario 3: AI service unreachable (network or crash)

**Detection**: repeated AI service HTTP timeouts or connection refused.

**Automated response**:
- circuit breaker opens after N consecutive failures (N=5 default)
- stop inference for all devices temporarily
- log operational alert
- retry with exponential backoff (1s, 2s, 4s, ..., max 60s)
- when AI service recovers: circuit breaker closes; reconciliation re-starts inference

**Operator action**:
- check AI service logs and health
- restart AI service if needed
- if persistent: escalate to AI service team

### Scenario 4: Storage full (object or meta storage)

**Detection**: storage write returns "quota exceeded" or "disk full".

**Automated response**:
- transition to `storage.full` operational state
- stop new dataset collection
- stop new security event attachments (continue metadata-only events)
- trigger retention cleanup immediately (purge expired objects)
- alert operator with critical severity

**Operator action**:
- check storage usage dashboard
- extend storage capacity or adjust retention policies
- manually purge old datasets/events if policy allows

### Scenario 5: VM connection lost for extended period (>1 hour)

**Detection**: VM connection state is `disconnected` or `tunnel_connection_error` for >1 hour.

**Automated response**:
- continue local inference (if models exist and policy allows offline operation)
- queue security events locally (up to queue limit)
- do **not** attempt dataset uploads or device sync
- retry tunnel/transport connection with exponential backoff (max interval: 5 minutes)
- emit periodic "VM unreachable" operational alerts
- **Do not restart authentication loop** until tunnel/transport are re-established

**Operator action**:
- check VM network reachability
- check tunnel/transport logs for failure reason
- verify WireGuard configuration and VM endpoint
- if VM is down: wait for VM recovery
- if Edge network issue: diagnose and fix
- if tunnel config changed: update Edge config and restart

**Production note**: Edge must distinguish between:
- **Tunnel failure** (WireGuard down): retry tunnel establishment
- **Transport failure** (HTTPS connection refused): retry after tunnel is up
- **Authentication failure** (VM rejects Edge cert): alert operator (cert may be revoked)

### Scenario 6: Partial dataset upload failure (network interruption)

**Detection**: dataset upload HTTP request fails mid-transfer (Edge → VM via HTTPS client).

**Automated response**:
- mark upload as `DatasetUploadInProgress` with last successful chunk offset and per-chunk checksums
- **chunk-level resumption**: VM responds with list of successfully received chunks (by chunk index and checksum)
- retry upload from last checkpoint (send only missing/failed chunks)
- exponential backoff between retries (1s, 2s, 4s, 8s, 16s, max 60s)
- **Verify tunnel/transport are still connected** before retry (if disconnected, wait for reconnection)
- after max retries (10 default): transition to error state and alert operator
- **chunk size**: 1 MB default (configurable); each chunk has SHA-256 checksum in upload manifest

**Operator action**:
- check network stability
- check VM endpoint reachability
- if persistent: manually trigger re-upload or re-collect dataset

### Scenario 7: Out-of-order model deployment (model arrives before device is ready)

**Detection**: model deployment notification arrives but device is in `undiscovered` or `unassigned` state.

**Automated response**:
- store model deployment as **pending** in a separate registry (in meta storage, bucket: `pending_model_deployments`)
- do **not** activate or fail
- **pending deployment TTL**: 24 hours (configurable); after TTL, discard pending deployment and log warning
- when device becomes `assigned` during reconciliation: match pending model by DeviceID and activate
- if multiple pending deployments for same device: use latest (by timestamp); discard others

### Scenario 8: Corrupted meta storage database

**Detection**: BoltDB (or equivalent) returns "corrupted page" or fails integrity check.

**Automated response**:
- enter global `RecoveryRequired` mode
- stop all inference
- stop all VM-facing operations (except resync protocol)
- request full resync from VM (device list + model inventory)
- rebuild meta storage from VM responses

**Operator action**:
- backup corrupted database for forensics
- approve VM-assisted resync
- if VM unavailable: restore from last known good backup (if available)

---

## VM protocol contracts (request/response semantics)

### Device inventory sync (Edge → VM)

**Request** (Edge calls `VMGateway.SyncDevices`):
```
POST https://{vm_endpoint}/api/v1/devices/sync
Headers: Authorization: Bearer {edge_token} (or mTLS client cert)
{
  "devices": [
    {
      "device_id": "cam-001",
      "device_type": "camera",
      "capabilities": ["video_streaming", "data_unit_capture"],
      "metadata": { "model": "Hikvision DS-2CD2", "firmware": "5.7.3" },
      "state": "connected"
    }
  ],
  "sync_timestamp": "2025-12-28T12:00:00Z"
}
```

**Response** (idempotent):
```
{
  "assignments": {
    "cam-001": {
      "assigned": true,
      "policy": "monitor_24x7",
      "dataset_required": true
    }
  },
  "sync_ack_timestamp": "2025-12-28T12:00:05Z"
}
```

**Semantics**: 
- VM returns current assignment for all devices in request. Edge must treat response as authoritative.
- Edge must call this **after** `capabilities_received` state is reached.
- Edge should call this on startup reconciliation and when new devices are discovered.
- VM may change assignments at any time; Edge must handle assignment updates gracefully.
- **Idempotency**: Edge includes `sync_id` (UUID) in request; VM deduplicates by (EdgeID, sync_id) tuple.
- **Partial failure**: if some devices in request are invalid, VM returns partial success with error details per device.

### Data unit sync (Edge → VM)

**Request** (Edge calls `VMGateway.SyncDataUnits`):
```
POST https://{vm_endpoint}/api/v1/data-units/sync
Headers: Authorization: Bearer {edge_token} (or mTLS client cert)
{
  "edge_id": "edge-001",
  "device_id": "device-001",
  "data_units": [
    {
      "data_unit_id": "unit-001",
      "device_id": "device-001",
      "object_key": "edge-storage/datasets/device-001/unit-001.jpg",
      "raw_data": "base64...",  // Optional: inline data
      "raw_data_format": "jpeg",  // "jpeg", "png", "json", "wav", etc.
      "label": "normal",
      "custom_label": "test-label",
      "description": "Training data unit",
      "metadata": {
        "timestamp": "2025-12-28T12:00:00Z",
        "device_type": "camera"
      },
      "created_at": 1703764800
    }
  ]
}
```

**Response**:
```
{
  "success": true,
  "message": "Data units synced successfully"
}
```

**Semantics**:
- Edge sends labeled data units (images, sensor readings, audio samples, etc.) to VM
- VM acknowledges receipt and queues training if dataset is complete
- Device-agnostic: supports all IoT device types and data formats
- **Idempotency**: data units with same `data_unit_id` are deduplicated by VM (idempotent upsert)
- **Batching**: Edge may send multiple data units in single request (max 100 per request, configurable)
- **Dataset completeness**: VM determines when dataset is complete based on assignment policy (e.g., "need 50 labeled units per label")

### Model deployment notification (VM → Edge, push)

**VM posts to Edge HTTPS server** (VM initiates):
```
POST https://{edge_https_server}/api/v1/models/deploy
Headers: Authorization: mTLS (VM client cert)
Content-Type: multipart/form-data

Form fields:
- metadata: JSON {
    "model_id": "model-xyz789",
    "device_id": "cam-001",
    "version": "1.0",
    "model_type": "yolov8n",
    "framework": "pytorch",
    "input_shape": [1, 3, 640, 640],
    "preprocessing": { ... },
    "signature": "base64..."
  }
- model: binary file (model.onnx or model.tar.gz)
```

**Edge response**:
```
{
  "success": true,
  "model_file_path": "edge-storage/models/model-xyz789.onnx",
  "deployment_id": "deploy-abc123"
}
```

**Edge acknowledgement to VM** (Edge → VM, via HTTPS client):
```
POST https://{vm_endpoint}/api/v1/models/{modelID}/status
{
  "deployment_id": "deploy-abc123",
  "status": "deployed" | "verification_failed" | "activation_failed",
  "timestamp": "2025-12-28T13:00:00Z",
  "error": "signature verification failed" (if failed)
}
```

**Production requirements**:
- Edge HTTPS server must validate VM client certificate (mTLS)
- Edge must verify model signature before storing
- Edge must acknowledge deployment status to VM (success or failure)
- If verification fails, Edge must send error details to VM for operator visibility

### Security event delivery (Edge → VM)

**Request** (batched, Edge → VM via HTTPS client):
```
POST https://{vm_endpoint}/api/v1/events/batch
Headers: Authorization: Bearer {edge_token} (or mTLS client cert)
{
  "edge_id": "edge-001",
  "events": [
    {
      "event_id": "evt-001",
      "device_id": "device-001",
      "device_type": "camera",
      "event_type": "person_detected",
      "timestamp": "2025-12-28T12:30:00Z",
      "confidence": 0.95,
      "model_id": "model-xyz789",
      "model_version": "1.0",
      "attachment_url": "edge-local://events/evt-001.jpg",  // Optional: if VM needs to fetch
      "attachment_inline": "base64..."  // Optional: inline attachment data
    }
  ]
}
```

**Response** (idempotent acks):
```
{
  "acknowledged": ["evt-001"],
  "failed": [],
  "duplicate": ["evt-002"]  // Already received previously
}
```

**Production requirements**:
- **Immediate delivery**: Edge sends events ASAP when VM is connected (no batching delay)
- **Immediate sync on reconnection**: when VM reconnects, Edge sends all unsynced events immediately (triggered by connection state transition, not waiting for reconciliation)
- **Idempotency**: VM deduplicates by (EdgeID, EventID) tuple
- **Batch size**: max 100 events per request (configurable)
- **Retry strategy**: exponential backoff on failure (1s, 2s, 4s, 8s, max 60s)
- **Priority**: oldest events first (FIFO)
- **Ordering**: VM does not guarantee ordered processing across devices; within-device ordering is best-effort
- **Attachment handling**: 
  - Option A: `attachment_url` with `edge-local://` scheme - VM fetches from Edge object storage via authenticated endpoint
  - Option B: `attachment_inline` - Edge includes base64-encoded attachment in request (for small attachments <1MB)
  - Edge chooses based on attachment size and policy

---

## Operator dashboards and runbooks

### Dashboard: ML lifecycle overview

Display per-device:
- DeviceID, type, capabilities
- ML lifecycle state (assigned, awaiting dataset, model stored, inference active, recovery required)
- last dataset upload timestamp and status
- current model version and deployment timestamp
- inference stats (events/hour, avg latency, error rate)
- health status (healthy / degraded / error)

### Dashboard: Resource usage

- Storage: used/available (meta and object storage)
- Queue depths: security events, dataset uploads
- Rate limit utilization: VM API, AI service, web UI
- Circuit breaker states: AI service, VM connectivity

### Runbook: Add new device

1. Ensure device is physically connected and powered
2. IoT service auto-discovers device (check logs)
3. Wait for VM sync (automatic on next reconciliation)
4. VM assigns device (visible in dashboard)
5. VM requests dataset (operator receives notification)
6. Operator captures and labels dataset via web UI
7. Edge uploads dataset automatically
8. VM trains model and deploys (automatic)
9. Edge activates inference (automatic)
10. Monitor dashboard for first security events

### Runbook: Recovery from storage corruption

1. Operator observes "RecoveryRequired" alert
2. Operator checks storage health via dashboard
3. If meta storage corrupted:
   - backup current state (for forensics)
   - approve VM-assisted resync via UI button
   - VM sends device assignments + model inventory
   - Edge rebuilds meta storage
   - inference resumes automatically
4. If object storage corrupted:
   - identify affected devices
   - request model re-deployment from VM per device
   - re-collect datasets if needed

### Runbook: Replace AI service

1. Deploy new AI service (different URL or version)
2. Update Edge config with new AI service URL
3. Restart Edge orchestrator (graceful)
4. Edge re-starts inference with new service
5. Monitor for inference errors or latency changes

---

## Appendix A: Event taxonomy (complete)

### VM Gateway events (connection state)

- `network.tunnel.connecting`: tunnel establishment initiated
- `network.tunnel.connected`: tunnel ready
- `network.tunnel.disconnected`: tunnel lost
- `network.tunnel.connection_error`: tunnel failure (with error details)
- `network.transport.connecting`: transport (HTTPS) handshake initiated
- `network.transport.connected`: transport ready
- `network.transport.disconnected`: transport lost
- `network.transport.connection_error`: transport failure
- `edge.authenticated`: Edge authenticated to VM (mTLS verified)
- `edge.capabilities_received`: VM acknowledged Edge capabilities

### IoT events (device lifecycle)

- `device.discovered`: new device found by plugin (CCTV/sensor/etc.)
- `device.registered`: device added to registry and assigned stable DeviceID
- `device.connected`: device became reachable (RTSP connected, sensor online, etc.)
- `device.disconnected`: device lost connectivity
- `device.capability_changed`: device capabilities updated (e.g., PTZ enabled)
- `device.error`: device-specific error (auth failed, timeout, etc.)

### IoT events (device data)

- `raw_device_data.frame_received`: video frame captured and stored
- `raw_device_data.clip_recorded`: video clip saved (video device specific; for other device types, use `raw_device_data.frame_received` or equivalent)
- `sensor_data.reading_received`: sensor reading captured (future)
- `audio_data.sample_received`: audio sample captured (future)

### Dataset workflow events

- `data_unit.requested`: VM requested dataset capture for device
- `data_unit.captured`: data unit (image, sensor reading, audio sample, etc.) captured and awaiting label
- `data_unit.labeled`: user labeled a unit via web UI
- `data_unit.saved`: labeled unit persisted to storage
- `data_unit.set_ready`: dataset complete and ready for upload
- `dataset.upload_started`: upload to VM initiated
- `dataset.upload_progress`: upload chunk completed
- `dataset.upload_completed`: upload fully acknowledged by VM
- `dataset.upload_failed`: upload failed after retries

### Model workflow events

- `model.deployment_received`: VM signaled model availability
- `model.verification_started`: Edge started model verification
- `model.verification_completed`: verification passed
- `model.verification_failed`: verification failed (signature/hash/compatibility)
- `model.stored`: model artifacts saved to storage
- `model.activation_started`: inference loop starting
- `model.activation_completed`: inference loop active
- `model.activation_failed`: inference loop failed to start
- `model.deactivated`: inference loop stopped (device offline, policy change, etc.)

### AI inference events

- `ai.inference_started`: inference request sent to AI service
- `ai.inference_completed`: inference response received
- `ai.inference_failed`: inference request failed (timeout, error response)
- `ai.detection`: security event detected (published to event bus for workflow use)
- `ai.circuit_breaker_opened`: AI service failures exceeded threshold
- `ai.circuit_breaker_closed`: AI service recovered

### Storage events

- `storage.warning`: storage usage >80% (configurable)
- `storage.full`: storage quota exceeded
- `storage.cleanup_started`: retention cleanup initiated
- `storage.cleanup_completed`: cleanup finished
- `storage.corruption_detected`: integrity check failed

### Operational events

- `system.startup`: Edge orchestrator started
- `system.shutdown`: Edge orchestrator stopping
- `system.reconciliation_started`: reconciliation loop initiated
- `system.reconciliation_completed`: reconciliation finished
- `health.degraded`: system entered degraded mode (AI service down, storage pressure, etc.)
- `health.recovered`: system recovered from degraded mode

---

## Appendix B: Metrics and SLOs

### Key metrics (must be instrumented)

**Availability**:
- VM connection uptime %
- Device connectivity uptime % (per device)
- Inference availability % (per device)

**Latency**:
- P50/P95/P99 inference latency (AI service response time)
- P50/P95/P99 dataset upload duration
- P50/P95/P99 model deployment end-to-end time (VM signal → inference active)

**Throughput**:
- Security events generated/sec (per device, aggregate)
- Dataset uploads completed/hour
- Model deployments completed/hour

**Error rates**:
- Inference failure rate % (per device)
- Dataset upload failure rate %
- Model verification failure rate %
- Storage corruption incidents/month

**Resource utilization**:
- Object storage usage % (current/quota)
- Meta storage usage % (current/quota)
- Security event queue depth (current/max)
- CPU usage % (inference load)

### Suggested SLOs (production targets)

- **VM connectivity**: 99.5% uptime (allows ~3.6 hours/month downtime)
- **Device connectivity**: 99.0% uptime per device (assuming reliable network)
- **Inference availability**: 99.9% (when device + VM are both up)
- **Inference latency**: P95 <5s, P99 <10s
- **Dataset upload success**: 99% (1% allowed for transient failures with retry)
- **Model deployment end-to-end**: P95 <10 minutes (VM training excluded)
- **Storage corruption**: <1 incident/year per Edge

### Alerting thresholds

- **Critical**:
  - Storage >95% full
  - Storage corruption detected
  - VM disconnected >4 hours
  - AI service down >30 minutes
  - Model verification failures (any occurrence)
- **Warning**:
  - Storage >80% full
  - VM disconnected >1 hour
  - Inference latency P95 >10s
  - Dataset upload failures >5% over 1 hour
  - Device connectivity <95% over 24 hours

---

## Appendix C: Testing and validation strategy

### Unit tests (per component)

- `state-mng`: workflow state transitions, idempotency, locking
- `vm-gateway`: connection state machine, retry logic
- `iot`: device registry, plugin system, data pipeline
- `ai-gateway`: inference request/response handling, circuit breaker

### Integration tests (cross-component)

- Full device lifecycle: discover → assign → dataset → model → inference
- Offline/reconnection: disconnect VM mid-workflow, verify queue/resume
- Storage corruption: inject corruption, verify recovery protocol
- Out-of-order events: send model before device registered, verify pending storage

### End-to-end tests (full system)

- Deploy Edge + VM (local Docker Compose)
- Add mock camera (RTSP simulator)
- Trigger full workflow via VM API
- Verify security events reach VM
- Verify metrics/logs are correct

### Chaos testing (production readiness)

- Kill AI service randomly, verify circuit breaker
- Disconnect VM for random intervals, verify queue behavior
- Fill storage, verify backpressure and cleanup
- Corrupt database files, verify recovery

### Performance tests (load and scale)

- 10 devices × 1 inference/sec for 1 hour: verify latency/throughput
- 1000 queued security events during VM downtime: verify upload after reconnect
- 100 MB dataset upload: verify resumable upload and integrity

---

## Appendix D: Deployment and migration strategy

### Version compatibility matrix

| Edge Version | VM Version | AI Service Version | Protocol Version |
|--------------|------------|-------------------|------------------|
| 1.0.x        | 1.0.x      | 1.0.x             | v1               |
| 1.1.x        | 1.1.x      | 1.1.x             | v2               |

**Rules**:
- Edge must support **protocol version negotiation** with VM (advertise supported versions in capabilities sync)
- Model manifest must include `protocol_version` and `schema_version` fields
- Edge must reject models with unsupported protocol versions (fail verification gate)
- All types use device-agnostic terminology (`DeviceID`, not `CameraID`)

### Rolling upgrade procedure

**For VM upgrades** (no Edge restart required):
1. VM advertises new protocol version
2. Edge continues using current protocol if compatible
3. Operator optionally upgrades Edge later to use new features

**For Edge upgrades** (requires restart):
1. Graceful shutdown: finish in-flight workflows (max 60s timeout)
2. Persist all ML lifecycle state and queue positions
3. Start new Edge version
4. Startup reconciliation rebuilds runtime state from persisted records
5. VM sync re-establishes assignments
6. Inference resumes automatically for devices with models

### Device-agnostic implementation requirements

All types and APIs must use device-agnostic terminology:
- Use `DeviceID` (not `CameraID`) for all device identifiers
- Security events must include `DeviceID`, `DeviceType`, `CapabilityContext`, `ModelID`
- All VM protocol messages use device-agnostic field names
- Event model supports all device types (cameras, sensors, audio devices, etc.)

### Database schema migration

Meta storage schema changes (e.g., adding new ML lifecycle states) must use versioned buckets:
- Store schema version in a `_meta` bucket
- On startup, check version and run migrations if needed
- Migrations must be **idempotent** and **reversible** (for rollback)

Example:
```
v1 → v2 migration: add "inference_active_timestamp" field to device records
- read all records in `ml_lifecycle` bucket
- for each record: if missing field, set to zero-value
- update schema version to v2
```

---

## Appendix E: Implementation checklist

Use this checklist to verify completeness before production deployment.

### State Manager core

- [ ] Per-device workflow serialization (mutex or work queue)
- [ ] Idempotent event handlers (state checks before acting)
- [ ] Startup reconciliation loop implemented
- [ ] Periodic reconciliation loop (5 min default)
- [ ] Reconnection reconciliation (on VM auth)
- [ ] ML lifecycle state persistence (crash-safe)
- [ ] Out-of-order model deployment handling
- [ ] Context timeout enforcement (30s VM calls, 10s AI calls)

### VM protocol implementation

- [ ] Device inventory sync (upsert semantics)
- [ ] Dataset upload (resumable, chunked)
- [ ] Model deployment receipt (push or pull)
- [ ] Security event delivery (batched, idempotent acks)
- [ ] VM resync protocol (for storage corruption recovery)
- [ ] mTLS with certificate pinning
- [ ] Protocol version negotiation
- [ ] Idempotency keys for all VM requests

### Model intake and verification

- [ ] Model manifest parsing and validation
- [ ] Signature verification (VM signing key pinned)
- [ ] Hash verification (all artifacts)
- [ ] Compatibility verification (runtime version, device capabilities)
- [ ] Policy verification (device assignment)
- [ ] Atomic activation (store → metadata → inference start)
- [ ] Rollback on activation failure

### Inference and event generation

- [ ] Device-agnostic ingestion (via IoT device data, not direct CCTV)
- [ ] Per-device model binding
- [ ] Confidence/class filtering
- [ ] Security event persistence (metadata + attachments)
- [ ] Event queue with backpressure
- [ ] At-least-once delivery to VM
- [ ] Circuit breaker for AI service failures
- [ ] Inference graceful stop on device disconnect

### Storage and resource management

- [ ] Object storage quota enforcement
- [ ] Meta storage quota enforcement
- [ ] Retention policies (datasets, events, models)
- [ ] Storage corruption detection (hash verification)
- [ ] Queue depth limits (security events, dataset uploads)
- [ ] Rate limiting (VM API, AI service, web UI)
- [ ] Bounded concurrency (inference, uploads)

### Failure handling

- [ ] Model verification failure → RecoveryRequired
- [ ] Storage corruption → VM resync protocol
- [ ] AI service down → circuit breaker + retry
- [ ] VM disconnect → offline queue + local inference
- [ ] Device disconnect → graceful inference stop
- [ ] Dataset upload failure → resumable retry
- [ ] Out-of-memory / storage full → backpressure + alerts

### Observability

- [ ] Health snapshot API (VM connection, devices, ML lifecycle, queues, storage)
- [ ] Metrics export (Prometheus or equivalent)
- [ ] Structured logging (correlation IDs per workflow)
- [ ] Operator dashboard (ML lifecycle overview, resource usage)
- [ ] Alerting (critical: storage full, corruption; warning: VM disconnect, latency)

### Audit and security

- [ ] Audit log calls for dataset lifecycle
- [ ] Audit log calls for model deployment
- [ ] Audit log calls for security events
- [ ] Audit log calls for recovery actions
- [ ] Audit log sync to VM
- [ ] Encrypt-at-rest for datasets and events
- [ ] Least-privilege access controls (web UI)
- [ ] No sensitive payloads in logs

### Testing

- [ ] Unit tests (state transitions, idempotency, locking)
- [ ] Integration tests (full device lifecycle)
- [ ] E2E tests (Edge + VM + mock camera)
- [ ] Chaos tests (kill AI service, disconnect VM, corrupt storage)
- [ ] Performance tests (10 devices × 1 inference/sec for 1 hour)
- [ ] Upgrade tests (v1 → v2 protocol version)

### Documentation

- [ ] Operator runbooks (add device, recovery from corruption, replace AI service)
- [ ] API documentation (VM protocol contracts)
- [ ] Configuration reference (all tunable parameters)
- [ ] Troubleshooting guide (common errors and solutions)
- [ ] Architecture diagrams (component interactions, state machines, data flows)

---

## Appendix F: Failure mode and effects analysis (FMEA)

| Failure Mode | Detection | Immediate Effect | Automated Mitigation | Operator Action Required | Severity |
|--------------|-----------|------------------|---------------------|-------------------------|----------|
| **AI service crash** | HTTP connection refused | Inference stops for all devices | Circuit breaker opens; retry with backoff | Check AI service logs; restart if needed | High |
| **AI service slow** | Inference timeout (>10s) | Inference backlog; queue growth | Circuit breaker opens; skip frames | Check AI service load; scale or optimize | Medium |
| **VM disconnect (transient)** | Connection state → disconnected | VM-facing operations pause | Queue events locally; retry connection | None (auto-recovery expected) | Low |
| **VM disconnect (extended, >1hr)** | Connection state disconnected >1hr | Event queue grows; no dataset uploads | Continue local inference; alert operator | Check VM reachability; diagnose network | Medium |
| **Device disconnect** | IoT state → disconnected | Inference stops for that device | Gracefully stop inference; wait for reconnect | Check device power/network | Low |
| **Storage full (object)** | Write returns quota exceeded | New datasets/events fail | Stop attachments; trigger cleanup; alert | Extend storage or adjust retention | Critical |
| **Storage full (meta)** | Write returns quota exceeded | ML lifecycle updates fail | Stop new workflows; alert | Extend storage or purge old records | Critical |
| **Storage corruption (object)** | Hash mismatch on read | Cannot load model/dataset | Mark affected devices RecoveryRequired; request resync | Approve VM resync; check disk health | High |
| **Storage corruption (meta)** | BoltDB error on open | Cannot load any ML state | Global RecoveryRequired; request full resync | Approve VM resync; restore from backup if available | Critical |
| **Model signature invalid** | Signature verification fails | Model not activated | Mark device RecoveryRequired; audit log; alert | Investigate VM signing process; re-deploy model | Critical (security) |
| **Model hash mismatch** | Hash verification fails | Model not activated | Mark device RecoveryRequired; request re-deploy | Check download integrity; re-deploy model | High |
| **Model incompatible runtime** | Compatibility check fails | Model not activated | Mark device RecoveryRequired; alert | Update Edge runtime or request compatible model | Medium |
| **Dataset upload fails (transient)** | HTTP error 5xx | Upload paused | Retry with backoff (resumable) | None (auto-recovery expected) | Low |
| **Dataset upload fails (persistent)** | Max retries exceeded | Device stuck in AwaitingDataset | Alert operator; mark upload as failed | Re-collect dataset or diagnose network | Medium |
| **Out-of-memory (Edge process)** | OOM killer or crash | Edge restart; inference stops | Startup reconciliation rebuilds state | Check memory config; increase if needed | High |
| **Device auth failure (ONVIF/RTSP)** | IoT connection error | Device unusable | Mark device as error state; alert | Check device credentials; update config | Medium |
| **Web UI credential leak** | Audit log: suspicious access patterns | Unauthorized dataset/event access | Rate limit; lock account; alert | Rotate credentials; investigate breach | Critical (security) |
| **VM compromised (worst case)** | Manual detection (external to Edge) | Malicious model deployment possible | Model signature verification (defense-in-depth) | Rotate VM signing keys; re-deploy trusted models | Critical (security) |

**Severity definitions**:
- **Critical**: system unsafe or unusable; immediate operator action required
- **High**: significant degradation; operator action required within hours
- **Medium**: partial degradation; operator action required within 24 hours
- **Low**: minor impact; auto-recovery expected; operator monitoring only

---

## Appendix G: Configuration parameters (tunable)

All parameters must have **sensible defaults** and **documented valid ranges**.

### State Manager configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `frame_processing_interval` | 30s | 1s - 300s | Interval between inference requests per device |
| `capability_sync_interval` | 5min | 30s - 1hr | How often to sync device inventory to VM |
| `max_concurrent_workflows` | 10 | 1 - 100 | Max parallel workflow executions across devices |
| `frame_capture_error_threshold` | 5 | 1 - 50 | Consecutive failures before device error state |
| `state_persistence_timeout` | 5s | 1s - 30s | Timeout for meta storage writes |
| `state_persistence_max_retries` | 3 | 0 - 10 | Max retry attempts for failed persistence |
| `event_dedup_window` | 1hr | 1min - 24hr | Time window for duplicate event detection |
| `serialize_workflows` | true | true/false | Whether to serialize workflows per device |

### VM Gateway configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `tunnel_connect_timeout` | 30s | 5s - 300s | Timeout for tunnel establishment |
| `tunnel_reconnect_interval` | 10s | 1s - 60s | Backoff between tunnel reconnect attempts |
| `transport_request_timeout` | 30s | 5s - 300s | Timeout for VM API calls |
| `transport_max_retries` | 3 | 0 - 10 | Max retries for failed VM requests |
| `auth_token_refresh_interval` | 1hr | 5min - 24hr | How often to refresh VM auth token (if applicable) |

### AI Gateway configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `ai_service_url` | (required) | valid URL | AI service endpoint |
| `inference_timeout` | 10s | 1s - 60s | Max time to wait for inference response |
| `confidence_threshold` | 0.5 | 0.0 - 1.0 | Min confidence for security event |
| `circuit_breaker_threshold` | 5 | 1 - 50 | Consecutive failures before circuit opens |
| `circuit_breaker_timeout` | 60s | 10s - 600s | How long circuit stays open before retry |
| `max_retries` | 3 | 0 - 10 | Max retries for failed inference |
| `retry_delay` | 1s | 100ms - 10s | Initial backoff between retries |

### IoT Service configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `discovery_interval` | 30s | 10s - 600s | How often to run device discovery |
| `discovery_timeout` | 10s | 1s - 60s | Timeout for discovery probes |
| `device_connect_timeout` | 10s | 1s - 60s | Timeout for device connection (RTSP, etc.) |
| `device_health_check_interval` | 60s | 10s - 600s | How often to check device connectivity |

### Storage configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `object_storage_quota_mb` | 100,000 | 1,000 - unlimited | Max object storage size (MB) |
| `meta_storage_quota_mb` | 1,000 | 100 - 10,000 | Max meta storage size (MB) |
| `dataset_retention_days` | 30 | 1 - 365 | How long to keep uploaded datasets |
| `event_retention_days` | 7 | 1 - 90 | How long to keep delivered events |
| `model_retention_versions` | 2 | 1 - 10 | How many model versions to keep per device |
| `storage_warning_threshold_pct` | 80 | 50 - 95 | Storage % that triggers warning alert |
| `storage_full_threshold_pct` | 95 | 80 - 100 | Storage % that triggers full alert |

### Queue configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `security_event_queue_max` | 10,000 | 100 - 1,000,000 | Max queued events before dropping oldest |
| `dataset_upload_queue_max` | 100 | 1 - 1,000 | Max concurrent dataset uploads |
| `event_batch_size` | 100 | 1 - 1,000 | Events per batch when uploading to VM |
| `event_batch_timeout` | 30s | 1s - 300s | Max time to wait before sending partial batch |

### Rate limiting configuration

| Parameter | Default | Valid Range | Description |
|-----------|---------|-------------|-------------|
| `vm_api_rate_limit_rpm` | 100 | 10 - 10,000 | Max VM API requests per minute |
| `ai_service_rate_limit_rps` | 1.0 | 0.1 - 100 | Max AI inference requests per second per device |
| `web_ui_rate_limit_rpm` | 1,000 | 10 - 100,000 | Max web UI requests per minute per IP |

**Configuration validation**: all parameters must be validated on startup; invalid values must cause startup failure with clear error message.

---

## Appendix H: Known limitations and future enhancements

### Current limitations (v1.0)

1. **Single VM per Edge**: Edge can only connect to one VM at a time. Multi-VM federation is not supported.
2. **No model A/B testing**: Cannot run two model versions in parallel for same device to compare results.
3. **No active learning feedback**: Edge cannot request re-training based on inference confidence patterns.
4. **Limited multi-tenancy**: All devices on one Edge belong to same tenant; no per-device tenant isolation.
5. **No real-time streaming to VM**: Security events are batched; live video streaming to VM not supported.
6. **Storage encryption at-rest is implementation-dependent**: Object storage provider must support encryption; Edge does not enforce it.
7. **No built-in anonymization**: PII (faces, license plates) are not automatically anonymized in datasets/events.

### Planned enhancements (v1.1+)

1. **Model performance monitoring**: Track inference accuracy over time; detect model drift; auto-request re-training.
2. **Federated learning support**: Edge contributes to federated model training without uploading raw data.
3. **Edge-to-Edge communication**: Devices can be load-balanced across multiple Edges in same site.
4. **Advanced retention policies**: Time-based + event-based (e.g., "keep events with confidence >0.9 for 90 days").
5. **Built-in anonymization pipeline**: Optional pre-processing to blur faces/plates before storage/upload.
6. **Real-time VM streaming**: Live video/audio streaming via WebRTC or low-latency RTMP for operator monitoring.
7. **Mobile Edge support**: Run State Manager on ARM/embedded devices with reduced resource footprint.
8. **Kubernetes-native deployment**: Helm charts, operator, and StatefulSet support for scalable Edge clusters.

### Non-goals (out of scope)

- **Edge-side training**: Training always happens on VM (or cloud); Edge only does inference.
- **Cross-tenant data sharing**: Data from one tenant never visible to another, even on shared Edge hardware.
- **Video analytics UI on Edge**: Web UI is for management/labeling only; rich video analytics belong on VM/cloud.

---

## Appendix I: Glossary

| Term | Definition |
|------|------------|
| **Edge** | Physical appliance running IoT services, inference, and VM connectivity |
| **VM** | Virtual Machine (or cloud service) that manages Edges, trains models, and stores long-term data |
| **DeviceID** | Stable identifier for an IoT device (camera, sensor, etc.) |
| **DatasetID** | Deterministic hash-based identifier for a labeled training dataset |
| **ModelID** | Immutable identifier for a specific trained model version |
| **SecurityEvent** | Detection/anomaly raised by AI inference on device data |
| **ML lifecycle state** | Per-device state machine tracking dataset → training → model → inference workflow |
| **RecoveryRequired** | Edge state indicating storage corruption or integrity failure; requires VM assistance to recover |
| **Reconciliation** | Idempotent background process that synchronizes runtime state with persisted state and VM truth |
| **Circuit breaker** | Fault-tolerance pattern that stops retrying a failing service after threshold reached |
| **Idempotency key** | Unique identifier for a request that allows safe retries (duplicate requests have same effect as single request) |
| **At-least-once delivery** | Message delivery guarantee where duplicates are possible but losses are not |
| **Backpressure** | Flow control mechanism that slows producers when consumers cannot keep up |
| **Tamper-evident** | Property of audit logs where modifications/deletions are detectable (e.g., via hash chaining) |
| **mTLS** | Mutual TLS; both client and server authenticate via X.509 certificates |

---

## Appendix J: workflow triggers (event-driven + reconciliation)

State Manager reacts to:
- VM connectivity transitions (from `vm-gateway`)
- device discovery/registration/connectivity changes (from `iot`)
- VM commands: assignment updates, dataset requests, model deployments
- user actions: labeling completion, dataset readiness
- storage pressure/health events

And always complements them with:
- startup reconciliation
- periodic reconciliation (default: every 5 minutes)
- reconnection reconciliation (on transition to authenticated/capabilities_received)

---

## Document revision history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-28 | Initial | Complete production workflow specification |
| 1.1 | 2025-12-28 | Polishing | Added production-critical details: EdgeID binding, certificate rotation, time sync, audit overflow, concurrent operations, storage backpressure, model rollback, reconciliation timeouts, chunk-level resumption, idempotency keys, partial failures, event bus drop policy |
| 1.2 | 2025-12-28 | Security Events | Clarified security event handling: immediate local persistence, ASAP delivery to VM when connected, immediate sync of unsynced events on VM reconnection. Enhanced event storage and delivery contracts with production requirements. |

**Approval**: This document must be reviewed and approved by architecture, security, and operations teams before implementation begins.

**Review cadence**: Quarterly review; update after each major Edge release.

**Status**: Production-ready. All critical production concerns addressed. Ready for implementation.

---

## Production-Critical Implementation Notes

### Security Event Handling (Critical)

**Key requirements**:
1. **Always persist locally first**: Security events detected by Edge/ai-service must be stored in Edge object storage immediately, regardless of VM connectivity
2. **Send ASAP when connected**: When VM is connected, send events immediately (no batching delay)
3. **Immediate sync on reconnection**: When VM reconnects after disconnection, immediately send all unsynced security event history (triggered by connection state transition)
4. **Queue management**: Max 10,000 events queued locally; oldest-first delivery; drop oldest on overflow (configurable)

### Certificate & Security Management

**Certificate rotation**:
- VM signals rotation via capabilities sync
- Edge downloads new CA cert before rotation
- Grace period: 7 days (both old and new CA trusted)
- After grace period: remove old CA

**Time synchronization**:
- Edge clock must be within ±5 minutes of VM
- If drift >30 minutes: fail authentication, emit critical alert

### Storage & Resource Management

**Gradual backpressure**:
- 80-90%: warnings, normal operation
- 90-95%: reduce capture rate by 50%
- >95%: stop captures, metadata-only events, aggressive cleanup

**Model rollback**:
- Automatic rollback if new model causes >50% inference errors for 1 hour
- Keep last 2 versions per device
- Retention: 7 days after purge eligibility

**Audit log overflow**:
- Queue up to 100,000 audit records locally
- If queue full: **do NOT drop** audit records; pause sensitive operations until sync resumes

### Reconciliation & Timeouts

**Reconciliation**:
- Entire pass must complete within 5 minutes (configurable)
- Consecutive failures >3 → emit unhealthy event
- Lock acquisition timeout: 30s

**Operation timeouts**:
- VM API calls: 30s default
- AI inference: 10s default
- Data unit capture: 60s default
- Model verification: 120s default
- Graceful shutdown: 60s total


