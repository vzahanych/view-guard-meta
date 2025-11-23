package events

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// Queue manages the event queue with priority and size limits
type Queue struct {
	stateManager *state.Manager
	logger       *logger.Logger
	maxSize      int
	mu           sync.RWMutex
}

// QueueConfig contains configuration for the event queue
type QueueConfig struct {
	StateManager *state.Manager
	MaxSize      int // Maximum queue size (0 = unlimited)
}

// NewQueue creates a new event queue
func NewQueue(config QueueConfig, log *logger.Logger) *Queue {
	return &Queue{
		stateManager: config.StateManager,
		logger:       log,
		maxSize:      config.MaxSize,
	}
}

// Enqueue adds an event to the queue
func (q *Queue) Enqueue(ctx context.Context, event *Event, priority int) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check queue size limit
	if q.maxSize > 0 {
		count, err := q.getQueueSize(ctx)
		if err != nil {
			return fmt.Errorf("failed to check queue size: %w", err)
		}
		if count >= q.maxSize {
			return fmt.Errorf("queue is full: %d/%d", count, q.maxSize)
		}
	}

	// Save event (this automatically adds it to the queue via state.Manager)
	eventState := event.ToEventState()
	err := q.stateManager.SaveEvent(ctx, eventState)
	if err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}

	// Update priority if not default
	if priority != 0 {
		err = q.setPriority(ctx, event.ID, priority)
		if err != nil {
			q.logger.Warn("Failed to set priority", "event_id", event.ID, "error", err)
			// Don't fail the enqueue operation if priority update fails
		}
	}

	q.logger.Debug("Event enqueued", "event_id", event.ID, "priority", priority)
	return nil
}

// Dequeue retrieves and removes the next event from the queue
func (q *Queue) Dequeue(ctx context.Context) (*Event, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Get next pending event (highest priority, oldest first)
	pending, err := q.stateManager.GetPendingEvents(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending events: %w", err)
	}

	if len(pending) == 0 {
		return nil, nil // Queue is empty
	}

	event := FromEventState(pending[0])
	return event, nil
}

// Peek retrieves the next event without removing it
func (q *Queue) Peek(ctx context.Context) (*Event, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	pending, err := q.stateManager.GetPendingEvents(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending events: %w", err)
	}

	if len(pending) == 0 {
		return nil, nil // Queue is empty
	}

	event := FromEventState(pending[0])
	return event, nil
}

// BatchDequeue retrieves multiple events from the queue
func (q *Queue) BatchDequeue(ctx context.Context, batchSize int) ([]*Event, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if batchSize <= 0 {
		batchSize = 10
	}

	pending, err := q.stateManager.GetPendingEvents(ctx, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending events: %w", err)
	}

	events := make([]*Event, len(pending))
	for i, es := range pending {
		events[i] = FromEventState(es)
	}

	return events, nil
}

// Size returns the current queue size
func (q *Queue) Size(ctx context.Context) (int, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.getQueueSize(ctx)
}

// IsEmpty checks if the queue is empty
func (q *Queue) IsEmpty(ctx context.Context) (bool, error) {
	size, err := q.Size(ctx)
	if err != nil {
		return false, err
	}
	return size == 0, nil
}

// Clear removes all events from the queue
func (q *Queue) Clear(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Get all pending events
	pending, err := q.stateManager.GetPendingEvents(ctx, 10000)
	if err != nil {
		return fmt.Errorf("failed to get pending events: %w", err)
	}

	// Mark all as transmitted (this removes them from queue)
	for _, es := range pending {
		err := q.stateManager.MarkEventTransmitted(ctx, es.ID)
		if err != nil {
			q.logger.Warn("Failed to mark event as transmitted", "event_id", es.ID, "error", err)
		}
	}

	q.logger.Info("Queue cleared", "count", len(pending))
	return nil
}

// IncrementRetryCount increments the retry count for an event
func (q *Queue) IncrementRetryCount(ctx context.Context, eventID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `
		UPDATE event_queue
		SET retry_count = retry_count + 1
		WHERE event_id = ?
	`

	_, err := q.stateManager.GetDB().ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	return nil
}

// GetRetryCount returns the retry count for an event
func (q *Queue) GetRetryCount(ctx context.Context, eventID string) (int, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	query := `SELECT retry_count FROM event_queue WHERE event_id = ?`

	var retryCount int
	err := q.stateManager.GetDB().QueryRowContext(ctx, query, eventID).Scan(&retryCount)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get retry count: %w", err)
	}

	return retryCount, nil
}

// SetPriority sets the priority for an event in the queue
func (q *Queue) SetPriority(ctx context.Context, eventID string, priority int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.setPriority(ctx, eventID, priority)
}

// setPriority sets the priority (internal, assumes lock is held)
func (q *Queue) setPriority(ctx context.Context, eventID string, priority int) error {
	query := `
		UPDATE event_queue
		SET priority = ?
		WHERE event_id = ?
	`

	_, err := q.stateManager.GetDB().ExecContext(ctx, query, priority, eventID)
	if err != nil {
		return fmt.Errorf("failed to set priority: %w", err)
	}

	return nil
}

// getQueueSize returns the current queue size (internal, assumes lock is held)
func (q *Queue) getQueueSize(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM event_queue`

	var count int
	err := q.stateManager.GetDB().QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get queue size: %w", err)
	}

	return count, nil
}

// GetQueueStats returns statistics about the queue
func (q *Queue) GetQueueStats(ctx context.Context) (*QueueStats, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := &QueueStats{}

	// Get queue size
	size, err := q.getQueueSize(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue size: %w", err)
	}
	stats.Size = size
	stats.MaxSize = q.maxSize

	// Get oldest event timestamp
	query := `
		SELECT MIN(eq.created_at)
		FROM event_queue eq
	`
	var oldestTimeStr sql.NullString
	err = q.stateManager.GetDB().QueryRowContext(ctx, query).Scan(&oldestTimeStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get oldest event time: %w", err)
	}
	if oldestTimeStr.Valid && oldestTimeStr.String != "" {
		oldestTime, parseErr := time.Parse("2006-01-02 15:04:05", oldestTimeStr.String)
		if parseErr == nil {
			stats.OldestEventAge = time.Since(oldestTime)
		}
	}

	// Get average retry count
	query = `
		SELECT AVG(retry_count)
		FROM event_queue
	`
	var avgRetry sql.NullFloat64
	err = q.stateManager.GetDB().QueryRowContext(ctx, query).Scan(&avgRetry)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get average retry count: %w", err)
	}
	if avgRetry.Valid {
		stats.AverageRetryCount = avgRetry.Float64
	}

	return stats, nil
}

// QueueStats contains queue statistics
type QueueStats struct {
	Size              int
	MaxSize           int
	OldestEventAge    time.Duration
	AverageRetryCount float64
}

