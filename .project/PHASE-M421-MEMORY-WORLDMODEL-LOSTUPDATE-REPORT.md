# M421 — Memory / world-model lost-update under concurrency (MEDIUM)

## Context
The first of the two MEDIUM findings deferred from M420. The memory-lite store
(`kernel/memory`) and the world-model graph (`kernel/worldmodel`) split into a pure
file-backed `Store` (each call individually locked) and a `Manager`/`Graph` layer
that does the read-modify-write. The RMW (`Get` → compute → `Put`) was two separately
-locked store calls, with no lock spanning the pair.

## The bug
Two concurrent writers on the same key interleave as read-read-write-write and lose
one update:
- The agent loop and the auto-distiller both `Remember` the same fact → one
  reinforcement is dropped.
- `Decay` reads entity E (weight 0.8) while `Upsert` concurrently reinforces E to 0.9
  and writes; `Decay` then writes `0.8 × factor`, clobbering the reinforcement and the
  refreshed `LastSeenMS` — a just-referenced entity is recorded as decayed/stale.

This is a *logical* lost-update, not a memory race (the `Store` is already
mutex-guarded), so `-race` would not flag it.

## The fix
A `sync.Mutex` on `Manager` (memory) and `Graph` (world-model), held across each
mutator's Get→Put:
- memory: `Remember`, `Forget`, `Supersede` (the latter locks only its own
  old-record section, since `Remember` locks internally — no re-entrancy).
- world-model: `Upsert`, `Forget`, `Relate` (locks only the relation Get→Put, after
  `resolveOrCreate` — which may `Upsert` and self-lock), and `Decay` (whole pass —
  read-all-then-write-each maintenance on a bounded graph; the coarse hold is
  acceptable per the store's documented write-volume contract).

Care was taken to avoid re-entrant deadlock: `Supersede`→`Remember` and
`Relate`→`resolveOrCreate`→`Upsert` never hold the lock when calling the
self-locking method.

## Verification
- **`kernel/memory/manager_test.go`** `TestManager_SerializesConcurrentWrites` and
  **`kernel/worldmodel/manager_test.go`** `TestGraph_SerializesConcurrentUpserts`:
  an instrumented store tracks the max number of overlapping Get→Put windows; 8
  goroutines reinforce the same key concurrently; with the lock the max stays 1.
  (Deterministic — a small sleep widens the RMW window so an unserialised writer
  reliably overlaps. This tests the logical race directly, which `-race` cannot.)
  - **Negative controls:** removing the lock from `Remember` / `Upsert` →
    `maxConcurrent == 8` → both tests FAIL. Restored byte-identical.
- The full existing world-model suite (which exercises `Relate`→`resolveOrCreate`→
  `Upsert`) passes — confirming no re-entrant deadlock was introduced.
- **Gate:** `gofmt -l` clean on all edited files, `go vet` clean, `GOOS=linux go build
  ./...` ok, `go.mod`/`go.sum` unchanged. Full suite **2274** passing (was 2272; +2).
  CHANGELOG Reliability entry added.

## Remaining deferred item
The other M420 MEDIUM — the cadence in-flight guard never clearing if a fired run
hangs forever — is still open (it needs a per-fire timeout / a documented run-cap
requirement, an operator-mitigable contract gap rather than a code defect).
