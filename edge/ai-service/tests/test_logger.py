"""
Unit tests for logging setup.
"""

import json
import logging
import sys
from io import StringIO
from pathlib import Path

import pytest

from ai_service.logger import setup_logging, JSONFormatter, TextFormatter
from ai_service.config import LogConfig


class TestJSONFormatter:
    """Tests for JSONFormatter."""
    
    def test_json_formatter_basic(self):
        """Test JSON formatter with basic log record."""
        formatter = JSONFormatter()
        record = logging.LogRecord(
            name="test",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="Test message",
            args=(),
            exc_info=None,
        )
        
        output = formatter.format(record)
        data = json.loads(output)
        
        assert data["level"] == "INFO"
        assert data["logger"] == "test"
        assert data["message"] == "Test message"
        assert "timestamp" in data
    
    def test_json_formatter_with_extra(self):
        """Test JSON formatter with extra fields."""
        formatter = JSONFormatter()
        record = logging.LogRecord(
            name="test",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="Test message",
            args=(),
            exc_info=None,
        )
        record.custom_field = "custom_value"
        
        output = formatter.format(record)
        data = json.loads(output)
        
        assert data["custom_field"] == "custom_value"
    
    def test_json_formatter_with_exception(self):
        """Test JSON formatter with exception."""
        formatter = JSONFormatter()
        try:
            raise ValueError("Test error")
        except ValueError:
            record = logging.LogRecord(
                name="test",
                level=logging.ERROR,
                pathname="test.py",
                lineno=1,
                msg="Test error",
                args=(),
                exc_info=sys.exc_info(),
            )
        
        output = formatter.format(record)
        data = json.loads(output)
        
        assert data["level"] == "ERROR"
        assert "exception" in data


class TestTextFormatter:
    """Tests for TextFormatter."""
    
    def test_text_formatter_basic(self):
        """Test text formatter with basic log record."""
        formatter = TextFormatter()
        record = logging.LogRecord(
            name="test",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="Test message",
            args=(),
            exc_info=None,
        )
        
        output = formatter.format(record)
        
        assert "INFO" in output
        assert "test" in output
        assert "Test message" in output
    
    def test_text_formatter_with_extra(self):
        """Test text formatter with extra fields."""
        formatter = TextFormatter()
        record = logging.LogRecord(
            name="test",
            level=logging.INFO,
            pathname="test.py",
            lineno=1,
            msg="Test message",
            args=(),
            exc_info=None,
        )
        record.custom_field = "custom_value"
        
        output = formatter.format(record)
        
        assert "custom_value" in output


class TestSetupLogging:
    """Tests for setup_logging function."""
    
    def test_setup_logging_json_format(self):
        """Test logging setup with JSON format."""
        config = LogConfig(level="INFO", format="json", output="stdout")
        setup_logging(config)
        
        logger = logging.getLogger("test")
        logger.info("Test message")
        
        # Verify root logger has handler
        root_logger = logging.getLogger()
        assert len(root_logger.handlers) > 0
        assert isinstance(root_logger.handlers[0].formatter, JSONFormatter)
    
    def test_setup_logging_text_format(self):
        """Test logging setup with text format."""
        config = LogConfig(level="DEBUG", format="text", output="stdout")
        setup_logging(config)
        
        logger = logging.getLogger("test")
        logger.debug("Test message")
        
        # Verify root logger has handler
        root_logger = logging.getLogger()
        assert len(root_logger.handlers) > 0
        assert isinstance(root_logger.handlers[0].formatter, TextFormatter)
    
    def test_setup_logging_file_output(self, temp_dir: Path):
        """Test logging setup with file output."""
        log_file = temp_dir / "test.log"
        config = LogConfig(level="INFO", format="text", output=str(log_file))
        setup_logging(config)
        
        logger = logging.getLogger("test")
        logger.info("Test message")
        
        # Verify log file was created
        assert log_file.exists()
        assert "Test message" in log_file.read_text()
    
    def test_setup_logging_log_levels(self):
        """Test logging setup with different log levels."""
        levels = ["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"]
        
        for level in levels:
            config = LogConfig(level=level, format="text", output="stdout")
            setup_logging(config)
            
            root_logger = logging.getLogger()
            assert root_logger.level == getattr(logging, level)
    
    def test_setup_logging_third_party_levels(self):
        """Test that third-party loggers are set to WARNING."""
        config = LogConfig(level="DEBUG", format="text", output="stdout")
        setup_logging(config)
        
        uvicorn_logger = logging.getLogger("uvicorn")
        assert uvicorn_logger.level == logging.WARNING
        
        fastapi_logger = logging.getLogger("fastapi")
        assert fastapi_logger.level == logging.WARNING

