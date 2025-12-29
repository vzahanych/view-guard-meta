# VM Gateway Refactoring Plan (Polishing and Alignment)

**Date**: 2025-12-28  
**Target Document**: `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`  
**Scope**: Polishing and alignment of `vm-gateway` package with production workflow requirements  
**Status**: vm-gateway has proper structure; this plan focuses on missing production features

---

## Executive Summary

The vm-gateway service has been refactored and follows a provider-agnostic architecture pattern. However, several production-critical features from `WORKFLOW_AND_BUSINESS_LOGIC.md` are missing or need enhancement:

1. **Certificate pinning, rotation, and revocation** - Not implemented
2. **Time synchronization checks** - Not implemented
3. **Idempotency keys** - Need verification and enhancement
4. **Rate limiting for inbound VM commands** - Not implemented
5. **HTTPS server ready before authentication** - Need verification
6. **Explicit timeouts** - Need verification and standardization
7. **Retry/backoff strategies** - Need verification against spec
8. **Event emission completeness** - Need verification
9. **Security enhancements** - Input validation, safe logging

This refactoring plan focuses on **polishing and alignment** rather than architectural changes.

---

## Epic 1: Certificate Security (Pinning, Rotation, Revocation)

**Goal**: Implement certificate pinning, rotation, and revocation as specified in WORKFLOW_AND_BUSINESS_LOGIC.md.

### Section 1.1: Certificate Pinning

#### Subsection 1.1.1: Certificate Pinning Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `CertificatePinningConfig` struct:
    - `VMCAFingerprint string` (SHA-256 fingerprint of VM CA root)
    - `EdgeCAFingerprint string` (SHA-256 fingerprint of Edge CA root)
    - `PinningEnabled bool` (default: true)
  - Add certificate pinning config to `HTTPSClientConfig` and `HTTPServerConfig`
  - Validate fingerprints on config load
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Certificate Pinning Implementation
- **Files**: `transport-service/http-impl/https-client-service/impl/cert_pinning.go` (new file)
- **Changes**:
  - Implement certificate pinning verification:
    - Extract certificate chain from TLS connection
    - Verify CA certificate fingerprint matches pinned value
    - Fail connection if fingerprint mismatch
  - Integrate with TLS handshake in HTTPS client
  - Log pinning failures with security alerts
- **Dependencies**: 1.1.1
- **Estimated Effort**: 2 days

#### Subsection 1.1.3: Server-Side Certificate Pinning
- **Files**: `transport-service/http-impl/https-server-service/impl/cert_pinning.go` (new file)
- **Changes**:
  - Implement server-side certificate pinning:
    - Verify VM client certificate CA fingerprint
    - Reject connections with unpinned certificates
  - Integrate with mTLS verification
- **Dependencies**: 1.1.2
- **Estimated Effort**: 1 day

### Section 1.2: Certificate Rotation

#### Subsection 1.2.1: Certificate Rotation Detection
- **Files**: `types/api.go`
- **Changes**:
  - Add `cert_rotation_scheduled_at` field to `SyncCapabilitiesResponse`:
    - `CertRotationScheduledAt *time.Time` (optional, signals upcoming rotation)
  - Add certificate rotation metadata to capabilities sync
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 1.2.2: Certificate Rotation Handler
- **Files**: `transport-service/http-impl/cert_rotation.go` (new file)
- **Changes**:
  - Implement certificate rotation handler:
    - Detect `cert_rotation_scheduled_at` in capabilities sync response
    - Download new CA cert from VM via authenticated endpoint: `GET /api/v1/edge/certificate-authority`
      - This endpoint is authenticated (requires current valid Edge certificate)
      - Edge presents its current certificate in mTLS handshake
      - VM validates Edge identity and returns new CA certificate
    - Validate new CA cert (must be signed by current CA or root CA)
    - Update trust store atomically (old CA remains trusted for 7 days grace period)
    - During grace period: accept either old or new VM cert
    - After grace period: remove old CA from trust store
  - Emit events: `certificate.rotation_scheduled`, `certificate.rotation_completed`
  - Log rotation events to audit-log
- **Dependencies**: 1.2.1
- **Estimated Effort**: 3 days

### Section 1.3: Certificate Revocation

#### Subsection 1.3.1: CRL/OCSP Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `CertificateRevocationConfig` struct:
    - `CRLEnabled bool` (enable CRL checking)
    - `CRLURL string` (CRL endpoint URL)
    - `OCSPEnabled bool` (enable OCSP checking)
    - `OCSPURL string` (OCSP endpoint URL)
    - `RevocationCacheTTL time.Duration` (default: 1 hour)
  - Add revocation config to `HTTPSClientConfig` and `HTTPSServerConfig`
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 1.3.2: Certificate Revocation Checking
- **Files**: `transport-service/http-impl/cert_revocation.go` (new file)
- **Changes**:
  - Implement certificate revocation checking:
    - Check CRL or OCSP on every authentication (configurable)
    - Cache revocation status (1 hour default)
    - Reject revoked certificates
    - Log revocation checks and failures
  - Integrate with TLS handshake
- **Dependencies**: 1.3.1
- **Estimated Effort**: 2 days

---

## Epic 2: Time Synchronization

**Goal**: Implement time synchronization checks as specified.

### Section 2.1: Time Synchronization Configuration

#### Subsection 2.1.1: Time Sync Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `TimeSyncConfig` struct:
    - `Enabled bool` (default: true)
    - `ToleranceMinutes int` (default: 5 minutes)
    - `CriticalDriftMinutes int` (default: 30 minutes)
  - Add time sync config to `VMGatewayConfig`
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 2.1.2: Time Synchronization Check
- **Files**: `transport-service/http-impl/time_sync.go` (new file)
- **Changes**:
  - Implement time synchronization check:
    - Extract VM clock time from mTLS handshake (certificate validity period comparison)
    - Compare Edge clock with VM clock
    - **Tolerance: ±5 minutes** (emit warning, continue operation)
      - If drift is within ±5 minutes: log warning, continue authentication
    - **Critical drift: >30 minutes** (FAIL authentication, emit CRITICAL alert, REQUIRE operator NTP fix)
      - If drift >30 minutes: fail authentication immediately
      - Emit critical alert: "Time synchronization critical: Edge clock drift >30 minutes"
      - Require operator to fix NTP configuration before authentication can succeed
    - Log time sync checks and failures
  - Integrate with authentication flow
- **Dependencies**: 2.1.1
- **Estimated Effort**: 2 days

---

## Epic 3: Idempotency Keys

**Goal**: Ensure all VM-facing requests include idempotency keys.

### Section 3.1: Idempotency Key Generation

#### Subsection 3.1.1: Idempotency Key Types
- **Files**: `types/api.go`
- **Changes**:
  - Add `IdempotencyKey` type (string, UUID format)
  - Add idempotency key fields to all VM request types:
    - `SyncDevicesRequest.IdempotencyKey`
    - `SyncDataUnitsRequest.IdempotencyKey`
    - `SyncAuditLogsRequest.IdempotencyKey`
    - `SendEvents` (batch idempotency key)
  - Document idempotency key format: `{EdgeID}-{operation}-{UUID}`
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Idempotency Key Generation
- **Files**: `transport-service/http-impl/idempotency.go` (new file)
- **Changes**:
  - Implement idempotency key generation:
    - Generate UUID for each VM request
    - Format: `{EdgeID}-{operation}-{UUID}`
    - Store idempotency keys in request metadata
  - Auto-generate idempotency keys if not provided
  - Log idempotency key usage
- **Dependencies**: 3.1.1
- **Estimated Effort**: 1 day

---

## Epic 4: Rate Limiting for Inbound VM Commands

**Goal**: Implement rate limiting for inbound VM commands to prevent resource exhaustion.

### Section 4.1: Rate Limiting Configuration

#### Subsection 4.1.1: Rate Limiting Config
- **Files**: `types/config.go`
- **Changes**:
  - Add `RateLimitConfig` struct:
    - `Enabled bool` (default: true)
    - `RequestsPerMinute int` (default: 100 per endpoint)
    - `BurstSize int` (default: 10)
    - `PerEndpointLimits map[string]int` (endpoint-specific limits)
  - Add rate limit config to `HTTPServerConfig`
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 4.1.2: Rate Limiter Implementation
- **Files**: `transport-service/http-impl/https-server-service/impl/rate_limiter.go` (new file)
- **Changes**:
  - Implement rate limiter for VM commands:
    - Track request counts per VM client (by certificate fingerprint)
    - Use token bucket algorithm
    - Support per-endpoint limits
    - Return HTTP 429 Too Many Requests with retry-after header
  - Integrate with HTTPS server middleware
  - Log rate limit violations
  - Emit events: `vm_gateway.rate_limit_exceeded`
- **Dependencies**: 4.1.1
- **Estimated Effort**: 2 days

---

## Epic 5: HTTPS Server Readiness Guarantee

**Goal**: Ensure HTTPS server is ready before authentication completes.

### Section 5.1: Server Readiness Verification

#### Subsection 5.1.1: Server Readiness Check
- **Files**: `transport-service/http-impl/https-server-service/impl/server_ready.go` (new file)
- **Changes**:
  - Implement server readiness check:
    - Verify HTTPS server is listening before starting authentication
    - Add readiness endpoint: `GET /api/health/ready`
    - Block authentication until server is ready
  - Integrate with gateway startup sequence
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Startup Sequence Enhancement
- **Files**: `vm_gateway_impl.go`
- **Changes**:
  - Update startup sequence:
    - Start HTTPS server first
    - Wait for server readiness
    - Then start authentication
  - Add timeout for server readiness (max 10s)
  - Log startup sequence steps
- **Dependencies**: 5.1.1
- **Estimated Effort**: 1 day

---

## Epic 6: Timeout Standardization

**Goal**: Ensure all operations have explicit timeouts as specified.

### Section 6.1: Timeout Configuration

#### Subsection 6.1.1: Timeout Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `TimeoutConfig` struct:
    - `TunnelEstablishmentTimeout time.Duration` (default: 30s)
    - `TransportEstablishmentTimeout time.Duration` (default: 30s)
    - `AuthenticationTimeout time.Duration` (default: 30s)
    - `VMAPIRequestTimeout time.Duration` (default: 30s)
    - `VMCommandProcessingTimeout time.Duration` (default: 10s)
  - Add timeout config to `VMGatewayConfig`
  - Validate timeout values (must be > 0)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 6.1.2: Timeout Enforcement
- **Files**: `transport-service/http-impl/` (multiple files)
- **Changes**:
  - Enforce timeouts in all operations:
    - Tunnel establishment: use `TunnelEstablishmentTimeout`
    - Transport establishment: use `TransportEstablishmentTimeout`
    - Authentication: use `AuthenticationTimeout`
    - VM API calls: use `VMAPIRequestTimeout`
    - VM command processing: use `VMCommandProcessingTimeout`
  - Use `context.WithTimeout` for all operations
  - Log timeout violations
- **Dependencies**: 6.1.1
- **Estimated Effort**: 2 days

---

## Epic 7: Retry and Backoff Strategies

**Goal**: Verify and enhance retry/backoff strategies to match spec.

### Section 7.1: Retry Configuration

#### Subsection 7.1.1: Retry Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `RetryConfig` struct:
    - `MaxRetries int` (default: 3)
    - `InitialBackoff time.Duration` (default: 1s)
    - `MaxBackoff time.Duration` (default: 60s)
    - `BackoffMultiplier float64` (default: 2.0)
    - `JitterEnabled bool` (default: true)
  - Add retry config to `VMGatewayConfig`
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 7.1.2: Retry Implementation
- **Files**: `transport-service/http-impl/retry.go` (new file)
- **Changes**:
  - Implement retry with exponential backoff:
    - Exponential backoff: 1s, 2s, 4s, 8s, max 60s
    - Add jitter to prevent thundering herd
    - Retry on transient failures (network errors, 5xx errors)
    - Don't retry on permanent failures (4xx errors, auth failures)
  - Apply retry to:
    - Authentication (exponential backoff: 10s, 20s, 40s, max 5min)
    - VM API calls (exponential backoff: 1s, 2s, 4s, 8s, max 60s)
  - Log retry attempts
- **Dependencies**: 7.1.1
- **Estimated Effort**: 2 days

---

## Epic 8: Event Emission Completeness

**Goal**: Verify all required events are emitted and enhance if needed.

### Section 8.1: Event Emission Verification

#### Subsection 8.1.1: Event Type Review
- **Files**: `transport-service/http-impl/` (multiple files)
- **Changes**:
  - Review event emission:
    - `network.tunnel.connecting` - emitted?
    - `network.tunnel.connected` - emitted?
    - `network.tunnel.disconnected` - emitted?
    - `network.tunnel.connection_error` - emitted?
    - `network.transport.connecting` - emitted?
    - `network.transport.connected` - emitted?
    - `network.transport.disconnected` - emitted?
    - `network.transport.connection_error` - emitted?
    - `edge.authenticated` - emitted?
    - `edge.capabilities_received` - emitted?
  - Add missing event emissions
  - Ensure events include required metadata (state, error details, timestamps)
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 8.1.2: Event Metadata Enhancement
- **Files**: `transport-service/http-impl/events.go` (new or enhance existing)
- **Changes**:
  - Enhance event metadata:
    - Include connection state transitions
    - Include error details for error events
    - Include timestamps
    - Include service names (tunnel, transport)
  - Standardize event format
- **Dependencies**: 8.1.1
- **Estimated Effort**: 1 day

---

## Epic 9: Input Validation and Security

**Goal**: Implement strict input validation and security enhancements.

### Section 9.1: Input Validation

#### Subsection 9.1.1: Request Validation
- **Files**: `transport-service/http-impl/https-server-service/impl/validation.go` (new file)
- **Changes**:
  - Implement input validation for VM commands:
    - Validate request payloads (JSON schema validation)
    - Validate query parameters
    - Validate path parameters
    - Sanitize inputs (prevent injection attacks)
  - Validate:
    - Capabilities sync requests
    - Data unit capture requests
    - Model deployment requests
  - Return HTTP 400 Bad Request for invalid inputs
  - Log validation failures
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 9.1.2: Resource Limits
- **Files**: `transport-service/http-impl/https-server-service/impl/resource_limits.go` (new file)
- **Changes**:
  - Implement resource limits:
    - Max request body size (configurable, default: 100MB for model deployments)
    - Max query parameter count
    - Max path parameter length
    - Max concurrent requests per VM client
  - Reject requests that exceed limits
  - Log resource limit violations
- **Dependencies**: 9.1.1
- **Estimated Effort**: 1 day

### Section 9.2: Safe Logging

#### Subsection 9.2.1: Logging Sanitization
- **Files**: `transport-service/http-impl/logging.go` (new file)
- **Changes**:
  - Implement safe logging:
    - Never log sensitive payloads (certificates, keys, model artifacts, datasets)
    - Log request metadata (method, path, client cert fingerprint) but not body
    - Log response metadata (status code, size) but not body
    - Sanitize error messages (don't leak internal details)
  - Implement logging middleware
  - Integrate with audit-log for security-sensitive operations
- **Dependencies**: None
- **Estimated Effort**: 1 day

---

## Epic 10: Health Monitoring Enhancements

**Goal**: Enhance health monitoring to include all required metrics.

### Section 10.1: Health Snapshot Enhancement

#### Subsection 10.1.1: Health Metrics
- **Files**: `vm_gateway.go`, `vm_gateway_impl.go`
- **Changes**:
  - Enhance `GatewayStatus` struct:
    - Add `CertificateRotationStatus` (rotation scheduled, in progress, completed)
    - Add `TimeSyncStatus` (synced, drift warning, drift critical)
    - Add `RateLimitStats` (requests per minute, violations)
    - Add `RetryStats` (retry counts, backoff durations)
    - Add `EventEmissionStats` (events emitted, failures)
  - Update `HealthSnapshot()` to include new metrics
- **Dependencies**: All previous epics
- **Estimated Effort**: 1 day

#### Subsection 10.1.2: Health Monitoring
- **Files**: `vm_gateway_impl.go`
- **Changes**:
  - Add periodic health monitoring:
    - Track connection uptime
    - Track authentication success/failure rates
    - Track VM API call success/failure rates
    - Track certificate rotation status
    - Track time sync status
  - Emit health events: `vm_gateway.health_degraded`, `vm_gateway.health_recovered`
- **Dependencies**: 10.1.1
- **Estimated Effort**: 1 day

---

## Epic 11: Documentation Updates

**Goal**: Update documentation to reflect production requirements.

### Section 11.1: Documentation Enhancement

#### Subsection 11.1.1: Package Documentation
- **Files**: `doc.go`
- **Changes**:
  - Update package documentation:
    - Document certificate pinning, rotation, revocation
    - Document time synchronization
    - Document idempotency keys
    - Document rate limiting
    - Document timeout configuration
    - Document retry/backoff strategies
    - Document security requirements
  - Add configuration examples
  - Add security best practices
- **Dependencies**: All previous epics
- **Estimated Effort**: 1 day

#### Subsection 11.1.2: API Documentation
- **Files**: All API files
- **Changes**:
  - Document idempotency key requirements
  - Document timeout requirements
  - Document retry behavior
  - Document error conditions
  - Add usage examples
- **Dependencies**: 11.1.1
- **Estimated Effort**: 1 day

---

## Implementation Order and Dependencies

### Phase 1: Security Foundation (Epics 1, 2, 9)
- **Duration**: ~3 weeks
- **Epics**: 1 (Certificate Security), 2 (Time Synchronization), 9 (Input Validation and Security)
- **Rationale**: Establishes security foundation

### Phase 2: Reliability Features (Epics 3, 4, 5, 6, 7)
- **Duration**: ~2.5 weeks
- **Epics**: 3 (Idempotency Keys), 4 (Rate Limiting), 5 (HTTPS Server Readiness), 6 (Timeout Standardization), 7 (Retry and Backoff)
- **Rationale**: Implements reliability features

### Phase 3: Observability and Polish (Epics 8, 10, 11)
- **Duration**: ~1.5 weeks
- **Epics**: 8 (Event Emission), 10 (Health Monitoring), 11 (Documentation)
- **Rationale**: Completes observability and documentation

**Total Estimated Duration**: ~7 weeks

---

## Migration Notes

### Breaking Changes
- Certificate pinning may reject connections if fingerprints don't match (requires config update)
- Time synchronization may reject authentication if clock drift >30 minutes (requires NTP config)
- Rate limiting may reject VM commands if limits exceeded (requires config tuning)
- Timeout changes may affect existing behavior (requires config review)

### Configuration Migration
- Add certificate pinning fingerprints to config
- Add time sync configuration
- Add rate limiting configuration
- Add timeout configuration
- Add retry configuration

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Update configuration documentation
- Gradual rollout to production with monitoring
- Monitor certificate rotation, time sync, rate limiting
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Certificate pinning implemented and tested
2. ✅ Certificate rotation implemented and tested
3. ✅ Certificate revocation implemented and tested
4. ✅ Time synchronization implemented and tested
5. ✅ Idempotency keys added to all VM requests
6. ✅ Rate limiting implemented and tested
7. ✅ HTTPS server ready before authentication verified
8. ✅ All operations have explicit timeouts
9. ✅ Retry/backoff strategies match spec
10. ✅ All required events are emitted
11. ✅ Input validation implemented
12. ✅ Safe logging implemented
13. ✅ Health monitoring enhanced
14. ✅ Documentation updated

---

## Notes

- **This is a polishing plan**: vm-gateway already has proper structure; focus on missing features
- **No architectural changes**: Keep existing provider-agnostic architecture
- **Backward compatibility**: Maintain compatibility where possible; document breaking changes
- **Security is critical**: Certificate pinning, rotation, revocation are production requirements
- **Reliability is critical**: Timeouts, retries, rate limiting are production requirements
- **Observability is critical**: Health monitoring and event emission are production requirements

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

