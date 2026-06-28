# SPDX-License-Identifier: MIT
#
# AGEZT Makefile
# Explicitly builds with CGO_ENABLED=0 (pure Go, no C dependencies)
#
# Requires: Go 1.26.4+ (see go.mod), Make
# Note: This project does NOT use CGO - pure Go build

.PHONY: all build test race clean vet install gen deps-check sdk-parity deadcode-check frontend-build frontend-test frontend-deadcode e2e check webui-e2e webui-e2e-ps

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
	@echo "Installing AGEZT..."
	go install -ldflags="-s -w" ./cmd/agezt

# Cross-compilation targets (CGO disabled, so pure Go cross-compile works)
linux:
	@set GOOS=linux && set GOARCH=amd64 && go build -ldflags="-s -w" -o bin/agezt-linux ./cmd/agezt

darwin:
	@set GOOS=darwin && set GOARCH=amd64 && go build -ldflags="-s -w" -o bin/agezt-darwin ./cmd/agezt

windows:
	go build -ldflags="-s -w" -o bin/agezt.exe ./cmd/agezt
