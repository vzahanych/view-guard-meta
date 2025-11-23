"""
Integration tests for HTTP API endpoints.

Tests the complete API integration including request handling and response formatting.
"""

import pytest
import threading
import time
from fastapi.testclient import TestClient

from ai_service.inference import InferenceEngine
from ai_service.detection import DetectionLogic


class TestAPIEndpoints:
    """Tests for HTTP API endpoints."""
    
    def test_inference_endpoint_integration(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """
        Test inference endpoint with real service.
        
        P0: Test AI service integration with Edge Orchestrator (HTTP/gRPC)
        """
        response = app_client.post(
            "/api/v1/inference",
            json={"image": base64_image},
        )
        
        assert response.status_code == 200
        data = response.json()
        
        # Verify response structure
        assert "bounding_boxes" in data
        assert "inference_time_ms" in data
        assert "frame_shape" in data
        assert "model_input_shape" in data
        assert "detection_count" in data
        
        # Verify bounding boxes structure
        assert isinstance(data["bounding_boxes"], list)
        for box in data["bounding_boxes"]:
            assert "x1" in box
            assert "y1" in box
            assert "x2" in box
            assert "y2" in box
            assert "confidence" in box
            assert "class_id" in box
            assert "class_name" in box
    
    def test_inference_endpoint_with_confidence_threshold(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """Test inference endpoint with confidence threshold override."""
        response = app_client.post(
            "/api/v1/inference",
            json={
                "image": base64_image,
                "confidence_threshold": 0.7,
            },
        )
        
        assert response.status_code == 200
        data = response.json()
        
        # Verify all detections meet threshold
        for box in data["bounding_boxes"]:
            assert box["confidence"] >= 0.7
    
    def test_inference_endpoint_with_class_filter(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """Test inference endpoint with class filtering."""
        response = app_client.post(
            "/api/v1/inference",
            json={
                "image": base64_image,
                "enabled_classes": ["person", "car"],
            },
        )
        
        assert response.status_code == 200
        data = response.json()
        
        # Verify all detections are in enabled classes
        for box in data["bounding_boxes"]:
            assert box["class_name"] in ["person", "car"]
    
    def test_batch_inference_endpoint(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """Test batch inference endpoint."""
        response = app_client.post(
            "/api/v1/inference/batch",
            json={
                "images": [base64_image, base64_image, base64_image],
            },
        )
        
        assert response.status_code == 200
        data = response.json()
        
        # Verify batch response structure
        assert "results" in data
        assert len(data["results"]) == 3
        assert "total_inference_time_ms" in data
        assert "average_inference_time_ms" in data
        
        # Verify each result
        for result in data["results"]:
            assert "bounding_boxes" in result
            assert "inference_time_ms" in result
    
    def test_file_upload_endpoint(
        self,
        app_client: TestClient,
        sample_frame,
    ):
        """Test file upload inference endpoint."""
        import cv2
        import io
        
        # Encode frame as JPEG
        success, encoded = cv2.imencode(".jpg", sample_frame)
        assert success
        
        image_bytes = encoded.tobytes()
        
        response = app_client.post(
            "/api/v1/inference/file",
            files={"file": ("test.jpg", image_bytes, "image/jpeg")},
        )
        
        assert response.status_code == 200
        data = response.json()
        assert "bounding_boxes" in data
    
    def test_inference_stats_endpoint(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """Test inference statistics endpoint."""
        # Perform some inferences first
        for _ in range(3):
            app_client.post(
                "/api/v1/inference",
                json={"image": base64_image},
            )
        
        # Get statistics
        response = app_client.get("/api/v1/inference/stats")
        
        assert response.status_code == 200
        data = response.json()
        
        assert "total_inferences" in data
        assert "total_time_ms" in data
        assert "average_time_ms" in data
        assert data["total_inferences"] >= 3


class TestConcurrentRequests:
    """Tests for concurrent inference requests."""
    
    def test_concurrent_inference_requests(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """
        Test concurrent inference requests.
        
        P0: Test concurrent inference requests
        """
        import concurrent.futures
        
        num_requests = 5
        results = []
        
        def make_request():
            response = app_client.post(
                "/api/v1/inference",
                json={"image": base64_image},
            )
            return response.status_code == 200
        
        # Execute concurrent requests
        with concurrent.futures.ThreadPoolExecutor(max_workers=num_requests) as executor:
            futures = [executor.submit(make_request) for _ in range(num_requests)]
            results = [future.result() for future in concurrent.futures.as_completed(futures)]
        
        # Verify all requests succeeded
        assert all(results)
        assert len(results) == num_requests
    
    def test_concurrent_batch_requests(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """Test concurrent batch inference requests."""
        import concurrent.futures
        
        num_requests = 3
        
        def make_batch_request():
            response = app_client.post(
                "/api/v1/inference/batch",
                json={"images": [base64_image, base64_image]},
            )
            return response.status_code == 200
        
        with concurrent.futures.ThreadPoolExecutor(max_workers=num_requests) as executor:
            futures = [executor.submit(make_batch_request) for _ in range(num_requests)]
            results = [future.result() for future in concurrent.futures.as_completed(futures)]
        
        assert all(results)


class TestServiceHealth:
    """Tests for service health and readiness."""
    
    def test_health_endpoint(
        self,
        app_client: TestClient,
    ):
        """Test basic health endpoint."""
        response = app_client.get("/health/")
        
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "ok"
    
    def test_liveness_endpoint(
        self,
        app_client: TestClient,
    ):
        """Test liveness probe endpoint."""
        response = app_client.get("/health/live")
        
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "alive"
    
    def test_readiness_endpoint_with_model(
        self,
        app_client: TestClient,
    ):
        """
        Test readiness endpoint with loaded model.
        
        P0: Test service health and readiness with loaded model
        """
        response = app_client.get("/health/ready")
        
        # Should be ready if model is loaded
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "ready"
    
    def test_detailed_health_endpoint(
        self,
        app_client: TestClient,
    ):
        """Test detailed health endpoint."""
        response = app_client.get("/health/detailed")
        
        assert response.status_code == 200
        data = response.json()
        
        assert "status" in data
        assert "uptime_seconds" in data
        assert "components" in data
        assert "openvino" in data["components"]

