#!/usr/bin/env python3
"""
Simple test script to verify the AI service structure.

This script tests basic functionality without requiring all dependencies.
"""

import sys
from pathlib import Path

# Add current directory to path
sys.path.insert(0, str(Path(__file__).parent))

def test_imports():
    """Test that all modules can be imported."""
    print("Testing imports...")
    try:
        from ai_service import config, logger, health
        print("✅ All modules imported successfully")
        return True
    except ImportError as e:
        print(f"❌ Import error: {e}")
        return False

def test_config():
    """Test configuration loading."""
    print("\nTesting configuration...")
    try:
        from ai_service.config import Config, load_config
        
        # Test default config
        default_config = Config()
        print(f"✅ Default config created: log level = {default_config.log.level}")
        
        # Test config loading (may fail if no config file, that's OK)
        try:
            loaded_config = load_config()
            print(f"✅ Config loaded: port = {loaded_config.server.port}")
        except Exception as e:
            print(f"⚠️  Config file not found (expected): {e}")
        
        return True
    except Exception as e:
        print(f"❌ Config error: {e}")
        return False

def test_logger():
    """Test logging setup."""
    print("\nTesting logger...")
    try:
        from ai_service.logger import setup_logging
        from ai_service.config import LogConfig
        
        log_config = LogConfig(level="INFO", format="text", output="stdout")
        setup_logging(log_config)
        
        import logging
        logger = logging.getLogger("test")
        logger.info("Test log message")
        print("✅ Logger setup successful")
        return True
    except Exception as e:
        print(f"❌ Logger error: {e}")
        return False

def test_health():
    """Test health check module."""
    print("\nTesting health checks...")
    try:
        from ai_service.health import (
            set_service_ready,
            is_service_ready,
            get_uptime_seconds,
            check_components,
        )
        
        # Test service ready
        set_service_ready(True)
        assert is_service_ready() == True
        print("✅ Service ready status works")
        
        # Test uptime
        uptime = get_uptime_seconds()
        assert uptime >= 0
        print(f"✅ Uptime calculation works: {uptime:.2f}s")
        
        # Test component checks
        components = check_components()
        assert "api" in components
        print(f"✅ Component checks work: {list(components.keys())}")
        
        return True
    except Exception as e:
        print(f"❌ Health check error: {e}")
        import traceback
        traceback.print_exc()
        return False

def main():
    """Run all tests."""
    print("=" * 50)
    print("Edge AI Service Structure Test")
    print("=" * 50)
    
    results = []
    results.append(("Imports", test_imports()))
    results.append(("Config", test_config()))
    results.append(("Logger", test_logger()))
    results.append(("Health", test_health()))
    
    print("\n" + "=" * 50)
    print("Test Results:")
    print("=" * 50)
    
    all_passed = True
    for name, passed in results:
        status = "✅ PASS" if passed else "❌ FAIL"
        print(f"{name}: {status}")
        if not passed:
            all_passed = False
    
    print("=" * 50)
    if all_passed:
        print("✅ All tests passed!")
        return 0
    else:
        print("❌ Some tests failed")
        return 1

if __name__ == "__main__":
    sys.exit(main())

