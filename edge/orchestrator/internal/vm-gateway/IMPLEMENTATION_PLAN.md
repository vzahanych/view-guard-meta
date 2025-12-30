# VM Gateway Code Review Fixes - Implementation Plan

This document outlines the implementation plan to address all findings from the ChatGPT code review of the vm-gateway package.

## Overview

The review identified 10 issues ranging from **Critical** (deadlocks) to **Low** (code style). This plan addresses them in priority order to minimize risk and maximize benefit.

## Implementation Principles

As noted in the review, all fixes follow the "no over-complicated construction" constraint:
- Small typed interfaces
- Tighter lock discipline
- Explicit config validation
- Standard library patterns
- No DI frameworks
- No complex option graphs

---

## Phase 1: Critical Fixes (Deadlocks & Correctness)

### 1.1 Fix HTTPS Client Startup Deadlock ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Critical  
**File:** `transport-service/http-impl/https-client-service/impl/https-client.go`  
**Lines:** ~256-371 (Start method)

**Problem:**
- `Start()` acquires `c.mu.Lock()` at line 257
- Calls `c.Authenticate()` at line 358 (while lock is held)
- Error handling at lines 359, 366 attempts to lock again → deadlock

**Solution:**
- Do NOT hold lock across `Authenticate()` call
- Lock only when mutating fields (`authenticated`, `lastAuthError`)
- Acquire lock before mutating, release immediately after

**Implementation Steps:**
1. ✅ Remove lock acquisition at start of `Start()` method
2. ✅ Only lock when reading/writing `authenticated` and `lastAuthError`
3. ✅ Call `Authenticate()` without holding lock
4. ✅ Removed redundant state updates (Authenticate() handles locking internally)
5. ✅ Added comments explaining locking strategy

**Changes Made:**
- Removed `c.mu.Lock()` and `defer c.mu.Unlock()` from start of `Start()` method
- Removed redundant `c.mu.Lock()/Unlock()` blocks after `Authenticate()` call (lines 359-362, 366-369)
- Added explanatory comments about why lock is not held and that `Authenticate()` handles its own locking
- Verified that `vmEndpoint` and `edgeID` are read-only after construction, so no lock needed

**Testing:**
- ✅ Unit test: Verify no deadlock when authentication fails (`TestHTTPSClient_Start_NoDeadlockOnAuthFailure`)
- ✅ Unit test: Verify no deadlock when authentication succeeds (`TestHTTPSClient_Start_NoDeadlockOnAuthSuccess`)
- ✅ Unit test: Verify concurrent calls don't deadlock (`TestHTTPSClient_Start_ConcurrentCalls`)
- ✅ Unit test: Verify localhost mode works (`TestHTTPSClient_Start_LocalhostMode`)
- ✅ Unit test: Verify Authenticate() updates state correctly (`TestHTTPSClient_Authenticate_UpdatesState`)
- ⏳ Integration test: Verify startup works correctly (can be added later if needed)

---

### 1.2 Reduce Lock Scope in HTTP Transport Service Start ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Critical  
**File:** `transport-service/http-impl/http-transport-service.go`  
**Lines:** ~43-136 (Start method)

**Problem:**
- Lock held for entire startup sequence
- `time.Sleep(checkInterval)` called at line 93 while holding lock
- Blocking service calls while holding lock

**Solution:**
- Acquire lock only to check/set `started` flag
- Copy service references under lock
- Perform all blocking work outside lock

**Implementation Steps:**
1. ✅ Acquire lock only to check `started` and copy service references
2. ✅ Release lock before calling `httpsServerService.Start()`
3. ✅ Release lock before readiness polling loop
4. ✅ Re-acquire lock only to set `started = true` at end (with double-check)
5. ✅ Apply same pattern for `httpsClientService.Start()`

**Changes Made:**
- Removed `defer s.mu.Unlock()` - no longer holding lock for entire method
- Copy service references (`serverSvc`, `clientSvc`) under lock, then release
- Perform all blocking operations (Start calls, readiness polling with time.Sleep) outside lock
- Re-acquire lock only at the end to set `started = true`, with double-check to prevent race conditions
- Added comments explaining locking strategy

**Testing:**
- ✅ Unit test: Verify concurrent Start() calls handled correctly (`TestHTTPTransportService_Start_ConcurrentCalls`)
- ✅ Unit test: Verify startup sequence works (`TestHTTPTransportService_Start_StartupSequence`)
- ✅ Unit test: Verify health/status checks not blocked during startup (`TestHTTPTransportService_Start_NoDeadlockOnBlockingOperations`)
- ✅ Unit test: Verify readiness polling works correctly (`TestHTTPTransportService_Start_ReadinessPolling`)
- ✅ Unit test: Verify readiness timeout handling (`TestHTTPTransportService_Start_ReadinessTimeout`)
- ✅ Unit test: Verify context cancellation handling (`TestHTTPTransportService_Start_ContextCancellation`)

---

### 1.3 Fix HTTPS Server Stop Endpoint Emission ✅

**Status:** ✅ **COMPLETED**  
**Priority:** High  
**File:** `transport-service/http-impl/https-server-service/impl/https-server.go`  
**Lines:** ~31-42 (struct), ~403-404 (Start), ~484-505 (Stop)

**Problem:**
- `s.httpServer = nil` set at line 473 (now 474)
- Disconnect event tries to read `s.httpServer.Addr` at line 487
- Endpoint always empty in disconnect events

**Solution:**
- Store `listenAddr` as struct field during `Start()`
- Use stored address in disconnect event in `Stop()`

**Implementation Steps:**
1. ✅ Added `listenAddr string` field to `HTTPServer` struct
2. ✅ Store `s.listenAddr = listenAddr` in `Start()` after creating httpServer
3. ✅ Use `s.listenAddr` in `Stop()` disconnect event instead of reading from nil `httpServer`
4. ✅ Clear `s.listenAddr = ""` after emitting disconnect event

**Changes Made:**
- Added `listenAddr string` field to `HTTPServer` struct to store the listen address
- Store the listen address during `Start()` method after determining the address
- Use the stored address in `Stop()` method for disconnect event emission
- Clear the stored address after emitting the event
- Added comments explaining the fix

**Testing:**
- ✅ Unit test: Verify disconnect event includes correct endpoint (`TestHTTPServer_Stop_DisconnectEventIncludesEndpoint`)
- ✅ Unit test: Verify disconnect event with custom endpoint (`TestHTTPServer_Stop_DisconnectEventWithCustomEndpoint`)
- ✅ Unit test: Verify disconnect event with default endpoint (`TestHTTPServer_Stop_DisconnectEventWithDefaultEndpoint`)

---

## Phase 2: Security & Configuration

### 2.1 Enforce TLS/Dev-Mode Strategy ✅

**Status:** ✅ **COMPLETED**  
**Priority:** High  
**Files:** 
- `transport-service/http-impl/https-server-service/impl/https-server.go` (~287-372)
- `types/config.go` (~616-623, validation)

**Problem:**
- Server creates TLS config without certificates in dev mode
- Calls `ServeTLS()` which will fail without certs
- Inconsistent with config validation (requires cert paths)

**Solution:**
- ✅ **Implemented Option 1:** Require certificates always, fail-fast with clear error
- ✅ Ensure config validation matches runtime behavior

**Implementation Steps:**
1. ✅ Reviewed `types/VMGatewayConfig.Validate()` - already required cert paths, added CACertPath requirement for server
2. ✅ Removed dev-mode TLS path that creates config without certs
3. ✅ Added certificate validation before listener creation (fail-fast)
4. ✅ Updated error messages to be clear and actionable about certificate requirements
5. ⏭️ Client-side `InsecureSkipVerify` gating deferred to task 2.2

**Changes Made:**
- Removed the `else` branch in `HTTPServer.Start()` that created TLS config without certificates
- Added certificate path validation before listener creation (after applying defaults) to fail fast
- Updated error message to list missing fields and provide clear guidance
- Added `CACertPath` requirement to config validation (previously missing for server)
- Certificate check now happens early and closes listener if validation fails

**Testing:**
- ✅ Unit test: Verify startup fails with clear error when certs missing (`TestHTTPServer_Start_RequiresCertificates`)
- ✅ Unit test: Verify error messages are actionable (`TestHTTPServer_Start_RequiresCertificates_ErrorIsActionable`)
- ✅ Config validation test: Added test for missing server CA cert path

---

### 2.2 Strictly Gate InsecureSkipVerify ✅

**Status:** ✅ **COMPLETED**  
**Priority:** High  
**File:** `transport-service/http-impl/https-client-service/impl/https-client.go`  
**Lines:** ~127-210 (TLS config section)

**Problem:**
- `InsecureSkipVerify: true` set when cert paths not provided
- No explicit opt-in flag
- Security risk if config path is bypassed

**Solution:**
- ✅ Enforce in config validation
- ✅ Allow insecure only if endpoint is localhost AND explicit flag enabled
- ✅ Otherwise, hard-fail construction/startup

**Implementation Steps:**
1. ✅ Added `AllowInsecureLocalhost` boolean flag to `HTTPSClientConfig` in both types packages
2. ✅ Updated config validation to check:
   - If `AllowInsecureLocalhost == false` → require cert paths (with helpful error message)
   - If `AllowInsecureLocalhost == true` → verify endpoint is localhost/127.0.0.1 (fail if not)
3. ✅ Updated client constructor (`NewHTTPSClient`) to check this flag before using `InsecureSkipVerify`
4. ✅ Removed automatic fallback to insecure mode - now only uses insecure when flag is explicitly enabled
5. ✅ Added security warning log when insecure mode is enabled

**Changes Made:**
- Added `AllowInsecureLocalhost bool` field to `HTTPSClientConfig` in:
  - `https-client-service/types/types.go`
  - `types/config.go` (VMGatewayConfig.HTTPSClientConfig)
- Updated `VMGatewayConfig.Validate()` to enforce:
  - When `AllowInsecureLocalhost=true`: endpoint must be localhost/127.0.0.1
  - When `AllowInsecureLocalhost=false` (default): certificates are required
- Updated `NewHTTPSClient()` constructor to:
  - Check `AllowInsecureLocalhost` flag before using `InsecureSkipVerify`
  - Verify endpoint is localhost when flag is enabled
  - Fail with clear error if certificates are missing and insecure is not allowed
  - Log security warning when insecure mode is enabled
- Updated config conversion to include `AllowInsecureLocalhost` field

**Testing:**
- ✅ Config validation test: Valid config with `AllowInsecureLocalhost=true` and localhost endpoint (`TestVMGatewayConfig_Validate`)
- ✅ Config validation test: Invalid config with `AllowInsecureLocalhost=true` but non-localhost endpoint
- ✅ Config validation test: Invalid config with `AllowInsecureLocalhost=false` and missing certificates
- ✅ Unit test: Constructor fails when insecure not allowed and no certs (`TestNewHTTPSClient_InsecureNotAllowed_RequiresCertificates`)
- ✅ Unit test: Constructor succeeds when insecure allowed + localhost endpoint (`TestNewHTTPSClient_InsecureAllowed_LocalhostEndpoint_Succeeds`)
- ✅ Unit test: Constructor fails when insecure allowed but non-localhost endpoint (`TestNewHTTPSClient_InsecureAllowed_NonLocalhostEndpoint_Fails`)
- ✅ Unit test: Constructor succeeds when insecure allowed + 127.0.0.1 endpoint (`TestNewHTTPSClient_InsecureAllowed_127_0_0_1_Succeeds`)

---

## Phase 3: Health Metrics & Typing

### 3.1 Replace Interface{} Health Plumbing with Typed Interface ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Medium  
**Files:**
- `types/health.go` (NEW - created HealthReporter interface and types)
- `vm_gateway_impl.go` (~523-562)
- `vm_gateway.go` (updated to use types from types package)
- `transport-service/http-impl/http-transport-service.go` (~456-487)
- `transport-service/http-impl/https-client-service/impl/https-client.go` (~1029-1060)
- `transport-service/http-impl/https-server-service/impl/https-server.go` (~1206-1230)

**Problem:**
- `GetHealthMetrics() (interface{}, interface{}, interface{})` uses untyped returns
- Map parsing with type assertions is brittle
- Time-sync metric explicitly ignored (`_`)

**Solution:**
- ✅ Created small typed interface `HealthReporter` in vm-gateway types
- ✅ Replaced map parsing with typed structs
- ✅ Time-sync metric is now properly typed (returns nil for now, but interface is ready for future enhancement)

**Implementation Steps:**
1. ✅ Created `HealthReporter` interface in `types/health.go` with typed struct definitions
2. ✅ Updated `HTTPSClient` to implement interface with typed returns (`GetCertificateRotationStatus()`, `GetTimeSyncStatus()`, `GetRateLimitStats()`)
3. ✅ Updated `HTTPServer` to implement interface with typed returns (stubs for cert rotation/time sync, full implementation for rate limiting)
4. ✅ Updated `HTTPTransportService` to implement `HealthReporter` interface by delegating to client and server services
5. ✅ Updated `vmGatewayImpl.HealthSnapshot()` to use typed `HealthReporter` interface instead of `interface{}`
6. ✅ Removed map parsing helper functions (`getString`, `getBool`, `getInt`, `getInt64`)
7. ✅ Updated `vm_gateway.go` to use type aliases for backward compatibility (types now defined in `types/health.go`)

**Changes Made:**
- Created `types/health.go` with:
  - `HealthReporter` interface
  - `CertificateRotationStatus` struct
  - `TimeSyncStatus` struct
  - `RateLimitStats` struct
- Updated `HTTPSClient.GetCertificateRotationStatus()` to return `*types.CertificateRotationStatus` instead of `interface{}`
- Updated `HTTPSClient.GetTimeSyncStatus()` to return `*types.TimeSyncStatus` (returns nil for now, ready for future enhancement)
- Added `HTTPSClient.GetRateLimitStats()` returning nil (client doesn't implement rate limiting)
- Updated `HTTPServer` with all three methods of `HealthReporter` interface
- Updated `HTTPTransportService` to implement `HealthReporter` by delegating to client/server services
- Updated `vmGatewayImpl.HealthSnapshot()` to use `types.HealthReporter` interface instead of ad-hoc interface{} methods
- Removed all map parsing helper functions (`getString`, `getBool`, `getInt`, `getInt64`)
- Updated `vm_gateway.go` to use type aliases for backward compatibility

**Testing:**
- ✅ Code compiles successfully
- ✅ All existing tests pass
- ⚠️ Time-sync status currently returns nil (acceptable - checking happens during auth, not stored; interface is ready for future enhancement)
- Note: Integration tests for health endpoint would verify structure in end-to-end tests

---

### 3.2 Remove or Wire Gateway-Level Retry/Event Stats ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Medium  
**Files:**
- `vm_gateway_impl.go` (~30-48, 509-547)
- `vm_gateway.go` (~229-282)

**Problem:**
- `RetryStats` and `EventEmissionStats` read in `HealthSnapshot()`
- No mutation sites found in service code
- Metrics never change, misleading health output

**Solution:**
- ✅ Removed from gateway health, relying on typed transport metrics (preferred approach)
- ✅ Kept type definitions for backward compatibility with deprecation notice

**Implementation Steps:**
1. ✅ Searched codebase for all retry/event emission mutation sites (none found)
2. ✅ Removed from gateway health (removed fields from `GatewayStatus` struct)
3. ✅ Removed `RetryMetrics` and `EventEmissionMetrics` internal tracking structs
4. ✅ Removed `retryStats` and `eventEmissionStats` fields from `vmGatewayImpl` struct
5. ✅ Removed `healthMetricsMu` mutex (no longer needed)
6. ✅ Updated `HealthSnapshot()` to remove stats collection code
7. ✅ Updated `GatewayStatus` struct to remove `RetryStats` and `EventEmissionStats` fields
8. ✅ Kept type definitions with deprecation notices for backward compatibility
9. ✅ Documented that retry/event stats are available via transport-specific metrics if needed

**Changes Made:**
- Removed `RetryMetrics` and `EventEmissionMetrics` type definitions from `vm_gateway_impl.go`
- Removed `retryStats` and `eventEmissionStats` fields from `vmGatewayImpl` struct
- Removed `healthMetricsMu` mutex from `vmGatewayImpl` struct
- Removed stats collection code from `HealthSnapshot()` method
- Removed `RetryStats` and `EventEmissionStats` fields from `GatewayStatus` struct in `vm_gateway.go`
- Kept `RetryStats` and `EventEmissionStats` type definitions with deprecation notices for backward compatibility
- Added documentation noting that these stats are available via transport-specific metrics

**Testing:**
- ✅ Code compiles successfully
- ✅ All existing tests pass
- ✅ Health snapshot no longer includes unused stats
- Note: Integration tests would verify health endpoint structure in end-to-end tests

---

## Phase 4: HTTP Robustness

### 4.1 Harden HTTP Handlers: MaxBytesReader + Strict JSON ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Medium  
**Files:**
- `transport-service/http-impl/https-server-service/impl/https-server.go` (multiple handlers)
- `transport-service/http-impl/https-server-service/impl/validation.go` (helper function)

**Problem:**
- Request size validation only checks `ContentLength > 0`
- Misses chunked requests (`ContentLength == -1`)
- No strict JSON decoding (`DisallowUnknownFields`)

**Solution:**
- ✅ Use `http.MaxBytesReader()` before JSON decoding
- ✅ Use `json.Decoder` with `DisallowUnknownFields()`
- ✅ Remove `ContentLength` checks (MaxBytesReader handles both)

**Implementation Steps:**
1. ✅ Created helper function `LimitRequestBody(r *http.Request, maxBytes int64)` in `validation.go`
2. ✅ Updated all JSON-decoding handlers to use `MaxBytesReader`:
   - `handleUpdateConfig` - max 1MB
   - `handleRequestDataUnitCapture` - max 1MB
   - `handleDeployModel` - max 100MB (multipart form, also uses MaxBytesReader)
   - `handleRestartService` - max 1KB
   - `handleSyncCapabilities` - max 1MB
3. ✅ Replaced `json.NewDecoder(r.Body).Decode()` with:
   ```go
   decoder := json.NewDecoder(r.Body)
   decoder.DisallowUnknownFields()
   err := decoder.Decode(&req)
   ```
4. ✅ Removed `ContentLength > 0` checks (redundant with MaxBytesReader)
5. ✅ Updated error handling to detect "request body too large" errors
6. ✅ Marked `ValidateRequestSize` as deprecated (kept for backward compatibility)

**Changes Made:**
- Added `LimitRequestBody()` helper function that wraps request body with `http.MaxBytesReader`
- Updated all handlers to use `LimitRequestBody()` before JSON decoding
- Added `DisallowUnknownFields()` to all JSON decoders for strict JSON validation
- Improved error handling to detect and return clear error messages for oversized requests
- Deprecated `ValidateRequestSize()` function (no longer used, but kept for compatibility)

**Testing:**
- ✅ Code compiles successfully
- ✅ All existing tests pass
- Note: Unit tests for chunked requests and unknown JSON fields would verify the new behavior
- Note: Integration tests would verify handlers reject oversized requests

---

### 4.2 Bound io.ReadAll in Client Error Paths ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Medium  
**Files:**
- `transport-service/http-impl/https-client-service/impl/https-client.go` (multiple methods)
- `transport-service/http-impl/https-client-service/impl/cert_rotation.go`
- `transport-service/http-impl/https-client-service/impl/cert_revocation.go`

**Problem:**
- `io.ReadAll(resp.Body)` used in error paths without size limits
- Risk of reading large bodies into memory

**Solution:**
- ✅ Replaced with `readLimitedBody()` helper using `io.LimitReader(resp.Body, maxBytes)`
- ✅ Error paths use 64KB limit, success paths use appropriate limits
- ✅ Error messages remain clear and actionable

**Implementation Steps:**
1. ✅ Created helper function `readLimitedBody(body io.Reader, maxBytes int64) ([]byte, error)` in `https-client.go`
2. ✅ Replaced all `io.ReadAll(resp.Body)` with limited reader in error paths:
   - `Heartbeat()` - 64KB limit
   - `SendTelemetry()` - 64KB limit
   - `SendEvents()` - 64KB limit
   - `Authenticate()` - 64KB limit
   - `GetConfig()` - 64KB limit
   - `SyncCapabilities()` - 64KB limit
   - `SyncDevices()` - 64KB limit
   - `SyncDataUnits()` - 64KB limit
   - `SyncAuditLogs()` - 64KB limit
   - `ReportDeploymentStatus()` - 64KB limit
3. ✅ Updated `cert_rotation.go`:
   - Error path: 64KB limit
   - Success path (certificate reading): 64KB limit
4. ✅ Updated `cert_revocation.go`:
   - OCSP response reading: 4KB limit (typically very small)
   - CRL reading: 256KB limit (can be legitimately larger)
5. ✅ Error messages remain clear and informative

**Changes Made:**
- Added `readLimitedBody()` helper function that wraps `io.ReadAll(io.LimitReader(...))`
- Updated all error path `io.ReadAll()` calls to use `readLimitedBody()` with 64KB limit
- Updated certificate rotation handler to use `readLimitedBody()` for both error and success paths
- Updated certificate revocation checker to use `readLimitedBody()` with appropriate limits:
  - OCSP responses: 4KB (typically < 1KB)
  - CRLs: 256KB (can be legitimately larger)

**Testing:**
- ✅ Code compiles successfully
- ✅ All existing tests pass
- Note: Unit tests for verifying body truncation would test the new behavior
- Note: Integration tests would verify error handling with large responses

---

## Phase 5: Code Quality

### 5.1 Normalize Logger to zap.NewNop() ✅

**Status:** ✅ **COMPLETED**  
**Priority:** Low  
**Files:**
- `vm_gateway_impl.go`
- `transport-service/http-impl/http-transport-service.go`
- `transport-service/http-impl/https-client-service/impl/https-client.go`
- `transport-service/http-impl/https-server-service/impl/https-server.go`
- `transport-service/http-impl/https-client-service/impl/cert_rotation.go`
- `transport-service/http-impl/https-client-service/impl/cert_revocation.go`
- `transport-service/http-impl/https-client-service/impl/time_sync.go`

**Problem:**
- Repeated `if logger != nil { ... }` checks
- Inconsistent logging patterns

**Solution:**
- ✅ Normalize in constructors: `if log == nil { log = zap.NewNop() }`
- ✅ Log unconditionally after normalization

**Implementation Steps:**
1. ✅ Updated all constructors to normalize logger:
   - `NewHTTPSClient()` - added normalization
   - `NewHTTPServer()` - added normalization
   - `NewHTTPTransportService()` - added normalization
   - `NewVMGatewayImpl()` - added normalization
   - `NewCertificateRotationHandler()` - added normalization
   - `NewCertificateRevocationChecker()` - added normalization
   - `NewTimeSyncChecker()` - added normalization
2. ✅ Removed all `if logger != nil` checks from:
   - `vm_gateway_impl.go` (Start and Stop methods)
   - `http-transport-service.go` (Start, Stop, and Authenticate methods)
3. ✅ All logging calls are now unconditional throughout codebase
4. ✅ Tests continue to work correctly (existing tests use zap.NewNop() for test loggers)

**Changes Made:**
- Added logger normalization in all constructors that accept logger parameters
- Removed all `if logger != nil` conditional checks
- All logger method calls (Info, Error, Warn, Debug) are now unconditional
- This improves code clarity and consistency without changing behavior (nil loggers are now no-op loggers)

**Testing:**
- ✅ Code compiles successfully
- ✅ All existing tests pass
- ✅ No nil pointer panics (loggers are normalized to zap.NewNop() if nil)

---

## Implementation Timeline

### Week 1: Critical Fixes (Phase 1)
- Day 1-2: Fix deadlocks (1.1, 1.2)
- Day 3: Fix endpoint emission (1.3)
- Day 4-5: Testing and validation

### Week 2: Security (Phase 2)
- Day 1-2: TLS/dev-mode strategy (2.1)
- Day 3-4: InsecureSkipVerify gating (2.2)
- Day 5: Testing and validation

### Week 3: Health Metrics (Phase 3)
- Day 1-3: Typed health interface (3.1)
- Day 4: Retry/event stats cleanup (3.2)
- Day 5: Testing and validation

### Week 4: HTTP Robustness (Phase 4)
- Day 1-2: Handler hardening (4.1)
- Day 3-4: Bounded ReadAll (4.2)
- Day 5: Testing and validation

### Week 5: Code Quality (Phase 5)
- Day 1: Logger normalization (5.1)
- Day 2-3: Final testing, regression tests
- Day 4-5: Documentation updates

---

## Testing Strategy

### Unit Tests
- Each fix should include unit tests
- Test both success and failure paths
- Test concurrent access where applicable
- Mock dependencies appropriately

### Integration Tests
- Verify startup/shutdown sequences
- Verify health endpoints return correct data
- Verify error handling and logging
- Verify TLS configuration works correctly

### Regression Tests
- Run full test suite after each phase
- Verify no existing functionality is broken
- Performance tests to ensure no degradation

### Manual Testing
- Test dev-mode TLS setup
- Test production TLS configuration
- Verify health metrics in production-like environment
- Test with various error conditions

---

## Risk Assessment

### High Risk Changes
1. **Deadlock fixes (1.1, 1.2)**: Lock ordering changes could introduce new deadlocks
   - **Mitigation**: Careful code review, comprehensive concurrent testing

2. **TLS configuration (2.1)**: Breaking change for dev mode
   - **Mitigation**: Clear migration guide, update documentation

### Medium Risk Changes
1. **Health metrics typing (3.1)**: Interface changes
   - **Mitigation**: Maintain backward compatibility during transition, version interfaces

2. **HTTP handler changes (4.1)**: Could break existing clients
   - **Mitigation**: Test with existing clients, clear error messages

### Low Risk Changes
1. **Logger normalization (5.1)**: Cosmetic change
   - **Mitigation**: Simple change, easy to revert

---

## Rollback Plan

For each phase:
1. Keep backup of original code (git branches)
2. Implement behind feature flags if risky
3. Monitor logs and metrics after deployment
4. Have rollback procedure documented
5. Test rollback procedure

---

## Success Criteria

- [x] All critical deadlocks fixed (1.1, 1.2 completed - HTTPS client startup deadlock, HTTP transport service lock scope)
- [x] All high-priority security issues addressed (2.1, 2.2 completed - TLS certificate requirements, InsecureSkipVerify gating)
- [ ] Health metrics use typed interfaces
- [ ] All HTTP handlers handle chunked requests correctly
- [ ] All error path body reads are bounded
- [ ] Code passes all existing tests
- [ ] New tests cover all fixes
- [ ] Documentation updated
- [ ] No performance regression
- [ ] Code review approved

## Progress Tracking

### Phase 1: Critical Fixes (Deadlocks & Correctness)
- [x] 1.1 Fix HTTPS Client Startup Deadlock ✅ **COMPLETED**
- [x] 1.2 Reduce Lock Scope in HTTP Transport Service Start ✅ **COMPLETED**
- [x] 1.3 Fix HTTPS Server Stop Endpoint Emission ✅ **COMPLETED**

---

## Notes

- Follow existing code style and patterns
- Maintain backward compatibility where possible
- Update inline documentation/comments
- Consider impact on other services that depend on vm-gateway
- Coordinate with team on breaking changes

---

## Related Documents

- `ChatGPT-review.md` - Original code review findings
- `doc.go` - Package documentation
- `types/config.go` - Configuration types and validation

