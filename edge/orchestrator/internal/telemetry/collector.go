package telemetry

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"

	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// Collector collects system and application metrics
type Collector struct {
	*service.ServiceBase
	config        *config.TelemetryConfig
	logger        *logger.Logger
	cameraManager *camera.Manager
	eventQueue    *events.Queue
	eventStorage  *events.Storage
	storageSvc    *storage.StorageService
	wgClient      *wireguard.Client
	mu            sync.RWMutex
	lastMetrics   *edge.TelemetryData
}

// NewCollector creates a new telemetry collector
func NewCollector(
	cfg *config.TelemetryConfig,
	log *logger.Logger,
	cameraManager *camera.Manager,
	eventQueue *events.Queue,
	eventStorage *events.Storage,
	storageSvc *storage.StorageService,
	wgClient *wireguard.Client,
) *Collector {
	return &Collector{
		ServiceBase:   service.NewServiceBase("telemetry-collector", log),
		config:        cfg,
		logger:        log,
		cameraManager: cameraManager,
		eventQueue:    eventQueue,
		eventStorage:  eventStorage,
		storageSvc:    storageSvc,
		wgClient:      wgClient,
	}
}

// Start starts the telemetry collector service
func (c *Collector) Start(ctx context.Context) error {
	c.GetStatus().SetStatus(service.StatusRunning)

	if !c.config.Enabled {
		c.LogInfo("Telemetry collection is disabled")
		return nil
	}

	c.LogInfo("Telemetry collector started")
	return nil
}

// Stop stops the telemetry collector service
func (c *Collector) Stop(ctx context.Context) error {
	c.LogInfo("Telemetry collector stopped")
	c.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

// Collect collects all system and application metrics
func (c *Collector) Collect(ctx context.Context) (*edge.TelemetryData, error) {
	data := &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    "edge-001", // TODO: Get from config/state
	}

	// System metrics
	systemMetrics, err := c.collectSystemMetrics(ctx)
	if err != nil {
		c.logger.Warn("Failed to collect system metrics", "error", err)
	} else {
		data.System = systemMetrics
	}

	// Application metrics
	appMetrics, err := c.collectApplicationMetrics(ctx)
	if err != nil {
		c.logger.Warn("Failed to collect application metrics", "error", err)
	} else {
		data.Application = appMetrics
	}

	// Camera statuses
	cameraStatuses, err := c.collectCameraStatuses(ctx)
	if err != nil {
		c.logger.Warn("Failed to collect camera statuses", "error", err)
	} else {
		data.Cameras = cameraStatuses
	}

	// Store last metrics
	c.mu.Lock()
	c.lastMetrics = data
	c.mu.Unlock()

	return data, nil
}

// GetLastMetrics returns the last collected metrics
func (c *Collector) GetLastMetrics() *edge.TelemetryData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastMetrics
}

// collectSystemMetrics collects system-level metrics
func (c *Collector) collectSystemMetrics(ctx context.Context) (*edge.SystemMetrics, error) {
	// CPU usage (simplified - would use gopsutil in production)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	cpuUsage := 0.1 // Placeholder
	if m.NumGC > 0 {
		cpuUsage = 0.3 // Placeholder
	}

	// Memory usage
	sysMem := uint64(8 * 1024 * 1024 * 1024) // Assume 8GB for PoC
	memUsed := m.Sys
	if memUsed > sysMem {
		memUsed = sysMem
	}

	// Disk usage
	var diskUsed, diskTotal uint64
	var diskUsagePercent float64
	if c.storageSvc != nil {
		diskUsage, err := c.storageSvc.GetDiskUsage(ctx)
		if err == nil {
			diskUsed = uint64(diskUsage.UsedBytes)
			diskTotal = uint64(diskUsage.TotalBytes)
			diskUsagePercent = diskUsage.UsagePercent
		}
	}

	return &edge.SystemMetrics{
		CpuUsagePercent:  cpuUsage * 100, // Convert to percentage
		MemoryUsedBytes:  memUsed,
		MemoryTotalBytes: sysMem,
		DiskUsedBytes:    diskUsed,
		DiskTotalBytes:   diskTotal,
		DiskUsagePercent: diskUsagePercent,
	}, nil
}

// collectApplicationMetrics collects application-level metrics
func (c *Collector) collectApplicationMetrics(ctx context.Context) (*edge.ApplicationMetrics, error) {
	// Event queue length
	eventQueueLength := int32(0)
	if c.eventQueue != nil {
		stats, err := c.eventQueue.GetQueueStats(ctx)
		if err == nil {
			eventQueueLength = int32(stats.Size)
		}
	}

	// Active cameras
	activeCameras := int32(0)
	if c.cameraManager != nil {
		cameras := c.cameraManager.ListCameras(false)
		for _, cam := range cameras {
			if cam.Status == camera.CameraStatusOnline {
				activeCameras++
			}
		}
	}

	// AI inference time (placeholder)
	aiInferenceTimeMs := 0.0

	// Storage stats
	storageClipsCount := int32(0)
	storageClipsSizeBytes := uint64(0)
	if c.storageSvc != nil {
		stats, err := c.storageSvc.GetStorageStats(ctx)
		if err == nil {
			storageClipsCount = int32(stats.TotalClips)
			storageClipsSizeBytes = uint64(stats.TotalSizeBytes)
		}
	}

	return &edge.ApplicationMetrics{
		EventQueueLength:      eventQueueLength,
		ActiveCameras:         activeCameras,
		AiInferenceTimeMs:     aiInferenceTimeMs,
		StorageClipsCount:     storageClipsCount,
		StorageClipsSizeBytes: storageClipsSizeBytes,
	}, nil
}

// collectCameraStatuses collects status of all cameras
func (c *Collector) collectCameraStatuses(ctx context.Context) ([]*edge.CameraStatus, error) {
	if c.cameraManager == nil {
		return []*edge.CameraStatus{}, nil
	}

	cameras := c.cameraManager.ListCameras(false)

	statuses := make([]*edge.CameraStatus, 0, len(cameras))
	for _, cam := range cameras {
		lastSeen := ""
		if cam.LastSeen != nil {
			lastSeen = cam.LastSeen.Format(time.RFC3339)
		}
		status := &edge.CameraStatus{
			CameraId:      cam.ID,
			Online:        cam.Status == camera.CameraStatusOnline,
			LastSeen:      lastSeen,
			StatusMessage: string(cam.Status),
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

