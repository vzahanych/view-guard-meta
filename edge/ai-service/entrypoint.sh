#!/bin/sh
set -e

MODEL_NAME="${AI_MODEL_NAME:-yolov8n}"
MODEL_DIR="${AI_MODEL_DIR:-/app/models}"
MODEL_XML="$MODEL_DIR/$MODEL_NAME.xml"
MODEL_ONNX="$MODEL_DIR/$MODEL_NAME.onnx"

# Edge AI Service should NOT download raw models on startup.
# Models are received from VM via model deployment flow (Epic 2.8).
# Only check if model exists (for cases where model was already deployed).
if [ -f "$MODEL_XML" ] || [ -f "$MODEL_ONNX" ]; then
  echo "✅ Model found: $([ -f "$MODEL_XML" ] && echo "$MODEL_XML" || echo "$MODEL_ONNX")"
  echo "   Model will be loaded for inference"
else
  echo "ℹ️  No model found in $MODEL_DIR"
  echo "   Edge AI Service will start without a model"
  echo "   Models will be deployed from VM via model deployment flow (Epic 2.8)"
  echo "   Service will be ready to receive models but inference will not work until model is deployed"
fi

exec python main.py

