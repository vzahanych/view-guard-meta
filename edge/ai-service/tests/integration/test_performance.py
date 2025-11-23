"""
Integration tests for performance under load.

Tests performance metrics and behavior under concurrent load.
"""

import pytest
import time
import threading
import concurrent.futures
from fastapi.testclient import TestClient

import numpy as np


class TestPerformanceUnderLoad:
    """Tests for performance under load."""
    
    @pytest.mark.skip(reason="Performance tests may be slow")
    def test_performance_under_load(
        self,
        app_client: TestClient,
        base64_image: str,
    ):
        """
        Test performance under load with multiple concurrent requests.
        
        P2: Test performance under load (multiple concurrent requests)
        """
        num_requests = 10
        num_threads = 5
        
        start_time = time.time()
        results = []
        
        def make_request():
            response = app_client.post(
                "/api/v1/inference",
                json={"image": base64_image},
            )
            if response.status_code == 200:
                data = response.json()
                return data.get("inference_time_ms", 0)
            return None
        
        # Execute concurrent requests
        with concurrent.futures.ThreadPoolExecutor(max_workers=num_threads) as executor:
            futures = [
                executor.submit(make_request)
                for _ in range(num_requests)
            ]
            results = [
                future.result()
                for future in concurrent.futures.as_completed(futures)
            ]
        
        total_time = time.time() - start_time
        
        # Verify all requests completed
        assert len(results) == num_requests
        assert all(r is not None for r in results)
        
        # Verify performance metrics
        avg_inference_time = sum(results) / len(results)
        requests_per_second = num_requests / total_time
        
        # Log performance metrics
        print(f"\nPerformance metrics:")
        print(f"  Total requests: {num_requests}")
        print(f"  Total time: {total_time:.2f}s")
        print(f"  Requests per second: {requests_per_second:.2f}")
        print(f"  Average inference time: {avg_inference_time:.2f}ms")
    
    def test_inference_statistics_accumulation(
        self,
        inference_engine,
        sample_frame: np.ndarray,
    ):
        """Test that inference statistics accumulate correctly."""
        # Reset statistics
        inference_engine.reset_statistics()
        
        # Perform multiple inferences
        num_inferences = 5
        for _ in range(num_inferences):
            inference_engine.infer(sample_frame)
        
        # Get statistics
        stats = inference_engine.get_statistics()
        
        assert stats["total_inferences"] == num_inferences
        assert stats["total_time_ms"] > 0
        assert stats["average_time_ms"] > 0
        assert stats["average_time_ms"] == stats["total_time_ms"] / num_inferences
    
    def test_batch_inference_performance(
        self,
        inference_engine,
        sample_frame: np.ndarray,
    ):
        """Test batch inference performance."""
        frames = [sample_frame] * 3
        
        start_time = time.time()
        results = inference_engine.infer_batch(frames)
        batch_time = time.time() - start_time
        
        # Verify batch results
        assert len(results) == len(frames)
        
        # Verify each inference time
        for result in results:
            assert result.inference_time_ms > 0
        
        # Batch should be reasonably efficient
        # (not necessarily faster, but should complete)
        assert batch_time < 10.0  # Should complete within 10 seconds


class TestConcurrentInference:
    """Tests for concurrent inference operations."""
    
    def test_concurrent_inference_thread_safety(
        self,
        inference_engine,
        sample_frame: np.ndarray,
    ):
        """Test thread safety of inference engine."""
        num_threads = 5
        num_inferences_per_thread = 2
        
        results = []
        errors = []
        
        def inference_worker():
            thread_results = []
            try:
                for _ in range(num_inferences_per_thread):
                    result = inference_engine.infer(sample_frame)
                    thread_results.append(result)
            except Exception as e:
                errors.append(e)
            return thread_results
        
        # Execute concurrent inferences
        with concurrent.futures.ThreadPoolExecutor(max_workers=num_threads) as executor:
            futures = [executor.submit(inference_worker) for _ in range(num_threads)]
            for future in concurrent.futures.as_completed(futures):
                thread_results = future.result()
                results.extend(thread_results)
        
        # Verify no errors occurred
        assert len(errors) == 0
        
        # Verify all inferences completed
        assert len(results) == num_threads * num_inferences_per_thread
        
        # Verify all results are valid
        assert all(isinstance(r, type(results[0])) for r in results)

