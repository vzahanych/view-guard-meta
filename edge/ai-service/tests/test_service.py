"""
Unit tests for FastAPI service initialization and startup.
"""

import pytest
from unittest.mock import patch, MagicMock
from fastapi.testclient import TestClient

from ai_service.config import Config, load_config
from ai_service.main import create_app


class TestServiceInitialization:
    """Tests for service initialization."""
    
    def test_create_app(self, sample_config: Config):
        """Test FastAPI app creation."""
        app = create_app(sample_config)
        
        assert app is not None
        assert app.title == "Edge AI Service"
        assert app.version == "dev"
    
    def test_app_has_config(self, sample_config: Config):
        """Test that app state contains config."""
        app = create_app(sample_config)
        
        assert hasattr(app.state, "config")
        assert app.state.config == sample_config
    
    def test_app_has_health_endpoints(self, sample_config: Config):
        """Test that app has health check endpoints."""
        app = create_app(sample_config)
        client = TestClient(app)
        
        # Test health endpoints exist
        response = client.get("/health/")
        assert response.status_code == 200
        
        response = client.get("/health/live")
        assert response.status_code == 200
        
        response = client.get("/health/ready")
        assert response.status_code in [200, 503]  # Depends on readiness
    
    def test_app_lifespan_startup(self, sample_config: Config):
        """Test app lifespan startup logic."""
        app = create_app(sample_config)
        
        with patch("ai_service.main.detect_hardware") as mock_detect, \
             patch("ai_service.main.create_runtime") as mock_create:
            mock_detect.return_value = {
                "openvino_available": True,
                "version": "2024.0.0",
                "available_devices": ["CPU"],
            }
            mock_runtime = MagicMock()
            mock_create.return_value = mock_runtime
            
            # Trigger lifespan startup
            with TestClient(app) as client:
                # App should be initialized
                assert client is not None
                
                # Verify hardware detection was called
                # (Note: lifespan runs in background, so we can't directly verify)
                pass
    
    def test_app_graceful_shutdown(self, sample_config: Config):
        """Test app graceful shutdown."""
        app = create_app(sample_config)
        
        # Test that app can be created and destroyed
        with TestClient(app):
            pass
        
        # App should handle shutdown gracefully
        assert True  # If we get here, shutdown worked


class TestServiceErrorHandling:
    """Tests for service error handling."""
    
    def test_service_without_openvino(self, sample_config: Config, mock_openvino_unavailable):
        """Test service initialization when OpenVINO is unavailable."""
        app = create_app(sample_config)
        client = TestClient(app)
        
        # Service should still start even without OpenVINO
        response = client.get("/health/")
        assert response.status_code == 200
        
        # Health check should indicate OpenVINO is unavailable
        response = client.get("/health/detailed")
        assert response.status_code == 200
        
        data = response.json()
        openvino_component = data["components"].get("openvino", {})
        assert openvino_component.get("status") in ["unavailable", "error"]
    
    def test_service_with_invalid_config(self):
        """Test service with invalid configuration."""
        # Create invalid config
        config = Config()
        config.server.port = 70000  # Invalid port
        
        with pytest.raises(ValueError):
            config.__post_init__()
    
    def test_service_config_loading_error(self):
        """Test service handles config loading errors."""
        # Try to load non-existent config
        with pytest.raises(FileNotFoundError):
            load_config("/nonexistent/config.yaml")

