package cctv

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// CCTVDevicePlugin implements types.DevicePlugin for cameras.
// It adapts CCTVService to the device-agnostic DevicePlugin interface,
// enabling cameras to be discovered and managed through the IoT service.
//
// **Backward Compatibility**: The CCTVService interface remains unchanged and continues to be used
// by state manager, web gateway, AI gateway, and other services. This plugin is an additional
// layer that makes cameras compatible with the device-agnostic DevicePlugin abstraction.
//
// **Usage Pattern**:
//   - Existing code: Uses CCTVService directly (no changes required)
//   - New device-agnostic code: Uses DevicePlugin interface, can work with CCTVDevicePlugin or other plugins
//   - Device registry: Can register CCTVDevicePlugin to enable camera discovery via IoT service
type CCTVDevicePlugin struct {
	cctvService CCTVService
	logger      *zap.Logger
}

var (
	_ types.DevicePlugin = (*CCTVDevicePlugin)(nil) // Ensure CCTVDevicePlugin implements DevicePlugin interface
)

// NewCCTVDevicePlugin creates a new CCTV device plugin.
//
// Parameters:
//   - cctvService: The CCTV service instance (required)
//   - logger: Logger for plugin operations (optional, defaults to no-op logger)
//
// Returns:
//   - *CCTVDevicePlugin: A DevicePlugin implementation for cameras
func NewCCTVDevicePlugin(cctvService CCTVService, logger *zap.Logger) *CCTVDevicePlugin {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CCTVDevicePlugin{
		cctvService: cctvService,
		logger:      logger,
	}
}

// GetDeviceType returns the device type this plugin handles.
func (p *CCTVDevicePlugin) GetDeviceType() types.DeviceType {
	return types.DeviceTypeCamera
}

// GetSupportedCapabilities returns the capabilities that cameras can support.
// Note: Camera-specific capabilities (PTZ, motion detection) are determined per-camera
// based on camera capabilities. This method returns the base capabilities that all cameras support.
func (p *CCTVDevicePlugin) GetSupportedCapabilities() []types.DeviceCapability {
	return []types.DeviceCapability{
		types.DeviceCapabilityDataCapture,
		types.DeviceCapabilityDataStreaming,
		types.DeviceCapabilityVideoCapture,
		types.DeviceCapabilityVideoStreaming,
		types.DeviceCapabilityVideoRecording,
		types.DeviceCapabilitySnapshot,
		// Note: PTZ, Control, MotionDetection, EventGeneration are camera-specific
		// and will be determined per-camera based on camera capabilities
	}
}

// DiscoverDevices discovers cameras using the CCTV service.
// It triggers camera discovery and converts discovered cameras to Device interface implementations.
//
// Returns:
//   - []types.Device: List of discovered camera devices
//   - error: Error if discovery fails
func (p *CCTVDevicePlugin) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	p.logger.Debug("Discovering cameras via CCTV service")

	// Trigger discovery
	if err := p.cctvService.DiscoverCameras(ctx); err != nil {
		p.logger.Error("Failed to discover cameras", zap.Error(err))
		return nil, fmt.Errorf("failed to discover cameras: %w", err)
	}

	// Get discovered cameras
	cameras, err := p.cctvService.GetDiscoveredCameras(ctx)
	if err != nil {
		p.logger.Error("Failed to get discovered cameras", zap.Error(err))
		return nil, fmt.Errorf("failed to get discovered cameras: %w", err)
	}

	// Convert cameras to devices
	devices := make([]types.Device, 0, len(cameras))
	for _, camera := range cameras {
		device, err := NewCameraDevice(ctx, camera.ID, p.cctvService, p.logger)
		if err != nil {
			p.logger.Warn("Failed to create device for camera",
				zap.String("camera_id", camera.ID),
				zap.Error(err))
			continue // Skip this camera but continue with others
		}
		devices = append(devices, device)
	}

	p.logger.Info("Discovered cameras",
		zap.Int("count", len(devices)),
		zap.Int("total_cameras", len(cameras)))

	return devices, nil
}

// CreateDevice creates a camera device instance from metadata.
// It validates the metadata and creates a CameraDevice adapter for the camera.
//
// Parameters:
//   - ctx: Context for the operation
//   - metadata: Device metadata containing camera information
//
// Returns:
//   - types.Device: A Device interface implementation for the camera
//   - error: Error if validation fails or device creation fails
func (p *CCTVDevicePlugin) CreateDevice(ctx context.Context, metadata types.DeviceMetadata) (types.Device, error) {
	// Validate metadata
	if err := p.ValidateMetadata(metadata); err != nil {
		return nil, fmt.Errorf("invalid camera metadata: %w", err)
	}

	p.logger.Info("Creating camera device",
		zap.String("camera_id", metadata.ID),
		zap.String("name", metadata.Name))

	// Verify camera exists in CCTV service
	camera, err := p.cctvService.GetCamera(ctx, metadata.ID)
	if err != nil {
		p.logger.Error("Camera not found in CCTV service",
			zap.String("camera_id", metadata.ID),
			zap.Error(err))
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	// Create CameraDevice adapter
	device, err := NewCameraDevice(ctx, metadata.ID, p.cctvService, p.logger)
	if err != nil {
		p.logger.Error("Failed to create camera device",
			zap.String("camera_id", metadata.ID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create camera device: %w", err)
	}

	p.logger.Info("Camera device created",
		zap.String("camera_id", metadata.ID),
		zap.String("name", camera.Name))

	return device, nil
}

// ValidateMetadata validates device metadata for cameras.
// It checks that the device type is correct, required fields are present,
// and camera-specific requirements are met (RTSP URL, ONVIF endpoint, or USB device path).
//
// Parameters:
//   - metadata: Device metadata to validate
//
// Returns:
//   - error: Error if validation fails
func (p *CCTVDevicePlugin) ValidateMetadata(metadata types.DeviceMetadata) error {
	// Validate device type
	if metadata.Type != types.DeviceTypeCamera {
		return fmt.Errorf("invalid device type: expected %s, got %s", types.DeviceTypeCamera, metadata.Type)
	}

	// Validate required fields
	if metadata.ID == "" {
		return fmt.Errorf("camera ID is required")
	}

	// Validate camera-specific requirements
	// Camera must have at least one of: RTSP URL, ONVIF endpoint, or USB device path
	hasRTSP := len(metadata.Endpoints) > 0
	hasONVIF := false
	for _, endpoint := range metadata.Endpoints {
		if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
			hasONVIF = true
			break
		}
	}
	hasUSB := metadata.DevicePath != ""

	if !hasRTSP && !hasONVIF && !hasUSB {
		return fmt.Errorf("camera must have at least one of: RTSP URL, ONVIF endpoint, or USB device path")
	}

	return nil
}

