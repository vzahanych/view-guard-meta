"""
Model Loader Service for OpenVINO Models.

Handles model loading, versioning, hot-reload, and validation.
"""

import hashlib
import logging
import threading
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Optional, Callable, Tuple
from datetime import datetime

logger = logging.getLogger(__name__)

# Try to import OpenVINO
try:
    from openvino import Core
    from openvino.runtime import Model
    OPENVINO_AVAILABLE = True
except ImportError:
    OPENVINO_AVAILABLE = False
    logger.warning("OpenVINO not available. Install with: pip install openvino")


@dataclass
class ModelInfo:
    """Model information and metadata."""
    name: str
    version: str
    format: str  # "openvino" or "onnx"
    path: Path
    xml_path: Path
    bin_path: Optional[Path] = None
    checksum: Optional[str] = None
    loaded_at: Optional[datetime] = None
    input_shape: Optional[tuple] = None
    output_shape: Optional[tuple] = None


@dataclass
class ModelVersion:
    """Model version information."""
    version: str
    path: Path
    checksum: str
    created_at: datetime


class ModelLoader:
    """
    Model loader service for OpenVINO models.
    
    Handles model loading, versioning, hot-reload, and validation.
    """
    
    def __init__(
        self,
        model_dir: str | Path,
        device: str = "CPU",
        runtime: Optional[object] = None,
        on_model_reloaded: Optional[Callable[[ModelInfo], None]] = None,
    ):
        """
        Initialize model loader.
        
        Args:
            model_dir: Directory containing model files
            device: Target device for inference
            runtime: OpenVINO runtime instance (optional)
            on_model_reloaded: Callback function called when model is reloaded
        """
        self.model_dir = Path(model_dir)
        self.device = device
        self.runtime = runtime
        self.on_model_reloaded = on_model_reloaded
        
        self._current_model: Optional[ModelInfo] = None
        self._compiled_model = None
        self._lock = threading.RLock()
        self._versions: Dict[str, ModelVersion] = {}
        
        # Hot-reload monitoring
        self._monitoring = False
        self._monitor_thread: Optional[threading.Thread] = None
        self._monitor_interval = 5.0  # seconds
    
    def load_model(
        self,
        model_name: str,
        model_format: str = "openvino",
        version: Optional[str] = None,
    ) -> ModelInfo:
        """
        Load a model from filesystem.
        
        Args:
            model_name: Name of the model
            model_format: Model format ("openvino" or "onnx")
            version: Specific version to load (default: latest)
        
        Returns:
            ModelInfo object with model metadata
        
        Raises:
            FileNotFoundError: If model files are not found
            ValueError: If model format is invalid
            RuntimeError: If OpenVINO is not available or loading fails
        """
        if not OPENVINO_AVAILABLE:
            raise RuntimeError(
                "OpenVINO is not available. Please install it with: pip install openvino"
            )
        
        with self._lock:
            # Find model files
            model_info = self._find_model_files(model_name, model_format, version)
            
            # Validate model files
            self._validate_model_files(model_info)
            
            # Load model
            self._load_openvino_model(model_info)
            
            # Update current model
            old_model = self._current_model
            self._current_model = model_info
            
            # Notify about reload if callback is set
            if self.on_model_reloaded and old_model is not None:
                try:
                    self.on_model_reloaded(model_info)
                except Exception as e:
                    logger.error("Error in model reload callback", exc_info=True, extra={"error": str(e)})
            
            logger.info(
                "Model loaded successfully",
                extra={
                    "model_name": model_name,
                    "version": model_info.version,
                    "format": model_format,
                    "device": self.device,
                },
            )
            
            return model_info
    
    def _find_model_files(
        self,
        model_name: str,
        model_format: str,
        version: Optional[str],
    ) -> ModelInfo:
        """Find model files in the filesystem."""
        if model_format == "openvino":
            # Look for .xml and .bin files
            if version:
                xml_path = self.model_dir / f"{model_name}_{version}.xml"
                bin_path = self.model_dir / f"{model_name}_{version}.bin"
            else:
                # Find latest version
                xml_path, bin_path, version = self._find_latest_version(model_name)
            
            if not xml_path.exists():
                raise FileNotFoundError(f"Model XML file not found: {xml_path}")
            
            return ModelInfo(
                name=model_name,
                version=version or "unknown",
                format="openvino",
                path=xml_path.parent,
                xml_path=xml_path,
                bin_path=bin_path if bin_path and bin_path.exists() else None,
            )
        
        elif model_format == "onnx":
            # Look for .onnx file
            if version:
                onnx_path = self.model_dir / f"{model_name}_{version}.onnx"
            else:
                # Find latest version
                onnx_path, _, version = self._find_latest_version(model_name, extension=".onnx")
            
            if not onnx_path.exists():
                raise FileNotFoundError(f"Model ONNX file not found: {onnx_path}")
            
            return ModelInfo(
                name=model_name,
                version=version or "unknown",
                format="onnx",
                path=onnx_path.parent,
                xml_path=onnx_path,
            )
        
        else:
            raise ValueError(f"Unsupported model format: {model_format}")
    
    def _find_latest_version(
        self,
        model_name: str,
        extension: str = ".xml",
    ) -> Tuple[Path, Optional[Path], str]:
        """
        Find the latest version of a model.
        
        Returns:
            Tuple of (model_path, bin_path (if OpenVINO), version)
        """
        # Look for files matching pattern: model_name_*.xml or model_name.xml
        pattern = f"{model_name}_*{extension}"
        matching_files = list(self.model_dir.glob(pattern))
        
        # Also check for model_name.xml (no version)
        base_file = self.model_dir / f"{model_name}{extension}"
        if base_file.exists():
            matching_files.append(base_file)
        
        if not matching_files:
            raise FileNotFoundError(
                f"No model files found for '{model_name}' in {self.model_dir}"
            )
        
        # Sort by modification time (newest first)
        matching_files.sort(key=lambda p: p.stat().st_mtime, reverse=True)
        latest_file = matching_files[0]
        
        # Extract version from filename
        version = self._extract_version(latest_file.stem, model_name)
        
        # Find corresponding .bin file for OpenVINO
        bin_path = None
        if extension == ".xml":
            bin_path = latest_file.with_suffix(".bin")
            if not bin_path.exists():
                bin_path = None
        
        return latest_file, bin_path, version
    
    def _extract_version(self, filename: str, model_name: str) -> str:
        """Extract version from filename."""
        # Remove model name prefix
        version_part = filename.replace(f"{model_name}_", "")
        if version_part == filename:
            # No version in filename
            return "latest"
        return version_part
    
    def _validate_model_files(self, model_info: ModelInfo):
        """Validate model files exist and are readable."""
        if not model_info.xml_path.exists():
            raise FileNotFoundError(f"Model file not found: {model_info.xml_path}")
        
        if model_info.format == "openvino" and model_info.bin_path:
            if not model_info.bin_path.exists():
                raise FileNotFoundError(f"Model binary file not found: {model_info.bin_path}")
        
        # Calculate checksum
        model_info.checksum = self._calculate_checksum(model_info.xml_path)
        if model_info.bin_path:
            bin_checksum = self._calculate_checksum(model_info.bin_path)
            model_info.checksum = f"{model_info.checksum}:{bin_checksum}"
    
    def _calculate_checksum(self, file_path: Path) -> str:
        """Calculate SHA256 checksum of a file."""
        sha256 = hashlib.sha256()
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(4096), b""):
                sha256.update(chunk)
        return sha256.hexdigest()
    
    def _load_openvino_model(self, model_info: ModelInfo):
        """Load model using OpenVINO runtime."""
        if self.runtime is None:
            from ai_service.openvino_runtime import create_runtime
            self.runtime = create_runtime(device=self.device)
            if self.runtime is None:
                raise RuntimeError("Failed to create OpenVINO runtime")
        
        core = self.runtime.get_core()
        device = self.runtime.get_device()
        
        # Load model
        if model_info.format == "openvino":
            model = core.read_model(str(model_info.xml_path))
        elif model_info.format == "onnx":
            model = core.read_model(str(model_info.xml_path))
        else:
            raise ValueError(f"Unsupported model format: {model_info.format}")
        
        # Compile model
        self._compiled_model = core.compile_model(model, device)
        
        # Get input/output shapes
        model_info.input_shape = tuple(self._compiled_model.input().shape)
        model_info.output_shape = tuple(self._compiled_model.output().shape)
        model_info.loaded_at = datetime.utcnow()
        
        logger.info(
            "Model compiled successfully",
            extra={
                "model_name": model_info.name,
                "input_shape": model_info.input_shape,
                "output_shape": model_info.output_shape,
                "device": device,
            },
        )
    
    def get_current_model(self) -> Optional[ModelInfo]:
        """Get currently loaded model information."""
        with self._lock:
            return self._current_model
    
    def get_compiled_model(self):
        """Get compiled model for inference."""
        with self._lock:
            return self._compiled_model
    
    def reload_model(self) -> ModelInfo:
        """
        Reload the current model.
        
        Returns:
            ModelInfo for reloaded model
        
        Raises:
            RuntimeError: If no model is currently loaded
        """
        if self._current_model is None:
            raise RuntimeError("No model currently loaded")
        
        return self.load_model(
            self._current_model.name,
            self._current_model.format,
            self._current_model.version,
        )
    
    def start_hot_reload_monitoring(self, interval: float = 5.0):
        """
        Start monitoring model files for changes and auto-reload.
        
        Args:
            interval: Check interval in seconds
        """
        if self._monitoring:
            logger.warning("Hot-reload monitoring already started")
            return
        
        if self._current_model is None:
            logger.warning("No model loaded, cannot start hot-reload monitoring")
            return
        
        self._monitor_interval = interval
        self._monitoring = True
        self._monitor_thread = threading.Thread(
            target=self._monitor_loop,
            daemon=True,
            name="ModelHotReloadMonitor",
        )
        self._monitor_thread.start()
        
        logger.info(
            "Hot-reload monitoring started",
            extra={"interval": interval, "model": self._current_model.name},
        )
    
    def stop_hot_reload_monitoring(self):
        """Stop hot-reload monitoring."""
        if not self._monitoring:
            return
        
        self._monitoring = False
        if self._monitor_thread:
            self._monitor_thread.join(timeout=2.0)
        
        logger.info("Hot-reload monitoring stopped")
    
    def _monitor_loop(self):
        """Monitor loop for hot-reload."""
        last_checksum = self._current_model.checksum if self._current_model else None
        
        while self._monitoring:
            try:
                time.sleep(self._monitor_interval)
                
                if self._current_model is None:
                    break
                
                # Check if model files have changed
                current_checksum = self._calculate_checksum(self._current_model.xml_path)
                if self._current_model.bin_path:
                    bin_checksum = self._calculate_checksum(self._current_model.bin_path)
                    current_checksum = f"{current_checksum}:{bin_checksum}"
                
                if current_checksum != last_checksum:
                    logger.info(
                        "Model files changed, reloading",
                        extra={"model": self._current_model.name},
                    )
                    try:
                        self.reload_model()
                        last_checksum = self._current_model.checksum
                    except Exception as e:
                        logger.error(
                            "Failed to reload model",
                            exc_info=True,
                            extra={"error": str(e)},
                        )
            
            except Exception as e:
                logger.error(
                    "Error in hot-reload monitoring",
                    exc_info=True,
                    extra={"error": str(e)},
                )
    
    def list_versions(self, model_name: str) -> list[ModelVersion]:
        """
        List all available versions of a model.
        
        Args:
            model_name: Name of the model
        
        Returns:
            List of ModelVersion objects
        """
        versions = []
        
        # Find all model files
        for xml_file in self.model_dir.glob(f"{model_name}_*.xml"):
            version = self._extract_version(xml_file.stem, model_name)
            checksum = self._calculate_checksum(xml_file)
            created_at = datetime.fromtimestamp(xml_file.stat().st_mtime)
            
            versions.append(
                ModelVersion(
                    version=version,
                    path=xml_file,
                    checksum=checksum,
                    created_at=created_at,
                )
            )
        
        # Also check for base model (no version)
        base_file = self.model_dir / f"{model_name}.xml"
        if base_file.exists():
            checksum = self._calculate_checksum(base_file)
            created_at = datetime.fromtimestamp(base_file.stat().st_mtime)
            versions.append(
                ModelVersion(
                    version="latest",
                    path=base_file,
                    checksum=checksum,
                    created_at=created_at,
                )
            )
        
        # Sort by creation time (newest first)
        versions.sort(key=lambda v: v.created_at, reverse=True)
        
        return versions

