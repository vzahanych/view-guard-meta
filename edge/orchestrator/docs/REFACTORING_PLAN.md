# Edge Orchestrator Refactoring Plan

This document provides a comprehensive refactoring plan based on the state transition review findings. The plan is organized into epics, sections, and subsections to guide systematic improvements.

## Epic 1: State Machine Architecture Refactoring

**Priority:** Critical/High  
**Goal:** Split global state machine into connection-level and per-camera state machines to support independent multi-camera flows. This refactoring will also provide foundation for future device abstraction (Epic 12).

### Section 1.0: State Machine Ownership & Service Boundaries (NEW - Target Architecture)

**Goal:** Make state ownership explicit and keep state machine implementations inside the services that *own* the state, while `state-mng` becomes a pure workflow orchestrator.

**Target architecture principles (end-state):**
- **Connection state machine lives in `vm-gateway`** and is exposed through the `vmgateway.VMGateway` top interface (or a small dedicated `vmgateway.ConnectionStateService` interface). `state-mng` does **not** embed a connection state machine implementation.
- **Per-device state machines live in `iot`** and are exposed through the `iot` top interfaces (e.g., `iot.DeviceRegistry` / `iot.DeviceStateMachineRegistry` / a dedicated `iot.DeviceStateService`). `state-mng` does **not** embed camera/device state machine implementations.
- **State manager becomes an orchestrator only**:
  - It listens to events and *derives actions* from observed states,
  - it triggers workflows (connect, sync, capture, process, recover),
  - but it does not “own” state machine implementations.

**Important note (current state vs target):**
- Epic 1 delivered a working multi-level state machine design inside `state-mng` (`ConnectionStateMachine` + `CameraStateMachine`) to unblock correctness, recovery, and testing quickly.
- Epic 12 introduced `iot`-level device state machines and a generic data pipeline.
- This section formalizes the next architectural step: move state machine ownership into the owning services (`vm-gateway`, `iot`) and slim down `state-mng`.

### Section 1.1: Multi-Level State Machine Design

#### Subsection 1.1.1: Design Connection-Level State Machine
- **Status:** ✅ DONE
- **Description:** Define connection-level states (disconnected, wireguard_connected, https_connected, authenticated, etc.)
- **Scope:** Connection-level states are global to the Edge appliance
- **Dependencies:** None
- **Related Findings:**
  - Finding #15: Global state machine blocks independent multi-camera flows
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/types/connection_state.go` (connection state types, constants, and interface definitions)
  - `edge/orchestrator/internal/state-mng/impl/connection_state_machine.go` (state machine implementation)
  - `edge/orchestrator/internal/state-mng/impl/connection_state_machine_test.go` (unit tests)
- **Refactoring Details:**
  - **Architecture note (next step):**
    - This was implemented inside `state-mng` to quickly establish a correct, testable connection state model.
    - **Target end-state:** move connection state machine ownership into `vm-gateway` and expose state via `vmgateway.VMGateway` (see Section 1.0 and Section 1.4).
  - **Code Organization:**
    - **Types Package (`types/`):** Contains only reusable type definitions, interfaces, and constants
      - `ConnectionState` type and constants (disconnected, wg_connecting, wireguard_connected, etc.)
      - `ConnectionStateInfo` struct for state metadata
      - `ConnectionStateMachine` interface definition
    - **Implementation Package (`impl/`):** Contains implementation-specific code
      - `ConnectionStateMachineImpl` struct implementing the interface
      - `ValidConnectionStateTransitions` map defining valid state transitions
      - `IsValidTransition()` helper function for transition validation
      - Thread-safe state machine with mutex protection
  - **Connection States Defined:**
    - `ConnectionStateDisconnected`: Edge is not connected to VM
    - `ConnectionStateWGConnecting`: WireGuard connection is being established
    - `ConnectionStateWireGuardConnected`: WireGuard tunnel is established
    - `ConnectionStateHTTPConnecting`: HTTPS connection is being established
    - `ConnectionStateHTTPSConnected`: HTTPS connection is established
    - `ConnectionStateAuthenticated`: Edge is authenticated with VM
    - `ConnectionStateCapabilitiesReceived`: Edge has received capabilities from VM
    - `ConnectionStateError`: Generic connection-level error
    - `ConnectionStateWGConnectionError`: WireGuard connection error
    - `ConnectionStateHTTPConnectionError`: HTTPS connection error
  - **State Machine Interface:**
    - `ConnectionStateMachine` interface with methods:
      - `GetState()`: Returns current connection state
      - `GetStateInfo()`: Returns detailed connection state information
      - `Transition(newState, errorMsg)`: Transitions to new state with validation
      - `CanTransition(newState)`: Checks if transition is valid
      - `IsConnected()`: Returns true if Edge is fully connected (authenticated)
      - `IsAuthenticated()`: Returns true if Edge is authenticated with VM
  - **State Transition Rules:**
    - Normal flow: `disconnected -> wg_connecting -> wireguard_connected -> http_connecting -> https_connected -> authenticated -> capabilities_received`
    - Error handling: Any state can transition to `disconnected` on connection loss
    - Error states: `wg_connection_error` and `http_connection_error` can recover to their connected states
    - Validation: Invalid transitions return errors (e.g., cannot skip from `disconnected` to `authenticated`)
  - **ConnectionStateInfo:**
    - Contains state, last updated timestamp, error message, network health, and VM reachability
    - Network health: "healthy", "degraded", or "unhealthy" based on connection state
    - VM reachability: Boolean indicating if VM is reachable (true for wireguard_connected, https_connected, authenticated, capabilities_received)
  - **Implementation:**
    - `ConnectionStateMachineImpl` implements the interface with thread-safe operations (mutex-protected)
    - State transitions are validated against `ValidConnectionStateTransitions` map (in `impl` package)
    - `IsValidTransition()` helper function validates transitions (in `impl` package)
    - State machine tracks metadata (error messages, network health, VM reachability)
    - Initial state defaults to `ConnectionStateDisconnected` with unhealthy network health
  - **Unit Tests:**
    - Tests for initial state (disconnected)
    - Tests for all valid state transitions
    - Tests for invalid transitions (should fail)
    - Tests for `IsConnected()` and `IsAuthenticated()` methods
    - Tests for network health and VM reachability calculations
    - Tests for concurrent access (thread safety)
    - Tests for state info updates
    - Tests for `IsValidTransition()` helper function
  - **Design Benefits:**
    - **Separation of concerns:** Connection states are separate from camera states
    - **Clear state transitions:** Explicit validation prevents invalid state changes
    - **Thread-safe:** Mutex protection ensures safe concurrent access
    - **Metadata tracking:** Network health and VM reachability provide operational insights
    - **Recovery support:** Error states can transition back to connected states
    - **Foundation for multi-camera independence:** Connection state is global, camera states will be per-camera
    - **Reusable types:** Types package can be imported by other services without implementation dependencies

#### Subsection 1.1.2: Design Per-Camera State Machine
- **Status:** ✅ DONE
- **Description:** Define per-camera states (camera_discovered, camera_synced, waiting_for_screenshots, screenshot_set_ready, model_deployed, frame_processing, etc.)
- **Scope:** Each camera has its own independent state machine keyed by camera ID
- **Dependencies:** 1.1.1
- **Related Findings:**
  - Finding #15: Global state machine blocks independent multi-camera flows
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/types/camera_state.go` (camera state types, constants, and interface definitions)
  - `edge/orchestrator/internal/state-mng/impl/camera_state_machine.go` (camera state machine implementation)
  - `edge/orchestrator/internal/state-mng/impl/camera_state_machine_test.go` (unit tests)
- **Refactoring Details:**
  - **Architecture note (next step):**
    - This per-camera state machine was implemented inside `state-mng` as part of the Epic 1 refactor.
    - **Target end-state:** evolve this into a per-device state machine owned by `iot` (Epic 12, Subsection 12.3.2) and have `state-mng` consume it via `iot` interfaces (see Section 1.0 and Section 1.4).
  - **Code Organization:**
    - **Types Package (`types/`):** Contains only reusable type definitions, interfaces, and constants
      - `CameraState` type and constants (undiscovered, discovered, synced, waiting_for_screenshots, etc.)
      - `CameraStateInfo` struct for camera state metadata
      - `CameraStateMachine` interface definition
    - **Implementation Package (`impl/`):** Contains implementation-specific code
      - `CameraStateMachineImpl` struct implementing the interface
      - `ValidCameraStateTransitions` map defining valid state transitions
      - `IsValidCameraTransition()` helper function for transition validation
      - Thread-safe state machine with mutex protection
  - **Camera States Defined:**
    - `CameraStateUndiscovered`: Camera has not been discovered yet
    - `CameraStateDiscovered`: Camera has been discovered but not yet synced with VM
    - `CameraStateSynced`: Camera has been synced with the VM
    - `CameraStateWaitingForScreenshots`: Waiting for user to provide labeled screenshots
    - `CameraStateScreenshotSetReady`: Labeled screenshots are ready for training
    - `CameraStateModelDeployed`: AI model has been deployed for this camera
    - `CameraStateFrameProcessing`: Camera is actively processing frames
    - `CameraStateError`: Camera-specific error occurred
    - `CameraStateDisconnected`: Camera connection was lost
  - **State Machine Interface:**
    - `CameraStateMachine` interface with methods:
      - `GetCameraID()`: Returns the camera ID this state machine is for
      - `GetState()`: Returns current camera state
      - `GetStateInfo()`: Returns detailed camera state information
      - `Transition(newState, errorMsg)`: Transitions to new state with validation
      - `CanTransition(newState)`: Checks if transition is valid
      - `IsOperational()`: Returns true if camera is operational (model_deployed or frame_processing)
      - `IsReadyForProcessing()`: Returns true if camera is ready to process frames
  - **State Transition Rules:**
    - Normal flow: `undiscovered -> discovered -> synced -> waiting_for_screenshots -> screenshot_set_ready -> model_deployed -> frame_processing`
    - Shortcut: `synced -> model_deployed` (can skip screenshots if model already exists)
    - Error handling: Any state can transition to `disconnected` on camera disconnection
    - Error states: `error` can recover to `discovered` (reset) or `synced` (retry)
    - Disconnection: `disconnected` can transition to `discovered` (reconnect)
    - Processing control: `frame_processing` can transition back to `model_deployed` (stop processing)
    - Validation: Invalid transitions return errors (e.g., cannot skip from `undiscovered` to `synced`)
  - **CameraStateInfo:**
    - Contains camera ID, state, last updated timestamp, error message
    - Tracks model ID (when model is deployed)
    - Tracks dataset ID (when dataset is ready)
    - `IsProcessing` flag: Automatically set to true when in `frame_processing` state
  - **Implementation:**
    - `CameraStateMachineImpl` implements the interface with thread-safe operations (mutex-protected)
    - Each camera has its own independent state machine instance
    - State transitions are validated against `ValidCameraStateTransitions` map (in `impl` package)
    - `IsValidCameraTransition()` helper function validates transitions (in `impl` package)
    - State machine tracks metadata (error messages, model ID, dataset ID, processing status)
    - Initial state defaults to `CameraStateUndiscovered` with `IsProcessing` set to false
    - Helper methods: `SetModelID()` and `SetDatasetID()` for tracking model and dataset IDs
  - **Unit Tests:**
    - Tests for initial state (undiscovered)
    - Tests for all valid state transitions (normal flow, shortcuts, error recovery, disconnection)
    - Tests for invalid transitions (should fail)
    - Tests for `IsOperational()` and `IsReadyForProcessing()` methods
    - Tests for state info updates (including IsProcessing flag)
    - Tests for model ID and dataset ID tracking
    - Tests for concurrent access (thread safety)
    - Tests for `IsValidCameraTransition()` helper function
  - **Design Benefits:**
    - **Per-camera independence:** Each camera operates independently with its own state machine
    - **Clear state transitions:** Explicit validation prevents invalid state changes
    - **Thread-safe:** Mutex protection ensures safe concurrent access
    - **Metadata tracking:** Model ID, dataset ID, and processing status provide operational insights
    - **Recovery support:** Error and disconnected states can transition back to operational states
    - **Flexible workflows:** Supports both full workflow (with screenshots) and shortcut (skip screenshots if model exists)
    - **Processing control:** Can start/stop frame processing without losing model deployment state
    - **Reusable types:** Types package can be imported by other services without implementation dependencies
    - **Foundation for multi-camera independence:** Enables cameras to be in different states simultaneously

#### Subsection 1.1.3: Implement State Machine Separation
- **Status:** ✅ DONE (Structure implemented, full transition in 1.1.4)
- **Description:** Refactor state manager to maintain separate state machines for connection and per-camera
- **Scope:** 
  - Connection state: `ConnectionStateMachine` interface (from subsection 1.1.1)
  - Camera state: `CameraStateMachine` interface with camera ID key (from subsection 1.1.2)
  - State manager maintains both state machines
  - Operational state: Separate metrics (cameras enabled, AI processing, storage health)
- **Dependencies:** 1.1.1, 1.1.2
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` (StateManagerImpl struct and helper methods)
- **Refactoring Details:**
  - **Architecture note (next step):**
    - This subsection intentionally placed state machines inside `state-mng` to complete the Epic 1 refactor quickly and safely.
    - **Target end-state:** `state-mng` should not embed state machines; it should orchestrate workflows based on state observed from `vm-gateway` (connection) and `iot` (devices). See Section 1.0 and Section 1.4 for the planned migration.
  - **State Machine Structure:**
    - **Removed:** Old `EdgeStatus` enum and `EdgeState` struct that mixed connection and camera states
    - **Added:** `connectionStateMachine` field of type `types.ConnectionStateMachine` (from subsection 1.1.1)
    - **Added:** `cameraStateMachines` map of type `map[string]types.CameraStateMachine` (cameraID -> state machine)
    - **Added:** `cameraStateMachinesMu` mutex for thread-safe access to camera state machines map
    - **Added:** `operationalState` struct for non-state-machine metrics (cameras enabled, AI processing, storage health)
    - **Added:** `operationalMu` mutex for thread-safe access to operational state
  - **Helper Methods Added:**
    - `getOrCreateCameraStateMachine(cameraID string)`: Gets or creates a camera state machine for a camera
    - `getCameraStateMachine(cameraID string)`: Gets a camera state machine (returns nil if not found)
    - `getAllCameraStateMachines()`: Returns all camera state machines (thread-safe copy)
    - `updateOperationalState(updateFn func(*OperationalState))`: Updates operational metrics
    - `getOperationalState()`: Returns a copy of operational state
  - **State Persistence:**
    - Updated `persistStateToStorage()` to accept `ConnectionState` and `map[string]CameraState` instead of `EdgeState`
    - Persists connection state, camera states, and operational metrics separately
  - **Initialization:**
    - Connection state machine initialized to `ConnectionStateDisconnected` on startup
    - Camera state machines created on-demand when cameras are discovered
    - Operational state initialized with default values (storage health: "healthy")
  - **State Access:**
    - Connection state accessed via `m.connectionStateMachine.GetState()` and `Transition()`
    - Camera states accessed via `m.getOrCreateCameraStateMachine(cameraID)` or `m.getCameraStateMachine(cameraID)`
    - Operational metrics accessed via `m.getOperationalState()` and `m.updateOperationalState()`
  - **Thread Safety:**
    - Connection state machine is thread-safe (uses internal mutex)
    - Camera state machines are thread-safe (each uses internal mutex)
    - Camera state machines map access is protected by `cameraStateMachinesMu`
    - Operational state access is protected by `operationalMu`
  - **Migration Notes:**
    - Old `EdgeStatus` and `EdgeState` types removed from state manager
    - State transition logic in `updateStateForEvent()` and `executeWorkflow()` still uses old structure
    - Full migration to new state machines will be completed in subsection 1.1.4
    - This subsection establishes the foundation and structure for the separated state machines
  - **Design Benefits:**
    - **Separation of concerns:** Connection state and camera states are completely independent
    - **Per-camera independence:** Each camera can be in different states simultaneously
    - **Clear state management:** State machines enforce valid transitions
    - **Thread-safe:** All state access is properly synchronized
    - **Operational metrics:** Non-state-machine metrics kept separate for clarity
    - **Foundation for multi-camera flows:** Enables independent camera workflows

#### Subsection 1.1.4: Update State Transition Logic
- **Status:** ✅ DONE (Core transition logic updated, some legacy handlers remain for compatibility)
- **Description:** Update all state transition handlers to work with separated state machines
- **Scope:** Modify event handlers to transition appropriate state machine(s)
- **Dependencies:** 1.1.3
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` (updateStateForEvent, executeWorkflow, and workflow handlers)
- **Refactoring Details:**
  - **State Transition Updates:**
    - **updateStateForEvent()**: Completely refactored to use connection and camera state machines
      - Network events (WireGuard, HTTPS, Authentication, Capabilities) update connection state machine
      - Camera events (Discovered, Registered, Connected, Disconnected) update camera state machines
      - Snapshot request events update camera state machines (waiting_for_screenshots, screenshot_set_ready)
      - Storage and AI events update operational state (metrics)
      - Returns `ConnectionState` and `map[string]CameraState` instead of `EdgeState`
    - **executeWorkflow()**: Updated to accept connection state and camera states
      - Workflow handlers check connection state and camera states separately
      - Per-camera workflows execute based on individual camera state machines
      - Connection-level workflows execute based on connection state machine
  - **Per-Camera Workflow Functions:**
    - Added `executeScreenshotSetReadyWorkflowForCamera(cameraID)`: Handles screenshot set ready workflow for a specific camera
    - Added `executeModelDeployedWorkflowForCamera(cameraID)`: Handles model deployment workflow for a specific camera
    - Added `executeFrameProcessingWorkflowForCamera(cameraID)`: Handles frame processing workflow for a specific camera
    - Added `handleConnectionErrorState()`: Handles connection-level error states
    - Added `handleCameraErrorState(cameraID, cameraState)`: Handles camera-level error states
  - **Event Handler Updates:**
    - **handleCameraDiscovered()**: Updated to check connection state and camera state separately
    - **handleSnapshotRequested()**: Camera state transition handled in `updateStateForEvent()`, handler just executes workflow
    - **handleScreenshotSetReady()**: Camera state transition handled in `updateStateForEvent()`, handler just executes workflow
    - **handleCapabilitiesReceived()**: Updated to use connection state machine for error handling
  - **Error Handling:**
    - Connection-level errors update connection state machine to `ConnectionStateError`
    - Camera-level errors update individual camera state machines to `CameraStateError`
    - Error handlers use new state machine methods instead of direct state manipulation
  - **State Persistence:**
    - All `persistStateToStorage()` calls updated to use new signature: `(ctx, ConnectionState, map[string]CameraState)`
    - Connection state and camera states persisted separately
  - **Compatibility:**
    - Legacy `GetStatus()` and `GetState()` methods maintained for backward compatibility
    - These methods map new state machines to old `EdgeStatus` and `EdgeState` types
    - Legacy workflow functions (e.g., `executeModelDeployedWorkflow(EdgeState)`) kept for compatibility but marked as deprecated
  - **Remaining Work:**
    - Some legacy handlers still reference old `m.state` structure (e.g., in `checkServicesHealth`, `syncCamerasWithVM`)
    - These will be fully migrated in future iterations
    - Core state transition logic is fully migrated to use separated state machines
  - **Design Benefits:**
    - **Clear separation**: Connection and camera states are managed independently
    - **Per-camera workflows**: Each camera can have its own workflow execution
    - **Thread-safe**: All state transitions use thread-safe state machine methods
    - **Type safety**: State transitions validated by state machine logic
    - **Better error handling**: Errors scoped to appropriate state machine (connection vs camera)

#### Subsection 1.1.5: Update State Persistence
- **Status:** ✅ DONE
- **Description:** Persist both connection and per-camera states separately
- **Scope:** Update meta-storage to persist connection state and per-camera states
- **Dependencies:** 1.1.3
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` (`persistStateToStorage`, `restoreStateFromStorage`, `Start`)
- **Refactoring Details:**
  - **Enhanced State Persistence:**
    - **persistStateToStorage()**: Enhanced to save connection state and camera states with full metadata
      - Connection state: Saves state, error message, network health, VM reachability, and timestamps
      - Camera states: Saves state, error, model ID, dataset ID, processing flag, and timestamps for each camera
      - Operational metrics: Saves cameras enabled, AI processing status, storage health
      - If no camera states provided, automatically persists all current camera state machines
      - Added error logging with context (connection state, camera count) for better debugging
      - Added success logging for state persistence operations
  - **State Restoration:**
    - **restoreStateFromStorage()**: New method to restore state from meta-storage on startup
      - Restores connection state with validation (only valid states are restored)
      - Restores connection state error messages if present
      - Restores operational state (storage health, cameras enabled, AI processing status)
      - Restores all camera states with full metadata (state, error, model ID, dataset ID)
      - Creates camera state machines on-demand during restoration
      - Validates camera states before restoring (invalid states are skipped)
      - Supports backward compatibility with string-only camera state format
      - Logs restoration progress and counts
  - **Startup Integration:**
    - **Start()**: Updated to restore state from storage before initializing
      - Attempts to restore state from meta-storage first
      - Falls back to default initialization if restoration fails or no previous state exists
      - Logs restoration status and camera state machine count
      - Persists current state after restoration/initialization
  - **Error Handling:**
    - State persistence errors are logged with context (connection state, camera count)
    - State restoration errors are logged and handled gracefully (fallback to defaults)
    - Invalid states in storage are detected and handled (fallback to safe defaults)
  - **State Structure:**
    - Connection state stored with: `connection_state`, `connection_error`, `network_health`, `vm_reachable`, `connection_last_updated`
    - Camera states stored as map: `camera_states[cameraID] = {state, error, model_id, dataset_id, is_processing, last_updated}`
    - Operational metrics stored with: `cameras_enabled`, `ai_processing_active`, `storage_health`, `operational_last_updated`
    - Overall timestamp: `last_updated` for the entire state snapshot
  - **Design Benefits:**
    - **Separate persistence**: Connection and camera states are persisted separately, enabling independent recovery
    - **Full metadata**: All state information (errors, model IDs, timestamps) is preserved
    - **State recovery**: System can recover to previous state after restart
    - **Backward compatibility**: Supports both new structured format and old string-only format
    - **Error resilience**: Invalid states in storage don't crash the system
    - **Better debugging**: Enhanced logging helps diagnose state persistence issues
- **Related Findings:**
  - Finding #98: State persistence is fire-and-forget without error checking
    - **Addressed**: Added error logging with context and success logging for state persistence operations

#### Subsection 1.1.6: Add State Recovery Logic
- **Status:** ✅ DONE
- **Description:** On restart, recover connection state and all camera states
- **Scope:** Load both connection and camera states from meta-storage on startup
- **Dependencies:** 1.1.5
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` (`restoreStateFromStorage`, `recoverActiveWorkflows`, `Start`)
- **Refactoring Details:**
  - **State Restoration:**
    - **restoreStateFromStorage()**: Restores connection and camera states from meta-storage (implemented in 1.1.5)
      - Called automatically in `Start()` before initializing defaults
      - Restores connection state with validation
      - Restores all camera states with full metadata
      - Restores operational metrics
      - Falls back to defaults if restoration fails
  - **Active Workflow Recovery:**
    - **recoverActiveWorkflows()**: New method to recover active workflows based on restored states
      - **Connection Workflow Recovery:**
        - `WGConnecting`: Re-initiates WireGuard connection in background goroutine
        - `HTTPConnecting`: Re-initiates HTTP connection synchronously
        - `Error` states: Transitions to disconnected and re-initiates connection
        - `CapabilitiesReceived`: Checks for cameras to sync with VM
      - **Camera Workflow Recovery:**
        - `FrameProcessing`: Resumes frame processing for cameras that were actively processing
        - `ModelDeployed`: Starts frame processing and transitions to `FrameProcessing` if successful
        - `Error`: Attempts recovery by resetting to `Discovered` state (allows re-workflow)
        - `ScreenshotSetReady`: Resumes screenshot sync to VM if connection is available
      - **Error Handling:**
        - Frame processing recovery failures transition camera to error state
        - Connection recovery failures are logged and connection state updated
        - All recovery operations are logged with context
  - **Startup Integration:**
    - **Start()**: Enhanced to restore state and recover workflows
      - Calls `restoreStateFromStorage()` first
      - Falls back to default initialization if restoration fails
      - After restoration, `recoverActiveWorkflows()` is called automatically
      - Logs recovery status and camera state machine count
      - Persists current state after recovery
  - **Recovery Scenarios:**
    - **Connection Recovery:**
      - In-progress connections (WGConnecting, HTTPConnecting) are re-initiated
      - Error states are cleared and connection is retried
      - Capabilities workflow is resumed if capabilities were received
    - **Camera Recovery:**
      - Active frame processing is resumed automatically
      - Model deployment workflows are continued (start frame processing)
      - Error states are cleared to allow re-workflow
      - Pending screenshot syncs are resumed
    - **Operational Recovery:**
      - Operational metrics (cameras enabled, AI processing, storage health) are restored
      - Frame processing intervals and configurations are preserved
  - **Design Benefits:**
    - **Seamless recovery**: System resumes operations after restart without manual intervention
    - **Workflow continuity**: Active workflows (frame processing, connections) are automatically resumed
    - **Error recovery**: Error states are cleared and workflows are retried
    - **State consistency**: All states are restored before workflows are recovered
    - **Graceful degradation**: If recovery fails, system falls back to safe defaults
    - **Comprehensive logging**: All recovery operations are logged for debugging
  - **Recovery Flow:**
    1. **State Restoration**: Load connection and camera states from meta-storage
    2. **State Validation**: Validate restored states and handle invalid states
    3. **Workflow Recovery**: Resume active workflows based on restored states
    4. **Error Handling**: Handle recovery failures gracefully
    5. **State Persistence**: Persist recovered state to ensure consistency

### Section 1.2: Disconnect Handling Alignment ✅ CORRECT BEHAVIOR

**Status:** This section is **NOT NEEDED** - the current implementation is correct.

**Rationale:**
- **Security-first design:** Edge must continue monitoring its security zone even when connection to VM is lost
- **Independent operation:** Edge operates autonomously and queues security events for later sync
- **No data loss:** All security events are preserved and transmitted as soon as connectivity is re-established

**Current Behavior (Correct):**
- Frame processing continues during HTTPS/WireGuard disconnection
- Security events detected during disconnection are queued in meta-storage
- Events are automatically synced to VM when connection is restored via `syncPendingSecurityEvents()` called after authentication
- Connection state transitions appropriately (HTTPS disconnect → WireGuard connected, WireGuard disconnect → Disconnected)
- Camera states remain unchanged during disconnection (cameras continue in `frame_processing` state)

**Implementation Details:**
- `syncPendingSecurityEvents()` is called in `EventTypeEdgeAuthenticated` handler
- Events are retrieved in batches of 100 and marked as transmitted
- The actual transmission to VM is handled by the normal event transmission flow
- Security events are stored in meta-storage with proper metadata for later sync

**Note:** This behavior ensures continuous security monitoring regardless of network connectivity, which is critical for edge security appliances. The Edge must operate independently and protect its security zone even when the connection to the VM is temporarily lost.

**Related Findings:**
- Finding #14: HTTPS disconnect doesn't transition from model_deployed/frame_processing → ✅ **CORRECT BEHAVIOR** (frame processing should continue)
- Finding #97: State transitions don't clean up per-camera state → ✅ **CORRECT BEHAVIOR** (state should persist for recovery)
- See `edge/orchestrator/docs/state-mng-code-review.md` sections 1 and 2 for detailed rationale

**Code Locations:**
- `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:540-543` (WireGuard disconnect)
- `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:575-583` (HTTPS disconnect)
- `edge/orchestrator/internal/state-mng/impl/event_storage.go` (Security event queuing and sync)

### Section 1.3: State Consistency Improvements

#### Subsection 1.3.1: Fix State Persistence Error Handling ✅ DONE
- **Status:** ✅ DONE
- **Description:** Add error checking for state persistence calls
- **Scope:** 
  - Check errors from `persistStateToStorage` calls
  - Implement retry logic for persistence failures
  - Add logging/alerting for persistence failures
- **Dependencies:** None
- **Related Findings:**
  - Finding #98: State persistence is fire-and-forget without error checking
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:2381-2524` (persistStateToStorage with retry logic)
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:2370-2380` (persistStateToStorageWithErrorHandling helper)
  - `edge/orchestrator/internal/state-mng/types/config.go:33-36, 85-91, 110-113` (retry configuration)
  - Multiple call sites updated to handle errors properly
- **Refactoring Details:**
  - **Retry Logic Implementation:**
    - Added exponential backoff retry mechanism to `persistStateToStorage`
    - Configurable max retries (default: 3) and initial backoff (default: 1 second)
    - Backoff increases exponentially: `backoff * 2^attempt`, capped at 10 seconds
    - Context cancellation is respected during retry attempts
    - All retry attempts are logged with appropriate log levels (WARN for retries, ERROR for final failure)
  - **Configuration:**
    - Added `StatePersistenceMaxRetries` field to `StateManagerConfig` (default: 3, range: 0-10)
    - Added `StatePersistenceRetryBackoff` field to `StateManagerConfig` (default: 1s, minimum: 100ms)
    - Configuration is validated at startup
    - Configuration is applied in `SetConfig()` method
  - **Error Handling:**
    - All `persistStateToStorage` call sites now properly handle errors
    - Added `persistStateToStorageWithErrorHandling` helper function for error paths where persistence failure shouldn't block the operation
    - Errors are logged with context (operation name, connection state, camera count)
    - Critical operations log errors, non-critical operations log warnings
  - **Unit Tests:**
    - `TestPersistStateToStorage_RetryLogic`: Tests successful retry after initial failures
    - `TestPersistStateToStorage_AllRetriesFail`: Tests error return after all retries exhausted
    - `TestPersistStateToStorage_NoRetries`: Tests behavior with max retries = 0
    - `TestPersistStateToStorage_ContextCancellation`: Tests context cancellation during retries
    - `TestPersistStateToStorage_ExponentialBackoff`: Tests exponential backoff timing
    - `TestPersistStateToStorage_BackoffCapped`: Tests that backoff is capped at 10 seconds
    - `TestPersistStateToStorageWithErrorHandling`: Tests the helper function
    - `TestPersistStateToStorage_Configuration`: Tests configuration application
  - **Benefits:**
    - Transient storage failures are automatically retried
    - Persistent failures are logged with full context for debugging
    - Configurable retry behavior allows tuning based on storage characteristics
    - Context cancellation prevents indefinite blocking
    - Exponential backoff reduces load on storage during outages

#### Subsection 1.3.2: Handle Out-of-Order Model Deployment ✅ DONE
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
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1462-1524` (handleModelDeployed)
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1000-1058` (handleScreenshotSetReady)
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:103-110` (PendingModelDeployment struct)
- **Refactoring Details:**
  - **Added `pendingModelDeployments` map**: A thread-safe map (`pendingModelDeployments map[string]*PendingModelDeployment`) protected by `pendingModelDeployMu` mutex to queue model deployment events that arrive when the camera is not in `screenshot_set_ready` state.
  - **Updated `handleModelDeployed`**: Modified to check if the camera is in `screenshot_set_ready` state. If yes, it transitions the camera to `model_deployed` immediately. If not, it queues the deployment event for later processing.
  - **Updated `handleScreenshotSetReady`**: Modified to check for pending model deployments when a camera reaches `screenshot_set_ready` state. If a pending deployment exists, it processes it by calling `handleModelDeployed` with the queued event data, then clears the pending deployment from the queue.
  - **Added `PendingModelDeployment` struct**: Stores the model ID, camera ID, event data, and timestamp for queued deployments.
  - **Thread Safety**: All access to `pendingModelDeployments` is protected by `pendingModelDeployMu` mutex to ensure thread-safe operations.
  - **Unit Tests**: Added comprehensive unit tests covering:
    - Multiple queued deployments for the same camera
    - Deployment queued then processed when camera reaches ready state
    - Concurrent deployments
    - Deployment queued then camera enters error state
    - Missing model_id or camera_id in deployment events
    - Different cameras with independent deployments

#### Subsection 1.3.3: Document Missing States ✅ DONE
- **Description:** Update documentation to include implemented but undocumented states
- **Scope:** Add documentation for `wg_connecting`, `http_connecting` states
- **Dependencies:** None
- **Related Findings:**
  - Finding #21: Documentation omits implemented states
  - Finding #101: Missing states not documented
- **Code Locations:**
  - `edge/orchestrator/docs/state-transition.md` - State transition documentation
  - `edge/orchestrator/internal/state-mng/types/connection_state.go` - State definitions
  - `edge/orchestrator/internal/state-mng/impl/connection_state_machine.go` - State machine implementation
- **Refactoring Details:**
  - **Added `wg_connecting` state documentation**: Documented the intermediate state that occurs when WireGuard connection is being established. This state is entered when `initiateWireGuardConnection()` is called and transitions to `wireguard_connected` when the tunnel is successfully established, or to `wg_connection_error` if the connection fails or times out (60 seconds).
  - **Added `http_connecting` state documentation**: Documented the intermediate state that occurs when HTTPS connection is being established. This state is entered when `initiateHTTPConnection()` is called and transitions to `https_connected` when HTTPS services are ready, or to `http_connection_error` if the connection fails.
  - **Updated state list**: Added `wg_connecting` and `http_connecting` to the states list in the documentation, along with error states (`wg_connection_error`, `http_connection_error`).
  - **Updated state diagram**: Modified the state diagram to show the complete transition path including intermediate states: `disconnected` → `wg_connecting` → `wireguard_connected` → `http_connecting` → `https_connected` → `authenticated`.
  - **Updated transition descriptions**: Added detailed descriptions for transitions involving intermediate states, including error handling and timeout behavior.
  - **Updated business flow diagram**: Updated the Phase 1 initialization flow to include intermediate states.
  - **Updated VM Gateway startup sequence**: Added documentation about when intermediate states are entered and how they transition.
  - **Note on `initializing` state**: The review mentioned an `initializing` state, but this state does not exist in the implementation. The word "initializing" appears only in log messages, not as an actual state. The initial state is `disconnected`.

### Section 1.4: Move State Machines into Owning Services (NEW)

**Status:** ⬜ TODO  
**Goal:** Align implementation with the Section 1.0 target architecture: `vm-gateway` owns connection state; `iot` owns per-device state; `state-mng` orchestrates only.

#### Subsection 1.4.1: Move Connection State Machine into `vm-gateway` ✅ DONE
- **Description:** Connection state machine should be part of `vm-gateway` implementation and exposed via the `vmgateway.VMGateway` top interface.
- **Scope:**
  - Define/relocate connection state types under `edge/orchestrator/internal/vm-gateway/types/` (or reuse existing `state-mng/types` types if kept reusable)
  - Implement connection state machine inside `edge/orchestrator/internal/vm-gateway/.../impl` (service-owned)
  - Expose connection state via `vmgateway.VMGateway` (e.g., `GetConnectionStateInfo()` or `ConnectionStateMachine()` accessor)
  - Update `state-mng` to stop embedding its own connection state machine and instead query `vm-gateway`
- **Dependencies:** Epic 1 completed (current baseline), VM gateway stability
- **Notes:**
  - Preserve the "types vs impl" separation rule (no impl in types).
  - Avoid import cycles: `vm-gateway` should not depend on `state-mng/impl`.
- **Code Locations:**
  - `edge/orchestrator/internal/vm-gateway/types/connection_state.go` - Connection state types and `ConnectionStateMachine` interface (moved from `state-mng/types`)
  - `edge/orchestrator/internal/vm-gateway/http-impl/impl/connection_state_machine.go` - Connection state machine implementation (moved from `state-mng/impl`)
  - `edge/orchestrator/internal/vm-gateway/vm_gateway.go` - Updated `VMGateway` interface with connection state methods:
    - `GetConnectionState() vmgatewaytypes.ConnectionState`
    - `GetConnectionStateInfo() vmgatewaytypes.ConnectionStateInfo`
    - `TransitionConnectionState(newState vmgatewaytypes.ConnectionState, errorMsg string) error`
    - `CanTransitionConnectionState(newState vmgatewaytypes.ConnectionState) bool`
    - `IsConnectionAuthenticated() bool`
  - `edge/orchestrator/internal/vm-gateway/http-impl/vm-gateway-http-impl.go` - Implementation of connection state methods via embedded `ConnectionStateMachineImpl`
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` - Refactored to remove internal `connectionStateMachine` field and use `vmGateway` interface for all connection state operations
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl_test.go` - Updated tests to use `mockVMGateway` instead of direct `connectionStateMachine` access
- **Implementation Summary:**
  - **Types Migration:** Moved `ConnectionState` enum, `ConnectionStateInfo` struct, and `ConnectionStateMachine` interface from `state-mng/types` to `vm-gateway/types`
  - **Implementation Migration:** Moved `ConnectionStateMachineImpl` and related helper functions from `state-mng/impl` to `vm-gateway/http-impl/impl`
  - **Interface Extension:** Extended `VMGateway` interface with 5 new methods for connection state management, maintaining backward compatibility
  - **Gateway Implementation:** `VmGatewayHttpImpl` now embeds `ConnectionStateMachineImpl` and delegates all connection state operations to it
  - **State Manager Refactoring:** Removed all direct `connectionStateMachine` field access from `StateManagerImpl`, replacing with `vmGateway` interface calls throughout:
    - `Start()`, `updateStateForEvent()`, `executeWorkflow()`, `persistStateToStorage()`, `restoreStateFromStorage()`
    - `getConnectionState()`, `initiateWireGuardConnection()`, `initiateHTTPConnection()`, `checkServicesHealth()`
    - `handleConnectionErrorState()`, `syncCamerasWithVM()`, `syncScreenshotsToVM()`, `recoverActiveWorkflows()`
  - **Test Updates:** All test files updated to use `mockVMGateway` with proper expectations instead of direct state machine access
  - **Architecture Alignment:** Connection state machine is now owned by `vm-gateway` service, making `state-mng` a pure orchestrator that queries state rather than managing it

#### Subsection 1.4.2: Adopt `iot` Device State Machines as the Source of Truth ✅ DONE
- **Description:** Per-device state machines should live in `iot` and be exposed via `iot` top interfaces. `state-mng` consumes these, it doesn't implement them.
- **Scope:**
  - Standardize device state access (e.g., via `iot.DeviceStateMachineRegistry` or a dedicated `iot.DeviceStateService`)
  - Ensure cameras use the `iot` device state machine as the canonical state source (camera-specific workflow state can be modeled as metadata / adapter)
  - Update `state-mng` to stop embedding camera state machines and instead query device state from `iot`
- **Dependencies:** Epic 12, Subsection 12.3.2 (device state machines) ✅ DONE
- **Status:** ✅ DONE
- **Code Locations:**
  - `edge/orchestrator/internal/iot/device-state-service.go` - Top interface for device state service (NEW)
  - `edge/orchestrator/internal/iot/camera_state_adapter.go` - Adapter bridging DeviceStateMachine to CameraStateMachine interface (NEW)
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` - Updated to use `DeviceStateService` instead of managing its own state machines
  - `edge/orchestrator/internal/state-mng/types/camera_state.go` - Added `CameraStateMachineWithMetadata` interface for backward compatibility
- **Implementation Summary:**
  - **Top Interface Pattern:** Created `DeviceStateService` interface in `iot` package following the "top interface first" pattern. This service wraps `DeviceStateMachineRegistry` and provides the canonical interface for accessing device state machines.
  - **Service Implementation:** Implemented `deviceStateServiceImpl` that delegates to `DeviceStateMachineRegistry`. Added `NewDeviceStateServiceWithDefaults()` convenience function that creates a factory, registers default transitions, and returns a configured service.
  - **Adapter Pattern:** Created `CameraStateMachineAdapter` that adapts `DeviceStateMachine` to the `CameraStateMachine` interface. This allows `state-mng` to continue using the `CameraStateMachine` interface while the actual state is managed by `iot` device state machines. The adapter:
    - Maps camera-specific workflow states (synced, waiting_for_screenshots, screenshot_set_ready, model_deployed, frame_processing) to generic device states (registered, active, processing)
    - Stores camera-specific metadata (model_id, dataset_id) in device state metadata
    - Maintains backward compatibility with existing `CameraStateMachine` interface
  - **State Manager Refactoring:** Updated `StateManagerImpl` to:
    - Add `deviceStateService` field and `SetDeviceStateService()` method
    - Replace `cameraStateMachines` map with `cameraStateMachineAdapters` cache (for backward compatibility)
    - Update `getOrCreateCameraStateMachine()` to use `DeviceStateService.GetOrCreateStateMachine()` and wrap result in `CameraStateMachineAdapter`
    - Update `getCameraStateMachine()` to query `DeviceStateService` and cache adapters
    - Update `getAllCameraStateMachines()` to query `DeviceStateService.GetStateMachinesByType(DeviceTypeCamera)` and wrap results
    - Maintain fallback to old `CameraStateMachineImpl` if `DeviceStateService` is not available (for gradual migration)
  - **Backward Compatibility:** Added `CameraStateMachineWithMetadata` interface that extends `CameraStateMachine` with `SetModelID()` and `SetDatasetID()` methods. Both `CameraStateMachineImpl` and `CameraStateMachineAdapter` implement this interface, allowing existing code to work without changes.
  - **State Persistence:** State persistence continues to work because:
    - `getAllCameraStateMachines()` returns adapters that implement `CameraStateMachine` interface
    - `GetState()` and `GetStateInfo()` methods work correctly through the adapter
    - State restoration creates device state machines via `DeviceStateService` and wraps them in adapters
- **Architecture Benefits:**
  - **Single Source of Truth:** Device state machines are now owned by `iot` package, making them the canonical source of device state
  - **Service Boundaries:** `state-mng` no longer embeds state machine implementations; it queries state from `iot` service
  - **Extensibility:** New device types can be added to `iot` without changes to `state-mng`
  - **Testability:** `DeviceStateService` can be easily mocked for testing
  - **Backward Compatibility:** Existing code continues to work through adapters
- **Migration Notes:**
  - The old `CameraStateMachineImpl` is still available as a fallback if `DeviceStateService` is not set
  - Camera-specific workflow states are stored in device state metadata, not as primary states
  - State transitions use generic device states (undiscovered, discovered, registered, active, processing) with workflow-specific metadata
  - The adapter handles mapping between camera workflow states and generic device states transparently

#### Subsection 1.4.3: Make `state-mng` a Pure Workflow Orchestrator (No Embedded State Machines) ✅ DONE
- **Description:** Refactor `state-mng` to orchestrate workflows based on observed state, without owning state machine implementations.
- **Scope:**
  - Replace internal `connectionStateMachine` usage with `vm-gateway` state queries
  - Replace internal `cameraStateMachines` usage with `iot` device state queries
  - Keep `state-mng` responsible for:
    - event handling and workflow triggering,
    - recovery orchestration (calling into services),
    - cross-service coordination and sequencing.
- **Dependencies:** 1.4.1 ✅ DONE, 1.4.2 ✅ DONE
- **Status:** ✅ DONE
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go` - Updated to query state from services instead of managing state machines
  - ✅ `edge/orchestrator/internal/state-mng/impl/camera_state_machine.go` - DELETED (old implementation removed)
  - ✅ `edge/orchestrator/internal/state-mng/impl/connection_state_machine.go` - DELETED (old implementation removed)
- **Implementation Summary:**
  - **Architecture Change:** `StateManagerImpl` is now a pure workflow orchestrator that does NOT own state machine implementations. It queries state from services and orchestrates workflows based on observed state.
  - **Connection State:** All connection state queries go through `vm-gateway.VMGateway` interface:
    - `m.vmGateway.GetConnectionState()` - Query current connection state
    - `m.vmGateway.TransitionConnectionState()` - Request state transitions (delegated to vm-gateway)
    - `m.vmGateway.GetConnectionStateInfo()` - Get detailed connection state information
    - Connection state machine is owned and managed by `vm-gateway` service
  - **Device State:** All device state queries go through `iot.DeviceStateService` interface:
    - `m.deviceStateService.GetOrCreateStateMachine()` - Get or create device state machine
    - `m.deviceStateService.GetStateMachine()` - Get existing device state machine
    - `m.deviceStateService.GetStateMachinesByType()` - Get all state machines of a type
    - Device state machines are owned and managed by `iot.DeviceStateService`
    - Results are wrapped in `CameraStateMachineAdapter` for backward compatibility
  - **State Manager Responsibilities:** `StateManagerImpl` is now responsible only for:
    - **Event Handling:** Processing events from the event bus and triggering appropriate workflows
    - **Workflow Orchestration:** Coordinating multi-step workflows (camera discovery → sync → screenshots → model deployment → frame processing)
    - **Recovery Orchestration:** Calling into services to recover from error states
    - **Cross-Service Coordination:** Sequencing operations across multiple services (CCTV, AI gateway, VM gateway, etc.)
    - **State Observation:** Querying state from services to make workflow decisions
  - **Service Requirements:** `StateManagerImpl` now requires services to be set:
    - `deviceStateService` must be set via `SetDeviceStateService()` before creating camera state machines
    - `vmGateway` must be set via `SetVMGateway()` before querying connection state
    - If services are not set, methods return errors or nil instead of creating fallback implementations
  - **Code Documentation:** Added comprehensive architecture documentation to `StateManagerImpl`:
    - Clear explanation that it's a pure workflow orchestrator
    - Documentation of state machine ownership (vm-gateway for connection, iot for devices)
    - Explanation of service requirements (services must be set before use)
- **Architecture Benefits:**
  - **Clear Separation of Concerns:** State machines are owned by their respective services (vm-gateway for connection, iot for devices), not by state-mng
  - **Single Source of Truth:** Each state machine has a single owner, eliminating state synchronization issues
  - **Service Autonomy:** Services can manage their own state machines independently
  - **Testability:** State-mng can be tested by mocking service interfaces without needing to mock state machine implementations
  - **Extensibility:** New device types can be added to iot service without changes to state-mng
  - **Maintainability:** State machine logic is centralized in the services that own them
- **Cleanup:**
  - ✅ Old state machine files (`camera_state_machine.go`, `connection_state_machine.go`) have been deleted
  - ✅ Old test files (`camera_state_machine_test.go`, `connection_state_machine_test.go`) have been deleted
  - ✅ Fallback paths in `getOrCreateCameraStateMachine()` have been removed
  - ✅ All state queries now go through service interfaces, ensuring proper service boundaries
  - ✅ `StateManagerImpl` now requires services to be set and returns errors if they are not available
- **Verification:**
  - ✅ Connection state is only queried from `vm-gateway.VMGateway` interface
  - ✅ Device state is only queried from `iot.DeviceStateService` interface
  - ✅ No direct state machine creation - all state machines are owned by services
  - ✅ StateManagerImpl only orchestrates workflows based on observed state
  - ✅ All state transitions are delegated to service interfaces
  - ✅ Old state machine implementations have been completely removed
  - ✅ Tests updated to use DeviceStateService instead of old implementations

---

## Epic 12: Device Abstraction Layer

**Priority:** High  
**Goal:** Prepare architecture for multiple IoT device types beyond CCTV cameras by introducing device abstraction and plugin architecture.

**Rationale:** Current architecture is camera-centric (`frameProcessing`, `screenshotSync`, `CCTV service`). Adding device abstraction early will enable easy extension to other IoT device types (sensors, access control, etc.) without major refactoring.

### Section 12.1: Device Type Abstraction

#### Subsection 12.1.1: Define Generic Device Interface ✅ DONE
- **Description:** Create device-agnostic interface that abstracts device capabilities
- **Scope:** 
  - Define `Device` interface with lifecycle methods (discover, register, connect, disconnect)
  - Abstract capture/data collection mechanisms
  - Create device capability negotiation framework
  - Design device metadata schema (type, capabilities, status)
- **Dependencies:** None
- **Code Locations:**
  - New interface package: `edge/orchestrator/internal/iot/device-iface.go`
- **Refactoring Details:**
  - **Created `Device` interface**: Comprehensive interface with lifecycle methods (`Start`, `Stop`), metadata management (`GetMetadata`, `UpdateMetadata`), enable/disable operations, and capability-based operations.
  - **Lifecycle methods**: `Start()` and `Stop()` for device initialization and cleanup, `Enable()` and `Disable()` for operational control.
  - **Capability-based operations**: Methods like `CaptureData()`, `StartDataStream()`, `ReadSensor()`, `ExecuteCommand()` that check device capabilities before execution. This allows the same interface to work for cameras, sensors, access control devices, etc.
  - **Device metadata schema**: `DeviceMetadata` struct includes core identification (ID, name, type, manufacturer, model), status information (enabled, status, last seen), capabilities, configuration, network/physical connection info, and location/zone information.
  - **Device types**: Defined comprehensive device type constants including:
    - Video devices: `camera`
    - Sensor devices: `motion_sensor`, `temperature_sensor`, `humidity_sensor`, `door_sensor`, `window_sensor`, `smoke_detector`, `co2_sensor`, `sensor` (generic)
    - Access control: `door_lock`, `keypad`, `card_reader`, `biometric`
    - Audio: `microphone`
    - Other: `unknown`
  - **Device capabilities**: Extensible capability system with constants for:
    - Data operations: `data_capture`, `data_streaming`
    - Video-specific: `video_capture`, `video_streaming`, `video_recording`, `snapshot`
    - Audio: `audio_capture`, `audio_streaming`
    - Sensors: `sensor_readings`
    - Control: `control`, `access_control`, `ptz`, `motion_detection`, `event_generation`
  - **DeviceCapabilities type**: Map-based capability set with helper methods (`Has`, `Add`, `Remove`) for capability management.
  - **DeviceData abstraction**: Generic `DeviceData` struct that can represent video frames, audio samples, sensor readings, or events, with type information and metadata.
  - **DeviceRegistry interface**: Interface for device discovery and registration, supporting discovery by type, capability filtering, and device management operations.
  - **DevicePlugin interface**: Plugin system for adding new device types at runtime, with methods for device type registration, discovery, creation, and metadata validation.
  - **Device status types**: `unknown`, `online`, `offline`, `connecting`, `error`, `maintenance`.
  - **Device filters**: `DeviceFilters` struct for querying devices by type, capability, enabled status, status, zone, or tags.
  - **Generated mocks**: Created mock interfaces for `Device` and `DeviceRegistry` using `go.uber.org/mock` for testing.
  - **Design principles**:
    - **Capability-based**: Operations check capabilities before execution, allowing graceful handling of unsupported features
    - **Extensible**: New device types can be added via plugins without modifying core interfaces
    - **Type-safe**: Strong typing for device types, capabilities, and data types
    - **Backward compatible**: Existing camera functionality can be wrapped as a `Device` implementation
    - **Flexible metadata**: Device-specific configuration stored in flexible `map[string]interface{}` fields

#### Subsection 12.1.2: Refactor Camera as Device Implementation ✅ DONE
- **Description:** Implement Camera as concrete `Device` interface implementation
- **Scope:** 
  - Wrap existing CCTV service as `CameraDevice` implementation
  - Map camera-specific operations to generic device interface
  - Maintain backward compatibility during transition
- **Dependencies:** 12.1.1
- **Code Locations:**
  - `edge/orchestrator/internal/iot/cctv/device_adapter.go` - CameraDevice adapter implementation
- **Refactoring Details:**
  - **Created `CameraDevice` adapter**: Implements the `Device` interface by wrapping the existing `CCTVService`. This is an **adapter pattern** that allows cameras to be used in device-agnostic code without breaking existing CCTVService consumers.
  - **Backward compatibility preserved**: The `CCTVService` interface remains **completely unchanged**. All existing code (state manager, web gateway, AI gateway, VM gateway) continues to use `CCTVService` directly with no modifications required.
  - **Dual interface support**: 
    - **Existing code**: Uses `CCTVService` interface (no changes needed)
    - **New device-agnostic code**: Uses `Device` interface, can work with `CameraDevice` or other device types
    - **Device registry**: Can register `CameraDevice` instances alongside other device types
  - **Lifecycle methods**: `Start()` and `Stop()` delegate to CCTV service lifecycle (typically no-op as CCTV service manages camera lifecycle). `Enable()` and `Disable()` map to `EnableCamera()` and `DisableCamera()`.
  - **Metadata mapping**: `GetMetadata()` converts `types.Camera` to `iot.DeviceMetadata`, mapping:
    - Camera type → `DeviceTypeCamera`
    - Camera status → Device status (`online`, `offline`, `connecting`, `error`, `unknown`)
    - Camera capabilities → Device capabilities (video capture, streaming, recording, snapshot, PTZ, motion detection, event generation)
    - Camera config → Device config map (recording_enabled, motion_detection, quality, frame_rate, resolution)
    - Network/physical info → Device endpoints and device path
  - **Capability mapping**: `buildCapabilitiesFromCamera()` maps camera capabilities to device capabilities:
    - All cameras: `data_capture`, `video_capture`, `video_streaming`, `video_recording`, `snapshot`, `data_streaming`, `event_generation`
    - PTZ cameras: `ptz`, `control`
    - Motion detection enabled: `motion_detection`
  - **Data capture**: `CaptureData()` maps `CaptureFrame()` to `DeviceData` with video frame type and JPEG format.
  - **Data streaming**: `StartDataStream()` wraps `StartMJPEGStream()` and converts MJPEG frames to `DeviceData` stream. `StopDataStream()` stops the MJPEG stream.
  - **Sensor operations**: `ReadSensor()` and `ReadAllSensors()` return errors (cameras don't have sensors).
  - **Control operations**: `ExecuteCommand()` supports PTZ commands (when PTZ capability is available). `GetAvailableCommands()` returns supported commands based on camera capabilities.
  - **Metadata updates**: `UpdateMetadata()` converts `DeviceMetadataUpdate` to `CameraUpdate` and updates via CCTV service, then refreshes metadata cache.
  - **Metadata caching**: Device metadata is cached and refreshed when camera changes. `RefreshMetadata()` method allows manual refresh.
  - **Thread safety**: Uses mutexes (`metadataMu`, `streamsMu`) to protect metadata cache and active streams map.
  - **Access to underlying service**: `GetCCTVService()` method allows code that needs camera-specific features to access the full `CCTVService` interface.
  - **Factory function**: `NewCameraDevice()` creates a `CameraDevice` adapter for a specific camera, verifying the camera exists and initializing metadata.
  - **Stream management**: Tracks active data streams in `activeStreams` map, automatically cleaning up on `Stop()`.
  - **Error handling**: All methods return descriptive errors, with capability checks before executing operations.
  - **Design pattern**: This is a classic **Adapter Pattern** - `CameraDevice` adapts `CCTVService` to the `Device` interface without modifying either interface.

#### Subsection 12.1.3: Create Device Capability Framework ✅ DONE
- **Description:** Design extensible capability system for different device types
- **Scope:** 
  - Define capability types (video_capture, sensor_readings, audio_capture, access_control, etc.)
  - Create capability negotiation protocol
  - Support capability queries and filtering
- **Dependencies:** 12.1.1
- **Code Locations:**
  - `edge/orchestrator/internal/iot/capabilities.go` - Capability framework implementation
  - `edge/orchestrator/internal/iot/device-iface.go` - Enhanced DeviceCapabilities type with utility methods
- **Refactoring Details:**
  - **Capability negotiation protocol**: Created `CapabilityNegotiation` type and `NegotiateCapabilities()` function that negotiates capabilities between a device and a set of requirements. Supports required and optional capabilities, tracks missing and unavailable capabilities, and validates dependencies.
  - **Capability requirements**: Created `CapabilityRequirement` type that specifies required/optional capabilities with descriptions and dependencies. Used in negotiation to validate device compatibility.
  - **Capability validation**: Added `ValidateCapabilityRequirements()` function to validate that a device meets all capability requirements before use.
  - **Capability groups**: Created `CapabilityGroup` type with groups: `data`, `video`, `audio`, `sensors`, `control`, `access`, `events`. Added `GetCapabilityGroup()` to categorize capabilities and `GetCapabilitiesByGroup()` to get all capabilities in a group.
  - **Capability queries**: Created `CapabilityQuery` type with support for:
    - Required capabilities (all must be present)
    - Any-of capabilities (at least one must be present)
    - Excluded capabilities (none should be present)
    - Group filter (devices must have at least one capability from a group)
    - Minimum capabilities count
  - **Query matching**: Added `Matches()` method to `CapabilityQuery` that checks if a device matches the query criteria.
  - **Device registry queries**: Added `QueryDevicesByCapability()` function to query devices from a registry using capability queries.
  - **Capability filter utilities**: Created `CapabilityFilter` type with builder methods:
    - `WithRequired()` - require specific capabilities
    - `WithAnyOf()` - require at least one capability
    - `WithExcluded()` - exclude devices with capabilities
    - `WithGroup()` - require capabilities from a group
    - `WithMinCapabilities()` - require minimum capability count
    - `Combine()` - combine multiple queries with AND logic
  - **Capability dependencies**: Created `CapabilityDependency` type and `KnownCapabilityDependencies()` function that defines capability dependencies (e.g., PTZ requires Control, VideoStreaming requires VideoCapture). Added `ValidateCapabilityDependencies()` to validate device dependencies.
  - **Enhanced DeviceCapabilities type**: Added utility methods to `DeviceCapabilities`:
    - `HasAll()` - check if device has all specified capabilities
    - `HasAny()` - check if device has any of specified capabilities
    - `Count()` - get number of capabilities
    - `List()` - get all capabilities as a slice
    - `Intersect()` - get capabilities present in both sets
    - `Union()` - get capabilities present in either set
    - `Difference()` - get capabilities in this set but not in other
  - **Capability descriptions**: Added `GetCapabilityDescription()` and `GetCapabilityName()` functions to provide human-readable descriptions and names for capabilities.
  - **Capability summary**: Created `CapabilitySummary` type and `GetCapabilitySummary()` function that provides a summary of device capabilities including total count, capabilities by group, and list of all capabilities.
  - **Design principles**:
    - **Extensible**: New capabilities can be added without breaking existing code
    - **Type-safe**: Strong typing for all capability operations
    - **Queryable**: Rich query and filtering capabilities
    - **Validated**: Dependency validation ensures devices have required capabilities
    - **Grouped**: Capabilities organized into logical groups for easier management

### Section 12.2: Device Plugin Architecture

#### Subsection 12.2.1: Design Device Plugin System ✅ DONE
- **Description:** Create plugin/adapter pattern for adding new device types
- **Scope:** 
  - Define device plugin registration interface
  - Create device type registry
  - Support runtime device type registration
  - Design plugin discovery mechanism
- **Dependencies:** 12.1.1
- **Code Locations:**
  - `edge/orchestrator/internal/iot/plugin_registry.go` - Plugin registry implementation
  - `edge/orchestrator/internal/iot/device-iface.go` - DevicePlugin interface (already defined)
- **Refactoring Details:**
  - **DevicePluginRegistry interface**: Created comprehensive interface for managing device type plugins with methods for registration, discovery, device creation, and metadata validation. Supports runtime plugin registration and unregistration.
  - **Plugin registration**: `RegisterPlugin()` registers a plugin for a device type with validation. Prevents duplicate registrations and validates plugin before registration. `UnregisterPlugin()` removes a plugin at runtime.
  - **Plugin retrieval**: `GetPlugin()` and `GetPluginForDeviceType()` retrieve plugins by device type. `ListPlugins()` returns all registered plugins. `IsDeviceTypeSupported()` checks if a device type has a registered plugin.
  - **Device discovery**: `DiscoverDevices()` discovers devices using all registered plugins. `DiscoverDevicesByType()` discovers devices of a specific type using the appropriate plugin. Errors from one plugin don't stop discovery from other plugins.
  - **Device creation**: `CreateDevice()` creates a device instance from metadata using the appropriate plugin. Validates metadata before creation. `ValidateMetadata()` validates device metadata using the plugin's validation logic.
  - **Plugin validation**: `validatePlugin()` validates plugins before registration, checking device type, supported capabilities, and basic capability validation.
  - **Thread safety**: All operations are protected with `sync.RWMutex` for concurrent access. Read operations use `RLock()`, write operations use `Lock()`.
  - **PluginManager**: High-level manager that wraps the registry and provides convenient methods for common operations. Simplifies plugin management for consumers.
  - **Plugin discovery mechanism**: Created `PluginDiscoveryConfig` and `PluginDiscoveryResult` types for future file-based plugin discovery. `DiscoverPlugins()` function is a placeholder for future dynamic plugin loading (currently plugins must be registered manually).
  - **Plugin discovery errors**: `PluginDiscoveryError` type tracks plugins that fail to load during discovery, allowing graceful handling of plugin loading failures.
  - **Supported device types**: `GetSupportedDeviceTypes()` returns all device types that have registered plugins, enabling runtime queries of available device types.
  - **Design patterns**:
    - **Registry Pattern**: Central registry manages all plugins
    - **Plugin Pattern**: Extensible system for adding new device types
    - **Factory Pattern**: Plugins act as factories for creating device instances
    - **Strategy Pattern**: Different plugins provide different discovery and creation strategies
  - **Usage example**:
    ```go
    // Create registry
    registry := iot.NewDevicePluginRegistry()
    
    // Register camera plugin
    cameraPlugin := &CameraDevicePlugin{...}
    registry.RegisterPlugin(cameraPlugin)
    
    // Discover devices
    devices, _ := registry.DiscoverDevices(ctx)
    
    // Create device from metadata
    device, _ := registry.CreateDevice(ctx, metadata)
    ```
  - **Future extensibility**: The discovery mechanism is designed to support:
    - File-based plugin discovery (scanning directories for .so files)
    - Dynamic plugin loading
    - Plugin versioning and compatibility checking
    - Plugin lifecycle management (start/stop plugins)
  - **Integration points**: The plugin registry can be integrated with:
    - Device registry for automatic device registration after discovery
    - State manager for device state management
    - Configuration system for plugin configuration
    - Event bus for plugin lifecycle events

#### Subsection 12.2.2: Implement Device Lifecycle Hooks ✅ DONE
- **Description:** Define and implement standard device lifecycle hooks
- **Scope:** 
  - Discovery hooks (device detection, identification)
  - Registration hooks (initialization, capability reporting)
  - Data collection hooks (capture, streaming, polling)
  - Teardown hooks (cleanup, resource release)
- **Dependencies:** 12.2.1
- **Code Locations:**
  - `edge/orchestrator/internal/iot/lifecycle_hooks.go` - Lifecycle hooks implementation
- **Refactoring Details:**
  - **Lifecycle hook types**: Defined four hook types: `HookTypeDiscovery`, `HookTypeRegistration`, `HookTypeDataCollection`, `HookTypeTeardown` covering all device lifecycle stages.
  - **Discovery hooks**: `DiscoveryHook` function type called during device discovery. `DiscoveryHookContext` provides device type, plugin, discovered devices, and metadata. Hooks can filter devices, add metadata, perform identification/validation, and log discovery events.
  - **Registration hooks**: `RegistrationHook` function type called during device registration. `RegistrationHookContext` provides device, metadata, registry, capabilities, and additional metadata. Hooks can validate devices, initialize resources, report capabilities, set up monitoring, and configure settings.
  - **Data collection hooks**: `DataCollectionHook` function type called during data collection operations (capture, streaming, polling). `DataCollectionHookContext` provides device, data type, data, operation type, and metadata. Hooks can pre/post-process data, monitor operations, route data to external systems, validate data quality, and handle errors.
  - **Teardown hooks**: `TeardownHook` function type called during device teardown. `TeardownHookContext` provides device, reason for teardown, and metadata. Hooks can clean up resources, release connections, save state, notify external systems, and log events.
  - **LifecycleHook type**: Represents a registered hook with ID, type, name, description, priority, hook function, enabled status, and filters (device type, capability). Supports filtering hooks by device type and capability.
  - **LifecycleHookRegistry interface**: Comprehensive interface for managing hooks with methods for registration, unregistration, retrieval, listing, and execution. Supports filtering and priority-based execution.
  - **Hook execution**: Hooks are executed in priority order (lower priority = earlier execution). Hooks can be filtered by device type and capability. Disabled hooks are skipped. Errors from one hook don't stop execution of other hooks (allows multiple hooks to run).
  - **Thread safety**: All operations protected with `sync.RWMutex`. Read operations use `RLock()`, write operations use `Lock()`.
  - **Hook filtering**: Hooks can be filtered by device type (`DeviceTypeFilter`) and capability (`CapabilityFilter`). Only matching hooks are executed, improving performance and allowing device-specific behavior.
  - **Priority-based execution**: Hooks are sorted by priority and executed in order. Lower priority hooks execute first, allowing dependency ordering (e.g., validation hooks before processing hooks).
  - **LifecycleHookManager**: High-level manager wrapping the registry with convenient methods for hook management and execution.
  - **HookBuilder**: Fluent builder pattern for creating hooks with methods: `WithDescription()`, `WithPriority()`, `WithDeviceTypeFilter()`, `WithCapabilityFilter()`, `WithDiscoveryHook()`, `WithRegistrationHook()`, `WithDataCollectionHook()`, `WithTeardownHook()`, `WithEnabled()`, `Build()`.
  - **Integration points**: Hooks can be integrated at:
    - **Discovery**: In `DevicePluginRegistry.DiscoverDevices()` and `DevicePluginRegistry.DiscoverDevicesByType()`
    - **Registration**: In `DeviceRegistry.RegisterDevice()`
    - **Data collection**: In `Device.CaptureData()`, `Device.StartDataStream()`, `Device.ReadSensor()`
    - **Teardown**: In `Device.Stop()`, `DeviceRegistry.DeleteDevice()`
  - **Usage example**:
    ```go
    // Create hook registry
    hookRegistry := iot.NewLifecycleHookRegistry()
    
    // Register discovery hook
    discoveryHook := iot.NewHookBuilder("device-validator", "Device Validator", iot.HookTypeDiscovery).
        WithDescription("Validates discovered devices").
        WithPriority(10).
        WithDiscoveryHook(func(ctx context.Context, hookCtx *iot.DiscoveryHookContext) error {
            // Validate devices
            return nil
        }).
        Build()
    hookRegistry.RegisterHook(discoveryHook)
    
    // Execute hooks during discovery
    hookCtx := &iot.DiscoveryHookContext{
        DeviceType: iot.DeviceTypeCamera,
        DiscoveredDevices: devices,
    }
    hookRegistry.ExecuteDiscoveryHooks(ctx, hookCtx)
    ```
  - **Hook context types**: Each hook type has a dedicated context type that provides relevant information:
    - `DiscoveryHookContext`: Device type, plugin, discovered devices, metadata
    - `RegistrationHookContext`: Device, metadata, registry, capabilities, additional metadata
    - `DataCollectionHookContext`: Device, data type, data, operation, metadata
    - `TeardownHookContext`: Device, reason, metadata
  - **Error handling**: Hook execution continues even if one hook fails, allowing multiple hooks to run. First error is returned, but all hooks are attempted.
  - **Extensibility**: New hook types can be added by extending `LifecycleHookType` and adding corresponding context types and execution methods.

### Section 12.3: Generic Data Pipeline

#### Subsection 12.3.1: Abstract Data Processing Pipeline ✅ DONE
- **Description:** Create device-agnostic data processing framework
- **Scope:** 
  - Abstract "frame processing" to "device data processing"
  - Create generic data transformation pipeline
  - Support multiple data types (video frames, sensor readings, audio, structured events)
  - Design pluggable data processors
- **Dependencies:** 12.1.1, Epic 1 (state machine separation)
- **Code Locations:**
  - `edge/orchestrator/internal/iot/data_pipeline.go` - Data pipeline implementation
  - `edge/orchestrator/internal/iot/processors.go` - Example processor implementations
- **Refactoring Details:**
  - **DataProcessor interface**: Defines a pluggable processor interface with methods: `Name()`, `Process()`, `SupportsDataType()`, `GetSupportedDataTypes()`, `GetPriority()`. Processors can transform, filter, analyze, or route device data. Processors return transformed data, nil (to drop data), or an error.
  - **DataProcessorRegistry interface**: Manages processor registration and retrieval. Methods: `RegisterProcessor()`, `UnregisterProcessor()`, `GetProcessor()`, `ListProcessors()`, `GetProcessorsForDataType()`. Thread-safe implementation with `sync.RWMutex`.
  - **Priority-based execution**: Processors are executed in priority order (lower priority = earlier in pipeline). Processors are sorted by priority when registered, enabling dependency ordering (e.g., validation before transformation).
  - **DataPipeline**: Processes device data through a series of processors. Data flows through processors in priority order. If a processor returns nil, data is dropped and processing stops. If a processor returns an error, processing stops and error is returned. Supports batch processing via `ProcessBatch()`.
  - **DataProcessingService**: High-level service wrapping the pipeline with methods: `ProcessDeviceData()`, `RegisterProcessor()`, `UnregisterProcessor()`, `ListProcessors()`, `GetProcessorsForDataType()`. Returns `DataProcessingContext` with processing results, applied processors, and duration.
  - **DataProcessingContext**: Provides context for processing operations including device, original data, processed data, processors applied, processing duration, and metadata.
  - **BaseProcessor**: Base implementation that processors can embed for default behavior. Provides default implementations for `Name()`, `SupportsDataType()`, `GetSupportedDataTypes()`, `GetPriority()`. `Process()` method must be implemented by concrete processors.
  - **ProcessorBuilder**: Fluent builder pattern for creating processors with methods: `WithSupportedTypes()`, `WithPriority()`, `WithProcessFunc()`, `Build()`. Enables easy creation of custom processors.
  - **Example processor types**:
    - **VideoFrameProcessor**: Processes video frame data (e.g., resize, compress, normalize, detect objects). Supports `DeviceDataTypeVideoFrame`.
    - **SensorDataProcessor**: Processes sensor reading data (e.g., normalize values, detect thresholds, aggregate readings). Supports `DeviceDataTypeSensorReading`.
    - **AudioDataProcessor**: Processes audio sample data (e.g., noise reduction, feature extraction, voice activity detection). Supports `DeviceDataTypeAudioSample`.
    - **EventDataProcessor**: Processes event data (e.g., enrich events, route to different handlers, aggregate events). Supports `DeviceDataTypeEvent`.
    - **MultiTypeProcessor**: Processes multiple data types using a custom function. Useful for logging, metrics, or routing processors.
    - **PassThroughProcessor**: Passes data through unchanged. Useful for testing or as a placeholder.
    - **FilterProcessor**: Filters (drops) data based on conditions. Returns nil if data should be dropped, otherwise returns data unchanged.
    - **TransformProcessor**: Transforms data using a custom function. Useful for data normalization, enrichment, or conversion.
    - **TimestampEnrichmentProcessor**: Enriches data with processing timestamp and processor name metadata.
  - **Data type support**: Pipeline supports all `DeviceDataType` values: `DeviceDataTypeVideoFrame`, `DeviceDataTypeAudioSample`, `DeviceDataTypeSensorReading`, `DeviceDataTypeEvent`, `DeviceDataTypeGeneric`.
  - **Thread safety**: All registry operations are protected with `sync.RWMutex`. Read operations use `RLock()`, write operations use `Lock()`.
  - **Error handling**: Processors can return errors to stop pipeline processing. Errors are propagated with context (processor name). Batch processing collects errors for each item without stopping the entire batch.
  - **Usage example**:
    ```go
    // Create registry and pipeline
    registry := iot.NewDataProcessorRegistry()
    service := iot.NewDataProcessingService(registry)
    
    // Register processors
    videoProcessor := iot.NewVideoFrameProcessor("resize", 10)
    registry.RegisterProcessor(videoProcessor)
    
    filterProcessor := iot.NewFilterProcessor("quality-filter", 
        []iot.DeviceDataType{iot.DeviceDataTypeVideoFrame}, 
        20,
        func(ctx context.Context, data *iot.DeviceData) bool {
            // Filter logic
            return true
        })
    registry.RegisterProcessor(filterProcessor)
    
    // Process device data
    deviceData := &iot.DeviceData{
        DeviceID: "camera-1",
        DataType: iot.DeviceDataTypeVideoFrame,
        Data: frameBytes,
    }
    ctx, err := service.ProcessDeviceData(context.Background(), device, deviceData)
    ```
  - **Integration points**: The pipeline can be integrated with:
    - **State Manager**: Replace `processFrameForCamera()` with generic `processDeviceData()` that uses the pipeline
    - **Device Registry**: Process data from any device type through the pipeline
    - **Lifecycle Hooks**: Use data collection hooks to inject data into the pipeline
    - **AI Gateway**: Video frame processors can prepare data before AI processing
    - **Storage Services**: Processors can route data to different storage backends
  - **Extensibility**: New processor types can be added by:
    - Implementing `DataProcessor` interface directly
    - Embedding `BaseProcessor` and implementing `Process()` method
    - Using `ProcessorBuilder` for quick processor creation
    - Creating specialized processor types (e.g., `VideoFrameProcessor`, `SensorDataProcessor`)
  - **Abstraction benefits**:
    - **Device-agnostic**: Same pipeline works for cameras, sensors, audio devices, etc.
    - **Pluggable**: Processors can be added/removed at runtime
    - **Composable**: Multiple processors can be chained together
    - **Testable**: Each processor can be tested independently
    - **Maintainable**: Clear separation of concerns between data capture, processing, and storage

#### Subsection 12.3.2: Implement Device State Machines ✅ DONE
- **Description:** Extend per-camera state machine to per-device state machine
- **Scope:** 
  - Generalize camera state machine to device state machine
  - Support device-type-specific state transitions
  - Maintain device-independent connection state machine
- **Dependencies:** Epic 1 (state machine refactoring), 12.1.2
- **Code Locations:**
  - `edge/orchestrator/internal/iot/device_state_machine.go` - Generic device state machine implementation
  - `edge/orchestrator/internal/iot/device_state_configs.go` - Device-type-specific state transition configurations
  - `edge/orchestrator/internal/iot/device_state_adapter.go` - Camera state adapter for backward compatibility
- **Architectural Decision:**
  - **State Manager uses `iot.DeviceStateMachine` interface**: The state manager should use the device state machine from the `iot` package instead of maintaining its own camera state machine implementation. This follows the "top interface only" rule and improves separation of concerns.
  - **Benefits:**
    - **Separation of concerns**: Device state management logic lives in `iot` package where it belongs
    - **Reusability**: Other services can use device state machines without depending on state manager
    - **Simplicity**: State manager becomes simpler - it just uses the interface
    - **Consistency**: All device types use the same state machine interface
    - **Maintainability**: Changes to device state logic only need to happen in one place
  - **Camera-specific states**: Cameras have workflow-specific states (e.g., `waiting_for_screenshots`, `screenshot_set_ready`, `model_deployed`, `frame_processing`) that are stored in device state metadata, while using generic device states (`undiscovered`, `discovered`, `registered`, `active`, `processing`) as primary states.
  - **CameraStateAdapter**: Provides backward compatibility by mapping camera-specific workflow states to generic device states and storing camera-specific information (model_id, dataset_id) in metadata.
- **Refactoring Details:**
  - **DeviceState enum**: Generic device states applicable to all device types: `undiscovered`, `discovered`, `registered`, `active`, `idle`, `processing`, `error`, `disconnected`, `disabled`. These provide a common foundation for all device types.
  - **DeviceStateInfo**: Contains device ID, device type, state, last updated timestamp, error message, metadata (for device-type-specific information), and is_active flag.
  - **DeviceStateMachine interface**: Generic interface for per-device state machines with methods: `GetDeviceID()`, `GetDeviceType()`, `GetState()`, `GetStateInfo()`, `Transition()`, `CanTransition()`, `IsOperational()`, `IsReadyForProcessing()`, `SetMetadata()`, `GetMetadata()`. Thread-safe implementation with `sync.RWMutex`.
  - **DeviceStateMachineFactory interface**: Creates device state machines for specific device types. Supports device-type-specific state transition rules via `RegisterDeviceTypeTransitions()`. Different device types can have different valid state transitions.
  - **DeviceStateMachineRegistry interface**: Manages device state machines with methods: `GetOrCreateStateMachine()`, `GetStateMachine()`, `GetAllStateMachines()`, `RemoveStateMachine()`, `GetStateMachinesByType()`. Thread-safe implementation with double-check locking pattern.
  - **Device-type-specific transitions**: Predefined state transition configurations for different device types:
    - **CameraDeviceStateTransitions**: Camera-specific transitions (maps to generic states: registered -> active -> processing)
    - **SensorDeviceStateTransitions**: Sensor-specific transitions (simpler flow: discovered -> registered -> active -> processing)
    - **AudioDeviceStateTransitions**: Audio device-specific transitions (similar to sensors)
    - **AccessControlDeviceStateTransitions**: Access control device-specific transitions (simpler operational states)
  - **Default transitions**: Generic transitions for unknown device types. All device types can transition from any state to `error` or `disconnected`.
  - **CameraStateAdapter**: Adapter that wraps `DeviceStateMachine` and provides camera-specific workflow state management:
    - Maps camera workflow states (`synced`, `waiting_for_screenshots`, `screenshot_set_ready`, `model_deployed`, `frame_processing`) to generic device states
    - Stores camera-specific information (model_id, dataset_id, workflow_state) in device state metadata
    - Provides methods: `GetCameraWorkflowState()`, `SetCameraWorkflowState()`, `TransitionToCameraWorkflowState()`, `SetModelID()`, `GetModelID()`, `SetDatasetID()`, `GetDatasetID()`, `GetCameraStateInfo()`
    - Maintains backward compatibility with existing camera state machine interface
  - **State mapping**: Camera-specific workflow states map to generic device states:
    - `synced` → `registered`
    - `waiting_for_screenshots`, `screenshot_set_ready` → `active`
    - `model_deployed`, `frame_processing` → `processing`
  - **Metadata storage**: Device-type-specific information (e.g., model_id, dataset_id, workflow_state) is stored in `DeviceStateInfo.Metadata` map, allowing generic states to carry device-specific context.
  - **Thread safety**: All state machine operations are protected with `sync.RWMutex`. Read operations use `RLock()`, write operations use `Lock()`.
  - **Usage example**:
    ```go
    // Create factory and registry
    factory := iot.NewDeviceStateMachineFactory()
    registry := iot.NewDeviceStateMachineRegistry(factory)
    
    // Register device-type-specific transitions
    iot.RegisterDefaultDeviceTypeTransitions(factory)
    
    // Create state machine for a camera
    deviceSM, err := registry.GetOrCreateStateMachine(ctx, "camera-1", iot.DeviceTypeCamera)
    
    // Use adapter for camera-specific workflow states
    cameraAdapter := iot.NewCameraStateAdapter(deviceSM)
    cameraAdapter.TransitionToCameraWorkflowState(iot.CameraWorkflowStateModelDeployed, "")
    cameraAdapter.SetModelID("model-123")
    
    // State manager uses the device state machine interface
    // No need for camera-specific state machine implementation
    ```
  - **Integration with State Manager**: 
    - State manager imports `iot` package and uses `iot.DeviceStateMachine` interface
    - State manager uses `iot.DeviceStateMachineRegistry` to manage device state machines
    - For cameras, state manager uses `iot.CameraStateAdapter` to access camera-specific workflow states
    - State manager no longer needs its own `CameraStateMachine` implementation
    - Connection state machine remains separate (device-independent) in `state-mng` package
  - **Migration path**:
    - State manager can gradually migrate from `types.CameraStateMachine` to `iot.DeviceStateMachine`
    - `CameraStateAdapter` provides backward compatibility during migration
    - Existing camera state machine can be deprecated once migration is complete
  - **Benefits for State Manager**:
    - **Simpler code**: No need to maintain camera state machine implementation
    - **Extensible**: Easy to add new device types without modifying state manager
    - **Consistent**: All devices use the same state machine interface
    - **Testable**: Device state machines can be tested independently

---

## Epic 2: Event System Reliability

**Priority:** High  
**Goal:** Implement reliable event delivery with durable queue and retry semantics for critical events.

### Section 2.1: Event Bus Reliability

#### Subsection 2.1.1: Design Durable Event Queue
- **Description:** Design persistent event storage for ALL events (not just critical ones) for debugging and troubleshooting
- **Status:** ✅ COMPLETE
- **Scope:** 
  - Persist ALL events to persistent storage (not just critical ones) for debugging and troubleshooting
  - Design queue structure using persistent storage (bbolt or meta-storage)
  - Implement event querying functionality for debugging
  - Maintain in-memory subscriptions for real-time event delivery
- **Dependencies:** None
- **Related Findings:**
  - Finding #16: In-memory event bus drops events when buffers are full
  - Finding #106: Event bus channels can fill up and events are silently dropped
- **Implementation:**
  - **Two implementations provided:**
    1. **Bbolt Event Bus** (`edge/orchestrator/internal/event-bus/bboltebus/bbolt_event_bus.go`):
       - Implements `EventBus` interface with bbolt persistence
       - Uses dedicated bbolt database at `{DataDir}/db/event-bus.db`
       - Stores events in three buckets for efficient querying:
         - `events`: Main events bucket (keyed by event ID)
         - `events_by_type`: Events indexed by type (keyed by `<event_type>_<timestamp>_<event_id>`)
         - `events_by_time`: Events indexed by time (keyed by `<timestamp>_<event_id>`)
       - Provider: `"bbolt"` (requires `DataDir` in config)
    2. **Meta-Storage Event Bus** (`edge/orchestrator/internal/event-bus/metastoragebus/meta_storage_event_bus.go`):
       - Implements `EventBus` interface using meta-storage top interface
       - Uses existing meta-storage instance (shared with other services)
       - Stores events in meta-storage `events` bucket
       - Leverages meta-storage's existing filtering and querying capabilities
       - Provider: `"metastorage"` (requires meta-storage to be available)
  - **Common Features (both implementations):**
    - Provides `QueryEvents()` method for querying events by type, source, time range, and limit
    - Provides `GetEventCount()` method for getting total event count
    - Events are persisted asynchronously (non-blocking) to avoid impacting real-time delivery
    - Maintains in-memory subscriptions for real-time event delivery
    - All events are persisted (not just critical ones) for debugging and troubleshooting
  - **Interface Extensions:**
    - Extended `MetaDataStore` interface with event storage methods:
      - `SaveEvent()`, `GetEvent()`, `ListEvents()`, `DeleteEvent()`, `GetEventCount()`
    - Implemented in `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go`
  - **Configuration:**
    - Updated `EventBusConfig` to include `DataDir` field for bbolt storage path
    - Updated `NewEventBus()` factory to support both `"bbolt"` and `"metastorage"` providers
    - Meta-storage provider requires meta-storage to be created before event-bus (dependency order adjusted in orchestrator)
- **Code Locations:**
  - `edge/orchestrator/internal/event-bus/bboltebus/bbolt_event_bus.go` - Bbolt implementation
  - `edge/orchestrator/internal/event-bus/metastoragebus/meta_storage_event_bus.go` - Meta-storage implementation
  - `edge/orchestrator/internal/event-bus/event_bus.go` - Factory updated to support both providers
  - `edge/orchestrator/internal/event-bus/types/types.go` - Config updated with `DataDir` field
  - `edge/orchestrator/internal/meta-storage/meta-storage-iface.go` - Interface extended with event methods
  - `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go` - Event storage methods implemented
  - `edge/orchestrator/internal/orchestrator/orchestrator.go` - Dependency order adjusted for meta-storage provider

#### Subsection 2.1.2: Implement Event Persistence
- **Description:** Persist all events before delivery using Meta-Storage Event Bus as default
- **Status:** ✅ COMPLETE (Event persistence implemented; delivery tracking and replay are future enhancements)
- **Scope:** 
  - ✅ Store all events in meta-storage before publishing (implemented in 2.1.1)
  - ⬜ Mark events as delivered/acknowledged after successful processing (future enhancement)
  - ⬜ Implement event replay mechanism for failed deliveries (future enhancement - Subsection 2.1.3)
- **Dependencies:** 2.1.1
- **Implementation:**
  - **Meta-Storage Event Bus is now the default event bus** (configured in `config.dev.yaml`)
  - All events are automatically persisted to meta-storage before publishing
  - Events are stored asynchronously (non-blocking) to avoid impacting real-time delivery
  - Event persistence is transparent to subscribers - they receive events in real-time via in-memory channels
  - All events are queryable via `QueryEvents()` method for debugging and troubleshooting
  - **Configuration:**
    - Updated `config.dev.yaml` to use `provider: metastorage`
    - Meta-storage must be configured and available before event-bus starts
    - Event-bus automatically uses the existing meta-storage instance
- **Code Locations:**
  - `edge/orchestrator/internal/event-bus/metastoragebus/meta_storage_event_bus.go` - Main implementation
  - `edge/orchestrator/internal/meta-storage/meta-storage-iface.go` - Event storage interface
  - `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go` - Event storage implementation
  - `edge/config/config.dev.yaml` - Default configuration updated to use metastorage provider
- **Note:** Delivery acknowledgment and replay mechanisms are planned for future subsections (2.1.3 and beyond) as they require additional infrastructure for tracking event processing status.

#### Subsection 2.1.3: Implement Event Retry Logic ✅ COMPLETED
- **Description:** Add retry mechanism for failed event processing
- **Scope:** 
  - Retry failed event processing with exponential backoff
  - Set maximum retry attempts
  - Move to dead letter queue after max retries
- **Dependencies:** 2.1.2
- **Status:** ✅ **COMPLETED**
- **Implementation Details:**
  - Extended `EventBusConfig` with retry configuration:
    - `max_retries`: Maximum number of retry attempts (default: 3)
    - `initial_backoff`: Initial backoff duration (default: 1s)
    - `max_backoff`: Maximum backoff duration to cap exponential backoff (default: 60s)
    - `backoff_multiplier`: Multiplier for exponential backoff (default: 2.0)
    - `retry_interval`: Interval between retry worker runs (default: 10s)
  - Extended `MetaDataStore` interface with event processing status methods:
    - `UpdateEventProcessingStatus()`: Updates event status, retry count, error message, and next retry time
    - `GetFailedEvents()`: Retrieves failed events ready for retry (next_retry_time <= now)
    - `GetDeadLetterEvents()`: Retrieves events from dead letter queue
    - `MoveEventToDeadLetter()`: Moves an event to dead letter queue after max retries
  - Implemented event processing status tracking:
    - Events are marked as "pending" when published
    - Status transitions: pending → processing → succeeded/failed
    - Failed events are scheduled for retry with exponential backoff
    - Events exceeding max retries are moved to dead letter queue
  - Added retry worker to `MetaStorageEventBus`:
    - Background goroutine that periodically checks for failed events ready for retry
    - Re-publishes failed events after backoff period
    - Uses exponential backoff: `backoff = initial_backoff * (backoff_multiplier ^ retry_count)`
    - Backoff is capped at `max_backoff`
  - Implemented dead letter queue:
    - Separate bucket in meta-storage (`dead_letter_events`)
    - Events moved to DLQ after exceeding `max_retries`
    - DLQ events can be queried for manual inspection and recovery
  - Added helper methods to `MetaStorageEventBus`:
    - `MarkEventFailed()`: Called by event processors to mark events as failed
    - `MarkEventSucceeded()`: Called by event processors to mark events as succeeded
    - These methods should be called by event subscribers/processors to track processing status
- **Code Locations:**
  - `edge/orchestrator/internal/event-bus/types/types.go` - Retry configuration and status types
  - `edge/orchestrator/internal/event-bus/metastoragebus/meta_storage_event_bus.go` - Retry worker and status tracking
  - `edge/orchestrator/internal/meta-storage/meta-storage-iface.go` - Event processing status interface
  - `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go` - Event processing status implementation
  - `edge/config/config.dev.yaml` - Retry configuration defaults
- **Usage:**
  - Event processors should call `MarkEventFailed()` when processing fails
  - Event processors should call `MarkEventSucceeded()` when processing succeeds
  - Retry worker automatically re-publishes failed events after backoff period
  - Events exceeding max retries are automatically moved to dead letter queue
- **Configuration Example:**
  ```yaml
  event_bus:
    provider: metastorage
    buffer_size: 100
    max_retries: 3
    initial_backoff: 1s
    max_backoff: 60s
    backoff_multiplier: 2.0
    retry_interval: 10s
  ```
- **Note:** 
  - Retry logic is only enabled for `metastorage` event bus provider
  - If `max_retries` is 0 or not set, retry logic is disabled
  - Event processors must explicitly call `MarkEventFailed()` or `MarkEventSucceeded()` to track status
  - Dead letter queue events can be queried using `GetDeadLetterEvents()` for manual inspection

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
- **Note on MetaStorageEventBus:**
  - Backpressure is **less critical** for `metastorage` provider because:
    - Events are **persisted to meta-storage before delivery** (durable)
    - If subscriber channels are full, only **real-time delivery is affected** - events are not lost
    - The retry mechanism (2.1.3) can handle processing failures
    - Events can be queried from storage if needed
  - However, backpressure could still be useful for:
    - Preventing memory buildup in subscriber channels
    - Ensuring timely delivery for real-time subscribers
    - Providing feedback to publishers about system load
  - **Priority:** High for `inmemory` provider (events are lost), Medium/Low for `metastorage` provider (events are durable)

#### Subsection 2.1.5: Add Event Ordering Guarantees ✅ COMPLETED
- **Description:** Ensure critical events are processed in order
- **Scope:** 
  - Add sequence numbers to events
  - Process events in order for same event source
  - Handle out-of-order events gracefully
- **Dependencies:** 2.1.2
- **Priority:** Focus on `metastorage` provider first (default event bus)
- **Status:** ✅ **COMPLETED**
- **Related Findings:**
  - Finding #35: Workflow execution is concurrent-per-event, creating ordering hazards
  - Finding #38: Workflows can run out-of-order relative to state transitions
- **Implementation Details:**
  - Extended `Event` type with `SequenceNumber` field (int64, 0 if not set for backward compatibility)
  - Added `OrderingMode` type with three modes:
    - `none`: No ordering guarantees (default, backward compatible)
    - `best_effort`: Reorder events if possible, deliver out-of-order if needed
    - `strict`: Buffer and wait for missing sequences, timeout if sequence doesn't arrive
  - Extended `EventBusConfig` with ordering configuration:
    - `ordering_mode`: Ordering mode ("none", "best_effort", "strict")
    - `ordering_buffer_size`: Buffer size for out-of-order events (default: 100)
    - `ordering_timeout`: Timeout for waiting for missing sequences in strict mode (default: 30s)
  - Implemented per-source sequence number generation:
    - Each event source has its own sequence counter
    - Sequence numbers start at 1 per source
    - Allows parallel processing of different sources
  - Implemented `OrderingBuffer` per source:
    - Tracks expected sequence number per source
    - Buffers out-of-order events
    - Delivers events in sequence order
    - Handles timeouts in strict mode
    - Cleans up old buffered events to prevent memory growth
  - Ordering modes behavior:
    - **best_effort**: Buffers future events, delivers consecutive events when available, delivers past events immediately
    - **strict**: Buffers future events, waits for missing sequences up to timeout, then skips and continues
  - Sequence numbers are persisted in event metadata for debugging and replay
  - Events without sequence numbers (SequenceNumber == 0) are delivered immediately (backward compatible)
- **Code Locations:**
  - `edge/orchestrator/internal/event-bus/types/types.go` - Event type with SequenceNumber, OrderingMode, EventBusConfig
  - `edge/orchestrator/internal/event-bus/metastoragebus/meta_storage_event_bus.go` - Ordering implementation
    - `OrderingBuffer` type and methods
    - `publishWithOrdering()` - Applies ordering logic
    - `getNextSequenceNumber()` - Generates sequence numbers per source
    - `getOrCreateOrderingBuffer()` - Manages per-source buffers
  - `edge/orchestrator/internal/event-bus/event_bus.go` - Ordering config creation and passing
  - `edge/config/config.dev.yaml` - Ordering configuration defaults
- **Configuration Example:**
  ```yaml
  event_bus:
    provider: metastorage
    buffer_size: 100
    ordering_mode: best_effort  # "none", "best_effort", or "strict"
    ordering_buffer_size: 100   # Buffer size for out-of-order events
    ordering_timeout: 30s        # Timeout for strict mode
  ```
- **Usage:**
  - Events are automatically assigned sequence numbers when ordering is enabled
  - Sequence numbers are per-source, allowing parallel processing of different sources
  - Events from the same source are delivered in sequence order
  - Out-of-order events are buffered and reordered when possible
  - In strict mode, missing sequences cause a timeout before skipping
- **Benefits:**
  - Prevents ordering hazards in workflow execution
  - Ensures state transitions happen in correct order per source
  - Allows parallel processing of different sources
  - Backward compatible (ordering disabled by default)
  - Configurable ordering guarantees based on requirements
- **Note:** 
  - Since `metastorage` is the default event bus, this implementation is prioritized
  - Other providers (inmemory, bbolt) can be updated later if needed
  - Sequence numbers are per-source to allow parallel processing of different sources
  - Events without sequence numbers (legacy or when ordering disabled) are delivered immediately

### Section 2.2: Workflow Execution Improvements

#### Subsection 2.2.1: Serialize Workflow Execution ✅ COMPLETED
- **Description:** Execute workflows sequentially or with proper ordering
- **Scope:** 
  - Remove concurrent-per-event workflow execution
  - Execute workflows in event order
  - Add workflow queue if needed
- **Dependencies:** 2.1.5 (Event Ordering Guarantees)
- **Status:** ✅ **COMPLETED**
- **Related Findings:**
  - Finding #35: Workflow execution is concurrent-per-event
  - Finding #38: Workflows can run out-of-order
- **Implementation Details:**
  - Leverages event ordering guarantees from Subsection 2.1.5 (metastorage event bus)
  - Implements per-source workflow queues for serialized execution
  - Workflows execute sequentially per event source, respecting event sequence numbers
  - Events without sequence numbers (SequenceNumber == 0) execute concurrently (backward compatible)
  - Per-source workflow queue workers ensure sequential execution per source
  - Workflow queue buffer size: 100 tasks per source
  - Global workflow semaphore still limits total concurrent workflows across all sources
  - Configuration option `serialize_workflows` (default: true) to enable/disable serialization
- **Architecture:**
  - `WorkflowTask` type holds event and state information for queued workflows
  - Per-source workflow queues (`workflowQueues map[string]chan *WorkflowTask`)
  - `workflowQueueWorker` processes workflows sequentially per source
  - `handleEvent` queues workflows when `serializeWorkflows` is enabled and event has sequence number
  - Falls back to concurrent execution if queue is full (with warning)
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`:
    - `WorkflowTask` type (line ~232)
    - `workflowQueues` map and `serializeWorkflows` flag (line ~141-142)
    - `handleEvent()` - queues workflows when serialization enabled (line ~700)
    - `queueWorkflow()` - queues workflow task per source (line ~720)
    - `getOrCreateWorkflowQueue()` - creates per-source queue and worker (line ~750)
    - `workflowQueueWorker()` - processes workflows sequentially per source (line ~780)
  - `edge/orchestrator/internal/state-mng/types/config.go`:
    - `SerializeWorkflows` configuration field (line ~48)
- **Configuration:**
  ```yaml
  state_manager:
    serialize_workflows: true  # Default: true (enabled)
  ```
- **Behavior:**
  - **When `serialize_workflows: true` (default):**
    - Events with sequence numbers (SequenceNumber > 0) are queued per source
    - Workflows execute sequentially per source, respecting event ordering
    - Events without sequence numbers execute concurrently (backward compatible)
  - **When `serialize_workflows: false`:**
    - All workflows execute concurrently (original behavior)
    - Workflow semaphore still limits total concurrency
- **Benefits:**
  - Prevents ordering hazards in workflow execution (Finding #35)
  - Ensures workflows run in event order per source (Finding #38)
  - Leverages event bus ordering guarantees (Subsection 2.1.5)
  - Allows parallel processing of different sources
  - Backward compatible (events without sequence numbers execute concurrently)
  - Configurable serialization mode
- **Integration with Event Ordering:**
  - Works seamlessly with `metastorage` event bus ordering (Subsection 2.1.5)
  - Events arrive in order per source (due to event bus ordering)
  - Workflows execute in the same order (due to per-source queues)
  - Sequence numbers ensure correct ordering even if events arrive out-of-order
- **Note:**
  - Since `metastorage` is the default event bus with ordering enabled, workflows automatically benefit from serialized execution
  - Per-source queues allow parallel processing of different sources while maintaining ordering within each source
  - Queue buffer size (100) prevents blocking event processing while allowing some buffering

#### Subsection 2.2.2: Add Workflow Concurrency Control ✅ COMPLETED
- **Description:** Limit concurrent workflow execution to prevent resource exhaustion
- **Scope:** 
  - Implement worker pool for workflow execution
  - Set maximum concurrent workflows
  - Queue workflows when at capacity
- **Dependencies:** 2.2.1 (Serialize Workflow Execution)
- **Status:** ✅ **COMPLETED**
- **Related Findings:**
  - Finding #93: executeWorkflow spawns unbounded goroutines
- **Implementation Details:**
  - Replaced semaphore-based concurrency control with worker pool pattern
  - Implemented global workflow queue (`workflowPoolQueue`) with buffer size 1000
  - Created fixed-size worker pool with configurable number of workers (default: 10)
  - Workers pull tasks from global queue and execute workflows sequentially
  - Prevents unbounded goroutine creation (Finding #93)
  - Queues workflows when at capacity (non-blocking, drops with warning if queue full)
  - Integrates with per-source serialization (Subsection 2.2.1):
    - Per-source queues feed into global worker pool
    - Worker pool limits total concurrent workflows across all sources
    - Per-source ordering is maintained while respecting global concurrency limit
- **Architecture:**
  - **Global Worker Pool:**
    - `workflowPoolQueue`: Global queue for all workflow tasks (buffer: 1000)
    - `workflowPoolWorkers`: Number of worker goroutines (configurable, default: 10)
    - `workflowPoolWorker()`: Worker function that processes tasks from global queue
    - Workers execute workflows sequentially (one at a time per worker)
  - **Integration with Per-Source Serialization:**
    - Per-source queues (`workflowQueues`) feed into global worker pool
    - Per-source workers pull from source-specific queues and enqueue to global pool
    - Global worker pool limits total concurrency while maintaining per-source ordering
  - **Workflow Execution Flow:**
    1. Event arrives → `handleEvent()` creates `WorkflowTask`
    2. If serialization enabled and event has sequence number:
       - Task queued to per-source queue
       - Per-source worker pulls from source queue
       - Per-source worker enqueues to global worker pool
    3. If serialization disabled or no sequence number:
       - Task directly enqueued to global worker pool
    4. Global worker pool worker pulls task and executes workflow
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`:
    - Worker pool fields (line ~137-142):
      - `workflowPoolQueue`: Global queue
      - `workflowPoolWorkers`: Worker count
      - `workflowPoolWg`: WaitGroup for workers
      - `workflowPoolStarted`: Start flag
      - `workflowPoolMu`: Mutex for pool state
    - `startWorkflowPool()`: Starts worker pool (line ~838)
    - `stopWorkflowPool()`: Stops worker pool (line ~860)
    - `workflowPoolWorker()`: Worker function (line ~890)
    - `enqueueWorkflowToPool()`: Enqueues task to global pool (line ~730)
    - `handleEvent()`: Uses worker pool for non-serialized workflows (line ~714)
    - `workflowQueueWorker()`: Enqueues to global pool (line ~833)
  - `edge/orchestrator/internal/state-mng/types/config.go`:
    - `MaxConcurrentWorkflows`: Worker pool size configuration
- **Configuration:**
  ```yaml
  state_manager:
    max_concurrent_workflows: 10  # Number of worker pool workers (default: 10)
  ```
- **Benefits:**
  - Prevents unbounded goroutine creation (Finding #93)
  - Limits resource usage with fixed worker pool
  - Queues workflows when at capacity
  - Works seamlessly with per-source serialization
  - Maintains global concurrency limit while respecting per-source ordering
  - Non-blocking enqueue (drops with warning if queue full)
- **Behavior:**
  - **Worker Pool:**
    - Fixed number of workers (configurable via `max_concurrent_workflows`)
    - Workers pull from global queue and execute workflows
    - Queue buffer: 1000 tasks
    - If queue full, tasks are dropped with warning (non-blocking)
  - **Integration with Serialization:**
    - Per-source queues maintain ordering per source
    - Global worker pool limits total concurrency
    - Both mechanisms work together: ordering + concurrency control
  - **Backward Compatibility:**
    - Semaphore kept for backward compatibility (deprecated)
    - Events without sequence numbers use worker pool directly
    - Configuration still works with existing `max_concurrent_workflows` setting
- **Shutdown:**
  - Worker pool stopped before per-source queues
  - Graceful shutdown with 30-second timeout
  - All workers finish processing before shutdown completes
- **Note:**
  - Worker pool size cannot be changed dynamically (requires restart)
  - Queue size (1000) is fixed but can be adjusted in code if needed
  - Worker pool works with both serialized and non-serialized workflows

#### Subsection 2.2.3: Make Workflows Idempotent ✅ COMPLETED
- **Description:** Ensure all workflows are idempotent to handle duplicate execution
- **Scope:** 
  - Review all workflow implementations
  - Add idempotency checks where needed
  - Test workflows with duplicate events
- **Dependencies:** None
- **Status:** ✅ **COMPLETED**
- **Implementation Details:**
  - Implemented duplicate event detection mechanism using event keys
  - Added idempotency checks to all critical workflows
  - Event deduplication window: 1 hour (configurable)
  - Automatic cleanup of old processed events (every 1000 events)
  - Per-workflow idempotency checks for operations with side effects
- **Event Deduplication:**
  - **Event Key Generation:**
    - Format: `event_type:source:sequence_number:camera_id:model_id:event_id`
    - Includes event type, source, sequence number, and relevant data fields
    - Unique per event instance
  - **Deduplication Window:**
    - Default: 1 hour
    - Events processed within window are considered duplicates
    - Events outside window are treated as new (allows retry after failures)
  - **Cleanup:**
    - Automatic cleanup when processed events map exceeds 1000 entries
    - Removes events outside deduplication window
    - Prevents unbounded memory growth
- **Workflow Idempotency Checks:**
  1. **`handleSnapshotRequested`:**
     - Checks if pending snapshot request already exists for camera
     - Skips duplicate requests (idempotent)
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1524-1558`
  2. **`syncScreenshotsToVM`:**
     - Tracks last sync timestamp per camera
     - Skips sync if synced within last 5 minutes (idempotent)
     - Updates sync timestamp after successful sync
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1621-1815`
  3. **`handleModelDeployed`:**
     - Checks if model is already deployed for camera
     - Compares model ID and camera state
     - Skips if model already deployed (idempotent)
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:2031-2110`
  4. **`initializeServicesAfterAuth`:**
     - Tracks whether services have been initialized
     - Skips if already initialized (idempotent)
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1397-1425`
  5. **`executeModelDeployedWorkflowForCamera`:**
     - Uses `startFrameProcessingForCamera` which has built-in idempotency
     - Double-check locking prevents duplicate goroutines
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:2147-2231`
  6. **`executeFrameProcessingWorkflowForCamera`:**
     - Uses `startFrameProcessingForCamera` which has built-in idempotency
     - Checks if frame processing already active before starting
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:2233-2310`
  7. **`handleCapabilitiesReceived`:**
     - Camera discovery is idempotent (won't discover same cameras twice)
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1427-1476`
  8. **`handleScreenshotSetReady`:**
     - Relies on `syncScreenshotsToVM` idempotency
     - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1576-1619`
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`:
    - Event deduplication fields (line ~145-150):
      - `processedEvents`: Map of processed event keys to timestamps
      - `processedEventsMu`: Mutex for processed events map
      - `eventDedupWindow`: Deduplication time window (default: 1 hour)
    - Workflow idempotency tracking (line ~150-155):
      - `lastScreenshotSync`: Map of camera ID to last sync timestamp
      - `screenshotSyncMu`: Mutex for screenshot sync tracking
      - `servicesInitialized`: Flag for services initialization
      - `servicesInitMu`: Mutex for services initialized flag
    - `isDuplicateEvent()`: Checks if event is duplicate (line ~960)
    - `markEventProcessed()`: Marks event as processed (line ~978)
    - `generateEventKey()`: Generates unique event key (line ~996)
    - `cleanupOldProcessedEvents()`: Cleans up old events (line ~1025)
    - `handleEvent()`: Adds duplicate detection (line ~734)
    - All workflow functions with idempotency checks (see list above)
- **Event Key Generation:**
  ```go
  // Format: event_type:source:sequence_number:camera_id:model_id:event_id
  key := fmt.Sprintf("%s:%s:%d", ev.Type, ev.Source, ev.SequenceNumber)
  if cameraID, ok := ev.Data["camera_id"].(string); ok && cameraID != "" {
      key += ":" + cameraID
  }
  if modelID, ok := ev.Data["model_id"].(string); ok && modelID != "" {
      key += ":" + modelID
  }
  if eventID, ok := ev.Data["event_id"].(string); ok && eventID != "" {
      key += ":" + eventID
  }
  ```
- **Idempotency Guarantees:**
  - **Duplicate Events:** Detected and skipped within deduplication window
  - **Snapshot Requests:** Only one pending request per camera (checked before creation)
  - **Screenshot Sync:** No sync within 5 minutes of last sync
  - **Model Deployment:** Only one model deployment per camera (checked before processing)
  - **Service Initialization:** Only initialized once after authentication
  - **Frame Processing:** Only one frame processing goroutine per camera (double-check locking)
  - **State Transitions:** State machine transitions are idempotent (same transition twice = no-op)
- **Benefits:**
  - Prevents duplicate workflow execution from duplicate events
  - Handles event replay scenarios gracefully
  - Prevents resource waste from redundant operations
  - Improves system reliability and predictability
  - Reduces unnecessary network traffic and storage operations
  - Handles out-of-order events correctly
- **Behavior:**
  - **Event Deduplication:**
    - Events processed within 1 hour are considered duplicates
    - Duplicate events are logged and skipped
    - Event keys include all relevant identifiers for uniqueness
  - **Workflow Idempotency:**
    - Each workflow checks its own idempotency conditions
    - State-based checks (e.g., camera state, model ID)
    - Time-based checks (e.g., last sync timestamp)
    - Flag-based checks (e.g., services initialized)
  - **Cleanup:**
    - Old processed events cleaned up automatically
    - Prevents unbounded memory growth
    - Cleanup triggered when map exceeds 1000 entries
- **Testing Recommendations:**
  - Test duplicate event handling (same event processed twice)
  - Test event replay scenarios (events outside deduplication window)
  - Test concurrent duplicate events (race conditions)
  - Test workflow idempotency (same workflow executed multiple times)
  - Test state-based idempotency (workflows with state checks)
  - Test time-based idempotency (workflows with time windows)
- **Note:**
  - Event deduplication window (1 hour) is configurable but not exposed in config yet
  - Screenshot sync window (5 minutes) is hardcoded but could be made configurable
  - All idempotency checks are thread-safe using mutexes
  - State machine transitions are inherently idempotent (same transition twice = no-op)

#### Subsection 2.2.4: Handle Missing Event Consumers ✅ COMPLETED
- **Description:** Add consumers for published but unhandled events
- **Scope:** 
  - Add handler for `model.deployment.status` events
  - Review all published events for consumers
  - Add observability for unhandled events
- **Dependencies:** None
- **Status:** ✅ **COMPLETED**
- **Related Findings:**
  - Finding #41: model.deployment.status events published but no consumer
- **Implementation Details:**
  - Added handler for `model.deployment.status` events
  - Added observability for unhandled events (default case in switch statement)
  - Reviewed all published events and documented consumer status
  - Handler reports deployment status to VM via `vmGateway.ReportDeploymentStatus()`
- **Event Handler Added:**
  1. **`model.deployment.status`:**
     - **Purpose:** Reports model deployment status to VM
     - **Published by:** `vm-gateway/http-impl/https-server-service` (line 746)
     - **Handler:** `handleModelDeploymentStatus()` (line ~2280)
     - **Functionality:**
       - Extracts `deployment_id`, `status`, `model_path`, `model_id` from event
       - Validates required fields (`deployment_id`, `status`)
       - Checks if HTTPS is connected (required for reporting)
       - Prepares error message for failed deployments
       - Calls `vmGateway.ReportDeploymentStatus()` with 10-second timeout
       - Logs success/failure for observability
     - **Code:** `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:2280-2350`
- **Observability for Unhandled Events:**
  - Added `default` case in `executeWorkflow()` switch statement
  - Logs warning for unhandled event types with full event context:
    - Event type
    - Source
    - Sequence number
    - Timestamp
  - Helps identify missing consumers during development and operations
  - Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1371-1378`
- **Published Events Review:**
  - **Events with Consumers:**
    - `network.wireguard.connected` → Handled
    - `network.wireguard.disconnected` → Handled
    - `network.https.connected` → Handled
    - `network.https.disconnected` → Handled
    - `edge.authenticated` → Handled
    - `edge.capabilities_received` → Handled
    - `camera.discovered` → Handled
    - `camera.registered` → Handled
    - `camera.connected` → Handled
    - `camera.disconnected` → Handled
    - `snapshot.requested` → Handled
    - `screenshot_set.ready` → Handled
    - `screenshot.saved` → Handled
    - `model.deployed` → Handled
    - `model.deployment.status` → **NOW HANDLED** ✅
    - `video.frame_received` → Handled (no-op, handled by AI gateway)
    - `video.clip_recorded` → Handled
    - `ai.detection` → Handled
    - `ai.inference` → Handled
    - `storage.full` → Handled
    - `storage.warning` → Handled
  - **Events without Consumers (Intentionally):**
    - `screenshot.updated` → No handler (metadata update, no state change needed)
    - `screenshot.deleted` → No handler (cleanup operation, no state change needed)
    - `camera.updated` → No handler (configuration update, no state change needed)
    - `camera.deleted` → No handler (cleanup operation, handled by deletion workflow)
    - `camera.discovery.requested` → No handler (triggers discovery, not state change)
    - `camera.capture_frame` → No handler (command event, handled by CCTV service)
    - `security.event.created` → No handler (stored in meta-storage, no state change)
    - `camera.frame.received` → No handler (handled by AI gateway directly)
    - `workflow.camera.discover` → No handler (internal workflow event, triggers discovery)
    - `workflow.ai.start_processing` → No handler (internal workflow event, triggers AI processing)
  - **Note:** Events without consumers are either:
    - Internal workflow events (not meant for state manager)
    - Metadata/cleanup events (no state machine impact)
    - Command events (handled by specific services)
    - Events handled by other services (AI gateway, CCTV service)
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`:
    - `EventTypeModelDeploymentStatus` constant (line ~60)
    - `handleModelDeploymentStatus()` handler (line ~2280)
    - Default case for unhandled events (line ~1371)
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go`:
    - Event publisher (line ~746)
- **Handler Implementation:**
  ```go
  func (m *StateManagerImpl) handleModelDeploymentStatus(ctx context.Context, ev eventbustypes.Event) {
      // Extract event data
      deploymentID, _ := ev.Data["deployment_id"].(string)
      status, _ := ev.Data["status"].(string)
      modelPath, _ := ev.Data["model_path"].(string)
      modelID, _ := ev.Data["model_id"].(string)
      
      // Validate required fields
      if deploymentID == "" || status == "" {
          return
      }
      
      // Check HTTPS connection (required for reporting)
      if !m.vmGateway.IsHTTPConnected() {
          return
      }
      
      // Prepare error message for failed deployments
      var errorMsg *string
      if status == "failed" || status == "error" {
          // Extract error message
      }
      
      // Report to VM with timeout
      callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
      defer cancel()
      m.vmGateway.ReportDeploymentStatus(callCtx, deploymentID, status, errorMsg, modelPathPtr)
  }
  ```
- **Observability Implementation:**
  ```go
  default:
      // Unhandled event - log for observability
      m.logger.Warn("Unhandled event type in workflow execution",
          zap.String("event_type", string(ev.Type)),
          zap.String("source", ev.Source),
          zap.Int64("sequence_number", ev.SequenceNumber),
          zap.Time("timestamp", ev.Timestamp),
      )
  ```
- **Benefits:**
  - All published events now have consumers or documented rationale
  - Deployment status reporting works correctly
  - Unhandled events are logged for observability
  - Easier to identify missing consumers during development
  - Better operational visibility into event processing
- **Behavior:**
  - **Model Deployment Status:**
    - Event received → Extract data → Validate → Check HTTPS connection → Report to VM
    - If HTTPS not connected, event is silently skipped (expected during disconnection)
    - If reporting fails, error is logged
    - Success is logged for observability
  - **Unhandled Events:**
    - Logged as warnings with full event context
    - Helps identify missing consumers
    - Does not block event processing
    - Can be monitored for operational insights
- **Testing Recommendations:**
  - Test `model.deployment.status` event handling
  - Test deployment status reporting to VM
  - Test handling when HTTPS is not connected
  - Test error handling for failed status reporting
  - Test unhandled event logging
  - Review logs for unhandled events in production
- **Note:**
  - Some events intentionally don't have handlers (internal workflow events, metadata updates)
  - Unhandled event warnings help identify missing consumers during development
  - Deployment status reporting requires HTTPS connection (expected behavior)
  - Timeout for status reporting: 10 seconds (configurable in code)

### Section 2.3: Event Persistence for Audit and Debugging

**Architecture Principles:**
- **Edge is NOT isolated:** Edge is always connected to VM (except temporary disconnections)
- **State Recovery:** Edge recovers from **meta-storage** and **object-storage** on restart, NOT from events
- **VM-Assisted Recovery:** If storage is lost, VM can help recover (models, configurations, etc.)
- **Events Purpose:** Events are for **audit logging** and **debugging/troubleshooting**, NOT for state recovery
- **Event Persistence:** Events are already persisted in meta-storage event bus (Subsection 2.1.2)
- **Audit Logging:** Use existing `audit-log` service for security-sensitive operations

**State Recovery Architecture:**
- **Primary Recovery Source:** Meta-storage (`GetCurrentEdgeState`, camera states, pending requests)
- **Secondary Recovery Source:** Object-storage (screenshots, clips, models)
- **VM-Assisted Recovery:** If storage is lost, VM provides:
  - Trained models (can be re-downloaded)
  - Configuration (can be re-synced)
  - Camera metadata (can be re-discovered)
- **Event Persistence:** Events are persisted for debugging/troubleshooting, not for state reconstruction

#### Subsection 2.3.1: Integrate Audit Log Service with Event Bus ✅ COMPLETED
- **Description:** Use existing audit-log service for security-sensitive event logging
- **Scope:** 
  - Integrate audit-log service with event bus
  - Log security-sensitive events to audit-log service
  - Use audit-log for compliance and security requirements
  - Keep event bus for debugging/troubleshooting
- **Dependencies:** None (audit-log service already exists)
- **Status:** ✅ **COMPLETED** (audit-log service exists and can be integrated)
- **Implementation Notes:**
  - Audit-log service already exists: `edge/orchestrator/internal/audit-log`
  - Provides tamper-proof audit logging with chain of hashes
  - Stores logs in object-storage and syncs to VM
  - Supports querying and export
  - Can be integrated with event bus to log security-sensitive events
- **Code Locations:**
  - `edge/orchestrator/internal/audit-log/audit-log-iface.go`: Audit log service interface
  - `edge/orchestrator/internal/audit-log/impl/audit-log-impl.go`: Implementation
  - `edge/orchestrator/internal/audit-log/types/types.go`: Types and entry definitions

#### Subsection 2.3.2: Enhance Event Persistence for Debugging
- **Description:** Enhance event persistence in meta-storage for debugging and troubleshooting
- **Scope:** 
  - Events are already persisted in meta-storage event bus (Subsection 2.1.2)
  - Add event query capabilities for debugging
  - Support event filtering by type, source, time range
  - Add event correlation for troubleshooting
  - Export events for analysis
- **Dependencies:** 2.1.2 (event persistence in meta-storage)
- **Status:** ⏸️ **DEFERRED** (basic event persistence already exists, enhancement can be done later)
- **Current State:**
  - Events are persisted in meta-storage via `SaveEvent()` (Subsection 2.1.2)
  - Events can be queried via `ListEvents()` and `GetEvent()`
  - Event bus provides `QueryEvents()` method
  - Basic persistence and query capabilities exist
- **Future Enhancements:**
  - Advanced filtering (by type, source, camera_id, etc.)
  - Event correlation and tracing
  - Event export for analysis
  - Event retention policies

#### Subsection 2.3.3: Document State Recovery Architecture
- **Description:** Document how Edge recovers state on restart
- **Scope:** 
  - Document state recovery from meta-storage
  - Document state recovery from object-storage
  - Document VM-assisted recovery scenarios
  - Clarify that events are NOT used for state recovery
- **Dependencies:** None
- **Status:** ✅ **COMPLETED** (documented below)
- **State Recovery Flow:**
  1. **On Edge Restart:**
     - Restore connection state from meta-storage (`GetCurrentEdgeState`)
     - Restore camera states from meta-storage
     - Restore pending snapshot requests from meta-storage
     - Restore pending model deployments from meta-storage
     - Recover active workflows (frame processing, etc.)
  2. **If Meta-Storage is OK:**
     - Edge can fully recover without VM
     - All state machines restored
     - Active workflows resumed
     - Frame processing can continue
  3. **If Storage is Lost:**
     - Edge connects to VM
     - VM provides:
       - Trained models (re-download)
       - Configuration (re-sync)
       - Camera metadata (re-discover)
     - Edge rebuilds state from VM data
  4. **Events Role:**
     - Events are persisted for debugging/troubleshooting
     - Events are NOT used for state recovery
     - Events help understand what happened (audit trail)
     - Events help diagnose issues (debugging)
- **Code Locations:**
  - `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`:
    - `restoreStateFromStorage()`: Restores state from meta-storage (line ~3297)
    - `recoverActiveWorkflows()`: Recovers active workflows (line ~3454)
    - `Start()`: Calls state restoration on startup (line ~599)

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
- **Status:** ✅ DONE
- **Description:** Implement comprehensive audit logging for all security-sensitive operations
- **Scope:** 
  - Log all device data access (reads, writes, deletions)
  - Log model deployments and configuration changes
  - Log authentication and authorization decisions
  - Implement tamper-proof audit log storage (append-only, cryptographic hashing)
- **Dependencies:** Epic 2, Section 2.3 (event persistence for audit and debugging)
- **Rationale:** Critical for security domain compliance and forensic investigation
- **Code Locations:**
  - `edge/orchestrator/internal/audit-log/` (audit log service)
  - `edge/orchestrator/internal/audit-log/types/types.go` (audit log entry types)
  - `edge/orchestrator/internal/audit-log/impl/audit-log-impl.go` (implementation)
  - `edge/orchestrator/config/config.go` (configuration)
- **Refactoring Details:**
  - Created comprehensive audit log service with tamper-proof storage using cryptographic hashing
  - Implemented chain of hashes: each entry includes hash of previous entry for integrity verification
  - Audit log entries stored temporarily in edge object storage (configurable retention, default 7 days)
  - Storage structure: `audit-logs/YYYY-MM-DD/entry-id.json` for organized date-based storage
  - Entry types implemented:
    - `DataAccessEntry`: Logs data access operations (reads, writes, deletions)
    - `AuthenticationEntry`: Logs authentication attempts with method and identity
    - `AuthorizationEntry`: Logs authorization decisions (granted/denied)
    - `ConfigurationChangeEntry`: Logs configuration changes with old/new values
    - `ModelDeploymentEntry`: Logs model deployment operations
    - `SecurityEventEntry`: Logs security-related events with severity levels
  - Each entry includes: ID, type, timestamp, edge ID, user ID, IP address, user agent, result, error, previous hash, and hash
  - Cryptographic hashing: SHA-256 hash of entry content + previous hash for chain integrity
  - Configuration added to main config: `AuditLogConfig` with `RetentionDays` (default: 7), `SyncInterval` (default: 1 hour), and `Enabled` flag
  - Periodic sync to VM: Background goroutine syncs audit logs to VM at configured interval (placeholder for VM sync implementation)
  - Periodic cleanup: Background goroutine removes old audit logs based on retention period (placeholder for cleanup implementation)
  - Service lifecycle: Proper start/stop with fx lifecycle management
  - Provider function: `AuditLogProvider` for dependency injection
  - **Note:** VM sync and cleanup implementations are placeholders and need to be completed:
    - `SyncToVM()`: Needs to be implemented to sync audit logs to VM via VMGateway
    - `CleanupOldLogs()`: Needs to be implemented to delete old audit log entries from object storage
  - Integration points prepared for:
    - Web gateway authentication middleware (to log auth attempts)
    - Web gateway handlers (to log data access operations)
    - Configuration changes (to log config modifications)
    - Model deployments (to log deployment operations)

#### Subsection 3.5.2: Add Audit Log Integration
- **Status:** ✅ DONE
- **Description:** Support external SIEM integration and audit log export
- **Scope:** 
  - Support standard audit log formats (CEF, JSON)
  - Enable real-time audit log streaming to external systems
  - Support audit log query API for compliance reporting
  - Add audit log retention policies
- **Dependencies:** 3.5.1
- **Code Locations:**
  - `edge/orchestrator/internal/audit-log/impl/export.go` (CEF/JSON export formats)
  - `edge/orchestrator/internal/audit-log/impl/query.go` (query functionality)
  - `edge/orchestrator/internal/audit-log/types/types.go` (ExportFormat, ExportEntry types)
  - `edge/orchestrator/internal/vm-gateway/vm_gateway.go` (SyncAuditLogs method)
  - `edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types/types.go` (sync request/response types)
- **Refactoring Details:**
  - **Export Formats Implementation:**
    - Implemented CEF (Common Event Format) export for SIEM integration
    - CEF format includes: version, vendor, product, signature ID, name, severity, and extension fields
    - Extension fields include: action, source, destination user, source IP, outcome, timestamps, edge ID, and hash fields for tamper-proofing
    - Implemented JSON export format (default)
    - Export formats are configurable per entry via `ExportFormat` type (JSON or CEF)
    - `ConvertToExportEntry` function converts audit log entries to export-ready format
    - `BatchExportEntries` function groups entries into batches for efficient transfer
  - **VM Gateway Integration:**
    - Added `SyncAuditLogs` method to `VMGateway` interface for syncing audit logs to VM
    - Implemented `SyncAuditLogs` in `HTTPSClient` and `VmGatewayHttpImpl`
    - Added `SyncAuditLogsRequest` and `SyncAuditLogsResponse` types for VM communication
    - Added `AuditLogEntry` type for VM transfer format (includes JSON and optional CEF representation)
    - Sync request includes: edge ID, time range (Unix timestamps), entry count, entries array, and format preference
    - Sync response includes: success status, error message, and synced count
    - HTTP endpoint: `POST /api/v1/audit-logs/sync` on VM
    - Audit log service uses proper fx dependency injection with `VMGateway` interface (not `interface{}`)
    - VM gateway is injected via `AuditLogProvider` constructor parameter
  - **Query API Implementation:**
    - Added `QueryAuditLogs` method to `AuditLogService` interface
    - Added `GetAuditLogEntry` method to retrieve specific entries by ID
    - Implemented `QueryFilters` type with support for:
      - Time range filtering (StartTime, EndTime)
      - Entry type filtering
      - User ID, IP address, result filtering
      - Resource type and ID filtering
      - Pagination (Limit, Offset)
    - `QueryAuditLogsFromStorage` function queries audit logs from object storage
    - `GetAuditLogEntryFromStorage` function retrieves specific entries by ID and timestamp
    - `matchesFilters` function applies filter criteria to entries
  - **VM Sync Implementation:**
    - `SyncToVM` method queries audit logs since last sync and sends them to VM
    - Syncs are batched (100 entries per batch) for efficient transfer
    - Tracks last sync time to avoid duplicate transfers
    - Supports both JSON and CEF formats in sync payload
    - Sync runs periodically based on `SyncInterval` configuration (default: 1 hour)
  - **Retention Policies:**
    - Retention period is configurable via `RetentionDays` (default: 7 days)
    - `CleanupOldLogs` method removes audit logs older than retention period
    - Cleanup runs periodically (daily) via background goroutine
    - Note: Full cleanup implementation requires object storage list operation (placeholder for now)
  - **Event-Based Coordination (Pending):**
    - Audit log sync coordination through state-mng via events is planned but not yet implemented
    - State manager should trigger sync events when VM connection is established
    - Sync can be triggered manually via `SyncToVM()` or automatically based on periodic timer (default: 1 hour)
    - Future enhancement: Add event bus integration to trigger sync on VM connection events
    - State manager can subscribe to VM connection events and trigger audit log sync when connection is established
  - **Code Organization:**
    - Export functionality moved to `impl/export.go` (implementation details)
    - Query functionality moved to `impl/query.go` (implementation details)
    - Types (`ExportFormat`, `ExportEntry`, `QueryFilters`) moved to `types/types.go`
    - Clean separation between interface, implementation, and types
  - **Dependency Injection:**
    - Audit log service uses proper fx dependency injection with `VMGateway` interface
    - `VMGateway` is injected via `AuditLogProvider` constructor parameter
    - No circular dependency: audit-log depends on vm-gateway, but vm-gateway doesn't depend on audit-log
    - Removed `SetVMGateway` method and `interface{}` workaround in favor of proper constructor injection
  - **Implementation Notes:**
    - Full query implementation requires object storage list operation (currently placeholder)
    - Cleanup implementation requires object storage list operation (currently placeholder)
    - CEF format severity mapping: success=3 (low), failure=6 (medium), denied=8 (high)
    - CEF extension fields use standard ArcSight CEF field names for SIEM compatibility
    - Export entries support both JSON and CEF formats simultaneously for flexibility
    - Sync uses proper type-safe `SyncAuditLogsRequest` and `SyncAuditLogsResponse` types
    - HTTP endpoint: `POST /api/v1/audit-logs/sync` on VM for receiving audit logs

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
- **Dependencies:** Epic 2, Section 2.3 (event persistence for audit and debugging), Epic 3, Section 3.5 (audit logging)

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
- **Dependencies:** Epic 2, Section 2.3 (event persistence for audit and debugging), 10.4.1

#### Subsection 10.4.3: Implement Device Activity Replay
- **Description:** Enable replay of device activities for investigation
- **Scope:** 
  - Replay device state transitions from event log (for debugging/forensics)
  - Replay device data processing activities (for debugging/forensics)
  - Support time-based replay (replay activities at specific time)
  - Support selective replay (replay specific device or operation type)
  - **Note:** Event replay is for debugging/forensics, NOT for state recovery (state is recovered from meta-storage)
- **Dependencies:** 10.4.2, Epic 2, Section 2.3 (event persistence for audit and debugging)

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
9. Epic 2, Section 2.3: Event Persistence for Audit and Debugging (High Reliability + Security)
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
- Epic 3 (Security) is independent and can be done in parallel, but Section 3.5 (Audit Logging) benefits from Epic 2, Section 2.3 (Event Persistence for Audit and Debugging)
- Epic 3, Section 3.9 (Device Identity/Provisioning/Attestation) depends on Epic 12 (device abstraction) and is a prerequisite for robust multi-IoT onboarding
- Epic 3, Section 3.10 (Secure Updates/Supply Chain/Secrets) is largely independent, but secrets management benefits from encryption + key management (Section 3.7)
- Epic 3, Section 3.11 (Plugin Sandboxing/Least Privilege) depends on Epic 12 (plugins) and ties into Epic 4, Section 4.4 (quotas) and Epic 10, Section 10.3 (monitoring)
- Epic 6 (Data Integrity) is independent and can be done in parallel
- Epic 7 (Performance) depends on Epic 6 (pagination, thumbnails)
- Epic 8 (Frame Lifecycle) depends on Epic 1 (state machine clarity), and Section 8.3 (Compliance) benefits from Epic 2, Section 2.3 (event persistence for audit and debugging) and Epic 3, Section 3.5 (audit logging)
- Epic 9, Section 9.3 (Multi-Tenancy) depends on Epic 12 (Device Abstraction)
- Epic 10 (Testing and Observability) depends on all other epics for comprehensive coverage, but core observability can start early
- Epic 10, Section 10.3 (Security Monitoring) and 10.4 (Forensics) depend on Epic 3, Section 3.5 (Audit Logging) and Epic 2, Section 2.3 (Event Persistence for Audit and Debugging)
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

