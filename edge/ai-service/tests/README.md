# AI Service Unit Tests

This directory contains unit tests for the Edge AI Service framework.

## Test Structure

- `conftest.py` - Pytest fixtures and configuration
- `test_config.py` - Configuration management tests
- `test_logger.py` - Logging setup tests
- `test_health.py` - Health check endpoint tests
- `test_openvino_runtime.py` - OpenVINO runtime and hardware detection tests
- `test_model_converter.py` - Model conversion utility tests
- `test_service.py` - FastAPI service initialization tests

## Running Tests

### Run all tests

```bash
pytest
```

### Run with coverage

```bash
pytest --cov=ai_service --cov-report=html
```

### Run specific test file

```bash
pytest tests/test_config.py
```

### Run specific test

```bash
pytest tests/test_config.py::TestConfig::test_default_config
```

### Run with verbose output

```bash
pytest -v
```

## Test Coverage

The tests cover:

- ✅ Configuration loading and validation
- ✅ Logging setup (JSON and text formats)
- ✅ Health check endpoints (liveness, readiness, detailed)
- ✅ OpenVINO runtime initialization and hardware detection
- ✅ Model conversion utilities (ONNX to IR)
- ✅ FastAPI service initialization
- ✅ Error handling (OpenVINO unavailable, invalid config)
- ✅ Graceful shutdown handling

## Test Requirements

Tests use mocking to avoid requiring:
- Actual OpenVINO installation (for most tests)
- Real hardware (GPU/iGPU)
- Actual model files

Some tests may require OpenVINO to be installed for full coverage.

## Fixtures

Common fixtures available in `conftest.py`:

- `temp_dir` - Temporary directory for test files
- `sample_config` - Sample configuration object
- `config_file` - Temporary YAML config file
- `mock_openvino_available` - Mock OpenVINO as available
- `mock_openvino_unavailable` - Mock OpenVINO as unavailable

