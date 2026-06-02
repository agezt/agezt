# M154 — `agt run --timeout <dur>` (per-run wall-clock timeout)

## Why
The third member of the per-run override family (`--model` M148, `--system` M149).
A run's wall-clock budget was fixed at the daemon-wide `AGEZT_RUN_TIMEOUT`
(`Config.MaxDuration`, M31). To bound a single run you might expect to hang — a
flaky provider, a heavy intent — you had to restart the daemon with a global cap
(which then bounds every run). A per-run `--timeout` is the natural fit, reusing the
M31 deadline machinery via the same ctx-override pattern as `--model`/`--system`.

## What
- **`kernel/runtime/runtime.go`** — `WithRunTimeout(ctx, d)` / `runTimeoutFromCtx`
  (mirroring `WithModel`/`WithSystem`). `RunWith` computes the effective budget as
  the per-run override when set, else `Config.MaxDuration`; either > 0 arms a
  `context.WithTimeout`, so an overrun cancels with `DeadlineExceeded` and the M30
  terminal emitter records `task.failed(reason=timeout)` — identical to the
  daemon-wide cap, and still distinct from a Halt (`Canceled`).
- **`kernel/controlplane/server.go`** — `handleRun` reads the optional `timeout`
  arg (a Go duration string); a malformed/non-positive value is a clear error.
- **`cmd/agt/main.go`** — `--timeout <dur>` / `--timeout=<dur>`; validated
  client-side for fast feedback, and the client connection deadline is extended to
  at least `timeout + 30s` so a long run isn't cut off by the client before the
  server's own deadline fires.

## Files
- `kernel/runtime/runtime.go` — `ctxKeyRunTimeout`, `WithRunTimeout`,
  `runTimeoutFromCtx`, effective-budget logic in `RunWith`.
- `kernel/controlplane/server.go` — parse `timeout` arg → `WithRunTimeout`.
- `cmd/agt/main.go` — `--timeout` flag, client-side validation + connection-deadline
  extension, help.
- `kernel/runtime/runtime_test.go` — `TestRunWith_PerRunTimeoutOverride`.
- `cmd/agt/run_test.go` — `TestCmdRun_InvalidTimeout`.

## Tests (+2, all passing)
- `TestRunWith_PerRunTimeoutOverride` — with `Config.MaxDuration` unset (0) and a
  blocking provider, a `WithRunTimeout(30ms)` ctx cancels the run with
  `context.DeadlineExceeded` within ~1s (proves the override alone bounds the run).
- `TestCmdRun_InvalidTimeout` — `--timeout notaduration` → exit 2 with an
  "invalid --timeout" message, before any daemon dial.

## Live proof (offline mock, real booted daemon)
```
$ agt run --timeout 5m "hi"        # valid: a normal run still completes
  --- final answer ---
  …
$ agt run --timeout 2x "hi"        # malformed → rejected client-side
  agt run: invalid --timeout "2x" (want a positive Go duration like 30s, 2m)
```
The actual timeout firing is rigorously covered by the runtime unit test
(`DeadlineExceeded` at ~30ms with no daemon-wide cap); the live run confirms the
flag plumbs through end-to-end without disturbing a normal run.

## Verification
- `go.mod` / `go.sum` unchanged.
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on all touched files.
- `go test ./...` — **FAIL 0**, **1484 tests** (was 1482; +2), 61 packages.

## Result
`agt run` now takes `--model`, `--system`, and `--timeout` per-run overrides (plus
intent from arg/stdin/file and a cost line) — a single command line bounds the
model, persona, and wall-clock of one run without touching daemon config.
