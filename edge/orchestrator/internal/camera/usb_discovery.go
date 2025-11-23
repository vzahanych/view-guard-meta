package camera

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// USBDiscoveryService discovers USB cameras connected to the system
type USBDiscoveryService struct {
	*service.ServiceBase
	discoveredCameras map[string]*DiscoveredCamera
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	discoveryInterval time.Duration
	videoDevPath     string
}

// NewUSBDiscoveryService creates a new USB camera discovery service
func NewUSBDiscoveryService(discoveryInterval time.Duration, videoDevPath string, log *logger.Logger) *USBDiscoveryService {
	ctx, cancel := context.WithCancel(context.Background())

	// Default to /dev if not specified
	if videoDevPath == "" {
		videoDevPath = "/dev"
	}

	return &USBDiscoveryService{
		ServiceBase:       service.NewServiceBase("usb-discovery", log),
		discoveredCameras: make(map[string]*DiscoveredCamera),
		ctx:               ctx,
		cancel:            cancel,
		discoveryInterval: discoveryInterval,
		videoDevPath:      videoDevPath,
	}
}

// Name returns the service name
func (s *USBDiscoveryService) Name() string {
	return "usb-discovery"
}

// Start starts the USB camera discovery service
func (s *USBDiscoveryService) Start(ctx context.Context) error {
	s.GetStatus().SetStatus(service.StatusStarting)
	s.LogInfo("Starting USB camera discovery service")

	// Start discovery loop
	go s.discoveryLoop()

	s.GetStatus().SetStatus(service.StatusRunning)
	return nil
}

// Stop stops the USB camera discovery service
func (s *USBDiscoveryService) Stop(ctx context.Context) error {
	s.GetStatus().SetStatus(service.StatusStopping)
	s.LogInfo("Stopping USB camera discovery service")

	s.cancel()
	s.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

// discoveryLoop runs periodic camera discovery
func (s *USBDiscoveryService) discoveryLoop() {
	ticker := time.NewTicker(s.discoveryInterval)
	defer ticker.Stop()

	// Run initial discovery
	s.discoverCameras()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.discoverCameras()
		}
	}
}

// discoverCameras discovers USB cameras on the system
func (s *USBDiscoveryService) discoverCameras() {
	s.LogInfo("Starting USB camera discovery")

	// Find all video devices
	videoDevices, err := s.findVideoDevices()
	if err != nil {
		s.LogError("Failed to find video devices", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Process discovered devices
	for _, device := range videoDevices {
		camera, err := s.probeUSBDevice(device)
		if err != nil {
			s.LogDebug("Failed to probe USB device", "device", device, "error", err)
			continue
		}

		// Update or add camera
		if existing, ok := s.discoveredCameras[camera.ID]; ok {
			existing.LastSeen = time.Now()
			existing.Capabilities = camera.Capabilities
		} else {
			camera.DiscoveredAt = time.Now()
			s.discoveredCameras[camera.ID] = camera

			// Publish discovery event
			if s.GetEventBus() != nil {
				s.PublishEvent(service.EventTypeCameraDiscovered, map[string]interface{}{
					"camera_id":    camera.ID,
					"device_path":  device,
					"manufacturer": camera.Manufacturer,
					"model":        camera.Model,
				})
			}

			s.LogInfo("Discovered new USB camera",
				"id", camera.ID,
				"device", device,
				"manufacturer", camera.Manufacturer,
				"model", camera.Model,
			)
		}
	}

	// Remove cameras that are no longer present
	for id, cam := range s.discoveredCameras {
		if !s.isDevicePresent(cam.IPAddress) { // Reusing IPAddress field for device path
			delete(s.discoveredCameras, id)
			s.LogInfo("USB camera disconnected", "id", id, "device", cam.IPAddress)
		}
	}

	s.LogInfo("USB discovery complete", "cameras_found", len(s.discoveredCameras))
}

// findVideoDevices finds all video devices in /dev
func (s *USBDiscoveryService) findVideoDevices() ([]string, error) {
	var devices []string

	// Look for /dev/video* devices
	pattern := filepath.Join(s.videoDevPath, "video*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob video devices: %w", err)
	}

	for _, match := range matches {
		// Check if it's a character device
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		// Check if it's a character device (c)
		if info.Mode()&os.ModeCharDevice != 0 {
			devices = append(devices, match)
		}
	}

	return devices, nil
}

// probeUSBDevice probes a USB device for camera information
func (s *USBDiscoveryService) probeUSBDevice(devicePath string) (*DiscoveredCamera, error) {
	// Get device information using v4l2-ctl if available, or basic detection
	camera := &DiscoveredCamera{
		ID:            fmt.Sprintf("usb-%s", filepath.Base(devicePath)),
		Manufacturer:  "Unknown",
		Model:         "USB Camera",
		IPAddress:     devicePath, // Reusing IPAddress field for device path
		ONVIFEndpoint: "",         // Not applicable for USB
		LastSeen:      time.Now(),
		Capabilities: CameraCapabilities{
			HasVideoStreams: true,
			HasSnapshot:      true,
		},
	}

	// Try to get device info using v4l2-ctl
	if info := s.getV4L2Info(devicePath); info != nil {
		camera.Manufacturer = info.Manufacturer
		camera.Model = info.Model
		camera.Capabilities = info.Capabilities
	} else {
		// Fallback: try to get info from sysfs
		if sysfsInfo := s.getSysfsInfo(devicePath); sysfsInfo != nil {
			camera.Manufacturer = sysfsInfo.Manufacturer
			camera.Model = sysfsInfo.Model
		}
	}

	// USB cameras don't have RTSP URLs, but we can note the device path
	// For FFmpeg, USB cameras are accessed via device path
	camera.RTSPURLs = []string{devicePath}

	return camera, nil
}

// v4l2DeviceInfo holds V4L2 device information
type v4l2DeviceInfo struct {
	Manufacturer string
	Model        string
	Capabilities CameraCapabilities
}

// getV4L2Info gets device information using v4l2-ctl
func (s *USBDiscoveryService) getV4L2Info(devicePath string) *v4l2DeviceInfo {
	// Check if v4l2-ctl is available
	if _, err := exec.LookPath("v4l2-ctl"); err != nil {
		return nil
	}

	info := &v4l2DeviceInfo{
		Manufacturer: "Unknown",
		Model:        "USB Camera",
		Capabilities: CameraCapabilities{
			HasVideoStreams: true,
			HasSnapshot:      true,
		},
	}

	// Get device capabilities
	cmd := exec.Command("v4l2-ctl", "--device", devicePath, "--info")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	// Parse v4l2-ctl output
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Card type") {
			// Extract model name
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.Model = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "Driver name") {
			// Sometimes driver name gives us manufacturer info
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				driver := strings.TrimSpace(parts[1])
				// Try to extract manufacturer from driver name
				if strings.Contains(strings.ToLower(driver), "uvc") {
					info.Manufacturer = "UVC"
				}
			}
		}
	}

	// Check for specific capabilities
	cmd = exec.Command("v4l2-ctl", "--device", devicePath, "--list-formats")
	formatOutput, err := cmd.Output()
	if err == nil {
		formatStr := string(formatOutput)
		if strings.Contains(formatStr, "H264") || strings.Contains(formatStr, "h264") {
			info.Capabilities.HasVideoStreams = true
		}
	}

	return info
}

// sysfsDeviceInfo holds sysfs device information
type sysfsDeviceInfo struct {
	Manufacturer string
	Model        string
}

// getSysfsInfo gets device information from sysfs
func (s *USBDiscoveryService) getSysfsInfo(devicePath string) *sysfsDeviceInfo {
	// Try to find the device in sysfs
	// /sys/class/video4linux/videoX/device points to the USB device
	deviceName := filepath.Base(devicePath)
	sysfsPath := fmt.Sprintf("/sys/class/video4linux/%s/device", deviceName)

	// Check if sysfs path exists
	if _, err := os.Stat(sysfsPath); err != nil {
		return nil
	}

	info := &sysfsDeviceInfo{
		Manufacturer: "Unknown",
		Model:        "USB Camera",
	}

	// Try to read vendor and product from sysfs
	// This is a simplified approach - full implementation would traverse USB device tree
	vendorPath := filepath.Join(sysfsPath, "../../idVendor")
	productPath := filepath.Join(sysfsPath, "../../idProduct")

	if vendor, err := os.ReadFile(vendorPath); err == nil {
		info.Manufacturer = strings.TrimSpace(string(vendor))
	}
	if product, err := os.ReadFile(productPath); err == nil {
		info.Model = strings.TrimSpace(string(product))
	}

	return info
}

// isDevicePresent checks if a device is still present
func (s *USBDiscoveryService) isDevicePresent(devicePath string) bool {
	_, err := os.Stat(devicePath)
	return err == nil
}

// GetDiscoveredCameras returns all discovered USB cameras
func (s *USBDiscoveryService) GetDiscoveredCameras() []*DiscoveredCamera {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cameras := make([]*DiscoveredCamera, 0, len(s.discoveredCameras))
	for _, cam := range s.discoveredCameras {
		cameras = append(cameras, cam)
	}

	return cameras
}

// GetCameraByID returns a discovered camera by ID
func (s *USBDiscoveryService) GetCameraByID(id string) *DiscoveredCamera {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.discoveredCameras[id]
}

// TriggerDiscovery triggers an immediate discovery scan
func (s *USBDiscoveryService) TriggerDiscovery() {
	go s.discoverCameras()
}


