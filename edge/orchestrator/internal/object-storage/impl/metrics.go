package impl

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// OperationType represents the type of storage operation.
type OperationType string

const (
	// OperationTypeStore represents a store operation
	OperationTypeStore OperationType = "store"
	// OperationTypeLoad represents a load operation
	OperationTypeLoad OperationType = "load"
	// OperationTypeDelete represents a delete operation
	OperationTypeDelete OperationType = "delete"
)

// OperationMetrics tracks metrics for a single operation type.
type OperationMetrics struct {
	// Count is the total number of operations
	Count int64

	// ErrorCount is the number of failed operations
	ErrorCount int64

	// TotalLatency is the cumulative latency in nanoseconds
	TotalLatency int64

	// LatencyHistory is a sliding window of recent latencies for percentile calculation
	// We keep the last 1000 latencies for P50, P95, P99 calculation
	LatencyHistory []time.Duration

	// MaxHistorySize is the maximum size of latency history
	MaxHistorySize int
}

// MetricsManager tracks operational metrics for the object storage service.
// It tracks operation counts, latencies, error rates, and quota utilization over time.
type MetricsManager struct {
	logger *zap.Logger

	// Operation metrics by operation type and data type
	mu              sync.RWMutex
	operationMetrics map[OperationType]map[string]*OperationMetrics // operation type -> data type -> metrics

	// Quota utilization history (last N samples)
	quotaHistory []QuotaSample

	// Retention cleanup statistics history
	cleanupHistory []CleanupSample

	// Max history sizes
	maxQuotaHistorySize   int
	maxCleanupHistorySize int
}

// QuotaSample represents a quota utilization sample at a point in time.
type QuotaSample struct {
	Timestamp time.Time
	Used      int64
	Limit     int64
	UsagePercent float64
}

// CleanupSample represents a cleanup operation sample.
type CleanupSample struct {
	Timestamp        time.Time
	ObjectsDeleted   int64
	SpaceFreedBytes  int64
	DataTypesProcessed int
	Duration         time.Duration
}

// NewMetricsManager creates a new metrics manager.
func NewMetricsManager(logger *zap.Logger) *MetricsManager {
	return &MetricsManager{
		logger:                logger,
		operationMetrics:      make(map[OperationType]map[string]*OperationMetrics),
		maxQuotaHistorySize:   100, // Keep last 100 quota samples
		maxCleanupHistorySize: 50,  // Keep last 50 cleanup samples
	}
}

// RecordOperation records a storage operation with its latency and result.
func (m *MetricsManager) RecordOperation(opType OperationType, dataType string, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize operation type map if needed
	if m.operationMetrics[opType] == nil {
		m.operationMetrics[opType] = make(map[string]*OperationMetrics)
	}

	// Initialize data type metrics if needed
	if m.operationMetrics[opType][dataType] == nil {
		m.operationMetrics[opType][dataType] = &OperationMetrics{
			MaxHistorySize: 1000, // Keep last 1000 latencies for percentile calculation
			LatencyHistory: make([]time.Duration, 0, 1000),
		}
	}

	metrics := m.operationMetrics[opType][dataType]
	metrics.Count++

	if err != nil {
		metrics.ErrorCount++
	}

	// Update latency metrics
	metrics.TotalLatency += latency.Nanoseconds()

	// Add to latency history (sliding window)
	metrics.LatencyHistory = append(metrics.LatencyHistory, latency)
	if len(metrics.LatencyHistory) > metrics.MaxHistorySize {
		// Remove oldest entry
		metrics.LatencyHistory = metrics.LatencyHistory[1:]
	}
}

// RecordQuotaSample records a quota utilization sample.
func (m *MetricsManager) RecordQuotaSample(quota *types.StorageQuota) {
	if quota == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sample := QuotaSample{
		Timestamp:    time.Now(),
		Used:         quota.Used,
		Limit:        quota.Limit,
		UsagePercent: 0,
	}

	if quota.Limit > 0 {
		sample.UsagePercent = float64(quota.Used) / float64(quota.Limit) * 100
	}

	m.quotaHistory = append(m.quotaHistory, sample)
	if len(m.quotaHistory) > m.maxQuotaHistorySize {
		// Remove oldest entry
		m.quotaHistory = m.quotaHistory[1:]
	}
}

// RecordCleanupSample records a cleanup operation sample.
func (m *MetricsManager) RecordCleanupSample(stats *CleanupStats) {
	if stats == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sample := CleanupSample{
		Timestamp:         time.Now(),
		ObjectsDeleted:    stats.ObjectsDeleted,
		SpaceFreedBytes:   stats.SpaceFreedBytes,
		DataTypesProcessed: stats.DataTypesProcessed,
		Duration:         stats.Duration,
	}

	m.cleanupHistory = append(m.cleanupHistory, sample)
	if len(m.cleanupHistory) > m.maxCleanupHistorySize {
		// Remove oldest entry
		m.cleanupHistory = m.cleanupHistory[1:]
	}
}

// GetOperationMetrics returns metrics for a specific operation type and data type.
func (m *MetricsManager) GetOperationMetrics(opType OperationType, dataType string) *OperationMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.operationMetrics[opType] == nil {
		return nil
	}

	return m.operationMetrics[opType][dataType]
}

// GetLatencyPercentiles calculates P50, P95, and P99 latencies for an operation type and data type.
func (m *MetricsManager) GetLatencyPercentiles(opType OperationType, dataType string) (p50, p95, p99 time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.operationMetrics[opType] == nil || m.operationMetrics[opType][dataType] == nil {
		return 0, 0, 0
	}

	metrics := m.operationMetrics[opType][dataType]
	if len(metrics.LatencyHistory) == 0 {
		return 0, 0, 0
	}

	// Create a copy and sort
	latencies := make([]time.Duration, len(metrics.LatencyHistory))
	copy(latencies, metrics.LatencyHistory)

	// Simple sort (bubble sort for small arrays, or use sort package)
	// For now, we'll use a simple approach - sort in place
	for i := 0; i < len(latencies)-1; i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	// Calculate percentiles
	n := len(latencies)
	if n > 0 {
		p50 = latencies[n*50/100]
		if n > 1 {
			p95 = latencies[n*95/100]
			p99 = latencies[n*99/100]
		}
	}

	return p50, p95, p99
}

// GetErrorRate calculates the error rate for an operation type and data type.
func (m *MetricsManager) GetErrorRate(opType OperationType, dataType string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.operationMetrics[opType] == nil || m.operationMetrics[opType][dataType] == nil {
		return 0
	}

	metrics := m.operationMetrics[opType][dataType]
	if metrics.Count == 0 {
		return 0
	}

	return float64(metrics.ErrorCount) / float64(metrics.Count) * 100
}

// GetQuotaHistory returns the quota utilization history.
func (m *MetricsManager) GetQuotaHistory() []QuotaSample {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	history := make([]QuotaSample, len(m.quotaHistory))
	copy(history, m.quotaHistory)
	return history
}

// GetCleanupHistory returns the cleanup statistics history.
func (m *MetricsManager) GetCleanupHistory() []CleanupSample {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	history := make([]CleanupSample, len(m.cleanupHistory))
	copy(history, m.cleanupHistory)
	return history
}

// GetMetricsSummary returns a summary of all metrics.
func (m *MetricsManager) GetMetricsSummary() MetricsSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := MetricsSummary{
		OperationCounts: make(map[string]map[string]int64), // operation type -> data type -> count
		ErrorRates:      make(map[string]map[string]float64), // operation type -> data type -> error rate
		LatencyP50:      make(map[string]map[string]time.Duration),
		LatencyP95:      make(map[string]map[string]time.Duration),
		LatencyP99:      make(map[string]map[string]time.Duration),
		QuotaHistory:    make([]QuotaSample, len(m.quotaHistory)),
		CleanupHistory:  make([]CleanupSample, len(m.cleanupHistory)),
	}

	// Copy quota and cleanup history
	copy(summary.QuotaHistory, m.quotaHistory)
	copy(summary.CleanupHistory, m.cleanupHistory)

	// Aggregate operation metrics
	for opType, dataTypeMetrics := range m.operationMetrics {
		opTypeStr := string(opType)
		summary.OperationCounts[opTypeStr] = make(map[string]int64)
		summary.ErrorRates[opTypeStr] = make(map[string]float64)
		summary.LatencyP50[opTypeStr] = make(map[string]time.Duration)
		summary.LatencyP95[opTypeStr] = make(map[string]time.Duration)
		summary.LatencyP99[opTypeStr] = make(map[string]time.Duration)

		for dataType, metrics := range dataTypeMetrics {
			summary.OperationCounts[opTypeStr][dataType] = metrics.Count
			if metrics.Count > 0 {
				summary.ErrorRates[opTypeStr][dataType] = float64(metrics.ErrorCount) / float64(metrics.Count) * 100
			}

			// Calculate percentiles
			p50, p95, p99 := m.calculatePercentiles(metrics.LatencyHistory)
			summary.LatencyP50[opTypeStr][dataType] = p50
			summary.LatencyP95[opTypeStr][dataType] = p95
			summary.LatencyP99[opTypeStr][dataType] = p99
		}
	}

	return summary
}

// calculatePercentiles calculates P50, P95, and P99 from a latency history.
func (m *MetricsManager) calculatePercentiles(latencies []time.Duration) (p50, p95, p99 time.Duration) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	// Create a copy and sort
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	// Simple bubble sort (for small arrays)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	n := len(sorted)
	p50 = sorted[n*50/100]
	if n > 1 {
		p95 = sorted[n*95/100]
		p99 = sorted[n*99/100]
	}

	return p50, p95, p99
}

// MetricsSummary contains a summary of all operational metrics.
type MetricsSummary struct {
	// OperationCounts is a map of operation type -> data type -> count
	OperationCounts map[string]map[string]int64

	// ErrorRates is a map of operation type -> data type -> error rate (percentage)
	ErrorRates map[string]map[string]float64

	// LatencyP50 is a map of operation type -> data type -> P50 latency
	LatencyP50 map[string]map[string]time.Duration

	// LatencyP95 is a map of operation type -> data type -> P95 latency
	LatencyP95 map[string]map[string]time.Duration

	// LatencyP99 is a map of operation type -> data type -> P99 latency
	LatencyP99 map[string]map[string]time.Duration

	// QuotaHistory is the quota utilization history
	QuotaHistory []QuotaSample

	// CleanupHistory is the cleanup statistics history
	CleanupHistory []CleanupSample
}

// StartPeriodicQuotaSampling starts a background goroutine that periodically samples quota status.
func (m *MetricsManager) StartPeriodicQuotaSampling(ctx context.Context, quotaManager *QuotaManager, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute // Default: every 5 minutes
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if quotaManager != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					quota, err := quotaManager.GetQuotaStatus(ctx)
					cancel()
					if err == nil && quota != nil {
						m.RecordQuotaSample(quota)
					}
				}
			}
		}
	}()
}

