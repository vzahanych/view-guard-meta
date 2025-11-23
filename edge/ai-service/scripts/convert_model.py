#!/usr/bin/env python3
"""
Model Conversion Script for OpenVINO.

Converts ONNX models to OpenVINO IR format.
"""

import argparse
import sys
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from ai_service.model_converter import convert_onnx_model, check_openvino_tools
from ai_service.logger import setup_logging
from ai_service.config import LogConfig


def main():
    """Main entry point for model conversion."""
    parser = argparse.ArgumentParser(
        description="Convert ONNX model to OpenVINO IR format"
    )
    parser.add_argument(
        "onnx_path",
        type=str,
        help="Path to ONNX model file",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=str,
        default="./models",
        help="Output directory for IR files (default: ./models)",
    )
    parser.add_argument(
        "-n",
        "--name",
        type=str,
        default=None,
        help="Model name for output files (default: input filename)",
    )
    parser.add_argument(
        "--input-shape",
        type=str,
        default=None,
        help="Input shape override (e.g., '1,3,640,640')",
    )
    parser.add_argument(
        "--fp16",
        action="store_true",
        help="Compress model to FP16",
    )
    parser.add_argument(
        "--check-tools",
        action="store_true",
        help="Check if OpenVINO tools are available",
    )
    args = parser.parse_args()
    
    # Setup logging
    setup_logging(LogConfig(level="INFO", format="text", output="stdout"))
    
    # Check tools if requested
    if args.check_tools:
        tools_info = check_openvino_tools()
        print("OpenVINO Tools Status:")
        print(f"  Available: {tools_info['tools_available']}")
        if tools_info.get("version"):
            print(f"  Version: {tools_info['version']}")
        if tools_info.get("error"):
            print(f"  Error: {tools_info['error']}")
        return 0 if tools_info["tools_available"] else 1
    
    # Parse input shape if provided
    input_shape = None
    if args.input_shape:
        try:
            input_shape = tuple(map(int, args.input_shape.split(",")))
        except ValueError:
            print(f"Error: Invalid input shape format: {args.input_shape}", file=sys.stderr)
            print("Expected format: '1,3,640,640'", file=sys.stderr)
            return 1
    
    # Convert model
    try:
        xml_path, bin_path = convert_onnx_model(
            onnx_path=args.onnx_path,
            output_dir=args.output,
            model_name=args.name,
            input_shape=input_shape,
            compress_to_fp16=args.fp16,
        )
        
        print(f"✅ Model conversion successful!")
        print(f"   XML: {xml_path}")
        print(f"   BIN: {bin_path}")
        print(f"   XML size: {xml_path.stat().st_size / 1024:.2f} KB")
        print(f"   BIN size: {bin_path.stat().st_size / 1024 / 1024:.2f} MB")
        
        return 0
        
    except Exception as e:
        print(f"❌ Model conversion failed: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())

