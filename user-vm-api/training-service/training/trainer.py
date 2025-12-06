"""
YOLOv8 fine-tuning trainer
Handles YOLOv8 model training, metrics collection, and ONNX export
"""

import json
import os
import shutil
from pathlib import Path
from typing import Dict, List, Optional, Callable, Tuple
import logging

from ultralytics import YOLO
from ultralytics.utils.callbacks import default_callbacks

from config import config

logger = logging.getLogger(__name__)


class TrainingMetrics:
    """Training metrics collected during training"""
    
    def __init__(self):
        self.train_loss: List[float] = []
        self.val_loss: List[float] = []
        self.map50: List[float] = []  # mAP@0.5
        self.map50_95: List[float] = []  # mAP@0.5:0.95
        self.precision: List[float] = []
        self.recall: List[float] = []
        self.current_epoch: int = 0
        self.total_epochs: int = 0
        self.current_train_loss: Optional[float] = None
        self.current_val_loss: Optional[float] = None
        self.current_map50: Optional[float] = None
        self.current_map50_95: Optional[float] = None
        self.current_precision: Optional[float] = None
        self.current_recall: Optional[float] = None
    
    def update_from_epoch(self, epoch: int, metrics_dict: Dict) -> None:
        """
        Update metrics from epoch results
        
        Args:
            epoch: Current epoch number
            metrics_dict: Dictionary of metrics from Ultralytics
        """
        self.current_epoch = epoch
        
        # Extract metrics (Ultralytics uses various key formats)
        train_loss = metrics_dict.get("train/box_loss") or metrics_dict.get("train_loss") or metrics_dict.get("loss")
        val_loss = metrics_dict.get("val/box_loss") or metrics_dict.get("val_loss")
        map50 = metrics_dict.get("metrics/mAP50") or metrics_dict.get("mAP50") or metrics_dict.get("map50")
        map50_95 = metrics_dict.get("metrics/mAP50-95") or metrics_dict.get("mAP50-95") or metrics_dict.get("map50_95")
        precision = metrics_dict.get("metrics/precision") or metrics_dict.get("precision")
        recall = metrics_dict.get("metrics/recall") or metrics_dict.get("recall")
        
        if train_loss is not None:
            self.current_train_loss = float(train_loss)
            self.train_loss.append(self.current_train_loss)
        
        if val_loss is not None:
            self.current_val_loss = float(val_loss)
            self.val_loss.append(self.current_val_loss)
        
        if map50 is not None:
            self.current_map50 = float(map50)
            self.map50.append(self.current_map50)
        
        if map50_95 is not None:
            self.current_map50_95 = float(map50_95)
            self.map50_95.append(self.current_map50_95)
        
        if precision is not None:
            self.current_precision = float(precision)
            self.precision.append(self.current_precision)
        
        if recall is not None:
            self.current_recall = float(recall)
            self.recall.append(self.current_recall)
    
    def to_dict(self) -> Dict:
        """Convert to dictionary"""
        return {
            "train_loss": self.train_loss,
            "val_loss": self.val_loss,
            "map50": self.map50,
            "map50_95": self.map50_95,
            "precision": self.precision,
            "recall": self.recall,
            "current_epoch": self.current_epoch,
            "total_epochs": self.total_epochs,
            "current_train_loss": self.current_train_loss,
            "current_val_loss": self.current_val_loss,
            "current_map50": self.current_map50,
            "current_map50_95": self.current_map50_95,
            "current_precision": self.current_precision,
            "current_recall": self.current_recall,
        }
    
    def save_to_file(self, file_path: str) -> None:
        """
        Save metrics to JSON file
        
        Args:
            file_path: Path to save metrics JSON file
        """
        os.makedirs(os.path.dirname(file_path), exist_ok=True)
        with open(file_path, "w") as f:
            json.dump(self.to_dict(), f, indent=2)
        logger.info(f"Training metrics saved to {file_path}")


class YOLOv8Trainer:
    """YOLOv8 fine-tuning trainer"""
    
    def __init__(
        self,
        model: YOLO,
        data_yaml_path: str,
        output_dir: str,
        num_classes: int,
    ):
        """
        Initialize YOLOv8 trainer
        
        Args:
            model: YOLOv8 model object (loaded baseline model)
            data_yaml_path: Path to data.yaml file (YOLOv8 dataset configuration)
            output_dir: Directory for training outputs
            num_classes: Number of output classes
        """
        self.model = model
        self.data_yaml_path = data_yaml_path
        self.output_dir = output_dir
        self.num_classes = num_classes
        self.metrics = TrainingMetrics()
        self._progress_callback: Optional[Callable] = None
    
    def set_progress_callback(self, callback: Callable) -> None:
        """
        Set callback function for training progress updates
        
        Args:
            callback: Function to call with (epoch, metrics_dict) on each epoch
        """
        self._progress_callback = callback
    
    def train(
        self,
        epochs: int = 50,
        batch_size: int = 16,
        learning_rate: float = 0.01,
        image_size: int = 640,
        data_augmentation: bool = True,
        freeze_backbone: bool = False,
    ) -> Dict:
        """
        Train YOLOv8 model with fine-tuning
        
        Args:
            epochs: Number of training epochs (50-100 for PoC)
            batch_size: Batch size (adjust based on GPU/CPU memory)
            learning_rate: Initial learning rate (with learning rate scheduler)
            image_size: Image size (640x640 is YOLOv8 default)
            data_augmentation: Enable data augmentation (flip, rotate, etc.)
            freeze_backbone: Freeze backbone layers for faster training (optional)
            
        Returns:
            Dictionary with training results and final metrics
        """
        logger.info(
            f"Starting YOLOv8 fine-tuning: "
            f"epochs={epochs}, batch={batch_size}, lr={learning_rate}, "
            f"imgsz={image_size}, augment={data_augmentation}, "
            f"freeze_backbone={freeze_backbone}, classes={self.num_classes}"
        )
        
        self.metrics.total_epochs = epochs
        
        # Configure training arguments
        training_args = {
            "data": self.data_yaml_path,
            "epochs": epochs,
            "batch": batch_size,
            "lr0": learning_rate,
            "imgsz": image_size,
            "augment": data_augmentation,
            "project": self.output_dir,
            "name": "train",
            "exist_ok": True,
            "save": True,  # Save checkpoints
            "save_period": max(1, epochs // 10),  # Save checkpoint every 10% of epochs
            "val": True,  # Run validation
            "plots": True,  # Generate training plots
        }
        
        # Freeze backbone if requested
        if freeze_backbone:
            logger.info("Freezing backbone layers (not fully supported by Ultralytics, will use transfer learning)")
            # Ultralytics handles transfer learning automatically, but we can set a lower learning rate
            training_args["lr0"] = learning_rate * 0.1  # Lower LR for fine-tuning
        
        # Train the model
        logger.info("Starting training...")
        results = self.model.train(**training_args)
        
        # Extract final metrics from results
        final_metrics = self._extract_final_metrics(results)
        
        # Update metrics object
        if hasattr(results, "results_dict"):
            self.metrics.update_from_epoch(epochs, results.results_dict)
        else:
            # Try to extract from results object directly
            self.metrics.update_from_epoch(epochs, final_metrics)
        
        logger.info(
            f"Training completed. "
            f"Final train loss: {self.metrics.current_train_loss}, "
            f"val loss: {self.metrics.current_val_loss}, "
            f"mAP@0.5: {self.metrics.current_map50}, "
            f"mAP@0.5:0.95: {self.metrics.current_map50_95}"
        )
        
        # Save metrics to JSON file
        metrics_file = os.path.join(self.output_dir, "train", "metrics.json")
        self.metrics.save_to_file(metrics_file)
        
        return {
            "results": results,
            "metrics": self.metrics.to_dict(),
            "final_metrics": final_metrics,
        }
    
    def _extract_final_metrics(self, results) -> Dict:
        """
        Extract final metrics from training results
        
        Args:
            results: Ultralytics training results object
            
        Returns:
            Dictionary of final metrics
        """
        metrics = {}
        
        # Try to extract from results object
        if hasattr(results, "results_dict"):
            metrics = results.results_dict.copy()
        elif hasattr(results, "metrics"):
            metrics = results.metrics
        elif isinstance(results, dict):
            metrics = results
        
        # Extract common metrics with various key formats
        final_metrics = {}
        
        # Loss metrics
        for key in ["train/box_loss", "train_loss", "loss"]:
            if key in metrics:
                final_metrics["train_loss"] = float(metrics[key])
                break
        
        for key in ["val/box_loss", "val_loss"]:
            if key in metrics:
                final_metrics["val_loss"] = float(metrics[key])
                break
        
        # mAP metrics
        for key in ["metrics/mAP50", "mAP50", "map50"]:
            if key in metrics:
                final_metrics["map50"] = float(metrics[key])
                break
        
        for key in ["metrics/mAP50-95", "mAP50-95", "map50_95"]:
            if key in metrics:
                final_metrics["map50_95"] = float(metrics[key])
                break
        
        # Precision and recall
        for key in ["metrics/precision", "precision"]:
            if key in metrics:
                final_metrics["precision"] = float(metrics[key])
                break
        
        for key in ["metrics/recall", "recall"]:
            if key in metrics:
                final_metrics["recall"] = float(metrics[key])
                break
        
        return final_metrics
    
    def export_to_onnx(
        self,
        output_path: str,
        image_size: int = 640,
        simplify: bool = True,
    ) -> str:
        """
        Export trained model to ONNX format
        
        Args:
            output_path: Path where ONNX model will be saved
            image_size: Image size for export (should match training size)
            simplify: Whether to simplify ONNX model (reduces size)
            
        Returns:
            Path to exported ONNX file
        """
        logger.info(f"Exporting model to ONNX format: {output_path}")
        
        # Export to ONNX
        # Ultralytics exports to the same directory as the model weights
        exported_path = self.model.export(
            format="onnx",
            imgsz=image_size,
            simplify=simplify,
        )
        
        # Find the exported ONNX file
        # Ultralytics may export to different locations depending on version
        if os.path.exists(exported_path):
            # Copy to desired output path
            os.makedirs(os.path.dirname(output_path), exist_ok=True)
            shutil.copy2(exported_path, output_path)
            logger.info(f"Model exported to {output_path}")
            return output_path
        else:
            # Try to find ONNX file in weights directory
            weights_dir = Path(self.output_dir) / "train" / "weights"
            onnx_files = list(weights_dir.glob("*.onnx"))
            
            if onnx_files:
                # Use the best model if available, otherwise use the first one
                best_onnx = weights_dir / "best.onnx"
                if best_onnx.exists():
                    shutil.copy2(str(best_onnx), output_path)
                else:
                    shutil.copy2(str(onnx_files[0]), output_path)
                logger.info(f"Model exported to {output_path}")
                return output_path
            else:
                raise ValueError(
                    f"Exported ONNX file not found. "
                    f"Expected at {exported_path} or in {weights_dir}"
                )
    
    def get_best_model_path(self) -> Optional[str]:
        """
        Get path to best model weights
        
        Returns:
            Path to best.pt file, or None if not found
        """
        weights_dir = Path(self.output_dir) / "train" / "weights"
        best_model = weights_dir / "best.pt"
        
        if best_model.exists():
            return str(best_model)
        
        return None
    
    def get_metrics(self) -> TrainingMetrics:
        """Get training metrics"""
        return self.metrics


def fine_tune_yolov8(
    model: YOLO,
    data_yaml_path: str,
    output_dir: str,
    num_classes: int,
    epochs: int = 50,
    batch_size: int = 16,
    learning_rate: float = 0.01,
    image_size: int = 640,
    data_augmentation: bool = True,
    freeze_backbone: bool = False,
) -> Tuple[Dict, str]:
    """
    Convenience function for YOLOv8 fine-tuning
    
    Args:
        model: YOLOv8 model object
        data_yaml_path: Path to data.yaml file
        output_dir: Output directory for training
        num_classes: Number of output classes
        epochs: Number of training epochs
        batch_size: Batch size
        learning_rate: Learning rate
        image_size: Image size
        data_augmentation: Enable data augmentation
        freeze_backbone: Freeze backbone layers
        
    Returns:
        Tuple of (training_results_dict, best_model_path)
    """
    trainer = YOLOv8Trainer(model, data_yaml_path, output_dir, num_classes)
    
    # Train the model
    results = trainer.train(
        epochs=epochs,
        batch_size=batch_size,
        learning_rate=learning_rate,
        image_size=image_size,
        data_augmentation=data_augmentation,
        freeze_backbone=freeze_backbone,
    )
    
    # Get best model path
    best_model_path = trainer.get_best_model_path()
    
    return results, best_model_path

