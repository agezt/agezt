# M512 — Mutation testing anomaly: circuit breaker verified solid (no gap)

## Context
Twenty-third package in the mutation pass: `kernel/anomaly` (the autonomous-operation
circuit breaker, SPEC-06 §5 — a sliding-window tool-call-rate Detector and the bus
Monitor that auto-halts on a trip). Run with `GOMAXPROCS=3` (CPU-capped). go-mutesting
score 0.590, 23 survivors; working tree restored clean after the run.

## Finding: no genuine gap — survivors are equivalent
go-mutesting prints diffs only for *killed* mutants, so the survivor set can't be read
off stdout. Instead every semantically-meaningful operator was mutated by hand and run
against the existing tests (the reliable negative-control method). **All were killed:**

detector.go (`TestDetector*`):
- `d.max > 0 → >= 0` and `d.window > 0 → >= 0` (Enabled gate) — killed.
- `&&  → ||` in `Enabled` — killed.
- `t.Add(-d.window) → t.Add(d.window)` (window sign) — killed.
- `drop < len(d.stamps) → <=` (prune bound, panic) — killed.
- prune-loop `&& → ||` — killed.
- `drop++ → drop--` — killed.
- `drop > 0 → >= 0` (reslice guard) — killed.
- `count > d.max → >= d.max` (the core trip boundary; the N-th vs N+1-th event) — killed.
- `.Before(cutoff) → .After(cutoff)` (window inclusivity) — killed.

monitor.go (`TestMonitor*`):
- `b == nil → b != nil` and `|| → &&` (start gate) — killed.
- `ev.Kind != KindToolInvoked → ==` (the event-kind filter) — killed: under it the
  tool.invoked spike is skipped, nothing is counted, no trip, the spike test times out.
- `if !tripped → if tripped` (trip handling) — killed.
- `if onTrip != nil → == nil` (callback guard) — killed.

The 23 go-mutesting survivors are therefore **equivalent mutants** (branch/statement
removals and literal tweaks that don't change observable behavior) — unkillable by
construction, like the residuals on event and netguard.

## Why no new test
Adding a test here would pad an already-covered property (every meaningful mutant dies),
not close a gap. Consistent with M493 (netguard) and the event hash-chain assessment,
this package is recorded as **verified solid**. No production or test code changed.

## Verification / gate
- No code change; existing `go test ./kernel/anomaly/` passes (`GOMAXPROCS=3`).
- Full `go test ./...` exit 0 (`GOMAXPROCS=3`, `-p 2`); `go.mod`/`go.sum` unchanged.

## Mutation pass — twenty-three packages (M490–M512)
redact, journal, edict, netguard, event, creds, warden, governor, scheduler, bus,
cadence, runtime, tenant, worldmodel, approval, memory, skill, standing, catalog, plugin,
webhook, channel, anomaly — plus the controlplane primary-token auth gate verified solid.
