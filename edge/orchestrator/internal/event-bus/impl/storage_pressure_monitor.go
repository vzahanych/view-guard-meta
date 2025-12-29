package impl

import (
	"context"
	"sync"
	"time"

	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	"go.uber.org/zap"
)

// StoragePressureMonitor monitors meta-storage quota status to detect storage pressure.
// It provides thread-safe access to storage pressure status and emits events when pressure is detected.
type StoragePressureMonitor struct {
	metaStorage metastorage.MetaDataStore
	logger      *zap.Logger
	threshold   int // Storage pressure threshold in percentage (default: 90%)

	// Cached pressure status
	mu                 sync.RWMutex
	isStoragePressure  bool
	storageUsagePercent float64
	lastCheckTime      time.Time
}

// NewStoragePressureMonitor creates a new storage pressure monitor.
func NewStoragePressureMonitor(metaStorage metastorage.MetaDataStore, threshold int, logger *zap.Logger) *StoragePressureMonitor {
	if threshold <= 0 {
		threshold = 90 // Default threshold: 90%
	}
	return &StoragePressureMonitor{
		metaStorage: metaStorage,
		logger:      logger,
		threshold:   threshold,
	}
}

// IsStoragePressure checks if storage pressure is currently active.
// Returns true if storage usage is above the threshold (default: 90%).
// This method uses cached status for performance and may be slightly stale.
func (m *StoragePressureMonitor) IsStoragePressure() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isStoragePressure
}

// GetStorageUsagePercent returns the current storage usage percentage.
// This method uses cached status for performance and may be slightly stale.
func (m *StoragePressureMonitor) GetStorageUsagePercent() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.storageUsagePercent
}

// CheckStoragePressure checks the current storage pressure status by querying meta-storage.
// This method performs an actual check and updates the cached status.
func (m *StoragePressureMonitor) CheckStoragePressure(ctx context.Context) (bool, error) {
	if m.metaStorage == nil {
		// No meta-storage available, assume no pressure
		return false, nil
	}

	// Query meta-storage HealthSnapshot to get quota status
	health := m.metaStorage.HealthSnapshot(ctx)
	
	// Calculate usage percentage from quota
	usagePercent := 0.0
	hasPressure := false
	
	if health.Quota != nil && health.Quota.Limit > 0 {
		usagePercent = float64(health.Quota.Used) / float64(health.Quota.Limit) * 100.0
		hasPressure = usagePercent >= float64(m.threshold)
	}

	// Update cached status
	m.mu.Lock()
	oldPressure := m.isStoragePressure
	m.isStoragePressure = hasPressure
	m.storageUsagePercent = usagePercent
	m.lastCheckTime = time.Now()
	m.mu.Unlock()

	// Emit event if pressure state changed
	if oldPressure != hasPressure && m.logger != nil {
		if hasPressure {
			m.logger.Warn("Storage pressure detected",
				zap.Float64("usage_percent", usagePercent),
				zap.Int("threshold", m.threshold))
		} else {
			m.logger.Info("Storage pressure cleared",
				zap.Float64("usage_percent", usagePercent))
		}
	}

	return hasPressure, nil
}

// GetLastCheckTime returns the time of the last storage pressure check.
func (m *StoragePressureMonitor) GetLastCheckTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastCheckTime
}

// SetThreshold sets the storage pressure threshold.
func (m *StoragePressureMonitor) SetThreshold(threshold int) {
	if threshold <= 0 {
		threshold = 90 // Default threshold
	}
	m.threshold = threshold
}

// GetThreshold returns the current storage pressure threshold.
func (m *StoragePressureMonitor) GetThreshold() int {
	return m.threshold
}

