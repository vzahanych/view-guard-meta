#!/usr/bin/env python3
"""
Edge AI Service - Main Entry Point

This service provides AI inference capabilities for the Edge Appliance.
It uses FastAPI for HTTP/gRPC endpoints and OpenVINO for model inference.
"""

import asyncio
import logging
import signal
import sys
from contextlib import asynccontextmanager
from pathlib import Path

import uvicorn
from fastapi import FastAPI

from ai_service.config import Config, load_config
from ai_service.logger import setup_logging
from ai_service.health import setup_health_endpoints, set_service_ready
from ai_service.openvino_runtime import detect_hardware, create_runtime
from ai_service.model_loader import ModelLoader
from ai_service.inference import InferenceEngine
from ai_service.detection import DetectionLogic
from ai_service.api import setup_inference_endpoints

# Global logger (will be initialized in main)
logger = logging.getLogger(__name__)

# Global app instance
app: FastAPI | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Lifespan context manager for FastAPI app.
    Handles startup and shutdown logic.
    """
    # Startup
    logger.info("Starting Edge AI Service", extra={"version": "dev"})
    
    # Detect hardware
    hardware_info = detect_hardware()
    logger.info("Hardware detection completed", extra=hardware_info)
    
    # Initialize OpenVINO runtime
    config = app.state.config if hasattr(app.state, "config") else None
    device = config.model.device if config else "AUTO"
    runtime = create_runtime(device=device)
    if runtime:
        app.state.runtime = runtime
        logger.info(
            "OpenVINO runtime initialized",
            extra={
                "device": runtime.get_device(),
                "version": runtime.get_version(),
            },
        )
    else:
        logger.warning("OpenVINO runtime not available")
        yield
        return
    
    # Initialize model loader
    model_loader = ModelLoader(
        model_dir=config.model.model_dir,
        device=device,
        runtime=runtime,
    )
    app.state.model_loader = model_loader
    
    # Try to load model if configured
    try:
        model_info = model_loader.load_model(
            config.model.model_name,
            config.model.model_format,
        )
        logger.info(
            "Model loaded successfully",
            extra={
                "model_name": model_info.name,
                "version": model_info.version,
                "format": model_info.format,
            },
        )
        
        # Initialize inference engine
        inference_engine = InferenceEngine(
            model_loader=model_loader,
            confidence_threshold=config.model.confidence_threshold,
            nms_threshold=config.model.nms_threshold,
        )
        app.state.inference_engine = inference_engine
        
        # Initialize detection logic
        detection_logic = DetectionLogic()
        app.state.detection_logic = detection_logic
        
        # Setup inference endpoints
        setup_inference_endpoints(app, inference_engine, detection_logic)
        
        # Mark service as ready
        set_service_ready(True)
        
    except Exception as e:
        logger.error(
            "Failed to load model",
            exc_info=True,
            extra={"error": str(e)},
        )
        # Service can still start but won't be ready
        set_service_ready(False)
    
    yield
    
    # Shutdown
    logger.info("Shutting down Edge AI Service")
    # TODO: Cleanup resources


def create_app(config: Config) -> FastAPI:
    """
    Create and configure the FastAPI application.
    
    Args:
        config: Application configuration
        
    Returns:
        Configured FastAPI application
    """
    app = FastAPI(
        title="Edge AI Service",
        description="AI inference service for Edge Appliance",
        version="dev",
        lifespan=lifespan,
    )
    
    # Store config in app state
    app.state.config = config
    
    # Setup health check endpoints
    setup_health_endpoints(app)
    
    # TODO: Register inference endpoints
    # TODO: Register model management endpoints
    
    return app


def setup_signal_handlers():
    """Setup signal handlers for graceful shutdown."""
    def signal_handler(sig, frame):
        logger.info("Received shutdown signal", extra={"signal": sig})
        sys.exit(0)
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)


def main():
    """Main entry point for the AI service."""
    # Parse command line arguments
    import argparse
    parser = argparse.ArgumentParser(description="Edge AI Service")
    parser.add_argument(
        "--config",
        type=str,
        default=None,
        help="Path to configuration file (default: auto-detect)",
    )
    parser.add_argument(
        "--host",
        type=str,
        default="0.0.0.0",
        help="Host to bind to (default: 0.0.0.0)",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8080,
        help="Port to bind to (default: 8080)",
    )
    parser.add_argument(
        "--log-level",
        type=str,
        default=None,
        choices=["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"],
        help="Log level override",
    )
    args = parser.parse_args()
    
    # Load configuration
    try:
        config = load_config(args.config)
    except Exception as e:
        print(f"Failed to load configuration: {e}", file=sys.stderr)
        sys.exit(1)
    
    # Override log level if provided
    if args.log_level:
        config.log.level = args.log_level.lower()
    
    # Setup logging
    setup_logging(config.log)
    
    # Setup signal handlers
    setup_signal_handlers()
    
    # Create FastAPI app
    global app
    app = create_app(config)
    
    # Override host/port from config if available
    host = getattr(config, "host", args.host) if hasattr(config, "host") else args.host
    port = getattr(config, "port", args.port) if hasattr(config, "port") else args.port
    
    # Start server
    logger.info(
        "Starting Edge AI Service server",
        extra={"host": host, "port": port},
    )
    
    try:
        uvicorn.run(
            app,
            host=host,
            port=port,
            log_config=None,  # Use our custom logging
            access_log=True,
        )
    except KeyboardInterrupt:
        logger.info("Server stopped by user")
    except Exception as e:
        logger.error("Server error", exc_info=True, extra={"error": str(e)})
        sys.exit(1)


if __name__ == "__main__":
    main()

