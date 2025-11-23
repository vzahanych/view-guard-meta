"""
Configuration management for Edge AI Service.

Handles loading and validation of configuration from YAML files and environment variables.
"""

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

import yaml
from dotenv import load_dotenv


@dataclass
class LogConfig:
    """Logging configuration."""
    level: str = "INFO"
    format: str = "json"  # "json" or "text"
    output: str = "stdout"  # "stdout" or file path


@dataclass
class ServerConfig:
    """Server configuration."""
    host: str = "0.0.0.0"
    port: int = 8080


@dataclass
class ModelConfig:
    """Model configuration."""
    model_dir: str = "./models"
    model_name: str = "yolov8n"
    model_format: str = "openvino"  # "openvino" or "onnx"
    device: str = "AUTO"  # "CPU", "GPU", "AUTO"
    confidence_threshold: float = 0.5
    nms_threshold: float = 0.4


@dataclass
class InferenceConfig:
    """Inference configuration."""
    batch_size: int = 1
    max_queue_size: int = 100
    timeout: float = 30.0


@dataclass
class Config:
    """Application configuration."""
    log: LogConfig = field(default_factory=LogConfig)
    server: ServerConfig = field(default_factory=ServerConfig)
    model: ModelConfig = field(default_factory=ModelConfig)
    inference: InferenceConfig = field(default_factory=InferenceConfig)
    
    def __post_init__(self):
        """Validate configuration after initialization."""
        # Validate log level
        valid_levels = ["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"]
        if self.log.level.upper() not in valid_levels:
            raise ValueError(f"Invalid log level: {self.log.level}")
        
        # Validate log format
        if self.log.format not in ["json", "text"]:
            raise ValueError(f"Invalid log format: {self.log.format}")
        
        # Validate port
        if not (1 <= self.server.port <= 65535):
            raise ValueError(f"Invalid port: {self.server.port}")
        
        # Validate confidence threshold
        if not (0.0 <= self.model.confidence_threshold <= 1.0):
            raise ValueError(
                f"Invalid confidence threshold: {self.model.confidence_threshold}"
            )


def load_config(config_path: Optional[str] = None) -> Config:
    """
    Load configuration from file and environment variables.
    
    Args:
        config_path: Path to YAML configuration file. If None, searches for
                     config files in common locations.
    
    Returns:
        Loaded and validated configuration
    
    Raises:
        FileNotFoundError: If config file is specified but not found
        ValueError: If configuration is invalid
    """
    # Load environment variables from .env file if present
    load_dotenv()
    
    # Default config paths to search
    default_paths = [
        Path("config/config.yaml"),
        Path("config/config.dev.yaml"),
        Path("../config/config.yaml"),
        Path("../config/config.dev.yaml"),
        Path("./config.yaml"),
    ]
    
    # Determine config file path
    if config_path:
        config_file = Path(config_path)
        if not config_file.exists():
            raise FileNotFoundError(f"Configuration file not found: {config_path}")
    else:
        # Search for config file in default locations
        config_file = None
        for path in default_paths:
            if path.exists():
                config_file = path
                break
        
        if config_file is None:
            # Use defaults if no config file found
            return Config()
    
    # Load YAML config
    with open(config_file, "r") as f:
        yaml_config = yaml.safe_load(f) or {}
    
    # Extract AI service config (nested under edge.ai_service or top-level)
    ai_config = yaml_config.get("ai_service", {})
    if not ai_config:
        # Try edge.ai_service
        edge_config = yaml_config.get("edge", {})
        ai_config = edge_config.get("ai_service", {})
    
    # Build config from YAML and environment variables
    config = Config(
        log=LogConfig(
            level=os.getenv("AI_LOG_LEVEL", ai_config.get("log", {}).get("level", "INFO")),
            format=os.getenv("AI_LOG_FORMAT", ai_config.get("log", {}).get("format", "json")),
            output=os.getenv("AI_LOG_OUTPUT", ai_config.get("log", {}).get("output", "stdout")),
        ),
        server=ServerConfig(
            host=os.getenv("AI_HOST", ai_config.get("server", {}).get("host", "0.0.0.0")),
            port=int(os.getenv("AI_PORT", ai_config.get("server", {}).get("port", 8080))),
        ),
        model=ModelConfig(
            model_dir=os.getenv("AI_MODEL_DIR", ai_config.get("model", {}).get("model_dir", "./models")),
            model_name=os.getenv("AI_MODEL_NAME", ai_config.get("model", {}).get("model_name", "yolov8n")),
            model_format=os.getenv("AI_MODEL_FORMAT", ai_config.get("model", {}).get("model_format", "openvino")),
            device=os.getenv("AI_DEVICE", ai_config.get("model", {}).get("device", "AUTO")),
            confidence_threshold=float(
                os.getenv("AI_CONFIDENCE_THRESHOLD", ai_config.get("model", {}).get("confidence_threshold", 0.5))
            ),
            nms_threshold=float(
                os.getenv("AI_NMS_THRESHOLD", ai_config.get("model", {}).get("nms_threshold", 0.4))
            ),
        ),
        inference=InferenceConfig(
            batch_size=int(os.getenv("AI_BATCH_SIZE", ai_config.get("inference", {}).get("batch_size", 1))),
            max_queue_size=int(
                os.getenv("AI_MAX_QUEUE_SIZE", ai_config.get("inference", {}).get("max_queue_size", 100))
            ),
            timeout=float(os.getenv("AI_TIMEOUT", ai_config.get("inference", {}).get("timeout", 30.0))),
        ),
    )
    
    return config

