# SPDX-License-Identifier: MIT
# Agezt Makefile — v1 substrate targets (gen / build / test).
# Requires GNU make + bash (Linux/macOS native; Git-Bash on Windows).

GO       ?= go
BIN_DIR  ?= bin
GOFLAGS  := -trimpath
LDFLAGS  := -s -w
CONTRACT := .project/agezt-contract.jsonc
GEN_FILE := contract/gen/types.gen.go
GEN_PKG  := gen

export CGO_ENABLED := 0

.PHONY: all help gen frontend-build frontend-test build install run test vet fmt lint clean check deps-check

all: build ## (default) build all binaries

help: ## show this help
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

gen: $(GEN_FILE) ## regenerate SDK types from the contract

frontend-build: ## build the React Web UI → kernel/webui/dist (committed, go:embed)
	cd frontend && npm ci && npm run build

frontend-test: ## unit-test the Web UI logic (Vitest)
	cd frontend && npm ci && npm test

$(GEN_FILE): $(CONTRACT) tools/jsonschemagen/main.go
	@mkdir -p $(dir $(GEN_FILE))
	$(GO) run ./tools/jsonschemagen -in $(CONTRACT) -out $(GEN_FILE) -pkg $(GEN_PKG)

build: gen ## build all binaries into $(BIN_DIR)/
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(BIN_DIR)/ ./cmd/...

install: gen ## install agt + agezt onto your PATH (GOBIN / GOPATH/bin)
	$(GO) install $(GOFLAGS) -ldflags='$(LDFLAGS)' ./cmd/...

run: build ## build, then run the agezt daemon in the foreground
	$(BIN_DIR)/agezt

test: gen ## run unit tests
	$(GO) test $(GOFLAGS) ./...

e2e: build ## boot the real daemon and smoke every core surface end-to-end
	bash scripts/e2e-smoke.sh $(BIN_DIR)/agezt $(BIN_DIR)/agt

vet: gen ## run go vet
	$(GO) vet ./...

fmt: ## format with gofmt
	$(GO) fmt ./...

lint: vet ## static checks (extend with golangci-lint later)

deps-check: ## fail on any module not listed in tools/depscheck/allowlist.txt
	$(GO) run ./tools/depscheck

check: gen vet test deps-check ## CI gate: gen + vet + test + deps-check

clean: ## remove build artifacts
	rm -rf $(BIN_DIR) $(GEN_FILE)
