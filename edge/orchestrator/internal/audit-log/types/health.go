package types

import "time"

// HealthStatus represents the health status of the audit log service.
type HealthStatus int

const (
	// HealthStatusHealthy indicates the audit log service is healthy and operating normally.
	HealthStatusHealthy HealthStatus = iota

	// HealthStatusWarning indicates the audit log service is in a warning state (e.g., queue >80% full).
	// The service continues to operate but may be degraded.
	HealthStatusWarning

	// HealthStatusQueueFull indicates the sync queue is 100% full and operations are paused.
	// This is a critical state that requires immediate attention.
	HealthStatusQueueFull

	// HealthStatusSyncFailed indicates sync failures have been detected.
	// The service continues to operate but entries are accumulating in the queue.
	HealthStatusSyncFailed

	// HealthStatusDegraded indicates the audit log service is degraded due to hash chain issues or provider errors.
	// The service may be partially functional but requires attention.
	HealthStatusDegraded
)

// String returns the string representation of HealthStatus.
func (s HealthStatus) String() string {
	switch s {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusWarning:
		return "warning"
	case HealthStatusQueueFull:
		return "queue_full"
	case HealthStatusSyncFailed:
		return "sync_failed"
	case HealthStatusDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// AuditLogHealth represents the health status of the audit log service.
// This follows the vm-gateway/meta-storage/object-storage pattern for health snapshots.
type AuditLogHealth struct {
	// Status is the overall health status.
	Status HealthStatus

	// QueueDepth is the current number of entries in the sync queue.
	QueueDepth int

	// QueueMaxSize is the maximum size of the sync queue (default: 100,000 records).
	QueueMaxSize int

	// QueueUsagePercent is the percentage of queue capacity used (0-100).
	QueueUsagePercent float64

	// IsPaused indicates whether operations are paused due to queue being full.
	IsPaused bool

	// LastSyncTime is when the last sync to VM was performed.
	LastSyncTime time.Time

	// LastSyncSuccess indicates whether the last sync attempt was successful.
	LastSyncSuccess bool

	// SyncFailures is the count of recent sync failures.
	SyncFailures int

	// EntriesLogged is the total count of audit log entries logged since service start.
	EntriesLogged int64

	// EntriesSynced is the total count of audit log entries successfully synced to VM.
	EntriesSynced int64

	// EntriesPending is the total count of audit log entries pending sync.
	EntriesPending int64

	// HashChainIntegrity indicates whether the hash chain integrity is intact.
	HashChainIntegrity bool

	// ProviderHealth is the provider-specific health status string.
	// Values: "healthy", "degraded", "unhealthy".
	ProviderHealth string

	// ProviderStatus contains provider-specific status details (optional).
	// This can include connection counts, transaction stats, or other provider-specific metrics.
	// This field follows the meta-storage health pattern for extensibility.
	ProviderStatus map[string]interface{} `json:"provider_status,omitempty"`
}

