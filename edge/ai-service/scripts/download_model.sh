#!/bin/bash
# Download YOLOv8 model script for Docker

set -e

MODEL_NAME="${AI_MODEL_NAME:-yolov8n}"
MODEL_DIR="${AI_MODEL_DIR:-/app/models}"

echo "📥 Downloading YOLOv8 model: $MODEL_NAME"
echo "📁 Model directory: $MODEL_DIR"

# Create models directory
mkdir -p "$MODEL_DIR"

# Run Python script to download and convert model
cd /app
python scripts/download_model.py \
    --model-name "$MODEL_NAME" \
    --model-dir "$MODEL_DIR"

echo "✅ Model download complete!"

