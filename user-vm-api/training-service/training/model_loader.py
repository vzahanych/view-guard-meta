"""
Model loader for YOLOv8 training
Loads baseline models from model storage and configures them for fine-tuning
"""

import json
import os
import logging
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import requests

from ultralytics import YOLO

from config import config

logger = logging.getLogger(__name__)


class ModelMetadata:
    """Model metadata from model catalog API"""
    
    def __init__(self, data: dict):
        self.model_id: str = data.get("model_id", "")
        self.version: str = data.get("version", "")
        self.camera_id: Optional[str] = data.get("camera_id")
        self.model_type: str = data.get("model_type", "")
        self.status: str = data.get("status", "")
        self.framework: str = data.get("framework", "")
        self.training_dataset_id: Optional[str] = data.get("training_dataset_id")
        self.training_date: Optional[str] = data.get("training_date")
        
        # Metadata from nested metadata object
        metadata_obj = data.get("metadata", {})
        self.input_shape: List[int] = metadata_obj.get("input_shape", [1, 3, 640, 640])
        self.latent_dim: Optional[int] = metadata_obj.get("latent_dim")
        self.threshold: Optional[float] = metadata_obj.get("threshold")
        self.onnx_file: str = metadata_obj.get("onnx_file", "model.onnx")
        self.preprocessing: Optional[Dict] = metadata_obj.get("preprocessing")
        
        # Performance metrics
        self.accuracy: Optional[float] = metadata_obj.get("accuracy")
        self.precision: Optional[float] = metadata_obj.get("precision")
        self.recall: Optional[float] = metadata_obj.get("recall")
        self.f1_score: Optional[float] = metadata_obj.get("f1_score")


class ModelLoader:
    """Loads and configures models for training"""
    
    def __init__(self, model_id: str):
        """
        Initialize model loader
        
        Args:
            model_id: Model ID to load
        """
        self.model_id = model_id
        self.metadata: Optional[ModelMetadata] = None
        self.model_path: Optional[str] = None
        self.model: Optional[YOLO] = None
        
    def load_metadata_from_catalog(self) -> ModelMetadata:
        """
        Load model metadata from model catalog API
        
        Returns:
            ModelMetadata object
            
        Raises:
            requests.RequestException: If API request fails
            ValueError: If model not found or invalid status
        """
        api_url = f"{config.MODEL_CATALOG_API_URL}/api/models/{self.model_id}"
        
        logger.info(f"Fetching model metadata from {api_url}")
        
        try:
            response = requests.get(api_url, timeout=10)
            response.raise_for_status()
            
            data = response.json()
            self.metadata = ModelMetadata(data)
            
            # Verify model status
            if self.metadata.status not in ["baseline", "ready"]:
                raise ValueError(
                    f"Model {self.model_id} has status '{self.metadata.status}', "
                    f"expected 'baseline' or 'ready'"
                )
            
            logger.info(
                f"Loaded model metadata: {self.model_id}, "
                f"type={self.metadata.model_type}, "
                f"status={self.metadata.status}, "
                f"framework={self.metadata.framework}"
            )
            
            return self.metadata
            
        except requests.exceptions.RequestException as e:
            logger.error(f"Failed to fetch model metadata: {e}")
            raise
        except (KeyError, ValueError) as e:
            logger.error(f"Invalid model metadata response: {e}")
            raise ValueError(f"Invalid model metadata: {e}")
    
    def load_metadata_from_file(self) -> ModelMetadata:
        """
        Load model metadata from local metadata.json file
        
        Returns:
            ModelMetadata object
            
        Raises:
            FileNotFoundError: If metadata.json not found
        """
        model_dir = Path(config.get_model_path(self.model_id))
        metadata_path = model_dir / "metadata.json"
        
        if not metadata_path.exists():
            raise FileNotFoundError(f"metadata.json not found at {metadata_path}")
        
        with open(metadata_path, "r") as f:
            data = json.load(f)
        
        # Convert to API format for consistency
        api_format = {
            "model_id": data.get("model_id", self.model_id),
            "version": data.get("version", "1.0"),
            "camera_id": data.get("camera_id"),
            "model_type": data.get("model_type", "yolov8"),
            "status": data.get("status", "baseline"),
            "framework": data.get("framework", "onnx"),
            "training_dataset_id": data.get("training_dataset_id"),
            "training_date": data.get("training_date"),
            "metadata": data,  # Nest metadata for consistency
        }
        
        self.metadata = ModelMetadata(api_format)
        return self.metadata
    
    def find_model_file(self, require_pytorch: bool = False) -> str:
        """
        Find model file in model directory
        
        Args:
            require_pytorch: If True, prioritize PyTorch models for training
        
        Returns:
            Path to model file
            
        Raises:
            FileNotFoundError: If model file not found
        """
        model_dir = Path(config.get_model_path(self.model_id))
        
        if not model_dir.exists():
            raise FileNotFoundError(f"Model directory not found: {model_dir}")
        
        # If we need PyTorch for training, prioritize .pt files
        if require_pytorch:
            # First, try to find existing PyTorch model
            pytorch_candidates = [
                model_dir / "yolov8n.pt",
                model_dir / "yolov8s.pt",
                model_dir / "yolov8m.pt",
                model_dir / "yolov8l.pt",
                model_dir / "yolov8x.pt",
            ]
            
            for candidate in pytorch_candidates:
                if candidate.exists() and candidate.is_file():
                    self.model_path = str(candidate)
                    logger.info(f"Found PyTorch model file: {self.model_path}")
                    return self.model_path
            
            # If no PyTorch model found, try to download it
            if self.metadata and self.metadata.model_type:
                model_type = self.metadata.model_type.lower()
                if "yolov8" in model_type or "yolo" in model_type:
                    # Extract model size (n, s, m, l, x) from model_type
                    size = "n"  # default
                    if "yolov8n" in model_type or "yolo8n" in model_type:
                        size = "n"
                    elif "yolov8s" in model_type or "yolo8s" in model_type:
                        size = "s"
                    elif "yolov8m" in model_type or "yolo8m" in model_type:
                        size = "m"
                    elif "yolov8l" in model_type or "yolo8l" in model_type:
                        size = "l"
                    elif "yolov8x" in model_type or "yolo8x" in model_type:
                        size = "x"
                    
                    pytorch_model_name = f"yolov8{size}.pt"
                    pytorch_path = model_dir / pytorch_model_name
                    
                    # Try to download PyTorch model using Ultralytics
                    # Note: model_dir might be read-only, so we'll try to download to a writable location first
                    if not pytorch_path.exists():
                        logger.info(f"ONNX model found but PyTorch model needed for training. Downloading {pytorch_model_name}...")
                        try:
                            from ultralytics import YOLO
                            import shutil
                            import tempfile
                            
                            # Try to download to a temporary writable location first
                            temp_dir = Path(config.TRAINING_OUTPUT_DIR) / ".pytorch_models"
                            temp_dir.mkdir(parents=True, exist_ok=True)
                            temp_pytorch_path = temp_dir / pytorch_model_name
                            
                            if not temp_pytorch_path.exists():
                                # Ultralytics will download the model automatically
                                temp_model = YOLO(pytorch_model_name)
                                # Get the actual path from the model
                                model_weights_path = getattr(temp_model.model, 'weights', None) or getattr(temp_model, 'ckpt_path', None)
                                if model_weights_path and os.path.exists(model_weights_path):
                                    shutil.copy(model_weights_path, str(temp_pytorch_path))
                                    logger.info(f"Downloaded PyTorch model to {temp_pytorch_path}")
                                else:
                                    # Try to find in Ultralytics cache
                                    from pathlib import Path as PathLib
                                    ultralytics_home = PathLib(os.path.expanduser("~")) / ".ultralytics"
                                    cached_models = list(ultralytics_home.rglob(pytorch_model_name))
                                    if cached_models:
                                        shutil.copy(str(cached_models[0]), str(temp_pytorch_path))
                                        logger.info(f"Copied PyTorch model from cache to {temp_pytorch_path}")
                                    else:
                                        # As last resort, download directly
                                        logger.info(f"Downloading {pytorch_model_name} directly from GitHub...")
                                        import urllib.request
                                        download_url = f"https://github.com/ultralytics/assets/releases/download/v8.3.0/{pytorch_model_name}"
                                        urllib.request.urlretrieve(download_url, str(temp_pytorch_path))
                                        logger.info(f"Downloaded PyTorch model to {temp_pytorch_path}")
                            
                            # Try to copy to model_dir (might fail if read-only, that's OK)
                            try:
                                if temp_pytorch_path.exists():
                                    shutil.copy(str(temp_pytorch_path), str(pytorch_path))
                                    logger.info(f"Copied PyTorch model to model directory: {pytorch_path}")
                            except (OSError, PermissionError) as e:
                                logger.info(f"Could not copy to model directory (read-only): {e}. Using temp location.")
                                pytorch_path = temp_pytorch_path
                                
                        except Exception as e:
                            logger.error(f"Failed to download PyTorch model: {e}")
                            raise
                    
                    if pytorch_path.exists():
                        self.model_path = str(pytorch_path)
                        logger.info(f"Using PyTorch model for training: {self.model_path}")
                        return self.model_path
                    
                    # If we require PyTorch but couldn't get it, raise error
                    if require_pytorch:
                        raise FileNotFoundError(
                            f"PyTorch model required for training but not found: {pytorch_path}. "
                            f"ONNX models cannot be used for training. Please ensure a PyTorch (.pt) model is available."
                        )
        
        # Try to find model file (for inference, ONNX is fine - only if require_pytorch=False)
        # Priority: 1. metadata.onnx_file, 2. model.onnx, 3. yolov8n.pt, 4. yolov8n.onnx
        candidates = []
        
        if self.metadata and self.metadata.onnx_file:
            candidates.append(model_dir / self.metadata.onnx_file)
        
        candidates.extend([
            model_dir / "model.onnx",
            model_dir / "yolov8n.pt",
            model_dir / "yolov8n.onnx",
        ])
        
        for candidate in candidates:
            if candidate.exists() and candidate.is_file():
                self.model_path = str(candidate)
                logger.info(f"Found model file: {self.model_path}")
                return self.model_path
        
        raise FileNotFoundError(
            f"Model file not found in {model_dir}. "
            f"Tried: {[str(c) for c in candidates]}"
        )
    
    def validate_model_compatibility(self) -> Tuple[bool, Optional[str]]:
        """
        Validate model compatibility for training
        
        Returns:
            Tuple of (is_compatible, error_message)
        """
        if self.metadata is None:
            return False, "Model metadata not loaded"
        
        # Check model type
        if self.metadata.model_type.lower() not in ["yolo", "yolov8", "yolov8n"]:
            return False, f"Unsupported model type: {self.metadata.model_type}"
        
        # Check framework
        if self.metadata.framework.lower() not in ["onnx", "pytorch", "torch"]:
            logger.warning(
                f"Model framework is {self.metadata.framework}, "
                f"may need conversion for Ultralytics"
            )
        
        # Check input shape (YOLOv8 typically expects [1, 3, 640, 640])
        if self.metadata.input_shape:
            if len(self.metadata.input_shape) != 4:
                return False, f"Invalid input shape: {self.metadata.input_shape}"
            
            # Check if it's a valid image shape [batch, channels, height, width]
            batch, channels, height, width = self.metadata.input_shape
            if channels != 3:
                return False, f"Expected 3 channels (RGB), got {channels}"
            if height != width:
                logger.warning(
                    f"Non-square input shape: {height}x{width}. "
                    f"YOLOv8 typically uses square inputs (640x640)"
                )
        
        return True, None
    
    def load_model(self) -> YOLO:
        """
        Load YOLOv8 model using Ultralytics
        
        Returns:
            YOLO model object
            
        Raises:
            FileNotFoundError: If model file not found
            ValueError: If model loading fails
        """
        if self.model_path is None:
            self.find_model_file()
        
        if not os.path.exists(self.model_path):
            raise FileNotFoundError(f"Model file not found: {self.model_path}")
        
        logger.info(f"Loading YOLOv8 model from {self.model_path}")
        
        try:
            # Ultralytics YOLO can load from .pt, .onnx, and other formats
            # For ONNX, it will convert internally if needed
            self.model = YOLO(self.model_path)
            
            logger.info(
                f"Model loaded successfully. "
                f"Type: {type(self.model)}, "
                f"Task: {getattr(self.model, 'task', 'unknown')}"
            )
            
            return self.model
            
        except Exception as e:
            logger.error(f"Failed to load model: {e}")
            raise ValueError(f"Failed to load model: {e}")
    
    def configure_for_fine_tuning(
        self,
        num_classes: int,
        freeze_backbone: bool = False
    ) -> YOLO:
        """
        Configure model for fine-tuning
        
        Args:
            num_classes: Number of output classes for fine-tuning
            freeze_backbone: Whether to freeze backbone layers (faster training, less adaptation)
            
        Returns:
            Configured YOLO model
        """
        if self.model is None:
            raise ValueError("Model not loaded. Call load_model() first.")
        
        # Get current number of classes from model
        # YOLOv8 models have a 'nc' attribute for number of classes
        current_classes = getattr(self.model.model, 'nc', None)
        
        if current_classes is None:
            # Try to get from model.yaml or model config
            logger.warning("Could not determine current number of classes, assuming 80 (COCO)")
            current_classes = 80
        
        logger.info(
            f"Configuring model for fine-tuning: "
            f"{current_classes} -> {num_classes} classes"
        )
        
        # If number of classes is different, we need to modify the model
        if current_classes != num_classes:
            logger.info(f"Adjusting model output layer for {num_classes} classes")
            
            # Ultralytics YOLO models can be modified by creating a new model
            # with the same architecture but different number of classes
            # For now, we'll let Ultralytics handle this during training
            # The model will be automatically adjusted when we call model.train()
            # with a dataset that has a different number of classes
        
        # Freeze backbone if requested
        if freeze_backbone:
            logger.info("Freezing backbone layers for faster training")
            # Ultralytics doesn't have a direct freeze_backbone method
            # We can freeze layers manually if needed, but for PoC we'll skip this
            # as Ultralytics handles fine-tuning well without explicit freezing
            logger.warning("Backbone freezing not implemented (Ultralytics handles this automatically)")
        
        return self.model
    
    def get_model_info(self) -> Dict:
        """
        Get model information for logging/debugging
        
        Returns:
            Dictionary with model information
        """
        info = {
            "model_id": self.model_id,
            "model_path": self.model_path,
        }
        
        if self.metadata:
            info.update({
                "model_type": self.metadata.model_type,
                "status": self.metadata.status,
                "framework": self.metadata.framework,
                "input_shape": self.metadata.input_shape,
                "version": self.metadata.version,
            })
        
        if self.model:
            info["model_loaded"] = True
            info["model_task"] = getattr(self.model, 'task', 'unknown')
        else:
            info["model_loaded"] = False
        
        return info


def load_baseline_model(
    model_id: str,
    num_classes: int,
    use_catalog_api: bool = True,
    freeze_backbone: bool = False
) -> Tuple[YOLO, ModelMetadata]:
    """
    Convenience function to load a baseline model for training
    
    Args:
        model_id: Model ID to load
        num_classes: Number of output classes for fine-tuning
        use_catalog_api: Whether to use catalog API (True) or local file (False)
        freeze_backbone: Whether to freeze backbone layers
        
    Returns:
        Tuple of (YOLO model, ModelMetadata)
        
    Raises:
        FileNotFoundError: If model not found
        ValueError: If model validation fails
    """
    loader = ModelLoader(model_id)
    
    # Load metadata
    if use_catalog_api:
        try:
            loader.load_metadata_from_catalog()
        except (requests.RequestException, ValueError) as e:
            logger.warning(f"Failed to load from catalog API: {e}, trying local file")
            loader.load_metadata_from_file()
    else:
        loader.load_metadata_from_file()
    
    # Validate compatibility
    is_compatible, error = loader.validate_model_compatibility()
    if not is_compatible:
        raise ValueError(f"Model compatibility validation failed: {error}")
    
    # Find and load model (require PyTorch for training)
    loader.find_model_file(require_pytorch=True)
    model = loader.load_model()
    
    # Configure for fine-tuning
    model = loader.configure_for_fine_tuning(num_classes, freeze_backbone)
    
    logger.info(f"Baseline model loaded and configured: {loader.get_model_info()}")
    
    return model, loader.metadata

