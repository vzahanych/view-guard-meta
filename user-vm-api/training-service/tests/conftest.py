"""
Pytest configuration and fixtures for training service tests
"""

import os
import tempfile
import shutil
from pathlib import Path
import pytest

from config import config


@pytest.fixture
def temp_dir():
    """Create a temporary directory for tests"""
    temp_path = tempfile.mkdtemp()
    yield temp_path
    shutil.rmtree(temp_path, ignore_errors=True)


@pytest.fixture
def test_dataset_dir(temp_dir):
    """Create a test dataset directory structure"""
    dataset_dir = os.path.join(temp_dir, "test_dataset")
    os.makedirs(dataset_dir, exist_ok=True)
    
    # Create screenshots directory
    screenshots_dir = os.path.join(dataset_dir, "screenshots")
    os.makedirs(screenshots_dir, exist_ok=True)
    
    # Create metadata.json
    metadata = {
        "edge_id": "test-edge-1",
        "camera_id": "test-camera-1",
        "total_snapshots": 100,
        "label_counts": {
            "normal": 60,
            "anomaly": 40
        },
        "synced_at": "2025-01-01T00:00:00Z"
    }
    
    import json
    metadata_path = os.path.join(dataset_dir, "metadata.json")
    with open(metadata_path, "w") as f:
        json.dump(metadata, f)
    
    return dataset_dir


@pytest.fixture
def test_model_dir(temp_dir):
    """Create a test model directory structure"""
    model_dir = os.path.join(temp_dir, "test_model")
    os.makedirs(model_dir, exist_ok=True)
    
    # Create metadata.json
    metadata = {
        "model_id": "baseline-yolov8n",
        "version": "1.0",
        "model_type": "yolov8n",
        "framework": "onnx",
        "input_shape": [1, 3, 640, 640],
        "onnx_file": "model.onnx"
    }
    
    import json
    metadata_path = os.path.join(model_dir, "metadata.json")
    with open(metadata_path, "w") as f:
        json.dump(metadata, f)
    
    # Create a dummy ONNX file (just empty file for testing)
    onnx_path = os.path.join(model_dir, "model.onnx")
    with open(onnx_path, "wb") as f:
        f.write(b"dummy onnx content")
    
    return model_dir


@pytest.fixture
def mock_config(monkeypatch, temp_dir):
    """Mock configuration for tests"""
    monkeypatch.setattr(config, "DATASETS_DIR", os.path.join(temp_dir, "datasets"))
    monkeypatch.setattr(config, "MODELS_DIR", os.path.join(temp_dir, "models"))
    monkeypatch.setattr(config, "TRAINING_OUTPUT_DIR", os.path.join(temp_dir, "training"))
    monkeypatch.setattr(config, "MODEL_CATALOG_API_URL", "http://localhost:8080")
    
    # Ensure directories exist
    os.makedirs(config.DATASETS_DIR, exist_ok=True)
    os.makedirs(config.MODELS_DIR, exist_ok=True)
    os.makedirs(config.TRAINING_OUTPUT_DIR, exist_ok=True)
    
    return config

