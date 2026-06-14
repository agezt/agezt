#!/bin/bash
# SPDX-License-Identifier: MIT
#
# AGEZT Build Script
# Explicitly builds with CGO_ENABLED=0 (pure Go, no C dependencies)
#
# Usage:
#   ./build.sh              # Build all packages
#   ./build.sh test         # Run tests
#   ./build.sh race         # Run tests with race detector (requires GCC)
#   ./build.sh clean        # Clean build artifacts
#
# Note: Race detector requires CGO (GCC). For normal builds, CGO is disabled.

set -euo pipefail

# Explicitly disable CGO - this is a PURE GO build
export CGO_ENABLED=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

case "${1:-build}" in
    build)
        echo "=== Building AGEZT (CGO_ENABLED=0) ==="
        go build -ldflags="-s -w" ./...
        echo "Build complete: $(go env GOBIN)/agezt"
        ;;

    test)
        echo "=== Running Tests (CGO_ENABLED=0) ==="
        go test -v ./...
        echo "All tests passed!"
        ;;

    race)
        echo "=== WARNING: Race detector requires CGO ==="
        echo "Setting CGO_ENABLED=1 (requires GCC)"
        export CGO_ENABLED=1
        go test -race -v ./...
        ;;

    clean)
        echo "=== Cleaning ==="
        go clean
        rm -rf bin/
        echo "Clean complete"
        ;;

    vet)
        echo "=== Running go vet ==="
        go vet ./...
        echo "Vet passed!"
        ;;

    *)
        echo "Usage: $0 {build|test|race|clean|vet}"
        exit 1
        ;;
esac
