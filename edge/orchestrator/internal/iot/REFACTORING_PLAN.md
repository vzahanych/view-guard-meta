# IoT Service Refactoring Plan (Polishing and Alignment)

**Date**: 2025-12-28  
**Target Document**: `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`  
**Scope**: Polishing and alignment of `iot` package with production workflow requirements  
**Status**: iot service has proper structure; this plan focuses on missing production features

---

## Executive Summary

The iot service has been refactored and follows a device-agnostic architecture pattern. However, several production-critical features from `WORKFLOW_AND_BUSINESS_LOGIC.md` are missing or need enhancement:

1. **Event emission to event bus** - Device lifecycle events not emitted
2. **Timeout and retry configuration** - Device operations need explicit timeouts
3. **Capability validation enhancements** - Need stricter capability checks
4. **Device discovery independence** - Must not block on VM connectivity failures
5. **Health monitoring enhancements** - Need more detailed metrics
6. **Data stream availability checks** - Need explicit availability verification
7. **Device ID stability guarantees** - Need verification and documentation

This refactoring plan focuses on **polishing and alignment** rather than architectural changes.

---

## Epic 1: Event Emission to Event Bus

**Goal**: Emit all required device lifecycle events to event bus as specified.

### Section 1.1: Event Bus Integration

#### Subsection 1.1.1: Event Bus Dependency
- **Files**: `iot_impl.go`, `iot_provider.go`
- **Changes**:
  - Add event bus dependency to `iotServiceImpl`:
    - `eventBus eventbus.EventBus` (optional, for event emission)
  - Add event bus parameter to `NewIoTService` constructor
  - Add event bus parameter to `IoTServiceProvider` (Fx provider)
  - Make event bus optional (service works without it, but doesn't emit events)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Device Lifecycle Events
- **Files**: `iot_impl.go`, `device-registry/registry.go`
- **Changes**:
  - Emit `device.discovered` event when device is discovered:
    - Event data: DeviceID, DeviceType, metadata
    - **Emit IMMEDIATELY when device is discovered by plugin (before registration)**
    - Emit in device plugin discovery (as soon as device is detected)
  - Emit `device.registered` event when device is registered:
    - Event data: DeviceID, DeviceType, metadata
    - **Emit AFTER device is successfully registered in device registry**
    - Emit in `RegisterDevice` (after successful registration)
  - Emit `device.connected` event when device becomes reachable:
    - Event data: DeviceID, DeviceType, connection details
    - Emit when device state transitions to connected
  - Emit `device.disconnected` event when device loses connectivity:
    - Event data: DeviceID, DeviceType, error details
    - Emit when device state transitions to disconnected
  - Emit `device.capability_changed` event when capabilities change:
    - Event data: DeviceID, DeviceType, old capabilities, new capabilities
    - Emit in `UpdateDevice` when capabilities change
  - Emit `device.error` event when device-specific error occurs:
    - Event data: DeviceID, DeviceType, error message, error type
    - Emit on device operation failures
- **Dependencies**: 1.1.1
- **Estimated Effort**: 2 days

#### Subsection 1.1.3: Device Data Events
- **Files**: `processing/pipeline.go`, `cctv/impl/`
- **Changes**:
  - Emit `raw_device_data.frame_received` event when video frame is captured:
    - Event data: DeviceID, DeviceType, frame metadata, object storage key
    - Emit in video data processor or CCTV service
  - Emit `raw_device_data.clip_recorded` event when video clip is saved:
    - Event data: DeviceID, DeviceType, clip metadata, object storage key
    - Emit in clip recorder
  - Emit `sensor_data.reading_received` event when sensor reading is captured:
    - Event data: DeviceID, DeviceType, sensor type, reading value
    - Emit in sensor data processor (future)
  - Emit `audio_data.sample_received` event when audio sample is captured:
    - Event data: DeviceID, DeviceType, sample metadata, object storage key
    - Emit in audio data processor (future)
  - Make events optional (only emit if event bus is available)
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

---

## Epic 2: Timeout and Retry Configuration

**Goal**: Add explicit timeouts and retry configuration for device operations.

### Section 2.1: Timeout Configuration

#### Subsection 2.1.1: Timeout Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `DeviceOperationConfig` struct:
    - `DeviceConnectTimeout time.Duration` (default: 10s)
    - `DeviceOperationTimeout time.Duration` (default: 30s)
    - `DataCaptureTimeout time.Duration` (default: 60s)
    - `DiscoveryTimeout time.Duration` (default: 10s, already exists, enhance)
  - Add device operation config to `IoTServiceConfig`
  - Validate timeout values (must be > 0)
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 2.1.2: Timeout Enforcement
- **Files**: `iot_impl.go`, `device-registry/registry.go`, `cctv/impl/`
- **Changes**:
  - Enforce timeouts in all device operations:
    - Device discovery: use `DiscoveryTimeout`
    - Device connection: use `DeviceConnectTimeout`
    - Device operations (capture, stream start): use `DeviceOperationTimeout`
    - Data capture: use `DataCaptureTimeout`
  - Use `context.WithTimeout` for all device operations
  - Log timeout violations
  - Return timeout errors with context
- **Dependencies**: 2.1.1
- **Estimated Effort**: 2 days

### Section 2.2: Retry Configuration

#### Subsection 2.2.1: Retry Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `RetryConfig` struct:
    - `MaxRetries int` (default: 3)
    - `InitialBackoff time.Duration` (default: 1s)
    - `MaxBackoff time.Duration` (default: 10s)
    - `BackoffMultiplier float64` (default: 2.0)
  - Add retry config to `IoTServiceConfig`
  - Add per-operation retry configs (discovery, connection, data capture)
- **Dependencies**: None
- **Estimated Effort**: 0.5 days

#### Subsection 2.2.2: Retry Implementation
- **Files**: `iot_impl.go`, `device-registry/registry.go`
- **Changes**:
  - Implement retry with exponential backoff for transient failures:
    - Device discovery failures (network errors)
    - Device connection failures (temporary network issues)
    - Device operation failures (timeouts, temporary errors)
  - Don't retry on permanent failures (invalid device, auth failures)
  - Log retry attempts
  - Emit events on retry failures
- **Dependencies**: 2.2.1
- **Estimated Effort**: 2 days

---

## Epic 3: Capability Validation Enhancements

**Goal**: Enhance capability checks to gate operations as specified.

### Section 3.1: Capability Check Enforcement

#### Subsection 3.1.1: Capability Check Middleware
- **Files**: `iot_impl.go`
- **Changes**:
  - Enhance capability checks in all operations:
    - `CaptureData`: Require `DeviceCapabilityDataCapture`
    - `StartDataStream`: Require `DeviceCapabilityDataStreaming`
    - `ReadSensor`: Require `DeviceCapabilitySensorReadings`
    - `ExecuteCommand`: Require `DeviceCapabilityControl`
  - Return clear error if capability not supported
  - Log capability check failures
  - Emit `device.error` event on capability check failures
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Capability Readiness Checks
- **Files**: `iot_impl.go`, `types/device.go`
- **Changes**:
  - Add capability readiness checks:
    - Check if capability is enabled (not just supported)
    - Check if capability is ready (device connected, capability initialized)
    - Check if capability is available (no errors, not degraded)
  - Add `IsCapabilityReady(capability DeviceCapability) bool` method to Device interface
  - Use readiness checks in operations
  - Document capability readiness requirements
- **Dependencies**: 3.1.1
- **Estimated Effort**: 2 days

---

## Epic 4: Device Discovery Independence

**Goal**: Ensure device discovery continues independently of VM connectivity failures.

### Section 4.1: Discovery Independence

#### Subsection 4.1.1: Discovery Isolation
- **Files**: `iot_impl.go`, `plugin-registry/registry.go`
- **Changes**:
  - Ensure device discovery is isolated from VM connectivity:
    - Discovery runs independently of VM connection state
    - Discovery failures don't affect VM operations
    - VM connectivity failures don't block discovery
  - Add discovery status tracking (independent of VM status)
  - Log discovery independence
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.1.2: Discovery Error Handling
- **Files**: `iot_impl.go`, `plugin-registry/registry.go`
- **Changes**:
  - Handle discovery errors gracefully:
    - Log discovery errors but don't fail entire discovery
    - Continue discovery for other plugins/devices on failure
    - Emit `device.error` events for discovery failures
    - Track discovery error rates
  - Add discovery health metrics
- **Dependencies**: 4.1.1
- **Estimated Effort**: 1 day

---

## Epic 5: Data Stream Availability Checks

**Goal**: Implement explicit data stream availability verification.

### Section 5.1: Stream Availability Verification

#### Subsection 5.1.1: Stream Availability API
- **Files**: `iot.go`, `iot_impl.go`
- **Changes**:
  - Add `IsDataStreamAvailable(ctx context.Context, deviceID string) (bool, error)` method:
    - Check if device is connected
    - Check if streaming capability is ready
    - Check if no device-level errors exist
    - Return availability status
  - Document availability requirements:
    - Device must be connected
    - Streaming capability must be ready
    - No device-level errors
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Stream Availability Integration
- **Files**: `iot_impl.go`, `state-machine/machine.go`
- **Changes**:
  - Integrate stream availability checks:
    - Use in `StartDataStream` to verify availability before starting
    - Use in state machine to track stream availability
    - Emit events when stream availability changes
  - Add stream availability to health snapshot
- **Dependencies**: 5.1.1
- **Estimated Effort**: 1 day

---

## Epic 6: Device ID Stability Guarantees

**Goal**: Verify and document device ID stability requirements.

### Section 6.1: Device ID Stability Verification

#### Subsection 6.1.1: Device ID Generation Review
- **Files**: `device-registry/registry.go`, `plugin-registry/registry.go`
- **Changes**:
  - Review device ID generation:
    - Verify device IDs are stable across reconnections
    - Verify device IDs are stable across Edge restarts
    - Verify device IDs are usable as VM correlation keys
    - Document device ID format and stability guarantees
  - Add device ID validation
  - Add device ID persistence verification
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 6.1.2: Device ID Stability Tests
- **Files**: `device-registry/registry_test.go`
- **Changes**:
  - Add tests for device ID stability:
    - Test device ID persistence across restarts
    - Test device ID stability across reconnections
    - Test device ID format validation
  - Document device ID stability guarantees
- **Dependencies**: 6.1.1
- **Estimated Effort**: 1 day

---

## Epic 7: Health Monitoring Enhancements

**Goal**: Enhance health monitoring to include all required metrics.

### Section 7.1: Health Snapshot Enhancement

#### Subsection 7.1.1: Health Metrics
- **Files**: `iot.go`, `iot_impl.go`
- **Changes**:
  - Enhance `IoTServiceStatus` struct:
    - Add `DiscoveryStatus` (active, paused, error)
    - Add `DeviceConnectionStats` (connected, disconnected, error counts)
    - Add `CapabilityStats` (capabilities by type, readiness counts)
    - Add `StreamAvailabilityStats` (available streams, unavailable streams)
    - Add `ErrorRates` (discovery errors, connection errors, operation errors)
    - Add `RetryStats` (retry counts, backoff durations)
  - Update `HealthSnapshot()` to include new metrics
- **Dependencies**: All previous epics
- **Estimated Effort**: 1 day

#### Subsection 7.1.2: Health Monitoring
- **Files**: `iot_impl.go`
- **Changes**:
  - Add periodic health monitoring:
    - Track device connection uptime
    - Track discovery success/failure rates
    - Track capability readiness
    - Track stream availability
    - Track error rates
  - Emit health events: `iot.health_degraded`, `iot.health_recovered`
- **Dependencies**: 7.1.1
- **Estimated Effort**: 1 day

---

## Epic 8: Robust Shutdown Semantics

**Goal**: Ensure robust shutdown semantics as specified.

### Section 8.1: Shutdown Enhancement

#### Subsection 8.1.1: Graceful Shutdown
- **Files**: `iot_impl.go`
- **Changes**:
  - Enhance `Stop()` method:
    - Stop all active data streams gracefully
    - Wait for in-flight operations to complete (with timeout)
    - Stop device connections gracefully
    - Stop discovery loops
    - Persist device state
  - Add shutdown timeout (max 60s)
  - Log shutdown progress
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 8.1.2: Shutdown Error Handling
- **Files**: `iot_impl.go`
- **Changes**:
  - Handle shutdown errors gracefully:
    - Log shutdown errors but don't block shutdown
    - Continue shutdown for other components on failure
    - Return aggregated shutdown errors
  - Add shutdown health tracking
- **Dependencies**: 8.1.1
- **Estimated Effort**: 0.5 days

---

## Epic 9: Documentation Updates

**Goal**: Update documentation to reflect production requirements.

### Section 9.1: Documentation Enhancement

#### Subsection 9.1.1: Package Documentation
- **Files**: `doc.go`
- **Changes**:
  - Update package documentation:
    - Document event emission requirements
    - Document timeout and retry configuration
    - Document capability validation
    - Document device discovery independence
    - Document data stream availability
    - Document device ID stability guarantees
    - Document health monitoring
    - Document shutdown semantics
  - Add configuration examples
  - Add production best practices
- **Dependencies**: All previous epics
- **Estimated Effort**: 1 day

#### Subsection 9.1.2: API Documentation
- **Files**: All API files
- **Changes**:
  - Document timeout requirements
  - Document retry behavior
  - Document capability requirements
  - Document error conditions
  - Document event emission
  - Add usage examples
- **Dependencies**: 9.1.1
- **Estimated Effort**: 1 day

---

## Implementation Order and Dependencies

### Phase 1: Event Emission and Independence (Epics 1, 4)
- **Duration**: ~1.5 weeks
- **Epics**: 1 (Event Emission), 4 (Device Discovery Independence)
- **Rationale**: Establishes event emission and independence guarantees

### Phase 2: Reliability Features (Epics 2, 3, 5, 6)
- **Duration**: ~2 weeks
- **Epics**: 2 (Timeout and Retry), 3 (Capability Validation), 5 (Stream Availability), 6 (Device ID Stability)
- **Rationale**: Implements reliability and validation features

### Phase 3: Observability and Polish (Epics 7, 8, 9)
- **Duration**: ~1 week
- **Epics**: 7 (Health Monitoring), 8 (Shutdown Semantics), 9 (Documentation)
- **Rationale**: Completes observability and documentation

**Total Estimated Duration**: ~4.5 weeks

---

## Migration Notes

### Breaking Changes
- Event bus dependency added (optional, but recommended)
- Timeout configuration may affect existing behavior (requires config review)
- Capability checks may reject operations that previously worked (requires capability verification)

### Configuration Migration
- Add event bus configuration (optional)
- Add timeout configuration
- Add retry configuration
- Review capability requirements

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Update configuration documentation
- Gradual rollout to production with monitoring
- Monitor event emission, timeouts, capability checks
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ All required device lifecycle events are emitted
2. ✅ All required device data events are emitted
3. ✅ Timeout configuration implemented and tested
4. ✅ Retry configuration implemented and tested
5. ✅ Capability validation enhanced and tested
6. ✅ Device discovery independence verified
7. ✅ Data stream availability checks implemented
8. ✅ Device ID stability verified and documented
9. ✅ Health monitoring enhanced
10. ✅ Robust shutdown semantics implemented
11. ✅ Documentation updated

---

## Notes

- **This is a polishing plan**: iot service already has proper structure; focus on missing features
- **No architectural changes**: Keep existing device-agnostic architecture
- **Backward compatibility**: Maintain compatibility where possible; document breaking changes
- **Event emission is critical**: Required for state manager integration
- **Timeout/retry is critical**: Required for production reliability
- **Capability validation is critical**: Required for safe operations
- **Device discovery independence is critical**: Required for offline operation

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

