# M420 — Cadence panic containment + memory/world-model supersede-resurrection

## Context
Two parallel review passes over the remaining stateful subsystems (content-addressed
stores; the scheduled-intents / DAG engines; the REST API). The REST API and the
scheduler DAG executor were found clean. Two genuine HIGH bugs surfaced.

## Fixes

### HIGH — a panicking scheduled run crashes the daemon (`kernel/cadence/cadence.go`)
`fireDue` dispatched each due entry on a bare `go func()` whose only defer was
`running.Delete` — no recover. The run closure (`cmd/agezt/main.go`) does:
`Bus.Publish` → `RunWith` (the agent loop, which recovers its OWN panics) →
`onAnswer` → `deliverScheduled` → `channelSend` → `ch.Send` into a telegram/slack/
discord plugin. The delivery runs **after** `RunWith` returned, on the fire
goroutine, with no recover — and the channel plugins have none either. A panic in
delivery (nil map deref, SDK panic on a malformed response) terminated the whole
process, killing every concurrent run and the API surfaces. This is the exact class
`kernel/standing` guards against with `safeFire` (M413); cadence had no equivalent.

Fix: extracted the goroutine body into `(*Engine).fireOne` with a `recover()`
backstop that logs and contains the panic; the in-flight guard is still cleared (so
the schedule isn't wedged). Extracting it also makes the containment synchronously
testable.

### HIGH — reinforce resurrects a superseded fact (`kernel/memory/manager.go`,
`kernel/worldmodel/manager.go`)
The reinforce branch of `Remember`/`Upsert` rebuilt the record/entity fresh
(`SupersededBy == ""`) and copied `CreatedMS`/`SourceEvent`/`Confidence`(`Weight`)/
tags — but not `SupersededBy`. So re-stating content that had been explicitly
*superseded* (which the auto-distiller does every task, re-extracting old facts)
overwrote it with an empty link, making the stale fact `Active()` again alongside its
replacement — silently, with no event recording the un-supersession. Reachable on a
single goroutine. Fix: preserve `existing.SupersededBy` across the reinforce in both
stores, so a superseded fact stays inactive (reviving a *tombstoned* fact remains
intentional).

## Verification
- **`kernel/cadence/cadence_test.go`** `TestEngine_FireOne_ContainsPanic`: a panicking
  RunFunc, run synchronously through `fireOne`, is contained and the in-flight guard
  is cleared.
  - **Negative control:** disabling `recover()` → the synchronous call panics the test
    goroutine → FAIL. Restored byte-identical.
- **`kernel/memory/manager_test.go`** `TestReinforceDoesNotResurrectSuperseded` and
  **`kernel/worldmodel/manager_test.go`** `TestUpsertDoesNotResurrectSuperseded`:
  supersede a fact, then re-state it → it stays inactive, only the successor is active.
  - **Negative controls:** dropping the `SupersededBy` preservation in each store →
    the respective test FAILs. Restored byte-identical.
- **Gate:** `gofmt -l` clean on all edited files (an unrelated CRLF artifact on the
  untouched `memory.go` in the local checkout is not part of this change — its
  committed LF form is clean), `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2272** passing (was 2269; +3). CHANGELOG
  Reliability entries added.

## Deferred review findings (documented, not fixed here)
- **MEDIUM — lost update in the memory/world-model Manager layer:** each mutator does
  `store.Get` → compute → `store.Put` as two separately-locked calls, so concurrent
  reinforces (or a reinforce racing `Decay`) can lose an update or clobber a refresh.
  The stores are documented as "not optimized for high write volume"; a coarse
  Manager-level mutex across the read-modify-write would close it. Deferred — needs a
  small design choice (Manager mutex vs a store-level `Update(id, fn)` CAS) and isn't
  a single-goroutine correctness bug.
- **MEDIUM — cadence in-flight guard never cleared if a fired run hangs forever:** the
  overlap guard clears only when `run` returns; an unbounded hang (no
  `AGEZT_MAX_DURATION` / per-run timeout) wedges that one schedule. A per-fire
  deadline on the cadence run closure would fix it. Deferred — operator-mitigable via
  the run cap and a contract gap rather than a code defect.

## Review status
The REST API (constant-time auth, body-size limits, fail-closed tenant scoping, no
pre-auth mutating route, ctx-on-disconnect) and the scheduler DAG executor (validated
cycle/dep checks, fail-closed gates, no leaks) were found clean. resolve.go has no
multi-hop traversal, so the cyclic-graph infinite-loop concern does not apply.
Content-hash determinism, snapshot determinism, decay math, and tombstone exclusion
after rebuild were all verified correct.
