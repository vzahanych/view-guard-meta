#!/usr/bin/env bash
set -euo pipefail

echo "=========================================="
echo "The Private AI Guardian - Bootstrap Script"
echo "=========================================="
echo ""

PUBLIC_SUBMODULES=("edge-oss" "crypto-oss" "proto-oss")
PRIVATE_SUBMODULES=("kvm-agent" "saas-backend" "saas-frontend" "infra")

echo "Step 1: Initializing public submodules..."
for submodule in "${PUBLIC_SUBMODULES[@]}"; do
    if [ -f ".gitmodules" ] && grep -q "\[submodule.*$submodule" .gitmodules 2>/dev/null; then
        echo "  → Initializing $submodule..."
        if git submodule update --init "$submodule" 2>/dev/null; then
            echo "    ✓ $submodule initialized"
        else
            echo "    ⚠ $submodule not found in .gitmodules (will be added later)"
        fi
    else
        echo "    ⚠ $submodule not configured yet (will be added when repositories are created)"
    fi
done

echo ""
echo "Step 2: Attempting to initialize private submodules (if you have access)..."
PRIVATE_INITIALIZED=0
for submodule in "${PRIVATE_SUBMODULES[@]}"; do
    if [ -f ".gitmodules" ] && grep -q "\[submodule.*$submodule" .gitmodules 2>/dev/null; then
        echo "  → Attempting to initialize $submodule..."
        if git submodule update --init "$submodule" 2>/dev/null; then
            echo "    ✓ $submodule initialized"
            PRIVATE_INITIALIZED=$((PRIVATE_INITIALIZED + 1))
        else
            echo "    ⚠ $submodule not accessible (expected if you're an external contributor)"
        fi
    fi
done

if [ $PRIVATE_INITIALIZED -eq 0 ]; then
    echo ""
    echo "  ℹ No private submodules were initialized. This is expected if you are:"
    echo "    - An external contributor"
    echo "    - Setting up the repository for the first time"
    echo "    - Private repositories haven't been created yet"
fi

echo ""
echo "Step 3: Running sanity checks on public repos..."

if [ -d "edge-oss" ] && [ -f "edge-oss/go.mod" ]; then
    echo "  → Checking edge-oss..."
    pushd edge-oss >/dev/null 2>&1 || true
    if command -v go >/dev/null 2>&1; then
        if go test ./... 2>/dev/null; then
            echo "    ✓ edge-oss tests passed"
        else
            echo "    ⚠ edge-oss tests failed or no tests found"
        fi
    else
        echo "    ⚠ Go not installed, skipping tests"
    fi
    popd >/dev/null 2>&1 || true
fi

if [ -d "proto-oss" ] && [ -f "proto-oss/go.mod" ]; then
    echo "  → Checking proto-oss..."
    pushd proto-oss >/dev/null 2>&1 || true
    if command -v go >/dev/null 2>&1; then
        if go test ./... 2>/dev/null || true; then
            echo "    ✓ proto-oss checks passed"
        else
            echo "    ⚠ proto-oss checks failed or no tests found"
        fi
    else
        echo "    ⚠ Go not installed, skipping tests"
    fi
    popd >/dev/null 2>&1 || true
fi

echo ""
echo "=========================================="
echo "Bootstrap complete!"
echo "=========================================="
echo ""
echo "Public components are ready for development."
echo ""
echo "Next steps:"
echo "  1. Review the documentation in the root directory"
echo "  2. Check out individual component repositories for detailed setup"
echo "  3. See PROJECT_STRUCTURE.md for repository organization"
echo ""

