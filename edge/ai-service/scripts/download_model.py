#!/usr/bin/env python3
"""
Download and convert YOLOv8 model for Edge AI Service.

This script downloads a YOLOv8 model and converts it to OpenVINO format.

NOTE: This is for local testing only. In production, models are downloaded
from a remote VM, not locally.
"""

import argparse
import logging
import sys
from pathlib import Path

# Add parent directory to path for imports (optional, not needed for ultralytics/openvino)
# sys.path.insert(0, str(Path(__file__).parent.parent))

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


def download_yolov8_model(model_name: str = "yolov8n", model_dir: Path = None):
    """
    Download YOLOv8 model and convert to OpenVINO format.
    
    Args:
        model_name: YOLOv8 model name (yolov8n, yolov8s, yolov8m, yolov8l, yolov8x)
        model_dir: Directory to save the model
    """
    try:
        from ultralytics import YOLO
    except ImportError as e:
        logger.error(f"ultralytics not installed or import failed: {e}")
        logger.error("Install with: pip install ultralytics")
        return False
    except Exception as e:
        logger.error(f"Error importing ultralytics (may be missing OpenCV dependencies): {e}")
        logger.error("Install OpenCV system dependencies: libgl1 libglib2.0-0")
        return False
    
    try:
        from openvino import convert_model, save_model
    except ImportError:
        logger.error("OpenVINO not installed. Install with: pip install openvino")
        return False
    
    if model_dir is None:
        model_dir = Path("./models")
    else:
        model_dir = Path(model_dir)
    
    model_dir.mkdir(parents=True, exist_ok=True)
    
    logger.info(f"Downloading YOLOv8 model: {model_name}")
    
    # Download YOLOv8 model (this will download the PyTorch model)
    model = YOLO(f"{model_name}.pt")
    
    # Export to ONNX first
    onnx_path = model_dir / f"{model_name}.onnx"
    logger.info(f"Exporting to ONNX: {onnx_path}")
    model.export(format="onnx", imgsz=640, simplify=True)
    
    # Move ONNX file to model directory
    # YOLO exports to current directory, so we need to find it
    onnx_files = list(Path(".").glob(f"{model_name}*.onnx"))
    if onnx_files:
        import shutil
        shutil.move(str(onnx_files[0]), str(onnx_path))
        logger.info(f"ONNX model saved to: {onnx_path}")
    else:
        logger.error("Failed to find exported ONNX file")
        return False
    
    # Convert ONNX to OpenVINO IR
    logger.info("Converting ONNX to OpenVINO IR...")
    try:
        from openvino import convert_model
        
        # Convert ONNX to OpenVINO
        ov_model = convert_model(str(onnx_path))
        
        # Save OpenVINO model
        xml_path = model_dir / f"{model_name}.xml"
        save_model(ov_model, str(xml_path))
        
        logger.info(f"OpenVINO model saved to: {xml_path}")
        logger.info("Model download and conversion complete!")
        return True
        
    except Exception as e:
        logger.error(f"Failed to convert to OpenVINO: {e}")
        logger.info(f"ONNX model is available at: {onnx_path}")
        logger.info("You can use ONNX format instead by setting model_format='onnx'")
        return False


def main():
    parser = argparse.ArgumentParser(description="Download and convert YOLOv8 model")
    parser.add_argument(
        "--model-name",
        default="yolov8n",
        choices=["yolov8n", "yolov8s", "yolov8m", "yolov8l", "yolov8x"],
        help="YOLOv8 model name (default: yolov8n)",
    )
    parser.add_argument(
        "--model-dir",
        type=str,
        default="./models",
        help="Directory to save the model (default: ./models)",
    )
    
    args = parser.parse_args()
    
    success = download_yolov8_model(
        model_name=args.model_name,
        model_dir=Path(args.model_dir),
    )
    
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()

