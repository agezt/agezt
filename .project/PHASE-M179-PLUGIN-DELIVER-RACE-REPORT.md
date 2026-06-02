# M179 — Race-safe plugin response delivery (no send-on-closed-channel crash)

## Why
The plugin-host security review (finding H3) identified a daemon-crash vector. The
read loop routed each terminal response like this:

```go
p.mu.Lock()
ch, ok := p.pending[f.ID]
p.mu.Unlock()       // <-- lock released
if !ok { continue }
ch <- &Response{...} // <-- send happens OUTSIDE the lock
```

Meanwhile `markDead` (read-loop terminal path) and `Close` close every pending
channel **under** `p.mu`:

```go
p.mu.Lock()
for id, ch := range p.pending { close(ch); delete(p.pending, id) }
p.mu.Unlock()
```

If a teardown runs in the window between the read loop's `Unlock()` and its `ch <- …`,
the channel is closed before the send → **send on closed channel panics**. The read
loop has no `recover`, so the goroutine dies and the daemon goes with it. A hostile
plugin widens the window deliberately: flood responses for in-flight ids while an
operator triggers `Reload`/`Close`.

## What
Two changes, both small.

1. **Locked, non-blocking delivery.** Extracted `Plugin.deliver(f inboundFrame)` that
   does the lookup AND the send under a single `p.mu` critical section, with a
   non-blocking `select { case ch <- resp: default: }`:
   - Because teardown closes+deletes channels under the same lock, `deliver` and
     teardown are now **mutually exclusive**. `deliver` either runs first (sends into
     the buffer, teardown later closes a channel that already holds the buffered item —
     legal) or after (the id is gone from `pending`, `ok == false`, nothing sent). The
     closed-but-still-in-map state that caused the panic can no longer be observed.
   - The pending channel is buffered (cap 1) and single-use, so the one legitimate
     response always fits the non-blocking send. A malicious **duplicate** terminal
     frame for the same id now hits `default` and is dropped, instead of blocking the
     read loop while it holds `mu` (a secondary DoS the old unlocked send also had).

2. **Defensive `recover` in `readLoop`.** The read loop processes untrusted plugin
   output; any unforeseen panic there now marks the plugin dead (callers fail fast)
   rather than crashing the whole daemon. Belt-and-suspenders on top of fix (1).

The `pending` field doc comment (which previously claimed sends happen *outside* the
lock "to avoid head-of-line blocking") was corrected — with a buffered single-use
channel and a non-blocking send, holding the lock across the send cannot block.

## Tests
`kernel/plugin/deliver_test.go` (white-box):
- `TestDeliver_DropsDuplicateWithoutBlocking` — a normal frame reaches its waiter; a
  duplicate terminal frame for the same id (buffer already full) returns without
  blocking (guarded by a 2s watchdog); an unknown id is a silent no-op.
- `TestDeliver_RaceWithMarkDeadNoPanic` — 500 iterations racing `deliver` against
  `markDead` on the same channel. Reaching the end without a panic is the assertion;
  before the fix this intermittently panicked with send-on-closed-channel.

## Verification
- `go test ./...` — 1569 passing, 0 failing.
- `go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — `deliver` method (locked non-blocking send), `readLoop`
  uses it + gains a `recover`; `pending` doc comment corrected.
- `kernel/plugin/deliver_test.go` — new.

## Follow-ups (same review, queued)
H4 (reload resets `nextID` → id reuse / response confusion), M1 (unbounded callback
goroutine fan-out — per-plugin semaphore), M2 (cap advertised tool count),
M3 (`Kill` nil-`Process` guard), M4 (process-group kill for orphaned grandchildren).
