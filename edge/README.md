# Edge Appliance

Edge Appliance software for The Private AI Guardian - local video processing, AI inference, and privacy-first security.

## Overview

The Edge Appliance runs on a Mini PC at the customer's premises and handles:
- Camera discovery and RTSP/ONVIF client
- Video processing with FFmpeg
- AI inference service (OpenVINO/YOLO)
- Local clip storage and retention
- WireGuard client for secure communication
- Event generation and queueing

## Directory Structure

- `orchestrator/` - Go main orchestrator service
- `ai-service/` - Python AI inference service
- `shared/` - Shared Go libraries
- `config/` - Configuration files and examples
- `scripts/` - Build and deployment scripts

## Dependencies

- Imports `crypto/go` from the meta repository for encryption
- Imports `proto/go` from the meta repository for gRPC proto stubs

## License

Apache 2.0

