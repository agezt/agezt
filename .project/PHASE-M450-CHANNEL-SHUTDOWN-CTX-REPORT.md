# M450 — Tie async channel runs to the daemon ctx (clean-shutdown drain)

## Context
Resolving the deferred "slack/discord async runs use `context.Background()`" item.
At the prior checkpoint I had judged this a defensible design choice; on closer
analysis of the daemon's shutdown ordering it is a strictly-better, low-risk
change with no downside, so it is now fixed.

## The gap
The Slack and Discord inbound handlers ack the webhook immediately and run the
agent on a *detached* goroutine — correct, so the run doesn't die when the HTTP
response is sent. But they detached to `context.Background()`, which is never
cancelled. On shutdown those in-flight runs were not cancelled; after the drain
window they were simply killed by process exit (an abrupt stop mid-work).

## Why the daemon ctx is strictly better (shutdown ordering verified)
The daemon shutdown sequence (`cmd/agezt/main.go`) is: signal → `draining.Store(true)`
(readiness off) → `drainWait(k.ActiveRuns, drainTimeout)` (wait for in-flight runs
to finish) → **then** `cancel()` (daemon ctx) → `Halt()`. Because `cancel()` runs
*after* the drain:
- During the drain, the daemon ctx is still alive, so a channel run (which is a
  kernel run counted in `ActiveRuns`) continues to completion exactly as before.
- Only a straggler still running past `drainTimeout` is then cancelled — and with
  the daemon ctx it gets a clean `ctx.Done()` (returns `ctx.Err`, journals a clean
  cancel) instead of being killed by process exit.

So normal operation is unchanged (the daemon ctx only cancels at shutdown, after
the drain); the change strictly improves the straggler path.

## The fix
Added a `baseCtx context.Context` field to each channel, defaulted to
`context.Background()` in `New` (so a handler driven directly in tests still
works) and set to the daemon ctx in `Start`. The async run spawn now uses
`c.baseCtx` instead of `context.Background()`. Storing a server-lifetime context
on the channel is the standard pattern for a handler that needs it; `go vet` is
clean.

## Verification
- **`plugins/channels/{slack,discord}/basectx_test.go`**
  `TestStart_TiesAsyncRunsToDaemonCtx`: `New` defaults `baseCtx` non-nil; `Start`
  with a cancellable ctx, then cancel — after `Start` returns, `baseCtx.Err() != nil`
  (the async run would observe the cancellation). Race-free (read after the Start
  goroutine signals done).
  - **Negative control:** remove `c.baseCtx = ctx` from `Start` → `baseCtx` stays
    the `New` default (`Background`, never cancelled) and the test FAILs. Restored.
- **Gate:** gofmt-clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged, full suite exit 0. CHANGELOG Reliability entry.

## Review status
This resolves the last reconsiderable design-choice deferral. The only remaining
documented item is the anthropic strict-stream-abort (a defensible strict-vs-lenient
choice returning a clean error) and external-wire work that cannot be implemented
or verified offline.
