# Edge Orchestrator State Transition Review

Scope
- Document: `edge/orchestrator/docs/state-transition.md`
- Code reviewed: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go`, `edge/orchestrator/internal/event-bus/inmemory/inmemory_event_bus.go`, `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go`

Goals
- Full code review aligned with the documented flow.
- Event flow review (ordering, reliability, and recoverability).
- Security patterns review (data provenance and access surface).
- Suggestions for improvement (no code changes in this review).

Findings
- Critical: HTTPS disconnect does not transition from `model_deployed`/`frame_processing` to `wireguard_connected`, so the system can remain in a processing state while connectivity is down and frame loops may keep running. This diverges from the documented reverse transitions and bypasses the stop-processing guard that runs only on state changes. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:438-546`, `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1244-1299`.
- High: The state machine is global, not per camera. A single camera snapshot request or readiness forces the whole Edge into `waiting_for_camera_screenshots` or `screenshot_set_ready`, which blocks independent multi-camera flows. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:92-118`, `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:715-806`.
- High: In-memory event bus drops events when buffers are full. Lost events can skip critical transitions (snapshot requests, model deployment, connection changes) with no retry or replay. Code: `edge/orchestrator/internal/event-bus/inmemory/inmemory_event_bus.go:71-103`.
- High: Web gateway accepts client-supplied `image_data` and writes it directly to storage, violating the documented "web gateway read-only" security model and enabling data poisoning or bypassing CCTV capture provenance. Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:260-350`.
- Medium: Data race risk. `executeWorkflow` mutates `m.state.AIProcessingActive` without locking while other goroutines read/write state under `m.mu`. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:595-600`.
- Medium: `pendingSync` is accessed across goroutines without synchronization, which can produce missed or repeated capability sync under concurrency. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1065-1084`, `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1900-2022`.
- Medium: Model deployment events are ignored if the current state is not `screenshot_set_ready`, so out-of-order or post-reconnect deployments can be lost even though the model arrives. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1180-1189`.
- Low: Documentation omits implemented states (`initializing`, `wg_connecting`, `http_connecting`), so the doc is incomplete for operational transitions. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:59-90`, doc: `edge/orchestrator/docs/state-transition.md`.

Additional Findings (code-backed)
- Critical: VM gateway HTTPS server “dev mode without certs” likely cannot start. The code path constructs a `tls.Config` without certificates, but still calls `ServeTLS(listener, "", "")`, which typically requires a certificate configured (either via provided cert/key files or `tls.Config.Certificates`). The warning suggests it can run “without TLS certificates”, but the implementation likely fails at runtime. Code: `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:291-337`.
- High: Screenshot thumbnails are referenced but not actually stored. The web gateway generates `thumbnailKey` and returns it, and the list/get endpoints attempt to load thumbnails (falling back to the full image), but `handleSaveScreenshot` only stores the full image and never creates/stores a thumbnail object. This makes the thumbnail API misleading and can cause repeated full-size reads for “thumbnails”. Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:69-105`, `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:334-377`.
- High: Screenshot content-type / encoding inconsistencies and potential data corruption:
  - `decodeBase64Image` accepts JPEG or PNG, but `StoreSnapshot` is always invoked with `"image/jpeg"`.
  - API responses wrap returned images as `data:image/jpeg;base64,...` even if the stored blob was PNG.
  - `/api/screenshots/:id/image` always responds with `Content-Type: image/jpeg`.
  These mismatches can break clients, confuse downstream tooling, and undermine dataset provenance. Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:165-175`, `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:297-349`, `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:671-722`.
- High: Resource usage and leakage risks in screenshot handlers:
  - List endpoints include base64 thumbnails by default, potentially returning large payloads and loading many objects into memory (worst case: N screenshots × full image bytes).
  - Several code paths read object storage streams with `io.ReadAll` but only close the reader on the success path (if `ReadAll` fails, `Close()` is skipped). Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:69-105`, `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:149-174`.
- High: The API surface implies pagination (`limit`/`offset`) but the storage layer ignores it. The web gateway parses `limit` and `offset` query params, but `meta-storage` `ListScreenshots` doesn’t implement them, so the API silently returns unbounded results. Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:50-59`, `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go:562-621`.
- Medium: Workflow execution is concurrent-per-event, which creates ordering hazards and “work duplication” risk. `handleEvent` spawns a goroutine per event to execute workflows, while state updates happen in the main event loop. This means:
  - workflows can run out-of-order relative to subsequent state transitions,
  - state-based workflows (`executeModelDeployedWorkflow`, etc.) can be invoked repeatedly on unrelated events,
  - the system relies on idempotency of the workflow bodies (some are, some are not). Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:388-633`.
- Medium: AIGateway behavior diverges from the documented frame lifecycle. The state-transition doc says the AI gateway deletes normal frames and moves suspicious frames to `security-events/...`, but the current `AIGateway.ProcessFrame` implementation only performs inference and event callback, and explicitly comments that lifecycle “should be handled by AI service” / “for now, rely on AI service”. This is a doc/implementation mismatch that affects storage growth and event semantics. Code: `edge/orchestrator/internal/ai-gateway/impl/ai-gateway-impl.go:397-425`.
- Medium: Edge state history ordering likely returns oldest-first, not “most recent first” as the function comment claims. The cursor iteration already collects keys in descending order, then the code iterates that key slice in reverse again. Code: `edge/orchestrator/internal/meta-storage/bbolt-imp/meta-storage-impl.go:974-1018`.
- Medium: `model.deployment.status` events are published but appear to have no consumer in the state manager. This can silently drop intended status reporting / UI signals and makes observability confusing. Code: publisher `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:740-756`, state manager event switch `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:555-620` (no case for that event).
- Low: In dev/localhost mode, `GetConfig` returns `wireguard.enabled: true` and hardcoded interface values, which can mislead the VM about the actual connection mode. Code: `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:451-467`.
- Low: Object storage MinIO client is hard-coded to `Secure: false` and uses a hard-coded bucket name (`edge-storage`). This is fine for local PoC but should be called out explicitly as a security/configuration limitation. Code: `edge/orchestrator/internal/object-storage/minio-imp/minio_object_storage.go:48-56`.

Event Flow Review
- The documented reverse transitions (HTTPS/WireGuard disconnect) do not fully match the actual state machine handling, especially for `model_deployed` and `frame_processing`. This creates a recovery gap and can leave processing active when connectivity is down.
- Event handling relies on best-effort in-memory pub/sub; there is no durable delivery or retry for critical events. Dropped events can leave the state machine stuck and workflows untriggered.
- The flow is implemented as a single global state while most events are camera-scoped; this mismatch will surface as soon as multiple cameras have concurrent snapshot and model flows.
- Because workflows are executed concurrently per event, event ordering is not sufficient to guarantee workflow ordering; any multi-step workflow that assumes serialized execution should be treated as potentially racy.

Security Patterns Review
- The "web gateway is read-only" pattern is not enforced in code; `image_data` allows direct injection of training data and bypasses capture provenance. This weakens trust in the dataset and model quality.
- Screenshot endpoints return base64 images without access control at the handler level. If the web gateway is reachable outside a trusted boundary, this increases data exposure risk.
- Model deployment and snapshot request endpoints are effectively “VM-trusted” surfaces; in dev they can run with relaxed TLS expectations, so they should be treated as sensitive even in localhost scenarios.

Recommendations
1) Split state into a connection-level state machine and a per-camera state machine keyed by camera ID, so camera flows can progress independently.
2) Make critical events reliable (durable queue or at least ack/retry semantics) for `snapshot.requested`, `model.deployed`, and connectivity events.
3) Align disconnect handling with the documented behavior: transition out of `model_deployed`/`frame_processing` on HTTPS/WireGuard loss and stop frame loops deterministically.
4) Enforce capture provenance by removing client-supplied `image_data` or requiring signed/verified capture tokens from CCTV service; add authentication/authorization gates if the web gateway is not strictly local.
5) Add concurrency protection around `m.state` and `pendingSync`, plus tests for multi-camera and disconnect/reconnect ordering.
6) Make screenshot media handling consistent end-to-end:
   - persist and return the correct content type / format,
   - ensure thumbnails are actually produced and stored (or remove thumbnail keys/fields),
   - avoid base64 thumbnails by default for list endpoints (make it opt-in), and implement real pagination at storage level.
7) Fix/clarify dev-mode TLS behavior for the VM gateway HTTPS server: either require certs even in dev, or implement an explicit insecure HTTP-only dev server (and document it clearly).
8) Decide and document who owns frame lifecycle (AI service vs AI gateway vs state manager) and align code + docs accordingly; add explicit retention/cleanup policies to prevent unbounded storage growth.
9) Add tests specifically targeting the current high-risk gaps:
   - race detector coverage for state manager (`go test -race`),
   - out-of-order event scenarios (model deployed before screenshot_set_ready, disconnect mid-deploy),
   - screenshot upload/capture flows for PNG/JPEG content-type correctness,
   - meta-storage edge state history ordering behavior.

Notes
- This review does not change code; it summarizes gaps between the current implementation and the documented intent.

Additional Review: Internal Service Interface Usage
- All core services (event-bus, meta-storage, object-storage, CCTV, AI gateway, state manager, web gateway, WG/HTTPS client/server) are referenced via their interface packages; only those interface packages import `impl` to construct concrete instances. This matches the "top interface only" usage rule.
- Exception: Orchestrator server uses the implementation type directly. `*impl.Server` is constructed in `edge/orchestrator/internal/orchestrator/orchestrator.go` and injected/used in `edge/orchestrator/cmd/server/main.go`.
- Note: `edge/orchestrator/internal/iot/cctv/cctv-iface.go` instantiates `impl.NewFFmpegWrapper` inside the top interface package. This keeps implementation usage localized but is still a direct `impl` dependency at the interface layer.

Additional Findings: Goroutine and Resource Management
- Critical: `stopAllFrameProcessing` clears `frameProcessingActive` map immediately after canceling contexts, but doesn't wait for goroutines to finish. The frame processing goroutines add themselves to `m.wg`, but map cleanup happens synchronously. If a goroutine attempts to access or modify the map after cleanup (unlikely but possible in edge cases), this could cause issues. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1365-1381`.
- Critical: `Stop()` method cancels context and waits for waitgroup, but doesn't explicitly stop frame processing loops first. Frame processing goroutines will eventually stop when context is canceled, but there's no explicit ordering guarantee. If frame processing is active during shutdown, goroutines might attempt operations on services that are being shut down. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:346-369`.
- Medium: Goroutine leak risk in `startFrameProcessingForCamera`: if `startFrameProcessingForCamera` is called multiple times for the same camera (e.g., due to concurrent events), the check for existing processing happens outside the critical section where the goroutine is actually started. Between the check and goroutine start, another caller could also start a goroutine, leading to multiple processing loops for the same camera. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1303-1345`.
- Medium: The `pendingSnapshotRequests` map is accessed from multiple methods (`SavePendingSnapshotRequest`, `GetPendingSnapshotRequest`, `ClearPendingSnapshotRequest`) under `m.mu` lock, but `GetAllPendingSnapshotRequests` at line 125 writes to it from a read lock context after upgrading from meta-storage. This pattern (write under read lock) is incorrect and could cause races. Code: `edge/orchestrator/internal/state-mng/impl/snapshot_request_storage.go:93-127`.
- Low: Frame processing interval is hardcoded to 30 seconds and not configurable via config file, limiting operational flexibility. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:181`.

Additional Findings: Error Handling and Validation
- High: No validation of required certificates at startup. The HTTPS server code checks for certificate files and falls back to "dev mode without certs", but never validates that the certificates are actually valid or match the expected identity. This could lead to silent mTLS failures or man-in-the-middle vulnerabilities. Code: `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:238-300`.
- High: Frame capture and processing errors are logged but don't trigger any recovery mechanism. If all cameras start failing to capture frames (e.g., due to hardware issues), the system will remain in `frame_processing` state but silently stop working. There's no health monitoring or alerting for persistent frame capture failures. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1412-1472`.
- Medium: Object storage operations in frame processing don't have explicit timeouts beyond the context timeout. If MinIO becomes slow, frame processing could block indefinitely on storage operations, causing frame processing loops to stall without clear indication. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1432-1447`.
- Medium: The `executeWorkflow` method spawns a goroutine per event without limiting concurrency. If events arrive faster than workflows can process them, unbounded goroutines could be created, leading to resource exhaustion. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:406-410`.
- Low: Configuration validation is minimal. The code uses fallback values when config is missing or invalid, but doesn't validate that the configuration makes sense (e.g., negative timeouts, invalid URLs, conflicting settings). This could lead to confusing runtime behavior.

Additional Findings: State Machine and Consistency
- High: State transitions from `model_deployed` or `frame_processing` don't clean up per-camera state. When transitioning back to `wireguard_connected` on disconnect, the code calls `stopAllFrameProcessing()` (line 545), but this only cancels goroutines. Any per-camera metadata, pending frames, or AI gateway state is not explicitly cleaned up. This could cause stale state to persist across reconnection cycles. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:542-546`.
- Medium: The state persistence call (`persistStateToStorage`) is fire-and-forget (no error checking at call sites). If meta-storage writes fail silently, the in-memory state and persisted state will diverge, causing incorrect recovery after restart. Code: multiple call sites, e.g., `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:532`.
- Medium: In `handleCapabilitiesReceived`, if camera discovery fails, the state is set to `EdgeStatusCCTVServiceError`, but the error state doesn't trigger any recovery workflow or retry logic. The system will remain stuck in error state until manual intervention. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:657-688`.
- Medium: `handleModelDeployed` only transitions state if current status is `screenshot_set_ready` (line 1174). If a model deployment event arrives while in any other state (e.g., after disconnect/reconnect, or if events are processed out of order), the deployment is silently ignored and the system cannot proceed to frame processing. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1172-1186`.
- Low: The "initializing", "wg_connecting", and "http_connecting" states are implemented but not documented in `state-transition.md`, creating a documentation gap for operational understanding.

Additional Findings: Memory and Performance
- High: `syncScreenshotsToVM` loads all labeled screenshot images into memory at once, base64-encodes them, and sends them in a single HTTP request. For large screenshot sets (e.g., 50+ images at 1-2 MB each), this could consume hundreds of MBs of memory and cause request timeouts or out-of-memory errors. No batching or streaming is implemented. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:808-955`.
- Medium: The capability sync loop runs every 5 minutes and processes all cameras every time, even if nothing has changed. For systems with many cameras, this creates unnecessary network traffic and CPU usage. No change detection or incremental sync is implemented. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1927-2022`.
- Medium: Event bus subscriber channels have a fixed buffer size (100 by default). Under high event load, channels can fill up and events are silently dropped (lines 88-92, 98-103). Critical events like `model.deployed` could be lost without any indication to operators. No backpressure or flow control mechanism exists. Code: `edge/orchestrator/internal/event-bus/inmemory/inmemory_event_bus.go:71-104`.
- Low: Frame processing creates a new `time.NewTicker` for each camera but doesn't pool or reuse tickers. For systems with many cameras, this creates unnecessary timer objects. Minor performance issue. Code: `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:1387`.

Additional Findings: Security and Data Integrity
- Critical: The web gateway accepts `image_data` in POST `/api/screenshots` and stores it directly without any provenance validation (line 297-311 in handlers_screenshots.go). This allows an attacker with access to the web gateway to inject arbitrary training data, bypassing camera capture entirely. The "CCTV service owns capture" security pattern is violated. Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:260-349`.
- High: Screenshot image data encoding/decoding assumes JPEG format, but `decodeBase64Image` accepts PNG as well (line 705). However, all stored images are marked as "image/jpeg" (line 338) and returned with JPEG content-type (line 214), even if the original was PNG. This can corrupt PNG screenshots or confuse clients expecting actual format to match content-type. Code: `edge/orchestrator/internal/web-gateway/impl/handlers_screenshots.go:680-722`, `338`, `214`.
- High: TLS certificates are loaded at startup but not validated for expiration or revocation. If certificates expire or are revoked, the system will continue to use them until restart, potentially creating security vulnerabilities or connectivity issues. Code: `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:238-292`.
- Medium: Model deployment accepts model files via multipart form but only validates size against metadata, not content integrity. A corrupted or malicious model file could be deployed without detection until inference fails. No checksum validation or signature verification is performed. Code: `edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/impl/https-server.go:654-757`.
- Low: Edge ID defaults to "edge-dev-001" in multiple places if not configured. This hardcoded fallback could lead to ID collisions in multi-edge deployments and makes it harder to diagnose configuration issues. Code: multiple locations, e.g., `edge/orchestrator/internal/state-mng/impl/state_mng_impl.go:998`, `1608`.

Updated Recommendations Summary
1. **State Machine Separation**: Split into connection-level and per-camera state machines to support independent multi-camera flows and prevent global state blocking.
2. **Event Reliability**: Implement durable event queue or at-least-once semantics for critical events (`snapshot.requested`, `model.deployed`, connectivity events) with explicit ack/retry.
3. **Disconnect Handling**: Fix HTTPS/WireGuard disconnect to properly transition out of `model_deployed`/`frame_processing` states and clean up per-camera state.
4. **Capture Provenance**: Remove client-supplied `image_data` from screenshot API or require signed/verified capture tokens from CCTV service. Add authentication/authorization if web gateway is not strictly local.
5. **Concurrency Safety**: Add proper locking for `pendingSnapshotRequests` in `GetAllPendingSnapshotRequests`, protect `m.state.AIProcessingActive` access, and prevent duplicate frame processing goroutines per camera.
6. **Screenshot Media Handling**: Persist and return correct content-type/format, actually create and store thumbnails (or remove thumbnail fields entirely), avoid base64 thumbnails in list endpoints (make opt-in), implement real pagination.
7. **TLS Configuration**: Fix or document dev-mode TLS behavior. Either require certificates even in dev, or implement explicit insecure HTTP-only dev server with clear warnings.
8. **Frame Lifecycle Ownership**: Decide and document who owns frame cleanup (AI service vs AI gateway vs state manager), implement explicit retention policies, and prevent unbounded storage growth.
9. **Resource Management**: Wait for goroutines to finish before clearing maps in `stopAllFrameProcessing`, explicitly stop frame processing before service shutdown, add goroutine leak prevention in camera processing.
10. **Memory Management**: Implement batching or streaming for screenshot sync to avoid loading all images into memory. Add change detection to capability sync. Increase event bus buffer size or add backpressure mechanism.
11. **Error Recovery**: Add health monitoring for frame capture failures, implement retry/recovery for camera discovery errors, validate and alert on persistent processing failures.
12. **Configuration**: Make frame processing interval configurable, validate certificates at startup, validate configuration completeness, remove hardcoded fallback defaults (especially Edge ID).
13. **Testing**: Add tests for race conditions (`go test -race`), out-of-order events, PNG/JPEG content-type correctness, state history ordering, disconnect/reconnect scenarios, goroutine cleanup.
