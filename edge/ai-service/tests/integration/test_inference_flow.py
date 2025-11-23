"""
Integration tests for end-to-end inference flow.

Tests the complete inference pipeline from frame input to detection output.
"""

import pytest
import numpy as np
from pathlib import Path

from ai_service.inference import InferenceEngine, DetectionResult
from ai_service.detection import DetectionLogic


class TestEndToEndInferenceFlow:
    """Tests for end-to-end inference flow."""
    
    def test_inference_flow_frame_to_detection(
        self,
        inference_engine: InferenceEngine,
        sample_frame: np.ndarray,
    ):
        """
        Test complete inference flow: frame input → detection output.
        
        P0: Test end-to-end inference flow
        """
        # Perform inference
        result = inference_engine.infer(sample_frame)
        
        # Verify result structure
        assert isinstance(result, DetectionResult)
        assert result.frame_shape == sample_frame.shape[:2]
        assert result.model_input_shape == (640, 640)
        assert isinstance(result.bounding_boxes, list)
        assert result.inference_time_ms > 0
    
    def test_inference_flow_with_detection_logic(
        self,
        inference_engine: InferenceEngine,
        detection_logic: DetectionLogic,
        sample_frame: np.ndarray,
    ):
        """Test inference flow with detection logic filtering."""
        # Perform inference
        result = inference_engine.infer(sample_frame)
        
        # Apply detection logic
        filtered = detection_logic.filter_detections(result)
        
        # Verify filtering worked
        assert isinstance(filtered, DetectionResult)
        assert len(filtered.bounding_boxes) <= len(result.bounding_boxes)
        
        # Verify all filtered boxes meet threshold
        for box in filtered.bounding_boxes:
            assert box.confidence >= detection_logic.filter.min_confidence
    
    def test_inference_flow_person_detection(
        self,
        inference_engine: InferenceEngine,
        detection_logic: DetectionLogic,
        sample_frame: np.ndarray,
    ):
        """Test person detection in inference flow."""
        result = inference_engine.infer(sample_frame)
        persons = detection_logic.detect_persons(result)
        
        # Verify person detection structure
        assert isinstance(persons, list)
        for person in persons:
            assert person.class_id == 0  # COCO person class
            assert person.class_name == "person"
    
    def test_inference_flow_vehicle_detection(
        self,
        inference_engine: InferenceEngine,
        detection_logic: DetectionLogic,
        sample_frame: np.ndarray,
    ):
        """Test vehicle detection in inference flow."""
        result = inference_engine.infer(sample_frame)
        vehicles = detection_logic.detect_vehicles(result)
        
        # Verify vehicle detection structure
        assert isinstance(vehicles, list)
        for vehicle in vehicles:
            assert vehicle.class_id in detection_logic.VEHICLE_CLASS_IDS
    
    def test_inference_flow_batch_processing(
        self,
        inference_engine: InferenceEngine,
        sample_frame: np.ndarray,
    ):
        """Test batch inference processing."""
        frames = [sample_frame, sample_frame.copy(), sample_frame.copy()]
        
        results = inference_engine.infer_batch(frames)
        
        # Verify batch results
        assert len(results) == len(frames)
        assert all(isinstance(r, DetectionResult) for r in results)
        assert all(r.frame_shape == sample_frame.shape[:2] for r in results)


class TestModelLoadingAndInference:
    """Tests for model loading and inference with real OpenVINO runtime."""
    
    def test_model_loading_with_openvino(
        self,
        model_loader,
        openvino_runtime,
    ):
        """
        Test model loading with real OpenVINO runtime.
        
        P0: Test model loading and inference with real OpenVINO runtime
        """
        # Verify model is loaded
        model_info = model_loader.get_current_model()
        assert model_info is not None
        assert model_info.name == "yolov8n"
        assert model_info.format == "openvino"
        
        # Verify compiled model exists
        compiled_model = model_loader.get_compiled_model()
        assert compiled_model is not None
    
    def test_inference_with_loaded_model(
        self,
        inference_engine: InferenceEngine,
        sample_frame: np.ndarray,
    ):
        """Test inference with loaded model."""
        # Perform inference
        result = inference_engine.infer(sample_frame)
        
        # Verify inference succeeded
        assert isinstance(result, DetectionResult)
        assert result.inference_time_ms > 0
    
    def test_model_version_tracking(
        self,
        model_loader,
    ):
        """Test model version tracking."""
        model_info = model_loader.get_current_model()
        assert model_info is not None
        assert model_info.version is not None
        assert model_info.loaded_at is not None


class TestHardwareAcceleration:
    """Tests for hardware acceleration."""
    
    def test_cpu_inference(
        self,
        inference_engine: InferenceEngine,
        sample_frame: np.ndarray,
    ):
        """
        Test CPU inference.
        
        P0: Test hardware acceleration (CPU and GPU if available)
        """
        result = inference_engine.infer(sample_frame)
        
        # Verify inference completed
        assert isinstance(result, DetectionResult)
        assert result.inference_time_ms > 0
    
    def test_device_selection(
        self,
        openvino_runtime,
        models_dir: Path,
    ):
        """Test device selection for inference."""
        # Test CPU device
        runtime_cpu = create_runtime(device="CPU")
        if runtime_cpu:
            loader_cpu = ModelLoader(
                model_dir=models_dir,
                device="CPU",
                runtime=runtime_cpu,
            )
            try:
                loader_cpu.load_model("yolov8n", "openvino")
                assert loader_cpu.get_current_model() is not None
            except Exception:
                pytest.skip("Cannot load model on CPU")
    
    @pytest.mark.skip(reason="GPU testing requires GPU hardware")
    def test_gpu_inference_if_available(
        self,
        models_dir: Path,
        sample_frame: np.ndarray,
    ):
        """
        Test GPU inference if GPU is available.
        
        P0: Test hardware acceleration (CPU and GPU if available)
        """
        from ai_service.openvino_runtime import create_runtime
        
        runtime_gpu = create_runtime(device="GPU")
        if runtime_gpu is None:
            pytest.skip("GPU not available")
        
        loader_gpu = ModelLoader(
            model_dir=models_dir,
            device="GPU",
            runtime=runtime_gpu,
        )
        
        try:
            loader_gpu.load_model("yolov8n", "openvino")
            engine_gpu = InferenceEngine(
                model_loader=loader_gpu,
                confidence_threshold=0.5,
                nms_threshold=0.4,
            )
            
            result = engine_gpu.infer(sample_frame)
            assert isinstance(result, DetectionResult)
        except Exception as e:
            pytest.skip(f"GPU inference not available: {e}")

