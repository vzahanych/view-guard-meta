# Web Gateway Refactoring Plan

**Date**: 2025-12-28  
**Target Documents**: 
- `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`
- `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- `edge/orchestrator/internal/vm-gateway/doc.go` (architectural pattern reference)

**Scope**: Complete refactoring of `web-gateway` package to align with production workflow requirements and follow vm-gateway architectural pattern  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the Web Gateway service implementation into full compliance with the production workflow specification and aligns it with the vm-gateway architectural pattern. The current implementation is camera-centric, lacks production features (rate limiting, comprehensive authZ, health monitoring), and doesn't follow the provider-agnostic architecture pattern.

**Key Transformation Areas**:
1. **Provider-agnostic architecture**: Follow vm-gateway pattern with interface, types, and implementation separation
2. **Device-agnostic design**: Replace camera-centric APIs with device-agnostic APIs
3. **Rate limiting**: Implement rate limiting per client IP (1000 req/min default, configurable)
4. **Enhanced authentication/authorization**: Strict authZ for dataset/event access (principle of least privilege)
5. **Health monitoring**: Add health snapshot API and operational metrics
6. **API surface updates**: Device-agnostic endpoints, data unit management, ML lifecycle visibility

---

## Epic 1: Provider-Agnostic Architecture (Following vm-gateway Pattern)

**Goal**: Restructure the codebase to follow the vm-gateway architectural pattern with clear separation of concerns.

### Section 1.1: Interface and Types Separation

#### Subsection 1.1.1: Main Interface File
- **Files**: `web_gateway.go` (already exists, enhance)
- **Changes**:
  - Enhance `WebGateway` interface:
    - Keep lifecycle methods (`Start`, `Stop`, `Name`)
    - Add `HealthSnapshot() WebGatewayHealth` method
    - Add `SetStateManager(stateManager interface{})` method (for workflow commands)
  - Define sentinel errors (similar to vm-gateway):
    - `ErrNotInitialized`
    - `ErrAlreadyStarted`
    - `ErrRateLimitExceeded` (when rate limit is exceeded)
    - `ErrUnauthorized` (authentication failed)
    - `ErrForbidden` (authorization failed)
  - Define factory function `NewWebGateway(...)`
  - Define provider function `WebGatewayProvider(...)` with fx lifecycle
  - Add comprehensive package documentation (similar to vm-gateway/doc.go)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Types Package Structure
- **Files**: `types/` directory (already exists, enhance)
- **Changes**:
  - Move all configuration types to `types/config.go`
  - Create `types/health.go` for health-related types:
    - `HealthStatus` enum (healthy, degraded, unhealthy)
    - `WebGatewayHealth` struct (status, metrics, rate limit stats)
  - Create `types/auth.go` for authentication/authorization types:
    - `AuthMethod` enum (api_key, token, certificate)
    - `Permission` enum (read_devices, write_devices, read_datasets, write_datasets, read_events, admin)
    - `UserContext` struct (user ID, permissions, IP address)
  - Create `types/rate_limit.go` for rate limiting types:
    - `RateLimitConfig` struct (requests per minute, burst size)
    - `RateLimitStatus` struct (current rate, limit, remaining)
  - Create `types/provider.go` for provider interface:
    - `WebServerProvider` interface (provider-agnostic web server operations)
    - Provider-specific configuration types
  - Create `types/errors.go` for error types
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Implementation Package Structure
- **Files**: `impl/` directory (already exists, enhance)
- **Changes**:
  - Create `impl/web_gateway_impl.go` (main implementation, already exists, enhance)
  - Create provider-specific implementations:
    - `impl/gin/gin_provider.go` (Gin framework implementation, extract from current)
    - `impl/echo/echo_provider.go` (future: Echo framework implementation)
  - Each provider implements `types.WebServerProvider` interface
  - Main implementation delegates to provider
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

### Section 1.2: Lifecycle Management

#### Subsection 1.2.1: Service Lifecycle
- **Files**: `impl/web_gateway_impl.go`
- **Changes**:
  - Implement `Start(ctx)` method:
    - Initialize provider (web server)
    - Verify dependencies (meta-storage, object-storage, state-manager)
    - Start background tasks (health monitoring, metrics collection)
    - Setup routes and middleware
  - Implement `Stop(ctx)` method:
    - Stop background tasks gracefully
    - Graceful shutdown of web server (wait for in-flight requests, max timeout)
    - Close provider connections
  - Follow vm-gateway pattern: service owns lifecycle of sub-components
- **Dependencies**: 1.1.3
- **Estimated Effort**: 1 day

#### Subsection 1.2.2: Provider Lifecycle
- **Files**: `impl/gin/gin_provider.go`
- **Changes**:
  - Implement provider-specific initialization
  - Implement provider-specific cleanup
  - Providers do NOT register their own fx.Lifecycle hooks (gateway-owned lifecycle pattern)
- **Dependencies**: 1.2.1
- **Estimated Effort**: 1 day

---

## Epic 2: Device-Agnostic Architecture

**Goal**: Transform the codebase from camera-centric to device-agnostic terminology and types.

### Section 2.1: API Endpoint Refactoring

#### Subsection 2.1.1: Device Endpoints
- **Files**: `impl/handlers_devices.go` (rename from `handlers_cameras.go`)
- **Changes**:
  - Replace camera endpoints with device endpoints:
    - `GET /api/cameras` → `GET /api/devices`
    - `GET /api/cameras/:id` → `GET /api/devices/:id`
    - `POST /api/cameras` → `POST /api/devices`
    - `PUT /api/cameras/:id` → `PUT /api/devices/:id`
    - `DELETE /api/cameras/:id` → `DELETE /api/devices/:id`
  - Update handlers to use `DeviceID` and `DeviceType`
  - Update request/response types to be device-agnostic
  - Keep backward compatibility endpoints (deprecated, redirect to new endpoints)
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 2.1.2: Data Unit Endpoints
- **Files**: `impl/handlers_data_units.go` (rename from `handlers_screenshots.go`)
- **Changes**:
  - Replace screenshot endpoints with data unit endpoints:
    - `GET /api/screenshots` → `GET /api/data-units`
    - `GET /api/screenshots/:id` → `GET /api/data-units/:id`
    - `POST /api/screenshots/:id/label` → `POST /api/data-units/:id/label`
    - `DELETE /api/screenshots/:id` → `DELETE /api/data-units/:id`
  - Update handlers to support all data unit types (images, sensor readings, audio samples)
  - Update request/response types to be device-agnostic
  - Support device-agnostic filtering (by DeviceID, DeviceType, DataType)
  - **Legacy endpoint deprecation**:
    - Legacy endpoints (`/api/screenshots`) are deprecated and will be removed in version 2.0
    - Return deprecation warning header: `Warning: 299 - "Endpoint deprecated, use /api/data-units instead"`
    - Support backward compatibility endpoints (deprecated, redirect to new endpoints) until version 2.0
- **Dependencies**: 2.1.1
- **Estimated Effort**: 2 days

#### Subsection 2.1.3: Dataset Endpoints
- **Files**: `impl/handlers_datasets.go` (new file)
- **Changes**:
  - Add dataset management endpoints:
    - `GET /api/devices/:deviceId/datasets` (list datasets for device)
    - `GET /api/datasets/:datasetId` (get dataset details)
    - `POST /api/datasets/:datasetId/ready` (mark dataset as ready for upload)
    - `GET /api/datasets/:datasetId/status` (get dataset upload status)
  - Integrate with state manager for dataset workflow
  - Support device-agnostic dataset operations
- **Dependencies**: 2.1.2
- **Estimated Effort**: 2 days

#### Subsection 2.1.4: ML Lifecycle Endpoints
- **Files**: `impl/handlers_ml_lifecycle.go` (new file)
- **Changes**:
  - Add ML lifecycle visibility endpoints:
    - `GET /api/devices/:deviceId/ml-lifecycle` (get ML lifecycle state for device)
    - `GET /api/ml-lifecycle` (list all devices with ML lifecycle states)
    - `GET /api/ml-lifecycle/stats` (aggregate ML lifecycle statistics)
  - Display ML lifecycle states (assigned, awaiting dataset, model stored, inference active, etc.)
  - Integrate with state manager for ML lifecycle queries
- **Dependencies**: 2.1.3
- **Estimated Effort**: 2 days

### Section 2.2: Handler Refactoring

#### Subsection 2.2.1: Device Handler Updates
- **Files**: `impl/handlers_devices.go`
- **Changes**:
  - Update `handleListDevices`:
    - Query IoT service for devices (not just cameras)
    - Support device type filtering
    - Return device-agnostic response format
  - Update `handleGetDevice`:
    - Query IoT service for device by DeviceID
    - Return device-agnostic metadata
  - Update `handleAddDevice`:
    - Support all device types (not just cameras)
    - Validate device type and capabilities
  - Remove CCTV service dependency (use IoT service instead)
- **Dependencies**: 2.1.1
- **Estimated Effort**: 2 days

#### Subsection 2.2.2: Data Unit Handler Updates
- **Files**: `impl/handlers_data_units.go`
- **Changes**:
  - Update `handleListDataUnits`:
    - Query meta-storage for data units (not just screenshots)
    - Support filtering by DeviceID, DeviceType, DataType
    - Return device-agnostic response format
  - Update `handleLabelDataUnit`:
    - Support labeling for all data unit types
    - Validate labels against VM-requested labels
    - Integrate with state manager for dataset workflow
  - Remove camera-specific logic
- **Dependencies**: 2.1.2
- **Estimated Effort**: 2 days

---

## Epic 3: Rate Limiting

**Goal**: Implement rate limiting per client IP as specified (1000 req/min default, configurable).

### Section 3.1: Rate Limiting Configuration

#### Subsection 3.1.1: Rate Limiting Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `RateLimitConfig` struct:
    - `Enabled bool` (default: true)
    - `RequestsPerMinute int` (default: 1000 per IP)
    - `BurstSize int` (default: 100 - allow burst of N requests)
    - `WhitelistIPs []string` (IPs exempt from rate limiting)
    - `PerEndpointLimits map[string]int` (endpoint-specific limits)
  - Add rate limiting configuration to `WebGatewayConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Rate Limiter Implementation
- **Files**: `impl/rate_limiter.go` (new file)
- **Changes**:
  - Implement `RateLimiter` struct:
    - Track request counts per IP (in-memory with TTL)
    - Use token bucket algorithm (or sliding window)
    - Support per-endpoint limits
  - Implement `CheckLimit(ctx, clientIP, endpoint) (allowed bool, retryAfter time.Duration, err error)`:
    - Check if request is allowed
    - Return retry-after hint if rate limited
    - Return `ErrRateLimitExceeded` if limit exceeded
  - Implement rate limit middleware:
    - Extract client IP from request, considering proxy headers:
      - Check `X-Forwarded-For` header (if present and from trusted proxy)
      - Check `X-Real-IP` header (if present and from trusted proxy)
      - Fall back to direct connection IP if no trusted proxy headers
      - Validate proxy headers: only trust if request comes from trusted proxy IPs (configurable whitelist)
    - Check rate limit before processing request
    - Return HTTP 429 Too Many Requests with retry-after header
    - Log rate limit violations
- **Dependencies**: 3.1.1, Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 3.1.3: Rate Limiting Metrics
- **Files**: `impl/rate_limiter.go`
- **Changes**:
  - Track rate limiting metrics:
    - Requests per IP over time
    - Rate limit violations per IP
    - Rate limit violations per endpoint
  - Expose metrics via health snapshot
  - Emit events: `web_gateway.rate_limit_exceeded` (for monitoring)
- **Dependencies**: 3.1.2
- **Estimated Effort**: 1 day

---

## Epic 4: Enhanced Authentication and Authorization

**Goal**: Implement strict authZ for dataset/event access (principle of least privilege).

### Section 4.1: Authentication Enhancement

#### Subsection 4.1.1: Authentication Methods
- **Files**: `types/auth.go`
- **Changes**:
  - Define `AuthMethod` enum:
    - `AuthMethodAPIKey` (Bearer token)
    - `AuthMethodToken` (JWT token)
    - `AuthMethodCertificate` (mTLS certificate)
  - Define `AuthConfig` struct:
    - `Enabled bool`
    - `Methods []AuthMethod` (supported methods)
    - `APIKey string` (for API key auth)
    - `TokenIssuer string` (for JWT auth)
    - `CertificateCA string` (for mTLS auth)
  - Update `WebGatewayConfig` to include enhanced auth config
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.1.2: Authentication Middleware
- **Files**: `impl/auth_middleware.go` (new file)
- **Changes**:
  - Implement authentication middleware:
    - Extract credentials from request (API key, token, certificate)
    - Validate credentials
    - Create `UserContext` with user ID and permissions
    - Attach `UserContext` to request context
  - Support multiple authentication methods
  - Log authentication attempts to audit-log
  - Return `ErrUnauthorized` on authentication failure
- **Dependencies**: 4.1.1
- **Estimated Effort**: 2 days

### Section 4.2: Authorization Implementation

#### Subsection 4.2.1: Permission Model
- **Files**: `types/auth.go`
- **Changes**:
  - Define `Permission` enum:
    - `PermissionReadDevices`
    - `PermissionWriteDevices`
    - `PermissionReadDatasets`
    - `PermissionWriteDatasets`
    - `PermissionReadEvents`
    - `PermissionWriteEvents`
    - `PermissionAdmin` (full access)
  - Define `UserContext` struct:
    - `UserID string`
    - `Permissions []Permission`
    - `IPAddress string`
    - `UserAgent string`
  - Define permission requirements per endpoint
- **Dependencies**: 4.1.1
- **Estimated Effort**: 1 day

#### Subsection 4.2.2: Authorization Middleware
- **Files**: `impl/authz_middleware.go` (new file)
- **Changes**:
  - Implement authorization middleware:
    - Extract `UserContext` from request context
    - Check required permissions for endpoint
    - Return `ErrForbidden` if insufficient permissions
  - Implement per-endpoint permission requirements:
    - Device endpoints: `ReadDevices` or `WriteDevices`
    - Dataset endpoints: `ReadDatasets` or `WriteDatasets`
    - Event endpoints: `ReadEvents`
    - Admin endpoints: `Admin`
  - Log authorization decisions to audit-log
  - Implement principle of least privilege (minimum required permissions)
- **Dependencies**: 4.2.1
- **Estimated Effort**: 2 days

---

## Epic 5: Input Validation and Security

**Goal**: Implement strict input validation and security requirements.

### Section 5.1: Input Validation

#### Subsection 5.1.1: Validation Framework
- **Files**: `impl/validation.go` (new file)
- **Changes**:
  - Implement input validation:
    - Validate request payloads (JSON schema validation)
    - Validate query parameters
    - Validate path parameters
    - Sanitize inputs (prevent injection attacks)
  - Implement validation middleware:
    - Validate before handler execution
    - Return HTTP 400 Bad Request for invalid inputs
    - Log validation failures
  - Support device-agnostic validation rules
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 5.1.2: Resource Exhaustion Prevention
- **Files**: `impl/resource_limits.go` (new file)
- **Changes**:
  - Implement resource limits:
    - Max request body size (configurable, default: 10MB)
    - Max query parameter count
    - Max path parameter length
    - Max concurrent requests per IP
  - Reject requests that exceed limits
  - Log resource limit violations
  - Emit alerts for sustained violations
- **Dependencies**: 5.1.1
- **Estimated Effort**: 1 day

### Section 5.2: Safe Logging

#### Subsection 5.2.1: Logging Sanitization
- **Files**: `impl/logging.go` (new file)
- **Changes**:
  - Implement safe logging:
    - Never log sensitive payloads (images, datasets, credentials)
    - Log request metadata (method, path, IP, user ID) but not body
    - Log response metadata (status code, size) but not body
    - Sanitize error messages (don't leak internal details)
  - Implement logging middleware:
    - Log request/response metadata
    - Skip sensitive endpoints from body logging
  - Integrate with audit-log for security-sensitive operations
- **Dependencies**: None
- **Estimated Effort**: 1 day

---

## Epic 6: Health Monitoring and Observability

**Goal**: Add comprehensive health monitoring following vm-gateway pattern.

### Section 6.1: Health Status Tracking

#### Subsection 6.1.1: Health Status Types
- **Files**: `types/health.go`
- **Changes**:
  - Define `HealthStatus` enum:
    - `HealthStatusHealthy`
    - `HealthStatusDegraded` (high error rate, dependency issues)
    - `HealthStatusUnhealthy` (server errors, critical failures)
  - Define `WebGatewayHealth` struct:
    - `Status HealthStatus`
    - `Uptime time.Duration`
    - `TotalRequests int64`
    - `ErrorRate float64` (recent error rate)
    - `RateLimitViolations int64`
    - `ActiveConnections int`
    - `DependencyHealth map[string]string` (meta-storage, object-storage, state-manager, vm-gateway)
    - `LastUpdated time.Time`
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 6.1.2: Health Snapshot API
- **Files**: `web_gateway.go`, `impl/web_gateway_impl.go`
- **Changes**:
  - Add `HealthSnapshot() WebGatewayHealth` method to interface
  - Implement health snapshot:
    - Query server status
    - Query dependency health
    - Aggregate metrics
    - Calculate error rates
    - Aggregate into `WebGatewayHealth` struct
  - Follow vm-gateway pattern for health snapshots
  - Expose via `GET /api/health` endpoint
- **Dependencies**: 6.1.1, Epic 3, Epic 4
- **Estimated Effort**: 2 days

### Section 6.2: Operational Metrics

#### Subsection 6.2.1: Metrics Tracking
- **Files**: `impl/metrics.go` (new file)
- **Changes**:
  - Track operational metrics:
    - Requests per second (by endpoint, by method)
    - Response latency (P50, P95, P99)
    - Error rates (by endpoint, by status code)
    - Rate limit violations per IP
    - Authentication/authorization failures
    - Active connections
  - Expose metrics via health snapshot or separate metrics endpoint (`GET /api/metrics`)
- **Dependencies**: 6.1.2
- **Estimated Effort**: 2 days

#### Subsection 6.2.2: Event Emission
- **Files**: `impl/web_gateway_impl.go`
- **Changes**:
  - Add event bus dependency (similar to vm-gateway)
  - Emit operational events:
    - `web_gateway.rate_limit_exceeded`
    - `web_gateway.auth_failed`, `web_gateway.authz_failed`
    - `web_gateway.health_degraded`, `web_gateway.health_recovered`
  - Use structured event types (similar to vm-gateway event types)
- **Dependencies**: 6.2.1
- **Estimated Effort**: 1 day

---

## Epic 7: State Manager Integration

**Goal**: Integrate with state manager for workflow commands and queries.

### Section 7.1: State Manager Dependency

#### Subsection 7.1.1: State Manager Interface
- **Files**: `impl/web_gateway_impl.go`
- **Changes**:
  - Add state manager dependency:
    - `stateManager statemng.StateManager` (interface dependency)
    - Use for workflow commands (dataset labeling, ML lifecycle queries)
  - Update `SetStateManager` method (if exists) or add to constructor
  - Remove direct meta-storage calls for workflow operations (delegate to state manager)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 7.1.2: Workflow Command Handlers
- **Files**: `impl/handlers_workflow.go` (new file)
- **Changes**:
  - Implement workflow command handlers:
    - `POST /api/devices/:deviceId/data-units/:dataUnitId/label` (label data unit)
    - `POST /api/devices/:deviceId/datasets/:datasetId/ready` (mark dataset ready)
    - `GET /api/devices/:deviceId/ml-lifecycle` (query ML lifecycle state)
  - Delegate to state manager for workflow operations
  - Return workflow state in responses
- **Dependencies**: 7.1.1
- **Estimated Effort**: 2 days

---

## Epic 8: Security Event Endpoints

**Goal**: Add endpoints for viewing security events with proper authorization.

### Section 8.1: Security Event Endpoints

#### Subsection 8.1.1: Event Listing Endpoints
- **Files**: `impl/handlers_security_events.go` (new file, enhance existing)
- **Changes**:
  - Add security event endpoints:
    - `GET /api/security-events` (list security events with filters)
    - `GET /api/security-events/:eventId` (get event details)
    - `GET /api/security-events/:eventId/attachment` (get event attachment)
    - `GET /api/devices/:deviceId/security-events` (list events for device)
  - Support filtering by:
    - DeviceID, DeviceType
    - Event type, confidence threshold
    - Time range
    - Status (pending_delivery, delivered)
  - Require `PermissionReadEvents` authorization
  - Integrate with state manager for event queries
- **Dependencies**: Epic 2, Epic 4
- **Estimated Effort**: 2 days

#### Subsection 8.1.2: Event Attachment Access
- **Files**: `impl/handlers_security_events.go`
- **Changes**:
  - Implement attachment access:
    - Serve attachments from object storage
    - Validate authorization (user must have `PermissionReadEvents`)
    - Log access to audit-log ("who accessed what")
    - Support streaming for large attachments
  - Implement attachment metadata endpoint:
    - Return attachment metadata without downloading
    - Include size, content type, device info
- **Dependencies**: 8.1.1
- **Estimated Effort**: 1 day

---

## Epic 9: Provider Implementation Refactoring

**Goal**: Refactor Gin provider to follow provider-agnostic pattern.

### Section 9.1: Gin Provider Refactoring

#### Subsection 9.1.1: Provider Interface Implementation
- **Files**: `impl/gin/gin_provider.go` (new file, extract from current implementation)
- **Changes**:
  - Implement `WebServerProvider` interface:
    - `Start(ctx, config) error`
    - `Stop(ctx) error`
    - `RegisterRoute(method, path, handler) error`
    - `RegisterMiddleware(middleware) error`
    - `GetRouter() interface{}` (for handler registration)
  - Extract Gin-specific code from main implementation
  - Make provider-agnostic
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 9.1.2: Provider Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `GinProviderConfig` struct:
    - `Mode string` (debug, release, test)
    - `TrustedProxies []string`
    - Gin-specific settings
  - Add provider-specific configuration to `WebGatewayConfig`
- **Dependencies**: 9.1.1
- **Estimated Effort**: 1 day

---

## Epic 10: API Documentation and OpenAPI

**Goal**: Add comprehensive API documentation and OpenAPI specification.

### Section 10.1: OpenAPI Specification

#### Subsection 10.1.1: OpenAPI Schema Generation
- **Files**: `types/openapi.go` (new file)
- **Changes**:
  - Define OpenAPI 3.0 specification:
    - All endpoints documented
    - Request/response schemas
    - Authentication requirements
    - Error responses
    - Rate limiting information
  - Generate from code annotations or maintain separately
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 10.1.2: API Documentation Endpoint
- **Files**: `impl/handlers_docs.go` (new file)
- **Changes**:
  - Add documentation endpoints:
    - `GET /api/docs` (serve OpenAPI spec)
    - `GET /api/docs/swagger` (serve Swagger UI)
  - Serve interactive API documentation
- **Dependencies**: 10.1.1
- **Estimated Effort**: 1 day

---

## Epic 11: Documentation and Testing

**Goal**: Add comprehensive documentation and testing following vm-gateway pattern.

### Section 11.1: Documentation

#### Subsection 11.1.1: Package Documentation
- **Files**: `doc.go` (new file)
- **Changes**:
  - Add comprehensive package documentation (similar to vm-gateway/doc.go):
    - Architecture overview
    - Provider-agnostic design
    - Device-agnostic APIs
    - Authentication and authorization
    - Rate limiting
    - Configuration examples
    - Usage examples
    - Lifecycle management
    - Health monitoring
  - Document device-agnostic design
  - Document security requirements
- **Dependencies**: All epics
- **Estimated Effort**: 1 day

#### Subsection 11.1.2: API Documentation
- **Files**: All handler files
- **Changes**:
  - Add comprehensive endpoint documentation
  - Document request/response formats
  - Document error conditions
  - Document authentication requirements
  - Document rate limiting
  - Add usage examples
- **Dependencies**: 11.1.1
- **Estimated Effort**: 1 day

### Section 11.2: Testing

#### Subsection 11.2.1: Unit Tests
- **Files**: `*_test.go` files
- **Changes**:
  - Test rate limiting
  - Test authentication and authorization
  - Test input validation
  - Test device-agnostic endpoints
  - Test health monitoring
  - Test provider abstraction
- **Dependencies**: All epics
- **Estimated Effort**: 3 days

#### Subsection 11.2.2: Integration Tests
- **Files**: `*_integration_test.go` files
- **Changes**:
  - Test full API lifecycle (request → response)
  - Test rate limiting with real requests
  - Test authentication/authorization with real credentials
  - Test device-agnostic endpoints with real devices
  - Test health monitoring
- **Dependencies**: 11.2.1
- **Estimated Effort**: 2 days

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~2 weeks
- **Epics**: 1 (Provider-Agnostic Architecture), 2 (Device-Agnostic Architecture)
- **Rationale**: Establishes the architectural foundation and device-agnostic API surface

### Phase 2: Security Features (Epics 3, 4, 5)
- **Duration**: ~2.5 weeks
- **Epics**: 3 (Rate Limiting), 4 (Enhanced Auth/AuthZ), 5 (Input Validation and Security)
- **Rationale**: Implements core security features

### Phase 3: Integration and Features (Epics 6, 7, 8)
- **Duration**: ~2 weeks
- **Epics**: 6 (Health Monitoring), 7 (State Manager Integration), 8 (Security Event Endpoints)
- **Rationale**: Completes integration and feature set

### Phase 4: Provider and Polish (Epics 9, 10, 11)
- **Duration**: ~1.5 weeks
- **Epics**: 9 (Provider Refactoring), 10 (API Documentation), 11 (Documentation and Testing)
- **Rationale**: Completes provider implementation, documentation, and testing

**Total Estimated Duration**: ~8 weeks

---

## Migration Notes

### Breaking Changes
- All `CameraID` references become `DeviceID`
- Camera endpoints replaced with device endpoints
- Screenshot endpoints replaced with data unit endpoints
- API paths change (`/api/cameras` → `/api/devices`, `/api/screenshots` → `/api/data-units`)
- Authentication/authorization requirements may change
- Rate limiting may affect existing clients

### Data Migration
- No data migration needed (web-gateway is stateless)
- API clients need to update to new endpoints
- Authentication credentials may need migration

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Update API documentation
- Notify API clients of endpoint changes
- Gradual rollout to production with monitoring
- Monitor rate limiting and auth failures
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Provider-agnostic architecture implemented (following vm-gateway pattern)
2. ✅ Device-agnostic types and endpoints implemented
3. ✅ Rate limiting implemented and tested
4. ✅ Enhanced authentication and authorization implemented and tested
5. ✅ Input validation and security implemented
6. ✅ Health monitoring implemented and tested
7. ✅ State manager integration implemented
8. ✅ Security event endpoints implemented
9. ✅ API documentation added (OpenAPI)
10. ✅ Comprehensive documentation added
11. ✅ Full test coverage (unit, integration)
12. ✅ Health snapshot API implemented
13. ✅ Event emission implemented

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **Architecture should follow vm-gateway pattern** (but simpler, as web-gateway is a simpler service)
- **Device-agnostic implementation is mandatory** (not just camera support)
- **Rate limiting is critical** (1000 req/min per IP, configurable)
- **Strict authZ is mandatory** (principle of least privilege for dataset/event access)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

