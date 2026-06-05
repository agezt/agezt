# M458 — Standing cron: don't launch orders after shutdown begins

## Context
`StartCron` (kernel/standing/cron.go) runs a background ticker that fires
cron-triggered standing orders each matching minute. Each fired order is dispatched
on its own goroutine (`go fire(ctx, ord, ...)`) inside `tickCron`.

## The bug
The driver loop:

```go
for {
    select {
    case <-ctx.Done():
        return
    case <-tk.C:
        tickCron(ctx, store, now(), lastFired, fire)
    }
}
```

A Go `select` with more than one ready case chooses one **at random**. When the
daemon shuts down, `ctx.Done()` and a pending `tk.C` can both be ready in the same
iteration — so the ticker case can be picked *after* cancellation has begun.
`tickCron` then spawns fresh `go fire(...)` order goroutines during teardown. Those
goroutines are not tracked or awaited, so an order's plan can start *after* the cron
loop was told to stop and after shutdown has begun closing stores — racing real
work (a brief sent post-shutdown, a run touching a store mid-close) against
teardown.

## The fix
Gate firing on `ctx.Err()` in two places (defense in depth):

1. **Top of `tickCron`** — `if ctx.Err() != nil { return nil }`. Even if the ticker
   case is selected during shutdown, no order is dispatched. This is the load-bearing
   fix and is deterministically testable (it takes `ctx` and is white-box tested).
2. **The `case <-tk.C:` branch** — a cheap re-check that returns from the loop, so a
   slow shutdown doesn't keep ticking.

No behaviour change while the context is live; orders fire exactly as before.

## Test + negative control
`kernel/standing/cron_test.go`:
`TestTickCron_DoesNotFireAfterContextCancel` — confirms a live context fires the
matching order, then asserts a **cancelled** context fires nothing (empty `fired`
slice and zero `fire` invocations), using a fresh `lastFired` and a matching minute
so only the shutdown gate can be what suppresses it. Existing
`TestTickCron_FiresOncePerMinute` / `_SkipsDisabled` still pass.

**Negative control:** disabling the gate (`ctx.Err() != nil && false`) made the test
report `cancelled ctx must fire nothing, got fired=[01KTCRF2...]` — the order was
dispatched during shutdown — `--- FAIL`. Restored; test passes.

## Provenance
Found by a scoped code review of kernel/state, kernel/scheduler, kernel/standing
for genuine concurrency/teardown bugs (this session's defect-hunt arc). The same
review flagged two scheduler items (1 ms busy-wait poll; gate nodes holding a
compute-pool slot while blocking on human approval) tracked for follow-up.

## Verification / gate
- `kernel/standing` tests pass.
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
