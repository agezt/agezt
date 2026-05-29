# Phase Report — Milestone 0 (Foundation)

> Status: **shipped** · Date: 2026-05-29 · Branch: `main`
> Per [BUILD-GUIDE §4](BUILD-GUIDE.md) and [ROADMAP §1](ROADMAP.md).

## What shipped

Repository foundation per [STRUCTURE.md](STRUCTURE.md) (M0 subset). Zero
external dependencies — stdlib only ([POLICY §1](POLICY.md)).

- `LICENSE` — MIT, repo root.
- `internal/brand/brand.go` — every name/path/protocol constant in one place
  (DECISIONS A1). `Name`, `Binary`, `CLI`, `EnvPrefix`, `ConfigDir`,
  `ProtocolVersion`, `Version`. Frozen-identity unit test.
- `cmd/agezt/main.go` — kernel binary; `--version`, `--help`, default
  identity print. Unit-tested via in-process `run()`.
- `cmd/agt/main.go` — CLI binary; `--version`, `--help`, default usage with
  planned-for-M0.5 commands listed. Unit-tested.
- `tools/jsonschemagen/` — JSON Schema → Go SDK type generator
  (DECISIONS G2). Reads `.project/agezt-contract.jsonc`, strips JSONC
  comments (string-aware), parses, emits a `gofmt`-clean Go file with
  deterministic ordering. Unit tests cover comment stripping, identifier
  conversion (snake_case → PascalCase with Go initialisms), `$ref`
  resolution, and an end-to-end pass over the real contract.
- `contract/gen/types.gen.go` — generated SDK types: `Event`, `Attachment`,
  `UnifiedMessage`, `RegisterParams`/`RegisterResult`, `HealthResult`,
  `Capability`, `Contribution`, `PluginError`, `EventEmit`,
  `CompletionRequest`/`CompletionChunk`, `ChatMessage`, `ToolDef`,
  `ToolCall`, `CompletionOptions`, `Usage`, `ToolSchema`, `ToolInvocation`,
  `ToolEvent`, `ProviderCompletionRequest`, `ProviderChunk`, `ModelInfo`,
  `Limits`.
- `Makefile` — `gen`, `build`, `test`, `vet`, `lint`, `deps-check`, `check`,
  `clean`. GNU make + bash assumed (works on Git-Bash on Windows).
- `.github/workflows/ci.yml` — three jobs: matrix `test` on Linux/macOS/Win
  (`go vet`, `go test -race`, `go build`); `codegen-in-sync` (regenerate +
  fail on diff); `multi-arch` cross-build (linux/macos/win × amd64/arm64);
  `deps-check` (fail if `require` appears in `go.mod`).
- `DEPENDENCIES.md` — POLICY §1 instructions + anti-list + supersedes-note
  on the older gRPC/protobuf mention.
- `README.md` — public entry point pointing at `.project/` for design.
- `.gitignore` — `/bin/`, generated types, editor noise.

## Demo transcript

```
$ go version
go version go1.26.3 windows/amd64

$ go mod init github.com/ersinkoc/agezt
go: creating new go.mod: module github.com/ersinkoc/agezt

$ go run ./tools/jsonschemagen \
    -in .project/agezt-contract.jsonc \
    -out contract/gen/types.gen.go \
    -pkg gen
(no output; types.gen.go produced, 9.5 KB, gofmt-clean)

$ go vet ./...
(no output)

$ go test ./...
ok  github.com/ersinkoc/agezt/cmd/agt            0.160s
ok  github.com/ersinkoc/agezt/cmd/agezt         0.148s
?   github.com/ersinkoc/agezt/contract/gen       [no test files]
ok  github.com/ersinkoc/agezt/internal/brand     0.164s
ok  github.com/ersinkoc/agezt/tools/jsonschemagen 0.164s

$ go build -trimpath -ldflags='-s -w' -o bin/ ./cmd/...
$ ls -la bin/
-rwxr-xr-x  1680384  agt.exe
-rwxr-xr-x  1680384  agezt.exe

$ ./bin/agezt --version
agezt 0.0.0-m0 (protocol v1)

$ ./bin/agt --version
agt 0.0.0-m0 (protocol v1)

$ cat go.mod
module github.com/ersinkoc/agezt
go 1.26.3

$ cat go.sum
cat: go.sum: No such file or directory
```

## Exit-criteria check (BUILD-GUIDE §4 M0)

| Criterion | Status |
|---|---|
| `make build test` (or equivalent) green | ✅ `go build/test ./...` |
| Empty kernel binary prints version | ✅ `agezt --version` → `agezt 0.0.0-m0 (protocol v1)` |
| Contracts generate cleanly | ✅ `jsonschemagen` produces a 9.5 KB `gofmt`-clean Go file with 23 types from the contract |
| Repo layout per `STRUCTURE.md` (M0 subset) | ✅ with one deliberate deviation (below) |
| MIT `LICENSE` at root, SPDX headers in sources | ✅ |
| Stdlib-first (`DEPENDENCIES.md` empty) | ✅ `go.sum` doesn't exist |
| CI skeleton (multi-arch build, test, lint, dep-check) | ✅ `.github/workflows/ci.yml` |
| Brand constants in one place | ✅ `internal/brand/brand.go` |

## Deviations from the spec (intentional)

1. **`.project/` not moved to `docs/`.** STRUCTURE.md says to relocate the
   spec suite under `docs/`. We kept `.project/` in place per user choice;
   the codegen reads `.project/agezt-contract.jsonc` directly and the
   README links to `.project/*.md`. If the layout is later reconciled, only
   the Makefile and `tools/jsonschemagen` default flag need updating.
2. **No `_ARCHIVE/` folder created.** BUILD-GUIDE §1 says deprecated files
   (old gRPC `agezt.proto`, `agezt.proto.deprecated`) should be moved to
   `_ARCHIVE/`. They remain in `.project/` per the "keep .project/ as-is"
   decision; both files are already named-suffixed/labelled as superseded
   and are not referenced by any build target.
3. **JSON Schema codegen completeness.** The generator handles all 23
   schemas currently in the contract. Enum-only fields without a `type`
   (e.g. `Capability.Kind`, `ChatMessage.Role`) currently emit
   `json.RawMessage`; this is correct (no information loss) but less
   ergonomic than a typed alias. Tracked as a follow-up; non-blocking for
   M0.5.

## Open items / TODOs for M0.5 entry

- **(blocker for M0.5):** add `kernel/event` package — canonical `Event`
  type with `Kind` constants, deterministic JSON encoding (precondition
  for the BLAKE3 hash chain, DECISIONS B3).
- **(blocker for M0.5):** add `kernel/journal` — segmented JSONL writer,
  hash-chain link, recover/verify (TASKS `P0-JRNL-01..06`).
- **(blocker for M0.5):** add `kernel/state` — first-class mutable store
  (DECISIONS B0c).
- **(blocker for M0.5):** add `kernel/bus` — subject router with
  durable-before-publish (TASKS `P0-BUS-01..03`).
- **(blocker for M0.5):** add `kernel/agent` — canonical `Provider`/`Tool`
  Go interfaces (in-process variant, DECISIONS B0a) plus the first-party
  single-agent tool-loop (DECISIONS B0d).
- **(non-blocker):** enhance `jsonschemagen` to detect all-string enums and
  emit typed string aliases instead of `json.RawMessage`.
- **(non-blocker):** enhance `jsonschemagen` to preserve field declaration
  order from the contract (currently alphabetical for determinism).
- **(non-blocker, doc):** reconcile `TASKS.md` (still references `.proto`
  files in `P0-PROTO-01..03`) and `ROADMAP.md §1/§6` (still mentions
  `buf`/`protoc`) with DECISIONS B0; add inline `(SUPERSEDED by B0 —
  use JSON-RPC + JSON Schema)` notes. Per BUILD-GUIDE §2, DECISIONS already
  wins, but the prose drift is a future-reader hazard.

## Pointers

- Build: `make build` or `go build ./...`
- Regenerate types: `make gen`
- Run binaries: `./bin/agezt --version`, `./bin/agt --help`
- Next milestone: [`ROADMAP.md §0.5`](ROADMAP.md) and the M0.5 success test.
