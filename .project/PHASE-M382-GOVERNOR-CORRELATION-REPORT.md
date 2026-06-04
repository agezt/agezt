# M382 — Thread run correlation onto all Governor decision events (priority-A observability)

## Audit (read-vs-code)
The M381 follow-up flagged that `capability.rerouted` / `capability.rejected`
lacked a correlation id — the same orphaning class fixed for the warden in M379.
Reading every `g.publish(event.Spec{...})` site in `kernel/governor/governor.go`
showed the gap was broader: the Governor's per-call decision events split into two
camps.

**Carried `CorrelationID` (correct):** `budget.consumed`, `budget.unpriced`,
`capability.degraded` (M381).

**Orphaned (no `CorrelationID`) — verified gap:** `routing.decision`,
`provider.fallback`, `rate.limited`, `budget.exceeded` (×2 sites: daily + per-task),
`capability.rerouted`, `capability.rejected`. All are emitted inside
`Complete(ctx, req)` where `req.CorrelationID` (set by the agent loop) is in scope,
so an operator inspecting a run could not see *why a model was rerouted/rejected,
why a fallback fired, or why a call was rate/budget blocked* via the run timeline
or `agt why` — the events existed but floated free of the run.

## What
- **`kernel/governor/governor.go`** — added `CorrelationID: req.CorrelationID` to
  all six orphaned event specs (7 publish sites). Purely additive metadata:
  out-of-run calls (empty `req.CorrelationID`) behave exactly as before. No
  behaviour change, only linkage.

## Verification
- **`kernel/governor/governor_correlation_test.go`**
  `TestGovernorEvents_CarryRunCorrelation` — six subtests, each drives one event
  kind through the real `Complete` path (capturing bus + real journal) with a
  known correlation and asserts the journaled event carries it:
  `routing.decision` (every call), `provider.fallback` (failing → fallback chain),
  `rate.limited` (1/min gate, 2nd call), `budget.exceeded` (low ceiling, 2nd call),
  `capability.rerouted` (down-route), `capability.rejected` (strict gate).
- **Negative control:** removing `CorrelationID` from the `routing.decision` spec
  → the routing.decision subtest FAILs (`CorrelationID = ""`, want `run-GOVCORR-1`);
  restored `governor.go` byte-identical.
- **Live demo** (mock provider, `agt run`): `routing.decision` now shares the
  run's `run-01KTA22N…` correlation with `task.received` (was empty), and
  `agt why <routing.decision id>` resolves the full 6-event run timeline (the
  event sits at seq=2) — previously it resolved nothing.
- `gofmt`/`go vet`/`GOOS=linux` clean; `go.mod`/`go.sum` unchanged. Full suite
  **2162** passing (was 2155; +7), 0 failures. CHANGELOG (Fixed, operator-visible).

## Scope notes
- This closes the M381-recorded follow-up (a) and generalises it to every
  orphaned Governor event in one coherent pass.
- Follow-up still open (recorded): rendering `capability.degraded` (and the other
  governor events) in the web UI run-detail card, parallel to isolation (M379) /
  policy (M380).
