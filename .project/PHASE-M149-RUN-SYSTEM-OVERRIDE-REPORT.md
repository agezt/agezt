# M149 — `agt run --system <prompt>` (per-run system-prompt override)

## Why
The sibling of M148 (`--model`). A run's system prompt was fixed at
`AGEZT_SYSTEM_PROMPT` (the kernel's configured `System`). To run one intent with a
different persona or instruction — "answer as a terse code reviewer", "be
extra-cautious for this one" — an operator had to restart the daemon with a new
`AGEZT_SYSTEM_PROMPT`. With `--model` already proving the per-run-override pattern,
adding `--system` rounds out per-run customization (model + system) for quick
experiments, with no daemon restart.

## What
- **`kernel/runtime/runtime.go`** — a new `WithSystem(ctx, system)` /
  `systemFromCtx(ctx)` pair (mirroring `WithModel`), and the run's system assembly
  now starts from the override when present: `system := k.cfg.System; if s :=
  systemFromCtx(runCtx); s != "" { system = s }`. The override REPLACES the base
  prompt; the existing memory / world / skill injection still layers on top, so a
  per-run persona doesn't lose what Agezt knows. Empty override = the configured
  default (unchanged behavior).
- **`kernel/controlplane/server.go`** — `handleRun` reads the optional `system` arg
  and sets `ctx = runtime.WithSystem(ctx, sys)` when non-blank.
- **`cmd/agt/main.go`** — `--system <prompt>` / `--system=<prompt>` on `agt run`,
  passed as the `system` run arg; documented in help.

## Files
- `kernel/runtime/runtime.go` — `ctxKeySystem`, `WithSystem`, `systemFromCtx`,
  override in system assembly.
- `kernel/controlplane/server.go` — read `system` arg → `WithSystem`.
- `cmd/agt/main.go` — `--system` flag + help.
- `kernel/controlplane/controlplane_test.go` — `TestRun_SystemOverride`.

## Tests (+1, all passing)
- `TestRun_SystemOverride` — a run submitted with `system: "You are a terse pirate."`
  reaches the provider with `req.System` containing the override (captured via the
  mock's `OnRequest`).

## Live proof (offline mock, real booted daemon)
```
$ agt run --system "You are a terse assistant." "hi"
  --- final answer ---
  [offline-mock] …            # run succeeds (exit 0)
  usage: mock · 2 iteration(s)
```

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1474 tests** (was 1473; +1), 61 packages.

## Result
`agt run` now takes per-run `--model` and `--system` overrides — quick model and
persona experiments from one command line, no daemon restart, with the cognitive
injections (memory/world/skills) still layered on top.
