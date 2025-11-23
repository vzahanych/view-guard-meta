"""
Model Conversion Utilities for OpenVINO.

Provides utilities for converting models to OpenVINO IR format.
"""

import logging
import subprocess
import sys
from pathlib import Path
from typing import Dict, Optional, Tuple

logger = logging.getLogger(__name__)

# Try to import OpenVINO tools
try:
    from openvino.tools import mo
    from openvino.tools.mo import convert_model
    OPENVINO_TOOLS_AVAILABLE = True
except ImportError:
    OPENVINO_TOOLS_AVAILABLE = False
    logger.warning(
        "OpenVINO Model Optimizer not available. "
        "Install with: pip install openvino[tools]"
    )


class ModelConverter:
    """
    Model converter for OpenVINO IR format.
    
    Supports conversion from ONNX, TensorFlow, PyTorch, etc.
    """
    
    def __init__(self):
        """Initialize model converter."""
        if not OPENVINO_TOOLS_AVAILABLE:
            raise RuntimeError(
                "OpenVINO Model Optimizer not available. "
                "Install with: pip install openvino[tools]"
            )
    
    def convert_onnx_to_ir(
        self,
        onnx_path: str | Path,
        output_dir: str | Path,
        model_name: Optional[str] = None,
        input_shape: Optional[Tuple] = None,
        compress_to_fp16: bool = False,
    ) -> Tuple[Path, Path]:
        """
        Convert ONNX model to OpenVINO IR format.
        
        Args:
            onnx_path: Path to ONNX model file
            output_dir: Output directory for IR files
            model_name: Name for output files (default: input filename)
            input_shape: Input shape override (e.g., (1, 3, 640, 640))
            compress_to_fp16: Whether to compress to FP16
        
        Returns:
            Tuple of (xml_path, bin_path)
        
        Raises:
            FileNotFoundError: If ONNX file doesn't exist
            RuntimeError: If conversion fails
        """
        onnx_path = Path(onnx_path)
        if not onnx_path.exists():
            raise FileNotFoundError(f"ONNX model not found: {onnx_path}")
        
        output_dir = Path(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)
        
        if model_name is None:
            model_name = onnx_path.stem
        
        logger.info(
            "Converting ONNX to OpenVINO IR",
            extra={
                "onnx_path": str(onnx_path),
                "output_dir": str(output_dir),
                "model_name": model_name,
            },
        )
        
        try:
            # Build conversion arguments
            args = {
                "input_model": str(onnx_path),
                "output_dir": str(output_dir),
                "model_name": model_name,
            }
            
            if input_shape:
                args["input_shape"] = input_shape
            
            if compress_to_fp16:
                args["compress_to_fp16"] = True
            
            # Convert model
            convert_model(**args)
            
            # Verify output files
            xml_path = output_dir / f"{model_name}.xml"
            bin_path = output_dir / f"{model_name}.bin"
            
            if not xml_path.exists():
                raise RuntimeError(f"Conversion failed: {xml_path} not found")
            if not bin_path.exists():
                raise RuntimeError(f"Conversion failed: {bin_path} not found")
            
            logger.info(
                "Model conversion completed",
                extra={
                    "xml_path": str(xml_path),
                    "bin_path": str(bin_path),
                    "xml_size": xml_path.stat().st_size,
                    "bin_size": bin_path.stat().st_size,
                },
            )
            
            return xml_path, bin_path
            
        except Exception as e:
            logger.error(
                "Model conversion failed",
                exc_info=True,
                extra={"error": str(e), "onnx_path": str(onnx_path)},
            )
            raise RuntimeError(f"Model conversion failed: {e}") from e
    
    def convert_pytorch_to_ir(
        self,
        model_path: str | Path,
        output_dir: str | Path,
        model_name: Optional[str] = None,
        input_shape: Optional[Tuple] = None,
        compress_to_fp16: bool = False,
    ) -> Tuple[Path, Path]:
        """
        Convert PyTorch model to OpenVINO IR format.
        
        Note: This requires the model to be exportable via torch.onnx.export first.
        
        Args:
            model_path: Path to PyTorch model file
            output_dir: Output directory for IR files
            model_name: Name for output files
            input_shape: Input shape override
            compress_to_fp16: Whether to compress to FP16
        
        Returns:
            Tuple of (xml_path, bin_path)
        """
        # PyTorch conversion typically requires ONNX export first
        # This is a placeholder - actual implementation would handle PyTorch models
        raise NotImplementedError(
            "PyTorch conversion requires ONNX export first. "
            "Use convert_onnx_to_ir after exporting to ONNX."
        )
    
    def check_conversion_tools(self) -> bool:
        """
        Check if conversion tools are available.
        
        Returns:
            True if tools are available, False otherwise
        """
        return OPENVINO_TOOLS_AVAILABLE


def convert_onnx_model(
    onnx_path: str | Path,
    output_dir: str | Path,
    model_name: Optional[str] = None,
    input_shape: Optional[Tuple] = None,
    compress_to_fp16: bool = False,
) -> Tuple[Path, Path]:
    """
    Convenience function to convert ONNX model to OpenVINO IR.
    
    Args:
        onnx_path: Path to ONNX model file
        output_dir: Output directory for IR files
        model_name: Name for output files
        input_shape: Input shape override
        compress_to_fp16: Whether to compress to FP16
    
    Returns:
        Tuple of (xml_path, bin_path)
    """
    converter = ModelConverter()
    return converter.convert_onnx_to_ir(
        onnx_path=onnx_path,
        output_dir=output_dir,
        model_name=model_name,
        input_shape=input_shape,
        compress_to_fp16=compress_to_fp16,
    )


def check_openvino_tools() -> Dict[str, any]:
    """
    Check OpenVINO tools availability and version.
    
    Returns:
        Dictionary with tool availability information
    """
    result = {
        "tools_available": OPENVINO_TOOLS_AVAILABLE,
        "version": None,
        "error": None,
    }
    
    if not OPENVINO_TOOLS_AVAILABLE:
        result["error"] = "OpenVINO tools not installed"
        return result
    
    try:
        from openvino import get_version
        result["version"] = get_version()
    except Exception as e:
        result["error"] = str(e)
    
    return result

