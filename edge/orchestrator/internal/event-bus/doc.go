/*
Package eventbus provides a unified, provider-agnostic interface for application event bus operations.

The Event Bus service manages event publishing, subscription, persistence, and delivery across the Edge orchestrator.
It abstracts away the details of storage provider implementations, allowing the system to work with any
storage backend (meta-storage, memory, NATS, etc.).

# Architecture Overview

The event bus service follows a provider-agnostic architecture pattern similar to vm-gateway:

	┌─────────────────────────────────────────────────────────┐
	│              EventBus Interface                          │
	│  (Unified API for Event Publishing and Subscription)    │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│ Meta-Storage       │         │  Memory Provider     │
	│ Provider           │         │  (Future)            │
	│ (Current)          │         │                      │
	│                    │         │                      │
	│ - Persistence      │         │ - In-memory only     │
	│ - Retry logic      │         │ - Testing            │
	│ - Dead letter      │         │                      │
	└────────────────────┘         └──────────────────────┘

The service is composed of:
  - EventBus interface: High-level operations (publish, subscribe, lifecycle)
  - EventBusProvider interface: Low-level persistence operations
  - Provider implementations: Meta-storage (current), Memory, NATS (future)
  - Managers: Retention, Storage Pressure Monitor, Event Category Registry

# Provider-Agnostic Design

The event bus is designed to be provider-agnostic:

  - Storage providers: Meta-storage (current), Memory (testing), NATS (future)
  - Event types: All application events (device, model, storage, network, etc.)
  - Ordering modes: None, best-effort, strict

This allows the system to:
  - Switch storage providers without changing application code
  - Support different deployment scenarios (production with persistence, testing with memory)
  - Add new providers by implementing the EventBusProvider interface

The EventBusProvider interface defines low-level operations:
  - PersistEvent, LoadEvent, ListEvents, DeleteEvent
  - DeleteExpiredEvents, GetEventCount
  - HealthCheck

The EventBus interface defines high-level operations:
  - Publish, Subscribe, SubscribeAll, Unsubscribe
  - Start, Stop (lifecycle)
  - HealthSnapshot

# Event Drop Policy

The event bus implements an event drop policy to handle storage pressure. This policy categorizes events into three categories based on their criticality and whether they can be safely dropped during storage pressure.

## Event Categorization Rules

Events are automatically categorized using the EventCategoryRegistry, which maps event types to categories. The categorization follows these rules:

### 1. Workflow Trigger Events (EventCategoryWorkflowTrigger)
**Can be dropped** - These events trigger workflows but can be safely dropped as reconciliation will catch them.

**Device Events:**
  - device.discovered, device.registered, device.connected, device.disconnected
  - device.capture_frame

**Data Unit Events:**
  - data_unit.requested, data_unit.saved, data_unit.set_ready
  - data_unit.updated, data_unit.deleted

**Raw Device Data Events:**
  - raw_device_data.frame_received, raw_device_data.clip_recorded

**Network Events:**
  - network.tunnel.connected, network.tunnel.disconnected
  - network.transport.connected, network.transport.disconnected

**Edge/VM Events:**
  - edge.authenticated, edge.capabilities_received

**Workflow Events:**
  - workflow.device.discover, workflow.ai.start_processing

**Security Events:**
  - security.event.created (workflow-related)

### 2. Operational/Health Events (EventCategoryOperationalHealth)
**Must NOT be dropped** - These events are critical for system monitoring and health tracking.

**Storage Events:**
  - storage.full, storage.warning, storage.quota_exceeded
  - storage.cleanup_started, storage.cleanup_completed
  - storage.schema_migration_started, storage.schema_migration_completed

**Event Bus Internal Events:**
  - event_bus.storage_pressure, event_bus.event_dropped
  - event_bus.cleanup_started, event_bus.cleanup_completed
  - event_bus.health_degraded

### 3. Critical Events (EventCategoryCritical)
**Must NOT be dropped (highest priority)** - These events are essential for system operation and recovery.

**Storage Corruption:**
  - storage.corruption_detected

## Storage Pressure Handling

The event bus monitors storage usage through the StoragePressureMonitor, which checks meta-storage quota status every 5 minutes (configurable).

### Storage Pressure Detection

Storage pressure is detected when storage usage exceeds the configured threshold (default: 90%). The monitor:
  1. Queries meta-storage HealthSnapshot to get quota status
  2. Calculates usage percentage: (used / limit) * 100
  3. Compares against threshold (default: 90%)
  4. Updates cached status for fast lookups
  5. Emits events when pressure state changes

### Event Drop Behavior

When storage >90% full (or configured threshold):

**For Droppable Events (Workflow Triggers):**
  - Event is dropped immediately without persistence attempt
  - Warning is logged with event type and category
  - ErrStoragePressure error is returned to caller
  - Operational event `event_bus.event_dropped` is emitted (if possible)
  - Event is NOT delivered to subscribers

**For Non-Droppable Events (Operational/Health/Critical):**
  - Event persistence is attempted (may still fail if storage is completely full)
  - If persistence fails, ErrEventDropped error is returned (critical error)
  - Event is still delivered to subscribers (if persistence succeeds)
  - Error is logged as critical issue requiring immediate attention

### Dynamic Category Registration

Event categories can be registered or updated at runtime using EventCategoryRegistry:
  - RegisterEventCategory(): Register or update category for an event type
  - SetDefaultCategory(): Set default category for unmapped events
  - GetCategory(): Get category for an event type
  - IsDroppable(): Check if an event type can be dropped

**Warning:** Changing critical or operational events to workflow trigger category will log a warning, as this may cause important events to be dropped.

# Retention and Cleanup

The event bus implements automatic retention and cleanup to prevent unbounded storage growth. The RetentionManager handles periodic cleanup of expired events.

## Retention Policy

**Retention Period**: 24 hours (configurable via RetentionConfig.RetentionHours)
  - Events older than the retention period are considered expired
  - Expired events are deleted during cleanup cycles
  - Retention is enforced per-event based on event timestamp

## Cleanup Process

**Cleanup Interval**: 6 hours (configurable via RetentionConfig.CleanupIntervalHours)
  - Cleanup runs as a background task started during EventBus.Start()
  - Each cleanup cycle:
    1. Calculates cutoff time: now - retention_period
    2. Queries provider for events older than cutoff time
    3. Deletes events in batches (to avoid overwhelming storage)
    4. Logs cleanup statistics
    5. Emits `event_bus.cleanup_completed` event with statistics

**Cleanup Batch Size**: 1000 events per batch (configurable via RetentionConfig.CleanupBatchSize)
  - Events are deleted in batches to avoid:
    - Overwhelming the storage provider
    - Long-running transactions
    - Memory exhaustion
  - If more events need deletion, multiple batches are processed

## Cleanup Statistics

Each cleanup operation tracks:
  - EventsDeleted: Number of events deleted
  - SpaceFreedBytes: Approximate space freed (if provider supports it)
  - Duration: How long the cleanup took
  - LastCleanupTime: Timestamp of last cleanup (available in HealthSnapshot)

## Error Handling

If cleanup fails:
  - Error is logged but does not stop the service
  - Next cleanup cycle will attempt again
  - Health status may be degraded if cleanup fails repeatedly

# Ordering Guarantees

The event bus supports different ordering modes:

  - **None**: No ordering guarantees (fastest)
  - **Best Effort**: Reorder events if possible (balanced)
  - **Strict**: Buffer and wait for missing sequences (slowest, strongest guarantee)

Ordering is per-source (events from the same source are ordered).

# Retry and Dead Letter Queue

The event bus implements retry logic for failed event processing:

  - **Max Retries**: 3 (configurable)
  - **Exponential Backoff**: Initial 1s, max 60s, multiplier 2.0 (configurable)
  - **Dead Letter Queue**: Events that exceed max retries are moved to dead letter queue

# Configuration

The event bus uses a provider-agnostic configuration structure.

## Basic Configuration (Meta-Storage Provider)

	provider: metastorage
	buffer_size: 100

## Advanced Configuration with Retry and Ordering

	provider: metastorage
	buffer_size: 100
	retry_config:
	  max_retries: 3
	  initial_backoff: 1s
	  max_backoff: 60s
	  backoff_multiplier: 2.0
	  retry_interval: 5s
	ordering_config:
	  mode: best_effort
	  buffer_size: 100
	  timeout: 30s
	  per_source_ordering: true

## Configuration with Retention and Drop Policy

	provider: metastorage
	buffer_size: 100
	retention_config:
	  retention_hours: 24
	  cleanup_interval_hours: 6
	  cleanup_batch_size: 1000
	drop_policy_config:
	  storage_pressure_threshold: 90
	  default_category: workflow_trigger

# Usage Examples

## Basic Usage with Dependency Injection (Fx)

	import (
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
		"go.uber.org/fx"
	)

	// In your Fx module
	var Module = fx.Module("event-bus",
		fx.Provide(eventbus.EventBusProvider),
	)

	// The service will be automatically started and stopped by Fx

## Manual Creation

	import (
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
		metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	)

	config := &types.EventBusConfig{
		Provider:   "metastorage",
		BufferSize: 100,
	}
	config.Validate()

	bus, err := eventbus.NewEventBus(ctx, config, logger, &metaStore)
	if err != nil {
		log.Fatal(err)
	}

	// Start the service
	if err := bus.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer bus.Stop(ctx)

## Publishing Events

	// Publish a typed event
	event := types.Event[types.DeviceDiscoveredEventData]{
		Type:      types.EventTypeDeviceDiscovered,
		Source:    "device-manager",
		Timestamp: time.Now(),
		Data: types.DeviceDiscoveredEventData{
			DeviceID: "camera-001",
			Name:     "Front Door Camera",
			Type:     "camera",
		},
	}
	if err := eventbus.PublishTyped(bus, event); err != nil {
		log.Error("Failed to publish event", zap.Error(err))
	}

	// Publish an untyped event
	eventAny := types.EventAny{
		Type:      types.EventTypeDeviceDiscovered,
		Source:    "device-manager",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"device_id":"camera-001"}`),
	}
	bus.Publish(eventAny)

## Subscribing to Events

	// Subscribe to specific event type
	ch := bus.Subscribe(types.EventTypeDeviceDiscovered)
	for event := range ch {
		log.Info("Received event", zap.String("type", string(event.Type)))
	}

	// Subscribe to all events
	chAll := bus.SubscribeAll()
	for event := range chAll {
		log.Info("Received event", zap.String("type", string(event.Type)))
	}

## Health Monitoring

	// Get health snapshot
	health := bus.HealthSnapshot()
	log.Info("Event bus health",
		zap.String("status", health.Status.String()),
		zap.Float64("storage_usage_percent", health.StorageUsagePercent),
		zap.Int64("events_published", health.EventsPublished),
		zap.Int64("events_dropped", health.EventsDropped[types.EventCategoryWorkflowTrigger]),
	)

# Error Handling

The event bus uses sentinel errors for programmatic error handling:

	import (
		"errors"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	)

	if err := bus.Start(ctx); err != nil {
		if errors.Is(err, types.ErrAlreadyStarted) {
			// Handle already started error
		}
	}

	if err := bus.Publish(event); err != nil {
		if errors.Is(err, types.ErrStoragePressure) {
			// Handle storage pressure (events may be dropped)
		}
		if errors.Is(err, types.ErrEventDropped) {
			// Handle non-droppable event drop (critical error)
		}
	}

# Observability

The event bus emits operational events for monitoring:

  - `event_bus.storage_pressure`: Emitted when storage >90% full
  - `event_bus.event_dropped`: Emitted when an event is dropped (with category)
  - `event_bus.cleanup_started`: Emitted when retention cleanup starts
  - `event_bus.cleanup_completed`: Emitted when retention cleanup completes
  - `event_bus.health_degraded`: Emitted when health issues are detected

The event bus also tracks operational metrics:
  - Events published per second (by event type)
  - Events dropped per second (by category)
  - Events persisted per second
  - Subscriber count over time
  - Publish latency (P50, P95, P99)
  - Storage usage over time
  - Cleanup statistics

# Lifecycle Management

The event bus follows the vm-gateway pattern for lifecycle management:

  - **Start()**: Initializes provider, starts background tasks (retention cleanup, storage pressure monitor, health check)
  - **Stop()**: Stops background tasks gracefully, closes provider connections, flushes pending operations

The service owns the lifecycle of its sub-components (providers, managers).

# Dependencies

The event bus depends on:
  - **meta-storage**: For event persistence (when using meta-storage provider)
  - **logger**: For structured logging (zap.Logger)

The meta-storage service must be started before the event bus.

# Notes

- **NO BACKWARD COMPATIBILITY**: This is a complete refactoring with breaking changes expected and encouraged
- **BREAKING CHANGES ARE ACCEPTABLE**: All API changes, type changes, and method removals are allowed to establish production best practices
- **META-STORAGE DEPENDENCY**: Only `meta-storage` has been properly refactored so far. Event-bus refactoring depends on the refactored meta-storage API (structured types: `EventBusEventMetadata`, `EventBusFilters`).
- **Provider-agnostic design is mandatory** (support meta-storage now, memory/NATS in future)
- **Event drop policy is critical** (workflow events can be dropped, operational/health events must not be dropped)
- **Retention and cleanup are mandatory** (24 hours retention, 6 hours cleanup)
*/
package eventbus

