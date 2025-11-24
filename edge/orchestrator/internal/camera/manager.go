package camera

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// CameraType represents the type of camera
type CameraType string

const (
	CameraTypeRTSP  CameraType = "rtsp"
	CameraTypeONVIF CameraType = "onvif"
	CameraTypeUSB  CameraType = "usb"
)

// Camera represents a unified camera interface
type Camera struct {
	ID            string
	Name          string
	Type          CameraType
	Manufacturer  string
	Model         string
	Enabled       bool
	Status        CameraStatus
	LastSeen      *time.Time
	DiscoveredAt  time.Time
	
	// Network camera fields
	IPAddress     string
	ONVIFEndpoint string
	RTSPURLs      []string
	
	// USB camera fields
	DevicePath    string
	
	// Configuration
	Config        CameraConfig
	Capabilities  CameraCapabilities
}

// CameraStatus represents camera connection status
type CameraStatus string

const (
	CameraStatusUnknown    CameraStatus = "unknown"
	CameraStatusOnline     CameraStatus = "online"
	CameraStatusOffline    CameraStatus = "offline"
	CameraStatusConnecting CameraStatus = "connecting"
	CameraStatusError      CameraStatus = "error"
)

// CameraConfig represents camera configuration
type CameraConfig struct {
	RecordingEnabled bool
	MotionDetection  bool
	Quality          string
	FrameRate        int
	Resolution       string
}

// Manager manages cameras (unified interface for network and USB cameras)
type Manager struct {
	*service.ServiceBase
	stateMgr        *state.Manager
	onvifDiscovery  *ONVIFDiscoveryService
	usbDiscovery    *USBDiscoveryService
	rtspClients     map[string]*RTSPClient
	cameras         map[string]*Camera
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	statusInterval  time.Duration
}

// NewManager creates a new camera manager
func NewManager(
	stateMgr *state.Manager,
	onvifDiscovery *ONVIFDiscoveryService,
	usbDiscovery *USBDiscoveryService,
	statusInterval time.Duration,
	log *logger.Logger,
) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		ServiceBase:    service.NewServiceBase("camera-manager", log),
		stateMgr:       stateMgr,
		onvifDiscovery: onvifDiscovery,
		usbDiscovery:   usbDiscovery,
		rtspClients:    make(map[string]*RTSPClient),
		cameras:        make(map[string]*Camera),
		ctx:            ctx,
		cancel:         cancel,
		statusInterval: statusInterval,
	}
}

// Name returns the service name
func (m *Manager) Name() string {
	return "camera-manager"
}

// Start starts the camera management service
func (m *Manager) Start(ctx context.Context) error {
	m.GetStatus().SetStatus(service.StatusStarting)
	m.LogInfo("Starting camera management service")

	// Recover cameras from state
	if err := m.recoverCameras(ctx); err != nil {
		m.LogError("Failed to recover cameras", err)
		return err
	}

	// Start monitoring
	go m.monitorCameras()

	// Subscribe to discovery events
	if m.GetEventBus() != nil {
		ch := m.GetEventBus().Subscribe(service.EventTypeCameraDiscovered)
		go m.handleCameraDiscovered(ch)
	}

	m.GetStatus().SetStatus(service.StatusRunning)
	return nil
}

// Stop stops the camera management service
func (m *Manager) Stop(ctx context.Context) error {
	m.GetStatus().SetStatus(service.StatusStopping)
	m.LogInfo("Stopping camera management service")

	m.cancel()

	// Stop all RTSP clients
	m.mu.Lock()
	for _, client := range m.rtspClients {
		client.Stop(ctx)
	}
	m.mu.Unlock()

	m.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

// recoverCameras recovers cameras from state on startup
func (m *Manager) recoverCameras(ctx context.Context) error {
	cameras, err := m.stateMgr.ListCameras(ctx, false)
	if err != nil {
		return fmt.Errorf("failed to list cameras: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, camState := range cameras {
		camera := m.cameraStateToCamera(camState)
		if camera != nil {
			m.cameras[camera.ID] = camera
			m.LogInfo("Recovered camera", "id", camera.ID, "name", camera.Name, "type", camera.Type)
		}
	}

	return nil
}

// cameraStateToCamera converts state.CameraState to Camera
func (m *Manager) cameraStateToCamera(camState state.CameraState) *Camera {
	// Parse camera type and details from stored state
	// For now, we'll infer from RTSPURL or device path
	camera := &Camera{
		ID:           camState.ID,
		Name:         camState.Name,
		Enabled:      camState.Enabled,
		LastSeen:     camState.LastSeen,
		Status:       CameraStatusUnknown,
		Config:       CameraConfig{RecordingEnabled: true, MotionDetection: true},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
	}

	// Determine camera type from stored data
	// This is a simplified approach - in production, we'd store type explicitly
	if camState.RTSPURL != "" {
		if camState.RTSPURL[0] == '/' {
			camera.Type = CameraTypeUSB
			camera.DevicePath = camState.RTSPURL
		} else {
			camera.Type = CameraTypeRTSP
			camera.RTSPURLs = []string{camState.RTSPURL}
		}
	}

	return camera
}

// handleCameraDiscovered handles camera discovery events
func (m *Manager) handleCameraDiscovered(ch <-chan service.Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if event.Type != service.EventTypeCameraDiscovered {
				continue
			}

			cameraID, _ := event.Data["camera_id"].(string)
	if cameraID == "" {
		return
	}

	// Get discovered camera from appropriate discovery service
	var discoveredCam *DiscoveredCamera
	if m.onvifDiscovery != nil {
		discoveredCam = m.onvifDiscovery.GetCameraByID(cameraID)
	}
	if discoveredCam == nil && m.usbDiscovery != nil {
		discoveredCam = m.usbDiscovery.GetCameraByID(cameraID)
	}

			if discoveredCam == nil {
				continue
			}

			// Register or update camera
			if err := m.RegisterCamera(context.Background(), discoveredCam); err != nil {
				m.LogError("Failed to register discovered camera", err, "camera_id", cameraID)
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// RegisterCamera registers a discovered camera
func (m *Manager) RegisterCamera(ctx context.Context, discovered *DiscoveredCamera) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Determine camera type
	cameraType := CameraTypeRTSP
	devicePath := ""
	rtspURLs := discovered.RTSPURLs

	if discovered.ONVIFEndpoint != "" {
		cameraType = CameraTypeONVIF
	} else if len(discovered.RTSPURLs) > 0 && discovered.RTSPURLs[0][0] == '/' {
		cameraType = CameraTypeUSB
		devicePath = discovered.RTSPURLs[0]
		rtspURLs = []string{}
		
		// For USB cameras, check if we already have a camera with the same device path(s)
		// This handles the case where the camera ID changed but it's the same physical device
		devicePaths := discovered.RTSPURLs
		if len(devicePaths) == 0 {
			devicePaths = []string{devicePath}
		}
		
		// Look for existing cameras with any of these device paths
		for existingID, existingCam := range m.cameras {
			if existingCam.Type == CameraTypeUSB && existingID != discovered.ID {
				// Check if device paths overlap
				existingPaths := existingCam.RTSPURLs
				if existingCam.DevicePath != "" {
					existingPaths = append(existingPaths, existingCam.DevicePath)
				}
				
				for _, newPath := range devicePaths {
					for _, existingPath := range existingPaths {
						if newPath == existingPath {
							// Found existing camera with same device path - delete old one
							m.LogInfo("Removing duplicate USB camera",
								"old_id", existingID,
								"new_id", discovered.ID,
								"device_path", newPath,
							)
							
							// Delete old camera from state
							if err := m.stateMgr.DeleteCamera(ctx, existingID); err != nil {
								m.LogError("Failed to delete old camera", err, "camera_id", existingID)
							}
							// Remove from memory
							delete(m.cameras, existingID)
							break
						}
					}
				}
			}
		}
	}

	// Create camera
	camera := &Camera{
		ID:            discovered.ID,
		Name:          discovered.Model,
		Type:          cameraType,
		Manufacturer:  discovered.Manufacturer,
		Model:         discovered.Model,
		Enabled:       true,
		Status:        CameraStatusOffline,
		DiscoveredAt:  discovered.DiscoveredAt,
		IPAddress:     discovered.IPAddress,
		ONVIFEndpoint: discovered.ONVIFEndpoint,
		RTSPURLs:      rtspURLs,
		DevicePath:    devicePath,
		Config: CameraConfig{
			RecordingEnabled: true,
			MotionDetection:  true,
			Quality:          "medium",
			FrameRate:        15,
		},
		Capabilities: discovered.Capabilities,
	}

	// Save to state
	camState := state.CameraState{
		ID:      camera.ID,
		Name:    camera.Name,
		RTSPURL: m.getPrimaryURL(camera),
		Enabled: camera.Enabled,
		LastSeen: camera.LastSeen,
	}

	if err := m.stateMgr.SaveCamera(ctx, camState); err != nil {
		return fmt.Errorf("failed to save camera to state: %w", err)
	}

	// Store in memory
	m.cameras[camera.ID] = camera

	m.LogInfo("Registered camera",
		"id", camera.ID,
		"name", camera.Name,
		"type", camera.Type,
		"manufacturer", camera.Manufacturer,
	)

	// Publish event
	if m.GetEventBus() != nil {
		m.PublishEvent(service.EventTypeCameraRegistered, map[string]interface{}{
			"camera_id": camera.ID,
			"name":      camera.Name,
			"type":      string(camera.Type),
		})
	}

	return nil
}

// getPrimaryURL returns the primary URL/device path for a camera
func (m *Manager) getPrimaryURL(camera *Camera) string {
	if camera.Type == CameraTypeUSB {
		return camera.DevicePath
	}
	if len(camera.RTSPURLs) > 0 {
		return camera.RTSPURLs[0]
	}
	return ""
}

// GetCamera retrieves a camera by ID
func (m *Manager) GetCamera(cameraID string) (*Camera, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	camera, ok := m.cameras[cameraID]
	if !ok {
		return nil, fmt.Errorf("camera not found: %s", cameraID)
	}

	return camera, nil
}

// ListCameras lists all cameras
func (m *Manager) ListCameras(enabledOnly bool) []*Camera {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var cameras []*Camera
	for _, cam := range m.cameras {
		if !enabledOnly || cam.Enabled {
			cameras = append(cameras, cam)
		}
	}

	return cameras
}

// UpdateCameraConfig updates camera configuration
func (m *Manager) UpdateCameraConfig(ctx context.Context, cameraID string, config CameraConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	camera, ok := m.cameras[cameraID]
	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	camera.Config = config

	// Save to state
	camState := state.CameraState{
		ID:      camera.ID,
		Name:    camera.Name,
		RTSPURL: m.getPrimaryURL(camera),
		Enabled: camera.Enabled,
		LastSeen: camera.LastSeen,
	}

	if err := m.stateMgr.SaveCamera(ctx, camState); err != nil {
		return fmt.Errorf("failed to save camera config: %w", err)
	}

	m.LogInfo("Updated camera config", "camera_id", cameraID)
	return nil
}

// EnableCamera enables a camera
func (m *Manager) EnableCamera(ctx context.Context, cameraID string) error {
	return m.setCameraEnabled(ctx, cameraID, true)
}

// DisableCamera disables a camera
func (m *Manager) DisableCamera(ctx context.Context, cameraID string) error {
	return m.setCameraEnabled(ctx, cameraID, false)
}

// setCameraEnabled sets camera enabled state
func (m *Manager) setCameraEnabled(ctx context.Context, cameraID string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	camera, ok := m.cameras[cameraID]
	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	camera.Enabled = enabled

	// Save to state
	camState := state.CameraState{
		ID:      camera.ID,
		Name:    camera.Name,
		RTSPURL: m.getPrimaryURL(camera),
		Enabled: camera.Enabled,
		LastSeen: camera.LastSeen,
	}

	if err := m.stateMgr.SaveCamera(ctx, camState); err != nil {
		return fmt.Errorf("failed to save camera state: %w", err)
	}

	m.LogInfo("Camera enabled/disabled", "camera_id", cameraID, "enabled", enabled)
	return nil
}

// DeleteCamera deletes a camera
func (m *Manager) DeleteCamera(ctx context.Context, cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.cameras[cameraID]
	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	// Stop RTSP client if running
	if client, ok := m.rtspClients[cameraID]; ok {
		client.Stop(ctx)
		delete(m.rtspClients, cameraID)
	}

	// Delete from state
	if err := m.stateMgr.DeleteCamera(ctx, cameraID); err != nil {
		return fmt.Errorf("failed to delete camera from state: %w", err)
	}

	// Remove from memory
	delete(m.cameras, cameraID)

	m.LogInfo("Deleted camera", "camera_id", cameraID)
	return nil
}

// AddCamera manually adds a camera (for API use)
func (m *Manager) AddCamera(ctx context.Context, camera *Camera) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if camera already exists
	if _, ok := m.cameras[camera.ID]; ok {
		return fmt.Errorf("camera already exists: %s", camera.ID)
	}

	// Set defaults
	if camera.DiscoveredAt.IsZero() {
		camera.DiscoveredAt = time.Now()
	}
	if camera.Config.FrameRate == 0 {
		camera.Config.FrameRate = 15
	}
	if camera.Config.Quality == "" {
		camera.Config.Quality = "medium"
	}

	// Save to state
	camState := state.CameraState{
		ID:      camera.ID,
		Name:    camera.Name,
		RTSPURL: m.getPrimaryURL(camera),
		Enabled: camera.Enabled,
		LastSeen: camera.LastSeen,
	}

	if err := m.stateMgr.SaveCamera(ctx, camState); err != nil {
		return fmt.Errorf("failed to save camera to state: %w", err)
	}

	// Store in memory
	m.cameras[camera.ID] = camera

	m.LogInfo("Added camera",
		"id", camera.ID,
		"name", camera.Name,
		"type", camera.Type,
	)

	// Publish event
	if m.GetEventBus() != nil {
		m.PublishEvent(service.EventTypeCameraRegistered, map[string]interface{}{
			"camera_id": camera.ID,
			"name":      camera.Name,
			"type":      string(camera.Type),
		})
	}

	return nil
}

// UpdateCamera updates an existing camera
func (m *Manager) UpdateCamera(ctx context.Context, cameraID string, updates *Camera) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	camera, ok := m.cameras[cameraID]
	if !ok {
		return fmt.Errorf("camera not found: %s", cameraID)
	}

	// Update fields
	if updates.Name != "" {
		camera.Name = updates.Name
	}
	if updates.Enabled != camera.Enabled {
		camera.Enabled = updates.Enabled
	}
	if len(updates.RTSPURLs) > 0 {
		camera.RTSPURLs = updates.RTSPURLs
	}
	if updates.DevicePath != "" {
		camera.DevicePath = updates.DevicePath
	}
	if updates.Config.FrameRate > 0 {
		camera.Config.FrameRate = updates.Config.FrameRate
	}
	if updates.Config.Quality != "" {
		camera.Config.Quality = updates.Config.Quality
	}
	if updates.Config.Resolution != "" {
		camera.Config.Resolution = updates.Config.Resolution
	}
	camera.Config.RecordingEnabled = updates.Config.RecordingEnabled
	camera.Config.MotionDetection = updates.Config.MotionDetection

	// Save to state
	camState := state.CameraState{
		ID:      camera.ID,
		Name:    camera.Name,
		RTSPURL: m.getPrimaryURL(camera),
		Enabled: camera.Enabled,
		LastSeen: camera.LastSeen,
	}

	if err := m.stateMgr.SaveCamera(ctx, camState); err != nil {
		return fmt.Errorf("failed to save camera to state: %w", err)
	}

	m.LogInfo("Updated camera", "camera_id", cameraID)
	return nil
}

// monitorCameras monitors camera status
func (m *Manager) monitorCameras() {
	ticker := time.NewTicker(m.statusInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateCameraStatuses()
		}
	}
}

// updateCameraStatuses updates status for all cameras
func (m *Manager) updateCameraStatuses() {
	m.mu.RLock()
	cameras := make([]*Camera, 0, len(m.cameras))
	for _, cam := range m.cameras {
		cameras = append(cameras, cam)
	}
	m.mu.RUnlock()

	for _, camera := range cameras {
		if !camera.Enabled {
			camera.Status = CameraStatusOffline
			continue
		}

		// Check camera status based on type
		switch camera.Type {
		case CameraTypeRTSP, CameraTypeONVIF:
			m.updateNetworkCameraStatus(camera)
		case CameraTypeUSB:
			m.updateUSBCameraStatus(camera)
		}

		// Update last seen if online
		if camera.Status == CameraStatusOnline {
			now := time.Now()
			camera.LastSeen = &now
			ctx := context.Background()
			m.stateMgr.UpdateCameraLastSeen(ctx, camera.ID)
		}
	}
}

// updateNetworkCameraStatus updates status for network cameras
func (m *Manager) updateNetworkCameraStatus(camera *Camera) {
	// Check if RTSP client exists and is connected
	if client, ok := m.rtspClients[camera.ID]; ok {
		if client.IsConnected() {
			camera.Status = CameraStatusOnline
		} else {
			camera.Status = CameraStatusOffline
		}
	} else {
		// Check if camera is still discovered
		var discovered *DiscoveredCamera
		if m.onvifDiscovery != nil {
			discovered = m.onvifDiscovery.GetCameraByID(camera.ID)
		}
		if discovered != nil {
			camera.Status = CameraStatusOffline
		} else {
			camera.Status = CameraStatusError
		}
	}
}

// updateUSBCameraStatus updates status for USB cameras
func (m *Manager) updateUSBCameraStatus(camera *Camera) {
	// Check if device path exists
	if camera.DevicePath == "" {
		camera.Status = CameraStatusError
		return
	}

	// Check if USB camera is still discovered
	if m.usbDiscovery != nil {
		discovered := m.usbDiscovery.GetCameraByID(camera.ID)
		if discovered != nil {
			camera.Status = CameraStatusOnline
		} else {
			camera.Status = CameraStatusOffline
		}
	} else {
		camera.Status = CameraStatusUnknown
	}
}

// GetCameraStatus returns the current status of a camera
func (m *Manager) GetCameraStatus(cameraID string) (CameraStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	camera, ok := m.cameras[cameraID]
	if !ok {
		return CameraStatusUnknown, fmt.Errorf("camera not found: %s", cameraID)
	}

	return camera.Status, nil
}

// GetDiscoveredCameras returns all discovered cameras from both discovery services
func (m *Manager) GetDiscoveredCameras() []*DiscoveredCamera {
	var discovered []*DiscoveredCamera

	if m.onvifDiscovery != nil {
		discovered = append(discovered, m.onvifDiscovery.GetDiscoveredCameras()...)
	}
	if m.usbDiscovery != nil {
		discovered = append(discovered, m.usbDiscovery.GetDiscoveredCameras()...)
	}

	return discovered
}

// TriggerDiscovery triggers discovery on both discovery services
func (m *Manager) TriggerDiscovery() {
	if m.onvifDiscovery != nil {
		m.onvifDiscovery.TriggerDiscovery()
	}
	if m.usbDiscovery != nil {
		m.usbDiscovery.TriggerDiscovery()
	}
}

