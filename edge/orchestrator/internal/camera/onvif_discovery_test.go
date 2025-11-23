package camera

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// TestONVIFDiscoveryHomeNetwork is a manual test to verify ONVIF discovery on home network
// Run with: go test -v -run TestONVIFDiscoveryHomeNetwork -timeout 30s
func TestONVIFDiscoveryHomeNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Check if we should run this test (set ONVIF_TEST_NETWORK=1 to enable)
	if os.Getenv("ONVIF_TEST_NETWORK") == "" {
		t.Skip("Set ONVIF_TEST_NETWORK=1 to run network discovery test")
	}

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer log.Sync()

	t.Log("Starting ONVIF discovery test on home network...")
	t.Log("This test will scan your local network for ONVIF cameras")
	t.Log("Make sure your development laptop and cameras are on the same WiFi network")

	// Create discovery service with short interval for testing
	discovery := NewONVIFDiscoveryService(1*time.Minute, log)

	// Start discovery
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = discovery.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start discovery service: %v", err)
	}

	// Trigger immediate discovery
	t.Log("Triggering immediate discovery scan...")
	discovery.TriggerDiscovery()

	// Wait a bit for discovery to complete
	time.Sleep(5 * time.Second)

	// Get discovered cameras
	cameras := discovery.GetDiscoveredCameras()

	t.Logf("\n=== Discovery Results ===")
	t.Logf("Found %d camera(s) on network\n", len(cameras))

	if len(cameras) == 0 {
		t.Log("No cameras found. This could mean:")
		t.Log("  - No ONVIF cameras on the network")
		t.Log("  - Cameras are on a different subnet")
		t.Log("  - Multicast is blocked by router/firewall")
		t.Log("  - Cameras don't support WS-Discovery")
		t.Log("\nTrying to trigger another discovery scan...")
		
		// Try one more time
		discovery.TriggerDiscovery()
		time.Sleep(5 * time.Second)
		cameras = discovery.GetDiscoveredCameras()
		t.Logf("After second scan: Found %d camera(s)", len(cameras))
	}

	// Print details of discovered cameras
	for i, cam := range cameras {
		t.Logf("\n--- Camera %d ---", i+1)
		t.Logf("ID:              %s", cam.ID)
		t.Logf("Manufacturer:    %s", cam.Manufacturer)
		t.Logf("Model:           %s", cam.Model)
		t.Logf("IP Address:      %s", cam.IPAddress)
		t.Logf("ONVIF Endpoint:  %s", cam.ONVIFEndpoint)
		t.Logf("Last Seen:       %s", cam.LastSeen.Format(time.RFC3339))
		t.Logf("Discovered At:   %s", cam.DiscoveredAt.Format(time.RFC3339))
		t.Logf("RTSP URLs:")
		if len(cam.RTSPURLs) == 0 {
			t.Logf("  (none detected)")
		} else {
			for _, url := range cam.RTSPURLs {
				t.Logf("  - %s", url)
			}
		}
		t.Logf("Capabilities:")
		t.Logf("  PTZ:           %v", cam.Capabilities.HasPTZ)
		t.Logf("  Snapshot:      %v", cam.Capabilities.HasSnapshot)
		t.Logf("  Video Streams: %v", cam.Capabilities.HasVideoStreams)
	}

	// Stop discovery
	err = discovery.Stop(ctx)
	if err != nil {
		t.Logf("Warning: Error stopping discovery: %v", err)
	}

	// Test passes if we can at least run the discovery
	// Finding cameras is optional (depends on network setup)
	if len(cameras) > 0 {
		t.Logf("\n✅ SUCCESS: Found %d camera(s) on your network!", len(cameras))
	} else {
		t.Logf("\n⚠️  No cameras found, but discovery service ran successfully")
		t.Logf("   This is OK if you don't have ONVIF cameras on the network")
	}
}

// TestONVIFDiscoveryNetworkInterface tests network interface detection
func TestONVIFDiscoveryNetworkInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer log.Sync()

	// Create discovery service
	discovery := NewONVIFDiscoveryService(1*time.Minute, log)

	// Test network interface detection
	t.Log("Testing network interface detection...")
	localAddr, err := discovery.findLocalNetworkInterface()
	if err != nil {
		t.Logf("⚠️  Could not detect local network interface: %v", err)
		t.Log("   This is OK - discovery will use default interface")
	} else {
		t.Logf("✅ Found local network interface: %s", localAddr)
	}
}

// TestONVIFDiscoveryWSDiscoveryProbe tests the WS-Discovery probe mechanism
func TestONVIFDiscoveryWSDiscoveryProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	if os.Getenv("ONVIF_TEST_NETWORK") == "" {
		t.Skip("Set ONVIF_TEST_NETWORK=1 to run network discovery test")
	}

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer log.Sync()

	// Create discovery service
	discovery := NewONVIFDiscoveryService(1*time.Minute, log)

	t.Log("Testing WS-Discovery probe...")
	t.Log("Sending multicast probe on local network...")

	// Test WS-Discovery probe
	devices, err := discovery.wsDiscoveryProbe()
	if err != nil {
		t.Fatalf("WS-Discovery probe failed: %v", err)
	}

	t.Logf("WS-Discovery probe completed: found %d device endpoint(s)", len(devices))
	for i, device := range devices {
		t.Logf("  Device %d: %s", i+1, device)
	}

	if len(devices) == 0 {
		t.Log("\n⚠️  No devices responded to WS-Discovery probe")
		t.Log("   This could mean:")
		t.Log("   - No ONVIF devices on the network")
		t.Log("   - Devices are on a different subnet")
		t.Log("   - Multicast is blocked")
	} else {
		t.Logf("\n✅ SUCCESS: WS-Discovery probe found %d device(s)", len(devices))
	}
}

