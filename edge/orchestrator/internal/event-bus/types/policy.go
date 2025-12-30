package types

// EventCategory represents the category of an event for drop policy enforcement.
// Events are categorized to determine whether they can be dropped during storage pressure.
//
// Categories are used by:
//   - EventDropPolicy: To determine if an event can be dropped
//   - StoragePressureMonitor: To apply drop policy during storage pressure
//   - EventCategoryRegistry: To manage category mappings
//
// See the package documentation for detailed categorization rules.
type EventCategory string

const (
	// EventCategoryWorkflowTrigger represents workflow trigger events that can be dropped.
	// These events can be safely dropped as reconciliation will catch them.
	//
	// Examples:
	//   - Device events (discovered, registered, connected, disconnected)
	//   - Data unit events (requested, saved, set_ready)
	//   - Network events (tunnel.connected, transport.connected)
	//   - Edge events (authenticated, capabilities_received)
	//
	// When storage >90% full, these events are dropped immediately.
	EventCategoryWorkflowTrigger EventCategory = "workflow_trigger"

	// EventCategoryOperationalHealth represents operational/health events that must NOT be dropped.
	// These events are critical for system monitoring and health tracking.
	//
	// Examples:
	//   - Storage events (full, warning, quota_exceeded, cleanup_started/completed)
	//   - Event bus internal events (storage_pressure, event_dropped, health_degraded)
	//
	// When storage >90% full, these events are still attempted to be persisted.
	EventCategoryOperationalHealth EventCategory = "operational_health"

	// EventCategoryCritical represents critical events that must NOT be dropped (highest priority).
	// These events are essential for system operation and recovery.
	//
	// Examples:
	//   - Storage corruption (corruption_detected)
	//
	// When storage >90% full, these events are still attempted to be persisted.
	// If persistence fails, this is a critical error requiring immediate attention.
	EventCategoryCritical EventCategory = "critical"
)

// String returns the string representation of EventCategory.
// Example: EventCategoryWorkflowTrigger.String() returns "workflow_trigger"
func (c EventCategory) String() string {
	return string(c)
}

// IsDroppable returns true if events of this category can be dropped under storage pressure.
// Only EventCategoryWorkflowTrigger events can be dropped.
// Operational/health and critical events must NOT be dropped.
//
// Example:
//   category := EventCategoryWorkflowTrigger
//   if category.IsDroppable() {
//       // Event can be dropped
//   }
func (c EventCategory) IsDroppable() bool {
	return c == EventCategoryWorkflowTrigger
}

// EventDropPolicy defines the event drop policy configuration.
// This policy determines which events can be dropped during storage pressure.
//
// The policy consists of:
//   - CategoryRules: Explicit mappings from event types to categories
//   - DefaultCategory: Category for event types not in CategoryRules
//
// Use DefaultEventDropPolicy() to get the standard policy with all known event types categorized.
//
// Example:
//   policy := DefaultEventDropPolicy()
//   category := policy.GetCategory(EventTypeDeviceDiscovered)
//   if policy.IsDroppable(EventTypeDeviceDiscovered) {
//       // Event can be dropped
//   }
type EventDropPolicy struct {
	// CategoryRules maps event types to categories.
	// If an event type is not in this map, DefaultCategory is used.
	// The map is read-only after policy creation (for thread safety).
	CategoryRules map[EventType]EventCategory

	// DefaultCategory is the default category for unmapped events.
	// This is used when an event type is not found in CategoryRules.
	// Default: EventCategoryWorkflowTrigger (can be dropped)
	DefaultCategory EventCategory
}

// GetCategory returns the category for an event type.
// Returns the mapped category if found in CategoryRules, otherwise returns DefaultCategory.
//
// This method is thread-safe if CategoryRules is not modified after policy creation.
//
// Example:
//   policy := DefaultEventDropPolicy()
//   category := policy.GetCategory(EventTypeDeviceDiscovered)
//   // Returns EventCategoryWorkflowTrigger
func (p *EventDropPolicy) GetCategory(eventType EventType) EventCategory {
	if category, ok := p.CategoryRules[eventType]; ok {
		return category
	}
	return p.DefaultCategory
}

// IsDroppable returns true if an event type can be dropped under storage pressure.
// This is a convenience method that calls GetCategory() and then IsDroppable() on the category.
//
// Example:
//   policy := DefaultEventDropPolicy()
//   if policy.IsDroppable(EventTypeDeviceDiscovered) {
//       // Event can be dropped
//   }
func (p *EventDropPolicy) IsDroppable(eventType EventType) bool {
	category := p.GetCategory(eventType)
	return category.IsDroppable()
}

// DefaultEventDropPolicy returns the default event drop policy with standard categorization rules.
// This categorizes all known event types according to the workflow document specification.
//
// The default policy includes:
//   - Workflow trigger events (can be dropped): device, data_unit, network, edge, workflow, security events
//   - Operational/health events (must NOT be dropped): storage events, event bus internal events
//   - Critical events (must NOT be dropped): storage corruption events
//
// See the package documentation for the complete list of categorized event types.
//
// Example:
//   policy := DefaultEventDropPolicy()
//   category := policy.GetCategory(EventTypeDeviceDiscovered)
//   // Returns EventCategoryWorkflowTrigger
func DefaultEventDropPolicy() *EventDropPolicy {
	return &EventDropPolicy{
		CategoryRules: map[EventType]EventCategory{
			// Workflow triggers (can be dropped) - Device events
			EventTypeDeviceDiscovered:   EventCategoryWorkflowTrigger,
			EventTypeDeviceRegistered:   EventCategoryWorkflowTrigger,
			EventTypeDeviceConnected:    EventCategoryWorkflowTrigger,
			EventTypeDeviceDisconnected: EventCategoryWorkflowTrigger,
			EventTypeDeviceCaptureFrame: EventCategoryWorkflowTrigger,

			// Workflow triggers (can be dropped) - Data unit events
			EventTypeDataUnitRequested: EventCategoryWorkflowTrigger,
			EventTypeDataUnitSaved:     EventCategoryWorkflowTrigger,
			EventTypeDataUnitSetReady:  EventCategoryWorkflowTrigger,
			EventTypeDataUnitUpdated:   EventCategoryWorkflowTrigger,
			EventTypeDataUnitDeleted:   EventCategoryWorkflowTrigger,

			// Workflow triggers (can be dropped) - Raw device data events
			EventTypeRawDeviceDataFrameReceived: EventCategoryWorkflowTrigger,
			EventTypeRawDeviceDataClipRecorded:  EventCategoryWorkflowTrigger,

			// Workflow triggers (can be dropped) - Network events
			EventTypeNetworkTunnelConnected:      EventCategoryWorkflowTrigger,
			EventTypeNetworkTunnelDisconnected:  EventCategoryWorkflowTrigger,
			EventTypeNetworkTransportConnected:   EventCategoryWorkflowTrigger,
			EventTypeNetworkTransportDisconnected: EventCategoryWorkflowTrigger,

			// Workflow triggers (can be dropped) - Edge/VM events
			EventTypeEdgeAuthenticated:        EventCategoryWorkflowTrigger,
			EventTypeEdgeCapabilitiesReceived: EventCategoryWorkflowTrigger,

			// Workflow triggers (can be dropped) - Workflow events
			EventTypeWorkflowDeviceDiscover:    EventCategoryWorkflowTrigger,
			EventTypeWorkflowAIStartProcessing: EventCategoryWorkflowTrigger,

			// Workflow triggers (can be dropped) - Security events (workflow-related)
			EventTypeSecurityEventCreated: EventCategoryWorkflowTrigger,

			// Operational/health (must NOT be dropped) - Storage events
			EventTypeStorageFull:                EventCategoryOperationalHealth,
			EventTypeStorageWarning:              EventCategoryOperationalHealth,
			EventTypeStorageQuotaExceeded:        EventCategoryOperationalHealth,
			EventTypeStorageCleanupStarted:       EventCategoryOperationalHealth,
			EventTypeStorageCleanupCompleted:     EventCategoryOperationalHealth,
			EventTypeStorageSchemaMigrationStarted:   EventCategoryOperationalHealth,
			EventTypeStorageSchemaMigrationCompleted: EventCategoryOperationalHealth,

			// Critical (must NOT be dropped, highest priority) - Storage corruption
			EventTypeStorageCorruptionDetected: EventCategoryCritical,

			// Event bus internal events (operational health - must NOT be dropped)
			// These are emitted by the event bus itself for self-monitoring
			EventType("event_bus.storage_pressure"):  EventCategoryOperationalHealth,
			EventType("event_bus.event_dropped"):     EventCategoryOperationalHealth,
			EventType("event_bus.cleanup_started"):   EventCategoryOperationalHealth,
			EventType("event_bus.cleanup_completed"): EventCategoryOperationalHealth,
			EventType("event_bus.health_degraded"):   EventCategoryOperationalHealth,

			// Operational/health (must NOT be dropped) - Audit log events
			EventTypeAuditLogQueueFull:      EventCategoryOperationalHealth,
			EventTypeAuditLogQueueResumed:   EventCategoryOperationalHealth,
			EventTypeAuditLogSyncFailed:     EventCategoryOperationalHealth,
			EventTypeAuditLogSyncSucceeded:  EventCategoryOperationalHealth,
			EventTypeAuditLogCleanupStarted: EventCategoryOperationalHealth,
			EventTypeAuditLogCleanupCompleted: EventCategoryOperationalHealth,
			EventTypeAuditLogHealthDegraded: EventCategoryOperationalHealth,

			// Critical (must NOT be dropped, highest priority) - Audit log tamper detection
			EventTypeAuditLogTamperDetected: EventCategoryCritical,
		},
		DefaultCategory: EventCategoryWorkflowTrigger, // Default: workflow triggers can be dropped
	}
}

