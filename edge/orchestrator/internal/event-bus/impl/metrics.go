package impl

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// MetricsManager tracks operational metrics for the event bus service.
// It tracks operation counts, latencies, error rates, and storage usage over time.
type MetricsManager struct {
	logger *zap.Logger

	// Operation metrics by event type
	mu              sync.RWMutex
	publishMetrics  map[types.EventType]*OperationMetrics // event type -> metrics
	persistMetrics  map[types.EventType]*OperationMetrics // event type -> metrics
	dropMetrics     map[types.EventCategory]*OperationMetrics // category -> metrics

	// Subscriber count over time (last N samples)
	subscriberHistory []SubscriberSample

	// Storage usage over time (last N samples)
	storageUsageHistory []StorageUsageSample

	// Cleanup statistics history
	cleanupHistory []CleanupSample

	// Subscription churn metrics
	subscriptionsCreated int64
	subscriptionsRemoved int64
	subscriptionsCleaned int64

	// Max history sizes
	maxSubscriberHistorySize int
	maxStorageHistorySize    int
	maxCleanupHistorySize    int
}

// OperationMetrics tracks metrics for a single operation type.
type OperationMetrics struct {
	// Count is the total number of operations
	Count int64

	// ErrorCount is the number of failed operations
	ErrorCount int64

	// LatencyHistory is a sliding window of recent latencies for percentile calculation
	// We keep the last 1000 latencies for P50, P95, P99 calculation
	LatencyHistory []time.Duration

	// MaxHistorySize is the maximum size of latency history
	MaxHistorySize int
}

// SubscriberSample represents a subscriber count sample at a point in time.
type SubscriberSample struct {
	Timestamp time.Time
	Count     int
}

// StorageUsageSample represents a storage usage sample at a point in time.
type StorageUsageSample struct {
	Timestamp        time.Time
	UsagePercent     float64
	StoragePressure  bool
}

// CleanupSample represents a cleanup operation sample.
type CleanupSample struct {
	Timestamp       time.Time
	EventsDeleted   int64
	SpaceFreedBytes int64
	Duration        time.Duration
}

// NewMetricsManager creates a new metrics manager.
func NewMetricsManager(logger *zap.Logger) *MetricsManager {
	return &MetricsManager{
		logger:                   logger,
		publishMetrics:           make(map[types.EventType]*OperationMetrics),
		persistMetrics:           make(map[types.EventType]*OperationMetrics),
		dropMetrics:              make(map[types.EventCategory]*OperationMetrics),
		maxSubscriberHistorySize: 100, // Keep last 100 subscriber samples
		maxStorageHistorySize:    100, // Keep last 100 storage usage samples
		maxCleanupHistorySize:    50,  // Keep last 50 cleanup samples
	}
}

// RecordPublish records a publish operation with its latency and result.
func (m *MetricsManager) RecordPublish(eventType types.EventType, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreatePublishMetrics(eventType)
	metrics.Count++
	if err != nil {
		metrics.ErrorCount++
	}

	// Add latency to history
	m.addLatencyToHistory(&metrics.LatencyHistory, latency, metrics.MaxHistorySize)
}

// RecordPersist records a persist operation with its latency and result.
func (m *MetricsManager) RecordPersist(eventType types.EventType, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreatePersistMetrics(eventType)
	metrics.Count++
	if err != nil {
		metrics.ErrorCount++
	}

	// Add latency to history
	m.addLatencyToHistory(&metrics.LatencyHistory, latency, metrics.MaxHistorySize)
}

// RecordDrop records a drop operation with its category.
func (m *MetricsManager) RecordDrop(category types.EventCategory) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.getOrCreateDropMetrics(category)
	metrics.Count++
}

// RecordSubscriberCount records the current subscriber count.
func (m *MetricsManager) RecordSubscriberCount(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sample := SubscriberSample{
		Timestamp: time.Now(),
		Count:     count,
	}

	m.subscriberHistory = append(m.subscriberHistory, sample)
	if len(m.subscriberHistory) > m.maxSubscriberHistorySize {
		m.subscriberHistory = m.subscriberHistory[1:]
	}
}

// RecordStorageUsage records the current storage usage.
func (m *MetricsManager) RecordStorageUsage(usagePercent float64, hasPressure bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sample := StorageUsageSample{
		Timestamp:       time.Now(),
		UsagePercent:    usagePercent,
		StoragePressure: hasPressure,
	}

	m.storageUsageHistory = append(m.storageUsageHistory, sample)
	if len(m.storageUsageHistory) > m.maxStorageHistorySize {
		m.storageUsageHistory = m.storageUsageHistory[1:]
	}
}

// RecordCleanup records a cleanup operation sample.
func (m *MetricsManager) RecordCleanup(eventsDeleted int64, spaceFreedBytes int64, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sample := CleanupSample{
		Timestamp:       time.Now(),
		EventsDeleted:   eventsDeleted,
		SpaceFreedBytes: spaceFreedBytes,
		Duration:        duration,
	}

	m.cleanupHistory = append(m.cleanupHistory, sample)
	if len(m.cleanupHistory) > m.maxCleanupHistorySize {
		m.cleanupHistory = m.cleanupHistory[1:]
	}
}

// GetMetricsSummary returns a comprehensive metrics summary.
func (m *MetricsManager) GetMetricsSummary() *MetricsSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := &MetricsSummary{
		PublishMetrics:      make(map[types.EventType]*OperationMetricsSummary),
		PersistMetrics:      make(map[types.EventType]*OperationMetricsSummary),
		DropMetrics:         make(map[types.EventCategory]*OperationMetricsSummary),
		SubscriberHistory:   make([]SubscriberSample, len(m.subscriberHistory)),
		StorageUsageHistory: make([]StorageUsageSample, len(m.storageUsageHistory)),
		CleanupHistory:      make([]CleanupSample, len(m.cleanupHistory)),
	}

	// Copy publish metrics
	for eventType, metrics := range m.publishMetrics {
		summary.PublishMetrics[eventType] = m.summarizeOperationMetrics(metrics)
	}

	// Copy persist metrics
	for eventType, metrics := range m.persistMetrics {
		summary.PersistMetrics[eventType] = m.summarizeOperationMetrics(metrics)
	}

	// Copy drop metrics
	for category, metrics := range m.dropMetrics {
		summary.DropMetrics[category] = &OperationMetricsSummary{
			Count:      metrics.Count,
			ErrorCount: metrics.ErrorCount,
		}
	}

	// Copy histories
	copy(summary.SubscriberHistory, m.subscriberHistory)
	copy(summary.StorageUsageHistory, m.storageUsageHistory)
	copy(summary.CleanupHistory, m.cleanupHistory)

	// Copy subscription churn metrics
	summary.SubscriptionsCreated = m.subscriptionsCreated
	summary.SubscriptionsRemoved = m.subscriptionsRemoved
	summary.SubscriptionsCleaned = m.subscriptionsCleaned

	return summary
}

// MetricsSummary contains a comprehensive summary of all metrics.
type MetricsSummary struct {
	PublishMetrics      map[types.EventType]*OperationMetricsSummary
	PersistMetrics      map[types.EventType]*OperationMetricsSummary
	DropMetrics         map[types.EventCategory]*OperationMetricsSummary
	SubscriberHistory   []SubscriberSample
	StorageUsageHistory []StorageUsageSample
	CleanupHistory      []CleanupSample
	// Subscription churn metrics
	SubscriptionsCreated int64
	SubscriptionsRemoved int64
	SubscriptionsCleaned  int64
}

// OperationMetricsSummary contains a summary of operation metrics.
type OperationMetricsSummary struct {
	Count         int64
	ErrorCount    int64
	ErrorRate     float64
	LatencyP50    time.Duration
	LatencyP95    time.Duration
	LatencyP99    time.Duration
}

// Helper methods

func (m *MetricsManager) getOrCreatePublishMetrics(eventType types.EventType) *OperationMetrics {
	if metrics, ok := m.publishMetrics[eventType]; ok {
		return metrics
	}
	metrics := &OperationMetrics{
		MaxHistorySize: 1000, // Keep last 1000 latencies
	}
	m.publishMetrics[eventType] = metrics
	return metrics
}

func (m *MetricsManager) getOrCreatePersistMetrics(eventType types.EventType) *OperationMetrics {
	if metrics, ok := m.persistMetrics[eventType]; ok {
		return metrics
	}
	metrics := &OperationMetrics{
		MaxHistorySize: 1000, // Keep last 1000 latencies
	}
	m.persistMetrics[eventType] = metrics
	return metrics
}

func (m *MetricsManager) getOrCreateDropMetrics(category types.EventCategory) *OperationMetrics {
	if metrics, ok := m.dropMetrics[category]; ok {
		return metrics
	}
	metrics := &OperationMetrics{
		MaxHistorySize: 0, // No latency tracking for drops
	}
	m.dropMetrics[category] = metrics
	return metrics
}

func (m *MetricsManager) addLatencyToHistory(history *[]time.Duration, latency time.Duration, maxSize int) {
	*history = append(*history, latency)
	if len(*history) > maxSize {
		*history = (*history)[1:]
	}
}

func (m *MetricsManager) summarizeOperationMetrics(metrics *OperationMetrics) *OperationMetricsSummary {
	summary := &OperationMetricsSummary{
		Count:      metrics.Count,
		ErrorCount: metrics.ErrorCount,
	}

	// Calculate error rate
	if metrics.Count > 0 {
		summary.ErrorRate = float64(metrics.ErrorCount) / float64(metrics.Count) * 100.0
	}

	// Calculate latency percentiles
	if len(metrics.LatencyHistory) > 0 {
		latencies := make([]time.Duration, len(metrics.LatencyHistory))
		copy(latencies, metrics.LatencyHistory)
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		summary.LatencyP50 = m.percentile(latencies, 50)
		summary.LatencyP95 = m.percentile(latencies, 95)
		summary.LatencyP99 = m.percentile(latencies, 99)
	}

	return summary
}

func (m *MetricsManager) percentile(sortedLatencies []time.Duration, p int) time.Duration {
	if len(sortedLatencies) == 0 {
		return 0
	}
	index := (p * len(sortedLatencies)) / 100
	if index >= len(sortedLatencies) {
		index = len(sortedLatencies) - 1
	}
	return sortedLatencies[index]
}

// StartPeriodicSampling starts background goroutines for periodic metric sampling.
func (m *MetricsManager) StartPeriodicSampling(ctx context.Context, subscriberCountFn func() int, storageUsageFn func() (float64, bool)) {
	// Sample subscriber count every 1 minute
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if subscriberCountFn != nil {
					m.RecordSubscriberCount(subscriberCountFn())
				}
			}
		}
	}()

	// Sample storage usage every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if storageUsageFn != nil {
					usagePercent, hasPressure := storageUsageFn()
					m.RecordStorageUsage(usagePercent, hasPressure)
				}
			}
		}
	}()
}

