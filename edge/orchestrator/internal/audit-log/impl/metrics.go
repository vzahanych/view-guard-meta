package impl

import (
	"container/list"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	"go.uber.org/zap"
)

// MetricsTracker tracks comprehensive operational metrics for the audit log service.
// This includes entries logged per second (by type), sync metrics, latency, queue depth,
// hash chain verification results, and cleanup statistics.
type MetricsTracker struct {
	logger *zap.Logger
	mu     sync.RWMutex

	// Entry logging metrics
	entriesLoggedByType map[types.AuditEntryType]int64 // Total entries logged by type
	entriesLoggedTotal  int64                     // Total entries logged
	serviceStartTime    time.Time                 // Service start time for rate calculations

	// Sync metrics
	syncOperations      int64         // Total sync operations
	syncSuccesses       int64         // Successful sync operations
	syncFailures        int64         // Failed sync operations
	entriesSyncedTotal  int64         // Total entries synced
	syncLatencies       *list.List    // Latency measurements (for percentile calculation)
	syncLatenciesMu     sync.RWMutex  // Mutex for latency list
	maxLatencySamples   int           // Maximum number of latency samples to keep (for P50/P95/P99)
	totalSyncDuration   time.Duration // Total sync duration (for average calculation)

	// Queue metrics
	queueDepthSamples      *list.List   // Queue depth over time
	queueDepthSamplesMu    sync.RWMutex // Mutex for queue depth samples
	maxQueueDepthSamples   int          // Maximum number of queue depth samples to keep
	queueDepthSampleWindow time.Duration // Time window for queue depth samples (e.g., last hour)

	// Hash chain verification metrics
	hashChainVerifications int64     // Total hash chain verifications
	hashChainFailures      int64     // Hash chain verification failures
	lastVerificationTime   time.Time // Last verification time
	lastVerificationResult bool      // Last verification result (integrity intact)

	// Cleanup metrics
	cleanupOperations      int64         // Total cleanup operations
	entriesDeletedTotal    int64         // Total entries deleted
	cleanupLatencies       *list.List    // Cleanup latency measurements
	cleanupLatenciesMu     sync.RWMutex  // Mutex for cleanup latency list
	maxCleanupLatencySamples int         // Maximum number of cleanup latency samples
	totalCleanupDuration   time.Duration // Total cleanup duration
}

// LatencySample represents a single latency measurement.
type LatencySample struct {
	Duration time.Duration
	Time     time.Time
}

// QueueDepthSample represents a queue depth measurement at a specific time.
type QueueDepthSample struct {
	Depth int
	Time  time.Time
}

// NewMetricsTracker creates a new metrics tracker.
func NewMetricsTracker(logger *zap.Logger) *MetricsTracker {
	return &MetricsTracker{
		logger:                  logger,
		entriesLoggedByType:     make(map[types.AuditEntryType]int64),
		serviceStartTime:        time.Now(),
		syncLatencies:           list.New(),
		maxLatencySamples:       1000, // Keep last 1000 latency samples
		queueDepthSamples:       list.New(),
		maxQueueDepthSamples:    3600, // Keep last hour (3600 seconds if sampling every second)
		queueDepthSampleWindow:  1 * time.Hour,
		cleanupLatencies:        list.New(),
		maxCleanupLatencySamples: 100, // Keep last 100 cleanup latency samples
	}
}

// RecordEntryLogged records that an entry of the given type was logged.
func (m *MetricsTracker) RecordEntryLogged(entryType types.AuditEntryType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entriesLoggedTotal++
	m.entriesLoggedByType[entryType]++
}

// RecordSyncSuccess records a successful sync operation.
func (m *MetricsTracker) RecordSyncSuccess(entriesSynced int64, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.syncOperations++
	m.syncSuccesses++
	m.entriesSyncedTotal += entriesSynced
	m.totalSyncDuration += duration

	// Record latency sample
	m.syncLatenciesMu.Lock()
	latencySample := &LatencySample{
		Duration: duration,
		Time:     time.Now(),
	}
	m.syncLatencies.PushBack(latencySample)
	
	// Trim old samples if exceeding max
	for m.syncLatencies.Len() > m.maxLatencySamples {
		m.syncLatencies.Remove(m.syncLatencies.Front())
	}
	m.syncLatenciesMu.Unlock()
}

// RecordSyncFailure records a failed sync operation.
func (m *MetricsTracker) RecordSyncFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.syncOperations++
	m.syncFailures++
}

// RecordQueueDepth records the current queue depth for time-series tracking.
func (m *MetricsTracker) RecordQueueDepth(depth int) {
	now := time.Now()

	m.queueDepthSamplesMu.Lock()
	defer m.queueDepthSamplesMu.Unlock()

	sample := &QueueDepthSample{
		Depth: depth,
		Time:  now,
	}
	m.queueDepthSamples.PushBack(sample)

	// Remove samples outside the time window
	for m.queueDepthSamples.Len() > 0 {
		front := m.queueDepthSamples.Front()
		sample := front.Value.(*QueueDepthSample)
		if now.Sub(sample.Time) > m.queueDepthSampleWindow {
			m.queueDepthSamples.Remove(front)
		} else {
			break
		}
	}

	// Trim if still exceeding max samples
	for m.queueDepthSamples.Len() > m.maxQueueDepthSamples {
		m.queueDepthSamples.Remove(m.queueDepthSamples.Front())
	}
}

// RecordHashChainVerification records a hash chain verification result.
func (m *MetricsTracker) RecordHashChainVerification(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hashChainVerifications++
	if !success {
		m.hashChainFailures++
	}
	m.lastVerificationTime = time.Now()
	m.lastVerificationResult = success
}

// RecordCleanup records a cleanup operation.
func (m *MetricsTracker) RecordCleanup(entriesDeleted int64, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupOperations++
	m.entriesDeletedTotal += entriesDeleted
	m.totalCleanupDuration += duration

	// Record cleanup latency sample
	m.cleanupLatenciesMu.Lock()
	latencySample := &LatencySample{
		Duration: duration,
		Time:     time.Now(),
	}
	m.cleanupLatencies.PushBack(latencySample)

	// Trim old samples if exceeding max
	for m.cleanupLatencies.Len() > m.maxCleanupLatencySamples {
		m.cleanupLatencies.Remove(m.cleanupLatencies.Front())
	}
	m.cleanupLatenciesMu.Unlock()
}

// GetEntriesLoggedPerSecond returns the average entries logged per second since service start.
func (m *MetricsTracker) GetEntriesLoggedPerSecond() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	elapsed := time.Since(m.serviceStartTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(m.entriesLoggedTotal) / elapsed
}

// GetEntriesLoggedPerSecondByType returns the average entries logged per second for a specific entry type.
func (m *MetricsTracker) GetEntriesLoggedPerSecondByType(entryType types.AuditEntryType) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	elapsed := time.Since(m.serviceStartTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	count := m.entriesLoggedByType[entryType]
	return float64(count) / elapsed
}

// GetEntriesSyncedPerSecond returns the average entries synced per second since service start.
func (m *MetricsTracker) GetEntriesSyncedPerSecond() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	elapsed := time.Since(m.serviceStartTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(m.entriesSyncedTotal) / elapsed
}

// GetSyncSuccessRate returns the sync success rate (0.0 to 1.0).
func (m *MetricsTracker) GetSyncSuccessRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.syncOperations == 0 {
		return 1.0 // No operations yet, assume 100% success
	}
	return float64(m.syncSuccesses) / float64(m.syncOperations)
}

// GetSyncLatencyPercentiles returns the P50, P95, and P99 latency percentiles.
// Returns (P50, P95, P99) in milliseconds.
func (m *MetricsTracker) GetSyncLatencyPercentiles() (p50 float64, p95 float64, p99 float64) {
	m.syncLatenciesMu.RLock()
	defer m.syncLatenciesMu.RUnlock()

	if m.syncLatencies.Len() == 0 {
		return 0, 0, 0
	}

	// Collect all latencies
	latencies := make([]time.Duration, 0, m.syncLatencies.Len())
	for e := m.syncLatencies.Front(); e != nil; e = e.Next() {
		sample := e.Value.(*LatencySample)
		latencies = append(latencies, sample.Duration)
	}

	return calculatePercentiles(latencies)
}

// GetAverageSyncLatency returns the average sync latency in milliseconds.
func (m *MetricsTracker) GetAverageSyncLatency() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.syncOperations == 0 {
		return 0
	}
	avgDuration := m.totalSyncDuration / time.Duration(m.syncOperations)
	return float64(avgDuration.Nanoseconds()) / 1e6 // Convert to milliseconds
}

// GetQueueDepthAverage returns the average queue depth over the tracked time window.
func (m *MetricsTracker) GetQueueDepthAverage() float64 {
	m.queueDepthSamplesMu.RLock()
	defer m.queueDepthSamplesMu.RUnlock()

	if m.queueDepthSamples.Len() == 0 {
		return 0
	}

	var sum int
	count := 0
	for e := m.queueDepthSamples.Front(); e != nil; e = e.Next() {
		sample := e.Value.(*QueueDepthSample)
		sum += sample.Depth
		count++
	}

	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

// GetQueueDepthMax returns the maximum queue depth over the tracked time window.
func (m *MetricsTracker) GetQueueDepthMax() int {
	m.queueDepthSamplesMu.RLock()
	defer m.queueDepthSamplesMu.RUnlock()

	max := 0
	for e := m.queueDepthSamples.Front(); e != nil; e = e.Next() {
		sample := e.Value.(*QueueDepthSample)
		if sample.Depth > max {
			max = sample.Depth
		}
	}
	return max
}

// GetHashChainVerificationResults returns hash chain verification statistics.
func (m *MetricsTracker) GetHashChainVerificationResults() (total int64, failures int64, lastResult bool, lastTime time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.hashChainVerifications, m.hashChainFailures, m.lastVerificationResult, m.lastVerificationTime
}

// GetCleanupStatistics returns cleanup operation statistics.
func (m *MetricsTracker) GetCleanupStatistics() (operations int64, entriesDeleted int64, avgLatency float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgLatencyMs := float64(0)
	if m.cleanupOperations > 0 {
		avgDuration := m.totalCleanupDuration / time.Duration(m.cleanupOperations)
		avgLatencyMs = float64(avgDuration.Nanoseconds()) / 1e6 // Convert to milliseconds
	}

	return m.cleanupOperations, m.entriesDeletedTotal, avgLatencyMs
}

// GetSnapshot returns a comprehensive metrics snapshot.
func (m *MetricsTracker) GetSnapshot() *MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p50, p95, p99 := m.GetSyncLatencyPercentiles()

	entriesByType := make(map[string]int64)
	for entryType, count := range m.entriesLoggedByType {
		entriesByType[string(entryType)] = count
	}

	return &MetricsSnapshot{
		ServiceUptime:              time.Since(m.serviceStartTime),
		EntriesLoggedTotal:         m.entriesLoggedTotal,
		EntriesLoggedPerSecond:     m.GetEntriesLoggedPerSecond(),
		EntriesLoggedByType:        entriesByType,
		EntriesSyncedTotal:         m.entriesSyncedTotal,
		EntriesSyncedPerSecond:     m.GetEntriesSyncedPerSecond(),
		SyncOperations:             m.syncOperations,
		SyncSuccesses:              m.syncSuccesses,
		SyncFailures:               m.syncFailures,
		SyncSuccessRate:            m.GetSyncSuccessRate(),
		SyncLatencyP50:             p50,
		SyncLatencyP95:             p95,
		SyncLatencyP99:             p99,
		SyncLatencyAverage:         m.GetAverageSyncLatency(),
		QueueDepthAverage:          m.GetQueueDepthAverage(),
		QueueDepthMax:              m.GetQueueDepthMax(),
		HashChainVerifications:     m.hashChainVerifications,
		HashChainFailures:          m.hashChainFailures,
		LastVerificationResult:     m.lastVerificationResult,
		LastVerificationTime:       m.lastVerificationTime,
		CleanupOperations:          m.cleanupOperations,
		EntriesDeletedTotal:        m.entriesDeletedTotal,
		CleanupLatencyAverage:      func() float64 { _, _, avg := m.GetCleanupStatistics(); return avg }(),
	}
}

// MetricsSnapshot represents a snapshot of all metrics at a point in time.
type MetricsSnapshot struct {
	ServiceUptime              time.Duration
	EntriesLoggedTotal         int64
	EntriesLoggedPerSecond     float64
	EntriesLoggedByType        map[string]int64
	EntriesSyncedTotal         int64
	EntriesSyncedPerSecond     float64
	SyncOperations             int64
	SyncSuccesses              int64
	SyncFailures               int64
	SyncSuccessRate            float64
	SyncLatencyP50             float64 // milliseconds
	SyncLatencyP95             float64 // milliseconds
	SyncLatencyP99             float64 // milliseconds
	SyncLatencyAverage         float64 // milliseconds
	QueueDepthAverage          float64
	QueueDepthMax              int
	HashChainVerifications     int64
	HashChainFailures          int64
	LastVerificationResult     bool
	LastVerificationTime       time.Time
	CleanupOperations          int64
	EntriesDeletedTotal        int64
	CleanupLatencyAverage      float64 // milliseconds
}

// calculatePercentiles calculates P50, P95, and P99 percentiles from a slice of durations.
// Returns values in milliseconds.
func calculatePercentiles(latencies []time.Duration) (p50 float64, p95 float64, p99 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	// Sort latencies (simple insertion sort for small arrays, or use sort package for larger)
	// For simplicity, using a simple sort
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	
	// Simple bubble sort (acceptable for small sample sizes)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	getPercentile := func(percentile float64) float64 {
		if len(sorted) == 0 {
			return 0
		}
		index := int(float64(len(sorted)-1) * percentile)
		return float64(sorted[index].Nanoseconds()) / 1e6 // Convert to milliseconds
	}

	return getPercentile(0.50), getPercentile(0.95), getPercentile(0.99)
}

