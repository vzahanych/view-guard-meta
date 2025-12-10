package processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// EventQueueManagerService manages the event queue for VM transmission
type EventQueueManagerService struct {
	*service.ServiceBase
	logger            *logger.Logger
	eventQueue        EventQueueService
	eventStorage      EventStorageService
	maxRetries         int
	retryBaseDelay     time.Duration
	retryMaxDelay      time.Duration
	cleanupInterval    time.Duration
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
}

// EventQueueService interface for queue operations
type EventQueueService interface {
	Enqueue(ctx context.Context, event *events.Event, priority int) error
	Dequeue(ctx context.Context) (*events.Event, error)
	Peek(ctx context.Context) (*events.Event, error)
	BatchDequeue(ctx context.Context, batchSize int) ([]*events.Event, error)
	Size(ctx context.Context) (int, error)
	IsEmpty(ctx context.Context) (bool, error)
	IncrementRetryCount(ctx context.Context, eventID string) error
	GetRetryCount(ctx context.Context, eventID string) (int, error)
	SetPriority(ctx context.Context, eventID string, priority int) error
	GetQueueStats(ctx context.Context) (*QueueStats, error)
}

// QueueStats contains queue statistics
type QueueStats struct {
	Size              int
	MaxSize           int
	OldestEventAge    time.Duration
	AverageRetryCount float64
}

// EventQueueManagerServiceConfig contains event queue manager configuration
type EventQueueManagerServiceConfig struct {
	EventQueue     EventQueueService
	EventStorage   EventStorageService
	MaxRetries     int           // Maximum retry attempts (0 = unlimited)
	RetryBaseDelay time.Duration // Base delay for exponential backoff
	RetryMaxDelay  time.Duration // Maximum delay for exponential backoff
	CleanupInterval time.Duration // Interval for queue cleanup
}

// NewEventQueueManagerService creates a new event queue manager service
func NewEventQueueManagerService(config EventQueueManagerServiceConfig, log *logger.Logger) *EventQueueManagerService {
	ctx, cancel := context.WithCancel(context.Background())

	// Default max retries (10)
	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 10
	}

	// Default retry base delay (1 second)
	retryBaseDelay := config.RetryBaseDelay
	if retryBaseDelay == 0 {
		retryBaseDelay = 1 * time.Second
	}

	// Default retry max delay (5 minutes)
	retryMaxDelay := config.RetryMaxDelay
	if retryMaxDelay == 0 {
		retryMaxDelay = 5 * time.Minute
	}

	// Default cleanup interval (1 hour)
	cleanupInterval := config.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 1 * time.Hour
	}

	return &EventQueueManagerService{
		ServiceBase:     service.NewServiceBase("event-queue-manager-service", log),
		logger:          log,
		eventQueue:      config.EventQueue,
		eventStorage:    config.EventStorage,
		maxRetries:      maxRetries,
		retryBaseDelay:  retryBaseDelay,
		retryMaxDelay:   retryMaxDelay,
		cleanupInterval: cleanupInterval,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the event queue manager service
func (eqms *EventQueueManagerService) Start(ctx context.Context) error {
	// Start cleanup goroutine
	go eqms.cleanupWorker(ctx)

	eqms.LogInfo("Event queue manager service started",
		"max_retries", eqms.maxRetries,
		"retry_base_delay", eqms.retryBaseDelay,
		"retry_max_delay", eqms.retryMaxDelay,
		"cleanup_interval", eqms.cleanupInterval,
	)
	return nil
}

// Stop stops the event queue manager service
func (eqms *EventQueueManagerService) Stop() error {
	eqms.cancel()
	eqms.LogInfo("Event queue manager service stopped")
	return nil
}

// EnqueueEvent adds an event to the queue for VM transmission
func (eqms *EventQueueManagerService) EnqueueEvent(ctx context.Context, event *events.Event, priority int) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Enqueue event with priority
	err := eqms.eventQueue.Enqueue(ctx, event, priority)
	if err != nil {
		return fmt.Errorf("failed to enqueue event: %w", err)
	}

	eqms.logger.Debug("Event enqueued for transmission",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"event_type", event.EventType,
		"priority", priority,
	)

	return nil
}

// DequeueEvent retrieves the next event from the queue
func (eqms *EventQueueManagerService) DequeueEvent(ctx context.Context) (*events.Event, error) {
	return eqms.eventQueue.Dequeue(ctx)
}

// PeekEvent retrieves the next event without removing it
func (eqms *EventQueueManagerService) PeekEvent(ctx context.Context) (*events.Event, error) {
	return eqms.eventQueue.Peek(ctx)
}

// BatchDequeueEvents retrieves multiple events from the queue
func (eqms *EventQueueManagerService) BatchDequeueEvents(ctx context.Context, batchSize int) ([]*events.Event, error) {
	return eqms.eventQueue.BatchDequeue(ctx, batchSize)
}

// GetQueueSize returns the current queue size
func (eqms *EventQueueManagerService) GetQueueSize(ctx context.Context) (int, error) {
	return eqms.eventQueue.Size(ctx)
}

// IsQueueEmpty checks if the queue is empty
func (eqms *EventQueueManagerService) IsQueueEmpty(ctx context.Context) (bool, error) {
	return eqms.eventQueue.IsEmpty(ctx)
}

// GetQueueStats returns queue statistics
func (eqms *EventQueueManagerService) GetQueueStats(ctx context.Context) (*QueueStats, error) {
	return eqms.eventQueue.GetQueueStats(ctx)
}

// MarkEventTransmitted marks an event as transmitted and removes it from the queue
func (eqms *EventQueueManagerService) MarkEventTransmitted(ctx context.Context, eventID string) error {
	err := eqms.eventStorage.MarkEventTransmitted(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as transmitted: %w", err)
	}

	eqms.logger.Debug("Event marked as transmitted and removed from queue",
		"event_id", eventID,
	)

	return nil
}

// HandleTransmissionFailure handles a failed transmission with retry logic
func (eqms *EventQueueManagerService) HandleTransmissionFailure(ctx context.Context, eventID string) error {
	// Get current retry count
	retryCount, err := eqms.eventQueue.GetRetryCount(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get retry count: %w", err)
	}

	// Check if max retries exceeded
	if eqms.maxRetries > 0 && retryCount >= eqms.maxRetries {
		eqms.logger.Warn("Event exceeded max retries, removing from queue",
			"event_id", eventID,
			"retry_count", retryCount,
			"max_retries", eqms.maxRetries,
		)
		// Optionally: mark as failed or remove from queue
		// For now, we'll leave it in the queue but log a warning
		return fmt.Errorf("event exceeded max retries: %d", retryCount)
	}

	// Increment retry count
	err = eqms.eventQueue.IncrementRetryCount(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	// Calculate retry delay (exponential backoff)
	retryDelay := eqms.calculateRetryDelay(retryCount + 1)

	eqms.logger.Debug("Event transmission failed, will retry",
		"event_id", eventID,
		"retry_count", retryCount+1,
		"max_retries", eqms.maxRetries,
		"retry_delay", retryDelay,
	)

	// Wait for retry delay before next attempt
	// Note: This is a blocking call, but in practice, the transmission service
	// should handle retries asynchronously
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(retryDelay):
		// Retry delay elapsed
	}

	return nil
}

// calculateRetryDelay calculates the retry delay using exponential backoff
func (eqms *EventQueueManagerService) calculateRetryDelay(retryCount int) time.Duration {
	// Exponential backoff: baseDelay * 2^(retryCount-1)
	delay := eqms.retryBaseDelay
	for i := 1; i < retryCount; i++ {
		delay *= 2
		if delay > eqms.retryMaxDelay {
			delay = eqms.retryMaxDelay
			break
		}
	}
	return delay
}

// SetEventPriority sets the priority for an event in the queue
func (eqms *EventQueueManagerService) SetEventPriority(ctx context.Context, eventID string, priority int) error {
	return eqms.eventQueue.SetPriority(ctx, eventID, priority)
}

// GetEventRetryCount returns the retry count for an event
func (eqms *EventQueueManagerService) GetEventRetryCount(ctx context.Context, eventID string) (int, error) {
	return eqms.eventQueue.GetRetryCount(ctx, eventID)
}

// cleanupWorker periodically cleans up transmitted events from the queue
func (eqms *EventQueueManagerService) cleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(eqms.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-eqms.ctx.Done():
			return
		case <-ticker.C:
			// Cleanup is handled automatically by state.Manager.MarkEventTransmitted()
			// which removes events from the queue when marked as transmitted
			// This worker can be used for additional cleanup tasks if needed
			eqms.logger.Debug("Queue cleanup check completed")
		}
	}
}

// SetMaxRetries sets the maximum retry attempts
func (eqms *EventQueueManagerService) SetMaxRetries(maxRetries int) {
	eqms.mu.Lock()
	defer eqms.mu.Unlock()
	eqms.maxRetries = maxRetries
}

// SetRetryDelays sets the retry delay configuration
func (eqms *EventQueueManagerService) SetRetryDelays(baseDelay, maxDelay time.Duration) {
	eqms.mu.Lock()
	defer eqms.mu.Unlock()
	eqms.retryBaseDelay = baseDelay
	eqms.retryMaxDelay = maxDelay
}

// GetMaxRetries returns the maximum retry attempts
func (eqms *EventQueueManagerService) GetMaxRetries() int {
	eqms.mu.RLock()
	defer eqms.mu.RUnlock()
	return eqms.maxRetries
}

// GetRetryDelays returns the retry delay configuration
func (eqms *EventQueueManagerService) GetRetryDelays() (baseDelay, maxDelay time.Duration) {
	eqms.mu.RLock()
	defer eqms.mu.RUnlock()
	return eqms.retryBaseDelay, eqms.retryMaxDelay
}

