# M168 — Agent loop panic firewall (code review)

## Why
Continuing the code-quality mandate, an independent review of the agent tool-loop
(`kernel/agent/agent.go` — the hottest path in the system, and the one M166 had
just modified to add cost accounting) was commissioned. It confirmed the M166 cost
code, the once-only `task.failed` emitter, the per-tool-timeout context handling,
and streaming/non-streaming response consistency are all correct — and found one
real, high-severity bug.

## The bug (High — daemon-wide blast radius)
The loop assigns `resp = r` from `Provider.Complete` / `CompleteStream` and then
**unconditionally dereferences** `resp.StopReason` / `resp.Message` / `resp.Usage`.
There was no nil check. The `Provider` contract guarantees a non-nil response with
a nil error, and every first-party provider honors it — but the package explicitly
supports **out-of-process plugins** ("satisfy the same interface via a thin
client"), i.e. third-party code that can break the contract, e.g. returning
`(nil, nil)` on an unexpected empty upstream body.

`agent.Run` executes in a **bare goroutine with no `recover()`** (the control plane,
the scheduler, and delegation all drive it this way). So a nil-`resp` deref — or any
other panic from a provider or tool plugin — was not a single failed run: it
`panic`ked → process exit → **every concurrent run on the daemon died**, and the
offending run never even journaled `task.failed`.

## Fix
Two layers in `kernel/agent/agent.go`:
1. **Nil-response guard** — after both the streaming and non-streaming calls:
   `if resp == nil { return "", fmt.Errorf("agent: provider %s returned a nil
   response without an error", …) }`. A contract violation becomes a clean,
   journaled run failure (`reason=error`) instead of a nil deref.
2. **Panic firewall** — a deferred `recover()` wraps the whole run; a recovered
   panic becomes `ErrPanic` (wrapping the original panic value). It's registered
   **after** the `task.failed` defer so, by LIFO, it runs **first** and sets the
   named `runErr` before the `task.failed` defer reads it — so a panicked run is
   journaled as `task.failed(reason=panic)` with the panic message preserved, and
   `Run` returns an error instead of unwinding into the daemon goroutine. Registered
   after `task.received` so a pre-run validation panic isn't mis-counted as a run.
   `failureReason` gains a `panic` tag (checked first).

The firewall lives in `Run` itself, so it protects **every** caller (control plane,
scheduler, sub-agent delegation), not just one goroutine. The panic message is
captured in the journaled `task.failed.error`, so the failure stays fully
debuggable — it's contained, not hidden.

## Reviewed-and-confirmed-correct (left unchanged)
- M166 cost cap: local accumulator, post-call `>=` threshold (inclusive, intended),
  `resp.Usage.Model`→`cfg.Model` attribution, no overflow under `MaxIter`, no race
  (local stack var). Streaming path **does** carry Usage, so the cap isn't bypassed
  by streaming.
- The deferred `task.failed` fires exactly once, never on success, never masks
  `runErr`; `runErr` is set on every error path (no naked returns).
- `ctx.Err()` checks (top of loop + before each tool call); per-tool timeout context
  always cancelled, run-cancel vs tool-timeout correctly distinguished.
- `callCounts` / `messages` are goroutine-local and `MaxIter`-bounded — no leak, no
  shared-map race.

(Two lower-severity observations from the review — the cost cap being inert for
*unpriced* models, and per-provider cost-cap test coverage — are noted for a
follow-up; M167 already surfaces the unpriced case in `--dry-run`.)

## Tests (+2, all passing)
`kernel/agent/panic_test.go`:
- `TestRun_NilResponse_FailsGracefully` — a provider returning `(nil, nil)` →
  `Run` returns a "nil response" error and journals exactly one
  `task.failed(reason=error)`, no `task.completed`, **no panic**.
- `TestRun_ProviderPanic_RecoveredAsFailure` — a provider that `panic`s →
  `Run` returns `ErrPanic` carrying the original panic text and journals
  `task.failed(reason=panic)`. The test process surviving the panic IS the proof
  that the daemon goroutine no longer crashes.

## Verification
- `go.mod` / `go.sum` unchanged; no new protocol command or event kind (reuses
  `task.failed` with a new reason tag).
- `go vet ./...` clean; `GOOS=linux go build ./...` ok.
- `gofmt -l` (LF-normalized) clean on both touched files.
- `go test ./... -count=1` — **FAIL 0**, **1542 tests** (was 1540; +2), 61 packages.

## Result
A single misbehaving provider or tool plugin — nil response or outright panic — now
fails its own run cleanly and is journaled, instead of taking down the daemon and
every other run in flight. The system's most critical path is now panic-safe for
all callers.
