"""
Unit tests for inference API endpoints.
"""

import base64
import pytest
import numpy as np
from unittest.mock import Mock, MagicMock, patch
from fastapi.testclient import TestClient
from fastapi import FastAPI

from ai_service.api import (
    setup_inference_endpoints,
    decode_image,
    encode_image,
    InferenceRequest,
    InferenceResponse,
)
from ai_service.inference import InferenceEngine, DetectionResult, BoundingBox
from ai_service.detection import DetectionLogic


class TestImageEncoding:
    """Tests for image encoding/decoding."""
    
    def test_decode_image_success(self):
        """Test successful image decoding."""
        with patch("ai_service.api.CV2_AVAILABLE", True), \
             patch("ai_service.api.cv2") as mock_cv2:
            # Create mock image
            mock_image = np.zeros((480, 640, 3), dtype=np.uint8)
            mock_cv2.imdecode.return_value = mock_image
            
            # Create base64 encoded dummy data
            image_bytes = b"dummy image data"
            encoded = base64.b64encode(image_bytes).decode("utf-8")
            
            result = decode_image(encoded)
            
            assert isinstance(result, np.ndarray)
            mock_cv2.imdecode.assert_called_once()
    
    def test_decode_image_failure(self):
        """Test image decoding failure."""
        with patch("ai_service.api.CV2_AVAILABLE", True), \
             patch("ai_service.api.cv2") as mock_cv2:
            mock_cv2.imdecode.return_value = None
            
            encoded = base64.b64encode(b"invalid").decode("utf-8")
            
            with pytest.raises(ValueError, match="Failed to decode image"):
                decode_image(encoded)
    
    def test_decode_image_opencv_unavailable(self):
        """Test decoding when OpenCV is unavailable."""
        with patch("ai_service.api.CV2_AVAILABLE", False):
            encoded = base64.b64encode(b"test").decode("utf-8")
            
            with pytest.raises(RuntimeError, match="OpenCV not available"):
                decode_image(encoded)
    
    def test_encode_image(self):
        """Test image encoding."""
        with patch("ai_service.api.CV2_AVAILABLE", True), \
             patch("ai_service.api.cv2") as mock_cv2:
            image = np.zeros((480, 640, 3), dtype=np.uint8)
            mock_cv2.imencode.return_value = (True, np.array([1, 2, 3], dtype=np.uint8))
            
            encoded = encode_image(image, format="JPEG")
            
            assert isinstance(encoded, str)
            mock_cv2.imencode.assert_called_once()


class TestInferenceEndpoints:
    """Tests for inference API endpoints."""
    
    @pytest.fixture
    def mock_inference_engine(self):
        """Create mock inference engine."""
        engine = MagicMock(spec=InferenceEngine)
        
        # Mock inference result
        result = DetectionResult(
            bounding_boxes=[
                BoundingBox(10, 10, 50, 50, 0.9, 0, "person"),
            ],
            inference_time_ms=10.0,
            frame_shape=(480, 640),
            model_input_shape=(640, 640),
        )
        engine.infer.return_value = result
        engine.infer_batch.return_value = [result]
        engine.get_statistics.return_value = {
            "total_inferences": 10,
            "total_time_ms": 100.0,
            "average_time_ms": 10.0,
        }
        
        return engine
    
    @pytest.fixture
    def mock_detection_logic(self):
        """Create mock detection logic."""
        logic = MagicMock(spec=DetectionLogic)
        logic.filter_detections = lambda x: x  # Pass through
        return logic
    
    @pytest.fixture
    def app(self, mock_inference_engine, mock_detection_logic):
        """Create FastAPI app with inference endpoints."""
        app = FastAPI()
        setup_inference_endpoints(app, mock_inference_engine, mock_detection_logic)
        return app
    
    @pytest.fixture
    def client(self, app):
        """Create test client."""
        return TestClient(app)
    
    @pytest.fixture
    def sample_image_base64(self):
        """Create sample base64-encoded image."""
        # Create a small test image
        image = np.zeros((100, 100, 3), dtype=np.uint8)
        with patch("ai_service.api.CV2_AVAILABLE", True), \
             patch("ai_service.api.cv2") as mock_cv2:
            mock_cv2.imencode.return_value = (True, np.array([1, 2, 3], dtype=np.uint8))
            return encode_image(image)
    
    def test_inference_endpoint_success(
        self,
        client: TestClient,
        mock_inference_engine,
        sample_image_base64,
    ):
        """Test successful inference endpoint."""
        with patch("ai_service.api.decode_image") as mock_decode:
            mock_decode.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            
            request = InferenceRequest(image=sample_image_base64)
            response = client.post("/api/v1/inference", json=request.dict())
            
            assert response.status_code == 200
            data = response.json()
            assert "bounding_boxes" in data
            assert "inference_time_ms" in data
            assert "detection_count" in data
    
    def test_inference_endpoint_invalid_image(self, client: TestClient):
        """Test inference endpoint with invalid image."""
        request = InferenceRequest(image="invalid_base64")
        
        with patch("ai_service.api.decode_image") as mock_decode:
            mock_decode.side_effect = ValueError("Invalid image")
            
            response = client.post("/api/v1/inference", json=request.dict())
            
            assert response.status_code == 400
    
    def test_inference_endpoint_model_error(self, client: TestClient, sample_image_base64):
        """Test inference endpoint with model error."""
        with patch("ai_service.api.decode_image") as mock_decode:
            mock_decode.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            
            # Mock inference engine to raise error
            with patch.object(client.app.state, "inference_engine") as mock_engine:
                mock_engine.infer.side_effect = RuntimeError("Model not loaded")
                
                request = InferenceRequest(image=sample_image_base64)
                response = client.post("/api/v1/inference", json=request.dict())
                
                assert response.status_code == 500
    
    def test_inference_endpoint_confidence_override(
        self,
        client: TestClient,
        mock_detection_logic,
        sample_image_base64,
    ):
        """Test inference endpoint with confidence threshold override."""
        with patch("ai_service.api.decode_image") as mock_decode:
            mock_decode.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            
            request = InferenceRequest(
                image=sample_image_base64,
                confidence_threshold=0.7,
            )
            response = client.post("/api/v1/inference", json=request.dict())
            
            assert response.status_code == 200
            mock_detection_logic.set_confidence_threshold.assert_called_with(0.7)
    
    def test_batch_inference_endpoint(
        self,
        client: TestClient,
        sample_image_base64,
    ):
        """Test batch inference endpoint."""
        with patch("ai_service.api.decode_image") as mock_decode:
            mock_decode.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            
            request = {
                "images": [sample_image_base64, sample_image_base64],
            }
            response = client.post("/api/v1/inference/batch", json=request)
            
            assert response.status_code == 200
            data = response.json()
            assert "results" in data
            assert len(data["results"]) == 2
            assert "total_inference_time_ms" in data
    
    def test_inference_file_endpoint(self, client: TestClient):
        """Test file upload inference endpoint."""
        with patch("ai_service.api.cv2") as mock_cv2:
            mock_cv2.imdecode.return_value = np.zeros((480, 640, 3), dtype=np.uint8)
            
            # Create dummy image file
            image_data = b"dummy image data"
            
            response = client.post(
                "/api/v1/inference/file",
                files={"file": ("test.jpg", image_data, "image/jpeg")},
            )
            
            assert response.status_code == 200
            data = response.json()
            assert "bounding_boxes" in data
    
    def test_inference_stats_endpoint(self, client: TestClient):
        """Test inference statistics endpoint."""
        response = client.get("/api/v1/inference/stats")
        
        assert response.status_code == 200
        data = response.json()
        assert "total_inferences" in data
        assert "average_time_ms" in data
    
    def test_reset_stats_endpoint(self, client: TestClient):
        """Test reset statistics endpoint."""
        response = client.post("/api/v1/inference/stats/reset")
        
        assert response.status_code == 200
        data = response.json()
        assert data["message"] == "Statistics reset"

