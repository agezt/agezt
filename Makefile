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

.PHONY: all help gen build test vet fmt lint clean check deps-check

all: build ## (default) build all binaries

help: ## show this help
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

gen: $(GEN_FILE) ## regenerate SDK types from the contract

$(GEN_FILE): $(CONTRACT) tools/jsonschemagen/main.go
	@mkdir -p $(dir $(GEN_FILE))
	$(GO) run ./tools/jsonschemagen -in $(CONTRACT) -out $(GEN_FILE) -pkg $(GEN_PKG)

build: gen ## build all binaries into $(BIN_DIR)/
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags='$(LDFLAGS)' -o $(BIN_DIR)/ ./cmd/...

test: gen ## run unit tests
	$(GO) test $(GOFLAGS) ./...

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
