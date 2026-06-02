# M181 — Bounded plugin host-callback fan-out

## Why
The plugin-host security review (finding M1) flagged a DoS amplifier. When a plugin
calls back into the host (`host/invoke`, M1.cb), the read loop dispatched it like:

```go
if f.Method != "" {
    go p.handleCallback(f)   // <-- one new goroutine per callback, no limit
    continue
}
```

The plugin's stdout is untrusted. A hostile plugin can stream `host/invoke` frames as
fast as the host reads them, and each spawns a goroutine that runs a curated host tool
with up to `InvokeTimeout` (default 2 minutes). That is an unbounded number of
concurrent goroutines and host-tool executions per plugin — goroutine/memory/FD
exhaustion, plus amplification of whatever those host tools reach (file reads, http
gets, shell). The existing per-call timeout caps each call's duration but not the
*count*, so it doesn't bound the flood.

## What
A per-plugin counting semaphore caps simultaneous callbacks.

- **`Config.MaxConcurrentCallbacks`** (default `DefaultMaxConcurrentCallbacks = 16`) —
  the cap, defaulted in `Spawn`.
- **`Plugin.cbSem chan struct{}`** — a buffered channel used as the semaphore, created
  once in `Spawn` and persisting across `Reload` so it bounds the plugin's whole life.
- **`dispatchCallback(f)`** — the read loop now calls this. It does a NON-blocking
  acquire (`select { case cbSem <- {}: go handleCallback(f); default: reject }`):
  - slot available → spawn `handleCallback`, which releases the slot in a `defer`
    (registered first, so it runs after the response is written).
  - semaphore full → `rejectCallback(f, ErrTooManyCallbacks)` writes the error inline
    on the read-loop goroutine (no goroutine spawned, no slot held) so the plugin sees
    a clear error on its callback id, exactly as if a host tool had failed.

Non-blocking (reject) rather than blocking (queue) is the deliberate choice for an
untrusted boundary: it keeps the read loop responsive (so the plugin's normal
responses are still processed under a flood) and makes goroutine count hard-bounded at
the cap. Well-behaved plugins issuing a handful of concurrent callbacks are unaffected.

## Tests
`kernel/plugin/callbacklimit_test.go` (white-box):
- `TestDispatchCallback_RejectsOverCap` — with the semaphore full, a dispatched
  callback is rejected inline with `ErrTooManyCallbacks` for its id and consumes/leaks
  no slot.
- `TestDispatchCallback_AcceptsUnderCap` — with a free slot, the callback is accepted
  (slot acquired then released by `handleCallback`, verified by polling the semaphore
  back to empty) and is NOT rejected; with `HostTools` nil it surfaces the
  callbacks-disabled error, proving it took the accept path.
- Existing `TestCallback_HappyPath` / `_DisabledWhenHostToolsNil` / `_ToolNotInAllowlist`
  still pass — normal callbacks flow through the semaphore unchanged.

## Verification
- `go test ./...` — 1572 passing, 0 failing.
- `go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — `DefaultMaxConcurrentCallbacks`, `ErrTooManyCallbacks`,
  `Config.MaxConcurrentCallbacks`, `Plugin.cbSem` (+ init in `Spawn`),
  `dispatchCallback`, `rejectCallback`, slot release in `handleCallback`.
- `kernel/plugin/callbacklimit_test.go` — new.

## Follow-ups (same review, remaining)
M2 (cap advertised tool count in the initialize result), M3 (`Kill` nil-`Process`
guard), M4 (process-group kill for orphaned grandchildren). C1/H2/H3/H4/M1 are now
fixed (M177–M181).
