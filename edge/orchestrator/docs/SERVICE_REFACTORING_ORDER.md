# Service Refactoring Order

**Date**: 2025-12-28  
**Purpose**: Define the optimal order for refactoring all services based on dependencies and architectural requirements

---

## Executive Summary

This document defines the recommended order for refactoring all services in the edge orchestrator. The order is based on:
1. **Dependency analysis**: Services that other services depend on should be refactored first
2. **Foundation services**: Core infrastructure services (storage, event bus) should be refactored early
3. **Parallel opportunities**: Services with no dependencies on each other can be refactored in parallel
4. **Orchestrator last**: The state-mng service orchestrates all others and should be refactored last

---

## Dependency Graph

```
┌─────────────┐
│ state-mng  │ (orchestrator - depends on all)
└─────┬──────┘
      │
      ├──► meta-storage ──┐
      ├──► object-storage │
      ├──► event-bus ─────┤ (foundation services)
      ├──► vm-gateway ────┤
      ├──► iot ───────────┤
      ├──► audit-log ─────┤ (depends on meta-storage)
      ├──► ai-gateway ────┤ (depends on iot, event-bus)
      └──► web-gateway ───┘ (depends on state-mng, meta-storage, object-storage)
```

---

## Refactoring Phases

### Phase 1: Foundation Services (Parallel Execution Recommended)

**Duration**: ~8-10 weeks  
**Services**: meta-storage, object-storage, event-bus  
**Rationale**: These services are foundational infrastructure used by all other services. Refactoring them first establishes the architectural patterns and provides stable interfaces for dependent services.

#### 1.1: meta-storage
- **Priority**: Critical
- **Dependencies**: None (foundation service)
- **Used by**: state-mng, event-bus, audit-log, web-gateway
- **Key Changes**: 
  - Provider-agnostic architecture (vm-gateway pattern)
  - Device-agnostic types (CameraID → DeviceID)
  - ML lifecycle state management (CAS operations)
  - Quota enforcement, retention policies, schema versioning
- **Estimated Duration**: ~8.5 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/meta-storage/REFACTORING_PLAN.md`

#### 1.2: object-storage
- **Priority**: Critical
- **Dependencies**: None (foundation service)
- **Used by**: state-mng, audit-log, web-gateway
- **Key Changes**:
  - Provider-agnostic architecture (vm-gateway pattern)
  - Device-agnostic types (CameraID → DeviceID)
  - Quota enforcement, retention policies
  - Storage integrity verification
- **Estimated Duration**: ~6 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/object-storage/REFACTORING_PLAN.md`

#### 1.3: event-bus
- **Priority**: Critical
- **Dependencies**: None (foundation service, but uses meta-storage for persistence)
- **Used by**: state-mng, vm-gateway, web-gateway, ai-gateway, audit-log, iot
- **Key Changes**:
  - Provider-agnostic architecture (vm-gateway pattern)
  - Event drop policy (workflow triggers vs operational/critical)
  - Retention and cleanup (24 hours, 6 hours)
  - Storage pressure handling
- **Estimated Duration**: ~7 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/event-bus/REFACTORING_PLAN.md`
- **Note**: Can start in parallel with meta-storage, but will need meta-storage interface finalized before completion

**Phase 1 Completion Criteria**:
- ✅ All foundation services refactored and tested
- ✅ Provider-agnostic architecture implemented
- ✅ Device-agnostic types implemented
- ✅ Production features (quota, retention, health monitoring) implemented
- ✅ Interfaces stable and documented

---

### Phase 2: Communication Services (Parallel Execution Recommended)

**Duration**: ~7-8 weeks  
**Services**: vm-gateway, iot  
**Rationale**: These services handle external communication and device management. They have good existing structure and need polishing/enhancement rather than complete refactoring. They can be done in parallel after foundation services are stable.

#### 2.1: vm-gateway
- **Priority**: High
- **Dependencies**: event-bus (for event emission)
- **Used by**: state-mng
- **Key Changes**:
  - Certificate pinning, rotation, revocation
  - Time synchronization checks
  - Idempotency keys
  - Rate limiting for inbound VM commands
  - HTTPS server readiness guarantee
  - Timeout standardization
  - Retry/backoff strategies
- **Estimated Duration**: ~7 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/vm-gateway/REFACTORING_PLAN.md`
- **Note**: This is a polishing plan - vm-gateway already has proper structure

#### 2.2: iot
- **Priority**: High
- **Dependencies**: event-bus (for event emission)
- **Used by**: state-mng, ai-gateway
- **Key Changes**:
  - Event emission to event bus (device lifecycle events)
  - Timeout and retry configuration
  - Capability validation enhancements
  - Device discovery independence (from VM connectivity)
  - Data stream availability checks
  - Device ID stability guarantees
  - Health monitoring enhancements
- **Estimated Duration**: ~4.5 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/iot/REFACTORING_PLAN.md`
- **Note**: This is a polishing plan - iot already has proper structure

**Phase 2 Completion Criteria**:
- ✅ All communication services polished and tested
- ✅ Event emission implemented
- ✅ Timeout/retry configuration implemented
- ✅ Security features (certificate pinning, time sync) implemented
- ✅ Health monitoring enhanced

---

### Phase 3: Dependent Services (Sequential Execution Recommended)

**Duration**: ~8-9 weeks  
**Services**: audit-log, ai-gateway  
**Rationale**: These services depend on foundation services (meta-storage, event-bus) and communication services (iot). They should be refactored after their dependencies are stable.

#### 3.1: audit-log
- **Priority**: High
- **Dependencies**: meta-storage (preferred) or object-storage, event-bus
- **Used by**: state-mng
- **Key Changes**:
  - Provider-agnostic architecture (vm-gateway pattern)
  - Device-agnostic types (CameraID → DeviceID)
  - Sync queue management (max 100,000 records, pause-on-full)
  - Sync trigger optimization (5 minutes or 1000 records)
  - Retention updated to 90 days
  - Hash chain integrity verification
- **Estimated Duration**: ~7.5 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/audit-log/REFACTORING_PLAN.md`
- **Note**: Should wait for meta-storage refactoring to be complete or near-complete

#### 3.2: ai-gateway
- **Priority**: High
- **Dependencies**: iot (for device data), event-bus (for event emission)
- **Used by**: state-mng
- **Key Changes**:
  - Provider-agnostic architecture (vm-gateway pattern)
  - Device-agnostic types (CameraID → DeviceID)
  - Task-based architecture (state manager creates tasks, AI gateway executes)
  - Circuit breaker implementation
  - Model-aware inference (per-device model selection)
  - IoT DataProcessor integration
- **Estimated Duration**: ~8.5 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/ai-gateway/REFACTORING_PLAN.md`
- **Note**: Should wait for iot refactoring to be complete or near-complete

**Phase 3 Completion Criteria**:
- ✅ All dependent services refactored and tested
- ✅ Integration with foundation services verified
- ✅ Production features implemented
- ✅ Health monitoring implemented

---

### Phase 4: Web Gateway

**Duration**: ~8 weeks  
**Service**: web-gateway  
**Rationale**: Web gateway depends on state-mng for workflow commands and uses meta-storage/object-storage. It should be refactored after state-mng is refactored, but can start earlier if state-mng interface is stable.

#### 4.1: web-gateway
- **Priority**: Medium
- **Dependencies**: state-mng (for workflow commands), meta-storage, object-storage
- **Used by**: Users (web UI)
- **Key Changes**:
  - Provider-agnostic architecture (vm-gateway pattern)
  - Device-agnostic APIs (CameraID → DeviceID)
  - Rate limiting (1000 req/min per IP)
  - Enhanced authentication/authorization
  - State manager integration
  - Security event endpoints
- **Estimated Duration**: ~8 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/web-gateway/REFACTORING_PLAN.md`
- **Note**: Can start after state-mng interface is stable, but should complete after state-mng refactoring

**Phase 4 Completion Criteria**:
- ✅ Web gateway refactored and tested
- ✅ Device-agnostic APIs implemented
- ✅ Rate limiting and authZ implemented
- ✅ State manager integration verified

---

### Phase 5: State Manager (Orchestrator)

**Duration**: ~10 weeks  
**Service**: state-mng  
**Rationale**: State manager orchestrates all other services and depends on all of them. It should be refactored last to ensure all dependencies have stable interfaces.

#### 5.1: state-mng
- **Priority**: Critical
- **Dependencies**: ALL other services
  - meta-storage (ML lifecycle state, events, datasets)
  - object-storage (model artifacts, event attachments)
  - event-bus (event publishing/subscription)
  - vm-gateway (VM communication)
  - iot (device management)
  - ai-gateway (inference)
  - audit-log (audit logging)
- **Used by**: web-gateway (for workflow commands)
- **Key Changes**:
  - Device-agnostic architecture (CameraID → DeviceID)
  - ML lifecycle state management (per-device state machine)
  - Reconciliation loops (startup, periodic, reconnection)
  - VM protocol implementation (SyncDevices, SyncDataUnits, SendEvents)
  - Model verification and activation
  - Security event storage and delivery
  - Storage integrity and recovery
  - Per-device workflow serialization
- **Estimated Duration**: ~10 weeks
- **Refactoring Plan**: `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- **Note**: This is the most complex refactoring and should be done last

**Phase 5 Completion Criteria**:
- ✅ State manager refactored and tested
- ✅ All workflow requirements implemented
- ✅ Integration with all services verified
- ✅ Production features (reconciliation, recovery, integrity) implemented

---

## Recommended Execution Strategy

### Option A: Sequential (Safest)
Execute phases sequentially, completing each phase before starting the next:
1. Phase 1: Foundation Services (8-10 weeks)
2. Phase 2: Communication Services (7-8 weeks)
3. Phase 3: Dependent Services (8-9 weeks)
4. Phase 4: Web Gateway (8 weeks)
5. Phase 5: State Manager (10 weeks)

**Total Duration**: ~41-45 weeks (~10-11 months)

### Option B: Parallel Foundation (Recommended)
Execute foundation services in parallel, then proceed sequentially:
1. Phase 1: Foundation Services (parallel execution, 8-10 weeks)
   - meta-storage: Weeks 1-8.5
   - object-storage: Weeks 1-6 (can start earlier)
   - event-bus: Weeks 1-7 (can start after meta-storage interface is stable)
2. Phase 2: Communication Services (parallel execution, 7-8 weeks)
   - vm-gateway: Weeks 9-16
   - iot: Weeks 9-13.5
3. Phase 3: Dependent Services (sequential, 8-9 weeks)
   - audit-log: Weeks 14-21.5
   - ai-gateway: Weeks 14-22.5
4. Phase 4: Web Gateway (8 weeks)
   - web-gateway: Weeks 23-31
5. Phase 5: State Manager (10 weeks)
   - state-mng: Weeks 32-42

**Total Duration**: ~42 weeks (~10.5 months)

### Option C: Aggressive Parallel (Fastest, Higher Risk)
Execute as many services in parallel as dependencies allow:
1. Phase 1: Foundation Services (parallel, 8-10 weeks)
2. Phase 2: Communication Services (parallel, 7-8 weeks) - start after foundation services are 50% complete
3. Phase 3: Dependent Services (parallel, 8-9 weeks) - start after dependencies are 50% complete
4. Phase 4: Web Gateway (8 weeks) - start after state-mng interface is stable
5. Phase 5: State Manager (10 weeks) - start after all dependencies are complete

**Total Duration**: ~30-35 weeks (~7.5-9 months) - **Higher risk due to parallel execution**

---

## Critical Dependencies and Blockers

### meta-storage is a blocker for:
- event-bus (uses meta-storage for persistence)
- audit-log (prefers meta-storage over object-storage)
- state-mng (uses meta-storage for ML lifecycle state)

### event-bus is a blocker for:
- vm-gateway (needs event-bus for event emission)
- iot (needs event-bus for event emission)
- state-mng (needs event-bus for event publishing)
- ai-gateway (needs event-bus for event emission)
- audit-log (needs event-bus for event emission)

### iot is a blocker for:
- ai-gateway (needs iot for device data)

### state-mng is a blocker for:
- web-gateway (needs state-mng for workflow commands)

---

## Risk Mitigation

### Interface Stability
- **Risk**: Changing interfaces during refactoring breaks dependent services
- **Mitigation**: 
  - Define interfaces early and freeze them before dependent services start
  - Use interface versioning if needed
  - Maintain backward compatibility during transition

### Integration Testing
- **Risk**: Services refactored in parallel may have integration issues
- **Mitigation**:
  - Continuous integration testing after each phase
  - Integration test suite for each service pair
  - Staged rollout with feature flags

### Resource Contention
- **Risk**: Multiple teams working on dependent services simultaneously
- **Mitigation**:
  - Clear ownership and communication channels
  - Regular sync meetings
  - Shared test environments

---

## Success Criteria

### Phase 1 (Foundation Services)
- ✅ All foundation services follow vm-gateway architectural pattern
- ✅ Device-agnostic types implemented
- ✅ Production features (quota, retention, health monitoring) implemented
- ✅ Interfaces stable and documented
- ✅ Full test coverage (unit, integration)

### Phase 2 (Communication Services)
- ✅ Event emission implemented
- ✅ Security features (certificate pinning, time sync) implemented
- ✅ Timeout/retry configuration implemented
- ✅ Health monitoring enhanced
- ✅ Full test coverage

### Phase 3 (Dependent Services)
- ✅ Integration with foundation services verified
- ✅ Production features implemented
- ✅ Health monitoring implemented
- ✅ Full test coverage

### Phase 4 (Web Gateway)
- ✅ Device-agnostic APIs implemented
- ✅ Rate limiting and authZ implemented
- ✅ State manager integration verified
- ✅ Full test coverage

### Phase 5 (State Manager)
- ✅ All workflow requirements implemented
- ✅ Integration with all services verified
- ✅ Production features (reconciliation, recovery, integrity) implemented
- ✅ Full test coverage

---

## Notes

- **Estimated durations are from refactoring plans** and may vary based on team size and complexity
- **Parallel execution** can reduce total duration but increases coordination overhead
- **Interface stability** is critical - freeze interfaces before dependent services start
- **Continuous testing** is essential - run full test suite after each phase
- **Documentation** should be updated as services are refactored
- **Migration strategy** should be planned for each service

---

**Document Status**: Ready for implementation  
**Next Steps**: 
1. Review and approve refactoring order
2. Assign teams to services
3. Begin Phase 1 (Foundation Services)
4. Establish interface contracts and freeze them
5. Set up continuous integration and testing

