"""
Structured logging setup for Edge AI Service.

Provides JSON and text logging formats with configurable levels.
"""

import json
import logging
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Dict

from ai_service.config import LogConfig


class JSONFormatter(logging.Formatter):
    """JSON formatter for structured logging."""
    
    def format(self, record: logging.LogRecord) -> str:
        """Format log record as JSON."""
        log_data: Dict[str, Any] = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        
        # Add exception info if present
        if record.exc_info:
            log_data["exception"] = self.formatException(record.exc_info)
        
        # Add extra fields from record
        if hasattr(record, "extra") and record.extra:
            log_data.update(record.extra)
        else:
            # Extract extra fields from record attributes
            for key, value in record.__dict__.items():
                if key not in [
                    "name", "msg", "args", "created", "filename", "funcName",
                    "levelname", "levelno", "lineno", "module", "msecs",
                    "message", "pathname", "process", "processName", "relativeCreated",
                    "thread", "threadName", "exc_info", "exc_text", "stack_info",
                ]:
                    if not key.startswith("_"):
                        log_data[key] = value
        
        return json.dumps(log_data)


class TextFormatter(logging.Formatter):
    """Human-readable text formatter for logging."""
    
    def __init__(self):
        super().__init__(
            fmt="%(asctime)s [%(levelname)-8s] %(name)s: %(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
        )
    
    def format(self, record: logging.LogRecord) -> str:
        """Format log record as text."""
        # Add extra fields to message if present
        message = super().format(record)
        
        # Extract extra fields
        extra_fields = {}
        for key, value in record.__dict__.items():
            if key not in [
                "name", "msg", "args", "created", "filename", "funcName",
                "levelname", "levelno", "lineno", "module", "msecs",
                "message", "pathname", "process", "processName", "relativeCreated",
                "thread", "threadName", "exc_info", "exc_text", "stack_info",
            ]:
                if not key.startswith("_") and value is not None:
                    extra_fields[key] = value
        
        if extra_fields:
            message += f" | {json.dumps(extra_fields)}"
        
        return message


def setup_logging(config: LogConfig):
    """
    Setup logging configuration.
    
    Args:
        config: Logging configuration
    """
    # Determine log level
    log_level = getattr(logging, config.level.upper(), logging.INFO)
    
    # Create formatter
    if config.format == "json":
        formatter = JSONFormatter()
    else:
        formatter = TextFormatter()
    
    # Setup handler
    if config.output == "stdout" or config.output == "":
        handler = logging.StreamHandler(sys.stdout)
    else:
        # File output
        log_file = Path(config.output)
        log_file.parent.mkdir(parents=True, exist_ok=True)
        handler = logging.FileHandler(log_file)
    
    handler.setFormatter(formatter)
    handler.setLevel(log_level)
    
    # Configure root logger
    root_logger = logging.getLogger()
    root_logger.setLevel(log_level)
    root_logger.handlers = [handler]  # Replace existing handlers
    
    # Set log level for third-party libraries
    logging.getLogger("uvicorn").setLevel(logging.WARNING)
    logging.getLogger("uvicorn.access").setLevel(logging.WARNING)
    logging.getLogger("fastapi").setLevel(logging.WARNING)

