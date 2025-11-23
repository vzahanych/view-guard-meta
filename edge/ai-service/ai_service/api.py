"""
HTTP/gRPC API endpoints for inference service.
"""

import base64
import logging
import time
from typing import List, Optional

import numpy as np
from fastapi import APIRouter, HTTPException, UploadFile, File
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

from ai_service.inference import InferenceEngine, DetectionResult, BoundingBox
from ai_service.detection import DetectionLogic, DetectionFilter

logger = logging.getLogger(__name__)

# Try to import OpenCV
try:
    import cv2
    CV2_AVAILABLE = True
except ImportError:
    CV2_AVAILABLE = False
    logger.warning("OpenCV not available. Install with: pip install opencv-python")


class InferenceRequest(BaseModel):
    """Inference request model."""
    image: str = Field(..., description="Base64-encoded image (JPEG/PNG)")
    confidence_threshold: Optional[float] = Field(None, ge=0.0, le=1.0, description="Confidence threshold override")
    enabled_classes: Optional[List[str]] = Field(None, description="Filter by class names")


class BoundingBoxResponse(BaseModel):
    """Bounding box response model."""
    x1: float
    y1: float
    x2: float
    y2: float
    confidence: float
    class_id: int
    class_name: str


class InferenceResponse(BaseModel):
    """Inference response model."""
    bounding_boxes: List[BoundingBoxResponse]
    inference_time_ms: float
    frame_shape: List[int]  # [height, width]
    model_input_shape: List[int]  # [height, width]
    detection_count: int


class BatchInferenceRequest(BaseModel):
    """Batch inference request model."""
    images: List[str] = Field(..., description="List of base64-encoded images")
    confidence_threshold: Optional[float] = Field(None, ge=0.0, le=1.0)
    enabled_classes: Optional[List[str]] = None


class BatchInferenceResponse(BaseModel):
    """Batch inference response model."""
    results: List[InferenceResponse]
    total_inference_time_ms: float
    average_inference_time_ms: float


def decode_image(image_data: str) -> np.ndarray:
    """
    Decode base64-encoded image to numpy array.
    
    Args:
        image_data: Base64-encoded image string
    
    Returns:
        Image as numpy array (BGR format)
    
    Raises:
        ValueError: If image cannot be decoded
    """
    if not CV2_AVAILABLE:
        raise RuntimeError("OpenCV not available for image decoding")
    
    try:
        # Decode base64
        image_bytes = base64.b64decode(image_data)
        
        # Convert bytes to numpy array
        nparr = np.frombuffer(image_bytes, np.uint8)
        
        # Decode image
        image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
        
        if image is None:
            raise ValueError("Failed to decode image")
        
        return image
    
    except Exception as e:
        logger.error("Failed to decode image", exc_info=True, extra={"error": str(e)})
        raise ValueError(f"Failed to decode image: {e}") from e


def encode_image(image: np.ndarray, format: str = "JPEG") -> str:
    """
    Encode numpy array image to base64 string.
    
    Args:
        image: Image as numpy array (BGR format)
        format: Image format ("JPEG" or "PNG")
    
    Returns:
        Base64-encoded image string
    """
    if not CV2_AVAILABLE:
        raise RuntimeError("OpenCV not available for image encoding")
    
    # Encode image
    encode_format = cv2.IMWRITE_JPEG_QUALITY if format == "JPEG" else cv2.IMWRITE_PNG_COMPRESSION
    success, encoded = cv2.imencode(f".{format.lower()}", image)
    
    if not success:
        raise ValueError(f"Failed to encode image as {format}")
    
    # Convert to base64
    image_bytes = encoded.tobytes()
    return base64.b64encode(image_bytes).decode("utf-8")


def setup_inference_endpoints(
    app,
    inference_engine: InferenceEngine,
    detection_logic: DetectionLogic,
):
    """
    Setup inference API endpoints on FastAPI app.
    
    Args:
        app: FastAPI application instance
        inference_engine: InferenceEngine instance
        detection_logic: DetectionLogic instance
    """
    router = APIRouter(prefix="/api/v1", tags=["inference"])
    
    @router.post("/inference", response_model=InferenceResponse)
    async def inference_endpoint(request: InferenceRequest):
        """
        Perform inference on a single image.
        
        Args:
            request: Inference request with base64-encoded image
        
        Returns:
            Inference response with detections
        """
        try:
            # Decode image
            frame = decode_image(request.image)
            
            # Override confidence threshold if provided
            if request.confidence_threshold is not None:
                detection_logic.set_confidence_threshold(request.confidence_threshold)
            
            # Set enabled classes if provided
            if request.enabled_classes is not None:
                detection_logic.set_enabled_classes(request.enabled_classes)
            
            # Perform inference
            result = inference_engine.infer(frame)
            
            # Apply detection filters
            filtered_result = detection_logic.filter_detections(result)
            
            # Convert to response format
            return InferenceResponse(
                bounding_boxes=[
                    BoundingBoxResponse(
                        x1=box.x1,
                        y1=box.y1,
                        x2=box.x2,
                        y2=box.y2,
                        confidence=box.confidence,
                        class_id=box.class_id,
                        class_name=box.class_name,
                    )
                    for box in filtered_result.bounding_boxes
                ],
                inference_time_ms=filtered_result.inference_time_ms,
                frame_shape=list(filtered_result.frame_shape),
                model_input_shape=list(filtered_result.model_input_shape),
                detection_count=len(filtered_result.bounding_boxes),
            )
        
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))
        except RuntimeError as e:
            raise HTTPException(status_code=500, detail=str(e))
        except Exception as e:
            logger.error("Inference error", exc_info=True, extra={"error": str(e)})
            raise HTTPException(status_code=500, detail=f"Inference failed: {str(e)}")
    
    @router.post("/inference/batch", response_model=BatchInferenceResponse)
    async def batch_inference_endpoint(request: BatchInferenceRequest):
        """
        Perform inference on multiple images (batch processing).
        
        Args:
            request: Batch inference request with list of images
        
        Returns:
            Batch inference response with results for each image
        """
        try:
            start_time = time.time()
            
            # Decode all images
            frames = []
            for image_data in request.images:
                frame = decode_image(image_data)
                frames.append(frame)
            
            # Override confidence threshold if provided
            if request.confidence_threshold is not None:
                detection_logic.set_confidence_threshold(request.confidence_threshold)
            
            # Set enabled classes if provided
            if request.enabled_classes is not None:
                detection_logic.set_enabled_classes(request.enabled_classes)
            
            # Perform batch inference
            results = inference_engine.infer_batch(frames)
            
            # Apply detection filters to each result
            filtered_results = [
                detection_logic.filter_detections(result) for result in results
            ]
            
            # Convert to response format
            response_results = []
            for result in filtered_results:
                response_results.append(
                    InferenceResponse(
                        bounding_boxes=[
                            BoundingBoxResponse(
                                x1=box.x1,
                                y1=box.y1,
                                x2=box.x2,
                                y2=box.y2,
                                confidence=box.confidence,
                                class_id=box.class_id,
                                class_name=box.class_name,
                            )
                            for box in result.bounding_boxes
                        ],
                        inference_time_ms=result.inference_time_ms,
                        frame_shape=list(result.frame_shape),
                        model_input_shape=list(result.model_input_shape),
                        detection_count=len(result.bounding_boxes),
                    )
                )
            
            total_time = (time.time() - start_time) * 1000
            avg_time = total_time / len(frames) if frames else 0.0
            
            return BatchInferenceResponse(
                results=response_results,
                total_inference_time_ms=total_time,
                average_inference_time_ms=avg_time,
            )
        
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))
        except RuntimeError as e:
            raise HTTPException(status_code=500, detail=str(e))
        except Exception as e:
            logger.error("Batch inference error", exc_info=True, extra={"error": str(e)})
            raise HTTPException(status_code=500, detail=f"Batch inference failed: {str(e)}")
    
    @router.post("/inference/file", response_model=InferenceResponse)
    async def inference_file_endpoint(file: UploadFile = File(...)):
        """
        Perform inference on uploaded image file.
        
        Args:
            file: Uploaded image file
        
        Returns:
            Inference response with detections
        """
        try:
            # Read file content
            file_content = await file.read()
            
            # Decode image
            nparr = np.frombuffer(file_content, np.uint8)
            frame = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
            
            if frame is None:
                raise ValueError("Failed to decode uploaded image")
            
            # Perform inference
            result = inference_engine.infer(frame)
            
            # Apply detection filters
            filtered_result = detection_logic.filter_detections(result)
            
            # Convert to response format
            return InferenceResponse(
                bounding_boxes=[
                    BoundingBoxResponse(
                        x1=box.x1,
                        y1=box.y1,
                        x2=box.x2,
                        y2=box.y2,
                        confidence=box.confidence,
                        class_id=box.class_id,
                        class_name=box.class_name,
                    )
                    for box in filtered_result.bounding_boxes
                ],
                inference_time_ms=filtered_result.inference_time_ms,
                frame_shape=list(filtered_result.frame_shape),
                model_input_shape=list(filtered_result.model_input_shape),
                detection_count=len(filtered_result.bounding_boxes),
            )
        
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))
        except RuntimeError as e:
            raise HTTPException(status_code=500, detail=str(e))
        except Exception as e:
            logger.error("File inference error", exc_info=True, extra={"error": str(e)})
            raise HTTPException(status_code=500, detail=f"Inference failed: {str(e)}")
    
    @router.get("/inference/stats")
    async def inference_stats_endpoint():
        """
        Get inference statistics.
        
        Returns:
            Dictionary with inference statistics
        """
        stats = inference_engine.get_statistics()
        return stats
    
    @router.post("/inference/stats/reset")
    async def reset_stats_endpoint():
        """Reset inference statistics."""
        inference_engine.reset_statistics()
        return {"message": "Statistics reset"}
    
    app.include_router(router)

