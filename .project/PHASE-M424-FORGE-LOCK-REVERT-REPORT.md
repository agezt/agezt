# M424 — Forge lifecycle: concurrency lock + revert state-machine gate

## Context
Review of the Forge self-improvement lifecycle (skills as a journaled state machine)
and the catalog. The state-machine table, auto-promote/quarantine threshold math,
markdown parsing, and `retrieve.go` (no draft/shadow/quarantined leak into active
context) were found clean. Two HIGH bugs surfaced in `forge.go` (catalog finding
handled separately).

## Fixes

### HIGH 1 — no lock: lost updates + quarantine resurrection
Every mutator did `store.Get` → mutate → `store.Put` as two separately-locked store
calls. The kernel serves control-plane connections concurrently, and every run calls
`Activate` then `RecordOutcome` on the single shared `Forge`, so these Get→Put pairs
interleave. Consequences:
- Lost metric updates → corrupted auto-quarantine/promote inputs.
- **Quarantine resurrection:** `Activate` wrote back the *entire* skill snapshot it
  captured from `Retrieve` (not just a metrics delta). If run A captured a skill while
  `active` and run B (or an auto-quarantine) flipped it to `quarantined` in between,
  A's `Put` clobbered the status back to `active` — silently returning a pulled skill
  to the retrieval pool.

Fix: a `sync.Mutex` on `Forge`, held across the Get→Put in every exported mutator
(`Create`, `Promote`, `Quarantine`, `Revert`, `Activate`, `RecordOutcome`,
`RecordShadowOutcome`), mirroring `kernel/memory.Manager` (M421). Lock discipline:
the unexported helpers (`promoteWithReason`, `maybeAuto*`) assume the lock is held;
`Quarantine` was split into a locked public method + `quarantineLocked` used by
`maybeAutoQuarantine` (already under the lock via `RecordOutcome`) to avoid re-entrant
deadlock. `ShadowEvaluate` deliberately stays lock-free — it holds no lock during the
LLM `provider.Complete` call and locks only per `RecordShadowOutcome`.

### HIGH 2 — revert bypasses the state machine
`Revert` walked the lineage and unconditionally set the first non-archived parent to
`active`, with no `CanTransition` check. Lineage parents can be `draft`, `shadow`, or
`quarantined`. So reverting could resurrect a quarantined parent, or push a `draft`
straight to `active` — skipping the shadow gate the state machine enforces everywhere
else. Fix: a parent is restored only when it is already active or `CanTransition(parent
.Status, active)` is true (shadow/quarantined → active); a draft (or otherwise
ineligible) parent is skipped, and the next-older lineage parent is tried.

## Verification
- **`kernel/skill/forge_m424_test.go`**:
  - `TestRevertDoesNotActivateDraftParent`: reverting a child whose only lineage parent
    is a draft restores nothing and leaves the draft a draft.
    - **Negative control:** removing the `CanTransition` gate → the draft is force-
      activated → FAIL. Restored.
  - `TestForge_SerializesConcurrentOutcomes`: an instrumented store detects overlapping
    Get→Put windows; 8 concurrent `RecordOutcome` calls stay serialized (max overlap 1).
    (The probe is reset after `Create`, whose standalone read-after-write would
    otherwise skew the baseline.)
    - **Negative control:** removing the `RecordOutcome` lock → maxConcurrent 8 → FAIL.
      Restored.
- The full skill suite (auto-shadow/quarantine/promote, transition matrix) still
  passes — confirming the lock discipline introduced no deadlock.
- **Gate:** `gofmt -l` clean, `go vet` clean, `GOOS=linux go build ./...` ok,
  `go.mod`/`go.sum` unchanged. Full suite **2280** passing (was 2278; +2). CHANGELOG
  Reliability entry.

## Next
The catalog finding (a valid-but-empty `null`/`{}` sync payload wiping the good
catalog → self-inflicted Governor outage) is fixed separately in M425. retrieve.go,
skillmd.go parsing, and the threshold arithmetic were found clean.
