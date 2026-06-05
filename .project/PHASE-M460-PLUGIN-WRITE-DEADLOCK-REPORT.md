# M460 — Plugin host: read-loop ↔ stdin-write deadlock (HIGH)

## Context
The plugin host (kernel/plugin/host.go) talks to each child plugin over stdio:
newline-framed JSON written to the child's stdin, frames read from its stdout by a
single read loop. The read loop routes a response to its waiter via `deliver`,
which takes `p.mu` to look up the `pending` map. Host→plugin writes
(`writeRequest`, `writeResponse`) also took `p.mu` — and held it **across the
blocking `p.stdin.Write`**.

## The bug (HIGH)
A plugin can send a `host/invoke` callback and then stop reading its stdin while
continuing to write stdout. The host's `handleCallback` goroutine runs the host
tool and calls `writeResponse`, which acquires `p.mu` and blocks on
`p.stdin.Write` because the child's stdin pipe buffer is full. Meanwhile the read
loop reads the next stdout frame and calls `deliver`, which blocks on `p.mu`. The
read loop is now wedged, so it stops draining the child's stdout; the child's
stdout pipe fills, the child blocks on its stdout write and never reads its stdin,
so the host's `Write` never returns. OS pipes have no write deadline, so nothing
breaks the cycle:

```
handleCallback → writeResponse → holds p.mu, blocked on stdin.Write (child stdin full)
read loop       → deliver       → blocked on p.mu
child           → blocked on stdout write (host read loop not draining) → never reads stdin
```

Impact: one misbehaving or hostile plugin permanently wedges its host slot. The
child is never `markDead`'d; the `handleCallback` goroutine and any future invoker
goroutines leak (callers escape only via their own ctx timeout, then leak the
dead-but-unmarked plugin). A single callback triggers it; the M181 callback cap
does not help. Severity HIGH.

## The fix
Serialize stdin writes on a dedicated `writeMu`, separate from `p.mu`, and never
hold `p.mu` across the write. `writeRequest`/`writeResponse` now delegate to a new
`writeFrame(raw, kind)`:

```go
func (p *Plugin) writeFrame(raw []byte, kind string) error {
    p.writeMu.Lock()
    defer p.writeMu.Unlock()
    p.mu.Lock()
    w := p.stdin   // snapshot: respawn swaps p.stdin under p.mu
    p.mu.Unlock()
    if w == nil { return fmt.Errorf("plugin: write %s: stdin closed", kind) }
    if _, err := w.Write(raw); err != nil { return fmt.Errorf("plugin: write %s: %w", kind, err) }
    return nil
}
```

Now `deliver` (and `markDead`, `Close`-drain, `callWithProgress`) take `p.mu`
freely while a write is stuck, so the read loop keeps draining stdout — which
drains the child's stdout pipe, lets the child resume reading its stdin, and
unblocks the write. Frames still never interleave (`writeMu` serializes them).
`p.stdin` is read under `p.mu` so a concurrent `respawn` swap is race-free. Lock
order is always `writeMu → p.mu` (and `p.mu` is released before the write), and no
path takes `p.mu` then `writeMu`, so no new lock-ordering hazard. Error strings are
unchanged (`plugin: write request/response: ...`).

## Test + negative control
`kernel/plugin/deliver_test.go`: `TestDeliver_NotBlockedByStuckStdinWrite` — a
`blockingStdin` whose `Write` blocks (modelling a full child stdin) is installed;
a `writeResponse` goroutine gets stuck mid-write, then the test asserts `deliver`
still completes. White-box (package plugin), deterministic via an `entered` signal.

**Negative control:** restoring `p.mu` held across the write (`defer p.mu.Unlock()`
in `writeFrame`) made `deliver` block — the test reported `deliver blocked by a
stuck stdin write — read-loop/writer deadlock` and FAILED (1 s timeout). Restored;
test passes.

## Provenance
Found by a scoped code review of kernel/controlplane + kernel/plugin for
concurrency/lifecycle bugs. The same review flagged two more items tracked as
follow-ups: a controlplane `Stop()` that can block on in-flight streaming handlers
when shutdown isn't driven by ctx cancellation (MED), and a bounded
`callWithProgress`/`Close` registration TOCTOU (LOW).

## Verification / gate
- `kernel/plugin` tests pass (incl. the existing flood/deliver/callback suites).
- gofmt-clean on staged LF blobs, `go vet` clean, `GOOS=linux` build clean, full
  `go test ./...` exit 0, `go.mod`/`go.sum` unchanged.
