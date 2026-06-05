# M464 — Plugin host: close the call-registration TOCTOU vs teardown

## Context
`callWithProgress` (kernel/plugin/host.go) issues a request to a plugin: it checks
the plugin is alive, registers a response channel in `pending` under `p.mu`, writes
the request, and waits on the channel. `Close`/`markDead` set `p.dead` and then
drain `pending` (close + delete every channel) under `p.mu`.

## The bug (LOW)
The liveness check was lock-free, separate from the under-lock registration:

```go
if p.dead.Load() {                       // lock-free check
    return nil, ...dead...
}
id := ...
ch := make(chan *Response, 1)
p.mu.Lock()
p.pending[id] = ch                       // registration
...
p.mu.Unlock()
```

If `Close`/`markDead` runs between the check and the registration — marking dead and
draining `pending` — the caller then inserts `ch` into `pending` **after** the
drain. Nothing will ever close that channel, so the caller blocks on its `select`
until its own `ctx` deadline (the invoke timeout) instead of failing fast. Bounded
(not a permanent hang) and the `defer` still cleans up the stale `pending`/`progress`
entries, so severity is LOW — but it is a real avoidable stall, and a stale entry
briefly lingers.

## The fix
Re-check `p.dead` **inside** the `p.mu` critical section, before registering:

```go
p.mu.Lock()
if p.dead.Load() {
    p.mu.Unlock()
    return nil, fmt.Errorf("plugin: dead: %w", p.deathError())
}
p.pending[id] = ch
...
```

Because `markDead`/`Close` set `dead` and drain under the same lock, the re-check
makes registration and teardown mutually exclusive: either the caller registers
*before* the drain (and the drain then closes its channel → the `select` returns
"connection lost" immediately), or it observes `dead` and bails. There is no longer
a window where a channel is registered after the drain and never closed.

## Test + negative control
`kernel/plugin/deliver_test.go`:
`TestCallWithProgress_DeadDuringRegistrationFailsFast` — uses `p.mu` itself as the
seam: the test holds `p.mu`, launches a caller (which passes the lock-free check
then parks on `p.mu`), then simulates teardown (set `dead` + drain `pending`) and
releases the lock. The caller must return a death error within 2 s (its own ctx is
10 s). Deterministic because the lock-free check precedes the `p.mu.Lock`, so
holding the lock parks the caller exactly in the vulnerable window.

**Negative control:** disabling the under-lock re-check (`p.dead.Load() && false`)
made the caller register an orphan channel and block — the test reported
`callWithProgress blocked after a concurrent close ... (TOCTOU)` and FAILED (2 s
timeout). Restored; test passes.

## Provenance
Third and final item from the scoped controlplane/plugin code review (after M460
HIGH deadlock and M461 MED Stop hang). The review's other findings are resolved or
documented; this closes its plugin-host list.

## Verification / gate
- `kernel/plugin` tests pass (incl. the deliver/callback/flood suites).
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
