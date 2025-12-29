# Event Bus Refactoring Plan

**Date**: 2025-12-28  
**Target Documents**: 
- `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`
- `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- `edge/orchestrator/internal/vm-gateway/doc.go` (architectural pattern reference)

**Scope**: Complete refactoring of `event-bus` package to align with production workflow requirements and follow vm-gateway architectural pattern  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the Event Bus service implementation into full compliance with the production workflow specification and aligns it with the vm-gateway architectural pattern. The current implementation has good foundations (persistence, retry, ordering) but lacks production features (event drop policy, retention/cleanup, storage pressure handling, health monitoring) and doesn't follow the provider-agnostic architecture pattern.

**Key Transformation Areas**:
1. **Provider-agnostic architecture**: Follow vm-gateway pattern with interface, types, and implementation separation
2. **Event drop policy**: Implement categorization of events (droppable vs non-droppable) and drop policy enforcement
3. **Retention and cleanup**: Implement 24-hour retention and 6-hour cleanup cycles
4. **Storage pressure handling**: Drop droppable events when storage >90% full
5. **Health monitoring**: Add health snapshot API and operational metrics
6. **Observability**: Add comprehensive observability following vm-gateway pattern

---

## Epic 1: Provider-Agnostic Architecture (Following vm-gateway Pattern)

**Goal**: Restructure the codebase to follow the vm-gateway architectural pattern with clear separation of concerns.

### Section 1.1: Interface and Types Separation

#### Subsection 1.1.1: Main Interface File
- **Files**: `event_bus.go` (already exists, enhance)
- **Changes**:
  - Enhance `EventBus` interface with lifecycle methods:
    - `Start(ctx) error` (initialize provider, start background tasks)
    - `Stop(ctx) error` (stop background tasks, close connections)
  - Define sentinel errors (similar to vm-gateway):
    - `ErrNotInitialized`
    - `ErrAlreadyStarted`
    - `ErrStoragePressure` (when storage >90% full and dropping events)
    - `ErrEventDropped` (when non-droppable event is dropped)
  - Define factory function `NewEventBus(ctx, config, logger, metaStore)`
  - Define provider function `EventBusProvider(lc, cfg, logger, metaStore)` with fx lifecycle
  - Add comprehensive package documentation (similar to vm-gateway/doc.go)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Types Package Structure
- **Files**: `types/` directory (already exists, enhance)
- **Changes**:
  - Move all configuration types to `types/config.go`
  - Create `types/health.go` for health-related types:
    - `HealthStatus` enum (healthy, warning, storage_pressure, degraded)
    - `EventBusHealth` struct (status, metrics, storage pressure info)
  - Create `types/policy.go` for event drop policy:
    - `EventCategory` enum (workflow_trigger, operational_health, critical)
    - `EventDropPolicy` struct (categorization rules)
  - Create `types/provider.go` for provider interface:
    - `EventBusProvider` interface (provider-agnostic operations)
    - Provider-specific configuration types
  - Create `types/errors.go` for error types
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Implementation Package Structure
- **Files**: `impl/` directory (rename from `metastoragebus/`)
- **Changes**:
  - Create `impl/event_bus_impl.go` (main implementation)
  - Create provider-specific implementations:
    - `impl/metastorage/metastorage_provider.go` (meta-storage implementation, rename from `metastoragebus/`)
    - `impl/memory/memory_provider.go` (future: in-memory implementation for testing)
    - `impl/nats/nats_provider.go` (future: NATS implementation for distributed systems)
  - Each provider implements `types.EventBusProvider` interface
  - Main implementation delegates to provider
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

### Section 1.2: Lifecycle Management

#### Subsection 1.2.1: Service Lifecycle
- **Files**: `impl/event_bus_impl.go`
- **Changes**:
  - Implement `Start(ctx)` method:
    - Initialize provider
    - Verify connectivity (meta-storage)
    - Start background tasks:
      - Retention cleanup worker (runs every 6 hours)
      - Storage pressure monitor (runs every 5 minutes)
      - Health check worker (runs every 1 minute)
    - Initialize event drop policy
  - Implement `Stop(ctx)` method:
    - Stop background tasks gracefully
    - Close provider connections
    - Flush pending operations
  - Follow vm-gateway pattern: service owns lifecycle of sub-components
- **Dependencies**: 1.1.3
- **Estimated Effort**: 1 day

#### Subsection 1.2.2: Provider Lifecycle
- **Files**: `impl/metastorage/metastorage_provider.go` (and other providers)
- **Changes**:
  - Implement provider-specific initialization
  - Implement provider-specific cleanup
  - Providers do NOT register their own fx.Lifecycle hooks (gateway-owned lifecycle pattern)
- **Dependencies**: 1.2.1
- **Estimated Effort**: 1 day

---

## Epic 2: Event Drop Policy

**Goal**: Implement event categorization and drop policy as specified in the workflow document.

### Section 2.1: Event Categorization

#### Subsection 2.1.1: Event Category Types
- **Files**: `types/policy.go`
- **Changes**:
  - Define `EventCategory` enum:
    - `EventCategoryWorkflowTrigger` - Can be dropped (reconciliation will catch)
    - `EventCategoryOperationalHealth` - Must NOT be dropped
    - `EventCategoryCritical` - Must NOT be dropped (highest priority)
  - Define `EventDropPolicy` struct:
    - `CategoryRules map[EventType]EventCategory` (event type → category mapping)
    - `DefaultCategory EventCategory` (default for unmapped events)
  - Define default categorization rules:
    - **Workflow triggers** (can be dropped): 
      - `device.discovered`, `device.registered`, `device.connected`, `device.disconnected`
      - `model.deployment_received`, `model.verification_completed`, `model.stored`, `model.activation_started`, `model.activation_completed`, `model.deactivated`
      - `data_unit.requested`, `data_unit.captured`, `data_unit.labeled`, `data_unit.saved`, `data_unit.set_ready`
      - `dataset.upload_started`, `dataset.upload_completed`, `dataset.upload_failed`
      - `raw_device_data.frame_received`, `raw_device_data.clip_recorded`
      - `network.tunnel.*`, `network.transport.*`, `edge.authenticated`, `edge.capabilities_received`
      - `ai.inference_started`, `ai.inference_completed`, `ai.detection`, `ai.circuit_breaker_opened`, `ai.circuit_breaker_closed`
    - **Operational/health** (must NOT be dropped):
      - `storage.full`, `storage.warning`, `health.degraded`, `health.recovered`, `reconciliation.unhealthy`
    - **Critical** (must NOT be dropped, highest priority):
      - `storage.corruption_detected`, `system.startup`, `system.shutdown`
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 2.1.2: Event Category Registry
- **Files**: `impl/event_category_registry.go` (new file)
- **Changes**:
  - Implement `EventCategoryRegistry` struct:
    - Track event type → category mappings
    - Support dynamic registration
  - Implement `GetCategory(eventType EventType) EventCategory`:
    - Look up category for event type
    - Return default category if not found
  - Implement `IsDroppable(eventType EventType) bool`:
    - Return true if event category is `EventCategoryWorkflowTrigger`
    - Return false for operational/health/critical events
  - Initialize with default categorization rules
- **Dependencies**: 2.1.1
- **Estimated Effort**: 1 day

### Section 2.2: Drop Policy Enforcement

#### Subsection 2.2.1: Drop Policy in Publish
- **Files**: `impl/event_bus_impl.go`
- **Changes**:
  - Update `Publish` method to check drop policy:
    - Check if event is droppable (workflow trigger)
    - Check storage pressure status
    - If storage >90% full and event is droppable: drop event, log warning
    - If storage >90% full and event is NOT droppable: attempt to persist (may fail, but try)
    - If storage <90% full: persist all events normally
  - Implement drop counter (track dropped events by category)
  - Emit `event_bus.event_dropped` event when dropping (for monitoring)
- **Dependencies**: 2.1.2, Epic 3
- **Estimated Effort**: 2 days

#### Subsection 2.2.2: Storage Pressure Detection
- **Files**: `impl/storage_pressure_monitor.go` (new file)
- **Changes**:
  - Implement `StoragePressureMonitor` struct:
    - Monitor meta-storage quota status
    - Track storage pressure threshold (90% default)
  - Implement `IsStoragePressure(ctx) bool`:
    - Query meta-storage quota status
    - Return true if storage >90% full
  - Implement background monitoring (runs every 5 minutes)
  - Emit `event_bus.storage_pressure` event when pressure detected
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

---

## Epic 3: Retention and Cleanup

**Goal**: Implement event retention (24 hours) and cleanup (every 6 hours) as specified.

### Section 3.1: Retention Configuration

#### Subsection 3.1.1: Retention Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `RetentionConfig` struct:
    - `RetentionHours int` (default: 24 hours)
    - `CleanupIntervalHours int` (default: 6 hours)
    - `CleanupBatchSize int` (default: 1000 events per batch)
  - Add retention configuration to `EventBusConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Retention Manager
- **Files**: `impl/retention_manager.go` (new file)
- **Changes**:
  - Implement `RetentionManager` struct:
    - Track retention policy
    - Track cleanup schedule
  - Implement `CleanupExpiredEvents(ctx) error`:
    - Query events older than retention period (24 hours)
    - Delete expired events in batches
    - Handle cleanup errors gracefully (log and continue)
    - Return cleanup statistics (events deleted, space freed)
  - Implement background cleanup task (runs every 6 hours, configurable)
  - Emit events: `event_bus.cleanup_started`, `event_bus.cleanup_completed`
- **Dependencies**: 3.1.1, Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 3.1.3: Event Timestamp Tracking
- **Files**: `impl/metastorage/metastorage_provider.go`
- **Changes**:
  - Ensure event timestamps are stored with events in meta-storage
  - Add index or query optimization for timestamp-based cleanup
  - Support efficient querying by timestamp range
- **Dependencies**: 3.1.2
- **Estimated Effort**: 1 day

---

## Epic 4: Health Monitoring and Observability

**Goal**: Add comprehensive health monitoring following vm-gateway pattern.

### Section 4.1: Health Status Tracking

#### Subsection 4.1.1: Health Status Types
- **Files**: `types/health.go`
- **Changes**:
  - Define `HealthStatus` enum:
    - `HealthStatusHealthy`
    - `HealthStatusWarning` (storage 80-90% full)
    - `HealthStatusStoragePressure` (storage >90% full)
    - `HealthStatusDegraded` (provider errors, high drop rate)
  - Define `EventBusHealth` struct:
    - `Status HealthStatus`
    - `StoragePressure bool`
    - `StorageUsagePercent float64`
    - `EventsPublished int64` (total count)
    - `EventsDropped int64` (total count, by category)
    - `EventsPersisted int64` (total count)
    - `ActiveSubscribers int` (current subscriber count)
    - `LastCleanupTime time.Time`
    - `CleanupStats` (events deleted, space freed)
    - `ProviderHealth string` (provider-specific health status)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.1.2: Health Snapshot API
- **Files**: `event_bus.go`, `impl/event_bus_impl.go`
- **Changes**:
  - Add `HealthSnapshot() EventBusHealth` method to interface
  - Implement health snapshot:
    - Query storage pressure status
    - Query event statistics (published, dropped, persisted)
    - Query active subscriber count
    - Query last cleanup time and stats
    - Query provider health
    - Aggregate into `EventBusHealth` struct
  - Follow vm-gateway pattern for health snapshots
- **Dependencies**: 4.1.1, Section 2.2, Section 3.1
- **Estimated Effort**: 2 days

### Section 4.2: Operational Metrics

#### Subsection 4.2.1: Metrics Tracking
- **Files**: `impl/metrics.go` (new file)
- **Changes**:
  - Track operational metrics:
    - Events published per second (by event type)
    - Events dropped per second (by category)
    - Events persisted per second
    - Subscriber count over time
    - Publish latency (P50, P95, P99)
    - Storage usage over time
    - Cleanup statistics
  - Expose metrics via health snapshot or separate metrics endpoint
- **Dependencies**: 4.1.2
- **Estimated Effort**: 2 days

#### Subsection 4.2.2: Event Emission
- **Files**: `impl/event_bus_impl.go`
- **Changes**:
  - Emit operational events:
    - `event_bus.storage_pressure` (when storage >90% full)
    - `event_bus.event_dropped` (when dropping events, with category)
    - `event_bus.cleanup_started`, `event_bus.cleanup_completed`
    - `event_bus.health_degraded` (when health issues detected)
  - Use structured event types (similar to vm-gateway event types)
  - Note: These events should be published to the bus itself (self-monitoring)
- **Dependencies**: 4.2.1
- **Estimated Effort**: 1 day

---

## Epic 5: Provider Implementation Refactoring

**Goal**: Refactor meta-storage provider to follow provider-agnostic pattern.

### Section 5.1: Meta-Storage Provider Refactoring

#### Subsection 5.1.1: Provider Interface Implementation
- **Files**: `impl/metastorage/metastorage_provider.go` (rename from `metastoragebus/`)
- **Changes**:
  - Implement `EventBusProvider` interface:
    - `PersistEvent(ctx, event EventAny) error`
    - `LoadEvent(ctx, eventID string) (*EventAny, error)`
    - `ListEvents(ctx, filters EventFilters) ([]EventAny, error)`
    - `DeleteEvent(ctx, eventID string) error`
    - `DeleteExpiredEvents(ctx, beforeTime time.Time) (int, error)`
    - `GetEventCount(ctx) (int, error)`
    - `HealthCheck(ctx) error`
  - Remove direct meta-storage coupling from main implementation
  - Make provider-agnostic
- **Dependencies**: Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 5.1.2: Provider Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `MetaStorageProviderConfig` struct:
    - `MetaStorageDependency` (reference to meta-storage service)
    - Provider-specific settings
  - Add provider-specific configuration to `EventBusConfig`
- **Dependencies**: 5.1.1
- **Estimated Effort**: 1 day

---

## Epic 6: Event Type System Enhancement

**Goal**: Ensure all event types are properly categorized and device-agnostic.

### Section 6.1: Event Type Categorization

#### Subsection 6.1.1: Complete Event Type Mapping
- **Files**: `impl/event_category_registry.go`
- **Changes**:
  - Map all event types to categories:
    - Network events: `network.tunnel.*`, `network.transport.*` → WorkflowTrigger
    - Edge/VM events: `edge.authenticated`, `edge.capabilities_received` → WorkflowTrigger
    - Device events: `device.discovered`, `device.registered`, `device.connected`, `device.disconnected` → WorkflowTrigger
    - Data unit events: `data_unit.requested`, `data_unit.saved`, `data_unit.set_ready` → WorkflowTrigger
    - Raw device data: `raw_device_data.frame_received`, `raw_device_data.clip_recorded` → WorkflowTrigger
    - Storage events: `storage.full`, `storage.warning` → OperationalHealth
    - Storage corruption: `storage.corruption_detected` → Critical
    - System events: `system.startup`, `system.shutdown` → Critical
    - Health events: `health.degraded`, `health.recovered` → OperationalHealth
    - Reconciliation: `reconciliation.unhealthy` → OperationalHealth
    - AI events: `ai.detection`, `ai.circuit_breaker_opened` → WorkflowTrigger
    - Model events: `model.deployment_received`, `model.verification_failed` → WorkflowTrigger
  - Ensure all event types from `types/types.go` are categorized
- **Dependencies**: Section 2.1
- **Estimated Effort**: 1 day

#### Subsection 6.1.2: Dynamic Event Category Registration
- **Files**: `impl/event_category_registry.go`
- **Changes**:
  - Add `RegisterEventCategory(eventType EventType, category EventCategory)` method
  - Support runtime registration of event categories (for extensibility)
  - Validate category assignments (warn on invalid assignments)
- **Dependencies**: 6.1.1
- **Estimated Effort**: 1 day

---

## Epic 7: Performance and Scalability

**Goal**: Optimize event bus performance and scalability for production workloads.

### Section 7.1: Publish Performance

#### Subsection 7.1.1: Non-Blocking Publish Optimization
- **Files**: `impl/event_bus_impl.go`
- **Changes**:
  - Ensure `Publish` is fully non-blocking:
    - Use buffered channels (already implemented)
    - Use goroutines for persistence (already implemented)
    - Drop events on channel overflow (already implemented)
    - Optimize lock contention (fine-grained locking)
  - Implement publish metrics (latency tracking)
  - Optimize subscriber notification (batch notifications if possible)
- **Dependencies**: None
- **Estimated Effort**: 2 days

#### Subsection 7.1.2: Persistence Optimization
- **Files**: `impl/metastorage/metastorage_provider.go`
- **Changes**:
  - Optimize event persistence:
    - Batch writes if possible (write multiple events in one transaction)
    - Use async writes with buffering
    - Implement write-back cache for hot events
  - Handle persistence failures gracefully (log, don't block publish)
- **Dependencies**: 7.1.1
- **Estimated Effort**: 2 days

### Section 7.2: Subscription Scalability

#### Subsection 7.2.1: Subscription Management
- **Files**: `impl/event_bus_impl.go`
- **Changes**:
  - Optimize subscription management:
    - Use efficient data structures for subscriber lookup
    - Support large number of subscribers (1000+)
    - Implement subscription cleanup (remove closed channels)
  - Track subscription metrics (active subscribers, subscription churn)
- **Dependencies**: None
- **Estimated Effort**: 1 day

---

## Epic 8: Ordering Enhancement

**Goal**: Enhance event ordering support for production requirements.

### Section 8.1: Ordering Configuration

#### Subsection 8.1.1: Ordering Configuration Enhancement
- **Files**: `types/config.go`
- **Changes**:
  - Enhance `OrderingConfig`:
    - `Mode OrderingMode` (none, best_effort, strict)
    - `BufferSize int` (default: 100)
    - `Timeout time.Duration` (default: 30s)
    - `PerSourceOrdering bool` (enable per-source ordering, default: true)
  - Add ordering configuration validation
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 8.1.2: Ordering Implementation Enhancement
- **Files**: `impl/ordering_manager.go` (new file, extract from current implementation)
- **Changes**:
  - Extract ordering logic into separate manager:
    - `OrderingManager` struct
    - Per-source sequence number tracking
    - Per-source ordering buffers
    - Timeout handling for strict mode
  - Enhance ordering:
    - Support per-source ordering (already implemented)
    - Support global ordering (if needed)
    - Handle ordering buffer overflow (drop oldest or reject)
  - Add ordering metrics (buffered events, reordered events, timeout events)
- **Dependencies**: 8.1.1
- **Estimated Effort**: 2 days

---

## Epic 9: Retry and Dead Letter Queue Enhancement

**Goal**: Enhance retry logic and dead letter queue for production reliability.

### Section 9.1: Retry Configuration

#### Subsection 9.1.1: Retry Configuration Enhancement
- **Files**: `types/config.go`
- **Changes**:
  - Enhance `RetryConfig`:
    - `MaxRetries int` (default: 3)
    - `InitialBackoff time.Duration` (default: 1s)
    - `MaxBackoff time.Duration` (default: 60s)
    - `BackoffMultiplier float64` (default: 2.0)
    - `RetryInterval time.Duration` (default: 5s)
    - `DeadLetterThreshold int` (move to dead letter after N retries, default: MaxRetries)
  - Add retry configuration validation
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 9.1.2: Retry Manager Enhancement
- **Files**: `impl/retry_manager.go` (new file, extract from current implementation)
- **Changes**:
  - Extract retry logic into separate manager:
    - `RetryManager` struct
    - Failed event tracking
    - Exponential backoff calculation
    - Retry scheduling
  - Enhance retry:
    - Support per-event-type retry policies
    - Support dead letter queue (move events after max retries)
    - Track retry metrics (retry count, success rate, dead letter count)
  - Implement dead letter queue:
    - Store events that exceeded max retries
    - Support dead letter queue queries
    - Support dead letter queue cleanup (retention policy)
- **Dependencies**: 9.1.1
- **Estimated Effort**: 3 days

---

## Epic 10: Documentation and Testing

**Goal**: Add comprehensive documentation and testing following vm-gateway pattern.

### Section 10.1: Documentation

#### Subsection 10.1.1: Package Documentation
- **Files**: `doc.go` (new file)
- **Changes**:
  - Add comprehensive package documentation (similar to vm-gateway/doc.go):
    - Architecture overview
    - Provider-agnostic design
    - Event drop policy
    - Retention and cleanup
    - Ordering guarantees
    - Retry and dead letter queue
    - Configuration examples
    - Usage examples
    - Lifecycle management
    - Health monitoring
  - Document event categorization rules
  - Document storage pressure handling
- **Dependencies**: All epics
- **Estimated Effort**: 1 day

#### Subsection 10.1.2: API Documentation
- **Files**: All interface files
- **Changes**:
  - Add comprehensive method documentation
  - Document error conditions
  - Document return values
  - Add usage examples
  - Document event drop policy
  - Document ordering guarantees
- **Dependencies**: 10.1.1
- **Estimated Effort**: 1 day

### Section 10.2: Testing

#### Subsection 10.2.1: Unit Tests
- **Files**: `*_test.go` files
- **Changes**:
  - Test event drop policy (droppable vs non-droppable)
  - Test retention and cleanup
  - Test storage pressure handling
  - Test ordering (best-effort and strict)
  - Test retry and dead letter queue
  - Test health monitoring
  - Test provider abstraction
- **Dependencies**: All epics
- **Estimated Effort**: 3 days

#### Subsection 10.2.2: Integration Tests
- **Files**: `*_integration_test.go` files
- **Changes**:
  - Test full event lifecycle (publish, persist, subscribe, cleanup)
  - Test event drop policy with real storage pressure
  - Test retention and cleanup with real provider
  - Test ordering with out-of-order events
  - Test retry and dead letter queue with failures
  - Test health monitoring
- **Dependencies**: 10.2.1
- **Estimated Effort**: 2 days

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~1.5 weeks
- **Epics**: 1 (Provider-Agnostic Architecture), 2 (Event Drop Policy)
- **Rationale**: Establishes the architectural foundation and event drop policy

### Phase 2: Core Features (Epics 3, 4)
- **Duration**: ~2 weeks
- **Epics**: 3 (Retention and Cleanup), 4 (Health Monitoring)
- **Rationale**: Implements core production features

### Phase 3: Enhancement (Epics 5, 6, 7, 8, 9)
- **Duration**: ~2.5 weeks
- **Epics**: 5 (Provider Refactoring), 6 (Event Type System), 7 (Performance), 8 (Ordering), 9 (Retry)
- **Rationale**: Enhances existing features and optimizes performance

### Phase 4: Polish (Epic 10)
- **Duration**: ~1 week
- **Epics**: 10 (Documentation and Testing)
- **Rationale**: Completes documentation and testing

**Total Estimated Duration**: ~7 weeks

---

## Migration Notes

### Breaking Changes
- Provider structure changes (metastoragebus → impl/metastorage)
- Configuration structure changes (retention, drop policy configs)
- Lifecycle methods added (Start/Stop)
- Health snapshot API added

### Data Migration
- Existing persisted events will be cleaned up according to retention policy
- No schema migration needed (events stored as JSON in meta-storage)

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Verify event drop policy behavior
- Verify retention and cleanup behavior
- Gradual rollout to production with monitoring
- Monitor event drop rates and storage pressure
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Provider-agnostic architecture implemented (following vm-gateway pattern)
2. ✅ Event drop policy implemented and tested
3. ✅ Retention and cleanup implemented and tested
4. ✅ Storage pressure handling implemented and tested
5. ✅ Health monitoring implemented and tested
6. ✅ Ordering support enhanced
7. ✅ Retry and dead letter queue enhanced
8. ✅ Performance optimized
9. ✅ Comprehensive documentation added
10. ✅ Full test coverage (unit, integration)
11. ✅ Health snapshot API implemented
12. ✅ Event emission implemented

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **Architecture should follow vm-gateway pattern** (but simpler, as event-bus is a simpler service)
- **Provider-agnostic design is mandatory** (support meta-storage now, memory/NATS in future)
- **Event drop policy is critical** (workflow events can be dropped, operational/health events must not be dropped)
- **Retention and cleanup are mandatory** (24 hours retention, 6 hours cleanup)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

