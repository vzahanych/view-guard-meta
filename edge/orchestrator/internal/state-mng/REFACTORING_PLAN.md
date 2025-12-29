# State Manager Refactoring Plan

**Date**: 2025-12-28  
**Target Document**: `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`  
**Scope**: Complete refactoring of `state-mng` package to align with production workflow requirements  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the State Manager implementation into full compliance with the production workflow specification defined in `WORKFLOW_AND_BUSINESS_LOGIC.md`. The current implementation is camera-centric and lacks critical production features including ML lifecycle management, reconciliation loops, device-agnostic architecture, and proper offline/error handling.

**Key Transformation Areas**:
1. **Device-agnostic architecture**: Replace camera-centric terminology with device-agnostic types
2. **ML lifecycle state management**: Implement per-device ML lifecycle state machine (replaces CameraState)
3. **Reconciliation loops**: Add startup, periodic, and reconnection reconciliation
4. **Security event handling**: Implement immediate persistence, ASAP delivery, and immediate sync on reconnection
5. **Model verification and activation**: Add verification gates and atomic activation
6. **VM protocol compliance**: Implement proper SyncDevices, SyncDataUnits, SendEvents, ReportModelStatus
7. **Storage integrity and recovery**: Add corruption detection and VM-assisted resync
8. **Per-device workflow serialization**: Implement proper locking per device

---

## Epic 1: Device-Agnostic Architecture Foundation

**Goal**: Transform the codebase from camera-centric to device-agnostic terminology and types.

### Section 1.1: Type System Refactoring

#### Subsection 1.1.1: Replace CameraID with DeviceID
- **Files**: All files in `types/` and `impl/`
- **Changes**:
  - Replace all `CameraID` references with `DeviceID`
  - Update type definitions: `CameraState` → `DeviceMLState` (see Epic 2)
  - Update function signatures and method names
  - Update variable names and map keys
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 1.1.2: Device-Agnostic Type Definitions
- **Files**: `types/types.go`, `types/camera_state.go`
- **Changes**:
  - Create `DeviceID` type alias (string)
  - Create `DeviceType` enum (camera, sensor, audio_device, etc.)
  - Create `DeviceCapability` type for capability modeling
  - Update `SecurityEvent` to use `DeviceID` and `DeviceType` instead of `CameraID`
  - Update `PendingSnapshotRequest` → `PendingDataUnitRequest` (device-agnostic)
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Update Service Interfaces
- **Files**: `state_mng.go`, `impl/state_mng_impl.go`
- **Changes**:
  - Update `StateManager` interface methods to use `DeviceID` instead of `CameraID`
  - Update method names: `SyncCameraCapabilities` → `SyncDeviceCapabilities`
  - Update method names: `UploadDatasetForCamera` → `UploadDatasetForDevice`
  - Update method names: `GetPendingSnapshotRequest` → `GetPendingDataUnitRequest`
- **Dependencies**: 1.1.1, 1.1.2
- **Estimated Effort**: 1 day

### Section 1.2: IoT Service Integration

#### Subsection 1.2.1: Device State Service Integration
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Remove camera state machine adapters cache (already using `DeviceStateService`)
  - Update all device state queries to use `iot.DeviceStateService` with `DeviceID`
  - Remove `getOrCreateCameraStateMachine` method (replaced by direct `DeviceStateService` calls)
  - Update device discovery event handlers to work with device-agnostic types
- **Dependencies**: 1.1.1, 1.1.2
- **Estimated Effort**: 2 days

#### Subsection 1.2.2: Device Data Pipeline Integration
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Update frame processing to use device-agnostic data types (`iot/types.DeviceData`)
  - Update inference triggers to work with `DeviceID` and `DeviceType`
  - Update data unit capture to support all device types (not just cameras)
- **Dependencies**: 1.2.1
- **Estimated Effort**: 2 days

---

## Epic 2: ML Lifecycle State Management

**Goal**: Implement per-device ML lifecycle state machine as specified in the workflow document.

### Section 2.1: ML Lifecycle State Type System

#### Subsection 2.1.1: Define ML Lifecycle States
- **Files**: `types/ml_lifecycle.go` (new file)
- **Changes**:
  - Define `MLLifecycleState` enum with all states:
    - `Unassigned`
    - `Assigned`
    - `AwaitingDataset`
    - `DatasetReadyLocal`
    - `DatasetUploadInProgress`
    - `DatasetUploaded`
    - `TrainingPending`
    - `ModelAvailable`
    - `ModelStored`
    - `InferenceActive`
    - `DegradedNoModel`
    - `RecoveryRequired`
  - Define `MLLifecycleStateInfo` struct with:
    - `DeviceID`, `State`, `LastUpdated`, `Error`
    - `ModelID`, `ModelVersion`, `DatasetID`
    - `OfflineInferenceAllowed` (policy flag)
    - `LastKnownGoodState` (for recovery)
- **Dependencies**: Epic 1
- **Estimated Effort**: 1 day

#### Subsection 2.1.2: ML Lifecycle State Machine Interface
- **Files**: `types/ml_lifecycle.go`
- **Changes**:
  - Define `MLLifecycleStateMachine` interface:
    - `GetState() MLLifecycleState`
    - `GetStateInfo() MLLifecycleStateInfo`
    - `Transition(newState MLLifecycleState, errorMsg string) error`
    - `CanTransition(newState MLLifecycleState) bool`
    - `IsOperational() bool`
  - Define state transition rules (valid transitions)
  - Define idempotent transition semantics
- **Dependencies**: 2.1.1
- **Estimated Effort**: 1 day

#### Subsection 2.1.3: ML Lifecycle Persistence Schema
- **Files**: `impl/ml_lifecycle_storage.go` (new file)
- **Changes**:
  - Define meta storage bucket: `ml_lifecycle` (key: `DeviceID`)
  - Define persistence format (JSON with version field)
  - Implement `SaveMLLifecycleState(ctx, deviceID, stateInfo) error`
  - Implement `LoadMLLifecycleState(ctx, deviceID) (*MLLifecycleStateInfo, error)`
  - Implement `LoadAllMLLifecycleStates(ctx) (map[DeviceID]*MLLifecycleStateInfo, error)`
  - Implement crash-safe persistence (write-ahead log or atomic updates)
- **Dependencies**: 2.1.1, 2.1.2
- **Estimated Effort**: 2 days

### Section 2.2: ML Lifecycle State Machine Implementation

#### Subsection 2.2.1: State Machine Core Implementation
- **Files**: `impl/ml_lifecycle_state_machine.go` (new file)
- **Changes**:
  - Implement `MLLifecycleStateMachine` interface
  - Implement state transition validation logic
  - Implement idempotent transitions (check current state before acting)
  - Integrate with meta storage for persistence
  - Add per-device locking for thread-safe transitions
- **Dependencies**: 2.1.3
- **Estimated Effort**: 3 days

#### Subsection 2.2.2: State Manager Integration
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Add `mlLifecycleStates map[DeviceID]MLLifecycleStateMachine` field
  - Add `mlLifecycleMu sync.RWMutex` for per-device locking
  - Implement `getOrCreateMLLifecycleStateMachine(deviceID) MLLifecycleStateMachine`
  - Replace all `CameraState` usage with `MLLifecycleState`
  - Update all workflow handlers to use ML lifecycle states
- **Dependencies**: 2.2.1
- **Estimated Effort**: 3 days

---

## Epic 3: Reconciliation Loops

**Goal**: Implement startup, periodic, and reconnection reconciliation loops as specified.

### Section 3.1: Reconciliation Framework

#### Subsection 3.1.1: Reconciliation Core Types
- **Files**: `types/reconciliation.go` (new file)
- **Changes**:
  - Define `ReconciliationPass` struct with:
    - `GenerationID` (UUID or incrementing counter)
    - `StartedAt`, `CompletedAt`, `Duration`
    - `DevicesProcessed`, `DevicesSkipped`, `DevicesFailed`
    - `Errors []error`
  - Define `ReconciliationHealth` struct:
    - `ConsecutiveFailures int`
    - `LastSuccessfulPass time.Time`
    - `IsHealthy bool`
  - Define reconciliation timeout configuration (default: 5 minutes)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Reconciliation Locking Strategy
- **Files**: `impl/reconciliation.go` (new file)
- **Changes**:
  - Implement global reconciliation mutex (only one reconciliation pass at a time)
  - Implement per-device lock acquisition with timeout (30s default)
  - Implement lock acquisition failure handling (skip device, retry next pass)
  - Add reconciliation generation ID tracking for debugging
- **Dependencies**: 3.1.1
- **Estimated Effort**: 2 days

### Section 3.2: Reconciliation Passes

#### Subsection 3.2.1: Reconcile VM Connectivity
- **Files**: `impl/reconciliation.go`
- **Changes**:
  - Implement `reconcileVMConnectivity(ctx) error`:
    - Query VM connection state from `vm-gateway`
    - Verify tunnel, transport, authentication state consistency
    - Handle authentication retry with exponential backoff (10s, 20s, 40s, max 5min)
    - Wait for capabilities with timeout (max 30s after authentication)
    - Validate capabilities (log warnings for unsupported capabilities)
  - Integrate with connection state transitions
- **Dependencies**: 3.1.2
- **Estimated Effort**: 2 days

#### Subsection 3.2.2: Reconcile Device Inventory
- **Files**: `impl/reconciliation.go`
- **Changes**:
  - Implement `reconcileDeviceInventory(ctx) error`:
    - Query IoT registry for all registered devices
    - For each device: ensure ML lifecycle record exists (create if missing, initial state: `Unassigned`)
    - If VM connected (`capabilities_received`):
      - Call `VMGateway.SyncDevices(ctx, devices)`
      - Process VM assignment response
      - Update ML lifecycle states based on assignments
      - Handle assignment changes (newly assigned → `Assigned`, unassigned → `Unassigned`)
  - Implement device assignment change detection
- **Dependencies**: 3.2.1, Epic 2
- **Estimated Effort**: 3 days

#### Subsection 3.2.3: Reconcile Model Availability and Integrity
- **Files**: `impl/reconciliation.go`
- **Changes**:
  - Implement `reconcileModelAvailability(ctx) error`:
    - For each device with ML lifecycle indicating model should exist:
      - Verify object storage presence
      - Verify integrity (hash verification)
      - Verify authenticity (signature verification)
      - Verify runtime compatibility (target runtime version, device capabilities)
      - If verification fails: transition to `RecoveryRequired` or `DegradedNoModel`
  - Integrate with model verification gates (see Epic 5)
- **Dependencies**: 3.2.2, Epic 5
- **Estimated Effort**: 2 days

#### Subsection 3.2.4: Reconcile Inference Execution
- **Files**: `impl/reconciliation.go`
- **Changes**:
  - Implement `reconcileInferenceExecution(ctx) error`:
    - For each device:
      - If `ModelStored` and device data stream available: ensure inference loop is running
      - If inference should not run (unassigned, policy disabled, device disconnected): ensure inference loop is stopped
  - Integrate with inference lifecycle management (see Epic 6)
- **Dependencies**: 3.2.3, Epic 6
- **Estimated Effort**: 2 days

#### Subsection 3.2.5: Reconcile Pending Outgoing Queues
- **Files**: `impl/reconciliation.go`
- **Changes**:
  - Implement `reconcilePendingQueues(ctx) error`:
    - **Security events**: query meta storage for `pending_delivery` or `delivery_failed` events
      - If VM connected: send immediately (no backoff delay)
      - Prioritize oldest events first (FIFO)
      - Batch up to 100 events per request
    - **Dataset uploads**: resume or restart from last verified checkpoint
  - Integrate with security event delivery (see Epic 7)
- **Dependencies**: 3.2.4, Epic 7
- **Estimated Effort**: 2 days

### Section 3.3: Reconciliation Orchestration

#### Subsection 3.3.1: Startup Reconciliation Loop
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Implement `runStartupReconciliation(ctx) error`:
    - Run all reconciliation passes in sequence
    - Load persisted ML lifecycle state per device
    - Execute full reconciliation pass
    - Handle timeout (5 minutes) and abort if exceeded
  - Call from `Start()` method after event bus subscription
- **Dependencies**: Section 3.2 (all subsections)
- **Estimated Effort**: 1 day

#### Subsection 3.3.2: Periodic Reconciliation Loop
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Implement `runPeriodicReconciliation(ctx) error`:
    - Run every N minutes (default: 5 minutes, configurable)
    - Execute all reconciliation passes
    - Track reconciliation health (consecutive failures >3 → emit `reconciliation.unhealthy` event)
    - Handle timeout and abort if exceeded
  - Start as background goroutine in `Start()` method
- **Dependencies**: 3.3.1
- **Estimated Effort**: 1 day

#### Subsection 3.3.3: Reconnection Reconciliation
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Implement `runReconnectionReconciliation(ctx) error`:
    - Triggered on VM connection transition: `disconnected` → `authenticated` → `capabilities_received`
    - **CRITICAL: Security event sync is triggered IMMEDIATELY** (highest priority, before other reconciliation passes)
      - Query meta storage for all events with status `pending_delivery` or `delivery_failed`
      - Send all unsynced events immediately (no delay, no waiting for reconciliation timer)
      - Process in batches of 100 events
      - Continue until all pending events are sent or connection fails again
    - Execute remaining reconciliation passes after security event sync
    - Handle timeout and abort if exceeded
  - Subscribe to connection state transition events
- **Dependencies**: 3.3.2
- **Estimated Effort**: 1 day

---

## Epic 4: VM Protocol Implementation

**Goal**: Implement proper VM protocol contracts (SyncDevices, SyncDataUnits, SendEvents, ReportModelStatus).

### Section 4.1: Device Sync Protocol

#### Subsection 4.1.1: SyncDevices Request/Response
- **Files**: `impl/vm_protocol.go` (new file)
- **Changes**:
  - Implement `syncDevicesToVM(ctx, devices []DeviceInfo) (*SyncDevicesResponse, error)`:
    - Build `SyncDevicesRequest` with:
      - `EdgeID`, `Devices []DeviceInfo` (DeviceID, DeviceType, Capabilities, Metadata, State)
      - `SyncTimestamp`, `SyncID` (UUID for idempotency)
    - Call `VMGateway.SyncDevices(ctx, request)`
    - Process response: `Assignments map[DeviceID]DeviceAssignment`
    - Update ML lifecycle states based on assignments
  - Handle partial failures (some devices invalid)
  - Implement idempotency (deduplicate by `(EdgeID, sync_id)`)
- **Dependencies**: Epic 1, Epic 2
- **Estimated Effort**: 2 days

#### Subsection 4.1.2: Device Assignment Handling
- **Files**: `impl/vm_protocol.go`
- **Changes**:
  - Implement assignment change detection:
    - Newly assigned → transition to `Assigned`, start dataset workflow
    - Unassigned (was assigned) → transition to `Unassigned`, stop inference
    - Keep local data (datasets, models, events) for 30 days (configurable)
  - Store assignment policy in ML lifecycle record (`offline_inference_allowed` flag)
  - **Production requirement**: HTTPS server must already be listening and ready to receive VM commands before authentication completes
    - Reference: vm-gateway Epic 5 (HTTPS Server Readiness Guarantee)
    - VM may send capabilities and data unit capture requests immediately after authentication
- **Dependencies**: 4.1.1
- **Estimated Effort**: 1 day

### Section 4.2: Data Unit Sync Protocol

#### Subsection 4.2.1: Dataset Packaging
- **Files**: `impl/dataset_packaging.go` (new file)
- **Changes**:
  - Implement deterministic `DatasetID` generation:
    - Hash includes: EdgeID, DeviceID, sorted list of (DataUnitID, label) pairs, schema version
    - Use SHA-256 for hash
    - Handle collision (regenerate with salt/timestamp if VM reports collision)
  - Implement dataset manifest structure:
    - DeviceID, EdgeID
    - DataUnitID, object_key, raw_data_format (jpeg, png, json, wav, etc.)
    - label, custom_label, description
    - metadata (timestamps, source device properties, device type)
    - created_at timestamp
- **Dependencies**: Epic 1
- **Estimated Effort**: 2 days

#### Subsection 4.2.2: SyncDataUnits Request/Response
- **Files**: `impl/vm_protocol.go`
- **Changes**:
  - Implement `syncDataUnitsToVM(ctx, deviceID, dataUnits []DataUnit) error`:
    - Build `SyncDataUnitsRequest` with:
      - EdgeID, DeviceID
      - DataUnits array (with all required fields from manifest)
    - Call `VMGateway.SyncDataUnits(ctx, request)`
    - Handle response and update ML lifecycle state:
      - `DatasetReadyLocal` → `DatasetUploadInProgress` → `DatasetUploaded`
    - Implement resumable upload (chunked, with per-chunk hashes)
    - Implement chunk-level resumption (VM responds with received chunks)
  - Handle idempotency (VM deduplicates by `data_unit_id`)
  - Support batching (max 100 data units per request, configurable)
- **Dependencies**: 4.2.1, Epic 2
- **Estimated Effort**: 3 days

### Section 4.2.3: VM → Edge Data Unit Capture Request (VM Push)
- **Files**: `impl/vm_protocol.go`
- **Changes**:
  - Implement handler for VM → Edge data unit capture requests:
    - **Endpoint**: `POST /api/v1/data-units/capture` (device-agnostic endpoint)
    - **Note**: Legacy endpoint `/api/v1/snapshots/capture` may be supported for backward compatibility but should be deprecated
    - Receive request from VM HTTPS server (VM → Edge push):
      ```
      POST https://{edge_https_server}/api/v1/data-units/capture
      Headers: Authorization: mTLS (VM client cert)
      {
        "device_id": "device-001",
        "device_type": "camera",
        "label": "normal",
        "custom_label": "",
        "count": 5,
        "auto_capture": false
      }
      ```
    - Validate VM client certificate (mTLS)
    - Store pending data unit capture request in meta-storage
    - Transition device ML lifecycle to **AwaitingDataset**
    - Publish `data_unit.requested` event to event bus
    - Handle multiple concurrent requests per device (queue requests, process one at a time)
    - If queue is full (configurable, default 10 per device): reject with HTTP 429 Too Many Requests
  - **Production requirement**: Edge must handle out-of-order requests gracefully (request may arrive before device is synced; queue and process when device becomes available)
- **Dependencies**: 4.2.2, Epic 2
- **Estimated Effort**: 2 days

### Section 4.3: Security Event Delivery Protocol

#### Subsection 4.3.1: SendEvents Request/Response
- **Files**: `impl/vm_protocol.go`
- **Changes**:
  - Implement `sendEventsToVM(ctx, events []SecurityEvent) (*SendEventsResponse, error)`:
    - Build `SendEventsRequest` with:
      - EdgeID
      - Events array (EventID, DeviceID, DeviceType, event_type, timestamp, confidence, ModelID, model_version, attachment_url or attachment_inline)
    - Call `VMGateway.SendEvents(ctx, request)` (or equivalent method)
    - Process response: `Acknowledged []EventID`, `Failed []EventID`, `Duplicate []EventID`
    - Update event status in meta storage: `pending_delivery` → `delivered`
  - Implement batching (max 100 events per request, configurable)
  - Implement idempotency (VM deduplicates by `(EdgeID, EventID)`)
- **Dependencies**: Epic 1, Epic 7
- **Estimated Effort**: 2 days

#### Subsection 4.3.2: Event Delivery Orchestration
- **Files**: `impl/event_delivery.go` (new file)
- **Changes**:
  - Implement immediate delivery when VM connected (no batching delay)
  - Implement immediate sync on reconnection (triggered by connection state transition)
  - Implement retry strategy with exponential backoff (1s, 2s, 4s, 8s, max 60s)
  - Implement FIFO priority (oldest events first)
  - Integrate with reconciliation (see 3.2.5)
- **Dependencies**: 4.3.1
- **Estimated Effort**: 2 days

### Section 4.4: Model Deployment Status Protocol

#### Subsection 4.4.1: ReportModelStatus
- **Files**: `impl/vm_protocol.go`
- **Changes**:
  - Implement `reportModelStatus(ctx, deploymentID, status, errorMessage, modelPath) error`:
    - Build `ReportModelStatusRequest` with:
      - DeploymentID, Status (deployed, verification_failed, activation_failed)
      - Timestamp, Error (if failed)
    - Call `VMGateway.ReportModelStatus(ctx, request)`
  - Call after model verification (success or failure)
  - Call after model activation (success or failure)
- **Dependencies**: Epic 5
- **Estimated Effort**: 1 day

---

## Epic 5: Model Intake, Verification, and Activation

**Goal**: Implement model verification gates and atomic activation as specified.

### Section 5.1: Model Artifact Envelope

#### Subsection 5.1.1: Model Manifest Structure
- **Files**: `types/model_manifest.go` (new file)
- **Changes**:
  - Define `ModelManifest` struct:
    - ModelID, DeviceID, Version
    - Target runtime (e.g., OpenVINO version)
    - Preprocessing config
    - Artifact list + hashes
    - Compatibility constraints
    - Training provenance (dataset references, VM job id)
    - Signature (VM model-signing identity)
    - Protocol version, schema version
  - Define `ModelArtifact` struct (binary + runtime artifacts)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Model Deployment Receipt
- **Files**: `impl/model_deployment.go` (new file)
- **Changes**:
  - Implement `receiveModelDeployment(ctx, deploymentRequest) error`:
    - Handle VM → Edge push via HTTPS server endpoint
    - Parse multipart/form-data (metadata JSON + model binary)
    - Validate VM client certificate (mTLS)
    - Store deployment as pending if device not ready (out-of-order handling)
    - Transition device ML lifecycle to appropriate state
  - Implement pending deployment registry (meta storage bucket: `pending_model_deployments`)
  - Implement pending deployment TTL (24 hours, configurable)
- **Dependencies**: 5.1.1, Epic 2
- **Estimated Effort**: 2 days

### Section 5.2: Model Verification Gates

#### Subsection 5.2.1: Authenticity Verification
- **Files**: `impl/model_verification.go` (new file)
- **Changes**:
  - Implement `verifyModelAuthenticity(ctx, manifest) error`:
    - Verify VM signature chain (pinned root CA)
    - Validate certificate chain
    - Check certificate revocation (CRL or OCSP, cache for 1 hour)
    - Store verification result in meta storage for audit
  - Handle certificate rotation (grace period: 7 days)
- **Dependencies**: 5.1.1
- **Estimated Effort**: 3 days

#### Subsection 5.2.2: Integrity Verification
- **Files**: `impl/model_verification.go`
- **Changes**:
  - Implement `verifyModelIntegrity(ctx, manifest, artifacts) error`:
    - Verify hashes for all artifacts (SHA-256)
    - Compare with manifest hashes
    - Store integrity evidence in meta storage
  - Handle hash mismatches (transition to `RecoveryRequired`)
- **Dependencies**: 5.2.1
- **Estimated Effort**: 2 days

#### Subsection 5.2.3: Compatibility Verification
- **Files**: `impl/model_verification.go`
- **Changes**:
  - Implement `verifyModelCompatibility(ctx, manifest, deviceInfo) error`:
    - Verify runtime versions match (target runtime vs. Edge runtime)
    - Verify device capabilities match (device must support required capabilities)
    - Verify protocol version compatibility
    - Store compatibility evidence in meta storage
  - Handle incompatibility (transition to `RecoveryRequired` or policy-defined quarantine)
- **Dependencies**: 5.2.2
- **Estimated Effort**: 2 days

#### Subsection 5.2.4: Policy Verification
- **Files**: `impl/model_verification.go`
- **Changes**:
  - Implement `verifyModelPolicy(ctx, manifest, deviceInfo) error`:
    - Verify device is assigned to this Edge
    - Verify rollout policy (allow/deny from VM assignment response)
    - Store policy verification result in meta storage
  - Handle policy violations (transition to `RecoveryRequired`)
- **Dependencies**: 5.2.3, Epic 2
- **Estimated Effort**: 1 day

### Section 5.3: Atomic Model Activation

#### Subsection 5.3.1: Model Storage
- **Files**: `impl/model_activation.go` (new file)
- **Changes**:
  - Implement `storeModelArtifacts(ctx, manifest, artifacts) error`:
    - Store artifacts in object storage
    - Store metadata in meta storage (deployment metadata, compatibility, integrity evidence)
    - Update ML lifecycle state to `ModelStored`
    - Implement atomic storage (all-or-nothing)
  - Handle storage failures (rollback on partial failure)
- **Dependencies**: Section 5.2 (all subsections)
- **Estimated Effort**: 2 days

#### Subsection 5.3.2: Inference Activation
- **Files**: `impl/model_activation.go`
- **Changes**:
  - Implement `activateInference(ctx, deviceID, modelID) error`:
    - Start inference loop for device (see Epic 6)
    - Update ML lifecycle state to `InferenceActive`
    - Implement graceful stop of previous inference (if active)
    - Handle concurrent deployment (reject if another deployment in progress)
  - Implement activation timeout (complete current inference batch, max 10s wait)
- **Dependencies**: 5.3.1, Epic 6
- **Estimated Effort**: 2 days

#### Subsection 5.3.3: Activation Rollback
- **Files**: `impl/model_activation.go`
- **Changes**:
  - Implement `rollbackModelActivation(ctx, deviceID) error`:
    - If activation fails mid-way: roll back to consistent state
    - Restore previous model version (if available within retention period)
    - Transition to `ModelStored` or `RecoveryRequired` based on error
  - Handle rollback failures (transition to `RecoveryRequired`)
- **Dependencies**: 5.3.2
- **Estimated Effort**: 1 day

---

## Epic 6: Inference and Event Generation

**Goal**: Implement device-agnostic inference and security event generation.

### Section 6.1: Device-Agnostic Inference Integration

#### Subsection 6.1.1: IoT Data Processor Integration
- **Files**: `impl/inference_lifecycle.go` (new file)
- **Changes**:
  - Implement inference as IoT `DataProcessor` for `DeviceDataTypeVideoFrame` (and other types)
  - Register processor with IoT service for each device with `ModelStored` state
  - Implement per-device model binding (no global default model)
  - Implement confidence/class filtering
  - Handle device-agnostic data types (video frames, sensor readings, audio samples)
- **Dependencies**: Epic 1, Epic 2, Epic 5
- **Estimated Effort**: 3 days

#### Subsection 6.1.2: Inference Loop Management
- **Files**: `impl/inference_lifecycle.go`
- **Changes**:
  - Implement `startInferenceLoop(ctx, deviceID) error`:
    - Start background goroutine for continuous inference
    - Process device data at configured interval (default: 30s)
    - Apply model selection per device
    - Handle inference failures (non-fatal, circuit breaker)
  - Implement `stopInferenceLoop(ctx, deviceID) error`:
    - Gracefully stop inference loop
    - Complete current inference batch (max 10s wait)
  - Integrate with ML lifecycle state transitions
- **Dependencies**: 6.1.1
- **Estimated Effort**: 2 days

#### Subsection 6.1.3: Circuit Breaker for AI Service
- **Files**: `impl/inference_lifecycle.go`
- **Changes**:
  - Implement circuit breaker pattern:
    - Open after N consecutive failures (N=5 default)
    - Stop inference for all devices when circuit opens
    - Retry with exponential backoff (1s, 2s, 4s, ..., max 60s)
    - Close when AI service recovers
  - Emit operational events: `ai.circuit_breaker_opened`, `ai.circuit_breaker_closed`
  - Integrate with reconciliation (re-start inference when circuit closes)
- **Dependencies**: 6.1.2
- **Estimated Effort**: 2 days

### Section 6.2: Security Event Generation

#### Subsection 6.2.1: Event Detection and Normalization
- **Files**: `impl/event_generation.go` (new file)
- **Changes**:
  - Implement `generateSecurityEvent(ctx, deviceID, inferenceResult) (*SecurityEvent, error)`:
    - Convert inference output to normalized detection/anomaly result
    - Create `SecurityEvent` with:
      - EventID (UUID), DeviceID, DeviceType
      - event_type, timestamp, confidence
      - ModelID, model_version
      - bounding_box (if applicable)
      - metadata (device-specific context)
    - Apply confidence threshold filtering
    - Apply class filters
  - Integrate with AI gateway inference results
- **Dependencies**: 6.1.1, Epic 1
- **Estimated Effort**: 2 days

#### Subsection 6.2.2: Event Persistence Contract
- **Files**: `impl/event_generation.go`
- **Changes**:
  - Implement immediate persistence (always, regardless of VM connectivity):
    - Persist metadata → meta storage (status: `pending_delivery`)
    - Persist attachments → object storage (images, sensor readings, audio samples)
    - Do NOT wait for VM connectivity
    - Do NOT block inference while persisting (async write)
  - Implement event queue management (max 10,000 events, configurable)
  - Implement queue overflow handling (drop oldest with FIFO, or stop generation based on policy)
- **Dependencies**: 6.2.1, Epic 7
- **Estimated Effort**: 2 days

---

## Epic 7: Security Event Storage and Delivery

**Goal**: Implement production-critical security event handling (immediate persistence, ASAP delivery, immediate sync on reconnection).

### Section 7.1: Event Storage Refactoring

#### Subsection 7.1.1: Event Status Management
- **Files**: `types/types.go`, `impl/event_storage.go`
- **Changes**:
  - Update `SecurityEvent` to include:
    - `Status` field (`pending_delivery`, `delivery_failed`, `delivered`)
    - `DeliveryAttempts int`
    - `LastDeliveryAttempt time.Time`
    - `VMACKTimestamp time.Time` (when VM acknowledged)
  - Update event storage schema in meta storage
  - Implement status transitions
- **Dependencies**: Epic 1
- **Estimated Effort**: 1 day

#### Subsection 7.1.2: Immediate Persistence Implementation
- **Files**: `impl/event_storage.go`
- **Changes**:
  - Update `SaveSecurityEvent` to:
    - Always persist immediately (no VM connectivity check)
    - Store with status `pending_delivery`
    - Store attachments in object storage asynchronously
    - Do NOT block inference while persisting
  - Implement async write queue for attachments
  - Handle storage failures (retry with backoff, but don't block inference)
- **Dependencies**: 7.1.1
- **Estimated Effort**: 2 days

### Section 7.2: Event Delivery Implementation

#### Subsection 7.2.1: Immediate Delivery When Connected
- **Files**: `impl/event_delivery.go`
- **Changes**:
  - Implement `deliverEventsToVM(ctx) error`:
    - Query meta storage for events with status `pending_delivery` or `delivery_failed`
    - If VM connected (`authenticated` + `capabilities_received`):
      - Send events ASAP (no batching delay, or batch up to 100 if multiple pending)
      - No backoff on first attempt
      - Retry with exponential backoff only on failure (1s, 2s, 4s, 8s, max 60s)
    - Prioritize oldest events first (FIFO)
    - Batch size: max 100 events per request (configurable)
  - Trigger from event generation (when new event created and VM connected)
- **Dependencies**: 7.1.2, Section 4.3
- **Estimated Effort**: 2 days

#### Subsection 7.2.2: Immediate Sync on Reconnection
- **Files**: `impl/event_delivery.go`
- **Changes**:
  - Implement `syncPendingEventsOnReconnection(ctx) error`:
    - Triggered on VM connection transition: `disconnected` → `authenticated` → `capabilities_received`
    - Query meta storage for all events with status `pending_delivery` or `delivery_failed`
    - Send all unsynced events immediately (no delay)
    - Process in batches of 100 events
    - Continue until all pending events are sent or connection fails again
  - Integrate with reconnection reconciliation (see 3.3.3)
  - High priority operation (do not wait for next reconciliation)
- **Dependencies**: 7.2.1
- **Estimated Effort**: 1 day

#### Subsection 7.2.3: At-Least-Once Delivery Guarantee
- **Files**: `impl/event_delivery.go`
- **Changes**:
  - Implement idempotency:
    - Edge persists events locally **before** attempting delivery
    - Edge retries until VM ack is received
    - VM deduplicates by `(EdgeID, EventID)` tuple
    - Handle VM duplicate response (`"duplicate": ["evt-001"]`)
  - Update event status to `delivered` only after VM acknowledgment
  - Implement retention policy (keep events locally for 7 days after VM ack, configurable)
- **Dependencies**: 7.2.2
- **Estimated Effort**: 1 day

### Section 7.3: Queue Management and Backpressure

#### Subsection 7.3.1: Queue Limits and Overflow Handling
- **Files**: `impl/event_delivery.go`
- **Changes**:
  - Implement queue limit: max 10,000 events (configurable)
  - Implement overflow handling:
    - If queue full: drop oldest events (FIFO) with operator alert
    - OR: stop new event generation (if policy requires all events to be delivered)
    - Policy configurable: `drop_oldest_on_overflow` (default: true)
  - Implement queue depth monitoring and alerts
- **Dependencies**: 7.2.3
- **Estimated Effort**: 1 day

#### Subsection 7.3.2: Storage Backpressure
- **Files**: `impl/event_delivery.go`
- **Changes**:
  - Implement gradual backpressure:
    - 80-90% full: emit warning, continue normal operation
    - 90-95% full: throttle event attachments (smaller images, reduced data samples)
    - >95% full: stop new event attachments (metadata-only events), enforce retention policies (oldest-first) **only if policy permits**, emit "storage pressure" critical alert
  - Integrate with storage health monitoring
- **Dependencies**: 7.3.1
- **Estimated Effort**: 1 day

---

## Epic 8: Storage Integrity and Recovery

**Goal**: Implement corruption detection and VM-assisted resync protocol.

### Section 8.1: Integrity Evidence

#### Subsection 8.1.1: Integrity Metadata Storage
- **Files**: `impl/storage_integrity.go` (new file)
- **Changes**:
  - For each dataset and model artifact:
    - Store content hash (SHA-256) in meta storage
    - Store signed manifest reference
    - Store creation time, source VM identity, audit record
  - Implement integrity metadata schema in meta storage
  - Implement periodic integrity verification (background task)
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 8.1.2: Corruption Detection
- **Files**: `impl/storage_integrity.go`
- **Changes**:
  - Implement corruption detection:
    - Missing objects referenced by meta storage
    - Hash mismatches
    - Signature validation failures
    - Meta/object storage internal health checks returning "unhealthy"
  - Emit `storage.corruption_detected` event
  - Transition affected devices to `RecoveryRequired` state
- **Dependencies**: 8.1.1, Epic 2
- **Estimated Effort**: 2 days

### Section 8.2: RecoveryRequired Behavior

#### Subsection 8.2.1: Per-Device Recovery
- **Files**: `impl/recovery.go` (new file)
- **Changes**:
  - Implement `handleDeviceRecovery(ctx, deviceID, reason) error`:
    - Transition ML lifecycle to `RecoveryRequired`
    - Stop inference for that device
    - Keep device management active (discovery/registration continues)
    - If VM is reachable: request resync for that device
  - Implement resync request to VM:
    - `POST /api/v1/devices/{deviceID}/resync`
    - Request body: EdgeID, DeviceID, recovery_reason, corrupted_resources, last_known_good_state
  - Process VM response: latest assignment + policy, model artifact download URL, optional dataset reference
- **Dependencies**: 8.1.2, Epic 2, Section 4.1
- **Estimated Effort**: 3 days

#### Subsection 8.2.2: Global Recovery
- **Files**: `impl/recovery.go`
- **Changes**:
  - Implement `handleGlobalRecovery(ctx) error`:
    - If meta storage is corrupted globally (cannot load ML lifecycle records):
      - Enter global `RecoveryRequired` mode
      - Disable inference until trust is restored
      - Request full resync from VM:
        - Device list assigned to this Edge
        - Model inventory per device + manifests
      - Rebuild local metadata from VM responses
  - Implement recovery audit trail (log all resync requests to audit-log)
- **Dependencies**: 8.2.1
- **Estimated Effort**: 2 days

---

## Epic 9: Per-Device Workflow Serialization

**Goal**: Implement per-device workflow serialization to prevent race conditions.

### Section 9.1: Per-Device Locking

#### Subsection 9.1.1: Device Lock Map
- **Files**: `impl/workflow_serialization.go` (new file)
- **Changes**:
  - Implement per-device mutex map: `deviceLocks map[DeviceID]*sync.RWMutex`
  - Implement `acquireDeviceLock(ctx, deviceID, timeout) (func(), error)`:
    - Acquire device-specific lock with timeout (30s default, configurable)
    - Return release function
    - Handle timeout (skip device in current pass, retry in next reconciliation)
  - Implement lock acquisition failure tracking (log warning if >3 consecutive failures)
- **Dependencies**: Epic 1
- **Estimated Effort**: 1 day

#### Subsection 9.1.2: Workflow Serialization Integration
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Update all workflow handlers to acquire device lock before acting:
    - Device sync workflows
    - Dataset upload workflows
    - Model deployment workflows
    - Inference activation workflows
    - Reconciliation passes (per device)
  - Ensure only one workflow operation executes per device at any time
  - Allow cross-device parallelism with bounded concurrency
- **Dependencies**: 9.1.1
- **Estimated Effort**: 3 days

---

## Epic 10: Offline Behavior and VM Disconnection Handling

**Goal**: Implement proper offline behavior when VM is disconnected.

### Section 10.1: Offline Inference Policy

#### Subsection 10.1.1: Offline Inference Configuration
- **Files**: `types/ml_lifecycle.go`
- **Changes**:
  - Add `OfflineInferenceAllowed bool` field to `MLLifecycleStateInfo`
  - Set from VM assignment response field `offline_inference_allowed` (default: true)
  - Store in ML lifecycle record
- **Dependencies**: Epic 2, Section 4.1
- **Estimated Effort**: 1 day

#### Subsection 10.1.2: Offline Inference Logic
- **Files**: `impl/offline_behavior.go` (new file)
- **Changes**:
  - Implement offline inference decision:
    - Do NOT stop inference for devices where:
      - Device is connected locally, AND
      - Verified model is available, AND
      - Policy allows offline detection (`offline_inference_allowed == true`)
    - Do stop/pause VM-dependent workflows:
      - Device sync to VM
      - Dataset upload
      - Model fetch (unless previously cached)
      - Event delivery to VM (but continue local event generation + persistence)
  - Integrate with VM connection state monitoring
- **Dependencies**: 10.1.1, Epic 2
- **Estimated Effort**: 2 days

### Section 10.2: VM Disconnection Handling

#### Subsection 10.2.1: Connection State Monitoring
- **Files**: `impl/offline_behavior.go`
- **Changes**:
  - Monitor VM connection state transitions
  - On transition away from `authenticated`/`capabilities_received`:
    - Pause VM-dependent workflows
    - Continue local inference (if policy allows)
    - Continue local event generation + persistence
  - On transition to `disconnected`:
    - Retry tunnel/transport connection with exponential backoff (max interval: 5 minutes)
    - Emit periodic "VM unreachable" operational alerts
    - Do NOT restart authentication loop until tunnel/transport are re-established
- **Dependencies**: 10.1.2
- **Estimated Effort**: 2 days

---

## Epic 11: Observability and Health

**Goal**: Implement health snapshot API and operational metrics.

### Section 11.1: Health Snapshot API

#### Subsection 11.1.1: Health Snapshot Structure
- **Files**: `types/health.go` (new file)
- **Changes**:
  - Define `HealthSnapshot` struct:
    - VM connection state (from `vm-gateway`)
    - Device counts by state (from `iot`)
    - ML lifecycle counts by state (assigned, dataset ready, model stored, inference active, recovery required)
    - Queue depth and oldest pending event age
    - Storage health (healthy/warning/full) and integrity error counters
    - Last successful sync timestamps (devices, datasets, models)
  - Define `OperationalMetrics` struct:
    - Inference latency (P50/P95/P99)
    - Event rate, upload throughput, retry counts
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 11.1.2: Health Snapshot Implementation
- **Files**: `impl/health.go` (new file)
- **Changes**:
  - Implement `GetHealthSnapshot(ctx) (*HealthSnapshot, error)`:
    - Query VM connection state from `vm-gateway`
    - Query device counts from `iot`
    - Query ML lifecycle counts from persisted state
    - Query queue depths from meta storage
    - Query storage health from meta/object storage
    - Aggregate metrics
  - Expose via `StateManager` interface
- **Dependencies**: 11.1.1
- **Estimated Effort**: 2 days

### Section 11.2: Operational Events

#### Subsection 11.2.1: Event Emission
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Emit operational events via event bus:
    - `system.startup`, `system.shutdown`
    - `system.reconciliation_started`, `system.reconciliation_completed`
    - `health.degraded`, `health.recovered`
    - `storage.warning`, `storage.full`, `storage.corruption_detected`
    - `reconciliation.unhealthy`
  - Use structured logging with correlation IDs per workflow and per device
- **Dependencies**: None
- **Estimated Effort**: 1 day

---

## Epic 12: Configuration and Timeouts

**Goal**: Implement all configuration parameters and timeout enforcement as specified.

### Section 12.1: Configuration Parameters

#### Subsection 12.1.1: Configuration Structure Updates
- **Files**: `types/config.go`
- **Changes**:
  - Add all configuration parameters from Appendix G:
    - Reconciliation timeout (5 minutes default)
    - Security event queue max (10,000 default)
    - Event batch size (100 default)
    - Storage warning/full thresholds (80%/95% default)
    - Retention policies (datasets: 30 days, events: 7 days, models: 2 versions)
    - Rate limiting (VM API: 100 req/min, AI service: 1 req/sec per device, Web UI: 1000 req/min per IP)
    - Timeouts (VM API: 30s, AI inference: 10s, data unit capture: 60s, model verification: 120s)
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 12.1.2: Configuration Application
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Apply all configuration parameters during initialization
  - Validate configuration on startup (fail with clear error if invalid)
  - Use configuration values throughout implementation
- **Dependencies**: 12.1.1
- **Estimated Effort**: 1 day

### Section 12.2: Timeout Enforcement

#### Subsection 12.2.1: Context Timeout Implementation
- **Files**: All implementation files
- **Changes**:
  - Add explicit timeouts to all operations:
    - VM API calls: 30s default (configurable)
    - AI inference: 10s default (configurable)
    - Storage operations: 5s default
    - Data unit capture: 60s default
    - Model verification: 120s default
  - Use `context.WithTimeout` for all external calls
  - Propagate cancellation correctly
- **Dependencies**: 12.1.1
- **Estimated Effort**: 2 days

#### Subsection 12.2.2: Graceful Shutdown
- **Files**: `impl/state_mng_impl.go`
- **Changes**:
  - Implement graceful shutdown on SIGTERM:
    - Stop accepting new workflows (60s total timeout)
    - Complete in-flight inference batches (or abort after 10s)
    - Flush security event queue to storage (at-least-once guarantee)
    - Persist all ML lifecycle state
    - Close all service connections
  - Integrate with fx lifecycle hooks
- **Dependencies**: 12.2.1
- **Estimated Effort**: 1 day

---

## Epic 13: Audit Logging Integration

**Goal**: Integrate with audit-log service for security-sensitive actions.

### Section 13.1: Audit Log Calls

#### Subsection 13.1.1: Dataset Lifecycle Audit
- **Files**: `impl/dataset_packaging.go`, `impl/vm_protocol.go`
- **Changes**:
  - Call audit-log on:
    - Dataset creation
    - Labeling completion
    - Upload initiation and completion
  - Include: EdgeID, DeviceID, DatasetID, timestamp, action type
- **Dependencies**: None (assumes audit-log service interface exists)
- **Estimated Effort**: 1 day

#### Subsection 13.1.2: Model Lifecycle Audit
- **Files**: `impl/model_deployment.go`, `impl/model_activation.go`
- **Changes**:
  - Call audit-log on:
    - Model deployment receipt
    - Model verification (success or failure)
    - Model activation (success or failure)
  - Include: EdgeID, DeviceID, ModelID, timestamp, action type, verification results
- **Dependencies**: 13.1.1
- **Estimated Effort**: 1 day

#### Subsection 13.1.3: Security Event and Recovery Audit
- **Files**: `impl/event_generation.go`, `impl/recovery.go`
- **Changes**:
  - Call audit-log on:
    - Security event creation and transmission attempts
    - Recovery actions (resync requests, storage corruption detection)
  - Include: EdgeID, DeviceID, EventID (if applicable), timestamp, action type, recovery reason
- **Dependencies**: 13.1.2
- **Estimated Effort**: 1 day

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~2 weeks
- **Epics**: 1 (Device-Agnostic Architecture), 2 (ML Lifecycle State Management)
- **Rationale**: Establishes the type system and state management foundation

### Phase 2: Core Workflows (Epics 3, 4, 5)
- **Duration**: ~3 weeks
- **Epics**: 3 (Reconciliation Loops), 4 (VM Protocol), 5 (Model Verification)
- **Rationale**: Implements core workflow orchestration and VM communication

### Phase 3: Event Handling (Epics 6, 7)
- **Duration**: ~2 weeks
- **Epics**: 6 (Inference and Event Generation), 7 (Security Event Storage and Delivery)
- **Rationale**: Implements inference and event handling

### Phase 4: Reliability (Epics 8, 9, 10)
- **Duration**: ~2 weeks
- **Epics**: 8 (Storage Integrity), 9 (Workflow Serialization), 10 (Offline Behavior)
- **Rationale**: Adds production reliability features

### Phase 5: Operations (Epics 11, 12, 13)
- **Duration**: ~1 week
- **Epics**: 11 (Observability), 12 (Configuration), 13 (Audit Logging)
- **Rationale**: Completes operational requirements

**Total Estimated Duration**: ~10 weeks

---

## Testing Strategy

### Unit Tests
- ML lifecycle state transitions and idempotency
- Reconciliation logic (each pass)
- Model verification gates
- Event delivery and queue management
- Workflow serialization and locking

### Integration Tests
- Full device lifecycle: discover → assign → dataset → model → inference
- Offline/reconnection: disconnect VM mid-workflow, verify queue/resume
- Storage corruption: inject corruption, verify recovery protocol
- Out-of-order events: send model before device registered, verify pending storage

### End-to-End Tests
- Deploy Edge + VM (local Docker Compose)
- Add mock devices (cameras, sensors, audio devices)
- Trigger full workflow via VM API
- Verify security events reach VM
- Verify metrics/logs are correct

### Chaos Tests
- Kill AI service randomly, verify circuit breaker
- Disconnect VM for random intervals, verify queue behavior
- Fill storage, verify backpressure and cleanup
- Corrupt database files, verify recovery

---

## Migration Notes

### Breaking Changes
- All `CameraID` references become `DeviceID`
- `CameraState` replaced by `MLLifecycleState`
- State machine interfaces change (device-agnostic)
- Service method signatures change (device-agnostic)

### Data Migration
- Existing camera state data must be migrated to ML lifecycle state
- Existing security events must be updated with `DeviceID` and `DeviceType`
- Existing snapshot requests must be migrated to data unit requests

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration, E2E, chaos)
- Gradual rollout to production with monitoring
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ All ML lifecycle states implemented and tested
2. ✅ All reconciliation loops implemented and tested
3. ✅ All VM protocol contracts implemented and tested
4. ✅ Security event handling meets production requirements (immediate persistence, ASAP delivery, immediate sync on reconnection)
5. ✅ Model verification gates all implemented and tested
6. ✅ Storage integrity and recovery protocol implemented and tested
7. ✅ Per-device workflow serialization implemented and tested
8. ✅ Offline behavior implemented and tested
9. ✅ Health snapshot API implemented
10. ✅ All configuration parameters implemented and validated
11. ✅ All timeouts enforced
12. ✅ Audit logging integrated
13. ✅ Full test coverage (unit, integration, E2E, chaos)
14. ✅ Documentation updated

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **All production-critical requirements from the workflow document must be implemented**
- **Device-agnostic implementation is mandatory** (not just camera support)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

