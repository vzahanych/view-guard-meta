#!/usr/bin/env python3
"""
Test that a downloaded model is healthy and can be loaded.

This script verifies that a model file can be loaded and used for inference.
"""

import argparse
import logging
import sys
from pathlib import Path

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


def test_openvino_model(model_path: Path):
    """Test loading an OpenVINO model."""
    try:
        from openvino import Core
        
        logger.info(f"Testing OpenVINO model: {model_path}")
        
        # Load model
        core = Core()
        model = core.read_model(str(model_path))
        
        # Compile model
        compiled_model = core.compile_model(model, "CPU")
        
        # Get input/output info
        input_layer = compiled_model.input()
        output_layer = compiled_model.output()
        
        logger.info("✅ Model loaded successfully!")
        logger.info(f"   Input shape: {input_layer.shape}")
        logger.info(f"   Output shape: {output_layer.shape}")
        logger.info(f"   Input name: {input_layer.get_any_name()}")
        logger.info(f"   Output name: {output_layer.get_any_name()}")
        
        # Test inference with dummy data
        import numpy as np
        input_shape = input_layer.shape
        # Create dummy input (batch, channels, height, width)
        dummy_input = np.random.randn(*input_shape).astype(np.float32)
        
        logger.info("Testing inference with dummy input...")
        result = compiled_model([dummy_input])
        
        logger.info("✅ Inference test successful!")
        logger.info(f"   Output shape: {result[output_layer].shape}")
        
        return True
        
    except ImportError:
        logger.error("OpenVINO not installed. Install with: pip install openvino")
        return False
    except Exception as e:
        logger.error(f"Failed to test model: {e}", exc_info=True)
        return False


def test_onnx_model(model_path: Path):
    """Test loading an ONNX model."""
    try:
        import onnxruntime as ort
        
        logger.info(f"Testing ONNX model: {model_path}")
        
        # Create inference session
        session = ort.InferenceSession(str(model_path))
        
        # Get input/output info
        input_name = session.get_inputs()[0].name
        output_name = session.get_outputs()[0].name
        input_shape = session.get_inputs()[0].shape
        
        logger.info("✅ Model loaded successfully!")
        logger.info(f"   Input name: {input_name}")
        logger.info(f"   Input shape: {input_shape}")
        logger.info(f"   Output name: {output_name}")
        
        # Test inference with dummy data
        import numpy as np
        # Create dummy input (handle dynamic batch size)
        if input_shape[0] == 'batch' or input_shape[0] is None:
            batch_size = 1
            actual_shape = [batch_size] + list(input_shape[1:])
        else:
            actual_shape = list(input_shape)
        
        dummy_input = np.random.randn(*actual_shape).astype(np.float32)
        
        logger.info("Testing inference with dummy input...")
        outputs = session.run([output_name], {input_name: dummy_input})
        
        logger.info("✅ Inference test successful!")
        logger.info(f"   Output shape: {outputs[0].shape}")
        
        return True
        
    except ImportError:
        logger.error("ONNXRuntime not installed. Install with: pip install onnxruntime")
        return False
    except Exception as e:
        logger.error(f"Failed to test model: {e}", exc_info=True)
        return False


def main():
    parser = argparse.ArgumentParser(description="Test that a model is healthy and can be loaded")
    parser.add_argument(
        "--model-path",
        type=str,
        required=True,
        help="Path to model file (.xml for OpenVINO or .onnx for ONNX)",
    )
    parser.add_argument(
        "--model-format",
        type=str,
        choices=["openvino", "onnx", "auto"],
        default="auto",
        help="Model format (default: auto-detect from file extension)",
    )
    
    args = parser.parse_args()
    
    model_path = Path(args.model_path)
    
    if not model_path.exists():
        logger.error(f"Model file not found: {model_path}")
        sys.exit(1)
    
    # Auto-detect format from extension
    if args.model_format == "auto":
        if model_path.suffix == ".xml":
            model_format = "openvino"
        elif model_path.suffix == ".onnx":
            model_format = "onnx"
        else:
            logger.error(f"Unknown model format. Expected .xml or .onnx, got: {model_path.suffix}")
            sys.exit(1)
    else:
        model_format = args.model_format
    
    logger.info(f"Testing {model_format.upper()} model: {model_path}")
    
    # Test model
    if model_format == "openvino":
        success = test_openvino_model(model_path)
    else:
        success = test_onnx_model(model_path)
    
    if success:
        logger.info("✅ Model health check passed!")
        sys.exit(0)
    else:
        logger.error("❌ Model health check failed!")
        sys.exit(1)


if __name__ == "__main__":
    main()

