package cctv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	iottypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
	"go.uber.org/zap"
)

// CameraDevice is an adapter that wraps CCTVService and implements the generic Device interface.
// This allows cameras to be used in device-agnostic code while maintaining backward compatibility
// with existing CCTVService consumers.
//
// **Backward Compatibility**: The CCTVService interface remains unchanged and continues to be used
// by state manager, web gateway, AI gateway, and other services. CameraDevice is an additional
// layer that makes cameras compatible with the Device abstraction.
//
// **Usage Pattern**:
//   - Existing code: Uses CCTVService directly (no changes required)
//   - New device-agnostic code: Uses Device interface, can work with CameraDevice or other device types
//   - Device registry: Can register CameraDevice instances alongside other device types
type CameraDevice struct {
	// cameraID is the ID of the camera this device represents
	cameraID string

	// cctvService is the underlying CCTV service that handles all camera operations
	cctvService CCTVService

	// logger for device operations
	logger *zap.Logger

	// activeStreams tracks active data streams (for video streaming capability)
	activeStreams map[string]chan *iottypes.DeviceData
	streamsMu     sync.RWMutex

	// metadata cache (updated when camera metadata changes)
	metadataCache *iottypes.DeviceMetadata
	metadataMu    sync.RWMutex
}

var (
	_ iottypes.Device = (*CameraDevice)(nil) // Ensure CameraDevice implements Device interface
)

// NewCameraDevice creates a new CameraDevice adapter for a specific camera.
// The adapter wraps the CCTVService and provides Device interface implementation.
//
// **Important**: This does NOT replace CCTVService. Existing code continues to use CCTVService
// directly. This adapter is for new device-agnostic code that needs to work with cameras
// through the Device interface.
//
// Parameters:
//   - cameraID: The ID of the camera to adapt
//   - cctvService: The CCTV service instance (shared across all CameraDevice instances)
//   - logger: Logger for device operations
//
// Returns:
//   - *CameraDevice: A Device interface implementation for the camera
//   - error: Error if camera cannot be found or initialized
func NewCameraDevice(ctx context.Context, cameraID string, cctvService CCTVService, logger *zap.Logger) (*CameraDevice, error) {
	if cctvService == nil {
		return nil, fmt.Errorf("CCTV service cannot be nil")
	}
	if cameraID == "" {
		return nil, fmt.Errorf("camera ID cannot be empty")
	}

	// Verify camera exists
	camera, err := cctvService.GetCamera(ctx, cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to get camera %s: %w", cameraID, err)
	}

	device := &CameraDevice{
		cameraID:      cameraID,
		cctvService:   cctvService,
		logger:        logger,
		activeStreams: make(map[string]chan *iottypes.DeviceData),
	}

	// Initialize metadata from camera
	device.updateMetadataFromCamera(camera)

	return device, nil
}

// updateMetadataFromCamera updates the device metadata cache from camera information
func (d *CameraDevice) updateMetadataFromCamera(camera *types.Camera) {
	d.metadataMu.Lock()
	defer d.metadataMu.Unlock()

	// Map camera type to device type
	deviceType := iottypes.DeviceTypeCamera

	// Map camera status to device status
	var deviceStatus iottypes.DeviceStatus
	switch camera.Status {
	case types.CameraStatusOnline:
		deviceStatus = iottypes.DeviceStatusOnline
	case types.CameraStatusOffline:
		deviceStatus = iottypes.DeviceStatusOffline
	case types.CameraStatusConnecting:
		deviceStatus = iottypes.DeviceStatusConnecting
	case types.CameraStatusError:
		deviceStatus = iottypes.DeviceStatusError
	default:
		deviceStatus = iottypes.DeviceStatusUnknown
	}

	// Build capabilities from camera capabilities
	capabilities := d.buildCapabilitiesFromCamera(camera)

	// Build endpoints list
	endpoints := make([]string, 0)
	if len(camera.RTSPURLs) > 0 {
		endpoints = append(endpoints, camera.RTSPURLs...)
	}
	if camera.ONVIFEndpoint != "" {
		endpoints = append(endpoints, camera.ONVIFEndpoint)
	}

	// Build device path
	devicePath := ""
	if camera.Type == types.CameraTypeUSB {
		devicePath = camera.DevicePath
	}

	// Build config map from camera config
	config := make(map[string]interface{})
	config["recording_enabled"] = camera.Config.RecordingEnabled
	config["motion_detection"] = camera.Config.MotionDetection
	config["quality"] = camera.Config.Quality
	config["frame_rate"] = camera.Config.FrameRate
	config["resolution"] = camera.Config.Resolution

	d.metadataCache = &iottypes.DeviceMetadata{
		ID:           camera.ID,
		Name:         camera.Name,
		Type:         deviceType,
		Manufacturer: camera.Manufacturer,
		Model:        camera.Model,
		Enabled:      camera.Enabled,
		Status:       deviceStatus,
		LastSeen:     camera.LastSeen,
		DiscoveredAt: camera.DiscoveredAt,
		Capabilities: capabilities,
		Config:       config,
		IPAddress:    camera.IPAddress,
		Endpoints:    endpoints,
		DevicePath:   devicePath,
	}
}

// buildCapabilitiesFromCamera maps camera capabilities to device capabilities
func (d *CameraDevice) buildCapabilitiesFromCamera(camera *types.Camera) iottypes.DeviceCapabilities {
	capabilities := make(iottypes.DeviceCapabilities)

	// All cameras support data capture (frames)
	capabilities.Add(iottypes.DeviceCapabilityDataCapture)

	// Video-specific capabilities
	capabilities.Add(iottypes.DeviceCapabilityVideoCapture)
	capabilities.Add(iottypes.DeviceCapabilityVideoStreaming) // MJPEG streaming
	capabilities.Add(iottypes.DeviceCapabilityVideoRecording) // Clip recording
	capabilities.Add(iottypes.DeviceCapabilitySnapshot)       // Screenshot capture

	// Data streaming (via MJPEG)
	capabilities.Add(iottypes.DeviceCapabilityDataStreaming)

	// PTZ capability
	if camera.Capabilities.HasPTZ {
		capabilities.Add(iottypes.DeviceCapabilityPTZ)
		capabilities.Add(iottypes.DeviceCapabilityControl) // PTZ requires control capability
	}

	// Motion detection
	if camera.Config.MotionDetection {
		capabilities.Add(iottypes.DeviceCapabilityMotionDetection)
	}

	// Event generation (cameras can generate events via AI detection)
	capabilities.Add(iottypes.DeviceCapabilityEventGeneration)

	return capabilities
}

// Start initializes the camera device and begins operation.
// For cameras, this is typically a no-op as the CCTV service manages camera lifecycle.
func (d *CameraDevice) Start(ctx context.Context) error {
	// Camera lifecycle is managed by CCTVService, so this is typically a no-op
	// However, we can refresh metadata to ensure it's up to date
	camera, err := d.cctvService.GetCamera(ctx, d.cameraID)
	if err != nil {
		return fmt.Errorf("failed to refresh camera metadata: %w", err)
	}
	d.updateMetadataFromCamera(camera)
	return nil
}

// Stop stops the camera device and releases resources.
// Stops any active data streams.
func (d *CameraDevice) Stop(ctx context.Context) error {
	// Stop all active streams
	d.streamsMu.Lock()
	for streamID := range d.activeStreams {
		delete(d.activeStreams, streamID)
	}
	d.streamsMu.Unlock()

	// Camera lifecycle is managed by CCTVService, so this is typically a no-op
	return nil
}

// GetID returns the camera ID
func (d *CameraDevice) GetID() string {
	return d.cameraID
}

// GetMetadata returns device metadata for the camera
func (d *CameraDevice) GetMetadata() iottypes.DeviceMetadata {
	d.metadataMu.RLock()
	defer d.metadataMu.RUnlock()

	// Return a copy to prevent external modification
	if d.metadataCache == nil {
		return iottypes.DeviceMetadata{}
	}

	metadata := *d.metadataCache
	return metadata
}

// UpdateMetadata updates camera metadata by updating the underlying camera
func (d *CameraDevice) UpdateMetadata(ctx context.Context, updates *iottypes.DeviceMetadataUpdate) error {
	// Convert DeviceMetadataUpdate to CameraUpdate
	cameraUpdate := &types.CameraUpdate{}

	if updates.Name != nil {
		cameraUpdate.Name = updates.Name
	}
	if updates.Enabled != nil {
		cameraUpdate.Enabled = updates.Enabled
	}

	// Update config if provided
	if updates.Config != nil {
		// Map device config to camera config
		if recordingEnabled, ok := updates.Config["recording_enabled"].(bool); ok {
			cameraUpdate.Config = &types.CameraConfig{
				RecordingEnabled: recordingEnabled,
			}
		}
		if motionDetection, ok := updates.Config["motion_detection"].(bool); ok {
			if cameraUpdate.Config == nil {
				cameraUpdate.Config = &types.CameraConfig{}
			}
			cameraUpdate.Config.MotionDetection = motionDetection
		}
	}

	// Update camera via CCTV service
	err := d.cctvService.UpdateCamera(ctx, d.cameraID, cameraUpdate)
	if err != nil {
		return fmt.Errorf("failed to update camera: %w", err)
	}

	// Refresh metadata cache
	camera, err := d.cctvService.GetCamera(ctx, d.cameraID)
	if err != nil {
		return fmt.Errorf("failed to refresh camera metadata: %w", err)
	}
	d.updateMetadataFromCamera(camera)

	return nil
}

// Enable enables the camera
func (d *CameraDevice) Enable(ctx context.Context) error {
	return d.cctvService.EnableCamera(ctx, d.cameraID)
}

// Disable disables the camera
func (d *CameraDevice) Disable(ctx context.Context) error {
	return d.cctvService.DisableCamera(ctx, d.cameraID)
}

// IsEnabled returns whether the camera is enabled
func (d *CameraDevice) IsEnabled() bool {
	d.metadataMu.RLock()
	defer d.metadataMu.RUnlock()

	if d.metadataCache == nil {
		return false
	}
	return d.metadataCache.Enabled
}

// GetStatus returns the current operational status of the camera
func (d *CameraDevice) GetStatus() iottypes.DeviceStatus {
	d.metadataMu.RLock()
	defer d.metadataMu.RUnlock()

	if d.metadataCache == nil {
		return iottypes.DeviceStatusUnknown
	}
	return d.metadataCache.Status
}

// HasCapability checks if the camera supports a specific capability
func (d *CameraDevice) HasCapability(capability iottypes.DeviceCapability) bool {
	d.metadataMu.RLock()
	defer d.metadataMu.RUnlock()

	if d.metadataCache == nil {
		return false
	}
	return d.metadataCache.Capabilities.Has(capability)
}

// GetCapabilities returns all capabilities supported by the camera
func (d *CameraDevice) GetCapabilities() iottypes.DeviceCapabilities {
	d.metadataMu.RLock()
	defer d.metadataMu.RUnlock()

	if d.metadataCache == nil {
		return make(iottypes.DeviceCapabilities)
	}

	// Return a copy to prevent external modification
	capabilities := make(iottypes.DeviceCapabilities)
	for cap, val := range d.metadataCache.Capabilities {
		capabilities[cap] = val
	}
	return capabilities
}

// CaptureData captures a frame from the camera
// Requires: DeviceCapabilityDataCapture or DeviceCapabilityVideoCapture
func (d *CameraDevice) CaptureData(ctx context.Context) (*iottypes.DeviceData, error) {
	if !d.HasCapability(iottypes.DeviceCapabilityVideoCapture) {
		return nil, fmt.Errorf("camera does not support video capture")
	}

	// Capture frame using CCTV service
	frame, err := d.cctvService.CaptureFrame(ctx, d.cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to capture frame: %w", err)
	}

	// Convert Frame to DeviceData
	deviceData := &iottypes.DeviceData{
		DeviceID:  d.cameraID,
		Timestamp: frame.Timestamp,
		DataType:  iottypes.DeviceDataTypeVideoFrame,
		Data:      frame.Data,
		Width:     frame.Width,
		Height:    frame.Height,
		Format:    "jpeg", // CCTV frames are JPEG-encoded
		Metadata: map[string]interface{}{
			"camera_id": frame.CameraID,
		},
	}

	return deviceData, nil
}

// StartDataStream starts an MJPEG stream from the camera
// Requires: DeviceCapabilityDataStreaming or DeviceCapabilityVideoStreaming
func (d *CameraDevice) StartDataStream(ctx context.Context) (<-chan *iottypes.DeviceData, error) {
	if !d.HasCapability(iottypes.DeviceCapabilityVideoStreaming) {
		return nil, fmt.Errorf("camera does not support video streaming")
	}

	// Start MJPEG stream using CCTV service
	mjpegStream, err := d.cctvService.StartMJPEGStream(ctx, d.cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to start MJPEG stream: %w", err)
	}

	// Create device data channel
	deviceDataChan := make(chan *iottypes.DeviceData, 10)

	// Start goroutine to convert MJPEG frames to DeviceData
	streamID := fmt.Sprintf("mjpeg_%s_%d", d.cameraID, time.Now().Unix())
	d.streamsMu.Lock()
	d.activeStreams[streamID] = deviceDataChan
	d.streamsMu.Unlock()

	go func() {
		defer close(deviceDataChan)
		defer func() {
			d.streamsMu.Lock()
			delete(d.activeStreams, streamID)
			d.streamsMu.Unlock()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-mjpegStream.Done:
				return
			case frameData, ok := <-mjpegStream.FrameChan:
				if !ok {
					return
				}

				deviceData := &iottypes.DeviceData{
					DeviceID:  d.cameraID,
					Timestamp: time.Now(),
					DataType:  iottypes.DeviceDataTypeVideoFrame,
					Data:      frameData,
					Format:    "jpeg",
					Metadata: map[string]interface{}{
						"stream_id": streamID,
					},
				}

				select {
				case deviceDataChan <- deviceData:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return deviceDataChan, nil
}

// StopDataStream stops an active data stream
// Requires: DeviceCapabilityDataStreaming
func (d *CameraDevice) StopDataStream(ctx context.Context) error {
	if !d.HasCapability(iottypes.DeviceCapabilityVideoStreaming) {
		return fmt.Errorf("camera does not support video streaming")
	}

	// Stop MJPEG stream using CCTV service
	err := d.cctvService.StopMJPEGStream(ctx, d.cameraID)
	if err != nil {
		return fmt.Errorf("failed to stop MJPEG stream: %w", err)
	}

	// Clean up active streams
	d.streamsMu.Lock()
	for streamID := range d.activeStreams {
		delete(d.activeStreams, streamID)
	}
	d.streamsMu.Unlock()

	return nil
}

// ReadSensor is not supported for cameras (cameras don't have sensors)
// Requires: DeviceCapabilitySensorReadings
func (d *CameraDevice) ReadSensor(ctx context.Context, sensorType string) (*iottypes.SensorReading, error) {
	return nil, fmt.Errorf("cameras do not support sensor readings")
}

// ReadAllSensors is not supported for cameras
// Requires: DeviceCapabilitySensorReadings
func (d *CameraDevice) ReadAllSensors(ctx context.Context) (map[string]*iottypes.SensorReading, error) {
	return nil, fmt.Errorf("cameras do not support sensor readings")
}

// ExecuteCommand executes a control command on the camera (e.g., PTZ movement)
// Requires: DeviceCapabilityControl
func (d *CameraDevice) ExecuteCommand(ctx context.Context, command iottypes.DeviceCommand) error {
	if !d.HasCapability(iottypes.DeviceCapabilityControl) {
		return fmt.Errorf("camera does not support control commands")
	}

	// Map device command to camera-specific operation
	switch command.CommandType {
	case "ptz_move":
		// PTZ movement command
		// Parameters: direction (up/down/left/right), speed (optional)
		// Note: This would require extending CCTVService with PTZ methods
		// For now, return not implemented
		return fmt.Errorf("PTZ commands not yet implemented in CCTV service")
	case "set_brightness":
		// Brightness adjustment
		// Note: This would require extending CCTVService with configuration methods
		return fmt.Errorf("brightness adjustment not yet implemented in CCTV service")
	default:
		return fmt.Errorf("unknown command type: %s", command.CommandType)
	}
}

// GetAvailableCommands returns list of commands supported by the camera
// Requires: DeviceCapabilityControl
func (d *CameraDevice) GetAvailableCommands(ctx context.Context) ([]iottypes.DeviceCommand, error) {
	if !d.HasCapability(iottypes.DeviceCapabilityControl) {
		return nil, fmt.Errorf("camera does not support control commands")
	}

	commands := make([]iottypes.DeviceCommand, 0)

	// Add PTZ commands if camera supports PTZ
	if d.HasCapability(iottypes.DeviceCapabilityPTZ) {
		commands = append(commands, iottypes.DeviceCommand{
			CommandType: "ptz_move",
			Parameters: map[string]interface{}{
				"description": "Move camera PTZ (pan/tilt/zoom)",
				"parameters": map[string]interface{}{
					"direction": "string (up/down/left/right/zoom_in/zoom_out)",
					"speed":     "int (optional, 1-10)",
				},
			},
		})
	}

	return commands, nil
}

// RefreshMetadata refreshes the device metadata from the underlying camera
// This should be called when camera metadata changes externally
func (d *CameraDevice) RefreshMetadata(ctx context.Context) error {
	camera, err := d.cctvService.GetCamera(ctx, d.cameraID)
	if err != nil {
		return fmt.Errorf("failed to refresh camera metadata: %w", err)
	}
	d.updateMetadataFromCamera(camera)
	return nil
}

// GetCCTVService returns the underlying CCTV service (for advanced operations)
// This allows code that needs camera-specific features to access the full CCTVService interface
func (d *CameraDevice) GetCCTVService() CCTVService {
	return d.cctvService
}
