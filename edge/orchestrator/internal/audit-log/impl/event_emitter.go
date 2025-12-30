package impl

import (
	"time"

	eventbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// emitEvent is a helper function to emit events via event bus.
// It safely handles the case where event bus is not available (optional dependency).
func (a *AuditLogImpl) emitEvent(eventType eventbustypes.EventType, data eventbustypes.AuditLogEventData) {
	if a.eventBus == nil {
		// Event bus is optional - silently skip if not available
		return
	}

	// Create typed event
	event := eventbustypes.Event[eventbustypes.AuditLogEventData]{
		Type:      eventType,
		Source:    "audit-log",
		Timestamp: time.Now(),
		Data:      data,
	}

	// Convert to EventAny and publish
	eventAny, err := eventbustypes.ToEventAny(event)
	if err != nil {
		// Log error but don't fail the operation
		if a.logger != nil {
			a.logger.Warn("Failed to convert event to EventAny",
				zap.String("event_type", string(eventType)),
				zap.Error(err))
		}
		return
	}

	// Publish event (non-blocking, may return error if storage pressure)
	if err := a.eventBus.Publish(eventAny); err != nil {
		// Log error but don't fail the operation (event bus handles storage pressure)
		if a.logger != nil {
			a.logger.Warn("Failed to publish event",
				zap.String("event_type", string(eventType)),
				zap.Error(err))
		}
	}
}

// emitQueueFullEvent emits an audit_log.queue_full event.
func (a *AuditLogImpl) emitQueueFullEvent(queueDepth, queueMaxSize int, queueUsagePercent float64) {
	a.emitEvent(eventbustypes.EventTypeAuditLogQueueFull, eventbustypes.AuditLogEventData{
		QueueDepth:        &queueDepth,
		QueueMaxSize:      &queueMaxSize,
		QueueUsagePercent: &queueUsagePercent,
	})
}

// emitQueueResumedEvent emits an audit_log.queue_resumed event.
func (a *AuditLogImpl) emitQueueResumedEvent(queueDepth, queueMaxSize int, queueUsagePercent float64) {
	a.emitEvent(eventbustypes.EventTypeAuditLogQueueResumed, eventbustypes.AuditLogEventData{
		QueueDepth:        &queueDepth,
		QueueMaxSize:      &queueMaxSize,
		QueueUsagePercent: &queueUsagePercent,
	})
}

// emitSyncFailedEvent emits an audit_log.sync_failed event.
func (a *AuditLogImpl) emitSyncFailedEvent(errorMessage string) {
	a.emitEvent(eventbustypes.EventTypeAuditLogSyncFailed, eventbustypes.AuditLogEventData{
		ErrorMessage: &errorMessage,
	})
}

// emitSyncSucceededEvent emits an audit_log.sync_succeeded event.
func (a *AuditLogImpl) emitSyncSucceededEvent(entriesSynced int64, syncDuration time.Duration) {
	durationStr := syncDuration.String()
	a.emitEvent(eventbustypes.EventTypeAuditLogSyncSucceeded, eventbustypes.AuditLogEventData{
		EntriesSynced: &entriesSynced,
		SyncDuration:  &durationStr,
	})
}

// emitTamperDetectedEvent emits an audit_log.tamper_detected event.
func (a *AuditLogImpl) emitTamperDetectedEvent(brokenLinks, tamperIndicators, verifiedEntries, totalEntries int) {
	a.emitEvent(eventbustypes.EventTypeAuditLogTamperDetected, eventbustypes.AuditLogEventData{
		BrokenLinks:      &brokenLinks,
		TamperIndicators: &tamperIndicators,
		VerifiedEntries:  &verifiedEntries,
		TotalEntries:     &totalEntries,
	})
}

// emitCleanupStartedEvent emits an audit_log.cleanup_started event.
func (a *AuditLogImpl) emitCleanupStartedEvent() {
	a.emitEvent(eventbustypes.EventTypeAuditLogCleanupStarted, eventbustypes.AuditLogEventData{})
}

// emitCleanupCompletedEvent emits an audit_log.cleanup_completed event.
func (a *AuditLogImpl) emitCleanupCompletedEvent(entriesDeleted, entriesSkipped int64, cleanupDuration time.Duration) {
	durationStr := cleanupDuration.String()
	a.emitEvent(eventbustypes.EventTypeAuditLogCleanupCompleted, eventbustypes.AuditLogEventData{
		EntriesDeleted:  &entriesDeleted,
		EntriesSkipped:  &entriesSkipped,
		CleanupDuration: &durationStr,
	})
}

// emitHealthDegradedEvent emits an audit_log.health_degraded event.
func (a *AuditLogImpl) emitHealthDegradedEvent(healthStatus, healthReason string) {
	a.emitEvent(eventbustypes.EventTypeAuditLogHealthDegraded, eventbustypes.AuditLogEventData{
		HealthStatus: &healthStatus,
		HealthReason: &healthReason,
	})
}

