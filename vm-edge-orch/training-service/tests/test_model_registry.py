"""
Unit tests for model registry
"""

import os
import json
import pytest
from unittest.mock import Mock, patch, MagicMock

from training.model_registry import ModelRegistry


class TestModelRegistry:
    """Tests for ModelRegistry class"""
    
    def test_register_trained_model_metadata_creation(self, temp_dir):
        """Test model metadata creation"""
        registry = ModelRegistry()
        
        # Create a dummy ONNX file
        onnx_path = os.path.join(temp_dir, "model.onnx")
        with open(onnx_path, "wb") as f:
            f.write(b"dummy onnx content" * 100)  # Make it non-empty
        
        model_output_dir = os.path.join(temp_dir, "model_output")
        os.makedirs(model_output_dir, exist_ok=True)
        
        with patch('training.model_registry.config.get_model_path', return_value=model_output_dir):
            with patch('training.model_registry.requests.post') as mock_post:
                # Mock successful API response
                mock_response = Mock()
                mock_response.status_code = 200
                mock_post.return_value = mock_response
                
                training_metrics = {
                    "train_loss": [0.8, 0.6, 0.5],
                    "val_loss": [0.9, 0.7, 0.6],
                    "map": [0.5, 0.6, 0.7],
                    "precision": [0.7, 0.8, 0.9],
                    "recall": [0.6, 0.7, 0.8],
                    "current_epoch": 3,
                    "total_epochs": 10,
                    "current_loss": 0.5,
                    "current_val_loss": 0.6,
                    "current_map": 0.7
                }
                
                result = registry.register_trained_model(
                    trained_model_id="test-model-1",
                    onnx_path=onnx_path,
                    baseline_model_id="baseline-yolov8n",
                    dataset_id="dataset-1",
                    camera_id="camera-1",
                    edge_id="edge-1",
                    model_type="yolov8n",
                    input_shape=[1, 3, 640, 640],
                    output_classes=2,
                    training_metrics=training_metrics,
                    image_size=640
                )
                
                assert result["registered"] is True
                assert result["model_id"] == "test-model-1"
                
                # Check metadata file was created
                metadata_path = os.path.join(model_output_dir, "metadata.json")
                assert os.path.exists(metadata_path)
                
                # Check metadata content
                with open(metadata_path, "r") as f:
                    metadata = json.load(f)
                
                assert metadata["model_id"] == "test-model-1"
                assert metadata["training_dataset_id"] == "dataset-1"
                assert metadata["camera_id"] == "camera-1"
                # Check that baseline_model_id and output_classes are in preprocessing
                assert metadata["preprocessing"]["baseline_model_id"] == "baseline-yolov8n"
                assert metadata["preprocessing"]["output_classes"] == 2
                assert "training_metrics" in metadata["preprocessing"]
    
    def test_register_trained_model_api_failure(self, temp_dir):
        """Test model registration with API failure"""
        registry = ModelRegistry()
        
        # Create a dummy ONNX file
        onnx_path = os.path.join(temp_dir, "model.onnx")
        with open(onnx_path, "wb") as f:
            f.write(b"dummy onnx content")
        
        model_output_dir = os.path.join(temp_dir, "model_output")
        os.makedirs(model_output_dir, exist_ok=True)
        
        with patch('training.model_registry.config.get_model_path', return_value=model_output_dir):
            with patch('training.model_registry.requests.post') as mock_post:
                # Mock API failure
                mock_post.side_effect = Exception("Connection error")
                
                result = registry.register_trained_model(
                    trained_model_id="test-model-1",
                    onnx_path=onnx_path,
                    baseline_model_id="baseline-yolov8n",
                    dataset_id="dataset-1",
                    camera_id="camera-1",
                    edge_id="edge-1",
                    model_type="yolov8n",
                    input_shape=[1, 3, 640, 640],
                    output_classes=2,
                    training_metrics={},
                    image_size=640
                )
                
                # Should still save locally even if API fails
                assert result["registered"] is False
                assert result["status"] == "partial_success"
                
                # Check metadata file was still created
                metadata_path = os.path.join(model_output_dir, "metadata.json")
                assert os.path.exists(metadata_path)
    
    def test_register_trained_model_missing_file(self, temp_dir):
        """Test registration with missing ONNX file"""
        registry = ModelRegistry()
        
        onnx_path = os.path.join(temp_dir, "nonexistent.onnx")
        
        with pytest.raises(ValueError, match="ONNX model file not found"):
            registry.register_trained_model(
                trained_model_id="test-model-1",
                onnx_path=onnx_path,
                baseline_model_id="baseline-yolov8n",
                dataset_id="dataset-1",
                camera_id="camera-1",
                edge_id="edge-1",
                model_type="yolov8n",
                input_shape=[1, 3, 640, 640],
                output_classes=2,
                training_metrics={},
                image_size=640
            )

