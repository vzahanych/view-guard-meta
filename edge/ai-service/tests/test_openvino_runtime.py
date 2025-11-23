"""
Unit tests for OpenVINO runtime and hardware detection.
"""

import pytest
from unittest.mock import Mock, patch, MagicMock


class TestOpenVINORuntime:
    """Tests for OpenVINORuntime class."""
    
    @pytest.fixture
    def mock_openvino_core(self, monkeypatch):
        """Mock OpenVINO Core."""
        mock_core = MagicMock()
        mock_core.available_devices = ["CPU", "GPU"]
        mock_core.get_property = MagicMock(return_value="Test Device")
        mock_core.get_property.side_effect = lambda device, prop: {
            "FULL_DEVICE_NAME": "Intel Core i7",
            "OPTIMIZATION_CAPABILITIES": ["FP32", "FP16"],
        }.get(prop, "unknown")
        
        def mock_get_version():
            return "2024.0.0"
        
        monkeypatch.setattr("ai_service.openvino_runtime.Core", lambda: mock_core)
        monkeypatch.setattr("ai_service.openvino_runtime.get_version", mock_get_version)
        monkeypatch.setattr("ai_service.openvino_runtime.OPENVINO_AVAILABLE", True)
        
        return mock_core
    
    def test_runtime_initialization_available(self, mock_openvino_core):
        """Test runtime initialization when OpenVINO is available."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="CPU")
        
        assert runtime.core is not None
        assert "CPU" in runtime.available_devices
        assert runtime.selected_device == "CPU"
    
    def test_runtime_initialization_unavailable(self, mock_openvino_unavailable):
        """Test runtime initialization when OpenVINO is unavailable."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        with pytest.raises(RuntimeError, match="OpenVINO is not available"):
            OpenVINORuntime(device="CPU")
    
    def test_device_selection_auto(self, mock_openvino_core):
        """Test automatic device selection."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="AUTO")
        # Should prefer GPU if available, otherwise CPU
        assert runtime.selected_device in ["CPU", "GPU"]
    
    def test_device_selection_cpu(self, mock_openvino_core):
        """Test CPU device selection."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="CPU")
        assert runtime.selected_device == "CPU"
    
    def test_device_selection_fallback(self, mock_openvino_core):
        """Test device selection fallback when preferred device not available."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        # Mock only CPU available
        mock_openvino_core.available_devices = ["CPU"]
        
        runtime = OpenVINORuntime(device="GPU")
        # Should fallback to CPU
        assert runtime.selected_device == "CPU"
    
    def test_get_device_info(self, mock_openvino_core):
        """Test getting device information."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="CPU")
        device_info = runtime.get_device_info("CPU")
        
        assert isinstance(device_info, dict)
    
    def test_is_gpu_available(self, mock_openvino_core):
        """Test GPU availability check."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="AUTO")
        assert runtime.is_gpu_available() is True
    
    def test_is_cpu_available(self, mock_openvino_core):
        """Test CPU availability check."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="AUTO")
        assert runtime.is_cpu_available() is True
    
    def test_get_version(self, mock_openvino_core):
        """Test getting OpenVINO version."""
        from ai_service.openvino_runtime import OpenVINORuntime
        
        runtime = OpenVINORuntime(device="CPU")
        version = runtime.get_version()
        
        assert version == "2024.0.0"


class TestHardwareDetection:
    """Tests for hardware detection functions."""
    
    def test_detect_hardware_available(self, mock_openvino_available):
        """Test hardware detection when OpenVINO is available."""
        from ai_service.openvino_runtime import detect_hardware
        
        with patch("ai_service.openvino_runtime.OpenVINORuntime") as mock_runtime_class:
            mock_runtime = MagicMock()
            mock_runtime.get_version.return_value = "2024.0.0"
            mock_runtime.get_available_devices.return_value = ["CPU", "GPU"]
            mock_runtime.is_gpu_available.return_value = True
            mock_runtime.is_cpu_available.return_value = True
            mock_runtime.get_device.return_value = "CPU"
            mock_runtime.device_info = {"CPU": {}, "GPU": {}}
            mock_runtime_class.return_value = mock_runtime
            
            result = detect_hardware()
            
            assert result["openvino_available"] is True
            assert result["version"] == "2024.0.0"
            assert "CPU" in result["available_devices"]
            assert result["gpu_available"] is True
            assert result["cpu_available"] is True
    
    def test_detect_hardware_unavailable(self, mock_openvino_unavailable):
        """Test hardware detection when OpenVINO is unavailable."""
        from ai_service.openvino_runtime import detect_hardware
        
        result = detect_hardware()
        
        assert result["openvino_available"] is False
        assert result["version"] is None
        assert result["available_devices"] == []
        assert result["gpu_available"] is False
        assert result["cpu_available"] is False
    
    def test_create_runtime_available(self, mock_openvino_available):
        """Test creating runtime when OpenVINO is available."""
        from ai_service.openvino_runtime import create_runtime
        
        with patch("ai_service.openvino_runtime.OpenVINORuntime") as mock_runtime_class:
            mock_runtime = MagicMock()
            mock_runtime_class.return_value = mock_runtime
            
            runtime = create_runtime(device="CPU")
            
            assert runtime is not None
            mock_runtime_class.assert_called_once_with(device="CPU")
    
    def test_create_runtime_unavailable(self, mock_openvino_unavailable):
        """Test creating runtime when OpenVINO is unavailable."""
        from ai_service.openvino_runtime import create_runtime
        
        runtime = create_runtime(device="CPU")
        
        assert runtime is None

