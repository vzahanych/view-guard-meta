package capabilities

import (
	"context"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	grpcclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/grpc"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
	edgeproto "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// SyncService periodically reports camera/dataset readiness to the VM
type SyncService struct {
	*service.ServiceBase
	cameraMgr     *camera.Manager
	screenshotSvc *screenshots.Service
	grpcClient    *grpcclient.Client
	minSnapshots  int
	interval      time.Duration
	cancel        context.CancelFunc
	syncTrigger   chan struct{} // Channel to trigger immediate sync
}

// NewSyncService creates a new capability sync service
func NewSyncService(cfg *config.Config, camMgr *camera.Manager, screenshotSvc *screenshots.Service, grpcClient *grpcclient.Client, log *logger.Logger) *SyncService {
	minSnapshots := cfg.Edge.AI.MinNormalSnapshots
	if minSnapshots <= 0 {
		minSnapshots = 50
	}

	// Default sync interval: 5 minutes (as per implementation plan)
	interval := 5 * time.Minute

	return &SyncService{
		ServiceBase:   service.NewServiceBase("capability-sync", log),
		cameraMgr:     camMgr,
		screenshotSvc: screenshotSvc,
		grpcClient:    grpcClient,
		minSnapshots:  minSnapshots,
		interval:      interval,
		syncTrigger:   make(chan struct{}, 1), // Buffered channel for immediate sync triggers
	}
}

// Start begins the sync loop
func (s *SyncService) Start(ctx context.Context) error {
	s.GetStatus().SetStatus(service.StatusRunning)

	if s.cameraMgr == nil || s.grpcClient == nil {
		s.LogInfo("Capability sync disabled (missing camera manager or gRPC client)")
		s.GetStatus().SetStatus(service.StatusStopped)
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Subscribe to WireGuard connection events for immediate sync
	if s.GetEventBus() != nil {
		ch := s.GetEventBus().Subscribe(service.EventTypeWireGuardConnected)
		go func() {
			for {
				select {
				case event, ok := <-ch:
					if !ok {
						return
					}
					s.handleWireGuardConnected(event)
				case <-runCtx.Done():
					return
				}
			}
		}()
		s.LogInfo("Subscribed to WireGuard connection events for immediate capability sync")
	}

	go s.syncLoop(runCtx)

	return nil
}

// Stop stops the sync service
func (s *SyncService) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	s.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

func (s *SyncService) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Attempt an immediate sync on startup
	s.syncOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		case <-s.syncTrigger:
			// Immediate sync triggered by WireGuard connection event
			s.LogInfo("Triggering immediate capability sync after WireGuard connection")
			s.syncOnce(ctx)
		}
	}
}

// handleWireGuardConnected handles WireGuard connection events and triggers immediate sync
func (s *SyncService) handleWireGuardConnected(event service.Event) {
	// Trigger immediate sync when WireGuard connects
	select {
	case s.syncTrigger <- struct{}{}:
		// Sync triggered successfully
	default:
		// Channel is full, sync already queued
	}
}

func (s *SyncService) syncOnce(ctx context.Context) {
	if !s.grpcClient.IsConnected() {
		s.LogDebug("Skipping capability sync - gRPC not connected")
		return
	}

	controlClient := s.grpcClient.GetControlClient()
	if controlClient == nil {
		s.LogDebug("Skipping capability sync - control client unavailable")
		return
	}

	cameras := s.cameraMgr.ListCameras(false)
	if len(cameras) == 0 {
		return
	}

	req := &edgeproto.SyncCapabilitiesRequest{
		SyncedAt: time.Now().UnixNano(),
	}

	for _, cam := range cameras {
		status := s.buildDatasetStatus(ctx, cam)
		if status != nil {
			s.cameraMgr.UpdateDatasetStatus(cam.ID, status)
			req.Cameras = append(req.Cameras, s.toProto(cam, status))
		}
	}

	if len(req.Cameras) == 0 {
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := controlClient.SyncCapabilities(callCtx, req)
	if err != nil {
		s.LogError("Capability sync failed", err)
		return
	}

	if !resp.Success {
		s.LogInfo("Capability sync rejected", "error", resp.ErrorMessage)
		return
	}

	s.LogInfo("Capability sync sent", "cameras", len(req.Cameras))
}

func (s *SyncService) buildDatasetStatus(ctx context.Context, cam *camera.Camera) *camera.CameraDatasetStatus {
	stats := &camera.CameraDatasetStatus{
		LabelCounts:           make(map[string]int),
		RequiredSnapshotCount: s.minSnapshots,
		LastSynced:            time.Now(),
	}

	if s.screenshotSvc == nil {
		stats.SnapshotRequired = true
		return stats
	}

	counts, err := s.screenshotSvc.GetLabelCounts(ctx, cam.ID)
	if err != nil {
		s.LogInfo("Failed to get label counts", "camera_id", cam.ID, "error", err)
		stats.SnapshotRequired = true
		return stats
	}

	for label, count := range counts {
		stats.LabelCounts[string(label)] = count
		if label == screenshots.LabelNormal {
			stats.LabeledSnapshotCount = count
		}
	}

	stats.SnapshotRequired = stats.LabeledSnapshotCount < s.minSnapshots
	return stats
}

func (s *SyncService) toProto(cam *camera.Camera, status *camera.CameraDatasetStatus) *edgeproto.CameraCapability {
	labelCounts := make(map[string]uint32, len(status.LabelCounts))
	for label, count := range status.LabelCounts {
		if count < 0 {
			continue
		}
		labelCounts[label] = uint32(count)
	}

	return &edgeproto.CameraCapability{
		CameraId:              cam.ID,
		Name:                  cam.Name,
		Type:                  string(cam.Type),
		Enabled:               cam.Enabled,
		Status:                string(cam.Status),
		LabelCounts:           labelCounts,
		LabeledSnapshotCount:  uint32(status.LabeledSnapshotCount),
		RequiredSnapshotCount: uint32(status.RequiredSnapshotCount),
		SnapshotRequired:      status.SnapshotRequired,
	}
}
