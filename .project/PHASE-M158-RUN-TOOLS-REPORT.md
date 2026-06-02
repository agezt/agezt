# M158 — `agt run --tools <csv>` / `--no-tools`: per-run tool restriction

## Why
The per-run override family (`--model`, `--system`, `--timeout`) lets an operator
scope one run without touching daemon config. The missing axis was *tools*: there
was no way to run a single intent against a restricted (or empty) toolset. Two
concrete needs:
- **Safe pure-reasoning** — `agt run --no-tools "summarize this"` guarantees the
  model cannot touch the shell, filesystem, or network for that run, no matter what
  it decides to call.
- **Scoped capability** — `agt run --tools file "read config.json and explain it"`
  permits a read without exposing shell/http for that one run.

This completes the per-run override family and is purely additive — omit the flag
and behavior is exactly as before (full kernel toolset).

## What
A new ctx-override following the established `WithModel`/`WithSystem`/`WithRunTimeout`
pattern, plus CLI flags and control-plane arg plumbing.

### kernel/runtime/runtime.go
- `ctxKeyTools` added to the ctx-key iota block.
- `WithTools(ctx, allow []string) context.Context` — carries the per-run allow-list.
- `toolsFromCtx(ctx) ([]string, bool)` — `ok=false` means *no restriction* (use all
  tools); `ok=true` with an empty/nil slice means *no tools* (the `--no-tools` case).
  This two-valued return is what distinguishes "unset" from "explicitly empty".
- `filterTools(tools map[string]agent.Tool, allow []string) map[string]agent.Tool` —
  returns the subset whose names appear in `allow`. Unknown names in `allow` are
  ignored; duplicates collapse; an empty/nil `allow` yields an empty map. Does not
  mutate the source map.
- In `RunWith`, after assembling the run ctx:
  ```go
  runTools := k.tools
  if allow, ok := toolsFromCtx(runCtx); ok {
      runTools = filterTools(k.tools, allow)
  }
  ```
  and `LoopConfig.Tools: runTools`. The agent loop advertises only `runTools` to the
  model, and any tool call it makes is dispatched against `runTools` — a filtered-out
  tool surfaces as `tool "X" is not available` (the same path as a genuinely unknown
  tool), fed back to the model so it can adapt.

### kernel/controlplane/server.go (`handleRun`)
- Parses a `"tools"` run arg (present, possibly an empty list) into `[]string` and
  sets `ctx = runtime.WithTools(ctx, allow)`. Absent arg → no restriction.

### cmd/agt/main.go (`run` subcommand)
- `--no-tools` → `toolsSet = true` with an empty list (disable all tools).
- `--tools <csv>` / `--tools=<csv>` → split on commas, trim, skip empties, append to
  the list (and `toolsSet = true`).
- An explicit empty allow-list is sent as `"tools": []` so the server can tell it
  apart from omission (`toolsList == nil` is normalized to `[]any{}` before send).

## Tests (+9, all passing)
New internal (`package runtime`) white-box test file `tools_internal_test.go`:
- `TestFilterTools` (7 subtests) — subset, single, empty-allow=no-tools,
  nil-allow=no-tools, unknown-name-ignored, all-names, duplicate-dedup; plus an
  assertion that the source toolset is not mutated.
- `TestWithTools_RoundTrip` — bare ctx → `ok=false`; non-empty allow round-trips;
  explicit empty allow → `ok=true` with zero names (the `--no-tools` distinction) and
  `filterTools` by it yields no tools.

## Live proof (offline mock, fresh daemon per behavior)
The mock fixture scripts a `shell` tool call then a final answer.
- `agt run --no-tools --json "list files"` → `tool.result` payload:
  `{"error": true, "output": "tool \"shell\" is not available", "tool": "shell"}` —
  shell correctly filtered out; the model's call is rejected and fed back.
- `agt run --tools shell --json "list files"` → `tool.result` payload:
  `{"error": false, "output": " Volume in drive C ... Directory of ...", "tool":
  "shell"}` — shell allowed; the call executes and returns a real listing.

(`tool.invoked` is published *before* the registry lookup, so its presence in the
event stream does not by itself indicate the tool ran — the `tool.result` `error`
flag and `output` are authoritative. Verified via `--json`.)

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./... -count=1` — **FAIL 0**, **1498 tests** (was 1489; +9), 61 packages.

## Result
An operator can now bound a single run's tool access — none, or a named subset —
without restarting or reconfiguring the daemon. The per-run override family
(`--model` / `--system` / `--timeout` / `--tools`) is complete.
