#!/usr/bin/env bash
# Setup baseline YOLOv8n model on VM side (PoC/local docker-compose)
#
# This script:
# - Ensures the python-ai-service container is running
# - Downloads YOLOv8n ONNX model into the shared `user-vm-models` volume
#   (mounted at /app/data/models in python-ai-service and user-vm-api)
# - Creates baseline model metadata.json under
#   /app/data/models/baseline/yolov8n/metadata.json
#
# Production note:
# - In production, baseline models will be synced from protected SaaS storage.
#   This script is only for the local PoC environment.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="${SCRIPT_DIR}"

BASELINE_DIR="/app/data/models/baseline"
TRAINED_DIR="/app/data/models/trained"
MODEL_CONTAINER_PATH="${BASELINE_DIR}/yolov8n"
MODEL_FILE_NAME="model.onnx"
# Try multiple possible URLs for YOLOv8n ONNX model
# Note: GitHub release URLs may change, so we try multiple sources
MODEL_URL="https://github.com/ultralytics/assets/releases/download/v8.3.0/yolov8n.onnx"
# Alternative: Use PyTorch Hub to export ONNX if direct download fails

echo -e "${BLUE}=============================================${NC}"
echo -e "${BLUE}Setup Baseline YOLOv8n Model on VM (PoC)${NC}"
echo -e "${BLUE}=============================================${NC}"

cd "${COMPOSE_DIR}"

# Ensure python-ai-service container is running (it has Python + ML stack and mounts user-vm-models)
echo -e "${YELLOW}Checking python-ai-service container...${NC}"
if ! docker compose ps python-ai-service 2>/dev/null | grep -q "Up"; then
  echo -e "${RED}✗ python-ai-service is not running${NC}"
  echo "Start the local stack first, e.g.:"
  echo "  cd infra/local && docker compose up -d"
  exit 1
fi

echo -e "${GREEN}✓ python-ai-service is running${NC}"

# Create model directory and download model inside python-ai-service container

echo -e "${YELLOW}Ensuring model directory structure exists in VM volume...${NC}"
docker compose exec -T python-ai-service bash -c "\
  mkdir -p '${BASELINE_DIR}' && \
  mkdir -p '${TRAINED_DIR}' && \
  mkdir -p '${MODEL_CONTAINER_PATH}'" >/dev/null

echo -e "${YELLOW}Checking if YOLOv8n ONNX model already exists...${NC}"
if docker compose exec -T python-ai-service bash -c "[ -f '${MODEL_CONTAINER_PATH}/${MODEL_FILE_NAME}' ]"; then
  echo -e "${GREEN}✓ Baseline model already present at ${MODEL_CONTAINER_PATH}/${MODEL_FILE_NAME}${NC}"
else
  echo -e "${YELLOW}Downloading YOLOv8n ONNX model into VM models volume...${NC}"
  docker compose exec -T python-ai-service bash -c "\
    set -euo pipefail && \
    apt-get update >/dev/null 2>&1 && apt-get install -y curl >/dev/null 2>&1 && \
    curl -L -o '${MODEL_CONTAINER_PATH}/${MODEL_FILE_NAME}' '${MODEL_URL}' && \
    echo 'Model downloaded to ${MODEL_CONTAINER_PATH}/${MODEL_FILE_NAME}' \
  "
  echo -e "${GREEN}✓ YOLOv8n model downloaded${NC}"
fi

# Create/update metadata.json for baseline model

echo -e "${YELLOW}Writing baseline model metadata.json...${NC}"
docker compose exec -T python-ai-service python - << 'PY'
import json
import os

model_dir = "/app/data/models/baseline/yolov8n"
metadata_path = os.path.join(model_dir, "metadata.json")

os.makedirs(model_dir, exist_ok=True)

metadata = {
    "model_id": "baseline-yolov8n",
    "version": "baseline-1.0",
    "camera_id": "",  # baseline, not camera-specific yet
    "model_type": "yolo",
    "input_shape": [1, 3, 640, 640],
    "latent_dim": 0,
    "threshold": 0.25,
    "training_dataset_id": "",  # will be filled when training produces a model
    "training_date": "",  # to be set after training
    "framework": "onnx",
    "onnx_file": "model.onnx",
    "preprocessing": {
        "resize": [640, 640],
        "normalize": True,
        "color_format": "BGR"
    }
}

with open(metadata_path, "w", encoding="utf-8") as f:
    json.dump(metadata, f, indent=2)

print(f"Baseline metadata written to {metadata_path}")
PY

echo -e "${GREEN}✓ Baseline model metadata created/updated${NC}"

echo -e "${BLUE}Done. Baseline YOLOv8n model is available on VM at:${NC}"
echo "  - Model file:     ${MODEL_CONTAINER_PATH}/${MODEL_FILE_NAME} (inside containers)"
echo "  - Metadata file:  ${MODEL_CONTAINER_PATH}/metadata.json (inside containers)"
echo "  - Volume:         user-vm-models"
