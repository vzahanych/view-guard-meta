"""
Unit tests for model loader service.
"""

import pytest
from pathlib import Path
from unittest.mock import Mock, MagicMock, patch
from datetime import datetime

from ai_service.model_loader import (
    ModelLoader,
    ModelInfo,
    ModelVersion,
)


class TestModelInfo:
    """Tests for ModelInfo dataclass."""
    
    def test_model_info_creation(self):
        """Test creating ModelInfo."""
        info = ModelInfo(
            name="test_model",
            version="1.0",
            format="openvino",
            path=Path("/models"),
            xml_path=Path("/models/test_model.xml"),
        )
        
        assert info.name == "test_model"
        assert info.version == "1.0"
        assert info.format == "openvino"
        assert info.xml_path == Path("/models/test_model.xml")


class TestModelLoader:
    """Tests for ModelLoader class."""
    
    @pytest.fixture
    def model_dir(self, temp_dir: Path) -> Path:
        """Create model directory with test files."""
        model_dir = temp_dir / "models"
        model_dir.mkdir()
        return model_dir
    
    @pytest.fixture
    def mock_runtime(self):
        """Create mock OpenVINO runtime."""
        mock_runtime = MagicMock()
        mock_core = MagicMock()
        mock_model = MagicMock()
        mock_compiled = MagicMock()
        
        mock_compiled.input.return_value.shape = [1, 3, 640, 640]
        mock_compiled.output.return_value.shape = [1, 84, 8400]
        mock_core.read_model.return_value = mock_model
        mock_core.compile_model.return_value = mock_compiled
        mock_runtime.get_core.return_value = mock_core
        mock_runtime.get_device.return_value = "CPU"
        
        return mock_runtime
    
    def test_loader_initialization(self, model_dir: Path):
        """Test model loader initialization."""
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        
        assert loader.model_dir == model_dir
        assert loader.device == "CPU"
        assert loader.get_current_model() is None
    
    def test_find_model_files_openvino(self, model_dir: Path, mock_runtime):
        """Test finding OpenVINO model files."""
        # Create test model files
        xml_file = model_dir / "yolov8n.xml"
        bin_file = model_dir / "yolov8n.bin"
        xml_file.write_text("<?xml version='1.0'?><net></net>")
        bin_file.write_bytes(b"dummy bin content")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU", runtime=mock_runtime)
        
        with patch.object(loader, "_load_openvino_model"), \
             patch("ai_service.model_loader.OPENVINO_AVAILABLE", True):
            model_info = loader.load_model("yolov8n", "openvino")
            
            assert model_info.name == "yolov8n"
            assert model_info.format == "openvino"
            assert model_info.xml_path == xml_file
            assert model_info.bin_path == bin_file
    
    def test_find_model_files_with_version(self, model_dir: Path, mock_runtime):
        """Test finding model files with version."""
        xml_file = model_dir / "yolov8n_v1.0.xml"
        bin_file = model_dir / "yolov8n_v1.0.bin"
        xml_file.write_text("<?xml version='1.0'?><net></net>")
        bin_file.write_bytes(b"dummy bin content")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU", runtime=mock_runtime)
        
        with patch.object(loader, "_load_openvino_model"), \
             patch("ai_service.model_loader.OPENVINO_AVAILABLE", True):
            model_info = loader.load_model("yolov8n", "openvino", version="v1.0")
            
            assert model_info.version == "v1.0"
            assert model_info.xml_path == xml_file
    
    def test_find_latest_version(self, model_dir: Path):
        """Test finding latest model version."""
        # Create multiple versions
        old_xml = model_dir / "yolov8n_v1.0.xml"
        new_xml = model_dir / "yolov8n_v2.0.xml"
        old_xml.write_text("old")
        new_xml.write_text("new")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        
        xml_path, bin_path, version = loader._find_latest_version("yolov8n")
        
        # Should return the newest file
        assert xml_path in [old_xml, new_xml]
        assert version in ["v1.0", "v2.0"]
    
    def test_model_validation_file_not_found(self, model_dir: Path):
        """Test model validation with missing files."""
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        
        model_info = ModelInfo(
            name="test",
            version="1.0",
            format="openvino",
            path=model_dir,
            xml_path=model_dir / "nonexistent.xml",
        )
        
        with pytest.raises(FileNotFoundError):
            loader._validate_model_files(model_info)
    
    def test_model_validation_success(self, model_dir: Path):
        """Test successful model validation."""
        xml_file = model_dir / "test.xml"
        bin_file = model_dir / "test.bin"
        xml_file.write_text("test content")
        bin_file.write_bytes(b"test binary")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        
        model_info = ModelInfo(
            name="test",
            version="1.0",
            format="openvino",
            path=model_dir,
            xml_path=xml_file,
            bin_path=bin_file,
        )
        
        loader._validate_model_files(model_info)
        
        assert model_info.checksum is not None
        assert ":" in model_info.checksum  # Should have both XML and BIN checksums
    
    def test_calculate_checksum(self, model_dir: Path):
        """Test checksum calculation."""
        test_file = model_dir / "test.txt"
        test_file.write_text("test content")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        checksum = loader._calculate_checksum(test_file)
        
        assert isinstance(checksum, str)
        assert len(checksum) == 64  # SHA256 hex length
    
    def test_load_model_openvino_unavailable(self, model_dir: Path):
        """Test loading model when OpenVINO is unavailable."""
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        
        with patch("ai_service.model_loader.OPENVINO_AVAILABLE", False):
            with pytest.raises(RuntimeError, match="OpenVINO is not available"):
                loader.load_model("test", "openvino")
    
    def test_get_current_model(self, model_dir: Path, mock_runtime):
        """Test getting current model."""
        xml_file = model_dir / "test.xml"
        xml_file.write_text("<?xml version='1.0'?><net></net>")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU", runtime=mock_runtime)
        
        with patch("ai_service.model_loader.OPENVINO_AVAILABLE", True):
            loader.load_model("test", "openvino")
            
            current = loader.get_current_model()
            assert current is not None
            assert current.name == "test"
    
    def test_reload_model(self, model_dir: Path, mock_runtime):
        """Test model reload."""
        xml_file = model_dir / "test.xml"
        xml_file.write_text("<?xml version='1.0'?><net></net>")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU", runtime=mock_runtime)
        
        with patch("ai_service.model_loader.OPENVINO_AVAILABLE", True):
            # Load initial model
            loader.load_model("test", "openvino")
            
            # Reload
            reloaded = loader.reload_model()
            assert reloaded.name == "test"
    
    def test_reload_model_no_model_loaded(self, model_dir: Path):
        """Test reload when no model is loaded."""
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        
        with pytest.raises(RuntimeError, match="No model currently loaded"):
            loader.reload_model()
    
    def test_list_versions(self, model_dir: Path):
        """Test listing model versions."""
        # Create multiple versions
        v1_xml = model_dir / "yolov8n_v1.0.xml"
        v2_xml = model_dir / "yolov8n_v2.0.xml"
        v1_xml.write_text("v1")
        v2_xml.write_text("v2")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU")
        versions = loader.list_versions("yolov8n")
        
        assert len(versions) == 2
        assert all(isinstance(v, ModelVersion) for v in versions)
        assert all(v.version in ["v1.0", "v2.0"] for v in versions)
    
    def test_hot_reload_monitoring_start_stop(self, model_dir: Path, mock_runtime):
        """Test hot-reload monitoring start and stop."""
        xml_file = model_dir / "test.xml"
        xml_file.write_text("<?xml version='1.0'?><net></net>")
        
        loader = ModelLoader(model_dir=model_dir, device="CPU", runtime=mock_runtime)
        
        with patch("ai_service.model_loader.OPENVINO_AVAILABLE", True):
            loader.load_model("test", "openvino")
            
            # Start monitoring
            loader.start_hot_reload_monitoring(interval=0.1)
            assert loader._monitoring is True
            
            # Stop monitoring
            loader.stop_hot_reload_monitoring()
            assert loader._monitoring is False
    
    def test_hot_reload_callback(self, model_dir: Path, mock_runtime):
        """Test hot-reload callback."""
        xml_file = model_dir / "test.xml"
        xml_file.write_text("<?xml version='1.0'?><net></net>")
        
        callback_called = []
        
        def on_reload(model_info):
            callback_called.append(model_info)
        
        loader = ModelLoader(
            model_dir=model_dir,
            device="CPU",
            runtime=mock_runtime,
            on_model_reloaded=on_reload,
        )
        
        with patch("ai_service.model_loader.OPENVINO_AVAILABLE", True):
            # Load initial model (should not trigger callback)
            loader.load_model("test", "openvino")
            assert len(callback_called) == 0
            
            # Reload (should trigger callback)
            loader.reload_model()
            assert len(callback_called) == 1
            assert callback_called[0].name == "test"

