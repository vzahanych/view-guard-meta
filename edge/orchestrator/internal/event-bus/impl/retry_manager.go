package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/zap"
)

// RetryManager manages event retry logic with exponential backoff and dead letter queue support.
// It tracks failed events, schedules retries, and moves events to dead letter queue after max retries.
type RetryManager struct {
	config      *types.RetryConfig
	provider    types.EventBusProvider
	logger      *zap.Logger
	metaStorage metastorage.MetaDataStore // Meta-storage for event status updates

	// Per-event-type retry policies (optional, uses default config if not set)
	mu              sync.RWMutex
	eventTypePolicies map[types.EventType]*types.RetryConfig

	// Metrics
	metricsMu          sync.RWMutex
	retryCount         int64 // Total retry attempts
	successCount       int64 // Successful retries
	deadLetterCount    int64 // Events moved to dead letter queue
	failedRetryCount   int64 // Failed retry attempts
}

// NewRetryManager creates a new retry manager.
func NewRetryManager(config *types.RetryConfig, provider types.EventBusProvider, logger *zap.Logger, metaStorage metastorage.MetaDataStore) *RetryManager {
	if config == nil {
		// Use default config if not provided
		config = &types.RetryConfig{
			MaxRetries:         3,
			InitialBackoff:     1 * time.Second,
			MaxBackoff:         60 * time.Second,
			BackoffMultiplier:  2.0,
			RetryInterval:      5 * time.Second,
			DeadLetterThreshold: 3,
		}
		config.Validate()
	}

	return &RetryManager{
		config:            config,
		provider:          provider,
		logger:            logger,
		metaStorage:       metaStorage,
		eventTypePolicies: make(map[types.EventType]*types.RetryConfig),
	}
}

// SetEventTypePolicy sets a custom retry policy for a specific event type.
func (m *RetryManager) SetEventTypePolicy(eventType types.EventType, policy *types.RetryConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if policy != nil {
		policy.Validate()
		m.eventTypePolicies[eventType] = policy
	} else {
		delete(m.eventTypePolicies, eventType)
	}
}

// GetEventTypePolicy returns the retry policy for a specific event type.
// Returns the default config if no custom policy is set.
func (m *RetryManager) GetEventTypePolicy(eventType types.EventType) *types.RetryConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if policy, ok := m.eventTypePolicies[eventType]; ok {
		return policy
	}
	return m.config
}

// CalculateBackoff calculates the exponential backoff duration for a given retry attempt.
func (m *RetryManager) CalculateBackoff(retryAttempt int, eventType types.EventType) time.Duration {
	policy := m.GetEventTypePolicy(eventType)

	// Calculate exponential backoff: initialBackoff * (multiplier ^ retryAttempt)
	backoff := float64(policy.InitialBackoff) * pow(policy.BackoffMultiplier, float64(retryAttempt))

	// Cap at max backoff
	maxBackoff := float64(policy.MaxBackoff)
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return time.Duration(backoff)
}

// pow calculates base^exponent for float64 values.
func pow(base, exponent float64) float64 {
	result := 1.0
	for i := 0; i < int(exponent); i++ {
		result *= base
	}
	// Handle fractional exponent (simple approximation)
	if exponent != float64(int(exponent)) {
		fractional := exponent - float64(int(exponent))
		result *= 1.0 + fractional*(base-1.0)
	}
	return result
}

// MarkEventFailed marks an event as failed and schedules a retry.
// Returns the next retry time and whether the event should be retried.
func (m *RetryManager) MarkEventFailed(ctx context.Context, eventID string, eventType types.EventType, retryCount int, errorMsg string) (time.Time, bool, error) {
	policy := m.GetEventTypePolicy(eventType)

	// Check if we've exceeded max retries
	if retryCount >= policy.MaxRetries {
		// Move to dead letter queue
		if err := m.MoveToDeadLetter(ctx, eventID, errorMsg); err != nil {
			return time.Time{}, false, fmt.Errorf("failed to move event to dead letter: %w", err)
		}

		m.metricsMu.Lock()
		m.deadLetterCount++
		m.metricsMu.Unlock()

		m.logger.Warn("Event moved to dead letter queue after max retries",
			zap.String("event_id", eventID),
			zap.String("event_type", string(eventType)),
			zap.Int("retry_count", retryCount),
			zap.String("error", errorMsg))
		return time.Time{}, false, nil
	}

	// Calculate next retry time
	backoff := m.CalculateBackoff(retryCount, eventType)
	nextRetryTime := time.Now().Add(backoff)

	// Update event status in meta-storage
	if err := m.updateEventStatus(ctx, eventID, types.EventStatusFailed, retryCount+1, errorMsg, &nextRetryTime); err != nil {
		return time.Time{}, false, fmt.Errorf("failed to update event status: %w", err)
	}

	m.metricsMu.Lock()
	m.retryCount++
	m.metricsMu.Unlock()

	m.logger.Debug("Event marked as failed, scheduled for retry",
		zap.String("event_id", eventID),
		zap.String("event_type", string(eventType)),
		zap.Int("retry_count", retryCount+1),
		zap.Duration("backoff", backoff),
		zap.Time("next_retry_time", nextRetryTime))

	return nextRetryTime, true, nil
}

// MarkEventSucceeded marks an event as successfully processed.
func (m *RetryManager) MarkEventSucceeded(ctx context.Context, eventID string) error {
	if err := m.updateEventStatus(ctx, eventID, types.EventStatusSucceeded, 0, "", nil); err != nil {
		return fmt.Errorf("failed to update event status: %w", err)
	}

	m.metricsMu.Lock()
	m.successCount++
	m.metricsMu.Unlock()

	return nil
}

// ProcessFailedEvents processes failed events that are ready for retry.
// This is called by the retry worker on a schedule.
// processFn receives the eventID and event, and should return an error if processing fails.
func (m *RetryManager) ProcessFailedEvents(ctx context.Context, processFn func(ctx context.Context, eventID string, event types.EventAny) error) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage is required for retry processing")
	}

	// Get failed events from meta-storage
	now := time.Now()
	eventMetadatas, err := m.metaStorage.GetFailedEvents(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to get failed event metadata: %w", err)
	}

	if len(eventMetadatas) == 0 {
		return nil // No events to retry
	}

	// Filter by next retry time
	readyEvents := make([]metastoragetypes.EventBusEventMetadata, 0, len(eventMetadatas))
	for _, metadata := range eventMetadatas {
		// Check if event is ready for retry (NextRetryTime <= now or nil)
		if metadata.NextRetryTime != nil && metadata.NextRetryTime.After(now) {
			// Not ready for retry yet
			continue
		}
		readyEvents = append(readyEvents, metadata)
	}

	if len(readyEvents) == 0 {
		return nil // No events ready for retry
	}

	m.logger.Info("Processing failed events for retry",
		zap.Int("count", len(readyEvents)))

	// Process each failed event
	successCount := 0
	failedCount := 0

	for _, metadata := range readyEvents {
		// Convert EventBusEventMetadata to EventAny
		event, err := m.metadataToEvent(metadata)
		if err != nil {
			m.logger.Warn("Failed to convert event metadata to EventAny",
				zap.String("event_id", metadata.EventID),
				zap.Error(err))
			failedCount++
			continue
		}

		// Try to process the event
		if err := processFn(ctx, metadata.EventID, event); err != nil {
			// Processing failed again, mark as failed with incremented retry count
			eventType := types.EventType(metadata.EventType)
			_, shouldRetry, updateErr := m.MarkEventFailed(ctx, metadata.EventID, eventType, metadata.RetryCount, err.Error())
			if updateErr != nil {
				m.logger.Error("Failed to mark event as failed",
					zap.String("event_id", metadata.EventID),
					zap.Error(updateErr))
			}
			if !shouldRetry {
				// Moved to dead letter queue (already counted in MarkEventFailed)
			} else {
				failedCount++
				m.metricsMu.Lock()
				m.failedRetryCount++
				m.metricsMu.Unlock()
			}
			m.logger.Debug("Event retry failed",
				zap.String("event_id", metadata.EventID),
				zap.String("event_type", string(eventType)),
				zap.Int("retry_count", metadata.RetryCount+1),
				zap.Error(err))
			continue
		}

		// Processing succeeded, mark as succeeded
		if err := m.MarkEventSucceeded(ctx, metadata.EventID); err != nil {
			m.logger.Error("Failed to mark event as succeeded",
				zap.String("event_id", metadata.EventID),
				zap.Error(err))
		} else {
			successCount++
		}
	}

	m.logger.Info("Processed failed events",
		zap.Int("total", len(readyEvents)),
		zap.Int("succeeded", successCount),
		zap.Int("failed", failedCount))

	return nil
}

// GetFailedEventsReadyForRetry returns failed events that are ready for retry (next retry time <= now).
func (m *RetryManager) GetFailedEventsReadyForRetry(ctx context.Context) ([]types.EventAny, error) {
	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage is required for retry management")
	}

	// Get failed events from meta-storage
	now := time.Now()
	eventMetadatas, err := m.metaStorage.GetFailedEvents(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed events: %w", err)
	}

	// Filter by next retry time and convert to EventAny
	result := make([]types.EventAny, 0, len(eventMetadatas))
	for _, eventMetadata := range eventMetadatas {
		// Check if event is ready for retry (NextRetryTime <= now or nil)
		if eventMetadata.NextRetryTime != nil && eventMetadata.NextRetryTime.After(now) {
			// Not ready for retry yet
			continue
		}

		// Convert EventBusEventMetadata to EventAny
		event, err := m.metadataToEvent(eventMetadata)
		if err != nil {
			m.logger.Warn("Failed to convert event metadata to EventAny",
				zap.String("event_id", eventMetadata.EventID),
				zap.Error(err))
			continue
		}

		result = append(result, event)
	}

	return result, nil
}

// MoveToDeadLetter moves an event to the dead letter queue.
func (m *RetryManager) MoveToDeadLetter(ctx context.Context, eventID string, errorMsg string) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage is required for dead letter queue")
	}

	// Move event to dead letter queue via meta-storage
	if err := m.metaStorage.MoveEventToDeadLetter(ctx, eventID); err != nil {
		return fmt.Errorf("failed to move event to dead letter: %w", err)
	}

	m.logger.Info("Event moved to dead letter queue",
		zap.String("event_id", eventID),
		zap.String("error", errorMsg))

	return nil
}

// GetDeadLetterEvents returns events in the dead letter queue.
func (m *RetryManager) GetDeadLetterEvents(ctx context.Context, limit int) ([]types.EventAny, error) {
	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage is required for dead letter queue")
	}

	// Get dead letter events from meta-storage
	eventMetadatas, err := m.metaStorage.GetDeadLetterEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get dead letter events: %w", err)
	}

	// Convert EventBusEventMetadata to EventAny
	result := make([]types.EventAny, 0, len(eventMetadatas))
	for _, eventMetadata := range eventMetadatas {
		event, err := m.metadataToEvent(eventMetadata)
		if err != nil {
			m.logger.Warn("Failed to convert dead letter event metadata to EventAny",
				zap.String("event_id", eventMetadata.EventID),
				zap.Error(err))
			continue
		}
		result = append(result, event)
	}

	return result, nil
}

// GetMetrics returns retry operation metrics.
func (m *RetryManager) GetMetrics() RetryMetrics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()

	// Calculate success rate
	successRate := 0.0
	totalRetries := m.retryCount + m.successCount
	if totalRetries > 0 {
		successRate = float64(m.successCount) / float64(totalRetries) * 100.0
	}

	return RetryMetrics{
		RetryCount:      m.retryCount,
		SuccessCount:    m.successCount,
		FailedRetryCount: m.failedRetryCount,
		DeadLetterCount: m.deadLetterCount,
		SuccessRate:     successRate,
	}
}

// RetryMetrics contains retry operation metrics.
type RetryMetrics struct {
	RetryCount       int64   // Total retry attempts
	SuccessCount     int64   // Successful retries
	FailedRetryCount int64   // Failed retry attempts
	DeadLetterCount  int64   // Events moved to dead letter queue
	SuccessRate      float64 // Success rate percentage (0-100)
}

// updateEventStatus updates the event status in meta-storage.
// This is a helper method that uses meta-storage's UpdateEventProcessingStatus method.
func (m *RetryManager) updateEventStatus(ctx context.Context, eventID string, status types.EventProcessingStatus, retryCount int, lastError string, nextRetryTime *time.Time) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage is required for event status updates")
	}

	return m.metaStorage.UpdateEventProcessingStatus(ctx, eventID, string(status), retryCount, lastError, nextRetryTime)
}

// metadataToEvent converts EventBusEventMetadata to EventAny.
// This is similar to the conversion in metastorage_provider.go.
func (m *RetryManager) metadataToEvent(eventMetadata metastoragetypes.EventBusEventMetadata) (types.EventAny, error) {
	event := types.EventAny{}

	// Extract type
	event.Type = types.EventType(eventMetadata.EventType)

	// Extract source from data map (stored as _source)
	if source, ok := eventMetadata.Data["_source"].(string); ok {
		event.Source = source
	} else {
		return event, fmt.Errorf("missing or invalid source field in event data")
	}

	// Extract timestamp
	event.Timestamp = eventMetadata.Timestamp

	// Extract sequence number from data map if present
	if seqNum, ok := eventMetadata.Data["_sequence_number"].(float64); ok {
		event.SequenceNumber = int64(seqNum)
	} else if seqNum, ok := eventMetadata.Data["_sequence_number"].(int64); ok {
		event.SequenceNumber = seqNum
	} else if seqNum, ok := eventMetadata.Data["_sequence_number"].(int); ok {
		event.SequenceNumber = int64(seqNum)
	}

	// Extract data as JSON (excluding internal fields)
	// Create a copy of the data map without internal fields
	dataMap := make(map[string]interface{})
	for k, v := range eventMetadata.Data {
		if k != "_source" && k != "_sequence_number" {
			dataMap[k] = v
		}
	}

	// Marshal data to JSON
	dataBytes, err := json.Marshal(dataMap)
	if err != nil {
		return event, fmt.Errorf("failed to marshal event data: %w", err)
	}
	event.Data = json.RawMessage(dataBytes)

	return event, nil
}

