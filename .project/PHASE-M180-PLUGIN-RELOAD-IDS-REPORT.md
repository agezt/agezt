# M180 — Plugin correlation ids stay monotonic across reload

## Why
The plugin-host security review (finding H4) flagged a response-confusion vector in
`Reload`. Each request the host sends a plugin is tagged with a monotonic correlation
id, minted in `callWithProgress`:

```go
id := "q-" + strconv.FormatInt(p.nextID.Add(1), 10)
```

But `respawn` (the in-place process swap behind `Reload`) reset the counter:

```go
p.nextID.Store(0)   // <-- reset on every reload
```

So after a reload the host started minting `q-1`, `q-2`, … again — the *same* ids it
had already used before the reload. Because ids are the only thing tying a response
back to its waiting request, a late or crafted response carrying a reused id can be
routed to the *new* request that happens to reuse that id, satisfying it with stale or
attacker-controlled data. Reusing correlation ids defeats the whole point of a
monotonic counter.

The same function's doc comment also misrepresented the locking: it claimed "Reload
holds `p.mu` for the duration," which is false — `Reload` takes no lock, and it
*can't* hold `p.mu` across `respawn` because respawn's own `initialize` round-trip
acquires `p.mu`.

## What
- **Removed the reset.** `respawn` no longer touches `p.nextID`; the counter keeps
  climbing for the plugin's whole lifetime, across any number of reloads, so an id is
  never reused. `nextID` is an `atomic.Int64` that starts at 0 on a fresh `Spawn`
  (zero value) and is simply preserved across the in-place swap. One-line fix, with a
  comment explaining why the reset is deliberately absent.
- **Corrected the `Reload` doc comment** to describe the actual concurrency model:
  `Reload` does not hold `p.mu` for its duration; it relies on `Close` marking the old
  child dead (so a racing caller either sees `dead==true` and fails fast, or sees the
  live new child) before `respawn` installs fresh state. With the monotonic counter, a
  late response from the old child can never satisfy a new request.

No behavior change for honest reloads beyond ids no longer restarting at 1.

## Tests
`kernel/plugin/reloadid_test.go` + `testdata/idechoplugin/` (live integration): the
idecho fixture returns the host-assigned request id as its Output. The test invokes
twice (observing ids climb), `Reload`s, then invokes again and asserts the post-reload
id **exceeds every pre-reload id**. With the old `nextID.Store(0)` reset the
post-reload id would collide with an early pre-reload value (e.g. `q-2`) and fail the
assertion; with the fix it climbs past it. Proven against a real subprocess in ~0.5s.
The shared echo fixture is untouched (no count/allowlist assertions affected).

## Verification
- `go test ./...` — 1570 passing, 0 failing.
- `go vet ./kernel/plugin/` clean.
- `gofmt -l` clean on touched files (CRLF-normalized).
- `GOOS=linux go build ./...` clean.
- `go.mod` / `go.sum` unchanged.
- Local commit only (no push); standard trailer.

## Files
- `kernel/plugin/host.go` — removed `p.nextID.Store(0)` in `respawn`; corrected the
  `Reload` concurrency doc comment.
- `kernel/plugin/reloadid_test.go` — new live test.
- `kernel/plugin/testdata/idechoplugin/main.go` — new fixture.

## Follow-ups (same review, queued)
M1 (unbounded callback goroutine fan-out — per-plugin semaphore), M2 (cap advertised
tool count), M3 (`Kill` nil-`Process` guard), M4 (process-group kill for orphaned
grandchildren). With C1/H2/H3/H4 fixed (M177–M180), the remaining items are MEDIUM DoS
hardening and orphan cleanup.
