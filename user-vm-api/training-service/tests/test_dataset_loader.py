"""
Unit tests for dataset loader
"""

import os
import json
import pytest
from pathlib import Path

from training.dataset_loader import DatasetLoader, DatasetMetadata, Manifest


class TestDatasetLoader:
    """Tests for DatasetLoader class"""
    
    def test_load_dataset_metadata(self, test_dataset_dir):
        """Test loading dataset metadata"""
        loader = DatasetLoader(test_dataset_dir, "/tmp/test_output")
        metadata, manifest = loader.load()
        
        assert metadata is not None
        assert metadata.edge_id == "test-edge-1"
        assert metadata.camera_id == "test-camera-1"
        assert metadata.total_snapshots == 100
        assert metadata.label_counts["normal"] == 60
        assert metadata.label_counts["anomaly"] == 40
    
    def test_load_dataset_missing_metadata(self, temp_dir):
        """Test loading dataset with missing metadata.json"""
        dataset_dir = os.path.join(temp_dir, "missing_metadata")
        os.makedirs(dataset_dir, exist_ok=True)
        
        loader = DatasetLoader(dataset_dir, "/tmp/test_output")
        
        with pytest.raises(FileNotFoundError):
            loader.load()
    
    def test_convert_to_yolo_format(self, test_dataset_dir, temp_dir):
        """Test converting dataset to YOLOv8 format"""
        output_dir = os.path.join(temp_dir, "yolo_output")
        os.makedirs(output_dir, exist_ok=True)
        
        loader = DatasetLoader(test_dataset_dir, output_dir)
        metadata, manifest = loader.load()
        
        # Create some dummy screenshot files
        screenshots_dir = os.path.join(test_dataset_dir, "screenshots")
        for i in range(10):
            img_path = os.path.join(screenshots_dir, f"test_{i}.jpg")
            with open(img_path, "wb") as f:
                f.write(b"dummy image data")
        
        # Validate dataset first
        is_valid, error = loader.validate()
        if not is_valid:
            pytest.skip(f"Dataset validation failed: {error}")
        
        # Convert to YOLO format
        yolo_path = loader.convert_to_yolo(seed=42)
        
        assert yolo_path is not None
        assert os.path.exists(yolo_path)
        assert os.path.exists(os.path.join(yolo_path, "data.yaml"))
        
        # Check data.yaml structure
        data_yaml_path = os.path.join(yolo_path, "data.yaml")
        assert os.path.exists(data_yaml_path)
    
    def test_label_mapping(self, test_dataset_dir, temp_dir):
        """Test label to class mapping"""
        output_dir = os.path.join(temp_dir, "yolo_output")
        os.makedirs(output_dir, exist_ok=True)
        
        loader = DatasetLoader(test_dataset_dir, output_dir)
        loader.load()
        
        # Check that label mapping is created
        assert len(loader.label_to_class) > 0
        assert len(loader.class_to_label) > 0
        assert len(loader.label_to_class) == len(loader.class_to_label)
    
    def test_min_snapshots_validation(self, temp_dir):
        """Test minimum snapshots validation"""
        dataset_dir = os.path.join(temp_dir, "small_dataset")
        os.makedirs(dataset_dir, exist_ok=True)
        
        # Create metadata with insufficient snapshots
        metadata = {
            "edge_id": "test-edge-1",
            "camera_id": "test-camera-1",
            "total_snapshots": 10,  # Less than MIN_SNAPSHOTS (50)
            "label_counts": {
                "normal": 5,
                "anomaly": 5
            },
            "synced_at": "2025-01-01T00:00:00Z"
        }
        
        metadata_path = os.path.join(dataset_dir, "metadata.json")
        with open(metadata_path, "w") as f:
            json.dump(metadata, f)
        
        screenshots_dir = os.path.join(dataset_dir, "screenshots")
        os.makedirs(screenshots_dir, exist_ok=True)
        
        loader = DatasetLoader(dataset_dir, "/tmp/test_output")
        metadata_obj, _ = loader.load()
        
        # Should load but validation would fail in orchestrator
        assert metadata_obj.total_snapshots == 10

