package snapshot_request

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/streaming"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service handles snapshot capture requests from VM
type Service struct {
	*service.ServiceBase
	logger          *logger.Logger
	cameraManager   *camera.Manager
	screenshotSvc   *screenshots.Service
	streamingSvc    *streaming.Service
	config          *config.Config
	pendingRequests map[string]*PendingRequest // camera_id -> request
	mu              sync.RWMutex
}

// PendingRequest represents a pending snapshot request from VM
type PendingRequest struct {
	CameraID    string
	Label       string
	CustomLabel string
	Count       int32
	RequestedAt time.Time
}

// NewService creates a new snapshot request service
func NewService(
	log *logger.Logger,
	cameraManager *camera.Manager,
	screenshotSvc *screenshots.Service,
	streamingSvc *streaming.Service,
	cfg *config.Config,
) *Service {
	return &Service{
		ServiceBase:     service.NewServiceBase("snapshot-request", log),
		logger:          log,
		cameraManager:   cameraManager,
		screenshotSvc:   screenshotSvc,
		streamingSvc:    streamingSvc,
		config:          cfg,
		pendingRequests: make(map[string]*PendingRequest),
	}
}

// RequestSnapshotCapture handles snapshot capture request from VM
func (s *Service) RequestSnapshotCapture(ctx context.Context, req *edge.RequestSnapshotCaptureRequest) (*edge.RequestSnapshotCaptureResponse, error) {
	if req.CameraId == "" {
		return &edge.RequestSnapshotCaptureResponse{
			Accepted: false,
			Message:  "camera_id is required",
		}, status.Error(codes.InvalidArgument, "camera_id is required")
	}

	// Validate camera exists
	cam, err := s.cameraManager.GetCamera(req.CameraId)
	if err != nil || cam == nil {
		return &edge.RequestSnapshotCaptureResponse{
			Accepted: false,
			Message:  fmt.Sprintf("camera %s not found", req.CameraId),
		}, status.Error(codes.NotFound, fmt.Sprintf("camera %s not found", req.CameraId))
	}

	// Set defaults
	label := req.Label
	if label == "" {
		label = "normal"
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}

	// Validate label
	validLabels := map[string]bool{
		"normal":   true,
		"threat":   true,
		"abnormal": true,
		"custom":   true,
	}
	if !validLabels[label] {
		return &edge.RequestSnapshotCaptureResponse{
			Accepted: false,
			Message:  fmt.Sprintf("invalid label: %s (must be normal, threat, abnormal, or custom)", label),
		}, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid label: %s", label))
	}

	if label == "custom" && req.CustomLabel == "" {
		return &edge.RequestSnapshotCaptureResponse{
			Accepted: false,
			Message:  "custom_label is required when label is 'custom'",
		}, status.Error(codes.InvalidArgument, "custom_label is required when label is 'custom'")
	}

	s.logger.Info("Received snapshot capture request from VM",
		"camera_id", req.CameraId,
		"label", label,
		"count", count,
		"auto_capture", req.AutoCapture)

	// If auto_capture is true, capture snapshots immediately (for integration tests)
	if req.AutoCapture {
		snapshotIDs, err := s.autoCaptureSnapshots(ctx, req.CameraId, label, req.CustomLabel, int(count))
		if err != nil {
			s.logger.Error("Auto-capture failed", err,
				"camera_id", req.CameraId,
				"label", label)
			return &edge.RequestSnapshotCaptureResponse{
				Accepted: false,
				Message:  fmt.Sprintf("auto-capture failed: %v", err),
			}, status.Error(codes.Internal, fmt.Sprintf("auto-capture failed: %v", err))
		}

		s.logger.Info("Auto-captured snapshots successfully",
			"camera_id", req.CameraId,
			"count", len(snapshotIDs),
			"snapshot_ids", snapshotIDs)

		return &edge.RequestSnapshotCaptureResponse{
			Accepted:    true,
			Message:     fmt.Sprintf("Captured %d snapshots", len(snapshotIDs)),
			SnapshotIds: snapshotIDs,
		}, nil
	}

	// Otherwise, store as pending request for UI notification
	s.mu.Lock()
	s.pendingRequests[req.CameraId] = &PendingRequest{
		CameraID:    req.CameraId,
		Label:       label,
		CustomLabel: req.CustomLabel,
		Count:       count,
		RequestedAt: time.Now(),
	}
	s.mu.Unlock()

	// Publish event for UI notification
	if s.GetEventBus() != nil {
		s.PublishEvent(service.EventTypeSnapshotRequested, map[string]interface{}{
			"camera_id":    req.CameraId,
			"label":        label,
			"custom_label": req.CustomLabel,
			"count":        count,
		})
	}

	s.logger.Info("Stored pending snapshot request for UI",
		"camera_id", req.CameraId,
		"label", label,
		"count", count)

	return &edge.RequestSnapshotCaptureResponse{
		Accepted: true,
		Message:  "Snapshot request stored, user will be notified in UI",
	}, nil
}

// autoCaptureSnapshots automatically captures snapshots from camera
func (s *Service) autoCaptureSnapshots(ctx context.Context, cameraID, label, customLabel string, count int) ([]string, error) {
	var snapshotIDs []string

	if s.streamingSvc == nil {
		return snapshotIDs, fmt.Errorf("streaming service not available")
	}

	for i := 0; i < count; i++ {
		// Capture frame from camera using streaming service
		frameData, err := s.streamingSvc.GetFrame(cameraID)
		if err != nil {
			return snapshotIDs, fmt.Errorf("failed to capture frame %d/%d: %w", i+1, count, err)
		}

		if len(frameData) == 0 {
			return snapshotIDs, fmt.Errorf("frame %d/%d has no data", i+1, count)
		}

		// Convert label string to Label type
		var screenshotLabel screenshots.Label
		switch label {
		case "normal":
			screenshotLabel = screenshots.LabelNormal
		case "threat":
			screenshotLabel = screenshots.LabelThreat
		case "abnormal":
			screenshotLabel = screenshots.LabelAbnormal
		case "custom":
			screenshotLabel = screenshots.LabelCustom
		default:
			screenshotLabel = screenshots.LabelNormal
		}

		// Save screenshot with label
		screenshot := &screenshots.Screenshot{
			CameraID:    cameraID,
			Label:       screenshotLabel,
			CustomLabel: customLabel,
			Description: fmt.Sprintf("Auto-captured snapshot %d/%d (VM request)", i+1, count),
		}

		if err := s.screenshotSvc.SaveScreenshot(ctx, screenshot, frameData); err != nil {
			return snapshotIDs, fmt.Errorf("failed to save screenshot %d/%d: %w", i+1, count, err)
		}

		snapshotIDs = append(snapshotIDs, screenshot.ID)

		// Small delay between captures
		if i < count-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return snapshotIDs, nil
}

// GetPendingRequest returns pending request for a camera
func (s *Service) GetPendingRequest(cameraID string) *PendingRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingRequests[cameraID]
}

// ClearPendingRequest clears pending request for a camera
func (s *Service) ClearPendingRequest(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingRequests, cameraID)
}

// GetAllPendingRequests returns all pending requests
func (s *Service) GetAllPendingRequests() map[string]*PendingRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*PendingRequest)
	for k, v := range s.pendingRequests {
		result[k] = v
	}
	return result
}

// SetStreamingService sets the streaming service (can be called after web server creates it)
func (s *Service) SetStreamingService(streamingSvc *streaming.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamingSvc = streamingSvc
}
