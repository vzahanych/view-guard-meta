package camera

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// ONVIFDiscoveryService discovers ONVIF cameras on the network
type ONVIFDiscoveryService struct {
	*service.ServiceBase
	discoveredCameras map[string]*DiscoveredCamera
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	discoveryInterval time.Duration
}

// DiscoveredCamera represents a discovered ONVIF camera
type DiscoveredCamera struct {
	ID              string
	Manufacturer    string
	Model           string
	IPAddress       string
	ONVIFEndpoint   string
	RTSPURLs        []string
	Capabilities    CameraCapabilities
	LastSeen        time.Time
	DiscoveredAt    time.Time
}

// CameraCapabilities represents camera capabilities
type CameraCapabilities struct {
	HasPTZ          bool
	HasSnapshot     bool
	HasVideoStreams bool
	StreamProfiles  []StreamProfile
}

// StreamProfile represents a video stream profile
type StreamProfile struct {
	Name        string
	Width       int
	Height      int
	FrameRate   float64
	RTSPURL     string
	Encoding    string
}

// NewONVIFDiscoveryService creates a new ONVIF discovery service
func NewONVIFDiscoveryService(discoveryInterval time.Duration, log *logger.Logger) *ONVIFDiscoveryService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &ONVIFDiscoveryService{
		ServiceBase:       service.NewServiceBase("onvif-discovery", log),
		discoveredCameras: make(map[string]*DiscoveredCamera),
		ctx:               ctx,
		cancel:            cancel,
		discoveryInterval: discoveryInterval,
	}
}

// Name returns the service name
func (s *ONVIFDiscoveryService) Name() string {
	return "onvif-discovery"
}

// Start starts the ONVIF discovery service
func (s *ONVIFDiscoveryService) Start(ctx context.Context) error {
	s.GetStatus().SetStatus(service.StatusStarting)
	s.LogInfo("Starting ONVIF discovery service")

	// Start discovery loop
	go s.discoveryLoop()

	s.GetStatus().SetStatus(service.StatusRunning)
	return nil
}

// Stop stops the ONVIF discovery service
func (s *ONVIFDiscoveryService) Stop(ctx context.Context) error {
	s.GetStatus().SetStatus(service.StatusStopping)
	s.LogInfo("Stopping ONVIF discovery service")

	s.cancel()
	s.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

// discoveryLoop runs periodic camera discovery
func (s *ONVIFDiscoveryService) discoveryLoop() {
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

// discoverCameras discovers ONVIF cameras on the network
func (s *ONVIFDiscoveryService) discoverCameras() {
	s.LogInfo("Starting ONVIF camera discovery on local network")

	// WS-Discovery probe for ONVIF devices
	devices, err := s.wsDiscoveryProbe()
	if err != nil {
		s.LogError("WS-Discovery probe failed", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Process discovered devices
	for _, device := range devices {
		camera, err := s.probeCamera(device)
		if err != nil {
			s.LogDebug("Failed to probe camera", "device", device, "error", err)
			continue
		}

		// Update or add camera
		if existing, ok := s.discoveredCameras[camera.ID]; ok {
			existing.LastSeen = time.Now()
			existing.IPAddress = camera.IPAddress
			existing.ONVIFEndpoint = camera.ONVIFEndpoint
			existing.RTSPURLs = camera.RTSPURLs
			existing.Capabilities = camera.Capabilities
		} else {
			camera.DiscoveredAt = time.Now()
			s.discoveredCameras[camera.ID] = camera
			
			// Publish discovery event
			if s.GetEventBus() != nil {
				s.PublishEvent(service.EventTypeCameraDiscovered, map[string]interface{}{
					"camera_id":    camera.ID,
					"manufacturer": camera.Manufacturer,
					"model":        camera.Model,
					"ip_address":   camera.IPAddress,
				})
			}
			
			s.LogInfo("Discovered new camera",
				"id", camera.ID,
				"manufacturer", camera.Manufacturer,
				"model", camera.Model,
				"ip", camera.IPAddress,
			)
		}
	}

	s.LogInfo("ONVIF discovery complete", "cameras_found", len(devices))
}

// wsDiscoveryProbe performs WS-Discovery probe for ONVIF devices
func (s *ONVIFDiscoveryService) wsDiscoveryProbe() ([]string, error) {
	// WS-Discovery uses UDP multicast on port 3702
	// This works on home WiFi networks as long as devices are on the same subnet
	multicastAddr := "239.255.255.250:3702"
	
	// Try to find a suitable network interface (WiFi or Ethernet)
	// On home networks, we want to use the interface connected to the local network
	localAddr, err := s.findLocalNetworkInterface()
	if err != nil {
		s.LogDebug("Could not determine local interface, using default", "error", err)
		localAddr = ":0" // Use default
	} else {
		s.LogDebug("Using network interface", "address", localAddr)
	}
	
	// Create UDP connection on local interface
	conn, err := net.ListenPacket("udp4", localAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}
	defer conn.Close()

	// Note: On home WiFi networks, multicast should work automatically
	// as long as devices are on the same subnet. The OS handles multicast
	// group membership automatically when we bind to the interface.

	// WS-Discovery Probe message
	probeMessage := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
	<s:Header>
		<a:Action s:mustUnderstand="1">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
		<a:MessageID>uuid:probe-message-id</a:MessageID>
		<a:To s:mustUnderstand="1">urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
	</s:Header>
	<s:Body>
		<d:Probe>
			<d:Types>dn:NetworkVideoTransmitter</d:Types>
		</d:Probe>
	</s:Body>
</s:Envelope>`

	// Send probe
	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	_, err = conn.WriteTo([]byte(probeMessage), addr)
	if err != nil {
		return nil, fmt.Errorf("failed to send probe: %w", err)
	}

	// Collect responses
	// On home WiFi networks, give a bit more time for responses
	devices := make([]string, 0)
	deadline := time.Now().Add(3 * time.Second) // Increased timeout for WiFi
	conn.SetReadDeadline(deadline)

	buffer := make([]byte, 4096)
	for {
		n, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			continue
		}

		// Parse response to extract device endpoint
		endpoint := s.parseProbeMatch(string(buffer[:n]))
		if endpoint != "" {
			// Avoid duplicates
			duplicate := false
			for _, existing := range devices {
				if existing == endpoint {
					duplicate = true
					break
				}
			}
			if !duplicate {
				devices = append(devices, endpoint)
				s.LogInfo("Received ONVIF probe response", "from", addr.String(), "endpoint", endpoint)
			}
		}
	}

	return devices, nil
}

// parseProbeMatch parses WS-Discovery ProbeMatch response to extract endpoint
func (s *ONVIFDiscoveryService) parseProbeMatch(xmlResponse string) string {
	// Simple XML parsing - extract XAddrs from ProbeMatch
	// In production, use proper XML parser
	start := strings.Index(xmlResponse, "<d:XAddrs>")
	if start == -1 {
		return ""
	}
	start += len("<d:XAddrs>")
	
	end := strings.Index(xmlResponse[start:], "</d:XAddrs>")
	if end == -1 {
		return ""
	}
	
	endpoint := strings.TrimSpace(xmlResponse[start : start+end])
	// Extract first URL if multiple
	if idx := strings.Index(endpoint, " "); idx != -1 {
		endpoint = endpoint[:idx]
	}
	
	return endpoint
}

// probeCamera probes a discovered device for camera information
func (s *ONVIFDiscoveryService) probeCamera(onvifEndpoint string) (*DiscoveredCamera, error) {
	// Extract IP address from endpoint
	ip := s.extractIPFromEndpoint(onvifEndpoint)
	if ip == "" {
		return nil, fmt.Errorf("failed to extract IP from endpoint: %s", onvifEndpoint)
	}

	// For PoC, create a basic camera structure
	// In production, you would make ONVIF API calls to get device info
	camera := &DiscoveredCamera{
		ID:            fmt.Sprintf("onvif-%s", ip),
		Manufacturer:  "Unknown",
		Model:         "Unknown",
		IPAddress:     ip,
		ONVIFEndpoint: onvifEndpoint,
		LastSeen:      time.Now(),
		Capabilities: CameraCapabilities{
			HasVideoStreams: true,
		},
	}

	// Try to extract RTSP URLs from ONVIF endpoint
	// In production, use ONVIF GetStreamUri
	rtspURLs := s.guessRTSPURLs(ip)
	camera.RTSPURLs = rtspURLs

	return camera, nil
}

// extractIPFromEndpoint extracts IP address from ONVIF endpoint URL
func (s *ONVIFDiscoveryService) extractIPFromEndpoint(endpoint string) string {
	// Extract IP from URL like http://192.168.1.100/onvif/device_service
	start := strings.Index(endpoint, "://")
	if start == -1 {
		return ""
	}
	start += 3
	
	end := strings.Index(endpoint[start:], "/")
	if end == -1 {
		end = len(endpoint)
	} else {
		end = start + end
	}
	
	// Remove port if present
	host := endpoint[start:end]
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	
	return host
}

// guessRTSPURLs guesses common RTSP URLs for a camera IP
func (s *ONVIFDiscoveryService) guessRTSPURLs(ip string) []string {
	// Common RTSP URL patterns
	commonPaths := []string{
		"/Streaming/Channels/101",           // Hikvision
		"/h264",                             // Generic
		"/live",                             // Generic
		"/stream1",                          // Generic
		"/videoMain",                        // Generic
		"/cam/realmonitor",                  // Dahua
		"/rtsp/videoMain",                   // Generic
	}

	urls := make([]string, 0)
	for _, path := range commonPaths {
		urls = append(urls, fmt.Sprintf("rtsp://%s%s", ip, path))
	}

	return urls
}

// GetDiscoveredCameras returns all discovered cameras
func (s *ONVIFDiscoveryService) GetDiscoveredCameras() []*DiscoveredCamera {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cameras := make([]*DiscoveredCamera, 0, len(s.discoveredCameras))
	for _, cam := range s.discoveredCameras {
		cameras = append(cameras, cam)
	}

	return cameras
}

// GetCameraByID returns a discovered camera by ID
func (s *ONVIFDiscoveryService) GetCameraByID(id string) *DiscoveredCamera {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.discoveredCameras[id]
}

// TriggerDiscovery triggers an immediate discovery scan
func (s *ONVIFDiscoveryService) TriggerDiscovery() {
	go s.discoverCameras()
}

