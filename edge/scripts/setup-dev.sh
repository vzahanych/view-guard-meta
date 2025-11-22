#!/usr/bin/env bash
set -euo pipefail

echo "=========================================="
echo "Edge Appliance - Development Setup"
echo "=========================================="
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EDGE_DIR="$(dirname "$SCRIPT_DIR")"

echo "Step 1: Checking prerequisites..."

if ! command -v go &> /dev/null; then
    echo "  ❌ Go is not installed. Please install Go 1.25+"
    echo "     Visit: https://go.dev/dl/"
    exit 1
else
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo "  ✓ Go $GO_VERSION found"
fi

if ! command -v python3 &> /dev/null; then
    echo "  ❌ Python 3 is not installed. Please install Python 3.12+"
    exit 1
else
    PYTHON_VERSION=$(python3 --version | awk '{print $2}')
    echo "  ✓ Python $PYTHON_VERSION found"
fi

echo ""
echo "Step 2: Setting up Go dependencies..."

cd "$EDGE_DIR"
if [ -f "go.mod" ]; then
    echo "  → Running go mod download..."
    go mod download
    echo "  ✓ Go dependencies installed"
else
    echo "  ⚠ go.mod not found, skipping"
fi

echo ""
echo "Step 3: Setting up Python environment..."

if [ ! -d "ai-service/venv" ]; then
    echo "  → Creating Python virtual environment..."
    cd "$EDGE_DIR/ai-service"
    python3 -m venv venv
    echo "  ✓ Virtual environment created"
else
    echo "  ✓ Virtual environment already exists"
fi

cd "$EDGE_DIR/ai-service"
if [ -f "requirements.txt" ]; then
    echo "  → Installing Python dependencies..."
    source venv/bin/activate
    pip install --upgrade pip
    pip install -r requirements.txt
    echo "  ✓ Python dependencies installed"
else
    echo "  ⚠ requirements.txt not found, creating placeholder..."
    cat > requirements.txt << EOF
# Python dependencies for Edge AI Service
# Add your dependencies here
EOF
fi

echo ""
echo "Step 4: Creating local data directories..."

mkdir -p "$EDGE_DIR/data/clips"
mkdir -p "$EDGE_DIR/data/snapshots"
mkdir -p "$EDGE_DIR/data/db"
echo "  ✓ Data directories created"

echo ""
echo "Step 5: Checking development tools..."

if command -v golangci-lint &> /dev/null; then
    echo "  ✓ golangci-lint found"
else
    echo "  ⚠ golangci-lint not found. Install with:"
    echo "     go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
fi

if command -v black &> /dev/null; then
    echo "  ✓ black (Python formatter) found"
else
    echo "  ⚠ black not found. Install with:"
    echo "     pip install black"
fi

if command -v pylint &> /dev/null; then
    echo "  ✓ pylint found"
else
    echo "  ⚠ pylint not found. Install with:"
    echo "     pip install pylint"
fi

echo ""
echo "=========================================="
echo "Development setup complete!"
echo "=========================================="
echo ""
echo "Next steps:"
echo "  1. Review config/config.dev.yaml for local development settings"
echo "  2. Start Docker Compose services: cd ../../infra/local && docker-compose up -d"
echo "  3. Test RTSP stream: rtsp://localhost:8554/test"
echo "  4. Start developing in edge/orchestrator/ and edge/ai-service/"
echo ""

