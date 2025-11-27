# Python AI Service Dockerfile
# Python 3.13+ with PyTorch, ONNX, OpenCV for CAE training and heavy model inference

FROM python:3.13-slim

WORKDIR /app

# Install system dependencies
# Note: libgl1-mesa-glx is not available on newer Debian; libgl1 provides the needed GL symbols.
RUN apt-get update && apt-get install -y \
    build-essential \
    libgl1 \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    libgomp1 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first for better caching
COPY user-vm-api/training-service/requirements.txt /app/requirements.txt

# Install Python dependencies
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r /app/requirements.txt

# Copy application code
COPY user-vm-api/training-service/ /app/

# Create data directories
RUN mkdir -p /app/data/datasets /app/data/models

# Expose FastAPI port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Run FastAPI service
CMD ["python", "main.py"]
