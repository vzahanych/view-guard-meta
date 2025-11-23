"""
Unit tests for health check endpoints.
"""

import pytest
from fastapi.testclient import TestClient
from fastapi import FastAPI

from ai_service.health import (
    setup_health_endpoints,
    set_service_ready,
    is_service_ready,
    get_uptime_seconds,
    check_components,
)


class TestHealthFunctions:
    """Tests for health check utility functions."""
    
    def test_set_service_ready(self):
        """Test setting service readiness."""
        set_service_ready(False)
        assert not is_service_ready()
        
        set_service_ready(True)
        assert is_service_ready()
    
    def test_get_uptime_seconds(self):
        """Test getting uptime."""
        import time
        
        uptime1 = get_uptime_seconds()
        time.sleep(0.1)
        uptime2 = get_uptime_seconds()
        
        assert uptime2 > uptime1
        assert uptime2 >= 0.1
    
    def test_check_components(self):
        """Test component health checks."""
        components = check_components()
        
        assert "api" in components
        assert components["api"]["status"] == "healthy"
        assert "model" in components
        assert "inference" in components
        assert "openvino" in components


class TestHealthEndpoints:
    """Tests for health check endpoints."""
    
    @pytest.fixture
    def app(self):
        """Create FastAPI app with health endpoints."""
        app = FastAPI()
        setup_health_endpoints(app)
        return app
    
    @pytest.fixture
    def client(self, app):
        """Create test client."""
        return TestClient(app)
    
    def test_health_check_endpoint(self, client: TestClient):
        """Test basic health check endpoint."""
        response = client.get("/health/")
        assert response.status_code == 200
        
        data = response.json()
        assert data["status"] == "ok"
        assert "timestamp" in data
    
    def test_liveness_endpoint(self, client: TestClient):
        """Test liveness probe endpoint."""
        response = client.get("/health/live")
        assert response.status_code == 200
        
        data = response.json()
        assert data["status"] == "alive"
        assert "timestamp" in data
    
    def test_readiness_endpoint_ready(self, client: TestClient):
        """Test readiness probe endpoint when ready."""
        set_service_ready(True)
        
        response = client.get("/health/ready")
        assert response.status_code == 200
        
        data = response.json()
        assert data["status"] == "ready"
        assert "timestamp" in data
    
    def test_readiness_endpoint_not_ready(self, client: TestClient):
        """Test readiness probe endpoint when not ready."""
        set_service_ready(False)
        
        response = client.get("/health/ready")
        assert response.status_code == 503
        
        data = response.json()
        assert data["status"] == "not_ready"
        assert "timestamp" in data
    
    def test_detailed_health_endpoint(self, client: TestClient):
        """Test detailed health check endpoint."""
        set_service_ready(True)
        
        response = client.get("/health/detailed")
        assert response.status_code == 200
        
        data = response.json()
        assert "status" in data
        assert "timestamp" in data
        assert "uptime_seconds" in data
        assert "components" in data
        
        components = data["components"]
        assert "api" in components
        assert "model" in components
        assert "inference" in components
        assert "openvino" in components

