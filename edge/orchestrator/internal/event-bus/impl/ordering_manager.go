package impl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// OrderingManager manages event ordering per source.
// It supports three ordering modes:
// - None: No ordering guarantees (events pass through immediately)
// - BestEffort: Reorder events if possible, but don't wait for missing sequences
// - Strict: Buffer events and wait for missing sequences (with timeout)
type OrderingManager struct {
	config *types.OrderingConfig
	logger *zap.Logger

	// Per-source ordering state
	mu              sync.RWMutex
	sourceBuffers   map[string]*sourceBuffer // source -> buffer
	expectedSeq     map[string]int64          // source -> expected sequence number
	timeoutTimers   map[string]*time.Timer    // source -> timeout timer (for strict mode)

	// Metrics
	metricsMu          sync.RWMutex
	bufferedEvents     int64 // Total events currently buffered
	reorderedEvents    int64 // Total events reordered
	timeoutEvents      int64 // Total events that timed out waiting for sequence
	droppedEvents      int64 // Total events dropped due to buffer overflow
}

// sourceBuffer holds buffered events for a single source.
type sourceBuffer struct {
	events map[int64]*bufferedEvent // sequence -> event
	mu     sync.RWMutex
}

// bufferedEvent represents an event waiting in the ordering buffer.
type bufferedEvent struct {
	event     types.EventAny
	receivedAt time.Time
}

// NewOrderingManager creates a new ordering manager.
func NewOrderingManager(config *types.OrderingConfig, logger *zap.Logger) *OrderingManager {
	if config == nil {
		// Use default config if not provided
		config = &types.OrderingConfig{
			Mode:             types.OrderingModeNone,
			BufferSize:       100,
			Timeout:          30 * time.Second,
			PerSourceOrdering: true,
		}
		config.Validate()
	}

	return &OrderingManager{
		config:        config,
		logger:        logger,
		sourceBuffers: make(map[string]*sourceBuffer),
		expectedSeq:   make(map[string]int64),
		timeoutTimers: make(map[string]*time.Timer),
	}
}

// ProcessEvent processes an event according to the ordering mode.
// Returns the event(s) to publish (may be empty, single, or multiple if buffered events are released).
// In "none" mode, returns the event immediately.
// In "best_effort" mode, attempts to reorder but doesn't wait.
// In "strict" mode, buffers and waits for missing sequences (with timeout).
func (m *OrderingManager) ProcessEvent(ctx context.Context, event types.EventAny) ([]types.EventAny, error) {
	// Fast path: no ordering mode
	if m.config.Mode == types.OrderingModeNone {
		return []types.EventAny{event}, nil
	}

	// If sequence number is not set, pass through immediately (no ordering possible)
	if event.SequenceNumber == 0 {
		m.logger.Debug("Event has no sequence number, passing through without ordering",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source))
		return []types.EventAny{event}, nil
	}

	// Determine source key (per-source ordering)
	sourceKey := event.Source
	if !m.config.PerSourceOrdering {
		// Global ordering: use empty string as source key
		sourceKey = ""
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create source buffer
	buffer := m.getOrCreateSourceBuffer(sourceKey)

	// Get expected sequence number for this source
	expectedSeq := m.expectedSeq[sourceKey]
	if expectedSeq == 0 {
		// First event from this source, initialize expected sequence
		expectedSeq = event.SequenceNumber
		m.expectedSeq[sourceKey] = expectedSeq
	}

	// Process based on ordering mode
	switch m.config.Mode {
	case types.OrderingModeBestEffort:
		return m.processBestEffort(ctx, event, sourceKey, expectedSeq, buffer)
	case types.OrderingModeStrict:
		return m.processStrict(ctx, event, sourceKey, expectedSeq, buffer)
	default:
		// Fallback to none mode
		return []types.EventAny{event}, nil
	}
}

// processBestEffort processes an event in best-effort ordering mode.
// Attempts to reorder events but doesn't wait for missing sequences.
func (m *OrderingManager) processBestEffort(ctx context.Context, event types.EventAny, sourceKey string, expectedSeq int64, buffer *sourceBuffer) ([]types.EventAny, error) {
	seq := event.SequenceNumber

	// If this is the expected sequence, process immediately
	if seq == expectedSeq {
		// Update expected sequence
		m.expectedSeq[sourceKey] = seq + 1

		// Check if we can release any buffered events
		released := m.releaseBufferedEvents(sourceKey, buffer, seq+1)

		// Record metrics
		if len(released) > 0 {
			m.metricsMu.Lock()
			m.reorderedEvents += int64(len(released))
			m.metricsMu.Unlock()
		}

		// Return this event plus any released buffered events
		return append([]types.EventAny{event}, released...), nil
	}

	// If this is a future sequence, buffer it
	if seq > expectedSeq {
		// Check buffer overflow
		if m.isBufferFull(buffer) {
			m.metricsMu.Lock()
			m.droppedEvents++
			m.metricsMu.Unlock()

			m.logger.Warn("Ordering buffer full, dropping event",
				zap.String("source", sourceKey),
				zap.Int64("sequence", seq),
				zap.Int64("expected", expectedSeq))
			return nil, fmt.Errorf("ordering buffer full for source %s", sourceKey)
		}

		// Buffer the event
		buffer.mu.Lock()
		buffer.events[seq] = &bufferedEvent{
			event:     event,
			receivedAt: time.Now(),
		}
		buffer.mu.Unlock()

		m.metricsMu.Lock()
		m.bufferedEvents++
		m.metricsMu.Unlock()

		m.logger.Debug("Buffered out-of-order event",
			zap.String("source", sourceKey),
			zap.Int64("sequence", seq),
			zap.Int64("expected", expectedSeq))
		return nil, nil // No events to publish yet
	}

	// If this is a past sequence (duplicate or out-of-order), drop it
	m.logger.Debug("Dropping duplicate or out-of-order event",
		zap.String("source", sourceKey),
		zap.Int64("sequence", seq),
		zap.Int64("expected", expectedSeq))
	return nil, nil
}

// processStrict processes an event in strict ordering mode.
// Buffers events and waits for missing sequences (with timeout).
func (m *OrderingManager) processStrict(ctx context.Context, event types.EventAny, sourceKey string, expectedSeq int64, buffer *sourceBuffer) ([]types.EventAny, error) {
	seq := event.SequenceNumber

	// If this is the expected sequence, process immediately
	if seq == expectedSeq {
		// Cancel any timeout timer for this source
		if timer, ok := m.timeoutTimers[sourceKey]; ok {
			timer.Stop()
			delete(m.timeoutTimers, sourceKey)
		}

		// Update expected sequence
		m.expectedSeq[sourceKey] = seq + 1

		// Check if we can release any buffered events
		released := m.releaseBufferedEvents(sourceKey, buffer, seq+1)

		// Record metrics
		if len(released) > 0 {
			m.metricsMu.Lock()
			m.reorderedEvents += int64(len(released))
			m.metricsMu.Unlock()
		}

		// Return this event plus any released buffered events
		return append([]types.EventAny{event}, released...), nil
	}

	// If this is a future sequence, buffer it and start timeout timer
	if seq > expectedSeq {
		// Check buffer overflow
		if m.isBufferFull(buffer) {
			m.metricsMu.Lock()
			m.droppedEvents++
			m.metricsMu.Unlock()

			m.logger.Warn("Ordering buffer full, dropping event",
				zap.String("source", sourceKey),
				zap.Int64("sequence", seq),
				zap.Int64("expected", expectedSeq))
			return nil, fmt.Errorf("ordering buffer full for source %s", sourceKey)
		}

		// Buffer the event
		buffer.mu.Lock()
		buffer.events[seq] = &bufferedEvent{
			event:     event,
			receivedAt: time.Now(),
		}
		buffer.mu.Unlock()

		m.metricsMu.Lock()
		m.bufferedEvents++
		m.metricsMu.Unlock()

		// Start or restart timeout timer
		if timer, ok := m.timeoutTimers[sourceKey]; ok {
			timer.Stop()
		}
		timer := time.AfterFunc(m.config.Timeout, func() {
			m.handleTimeout(sourceKey, expectedSeq)
		})
		m.timeoutTimers[sourceKey] = timer

		m.logger.Debug("Buffered out-of-order event (strict mode)",
			zap.String("source", sourceKey),
			zap.Int64("sequence", seq),
			zap.Int64("expected", expectedSeq),
			zap.Duration("timeout", m.config.Timeout))
		return nil, nil // No events to publish yet
	}

	// If this is a past sequence (duplicate or out-of-order), drop it
	m.logger.Debug("Dropping duplicate or out-of-order event (strict mode)",
		zap.String("source", sourceKey),
		zap.Int64("sequence", seq),
		zap.Int64("expected", expectedSeq))
	return nil, nil
}

// releaseBufferedEvents releases buffered events that are now in order.
// Returns the released events in sequence order.
func (m *OrderingManager) releaseBufferedEvents(sourceKey string, buffer *sourceBuffer, nextExpectedSeq int64) []types.EventAny {
	var released []types.EventAny

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	// Release events in sequence order
	for {
		if buffered, ok := buffer.events[nextExpectedSeq]; ok {
			released = append(released, buffered.event)
			delete(buffer.events, nextExpectedSeq)
			nextExpectedSeq++

			m.metricsMu.Lock()
			m.bufferedEvents--
			m.metricsMu.Unlock()
		} else {
			break
		}
	}

	// Update expected sequence
	if len(released) > 0 {
		m.mu.Lock()
		m.expectedSeq[sourceKey] = nextExpectedSeq
		m.mu.Unlock()
	}

	return released
}

// handleTimeout handles timeout for missing sequences in strict mode.
// Releases all buffered events up to the next missing sequence.
func (m *OrderingManager) handleTimeout(sourceKey string, expectedSeq int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	buffer, ok := m.sourceBuffers[sourceKey]
	if !ok {
		return
	}

	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	// Find the next available sequence (or end of buffer)
	nextSeq := expectedSeq
	for {
		if _, ok := buffer.events[nextSeq]; ok {
			// Found next sequence, update expected
			m.expectedSeq[sourceKey] = nextSeq + 1
			delete(buffer.events, nextSeq)

			m.metricsMu.Lock()
			m.bufferedEvents--
			m.timeoutEvents++
			m.metricsMu.Unlock()

			m.logger.Warn("Timeout waiting for sequence, releasing buffered event",
				zap.String("source", sourceKey),
				zap.Int64("sequence", nextSeq),
				zap.Int64("expected", expectedSeq))

			// Continue to next sequence
			nextSeq++
		} else {
			// No more buffered events, update expected sequence
			if nextSeq > expectedSeq {
				m.expectedSeq[sourceKey] = nextSeq
			}
			break
		}
	}

	// Clean up timeout timer
	delete(m.timeoutTimers, sourceKey)
}

// getOrCreateSourceBuffer gets or creates a source buffer for the given source.
// Must be called with m.mu locked.
func (m *OrderingManager) getOrCreateSourceBuffer(sourceKey string) *sourceBuffer {
	buffer, ok := m.sourceBuffers[sourceKey]
	if !ok {
		buffer = &sourceBuffer{
			events: make(map[int64]*bufferedEvent),
		}
		m.sourceBuffers[sourceKey] = buffer
	}
	return buffer
}

// isBufferFull checks if the buffer is full.
func (m *OrderingManager) isBufferFull(buffer *sourceBuffer) bool {
	buffer.mu.RLock()
	defer buffer.mu.RUnlock()
	return len(buffer.events) >= m.config.BufferSize
}

// GetMetrics returns ordering metrics.
func (m *OrderingManager) GetMetrics() OrderingMetrics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Count total buffered events across all sources
	totalBuffered := int64(0)
	for _, buffer := range m.sourceBuffers {
		buffer.mu.RLock()
		totalBuffered += int64(len(buffer.events))
		buffer.mu.RUnlock()
	}

	return OrderingMetrics{
		BufferedEvents:  totalBuffered,
		ReorderedEvents: m.reorderedEvents,
		TimeoutEvents:   m.timeoutEvents,
		DroppedEvents:   m.droppedEvents,
		ActiveSources:   len(m.sourceBuffers),
	}
}

// OrderingMetrics contains ordering operation metrics.
type OrderingMetrics struct {
	BufferedEvents  int64 // Total events currently buffered
	ReorderedEvents int64 // Total events reordered
	TimeoutEvents   int64 // Total events that timed out waiting for sequence
	DroppedEvents   int64 // Total events dropped due to buffer overflow
	ActiveSources   int   // Number of active sources with buffers
}

// Flush flushes all buffered events for a source (or all sources if sourceKey is empty).
// This is useful for cleanup or when switching ordering modes.
func (m *OrderingManager) Flush(sourceKey string) []types.EventAny {
	m.mu.Lock()
	defer m.mu.Unlock()

	var flushed []types.EventAny

	if sourceKey == "" {
		// Flush all sources
		for key, buffer := range m.sourceBuffers {
			flushed = append(flushed, m.flushSource(key, buffer)...)
		}
	} else {
		// Flush specific source
		if buffer, ok := m.sourceBuffers[sourceKey]; ok {
			flushed = append(flushed, m.flushSource(sourceKey, buffer)...)
		}
	}

	return flushed
}

// flushSource flushes all buffered events for a source.
// Must be called with m.mu locked.
func (m *OrderingManager) flushSource(sourceKey string, buffer *sourceBuffer) []types.EventAny {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	var flushed []types.EventAny

	// Collect all events in sequence order
	sequences := make([]int64, 0, len(buffer.events))
	for seq := range buffer.events {
		sequences = append(sequences, seq)
	}

	// Sort sequences (simple insertion sort for small arrays)
	for i := 1; i < len(sequences); i++ {
		key := sequences[i]
		j := i - 1
		for j >= 0 && sequences[j] > key {
			sequences[j+1] = sequences[j]
			j--
		}
		sequences[j+1] = key
	}

	// Release events in order
	for _, seq := range sequences {
		if bufferedEvent, ok := buffer.events[seq]; ok {
			flushed = append(flushed, bufferedEvent.event)
			delete(buffer.events, seq)

			m.metricsMu.Lock()
			m.bufferedEvents--
			m.metricsMu.Unlock()
		}
	}

	// Clean up if buffer is empty
	if len(buffer.events) == 0 {
		delete(m.sourceBuffers, sourceKey)
		delete(m.expectedSeq, sourceKey)
		if timer, ok := m.timeoutTimers[sourceKey]; ok {
			timer.Stop()
			delete(m.timeoutTimers, sourceKey)
		}
	}

	return flushed
}

