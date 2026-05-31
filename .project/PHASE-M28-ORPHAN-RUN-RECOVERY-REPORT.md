# Phase Report — Milestone M28 (Orphaned-run recovery on boot)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/recovery). First step on a fresh axis after the M14–M27
> policy + capability arcs: the daemon now reconciles runs that were in-flight
> when a prior instance exited, instead of leaving them "running" forever.

## Why

A run's lifecycle is journaled as `task.received` … `task.completed`. But
`task.completed` is published **only on the success path** (`kernel/agent/agent.go`
— it fires when the loop reaches a final answer). A run that crashed mid-flight,
was cancelled by a graceful shutdown, or errored out emits a `task.received` and
no completion. `agt runs` pairs the two and, finding no completion, reports the
run as **`running`** — forever. An operator looking at `agt runs` after a crash
sees phantom live runs that no daemon is actually executing.

The fix is squarely in the event-sourced model: the journal already records
exactly which runs never completed; the daemon just needs to reconcile them at
boot, the same way it replays durable policy (M20).

## What shipped

- **`event.KindTaskAbandoned`** (`task.abandoned`) — marks a run that was received
  but never completed in a prior session. Registered in the validation map.
- **`runScan` + `reconcileOrphanRuns` (`cmd/agezt/main.go`)** — a pure scanner
  (`newRunScan`/`observe`/`orphans`) folds the journal's `task.*` events and
  returns the orphans: correlations with a `task.received` but neither a
  `task.completed` (never finished) nor a `task.abandoned` (the **idempotency
  guard** — already reconciled on an earlier boot). `reconcileOrphanRuns` runs the
  scan over `k.Journal().Range`, publishes one `task.abandoned` per orphan
  (carrying the intent, a reason, and the original start time), and returns the
  count. Wired into boot **before any run is dispatched** (so the scan can't see
  a live run), with a banner line: `recovery : N run(s) abandoned …` or `clean`.
- **`agt runs` status (`kernel/controlplane/runs.go`)** — the run pairing now
  tracks `task.abandoned` and renders `status="abandoned"` (completed always wins
  if both somehow appear).

No `go.mod` change. No new control-plane command (reuses `CmdRunsList`). The pure
scanner is unit-tested without a kernel; the rendering via the control-plane
harness.

## Proven

- **Unit (`cmd/agezt`):** `runScan` over a synthetic history — received+completed
  → not orphaned; received-only → orphaned; received+abandoned → not orphaned
  (idempotent); orphans sorted by start time; empty history → none.
- **Unit (`kernel/controlplane`):** a journal with `task.received` +
  `task.abandoned` renders `status="abandoned"` with the right intent; a run with
  received + abandoned + completed renders `completed` (completed beats
  abandoned).
- **Live (three boots, hanging provider):**
  1. Boot #1 banner `recovery : clean`; an `agt run` against a black-holed
     provider URL hangs on the dial, journaling `task.received` with no
     completion; graceful shutdown cancels it (still no completion). Journal:
     1 received, 0 completed — a genuine orphan.
  2. Boot #2 banner `recovery : 1 run(s) abandoned on restart …`; the journal
     gains one `task.abandoned` (correct correlation id + intent); `agt runs list`
     shows `status: abandoned`.
  3. Boot #3 banner `recovery : clean` — **idempotent**, the already-abandoned run
     is not re-abandoned (still exactly one `task.abandoned` event).

5 new tests; suite **1202** green, `go vet` clean, `GOOS=linux` builds,
`go.mod` unchanged.

## Deferred — named

- **Per-run wall-clock timeout** — a run can still hang indefinitely *within a
  live session* (a slow provider or a blocking tool); M28 only reconciles across
  restarts. An optional `MaxDuration` wrapping the agent loop is the natural next
  reliability step (touches the agent hot path — wants careful testing).
- **Run statistics** (`agt runs stats`) — aggregate success rate / durations /
  error counts over the journal; a pure additive observability layer on the same
  pairing logic.
- **Distinguishing crash vs. cancel vs. error** — `task.abandoned` is one honest
  label for "received, never completed"; a future `task.failed` emitted on the
  error path would let `agt runs` tell a failure apart from a true orphan.
