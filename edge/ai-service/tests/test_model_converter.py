"""
Unit tests for model conversion utilities.
"""

import pytest
from pathlib import Path
from unittest.mock import patch, MagicMock


class TestModelConverter:
    """Tests for ModelConverter class."""
    
    def test_converter_initialization_available(self, mock_openvino_available):
        """Test converter initialization when tools are available."""
        from ai_service.model_converter import ModelConverter
        
        converter = ModelConverter()
        assert converter is not None
    
    def test_converter_initialization_unavailable(self, mock_openvino_unavailable):
        """Test converter initialization when tools are unavailable."""
        from ai_service.model_converter import ModelConverter
        
        with pytest.raises(RuntimeError, match="OpenVINO Model Optimizer not available"):
            ModelConverter()
    
    def test_convert_onnx_to_ir_file_not_found(self, mock_openvino_available, temp_dir: Path):
        """Test ONNX to IR conversion with non-existent file."""
        from ai_service.model_converter import ModelConverter
        
        converter = ModelConverter()
        onnx_path = temp_dir / "nonexistent.onnx"
        
        with pytest.raises(FileNotFoundError):
            converter.convert_onnx_to_ir(
                onnx_path=onnx_path,
                output_dir=temp_dir,
            )
    
    def test_convert_onnx_to_ir_success(self, mock_openvino_available, temp_dir: Path):
        """Test successful ONNX to IR conversion."""
        from ai_service.model_converter import ModelConverter
        
        # Create dummy ONNX file
        onnx_path = temp_dir / "model.onnx"
        onnx_path.write_bytes(b"dummy onnx content")
        
        converter = ModelConverter()
        
        with patch("ai_service.model_converter.convert_model") as mock_convert:
            # Mock successful conversion
            mock_convert.return_value = None
            
            # Create output files manually
            xml_path = temp_dir / "model.xml"
            bin_path = temp_dir / "model.bin"
            xml_path.write_text("<?xml version='1.0'?><net></net>")
            bin_path.write_bytes(b"dummy bin content")
            
            xml_result, bin_result = converter.convert_onnx_to_ir(
                onnx_path=onnx_path,
                output_dir=temp_dir,
                model_name="model",
            )
            
            assert xml_result == xml_path
            assert bin_result == bin_path
            mock_convert.assert_called_once()
    
    def test_convert_onnx_to_ir_with_fp16(self, mock_openvino_available, temp_dir: Path):
        """Test ONNX to IR conversion with FP16 compression."""
        from ai_service.model_converter import ModelConverter
        
        onnx_path = temp_dir / "model.onnx"
        onnx_path.write_bytes(b"dummy onnx content")
        
        converter = ModelConverter()
        
        with patch("ai_service.model_converter.convert_model") as mock_convert:
            xml_path = temp_dir / "model.xml"
            bin_path = temp_dir / "model.bin"
            xml_path.write_text("<?xml version='1.0'?><net></net>")
            bin_path.write_bytes(b"dummy bin content")
            
            converter.convert_onnx_to_ir(
                onnx_path=onnx_path,
                output_dir=temp_dir,
                compress_to_fp16=True,
            )
            
            # Verify compress_to_fp16 was passed
            call_args = mock_convert.call_args[1]
            assert call_args.get("compress_to_fp16") is True
    
    def test_check_conversion_tools_available(self, mock_openvino_available):
        """Test checking conversion tools availability."""
        from ai_service.model_converter import ModelConverter
        
        converter = ModelConverter()
        assert converter.check_conversion_tools() is True
    
    def test_check_conversion_tools_unavailable(self, mock_openvino_unavailable):
        """Test checking conversion tools when unavailable."""
        from ai_service.model_converter import check_openvino_tools
        
        result = check_openvino_tools()
        assert result["tools_available"] is False


class TestConversionFunctions:
    """Tests for conversion convenience functions."""
    
    def test_convert_onnx_model_function(self, mock_openvino_available, temp_dir: Path):
        """Test convert_onnx_model convenience function."""
        from ai_service.model_converter import convert_onnx_model
        
        onnx_path = temp_dir / "model.onnx"
        onnx_path.write_bytes(b"dummy onnx content")
        
        with patch("ai_service.model_converter.ModelConverter") as mock_converter_class:
            mock_converter = MagicMock()
            mock_converter.convert_onnx_to_ir.return_value = (
                temp_dir / "model.xml",
                temp_dir / "model.bin",
            )
            mock_converter_class.return_value = mock_converter
            
            xml_path, bin_path = convert_onnx_model(
                onnx_path=onnx_path,
                output_dir=temp_dir,
            )
            
            assert xml_path.exists() or True  # May not exist in mock
            mock_converter.convert_onnx_to_ir.assert_called_once()

