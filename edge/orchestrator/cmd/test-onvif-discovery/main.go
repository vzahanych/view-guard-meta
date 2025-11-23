package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func main() {
	fmt.Println("=== ONVIF Camera Discovery Test ===")
	fmt.Println("This tool will scan your home network for ONVIF cameras")
	fmt.Println("Make sure your laptop and cameras are on the same WiFi network")
	fmt.Println()

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Create discovery service
	discovery := camera.NewONVIFDiscoveryService(1*time.Minute, log)

	// Start discovery
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Println("Starting discovery service...")
	err = discovery.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}

	// Trigger immediate discovery
	fmt.Println("Triggering network scan...")
	discovery.TriggerDiscovery()

	// Wait for discovery to complete
	fmt.Println("Waiting for responses (this may take a few seconds)...")
	time.Sleep(6 * time.Second)

	// Get discovered cameras
	cameras := discovery.GetDiscoveredCameras()

	fmt.Println()
	fmt.Println("=== Discovery Results ===")
	fmt.Printf("Found %d camera(s) on network\n\n", len(cameras))

	if len(cameras) == 0 {
		fmt.Println("❌ No cameras found")
		fmt.Println()
		fmt.Println("Possible reasons:")
		fmt.Println("  - No ONVIF cameras on the network")
		fmt.Println("  - Cameras are on a different subnet")
		fmt.Println("  - Multicast is blocked by router/firewall")
		fmt.Println("  - Cameras don't support WS-Discovery")
		fmt.Println()
		fmt.Println("Trying one more scan...")
		discovery.TriggerDiscovery()
		time.Sleep(5 * time.Second)
		cameras = discovery.GetDiscoveredCameras()
		fmt.Printf("After second scan: Found %d camera(s)\n\n", len(cameras))
	}

	// Print camera details
	for i, cam := range cameras {
		fmt.Printf("--- Camera %d ---\n", i+1)
		fmt.Printf("  ID:              %s\n", cam.ID)
		fmt.Printf("  Manufacturer:    %s\n", cam.Manufacturer)
		fmt.Printf("  Model:           %s\n", cam.Model)
		fmt.Printf("  IP Address:      %s\n", cam.IPAddress)
		fmt.Printf("  ONVIF Endpoint:  %s\n", cam.ONVIFEndpoint)
		fmt.Printf("  Last Seen:       %s\n", cam.LastSeen.Format(time.RFC3339))
		fmt.Printf("  Discovered At:   %s\n", cam.DiscoveredAt.Format(time.RFC3339))
		fmt.Printf("  RTSP URLs:\n")
		if len(cam.RTSPURLs) == 0 {
			fmt.Printf("    (none detected)\n")
		} else {
			for _, url := range cam.RTSPURLs {
				fmt.Printf("    - %s\n", url)
			}
		}
		fmt.Printf("  Capabilities:\n")
		fmt.Printf("    PTZ:           %v\n", cam.Capabilities.HasPTZ)
		fmt.Printf("    Snapshot:      %v\n", cam.Capabilities.HasSnapshot)
		fmt.Printf("    Video Streams: %v\n", cam.Capabilities.HasVideoStreams)
		fmt.Println()
	}

	// Stop discovery
	err = discovery.Stop(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error stopping discovery: %v\n", err)
	}

	if len(cameras) > 0 {
		fmt.Printf("✅ SUCCESS: Found %d camera(s) on your network!\n", len(cameras))
		os.Exit(0)
	} else {
		fmt.Println("⚠️  No cameras found, but discovery service ran successfully")
		fmt.Println("   This is OK if you don't have ONVIF cameras on the network")
		os.Exit(0)
	}
}

