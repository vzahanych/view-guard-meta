#!/usr/bin/env bash
set -euo pipefail

echo "Testing RTSP stream connection..."
echo ""

RTSP_URL="${1:-rtsp://localhost:8554/test}"

echo "Testing RTSP URL: $RTSP_URL"
echo ""

if command -v ffprobe &> /dev/null; then
    echo "Using ffprobe to test stream..."
    if ffprobe -v error -show_entries stream=codec_name,width,height -of default=noprint_wrappers=1 "$RTSP_URL" 2>&1; then
        echo ""
        echo "✓ RTSP stream is accessible"
    else
        echo ""
        echo "✗ Failed to connect to RTSP stream"
        echo ""
        echo "Make sure:"
        echo "  1. Docker Compose services are running: cd ../../infra/local && docker-compose up -d"
        echo "  2. RTSP test stream container is running"
        exit 1
    fi
else
    echo "⚠ ffprobe not found. Install FFmpeg to test RTSP streams."
    echo ""
    echo "You can test manually with:"
    echo "  ffplay $RTSP_URL"
    echo "  or"
    echo "  vlc $RTSP_URL"
fi

