package capabilities

import (
	"context"
	"fmt"
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

		// Subscribe to screenshot events for immediate dataset status refresh (Step 2.2.2.1.2)
		screenshotCh := s.GetEventBus().Subscribe(service.EventTypeScreenshotSaved)
		go s.handleScreenshotSaved(runCtx, screenshotCh)

		updateCh := s.GetEventBus().Subscribe(service.EventTypeScreenshotUpdated)
		go s.handleScreenshotSaved(runCtx, updateCh) // Reuse same handler

		deleteCh := s.GetEventBus().Subscribe(service.EventTypeScreenshotDeleted)
		go s.handleScreenshotSaved(runCtx, deleteCh) // Reuse same handler
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

// handleScreenshotSaved handles screenshot saved/updated/deleted events and triggers immediate dataset status refresh
func (s *SyncService) handleScreenshotSaved(ctx context.Context, ch <-chan service.Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			// Extract camera_id from event data
			cameraID, ok := event.Data["camera_id"].(string)
			if !ok || cameraID == "" {
				continue
			}

			// Get camera and refresh its dataset status immediately
			if s.cameraMgr != nil {
				cam, err := s.cameraMgr.GetCamera(cameraID)
				if err == nil && cam != nil {
					// Recalculate dataset status for this camera
					status := s.buildDatasetStatus(ctx, cam)
					s.cameraMgr.UpdateDatasetStatus(cameraID, status)
					s.LogDebug("Refreshed dataset status after screenshot event", "camera_id", cameraID, "event_type", event.Type)
				}
			}

			// Trigger capability sync to VM (if connected)
			select {
			case s.syncTrigger <- struct{}{}:
				// Sync triggered successfully
			default:
				// Channel is full, sync already queued
			}
		case <-ctx.Done():
			return
		}
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

// SyncCameraCapabilities syncs capabilities for a single camera to the VM
// This is used when a dataset is uploaded to immediately update the VM with the latest status
func (s *SyncService) SyncCameraCapabilities(ctx context.Context, cameraID string) error {
	if !s.grpcClient.IsConnected() {
		return fmt.Errorf("gRPC not connected")
	}

	controlClient := s.grpcClient.GetControlClient()
	if controlClient == nil {
		return fmt.Errorf("control client unavailable")
	}

	// Get camera
	cam, err := s.cameraMgr.GetCamera(cameraID)
	if err != nil {
		return fmt.Errorf("failed to get camera: %w", err)
	}

	// Build dataset status for this camera
	status := s.buildDatasetStatus(ctx, cam)
	if status == nil {
		return fmt.Errorf("failed to build dataset status")
	}

	// Update camera manager with latest status
	s.cameraMgr.UpdateDatasetStatus(cameraID, status)

	// Build sync request with single camera
	req := &edgeproto.SyncCapabilitiesRequest{
		SyncedAt: time.Now().UnixNano(),
		Cameras:  []*edgeproto.CameraCapability{s.toProto(cam, status)},
	}

	// Call SyncCapabilities with timeout
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := controlClient.SyncCapabilities(callCtx, req)
	if err != nil {
		return fmt.Errorf("capability sync failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("capability sync rejected: %s", resp.ErrorMessage)
	}

	s.LogInfo("Camera capability synced", "camera_id", cameraID)
	return nil
}

func (s *SyncService) buildDatasetStatus(ctx context.Context, cam *camera.Camera) *camera.CameraDatasetStatus {
	// Use the shared helper method from ScreenshotService
	if s.screenshotSvc == nil {
		return &camera.CameraDatasetStatus{
			LabelCounts:           make(map[string]int),
			RequiredSnapshotCount: s.minSnapshots,
			SnapshotRequired:      true,
			LastSynced:            time.Now(),
		}
	}

	datasetStatus, err := s.screenshotSvc.GetDatasetStatus(ctx, cam.ID, s.minSnapshots)
	if err != nil {
		s.LogInfo("Failed to get dataset status", "camera_id", cam.ID, "error", err)
		return &camera.CameraDatasetStatus{
			LabelCounts:           make(map[string]int),
			RequiredSnapshotCount: s.minSnapshots,
			SnapshotRequired:      true,
			LastSynced:            time.Now(),
		}
	}

	// Convert DatasetStatus to CameraDatasetStatus
	return &camera.CameraDatasetStatus{
		LabelCounts:           datasetStatus.LabelCounts,
		LabeledSnapshotCount:  datasetStatus.LabeledSnapshotCount,
		RequiredSnapshotCount: datasetStatus.RequiredSnapshotCount,
		SnapshotRequired:      datasetStatus.SnapshotRequired,
		LastSynced:            datasetStatus.LastSynced,
	}
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
