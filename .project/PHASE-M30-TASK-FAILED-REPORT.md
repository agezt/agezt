# Phase Report — Milestone M30 (`task.failed` terminal event)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/resilience). Third step on the resilience/observability
> axis: M28 reconciles orphans at boot, M29 aggregates run health, and M30 gives
> an errored run a real terminal event so the two can tell failure from orphan.

## Why

A run's lifecycle was journaled as `task.received` → `task.completed`, with
`task.completed` emitted **only on the success path**. A run that errored out —
a provider failure, an exhausted iteration budget, a cancelled context — emitted
a `task.received` and then nothing. To `agt runs` that is indistinguishable from
a true orphan (a crash mid-run): both show `running` until M28's boot
reconciliation abandons them. An operator couldn't tell "this run failed at
19:04" from "the daemon died holding this run", and M29's `success_rate` had no
failure term at all.

The fix completes the terminal-event model: every run now ends in exactly one of
`task.completed` (success), `task.failed` (errored live), or — only as a boot
safety net for runs that emitted neither — `task.abandoned`.

## What shipped

- **`event.KindTaskFailed`** (`task.failed`) — registered in the validation map.
  Payload `{error, reason}`, `reason ∈ {error, max_iters, canceled, timeout}`.
- **Agent-loop emission (`kernel/agent/agent.go`)** — `Run` now uses a named
  return (`runErr`) and registers a deferred terminal emitter **immediately after
  `task.received` succeeds** (so the pre-task validation errors, where no run
  started, don't emit it). On any error return the defer publishes one
  `task.failed`, best-effort (a failed publish must not mask `runErr`). A clean
  completion returns `runErr==nil` and the defer no-ops — the run is already
  terminal via `task.completed`. New `failureReason(ctx, err)` classifies the
  error with `errors.Is` (so a wrapped provider error still resolves
  `context.Canceled`/`DeadlineExceeded`) plus a `ctx.Err()` fallback.
- **Boot reconciliation (`cmd/agezt/main.go`)** — `runScan` learned a `failed`
  set; a run carrying `task.failed` is terminal and never abandoned (M28's
  idempotency guard now covers both `task.completed` and `task.failed`).
- **Control plane (`kernel/controlplane/runs.go`)** — `runEntry` gained
  `Failed`/`FailedUnixMS`/`FailReason`; `collectRuns` folds `task.failed`;
  `handleRunsList` renders `status="failed"` + `reason` + a real duration
  (terminal timestamp − start); `handleRunsStats` counts `failed`, includes it in
  `terminal`, and uses `completed / (completed + failed + abandoned)` for the
  success rate. New `extractReason` payload helper.
- **CLI (`cmd/agt/runs.go`)** — `runs list` shows `failed (reason)` and a
  duration for failed runs; `runs stats` prints a `failed` count line.

## Design decisions

- **Status precedence `completed > failed > abandoned > running`.** A run should
  only ever carry one terminal marker, but the fold is defensive: if several
  appear (e.g. a stale `task.failed` plus a later `task.completed`), the most
  authoritative wins. Tested both directions.
- **Cancellation is a failure terminal (`reason=canceled`), not a separate
  status.** A deliberate halt is still a non-success end; the `reason` tag
  carries the nuance without exploding the status enum. This keeps "every run is
  exactly one of completed/failed/abandoned/running" true.
- **Best-effort, defer-based emission.** One registration point covers every
  error return in a function with a dozen of them — far less error-prone than
  emitting at each `return`. If the bus is already torn down (e.g. a hard
  shutdown stuck in a syscall dial), the publish silently fails and M28's boot
  reconciliation abandons the run instead — defense in depth, never a double mark.
- **`failed` counts against `success_rate`; `running` does not.** In-flight runs
  haven't failed — they just haven't finished — so only terminal non-success
  states (failed + abandoned) lower the rate.

## Tests

- `kernel/agent/agent_test.go` — provider error → `task.failed(reason=error)` and
  **no** `task.completed`; max-iters → `reason=max_iters`; cancelled context →
  `reason=canceled`; and the happy path emits **no** `task.failed`.
- `cmd/agezt/main_test.go` — `runScan` does not abandon a run carrying
  `task.failed` (it's terminal).
- `kernel/controlplane/runs_test.go` — a failed run renders `status="failed"` +
  `reason`; `completed` beats `failed` in the precedence; and the stats test now
  carries a failed run (3 completed / 1 failed / 1 abandoned / 1 running →
  `terminal=5`, `success_rate=0.6`).

Test count: **1212 → 1218**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged. My added lines are gofmt-clean (the CRLF/doc-comment artifacts that
`gofmt -l` flags in `runs.go`/`runs_test.go`/`cmd/agt/runs.go` are all
pre-existing and untouched by my edits).

## Live proof (mock provider + strict capability gate)

The always-on mock fallback rescues ordinary provider errors, but the M25 strict
capability gate (`AGEZT_MODEL_STRICT=on`) rejects a tools request to a
tool-incapable catalog model **terminally** (no fallback) — a clean,
network-free way to drive a real agent-loop error:

```
$ agt run "list files"
  [evt seq=0 kind=task.received]
  [evt seq=1 kind=llm.request]
  [evt seq=3 kind=task.failed]
agt run: …: governor: model does not support tool-use: model "weak-1" (request carries 7 tool(s))

$ agt runs list
  run-01KSZCF32TBFYTTB383RQ4XM14
    started : 2026-05-31 19:04:27   status: failed (error)      duration: 2ms   iters: 0
    intent  : list files

$ agt runs stats
  completed : 0
  failed    : 1
  running   : 0
  abandoned : 0
  success   : 0.0% (0/1 terminal)
```

The journal carries `task.failed` with `reason:"error"` and the full message —
emitted live by the agent loop, folded by the shared `collectRuns`, rendered by
both `runs list` and `runs stats`.

## What's next

The resilience/observability axis still has the most runway:

1. **Per-run wall-clock timeout** (MED risk) — an optional `MaxDuration` around
   the agent loop so a slow provider / blocking tool can't hang a run *within* a
   live session. Pairs directly with M30: a timeout would return an error whose
   `failureReason` is already wired to `timeout`, so it lands as
   `task.failed(reason=timeout)` for free. The remaining work is the cancel
   plumbing in `kernel/runtime` + distinguishing it from an operator halt.
2. **`agt runs stats --since <dur>`** — a time-bounded health view, now that the
   failure term makes the rate meaningful.
3. **Clean run-cancel plumbing** — make daemon shutdown / `agt halt` cancel
   in-flight run contexts so they emit `task.failed(reason=canceled)` live
   instead of relying on M28's boot abandon. (Today a dial stuck in a syscall
   can't be cancelled, so M28 stays the safety net.)
