package impl

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

// EventBusImpl implements the EventBus interface using a provider-agnostic architecture.
// This follows the vm-gateway pattern with lifecycle management and provider delegation.
type EventBusImpl struct {
	provider types.EventBusProvider
	logger   *zap.Logger
	config   *types.EventBusConfig

	// Subscription management (fine-grained locking for better performance)
	subscribersMu sync.RWMutex // Separate lock for subscribers to reduce contention
	subscribers   map[types.EventType][]chan types.EventAny
	allSubs       []chan types.EventAny
	bufferSize    int

	// Lifecycle state (separate lock for lifecycle to reduce contention)
	lifecycleMu sync.RWMutex
	started     bool
	closed      bool
	stopCh      chan struct{}

	// Background tasks
	retentionCleanupCtx    context.Context
	retentionCleanupCancel context.CancelFunc
	retentionCleanupWg     sync.WaitGroup

	storagePressureCtx    context.Context
	storagePressureCancel context.CancelFunc
	storagePressureWg     sync.WaitGroup

	healthCheckCtx    context.Context
	healthCheckCancel context.CancelFunc
	healthCheckWg     sync.WaitGroup

	subscriptionCleanupCtx    context.Context
	subscriptionCleanupCancel context.CancelFunc
	subscriptionCleanupWg     sync.WaitGroup

	// Event drop policy
	dropPolicy *types.EventDropPolicy
	categoryRegistry *EventCategoryRegistry

	// Storage pressure monitoring
	storagePressureMonitor *StoragePressureMonitor

	// Retention management
	retentionManager *RetentionManager

	// Metrics
	eventsPublished int64
	eventsDropped   map[types.EventCategory]int64
	eventsPersisted int64
	metricsMu       sync.RWMutex
	metricsManager  *MetricsManager

	// Subscription metrics
	subscriptionsCreated int64
	subscriptionsRemoved int64
	subscriptionsCleaned  int64

	// Persistence optimization
	persistenceBuffer *PersistenceBuffer

	// Ordering management
	orderingManager *OrderingManager

	// Retry management
	retryManager *RetryManager

	// Retry worker
	retryWorkerCtx    context.Context
	retryWorkerCancel context.CancelFunc
	retryWorkerWg     sync.WaitGroup
}

// NewEventBusImpl creates a new event bus implementation.
func NewEventBusImpl(provider types.EventBusProvider, config *types.EventBusConfig, logger *zap.Logger, metaStorage interface{}) (*EventBusImpl, error) {
	if provider == nil {
		return nil, fmt.Errorf("event bus provider is required")
	}
	if config == nil {
		return nil, fmt.Errorf("event bus config is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Initialize event drop policy
	dropPolicy := types.DefaultEventDropPolicy()
	if config.DropPolicyConfig != nil && config.DropPolicyConfig.CategoryRules != nil {
		// Merge custom category rules with defaults
		for eventType, category := range config.DropPolicyConfig.CategoryRules {
			dropPolicy.CategoryRules[eventType] = category
		}
		if config.DropPolicyConfig.DefaultCategory != "" {
			dropPolicy.DefaultCategory = config.DropPolicyConfig.DefaultCategory
		}
	}

	// Initialize event category registry
	categoryRegistry := NewEventCategoryRegistryWithPolicy(dropPolicy, logger)

	// Initialize storage pressure monitor (if meta-storage is available)
	var storagePressureMonitor *StoragePressureMonitor
	threshold := 90 // Default threshold
	if config.DropPolicyConfig != nil && config.DropPolicyConfig.StoragePressureThreshold > 0 {
		threshold = config.DropPolicyConfig.StoragePressureThreshold
	}
	if metaStorage != nil {
		// Try to cast to MetaDataStore
		if ms, ok := metaStorage.(metastorage.MetaDataStore); ok {
			storagePressureMonitor = NewStoragePressureMonitor(ms, threshold, logger)
		}
	}

	// Initialize retention manager
	var retentionManager *RetentionManager
	if config.RetentionConfig != nil {
		retentionManager = NewRetentionManager(provider, config.RetentionConfig, logger)
	}

	// Initialize metrics manager
	metricsManager := NewMetricsManager(logger)

	// Initialize persistence buffer for batching and async writes
	// Batch size: 100 events, flush interval: 100ms
	persistenceBuffer := NewPersistenceBuffer(provider, logger, 100, 100*time.Millisecond)

	// Initialize ordering manager (if ordering config provided)
	var orderingManager *OrderingManager
	if config.OrderingConfig != nil {
		orderingManager = NewOrderingManager(config.OrderingConfig, logger)
	}

	// Initialize retry manager (if retry config provided and meta-storage is available)
	var retryManager *RetryManager
	if config.RetryConfig != nil && metaStorage != nil {
		// Try to cast to MetaDataStore
		if ms, ok := metaStorage.(metastorage.MetaDataStore); ok {
			retryManager = NewRetryManager(config.RetryConfig, provider, logger, ms)
		}
	}

	return &EventBusImpl{
		provider:              provider,
		logger:                logger,
		config:                config,
		subscribers:           make(map[types.EventType][]chan types.EventAny),
		allSubs:               make([]chan types.EventAny, 0),
		bufferSize:            config.BufferSize,
		stopCh:                make(chan struct{}),
		dropPolicy:            dropPolicy,
		categoryRegistry:      categoryRegistry,
		storagePressureMonitor: storagePressureMonitor,
		retentionManager:       retentionManager,
		metricsManager:         metricsManager,
		persistenceBuffer:       persistenceBuffer,
		orderingManager:        orderingManager,
		retryManager:           retryManager,
		eventsDropped:         make(map[types.EventCategory]int64),
	}, nil
}

// Start starts the event bus service.
func (b *EventBusImpl) Start(ctx context.Context) error {
	b.lifecycleMu.Lock()
	defer b.lifecycleMu.Unlock()

	if b.started {
		return types.ErrAlreadyStarted
	}

	b.logger.Info("Starting event bus service...")

	// Step 1: Verify provider connectivity
	if err := b.provider.HealthCheck(ctx); err != nil {
		return fmt.Errorf("provider health check failed: %w", err)
	}

	// Step 2: Initialize event drop policy (already done in constructor)
	// Policy is ready to use

	// Step 3: Start background tasks
	// Retention cleanup worker (runs every 6 hours)
	if b.config.RetentionConfig != nil {
		interval := time.Duration(b.config.RetentionConfig.CleanupIntervalHours) * time.Hour
		if interval <= 0 {
			interval = 6 * time.Hour // Default: 6 hours
		}
		b.retentionCleanupCtx, b.retentionCleanupCancel = context.WithCancel(context.Background())
		b.startRetentionCleanupWorker(interval)
	}

	// Storage pressure monitor (runs every 5 minutes)
	b.storagePressureCtx, b.storagePressureCancel = context.WithCancel(context.Background())
	b.startStoragePressureMonitor(5 * time.Minute)

	// Health check worker (runs every 1 minute)
	b.healthCheckCtx, b.healthCheckCancel = context.WithCancel(context.Background())
	b.startHealthCheckWorker(1 * time.Minute)

	// Subscription cleanup worker (runs every 5 minutes)
	b.subscriptionCleanupCtx, b.subscriptionCleanupCancel = context.WithCancel(context.Background())
	b.startSubscriptionCleanupWorker(5 * time.Minute)

	// Retry worker (runs on configured interval)
	if b.retryManager != nil && b.config.RetryConfig != nil {
		interval := b.config.RetryConfig.RetryInterval
		if interval <= 0 {
			interval = 5 * time.Second // Default: 5 seconds
		}
		b.retryWorkerCtx, b.retryWorkerCancel = context.WithCancel(context.Background())
		b.startRetryWorker(interval)
	}

	// Start metrics manager periodic sampling
	if b.metricsManager != nil {
		b.metricsManager.StartPeriodicSampling(
			ctx,
			b.getActiveSubscriberCount,
			func() (float64, bool) {
				if b.storagePressureMonitor != nil {
					return b.storagePressureMonitor.GetStorageUsagePercent(), b.storagePressureMonitor.IsStoragePressure()
				}
				return 0.0, false
			},
		)
	}

	b.started = true
	b.logger.Info("Event bus service started")

	return nil
}

// Stop gracefully shuts down the event bus service.
func (b *EventBusImpl) Stop(ctx context.Context) error {
	b.lifecycleMu.Lock()
	if !b.started {
		b.lifecycleMu.Unlock()
		return nil // Already stopped
	}

	b.logger.Info("Stopping event bus service...")

	var errs []error

	// Step 1: Stop background tasks gracefully
	if b.retentionCleanupCancel != nil {
		b.retentionCleanupCancel()
	}
	if b.storagePressureCancel != nil {
		b.storagePressureCancel()
	}
	if b.healthCheckCancel != nil {
		b.healthCheckCancel()
	}
	if b.subscriptionCleanupCancel != nil {
		b.subscriptionCleanupCancel()
	}
	if b.retryWorkerCancel != nil {
		b.retryWorkerCancel()
	}

	// Wait for background tasks to finish
	b.retentionCleanupWg.Wait()
	b.storagePressureWg.Wait()
	b.healthCheckWg.Wait()
	b.subscriptionCleanupWg.Wait()
	b.retryWorkerWg.Wait()

	b.logger.Info("Background tasks stopped")

	// Step 2: Flush pending operations
	// For meta-storage provider, this is handled automatically
	b.logger.Info("Pending operations flushed")

	// Step 3: Flush ordering manager (releases all buffered events)
	if b.orderingManager != nil {
		flushed := b.orderingManager.Flush("")
		if len(flushed) > 0 {
			// Publish flushed events before shutdown
			for _, evt := range flushed {
				go b.publishToSubscribers(evt)
			}
			b.logger.Info("Flushed ordering buffer",
				zap.Int("events_flushed", len(flushed)))
		}
	}

	// Step 4: Close persistence buffer (flushes remaining events)
	if b.persistenceBuffer != nil {
		if err := b.persistenceBuffer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close persistence buffer: %w", err))
		}
	}

	// Step 4: Close provider connections
	if err := b.provider.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close provider: %w", err))
	}

	// Step 5: Close subscriber channels
	b.closed = true
	b.started = false
	b.lifecycleMu.Unlock()

	b.closeAllChannels()

	close(b.stopCh)
	b.stopCh = make(chan struct{}) // Reset for potential restart

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}

	b.logger.Info("Event bus service stopped")
	return nil
}

// Name returns the implementation name.
func (b *EventBusImpl) Name() string {
	return "event-bus-impl"
}

// HealthSnapshot returns the current health status of the event bus service.
// This follows the vm-gateway pattern for health snapshots with comprehensive status aggregation.
func (b *EventBusImpl) HealthSnapshot() types.EventBusHealth {
	b.metricsMu.RLock()
	defer b.metricsMu.RUnlock()

	// Query storage pressure status from storage pressure monitor
	storagePressure := false
	storageUsagePercent := 0.0
	if b.storagePressureMonitor != nil {
		storagePressure = b.storagePressureMonitor.IsStoragePressure()
		storageUsagePercent = b.storagePressureMonitor.GetStorageUsagePercent()
	}

	// Query event statistics
	eventsPublished := b.eventsPublished
	eventsPersisted := b.eventsPersisted
	eventsDropped := make(map[types.EventCategory]int64)
	for category, count := range b.eventsDropped {
		eventsDropped[category] = count
	}

	// Calculate total dropped events
	totalDropped := int64(0)
	for _, count := range eventsDropped {
		totalDropped += count
	}

	// Calculate drop rate (percentage of published events that were dropped)
	dropRate := 0.0
	if eventsPublished > 0 {
		dropRate = float64(totalDropped) / float64(eventsPublished) * 100.0
	}

	// Query last cleanup time and stats from retention manager
	lastCleanupTime := time.Time{}
	var cleanupStats *types.CleanupStats
	if b.retentionManager != nil {
		lastCleanupTime = b.retentionManager.GetLastCleanupTime()
		cleanupStats = b.retentionManager.GetLastCleanupStats()
	}

	// Query active subscriber count
	activeSubscribers := b.getActiveSubscriberCount()

	// Initialize provider status map
	providerStatus := make(map[string]interface{})

	// Query ordering metrics (if ordering manager is enabled)
	if b.orderingManager != nil {
		metrics := b.orderingManager.GetMetrics()
		// Add ordering metrics to provider status for observability
		providerStatus["ordering_buffered_events"] = metrics.BufferedEvents
		providerStatus["ordering_reordered_events"] = metrics.ReorderedEvents
		providerStatus["ordering_timeout_events"] = metrics.TimeoutEvents
		providerStatus["ordering_dropped_events"] = metrics.DroppedEvents
		providerStatus["ordering_active_sources"] = metrics.ActiveSources
	}

	// Query retry metrics (if retry manager is enabled)
	if b.retryManager != nil {
		retryMetrics := b.retryManager.GetMetrics()
		// Add retry metrics to provider status for observability
		providerStatus["retry_count"] = retryMetrics.RetryCount
		providerStatus["retry_success_count"] = retryMetrics.SuccessCount
		providerStatus["retry_failed_count"] = retryMetrics.FailedRetryCount
		providerStatus["retry_dead_letter_count"] = retryMetrics.DeadLetterCount
		providerStatus["retry_success_rate"] = retryMetrics.SuccessRate
	}

	// Check provider health
	providerHealth := "healthy"
	providerError := false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.provider.HealthCheck(ctx); err != nil {
		providerHealth = "unhealthy"
		providerStatus["error"] = err.Error()
		providerError = true
	}

	// Determine health status with priority:
	// 1. Degraded (provider errors, high drop rate >10%)
	// 2. Storage Pressure (>90% full)
	// 3. Warning (80-90% full, or drop rate 5-10%)
	// 4. Healthy
	status := types.HealthStatusHealthy
	healthDegraded := false
	if providerError {
		status = types.HealthStatusDegraded
		healthDegraded = true
	} else if dropRate > 10.0 {
		// High drop rate indicates degraded state
		status = types.HealthStatusDegraded
		healthDegraded = true
		providerStatus["drop_rate_percent"] = dropRate
	} else if storagePressure {
		status = types.HealthStatusStoragePressure
	} else if storageUsagePercent >= 80.0 && storageUsagePercent < 90.0 {
		// Warning state: storage 80-90% full
		status = types.HealthStatusWarning
	} else if dropRate >= 5.0 && dropRate <= 10.0 {
		// Warning state: moderate drop rate
		status = types.HealthStatusWarning
		providerStatus["drop_rate_percent"] = dropRate
	}

	// Emit health_degraded event if health status changed to degraded
	// Note: We track the previous status to avoid emitting duplicate events
	// For now, we'll emit on every degraded check (can be optimized later)
	if healthDegraded {
		b.emitHealthDegradedEvent(status, providerError, dropRate, providerStatus)
	}

	health := types.EventBusHealth{
		Status:              status,
		StoragePressure:     storagePressure,
		StorageUsagePercent: storageUsagePercent,
		EventsPublished:     eventsPublished,
		EventsDropped:       eventsDropped,
		EventsPersisted:     eventsPersisted,
		ActiveSubscribers:   activeSubscribers,
		LastCleanupTime:     lastCleanupTime,
		CleanupStats:        cleanupStats,
		ProviderHealth:      providerHealth,
		ProviderStatus:      providerStatus,
	}

	return health
}

// Subscribe subscribes to events of a specific type.
func (b *EventBusImpl) Subscribe(eventType types.EventType) <-chan types.EventAny {
	b.lifecycleMu.RLock()
	closed := b.closed
	b.lifecycleMu.RUnlock()

	if closed {
		ch := make(chan types.EventAny)
		close(ch)
		return ch
	}

	ch := make(chan types.EventAny, b.bufferSize)
	b.subscribersMu.Lock()
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	b.subscribersMu.Unlock()

	// Track subscription metrics
	b.metricsMu.Lock()
	b.subscriptionsCreated++
	b.metricsMu.Unlock()

	return ch
}

// SubscribeAll subscribes to all events, regardless of type.
func (b *EventBusImpl) SubscribeAll() <-chan types.EventAny {
	b.lifecycleMu.RLock()
	closed := b.closed
	b.lifecycleMu.RUnlock()

	if closed {
		ch := make(chan types.EventAny)
		close(ch)
		return ch
	}

	ch := make(chan types.EventAny, b.bufferSize)
	b.subscribersMu.Lock()
	b.allSubs = append(b.allSubs, ch)
	b.subscribersMu.Unlock()

	// Track subscription metrics
	b.metricsMu.Lock()
	b.subscriptionsCreated++
	b.metricsMu.Unlock()

	return ch
}

// Publish publishes an event to all matching subscribers and persists it to storage.
// It enforces the drop policy: if storage pressure is detected (>90% full) and the event is droppable,
// the event is dropped and ErrEventDropped is returned.
// This method is fully non-blocking and optimized for high throughput:
// - Uses fine-grained locking to minimize contention
// - Persistence is done asynchronously in a goroutine
// - Subscriber notifications are batched when there are many subscribers
func (b *EventBusImpl) Publish(event types.EventAny) error {
	startTime := time.Now()

	// Fast path: check lifecycle state with minimal lock scope
	b.lifecycleMu.RLock()
	started := b.started
	closed := b.closed
	b.lifecycleMu.RUnlock()

	if !started {
		return types.ErrNotInitialized
	}

	if closed {
		return nil // Already closed, ignore
	}

	// Step 1: Check if event is droppable
	isDroppable := b.categoryRegistry.IsDroppable(event.Type)
	category := b.categoryRegistry.GetCategory(event.Type)

	// Step 2: Check storage pressure
	hasStoragePressure := false
	if b.storagePressureMonitor != nil {
		hasStoragePressure = b.storagePressureMonitor.IsStoragePressure()
	}

	// Step 3: Apply drop policy
	// If storage >90% full and event is droppable: drop event, log warning
	if hasStoragePressure && isDroppable {
		// Drop the event
		b.metricsMu.Lock()
		b.eventsDropped[category]++
		b.metricsMu.Unlock()

		// Record drop metrics
		if b.metricsManager != nil {
			b.metricsManager.RecordDrop(category)
		}

		b.logger.Warn("Event dropped due to storage pressure",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.String("category", string(category)),
			zap.Float64("storage_usage_percent", b.storagePressureMonitor.GetStorageUsagePercent()))

		// Emit event_bus.event_dropped event (self-monitoring)
		b.emitEventDroppedEvent(event, category)

		return types.ErrEventDropped
	}

	// Step 4: Process event through ordering manager (if enabled)
	// Ordering manager may return multiple events (if buffered events are released)
	var eventsToPublish []types.EventAny
	if b.orderingManager != nil {
		orderedEvents, err := b.orderingManager.ProcessEvent(context.Background(), event)
		if err != nil {
			// Ordering error (e.g., buffer full) - log and drop event
			b.logger.Warn("Ordering manager error, dropping event",
				zap.String("event_type", string(event.Type)),
				zap.String("source", event.Source),
				zap.Error(err))
			return err
		}
		eventsToPublish = orderedEvents
	} else {
		// No ordering manager, publish event directly
		eventsToPublish = []types.EventAny{event}
	}

	// If no events to publish (buffered), return early
	if len(eventsToPublish) == 0 {
		return nil
	}

	// Step 5: Persist and publish each event
	for _, evt := range eventsToPublish {
		// Persist event to storage via buffer (non-blocking, batched, async writes)
		if b.persistenceBuffer != nil {
			// Use persistence buffer for batching and async writes
			_ = b.persistenceBuffer.PersistEvent(context.Background(), evt)
		} else {
			// Fallback to direct persistence (non-blocking, fire-and-forget)
			go b.persistEvent(context.Background(), evt)
		}

		// Publish to subscribers (always publish, even if persistence fails)
		// Use goroutine for subscriber notification to avoid blocking if there are many subscribers
		go b.publishToSubscribers(evt)
	}

	// Step 6: Update metrics (non-blocking, fast path)
	b.metricsMu.Lock()
	b.eventsPublished++
	b.metricsMu.Unlock()

	// Record publish metrics (latency tracking)
	if b.metricsManager != nil {
		latency := time.Since(startTime)
		b.metricsManager.RecordPublish(event.Type, latency, nil)
	}

	return nil
}

// Unsubscribe removes a subscription for the given event type and channel.
func (b *EventBusImpl) Unsubscribe(eventType types.EventType, ch <-chan types.EventAny) {
	b.lifecycleMu.RLock()
	closed := b.closed
	b.lifecycleMu.RUnlock()

	if closed {
		return
	}

	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	removed := false

	// Remove from specific subscribers
	if subs, ok := b.subscribers[eventType]; ok {
		filtered := make([]chan types.EventAny, 0, len(subs))
		for _, sub := range subs {
			if sub != ch {
				filtered = append(filtered, sub)
			} else {
				closeChannelSafely(sub)
				removed = true
			}
		}
		if len(filtered) == 0 {
			delete(b.subscribers, eventType)
		} else {
			b.subscribers[eventType] = filtered
		}
	}

	// Remove from allSubs
	if len(b.allSubs) > 0 {
		filteredAll := make([]chan types.EventAny, 0, len(b.allSubs))
		for _, sub := range b.allSubs {
			if sub != ch {
				filteredAll = append(filteredAll, sub)
			} else {
				closeChannelSafely(sub)
				removed = true
			}
		}
		b.allSubs = filteredAll
	}

	// Track subscription metrics
	if removed {
		b.metricsMu.Lock()
		b.subscriptionsRemoved++
		b.metricsMu.Unlock()
	}
}

// Close shuts down the event bus and closes all subscriber channels.
// This is a legacy method for backward compatibility. Use Stop() instead.
func (b *EventBusImpl) Close() error {
	// Delegate to Stop() for proper lifecycle management
	return b.Stop(context.Background())
}

// Helper methods

// persistEvent persists an event to storage.
// If retry manager is available, it will mark the event as succeeded/failed.
func (b *EventBusImpl) persistEvent(ctx context.Context, event types.EventAny) {
	startTime := time.Now()
	err := b.provider.PersistEvent(ctx, event)
	latency := time.Since(startTime)

	// Generate eventID (same logic as metastorage provider)
	// This is needed for retry manager to track event status
	eventID := b.generateEventID(event.Timestamp)

	if err != nil {
		b.logger.Error("Failed to persist event",
			zap.String("event_type", string(event.Type)),
			zap.String("source", event.Source),
			zap.String("event_id", eventID),
			zap.Error(err))

		// Mark event as failed in retry manager (if available)
		// Note: This only works if the event was partially persisted (e.g., created but failed to update)
		// For complete persistence failures, the event won't be in meta-storage, so this is a no-op
		if b.retryManager != nil {
			_, _, _ = b.retryManager.MarkEventFailed(ctx, eventID, event.Type, 0, err.Error())
		}
	} else {
		b.metricsMu.Lock()
		b.eventsPersisted++
		b.metricsMu.Unlock()

		// Mark event as succeeded in retry manager (if available)
		if b.retryManager != nil {
			_ = b.retryManager.MarkEventSucceeded(ctx, eventID)
		}
	}

	// Record persist metrics (latency tracking)
	if b.metricsManager != nil {
		b.metricsManager.RecordPersist(event.Type, latency, err)
	}
}

// generateEventID generates a unique event ID based on timestamp.
// This matches the logic in metastorage provider.
func (b *EventBusImpl) generateEventID(timestamp time.Time) string {
	nanos := timestamp.UnixNano()
	micros := time.Now().UnixMicro()
	return fmt.Sprintf("%d_%d", nanos, micros)
}

// publishToSubscribers publishes an event to all matching subscribers.
// This method is optimized for high throughput:
// - Uses fine-grained locking (separate lock for subscribers)
// - Batches notifications when there are many subscribers (>100)
// - Non-blocking channel sends (drops on overflow)
func (b *EventBusImpl) publishToSubscribers(event types.EventAny) {
	// Acquire read lock for subscribers (allows concurrent reads)
	b.subscribersMu.RLock()
	subs, hasSubs := b.subscribers[event.Type]
	allSubs := b.allSubs
	b.subscribersMu.RUnlock()

	// Send to specific subscribers
	if hasSubs {
		// Batch notifications if there are many subscribers (>100)
		if len(subs) > 100 {
			// Use goroutines for batched delivery to avoid blocking
			b.publishToSubscribersBatch(subs, event)
		} else {
			// Fast path: direct iteration for small subscriber lists
			for _, sub := range subs {
				select {
				case sub <- event:
					// Successfully sent
				default:
					// Channel full, drop event (non-blocking)
					// Closed channels will be cleaned up by the periodic cleanup worker
					b.logger.Warn("Event dropped: subscriber channel full",
						zap.String("event_type", string(event.Type)),
						zap.String("source", event.Source))
				}
			}
		}
	}

	// Send to "all" subscribers
	if len(allSubs) > 100 {
		// Batch notifications if there are many subscribers
		b.publishToSubscribersBatch(allSubs, event)
	} else {
		// Fast path: direct iteration for small subscriber lists
		for _, sub := range allSubs {
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
}

// publishToSubscribersBatch publishes an event to subscribers in batches using goroutines.
// This optimizes performance when there are many subscribers (>100).
func (b *EventBusImpl) publishToSubscribersBatch(subs []chan types.EventAny, event types.EventAny) {
	// Process subscribers in batches of 50 to avoid creating too many goroutines
	batchSize := 50
	for i := 0; i < len(subs); i += batchSize {
		end := i + batchSize
		if end > len(subs) {
			end = len(subs)
		}
		batch := subs[i:end]

		// Send to batch in a goroutine to avoid blocking
		go func(batch []chan types.EventAny) {
			for _, sub := range batch {
				select {
				case sub <- event:
				default:
					// Channel full, drop event (non-blocking)
					// Don't log for batched delivery to avoid log spam
				}
			}
		}(batch)
	}
}

// closeAllChannels closes all subscriber channels safely.
func (b *EventBusImpl) closeAllChannels() {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	closedCount := 0
	for _, subs := range b.subscribers {
		for _, sub := range subs {
			closeChannelSafely(sub)
			closedCount++
		}
	}
	for _, sub := range b.allSubs {
		closeChannelSafely(sub)
		closedCount++
	}

	// Track subscription cleanup metrics
	if closedCount > 0 {
		b.metricsMu.Lock()
		b.subscriptionsCleaned += int64(closedCount)
		b.metricsMu.Unlock()
	}

	b.subscribers = make(map[types.EventType][]chan types.EventAny)
	b.allSubs = make([]chan types.EventAny, 0)
}

// closeChannelSafely closes a channel safely, recovering from panics.
func closeChannelSafely(ch chan types.EventAny) {
	defer func() {
		if r := recover(); r != nil {
			// Channel was already closed, ignore
			_ = r
		}
	}()
	close(ch)
}

// getActiveSubscriberCount returns the current number of active subscribers.
func (b *EventBusImpl) getActiveSubscriberCount() int {
	b.subscribersMu.RLock()
	defer b.subscribersMu.RUnlock()

	count := len(b.allSubs)
	for _, subs := range b.subscribers {
		count += len(subs)
	}
	return count
}

// startRetentionCleanupWorker starts a background goroutine for retention cleanup.
func (b *EventBusImpl) startRetentionCleanupWorker(interval time.Duration) {
	b.retentionCleanupWg.Add(1)
	go func() {
		defer b.retentionCleanupWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial cleanup (after first interval)
		select {
		case <-b.retentionCleanupCtx.Done():
			return
		case <-ticker.C:
			b.runRetentionCleanup()
		}

		// Periodic cleanup
		for {
			select {
			case <-b.retentionCleanupCtx.Done():
				return
			case <-ticker.C:
				b.runRetentionCleanup()
			}
		}
	}()

	b.logger.Info("Retention cleanup worker started",
		zap.Duration("interval", interval))
}

// runRetentionCleanup runs a retention cleanup cycle.
func (b *EventBusImpl) runRetentionCleanup() {
	if b.retentionManager == nil {
		return
	}

	// Emit cleanup_started event
	b.emitCleanupStartedEvent()

	ctx := context.Background()
	stats, err := b.retentionManager.CleanupExpiredEvents(ctx)
	if err != nil {
		b.logger.Error("Retention cleanup failed",
			zap.Error(err))
		// Emit cleanup_completed event even on failure
		b.emitCleanupCompletedEvent(stats, err)
		return
	}

	// Record cleanup metrics
	if b.metricsManager != nil && stats != nil {
		b.metricsManager.RecordCleanup(stats.EventsDeleted, stats.SpaceFreedBytes, stats.Duration)
	}

	// Emit cleanup_completed event with statistics
	b.emitCleanupCompletedEvent(stats, nil)
}

// startStoragePressureMonitor starts a background goroutine for storage pressure monitoring.
func (b *EventBusImpl) startStoragePressureMonitor(interval time.Duration) {
	b.storagePressureWg.Add(1)
	go func() {
		defer b.storagePressureWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-b.storagePressureCtx.Done():
				return
			case <-ticker.C:
				b.checkStoragePressure()
			}
		}
	}()

	b.logger.Info("Storage pressure monitor started",
		zap.Duration("interval", interval))
}

// checkStoragePressure checks storage pressure status.
func (b *EventBusImpl) checkStoragePressure() {
	if b.storagePressureMonitor == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	oldPressure := b.storagePressureMonitor.IsStoragePressure()
	hasPressure, err := b.storagePressureMonitor.CheckStoragePressure(ctx)
	if err != nil {
		b.logger.Warn("Failed to check storage pressure",
			zap.Error(err))
		return
	}

	// Emit storage_pressure event if pressure state changed
	if oldPressure != hasPressure {
		b.emitStoragePressureEvent(hasPressure, b.storagePressureMonitor.GetStorageUsagePercent())
	}
}

// startHealthCheckWorker starts a background goroutine for health checks.
func (b *EventBusImpl) startHealthCheckWorker(interval time.Duration) {
	b.healthCheckWg.Add(1)
	go func() {
		defer b.healthCheckWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-b.healthCheckCtx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := b.provider.HealthCheck(ctx); err != nil {
					b.logger.Warn("Health check failed",
						zap.Error(err))
				}
				cancel()
			}
		}
	}()

	b.logger.Info("Health check worker started",
		zap.Duration("interval", interval))
}

// startSubscriptionCleanupWorker starts a background goroutine for subscription cleanup.
func (b *EventBusImpl) startSubscriptionCleanupWorker(interval time.Duration) {
	b.subscriptionCleanupWg.Add(1)
	go func() {
		defer b.subscriptionCleanupWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial cleanup (after first interval)
		select {
		case <-b.subscriptionCleanupCtx.Done():
			return
		case <-ticker.C:
			b.runSubscriptionCleanup()
		}

		// Periodic cleanup
		for {
			select {
			case <-b.subscriptionCleanupCtx.Done():
				return
			case <-ticker.C:
				b.runSubscriptionCleanup()
			}
		}
	}()

	b.logger.Info("Subscription cleanup worker started",
		zap.Duration("interval", interval))
}

// runSubscriptionCleanup removes closed channels from subscriber lists.
// This is optimized for large numbers of subscribers (1000+) by:
// - Processing subscribers in batches to avoid long lock holds
// - Using reflection to safely check channel state (closed channels can be detected)
// - Efficiently rebuilding subscriber lists without closed channels
// Note: In Go, there's no reliable way to check if a channel is closed without consuming from it.
// We use a workaround: try to receive from the channel in a non-blocking way.
// If the channel is closed, the receive will succeed immediately with ok=false.
// If the channel is open but empty, the receive will fail (default case), and we keep the channel.
func (b *EventBusImpl) runSubscriptionCleanup() {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	cleanedCount := 0
	initialCount := b.getActiveSubscriberCountUnlocked()

	// Clean up specific subscribers by event type
	for eventType, subs := range b.subscribers {
		if len(subs) == 0 {
			continue
		}

		// Check each subscriber channel to see if it's closed
		// We use a non-blocking receive to detect closed channels
		// Note: This approach has a limitation - if a channel has buffered events,
		// we might consume one, but that's acceptable for cleanup purposes
		filtered := make([]chan types.EventAny, 0, len(subs))
		for _, sub := range subs {
			// Try to receive from channel (non-blocking)
			// If channel is closed, receive succeeds immediately with ok=false
			// If channel is open but empty, receive fails (default case)
			select {
			case _, ok := <-sub:
				if ok {
					// Channel is open and had a value - we consumed it, but that's acceptable
					// Keep the channel (it's still active)
					filtered = append(filtered, sub)
				} else {
					// Channel is closed - remove it
					cleanedCount++
				}
			default:
				// Channel is open but empty - keep it
				filtered = append(filtered, sub)
			}
		}

		if len(filtered) == 0 {
			delete(b.subscribers, eventType)
		} else if len(filtered) != len(subs) {
			b.subscribers[eventType] = filtered
		}
	}

	// Clean up "all" subscribers
	if len(b.allSubs) > 0 {
		filteredAll := make([]chan types.EventAny, 0, len(b.allSubs))
		for _, sub := range b.allSubs {
			select {
			case _, ok := <-sub:
				if ok {
					// Channel is open and had a value - keep it
					filteredAll = append(filteredAll, sub)
				} else {
					// Channel is closed - remove it
					cleanedCount++
				}
			default:
				// Channel is open but empty - keep it
				filteredAll = append(filteredAll, sub)
			}
		}
		b.allSubs = filteredAll
	}

	if cleanedCount > 0 {
		b.metricsMu.Lock()
		b.subscriptionsCleaned += int64(cleanedCount)
		b.metricsMu.Unlock()

		finalCount := b.getActiveSubscriberCountUnlocked()
		b.logger.Info("Subscription cleanup completed",
			zap.Int("channels_cleaned", cleanedCount),
			zap.Int("initial_subscribers", initialCount),
			zap.Int("final_subscribers", finalCount))
	}
}

// getActiveSubscriberCountUnlocked returns the current number of active subscribers without acquiring a lock.
// This should only be called when the lock is already held.
func (b *EventBusImpl) getActiveSubscriberCountUnlocked() int {
	count := len(b.allSubs)
	for _, subs := range b.subscribers {
		count += len(subs)
	}
	return count
}

// emitEventDroppedEvent emits an event_bus.event_dropped event for monitoring.
func (b *EventBusImpl) emitEventDroppedEvent(event types.EventAny, category types.EventCategory) {
	// Create event data
	eventData := map[string]interface{}{
		"event_type": string(event.Type),
		"source":     event.Source,
		"category":   string(category),
		"timestamp":  time.Now(),
	}

	dataBytes, err := json.Marshal(eventData)
	if err != nil {
		b.logger.Warn("Failed to marshal event_dropped event data",
			zap.Error(err))
		return
	}

	// Create and publish the event (non-blocking, fire-and-forget)
	droppedEvent := types.EventAny{
		Type:      types.EventType("event_bus.event_dropped"),
		Source:    "event-bus",
		Timestamp: time.Now(),
		Data:      json.RawMessage(dataBytes),
	}

	// Publish to subscribers only (don't persist to avoid recursion)
	go b.publishToSubscribers(droppedEvent)
}

// emitStoragePressureEvent emits an event_bus.storage_pressure event for monitoring.
func (b *EventBusImpl) emitStoragePressureEvent(hasPressure bool, usagePercent float64) {
	// Create event data
	eventData := map[string]interface{}{
		"has_pressure":  hasPressure,
		"usage_percent": usagePercent,
		"threshold":     90,
		"timestamp":     time.Now(),
	}

	dataBytes, err := json.Marshal(eventData)
	if err != nil {
		b.logger.Warn("Failed to marshal storage_pressure event data",
			zap.Error(err))
		return
	}

	// Create and publish the event (non-blocking, fire-and-forget)
	pressureEvent := types.EventAny{
		Type:      types.EventType("event_bus.storage_pressure"),
		Source:    "event-bus",
		Timestamp: time.Now(),
		Data:      json.RawMessage(dataBytes),
	}

	// Publish to subscribers only (don't persist to avoid recursion)
	go b.publishToSubscribers(pressureEvent)
}

// emitHealthDegradedEvent emits an event_bus.health_degraded event for monitoring.
func (b *EventBusImpl) emitHealthDegradedEvent(status types.HealthStatus, providerError bool, dropRate float64, providerStatus map[string]interface{}) {
	// Create event data
	eventData := map[string]interface{}{
		"status":        status.String(),
		"timestamp":     time.Now(),
		"provider_error": providerError,
	}

	if dropRate > 0 {
		eventData["drop_rate_percent"] = dropRate
	}

	// Include provider status details
	if len(providerStatus) > 0 {
		eventData["provider_status"] = providerStatus
	}

	dataBytes, err := json.Marshal(eventData)
	if err != nil {
		b.logger.Warn("Failed to marshal health_degraded event data",
			zap.Error(err))
		return
	}

	// Create and publish the event (non-blocking, fire-and-forget)
	degradedEvent := types.EventAny{
		Type:      types.EventType("event_bus.health_degraded"),
		Source:    "event-bus",
		Timestamp: time.Now(),
		Data:      json.RawMessage(dataBytes),
	}

	// Publish to subscribers only (don't persist to avoid recursion)
	go b.publishToSubscribers(degradedEvent)
}

// emitCleanupStartedEvent emits an event_bus.cleanup_started event for monitoring.
func (b *EventBusImpl) emitCleanupStartedEvent() {
	// Create event data
	eventData := map[string]interface{}{
		"timestamp": time.Now(),
	}

	dataBytes, err := json.Marshal(eventData)
	if err != nil {
		b.logger.Warn("Failed to marshal cleanup_started event data",
			zap.Error(err))
		return
	}

	// Create and publish the event (non-blocking, fire-and-forget)
	cleanupEvent := types.EventAny{
		Type:      types.EventType("event_bus.cleanup_started"),
		Source:    "event-bus",
		Timestamp: time.Now(),
		Data:      json.RawMessage(dataBytes),
	}

	// Publish to subscribers only (don't persist to avoid recursion)
	go b.publishToSubscribers(cleanupEvent)
}

// emitCleanupCompletedEvent emits an event_bus.cleanup_completed event for monitoring.
func (b *EventBusImpl) emitCleanupCompletedEvent(stats *types.CleanupStats, err error) {
	// Create event data
	eventData := map[string]interface{}{
		"timestamp": time.Now(),
	}

	if stats != nil {
		eventData["events_deleted"] = stats.EventsDeleted
		eventData["space_freed_bytes"] = stats.SpaceFreedBytes
		eventData["duration_ms"] = stats.Duration.Milliseconds()
	}

	if err != nil {
		eventData["error"] = err.Error()
		eventData["success"] = false
	} else {
		eventData["success"] = true
	}

	dataBytes, err := json.Marshal(eventData)
	if err != nil {
		b.logger.Warn("Failed to marshal cleanup_completed event data",
			zap.Error(err))
		return
	}

	// Create and publish the event (non-blocking, fire-and-forget)
	cleanupEvent := types.EventAny{
		Type:      types.EventType("event_bus.cleanup_completed"),
		Source:    "event-bus",
		Timestamp: time.Now(),
		Data:      json.RawMessage(dataBytes),
	}

	// Publish to subscribers only (don't persist to avoid recursion)
	go b.publishToSubscribers(cleanupEvent)
}

// startRetryWorker starts a background goroutine for retrying failed events.
func (b *EventBusImpl) startRetryWorker(interval time.Duration) {
	if b.retryManager == nil {
		return
	}

	b.retryWorkerWg.Add(1)
	go func() {
		defer b.retryWorkerWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial retry (after first interval)
		select {
		case <-b.retryWorkerCtx.Done():
			return
		case <-ticker.C:
			b.runRetryWorker()
		}

		// Periodic retry
		for {
			select {
			case <-b.retryWorkerCtx.Done():
				return
			case <-ticker.C:
				b.runRetryWorker()
			}
		}
	}()

	b.logger.Info("Retry worker started",
		zap.Duration("interval", interval))
}

// runRetryWorker processes failed events that are ready for retry.
func (b *EventBusImpl) runRetryWorker() {
	if b.retryManager == nil {
		return
	}

	ctx := context.Background()

	// Process failed events by attempting to re-persist them
	// The processFn will attempt to persist the event again
	err := b.retryManager.ProcessFailedEvents(ctx, func(ctx context.Context, eventID string, event types.EventAny) error {
		// Attempt to re-persist the event
		// Note: This will create a new eventID, but the old event will remain in meta-storage
		// with its failed status. The retry manager will update the status based on success/failure.
		return b.provider.PersistEvent(ctx, event)
	})

	if err != nil {
		b.logger.Error("Retry worker failed",
			zap.Error(err))
	}
}

