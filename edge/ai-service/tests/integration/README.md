# Integration Tests for AI Service

This directory contains integration tests for the AI service that verify end-to-end functionality.

## Test Structure

- **`test_inference_flow.py`**: Tests for end-to-end inference flow, model loading, and hardware acceleration
- **`test_api_integration.py`**: Tests for HTTP API endpoints, concurrent requests, and service health
- **`test_model_hot_reload.py`**: Tests for model hot-reload functionality and error recovery
- **`test_performance.py`**: Tests for performance under load and concurrent operations

## Running Integration Tests

### Run all integration tests:
```bash
pytest tests/integration/ -v
```

### Run specific test file:
```bash
pytest tests/integration/test_inference_flow.py -v
```

### Run with OpenVINO (if available):
```bash
pytest tests/integration/ -v -m "openvino"
```

### Skip integration tests:
```bash
pytest -m "not integration" -v
```

## Prerequisites

Integration tests require:
- OpenVINO runtime (tests will skip if not available)
- Model files in the test models directory (created automatically by fixtures)
- OpenCV for image processing

## Test Coverage

### P0 (Critical) Tests:
- ✅ End-to-end inference flow (frame input → detection output)
- ✅ Model loading and inference with real OpenVINO runtime
- ✅ Hardware acceleration (CPU, GPU if available)
- ✅ Concurrent inference requests
- ✅ Service health and readiness with loaded model
- ✅ HTTP API integration

### P1 (Important) Tests:
- ✅ Model hot-reload during service operation
- ✅ Error recovery (model reload after failure)

### P2 (Nice-to-have) Tests:
- ✅ Performance under load (multiple concurrent requests)

## Notes

- Integration tests use real OpenVINO runtime when available
- Tests automatically skip if OpenVINO or required dependencies are not available
- Mock model files are created automatically for testing
- Performance tests are marked as slow and may be skipped in CI

