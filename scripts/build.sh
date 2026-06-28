#!/bin/bash
# SPDX-License-Identifier: MIT
#
# AGEZT Build Script
# Explicitly builds with CGO_ENABLED=0 (pure Go, no C dependencies)
#
# Usage:
#   ./scripts/build.sh              # Build all packages
#   ./scripts/build.sh test         # Run tests
#   ./scripts/build.sh race         # Run tests with race detector (requires GCC)
#   ./scripts/build.sh clean        # Clean build artifacts
#
# Note: Race detector requires CGO (GCC). For normal builds, CGO is disabled.

set -euo pipefail

# Explicitly disable CGO - this is a PURE GO build
export CGO_ENABLED=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

# ----- Build identity -----------------------------------------------------
#
# Mirror of the Makefile's LDFLAGS / GOFLAGS so a `bash scripts/build.sh`
# call produces the same stamped binary as `make build`. Defaults are
# git-derived when run inside a checkout; override on the command line
# for release builds, e.g.:
#
#   VERSION=1.2.3 COMMIT=$(git rev-parse --short HEAD) \
#     BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) ./scripts/build.sh
#
VERSION="${VERSION:-$(git describe --tags --always --dirty=-dev 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)}"

LDFLAGS="-s -w \
    -X 'github.com/agezt/agezt/internal/brand.Version=${VERSION}' \
    -X 'github.com/agezt/agezt/internal/brand.BuildCommit=${COMMIT}' \
    -X 'github.com/agezt/agezt/internal/brand.BuildTime=${BUILD_TIME}'"

# `-trimpath` strips the absolute paths of the build host from the
# binary's symbol info so the same source reproduces the same bit-for-bit
# output across machines.
GOFLAGS="-trimpath"

case "${1:-build}" in
    build)
        echo "=== Building AGEZT (CGO_ENABLED=0, version=${VERSION}, commit=${COMMIT}) ==="
        go build ${GOFLAGS} -ldflags="${LDFLAGS}" ./...
        echo "Build complete: $(go env GOBIN)/agezt"
        ;;

    test)
        echo "=== Running Tests (CGO_ENABLED=0) ==="
        go test ./...
        echo "All tests passed!"
        ;;

    race)
        echo "=== WARNING: Race detector requires CGO ==="
        echo "Setting CGO_ENABLED=1 (requires GCC)"
        export CGO_ENABLED=1
        go test -race ./...
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
