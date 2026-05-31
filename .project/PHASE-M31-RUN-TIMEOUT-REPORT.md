# Phase Report — Milestone M31 (Per-run wall-clock timeout)

> Status: **shipped** · Date: 2026-05-31
> SPEC-08 (operability/resilience). Fourth step on the resilience/observability
> axis (M28 → M29 → M30 → M31). M30 made an errored run terminal; M31 makes a
> *hung* run errored — bounding a run's wall-clock so it can't stall forever
> inside a live session.

## Why

A run had two natural bounds: `MaxIter` (tool-call rounds) and an explicit
`Halt`. Neither caps wall-clock. A provider that dials a dead endpoint, a tool
that blocks on a slow syscall, or a model that streams glacially could hang a
single run indefinitely *within a live session* — M28's orphan recovery only
reclaims such a run across a **restart**, which is far too late for an operator
watching a run that should have finished in seconds.

M30 already built the landing pad: a run that returns an error after
`task.received` emits `task.failed`, and `failureReason` maps
`context.DeadlineExceeded` to `reason=timeout`. So all M31 needs is to arm a
deadline on the run context — the terminal event, the `agt runs` rendering, and
the success-rate accounting all fall out for free.

## What shipped

- **`runtime.Config.MaxDuration time.Duration`** — optional per-run wall-clock
  budget. `0` (default) means no cap.
- **`RunWith` deadline wrap (`kernel/runtime/runtime.go`)** — when
  `MaxDuration > 0`, the run context is `context.WithTimeout(ctx, MaxDuration)`
  instead of `context.WithCancel(ctx)`. The cancel registered in `k.runs` (the
  halt handle) is the timeout's cancel, so **Halt still works** — and because a
  manual `cancel()` cancels with `Canceled` while a fired deadline cancels with
  `DeadlineExceeded`, the halt-vs-timeout distinction is automatic via context
  semantics (→ `reason=canceled` vs `reason=timeout`). No extra bookkeeping.
- **Daemon wiring (`cmd/agezt/main.go`)** — `AGEZT_RUN_TIMEOUT=<duration>` parsed
  with `time.ParseDuration`; a malformed value is a hard startup error, a
  non-positive value is treated as off. Boot banner line `run timeout : …`.
- **No new event, no new control-plane verb, no CLI change** — `agt runs`
  already renders `failed (timeout)` because M30 carries the reason through.

## Design decisions

- **Reuse context semantics for the halt/timeout split.** The obvious
  alternative — a flag or sentinel to mark "this cancel was a timeout" — is
  redundant: `WithTimeout` already distinguishes a fired deadline
  (`DeadlineExceeded`) from a manual cancel (`Canceled`). `failureReason` (M30)
  keys on exactly those, so the timeout reason is correct with zero new state.
  `TestRunWith_HaltBeatsTimeout` pins this: a halt under an armed (long) timeout
  still yields `Canceled`, never `DeadlineExceeded`.
- **Off by default.** A wall-clock cap is a policy choice (some runs legitimately
  take minutes); defaulting it on would silently truncate long agent tasks. The
  operator opts in per deployment.
- **The deadline rides the existing run context**, so it composes with everything
  downstream: the provider's HTTP request context (the dial is cancelled
  mid-flight — verified live), tool invocations, and the agent loop's
  per-iteration `ctx.Err()` check. No code path needed a timeout parameter.
- **Governor never masks it.** `shouldFallback` already returns false for
  `context.DeadlineExceeded`, so a timed-out primary is not silently rescued by
  the mock fallback — the deadline error reaches the agent loop and becomes
  `task.failed(reason=timeout)`.
- **Scope: single runs (`RunWith`).** Plan nodes route through `LoopRunner →
  RunWith`, so each node inherits a per-node wall-clock cap automatically;
  `RunPlan`'s own context stays a plain cancel (a per-*plan* budget is a separate,
  future knob).

## Tests

`kernel/runtime/runtime_test.go`:
- `TestRunWith_TimesOut` — a blocking provider under a 30 ms budget returns
  `context.DeadlineExceeded` within ~1 s, and the journal's `task.failed` carries
  `reason=timeout`.
- `TestRunWith_HaltBeatsTimeout` — with a 10 s budget armed, an explicit `Halt`
  at 50 ms still cancels with `Canceled` (never `DeadlineExceeded`), proving the
  operator-halt path stays distinct from a timeout.
- `TestRunWith_CompletesUnderTimeout` — a fast run under a generous budget
  finishes normally; the deadline doesn't interfere with the happy path.

Test count: **1218 → 1221**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, all touched files gofmt-clean.

## Live proof (mock provider + black-hole endpoint)

```
$ AGEZT_RUN_TIMEOUT=2s agezt …
  run timeout      : 2s per run (task.failed reason=timeout on overrun)

$ agt run "say hello"          # provider dials a non-routable 10.255.255.1:81
  [evt seq=3 kind=task.failed]
agt run: …: openai: http: Post "http://10.255.255.1:81/v1/chat/completions": context deadline exceeded
(elapsed ~2s)

$ agt runs list
  run-01KSZEGH926M8CGA0SY2Y155T6
    started : 2026-05-31 19:40:11   status: failed (timeout)    duration: 2.0s   iters: 0
    intent  : say hello

$ agt runs stats
  completed : 0
  failed    : 1
  …
  success   : 0.0% (0/1 terminal)
```

The dial was cut off at exactly the 2 s budget (the HTTP request honoured the run
context), the agent loop emitted `task.failed(reason=timeout)` live, and both
`runs list` and `runs stats` rendered it — confirming the full M30→M31 wiring.

## What's next

The resilience/observability axis still has runway:

1. **Clean run-cancel plumbing for halt/shutdown** (MED) — make daemon shutdown
   and `agt halt` cancel in-flight run contexts so they emit
   `task.failed(reason=canceled)` live, rather than relying on M28's boot abandon.
   (A dial stuck in a non-cancellable syscall still needs M28 as a safety net.)
2. **`agt runs stats --since <dur>`** (LOW) — time-bounded health view; the
   failure + timeout terms now make a windowed rate meaningful.
3. **Per-tool timeout** (MED) — a finer cap than per-run, so one slow tool fails
   its call (fed back to the model) without killing the whole run.
