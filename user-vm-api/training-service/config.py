"""
Configuration management for training service
"""

import os
from pathlib import Path


class Config:
    """Training service configuration loaded from environment variables"""
    
    # Service configuration
    HOST: str = os.getenv("PYTHON_AI_SERVICE_HOST", "0.0.0.0")
    PORT: int = int(os.getenv("PYTHON_AI_SERVICE_PORT", "8000"))
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO").upper()
    
    # Directory paths
    DATASETS_DIR: str = os.getenv("DATASETS_DIR", "/app/data/datasets")
    MODELS_DIR: str = os.getenv("MODELS_DIR", "/app/data/models")
    TRAINING_OUTPUT_DIR: str = os.getenv("TRAINING_OUTPUT_DIR", "/app/data/training")
    
    # Model catalog API endpoint (for querying models)
    MODEL_CATALOG_API_URL: str = os.getenv(
        "MODEL_CATALOG_API_URL", 
        "http://user-vm-api:8280"
    )
    
    @classmethod
    def ensure_directories(cls) -> None:
        """Ensure all required directories exist"""
        directories = [
            cls.DATASETS_DIR,
            cls.MODELS_DIR,
            cls.TRAINING_OUTPUT_DIR,
        ]
        for directory in directories:
            Path(directory).mkdir(parents=True, exist_ok=True)
    
    @classmethod
    def get_dataset_path(cls, edge_id: str, camera_id: str, dataset_id: str) -> str:
        """Get full path to a dataset directory"""
        return os.path.join(cls.DATASETS_DIR, edge_id, camera_id, dataset_id)
    
    @classmethod
    def get_model_path(cls, model_id: str) -> str:
        """Get full path to a model directory"""
        return os.path.join(cls.MODELS_DIR, model_id)
    
    @classmethod
    def get_training_output_path(cls, job_id: str) -> str:
        """Get full path to a training job output directory"""
        return os.path.join(cls.TRAINING_OUTPUT_DIR, job_id)
    
    @classmethod
    def to_dict(cls) -> dict:
        """Convert configuration to dictionary for API responses"""
        return {
            "host": cls.HOST,
            "port": cls.PORT,
            "log_level": cls.LOG_LEVEL,
            "datasets_dir": cls.DATASETS_DIR,
            "models_dir": cls.MODELS_DIR,
            "training_output_dir": cls.TRAINING_OUTPUT_DIR,
            "model_catalog_api_url": cls.MODEL_CATALOG_API_URL,
        }


# Global configuration instance
config = Config()

