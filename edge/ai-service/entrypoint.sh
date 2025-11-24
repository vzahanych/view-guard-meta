#!/bin/sh
set -e

MODEL_NAME="${AI_MODEL_NAME:-yolov8n}"
MODEL_DIR="${AI_MODEL_DIR:-/app/models}"
MODEL_XML="$MODEL_DIR/$MODEL_NAME.xml"
MODEL_ONNX="$MODEL_DIR/$MODEL_NAME.onnx"

# Check if model exists (OpenVINO or ONNX)
if [ ! -f "$MODEL_XML" ] && [ ! -f "$MODEL_ONNX" ]; then
  echo "⚠️  Model not found in $MODEL_DIR, downloading for testing..."
  echo "   Note: In production, models are downloaded from remote VM"
  
  # Try to download model
  if /app/scripts/download_model.sh; then
    echo "✅ Model downloaded successfully"
  else
    echo "❌ Model download failed. Service will start but inference will not work."
    echo "   You can manually download the model or mount it as a volume."
    echo "   To retry: docker-compose exec edge-ai-service /app/scripts/download_model.sh"
  fi
else
  echo "✅ Model found: $([ -f "$MODEL_XML" ] && echo "$MODEL_XML" || echo "$MODEL_ONNX")"
fi

exec python main.py

