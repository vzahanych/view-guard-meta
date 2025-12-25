"""
Dataset loader for YOLOv8 training
Converts labeled snapshot datasets to YOLOv8 format for classification training
"""

import json
import os
import random
import shutil
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import logging

from PIL import Image

logger = logging.getLogger(__name__)


class DatasetMetadata:
    """Dataset metadata from metadata.json"""
    
    def __init__(self, data: dict):
        self.edge_id: str = data.get("edge_id", "")
        self.camera_id: str = data.get("camera_id", "")
        self.total_snapshots: int = data.get("total_snapshots", 0)
        self.label_counts: Dict[str, int] = data.get("label_counts", {})
        self.synced_at: str = data.get("synced_at", "")


class ManifestEntry:
    """Manifest entry for a single screenshot"""
    
    def __init__(self, data: dict):
        self.screenshot_id: str = data.get("screenshot_id", "")
        self.label: str = data.get("label", "normal")
        self.custom_label: Optional[str] = data.get("custom_label")
        self.description: Optional[str] = data.get("description")
        self.created_at: str = data.get("created_at", "")
        self.file_path: str = data.get("file_path", "")


class Manifest:
    """Manifest containing all screenshots"""
    
    def __init__(self, data: dict):
        self.camera_id: str = data.get("camera_id", "")
        self.synced_at: str = data.get("synced_at", "")
        self.screenshots: List[ManifestEntry] = [
            ManifestEntry(entry) for entry in data.get("screenshots", [])
        ]


class DatasetLoader:
    """Loads and converts datasets to YOLOv8 format"""
    
    MIN_SNAPSHOTS = 5
    TRAIN_SPLIT = 0.8  # 80% train, 20% val
    
    def __init__(self, dataset_path: str, output_path: str):
        """
        Initialize dataset loader
        
        Args:
            dataset_path: Path to dataset directory (contains metadata.json, screenshots/, manifest.json)
            output_path: Path where YOLOv8 formatted dataset will be created
        """
        self.dataset_path = Path(dataset_path)
        self.output_path = Path(output_path)
        self.metadata: Optional[DatasetMetadata] = None
        self.manifest: Optional[Manifest] = None
        self.label_to_class: Dict[str, int] = {}
        self.class_to_label: Dict[int, str] = {}
        
    def load(self) -> Tuple[DatasetMetadata, Manifest]:
        """
        Load dataset metadata and manifest
        
        Returns:
            Tuple of (DatasetMetadata, Manifest)
            
        Raises:
            FileNotFoundError: If metadata.json or screenshots directory not found
            ValueError: If dataset structure is invalid
        """
        # Load metadata.json
        metadata_path = self.dataset_path / "metadata.json"
        if not metadata_path.exists():
            raise FileNotFoundError(f"metadata.json not found at {metadata_path}")
        
        with open(metadata_path, "r") as f:
            metadata_data = json.load(f)
        self.metadata = DatasetMetadata(metadata_data)
        
        # Load manifest.json (optional)
        manifest_path = self.dataset_path / "manifest.json"
        if manifest_path.exists():
            with open(manifest_path, "r") as f:
                manifest_data = json.load(f)
            self.manifest = Manifest(manifest_data)
        else:
            logger.warning(f"manifest.json not found at {manifest_path}, will scan screenshots directory")
            self.manifest = None
        
        # Verify screenshots directory exists
        screenshots_dir = self.dataset_path / "screenshots"
        if not screenshots_dir.exists() or not screenshots_dir.is_dir():
            raise FileNotFoundError(f"screenshots directory not found at {screenshots_dir}")
        
        return self.metadata, self.manifest
    
    def validate(self) -> Tuple[bool, Optional[str]]:
        """
        Validate dataset for training
        
        Returns:
            Tuple of (is_valid, error_message)
        """
        if self.metadata is None:
            return False, "Dataset metadata not loaded"
        
        # Check minimum snapshot count
        if self.metadata.total_snapshots < self.MIN_SNAPSHOTS:
            return False, f"Dataset has {self.metadata.total_snapshots} snapshots, minimum {self.MIN_SNAPSHOTS} required"
        
        # Check label distribution
        if not self.metadata.label_counts:
            return False, "No labels found in dataset"
        
        # Check if at least one class has sufficient samples
        # For testing with small datasets, use 20% of MIN_SNAPSHOTS, but at least 2
        min_samples_per_class = max(2, self.MIN_SNAPSHOTS // 5)  # At least 2 or 20% of minimum
        has_sufficient_samples = any(
            count >= min_samples_per_class 
            for count in self.metadata.label_counts.values()
        )
        if not has_sufficient_samples:
            return False, f"No class has sufficient samples (minimum {min_samples_per_class} per class)"
        
        # Verify screenshots directory has images
        screenshots_dir = self.dataset_path / "screenshots"
        image_files = list(screenshots_dir.glob("*.jpg")) + list(screenshots_dir.glob("*.jpeg"))
        if len(image_files) < self.MIN_SNAPSHOTS:
            return False, f"Only {len(image_files)} image files found, minimum {self.MIN_SNAPSHOTS} required"
        
        return True, None
    
    def _build_label_mapping(self) -> None:
        """Build mapping between labels and class indices"""
        if self.metadata is None:
            return
        
        # Sort labels to ensure consistent mapping
        # "normal" always maps to class 0
        labels = sorted(self.metadata.label_counts.keys())
        
        # Ensure "normal" is first if it exists
        if "normal" in labels:
            labels.remove("normal")
            labels.insert(0, "normal")
        
        self.label_to_class = {label: idx for idx, label in enumerate(labels)}
        self.class_to_label = {idx: label for label, idx in self.label_to_class.items()}
        
        logger.info(f"Label mapping: {self.label_to_class}")
    
    def _get_image_label(self, screenshot_id: str) -> str:
        """
        Get label for a screenshot
        
        Args:
            screenshot_id: Screenshot ID
            
        Returns:
            Label string (defaults to "normal")
        """
        # Try to get from manifest first
        if self.manifest:
            for entry in self.manifest.screenshots:
                if entry.screenshot_id == screenshot_id:
                    # Use label for training (custom_label is just for user reference)
                    return entry.label if entry.label else "normal"
        
        # Fallback: check if we can infer from metadata
        # For PoC, assume all are "normal" if manifest not available
        return "normal"
    
    def _get_all_images(self) -> List[Tuple[str, str]]:
        """
        Get all images with their labels
        
        Returns:
            List of (screenshot_id, label) tuples
        """
        screenshots_dir = self.dataset_path / "screenshots"
        images = []
        
        # Get all image files
        image_files = list(screenshots_dir.glob("*.jpg")) + list(screenshots_dir.glob("*.jpeg"))
        
        for image_file in image_files:
            # Extract screenshot_id from filename (remove extension)
            screenshot_id = image_file.stem
            
            # Get label for this screenshot
            label = self._get_image_label(screenshot_id)
            
            images.append((screenshot_id, label))
        
        return images
    
    def convert_to_yolo(self, seed: Optional[int] = None) -> str:
        """
        Convert dataset to YOLOv8 classification format
        
        YOLOv8 classification format:
        - dataset/
          - train/
            - class0/  (e.g., normal/)
              - image1.jpg
              - image2.jpg
            - class1/  (e.g., anomaly/)
              - image1.jpg
          - val/
            - class0/
              - image1.jpg
            - class1/
              - image1.jpg
          - data.yaml
        
        Args:
            seed: Random seed for train/val split (for reproducibility)
            
        Returns:
            Path to created YOLOv8 dataset directory
        """
        if self.metadata is None:
            raise ValueError("Dataset metadata not loaded. Call load() first.")
        
        # Build label mapping
        self._build_label_mapping()
        
        # Get all images with labels
        images = self._get_all_images()
        
        if len(images) < self.MIN_SNAPSHOTS:
            raise ValueError(f"Insufficient images: {len(images)} < {self.MIN_SNAPSHOTS}")
        
        # Shuffle for train/val split
        if seed is not None:
            random.seed(seed)
        random.shuffle(images)
        
        # Split into train and val
        split_idx = int(len(images) * self.TRAIN_SPLIT)
        train_images = images[:split_idx]
        val_images = images[split_idx:]
        
        logger.info(f"Splitting {len(images)} images: {len(train_images)} train, {len(val_images)} val")
        
        # Create output directory structure
        train_dir = self.output_path / "train"
        val_dir = self.output_path / "val"
        
        # Create class directories
        for class_idx, label in self.class_to_label.items():
            (train_dir / label).mkdir(parents=True, exist_ok=True)
            (val_dir / label).mkdir(parents=True, exist_ok=True)
        
        # Copy train images
        screenshots_dir = self.dataset_path / "screenshots"
        for screenshot_id, label in train_images:
            src = screenshots_dir / f"{screenshot_id}.jpg"
            if not src.exists():
                # Try .jpeg extension
                src = screenshots_dir / f"{screenshot_id}.jpeg"
            if not src.exists():
                logger.warning(f"Image not found: {screenshot_id}")
                continue
            
            # Verify image is valid
            try:
                img = Image.open(src)
                img.verify()
            except Exception as e:
                logger.warning(f"Invalid image {screenshot_id}: {e}")
                continue
            
            dst = train_dir / label / f"{screenshot_id}.jpg"
            shutil.copy2(src, dst)
        
        # Copy val images
        for screenshot_id, label in val_images:
            src = screenshots_dir / f"{screenshot_id}.jpg"
            if not src.exists():
                # Try .jpeg extension
                src = screenshots_dir / f"{screenshot_id}.jpeg"
            if not src.exists():
                logger.warning(f"Image not found: {screenshot_id}")
                continue
            
            # Verify image is valid
            try:
                img = Image.open(src)
                img.verify()
            except Exception as e:
                logger.warning(f"Invalid image {screenshot_id}: {e}")
                continue
            
            dst = val_dir / label / f"{screenshot_id}.jpg"
            shutil.copy2(src, dst)
        
        # Create data.yaml
        self._create_data_yaml()
        
        logger.info(f"YOLOv8 dataset created at {self.output_path}")
        return str(self.output_path)
    
    def _create_data_yaml(self) -> None:
        """Create data.yaml file for YOLOv8"""
        # Get class names in order
        num_classes = len(self.class_to_label)
        class_names = [self.class_to_label[i] for i in range(num_classes)]
        
        # Get absolute paths
        train_path = str((self.output_path / "train").absolute())
        val_path = str((self.output_path / "val").absolute())
        
        data_yaml = {
            "path": str(self.output_path.absolute()),
            "train": train_path,
            "val": val_path,
            "nc": num_classes,
            "names": class_names,
        }
        
        yaml_path = self.output_path / "data.yaml"
        import yaml
        with open(yaml_path, "w") as f:
            yaml.dump(data_yaml, f, default_flow_style=False, sort_keys=False)
        
        logger.info(f"Created data.yaml at {yaml_path} with {num_classes} classes: {class_names}")


def load_dataset(
    dataset_path: str,
    output_path: str,
    seed: Optional[int] = None
) -> Tuple[str, DatasetMetadata]:
    """
    Convenience function to load and convert a dataset to YOLOv8 format
    
    Args:
        dataset_path: Path to dataset directory
        output_path: Path where YOLOv8 dataset will be created
        seed: Random seed for train/val split
        
    Returns:
        Tuple of (yolo_dataset_path, metadata)
        
    Raises:
        FileNotFoundError: If dataset files not found
        ValueError: If dataset validation fails
    """
    loader = DatasetLoader(dataset_path, output_path)
    
    # Load dataset
    metadata, manifest = loader.load()
    logger.info(f"Loaded dataset: {metadata.total_snapshots} snapshots, labels: {metadata.label_counts}")
    
    # Validate dataset
    is_valid, error = loader.validate()
    if not is_valid:
        raise ValueError(f"Dataset validation failed: {error}")
    
    # Convert to YOLOv8 format
    yolo_path = loader.convert_to_yolo(seed=seed)
    
    return yolo_path, metadata

