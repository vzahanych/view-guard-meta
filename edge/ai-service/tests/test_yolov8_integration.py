"""
Unit tests for YOLOv8 model integration.
"""

import pytest
from pathlib import Path
from unittest.mock import Mock, MagicMock, patch


class TestYOLOv8Integration:
    """Tests for YOLOv8Integration class."""
    
    @pytest.fixture
    def model_dir(self, temp_dir: Path) -> Path:
        """Create model directory."""
        model_dir = temp_dir / "models"
        model_dir.mkdir()
        return model_dir
    
    @pytest.fixture
    def integration(self, model_dir: Path):
        """Create YOLOv8Integration instance."""
        from ai_service.yolov8_integration import YOLOv8Integration
        return YOLOv8Integration(model_dir=model_dir)
    
    def test_initialization(self, model_dir: Path):
        """Test YOLOv8Integration initialization."""
        from ai_service.yolov8_integration import YOLOv8Integration
        
        integration = YOLOv8Integration(model_dir=model_dir)
        assert integration.model_dir == model_dir
        assert integration.model_dir.exists()
    
    def test_download_model_unavailable(self, integration):
        """Test download when ultralytics is unavailable."""
        with patch("ai_service.yolov8_integration.ULTRALYTICS_AVAILABLE", False):
            with pytest.raises(RuntimeError, match="Ultralytics not available"):
                integration.download_model("yolov8n")
    
    def test_download_model_success(self, integration):
        """Test successful model download."""
        with patch("ai_service.yolov8_integration.ULTRALYTICS_AVAILABLE", True), \
             patch("ai_service.yolov8_integration.YOLO") as mock_yolo:
            mock_model = MagicMock()
            mock_yolo.return_value = mock_model
            
            result = integration.download_model("yolov8n")
            
            assert isinstance(result, Path)
            mock_yolo.assert_called_once_with("yolov8n.pt")
    
    def test_convert_to_onnx_file_not_found(self, integration):
        """Test ONNX conversion with non-existent file."""
        with patch("ai_service.yolov8_integration.ULTRALYTICS_AVAILABLE", True):
            with pytest.raises(FileNotFoundError):
                integration.convert_to_onnx("/nonexistent/model.pt")
    
    def test_convert_to_onnx_success(self, integration, temp_dir: Path):
        """Test successful ONNX conversion."""
        pt_file = temp_dir / "model.pt"
        pt_file.write_bytes(b"dummy pytorch model")
        
        with patch("ai_service.yolov8_integration.ULTRALYTICS_AVAILABLE", True), \
             patch("ai_service.yolov8_integration.YOLO") as mock_yolo:
            mock_model = MagicMock()
            mock_yolo.return_value = mock_model
            
            # Create output ONNX file
            onnx_file = temp_dir / "model.onnx"
            onnx_file.write_bytes(b"dummy onnx model")
            
            result = integration.convert_to_onnx(pt_file)
            
            assert isinstance(result, Path)
            mock_model.export.assert_called_once()
    
    def test_convert_to_openvino_ir(self, integration, temp_dir: Path):
        """Test ONNX to OpenVINO IR conversion."""
        onnx_file = temp_dir / "model.onnx"
        onnx_file.write_bytes(b"dummy onnx model")
        
        with patch("ai_service.yolov8_integration.convert_onnx_model") as mock_convert:
            xml_path = temp_dir / "model.xml"
            bin_path = temp_dir / "model.bin"
            xml_path.write_text("<?xml version='1.0'?><net></net>")
            bin_path.write_bytes(b"dummy bin")
            
            mock_convert.return_value = (xml_path, bin_path)
            
            result_xml, result_bin = integration.convert_to_openvino_ir(onnx_file)
            
            assert result_xml == xml_path
            assert result_bin == bin_path
            mock_convert.assert_called_once()
    
    def test_convert_to_openvino_ir_with_fp16(self, integration, temp_dir: Path):
        """Test ONNX to OpenVINO IR conversion with FP16."""
        onnx_file = temp_dir / "model.onnx"
        onnx_file.write_bytes(b"dummy onnx model")
        
        with patch("ai_service.yolov8_integration.convert_onnx_model") as mock_convert:
            xml_path = temp_dir / "model.xml"
            bin_path = temp_dir / "model.bin"
            xml_path.write_text("<?xml version='1.0'?><net></net>")
            bin_path.write_bytes(b"dummy bin")
            
            mock_convert.return_value = (xml_path, bin_path)
            
            integration.convert_to_openvino_ir(onnx_file, compress_to_fp16=True)
            
            # Verify FP16 was passed
            call_kwargs = mock_convert.call_args[1]
            assert call_kwargs.get("compress_to_fp16") is True
    
    def test_download_and_convert_to_onnx(self, integration):
        """Test download and convert to ONNX."""
        with patch.object(integration, "download_model") as mock_download, \
             patch.object(integration, "convert_to_onnx") as mock_convert:
            pt_path = Path("/models/yolov8n.pt")
            onnx_path = Path("/models/yolov8n.onnx")
            
            mock_download.return_value = pt_path
            mock_convert.return_value = onnx_path
            
            result_path, result_bin = integration.download_and_convert(
                model_name="yolov8n",
                target_format="onnx",
            )
            
            assert result_path == onnx_path
            assert result_bin is None
            mock_download.assert_called_once()
            mock_convert.assert_called_once()
    
    def test_download_and_convert_to_openvino(self, integration):
        """Test download and convert to OpenVINO IR."""
        with patch.object(integration, "download_model") as mock_download, \
             patch.object(integration, "convert_to_onnx") as mock_onnx, \
             patch.object(integration, "convert_to_openvino_ir") as mock_ir:
            pt_path = Path("/models/yolov8n.pt")
            onnx_path = Path("/models/yolov8n.onnx")
            xml_path = Path("/models/yolov8n.xml")
            bin_path = Path("/models/yolov8n.bin")
            
            mock_download.return_value = pt_path
            mock_onnx.return_value = onnx_path
            mock_ir.return_value = (xml_path, bin_path)
            
            result_path, result_bin = integration.download_and_convert(
                model_name="yolov8n",
                target_format="openvino",
            )
            
            assert result_path == xml_path
            assert result_bin == bin_path
            mock_download.assert_called_once()
            mock_onnx.assert_called_once()
            mock_ir.assert_called_once()
    
    def test_optimize_for_hardware(self, integration, temp_dir: Path):
        """Test model optimization for hardware."""
        model_path = temp_dir / "model.xml"
        model_path.write_text("<?xml version='1.0'?><net></net>")
        
        result = integration.optimize_for_hardware(
            model_path,
            device="GPU",
            compress_to_fp16=True,
        )
        
        assert result == model_path

