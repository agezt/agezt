# M525 — Mutation testing pulse: pin the DiskObserver threshold edges

## Context
Continuing `kernel/pulse` (after salience M523/M524), into `observers.go` — the
`DiskObserver` free-space alert. Run with `GOMAXPROCS=3` (CPU-capped).

## The genuine gap (closed)
`DiskObserver.Poll` has two inclusive thresholds:

```
low := freePct < o.minPct          // alert when free% drops below the floor
…
if freePct < o.minPct/2 { sev = SevCritical }   // escalate at half the floor
```

`TestDiskObserverTransitions` crosses at 5% against a 10% floor and
`TestDiskObserverCriticalWhenVeryLow` uses 2% against a 5% critical line — both well clear
of the edge. So neither `<` was pinned at its boundary, and both survived `→ <=`
(confirmed by hand-applied negative control):
- `freePct < minPct → <=`: free space sitting *exactly* on the floor (10% free, floor 10)
  would be flagged low and fire a spurious `disk_low` — a false alarm at the boundary.
- `freePct < minPct/2 → <=`: free space *exactly* at the half-floor (5%, floor 10) would
  escalate to Critical instead of High — over-paging the operator at the edge.

## Fix
Added `TestDiskObserver_ThresholdEdges` (two sub-tests): free% exactly at the floor causes
no transition from the baseline (not low); a transition to exactly `minPct/2` emits
`disk_low` at severity **High**, not Critical.

## Negative control (manual, CPU-capped)
`freePct < minPct → <=` and `freePct < minPct/2 → <=` each FAIL under the new test.
Restored byte-for-byte (`git diff --ignore-all-space` on observers.go empty); passes again.

## Verification / gate
- Test passes; `go vet` + `staticcheck` clean; gofmt-clean on the staged LF blob.
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — thirty-three packages (M490–M525)
pulse counted once; its salience bands (M523), novelty TTL (M524), and now the disk
observer thresholds (M525) are pinned at every inclusive edge. The ProbeObserver
green↔red transition and `ParseProbeSpec` are already covered by the existing tests.
