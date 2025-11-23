# Testing ONVIF Camera Discovery on Home Network

This guide explains how to test ONVIF camera discovery from your development laptop on your home network.

## Quick Start

The easiest way to test ONVIF discovery:

```bash
cd edge/orchestrator
go build -o bin/test-onvif-discovery ./cmd/test-onvif-discovery
./bin/test-onvif-discovery
```

Or use the test script:

```bash
cd edge/orchestrator
./scripts/test-onvif-discovery.sh
```

## What the Test Does

1. **Scans your local network** for ONVIF cameras using WS-Discovery
2. **Discovers cameras** on the same WiFi network
3. **Displays camera information** including:
   - IP address
   - Manufacturer and model
   - ONVIF endpoint
   - Detected RTSP URLs
   - Camera capabilities

## Requirements

- ✅ Your development laptop must be on the same WiFi network as the cameras
- ✅ Cameras must support ONVIF and WS-Discovery protocol
- ✅ Network must allow multicast traffic (most home routers do by default)
- ✅ No firewall blocking UDP port 3702 (WS-Discovery)

## Expected Output

### If cameras are found:

```
=== Discovery Results ===
Found 2 camera(s) on network

--- Camera 1 ---
  ID:              onvif-192.168.1.100
  Manufacturer:    Hikvision
  Model:           DS-2CD2342WD-I
  IP Address:      192.168.1.100
  ONVIF Endpoint:  http://192.168.1.100/onvif/device_service
  RTSP URLs:
    - rtsp://192.168.1.100/Streaming/Channels/101
    - rtsp://192.168.1.100/h264
  ...

✅ SUCCESS: Found 2 camera(s) on your network!
```

### If no cameras are found:

```
=== Discovery Results ===
Found 0 camera(s) on network

❌ No cameras found

Possible reasons:
  - No ONVIF cameras on the network
  - Cameras are on a different subnet
  - Multicast is blocked by router/firewall
  - Cameras don't support WS-Discovery
```

## Alternative: Using Go Tests

You can also run the Go test directly:

```bash
cd edge/orchestrator
ONVIF_TEST_NETWORK=1 go test -v -run TestONVIFDiscoveryHomeNetwork -timeout 30s ./internal/camera
```

This runs the same discovery test but as part of the Go test suite.

## Troubleshooting

### No cameras found

1. **Check network connectivity**: Ensure your laptop and cameras are on the same WiFi network
2. **Check camera ONVIF support**: Not all IP cameras support ONVIF. Check your camera's documentation
3. **Check firewall**: Some firewalls block multicast. Try temporarily disabling firewall
4. **Check router settings**: Some routers have "AP Isolation" or "Client Isolation" that blocks device-to-device communication
5. **Try manual RTSP connection**: If ONVIF discovery doesn't work, you can still use manual RTSP URLs

### Network interface issues

The test automatically detects your network interface. If you have issues:

1. Check that you're connected to WiFi (not VPN)
2. Ensure you have a private IP address (192.168.x.x, 10.x.x.x, or 172.16-31.x.x)
3. Check network interface with: `ip addr` or `ifconfig`

### Multicast issues

If multicast is blocked:

1. Check router settings for "Multicast" or "IGMP" settings
2. Some routers require enabling "IGMP Snooping"
3. Try connecting both devices via Ethernet instead of WiFi

## Next Steps

Once cameras are discovered:

1. **Test RTSP connection**: Use the discovered RTSP URLs to test video streaming
2. **Integrate with orchestrator**: The discovery service can be integrated into the main orchestrator
3. **Automatic connection**: Set up automatic connection to discovered cameras

## Manual RTSP Testing

If ONVIF discovery doesn't work, you can still test RTSP connections manually:

```bash
# Test RTSP stream with ffplay (if installed)
ffplay rtsp://192.168.1.100/Streaming/Channels/101

# Or use the RTSP client test
cd edge/orchestrator
go test -v -run TestRTSPClient ./internal/camera
```

