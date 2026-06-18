# SPDX-License-Identifier: MIT
#
# AGEZT Makefile
# Explicitly builds with CGO_ENABLED=0 (pure Go, no C dependencies)
#
# Requires: Go 1.21+, Make
# Note: This project does NOT use CGO - pure Go build

.PHONY: all build test race clean vet install webui-e2e webui-e2e-ps

# Explicitly disable CGO - this is a PURE GO build
export CGO_ENABLED := 0

all: build

build:
	@echo "Building AGEZT (CGO_ENABLED=0)..."
	go build -ldflags="-s -w" ./...
	@echo "Build complete"

test:
	@echo "Running Tests (CGO_ENABLED=0)..."
	go test -v ./...

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

install:
	@echo "Installing AGEZT..."
	go install -ldflags="-s -w" ./cmd/agezt

# Cross-compilation targets (CGO disabled, so pure Go cross-compile works)
linux:
	@set GOOS=linux && set GOARCH=amd64 && go build -ldflags="-s -w" -o bin/agezt-linux ./cmd/agezt

darwin:
	@set GOOS=darwin && set GOARCH=amd64 && go build -ldflags="-s -w" -o bin/agezt-darwin ./cmd/agezt

windows:
	go build -ldflags="-s -w" -o bin/agezt.exe ./cmd/agezt
