# SPDX-License-Identifier: MIT
#
# AGEZT Makefile
# Explicitly builds with CGO_ENABLED=0 (pure Go, no C dependencies)
#
# Requires: Go 1.26.4+ (see go.mod), Make, git (for version stamping)
# Note: This project does NOT use CGO - pure Go build

.PHONY: all build test race clean vet install gen deps-check sdk-parity deadcode-check frontend-build frontend-test frontend-deadcode e2e check webui-e2e webui-e2e-ps

# Explicitly disable CGO - this is a PURE GO build
export CGO_ENABLED := 0

# ----- Build identity ------------------------------------------------------
#
# Every build target below stamps the kernel binary with a consistent
# identity triple (semver + commit + UTC timestamp) via `-X` ldflags, and
# uses `-trimpath` so build-host paths don't leak into the output. The
# defaults auto-detect from the git checkout when available, with safe
# fallbacks for tarball / shallow-clone / no-git-build-machine builds.
#
# Override on the command line, e.g.:
#
#   make build VERSION=1.2.3 COMMIT=$(git rev-parse --short HEAD) BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
#
# CI pipelines that publish artefacts MUST set VERSION and COMMIT so the
# embed is reproducible; the auto-detect fallback below is "dev" for
# VERSION and "unknown" for the others.

VERSION ?= $(shell git describe --tags --always --dirty=-dev 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# Use a portable timestamp invocation: `date` is in coreutils on every
# host the Makefile is expected to run on (Linux, macOS, Git Bash on
# Windows); for pure-cmd shells, scripts/build.sh's git- or fallback-
# derived stamp is the canonical path.
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)

# Stamp the brand package vars. Single-quoted to keep Go's ldflag parser
# from interpreting spaces / punctuation in $VERSION. `-s -w` strips
# symbol tables to reduce binary size; `-X` injects the values.
LDFLAGS := -s -w \
	-X 'github.com/agezt/agezt/internal/brand.Version=$(VERSION)' \
	-X 'github.com/agezt/agezt/internal/brand.BuildCommit=$(COMMIT)' \
	-X 'github.com/agezt/agezt/internal/brand.BuildTime=$(BUILD_TIME)'

# `-trimpath` strips the absolute paths of the build host from the
# binary's symbol info so the same source reproduces the same bit-for-bit
# output across machines. Combined with the stamped ldflags above, this
# makes the build reproducible.
GOFLAGS ?= -trimpath

all: build

build:
	@echo "Building AGEZT (CGO_ENABLED=0, version=$(VERSION), commit=$(COMMIT))..."
	go build $(GOFLAGS) -ldflags="$(LDFLAGS)" ./...
	@echo "Build complete"

test:
	@echo "Running Tests (CGO_ENABLED=0)..."
	go test ./...

webui-e2e:
	bash scripts/webui-e2e.sh

webui-e2e-ps:
	powershell -ExecutionPolicy Bypass -File scripts/webui-e2e.ps1

test-race:
	@echo "=== WARNING: Race detector requires CGO ==="
	@echo "Setting CGO_ENABLED=1 (requires GCC installed)"
	@set CGO_ENABLED=1 && go test -race -v ./...

clean:
	@echo "Cleaning..."
	go clean
	@if exist bin rmdir /s /q bin
	@echo "Clean complete"

vet:
	@echo "Running go vet..."
	go vet ./...
	@echo "Vet passed!"

gen:
	@echo "Generating contract types..."
	go run ./tools/jsonschemagen -in .project/agezt-contract.jsonc -out contract/gen/types.gen.go -pkg gen

deps-check:
	@echo "Checking dependency allowlist..."
	go run ./tools/depscheck

sdk-parity:
	@echo "Checking SDK parity report..."
	go run ./tools/sdkparity -check docs/SDK-PARITY.md

deadcode-check:
	@echo "Checking for unexpected dead code..."
	go run ./tools/deadcodecheck

frontend-build:
	@echo "Building frontend..."
	cd frontend && npm run build

frontend-test:
	@echo "Testing frontend..."
	cd frontend && npm test

frontend-deadcode:
	@echo "Checking frontend for unused exports and dependencies..."
	cd frontend && npm run deadcode

e2e:
	bash scripts/e2e-smoke.sh

check: gen vet test deps-check sdk-parity deadcode-check frontend-deadcode frontend-test

install:
	@echo "Installing AGEZT (version=$(VERSION), commit=$(COMMIT))..."
	go install $(GOFLAGS) -ldflags="$(LDFLAGS)" ./cmd/agezt

# Cross-compilation targets (CGO disabled, so pure Go cross-compile works).
# Every target uses the same LDFLAGS/GOFLAGS so the only thing that
# varies across platforms is the output filename and GOOS/GOARCH.
linux:
	@set GOOS=linux && set GOARCH=amd64 && go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/agezt-linux ./cmd/agezt

darwin:
	@set GOOS=darwin && set GOARCH=amd64 && go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/agezt-darwin ./cmd/agezt

windows:
	set GOOS=windows GOARCH=amd64 && go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o bin/agezt.exe ./cmd/agezt

