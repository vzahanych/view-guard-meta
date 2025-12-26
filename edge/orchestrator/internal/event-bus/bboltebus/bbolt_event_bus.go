package bboltebus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
)

const (
	eventsBucket       = "events"
	eventsByTypeBucket = "events_by_type"
	eventsByTimeBucket = "events_by_time"
)

// BboltEventBus is a durable event bus implementation using bbolt for persistence.
// It persists ALL events (not just critical ones) for debugging and troubleshooting.
// It also maintains in-memory subscriptions for real-time event delivery.
type BboltEventBus struct {
	mu          sync.RWMutex
	db          *bbolt.DB
	logger      *zap.Logger
	subscribers map[types.EventType][]chan types.Event
	allSubs     []chan types.Event
	bufferSize  int
	closed      bool
}

// NewBboltEventBus creates a new bbolt-based event bus implementation.
// dbPath is the path to the bbolt database file.
// bufferSize controls the size of subscriber channels; non-positive values are treated as a default of 100.
func NewBboltEventBus(dbPath string, bufferSize int, logger *zap.Logger) (*BboltEventBus, error) {
	if bufferSize <= 0 {
		bufferSize = 100
	}

	// Ensure parent directories exist (bbolt.Open does not create them)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}

	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open event bus database: %w", err)
	}

	bus := &BboltEventBus{
		db:          db,
		logger:      logger,
		subscribers: make(map[types.EventType][]chan types.Event),
		allSubs:     make([]chan types.Event, 0),
		bufferSize:  bufferSize,
	}

	// Initialize buckets
	if err := db.Update(func(tx *bbolt.Tx) error {
		buckets := []string{
			eventsBucket,
			eventsByTypeBucket,
			eventsByTimeBucket,
		}
		for _, bucketName := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
				return fmt.Errorf("failed to create %s bucket: %w", bucketName, err)
			}
		}
		return nil
	}); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("Failed to close database after initialization error", zap.Error(closeErr))
		}
		return nil, err
	}

	return bus, nil
}

// Name returns the implementation name.
func (b *BboltEventBus) Name() string {
	return "bbolt-event-bus"
}

// Subscribe subscribes to events of a specific type.
func (b *BboltEventBus) Subscribe(eventType types.EventType) <-chan types.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan types.Event)
		close(ch)
		return ch
	}

	ch := make(chan types.Event, b.bufferSize)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// SubscribeAll subscribes to all events, regardless of type.
func (b *BboltEventBus) SubscribeAll() <-chan types.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan types.Event)
		close(ch)
		return ch
	}

	ch := make(chan types.Event, b.bufferSize)
	b.allSubs = append(b.allSubs, ch)
	return ch
}

// Publish publishes an event to all matching subscribers and persists it to bbolt.
func (b *BboltEventBus) Publish(event types.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Persist event to bbolt (non-blocking, fire-and-forget)
	go b.persistEvent(event)

	// Send to specific subscribers
	if subs, ok := b.subscribers[event.Type]; ok {
		for _, sub := range subs {
			select {
			case sub <- event:
			default:
				// Channel full, drop event (non-blocking)
				b.logger.Warn("Event dropped: subscriber channel full",
					zap.String("event_type", string(event.Type)),
					zap.String("source", event.Source))
			}
		}
	}

	// Send to "all" subscribers
	for _, sub := range b.allSubs {
		select {
		case sub <- event:
		default:
			// Channel full, drop event
			b.logger.Warn("Event dropped: subscriber channel full (all events)",
				zap.String("event_type", string(event.Type)),
				zap.String("source", event.Source))
		}
	}
}

// persistEvent persists an event to bbolt storage.
// This is called asynchronously to avoid blocking event publishing.
func (b *BboltEventBus) persistEvent(event types.Event) {
	// Generate a unique event ID based on timestamp and a counter
	eventID := b.generateEventID(event.Timestamp)

	// Marshal event to JSON
	eventData, err := json.Marshal(event)
	if err != nil {
		b.logger.Error("Failed to marshal event for persistence",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.Error(err))
		return
	}

	// Store event in multiple buckets for efficient querying
	persistErr := b.db.Update(func(tx *bbolt.Tx) error {
		// Store in main events bucket (keyed by event ID)
		eventsBucket := tx.Bucket([]byte(eventsBucket))
		if eventsBucket == nil {
			return fmt.Errorf("events bucket not found")
		}
		if err := eventsBucket.Put([]byte(eventID), eventData); err != nil {
			return fmt.Errorf("failed to store event in events bucket: %w", err)
		}

		// Store in events_by_type bucket (keyed by event type + timestamp)
		eventsByTypeBucket := tx.Bucket([]byte(eventsByTypeBucket))
		if eventsByTypeBucket == nil {
			return fmt.Errorf("events_by_type bucket not found")
		}
		typeKey := b.buildTypeKey(string(event.Type), event.Timestamp, eventID)
		if err := eventsByTypeBucket.Put(typeKey, []byte(eventID)); err != nil {
			return fmt.Errorf("failed to store event in events_by_type bucket: %w", err)
		}

		// Store in events_by_time bucket (keyed by timestamp)
		eventsByTimeBucket := tx.Bucket([]byte(eventsByTimeBucket))
		if eventsByTimeBucket == nil {
			return fmt.Errorf("events_by_time bucket not found")
		}
		timeKey := b.buildTimeKey(event.Timestamp, eventID)
		if err := eventsByTimeBucket.Put(timeKey, []byte(eventID)); err != nil {
			return fmt.Errorf("failed to store event in events_by_time bucket: %w", err)
		}

		return nil
	})

	if persistErr != nil {
		b.logger.Error("Failed to persist event to bbolt",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.String("event_id", eventID),
			zap.Error(persistErr))
	} else {
		b.logger.Debug("Event persisted to bbolt",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.String("event_id", eventID))
	}
}

// generateEventID generates a unique event ID based on timestamp and a counter.
// Format: <timestamp_nanos>_<counter>
func (b *BboltEventBus) generateEventID(timestamp time.Time) string {
	// Use timestamp in nanoseconds as the base
	// Add a small random component to ensure uniqueness
	nanos := timestamp.UnixNano()
	// Use a simple counter based on current time in microseconds for uniqueness
	micros := time.Now().UnixMicro()
	return fmt.Sprintf("%d_%d", nanos, micros)
}

// buildTypeKey builds a key for the events_by_type bucket.
// Format: <event_type>_<timestamp_nanos>_<event_id>
func (b *BboltEventBus) buildTypeKey(eventType string, timestamp time.Time, eventID string) []byte {
	nanos := timestamp.UnixNano()
	key := fmt.Sprintf("%s_%d_%s", eventType, nanos, eventID)
	return []byte(key)
}

// buildTimeKey builds a key for the events_by_time bucket.
// Format: <timestamp_nanos>_<event_id>
func (b *BboltEventBus) buildTimeKey(timestamp time.Time, eventID string) []byte {
	nanos := timestamp.UnixNano()
	key := fmt.Sprintf("%d_%s", nanos, eventID)
	return []byte(key)
}

// buildTimeKeyPrefix builds a prefix key for time-based queries.
// Format: <timestamp_nanos>_
func (b *BboltEventBus) buildTimeKeyPrefix(timestamp time.Time) []byte {
	nanos := timestamp.UnixNano()
	key := fmt.Sprintf("%d_", nanos)
	return []byte(key)
}

// Unsubscribe removes a subscription for the given event type and channel.
func (b *BboltEventBus) Unsubscribe(eventType types.EventType, ch <-chan types.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If already closed, channels are already closed by Close(), so don't close again
	if b.closed {
		return
	}

	// Remove from specific subscribers and close the channel
	if subs, ok := b.subscribers[eventType]; ok {
		filtered := make([]chan types.Event, 0, len(subs))
		for _, sub := range subs {
			if sub != ch {
				filtered = append(filtered, sub)
			} else {
				// Close channel safely (recover from panic if already closed)
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel was already closed, ignore panic
							_ = r
						}
					}()
					close(sub)
				}()
			}
		}
		if len(filtered) == 0 {
			delete(b.subscribers, eventType)
		} else {
			b.subscribers[eventType] = filtered
		}
	}

	// Remove from allSubs if present and close the channel
	if len(b.allSubs) > 0 {
		filteredAll := make([]chan types.Event, 0, len(b.allSubs))
		for _, sub := range b.allSubs {
			if sub != ch {
				filteredAll = append(filteredAll, sub)
			} else {
				// Close channel safely (recover from panic if already closed)
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel was already closed, ignore panic
							_ = r
						}
					}()
					close(sub)
				}()
			}
		}
		b.allSubs = filteredAll
	}
}

// Close shuts down the event bus and closes all subscriber channels.
func (b *BboltEventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all channels safely (in case Unsubscribe() already closed some)
	closeChannel := func(ch chan types.Event) {
		defer func() {
			// Recover from panic if channel is already closed
			if r := recover(); r != nil {
				// Channel was already closed, ignore
				_ = r
			}
		}()
		close(ch)
	}

	for _, subs := range b.subscribers {
		for _, sub := range subs {
			closeChannel(sub)
		}
	}
	for _, sub := range b.allSubs {
		closeChannel(sub)
	}

	b.subscribers = make(map[types.EventType][]chan types.Event)
	b.allSubs = make([]chan types.Event, 0)

	// Close the database
	if err := b.db.Close(); err != nil {
		return fmt.Errorf("failed to close event bus database: %w", err)
	}

	return nil
}

// QueryEvents queries events from bbolt storage.
// This is useful for debugging and troubleshooting.
func (b *BboltEventBus) QueryEvents(ctx context.Context, filters *EventQueryFilters) ([]types.Event, error) {
	if filters == nil {
		filters = &EventQueryFilters{}
	}

	var events []types.Event
	var eventIDs map[string]bool

	// Collect event IDs based on filters
	err := b.db.View(func(tx *bbolt.Tx) error {
		if filters.EventType != nil {
			// Query by event type
			eventsByTypeBucket := tx.Bucket([]byte(eventsByTypeBucket))
			if eventsByTypeBucket == nil {
				return fmt.Errorf("events_by_type bucket not found")
			}

			eventIDs = make(map[string]bool)
			prefix := []byte(*filters.EventType + "_")
			c := eventsByTypeBucket.Cursor()
			for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = c.Next() {
				eventID := string(v)
				eventIDs[eventID] = true
			}
		} else if filters.StartTime != nil || filters.EndTime != nil {
			// Query by time range
			eventsByTimeBucket := tx.Bucket([]byte(eventsByTimeBucket))
			if eventsByTimeBucket == nil {
				return fmt.Errorf("events_by_time bucket not found")
			}

			eventIDs = make(map[string]bool)
			c := eventsByTimeBucket.Cursor()

			var startNanos, endNanos int64
			if filters.StartTime != nil {
				startNanos = filters.StartTime.UnixNano()
			}
			if filters.EndTime != nil {
				endNanos = filters.EndTime.UnixNano()
			}

			// Iterate through all events and filter by time range
			for k, v := c.First(); k != nil; k, v = c.Next() {
				// Parse timestamp from key (format: <timestamp_nanos>_<event_id>)
				keyStr := string(k)
				var eventNanos int64
				if _, err := fmt.Sscanf(keyStr, "%d_", &eventNanos); err != nil {
					continue
				}

				// Check if event is within time range
				if filters.StartTime != nil && eventNanos < startNanos {
					continue
				}
				if filters.EndTime != nil && eventNanos > endNanos {
					break // Since keys are sorted by timestamp, we can break here
				}

				eventID := string(v)
				eventIDs[eventID] = true
			}
		} else {
			// Query all events
			eventsBucket := tx.Bucket([]byte(eventsBucket))
			if eventsBucket == nil {
				return fmt.Errorf("events bucket not found")
			}

			eventIDs = make(map[string]bool)
			c := eventsBucket.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				eventID := string(k)
				eventIDs[eventID] = true
			}
		}

		// Load events from the main events bucket
		eventsBucket := tx.Bucket([]byte(eventsBucket))
		if eventsBucket == nil {
			return fmt.Errorf("events bucket not found")
		}

		for eventID := range eventIDs {
			eventData := eventsBucket.Get([]byte(eventID))
			if eventData == nil {
				continue
			}

			var event types.Event
			if err := json.Unmarshal(eventData, &event); err != nil {
				b.logger.Warn("Failed to unmarshal event",
					zap.String("event_id", eventID),
					zap.Error(err))
				continue
			}

			// Apply additional filters
			if filters.Source != nil && event.Source != *filters.Source {
				continue
			}

			events = append(events, event)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	// Sort events by timestamp (newest first)
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[i].Timestamp.Before(events[j].Timestamp) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}

	// Apply limit
	if filters.Limit > 0 && len(events) > filters.Limit {
		events = events[:filters.Limit]
	}

	return events, nil
}

// EventQueryFilters defines filters for querying events.
type EventQueryFilters struct {
	EventType *types.EventType
	Source    *string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}

// GetEventCount returns the total number of events stored in bbolt.
func (b *BboltEventBus) GetEventCount(ctx context.Context) (int, error) {
	var count int
	err := b.db.View(func(tx *bbolt.Tx) error {
		eventsBucket := tx.Bucket([]byte(eventsBucket))
		if eventsBucket == nil {
			return fmt.Errorf("events bucket not found")
		}
		count = eventsBucket.Stats().KeyN
		return nil
	})
	return count, err
}
