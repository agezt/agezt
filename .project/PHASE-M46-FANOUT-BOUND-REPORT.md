# Phase Report — Milestone M46 (Bound sub-agent fan-out)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Sixth step on the multi-agent axis —
> and the FIRST that *governs* rather than *observes*: from "see the fan-out"
> (M45) to "bound the fan-out".

## Why

The multi-agent axis was, through M45, entirely observational: M41–M44 surfaced
each delegation, M45 surfaced the *scale* of fan-out in aggregate. But the
runtime still let a single lead run spawn an **unbounded** number of sub-agents
at one level — only delegation *depth* was capped (`SubAgentMaxDepth`, default 1).
Depth caps recursion; nothing capped breadth. A runaway or adversarial lead could
fan out without limit, multiplying provider spend and load (sub-agents share the
lead's governor — `subagent.go:141`). M45 made that breadth *visible*; M46 makes
it *governable* — the natural sequel, and the first governance lever on this axis.

## What shipped

- **`Config.SubAgentMaxFanout int` (`kernel/runtime/runtime.go`)** — caps how many
  sub-agents a single agent run may spawn at its level. `0` (the default) means
  unbounded — the historical behaviour preserved exactly.
- **Per-correlation fan-out tally (`Kernel.fanout map[string]int`)** — an
  in-memory counter keyed by the *spawning* correlation (the lead run, or a
  sub-agent that itself delegates), guarded by the existing `k.mu`. Initialised in
  `Open` alongside `k.runs`.
- **The guard in `runSubAgent`** — after the depth check, when `SubAgentMaxFanout
  > 0` and the spawner has a correlation, the Nth+1 delegate call is refused with
  `max sub-agent fan-out N reached` (returned as a tool error the lead adapts to —
  exactly the depth guard's shape). The tally increments under the lock before the
  spawn proceeds.
- **Lifecycle cleanup, leak-free** — the tally for a top-level run is released in
  `RunWith`'s existing defer (next to `delete(k.runs, corr)`); the tally for a
  nested spawner (a sub-agent that delegated) is released by a defer in
  `runSubAgent` keyed on that child's own correlation. No entry outlives its
  spawning run.
- **Daemon wiring (`cmd/agezt/main.go`)** — `AGEZT_SUBAGENT_FANOUT=<n>` sets the
  cap; `0`/absent/malformed = unbounded. Single enable point, default-off.
- **Demo hook generalised** — `AGEZT_DEMO_DELEGATE=N` (N≥2) scripts the lead
  attempting N delegations so the cap is observable live with
  `AGEZT_SUBAGENT_FANOUT=N-1` (the `=1` single-delegation demo is unchanged).

## Design decisions

- **Mirror the depth guard, don't invent a mechanism.** The sibling
  `SubAgentMaxDepth` check returns a plain error that surfaces as a `delegate` tool
  error; M46 does exactly the same for breadth. Same shape, same recovery path
  (the lead sees the error and works around it), same default-off ergonomics. No
  new event kind, no Edict rule — consistency over novelty.
- **Observable for free via `tool.result`.** A refused delegate produces no
  `subagent.spawned` (correct — it didn't delegate), but the agent loop already
  journals the tool error as a `tool.result` with `IsError`. So the denial is
  visible in `agt runs show` / `agt journal tail` and the M45 metric correctly
  excludes it — no new observability plumbing needed. (Proven: the journal shows
  `delegation failed: max sub-agent fan-out 2 reached`.)
- **In-memory tally, not a journal scan.** Counting `subagent.spawned` events per
  correlation on every delegate call would put an O(journal) scan in the hot agent
  loop. A mutex-guarded map keyed by spawning correlation is O(1) and cleaned up
  deterministically when the spawning run ends — no unbounded growth on a
  long-lived kernel.
- **Key on the spawner, attribute honestly.** The counter keys on the *current
  agent's* correlation, so a nested sub-agent that delegates gets its own
  independent budget (its fan-out isn't charged to its lead). A correlation-less
  spawn (no run context) can't be attributed and is left unbounded — an edge case
  that never arises through `RunWith` (which requires a correlation).

## Tests

`kernel/runtime/subagent_test.go`:
- `TestSubAgent_FanoutGuard` — `maxFanout=2`, lead delegates three times; exactly
  2 `subagent.spawned` events (3rd refused) and the lead still completes
  (`lead done`).
- `TestSubAgent_FanoutUnboundedByDefault` — `maxFanout=0`, three delegate rounds →
  three spawns, none refused (historical behaviour preserved).

Test count: **1267 → 1269**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=3 + AGEZT_SUBAGENT_FANOUT=2)

```
$ agt runs stats
  …
  delegations: 2 (from 1 run(s), max fan-out 2)     # M45 metric — cap held at 2

$ grep 'fan-out' $AGEZT_HOME/journal/*.jsonl
  delegation failed: max sub-agent fan-out 2 reached  # the 3rd attempt, refused

$ grep -c 'subagent.spawned' $AGEZT_HOME/journal/*.jsonl
  2
```

The lead attempted three delegations; the cap admitted two and refused the third
(journaled as a tool error), and the M45 aggregate confirms `max fan-out 2`.

## What's next

The multi-agent axis now has its first *governance* lever (M46) atop a complete
observability surface (M41–M45). Sharpest remaining frontiers:

1. **Per-delegation spend** (MED-HIGH) — sub-agents share the lead's governor
   budget; spend is journaled per provider-call, not per delegation. Thread the
   child correlation into the governor's budget-consumed events so `agt budget` /
   `runs stats` can attribute cost to a delegation. Makes the *spend* side of fan-out
   governable (M46 bounds the count; this would bound/surface the cost).
2. **Journal the run answer** (MED) — `llm.response`/`task.completed` carry
   `text_chars`/`usage`, not the body, so the M44 outcome and the "final answer:"
   arc section can't show real text. Adding it (weigh journal size + redaction)
   lights up both.
3. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` (M39
   pattern) + tenant-token allowlist; the last non-tenant-aware control surface.
4. **Surface the cap in `agt status` / boot banner** (LOW) — show the active
   fan-out/depth ceilings so an operator sees the delegation governance in effect.
