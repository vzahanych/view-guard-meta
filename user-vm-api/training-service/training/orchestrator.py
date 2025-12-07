"""
Training orchestrator
Coordinates the complete training pipeline: dataset loading, model loading, training, and model saving
"""

import json
import os
import shutil
import uuid
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import logging

from ultralytics import YOLO

from config import config
from training.dataset_loader import load_dataset, DatasetMetadata
from training.model_loader import load_baseline_model, ModelMetadata
from training.model_registry import ModelRegistry

logger = logging.getLogger(__name__)


class TrainingConfig:
    """Training configuration parameters"""
    
    def __init__(
        self,
        epochs: int = 50,
        batch_size: int = 16,
        learning_rate: float = 0.01,
        image_size: int = 640,
        data_augmentation: bool = True,
        freeze_backbone: bool = False,
    ):
        self.epochs = epochs
        self.batch_size = batch_size
        self.learning_rate = learning_rate
        self.image_size = image_size
        self.data_augmentation = data_augmentation
        self.freeze_backbone = freeze_backbone
    
    def to_dict(self) -> Dict:
        """Convert to dictionary"""
        return {
            "epochs": self.epochs,
            "batch_size": self.batch_size,
            "learning_rate": self.learning_rate,
            "image_size": self.image_size,
            "data_augmentation": self.data_augmentation,
            "freeze_backbone": self.freeze_backbone,
        }
    
    @classmethod
    def from_dict(cls, data: Dict) -> "TrainingConfig":
        """Create from dictionary"""
        return cls(
            epochs=data.get("epochs", 50),
            batch_size=data.get("batch_size", 16),
            learning_rate=data.get("learning_rate", 0.01),
            image_size=data.get("image_size", 640),
            data_augmentation=data.get("data_augmentation", True),
            freeze_backbone=data.get("freeze_backbone", False),
        )


class TrainingMetrics:
    """Training metrics collected during training"""
    
    def __init__(self):
        self.train_loss: List[float] = []
        self.val_loss: List[float] = []
        self.map: List[float] = []
        self.precision: List[float] = []
        self.recall: List[float] = []
        self.current_epoch: int = 0
        self.total_epochs: int = 0
        self.current_loss: Optional[float] = None
        self.current_val_loss: Optional[float] = None
        self.current_map: Optional[float] = None
    
    def to_dict(self) -> Dict:
        """Convert to dictionary"""
        return {
            "train_loss": self.train_loss,
            "val_loss": self.val_loss,
            "map": self.map,
            "precision": self.precision,
            "recall": self.recall,
            "current_epoch": self.current_epoch,
            "total_epochs": self.total_epochs,
            "current_loss": self.current_loss,
            "current_val_loss": self.current_val_loss,
            "current_map": self.current_map,
        }
    
    def update_from_results(self, results) -> None:
        """Update metrics from Ultralytics training results"""
        if hasattr(results, "results_dict"):
            # Extract metrics from results
            if "train/box_loss" in results.results_dict:
                self.current_loss = results.results_dict.get("train/box_loss", 0)
                self.train_loss.append(self.current_loss)
            
            if "val/box_loss" in results.results_dict:
                self.current_val_loss = results.results_dict.get("val/box_loss", 0)
                self.val_loss.append(self.current_val_loss)
            
            if "metrics/mAP50" in results.results_dict:
                self.current_map = results.results_dict.get("metrics/mAP50", 0)
                self.map.append(self.current_map)
            
            if "metrics/precision" in results.results_dict:
                self.precision.append(results.results_dict.get("metrics/precision", 0))
            
            if "metrics/recall" in results.results_dict:
                self.recall.append(results.results_dict.get("metrics/recall", 0))
        
        # Update epoch info
        if hasattr(results, "epoch"):
            self.current_epoch = results.epoch
        if hasattr(results, "epochs"):
            self.total_epochs = results.epochs
    
    def update_from_epoch(self, epoch: int, metrics_dict: Dict) -> None:
        """
        Update metrics from epoch results (for periodic updates during training)
        
        Args:
            epoch: Current epoch number
            metrics_dict: Dictionary of metrics from training
        """
        self.current_epoch = epoch
        
        # Extract metrics (Ultralytics uses various key formats)
        train_loss = metrics_dict.get("train/box_loss") or metrics_dict.get("train_loss") or metrics_dict.get("loss")
        val_loss = metrics_dict.get("val/box_loss") or metrics_dict.get("val_loss")
        map50 = metrics_dict.get("metrics/mAP50") or metrics_dict.get("mAP50") or metrics_dict.get("map50")
        precision = metrics_dict.get("metrics/precision") or metrics_dict.get("precision")
        recall = metrics_dict.get("metrics/recall") or metrics_dict.get("recall")
        
        if train_loss is not None:
            self.current_loss = float(train_loss)
            if len(self.train_loss) < epoch:
                self.train_loss.append(self.current_loss)
            elif len(self.train_loss) == epoch - 1:
                self.train_loss.append(self.current_loss)
            else:
                # Update existing entry if we're updating a previous epoch
                if epoch - 1 < len(self.train_loss):
                    self.train_loss[epoch - 1] = self.current_loss
        
        if val_loss is not None:
            self.current_val_loss = float(val_loss)
            if len(self.val_loss) < epoch:
                self.val_loss.append(self.current_val_loss)
            elif len(self.val_loss) == epoch - 1:
                self.val_loss.append(self.current_val_loss)
            else:
                if epoch - 1 < len(self.val_loss):
                    self.val_loss[epoch - 1] = self.current_val_loss
        
        if map50 is not None:
            self.current_map = float(map50)
            if len(self.map) < epoch:
                self.map.append(self.current_map)
            elif len(self.map) == epoch - 1:
                self.map.append(self.current_map)
            else:
                if epoch - 1 < len(self.map):
                    self.map[epoch - 1] = self.current_map
        
        if precision is not None:
            prec = float(precision)
            if len(self.precision) < epoch:
                self.precision.append(prec)
            elif len(self.precision) == epoch - 1:
                self.precision.append(prec)
            else:
                if epoch - 1 < len(self.precision):
                    self.precision[epoch - 1] = prec
        
        if recall is not None:
            rec = float(recall)
            if len(self.recall) < epoch:
                self.recall.append(rec)
            elif len(self.recall) == epoch - 1:
                self.recall.append(rec)
            else:
                if epoch - 1 < len(self.recall):
                    self.recall[epoch - 1] = rec


class TrainingJob:
    """Represents a training job"""
    
    def __init__(
        self,
        job_id: str,
        baseline_model_id: str,
        dataset_id: str,
        edge_id: str,
        camera_id: str,
        training_config: TrainingConfig,
    ):
        self.job_id = job_id
        self.baseline_model_id = baseline_model_id
        self.dataset_id = dataset_id
        self.edge_id = edge_id
        self.camera_id = camera_id
        self.training_config = training_config
        
        self.status: str = "queued"  # queued, running, completed, failed, cancelled
        self.trained_model_id: Optional[str] = None
        self.error_message: Optional[str] = None
        self.metrics = TrainingMetrics()
        
        self.started_at: Optional[datetime] = None
        self.completed_at: Optional[datetime] = None
        
        # Internal state
        self._yolo_dataset_path: Optional[str] = None
        self._model: Optional[YOLO] = None
        self._dataset_metadata: Optional[DatasetMetadata] = None
        self._baseline_metadata: Optional[ModelMetadata] = None
        self._output_dir: Optional[str] = None
        self._cancelled: bool = False
        self._training_thread: Optional[threading.Thread] = None
    
    def to_dict(self) -> Dict:
        """Convert to dictionary for API responses"""
        return {
            "job_id": self.job_id,
            "status": self.status,
            "baseline_model_id": self.baseline_model_id,
            "dataset_id": self.dataset_id,
            "camera_id": self.camera_id,
            "edge_id": self.edge_id,
            "trained_model_id": self.trained_model_id,
            "training_config": self.training_config.to_dict(),
            "metrics": self.metrics.to_dict(),
            "error": self.error_message,
            "started_at": self.started_at.isoformat() if self.started_at else None,
            "completed_at": self.completed_at.isoformat() if self.completed_at else None,
        }


class TrainingOrchestrator:
    """Orchestrates the complete training pipeline"""
    
    def __init__(self):
        self.jobs: Dict[str, TrainingJob] = {}
        self.model_registry = ModelRegistry()
    
    def create_job(
        self,
        baseline_model_id: str,
        dataset_id: str,
        edge_id: str,
        camera_id: str,
        training_config: Optional[TrainingConfig] = None,
    ) -> TrainingJob:
        """
        Create a new training job
        
        Args:
            baseline_model_id: ID of baseline model to use
            dataset_id: ID of dataset to train on
            edge_id: Edge device ID
            camera_id: Camera ID
            training_config: Training configuration (optional, uses defaults if not provided)
            
        Returns:
            TrainingJob object
        """
        job_id = str(uuid.uuid4())
        
        if training_config is None:
            training_config = TrainingConfig()
        
        job = TrainingJob(
            job_id=job_id,
            baseline_model_id=baseline_model_id,
            dataset_id=dataset_id,
            edge_id=edge_id,
            camera_id=camera_id,
            training_config=training_config,
        )
        
        self.jobs[job_id] = job
        logger.info(f"Created training job {job_id} for model {baseline_model_id}, dataset {dataset_id}")
        
        return job
    
    def validate_training_request(self, job: TrainingJob) -> Tuple[bool, Optional[str]]:
        """
        Validate training request before starting
        
        Args:
            job: Training job to validate
            
        Returns:
            Tuple of (is_valid, error_message)
        """
        # Validate dataset exists
        dataset_path = config.get_dataset_path(job.edge_id, job.camera_id, job.dataset_id)
        if not os.path.exists(dataset_path):
            return False, f"Dataset not found: {dataset_path}"
        
        # Validate dataset structure
        metadata_path = os.path.join(dataset_path, "metadata.json")
        if not os.path.exists(metadata_path):
            return False, f"Dataset metadata.json not found: {metadata_path}"
        
        screenshots_dir = os.path.join(dataset_path, "screenshots")
        if not os.path.exists(screenshots_dir):
            return False, f"Dataset screenshots directory not found: {screenshots_dir}"
        
        # Validate baseline model exists (try to load metadata)
        try:
            model_path = config.get_model_path(job.baseline_model_id)
            if not os.path.exists(model_path):
                return False, f"Baseline model directory not found: {model_path}"
        except Exception as e:
            return False, f"Failed to validate baseline model: {e}"
        
        return True, None
    
    def execute_training(self, job: TrainingJob) -> str:
        """
        Execute the complete training pipeline
        
        Args:
            job: Training job to execute
            
        Returns:
            Trained model ID
            
        Raises:
            ValueError: If training fails
        """
        job.status = "running"
        job.started_at = datetime.now()
        
        try:
            # Step 1: Validate training request
            logger.info(f"[Job {job.job_id}] Step 1: Validating training request")
            is_valid, error = self.validate_training_request(job)
            if not is_valid:
                raise ValueError(f"Training request validation failed: {error}")
            
            # Step 2: Load dataset and convert to YOLOv8 format
            logger.info(f"[Job {job.job_id}] Step 2: Loading dataset and converting to YOLOv8 format")
            dataset_path = config.get_dataset_path(job.edge_id, job.camera_id, job.dataset_id)
            output_path = os.path.join(
                config.TRAINING_OUTPUT_DIR,
                job.job_id,
                "yolo_dataset"
            )
            os.makedirs(output_path, exist_ok=True)
            
            yolo_dataset_path, dataset_metadata = load_dataset(
                dataset_path,
                output_path,
                seed=42  # Fixed seed for reproducibility
            )
            job._yolo_dataset_path = yolo_dataset_path
            job._dataset_metadata = dataset_metadata
            
            # Determine number of classes from dataset
            num_classes = len(dataset_metadata.label_counts)
            logger.info(f"[Job {job.job_id}] Dataset has {num_classes} classes: {list(dataset_metadata.label_counts.keys())}")
            
            # Step 3: Load baseline model
            logger.info(f"[Job {job.job_id}] Step 3: Loading baseline model")
            model, baseline_metadata = load_baseline_model(
                job.baseline_model_id,
                num_classes=num_classes,
                use_catalog_api=True,
                freeze_backbone=job.training_config.freeze_backbone,
            )
            job._model = model
            job._baseline_metadata = baseline_metadata
            
            # Step 4: Configure training parameters
            logger.info(f"[Job {job.job_id}] Step 4: Configuring training parameters")
            training_args = {
                "data": os.path.join(yolo_dataset_path, "data.yaml"),
                "epochs": job.training_config.epochs,
                "batch": job.training_config.batch_size,
                "lr0": job.training_config.learning_rate,
                "imgsz": job.training_config.image_size,
                "augment": job.training_config.data_augmentation,
                "project": os.path.join(config.TRAINING_OUTPUT_DIR, job.job_id),
                "name": "train",
                "exist_ok": True,
            }
            
            logger.info(f"[Job {job.job_id}] Training configuration: {training_args}")
            
            # Step 5: Execute training using Ultralytics YOLO API
            logger.info(f"[Job {job.job_id}] Step 5: Starting training")
            job.metrics.total_epochs = job.training_config.epochs
            
            # Check for cancellation before training
            if job._cancelled:
                raise ValueError("Training job was cancelled")
            
            # Train the model
            # Note: Ultralytics doesn't provide easy epoch-by-epoch callbacks
            # Metrics will be updated after training completes
            # For periodic updates, we would need to monitor training output files
            # or use Ultralytics callbacks (requires more complex integration)
            results = model.train(**training_args)
            
            # Check for cancellation after training
            if job._cancelled:
                raise ValueError("Training job was cancelled")
            
            # Update metrics from training results
            job.metrics.update_from_results(results)
            
            logger.info(
                f"[Job {job.job_id}] Training completed. "
                f"Final loss: {job.metrics.current_loss}, "
                f"mAP: {job.metrics.current_map}"
            )
            
            # Step 6: Save trained model to ONNX format
            logger.info(f"[Job {job.job_id}] Step 6: Exporting trained model to ONNX")
            trained_model_id = f"{job.baseline_model_id}-{job.dataset_id}-{datetime.now().strftime('%Y%m%d%H%M%S')}"
            model_output_dir = config.get_model_path(trained_model_id)
            os.makedirs(model_output_dir, exist_ok=True)
            
            # Export to ONNX
            onnx_path = os.path.join(model_output_dir, "model.onnx")
            model.export(format="onnx", imgsz=job.training_config.image_size)
            
            # Find the exported ONNX file (Ultralytics saves it in the training output directory)
            training_output_dir = os.path.join(config.TRAINING_OUTPUT_DIR, job.job_id, "train", "weights")
            exported_onnx = os.path.join(training_output_dir, "best.onnx")
            
            if os.path.exists(exported_onnx):
                shutil.copy2(exported_onnx, onnx_path)
                logger.info(f"[Job {job.job_id}] Model exported to {onnx_path}")
            else:
                # Try to find any .onnx file in the weights directory
                weights_dir = Path(training_output_dir)
                onnx_files = list(weights_dir.glob("*.onnx"))
                if onnx_files:
                    shutil.copy2(str(onnx_files[0]), onnx_path)
                    logger.info(f"[Job {job.job_id}] Model exported to {onnx_path}")
                else:
                    raise ValueError(f"Exported ONNX file not found in {training_output_dir}")
            
            job.trained_model_id = trained_model_id
            job._output_dir = model_output_dir
            
            # Step 7: Generate model metadata and register in catalog
            logger.info(f"[Job {job.job_id}] Step 7: Generating model metadata and registering in catalog")
            self._register_trained_model(job, onnx_path)
            
            # Step 8: Update training job status
            job.status = "completed"
            job.completed_at = datetime.now()
            
            logger.info(f"[Job {job.job_id}] Training job completed successfully. Model ID: {trained_model_id}")
            
            return trained_model_id
            
        except Exception as e:
            logger.error(f"[Job {job.job_id}] Training failed: {e}", exc_info=True)
            job.status = "failed"
            job.error_message = str(e)
            job.completed_at = datetime.now()
            
            # Clean up temporary files
            self._cleanup(job)
            
            raise
    
    def _register_trained_model(self, job: TrainingJob, onnx_path: str) -> None:
        """
        Register trained model in model catalog
        
        Args:
            job: Training job
            onnx_path: Path to exported ONNX model file
        """
        # Prepare training metrics dictionary
        training_metrics = {
            "train_loss": job.metrics.train_loss,
            "val_loss": job.metrics.val_loss,
            "map": job.metrics.map,
            "precision": job.metrics.precision,
            "recall": job.metrics.recall,
            "current_epoch": job.metrics.current_epoch,
            "total_epochs": job.metrics.total_epochs,
            "current_loss": job.metrics.current_loss,
            "current_val_loss": job.metrics.current_val_loss,
            "current_map": job.metrics.current_map,
        }
        
        # Get baseline metadata as dict if available
        baseline_metadata_dict = None
        if job._baseline_metadata:
            baseline_metadata_dict = {
                "model_id": job._baseline_metadata.model_id,
                "model_type": job._baseline_metadata.model_type,
                "input_shape": job._baseline_metadata.input_shape,
                "framework": job._baseline_metadata.framework,
            }
        
        # Determine number of output classes from dataset
        output_classes = len(job._dataset_metadata.label_counts) if job._dataset_metadata else 2
        
        # Get input shape from baseline metadata or use default
        input_shape = (
            job._baseline_metadata.input_shape
            if job._baseline_metadata and job._baseline_metadata.input_shape
            else [1, 3, 640, 640]
        )
        
        # Get model type from baseline metadata or use default
        model_type = (
            job._baseline_metadata.model_type
            if job._baseline_metadata and job._baseline_metadata.model_type
            else "yolov8"
        )
        
        # Register model using model registry
        result = self.model_registry.register_trained_model(
            trained_model_id=job.trained_model_id,
            onnx_path=onnx_path,
            baseline_model_id=job.baseline_model_id,
            dataset_id=job.dataset_id,
            camera_id=job.camera_id,
            edge_id=job.edge_id,
            model_type=model_type,
            input_shape=input_shape,
            output_classes=output_classes,
            training_metrics=training_metrics,
            baseline_metadata=baseline_metadata_dict,
            image_size=job.training_config.image_size,
        )
        
        if result["registered"]:
            logger.info(f"[Job {job.job_id}] Model {job.trained_model_id} successfully registered in catalog")
            # Publish model.trained event to trigger deployment
            # For PoC, we rely on deployment service's periodic catalog scan
            # Future: Publish event via HTTP to VM API Gateway event bus
            try:
                self._publish_model_trained_event(job.trained_model_id, job.edge_id, job.camera_id)
            except Exception as e:
                logger.warning(f"[Job {job.job_id}] Failed to publish model.trained event: {e}")
        else:
            logger.warning(
                f"[Job {job.job_id}] Model {job.trained_model_id} saved locally but "
                f"API registration failed: {result.get('error', 'Unknown error')}"
            )
    
    def _cleanup(self, job: TrainingJob) -> None:
        """
        Clean up temporary files for a training job
        
        Args:
            job: Training job to clean up
        """
        try:
            if job._yolo_dataset_path and os.path.exists(job._yolo_dataset_path):
                # Keep YOLO dataset for debugging, but could remove if needed
                logger.debug(f"[Job {job.job_id}] Keeping YOLO dataset at {job._yolo_dataset_path}")
            
            # Clean up training output directory if job failed
            if job.status == "failed":
                training_output_dir = os.path.join(config.TRAINING_OUTPUT_DIR, job.job_id)
                if os.path.exists(training_output_dir):
                    logger.info(f"[Job {job.job_id}] Cleaning up training output directory: {training_output_dir}")
                    shutil.rmtree(training_output_dir, ignore_errors=True)
        
        except Exception as e:
            logger.warning(f"[Job {job.job_id}] Error during cleanup: {e}")
    
    def get_job(self, job_id: str) -> Optional[TrainingJob]:
        """Get training job by ID"""
        return self.jobs.get(job_id)
    
    def list_jobs(
        self,
        camera_id: Optional[str] = None,
        edge_id: Optional[str] = None,
        status: Optional[str] = None,
        limit: int = 100,
        offset: int = 0,
    ) -> List[TrainingJob]:
        """
        List training jobs with optional filters
        
        Args:
            camera_id: Filter by camera ID
            edge_id: Filter by edge ID
            status: Filter by status
            limit: Maximum number of jobs to return
            offset: Offset for pagination
            
        Returns:
            List of TrainingJob objects
        """
        jobs = list(self.jobs.values())
        
        # Apply filters
        if camera_id:
            jobs = [j for j in jobs if j.camera_id == camera_id]
        if edge_id:
            jobs = [j for j in jobs if j.edge_id == edge_id]
        if status:
            jobs = [j for j in jobs if j.status == status]
        
        # Sort by started_at (most recent first)
        jobs.sort(key=lambda j: j.started_at or datetime.min, reverse=True)
        
        # Apply pagination
        return jobs[offset:offset + limit]
    
    def cancel_job(self, job_id: str) -> bool:
        """
        Cancel a training job
        
        Args:
            job_id: Job ID to cancel
            
        Returns:
            True if job was cancelled, False if job not found or already completed
        """
        job = self.jobs.get(job_id)
        if not job:
            return False
        
        # Check if job can be cancelled
        if job.status in ["completed", "failed", "cancelled"]:
            logger.warning(f"[Job {job_id}] Cannot cancel job with status: {job.status}")
            return False
        
        # Mark job as cancelled
        job._cancelled = True
        job.status = "cancelled"
        job.completed_at = datetime.now()
        job.error_message = "Training job was cancelled by user"
        
        logger.info(f"[Job {job_id}] Training job cancelled")
        
        # Clean up temporary files
        self._cleanup(job)
        
        return True

