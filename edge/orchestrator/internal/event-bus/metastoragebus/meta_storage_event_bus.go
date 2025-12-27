package metastoragebus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	"go.uber.org/zap"
)

// MetaStorageEventBus is a durable event bus implementation using meta-storage for persistence.
// It persists ALL events (not just critical ones) for debugging and troubleshooting.
// It also maintains in-memory subscriptions for real-time event delivery.
// It includes retry logic with exponential backoff and dead letter queue support.
// It supports event ordering guarantees per source.
type MetaStorageEventBus struct {
	mu              sync.RWMutex
	metaStorage     metastorage.MetaDataStore
	logger          *zap.Logger
	subscribers     map[types.EventType][]chan types.EventAny
	allSubs         []chan types.EventAny
	bufferSize      int
	closed          bool
	retryConfig     *RetryConfig
	retryWorkerCtx  context.Context
	retryWorkerCancel context.CancelFunc
	retryWorkerWg   sync.WaitGroup
	// Ordering support
	orderingConfig  *OrderingConfig
	sequenceNumbers map[string]int64 // Per-source sequence number counter
	orderingBuffers map[string]*OrderingBuffer // Per-source ordering buffers
	orderingMu      sync.RWMutex // Separate mutex for ordering to avoid contention
}

// RetryConfig contains configuration for event retry logic
type RetryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialBackoff  time.Duration // Initial backoff duration
	MaxBackoff      time.Duration // Maximum backoff duration (caps exponential backoff)
	BackoffMultiplier float64     // Multiplier for exponential backoff
	RetryInterval   time.Duration // Interval between retry worker runs
}

// OrderingConfig contains configuration for event ordering
type OrderingConfig struct {
	Mode            types.OrderingMode // Ordering mode: none, best_effort, strict
	BufferSize      int                // Buffer size for out-of-order events
	Timeout         time.Duration       // Timeout for waiting for missing sequences in strict mode
}

// OrderingBuffer manages buffering and reordering of events per source
type OrderingBuffer struct {
	source          string
	expectedSeq     int64                    // Next expected sequence number
	buffered        map[int64]types.EventAny    // Buffered events by sequence number
	lastDelivered   int64                    // Last delivered sequence number
	timeouts        map[int64]time.Time       // Timeout tracking for strict mode
	mu              sync.Mutex
	config          *OrderingConfig
	logger          *zap.Logger
}

// NewMetaStorageEventBus creates a new meta-storage-based event bus implementation.
// bufferSize controls the size of subscriber channels; non-positive values are treated as a default of 100.
// retryConfig is optional - if nil, retry logic is disabled.
// orderingConfig is optional - if nil, ordering is disabled.
func NewMetaStorageEventBus(metaStorage metastorage.MetaDataStore, bufferSize int, logger *zap.Logger, retryConfig *RetryConfig, orderingConfig *OrderingConfig) (*MetaStorageEventBus, error) {
	if metaStorage == nil {
		return nil, fmt.Errorf("meta-storage is required")
	}

	if bufferSize <= 0 {
		bufferSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	bus := &MetaStorageEventBus{
		metaStorage:      metaStorage,
		logger:           logger,
		subscribers:      make(map[types.EventType][]chan types.EventAny),
		allSubs:          make([]chan types.EventAny, 0),
		bufferSize:       bufferSize,
		retryConfig:      retryConfig,
		retryWorkerCtx:   ctx,
		retryWorkerCancel: cancel,
		orderingConfig:   orderingConfig,
		sequenceNumbers:  make(map[string]int64),
		orderingBuffers:  make(map[string]*OrderingBuffer),
	}

	// Start retry worker if retry config is provided
	if retryConfig != nil && retryConfig.MaxRetries > 0 {
		bus.startRetryWorker()
	}

	return bus, nil
}

// Name returns the implementation name.
func (b *MetaStorageEventBus) Name() string {
	return "metastorage-event-bus"
}

// Subscribe subscribes to events of a specific type.
func (b *MetaStorageEventBus) Subscribe(eventType types.EventType) <-chan types.EventAny {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan types.EventAny)
		close(ch)
		return ch
	}

	ch := make(chan types.EventAny, b.bufferSize)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// SubscribeAll subscribes to all events, regardless of type.
func (b *MetaStorageEventBus) SubscribeAll() <-chan types.EventAny {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan types.EventAny)
		close(ch)
		return ch
	}

	ch := make(chan types.EventAny, b.bufferSize)
	b.allSubs = append(b.allSubs, ch)
	return ch
}

// Publish publishes an event to all matching subscribers and persists it to meta-storage.
// If ordering is enabled, events are assigned sequence numbers and delivered in order per source.
func (b *MetaStorageEventBus) Publish(event types.EventAny) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Generate sequence number if ordering is enabled
	if b.orderingConfig != nil && b.orderingConfig.Mode != types.OrderingModeNone {
		event.SequenceNumber = b.getNextSequenceNumber(event.Source)
	}

	// Persist event to meta-storage (non-blocking, fire-and-forget)
	go b.persistEvent(context.Background(), event)

	// Apply ordering logic if enabled
	if b.orderingConfig != nil && b.orderingConfig.Mode != types.OrderingModeNone {
		b.publishWithOrdering(event)
	} else {
		// No ordering - publish directly
		b.publishDirect(event)
	}
}

// publishDirect publishes an event directly to subscribers without ordering
func (b *MetaStorageEventBus) publishDirect(event types.EventAny) {
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

// publishWithOrdering publishes an event with ordering guarantees per source
func (b *MetaStorageEventBus) publishWithOrdering(event types.EventAny) {
	// Get or create ordering buffer for this source
	buffer := b.getOrCreateOrderingBuffer(event.Source)
	
	// Process event through ordering buffer
	orderedEvents := buffer.AddEvent(event)
	
	// Deliver ordered events to subscribers
	for _, orderedEvent := range orderedEvents {
		b.publishDirect(orderedEvent)
	}
}

// getNextSequenceNumber generates the next sequence number for a source
func (b *MetaStorageEventBus) getNextSequenceNumber(source string) int64 {
	b.orderingMu.Lock()
	defer b.orderingMu.Unlock()
	
	b.sequenceNumbers[source]++
	return b.sequenceNumbers[source]
}

// getOrCreateOrderingBuffer gets or creates an ordering buffer for a source
func (b *MetaStorageEventBus) getOrCreateOrderingBuffer(source string) *OrderingBuffer {
	b.orderingMu.RLock()
	buffer, exists := b.orderingBuffers[source]
	b.orderingMu.RUnlock()
	
	if exists {
		return buffer
	}
	
	b.orderingMu.Lock()
	defer b.orderingMu.Unlock()
	
	// Double-check after acquiring write lock
	if buffer, exists := b.orderingBuffers[source]; exists {
		return buffer
	}
	
	// Create new buffer
	buffer = NewOrderingBuffer(source, b.orderingConfig, b.logger)
	b.orderingBuffers[source] = buffer
	return buffer
}

// persistEvent persists an event to meta-storage.
// This is called asynchronously to avoid blocking event publishing.
func (b *MetaStorageEventBus) persistEvent(ctx context.Context, event types.EventAny) {
	// Generate a unique event ID based on timestamp and a counter
	eventID := b.generateEventID(event.Timestamp)

	// Unmarshal event data from json.RawMessage to map for storage
	var dataMap map[string]interface{}
	if len(event.Data) > 0 {
		if err := json.Unmarshal(event.Data, &dataMap); err != nil {
			b.logger.Warn("Failed to unmarshal event data for storage, storing as empty map",
				zap.String("event_id", eventID),
				zap.Error(err))
			dataMap = make(map[string]interface{})
		}
	} else {
		dataMap = make(map[string]interface{})
	}

	// Convert event to map[string]interface{} for storage
	eventData := map[string]interface{}{
		"event_id":  eventID, // Store event ID for retry lookup
		"type":      string(event.Type),
		"source":    event.Source,
		"timestamp": event.Timestamp.Format(time.RFC3339Nano),
		"data":      dataMap,
	}

	// Store sequence number if present
	if event.SequenceNumber > 0 {
		eventData["sequence_number"] = event.SequenceNumber
	}

	// Set initial processing status to "pending" if retry is enabled
	if b.retryConfig != nil {
		eventData["processing_status"] = string(types.EventStatusPending)
		eventData["retry_count"] = 0
		eventData["created_at"] = time.Now().Format(time.RFC3339Nano)
	}

	// Store event in meta-storage
	err := b.metaStorage.SaveEvent(ctx, eventID, eventData)
	if err != nil {
		b.logger.Error("Failed to persist event to meta-storage",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.String("event_id", eventID),
			zap.Error(err))
	} else {
		b.logger.Debug("Event persisted to meta-storage",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.String("event_id", eventID))
	}
}

// generateEventID generates a unique event ID based on timestamp and a counter.
// Format: <timestamp_nanos>_<counter>
func (b *MetaStorageEventBus) generateEventID(timestamp time.Time) string {
	// Use timestamp in nanoseconds as the base
	// Add a small random component to ensure uniqueness
	nanos := timestamp.UnixNano()
	// Use a simple counter based on current time in microseconds for uniqueness
	micros := time.Now().UnixMicro()
	return fmt.Sprintf("%d_%d", nanos, micros)
}

// Unsubscribe removes a subscription for the given event type and channel.
func (b *MetaStorageEventBus) Unsubscribe(eventType types.EventType, ch <-chan types.EventAny) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If already closed, channels are already closed by Close(), so don't close again
	if b.closed {
		return
	}

	// Remove from specific subscribers and close the channel
	if subs, ok := b.subscribers[eventType]; ok {
		filtered := make([]chan types.EventAny, 0, len(subs))
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
		filteredAll := make([]chan types.EventAny, 0, len(b.allSubs))
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
func (b *MetaStorageEventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Stop retry worker
	if b.retryWorkerCancel != nil {
		b.retryWorkerCancel()
	}
	b.retryWorkerWg.Wait()

	// Clean up ordering buffers
	b.orderingMu.Lock()
	b.orderingBuffers = make(map[string]*OrderingBuffer)
	b.sequenceNumbers = make(map[string]int64)
	b.orderingMu.Unlock()

	// Close all channels safely (in case Unsubscribe() already closed some)
	closeChannel := func(ch chan types.EventAny) {
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

	b.subscribers = make(map[types.EventType][]chan types.EventAny)
	b.allSubs = make([]chan types.EventAny, 0)

	return nil
}

// QueryEvents queries events from meta-storage.
// This is useful for debugging and troubleshooting.
func (b *MetaStorageEventBus) QueryEvents(ctx context.Context, filters *EventQueryFilters) ([]types.EventAny, error) {
	if filters == nil {
		filters = &EventQueryFilters{}
	}

	// Build filters map for meta-storage
	filterMap := make(map[string]interface{})
	if filters.EventType != nil {
		filterMap["type"] = string(*filters.EventType)
	}
	if filters.Source != nil {
		filterMap["source"] = *filters.Source
	}
	if filters.StartTime != nil {
		filterMap["start_time"] = filters.StartTime.Format(time.RFC3339Nano)
	}
	if filters.EndTime != nil {
		filterMap["end_time"] = filters.EndTime.Format(time.RFC3339Nano)
	}
	if filters.Limit > 0 {
		filterMap["limit"] = filters.Limit
	}

	// Query events from meta-storage
	eventMaps, err := b.metaStorage.ListEvents(ctx, filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	// Convert map[string]interface{} to types.Event
	events := make([]types.EventAny, 0, len(eventMaps))
	for _, eventMap := range eventMaps {
		event, err := b.mapToEvent(eventMap)
		if err != nil {
			b.logger.Warn("Failed to convert event map to Event",
				zap.Error(err))
			continue
		}

		// Apply additional filters that meta-storage might not support
		if filters.EventType != nil && event.Type != *filters.EventType {
			continue
		}
		if filters.Source != nil && event.Source != *filters.Source {
			continue
		}
		if filters.StartTime != nil && event.Timestamp.Before(*filters.StartTime) {
			continue
		}
		if filters.EndTime != nil && event.Timestamp.After(*filters.EndTime) {
			continue
		}

		events = append(events, event)
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

// mapToEvent converts a map[string]interface{} to types.EventAny.
func (b *MetaStorageEventBus) mapToEvent(eventMap map[string]interface{}) (types.EventAny, error) {
	event := types.EventAny{}

	// Extract type
	if typeStr, ok := eventMap["type"].(string); ok {
		event.Type = types.EventType(typeStr)
	} else {
		return event, fmt.Errorf("missing or invalid type field")
	}

	// Extract source
	if source, ok := eventMap["source"].(string); ok {
		event.Source = source
	} else {
		return event, fmt.Errorf("missing or invalid source field")
	}

	// Extract timestamp
	if timestampStr, ok := eventMap["timestamp"].(string); ok {
		timestamp, err := time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			return event, fmt.Errorf("invalid timestamp format: %w", err)
		}
		event.Timestamp = timestamp
	} else {
		return event, fmt.Errorf("missing or invalid timestamp field")
	}

	// Extract data as JSON
	if data, ok := eventMap["data"]; ok {
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return event, fmt.Errorf("failed to marshal event data: %w", err)
		}
		event.Data = json.RawMessage(dataBytes)
	} else {
		event.Data = json.RawMessage("{}")
	}

	// Extract sequence number if present
	if seqNum, ok := eventMap["sequence_number"].(float64); ok {
		event.SequenceNumber = int64(seqNum)
	} else if seqNum, ok := eventMap["sequence_number"].(int64); ok {
		event.SequenceNumber = seqNum
	} else if seqNum, ok := eventMap["sequence_number"].(int); ok {
		event.SequenceNumber = int64(seqNum)
	}

	return event, nil
}

// EventQueryFilters defines filters for querying events.
type EventQueryFilters struct {
	EventType *types.EventType
	Source    *string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}

// GetEventCount returns the total number of events stored in meta-storage.
func (b *MetaStorageEventBus) GetEventCount(ctx context.Context) (int, error) {
	return b.metaStorage.GetEventCount(ctx)
}

// MarkEventFailed marks an event as failed and schedules it for retry.
// This should be called by event processors when event processing fails.
func (b *MetaStorageEventBus) MarkEventFailed(ctx context.Context, eventID string, errorMsg string) error {
	if b.retryConfig == nil {
		return nil // Retry disabled
	}

	// Get event to check current retry count
	eventData, exists := b.metaStorage.GetEvent(ctx, eventID)
	if !exists {
		return fmt.Errorf("event %s not found", eventID)
	}

	// Get current retry count
	retryCount := 0
	if count, ok := eventData["retry_count"].(float64); ok {
		retryCount = int(count)
	}

	// Check if max retries exceeded
	if retryCount >= b.retryConfig.MaxRetries {
		// Move to dead letter queue
		b.logger.Warn("Event exceeded max retries, moving to dead letter queue",
			zap.String("event_id", eventID),
			zap.Int("retry_count", retryCount),
			zap.String("error", errorMsg))
		return b.metaStorage.MoveEventToDeadLetter(ctx, eventID)
	}

	// Calculate next retry time with exponential backoff
	backoff := b.calculateBackoff(retryCount)
	nextRetryTime := time.Now().Add(backoff)

	// Update event status
	err := b.metaStorage.UpdateEventProcessingStatus(ctx, eventID, string(types.EventStatusFailed), retryCount+1, errorMsg, &nextRetryTime)
	if err != nil {
		b.logger.Error("Failed to update event processing status",
			zap.String("event_id", eventID),
			zap.Error(err))
		return err
	}

	b.logger.Info("Event marked as failed, scheduled for retry",
		zap.String("event_id", eventID),
		zap.Int("retry_count", retryCount+1),
		zap.Duration("backoff", backoff),
		zap.String("error", errorMsg))

	return nil
}

// MarkEventSucceeded marks an event as successfully processed.
// This should be called by event processors when event processing succeeds.
func (b *MetaStorageEventBus) MarkEventSucceeded(ctx context.Context, eventID string) error {
	if b.retryConfig == nil {
		return nil // Retry disabled
	}

	err := b.metaStorage.UpdateEventProcessingStatus(ctx, eventID, string(types.EventStatusSucceeded), 0, "", nil)
	if err != nil {
		b.logger.Error("Failed to mark event as succeeded",
			zap.String("event_id", eventID),
			zap.Error(err))
		return err
	}

	return nil
}

// calculateBackoff calculates the backoff duration for a given retry count using exponential backoff.
func (b *MetaStorageEventBus) calculateBackoff(retryCount int) time.Duration {
	if b.retryConfig == nil {
		return time.Second
	}

	// Calculate exponential backoff: initial * (multiplier ^ retryCount)
	backoff := float64(b.retryConfig.InitialBackoff) * (b.retryConfig.BackoffMultiplier * float64(retryCount))
	if backoff > float64(b.retryConfig.MaxBackoff) {
		backoff = float64(b.retryConfig.MaxBackoff)
	}

	return time.Duration(backoff)
}

// startRetryWorker starts a background goroutine that periodically processes failed events.
func (b *MetaStorageEventBus) startRetryWorker() {
	b.retryWorkerWg.Add(1)
	go func() {
		defer b.retryWorkerWg.Done()

		ticker := time.NewTicker(b.retryConfig.RetryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-b.retryWorkerCtx.Done():
				b.logger.Info("Retry worker stopped")
				return
			case <-ticker.C:
				b.processFailedEvents()
			}
		}
	}()

	b.logger.Info("Retry worker started",
		zap.Duration("retry_interval", b.retryConfig.RetryInterval),
		zap.Int("max_retries", b.retryConfig.MaxRetries))
}

// processFailedEvents retrieves failed events that are ready for retry and re-publishes them.
func (b *MetaStorageEventBus) processFailedEvents() {
	ctx := context.Background()
	now := time.Now()

	// Get failed events ready for retry
	failedEvents, err := b.metaStorage.GetFailedEvents(ctx, now)
	if err != nil {
		b.logger.Error("Failed to get failed events for retry",
			zap.Error(err))
		return
	}

	if len(failedEvents) == 0 {
		return // No events to retry
	}

	b.logger.Debug("Processing failed events for retry",
		zap.Int("count", len(failedEvents)))

	for _, eventData := range failedEvents {
		// Convert event data back to Event
		event, err := b.mapToEvent(eventData)
		if err != nil {
			b.logger.Warn("Failed to convert event data to Event for retry",
				zap.Error(err))
			continue
		}

		// Get event ID from data (we need to find it)
		eventID := b.findEventID(eventData)
		if eventID == "" {
			b.logger.Warn("Failed to find event ID for retry")
			continue
		}

		// Mark as processing
		err = b.metaStorage.UpdateEventProcessingStatus(ctx, eventID, string(types.EventStatusProcessing), 0, "", nil)
		if err != nil {
			b.logger.Warn("Failed to mark event as processing",
				zap.String("event_id", eventID),
				zap.Error(err))
			continue
		}

		// Re-publish event
		b.logger.Info("Re-publishing failed event for retry",
			zap.String("event_id", eventID),
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source))

		b.Publish(event)
	}
}

// findEventID extracts the event ID from event data.
func (b *MetaStorageEventBus) findEventID(eventData map[string]interface{}) string {
	if eventID, ok := eventData["event_id"].(string); ok {
		return eventID
	}
	return ""
}

// NewOrderingBuffer creates a new ordering buffer for a source
func NewOrderingBuffer(source string, config *OrderingConfig, logger *zap.Logger) *OrderingBuffer {
	return &OrderingBuffer{
		source:        source,
		expectedSeq:   1, // Start from 1
		lastDelivered: 0,
		buffered:      make(map[int64]types.EventAny),
		timeouts:      make(map[int64]time.Time),
		config:        config,
		logger:        logger,
	}
}

// AddEvent adds an event to the ordering buffer and returns any events ready for delivery
// Returns events in sequence order that can be delivered
func (ob *OrderingBuffer) AddEvent(event types.EventAny) []types.EventAny {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	seq := event.SequenceNumber
	if seq <= 0 {
		// No sequence number - deliver immediately
		return []types.EventAny{event}
	}

	var readyEvents []types.EventAny

	switch ob.config.Mode {
	case types.OrderingModeBestEffort:
		readyEvents = ob.handleBestEffort(seq, event)
	case types.OrderingModeStrict:
		readyEvents = ob.handleStrict(seq, event)
	default:
		// No ordering - deliver immediately
		readyEvents = []types.EventAny{event}
	}

	return readyEvents
}

// handleBestEffort handles events in best-effort mode (reorder if possible)
func (ob *OrderingBuffer) handleBestEffort(seq int64, event types.EventAny) []types.EventAny {
	var readyEvents []types.EventAny

	// If this is the expected sequence, deliver it and any buffered consecutive events
	if seq == ob.expectedSeq {
		readyEvents = append(readyEvents, event)
		ob.expectedSeq++
		ob.lastDelivered = seq

		// Check for consecutive buffered events
		for {
			if buffered, ok := ob.buffered[ob.expectedSeq]; ok {
				readyEvents = append(readyEvents, buffered)
				delete(ob.buffered, ob.expectedSeq)
				ob.expectedSeq++
				ob.lastDelivered = buffered.SequenceNumber
			} else {
				break
			}
		}
	} else if seq > ob.expectedSeq {
		// Future event - buffer it
		ob.buffered[seq] = event
		ob.logger.Debug("Buffered out-of-order event",
			zap.String("source", ob.source),
			zap.Int64("sequence", seq),
			zap.Int64("expected", ob.expectedSeq))
	} else {
		// Past event (seq < expectedSeq) - deliver immediately (best effort)
		ob.logger.Warn("Received past event, delivering out of order",
			zap.String("source", ob.source),
			zap.Int64("sequence", seq),
			zap.Int64("expected", ob.expectedSeq))
		readyEvents = []types.EventAny{event}
	}

	// Cleanup old buffered events (keep buffer size manageable)
	ob.cleanupBuffer()

	return readyEvents
}

// handleStrict handles events in strict mode (wait for missing sequences)
func (ob *OrderingBuffer) handleStrict(seq int64, event types.EventAny) []types.EventAny {
	var readyEvents []types.EventAny

	// If this is the expected sequence, deliver it and any buffered consecutive events
	if seq == ob.expectedSeq {
		readyEvents = append(readyEvents, event)
		ob.expectedSeq++
		ob.lastDelivered = seq
		delete(ob.timeouts, seq)

		// Check for consecutive buffered events
		for {
			if buffered, ok := ob.buffered[ob.expectedSeq]; ok {
				readyEvents = append(readyEvents, buffered)
				delete(ob.buffered, ob.expectedSeq)
				delete(ob.timeouts, ob.expectedSeq)
				ob.expectedSeq++
				ob.lastDelivered = buffered.SequenceNumber
			} else {
				break
			}
		}
	} else if seq > ob.expectedSeq {
		// Future event - buffer it and set timeout
		ob.buffered[seq] = event
		ob.timeouts[seq] = time.Now()
		ob.logger.Debug("Buffered out-of-order event (strict mode)",
			zap.String("source", ob.source),
			zap.Int64("sequence", seq),
			zap.Int64("expected", ob.expectedSeq))
	} else {
		// Past event - log warning but deliver (shouldn't happen in strict mode)
		ob.logger.Warn("Received past event in strict mode",
			zap.String("source", ob.source),
			zap.Int64("sequence", seq),
			zap.Int64("expected", ob.expectedSeq))
		readyEvents = []types.EventAny{event}
	}

	// Check for timed-out events in strict mode
	readyEvents = append(readyEvents, ob.checkTimeouts()...)

	// Cleanup old buffered events
	ob.cleanupBuffer()

	return readyEvents
}

// checkTimeouts checks for timed-out events in strict mode and delivers them
func (ob *OrderingBuffer) checkTimeouts() []types.EventAny {
	if ob.config.Mode != types.OrderingModeStrict {
		return nil
	}

	var readyEvents []types.EventAny
	now := time.Now()

	// Check for missing sequences that have timed out
	for seq := ob.expectedSeq; seq <= ob.expectedSeq+int64(ob.config.BufferSize); seq++ {
		if timeout, ok := ob.timeouts[seq]; ok {
			if now.Sub(timeout) > ob.config.Timeout {
				// Timeout exceeded - skip this sequence and deliver next available
				ob.logger.Warn("Sequence timeout exceeded, skipping",
					zap.String("source", ob.source),
					zap.Int64("sequence", seq),
					zap.Duration("timeout", ob.config.Timeout))
				delete(ob.timeouts, seq)
				ob.expectedSeq = seq + 1
			}
		} else if event, ok := ob.buffered[seq]; ok {
			// Found buffered event - deliver it
			readyEvents = append(readyEvents, event)
			delete(ob.buffered, seq)
			delete(ob.timeouts, seq)
			ob.expectedSeq = seq + 1
			ob.lastDelivered = seq
			break // Only deliver one at a time to maintain ordering
		}
	}

	return readyEvents
}

// cleanupBuffer removes old buffered events to prevent memory growth
func (ob *OrderingBuffer) cleanupBuffer() {
	// Remove events that are too far behind (more than buffer size)
	maxBufferSize := ob.config.BufferSize
	if len(ob.buffered) > maxBufferSize*2 {
		// Remove oldest buffered events
		removed := 0
		for seq := range ob.buffered {
			if seq < ob.expectedSeq-int64(maxBufferSize) {
				delete(ob.buffered, seq)
				delete(ob.timeouts, seq)
				removed++
			}
		}
		if removed > 0 {
			ob.logger.Debug("Cleaned up old buffered events",
				zap.String("source", ob.source),
				zap.Int("removed", removed))
		}
	}
}
