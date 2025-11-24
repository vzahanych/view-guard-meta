#!/bin/bash
# Test video capture from 5MP USB Camera and AI inference

set -e

CAMERA_DEVICE="/dev/video2"
AI_SERVICE_URL="${AI_SERVICE_URL:-http://edge-ai-service:8080}"
FRAME_INTERVAL="${FRAME_INTERVAL:-2}"  # seconds

echo "=== Video Capture & AI Inference Test ==="
echo "Camera device: $CAMERA_DEVICE"
echo "AI Service URL: $AI_SERVICE_URL"
echo "Frame interval: ${FRAME_INTERVAL}s"
echo ""

# Test camera access
echo "Testing camera access..."
if [ ! -c "$CAMERA_DEVICE" ]; then
    echo "❌ Camera device not accessible: $CAMERA_DEVICE"
    exit 1
fi
echo "✅ Camera device accessible"

# Test AI service
echo "Testing AI service connection..."
if ! curl -f -s "${AI_SERVICE_URL}/health" > /dev/null; then
    echo "❌ AI service not reachable: $AI_SERVICE_URL"
    exit 1
fi
echo "✅ AI service is healthy"
echo ""

# Start capturing and processing frames
echo "Starting video capture and AI inference..."
echo "Press Ctrl+C to stop"
echo ""

frame_count=0
detection_count=0

while true; do
    frame_count=$((frame_count + 1))
    echo "[Frame $frame_count] Capturing frame..."
    
    # Capture a single frame using FFmpeg
    frame_file="/tmp/frame_$$.jpg"
    if ! ffmpeg -f v4l2 -input_format mjpeg -video_size 640x480 -framerate 30 \
        -i "$CAMERA_DEVICE" -frames:v 1 -q:v 2 "$frame_file" -y 2>/dev/null; then
        echo "  ❌ Failed to capture frame"
        sleep "$FRAME_INTERVAL"
        continue
    fi
    
    # Send frame to AI service for inference
    echo "  Sending frame to AI service..."
    response=$(curl -s -X POST \
        -F "image=@$frame_file" \
        "${AI_SERVICE_URL}/api/v1/inference/file" 2>/dev/null)
    
    if [ $? -ne 0 ]; then
        echo "  ❌ Failed to run inference"
        rm -f "$frame_file"
        sleep "$FRAME_INTERVAL"
        continue
    fi
    
    # Parse detections from JSON response
    detection_count_json=$(echo "$response" | grep -o '"detection_count":[0-9]*' | cut -d: -f2 || echo "0")
    
    if [ -z "$detection_count_json" ] || [ "$detection_count_json" = "0" ]; then
        echo "  ℹ️  No detections"
    else
        detection_count=$((detection_count + 1))
        echo "  ✅ Detections found: $detection_count_json"
        # Extract class names and confidences
        echo "$response" | grep -o '"class_name":"[^"]*"' | sed 's/"class_name":"\([^"]*\)"/    - \1/' || true
        echo "$response" | grep -o '"confidence":[0-9.]*' | sed 's/"confidence":\([0-9.]*\)/      (confidence: \1)/' || true
    fi
    
    # Clean up
    rm -f "$frame_file"
    
    sleep "$FRAME_INTERVAL"
done

