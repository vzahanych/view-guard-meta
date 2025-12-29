package impl

import (
	"context"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// PersistenceBuffer manages buffered event persistence with batching.
// This optimizes persistence performance by batching writes and using async buffering.
// Note: Batch writes (multiple events in one transaction) require provider interface enhancement.
// For now, this buffers events and flushes them individually but asynchronously.
type PersistenceBuffer struct {
	provider types.EventBusProvider
	logger   *zap.Logger

	// Buffer configuration
	batchSize     int           // Maximum number of events per batch
	flushInterval time.Duration // Maximum time to wait before flushing a batch

	// Event buffer for batching
	mu          sync.Mutex
	eventBuffer []types.EventAny
	lastFlush   time.Time

	// Background flush worker
	flushCtx    context.Context
	flushCancel context.CancelFunc
	flushWg     sync.WaitGroup
}

// NewPersistenceBuffer creates a new persistence buffer.
func NewPersistenceBuffer(provider types.EventBusProvider, logger *zap.Logger, batchSize int, flushInterval time.Duration) *PersistenceBuffer {
	if batchSize <= 0 {
		batchSize = 100 // Default: 100 events per batch
	}
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond // Default: 100ms
	}

	ctx, cancel := context.WithCancel(context.Background())

	buffer := &PersistenceBuffer{
		provider:      provider,
		logger:        logger,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		eventBuffer:   make([]types.EventAny, 0, batchSize),
		lastFlush:     time.Now(),
		flushCtx:      ctx,
		flushCancel:   cancel,
	}

	// Start background flush worker
	buffer.startFlushWorker()

	return buffer
}

// PersistEvent adds an event to the buffer for batched persistence.
// This method is non-blocking and returns immediately.
func (b *PersistenceBuffer) PersistEvent(ctx context.Context, event types.EventAny) error {
	b.mu.Lock()
	b.eventBuffer = append(b.eventBuffer, event)
	shouldFlush := len(b.eventBuffer) >= b.batchSize
	b.mu.Unlock()

	// Flush immediately if buffer is full
	if shouldFlush {
		go b.flushBuffer(ctx)
	}

	return nil
}

// flushBuffer flushes the event buffer to the provider in a batch.
func (b *PersistenceBuffer) flushBuffer(ctx context.Context) {
	b.mu.Lock()
	if len(b.eventBuffer) == 0 {
		b.mu.Unlock()
		return
	}

	// Copy buffer and clear it
	events := make([]types.EventAny, len(b.eventBuffer))
	copy(events, b.eventBuffer)
	b.eventBuffer = b.eventBuffer[:0]
	b.lastFlush = time.Now()
	b.mu.Unlock()

	// Persist events in batch (one at a time for now, as provider doesn't support batch writes)
	// TODO: Enhance provider interface to support batch writes for better performance
	// For now, we persist events individually but asynchronously to avoid blocking
	for _, event := range events {
		// Persist each event asynchronously to avoid blocking
		go func(ev types.EventAny) {
			if err := b.provider.PersistEvent(ctx, ev); err != nil {
				b.logger.Warn("Failed to persist event in batch",
					zap.String("event_type", string(ev.Type)),
					zap.String("source", ev.Source),
					zap.Error(err))
			}
		}(event)
	}
}

// startFlushWorker starts a background goroutine that periodically flushes the buffer.
func (b *PersistenceBuffer) startFlushWorker() {
	b.flushWg.Add(1)
	go func() {
		defer b.flushWg.Done()

		ticker := time.NewTicker(b.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-b.flushCtx.Done():
				// Flush remaining events on shutdown
				b.flushBuffer(context.Background())
				return
			case <-ticker.C:
				// Check if buffer needs flushing
				b.mu.Lock()
				shouldFlush := len(b.eventBuffer) > 0 && time.Since(b.lastFlush) >= b.flushInterval
				b.mu.Unlock()

				if shouldFlush {
					b.flushBuffer(context.Background())
				}
			}
		}
	}()
}


// Close closes the persistence buffer and flushes remaining events.
func (b *PersistenceBuffer) Close() error {
	b.flushCancel()
	b.flushWg.Wait()

	// Flush remaining events
	b.flushBuffer(context.Background())

	return nil
}

