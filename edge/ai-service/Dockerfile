# Use Python 3.12 slim image
FROM python:3.12-slim

WORKDIR /app

# Install system dependencies (including OpenCV runtime deps and curl for health checks)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libgl1 \
    libglib2.0-0 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first for better caching
COPY requirements.txt .

# Install Python dependencies (including OpenVINO and ultralytics for model download)
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY . .

# Create models directory
RUN mkdir -p /app/models

# Copy model download script
COPY scripts/download_model.py /app/scripts/download_model.py
COPY scripts/download_model.sh /app/scripts/download_model.sh
RUN chmod +x /app/scripts/download_model.sh

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Expose API port
EXPOSE 8080

# Run the AI service (with model download if needed)
# Note: In production, models are downloaded from remote VM, not locally
CMD ["/app/entrypoint.sh"]

