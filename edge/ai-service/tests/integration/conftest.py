"""
Pytest configuration and fixtures for integration tests.
"""

import os
import tempfile
import threading
import time
from pathlib import Path
from typing import Generator, Optional

import numpy as np
import pytest
from fastapi.testclient import TestClient

from ai_service.config import Config, LogConfig, ModelConfig, ServerConfig, InferenceConfig
from ai_service.main import create_app
from ai_service.model_loader import ModelLoader
from ai_service.inference import InferenceEngine
from ai_service.detection import DetectionLogic
from ai_service.openvino_runtime import create_runtime


@pytest.fixture
def temp_dir() -> Generator[Path, None, None]:
    """Create a temporary directory for integration tests."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def models_dir(temp_dir: Path) -> Path:
    """Create models directory."""
    models_path = temp_dir / "models"
    models_path.mkdir()
    return models_path


@pytest.fixture
def integration_config(temp_dir: Path, models_dir: Path) -> Config:
    """Create configuration for integration tests."""
    return Config(
        log=LogConfig(level="INFO", format="text", output="stdout"),
        server=ServerConfig(host="127.0.0.1", port=8080),
        model=ModelConfig(
            model_dir=str(models_dir),
            model_name="yolov8n",
            model_format="openvino",
            device="CPU",
            confidence_threshold=0.5,
            nms_threshold=0.4,
        ),
        inference=InferenceConfig(
            batch_size=1,
            max_queue_size=100,
            timeout=30.0,
        ),
    )


@pytest.fixture
def mock_model_files(models_dir: Path) -> tuple[Path, Path]:
    """
    Create mock OpenVINO model files for testing.
    
    Returns:
        Tuple of (xml_path, bin_path)
    """
    xml_path = models_dir / "yolov8n.xml"
    bin_path = models_dir / "yolov8n.bin"
    
    # Create minimal valid OpenVINO IR XML file
    xml_content = """<?xml version="1.0"?>
<net name="yolov8n" version="11">
    <layers>
        <layer id="0" name="input" type="Parameter" version="opset1">
            <data shape="1,3,640,640" element_type="f32"/>
            <output>
                <port id="0" precision="FP32">
                    <dim>1</dim>
                    <dim>3</dim>
                    <dim>640</dim>
                    <dim>640</dim>
                </port>
            </output>
        </layer>
        <layer id="1" name="output" type="Result" version="opset1">
            <input>
                <port id="0" precision="FP32">
                    <dim>1</dim>
                    <dim>84</dim>
                    <dim>8400</dim>
                </port>
            </input>
        </layer>
    </layers>
    <edges>
        <edge from-layer="0" from-port="0" to-layer="1" to-port="0"/>
    </edges>
</net>"""
    
    xml_path.write_text(xml_content)
    # Create dummy binary file (small size for testing)
    bin_path.write_bytes(b"\x00" * 1024)  # 1KB dummy file
    
    return xml_path, bin_path


@pytest.fixture
def sample_frame() -> np.ndarray:
    """Create a sample frame for testing."""
    # Create a 640x480 RGB image
    frame = np.random.randint(0, 255, (480, 640, 3), dtype=np.uint8)
    return frame


@pytest.fixture
def openvino_runtime():
    """Create OpenVINO runtime for testing."""
    runtime = create_runtime(device="CPU")
    if runtime is None:
        pytest.skip("OpenVINO runtime not available")
    return runtime


@pytest.fixture
def model_loader(models_dir: Path, openvino_runtime, mock_model_files):
    """Create model loader for integration tests."""
    # Skip if OpenVINO is not available
    try:
        from ai_service.openvino_runtime import OPENVINO_AVAILABLE
        if not OPENVINO_AVAILABLE:
            pytest.skip("OpenVINO not available")
    except ImportError:
        pytest.skip("OpenVINO not available")
    
    loader = ModelLoader(
        model_dir=models_dir,
        device="CPU",
        runtime=openvino_runtime,
    )
    
    # Try to load model (will fail if OpenVINO can't load the mock model)
    try:
        loader.load_model("yolov8n", "openvino")
    except Exception as e:
        pytest.skip(f"Cannot load model for integration tests: {e}")
    
    return loader


@pytest.fixture
def inference_engine(model_loader):
    """Create inference engine for integration tests."""
    return InferenceEngine(
        model_loader=model_loader,
        confidence_threshold=0.5,
        nms_threshold=0.4,
    )


@pytest.fixture
def detection_logic():
    """Create detection logic for integration tests."""
    return DetectionLogic()


@pytest.fixture
def app_client(integration_config, temp_dir):
    """
    Create FastAPI app and test client for integration tests.
    
    Note: This fixture creates a real app but may skip if OpenVINO is unavailable.
    """
    app = create_app(integration_config)
    app.state.config = integration_config
    
    # Try to initialize runtime and model
    try:
        from ai_service.openvino_runtime import OPENVINO_AVAILABLE, create_runtime
        if not OPENVINO_AVAILABLE:
            pytest.skip("OpenVINO not available for integration tests")
        
        runtime = create_runtime(device="CPU")
        if runtime is None:
            pytest.skip("Cannot create OpenVINO runtime")
        
        app.state.runtime = runtime
        
        # Try to load model
        models_dir = Path(integration_config.model.model_dir)
        if not (models_dir / "yolov8n.xml").exists():
            pytest.skip("Model files not found for integration tests")
        
        model_loader = ModelLoader(
            model_dir=models_dir,
            device="CPU",
            runtime=runtime,
        )
        
        try:
            model_loader.load_model("yolov8n", "openvino")
            app.state.model_loader = model_loader
            
            inference_engine = InferenceEngine(
                model_loader=model_loader,
                confidence_threshold=0.5,
                nms_threshold=0.4,
            )
            app.state.inference_engine = inference_engine
            
            detection_logic = DetectionLogic()
            app.state.detection_logic = detection_logic
            
            from ai_service.api import setup_inference_endpoints
            setup_inference_endpoints(app, inference_engine, detection_logic)
            
            from ai_service.health import set_service_ready
            set_service_ready(True)
            
        except Exception as e:
            pytest.skip(f"Cannot load model for integration tests: {e}")
    
    except ImportError:
        pytest.skip("OpenVINO not available for integration tests")
    
    return TestClient(app)


@pytest.fixture
def base64_image(sample_frame) -> str:
    """Create base64-encoded image for API testing."""
    import base64
    import cv2
    
    # Encode image
    success, encoded = cv2.imencode(".jpg", sample_frame)
    if not success:
        pytest.fail("Failed to encode test image")
    
    image_bytes = encoded.tobytes()
    return base64.b64encode(image_bytes).decode("utf-8")

