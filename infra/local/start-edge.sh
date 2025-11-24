#!/bin/bash
set -e

# Quick start script for Edge Appliance with USB camera

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🚀 Starting Edge Appliance services..."

# Check if USB camera is detected
if [ ! -e /dev/video0 ]; then
    echo "⚠️  Warning: /dev/video0 not found. USB camera may not be connected."
    echo "   Continuing anyway - camera discovery will be attempted..."
else
    echo "✅ USB camera detected: /dev/video0"
    ls -la /dev/video* 2>/dev/null | head -3
fi

# Check if config file exists
if [ ! -f "config.docker.yaml" ]; then
    echo "❌ Error: config.docker.yaml not found in $SCRIPT_DIR"
    echo "   Please create it or copy from edge/config/config.dev.yaml"
    exit 1
fi

# Build and start services
echo ""
echo "📦 Building Docker images..."
docker-compose -f docker-compose.edge.yml build

echo ""
echo "🚀 Starting services..."
docker-compose -f docker-compose.edge.yml up -d

echo ""
echo ""
echo "⏳ Waiting for services to start..."
echo "   Note: First start may take 2-3 minutes while AI model downloads"
sleep 10

# Check service status
echo ""
echo "📊 Service status:"
docker-compose -f docker-compose.edge.yml ps

echo ""
echo "📋 View logs with:"
echo "   docker-compose -f docker-compose.edge.yml logs -f"
echo ""
echo "📋 View orchestrator logs:"
echo "   docker-compose -f docker-compose.edge.yml logs -f edge-orchestrator"
echo ""
echo "📋 View AI service logs:"
echo "   docker-compose -f docker-compose.edge.yml logs -f edge-ai-service"
echo ""
echo "🛑 Stop services with:"
echo "   docker-compose -f docker-compose.edge.yml down"
echo ""

# Check health endpoints
echo ""
echo "🏥 Checking health endpoints..."
sleep 5

# Check AI service (may take longer on first start due to model download)
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ AI service health check: OK"
else
    echo "⚠️  AI service health check: Not ready yet"
    echo "   This is normal on first start - model download may take 2-3 minutes"
    echo "   Check logs: docker-compose -f docker-compose.edge.yml logs -f edge-ai-service"
fi

echo ""
echo "✨ Done! Services are starting. Check logs to see camera discovery and processing."

