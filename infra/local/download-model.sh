#!/bin/bash
# Standalone script to download model before starting Docker

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODEL_DIR="$SCRIPT_DIR/models"
MODEL_NAME="${1:-yolov8n}"

echo "📥 Downloading YOLOv8 model: $MODEL_NAME"
echo "📁 Model directory: $MODEL_DIR"

# Create models directory
mkdir -p "$MODEL_DIR"

# Check if Python is available
if ! command -v python3 &> /dev/null; then
    echo "❌ Python 3 is required but not found"
    exit 1
fi

# Check if ultralytics is installed
if ! python3 -c "import ultralytics" 2>/dev/null; then
    echo "⚠️  ultralytics not found. Installing..."
    pip3 install ultralytics openvino[tools] --quiet
fi

# Download model
cd "$SCRIPT_DIR"
python3 -c "
import sys
sys.path.insert(0, '../../edge/ai-service')
from scripts.download_model import download_yolov8_model
from pathlib import Path
success = download_yolov8_model('$MODEL_NAME', Path('$MODEL_DIR'))
sys.exit(0 if success else 1)
"

echo "✅ Model downloaded to: $MODEL_DIR"
echo ""
echo "You can now start Docker Compose:"
echo "  docker-compose -f docker-compose.edge.yml up -d"

