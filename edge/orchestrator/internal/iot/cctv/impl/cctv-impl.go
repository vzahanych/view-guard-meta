package impl

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/internal/discovery"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/internal/rtsp"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"go.uber.org/zap"
)

// cctvService implements the CCTVService interface
type CCTVServiceImpl struct {
	logger              *zap.Logger
	metaStore           metastorage.MetaDataStore
	objectStore         objectstorage.ObjectStorageService
	eventBus            eventbus.EventBus
	onvifDiscovery      *discovery.ONVIFDiscoveryService
	usbDiscovery        *discovery.USBDiscoveryService
	rtspClients         map[string]*rtsp.RTSPClient
	activeRecordings    map[string]*recordingSession
	activeFrameCaptures map[string]*frameCaptureSession
	activeStreams       map[string]*streamingSession
	ffmpeg              *FFmpegWrapper
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
}

// recordingSession tracks an active recording
type recordingSession struct {
	CameraID  string
	EventID   string
	ClipID    string
	StartTime time.Time
	Duration  time.Duration
	Cancel    context.CancelFunc
}

// frameCaptureSession tracks an active frame capture
type frameCaptureSession struct {
	CameraID string
	OnFrame  func(*cctvtypes.Frame)
	Cancel   context.CancelFunc
}

// NewCCTVService creates a new CCTV service implementation
func NewCCTVService(
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	onvifDiscovery *discovery.ONVIFDiscoveryService,
	usbDiscovery *discovery.USBDiscoveryService,
	ffmpeg *FFmpegWrapper,
	log *zap.Logger,
) *CCTVServiceImpl {
	ctx, cancel := context.WithCancel(context.Background())

	return &CCTVServiceImpl{
		logger:              log,
		metaStore:           metaStore,
		objectStore:         objectStore,
		eventBus:            eventBus,
		onvifDiscovery:      onvifDiscovery,
		usbDiscovery:        usbDiscovery,
		rtspClients:         make(map[string]*rtsp.RTSPClient),
		activeRecordings:    make(map[string]*recordingSession),
		activeFrameCaptures: make(map[string]*frameCaptureSession),
		activeStreams:       make(map[string]*streamingSession),
		ffmpeg:              ffmpeg,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// Name returns the service name
func (s *CCTVServiceImpl) Name() string {
	return "cctv-service"
}

// Start starts the CCTV service
func (s *CCTVServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Starting CCTV service")

	// Reinitialize context for restart safety
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel() // Cancel previous context if restarting
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// NOTE: Discovery services are NOT started automatically.
	// They will be started by DiscoverCameras() when triggered by state manager
	// after camera capabilities are confirmed from VM.

	// Subscribe to discovery events
	if s.eventBus != nil {
		ch := s.eventBus.Subscribe(evtbusstypes.EventTypeDeviceDiscovered)
		go s.handleCameraDiscovered(ch)
	}

	return nil
}

// Stop stops the CCTV service
func (s *CCTVServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Stopping CCTV service")

	s.cancel()

	// Stop all RTSP clients
	s.mu.Lock()
	for _, client := range s.rtspClients {
		client.Stop(ctx)
	}
	s.mu.Unlock()

	// Stop discovery services
	if s.onvifDiscovery != nil {
		s.onvifDiscovery.Stop(ctx)
	}
	if s.usbDiscovery != nil {
		s.usbDiscovery.Stop(ctx)
	}

	return nil
}

// handleCameraDiscovered handles camera discovery events
func (s *CCTVServiceImpl) handleCameraDiscovered(ch <-chan evtbusstypes.Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if event.Type != evtbusstypes.EventTypeDeviceDiscovered {
				continue
			}

			cameraID, _ := event.Data["camera_id"].(string)
			if cameraID == "" {
				continue
			}

			// Get discovered camera from appropriate discovery service
			var discoveredCam *discovery.DiscoveredCamera
			if s.onvifDiscovery != nil {
				discoveredCam = s.onvifDiscovery.GetCameraByID(cameraID)
			}
			if discoveredCam == nil && s.usbDiscovery != nil {
				discoveredCam = s.usbDiscovery.GetCameraByID(cameraID)
			}

			if discoveredCam == nil {
				continue
			}

			// Convert to CCTV Camera and register
			cctvCam := s.discoveredToCCTV(discoveredCam)
			if err := s.RegisterCamera(context.Background(), cctvCam); err != nil {
				s.logger.Error("Failed to register discovered camera", zap.Error(err), zap.String("camera_id", cameraID))
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// discoveredToCCTV converts discovery.DiscoveredCamera to cctvtypes.Camera
func (s *CCTVServiceImpl) discoveredToCCTV(discovered *discovery.DiscoveredCamera) *cctvtypes.Camera {
	cameraType := cctvtypes.CameraTypeRTSP
	devicePath := ""
	rtspURLs := discovered.RTSPURLs

	if discovered.ONVIFEndpoint != "" {
		cameraType = cctvtypes.CameraTypeONVIF
	} else if len(discovered.RTSPURLs) > 0 && discovered.RTSPURLs[0] != "" && len(discovered.RTSPURLs[0]) > 0 && discovered.RTSPURLs[0][0] == '/' {
		// USB camera detected (device path starts with /)
		cameraType = cctvtypes.CameraTypeUSB
		devicePath = discovered.RTSPURLs[0]
		rtspURLs = []string{}
	}

	// Convert capabilities
	streamProfiles := make([]cctvtypes.StreamProfile, len(discovered.Capabilities.StreamProfiles))
	for i, sp := range discovered.Capabilities.StreamProfiles {
		streamProfiles[i] = cctvtypes.StreamProfile{
			Name:      sp.Name,
			Width:     sp.Width,
			Height:    sp.Height,
			FrameRate: sp.FrameRate,
			RTSPURL:   sp.RTSPURL,
			Encoding:  sp.Encoding,
		}
	}

	return &cctvtypes.Camera{
		ID:            discovered.ID,
		Name:          discovered.Model,
		Type:          cameraType,
		Manufacturer:  discovered.Manufacturer,
		Model:         discovered.Model,
		Enabled:       true,
		Status:        cctvtypes.CameraStatusOffline,
		DiscoveredAt:  discovered.DiscoveredAt,
		IPAddress:     discovered.IPAddress,
		ONVIFEndpoint: discovered.ONVIFEndpoint,
		RTSPURLs:      rtspURLs,
		DevicePath:    devicePath,
		Config: cctvtypes.CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  true,
			Quality:          "medium",
			FrameRate:        15,
		},
		Capabilities: cctvtypes.CameraCapabilities{
			HasPTZ:          discovered.Capabilities.HasPTZ,
			HasSnapshot:     discovered.Capabilities.HasSnapshot,
			HasVideoStreams: discovered.Capabilities.HasVideoStreams,
			StreamProfiles:  streamProfiles,
		},
	}
}

// DiscoverCameras triggers immediate camera discovery
// This should only be called by state manager after camera capabilities are confirmed from VM
func (s *CCTVServiceImpl) DiscoverCameras(ctx context.Context) error {
	s.logger.Info("Initiating camera discovery (triggered by state manager)")
	
	// Start discovery services if they're not already running
	// This allows discovery to be triggered on-demand rather than automatically on startup
	// We always start them here since they're not started automatically in Start()
	if s.onvifDiscovery != nil {
		if err := s.onvifDiscovery.Start(ctx); err != nil {
			s.logger.Error("Failed to start ONVIF discovery", zap.Error(err))
			return fmt.Errorf("failed to start ONVIF discovery: %w", err)
		}
		// Trigger immediate discovery
		s.onvifDiscovery.TriggerDiscovery()
	}

	if s.usbDiscovery != nil {
		if err := s.usbDiscovery.Start(ctx); err != nil {
			s.logger.Error("Failed to start USB discovery", zap.Error(err))
			return fmt.Errorf("failed to start USB discovery: %w", err)
		}
		// Trigger immediate discovery
		s.usbDiscovery.TriggerDiscovery()
	}
	
	// Wait a moment for discovery to complete
	time.Sleep(500 * time.Millisecond)
	
	// Get all discovered cameras from discovery services
	discoveredCameras, err := s.GetDiscoveredCameras(ctx)
	if err != nil {
		return fmt.Errorf("failed to get discovered cameras: %w", err)
	}

	// Register all discovered cameras and publish events
	for _, camera := range discoveredCameras {
		// Check if camera is already registered
		_, found := s.metaStore.GetCamera(ctx, camera.ID)
		if !found {
			// Register the camera
			if err := s.RegisterCamera(ctx, camera); err != nil {
				s.logger.Warn("Failed to register discovered camera", 
					zap.String("camera_id", camera.ID),
					zap.Error(err))
				continue
			}
		}

		// Publish camera.discovered event
		if s.eventBus != nil {
			s.eventBus.Publish(evtbusstypes.Event{
				Type:      evtbusstypes.EventTypeDeviceDiscovered,
				Source:    s.Name(),
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"camera_id": camera.ID,
					"name":       camera.Name,
					"type":       string(camera.Type),
				},
			})
		}
	}

	if len(discoveredCameras) == 0 {
		s.logger.Info("No cameras discovered")
	} else {
		s.logger.Info("Camera discovery completed", zap.Int("count", len(discoveredCameras)))
	}

	return nil
}

// GetDiscoveredCameras returns all discovered cameras
func (s *CCTVServiceImpl) GetDiscoveredCameras(ctx context.Context) ([]*cctvtypes.Camera, error) {
	var cameras []*cctvtypes.Camera

	if s.onvifDiscovery != nil {
		discovered := s.onvifDiscovery.GetDiscoveredCameras()
		for _, dc := range discovered {
			cameras = append(cameras, s.discoveredToCCTV(dc))
		}
	}

	if s.usbDiscovery != nil {
		discovered := s.usbDiscovery.GetDiscoveredCameras()
		for _, dc := range discovered {
			cameras = append(cameras, s.discoveredToCCTV(dc))
		}
	}

	return cameras, nil
}

// GetCamera retrieves a camera by ID
func (s *CCTVServiceImpl) GetCamera(ctx context.Context, cameraID string) (*cctvtypes.Camera, error) {
	meta, found := s.metaStore.GetCamera(ctx, cameraID)
	if !found {
		return nil, fmt.Errorf("camera not found: %s", cameraID)
	}

	return s.metaToCCTV(meta), nil
}

// ListCameras lists all registered cameras
func (s *CCTVServiceImpl) ListCameras(ctx context.Context, enabledOnly bool) ([]*cctvtypes.Camera, error) {
	filters := &metastorage.CameraFilters{}
	if enabledOnly {
		enabled := true
		filters.EnabledOnly = &enabled
	}
	metas, err := s.metaStore.ListCameras(ctx, filters)
	if err != nil {
		return nil, err
	}

	cameras := make([]*cctvtypes.Camera, len(metas))
	for i, meta := range metas {
		cameras[i] = s.metaToCCTV(meta)
	}

	return cameras, nil
}

// RegisterCamera registers a discovered camera
func (s *CCTVServiceImpl) RegisterCamera(ctx context.Context, camera *cctvtypes.Camera) error {
	meta := s.cctvToMeta(camera)
	if err := s.metaStore.SaveCamera(ctx, meta); err != nil {
		return fmt.Errorf("failed to save camera metadata: %w", err)
	}

	// Publish event
	if s.eventBus != nil {
		s.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventTypeDeviceRegistered,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id": camera.ID,
				"name":      camera.Name,
				"type":      string(camera.Type),
			},
		})
	}

	s.logger.Info("Registered camera", zap.String("id", camera.ID), zap.String("name", camera.Name))
	return nil
}

// UpdateCamera updates camera configuration
func (s *CCTVServiceImpl) UpdateCamera(ctx context.Context, cameraID string, updates *cctvtypes.CameraUpdate) error {
	_, err := s.metaStore.UpdateCamera(ctx, cameraID, func(meta metastorage.CameraMetadata) metastorage.CameraMetadata {
		if updates.Name != nil {
			meta.Name = *updates.Name
		}
		if updates.Enabled != nil {
			meta.Enabled = *updates.Enabled
		}
		if updates.Config != nil {
			// Update config in metadata
			if meta.Config == nil {
				meta.Config = make(map[string]interface{})
			}
			meta.Config["recording_enabled"] = updates.Config.RecordingEnabled
			meta.Config["motion_detection"] = updates.Config.MotionDetection
			meta.Config["quality"] = updates.Config.Quality
			meta.Config["frame_rate"] = updates.Config.FrameRate
			meta.Config["resolution"] = updates.Config.Resolution
		}
		return meta
	})
	return err
}

// EnableCamera enables a camera
func (s *CCTVServiceImpl) EnableCamera(ctx context.Context, cameraID string) error {
	return s.UpdateCamera(ctx, cameraID, &cctvtypes.CameraUpdate{Enabled: boolPtr(true)})
}

// DisableCamera disables a camera
func (s *CCTVServiceImpl) DisableCamera(ctx context.Context, cameraID string) error {
	return s.UpdateCamera(ctx, cameraID, &cctvtypes.CameraUpdate{Enabled: boolPtr(false)})
}

// DeleteCamera deletes a camera
func (s *CCTVServiceImpl) DeleteCamera(ctx context.Context, cameraID string) error {
	// Stop any active recordings or frame captures
	s.StopRecording(ctx, cameraID)
	s.StopFrameCapture(ctx, cameraID)

	// Stop RTSP client if active
	s.mu.Lock()
	if client, ok := s.rtspClients[cameraID]; ok {
		client.Stop(ctx)
		delete(s.rtspClients, cameraID)
	}
	s.mu.Unlock()

	// Delete from metadata store
	return s.metaStore.DeleteCamera(ctx, cameraID)
}

// Helper functions for conversion

func (s *CCTVServiceImpl) cctvToMeta(camera *cctvtypes.Camera) metastorage.CameraMetadata {
	config := map[string]interface{}{
		"recording_enabled": camera.Config.RecordingEnabled,
		"motion_detection":  camera.Config.MotionDetection,
		"quality":           camera.Config.Quality,
		"frame_rate":        camera.Config.FrameRate,
		"resolution":        camera.Config.Resolution,
	}

	capabilities := map[string]interface{}{
		"has_ptz":           camera.Capabilities.HasPTZ,
		"has_snapshot":      camera.Capabilities.HasSnapshot,
		"has_video_streams": camera.Capabilities.HasVideoStreams,
		"stream_profiles":   camera.Capabilities.StreamProfiles,
	}

	return metastorage.CameraMetadata{
		ID:            camera.ID,
		Name:          camera.Name,
		Type:          string(camera.Type),
		Manufacturer:  camera.Manufacturer,
		Model:         camera.Model,
		Enabled:       camera.Enabled,
		Status:        string(camera.Status),
		LastSeen:      camera.LastSeen,
		DiscoveredAt:  camera.DiscoveredAt,
		IPAddress:     camera.IPAddress,
		ONVIFEndpoint: camera.ONVIFEndpoint,
		RTSPURLs:      camera.RTSPURLs,
		DevicePath:    camera.DevicePath,
		Config:        config,
		Capabilities:  capabilities,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func (s *CCTVServiceImpl) metaToCCTV(meta metastorage.CameraMetadata) *cctvtypes.Camera {
	var config cctvtypes.CameraConfig
	if meta.Config != nil {
		if rec, ok := meta.Config["recording_enabled"].(bool); ok {
			config.RecordingEnabled = rec
		}
		if md, ok := meta.Config["motion_detection"].(bool); ok {
			config.MotionDetection = md
		}
		if q, ok := meta.Config["quality"].(string); ok {
			config.Quality = q
		}
		if fr, ok := meta.Config["frame_rate"].(int); ok {
			config.FrameRate = fr
		}
		if res, ok := meta.Config["resolution"].(string); ok {
			config.Resolution = res
		}
	}

	var capabilities cctvtypes.CameraCapabilities
	if meta.Capabilities != nil {
		if hasPTZ, ok := meta.Capabilities["has_ptz"].(bool); ok {
			capabilities.HasPTZ = hasPTZ
		}
		if hasSnapshot, ok := meta.Capabilities["has_snapshot"].(bool); ok {
			capabilities.HasSnapshot = hasSnapshot
		}
		if hasVideoStreams, ok := meta.Capabilities["has_video_streams"].(bool); ok {
			capabilities.HasVideoStreams = hasVideoStreams
		}
		// Stream profiles would need more complex conversion
	}

	return &cctvtypes.Camera{
		ID:            meta.ID,
		Name:          meta.Name,
		Type:          cctvtypes.CameraType(meta.Type),
		Manufacturer:  meta.Manufacturer,
		Model:         meta.Model,
		Enabled:       meta.Enabled,
		Status:        cctvtypes.CameraStatus(meta.Status),
		LastSeen:      meta.LastSeen,
		DiscoveredAt:  meta.DiscoveredAt,
		IPAddress:     meta.IPAddress,
		ONVIFEndpoint: meta.ONVIFEndpoint,
		RTSPURLs:      meta.RTSPURLs,
		DevicePath:    meta.DevicePath,
		Config:        config,
		Capabilities:  capabilities,
	}
}

func boolPtr(b bool) *bool {
	return &b
}

// Frame capture, screenshot, and clip recording methods will be implemented next
// These are placeholders for now

func (s *CCTVServiceImpl) StartFrameCapture(ctx context.Context, cameraID string, onFrame func(frame *cctvtypes.Frame)) error {
	// TODO: Implement frame capture
	return fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) StopFrameCapture(ctx context.Context, cameraID string) error {
	// TODO: Implement stop frame capture
	return fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) CaptureFrame(ctx context.Context, cameraID string) (*cctvtypes.Frame, error) {
	// Get camera
	cam, err := s.GetCamera(ctx, cameraID)
	if err != nil {
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	// Get input source
	input := s.getCameraInput(cam)
	if input == "" {
		return nil, fmt.Errorf("no valid input source for camera %s", cameraID)
	}

	// Add timeout to frame capture to prevent hanging
	// Use parent context if it has a deadline, otherwise add 10 second timeout
	captureCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		captureCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	// Use FFmpeg wrapper to capture frame
	frameData, err := s.ffmpeg.CaptureFrameJPEG(captureCtx, input, 85)
	if err != nil {
		return nil, err
	}

	// Create Frame struct
	return &cctvtypes.Frame{
		Data:      frameData,
		Timestamp: time.Now(),
		Width:     0, // Will be determined if frame is decoded later
		Height:    0, // Will be determined if frame is decoded later
		CameraID:  cameraID,
	}, nil
}

// Screenshot methods are implemented in screenshot-impl.go

func (s *CCTVServiceImpl) StartRecording(ctx context.Context, cameraID string, eventID string, duration time.Duration) (string, error) {
	// TODO: Implement clip recording
	return "", fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) StopRecording(ctx context.Context, cameraID string) error {
	// TODO: Implement stop recording
	return fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) GetClip(ctx context.Context, clipID string) (*cctvtypes.Clip, error) {
	// TODO: Implement get clip
	return nil, fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) ListClips(ctx context.Context, filters *cctvtypes.ClipFilters) ([]*cctvtypes.Clip, error) {
	// TODO: Implement list clips
	return nil, fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) DeleteClip(ctx context.Context, clipID string) error {
	// TODO: Implement delete clip
	return fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) GetClipStream(ctx context.Context, clipID string) (io.ReadCloser, error) {
	// TODO: Implement get clip stream
	return nil, fmt.Errorf("not implemented")
}

// StartMJPEGStream starts an MJPEG stream for a camera (for web UI)
func (s *CCTVServiceImpl) StartMJPEGStream(ctx context.Context, cameraID string) (*cctvtypes.MJPEGStream, error) {
	stream, err := s.startMJPEGStream(ctx, cameraID)
	if err != nil {
		return nil, err
	}

	return &cctvtypes.MJPEGStream{
		CameraID:     stream.CameraID,
		FrameChan:    stream.FrameChan,
		Done:         stream.Done(),
		GetLastFrame: stream.GetLastFrame,
	}, nil
}

// StopMJPEGStream stops an MJPEG stream
func (s *CCTVServiceImpl) StopMJPEGStream(ctx context.Context, cameraID string) error {
	s.stopMJPEGStream(cameraID)
	return nil
}

// GetMJPEGStream gets an active MJPEG stream
func (s *CCTVServiceImpl) GetMJPEGStream(ctx context.Context, cameraID string) (*cctvtypes.MJPEGStream, error) {
	stream, err := s.getMJPEGStream(cameraID)
	if err != nil {
		return nil, err
	}

	return &cctvtypes.MJPEGStream{
		CameraID:     stream.CameraID,
		FrameChan:    stream.FrameChan,
		Done:         stream.Done(),
		GetLastFrame: stream.GetLastFrame,
	}, nil
}
