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
	fmt.Println("=== Complete Camera Discovery Test ===")
	fmt.Println("This tool will scan for both USB cameras and ONVIF network cameras")
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test USB cameras
	fmt.Println("=== USB Camera Discovery ===")
	usbDiscovery := camera.NewUSBDiscoveryService(1*time.Minute, "/dev", log)
	err = usbDiscovery.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start USB discovery: %v\n", err)
	} else {
		usbDiscovery.TriggerDiscovery()
		time.Sleep(2 * time.Second)
		usbCameras := usbDiscovery.GetDiscoveredCameras()
		fmt.Printf("Found %d USB camera(s)\n", len(usbCameras))
		for i, cam := range usbCameras {
			fmt.Printf("  %d. %s (%s) at %s\n", i+1, cam.Model, cam.Manufacturer, cam.IPAddress)
		}
		usbDiscovery.Stop(ctx)
	}

	fmt.Println()

	// Test ONVIF cameras
	fmt.Println("=== ONVIF Network Camera Discovery ===")
	fmt.Println("Scanning local network for ONVIF cameras...")
	onvifDiscovery := camera.NewONVIFDiscoveryService(1*time.Minute, log)
	err = onvifDiscovery.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start ONVIF discovery: %v\n", err)
	} else {
		onvifDiscovery.TriggerDiscovery()
		time.Sleep(6 * time.Second)
		onvifCameras := onvifDiscovery.GetDiscoveredCameras()
		fmt.Printf("Found %d ONVIF camera(s)\n", len(onvifCameras))
		for i, cam := range onvifCameras {
			fmt.Printf("  %d. %s (%s) at %s\n", i+1, cam.Model, cam.Manufacturer, cam.IPAddress)
			if len(cam.RTSPURLs) > 0 {
				fmt.Printf("     RTSP: %s\n", cam.RTSPURLs[0])
			}
		}
		onvifDiscovery.Stop(ctx)
	}

	fmt.Println()
	fmt.Println("=== Summary ===")
	totalUSB := len(usbDiscovery.GetDiscoveredCameras())
	totalONVIF := len(onvifDiscovery.GetDiscoveredCameras())
	total := totalUSB + totalONVIF
	fmt.Printf("Total cameras found: %d (%d USB, %d ONVIF)\n", total, totalUSB, totalONVIF)

	if total == 0 {
		fmt.Println()
		fmt.Println("No cameras found. This is OK if:")
		fmt.Println("  - No USB cameras are connected")
		fmt.Println("  - No ONVIF cameras are on the network")
		fmt.Println("  - Cameras are on a different network")
	} else {
		fmt.Println()
		fmt.Println("✅ Camera discovery is working!")
	}

	os.Exit(0)
}


