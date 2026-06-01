# Phase Report — Milestone M116 (agent loop guard)

> Status: **shipped** · Date: 2026-06-02 · agent robustness.

## Why

A model can get stuck retrying the SAME tool call with identical input — a
failing shell command, a 429'd HTTP fetch, a poll that never changes. Before
this, the loop re-executed that call every iteration up to MaxIter (25),
repeating the side effect and the cost 25 times before giving up. The agent
needs to notice and stop re-running a call that won't change.

## What shipped

- **Loop guard in the agent loop** — per run, the loop counts each exact
  `(tool, input)` request. Once a call exceeds `MaxIdenticalToolCalls` (default
  5), the loop REFUSES to execute it again and feeds the model a clear nudge
  ("…called with this exact input N times; the result will not change. Use
  different input or stop and answer.") instead of invoking the tool. The run
  isn't terminated — the model can adapt or finalize.
- **`LoopConfig.MaxIdenticalToolCalls`** — 0 → default; a negative value
  disables the guard.

## Design notes

- **Identical-input only.** A re-call with DIFFERENT arguments is never capped,
  so legitimate varied tool use is unaffected (proven by a test where distinct
  inputs all run).
- **Refuse, don't kill.** The guard stops the wasteful re-execution and its side
  effects / cost, but leaves the run alive so a well-behaved model recovers; a
  pathological model still hits MaxIter, just without re-running the tool dozens
  of times.

## Tests

- `TestRun_LoopGuard_CapsIdenticalCalls` — a provider that always asks for the
  same call executes the tool exactly the cap times (3), not all 12 iterations.
- `TestRun_LoopGuard_DistinctInputsNotCapped` — distinct inputs all run (no
  false positive).
- `TestRun_LoopGuard_DisabledByNegative` — a negative cap runs every iteration.

Test count: **1386 → 1389**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ AGEZT_DEMO_LOOP=1 agezt &     # a mock that always repeats the same shell call
$ agt run "loop forever"
$ agt journal grep "tool.invoked" --kind tool.invoked   → 5 match(es)   # capped (not 25)
$ agt journal grep "loop guard"                          → 20 match(es) # the refusals
$ agt tool log --errors
  ERROR  shell  loop guard: "shell" was already called with this exact input 5 times …
```
