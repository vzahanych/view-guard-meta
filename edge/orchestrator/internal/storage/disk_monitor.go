package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// DiskMonitor monitors disk space usage
type DiskMonitor struct {
	path              string
	maxUsagePercent   float64
	logger            *logger.Logger
	mu                sync.RWMutex
	lastCheck         time.Time
	cacheDuration     time.Duration
	cachedUsage       *DiskUsage
}

// DiskUsage contains disk usage information
type DiskUsage struct {
	TotalBytes     int64
	UsedBytes     int64
	AvailableBytes int64
	UsagePercent   float64
}

// NewDiskMonitor creates a new disk monitor
func NewDiskMonitor(path string, maxUsagePercent float64, log *logger.Logger) (*DiskMonitor, error) {
	return &DiskMonitor{
		path:            path,
		maxUsagePercent: maxUsagePercent,
		logger:          log,
		cacheDuration:   30 * time.Second, // Cache for 30 seconds
	}, nil
}

// GetUsage returns current disk usage
func (d *DiskMonitor) GetUsage(ctx context.Context) (*DiskUsage, error) {
	d.mu.RLock()
	// Check cache
	if d.cachedUsage != nil && time.Since(d.lastCheck) < d.cacheDuration {
		usage := *d.cachedUsage
		d.mu.RUnlock()
		return &usage, nil
	}
	d.mu.RUnlock()

	// Get fresh usage
	usage, err := d.getDiskUsage()
	if err != nil {
		return nil, err
	}

	// Update cache
	d.mu.Lock()
	d.cachedUsage = usage
	d.lastCheck = time.Now()
	d.mu.Unlock()

	return usage, nil
}

// CheckSpace checks if there's enough disk space (below max usage)
func (d *DiskMonitor) CheckSpace(ctx context.Context) (bool, error) {
	usage, err := d.GetUsage(ctx)
	if err != nil {
		return false, err
	}

	return usage.UsagePercent < d.maxUsagePercent, nil
}

// IsDiskFull returns true if disk usage exceeds max usage
func (d *DiskMonitor) IsDiskFull(ctx context.Context) (bool, error) {
	hasSpace, err := d.CheckSpace(ctx)
	if err != nil {
		return false, err
	}
	return !hasSpace, nil
}

// getDiskUsage gets disk usage for the path
func (d *DiskMonitor) getDiskUsage() (*DiskUsage, error) {
	// Use os.Stat to get directory info, then use a cross-platform approach
	// For Linux, we can use syscall.Statfs, but for portability we'll use a simpler approach
	// that works on most systems by checking the parent directory's filesystem
	
	// Get absolute path
	absPath, err := filepath.Abs(d.path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Use syscall for Linux (most common case for edge devices)
	// For other platforms, we'd need platform-specific code
	var stat syscall.Statfs_t
	if err := syscall.Statfs(absPath, &stat); err != nil {
		return nil, fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Calculate sizes
	totalBytes := int64(stat.Blocks) * int64(stat.Bsize)
	availableBytes := int64(stat.Bavail) * int64(stat.Bsize)
	usedBytes := totalBytes - availableBytes

	// Calculate usage percent
	usagePercent := float64(usedBytes) / float64(totalBytes) * 100.0

	return &DiskUsage{
		TotalBytes:     totalBytes,
		UsedBytes:      usedBytes,
		AvailableBytes: availableBytes,
		UsagePercent:   usagePercent,
	}, nil
}

