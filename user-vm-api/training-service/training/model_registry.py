"""
Model registry for trained model registration
Handles registration of trained models in the model catalog
"""

import json
import os
from datetime import datetime
from typing import Dict, Optional
import logging
import requests

from config import config

logger = logging.getLogger(__name__)


class ModelRegistry:
    """Handles registration of trained models in the model catalog"""
    
    def __init__(self):
        self.api_url = f"{config.MODEL_CATALOG_API_URL}/api/models"
    
    def register_trained_model(
        self,
        trained_model_id: str,
        onnx_path: str,
        baseline_model_id: str,
        dataset_id: str,
        camera_id: str,
        edge_id: str,
        model_type: str,
        input_shape: list,
        output_classes: int,
        training_metrics: Dict,
        baseline_metadata: Optional[Dict] = None,
        image_size: int = 640,
    ) -> Dict:
        """
        Register a trained model in the model catalog
        
        Args:
            trained_model_id: Unique ID for the trained model
            onnx_path: Path to the exported ONNX model file
            baseline_model_id: ID of the baseline model used for training
            dataset_id: ID of the dataset used for training
            camera_id: Camera ID the model was trained for
            edge_id: Edge device ID
            model_type: Model type (e.g., "yolov8", "yolov8n")
            input_shape: Model input shape (e.g., [1, 3, 640, 640])
            output_classes: Number of output classes
            training_metrics: Training metrics dictionary (loss, mAP, precision, recall, etc.)
            baseline_metadata: Optional baseline model metadata
            image_size: Image size used for training
            
        Returns:
            Dictionary with registration result
            
        Raises:
            ValueError: If registration fails
        """
        # Validate ONNX file exists
        if not os.path.exists(onnx_path):
            raise ValueError(f"ONNX model file not found: {onnx_path}")
        
        # Get model file size
        file_size = os.path.getsize(onnx_path)
        
        # Get model output directory
        model_output_dir = config.get_model_path(trained_model_id)
        os.makedirs(model_output_dir, exist_ok=True)
        
        # Create comprehensive model metadata
        metadata = self._create_model_metadata(
            trained_model_id=trained_model_id,
            baseline_model_id=baseline_model_id,
            dataset_id=dataset_id,
            camera_id=camera_id,
            model_type=model_type,
            input_shape=input_shape,
            output_classes=output_classes,
            training_metrics=training_metrics,
            file_size=file_size,
            image_size=image_size,
            baseline_metadata=baseline_metadata,
        )
        
        # Save metadata.json locally
        metadata_path = os.path.join(model_output_dir, "metadata.json")
        self._save_metadata_file(metadata_path, metadata)
        logger.info(f"Model metadata saved to {metadata_path}")
        
        # Register in model catalog via API
        try:
            self._register_via_api(trained_model_id, onnx_path, metadata)
            logger.info(f"Model {trained_model_id} successfully registered in catalog")
            return {
                "model_id": trained_model_id,
                "status": "success",
                "metadata_path": metadata_path,
                "registered": True,
            }
        except requests.RequestException as e:
            logger.warning(
                f"Failed to register model via API: {e}. "
                f"Model saved locally at {model_output_dir}"
            )
            # Model is still saved locally, so registration is partially successful
            return {
                "model_id": trained_model_id,
                "status": "partial_success",
                "metadata_path": metadata_path,
                "registered": False,
                "error": str(e),
            }
    
    def _create_model_metadata(
        self,
        trained_model_id: str,
        baseline_model_id: str,
        dataset_id: str,
        camera_id: str,
        model_type: str,
        input_shape: list,
        output_classes: int,
        training_metrics: Dict,
        file_size: int,
        image_size: int,
        baseline_metadata: Optional[Dict] = None,
    ) -> Dict:
        """
        Create comprehensive model metadata
        
        Args:
            trained_model_id: Trained model ID
            baseline_model_id: Baseline model ID
            dataset_id: Dataset ID
            camera_id: Camera ID
            model_type: Model type
            input_shape: Input shape
            output_classes: Number of output classes
            training_metrics: Training metrics
            file_size: Model file size in bytes
            image_size: Image size
            baseline_metadata: Optional baseline metadata
            
        Returns:
            Metadata dictionary
        """
        # Get current timestamp
        created_at = datetime.now().isoformat()
        
        # Build metadata according to ModelMetadata struct and plan requirements
        metadata = {
            # Core identification fields
            "model_id": trained_model_id,
            "version": "1.0",
            "camera_id": camera_id,
            "model_type": model_type,
            
            # Model configuration
            "input_shape": input_shape,
            "framework": "onnx",
            "onnx_file": "model.onnx",
            "preprocessing": {
                "normalize": True,
                "image_size": image_size,
            },
            
            # Training information
            "training_dataset_id": dataset_id,
            "training_date": created_at,
            
            # Additional fields for tracking (stored in preprocessing for now)
            # Note: baseline_model_id, output_classes, file_size, created_at, training_metrics
            # are not in the Go ModelMetadata struct, so we store them in preprocessing
            # or as additional metadata fields that can be parsed
        }
        
        # Store additional fields in preprocessing map (since they're not in ModelMetadata struct)
        # These can be accessed via preprocessing["baseline_model_id"], etc.
        metadata["preprocessing"]["baseline_model_id"] = baseline_model_id
        metadata["preprocessing"]["output_classes"] = output_classes
        metadata["preprocessing"]["file_size"] = file_size
        metadata["preprocessing"]["created_at"] = created_at
        
        # Store training metrics in preprocessing (comprehensive metrics)
        metadata["preprocessing"]["training_metrics"] = training_metrics
        
        # Add performance metrics if available (these are in ModelMetadata struct)
        if training_metrics.get("current_map") is not None:
            metadata["accuracy"] = training_metrics["current_map"]
        if training_metrics.get("precision") and len(training_metrics["precision"]) > 0:
            metadata["precision"] = training_metrics["precision"][-1]
        if training_metrics.get("recall") and len(training_metrics["recall"]) > 0:
            metadata["recall"] = training_metrics["recall"][-1]
        
        # Add baseline metadata if available
        if baseline_metadata:
            metadata["preprocessing"]["baseline_metadata"] = baseline_metadata
        
        return metadata
    
    def _save_metadata_file(self, metadata_path: str, metadata: Dict) -> None:
        """
        Save metadata to JSON file
        
        Args:
            metadata_path: Path to save metadata file
            metadata: Metadata dictionary
        """
        with open(metadata_path, "w") as f:
            json.dump(metadata, f, indent=2)
    
    def _register_via_api(
        self,
        model_id: str,
        onnx_path: str,
        metadata: Dict,
    ) -> None:
        """
        Register model via model catalog API
        
        Args:
            model_id: Model ID
            onnx_path: Path to ONNX file
            metadata: Model metadata dictionary
            
        Raises:
            requests.RequestException: If API call fails
        """
        # Read model file
        with open(onnx_path, "rb") as f:
            model_data = f.read()
        
        # Create multipart form data
        # Note: API expects model_id as separate form field, not just in metadata
        files = {
            "model": ("model.onnx", model_data, "application/octet-stream")
        }
        data = {
            "model_id": model_id,  # Required: model_id as separate form field
            "metadata": json.dumps(metadata),
        }
        
        # Make API request
        response = requests.post(
            self.api_url,
            files=files,
            data=data,
            timeout=60,
        )
        response.raise_for_status()
        
        logger.debug(f"API response: {response.status_code} - {response.text}")

