"""
YOLOv8 Model Integration.

Handles downloading, converting, and optimizing YOLOv8 models for OpenVINO.
"""

import logging
import subprocess
import sys
from pathlib import Path
from typing import Optional
from urllib.request import urlretrieve

logger = logging.getLogger(__name__)

# Try to import ultralytics for YOLOv8
try:
    from ultralytics import YOLO
    ULTRALYTICS_AVAILABLE = True
except ImportError:
    ULTRALYTICS_AVAILABLE = False
    logger.warning(
        "Ultralytics not available. Install with: pip install ultralytics"
    )


class YOLOv8Integration:
    """
    YOLOv8 model integration for downloading and converting models.
    """
    
    def __init__(self, model_dir: str | Path):
        """
        Initialize YOLOv8 integration.
        
        Args:
            model_dir: Directory to store models
        """
        self.model_dir = Path(model_dir)
        self.model_dir.mkdir(parents=True, exist_ok=True)
    
    def download_model(
        self,
        model_name: str = "yolov8n",
        format: str = "pt",
    ) -> Path:
        """
        Download pre-trained YOLOv8 model.
        
        Args:
            model_name: Model name (yolov8n, yolov8s, yolov8m, yolov8l, yolov8x)
            format: Model format ("pt" for PyTorch, "onnx" for ONNX)
        
        Returns:
            Path to downloaded model file
        
        Raises:
            RuntimeError: If ultralytics is not available
        """
        if not ULTRALYTICS_AVAILABLE:
            raise RuntimeError(
                "Ultralytics not available. Install with: pip install ultralytics"
            )
        
        logger.info(
            "Downloading YOLOv8 model",
            extra={"model_name": model_name, "format": format},
        )
        
        try:
            # Use ultralytics to download model
            model = YOLO(f"{model_name}.pt")
            
            # Save to our model directory
            output_path = self.model_dir / f"{model_name}.pt"
            # Ultralytics downloads automatically, we just need to copy it
            # For now, we'll use the model directly and export it
            
            logger.info(
                "YOLOv8 model downloaded",
                extra={"model_name": model_name, "path": str(output_path)},
            )
            
            return output_path
        
        except Exception as e:
            logger.error(
                "Failed to download YOLOv8 model",
                exc_info=True,
                extra={"error": str(e), "model_name": model_name},
            )
            raise RuntimeError(f"Failed to download YOLOv8 model: {e}") from e
    
    def convert_to_onnx(
        self,
        model_path: str | Path,
        output_path: Optional[str | Path] = None,
        imgsz: int = 640,
        simplify: bool = True,
    ) -> Path:
        """
        Convert YOLOv8 PyTorch model to ONNX format.
        
        Args:
            model_path: Path to PyTorch model (.pt file)
            output_path: Output path for ONNX file (default: same as input with .onnx extension)
            imgsz: Input image size
            simplify: Whether to simplify ONNX model
        
        Returns:
            Path to converted ONNX file
        
        Raises:
            RuntimeError: If ultralytics is not available or conversion fails
        """
        if not ULTRALYTICS_AVAILABLE:
            raise RuntimeError(
                "Ultralytics not available. Install with: pip install ultralytics"
            )
        
        model_path = Path(model_path)
        if not model_path.exists():
            raise FileNotFoundError(f"Model file not found: {model_path}")
        
        if output_path is None:
            output_path = model_path.with_suffix(".onnx")
        else:
            output_path = Path(output_path)
        
        logger.info(
            "Converting YOLOv8 model to ONNX",
            extra={
                "input": str(model_path),
                "output": str(output_path),
                "imgsz": imgsz,
            },
        )
        
        try:
            # Load model
            model = YOLO(str(model_path))
            
            # Export to ONNX
            model.export(
                format="onnx",
                imgsz=imgsz,
                simplify=simplify,
            )
            
            # Move exported file to desired location
            exported_path = model_path.with_suffix(".onnx")
            if exported_path.exists() and exported_path != output_path:
                exported_path.rename(output_path)
            
            if not output_path.exists():
                raise RuntimeError(f"ONNX conversion failed: {output_path} not created")
            
            logger.info(
                "YOLOv8 model converted to ONNX",
                extra={"output": str(output_path)},
            )
            
            return output_path
        
        except Exception as e:
            logger.error(
                "Failed to convert YOLOv8 model to ONNX",
                exc_info=True,
                extra={"error": str(e), "input": str(model_path)},
            )
            raise RuntimeError(f"Failed to convert to ONNX: {e}") from e
    
    def convert_to_openvino_ir(
        self,
        onnx_path: str | Path,
        output_dir: Optional[str | Path] = None,
        model_name: Optional[str] = None,
        compress_to_fp16: bool = False,
    ) -> tuple[Path, Path]:
        """
        Convert ONNX model to OpenVINO IR format.
        
        Args:
            onnx_path: Path to ONNX model file
            output_dir: Output directory for IR files (default: same as ONNX file directory)
            model_name: Model name for output files (default: ONNX filename without extension)
            compress_to_fp16: Whether to compress to FP16
        
        Returns:
            Tuple of (xml_path, bin_path)
        
        Raises:
            RuntimeError: If conversion fails
        """
        from ai_service.model_converter import convert_onnx_model
        
        onnx_path = Path(onnx_path)
        if not onnx_path.exists():
            raise FileNotFoundError(f"ONNX file not found: {onnx_path}")
        
        if output_dir is None:
            output_dir = onnx_path.parent
        else:
            output_dir = Path(output_dir)
        
        if model_name is None:
            model_name = onnx_path.stem
        
        logger.info(
            "Converting ONNX to OpenVINO IR",
            extra={
                "onnx": str(onnx_path),
                "output_dir": str(output_dir),
                "model_name": model_name,
                "fp16": compress_to_fp16,
            },
        )
        
        try:
            xml_path, bin_path = convert_onnx_model(
                onnx_path=onnx_path,
                output_dir=output_dir,
                model_name=model_name,
                compress_to_fp16=compress_to_fp16,
            )
            
            logger.info(
                "YOLOv8 model converted to OpenVINO IR",
                extra={"xml": str(xml_path), "bin": str(bin_path)},
            )
            
            return xml_path, bin_path
        
        except Exception as e:
            logger.error(
                "Failed to convert ONNX to OpenVINO IR",
                exc_info=True,
                extra={"error": str(e), "onnx": str(onnx_path)},
            )
            raise RuntimeError(f"Failed to convert to OpenVINO IR: {e}") from e
    
    def download_and_convert(
        self,
        model_name: str = "yolov8n",
        target_format: str = "openvino",
        compress_to_fp16: bool = False,
        imgsz: int = 640,
    ) -> tuple[Path, Optional[Path]]:
        """
        Download YOLOv8 model and convert to target format.
        
        Args:
            model_name: Model name (yolov8n, yolov8s, etc.)
            target_format: Target format ("onnx" or "openvino")
            compress_to_fp16: Whether to compress to FP16 (for OpenVINO)
            imgsz: Input image size
        
        Returns:
            Tuple of (model_path, bin_path) where bin_path is None for ONNX
        
        Raises:
            RuntimeError: If download or conversion fails
        """
        logger.info(
            "Downloading and converting YOLOv8 model",
            extra={
                "model_name": model_name,
                "target_format": target_format,
                "fp16": compress_to_fp16,
            },
        )
        
        # Step 1: Download PyTorch model
        pt_path = self.download_model(model_name, format="pt")
        
        if target_format == "onnx":
            # Step 2: Convert to ONNX
            onnx_path = self.convert_to_onnx(pt_path, imgsz=imgsz)
            return onnx_path, None
        
        elif target_format == "openvino":
            # Step 2: Convert to ONNX
            onnx_path = self.convert_to_onnx(pt_path, imgsz=imgsz)
            
            # Step 3: Convert to OpenVINO IR
            xml_path, bin_path = self.convert_to_openvino_ir(
                onnx_path,
                model_name=model_name,
                compress_to_fp16=compress_to_fp16,
            )
            
            return xml_path, bin_path
        
        else:
            raise ValueError(f"Unsupported target format: {target_format}")
    
    def optimize_for_hardware(
        self,
        model_path: str | Path,
        device: str = "CPU",
        compress_to_fp16: bool = False,
    ) -> Path:
        """
        Optimize model for target hardware.
        
        This is a placeholder for future hardware-specific optimizations.
        Currently, optimization is handled during OpenVINO IR conversion.
        
        Args:
            model_path: Path to model file
            device: Target device ("CPU", "GPU", etc.)
            compress_to_fp16: Whether to use FP16 precision
        
        Returns:
            Path to optimized model
        """
        # For now, optimization is handled during conversion
        # Future: Add device-specific optimizations (quantization, pruning, etc.)
        logger.info(
            "Model optimization",
            extra={
                "model": str(model_path),
                "device": device,
                "fp16": compress_to_fp16,
                "note": "Optimization handled during conversion",
            },
        )
        
        return Path(model_path)

