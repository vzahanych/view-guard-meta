package types

import "time"

// HealthStatus represents the health status of the event bus service.
// This follows the vm-gateway pattern for health status reporting.
//
// Health status hierarchy (from best to worst):
//   - Healthy: Normal operation
//   - Warning: Storage usage 80-90% (monitoring)
//   - StoragePressure: Storage >90% full (dropping events)
//   - Degraded: Provider errors or high drop rate (critical issues)
type HealthStatus int

const (
	// HealthStatusHealthy indicates the event bus is healthy.
	// All systems are operating normally, storage usage is <80%.
	HealthStatusHealthy HealthStatus = iota

	// HealthStatusWarning indicates the event bus is in a warning state.
	// Storage usage is 80-90% full. Events are still being persisted normally.
	// This is a monitoring state - no action required yet.
	HealthStatusWarning

	// HealthStatusStoragePressure indicates storage pressure is active.
	// Storage usage is >90% full. Droppable events (workflow triggers) are being dropped.
	// Non-droppable events (operational/health/critical) are still being persisted.
	// This requires attention - consider increasing storage or adjusting retention policy.
	HealthStatusStoragePressure

	// HealthStatusDegraded indicates the event bus is degraded.
	// Provider errors are occurring or drop rate is very high.
	// This is a critical state requiring immediate attention.
	HealthStatusDegraded
)

// String returns the string representation of HealthStatus.
// Returns: "healthy", "warning", "storage_pressure", or "degraded".
//
// Example:
//   status := HealthStatusHealthy
//   fmt.Println(status.String()) // "healthy"
func (s HealthStatus) String() string {
	switch s {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusWarning:
		return "warning"
	case HealthStatusStoragePressure:
		return "storage_pressure"
	case HealthStatusDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// EventBusHealth contains comprehensive health information about the event bus service.
// This follows the vm-gateway pattern for health snapshots.
//
// This structure is returned by EventBus.HealthSnapshot() and provides a complete
// view of the event bus health for monitoring and debugging.
//
// Example:
//   health := bus.HealthSnapshot()
//   if health.Status == HealthStatusStoragePressure {
//       // Handle storage pressure
//   }
type EventBusHealth struct {
	// Status is the overall health status.
	// This is the primary indicator of event bus health.
	Status HealthStatus

	// StoragePressure indicates if storage pressure is currently detected (>90% full).
	// When true, droppable events are being dropped.
	StoragePressure bool

	// StorageUsagePercent is the current storage usage percentage (0-100).
	// This is calculated from meta-storage quota: (used / limit) * 100.
	StorageUsagePercent float64

	// EventsPublished is the total count of events published since service start.
	// This includes both persisted and dropped events.
	EventsPublished int64

	// EventsDropped is the total count of events dropped, grouped by category.
	// This map contains counts for each EventCategory (workflow_trigger, operational_health, critical).
	// Only workflow_trigger events should have non-zero counts (they can be dropped).
	EventsDropped map[EventCategory]int64

	// EventsPersisted is the total count of events successfully persisted to storage.
	// This excludes dropped events.
	EventsPersisted int64

	// ActiveSubscribers is the current number of active subscribers.
	// This includes both Subscribe() and SubscribeAll() subscribers.
	ActiveSubscribers int

	// LastCleanupTime is when the last retention cleanup was performed.
	// This is zero if no cleanup has run yet.
	LastCleanupTime time.Time

	// CleanupStats contains statistics from the last cleanup operation.
	// This is nil if no cleanup has run yet or if cleanup hasn't completed.
	CleanupStats *CleanupStats

	// ProviderHealth is the provider-specific health status string.
	// This is a human-readable string from the provider's health check.
	ProviderHealth string

	// ProviderStatus contains provider-specific status details.
	// This is a flexible map for provider-specific information (e.g., connection status, metrics).
	ProviderStatus map[string]interface{}
}

// CleanupStats contains statistics about cleanup operations.
// This is populated after each retention cleanup cycle.
//
// Example:
//   health := bus.HealthSnapshot()
//   if health.CleanupStats != nil {
//       fmt.Printf("Deleted %d events, freed %d bytes in %v\n",
//           health.CleanupStats.EventsDeleted,
//           health.CleanupStats.SpaceFreedBytes,
//           health.CleanupStats.Duration)
//   }
type CleanupStats struct {
	// EventsDeleted is the number of events deleted in the last cleanup.
	// This is the count of expired events that were removed.
	EventsDeleted int64

	// SpaceFreedBytes is the approximate space freed in bytes.
	// This is an approximation - exact space freed depends on storage provider.
	// May be 0 if the provider doesn't support space calculation.
	SpaceFreedBytes int64

	// Duration is how long the cleanup operation took.
	// This includes querying for expired events and batch deletion.
	Duration time.Duration
}

