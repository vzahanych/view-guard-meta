"""
Health check endpoints for Edge AI Service.

Provides liveness, readiness, and detailed health status endpoints.
"""

import logging
from datetime import datetime
from typing import Dict, Any

from fastapi import APIRouter, status
from fastapi.responses import JSONResponse
from pydantic import BaseModel

logger = logging.getLogger(__name__)

# Health status
_service_ready = False
_service_start_time = datetime.utcnow()


class HealthResponse(BaseModel):
    """Health check response model."""
    status: str
    timestamp: str
    version: str = "dev"


class DetailedHealthResponse(BaseModel):
    """Detailed health check response model."""
    status: str
    timestamp: str
    version: str
    uptime_seconds: float
    components: Dict[str, Any]


def set_service_ready(ready: bool = True):
    """
    Set service readiness status.
    
    Args:
        ready: Whether the service is ready to handle requests
    """
    global _service_ready
    _service_ready = ready
    logger.info("Service readiness changed", extra={"ready": ready})


def is_service_ready() -> bool:
    """
    Check if service is ready.
    
    Returns:
        True if service is ready, False otherwise
    """
    return _service_ready


def get_uptime_seconds() -> float:
    """
    Get service uptime in seconds.
    
    Returns:
        Uptime in seconds
    """
    delta = datetime.utcnow() - _service_start_time
    return delta.total_seconds()


def check_components() -> Dict[str, Any]:
    """
    Check health of service components.
    
    Returns:
        Dictionary with component health status
    """
    components = {
        "api": {
            "status": "healthy",
            "message": "API is operational",
        },
        "model": {
            "status": "unknown",
            "message": "Model not loaded yet",
        },
        "inference": {
            "status": "unknown",
            "message": "Inference engine not initialized",
        },
    }
    
    # Check OpenVINO runtime (if available in app state)
    try:
        from fastapi import Request
        # This will be called from within FastAPI context
        # We'll check runtime via a different method
        pass
    except Exception:
        pass
    
    # Check OpenVINO availability
    try:
        from ai_service.openvino_runtime import OPENVINO_AVAILABLE, detect_hardware
        
        if OPENVINO_AVAILABLE:
            hardware = detect_hardware()
            components["openvino"] = {
                "status": "available" if hardware.get("openvino_available") else "unavailable",
                "message": f"OpenVINO {hardware.get('version', 'unknown')}",
                "devices": hardware.get("available_devices", []),
                "selected_device": hardware.get("selected_device"),
                "gpu_available": hardware.get("gpu_available", False),
            }
        else:
            components["openvino"] = {
                "status": "unavailable",
                "message": "OpenVINO not installed",
            }
    except Exception as e:
        components["openvino"] = {
            "status": "error",
            "message": f"OpenVINO check failed: {str(e)}",
        }
    
    # TODO: Add actual component health checks
    # - Check if model is loaded
    # - Check if inference engine is ready
    # - Check memory usage
    
    return components


def setup_health_endpoints(app):
    """
    Setup health check endpoints on the FastAPI app.
    
    Args:
        app: FastAPI application instance
    """
    router = APIRouter(prefix="/health", tags=["health"])
    
    @router.get("/", response_model=HealthResponse)
    async def health_check():
        """
        Basic health check endpoint.
        
        Returns 200 if service is running.
        """
        return HealthResponse(
            status="ok",
            timestamp=datetime.utcnow().isoformat() + "Z",
        )
    
    @router.get("/live", response_model=HealthResponse)
    async def liveness_check():
        """
        Liveness probe endpoint.
        
        Returns 200 if service is alive (running).
        Used by Kubernetes/container orchestrators.
        """
        return HealthResponse(
            status="alive",
            timestamp=datetime.utcnow().isoformat() + "Z",
        )
    
    @router.get("/ready", response_model=HealthResponse)
    async def readiness_check():
        """
        Readiness probe endpoint.
        
        Returns 200 if service is ready to handle requests.
        Returns 503 if service is not ready.
        Used by Kubernetes/container orchestrators.
        """
        if _service_ready:
            return HealthResponse(
                status="ready",
                timestamp=datetime.utcnow().isoformat() + "Z",
            )
        else:
            return JSONResponse(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                content={
                    "status": "not_ready",
                    "timestamp": datetime.utcnow().isoformat() + "Z",
                },
            )
    
    @router.get("/detailed", response_model=DetailedHealthResponse)
    async def detailed_health_check():
        """
        Detailed health check endpoint.
        
        Returns comprehensive health status including component checks.
        """
        components = check_components()
        
        # Determine overall status
        overall_status = "healthy"
        if not _service_ready:
            overall_status = "not_ready"
        else:
            # Check component statuses
            for component_name, component_info in components.items():
                if component_info.get("status") not in ["healthy", "unknown"]:
                    overall_status = "degraded"
                    break
        
        return DetailedHealthResponse(
            status=overall_status,
            timestamp=datetime.utcnow().isoformat() + "Z",
            uptime_seconds=get_uptime_seconds(),
            components=components,
        )
    
    app.include_router(router)

