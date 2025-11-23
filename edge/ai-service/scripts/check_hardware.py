#!/usr/bin/env python3
"""
Hardware Detection Script for OpenVINO.

Detects available hardware and displays OpenVINO device information.
"""

import json
import sys
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from ai_service.openvino_runtime import detect_hardware, OPENVINO_AVAILABLE
from ai_service.logger import setup_logging
from ai_service.config import LogConfig


def main():
    """Main entry point for hardware detection."""
    import argparse
    
    parser = argparse.ArgumentParser(description="Detect OpenVINO hardware")
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output in JSON format",
    )
    args = parser.parse_args()
    
    # Setup logging (quiet mode)
    setup_logging(LogConfig(level="WARNING", format="text", output="stdout"))
    
    # Detect hardware
    hardware_info = detect_hardware()
    
    if args.json:
        print(json.dumps(hardware_info, indent=2))
    else:
        print("=" * 60)
        print("OpenVINO Hardware Detection")
        print("=" * 60)
        
        if not hardware_info.get("openvino_available"):
            print("❌ OpenVINO is not available")
            print("\nInstall with: pip install openvino")
            return 1
        
        print(f"✅ OpenVINO Version: {hardware_info.get('version', 'unknown')}")
        print()
        
        print("Available Devices:")
        devices = hardware_info.get("available_devices", [])
        if devices:
            for device in devices:
                device_info = hardware_info.get("device_info", {}).get(device, {})
                print(f"  • {device}")
                if device_info.get("FULL_DEVICE_NAME"):
                    print(f"    Name: {device_info['FULL_DEVICE_NAME']}")
                if device_info.get("OPTIMIZATION_CAPABILITIES"):
                    caps = device_info["OPTIMIZATION_CAPABILITIES"]
                    if isinstance(caps, list):
                        print(f"    Capabilities: {', '.join(caps)}")
        else:
            print("  No devices available")
        
        print()
        print(f"CPU Available: {'✅' if hardware_info.get('cpu_available') else '❌'}")
        print(f"GPU Available: {'✅' if hardware_info.get('gpu_available') else '❌'}")
        
        if hardware_info.get("selected_device"):
            print(f"Selected Device: {hardware_info['selected_device']}")
        
        if hardware_info.get("error"):
            print(f"\n⚠️  Warning: {hardware_info['error']}")
        
        print("=" * 60)
    
    return 0 if hardware_info.get("openvino_available") else 1


if __name__ == "__main__":
    sys.exit(main())

