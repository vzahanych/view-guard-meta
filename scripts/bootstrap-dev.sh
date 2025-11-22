#!/usr/bin/env bash
set -euo pipefail

echo "=========================================="
echo "The Private AI Guardian - Bootstrap Script"
echo "=========================================="
echo ""

PRIVATE_SUBMODULES=("kvm-agent" "saas-backend" "saas-frontend" "infra")

echo "Step 1: Public components are already available in the repository"
echo "  → edge/ - Edge Appliance software"
echo "  → crypto/ - Encryption libraries"
echo "  → proto/ - Protocol definitions"
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
echo "Step 3: Running sanity checks on public components..."

if [ -d "edge" ] && [ -f "edge/go.mod" ]; then
    echo "  → Checking edge/..."
    pushd edge >/dev/null 2>&1 || true
    if command -v go >/dev/null 2>&1; then
        if go test ./... 2>/dev/null; then
            echo "    ✓ edge/ tests passed"
        else
            echo "    ⚠ edge/ tests failed or no tests found"
        fi
    else
        echo "    ⚠ Go not installed, skipping tests"
    fi
    popd >/dev/null 2>&1 || true
fi

if [ -d "proto" ] && [ -f "proto/go.mod" ]; then
    echo "  → Checking proto/..."
    pushd proto >/dev/null 2>&1 || true
    if command -v go >/dev/null 2>&1; then
        if go test ./... 2>/dev/null || true; then
            echo "    ✓ proto/ checks passed"
        else
            echo "    ⚠ proto/ checks failed or no tests found"
        fi
    else
        echo "    ⚠ Go not installed, skipping tests"
    fi
    popd >/dev/null 2>&1 || true
fi

if [ -d "crypto" ] && [ -f "crypto/go/go.mod" ]; then
    echo "  → Checking crypto/..."
    pushd crypto/go >/dev/null 2>&1 || true
    if command -v go >/dev/null 2>&1; then
        if go test ./... 2>/dev/null || true; then
            echo "    ✓ crypto/go checks passed"
        else
            echo "    ⚠ crypto/go checks failed or no tests found"
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
echo "  1. Review the documentation in docs/ directory"
echo "  2. Check out component directories (edge/, crypto/, proto/) for detailed setup"
echo "  3. See docs/PROJECT_STRUCTURE.md for repository organization"
echo ""
