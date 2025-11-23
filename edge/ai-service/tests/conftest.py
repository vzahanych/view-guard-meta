"""
Pytest configuration and shared fixtures.
"""

import os
import tempfile
from pathlib import Path
from typing import Generator

import pytest

from ai_service.config import Config, LogConfig, ModelConfig, ServerConfig, InferenceConfig


@pytest.fixture
def temp_dir() -> Generator[Path, None, None]:
    """Create a temporary directory for tests."""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def sample_config() -> Config:
    """Create a sample configuration for testing."""
    return Config(
        log=LogConfig(level="INFO", format="text", output="stdout"),
        server=ServerConfig(host="127.0.0.1", port=8080),
        model=ModelConfig(
            model_dir="./models",
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
def config_file(temp_dir: Path, sample_config: Config) -> Path:
    """Create a temporary config file."""
    import yaml
    
    config_path = temp_dir / "config.yaml"
    config_dict = {
        "ai_service": {
            "log": {
                "level": sample_config.log.level,
                "format": sample_config.log.format,
                "output": sample_config.log.output,
            },
            "server": {
                "host": sample_config.server.host,
                "port": sample_config.server.port,
            },
            "model": {
                "model_dir": sample_config.model.model_dir,
                "model_name": sample_config.model.model_name,
                "model_format": sample_config.model.model_format,
                "device": sample_config.model.device,
                "confidence_threshold": sample_config.model.confidence_threshold,
                "nms_threshold": sample_config.model.nms_threshold,
            },
            "inference": {
                "batch_size": sample_config.inference.batch_size,
                "max_queue_size": sample_config.inference.max_queue_size,
                "timeout": sample_config.inference.timeout,
            },
        },
    }
    
    with open(config_path, "w") as f:
        yaml.dump(config_dict, f)
    
    return config_path


@pytest.fixture
def mock_openvino_available(monkeypatch):
    """Mock OpenVINO as available."""
    monkeypatch.setattr("ai_service.openvino_runtime.OPENVINO_AVAILABLE", True)
    monkeypatch.setattr("ai_service.model_converter.OPENVINO_TOOLS_AVAILABLE", True)


@pytest.fixture
def mock_openvino_unavailable(monkeypatch):
    """Mock OpenVINO as unavailable."""
    monkeypatch.setattr("ai_service.openvino_runtime.OPENVINO_AVAILABLE", False)
    monkeypatch.setattr("ai_service.model_converter.OPENVINO_TOOLS_AVAILABLE", False)

