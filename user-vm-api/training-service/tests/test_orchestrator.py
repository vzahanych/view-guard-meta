"""
Unit tests for training orchestrator
"""

import os
import pytest
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime

from training.orchestrator import TrainingOrchestrator, TrainingJob, TrainingConfig, TrainingMetrics


class TestTrainingOrchestrator:
    """Tests for TrainingOrchestrator class"""
    
    def test_create_job(self):
        """Test creating a training job"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1",
            training_config=TrainingConfig(epochs=50, batch_size=16)
        )
        
        assert job is not None
        assert job.job_id is not None
        assert job.baseline_model_id == "baseline-yolov8n"
        assert job.dataset_id == "dataset-1"
        assert job.status == "queued"
        assert job.training_config.epochs == 50
        assert job.training_config.batch_size == 16
    
    def test_get_job(self):
        """Test retrieving a job by ID"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1"
        )
        
        retrieved_job = orchestrator.get_job(job.job_id)
        assert retrieved_job is not None
        assert retrieved_job.job_id == job.job_id
    
    def test_get_job_not_found(self):
        """Test retrieving non-existent job"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.get_job("nonexistent-job-id")
        assert job is None
    
    def test_list_jobs(self):
        """Test listing jobs with filters"""
        orchestrator = TrainingOrchestrator()
        
        # Create multiple jobs
        job1 = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1"
        )
        
        job2 = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-2",
            edge_id="edge-1",
            camera_id="camera-2"
        )
        
        # List all jobs
        all_jobs = orchestrator.list_jobs()
        assert len(all_jobs) >= 2
        
        # Filter by camera_id
        camera_jobs = orchestrator.list_jobs(camera_id="camera-1")
        assert len(camera_jobs) >= 1
        assert all(job.camera_id == "camera-1" for job in camera_jobs)
        
        # Filter by edge_id
        edge_jobs = orchestrator.list_jobs(edge_id="edge-1")
        assert len(edge_jobs) >= 2
    
    def test_validate_training_request(self, test_dataset_dir, test_model_dir):
        """Test training request validation"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1"
        )
        
        with patch('training.orchestrator.config.get_dataset_path', return_value=test_dataset_dir):
            with patch('training.orchestrator.config.get_model_path', return_value=test_model_dir):
                is_valid, error = orchestrator.validate_training_request(job)
                
                assert is_valid is True
                assert error is None
    
    def test_validate_training_request_missing_dataset(self, test_model_dir):
        """Test validation with missing dataset"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="nonexistent-dataset",
            edge_id="edge-1",
            camera_id="camera-1"
        )
        
        with patch('training.orchestrator.config.get_dataset_path', return_value="/nonexistent/path"):
            is_valid, error = orchestrator.validate_training_request(job)
            
            assert is_valid is False
            assert error is not None
            assert "not found" in error.lower()
    
    def test_cancel_job(self):
        """Test cancelling a training job"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1"
        )
        
        # Cancel the job
        cancelled = orchestrator.cancel_job(job.job_id)
        assert cancelled is True
        
        # Check job status
        cancelled_job = orchestrator.get_job(job.job_id)
        assert cancelled_job.status == "cancelled"
        assert cancelled_job._cancelled is True
    
    def test_cancel_completed_job(self):
        """Test cancelling a completed job (should fail)"""
        orchestrator = TrainingOrchestrator()
        
        job = orchestrator.create_job(
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1"
        )
        
        # Mark job as completed
        job.status = "completed"
        
        # Try to cancel
        cancelled = orchestrator.cancel_job(job.job_id)
        assert cancelled is False


class TestTrainingJob:
    """Tests for TrainingJob class"""
    
    def test_job_to_dict(self):
        """Test converting job to dictionary"""
        job = TrainingJob(
            job_id="test-job-1",
            baseline_model_id="baseline-yolov8n",
            dataset_id="dataset-1",
            edge_id="edge-1",
            camera_id="camera-1",
            training_config=TrainingConfig()
        )
        
        job_dict = job.to_dict()
        
        assert job_dict["job_id"] == "test-job-1"
        assert job_dict["status"] == "queued"
        assert job_dict["baseline_model_id"] == "baseline-yolov8n"
        assert "training_config" in job_dict
        assert "metrics" in job_dict


class TestTrainingMetrics:
    """Tests for TrainingMetrics class"""
    
    def test_metrics_update_from_epoch(self):
        """Test updating metrics from epoch results"""
        metrics = TrainingMetrics()
        
        metrics_dict = {
            "train/box_loss": 0.5,
            "val/box_loss": 0.6,
            "metrics/mAP50": 0.7,
            "metrics/precision": 0.8,
            "metrics/recall": 0.9
        }
        
        metrics.update_from_epoch(1, metrics_dict)
        
        assert metrics.current_epoch == 1
        assert metrics.current_loss == 0.5
        assert metrics.current_val_loss == 0.6
        assert metrics.current_map == 0.7
        assert len(metrics.train_loss) == 1
        assert len(metrics.val_loss) == 1
        assert len(metrics.map) == 1
    
    def test_metrics_to_dict(self):
        """Test converting metrics to dictionary"""
        metrics = TrainingMetrics()
        metrics.current_epoch = 5
        metrics.total_epochs = 10
        metrics.train_loss = [0.8, 0.7, 0.6, 0.5, 0.4]
        
        metrics_dict = metrics.to_dict()
        
        assert metrics_dict["current_epoch"] == 5
        assert metrics_dict["total_epochs"] == 10
        assert len(metrics_dict["train_loss"]) == 5

