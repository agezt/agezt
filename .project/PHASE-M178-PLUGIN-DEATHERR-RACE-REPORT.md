# M178 — Plugin death-cause field made atomic (data-race fix)

## Why
The plugin-host security review (finding H2) flagged a genuine data race. `Plugin`
tracks two things when a child dies:

- `dead atomic.Bool` — correctly atomic; gates fail-fast in `callWithProgress` and
  `remoteTool.Invoke`.
- `deathErr error` — a **plain** field holding the cause, written by the read-loop
  goroutine in `markDead` and by `Close`/`respawn`, and read (unsynchronized) by
  callers at `callWithProgress` (`plugin: dead` / `connection lost` wrapping) and by
  `remoteTool.Invoke` (`plugin process is dead: …`).

An `atomic.Bool` orders the publication of *itself*, not of a neighbouring plain
field. Under Go's memory model a reader that observes `dead == true` is **not**
guaranteed to see a safely-published `deathErr` — the write/read pair on the plain
field is a data race. A plugin that crashes (stdout EOF) at the exact moment a caller
enters `Invoke` triggers it: `go test -race` would report it, and on weakly-ordered
architectures a reader could observe a torn/partially-published interface value.

## What
`deathErr` is now an `atomic.Pointer[error]`, with two small accessors:

```go
func (p *Plugin) deathError() error      // Load → deref, nil-safe
func (p *Plugin) setDeathErr(err error)  // Store(&err), or Store(nil) to clear
```

All seven sites converted:
- writes: `markDead` (`setDeathErr(cause)`), `Close` (`setDeathErr(errors.New("plugin: closed"))`),
  `respawn` reset (`setDeathErr(nil)`).
- reads: `callWithProgress` ×2, `remoteTool.Invoke` ×1, all via `deathError()`.

Storing a heap pointer to the interface value publishes the cause safely; readers
load the pointer atomically and deref. No behavior change — the same error strings
surface; only the access is now race-free. (This also resolves the `go vet`
copylocks warnings that a plain read would otherwise trigger now that the field
carries `atomic.noCopy`.)

## Tests
- `kernel/plugin/deatherr_test.go` (white-box) — `TestDeathErr_ConcurrentReadWrite`
  spins 8 reader goroutines calling `deathError()`/`IsAlive()` against a concurrent
  `markDead`, then asserts the recorded cause is `"boom"` and that a second `markDead`
  does NOT overwrite it (CompareAndSwap idempotence). The test is written to be
  meaningful under `go test -race`.

### Note on `-race` in this environment
The Go race detector requires CGO (a C toolchain). This Windows dev box has no `gcc`
on PATH, so `-race` can't run here; the test still passes in normal mode and is
race-detector-ready for a CI runner that has CGO. The fix's correctness rests on the
`atomic.Pointer` semantics, not on local detector output.

## Verification
- `go test ./...` — 1567 passing, 0 failing.
- `go vet ./kernel/plugin/` clean (no copylocks warnings).
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — `deathErr` → `atomic.Pointer[error]`; `deathError()` /
  `setDeathErr()` accessors; all 7 access sites converted.
- `kernel/plugin/deatherr_test.go` — new concurrency test.

## Follow-ups (same review, queued)
H3 (send-on-closed-channel in dispatch vs `Close` race), H4 (reload resets `nextID`
→ id reuse / response confusion), M1 (unbounded callback goroutine fan-out),
M3 (`Kill` nil-`Process` guard), M4 (process-group kill for orphaned grandchildren).
