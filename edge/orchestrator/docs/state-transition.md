# Edge Orchestrator State Transition

This document describes the state transitions of the Edge Orchestrator during startup and normal operation.

## Overview

The Edge Orchestrator manages the lifecycle of the Edge Appliance, coordinating the establishment of secure communication with the VM and transitioning through various states until it becomes fully operational.

## States

The Edge Orchestrator can be in one of the following states:

1. **`disconnected`** - Initial state when Edge starts. No network connection to VM.
2. **`wireguard_connected`** - WireGuard tunnel is established to VM.
3. **`https_connected`** - HTTPS connection is established over WireGuard tunnel.
4. **`authenticated`** - Edge has successfully authenticated with VM (transport + identity).
5. **`capabilities_received`** - Edge has received its capabilities from VM (e.g. `cctv_camera`).
6. **`camera_discovered`** - Edge has performed camera discovery (CCTV service) after having camera capability.
7. **`camera_synced`** - Edge has synced discovered cameras with VM, and VM has decided which cameras this Edge should serve.
8. **`waiting_for_camera_screenshots`** - Edge is waiting for user to capture and label screenshots for model training.
9. **`screenshot_set_ready`** - User has captured and labeled screenshots, ready to sync to VM for training.
10. **`model_deployed`** - Trained model has been deployed to Edge and is ready for use.
11. **`frame_processing`** - Edge is actively processing frames from cameras using the deployed model (final operational state).
12. **`error`** - Error state when something goes wrong.

## State Transition Flow

### Initial State: `disconnected`

When the Edge Orchestrator starts, it begins in the `disconnected` state. In this state:
- Edge has no network connection to the VM
- All VM-related services are not yet operational
- Edge is waiting to establish a WireGuard tunnel

**Actions in this state:**
- Edge reads WireGuard configuration from config file
- Edge initializes WireGuard client service
- Edge attempts to establish WireGuard tunnel to VM endpoint

### Transition: `disconnected` → `wireguard_connected`

**Trigger:** WireGuard tunnel is successfully established.

**Event:** `network.wireguard.connected`

**What happens:**
1. WireGuard client service starts and configures the WireGuard interface
2. WireGuard tunnel is established to the VM endpoint (configured via `KVMEndpoint` in config)
3. Edge can now communicate with VM over the encrypted WireGuard tunnel
4. State transitions to `wireguard_connected`
5. `NetworkConnected` flag is set to `true`

**State properties:**
- `Status`: `wireguard_connected`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `false`

### Transition: `wireguard_connected` → `https_connected`

**Trigger:** HTTPS services are started and ready over WireGuard tunnel.

**Event:** `network.https.connected`

**What happens:**
1. HTTPS server service starts (listens on WireGuard interface for VM → Edge communication)
2. HTTPS client service starts (ready for Edge → VM communication)
3. State transitions to `https_connected`

**State properties:**
- `Status`: `https_connected`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `false`

### Transition: `https_connected` → `authenticated`

**Trigger:** Edge successfully authenticates with VM using the HTTPS client.

**Event:** `edge.authenticated`

**What happens:**
1. HTTPS client service automatically attempts authentication after startup (with 2-second delay)
2. Edge sends authentication request to VM endpoint: `POST https://{vm_endpoint}/api/v1/auth/authenticate`
   - Request body: `{"edge_id": "<edge_id>"}`
   - Uses mTLS (mutual TLS) with client certificates
3. VM validates the Edge credentials and responds with success
4. State transitions to `authenticated`
5. `VMAuthenticated` flag is set to `true`

**State properties:**
- `Status`: `authenticated`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

**Authentication Details:**
- Authentication uses the VM Gateway HTTP client service
- The authentication request is sent over the WireGuard tunnel using HTTPS
- Client certificates are used for mTLS authentication
- Edge ID is sent in the authentication request body

### Transition: `authenticated` → `capabilities_received`

**Trigger:** Edge receives capabilities from VM via the VM Gateway HTTPS server.

**Event:** `edge.capabilities_received`

**What happens:**
1. VM sends capabilities for this Edge to the Edge HTTPS server endpoint (e.g. `/api/v1/capabilities/sync`).
2. Capabilities are stored in meta-storage and exposed to the rest of the system.
3. If the Edge has CCTV capability (e.g. `"cctv_camera": true`), the State Manager can later trigger camera discovery.
4. State transitions to `capabilities_received`.

**State properties:**
- `Status`: `capabilities_received`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

### Transition: `capabilities_received` → `camera_discovered`

**Trigger:** Cameras are discovered by the CCTV service after CCTV capability is present.

**Event:** `camera.discovered`

**What happens:**
1. The State Manager sees CCTV capability for this Edge.
2. The State Manager triggers camera discovery via the CCTV service top interface (`DiscoverCameras`).
3. When cameras are discovered, CCTV publishes `camera.discovered` events.
4. The State Manager updates state to `camera_discovered`.

**State properties:**
- `Status`: `camera_discovered`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

### Transition: `camera_discovered` → `camera_synced`

**Trigger:** Discovered cameras are synced to VM, and VM responds with which cameras should be enabled.

**What happens:**
1. When state becomes `camera_discovered`, the State Manager collects discovered cameras from CCTV.
2. State Manager calls VM Gateway `SyncCameras`, which sends the discovered camera list to VM.
3. Mock / VM responds with a list of cameras and enablement decisions.
4. State Manager enables cameras that VM decided to enable via CCTV service.
5. State transitions to `camera_synced`.

**State properties:**
- `Status`: `camera_synced`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

### Transition: `camera_synced` → `waiting_for_camera_screenshots`

**Trigger:** VM requests labeled screenshots for model training.

**Event:** `snapshot.requested`

**What happens:**
1. VM sends a snapshot capture request to Edge via HTTPS server.
2. State Manager stores the pending snapshot request in meta-storage.
3. State transitions to `waiting_for_camera_screenshots`.
4. Edge is now waiting for user to capture and label screenshots.

**State properties:**
- `Status`: `waiting_for_camera_screenshots`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

### Transition: `waiting_for_camera_screenshots` → `screenshot_set_ready`

**Trigger:** User has captured and labeled all required screenshots and marked the set as ready.

**Event:** `screenshot_set.ready`

**What happens:**
1. User captures screenshots via web gateway API.
2. User labels each screenshot.
3. User marks the screenshot set as ready via API.
4. State Manager syncs labeled screenshots to VM for model training.
5. State transitions to `screenshot_set_ready`.

**State properties:**
- `Status`: `screenshot_set_ready`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

### Transition: `screenshot_set_ready` → `model_deployed`

**Trigger:** VM deploys a trained model to Edge.

**Event:** `model.deployed`

**What happens:**
1. VM sends trained model to Edge via HTTPS server (`/api/v1/models/deploy`).
2. Model is stored in object storage.
3. Model metadata is stored in meta-storage.
4. State Manager receives `model.deployed` event.
5. State Manager notifies AI gateway about model deployment.
6. State transitions to `model_deployed`.

**State properties:**
- `Status`: `model_deployed`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

### Transition: `model_deployed` → `frame_processing`

**Trigger:** Frame processing successfully starts for enabled cameras.

**What happens:**
1. State Manager executes `model_deployed` workflow.
2. State Manager gets all enabled cameras from CCTV service.
3. State Manager starts frame processing goroutine for each enabled camera.
4. Each camera's frame processing loop:
   - Captures frames at configured interval (default: 30 seconds)
   - Stores frames in object storage (`frames/{cameraID}/{date}/frame-{id}.jpg`)
   - Sends frames to AI gateway for processing
5. When at least one camera's frame processing starts successfully, state transitions to `frame_processing`.
6. Edge is now actively monitoring and processing camera frames.

**State properties:**
- `Status`: `frame_processing`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

**Frame Processing Details:**
- Frames are captured periodically (configurable interval, default: 30 seconds)
- Frames are stored in object storage for AI processing
- AI service processes frames using the deployed model
- Normal frames are deleted after processing
- Suspicious frames are moved to `security-events/{cameraID}/{date}/` bucket
- Security events are created for abnormal detections

## Reverse Transitions (Error Handling)

### `frame_processing` / `model_deployed` / `screenshot_set_ready` / `authenticated` / `https_connected` → `wireguard_connected`

**Trigger:** HTTPS connection is lost.

**Event:** `network.https.disconnected`

**What happens:**
- State transitions back to `wireguard_connected`
- `VMAuthenticated` is set to `false`
- Edge will attempt to re-establish HTTPS connection

### Any state → `disconnected`

**Trigger:** WireGuard tunnel is lost.

**Event:** `network.wireguard.disconnected`

**What happens:**
- State transitions to `disconnected`
- `NetworkConnected` is set to `false`
- `VMAuthenticated` is set to `false`
- Edge will attempt to re-establish WireGuard tunnel

## State Diagram

```
┌──────────────┐
│ disconnected │ (Initial State)
└──────┬───────┘
       │ WireGuard tunnel established
       ▼
┌──────────────────────┐
│ wireguard_connected   │
└──────┬────────────────┘
       │ HTTPS services started
       ▼
┌──────────────────┐
│ https_connected   │
└──────┬───────────┘
       │ Authentication successful
       ▼
┌──────────────────┐
│ authenticated    │
└──────┬───────────┘
       │ Capabilities received
       ▼
┌──────────────────────┐
│ capabilities_received│
└──────┬───────────────┘
       │ Camera discovered
       ▼
┌──────────────────┐
│ camera_discovered│
└──────┬───────────┘
       │ Cameras synced with VM
       ▼
┌──────────────────┐
│ camera_synced    │
└──────┬───────────┘
       │ Snapshot request received
       ▼
┌──────────────────────────────┐
│ waiting_for_camera_screenshots│
└──────┬───────────────────────┘
       │ Screenshots captured and labeled
       ▼
┌──────────────────┐
│ screenshot_set_ready│
└──────┬───────────┘
       │ Model deployed from VM
       ▼
┌──────────────────┐
│ model_deployed   │ (Model Ready)
└──────┬───────────┘
       │ Frame processing started
       ▼
┌──────────────────┐
│ frame_processing │ (Final Operational State)
└──────────────────┘

Reverse transitions (on errors):
- HTTPS disconnected → wireguard_connected (stops frame processing)
- WireGuard disconnected → disconnected (stops frame processing)
- Any error state → stops frame processing
```

## Implementation Details

### VM Gateway Startup Sequence

The VM Gateway (`VMGateway`) coordinates three services in the following order:

1. **WireGuard Client Service** - Establishes tunnel first
2. **HTTPS Server Service** - Starts after WireGuard (for VM → Edge communication)
3. **HTTPS Client Service** - Starts after WireGuard (for Edge → VM communication)

The HTTPS Client Service automatically attempts authentication 2 seconds after startup.

### State Manager

The State Manager (`StateManager`) listens to events from the event bus and updates the Edge state accordingly. It:
- Tracks state transitions
- Executes workflows based on state changes
- Coordinates service initialization
- Handles error recovery

### Events

Key events that trigger state transitions:
- `network.wireguard.connected` - WireGuard tunnel established
- `network.wireguard.disconnected` - WireGuard tunnel lost
- `network.https.connected` - HTTPS connection established
- `network.https.disconnected` - HTTPS connection lost
- `edge.authenticated` - Edge authenticated with VM

## Error Handling

If authentication fails:
- Edge remains in `https_connected` state
- Authentication can be retried (e.g., on next heartbeat)
- Edge will continue attempting to authenticate

If WireGuard tunnel is lost:
- Edge transitions to `disconnected`
- All services dependent on the tunnel are stopped
- Edge will attempt to re-establish the tunnel

## Notes

- The authentication process uses the VM Gateway HTTP client service, which communicates over the WireGuard tunnel
- All communication between Edge and VM is encrypted via WireGuard
- HTTPS communication uses mTLS for additional security
- State transitions are logged for debugging and monitoring

---

## Complete Business Logic Flow

This section provides a comprehensive overview of the Edge Orchestrator's business logic, data flows, and component interactions. This documentation is designed for AI model review and improvement suggestions.

### Architecture Overview

The Edge Orchestrator follows an **event-driven architecture** with the following core components:

1. **State Manager** - Central coordinator that manages Edge lifecycle and state transitions
2. **VM Gateway** - Handles bidirectional secure communication with VM (WireGuard + HTTPS)
3. **CCTV Service** - Manages camera discovery, frame capture, and screenshot management
4. **AI Gateway** - Bridges between Edge and AI service for frame processing
5. **Object Storage** - Stores images, models, and frames (MinIO/S3-compatible)
6. **Meta Storage** - Stores metadata, camera info, and Edge state (bbolt/BoltDB)
7. **Event Bus** - In-memory event system for inter-service communication
8. **Web Gateway** - HTTP API for user interactions (screenshot capture, labeling)

### Complete End-to-End Business Flow

#### Phase 1: Initialization and Connection (States: `disconnected` → `authenticated`)

**Business Logic:**
1. Edge starts in `disconnected` state
2. WireGuard client service initializes and establishes encrypted tunnel to VM
3. HTTPS server and client services start over WireGuard tunnel
4. HTTPS client automatically attempts authentication (2-second delay)
5. Authentication uses mTLS with client certificates
6. VM validates Edge credentials and responds
7. Edge transitions through: `disconnected` → `wireguard_connected` → `https_connected` → `authenticated`

**Data Flow:**
- **Outbound**: Edge → VM: `POST /api/v1/auth/authenticate` with `{"edge_id": "<edge_id>"}`
- **Inbound**: VM → Edge: Authentication response
- **Storage**: No persistent storage at this stage
- **Events**: `network.wireguard.connected`, `network.https.connected`, `edge.authenticated`

**Error Handling:**
- WireGuard connection failure: Retry with exponential backoff
- HTTPS connection failure: Retry after WireGuard reconnection
- Authentication failure: Remain in `https_connected`, retry on next heartbeat

#### Phase 2: Capability and Camera Setup (States: `authenticated` → `camera_synced`)

**Business Logic:**
1. VM sends capabilities to Edge via HTTPS server (`POST /api/v1/capabilities/sync`)
2. Capabilities stored in meta-storage (e.g., `{"cctv_camera": true}`)
3. State Manager checks for CCTV capability
4. If CCTV capability present, State Manager triggers camera discovery
5. CCTV service discovers cameras (USB and/or ONVIF)
6. Discovered cameras stored in meta-storage
7. State Manager collects discovered cameras and syncs to VM via `SyncCameras()`
8. VM responds with camera enablement decisions
9. State Manager enables/disables cameras based on VM decisions
10. Edge transitions: `authenticated` → `capabilities_received` → `camera_discovered` → `camera_synced`

**Data Flow:**
- **Inbound**: VM → Edge: `POST /api/v1/capabilities/sync` with capabilities JSON
- **Storage**: Capabilities stored in meta-storage
- **Outbound**: Edge → VM: `POST /api/v1/cameras/sync` with discovered camera list
- **Inbound**: VM → Edge: Camera enablement decisions
- **Storage**: Camera metadata stored in meta-storage
- **Events**: `edge.capabilities_received`, `camera.discovered`, `camera.registered`

**Camera Discovery Details:**
- **USB Cameras**: Scans `/dev/video*` devices, validates with FFmpeg
- **ONVIF Cameras**: Discovers on local network, validates ONVIF endpoints
- **Validation**: Each camera validated with 5-second timeout to prevent hangs
- **Storage**: Camera metadata includes: ID, name, type, device path, ONVIF endpoint, IP address, enabled status

#### Phase 3: Screenshot Collection for Training (States: `camera_synced` → `screenshot_set_ready`)

**Business Logic:**
1. VM requests labeled screenshots via HTTPS server (`POST /api/v1/snapshots/capture`)
2. Request includes: `camera_id`, `label`, `count`, `auto_capture` flag
3. State Manager stores pending snapshot request in meta-storage
4. State transitions to `waiting_for_camera_screenshots`
5. User captures screenshots via Web Gateway API (`POST /api/cameras/{camera_id}/capture`)
6. Web Gateway calls CCTV Service `CaptureScreenshot()` (security pattern: web gateway doesn't directly access storage)
7. CCTV Service:
   - Captures frame from camera using FFmpeg (10-second timeout)
   - Stores image in object storage at `snapshots/{date}/{camera_id}_{timestamp}_{uuid}.jpg`
   - Saves screenshot metadata in meta-storage (initially unlabeled)
   - Publishes `screenshot.saved` event
8. User labels each screenshot via Web Gateway API (`PUT /api/screenshots/{screenshot_id}`)
9. Web Gateway updates metadata in meta-storage (read-only pattern maintained)
10. User marks screenshot set as ready (`POST /api/snapshot-requests/{camera_id}/ready`)
11. State Manager receives `screenshot_set.ready` event
12. State transitions to `screenshot_set_ready`

**Data Flow:**
- **Inbound**: VM → Edge: `POST /api/v1/snapshots/capture` with snapshot request
- **Storage**: Pending request stored in meta-storage
- **User Action**: `POST /api/cameras/{camera_id}/capture` → CCTV Service
- **Storage**: Image stored in object storage, metadata in meta-storage
- **User Action**: `PUT /api/screenshots/{screenshot_id}` → Updates metadata in meta-storage
- **User Action**: `POST /api/snapshot-requests/{camera_id}/ready` → Triggers sync
- **Events**: `snapshot.requested`, `screenshot.saved`, `screenshot_set.ready`

**Security Pattern:**
- **Web Gateway is read-only for sensitive operations**: Web gateway cannot directly capture frames or store images
- **CCTV Service owns capture and storage**: Only CCTV service can capture frames and store images
- **Separation of concerns**: Web gateway triggers actions and reads results, but doesn't perform storage operations
- **Prevents tampering**: Web gateway cannot substitute screenshots or modify stored images

**Screenshot Metadata Structure:**
```json
{
  "screenshot_id": "uuid",
  "camera_id": "camera-id",
  "label": "normal",
  "custom_label": "user-friendly-label",
  "description": "description text",
  "object_key": "snapshots/2025-12-25/camera_id_timestamp_uuid.jpg",
  "created_at": "2025-12-25T10:36:27+01:00",
  "updated_at": "2025-12-25T10:36:37+01:00"
}
```

#### Phase 4: Screenshot Sync to VM for Training (State: `screenshot_set_ready`)

**Business Logic:**
1. When state becomes `screenshot_set_ready`, State Manager executes `executeScreenshotSetReadyWorkflow()`
2. State Manager fetches all labeled screenshots for the camera from meta-storage
3. For each labeled screenshot:
   - Retrieves image data from object storage using `object_key`
   - Base64-encodes image data
   - Determines image format from file extension
   - Creates `ScreenshotInfo` with metadata and encoded image
4. State Manager calls VM Gateway `SyncScreenshots()` with array of `ScreenshotInfo`
5. VM Gateway HTTPS client sends `POST /api/v1/screenshots/sync` to VM
6. VM receives screenshots, stores images and metadata
7. State Manager clears pending snapshot request from meta-storage
8. State remains `screenshot_set_ready` (waiting for model deployment)

**Data Flow:**
- **Storage Read**: Meta-storage → Labeled screenshots metadata
- **Storage Read**: Object storage → Image files (JPEG/PNG)
- **Processing**: Base64 encoding of image data
- **Outbound**: Edge → VM: `POST /api/v1/screenshots/sync` with:
  ```json
  {
    "edge_id": "edge-id",
    "camera_id": "camera-id",
    "screenshots": [
      {
        "screenshot_id": "uuid",
        "camera_id": "camera-id",
        "object_key": "snapshots/...",
        "label": "normal",
        "custom_label": "label",
        "description": "description",
        "metadata": {},
        "created_at": 1234567890,
        "image_data": "base64-encoded-image",
        "image_format": "jpeg"
      }
    ]
  }
  ```
- **Storage**: VM stores images and metadata on disk
- **Events**: None (sync is synchronous operation)

**Screenshot Sync Details:**
- Only **labeled** screenshots are synced (unlabeled are skipped)
- Image data is **base64-encoded** for JSON transport
- Image format is determined from file extension (jpeg, png, gif, webp)
- Metadata excludes `image_data` when stored separately (to keep JSON clean)
- VM stores: individual metadata JSON files, image files, and summary JSON

#### Phase 5: Model Training on VM (External to Edge)

**Business Logic (VM Side):**
1. VM receives labeled screenshots via `/api/v1/screenshots/sync`
2. VM stores screenshots in `edge/mocks/vm-server/data/{edge_id}/{camera_id}/`
3. Training service converts screenshots to dataset format:
   - Creates `metadata.json` with dataset info
   - Creates `manifest.json` with screenshot list
   - Copies images to `screenshots/` directory
   - Uses `label` field (not `custom_label`) for training classes
4. Training service loads baseline YOLOv8 model
5. Training service trains model on dataset
6. Trained model exported as ONNX format
7. Model stored in training output directory

**Data Transformation:**
- **Input**: Screenshot files + metadata JSON
- **Output**: YOLOv8 dataset format with:
  - `metadata.json`: Dataset metadata, label counts
  - `manifest.json`: Screenshot list with labels
  - `screenshots/`: Image files organized by label
- **Training**: YOLOv8 classification training
- **Output**: Trained ONNX model file

#### Phase 6: Model Deployment to Edge (States: `screenshot_set_ready` → `model_deployed`)

**Business Logic:**
1. VM sends trained model to Edge via HTTPS server (`POST /api/v1/models/deploy`)
2. Request is multipart form with:
   - `metadata`: JSON with model metadata (model_id, version, model_type, camera_id, framework, training_dataset_id, training_date, input_shape, preprocessing, total_size)
   - `model`: Binary model file (ONNX format)
3. VM Gateway HTTPS server receives request
4. VM Gateway validates model size matches metadata
5. VM Gateway stores model in object storage:
   - Model file: `models/{model_id}/model.onnx`
   - Metadata file: `models/{model_id}/metadata.json`
6. VM Gateway saves model metadata in meta-storage:
   - Model ID, deployment ID, model path, metadata path, deployed timestamp, version, framework, model type
7. VM Gateway publishes two events:
   - `model.deployment.status` (status: "deployed", deployment_id, model_path, model_id)
   - `model.deployed` (model_id, deployment_id, version, model_type, framework, model_path, metadata_path, camera_id)
8. State Manager receives `model.deployed` event
9. State Manager transitions from `screenshot_set_ready` → `model_deployed`
10. State Manager notifies AI gateway about model deployment
11. AI gateway notifies AI service to load model from MinIO

**Data Flow:**
- **Inbound**: VM → Edge: `POST /api/v1/models/deploy` (multipart form)
- **Storage**: Model file → Object storage at `models/{model_id}/model.onnx`
- **Storage**: Metadata JSON → Object storage at `models/{model_id}/metadata.json`
- **Storage**: Model metadata → Meta-storage (DeployedModelMetadata)
- **Events**: `model.deployment.status`, `model.deployed`
- **Outbound**: State Manager → AI Gateway: `NotifyModelDeployment()`
- **Outbound**: AI Gateway → AI Service: Model load notification

**Model Metadata Structure:**
```json
{
  "deployment_id": "deploy-timestamp",
  "model_id": "baseline-yolov8n-dataset-id-timestamp",
  "version": "1.0",
  "model_type": "yolov8n",
  "camera_id": "camera-id",
  "framework": "pytorch",
  "training_dataset_id": "dataset-id",
  "training_date": "2025-12-25T10:36:45Z",
  "input_shape": [1, 3, 640, 640],
  "preprocessing": {
    "normalize": true,
    "resize": [640, 640]
  },
  "total_size": 12582912
}
```

#### Phase 7: Continuous Frame Processing (States: `model_deployed` → `frame_processing`)

**Business Logic:**
1. When state becomes `model_deployed`, State Manager executes `executeModelDeployedWorkflow()`
2. State Manager gets all enabled cameras from CCTV service
3. For each enabled camera:
   - Verifies camera exists and is enabled
   - Creates cancel context for frame processing
   - Starts frame processing goroutine
4. Each camera's frame processing loop:
   - Waits for configured interval (default: 30 seconds, configurable)
   - Captures frame from camera using CCTV Service `CaptureFrame()`
   - Generates frame ID: `frame-{cameraID}-{timestamp}`
   - Creates frame key: `frames/{cameraID}/{date}/frame-{id}.jpg`
   - Stores frame in object storage using `StoreFrame()`
   - Sends frame to AI gateway using `ProcessFrame()`
5. When at least one camera's frame processing starts successfully:
   - State transitions from `model_deployed` → `frame_processing`
   - Edge is now actively monitoring

**Frame Processing Loop Details:**
- **Interval**: Configurable (default: 30 seconds)
- **First Frame**: Captured immediately (no initial delay)
- **Subsequent Frames**: Captured at regular intervals
- **Error Handling**: Frame capture failures are logged but don't stop the loop
- **Cancellation**: Loop stops when camera is disabled or state changes

**Data Flow (Per Frame):**
- **Capture**: CCTV Service → Camera → Frame data (JPEG bytes)
- **Storage**: Frame → Object storage at `frames/{cameraID}/{date}/frame-{id}.jpg`
- **Processing**: Frame → AI Gateway → AI Service
- **AI Processing**: AI Service loads model from MinIO, processes frame
- **Result**: Normal frame deleted, suspicious frame moved to security-events bucket

**AI Gateway Processing:**
1. AI Gateway receives frame via `ProcessFrame(cameraID, frameKey, frameData)`
2. AI Gateway sends frame to AI service for inference
3. AI Service:
   - Loads model from MinIO if not already loaded (using model metadata)
   - Processes frame with model
   - Determines if frame is similar to training set (normal) or has anomalies (suspicious)
   - Returns inference results with confidence scores
4. AI Gateway processes results:
   - If confidence exceeds threshold: Creates security event
   - Normal frames: Deleted from object storage
   - Suspicious frames: Moved to `security-events/{cameraID}/{date}/` bucket

**Frame Lifecycle:**
- **Normal Frame**: Stored → Processed → Deleted
- **Suspicious Frame**: Stored → Processed → Moved to security-events bucket → Security event created

**Security Event Structure:**
- Event ID, camera ID, timestamp
- Event type: `anomaly_detected`
- Detection details: bounding boxes, confidence scores, detected objects
- Frame path in security-events bucket

### Component Interactions

#### State Manager ↔ CCTV Service
- **State Manager → CCTV**: `DiscoverCameras()`, `GetCamera()`, `ListCameras()`, `CaptureFrame()`, `CaptureScreenshot()`
- **CCTV → State Manager**: Events (`camera.discovered`, `screenshot.saved`)
- **Purpose**: Camera management and frame/screenshot capture

#### State Manager ↔ VM Gateway
- **State Manager → VM Gateway**: `SyncCameras()`, `SyncScreenshots()`, `Authenticate()`, `ReportDeploymentStatus()`
- **VM Gateway → State Manager**: Events (via HTTPS server handlers)
- **Purpose**: Bidirectional communication with VM

#### State Manager ↔ AI Gateway
- **State Manager → AI Gateway**: `NotifyModelDeployment()`, `ProcessFrame()`
- **AI Gateway → State Manager**: Events (`ai.detection`, `ai.inference`)
- **Purpose**: AI model management and frame processing

#### State Manager ↔ Object Storage
- **State Manager → Object Storage**: `StoreFrame()`, `LoadFrame()`, `DeleteFrame()`, `MoveFrameToSecurityEvent()`, `StoreSnapshot()`, `LoadSnapshot()`, `StoreModel()`, `LoadModel()`
- **Purpose**: Binary data storage (images, models, frames)

#### State Manager ↔ Meta Storage
- **State Manager → Meta Storage**: `SaveEdgeState()`, `GetEdgeState()`, `SaveCamera()`, `GetCamera()`, `SaveScreenshot()`, `GetScreenshot()`, `SaveDeployedModel()`, `GetDeployedModel()`, `SavePendingSnapshotRequest()`, `GetPendingSnapshotRequest()`
- **Purpose**: Metadata and state persistence

#### CCTV Service ↔ Object Storage
- **CCTV → Object Storage**: `StoreSnapshot()`, `StoreFrame()` (via State Manager)
- **Purpose**: Image storage for screenshots and frames

#### CCTV Service ↔ Meta Storage
- **CCTV → Meta Storage**: `SaveCamera()`, `SaveScreenshot()`, `UpdateScreenshot()`
- **Purpose**: Camera and screenshot metadata

#### AI Gateway ↔ AI Service
- **AI Gateway → AI Service**: HTTP requests for inference, model loading
- **AI Service → AI Gateway**: Inference responses
- **Purpose**: Frame processing and anomaly detection

### Event-Driven Architecture

**Event Bus Pattern:**
- All services communicate via event bus (in-memory pub/sub)
- Events are typed and include: Type, Source, Timestamp, Data
- State Manager subscribes to all relevant events
- Services publish events for state changes and operations

**Key Events:**
- `network.wireguard.connected` - WireGuard tunnel established
- `network.https.connected` - HTTPS connection established
- `edge.authenticated` - Edge authenticated with VM
- `edge.capabilities_received` - Capabilities received from VM
- `camera.discovered` - Camera discovered by CCTV service
- `camera.registered` - Camera registered in meta-storage
- `snapshot.requested` - VM requested labeled screenshots
- `screenshot.saved` - Screenshot captured and saved
- `screenshot_set.ready` - User marked screenshot set as ready
- `model.deployed` - Model deployed to Edge
- `model.deployment.status` - Model deployment status update
- `ai.detection` - AI detected anomaly
- `ai.inference` - AI inference completed

### Storage Patterns

#### Object Storage (MinIO/S3)
- **Buckets**: Single bucket with key-based organization
- **Screenshots**: `snapshots/{date}/{camera_id}_{timestamp}_{uuid}.jpg`
- **Frames**: `frames/{cameraID}/{date}/frame-{id}.jpg`
- **Models**: `models/{model_id}/model.onnx`, `models/{model_id}/metadata.json`
- **Security Events**: `security-events/{cameraID}/{date}/{filename}.jpg`
- **Clips**: `clips/{cameraID}/{timestamp}.mp4`

#### Meta Storage (bbolt/BoltDB)
- **Edge State**: Current state, network status, authentication status
- **Cameras**: Camera metadata, enabled status, configuration
- **Screenshots**: Screenshot metadata (ID, camera, label, object_key, timestamps)
- **Models**: Deployed model metadata (ID, path, version, framework, deployment info)
- **Pending Requests**: Snapshot capture requests from VM

### Security Considerations

1. **mTLS Authentication**: All VM ↔ Edge communication uses mutual TLS
2. **WireGuard Encryption**: All traffic encrypted via WireGuard tunnel
3. **Read-Only Web Gateway**: Web gateway cannot directly modify stored images
4. **CCTV Service Ownership**: Only CCTV service can capture and store images
5. **Model Validation**: Model size validated against metadata
6. **Frame Processing Isolation**: Each camera's frame processing runs in separate goroutine

### Error Handling Strategies

1. **Network Failures**: Automatic retry with exponential backoff
2. **Service Unavailable**: Graceful degradation, state remains in previous state
3. **Frame Capture Failures**: Logged but don't stop processing loop
4. **Storage Failures**: Logged, operations retried
5. **State Transitions**: Validated before execution
6. **Timeout Protection**: All external operations have timeouts (FFmpeg: 5s, Frame capture: 10s)

### Performance Considerations

1. **Frame Processing Interval**: Configurable (default: 30s) to balance performance and detection latency
2. **Concurrent Processing**: Each camera processes frames in separate goroutine
3. **Base64 Encoding**: Used for screenshot sync (necessary for JSON transport, but adds ~33% overhead)
4. **Object Storage**: Direct file storage for frames (no encoding overhead)
5. **Event Bus**: In-memory for low latency
6. **State Persistence**: Async writes to meta-storage

### Configuration

**Frame Processing Interval**: Default 30 seconds, configurable via state manager
**Timeouts**:
- FFmpeg validation: 5 seconds
- Frame capture: 10 seconds
- HTTP requests: 30 seconds
- Authentication: 30 seconds

### Data Flow Summary

**Screenshot Collection Flow:**
```
User → Web Gateway → CCTV Service → Object Storage (image) + Meta Storage (metadata)
User → Web Gateway → Meta Storage (label update)
User → Web Gateway → State Manager → Event Bus → State Manager → VM Gateway → VM
```

**Model Deployment Flow:**
```
VM → VM Gateway HTTPS Server → Object Storage (model) + Meta Storage (metadata) → Event Bus → State Manager → AI Gateway → AI Service
```

**Frame Processing Flow:**
```
State Manager → CCTV Service → Camera → Frame → Object Storage → AI Gateway → AI Service → Object Storage (delete/move)
```

### Complete Business Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    EDGE ORCHESTRATOR BUSINESS FLOW                       │
└─────────────────────────────────────────────────────────────────────────┘

PHASE 1: INITIALIZATION
────────────────────────
Edge Start → WireGuard Tunnel → HTTPS Services → Authentication
  ↓              ↓                    ↓                ↓
disconnected → wireguard_connected → https_connected → authenticated

PHASE 2: CAPABILITY & CAMERA SETUP
───────────────────────────────────
VM → Capabilities → Edge → Camera Discovery → Camera Sync
  ↓                    ↓              ↓                ↓
capabilities_received → camera_discovered → camera_synced

PHASE 3: SCREENSHOT COLLECTION
───────────────────────────────
VM → Snapshot Request → Edge → User Captures → User Labels → Ready
  ↓                        ↓            ↓            ↓          ↓
waiting_for_camera_screenshots → screenshot_set_ready

PHASE 4: SCREENSHOT SYNC
─────────────────────────
Edge → Fetch Labeled Screenshots → Base64 Encode → Sync to VM
  (meta-storage)      (object-storage)      (VM Gateway)

PHASE 5: MODEL TRAINING (VM SIDE)
──────────────────────────────────
VM → Convert Screenshots → Train Model → Export ONNX

PHASE 6: MODEL DEPLOYMENT
──────────────────────────
VM → Deploy Model → Edge → Store Model → Notify AI Gateway
  ↓                    ↓         ↓              ↓
model_deployed → (object-storage) → (meta-storage) → (AI Gateway)

PHASE 7: FRAME PROCESSING
───────────────────────────
Edge → Start Frame Loops → Capture Frames → Store → Process → Delete/Move
  ↓              ↓              ↓           ↓        ↓          ↓
frame_processing → (30s interval) → (object-storage) → (AI Service) → (cleanup)
```

### Data Flow Architecture

```
┌─────────────┐
│     VM      │
└──────┬──────┘
       │ HTTPS/mTLS over WireGuard
       ▼
┌─────────────────┐
│   VM Gateway     │◄──┐
│  (HTTPS Server)  │   │ Events
└──────┬──────────┘   │
       │              │
       ▼              │
┌─────────────────┐   │
│  State Manager  │───┘
│  (Coordinator)  │
└─────┬───┬───┬───┘
      │   │   │
      │   │   └──► Event Bus
      │   │
      │   ├──► CCTV Service ──► Camera ──► Frames/Screenshots
      │   │                          │
      │   │                          ▼
      │   └──► AI Gateway ──► AI Service ──► Model Processing
      │
      └──► Object Storage (MinIO) ──► Images, Models, Frames
      │
      └──► Meta Storage (bbolt) ──► Metadata, State, Config
```

### Key Design Decisions

1. **Event-Driven Architecture**: Loose coupling between services via event bus
2. **State Manager as Coordinator**: Single point of coordination for all workflows
3. **Security Pattern**: Web gateway is read-only, CCTV service owns capture/storage
4. **Separation of Concerns**: Each service has clear responsibilities
5. **Storage Separation**: Binary data (object storage) vs metadata (meta storage)
6. **Async Processing**: Frame processing runs in separate goroutines per camera
7. **Timeout Protection**: All external operations have timeouts to prevent hangs

### State Persistence

- **Edge State**: Persisted to meta-storage on every state transition
- **Recovery**: On restart, Edge loads last known state from meta-storage
- **Atomicity**: State transitions are atomic (lock → update → persist → unlock)

### Concurrency Model

- **State Manager**: Single goroutine processes events sequentially
- **Frame Processing**: One goroutine per camera (concurrent frame capture)
- **VM Gateway**: Separate goroutines for HTTPS server and client
- **CCTV Service**: Separate goroutines for discovery and frame capture
- **Synchronization**: Mutexes protect shared state, channels for coordination

### Potential Improvements for AI Review

1. **Frame Processing Interval**: Make configurable via config file
2. **Batch Processing**: Consider batching multiple frames for AI processing
3. **Frame Compression**: Compress frames before storage to reduce storage usage
4. **Model Versioning**: Support multiple model versions and A/B testing
5. **Frame Retention**: Configurable retention policy for normal frames
6. **Security Event Aggregation**: Aggregate similar events to reduce noise
7. **Health Monitoring**: Add health checks for frame processing loops
8. **Metrics**: Add metrics for frame processing rate, AI inference latency, storage usage
9. **Graceful Shutdown**: Ensure frame processing loops stop gracefully on shutdown
10. **Error Recovery**: Automatic recovery from frame processing failures
11. **Model Hot-Swapping**: Support model updates without stopping frame processing
12. **Frame Queue**: Add queue for frames awaiting AI processing to handle bursts
13. **State Machine Validation**: Add validation for all state transitions
14. **Event Ordering**: Ensure events are processed in correct order
15. **Distributed Tracing**: Add tracing for end-to-end request flow
16. **Circuit Breaker**: Add circuit breaker for AI service calls
17. **Rate Limiting**: Add rate limiting for frame processing to prevent overload
18. **Backpressure**: Handle backpressure when AI service is slow
19. **Frame Sampling**: Support configurable frame sampling (e.g., every Nth frame)
20. **Multi-Model Support**: Support multiple models per camera for different detection types

