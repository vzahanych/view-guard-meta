"""
Python AI Service - FastAPI service for CAE model training and heavy model inference
"""

import logging
from fastapi import FastAPI
from fastapi.responses import JSONResponse
import uvicorn

from config import config
from api.training import router as training_router

# Configure logging
logging.basicConfig(
    level=getattr(logging, config.LOG_LEVEL, logging.INFO),
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="Python AI Service",
    description="CAE model training and heavy model inference service",
    version="0.1.0"
)

# Ensure required directories exist
config.ensure_directories()

# Register API routers
app.include_router(training_router)


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
            "config": config.to_dict()
        }
    )


# Training endpoints are registered via training_router
# TODO: Implement inference endpoints


if __name__ == "__main__":
    logger.info(f"Starting Python AI Service on {config.HOST}:{config.PORT}")
    logger.info(f"Datasets directory: {config.DATASETS_DIR}")
    logger.info(f"Models directory: {config.MODELS_DIR}")
    logger.info(f"Training output directory: {config.TRAINING_OUTPUT_DIR}")
    logger.info(f"Model catalog API URL: {config.MODEL_CATALOG_API_URL}")
    
    uvicorn.run(
        app,
        host=config.HOST,
        port=config.PORT,
        log_level=config.LOG_LEVEL.lower()
    )
