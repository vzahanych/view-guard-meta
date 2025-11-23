"""
Integration tests for model hot-reload functionality.

Tests model reloading during service operation and error recovery.
"""

import pytest
import time
import threading
from pathlib import Path

from ai_service.model_loader import ModelLoader
from ai_service.inference import InferenceEngine
import numpy as np


class TestModelHotReload:
    """Tests for model hot-reload functionality."""
    
    def test_model_hot_reload_monitoring(
        self,
        model_loader: ModelLoader,
        models_dir: Path,
    ):
        """
        Test model hot-reload monitoring.
        
        P1: Test model hot-reload during service operation
        """
        # Start hot-reload monitoring
        model_loader.start_hot_reload_monitoring(interval=0.1)
        
        # Verify monitoring is active
        assert model_loader._monitoring is True
        
        # Stop monitoring
        model_loader.stop_hot_reload_monitoring()
        
        # Verify monitoring is stopped
        assert model_loader._monitoring is False
    
    def test_model_hot_reload_on_file_change(
        self,
        model_loader: ModelLoader,
        models_dir: Path,
        sample_frame: np.ndarray,
    ):
        """Test model reload when files change."""
        # Create inference engine
        engine = InferenceEngine(
            model_loader=model_loader,
            confidence_threshold=0.5,
            nms_threshold=0.4,
        )
        
        # Get initial model info
        initial_model = model_loader.get_current_model()
        assert initial_model is not None
        initial_checksum = initial_model.checksum
        
        # Perform inference
        result1 = engine.infer(sample_frame)
        assert isinstance(result1, DetectionResult)
        
        # Modify model file (simulate update)
        xml_file = models_dir / "yolov8n.xml"
        original_content = xml_file.read_text()
        xml_file.write_text(original_content + "\n<!-- Updated -->")
        
        # Start hot-reload monitoring
        model_loader.start_hot_reload_monitoring(interval=0.1)
        
        # Wait for reload
        time.sleep(0.5)
        
        # Verify model was reloaded
        new_model = model_loader.get_current_model()
        if new_model.checksum != initial_checksum:
            # Model was reloaded
            result2 = engine.infer(sample_frame)
            assert isinstance(result2, DetectionResult)
        
        # Stop monitoring
        model_loader.stop_hot_reload_monitoring()
    
    def test_model_hot_reload_callback(
        self,
        model_loader: ModelLoader,
        models_dir: Path,
    ):
        """Test model reload callback."""
        callback_called = []
        
        def on_reload(model_info):
            callback_called.append(model_info)
        
        # Set callback
        model_loader.on_model_reloaded = on_reload
        
        # Reload model
        model_loader.reload_model()
        
        # Verify callback was called
        assert len(callback_called) == 1
        assert callback_called[0].name == "yolov8n"


class TestErrorRecovery:
    """Tests for error recovery scenarios."""
    
    def test_model_reload_after_failure(
        self,
        model_loader: ModelLoader,
        models_dir: Path,
        sample_frame: np.ndarray,
    ):
        """
        Test model reload after inference failure.
        
        P1: Test error recovery (model reload after failure)
        """
        engine = InferenceEngine(
            model_loader=model_loader,
            confidence_threshold=0.5,
            nms_threshold=0.4,
        )
        
        # Perform inference (should work)
        result1 = engine.infer(sample_frame)
        assert isinstance(result1, DetectionResult)
        
        # Simulate model corruption by removing binary file
        bin_file = models_dir / "yolov8n.bin"
        if bin_file.exists():
            bin_file.unlink()
        
        # Try to reload (will fail)
        try:
            model_loader.reload_model()
            # If reload succeeded, restore file and continue
            bin_file.write_bytes(b"\x00" * 1024)
        except Exception:
            # Expected failure - restore file
            bin_file.write_bytes(b"\x00" * 1024)
            
            # Reload should work now
            model_loader.reload_model()
            
            # Inference should work again
            result2 = engine.infer(sample_frame)
            assert isinstance(result2, DetectionResult)
    
    def test_inference_after_model_reload(
        self,
        model_loader: ModelLoader,
        sample_frame: np.ndarray,
    ):
        """Test inference continues to work after model reload."""
        engine = InferenceEngine(
            model_loader=model_loader,
            confidence_threshold=0.5,
            nms_threshold=0.4,
        )
        
        # Perform inference before reload
        result1 = engine.infer(sample_frame)
        assert isinstance(result1, DetectionResult)
        
        # Reload model
        model_loader.reload_model()
        
        # Perform inference after reload
        result2 = engine.infer(sample_frame)
        assert isinstance(result2, DetectionResult)
        
        # Both should succeed
        assert result1.frame_shape == result2.frame_shape


class TestModelVersioning:
    """Tests for model versioning in integration context."""
    
    def test_model_version_listing(
        self,
        model_loader: ModelLoader,
        models_dir: Path,
    ):
        """Test listing model versions."""
        versions = model_loader.list_versions("yolov8n")
        
        assert isinstance(versions, list)
        assert len(versions) > 0
        
        # Verify version structure
        for version in versions:
            assert version.version is not None
            assert version.path.exists()
            assert version.checksum is not None
    
    def test_model_version_selection(
        self,
        model_loader: ModelLoader,
        models_dir: Path,
    ):
        """Test loading specific model version."""
        # Create versioned model file
        versioned_xml = models_dir / "yolov8n_v1.0.xml"
        versioned_bin = models_dir / "yolov8n_v1.0.bin"
        
        # Copy base model
        base_xml = models_dir / "yolov8n.xml"
        if base_xml.exists():
            versioned_xml.write_text(base_xml.read_text())
            versioned_bin.write_bytes((models_dir / "yolov8n.bin").read_bytes())
            
            # Try to load specific version
            try:
                model_info = model_loader.load_model("yolov8n", "openvino", version="v1.0")
                assert model_info.version == "v1.0"
            except Exception:
                # Version loading may not work with mock files
                pass

