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
	fmt.Println("=== USB Camera Discovery Test ===")
	fmt.Println("This tool will scan for USB cameras connected to your computer")
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

	// Create USB discovery service
	discovery := camera.NewUSBDiscoveryService(1*time.Minute, "/dev", log)

	// Start discovery
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Starting USB camera discovery...")
	err = discovery.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start discovery: %v\n", err)
		os.Exit(1)
	}

	// Trigger immediate discovery
	fmt.Println("Scanning for USB cameras...")
	discovery.TriggerDiscovery()

	// Wait for discovery to complete
	time.Sleep(2 * time.Second)

	// Get discovered cameras
	cameras := discovery.GetDiscoveredCameras()

	fmt.Println()
	fmt.Println("=== Discovery Results ===")
	fmt.Printf("Found %d USB camera(s)\n\n", len(cameras))

	if len(cameras) == 0 {
		fmt.Println("❌ No USB cameras found")
		fmt.Println()
		fmt.Println("Possible reasons:")
		fmt.Println("  - No USB cameras connected")
		fmt.Println("  - Camera not recognized by system")
		fmt.Println("  - Camera driver not loaded")
		fmt.Println("  - Insufficient permissions (try running with sudo)")
		fmt.Println()
		fmt.Println("To check manually:")
		fmt.Println("  ls -l /dev/video*")
		fmt.Println("  v4l2-ctl --list-devices")
	} else {
		// Print camera details
		for i, cam := range cameras {
			fmt.Printf("--- USB Camera %d ---\n", i+1)
			fmt.Printf("  ID:              %s\n", cam.ID)
			fmt.Printf("  Device Path:     %s\n", cam.IPAddress) // Reusing IPAddress for device path
			fmt.Printf("  Manufacturer:    %s\n", cam.Manufacturer)
			fmt.Printf("  Model:           %s\n", cam.Model)
			fmt.Printf("  Discovered At:   %s\n", cam.DiscoveredAt.Format(time.RFC3339))
			fmt.Printf("  Last Seen:       %s\n", cam.LastSeen.Format(time.RFC3339))
			fmt.Printf("  Capabilities:\n")
			fmt.Printf("    Video Streams: %v\n", cam.Capabilities.HasVideoStreams)
			fmt.Printf("    Snapshot:      %v\n", cam.Capabilities.HasSnapshot)
			fmt.Printf("    PTZ:           %v\n", cam.Capabilities.HasPTZ)
			fmt.Printf("  Access Path:     %s\n", cam.RTSPURLs[0]) // Device path for FFmpeg
			fmt.Println()
		}

		fmt.Printf("✅ SUCCESS: Found %d USB camera(s)!\n", len(cameras))
		fmt.Println()
		fmt.Println("To test with FFmpeg:")
		for _, cam := range cameras {
			fmt.Printf("  ffplay %s\n", cam.RTSPURLs[0])
		}
	}

	// Stop discovery
	err = discovery.Stop(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error stopping discovery: %v\n", err)
	}

	os.Exit(0)
}


