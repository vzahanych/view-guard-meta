"""
Unit tests for inference service.
"""

import pytest
import numpy as np
from unittest.mock import Mock, MagicMock, patch
from pathlib import Path

from ai_service.inference import (
    FramePreprocessor,
    PostProcessor,
    InferenceEngine,
    BoundingBox,
    DetectionResult,
)


class TestFramePreprocessor:
    """Tests for FramePreprocessor."""
    
    def test_preprocessor_initialization(self):
        """Test preprocessor initialization."""
        preprocessor = FramePreprocessor(target_size=(640, 640))
        assert preprocessor.target_size == (640, 640)
        assert preprocessor.target_width == 640
        assert preprocessor.target_height == 640
    
    def test_preprocess_basic(self):
        """Test basic frame preprocessing."""
        with patch("ai_service.inference.CV2_AVAILABLE", True), \
             patch("ai_service.inference.cv2") as mock_cv2:
            # Create mock frame
            frame = np.zeros((480, 640, 3), dtype=np.uint8)
            
            # Mock OpenCV functions
            mock_cv2.resize.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            mock_cv2.COLOR_BGR2RGB = 4
            
            preprocessor = FramePreprocessor(target_size=(640, 640))
            preprocessed, scale, padding = preprocessor.preprocess(frame)
            
            assert preprocessed.shape[0] == 1  # Batch dimension
            assert preprocessed.shape[1] == 3  # Channels
            assert isinstance(scale, float)
            assert isinstance(padding, tuple)
            assert len(padding) == 2
    
    def test_preprocess_opencv_unavailable(self):
        """Test preprocessing when OpenCV is unavailable."""
        with patch("ai_service.inference.CV2_AVAILABLE", False):
            preprocessor = FramePreprocessor()
            frame = np.zeros((480, 640, 3), dtype=np.uint8)
            
            with pytest.raises(RuntimeError, match="OpenCV not available"):
                preprocessor.preprocess(frame)
    
    def test_preprocess_batch(self):
        """Test batch preprocessing."""
        with patch("ai_service.inference.CV2_AVAILABLE", True), \
             patch("ai_service.inference.cv2") as mock_cv2:
            frames = [
                np.zeros((480, 640, 3), dtype=np.uint8),
                np.zeros((360, 480, 3), dtype=np.uint8),
            ]
            
            mock_cv2.resize.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            mock_cv2.COLOR_BGR2RGB = 4
            
            preprocessor = FramePreprocessor()
            batch, scales, paddings = preprocessor.preprocess_batch(frames)
            
            assert batch.shape[0] == 2  # Batch size
            assert len(scales) == 2
            assert len(paddings) == 2


class TestPostProcessor:
    """Tests for PostProcessor."""
    
    def test_postprocessor_initialization(self):
        """Test postprocessor initialization."""
        postprocessor = PostProcessor(
            confidence_threshold=0.5,
            nms_threshold=0.4,
        )
        assert postprocessor.confidence_threshold == 0.5
        assert postprocessor.nms_threshold == 0.4
        assert len(postprocessor.class_names) > 0
    
    def test_process_output_empty(self):
        """Test processing empty output."""
        with patch("ai_service.inference.CV2_AVAILABLE", True):
            postprocessor = PostProcessor()
            output = np.zeros((1, 0, 84))
            
            boxes = postprocessor.process_output(
                output=output,
                scale=1.0,
                padding=(0, 0),
                original_shape=(480, 640),
            )
            
            assert len(boxes) == 0
    
    def test_process_output_with_detections(self):
        """Test processing output with detections."""
        with patch("ai_service.inference.CV2_AVAILABLE", True), \
             patch.object(PostProcessor, "_apply_nms") as mock_nms:
            postprocessor = PostProcessor(confidence_threshold=0.3)
            
            # Create mock output with one detection
            output = np.zeros((1, 1, 84))
            output[0, 0, 0] = 0.5  # x_center
            output[0, 0, 1] = 0.5  # y_center
            output[0, 0, 2] = 0.2  # width
            output[0, 0, 3] = 0.2  # height
            output[0, 0, 4] = 0.8  # class 0 confidence (person)
            
            mock_nms.return_value = []
            
            boxes = postprocessor.process_output(
                output=output,
                scale=1.0,
                padding=(0, 0),
                original_shape=(640, 640),
            )
            
            # NMS returns empty, so no boxes
            assert len(boxes) == 0
    
    def test_apply_nms_simple(self):
        """Test simple NMS application."""
        with patch("ai_service.inference.CV2_AVAILABLE", True):
            postprocessor = PostProcessor()
            
            boxes = [
                BoundingBox(10, 10, 50, 50, 0.9, 0, "person"),
                BoundingBox(12, 12, 52, 52, 0.7, 0, "person"),  # Overlapping
                BoundingBox(100, 100, 150, 150, 0.8, 1, "car"),  # Non-overlapping
            ]
            
            filtered = postprocessor._apply_nms(boxes)
            
            # Should keep highest confidence boxes
            assert len(filtered) <= len(boxes)
    
    def test_calculate_iou(self):
        """Test IoU calculation."""
        postprocessor = PostProcessor()
        
        box1 = BoundingBox(10, 10, 50, 50, 0.9, 0, "person")
        box2 = BoundingBox(12, 12, 52, 52, 0.7, 0, "person")
        
        iou = postprocessor._calculate_iou(box1, box2)
        
        assert 0.0 <= iou <= 1.0
        assert iou > 0.0  # Should have some overlap


class TestInferenceEngine:
    """Tests for InferenceEngine."""
    
    @pytest.fixture
    def mock_model_loader(self):
        """Create mock model loader."""
        loader = MagicMock()
        loader.get_compiled_model.return_value = None
        loader.get_current_model.return_value = None
        return loader
    
    @pytest.fixture
    def mock_compiled_model(self):
        """Create mock compiled model."""
        model = MagicMock()
        
        # Mock inference result
        output_tensor = MagicMock()
        output_tensor.data = np.zeros((1, 84, 8400))  # YOLOv8 output shape
        model.return_value = {"output": output_tensor}
        
        return model
    
    def test_engine_initialization(self, mock_model_loader):
        """Test inference engine initialization."""
        engine = InferenceEngine(
            model_loader=mock_model_loader,
            confidence_threshold=0.5,
            nms_threshold=0.4,
        )
        
        assert engine.model_loader == mock_model_loader
        assert engine.preprocessor is not None
        assert engine.postprocessor is not None
    
    def test_infer_model_not_loaded(self, mock_model_loader):
        """Test inference when model is not loaded."""
        engine = InferenceEngine(model_loader=mock_model_loader)
        frame = np.zeros((480, 640, 3), dtype=np.uint8)
        
        with pytest.raises(RuntimeError, match="Model not loaded"):
            engine.infer(frame)
    
    def test_infer_success(self, mock_model_loader, mock_compiled_model):
        """Test successful inference."""
        with patch("ai_service.inference.CV2_AVAILABLE", True), \
             patch("ai_service.inference.cv2") as mock_cv2:
            # Setup mocks
            mock_model_loader.get_compiled_model.return_value = mock_compiled_model
            mock_model_loader.get_current_model.return_value = MagicMock()
            
            mock_cv2.resize.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            mock_cv2.COLOR_BGR2RGB = 4
            
            engine = InferenceEngine(model_loader=mock_model_loader)
            frame = np.zeros((480, 640, 3), dtype=np.uint8)
            
            result = engine.infer(frame)
            
            assert isinstance(result, DetectionResult)
            assert isinstance(result.bounding_boxes, list)
            assert result.inference_time_ms > 0
            assert result.frame_shape == (480, 640)
    
    def test_infer_batch(self, mock_model_loader, mock_compiled_model):
        """Test batch inference."""
        with patch("ai_service.inference.CV2_AVAILABLE", True), \
             patch("ai_service.inference.cv2") as mock_cv2:
            mock_model_loader.get_compiled_model.return_value = mock_compiled_model
            mock_model_loader.get_current_model.return_value = MagicMock()
            
            mock_cv2.resize.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            mock_cv2.COLOR_BGR2RGB = 4
            
            engine = InferenceEngine(model_loader=mock_model_loader)
            frames = [
                np.zeros((480, 640, 3), dtype=np.uint8),
                np.zeros((360, 480, 3), dtype=np.uint8),
            ]
            
            results = engine.infer_batch(frames)
            
            assert len(results) == 2
            assert all(isinstance(r, DetectionResult) for r in results)
    
    def test_get_statistics(self, mock_model_loader):
        """Test getting inference statistics."""
        engine = InferenceEngine(model_loader=mock_model_loader)
        
        stats = engine.get_statistics()
        
        assert "total_inferences" in stats
        assert "total_time_ms" in stats
        assert "average_time_ms" in stats
        assert stats["total_inferences"] == 0
    
    def test_reset_statistics(self, mock_model_loader):
        """Test resetting statistics."""
        engine = InferenceEngine(model_loader=mock_model_loader)
        
        engine._inference_count = 10
        engine._total_inference_time = 100.0
        
        engine.reset_statistics()
        
        assert engine._inference_count == 0
        assert engine._total_inference_time == 0.0

