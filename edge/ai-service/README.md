# Edge AI Service

Python AI inference service for the Edge Appliance. Provides object detection capabilities using OpenVINO and YOLOv8 models.

## Overview

The Edge AI Service is a FastAPI-based service that:
- Receives video frames from the Edge Orchestrator
- Performs object detection (people, vehicles, etc.) using YOLOv8 models
- Returns detection results with bounding boxes and confidence scores
- Supports both HTTP and gRPC interfaces

## Features

- **FastAPI Framework**: Modern, async-friendly HTTP/gRPC service
- **OpenVINO Integration**: Hardware-accelerated inference (CPU/iGPU)
- **Health Checks**: Liveness, readiness, and detailed health endpoints
- **Structured Logging**: JSON and text logging formats
- **Configuration Management**: YAML config with environment variable overrides

## Project Structure

```
ai-service/
├── main.py                 # Main entry point
├── ai_service/
│   ├── __init__.py        # Package initialization
│   ├── config.py          # Configuration management
│   ├── logger.py          # Logging setup
│   └── health.py          # Health check endpoints
├── requirements.txt        # Python dependencies
└── README.md              # This file
```

## Installation

### Prerequisites

- Python 3.12+
- Virtual environment (recommended)

### Setup

```bash
# Create virtual environment
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r requirements.txt
```

## Configuration

The service can be configured via:
1. YAML configuration file
2. Environment variables
3. Command-line arguments

### Configuration File

Create a `config/config.yaml` file:

```yaml
ai_service:
  log:
    level: INFO
    format: json
    output: stdout
  server:
    host: 0.0.0.0
    port: 8080
  model:
    model_dir: ./models
    model_name: yolov8n
    model_format: openvino
    device: AUTO
    confidence_threshold: 0.5
    nms_threshold: 0.4
  inference:
    batch_size: 1
    max_queue_size: 100
    timeout: 30.0
```

### Environment Variables

- `AI_LOG_LEVEL`: Log level (DEBUG, INFO, WARNING, ERROR, CRITICAL)
- `AI_LOG_FORMAT`: Log format (json, text)
- `AI_LOG_OUTPUT`: Log output (stdout or file path)
- `AI_HOST`: Server host (default: 0.0.0.0)
- `AI_PORT`: Server port (default: 8080)
- `AI_MODEL_DIR`: Model directory (default: ./models)
- `AI_MODEL_NAME`: Model name (default: yolov8n)
- `AI_DEVICE`: Inference device (CPU, GPU, AUTO)
- `AI_CONFIDENCE_THRESHOLD`: Confidence threshold (default: 0.5)

## Running

### Development

```bash
# Run with default configuration
python main.py

# Run with custom config file
python main.py --config config/config.dev.yaml

# Run with custom host/port
python main.py --host 127.0.0.1 --port 8080

# Run with debug logging
python main.py --log-level DEBUG
```

### Production

```bash
# Using uvicorn directly
uvicorn main:app --host 0.0.0.0 --port 8080

# With custom config
uvicorn main:app --host 0.0.0.0 --port 8080 --env-file .env
```

## API Endpoints

### Health Checks

- `GET /health/` - Basic health check
- `GET /health/live` - Liveness probe
- `GET /health/ready` - Readiness probe
- `GET /health/detailed` - Detailed health status

### Inference (TODO)

- `POST /inference/detect` - Object detection endpoint
- `POST /inference/batch` - Batch inference endpoint

## Health Checks

The service provides multiple health check endpoints:

- **`/health/`**: Basic health check (always returns 200 if service is running)
- **`/health/live`**: Liveness probe (returns 200 if service is alive)
- **`/health/ready`**: Readiness probe (returns 200 if ready, 503 if not ready)
- **`/health/detailed`**: Detailed health status with component checks

## Logging

The service supports structured logging in JSON or text format:

```json
{
  "timestamp": "2025-01-27T10:00:00Z",
  "level": "INFO",
  "logger": "ai_service.main",
  "message": "Starting Edge AI Service",
  "version": "dev"
}
```

## Development

### Code Style

- Follow PEP 8 style guide
- Use type hints
- Document functions and classes

### Testing

```bash
# Run tests (when implemented)
pytest

# With coverage
pytest --cov=ai_service --cov-report=html
```

## OpenVINO Setup

### Hardware Detection

Check available hardware:

```bash
python scripts/check_hardware.py
```

### Model Conversion

Convert ONNX models to OpenVINO IR format:

```bash
# Convert ONNX to OpenVINO IR
python scripts/convert_model.py model.onnx -o ./models -n yolov8n

# With FP16 compression
python scripts/convert_model.py model.onnx -o ./models --fp16

# With custom input shape
python scripts/convert_model.py model.onnx -o ./models --input-shape 1,3,640,640
```

### Runtime Configuration

The service automatically detects and configures OpenVINO runtime on startup. Device selection can be configured via:

- Config file: `model.device` (CPU, GPU, AUTO)
- Environment variable: `AI_DEVICE`

## Next Steps

- [ ] Implement model loading and management
- [ ] Implement inference pipeline
- [ ] Add gRPC endpoints
- [ ] Add unit tests
- [ ] Add integration tests

## License

Apache 2.0

