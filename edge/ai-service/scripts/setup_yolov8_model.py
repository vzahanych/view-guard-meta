#!/usr/bin/env python3
"""
Script to download and convert YOLOv8 models.

Downloads YOLOv8 models and converts them to OpenVINO IR format.
"""

import argparse
import sys
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from ai_service.yolov8_integration import YOLOv8Integration
from ai_service.logger import setup_logging
from ai_service.config import LogConfig


def main():
    """Main entry point for YOLOv8 model setup."""
    parser = argparse.ArgumentParser(
        description="Download and convert YOLOv8 models"
    )
    parser.add_argument(
        "--model",
        type=str,
        default="yolov8n",
        choices=["yolov8n", "yolov8s", "yolov8m", "yolov8l", "yolov8x"],
        help="YOLOv8 model name (default: yolov8n)",
    )
    parser.add_argument(
        "--format",
        type=str,
        default="openvino",
        choices=["onnx", "openvino"],
        help="Target format (default: openvino)",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default="./models",
        help="Output directory for models (default: ./models)",
    )
    parser.add_argument(
        "--fp16",
        action="store_true",
        help="Compress model to FP16 (for OpenVINO)",
    )
    parser.add_argument(
        "--imgsz",
        type=int,
        default=640,
        help="Input image size (default: 640)",
    )
    args = parser.parse_args()
    
    # Setup logging
    setup_logging(LogConfig(level="INFO", format="text", output="stdout"))
    
    # Create output directory
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    
    # Initialize YOLOv8 integration
    integration = YOLOv8Integration(model_dir=output_dir)
    
    try:
        print(f"Downloading and converting {args.model} to {args.format} format...")
        print(f"Output directory: {output_dir}")
        print()
        
        # Download and convert
        model_path, bin_path = integration.download_and_convert(
            model_name=args.model,
            target_format=args.format,
            compress_to_fp16=args.fp16,
            imgsz=args.imgsz,
        )
        
        print()
        print("✅ Model setup complete!")
        print(f"   Model: {model_path}")
        if bin_path:
            print(f"   Binary: {bin_path}")
        print()
        print(f"You can now use this model with the AI service.")
        print(f"Configure it in your config file:")
        print(f"  model:")
        print(f"    model_name: {args.model}")
        print(f"    model_format: {args.format}")
        
        return 0
        
    except Exception as e:
        print(f"❌ Model setup failed: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())

