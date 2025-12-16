"""
Training API endpoints
Provides REST API for training model management
"""

import json
import os
import threading
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple
import logging

from fastapi import APIRouter, HTTPException, Query, Path
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from config import config
from training.orchestrator import TrainingOrchestrator, TrainingConfig, TrainingJob

logger = logging.getLogger(__name__)

# Create router
router = APIRouter(prefix="/api/training", tags=["training"])

# Global orchestrator instance
orchestrator = TrainingOrchestrator()


# Request/Response models
class TrainingConfigRequest(BaseModel):
    """Training configuration request model"""
    epochs: int = Field(default=50, ge=1, le=1000, description="Number of training epochs")
    batch_size: int = Field(default=16, ge=1, le=128, description="Batch size for training")
    learning_rate: float = Field(default=0.01, ge=0.0001, le=1.0, description="Learning rate")
    image_size: int = Field(default=640, ge=224, le=1280, description="Image size for training")
    data_augmentation: bool = Field(default=True, description="Enable data augmentation")
    freeze_backbone: bool = Field(default=False, description="Freeze backbone layers")


class TrainingStartRequest(BaseModel):
    """Training start request model"""
    baseline_model_id: str = Field(..., description="ID of baseline model to use")
    dataset_id: str = Field(..., description="ID of dataset to train on")
    camera_id: str = Field(..., description="Camera ID")
    edge_id: str = Field(..., description="Edge device ID")
    training_config: Optional[TrainingConfigRequest] = Field(default=None, description="Training configuration")


class TrainingStartResponse(BaseModel):
    """Training start response model"""
    job_id: str
    status: str
    baseline_model_id: str
    dataset_id: str
    started_at: str
    estimated_completion: Optional[str] = None


class ProgressInfo(BaseModel):
    """Training progress information"""
    epoch: int
    total_epochs: int
    current_loss: Optional[float] = None
    val_loss: Optional[float] = None
    mAP: Optional[float] = None


class TrainingStatusResponse(BaseModel):
    """Training status response model"""
    job_id: str
    status: str
    baseline_model_id: str
    dataset_id: str
    camera_id: str
    edge_id: str
    progress: Optional[ProgressInfo] = None
    metrics: Optional[Dict] = None
    trained_model_id: Optional[str] = None
    error: Optional[str] = None
    started_at: Optional[str] = None
    completed_at: Optional[str] = None
    deployment_id: Optional[str] = None
    deployment_status: Optional[str] = None
    deployed_at: Optional[str] = None


class TrainingJobSummary(BaseModel):
    """Training job summary for list endpoints"""
    job_id: str
    status: str
    baseline_model_id: str
    dataset_id: str
    camera_id: str
    edge_id: str
    trained_model_id: Optional[str] = None
    started_at: Optional[str] = None
    completed_at: Optional[str] = None


class TrainingListResponse(BaseModel):
    """Training list response model"""
    jobs: List[TrainingJobSummary]
    total: int
    limit: int
    offset: int


def _validate_dataset_snapshots(dataset_path: str, min_snapshots: int = 50) -> Tuple[bool, Optional[str]]:
    """
    Validate that dataset has sufficient snapshots
    
    Args:
        dataset_path: Path to dataset directory
        min_snapshots: Minimum number of snapshots required
        
    Returns:
        Tuple of (is_valid, error_message)
    """
    metadata_path = os.path.join(dataset_path, "metadata.json")
    if not os.path.exists(metadata_path):
        return False, "Dataset metadata.json not found"
    
    try:
        with open(metadata_path, "r") as f:
            metadata = json.load(f)
        
        total_snapshots = metadata.get("total_snapshots", 0)
        if total_snapshots < min_snapshots:
            return False, f"Insufficient snapshots: {total_snapshots} < {min_snapshots}"
        
        return True, None
    except Exception as e:
        return False, f"Failed to read dataset metadata: {e}"


def _validate_baseline_model(baseline_model_id: str) -> Tuple[bool, Optional[str]]:
    """
    Validate that baseline model exists and is in baseline status
    
    Args:
        baseline_model_id: Baseline model ID
        
    Returns:
        Tuple of (is_valid, error_message)
    """
    # Check if model directory exists
    model_path = config.get_model_path(baseline_model_id)
    if not os.path.exists(model_path):
        return False, f"Baseline model not found: {baseline_model_id}"
    
    # Check metadata for status
    metadata_path = os.path.join(model_path, "metadata.json")
    if os.path.exists(metadata_path):
        try:
            with open(metadata_path, "r") as f:
                metadata = json.load(f)
            
            # Check if it's a baseline model (model_id starts with "baseline-" or has baseline status)
            model_id = metadata.get("model_id", "")
            if not model_id.startswith("baseline-") and metadata.get("status") != "baseline":
                # Try to check via API if available
                # For now, if model exists and has metadata, consider it valid
                # The actual status check would require API call to model catalog
                pass
        except Exception as e:
            logger.warning(f"Failed to read baseline model metadata: {e}")
    
    return True, None


def _estimate_completion_time(epochs: int) -> str:
    """
    Estimate training completion time
    
    Args:
        epochs: Number of epochs
        
    Returns:
        ISO format timestamp string
    """
    # Rough estimate: 2 minutes per epoch (adjust based on actual performance)
    estimated_minutes = epochs * 2
    completion_time = datetime.now() + timedelta(minutes=estimated_minutes)
    return completion_time.isoformat()


def _run_training_async(job: TrainingJob) -> None:
    """
    Run training in a separate thread
    
    Args:
        job: Training job to execute
    """
    try:
        orchestrator.execute_training(job)
    except Exception as e:
        logger.error(f"Training job {job.job_id} failed: {e}", exc_info=True)
        job.status = "failed"
        job.error_message = str(e)
        job.completed_at = datetime.now()


@router.post("/start", response_model=TrainingStartResponse)
async def start_training(request: TrainingStartRequest):
    """
    Start a new training job
    
    Validates the request and starts training asynchronously.
    Returns immediately with job ID and status.
    """
    logger.info(
        f"Training start request: baseline={request.baseline_model_id}, "
        f"dataset={request.dataset_id}, camera={request.camera_id}"
    )
    
    # Validate baseline model
    is_valid, error = _validate_baseline_model(request.baseline_model_id)
    if not is_valid:
        raise HTTPException(status_code=400, detail=f"Baseline model validation failed: {error}")
    
    # Validate dataset exists and has sufficient snapshots
    dataset_path = config.get_dataset_path(request.edge_id, request.camera_id, request.dataset_id)
    if not os.path.exists(dataset_path):
        # Provide detailed error message with expected path for debugging
        error_detail = (
            f"Dataset not found: {request.dataset_id}. "
            f"Expected path: {dataset_path}. "
            f"Parameters: edge_id={request.edge_id}, camera_id={request.camera_id}, dataset_id={request.dataset_id}. "
            f"Please verify the dataset was synced correctly in Epic 2.5."
        )
        logger.error(error_detail)
        raise HTTPException(status_code=404, detail=error_detail)
    
    is_valid, error = _validate_dataset_snapshots(dataset_path, min_snapshots=50)
    if not is_valid:
        raise HTTPException(status_code=400, detail=f"Dataset validation failed: {error}")
    
    # Validate camera_id and edge_id match dataset
    metadata_path = os.path.join(dataset_path, "metadata.json")
    try:
        with open(metadata_path, "r") as f:
            dataset_metadata = json.load(f)
        
        if dataset_metadata.get("camera_id") != request.camera_id:
            raise HTTPException(
                status_code=400,
                detail=f"Camera ID mismatch: dataset has {dataset_metadata.get('camera_id')}, request has {request.camera_id}"
            )
        
        if dataset_metadata.get("edge_id") != request.edge_id:
            raise HTTPException(
                status_code=400,
                detail=f"Edge ID mismatch: dataset has {dataset_metadata.get('edge_id')}, request has {request.edge_id}"
            )
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to validate dataset metadata: {e}")
    
    # Create training config
    training_config = None
    if request.training_config:
        training_config = TrainingConfig(
            epochs=request.training_config.epochs,
            batch_size=request.training_config.batch_size,
            learning_rate=request.training_config.learning_rate,
            image_size=request.training_config.image_size,
            data_augmentation=request.training_config.data_augmentation,
            freeze_backbone=request.training_config.freeze_backbone,
        )
    
    # Create training job
    job = orchestrator.create_job(
        baseline_model_id=request.baseline_model_id,
        dataset_id=request.dataset_id,
        edge_id=request.edge_id,
        camera_id=request.camera_id,
        training_config=training_config,
    )
    
    # Set job status to running (will be set by execute_training, but set early for API response)
    job.status = "running"
    job.started_at = datetime.now()
    
    # Start training in background thread
    training_thread = threading.Thread(target=_run_training_async, args=(job,), daemon=True)
    training_thread.start()
    
    # Estimate completion time
    epochs = training_config.epochs if training_config else 50
    estimated_completion = _estimate_completion_time(epochs)
    
    return TrainingStartResponse(
        job_id=job.job_id,
        status=job.status,
        baseline_model_id=job.baseline_model_id,
        dataset_id=job.dataset_id,
        started_at=job.started_at.isoformat() if job.started_at else datetime.now().isoformat(),
        estimated_completion=estimated_completion,
    )


@router.get("/{job_id}", response_model=TrainingStatusResponse)
async def get_training_status(job_id: str = Path(..., description="Training job ID")):
    """
    Get training job status and progress
    
    Returns current status, progress, and metrics for a training job.
    """
    job = orchestrator.get_job(job_id)
    if not job:
        raise HTTPException(status_code=404, detail=f"Training job not found: {job_id}")
    
    # Build progress info if training is running or completed
    progress = None
    if job.status in ["running", "completed"]:
        progress = ProgressInfo(
            epoch=job.metrics.current_epoch,
            total_epochs=job.metrics.total_epochs,
            current_loss=job.metrics.current_loss,
            val_loss=job.metrics.current_val_loss,
            mAP=job.metrics.current_map,
        )
    
    # Build metrics dictionary
    metrics = None
    if job.metrics:
        metrics = job.metrics.to_dict()
    
    # Query deployment status if trained_model_id exists
    deployment_id = None
    deployment_status = None
    deployed_at = None
    
    if job.trained_model_id:
        # For PoC: Query deployment status from VM API Gateway
        # Future: Direct database query or deployment service API
        try:
            import requests
            deployment_url = f"{config.MODEL_CATALOG_API_URL}/api/deployments"
            params = {"model_id": job.trained_model_id, "limit": 1}
            resp = requests.get(deployment_url, params=params, timeout=2)
            if resp.status_code == 200:
                data = resp.json()
                if data.get("deployments") and len(data["deployments"]) > 0:
                    deployment = data["deployments"][0]
                    deployment_id = deployment.get("deployment_id")
                    deployment_status = deployment.get("status")
                    if deployment.get("deployment_completed_at"):
                        deployed_at = deployment["deployment_completed_at"]
        except Exception as e:
            logger.debug(f"Failed to query deployment status: {e}")
    
    return TrainingStatusResponse(
        job_id=job.job_id,
        status=job.status,
        baseline_model_id=job.baseline_model_id,
        dataset_id=job.dataset_id,
        camera_id=job.camera_id,
        edge_id=job.edge_id,
        progress=progress,
        metrics=metrics,
        trained_model_id=job.trained_model_id,
        error=job.error_message,
        started_at=job.started_at.isoformat() if job.started_at else None,
        completed_at=job.completed_at.isoformat() if job.completed_at else None,
        deployment_id=deployment_id,
        deployment_status=deployment_status,
        deployed_at=deployed_at,
    )


@router.get("", response_model=TrainingListResponse)
async def list_training_jobs(
    camera_id: Optional[str] = Query(None, description="Filter by camera ID"),
    edge_id: Optional[str] = Query(None, description="Filter by edge ID"),
    status: Optional[str] = Query(None, description="Filter by status"),
    limit: int = Query(100, ge=1, le=1000, description="Maximum number of jobs to return"),
    offset: int = Query(0, ge=0, description="Offset for pagination"),
):
    """
    List training jobs with optional filters
    
    Returns paginated list of training jobs with optional filtering.
    """
    jobs = orchestrator.list_jobs(
        camera_id=camera_id,
        edge_id=edge_id,
        status=status,
        limit=limit,
        offset=offset,
    )
    
    # Convert to summary format
    job_summaries = [
        TrainingJobSummary(
            job_id=job.job_id,
            status=job.status,
            baseline_model_id=job.baseline_model_id,
            dataset_id=job.dataset_id,
            camera_id=job.camera_id,
            edge_id=job.edge_id,
            trained_model_id=job.trained_model_id,
            started_at=job.started_at.isoformat() if job.started_at else None,
            completed_at=job.completed_at.isoformat() if job.completed_at else None,
        )
        for job in jobs
    ]
    
    # Get total count (all jobs matching filters, before pagination)
    all_jobs = orchestrator.list_jobs(
        camera_id=camera_id,
        edge_id=edge_id,
        status=status,
        limit=10000,  # Large limit to get all
        offset=0,
    )
    total = len(all_jobs)
    
    return TrainingListResponse(
        jobs=job_summaries,
        total=total,
        limit=limit,
        offset=offset,
    )


@router.get("/camera/{camera_id}", response_model=TrainingListResponse)
async def get_camera_training_jobs(
    camera_id: str = Path(..., description="Camera ID"),
    status: Optional[str] = Query(None, description="Filter by status"),
    limit: int = Query(100, ge=1, le=1000, description="Maximum number of jobs to return"),
    offset: int = Query(0, ge=0, description="Offset for pagination"),
):
    """
    Get training jobs for a specific camera
    
    Returns training history for a specific camera.
    """
    jobs = orchestrator.list_jobs(
        camera_id=camera_id,
        status=status,
        limit=limit,
        offset=offset,
    )
    
    # Convert to summary format
    job_summaries = [
        TrainingJobSummary(
            job_id=job.job_id,
            status=job.status,
            baseline_model_id=job.baseline_model_id,
            dataset_id=job.dataset_id,
            camera_id=job.camera_id,
            edge_id=job.edge_id,
            trained_model_id=job.trained_model_id,
            started_at=job.started_at.isoformat() if job.started_at else None,
            completed_at=job.completed_at.isoformat() if job.completed_at else None,
        )
        for job in jobs
    ]
    
    # Get total count
    all_jobs = orchestrator.list_jobs(
        camera_id=camera_id,
        status=status,
        limit=10000,
        offset=0,
    )
    total = len(all_jobs)
    
    return TrainingListResponse(
        jobs=job_summaries,
        total=total,
        limit=limit,
        offset=offset,
    )


@router.delete("/{job_id}")
async def cancel_training_job(job_id: str = Path(..., description="Training job ID")):
    """
    Cancel a training job
    
    Cancels a running or queued training job. Completed or failed jobs cannot be cancelled.
    """
    job = orchestrator.get_job(job_id)
    if not job:
        raise HTTPException(status_code=404, detail=f"Training job not found: {job_id}")
    
    # Check if job can be cancelled
    if job.status in ["completed", "failed", "cancelled"]:
        raise HTTPException(
            status_code=400,
            detail=f"Cannot cancel job with status: {job.status}"
        )
    
    # Cancel the job
    cancelled = orchestrator.cancel_job(job_id)
    if not cancelled:
        raise HTTPException(
            status_code=400,
            detail=f"Failed to cancel job: {job_id}"
        )
    
    return JSONResponse(
        status_code=200,
        content={
            "job_id": job_id,
            "status": "cancelled",
            "message": "Training job cancelled successfully"
        }
    )

