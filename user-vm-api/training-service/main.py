"""
Python AI Service - FastAPI service for CAE model training and heavy model inference
"""

import os
import logging
from fastapi import FastAPI
from fastapi.responses import JSONResponse
import uvicorn

# Configure logging
log_level = os.getenv("LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=getattr(logging, log_level, logging.INFO),
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="Python AI Service",
    description="CAE model training and heavy model inference service",
    version="0.1.0"
)

# Get configuration from environment
HOST = os.getenv("PYTHON_AI_SERVICE_HOST", "0.0.0.0")
PORT = int(os.getenv("PYTHON_AI_SERVICE_PORT", "8000"))
DATASETS_DIR = os.getenv("DATASETS_DIR", "/app/data/datasets")
MODELS_DIR = os.getenv("MODELS_DIR", "/app/data/models")


@app.get("/health")
async def health():
    """Health check endpoint"""
    return JSONResponse(
        status_code=200,
        content={"status": "healthy", "service": "python-ai-service"}
    )


@app.get("/")
async def root():
    """Root endpoint"""
    return JSONResponse(
        status_code=200,
        content={
            "service": "python-ai-service",
            "version": "0.1.0",
            "status": "running",
            "datasets_dir": DATASETS_DIR,
            "models_dir": MODELS_DIR
        }
    )


# TODO: Implement training endpoints
# TODO: Implement inference endpoints


if __name__ == "__main__":
    logger.info(f"Starting Python AI Service on {HOST}:{PORT}")
    logger.info(f"Datasets directory: {DATASETS_DIR}")
    logger.info(f"Models directory: {MODELS_DIR}")
    
    uvicorn.run(
        app,
        host=HOST,
        port=PORT,
        log_level=log_level.lower()
    )
