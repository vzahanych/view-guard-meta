# Edge Orchestrator Refactoring Plan

This document provides a comprehensive refactoring plan based on the state transition review findings. The plan is organized into epics, sections, and subsections to guide systematic improvements.

## Epic 1: State Machine Architecture Refactoring

**Priority:** Critical/High  
**Goal:** Split global state machine into connection-level and per-camera state machines to support independent multi-camera flows. This refactoring will also provide foundation for future device abstraction (Epic 12).

### Section 1.1: Multi-Level State Machine Design

#### Subsection 1.1.1: Design Connection-Level State Machine
- **Description:** Define connection-level states (disconnected, wireguard_connected, https_connected, authenticated, etc.)
- **Scope:** Connection-level states are global to the Edge appliance
- **Dependencies:** None
- **Related Findings:**
  - Finding #15: Global state machine blocks independent multi-camera flows

#### Subsection 1.1.2: Design Per-Camera State Machine
- **Description:** Define per-camera states (camera_discovered, camera_synced, waiting_for_screenshots, screenshot_set_ready, model_deployed, frame_processing, etc.)
- **Scope:** Each camera has its own independent state machine keyed by camera ID
- **Dependencies:** 1.1.1
- **Related Findings:**
  - Finding #15: Global state machine blocks independent multi-camera flows

#### Subsection 1.1.3: Implement State Machine Separation
- **Description:** Refactor state manager to maintain separate state machines for connection and per-camera
- **Scope:** 
  - Connection state: `EdgeConnectionState` type
  - Camera state: `CameraState` type with camera ID key
  - State manager maintains both state machines
- **Dependencies:** 1.1.1, 1.1.2
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:92-118`

#### Subsection 1.1.4: Update State Transition Logic
- **Description:** Update all state transition handlers to work with separated state machines
- **Scope:** Modify event handlers to transition appropriate state machine(s)
- **Dependencies:** 1.1.3
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:715-806`

#### Subsection 1.1.5: Update State Persistence
- **Description:** Persist both connection and per-camera states separately
- **Scope:** Update meta-storage to persist connection state and per-camera states
- **Dependencies:** 1.1.3
- **Related Findings:**
  - Finding #98: State persistence is fire-and-forget without error checking

#### Subsection 1.1.6: Add State Recovery Logic
- **Description:** On restart, recover connection state and all camera states
- **Scope:** Load both connection and camera states from meta-storage on startup
- **Dependencies:** 1.1.5

### Section 1.2: Disconnect Handling Alignment

#### Subsection 1.2.1: Fix HTTPS Disconnect Transitions
- **Description:** Ensure HTTPS disconnect properly transitions from `model_deployed`/`frame_processing` to `wireguard_connected`
- **Scope:** 
  - Update `handleHTTPSDisconnected` to stop frame processing
  - Transition per-camera states appropriately
  - Clean up per-camera resources
- **Dependencies:** 1.1.3
- **Related Findings:**
  - Finding #14: HTTPS disconnect doesn't transition from model_deployed/frame_processing
  - Finding #97: State transitions don't clean up per-camera state
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:438-546`

#### Subsection 1.2.2: Fix WireGuard Disconnect Transitions
- **Description:** Ensure WireGuard disconnect properly transitions all states to `disconnected`
- **Scope:** Update disconnect handler to stop all processing and transition all cameras appropriately
- **Dependencies:** 1.1.3, 1.2.1
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1244-1299`

#### Subsection 1.2.3: Implement Per-Camera State Cleanup
- **Description:** Clean up per-camera metadata, pending frames, and AI gateway state on disconnect
- **Scope:** 
  - Clear camera-specific state on disconnect
  - Stop and clean up frame processing goroutines
  - Clear pending snapshot requests per camera
- **Dependencies:** 1.2.1, 1.2.2
- **Related Findings:**
  - Finding #97: State transitions don't clean up per-camera state

### Section 1.3: State Consistency Improvements

#### Subsection 1.3.1: Fix State Persistence Error Handling
- **Description:** Add error checking for state persistence calls
- **Scope:** 
  - Check errors from `persistStateToStorage` calls
  - Implement retry logic for persistence failures
  - Add logging/alerting for persistence failures
- **Dependencies:** None
- **Related Findings:**
  - Finding #98: State persistence is fire-and-forget without error checking
- **Code Locations:**
  - Multiple call sites in `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`

#### Subsection 1.3.2: Handle Out-of-Order Model Deployment
- **Description:** Allow model deployment events in states other than `screenshot_set_ready`
- **Scope:** 
  - Update `handleModelDeployed` to handle deployment in various states
  - Store deployment for later use if not ready
  - Queue deployment for when camera reaches appropriate state
- **Dependencies:** 1.1.3
- **Related Findings:**
  - Finding #20: Model deployment events ignored if not in screenshot_set_ready state
  - Finding #100: handleModelDeployed only transitions if current status is screenshot_set_ready
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1172-1186`

#### Subsection 1.3.3: Document Missing States
- **Description:** Update documentation to include implemented but undocumented states
- **Scope:** Add documentation for `initializing`, `wg_connecting`, `http_connecting` states
- **Dependencies:** None
- **Related Findings:**
  - Finding #21: Documentation omits implemented states
  - Finding #101: Missing states not documented

---

## Epic 12: Device Abstraction Layer

**Priority:** High  
**Goal:** Prepare architecture for multiple IoT device types beyond CCTV cameras by introducing device abstraction and plugin architecture.

**Rationale:** Current architecture is camera-centric (`frameProcessing`, `screenshotSync`, `CCTV service`). Adding device abstraction early will enable easy extension to other IoT device types (sensors, access control, etc.) without major refactoring.

### Section 12.1: Device Type Abstraction

#### Subsection 12.1.1: Define Generic Device Interface
- **Description:** Create device-agnostic interface that abstracts device capabilities
- **Scope:** 
  - Define `Device` interface with lifecycle methods (discover, register, connect, disconnect)
  - Abstract capture/data collection mechanisms
  - Create device capability negotiation framework
  - Design device metadata schema (type, capabilities, status)
- **Dependencies:** None
- **Code Locations:**
  - New interface package: `edge/orchestrator/internal/device/device-iface.go`

#### Subsection 12.1.2: Refactor Camera as Device Implementation
- **Description:** Implement Camera as concrete `Device` interface implementation
- **Scope:** 
  - Wrap existing CCTV service as `CameraDevice` implementation
  - Map camera-specific operations to generic device interface
  - Maintain backward compatibility during transition
- **Dependencies:** 12.1.1
- **Code Locations:**
  - `edge/orchestrator/internal/iot/cctv/` (refactor as device implementation)

#### Subsection 12.1.3: Create Device Capability Framework
- **Description:** Design extensible capability system for different device types
- **Scope:** 
  - Define capability types (video_capture, sensor_readings, audio_capture, access_control, etc.)
  - Create capability negotiation protocol
  - Support capability queries and filtering
- **Dependencies:** 12.1.1

### Section 12.2: Device Plugin Architecture

#### Subsection 12.2.1: Design Device Plugin System
- **Description:** Create plugin/adapter pattern for adding new device types
- **Scope:** 
  - Define device plugin registration interface
  - Create device type registry
  - Support runtime device type registration
  - Design plugin discovery mechanism
- **Dependencies:** 12.1.1

#### Subsection 12.2.2: Implement Device Lifecycle Hooks
- **Description:** Define and implement standard device lifecycle hooks
- **Scope:** 
  - Discovery hooks (device detection, identification)
  - Registration hooks (initialization, capability reporting)
  - Data collection hooks (capture, streaming, polling)
  - Teardown hooks (cleanup, resource release)
- **Dependencies:** 12.2.1

### Section 12.3: Generic Data Pipeline

#### Subsection 12.3.1: Abstract Data Processing Pipeline
- **Description:** Create device-agnostic data processing framework
- **Scope:** 
  - Abstract "frame processing" to "device data processing"
  - Create generic data transformation pipeline
  - Support multiple data types (video frames, sensor readings, audio, structured events)
  - Design pluggable data processors
- **Dependencies:** 12.1.1, Epic 1 (state machine separation)

#### Subsection 12.3.2: Implement Device State Machines
- **Description:** Extend per-camera state machine to per-device state machine
- **Scope:** 
  - Generalize camera state machine to device state machine
  - Support device-type-specific state transitions
  - Maintain device-independent connection state machine
- **Dependencies:** Epic 1 (state machine refactoring), 12.1.2

---

## Epic 2: Event System Reliability

**Priority:** High  
**Goal:** Implement reliable event delivery with durable queue and retry semantics for critical events.

### Section 2.1: Event Bus Reliability

#### Subsection 2.1.1: Design Durable Event Queue
- **Description:** Design persistent event storage for critical events
- **Scope:** 
  - Identify critical events: `snapshot.requested`, `model.deployed`, connectivity events
  - Design queue structure (meta-storage or separate queue service)
  - Define event acknowledgment and retry semantics
- **Dependencies:** None
- **Related Findings:**
  - Finding #16: In-memory event bus drops events when buffers are full
  - Finding #106: Event bus channels can fill up and events are silently dropped

#### Subsection 2.1.2: Implement Event Persistence
- **Description:** Persist critical events before delivery
- **Scope:** 
  - Store critical events in meta-storage before publishing
  - Mark events as delivered/acknowledged after successful processing
  - Implement event replay mechanism for failed deliveries
- **Dependencies:** 2.1.1
- **Code Locations:**
  - `edge/orchestrator/internal/event-bus/inmemory/inmemory_event_bus.go`

#### Subsection 2.1.3: Implement Event Retry Logic
- **Description:** Add retry mechanism for failed event processing
- **Scope:** 
  - Retry failed event processing with exponential backoff
  - Set maximum retry attempts
  - Move to dead letter queue after max retries
- **Dependencies:** 2.1.2

#### Subsection 2.1.4: Add Event Bus Backpressure
- **Description:** Implement flow control to prevent event drops
- **Scope:** 
  - Increase event bus buffer size or make it configurable
  - Implement backpressure mechanism (block publishers when buffers full)
  - Add metrics for event drop rates
- **Dependencies:** None
- **Related Findings:**
  - Finding #106: Event bus channels can fill up and events are silently dropped
- **Code Locations:**
  - `edge/orchestrator/internal/event-bus/inmemory/inmemory_event_bus.go:71-104`

#### Subsection 2.1.5: Add Event Ordering Guarantees
- **Description:** Ensure critical events are processed in order
- **Scope:** 
  - Add sequence numbers to events
  - Process events in order for same event source
  - Handle out-of-order events gracefully
- **Dependencies:** 2.1.2
- **Related Findings:**
  - Finding #35: Workflow execution is concurrent-per-event, creating ordering hazards
  - Finding #38: Workflows can run out-of-order relative to state transitions

### Section 2.2: Workflow Execution Improvements

#### Subsection 2.2.1: Serialize Workflow Execution
- **Description:** Execute workflows sequentially or with proper ordering
- **Scope:** 
  - Remove concurrent-per-event workflow execution
  - Execute workflows in event order
  - Add workflow queue if needed
- **Dependencies:** 2.1.5
- **Related Findings:**
  - Finding #35: Workflow execution is concurrent-per-event
  - Finding #38: Workflows can run out-of-order
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:388-633`

#### Subsection 2.2.2: Add Workflow Concurrency Control
- **Description:** Limit concurrent workflow execution to prevent resource exhaustion
- **Scope:** 
  - Implement worker pool for workflow execution
  - Set maximum concurrent workflows
  - Queue workflows when at capacity
- **Dependencies:** 2.2.1
- **Related Findings:**
  - Finding #93: executeWorkflow spawns unbounded goroutines

#### Subsection 2.2.3: Make Workflows Idempotent
- **Description:** Ensure all workflows are idempotent to handle duplicate execution
- **Scope:** 
  - Review all workflow implementations
  - Add idempotency checks where needed
  - Test workflows with duplicate events
- **Dependencies:** None

#### Subsection 2.2.4: Handle Missing Event Consumers
- **Description:** Add consumers for published but unhandled events
- **Scope:** 
  - Add handler for `model.deployment.status` events
  - Review all published events for consumers
  - Add observability for unhandled events
- **Dependencies:** None
- **Related Findings:**
  - Finding #41: model.deployment.status events published but no consumer
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:555-620`

### Section 2.3: Event Sourcing and Audit Trail

#### Subsection 2.3.1: Implement Event Sourcing
- **Description:** Persist all events as immutable log for audit and replay
- **Scope:** 
  - Store all events in append-only event log
  - Support event replay for debugging and recovery
  - Enable event-based state reconstruction
  - Add event versioning for schema evolution
- **Dependencies:** 2.1.2 (event persistence)
- **Rationale:** Essential for security domain audit requirements and incident investigation

#### Subsection 2.3.2: Add Audit Trail Support
- **Description:** Enable event-based audit trail reconstruction
- **Scope:** 
  - Support time-based event queries
  - Enable event filtering by device, operation, user
  - Support event correlation across devices
  - Add audit log export capabilities
- **Dependencies:** 2.3.1

#### Subsection 2.3.3: Implement Cross-Device Event Correlation
- **Description:** Support correlation IDs and distributed tracing across devices
- **Scope:** 
  - Add correlation IDs to events for multi-device workflows
  - Enable event tracing across device boundaries
  - Support distributed workflow orchestration (future multi-device scenarios)
  - Add event dependency tracking
- **Dependencies:** 2.3.1

---

## Epic 3: Security and Data Provenance

**Priority:** Critical/High  
**Goal:** Enforce capture provenance, fix TLS configuration, add proper authentication/authorization, implement comprehensive audit logging, RBAC, data encryption, and tamper detection for security domain compliance.

### Section 3.1: Capture Provenance Enforcement

#### Subsection 3.1.1: Remove Client-Supplied image_data
- **Status:** ✅ DONE
- **Description:** Remove ability to inject image data directly via API
- **Scope:** 
  - Remove `image_data` field from POST `/api/screenshots` endpoint
  - Only allow screenshots captured via CCTV service
  - Update API documentation
- **Dependencies:** None
- **Related Findings:**
  - Finding #17: Web gateway accepts client-supplied image_data violating security model
  - Finding #110: image_data allows data poisoning bypassing CCTV capture
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:260-350`
- **Refactoring Details:**
  - Removed `ImageData` field from request struct in `handleSaveScreenshot` function
  - Removed conditional logic that processed client-supplied `image_data` (lines 297-310)
  - Enforced screenshot capture exclusively via CCTV service - removed the else branch and made CCTV service capture mandatory
  - Updated frontend components (`Screenshots.tsx` and `CameraViewer.tsx`) to remove `image_data` from POST requests
  - Screenshots are now always captured directly from cameras via CCTV service, ensuring capture provenance and preventing data poisoning attacks
  - The `decodeBase64Image` function remains in the codebase but is no longer used (can be removed in future cleanup)

#### Subsection 3.1.2: Implement Capture Token Verification (Alternative)
- **Status:** ❌ CANCELLED - Not needed
- **Description:** If image_data must be supported, implement signed capture tokens
- **Scope:** 
  - Generate signed tokens from CCTV service after capture
  - Verify tokens in web gateway before accepting image_data
  - Require tokens for all image uploads
- **Dependencies:** 3.1.1 (if alternative approach chosen)
- **Reason for Cancellation:**
  - Subsection 3.1.1 was implemented, which completely removed client-supplied `image_data` from the API
  - This alternative approach was only needed if we wanted to keep `image_data` support but secure it with tokens
  - Since we chose to remove `image_data` entirely (more secure approach), this token verification alternative is no longer applicable
  - All screenshots are now captured exclusively via CCTV service, eliminating the need for token-based verification of client-supplied data

#### Subsection 3.1.3: Add Web Gateway Authentication
- **Status:** ✅ DONE
- **Description:** Add authentication/authorization to web gateway endpoints
- **Scope:** 
  - Implement authentication middleware
  - Add authorization checks for sensitive operations
  - Document security model for web gateway access
- **Dependencies:** None
- **Related Findings:**
  - Finding #53: Screenshot endpoints return data without access control
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/types/types.go`
  - `edge/orchestrator/internal/web-gateway/impl/web-gateway-impl.go`
- **Refactoring Details:**
  - Added `AuthConfig` struct to `WebGatewayConfig` with fields for enabling/disabling auth, API key, and public endpoints
  - Implemented `createAuthMiddleware` function that validates Bearer token API keys from Authorization header
  - Applied authentication middleware globally to all API routes when auth is enabled
  - Configured default public endpoints (`/api/health`, `/api/status`) that don't require authentication
  - Added support for custom public endpoints via configuration
  - Authentication is opt-in (disabled by default) - when enabled, all endpoints except public ones require valid API key
  - Added security logging for authentication failures (invalid/missing API keys)
  - Added warning logs when authentication is disabled to raise awareness of security implications
  - API key is provided via Bearer token format: `Authorization: Bearer <api_key>`
  - Security model: All sensitive endpoints (screenshots, cameras, events, config) are protected when auth is enabled

### Section 3.2: TLS and Certificate Management

#### Subsection 3.2.1: Fix Dev Mode TLS Behavior
- **Description:** Fix or properly implement dev mode TLS configuration
- **Scope:** 
  - Either require certificates even in dev mode
  - Or implement explicit HTTP-only dev server (not HTTPS)
  - Add clear warnings/documentation for insecure dev mode
- **Dependencies:** None
- **Related Findings:**
  - Finding #24: VM gateway HTTPS server dev mode likely cannot start
  - Finding #123: Fix/clarify dev-mode TLS behavior
- **Code Locations:**
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:291-337`

#### Subsection 3.2.2: Add Certificate Validation
- **Description:** Validate certificates at startup (expiration, format, identity)
- **Scope:** 
  - Validate certificate expiration dates
  - Verify certificate format and validity
  - Check certificate identity matches expected
  - Fail fast on invalid certificates
- **Dependencies:** None
- **Related Findings:**
  - Finding #90: No validation of required certificates at startup
  - Finding #112: TLS certificates not validated for expiration or revocation
- **Code Locations:**
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:238-300`

#### Subsection 3.2.3: Add Certificate Expiration Monitoring
- **Description:** Monitor certificate expiration and warn before expiry
- **Scope:** 
  - Check certificate expiration periodically
  - Log warnings when certificates approaching expiry
  - Alert on certificate expiration
- **Dependencies:** 3.2.2

### Section 3.3: Model Deployment Security

#### Subsection 3.3.1: Add Model Integrity Verification
- **Description:** Validate model file integrity before deployment
- **Scope:** 
  - Calculate and verify checksums (SHA256) for model files
  - Compare against metadata checksum if provided
  - Reject models with invalid checksums
- **Dependencies:** None
- **Related Findings:**
  - Finding #113: Model deployment only validates size, not content integrity
- **Code Locations:**
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:654-757`

#### Subsection 3.3.2: Add Model Signature Verification (Future)
- **Description:** Implement signature verification for model files
- **Scope:** 
  - Support signed models from VM
  - Verify signatures before deployment
  - Reject unsigned or invalidly signed models
- **Dependencies:** 3.3.1

### Section 3.4: Configuration Security

#### Subsection 3.4.1: Remove Hardcoded Defaults
- **Description:** Remove hardcoded fallback values, especially Edge ID
- **Scope:** 
  - Require Edge ID in configuration
  - Remove hardcoded "edge-dev-001" default
  - Fail startup if required config missing
- **Dependencies:** None
- **Related Findings:**
  - Finding #114: Edge ID defaults to "edge-dev-001" in multiple places
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:998`, `1608`

#### Subsection 3.4.2: Document Security Limitations
- **Description:** Document security limitations of current implementation
- **Scope:** 
  - Document hard-coded MinIO Secure: false setting
  - Document hard-coded bucket name
  - Add security warnings for production use
- **Dependencies:** None
- **Related Findings:**
  - Finding #43: Object storage hard-coded to Secure: false and hard-coded bucket name
- **Code Locations:**
  - `edge/orchestrator/internal/object-storage/minio-imp/minio_object_storage.go:48-56`

### Section 3.5: Audit Logging Framework

#### Subsection 3.5.1: Implement Tamper-Proof Audit Log
- **Description:** Implement comprehensive audit logging for all security-sensitive operations
- **Scope:** 
  - Log all device data access (reads, writes, deletions)
  - Log model deployments and configuration changes
  - Log authentication and authorization decisions
  - Implement tamper-proof audit log storage (append-only, cryptographic hashing)
- **Dependencies:** Epic 2, Section 2.3 (event sourcing)
- **Rationale:** Critical for security domain compliance and forensic investigation

#### Subsection 3.5.2: Add Audit Log Integration
- **Description:** Support external SIEM integration and audit log export
- **Scope:** 
  - Support standard audit log formats (CEF, JSON)
  - Enable real-time audit log streaming to external systems
  - Support audit log query API for compliance reporting
  - Add audit log retention policies
- **Dependencies:** 3.5.1

### Section 3.6: Role-Based Access Control (RBAC)

#### Subsection 3.6.1: Define RBAC Roles and Permissions
- **Description:** Design role-based access control system
- **Scope:** 
  - Define roles (admin, operator, viewer, device)
  - Define permissions per role (read, write, configure, deploy)
  - Support fine-grained device-level permissions
  - Design permission inheritance and delegation
- **Dependencies:** None
- **Rationale:** Essential for multi-user security domain deployments

#### Subsection 3.6.2: Implement Authorization Middleware
- **Description:** Add authorization checks for all sensitive operations
- **Scope:** 
  - Implement authorization middleware for API endpoints
  - Add permission checks for device operations
  - Add permission checks for model deployment
  - Add permission checks for configuration changes
- **Dependencies:** 3.6.1
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go`
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go`

### Section 3.7: Data-at-Rest Encryption

#### Subsection 3.7.1: Implement Storage Encryption
- **Description:** Encrypt sensitive data at rest in meta-storage and object storage
- **Scope:** 
  - Encrypt sensitive configuration data (credentials, certificates, keys)
  - Encrypt device configuration and metadata
  - Encrypt screenshots/frames in object storage
  - Support encryption key rotation
- **Dependencies:** None
- **Rationale:** Defense-in-depth for security-critical data

#### Subsection 3.7.2: Design Key Management Strategy
- **Description:** Design key management and storage approach
- **Scope:** 
  - Design key storage mechanism (file-based, HSM consideration for future)
  - Define key rotation policies
  - Support multiple encryption keys (for key rotation)
  - Document key backup and recovery procedures
- **Dependencies:** 3.7.1

### Section 3.8: Tamper Detection

#### Subsection 3.8.1: Implement Configuration Integrity Checks
- **Description:** Detect unauthorized modifications to configuration files
- **Scope:** 
  - Calculate and store configuration file checksums
  - Verify checksums at startup and periodically
  - Alert on configuration tampering
  - Support configuration file signing (future enhancement)
- **Dependencies:** None

#### Subsection 3.8.2: Add Device Data Provenance Verification
- **Description:** Verify device data provenance chains and detect anomalies
- **Scope:** 
  - Maintain provenance chain for all device data
  - Verify data integrity from capture to storage
  - Detect suspicious activity patterns (unusual access, mass deletions)
  - Alert on potential security incidents
- **Dependencies:** 3.1.1 (capture provenance), 3.5.1 (audit logging)

### Section 3.9: Device Identity, Provisioning, and Attestation

#### Subsection 3.9.1: Define Device Identity Model
- **Description:** Define how devices are uniquely identified and authenticated to the Edge
- **Scope:**
  - Define immutable device identity (device ID, device type, manufacturer/model)
  - Define device credentials (certs/keys/tokens) lifecycle (issue, rotate, revoke)
  - Define onboarding and decommissioning procedures
  - Support identity binding to device groups/tenants (if multi-tenancy is enabled)
- **Dependencies:** Epic 12 (device abstraction)
- **Rationale:** A consistent identity model is the foundation for zero-trust device access and future multi-IoT support

#### Subsection 3.9.2: Implement Secure Provisioning Flow
- **Description:** Design secure device onboarding/provisioning with least privilege
- **Scope:**
  - Provisioning protocol (out-of-band enrollment, one-time tokens, mTLS bootstrap)
  - Rate limiting and abuse controls for onboarding endpoints
  - Certificate issuance/rotation/revocation strategy for devices
  - Audit logging for all provisioning operations
- **Dependencies:** 3.9.1, 3.5.1 (audit logging), 3.6.2 (authorization middleware)

#### Subsection 3.9.3: Add Hardware / Software Attestation (Future)
- **Description:** Add support for device attestation to verify device integrity before granting access
- **Scope:**
  - Define attestation formats (TPM-based, signed claims, etc.)
  - Verify device integrity posture during onboarding and periodically
  - Support quarantine/deny-list for failing devices
  - Emit security events for attestation failures
- **Dependencies:** 3.9.1, Epic 10, Section 10.3 (security monitoring)

### Section 3.10: Secure Updates and Supply Chain Security

#### Subsection 3.10.1: Implement Signed Update and Rollback Strategy
- **Description:** Ensure Edge updates are authenticated, integrity-protected, and safely recoverable
- **Scope:**
  - Signed release artifacts and signature verification in updater
  - Safe rollout strategy (staged rollout, health checks, automatic rollback)
  - Version pinning and downgrade protection policies
  - Audit log for update operations and outcomes
- **Dependencies:** 3.5.1 (audit logging)
- **Rationale:** Security domain systems require trustworthy updates with deterministic recovery

#### Subsection 3.10.2: Add SBOM and Dependency Governance
- **Description:** Track and govern software dependencies to reduce supply-chain risk
- **Scope:**
  - Generate SBOM for Edge builds (and ideally VM side too)
  - Track vulnerabilities (CVE scanning) and upgrade policies
  - Define dependency allow/deny rules and review process
  - Document secure build/release practices
- **Dependencies:** None

#### Subsection 3.10.3: Secure Secrets Management
- **Description:** Centralize secret storage and enable rotation without redeployments
- **Scope:**
  - Define secret storage mechanism (encrypted at rest, minimal access surface)
  - Support secret rotation (device credentials, API keys, storage creds)
  - Prevent secret leakage in logs/metrics
  - Provide operational runbooks for rotation and recovery
- **Dependencies:** 3.7.1 (storage encryption), 3.7.2 (key management)

### Section 3.11: Plugin Sandboxing and Least Privilege (Device Extensibility Security)

#### Subsection 3.11.1: Define Plugin Permission Model
- **Description:** Define capabilities/permissions for device plugins and enforce least privilege
- **Scope:**
  - Define what plugins can access (network, storage, device APIs, event bus)
  - Define permission scopes (per-device, per-tenant/group, per-data-type)
  - Define plugin identity and signing requirements
  - Enforce audit logging for privileged plugin actions
- **Dependencies:** Epic 12 (device plugin architecture), 3.5.1 (audit logging)

#### Subsection 3.11.2: Implement Plugin Isolation Strategy (Future)
- **Description:** Prevent plugins from compromising the Edge via sandboxing/isolation
- **Scope:**
  - Evaluate isolation mechanisms (process isolation, containers, seccomp/AppArmor)
  - Define safe IPC interfaces for plugin <-> core communication
  - Add runtime resource limits for plugins (ties to per-device quotas)
  - Add monitoring and alerts for plugin misbehavior
- **Dependencies:** 3.11.1, Epic 4, Section 4.4 (resource quotas), Epic 10, Section 10.3 (security monitoring)

---

## Epic 4: Concurrency and Resource Management

**Priority:** Critical/High  
**Goal:** Fix race conditions, prevent goroutine leaks, and ensure proper resource cleanup.

### Section 4.1: Concurrency Safety

#### Subsection 4.1.1: Fix AIProcessingActive Race Condition
- **Description:** Add proper locking for `m.state.AIProcessingActive` access
- **Scope:** 
  - Protect all reads/writes to `AIProcessingActive` with mutex
  - Ensure `executeWorkflow` uses proper locking
- **Dependencies:** None
- **Related Findings:**
  - Finding #18: Data race risk with AIProcessingActive mutation
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:595-600`

#### Subsection 4.1.2: Fix pendingSync Synchronization
- **Description:** Add proper synchronization for `pendingSync` access
- **Scope:** 
  - Protect `pendingSync` with mutex or atomic operations
  - Ensure all access is synchronized
- **Dependencies:** None
- **Related Findings:**
  - Finding #19: pendingSync accessed without synchronization
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1065-1084`, `1900-2022`

#### Subsection 4.1.3: Fix pendingSnapshotRequests Locking
- **Description:** Fix incorrect locking pattern in `GetAllPendingSnapshotRequests`
- **Scope:** 
  - Fix write under read lock pattern
  - Use proper write lock for map mutations
- **Dependencies:** None
- **Related Findings:**
  - Finding #86: pendingSnapshotRequests write under read lock
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/snapshot_request_storage.go:93-127`

#### Subsection 4.1.4: Add Race Condition Tests
- **Description:** Add tests with race detector to verify fixes
- **Scope:** 
  - Run `go test -race` on state manager
  - Add concurrent access test scenarios
  - Verify no race conditions remain
- **Dependencies:** 4.1.1, 4.1.2, 4.1.3
- **Related Findings:**
  - Recommendation #69: Add race detector coverage

### Section 4.2: Goroutine and Resource Management

#### Subsection 4.2.1: Fix stopAllFrameProcessing Resource Cleanup
- **Status:** ✅ DONE
- **Description:** Wait for goroutines to finish before clearing maps
- **Scope:** 
  - Wait for waitgroup before clearing `frameProcessingActive` map
  - Ensure all goroutines have completed before cleanup
- **Dependencies:** None
- **Related Findings:**
  - Finding #83: stopAllFrameProcessing doesn't wait for goroutines to finish
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1365-1381`
- **Refactoring Details:**
  - Added dedicated `frameProcessingWg` WaitGroup specifically for frame processing goroutines to enable proper synchronization
  - Updated `startFrameProcessingForCamera` to call both `m.wg.Add(1)` (shared waitgroup) and `m.frameProcessingWg.Add(1)` (frame processing waitgroup)
  - Updated `frameProcessingLoop` to call both `m.wg.Done()` and `m.frameProcessingWg.Done()` on exit
  - Modified `stopAllFrameProcessing` to properly wait for goroutines to finish:
    - Cancel all frame processing contexts first to signal goroutines to stop
    - Unlock mutex to allow goroutines to finish and call `frameProcessingWg.Done()`
    - Wait for `frameProcessingWg` with a 10-second timeout using channel-based pattern
    - Lock mutex again and clear the map only after goroutines have finished
  - Added logging for timeout cases and completion status
  - This ensures proper resource cleanup and prevents race conditions where the map is cleared while goroutines are still running

#### Subsection 4.2.2: Fix Stop() Method Shutdown Ordering
- **Status:** ✅ DONE
- **Description:** Explicitly stop frame processing before service shutdown
- **Scope:** 
  - Call `stopAllFrameProcessing()` before canceling main context
  - Ensure proper shutdown ordering
  - Wait for frame processing to stop before proceeding
- **Dependencies:** 4.2.1
- **Related Findings:**
  - Finding #84: Stop() doesn't explicitly stop frame processing first
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:346-369`
- **Refactoring Details:**
  - Modified `Stop()` method to call `stopAllFrameProcessing()` before canceling the main context
  - Ensures frame processing goroutines are stopped gracefully and wait for completion before proceeding with shutdown
  - Added comment explaining the shutdown ordering rationale
  - Shutdown sequence is now: stop frame processing → cancel main context → wait for all goroutines → complete shutdown
  - This prevents race conditions and ensures frame processing resources are properly cleaned up before other shutdown operations
  - Leverages the fix from 4.2.1 which ensures `stopAllFrameProcessing()` properly waits for goroutines to finish

#### Subsection 4.2.3: Prevent Duplicate Frame Processing Goroutines
- **Status:** ✅ DONE
- **Description:** Fix race condition in `startFrameProcessingForCamera`
- **Scope:** 
  - Move existence check inside critical section
  - Use atomic operations or proper locking
  - Ensure only one goroutine per camera can be started
- **Dependencies:** None
- **Related Findings:**
  - Finding #85: Goroutine leak risk in startFrameProcessingForCamera
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1303-1345`
- **Refactoring Details:**
  - Fixed race condition by implementing double-check locking pattern
  - First check: Quick read-lock check to see if camera is already processing (early exit if found)
  - External validation: Perform camera validation (GetCamera, check enabled) outside the lock to avoid blocking other operations
  - Second check: Re-check existence with write lock before adding to map - prevents duplicate goroutines if another thread started processing during validation
  - Changed from holding write lock throughout to: read lock → validate → write lock → re-check → add
  - This ensures that even if two goroutines call this function concurrently for the same camera, only one will successfully start frame processing
  - Added comment explaining thread-safety and duplicate prevention
  - Prevents goroutine leaks by ensuring only one frame processing goroutine per camera can exist

#### Subsection 4.2.4: Add Goroutine Leak Tests
- **Status:** ✅ DONE
- **Description:** Add tests to verify goroutines are properly cleaned up
- **Scope:** 
  - Test frame processing goroutine cleanup
  - Test shutdown scenarios
  - Verify no goroutine leaks
- **Dependencies:** 4.2.1, 4.2.2, 4.2.3
- **Related Findings:**
  - Recommendation #129: Add tests for goroutine cleanup
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl_test.go`
  - `edge/orchestrator/Makefile` (simplified mock generation)
  - `edge/orchestrator/internal/*/mocks/` (generated mocks, co-located with each service)
  - `edge/orchestrator/internal/*/*-iface.go` and `*_gateway.go` (interface files with `//go:generate` directives)
- **Refactoring Details:**
  - Set up automated mock generation using `go.uber.org/mock` (formerly gomock) with `//go:generate` directives
  - Added `//go:generate` directives directly in each interface file (e.g., `cctv-iface.go`, `vm_gateway.go`)
  - Each interface file contains: `//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_*.go -package=mocks`
  - Simplified Makefile: `generate-mocks` target now just runs `go generate ./...` which discovers all `//go:generate` directives
  - Mocks are generated into each service's own `mocks/` directory (e.g., `internal/iot/cctv/mocks/`)
  - This co-location approach keeps mocks close to their corresponding service interfaces for better organization
  - Each service owns its mock, making it easier to maintain and understand dependencies
  - Benefits: No need to maintain mock generation commands in Makefile, directives are co-located with interfaces, can generate individual service mocks with `go generate ./internal/iot/cctv/...`
  - Implemented comprehensive goroutine leak tests using `runtime.NumGoroutine()`:
    - `TestFrameProcessingGoroutineCleanup`: Verifies single camera frame processing goroutine cleanup
    - `TestStopAllFrameProcessingCleanup`: Verifies cleanup of multiple frame processing goroutines
    - `TestShutdownOrdering`: Verifies proper shutdown ordering (frame processing stops before context cancellation)
    - `TestDuplicateGoroutinePrevention`: Verifies that concurrent calls don't create duplicate goroutines
  - Tests use generated mocks instead of manual mocks, making them maintainable and scalable
  - Added `waitForGoroutines` utility function to wait for goroutines to stabilize during cleanup
  - All tests verify that goroutine counts return to initial levels after cleanup, preventing resource leaks
  - Mock generation can be run with `make generate-mocks` and should be run when service interfaces change

### Section 4.3: Resource Leak Prevention

#### Subsection 4.3.1: Fix Object Storage Stream Closing
- **Description:** Ensure object storage readers are always closed
- **Scope:** 
  - Use defer for Close() calls
  - Ensure Close() is called even on error paths
  - Add tests for resource cleanup
- **Dependencies:** None
- **Related Findings:**
  - Finding #33: Object storage streams not closed on error paths
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:149-174`

#### Subsection 4.3.2: Add Resource Monitoring
- **Description:** Add monitoring for goroutine counts and resource usage
- **Scope:** 
  - Track active goroutines
  - Monitor file descriptor usage
  - Alert on resource leaks
- **Dependencies:** None

### Section 4.4: Per-Device Resource Quotas

#### Subsection 4.4.1: Implement Resource Limits per Device
- **Description:** Implement resource quotas to prevent single device from exhausting resources
- **Scope:** 
  - Set memory limits per device (processing, buffering)
  - Set CPU limits per device (processing time, priority)
  - Set storage quotas per device (frame storage, snapshot storage)
  - Set bandwidth limits per device (network traffic)
- **Dependencies:** Epic 12 (device abstraction)
- **Rationale:** Critical for multi-device scalability - prevents one misbehaving device from impacting others

#### Subsection 4.4.2: Add Resource Usage Metrics
- **Description:** Track and report resource usage per device
- **Scope:** 
  - Monitor resource usage per device over time
  - Track quota utilization and thresholds
  - Alert when devices approach quota limits
  - Support quota adjustment and dynamic limits
- **Dependencies:** 4.4.1

#### Subsection 4.4.3: Implement Fair Scheduling
- **Description:** Ensure fair resource allocation across devices
- **Scope:** 
  - Implement fair scheduling for device processing
  - Prevent device starvation
  - Support priority-based scheduling (e.g., security-critical devices)
  - Balance load across available resources
- **Dependencies:** 4.4.1

---

## Epic 5: Error Handling and Recovery

**Priority:** High/Medium  
**Goal:** Implement comprehensive error handling, recovery mechanisms, and health monitoring.

### Section 5.1: Frame Processing Error Handling

#### Subsection 5.1.1: Add Frame Capture Failure Monitoring
- **Description:** Monitor and alert on persistent frame capture failures
- **Scope:** 
  - Track consecutive frame capture failures per camera
  - Set threshold for error state transition
  - Add health monitoring for frame capture
- **Dependencies:** None
- **Related Findings:**
  - Finding #91: Frame capture errors don't trigger recovery mechanism
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1412-1472`

#### Subsection 5.1.2: Implement Frame Processing Recovery
- **Description:** Add recovery mechanism for frame processing failures
- **Scope:** 
  - Retry frame capture with backoff
  - Transition camera to error state after threshold
  - Implement recovery workflow for camera error state
- **Dependencies:** 5.1.1

#### Subsection 5.1.3: Add Storage Operation Timeouts
- **Description:** Add explicit timeouts for object storage operations
- **Scope:** 
  - Add context with timeout for storage operations
  - Prevent blocking on slow storage
  - Handle timeout errors gracefully
- **Dependencies:** None
- **Related Findings:**
  - Finding #92: Object storage operations don't have explicit timeouts
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1432-1447`

### Section 5.2: Camera Discovery Error Handling

#### Subsection 5.2.1: Add Camera Discovery Recovery
- **Description:** Implement retry logic for camera discovery failures
- **Scope:** 
  - Retry camera discovery on failure
  - Add exponential backoff
  - Transition to recovery state instead of permanent error
- **Dependencies:** None
- **Related Findings:**
  - Finding #99: Camera discovery errors don't trigger recovery workflow
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:657-688`

#### Subsection 5.2.2: Improve Error State Handling
- **Description:** Allow recovery from error states
- **Scope:** 
  - Add recovery workflows for error states
  - Allow retry transitions from error states
  - Don't leave system stuck in error state
- **Dependencies:** 5.2.1

### Section 5.3: Configuration Validation

#### Subsection 5.3.1: Add Comprehensive Configuration Validation
- **Description:** Validate configuration values at startup
- **Scope:** 
  - Validate timeout values (positive, reasonable ranges)
  - Validate URL formats
  - Check for conflicting settings
  - Fail fast on invalid configuration
- **Dependencies:** None
- **Related Findings:**
  - Finding #94: Configuration validation is minimal
  - Recommendation #128: Validate configuration completeness

#### Subsection 5.3.2: Make Frame Processing Interval Configurable
- **Description:** Allow frame processing interval to be configured
- **Scope:** 
  - Add interval to configuration file
  - Use configured value instead of hardcoded 30 seconds
  - Validate interval value (minimum, maximum)
- **Dependencies:** 5.3.1
- **Related Findings:**
  - Finding #87: Frame processing interval hardcoded to 30 seconds
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:181`

---

## Epic 6: Data Integrity and Media Handling

**Priority:** High  
**Goal:** Fix content-type inconsistencies, implement proper thumbnail handling, and ensure data integrity.

### Section 6.1: Screenshot Content-Type Handling

#### Subsection 6.1.1: Fix Content-Type Detection and Storage
- **Description:** Properly detect, store, and return correct content types
- **Scope:** 
  - Detect actual image format from file data, not just extension
  - Store content-type with screenshot metadata
  - Return correct Content-Type header in API responses
- **Dependencies:** None
- **Related Findings:**
  - Finding #26: Content-type inconsistencies (JPEG vs PNG)
  - Finding #111: Image encoding/decoding assumes JPEG format
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:165-175`, `297-349`, `671-722`

#### Subsection 6.1.2: Fix Base64 Encoding Format
- **Description:** Use correct format in base64 data URLs
- **Scope:** 
  - Use actual image format in `data:image/{format};base64,...` URLs
  - Don't assume JPEG for all images
  - Match stored format with returned format
- **Dependencies:** 6.1.1

#### Subsection 6.1.3: Add Content-Type Validation Tests
- **Description:** Add tests for PNG/JPEG content-type correctness
- **Scope:** 
  - Test PNG screenshot upload and retrieval
  - Test JPEG screenshot upload and retrieval
  - Verify correct content-type in responses
- **Dependencies:** 6.1.1, 6.1.2
- **Related Findings:**
  - Recommendation #71: Add tests for PNG/JPEG content-type correctness

### Section 6.2: Thumbnail Implementation

#### Subsection 6.2.1: Implement Thumbnail Generation
- **Description:** Generate and store thumbnails for screenshots
- **Scope:** 
  - Generate thumbnails when screenshots are saved
  - Store thumbnails in object storage
  - Use thumbnail key consistently
- **Dependencies:** None
- **Related Findings:**
  - Finding #25: Thumbnails referenced but not actually stored
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:334-377`

#### Subsection 6.2.2: Remove Thumbnail References (Alternative)
- **Description:** If thumbnails not needed, remove all thumbnail references
- **Scope:** 
  - Remove thumbnailKey from API responses
  - Remove thumbnail loading logic
  - Update API documentation
- **Dependencies:** None (alternative to 6.2.1)

### Section 6.3: API Improvements

#### Subsection 6.3.1: Implement Real Pagination
- **Description:** Implement pagination at storage layer
- **Scope:** 
  - Update `ListScreenshots` to support limit/offset
  - Implement cursor-based pagination (preferred) or offset-based
  - Return pagination metadata (total count, has_more, etc.)
- **Dependencies:** None
- **Related Findings:**
  - Finding #34: API implies pagination but storage layer ignores it
- **Code Locations:**
  - `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go:562-621`
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:50-59`

#### Subsection 6.3.2: Make Thumbnails Opt-In for List Endpoints
- **Description:** Don't include base64 thumbnails by default in list responses
- **Scope:** 
  - Add query parameter to opt-in to thumbnails (e.g., `?include_thumbnails=true`)
  - Only include thumbnails when requested
  - Reduce payload size for default list requests
- **Dependencies:** 6.2.1 or 6.2.2
- **Related Findings:**
  - Finding #32: List endpoints include base64 thumbnails by default (large payloads)
- **Code Locations:**
  - `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:69-105`

---

## Epic 7: Performance and Memory Optimization

**Priority:** High/Medium  
**Goal:** Optimize memory usage, implement batching, and improve performance for large datasets.

### Section 7.1: Screenshot Sync Optimization

#### Subsection 7.1.1: Implement Screenshot Batching
- **Description:** Batch screenshot sync instead of loading all into memory
- **Scope:** 
  - Split large screenshot sets into batches (e.g., 10-20 screenshots per batch)
  - Process batches sequentially
  - Stream batches to VM instead of single large request
- **Dependencies:** None
- **Related Findings:**
  - Finding #104: syncScreenshotsToVM loads all images into memory at once
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:808-955`

#### Subsection 7.1.2: Implement Streaming for Large Requests
- **Description:** Stream screenshot data instead of base64 encoding everything
- **Scope:** 
  - Use multipart/form-data for large syncs
  - Stream images directly without base64 encoding
  - Reduce memory footprint for large syncs
- **Dependencies:** 7.1.1 (optional enhancement)

### Section 7.2: Capability Sync Optimization

#### Subsection 7.2.1: Implement Incremental Capability Sync
- **Description:** Only sync cameras when changes are detected
- **Scope:** 
  - Track last sync timestamp per camera
  - Compare current camera state with last sync
  - Only sync changed cameras
- **Dependencies:** None
- **Related Findings:**
  - Finding #105: Capability sync processes all cameras every time
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1927-2022`

#### Subsection 7.2.2: Add Change Detection for Cameras
- **Description:** Detect camera configuration changes
- **Scope:** 
  - Compare camera metadata with last sync
  - Track changes to enabled/disabled status
  - Track new/removed cameras
- **Dependencies:** 7.2.1

### Section 7.3: Memory Management Improvements

#### Subsection 7.3.1: Optimize List Endpoint Memory Usage
- **Description:** Reduce memory usage for screenshot list endpoints
- **Scope:** 
  - Don't load full images for list endpoints
  - Use pagination to limit results
  - Stream responses when possible
- **Dependencies:** 6.3.1 (pagination), 6.3.2 (opt-in thumbnails)
- **Related Findings:**
  - Finding #32: List endpoints load many objects into memory

#### Subsection 7.3.2: Add Memory Usage Monitoring
- **Description:** Monitor memory usage and alert on high usage
- **Scope:** 
  - Track memory usage per operation
  - Add metrics for memory usage
  - Alert on memory pressure
- **Dependencies:** None

---

## Epic 8: Frame Lifecycle and Storage Management

**Priority:** Medium  
**Goal:** Clarify frame lifecycle ownership and implement retention policies.

### Section 8.1: Frame Lifecycle Ownership

#### Subsection 8.1.1: Document Frame Lifecycle Ownership
- **Description:** Document who owns frame cleanup (AI service vs AI gateway vs state manager)
- **Scope:** 
  - Clarify responsibilities in documentation
  - Update state-transition.md with frame lifecycle details
- **Dependencies:** None
- **Related Findings:**
  - Finding #39: AIGateway behavior diverges from documented frame lifecycle
  - Recommendation #8: Decide and document frame lifecycle ownership

#### Subsection 8.1.2: Align Implementation with Documentation
- **Description:** Implement frame cleanup according to documented ownership
- **Scope:** 
  - Either implement cleanup in AI gateway as documented
  - Or update documentation to match current implementation
  - Ensure consistency between code and docs
- **Dependencies:** 8.1.1
- **Code Locations:**
  - `edge/orchestrator/internal/ai-gateway/impl/ai-gateway-impl.go:397-425`

#### Subsection 8.1.3: Implement Frame Retention Policies
- **Description:** Add configurable retention policies for frames
- **Scope:** 
  - Configure retention period for normal frames
  - Configure retention period for security event frames
  - Implement cleanup jobs for expired frames
- **Dependencies:** 8.1.2

### Section 8.2: Storage Growth Prevention

#### Subsection 8.2.1: Implement Frame Cleanup Jobs
- **Description:** Add periodic jobs to clean up old frames
- **Scope:** 
  - Schedule cleanup jobs for expired frames
  - Clean up normal frames after retention period
  - Preserve security event frames according to policy
- **Dependencies:** 8.1.3

#### Subsection 8.2.2: Add Storage Usage Monitoring
- **Description:** Monitor storage usage and alert on growth
- **Scope:** 
  - Track storage usage over time
  - Alert on rapid growth
  - Report storage usage by category (frames, screenshots, models, events)
- **Dependencies:** None

### Section 8.3: Compliance and Data Governance

#### Subsection 8.3.1: Implement Configurable Data Retention Policies
- **Description:** Implement flexible data retention policies for compliance
- **Scope:** 
  - Configure retention period per device type
  - Configure retention period per data type (frames, screenshots, events, logs)
  - Support different retention policies (time-based, size-based, event-based)
  - Implement retention policy enforcement
- **Dependencies:** 8.1.3 (frame retention policies)
- **Rationale:** Security domain often has regulatory requirements (GDPR, CCPA, industry-specific)

#### Subsection 8.3.2: Support Data Deletion Workflows
- **Description:** Implement secure data deletion for compliance (e.g., GDPR right to deletion)
- **Scope:** 
  - Support explicit data deletion requests
  - Implement secure deletion (overwrite, cryptographic erasure)
  - Support deletion logging and audit trail
  - Handle cascading deletions (related data, backups)
- **Dependencies:** 8.3.1, Epic 3, Section 3.5 (audit logging)

#### Subsection 8.3.3: Add Data Lineage Tracking
- **Description:** Track data lineage for compliance and audit purposes
- **Scope:** 
  - Track data origin (device, capture time, capture method)
  - Track data transformations and processing
  - Track data access and sharing
  - Support lineage query API for compliance audits
- **Dependencies:** Epic 2, Section 2.3 (event sourcing), Epic 3, Section 3.5 (audit logging)

#### Subsection 8.3.4: Implement Data Export Capabilities
- **Description:** Support data export for compliance audits and data portability
- **Scope:** 
  - Export device data in standard formats
  - Support time-range based exports
  - Support device-specific exports
  - Support export with metadata and lineage
- **Dependencies:** 8.3.3

---

## Epic 9: Configuration and Infrastructure

**Priority:** Medium/Low  
**Goal:** Improve configuration management and fix infrastructure issues.

### Section 9.1: Configuration Improvements

#### Subsection 9.1.1: Make Object Storage Configurable
- **Description:** Make MinIO configuration configurable (secure, bucket name, etc.)
- **Scope:** 
  - Add object storage configuration section
  - Make Secure flag configurable
  - Make bucket name configurable
  - Remove hardcoded values
- **Dependencies:** None
- **Related Findings:**
  - Finding #43: Object storage hard-coded settings

#### Subsection 9.1.2: Fix Dev Mode Configuration
- **Description:** Fix misleading dev mode configuration
- **Scope:** 
  - Return accurate wireguard.enabled value in dev mode
  - Don't hardcode interface values
  - Reflect actual connection mode
- **Dependencies:** None
- **Related Findings:**
  - Finding #42: Dev mode returns misleading wireguard.enabled value
- **Code Locations:**
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:451-467`

### Section 9.2: Data Consistency

#### Subsection 9.2.1: Fix State History Ordering
- **Description:** Fix edge state history to return most recent first
- **Scope:** 
  - Review cursor iteration logic
  - Fix double-reverse iteration bug
  - Verify ordering with tests
- **Dependencies:** None
- **Related Findings:**
  - Finding #40: State history ordering returns oldest-first instead of most recent first
- **Code Locations:**
  - `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go:974-1018`

### Section 9.3: Device Isolation and Multi-Tenancy

#### Subsection 9.3.1: Design Device Grouping and Segmentation
- **Description:** Design device grouping mechanism for multi-tenant scenarios
- **Scope:** 
  - Define device group/tenant concept
  - Support device grouping by organization, location, function
  - Design group-level configuration and policies
  - Support group-level isolation
- **Dependencies:** Epic 12 (device abstraction)
- **Rationale:** Essential for scenarios where multiple organizations share Edge or logical device segmentation

#### Subsection 9.3.2: Implement Data Isolation
- **Description:** Ensure data isolation between device groups/tenants
- **Scope:** 
  - Isolate storage per device group
  - Isolate device state per group
  - Isolate event streams per group
  - Support cross-group operations (if authorized)
- **Dependencies:** 9.3.1

### Section 9.4: API Versioning Strategy

#### Subsection 9.4.1: Design API Versioning Scheme
- **Description:** Design API versioning approach for long-term API evolution
- **Scope:** 
  - Choose versioning strategy (URL-based vs header-based)
  - Define versioning policy (semantic versioning, deprecation timeline)
  - Design backward compatibility strategy
  - Document versioning guidelines
- **Dependencies:** None
- **Rationale:** Essential for long-term extensibility as IoT device types and protocols evolve

#### Subsection 9.4.2: Implement Version Support
- **Description:** Implement version routing and compatibility layer
- **Scope:** 
  - Support multiple API versions simultaneously
  - Implement version routing in API handlers
  - Create compatibility layer for deprecated versions
  - Support graceful version migration
- **Dependencies:** 9.4.1

#### Subsection 9.4.3: Document Deprecation Policy
- **Description:** Define and document API deprecation and sunset policy
- **Scope:** 
  - Define deprecation notice timeline
  - Document sunset schedule
  - Provide migration guides
  - Support deprecation warnings in API responses
- **Dependencies:** 9.4.2

---

## Epic 10: Testing and Observability

**Priority:** Critical/High  
**Goal:** Add comprehensive tests and improve observability. For security domain applications, observability is critical for security visibility and incident response.

### Section 10.1: Test Coverage

#### Subsection 10.1.1: Add Race Condition Tests
- **Description:** Add tests with race detector enabled
- **Scope:** 
  - Run `go test -race` on all packages
  - Add concurrent access test scenarios
  - Fix any detected race conditions
- **Dependencies:** Epic 4 (concurrency fixes)
- **Related Findings:**
  - Recommendation #69: Add race detector coverage

#### Subsection 10.1.2: Add Out-of-Order Event Tests
- **Description:** Test handling of out-of-order events
- **Scope:** 
  - Test model deployed before screenshot_set_ready
  - Test disconnect during deployment
  - Test event replay scenarios
- **Dependencies:** Epic 2 (event reliability)
- **Related Findings:**
  - Recommendation #70: Add out-of-order event scenario tests

#### Subsection 10.1.3: Add Disconnect/Reconnect Tests
- **Description:** Test disconnect and reconnect scenarios
- **Scope:** 
  - Test HTTPS disconnect during frame processing
  - Test WireGuard disconnect during frame processing
  - Test reconnect and state recovery
- **Dependencies:** Epic 1 (state machine refactoring)
- **Related Findings:**
  - Recommendation #129: Add disconnect/reconnect scenario tests

#### Subsection 10.1.4: Add Multi-Camera Tests
- **Description:** Test multi-camera scenarios
- **Scope:** 
  - Test independent camera state machines
  - Test concurrent camera operations
  - Test camera-specific workflows
- **Dependencies:** Epic 1 (state machine refactoring)

### Section 10.2: Observability Improvements

#### Subsection 10.2.1: Add Event Metrics
- **Description:** Add metrics for event processing
- **Scope:** 
  - Track event publish/delivery rates
  - Track event drop rates
  - Track event processing latency
- **Dependencies:** None

#### Subsection 10.2.2: Add State Transition Metrics
- **Description:** Add metrics for state transitions
- **Scope:** 
  - Track state transition counts
  - Track time spent in each state
  - Alert on unexpected state transitions
- **Dependencies:** None

#### Subsection 10.2.3: Add Frame Processing Metrics
- **Description:** Add metrics for frame processing
- **Scope:** 
  - Track frame capture rate
  - Track AI inference latency
  - Track frame processing errors
- **Dependencies:** None

### Section 10.3: Security Monitoring

#### Subsection 10.3.1: Implement Real-Time Anomaly Detection
- **Description:** Detect unusual device behavior and security anomalies
- **Scope:** 
  - Monitor device behavior patterns (access frequency, data volume, timing)
  - Detect anomalies (unusual access patterns, failed authentication spikes, unusual data transfers)
  - Support configurable anomaly detection rules
  - Alert on detected anomalies
- **Dependencies:** Epic 3, Section 3.5 (audit logging)
- **Rationale:** Critical for security domain - early detection of security incidents

#### Subsection 10.3.2: Add Security Event Alerting
- **Description:** Implement alerting system for security events
- **Scope:** 
  - Alert on authentication failures
  - Alert on unauthorized access attempts
  - Alert on configuration changes
  - Alert on data integrity violations
  - Support multiple alert channels (email, webhook, SIEM integration)
- **Dependencies:** 10.3.1

#### Subsection 10.3.3: Implement Device Health Dashboards
- **Description:** Create dashboards for device health and security status
- **Scope:** 
  - Dashboard for device status overview
  - Dashboard for security events and alerts
  - Dashboard for resource usage and quotas
  - Dashboard for compliance status
- **Dependencies:** 10.2 (observability improvements), 10.3.1

#### Subsection 10.3.4: Add Compliance Reporting
- **Description:** Generate compliance reports for regulatory requirements
- **Scope:** 
  - Generate audit reports (access logs, configuration changes)
  - Generate data retention compliance reports
  - Generate security incident reports
  - Support scheduled and on-demand report generation
- **Dependencies:** Epic 3, Section 3.5 (audit logging), Epic 8, Section 8.3 (compliance)

### Section 10.4: Forensics Support

#### Subsection 10.4.1: Implement Structured Logging for Forensics
- **Description:** Add structured logging optimized for forensic analysis
- **Scope:** 
  - Use structured log format (JSON) with consistent fields
  - Include correlation IDs for event tracing
  - Include device IDs, user IDs, timestamps in all logs
  - Support log aggregation and search
- **Dependencies:** None

#### Subsection 10.4.2: Add Event Timeline Reconstruction
- **Description:** Enable reconstruction of event timelines for incident investigation
- **Scope:** 
  - Support time-range based event queries
  - Support event filtering and correlation
  - Generate event timelines for specific incidents
  - Support export of event timelines for analysis
- **Dependencies:** Epic 2, Section 2.3 (event sourcing), 10.4.1

#### Subsection 10.4.3: Implement Device Activity Replay
- **Description:** Enable replay of device activities for investigation
- **Scope:** 
  - Replay device state transitions from event log
  - Replay device data processing activities
  - Support time-based replay (replay activities at specific time)
  - Support selective replay (replay specific device or operation type)
- **Dependencies:** 10.4.2, Epic 2, Section 2.3 (event sourcing)

#### Subsection 10.4.4: Add SOC Integration Support
- **Description:** Support integration with Security Operations Centers (SOC)
- **Scope:** 
  - Support standard security event formats (CEF, STIX, JSON)
  - Enable real-time event streaming to SOC tools
  - Support SIEM integration (Splunk, QRadar, etc.)
  - Provide API for SOC tools to query events and state
- **Dependencies:** 10.4.1, Epic 3, Section 3.5 (audit logging)

---

## Epic 11: Documentation Updates

**Priority:** Low  
**Goal:** Update documentation to match implementation and fill gaps.

### Section 11.1: State Machine Documentation

#### Subsection 11.1.1: Document Missing States
- **Description:** Add documentation for undocumented states
- **Scope:** 
  - Document `initializing` state
  - Document `wg_connecting` state
  - Document `http_connecting` state
- **Dependencies:** None
- **Related Findings:**
  - Finding #21: Documentation omits implemented states
  - Finding #101: Missing states not documented

#### Subsection 11.1.2: Update State Transition Diagrams
- **Description:** Update diagrams to reflect multi-level state machine
- **Scope:** 
  - Add connection-level state diagram
  - Add per-camera state diagram
  - Show relationships between state machines
- **Dependencies:** Epic 1 (state machine refactoring)

### Section 11.2: Security Documentation

#### Subsection 11.2.1: Document Security Model
- **Description:** Document security patterns and limitations
- **Scope:** 
  - Document capture provenance requirements
  - Document web gateway access model
  - Document TLS/certificate requirements
  - Document security limitations
- **Dependencies:** Epic 3 (security improvements)

---

## Implementation Priority Summary

### Phase 1 (Critical - Immediate)
1. Epic 3, Section 3.1: Capture Provenance Enforcement (Critical Security)
2. Epic 4, Section 4.1: Concurrency Safety (Critical Stability)
3. Epic 3, Section 3.5: Audit Logging Framework (Critical Security - Compliance)
4. Epic 4, Section 4.2: Goroutine and Resource Management (Critical Stability)
5. Epic 1, Section 1.2: Disconnect Handling Alignment (Critical Functionality)

### Phase 2 (High - Short Term)
6. Epic 1, Section 1.1: Multi-Level State Machine Design (High Functionality)
7. Epic 12: Device Abstraction Layer (High Extensibility - Enable IoT expansion)
8. Epic 2, Section 2.1: Event Bus Reliability (High Reliability)
9. Epic 2, Section 2.3: Event Sourcing and Audit Trail (High Reliability + Security)
10. Epic 3, Section 3.2: TLS and Certificate Management (High Security)
11. Epic 3, Section 3.6: Role-Based Access Control (High Security)
12. Epic 3, Section 3.9: Device Identity, Provisioning, and Attestation (High Security - Zero trust foundation)
13. Epic 3, Section 3.10: Secure Updates and Supply Chain Security (High Security)
14. Epic 6, Section 6.1: Screenshot Content-Type Handling (High Data Integrity)
15. Epic 10: Testing and Observability (High Priority - Security visibility)

### Phase 3 (Medium - Medium Term)
16. Epic 3, Section 3.7: Data-at-Rest Encryption (Medium Security - Defense-in-depth)
17. Epic 3, Section 3.8: Tamper Detection (Medium Security)
18. Epic 3, Section 3.11: Plugin Sandboxing and Least Privilege (Medium Security - Extensibility hardening)
19. Epic 5, Section 5.1: Frame Processing Error Handling (Medium Reliability)
20. Epic 6, Section 6.2: Thumbnail Implementation (Medium Feature Completeness)
21. Epic 7, Section 7.1: Screenshot Sync Optimization (Medium Performance)
22. Epic 8, Section 8.1: Frame Lifecycle Ownership (Medium Clarity)
23. Epic 8, Section 8.3: Compliance and Data Governance (Medium Compliance)
24. Epic 4, Section 4.4: Per-Device Resource Quotas (Medium Scalability)
25. Epic 10, Section 10.3: Security Monitoring (Medium Security Operations)
26. Epic 10, Section 10.4: Forensics Support (Medium Incident Response)

### Phase 4 (Lower - Long Term)
27. Epic 9, Sections 9.1-9.2: Configuration and Infrastructure (Lower Priority)
28. Epic 9, Section 9.3: Device Isolation and Multi-Tenancy (Lower Priority)
29. Epic 9, Section 9.4: API Versioning Strategy (Lower Priority)
30. Epic 11: Documentation Updates (Lower Priority, but ongoing)

---

## Dependencies Between Epics

- Epic 1 (State Machine) should be completed before Epic 2 (Events) for proper event routing, and before Epic 12 (Device Abstraction) for device state machines
- Epic 4 (Concurrency) should be completed early to prevent stability issues
- Epic 3 (Security) is independent and can be done in parallel, but Section 3.5 (Audit Logging) benefits from Epic 2, Section 2.3 (Event Sourcing)
- Epic 3, Section 3.9 (Device Identity/Provisioning/Attestation) depends on Epic 12 (device abstraction) and is a prerequisite for robust multi-IoT onboarding
- Epic 3, Section 3.10 (Secure Updates/Supply Chain/Secrets) is largely independent, but secrets management benefits from encryption + key management (Section 3.7)
- Epic 3, Section 3.11 (Plugin Sandboxing/Least Privilege) depends on Epic 12 (plugins) and ties into Epic 4, Section 4.4 (quotas) and Epic 10, Section 10.3 (monitoring)
- Epic 6 (Data Integrity) is independent and can be done in parallel
- Epic 7 (Performance) depends on Epic 6 (pagination, thumbnails)
- Epic 8 (Frame Lifecycle) depends on Epic 1 (state machine clarity), and Section 8.3 (Compliance) benefits from Epic 2, Section 2.3 (event sourcing) and Epic 3, Section 3.5 (audit logging)
- Epic 9, Section 9.3 (Multi-Tenancy) depends on Epic 12 (Device Abstraction)
- Epic 10 (Testing and Observability) depends on all other epics for comprehensive coverage, but core observability can start early
- Epic 10, Section 10.3 (Security Monitoring) and 10.4 (Forensics) depend on Epic 3, Section 3.5 (Audit Logging) and Epic 2, Section 2.3 (Event Sourcing)
- Epic 12 (Device Abstraction) can start early but benefits from Epic 1 (state machine separation) being completed first
- Epic 4, Section 4.4 (Resource Quotas) depends on Epic 12 (device abstraction)
- Epic 11 (Documentation) should be updated as epics are completed

---

## Notes

- This plan is organized to minimize breaking changes where possible
- Each epic can be broken down into smaller work items (tasks/stories)
- Testing should be added incrementally as features are refactored
- Documentation should be updated continuously, not just at the end
- Some findings may be addressed by multiple epics (e.g., state machine issues)
- Priority and dependencies may be adjusted based on business needs

### Architectural Scalability Considerations

**Current Architecture Strengths:**
- Interface-based design (good for extensibility)
- Event-driven architecture (good for decoupling)
- Separate service layers (state manager, gateways, storage)

**Future Scalability Enhancements (Beyond This Plan):**
- **Horizontal Scalability:** Consider distributed state management (e.g., Raft consensus) for multi-Edge coordination; design state machines to be shardable by device ID
- **Data Volume Scalability:** Consider time-series database for device metrics (vs. BoltDB); implement object storage tiering (hot/warm/cold) for old footage
- **Device Count Scalability:** Implement device connection pooling/multiplexing; add device discovery service registry for 100s+ devices
- **Processing Scalability:** Consider job queue + worker pool architecture for AI processing to enable prioritization, load balancing, and external GPU cluster integration

**Security Domain Focus:**
- All security enhancements (Epic 3) are prioritized for compliance and defense-in-depth
- Audit logging and forensics support (Epic 10) are essential for incident investigation
- Device abstraction (Epic 12) enables secure extension to diverse IoT device types

