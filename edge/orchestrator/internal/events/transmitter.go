package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// Transmitter handles event transmission with retry logic
type Transmitter struct {
	*service.ServiceBase
	queue        *Queue
	storage      *Storage
	logger       *logger.Logger
	config       TransmitterConfig
	transmitting bool
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// TransmitterConfig contains configuration for the transmitter
type TransmitterConfig struct {
	Queue              *Queue
	Storage            *Storage
	BatchSize          int
	TransmissionInterval time.Duration
	MaxRetries         int
	RetryDelay         time.Duration
	OnTransmit         func(ctx context.Context, events []*Event) error // Transmission callback (will be gRPC client in Epic 1.6)
}

// NewTransmitter creates a new event transmitter
func NewTransmitter(config TransmitterConfig, log *logger.Logger) *Transmitter {
	ctx, cancel := context.WithCancel(context.Background())

	// Set defaults
	if config.BatchSize <= 0 {
		config.BatchSize = 10
	}
	if config.TransmissionInterval == 0 {
		config.TransmissionInterval = 5 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}

	return &Transmitter{
		ServiceBase: service.NewServiceBase("event-transmitter", log),
		queue:        config.Queue,
		storage:      config.Storage,
		logger:       log,
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Name returns the service name
func (t *Transmitter) Name() string {
	return "event-transmitter"
}

// Start starts the transmitter service
func (t *Transmitter) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.transmitting {
		return nil // Already running
	}

	t.transmitting = true
	t.GetStatus().SetStatus(service.StatusRunning)

	// Start transmission loop
	go t.transmissionLoop(ctx)

	t.LogInfo("Event transmitter started")
	return nil
}

// Stop stops the transmitter service
func (t *Transmitter) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.transmitting {
		return nil
	}

	t.cancel()
	t.transmitting = false
	t.GetStatus().SetStatus(service.StatusStopped)

	t.LogInfo("Event transmitter stopped")
	return nil
}

// transmissionLoop continuously processes the event queue
func (t *Transmitter) transmissionLoop(ctx context.Context) {
	ticker := time.NewTicker(t.config.TransmissionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			// Process queue
			err := t.processQueue(ctx)
			if err != nil {
				t.LogError("Failed to process queue", err)
			}
		}
	}
}

// processQueue processes events from the queue
func (t *Transmitter) processQueue(ctx context.Context) error {
	// Check if queue is empty
	isEmpty, err := t.queue.IsEmpty(ctx)
	if err != nil {
		return fmt.Errorf("failed to check queue: %w", err)
	}
	if isEmpty {
		return nil // Nothing to process
	}

	// Get batch of events
	events, err := t.queue.BatchDequeue(ctx, t.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to dequeue events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	t.LogDebug("Processing event batch", "count", len(events))

	// Attempt transmission
		err = t.transmitEvents(ctx, events)
	if err != nil {
		t.LogError("Transmission failed, will retry", err, "event_count", len(events))

		// Increment retry count for all events
		for _, event := range events {
			retryErr := t.queue.IncrementRetryCount(ctx, event.ID)
			if retryErr != nil {
				t.LogError("Failed to increment retry count", retryErr, "event_id", event.ID)
			}

			// Check if max retries exceeded
			retryCount, retryErr := t.queue.GetRetryCount(ctx, event.ID)
			if retryErr != nil {
				t.LogError("Failed to get retry count", retryErr, "event_id", event.ID)
				continue
			}

			if retryCount >= t.config.MaxRetries {
				t.LogError(
					"Event exceeded max retries, marking as failed",
					fmt.Errorf("max retries exceeded: %d", retryCount),
					"event_id", event.ID,
					"retry_count", retryCount,
				)
				// Mark as transmitted to remove from queue (or could mark as failed)
				// For now, we'll mark as transmitted to prevent infinite retries
				markErr := t.storage.MarkEventTransmitted(ctx, event.ID)
				if markErr != nil {
					t.LogError("Failed to mark event as transmitted", markErr, "event_id", event.ID)
				}
			}
		}

		return err
	}

	// Transmission successful - mark all events as transmitted
	for _, event := range events {
		err := t.storage.MarkEventTransmitted(ctx, event.ID)
		if err != nil {
			t.LogError("Failed to mark event as transmitted", err, "event_id", event.ID)
		}
	}

	t.LogDebug("Event batch transmitted successfully", "count", len(events))
	return nil
}

// transmitEvents transmits events using the configured callback
func (t *Transmitter) transmitEvents(ctx context.Context, events []*Event) error {
	if t.config.OnTransmit == nil {
		// No transmission callback configured - this is expected until Epic 1.6
		// For now, we'll just log that transmission would happen
		t.LogDebug("Transmission callback not configured (will be gRPC client in Epic 1.6)", "event_count", len(events))
		return nil // Return success for now
	}

	return t.config.OnTransmit(ctx, events)
}

// TransmitNow forces immediate transmission of pending events
func (t *Transmitter) TransmitNow(ctx context.Context) error {
	return t.processQueue(ctx)
}

// RecoverQueue recovers the queue on startup
func (t *Transmitter) RecoverQueue(ctx context.Context) error {
	// Get all pending events
	pending, err := t.storage.GetPendingEvents(ctx, 10000)
	if err != nil {
		return fmt.Errorf("failed to get pending events: %w", err)
	}

	t.LogInfo("Recovered queue", "pending_events", len(pending))

	// Events are already in the queue (via state.Manager)
	// We just need to log the recovery
	return nil
}

// GetTransmissionStats returns transmission statistics
func (t *Transmitter) GetTransmissionStats(ctx context.Context) (*TransmissionStats, error) {
	queueStats, err := t.queue.GetQueueStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	stats := &TransmissionStats{
		QueueStats:     queueStats,
		Transmitting:    t.isTransmitting(),
		BatchSize:       t.config.BatchSize,
		TransmissionInterval: t.config.TransmissionInterval,
		MaxRetries:      t.config.MaxRetries,
	}

	return stats, nil
}

// GetConfig returns the transmitter configuration
func (t *Transmitter) GetConfig() TransmitterConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.config
}

// SetConfig updates the transmitter configuration
func (t *Transmitter) SetConfig(config TransmitterConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.config = config
}

// isTransmitting returns whether the transmitter is running
func (t *Transmitter) isTransmitting() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.transmitting
}

// TransmissionStats contains transmission statistics
type TransmissionStats struct {
	QueueStats          *QueueStats
	Transmitting        bool
	BatchSize           int
	TransmissionInterval time.Duration
	MaxRetries          int
}

