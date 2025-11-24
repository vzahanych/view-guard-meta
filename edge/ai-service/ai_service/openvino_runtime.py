"""
OpenVINO Runtime Configuration and Hardware Detection.

Handles OpenVINO toolkit initialization, hardware detection, and runtime configuration.
"""

import logging
from typing import Dict, List, Optional, Tuple

logger = logging.getLogger(__name__)

# Try to import OpenVINO, handle gracefully if not available
try:
    from openvino import Core, get_version
    # Device is not needed for basic operations, skip if not available
    try:
        from openvino.runtime import Device
    except ImportError:
        Device = None  # Device enum not critical for basic operations
    OPENVINO_AVAILABLE = True
except ImportError:
    OPENVINO_AVAILABLE = False
    Device = None
    logger.warning("OpenVINO not available. Install with: pip install openvino")


class OpenVINORuntime:
    """
    OpenVINO runtime manager.
    
    Handles OpenVINO Core initialization, device detection, and configuration.
    """
    
    def __init__(self, device: str = "AUTO"):
        """
        Initialize OpenVINO runtime.
        
        Args:
            device: Target device ("CPU", "GPU", "AUTO", etc.)
        
        Raises:
            RuntimeError: If OpenVINO is not available or initialization fails
        """
        if not OPENVINO_AVAILABLE:
            raise RuntimeError(
                "OpenVINO is not available. Please install it with: pip install openvino"
            )
        
        self.device = device
        self.core: Optional[Core] = None
        self.available_devices: List[str] = []
        self.selected_device: Optional[str] = None
        self.device_info: Dict[str, Dict] = {}
        
        self._initialize()
    
    def _initialize(self):
        """Initialize OpenVINO Core and detect available devices."""
        try:
            self.core = Core()
            
            # Get available devices
            self.available_devices = self.core.available_devices
            
            logger.info(
                "OpenVINO initialized",
                extra={
                    "version": get_version(),
                    "available_devices": self.available_devices,
                },
            )
            
            # Select device
            self.selected_device = self._select_device(self.device)
            
            # Get device information
            self._collect_device_info()
            
        except Exception as e:
            logger.error("Failed to initialize OpenVINO", exc_info=True, extra={"error": str(e)})
            raise RuntimeError(f"OpenVINO initialization failed: {e}") from e
    
    def _select_device(self, preferred_device: str) -> str:
        """
        Select the best available device based on preference.
        
        Args:
            preferred_device: Preferred device ("CPU", "GPU", "AUTO", etc.)
        
        Returns:
            Selected device name
        """
        if not self.available_devices:
            raise RuntimeError("No OpenVINO devices available")
        
        # Handle AUTO device selection
        if preferred_device.upper() == "AUTO":
            # Prefer GPU if available, otherwise CPU
            if "GPU" in self.available_devices:
                return "GPU"
            elif "CPU" in self.available_devices:
                return "CPU"
            else:
                # Use first available device
                return self.available_devices[0]
        
        # Check if preferred device is available
        preferred_upper = preferred_device.upper()
        for device in self.available_devices:
            if device.upper() == preferred_upper:
                return device
        
        # Fallback to CPU if preferred device not available
        if "CPU" in self.available_devices:
            logger.warning(
                f"Preferred device '{preferred_device}' not available, using CPU",
                extra={"available_devices": self.available_devices},
            )
            return "CPU"
        
        # Last resort: use first available device
        logger.warning(
            f"Preferred device '{preferred_device}' not available, using '{self.available_devices[0]}'",
            extra={"available_devices": self.available_devices},
        )
        return self.available_devices[0]
    
    def _collect_device_info(self):
        """Collect information about available devices."""
        for device_name in self.available_devices:
            try:
                device_info = {}
                
                # Get device capabilities
                device_caps = self.core.get_property(device_name, "SUPPORTED_PROPERTIES")
                if device_caps:
                    for prop in device_caps:
                        try:
                            value = self.core.get_property(device_name, prop)
                            device_info[prop] = str(value)
                        except Exception:
                            pass  # Skip properties that can't be read
                
                # Get specific useful properties
                try:
                    device_info["FULL_DEVICE_NAME"] = self.core.get_property(
                        device_name, "FULL_DEVICE_NAME"
                    )
                except Exception:
                    pass
                
                try:
                    device_info["OPTIMIZATION_CAPABILITIES"] = self.core.get_property(
                        device_name, "OPTIMIZATION_CAPABILITIES"
                    )
                except Exception:
                    pass
                
                self.device_info[device_name] = device_info
                
            except Exception as e:
                logger.warning(
                    f"Failed to get info for device '{device_name}'",
                    exc_info=True,
                    extra={"error": str(e)},
                )
    
    def get_core(self) -> Core:
        """
        Get OpenVINO Core instance.
        
        Returns:
            OpenVINO Core instance
        """
        if self.core is None:
            raise RuntimeError("OpenVINO Core not initialized")
        return self.core
    
    def get_device(self) -> str:
        """
        Get selected device name.
        
        Returns:
            Selected device name
        """
        return self.selected_device
    
    def get_device_info(self, device_name: Optional[str] = None) -> Dict:
        """
        Get information about a device.
        
        Args:
            device_name: Device name (default: selected device)
        
        Returns:
            Device information dictionary
        """
        if device_name is None:
            device_name = self.selected_device
        
        return self.device_info.get(device_name, {})
    
    def get_available_devices(self) -> List[str]:
        """
        Get list of available devices.
        
        Returns:
            List of available device names
        """
        return self.available_devices.copy()
    
    def is_gpu_available(self) -> bool:
        """
        Check if GPU device is available.
        
        Returns:
            True if GPU is available, False otherwise
        """
        return "GPU" in self.available_devices or any(
            "GPU" in dev.upper() for dev in self.available_devices
        )
    
    def is_cpu_available(self) -> bool:
        """
        Check if CPU device is available.
        
        Returns:
            True if CPU is available, False otherwise
        """
        return "CPU" in self.available_devices
    
    def get_version(self) -> str:
        """
        Get OpenVINO version.
        
        Returns:
            OpenVINO version string
        """
        if not OPENVINO_AVAILABLE:
            return "not available"
        return get_version()


def detect_hardware() -> Dict[str, any]:
    """
    Detect available hardware for OpenVINO inference.
    
    Returns:
        Dictionary with hardware detection results
    """
    result = {
        "openvino_available": OPENVINO_AVAILABLE,
        "version": None,
        "available_devices": [],
        "gpu_available": False,
        "cpu_available": False,
        "device_info": {},
    }
    
    if not OPENVINO_AVAILABLE:
        logger.warning("OpenVINO not available for hardware detection")
        return result
    
    try:
        runtime = OpenVINORuntime(device="AUTO")
        result["version"] = runtime.get_version()
        result["available_devices"] = runtime.get_available_devices()
        result["gpu_available"] = runtime.is_gpu_available()
        result["cpu_available"] = runtime.is_cpu_available()
        result["device_info"] = runtime.device_info
        result["selected_device"] = runtime.get_device()
        
        logger.info(
            "Hardware detection completed",
            extra=result,
        )
        
    except Exception as e:
        logger.error("Hardware detection failed", exc_info=True, extra={"error": str(e)})
        result["error"] = str(e)
    
    return result


def create_runtime(device: str = "AUTO") -> Optional[OpenVINORuntime]:
    """
    Create and initialize OpenVINO runtime.
    
    Args:
        device: Target device ("CPU", "GPU", "AUTO", etc.)
    
    Returns:
        OpenVINO runtime instance, or None if OpenVINO is not available
    """
    if not OPENVINO_AVAILABLE:
        logger.warning("OpenVINO not available, cannot create runtime")
        return None
    
    try:
        return OpenVINORuntime(device=device)
    except Exception as e:
        logger.error("Failed to create OpenVINO runtime", exc_info=True, extra={"error": str(e)})
        return None

