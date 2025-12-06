"""
Unit tests for model loader
"""

import os
import json
import pytest
from unittest.mock import Mock, patch, MagicMock

from training.model_loader import ModelLoader, ModelMetadata


class TestModelLoader:
    """Tests for ModelLoader class"""
    
    def test_load_model_from_local(self, test_model_dir):
        """Test loading model from local directory"""
        loader = ModelLoader("baseline-yolov8n", use_catalog_api=False)
        
        # Mock the model path
        with patch('training.model_loader.config.get_model_path', return_value=test_model_dir):
            with patch('ultralytics.YOLO') as mock_yolo:
                mock_model = MagicMock()
                mock_yolo.return_value = mock_model
                
                model, metadata = loader.load()
                
                assert model is not None
                assert metadata is not None
                assert metadata.model_id == "baseline-yolov8n"
                assert metadata.model_type == "yolov8n"
                assert metadata.framework == "onnx"
    
    @patch('training.model_loader.requests.get')
    def test_load_model_from_api(self, mock_get, test_model_dir):
        """Test loading model from catalog API"""
        # Mock API response
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "model_id": "baseline-yolov8n",
            "version": "1.0",
            "model_type": "yolov8n",
            "framework": "onnx",
            "input_shape": [1, 3, 640, 640],
            "onnx_file": "model.onnx",
            "status": "baseline"
        }
        mock_get.return_value = mock_response
        
        loader = ModelLoader("baseline-yolov8n", use_catalog_api=True)
        
        with patch('training.model_loader.config.get_model_path', return_value=test_model_dir):
            with patch('ultralytics.YOLO') as mock_yolo:
                mock_model = MagicMock()
                mock_yolo.return_value = mock_model
                
                model, metadata = loader.load()
                
                assert model is not None
                assert metadata is not None
                assert metadata.model_id == "baseline-yolov8n"
                mock_get.assert_called_once()
    
    def test_model_metadata_parsing(self, test_model_dir):
        """Test parsing model metadata"""
        metadata_path = os.path.join(test_model_dir, "metadata.json")
        
        with open(metadata_path, "r") as f:
            metadata_data = json.load(f)
        
        metadata = ModelMetadata(metadata_data)
        
        assert metadata.model_id == "baseline-yolov8n"
        assert metadata.model_type == "yolov8n"
        assert metadata.framework == "onnx"
        assert metadata.input_shape == [1, 3, 640, 640]
    
    def test_model_not_found(self, temp_dir):
        """Test loading non-existent model"""
        loader = ModelLoader("nonexistent-model", use_catalog_api=False)
        
        with patch('training.model_loader.config.get_model_path', return_value=os.path.join(temp_dir, "nonexistent")):
            with pytest.raises(FileNotFoundError):
                loader.load()
    
    @patch('training.model_loader.requests.get')
    def test_api_error_handling(self, mock_get):
        """Test handling API errors"""
        # Mock API error
        mock_response = Mock()
        mock_response.status_code = 404
        mock_response.raise_for_status.side_effect = Exception("Not found")
        mock_get.return_value = mock_response
        
        loader = ModelLoader("baseline-yolov8n", use_catalog_api=True)
        
        with pytest.raises(Exception):
            loader.load()

