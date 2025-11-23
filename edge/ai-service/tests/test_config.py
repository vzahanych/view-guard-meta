"""
Unit tests for configuration management.
"""

import os
import tempfile
from pathlib import Path

import pytest
import yaml

from ai_service.config import (
    Config,
    LogConfig,
    ModelConfig,
    ServerConfig,
    InferenceConfig,
    load_config,
)


class TestLogConfig:
    """Tests for LogConfig."""
    
    def test_default_log_config(self):
        """Test default log configuration."""
        config = LogConfig()
        assert config.level == "INFO"
        assert config.format == "json"
        assert config.output == "stdout"
    
    def test_custom_log_config(self):
        """Test custom log configuration."""
        config = LogConfig(level="DEBUG", format="text", output="/tmp/log.txt")
        assert config.level == "DEBUG"
        assert config.format == "text"
        assert config.output == "/tmp/log.txt"


class TestServerConfig:
    """Tests for ServerConfig."""
    
    def test_default_server_config(self):
        """Test default server configuration."""
        config = ServerConfig()
        assert config.host == "0.0.0.0"
        assert config.port == 8080
    
    def test_custom_server_config(self):
        """Test custom server configuration."""
        config = ServerConfig(host="127.0.0.1", port=9000)
        assert config.host == "127.0.0.1"
        assert config.port == 9000


class TestModelConfig:
    """Tests for ModelConfig."""
    
    def test_default_model_config(self):
        """Test default model configuration."""
        config = ModelConfig()
        assert config.model_dir == "./models"
        assert config.model_name == "yolov8n"
        assert config.model_format == "openvino"
        assert config.device == "AUTO"
        assert config.confidence_threshold == 0.5
        assert config.nms_threshold == 0.4
    
    def test_custom_model_config(self):
        """Test custom model configuration."""
        config = ModelConfig(
            model_dir="/custom/models",
            model_name="yolov8s",
            device="GPU",
            confidence_threshold=0.7,
        )
        assert config.model_dir == "/custom/models"
        assert config.model_name == "yolov8s"
        assert config.device == "GPU"
        assert config.confidence_threshold == 0.7


class TestConfig:
    """Tests for Config."""
    
    def test_default_config(self):
        """Test default configuration."""
        config = Config()
        assert isinstance(config.log, LogConfig)
        assert isinstance(config.server, ServerConfig)
        assert isinstance(config.model, ModelConfig)
        assert isinstance(config.inference, InferenceConfig)
    
    def test_config_validation_log_level(self):
        """Test config validation for log level."""
        config = Config()
        config.log.level = "INVALID"
        
        with pytest.raises(ValueError, match="Invalid log level"):
            config.__post_init__()
    
    def test_config_validation_log_format(self):
        """Test config validation for log format."""
        config = Config()
        config.log.format = "invalid"
        
        with pytest.raises(ValueError, match="Invalid log format"):
            config.__post_init__()
    
    def test_config_validation_port(self):
        """Test config validation for port."""
        config = Config()
        config.server.port = 70000  # Invalid port
        
        with pytest.raises(ValueError, match="Invalid port"):
            config.__post_init__()
    
    def test_config_validation_confidence_threshold(self):
        """Test config validation for confidence threshold."""
        config = Config()
        config.model.confidence_threshold = 1.5  # Invalid threshold
        
        with pytest.raises(ValueError, match="Invalid confidence threshold"):
            config.__post_init__()


class TestLoadConfig:
    """Tests for load_config function."""
    
    def test_load_config_defaults(self):
        """Test loading config with defaults when no file exists."""
        # Clear any existing config file
        config = load_config(config_path=None)
        assert isinstance(config, Config)
        assert config.log.level == "INFO"
        assert config.server.port == 8080
    
    def test_load_config_from_file(self, config_file: Path):
        """Test loading config from YAML file."""
        config = load_config(str(config_file))
        assert isinstance(config, Config)
        assert config.log.level == "INFO"
        assert config.server.host == "127.0.0.1"
        assert config.server.port == 8080
    
    def test_load_config_file_not_found(self):
        """Test loading config with non-existent file."""
        with pytest.raises(FileNotFoundError):
            load_config("/nonexistent/config.yaml")
    
    def test_load_config_env_override(self, config_file: Path, monkeypatch):
        """Test environment variable overrides."""
        monkeypatch.setenv("AI_LOG_LEVEL", "DEBUG")
        monkeypatch.setenv("AI_PORT", "9000")
        monkeypatch.setenv("AI_DEVICE", "GPU")
        
        config = load_config(str(config_file))
        assert config.log.level == "DEBUG"
        assert config.server.port == 9000
        assert config.model.device == "GPU"
    
    def test_load_config_nested_structure(self, temp_dir: Path):
        """Test loading config with nested edge.ai_service structure."""
        config_path = temp_dir / "config.yaml"
        config_dict = {
            "edge": {
                "ai_service": {
                    "log": {"level": "WARNING"},
                    "server": {"port": 9090},
                },
            },
        }
        
        with open(config_path, "w") as f:
            yaml.dump(config_dict, f)
        
        config = load_config(str(config_path))
        assert config.log.level == "WARNING"
        assert config.server.port == 9090

