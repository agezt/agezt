# Phase Report — Milestone M48 (Per-delegation spend cap)

> Status: **shipped** · Date: 2026-06-01
> SPEC-12 (multi-agent orchestration). Eighth step on the multi-agent axis,
> and the third *governance* increment — it **closes the count→cost→cap loop**:
> M46 bounded delegation COUNT, M47 attributed its COST, M48 CAPS that cost.

## Why

M47 made sub-agent spend visible (attributed per correlation), but visibility
isn't a guardrail: a lead could still fan out (within the M46 count cap) and burn
arbitrary budget — each sub-agent runs through the lead's own governor with no
spend ceiling of its own. The count cap (M46) limits *how many* sub-agents; it
says nothing about *how expensive* they are. M48 adds the cost ceiling: once a
run's sub-agents have collectively spent past `AGEZT_SUBAGENT_SPEND_CAP`, the next
delegation is refused — the cost analogue of the fan-out count cap.

## What shipped

- **`Config.SubAgentMaxSpendMicrocents int64` (`kernel/runtime/runtime.go`)** —
  caps the total spend a single run's sub-agents may collectively consume. `0`
  (default) = unbounded.
- **The guard in `runSubAgent`** (`kernel/runtime/subagent.go`) — after the depth
  and fan-out checks, when the cap is set and the spawner has a correlation, the
  delegation is refused with `max sub-agent spend $X.XXXX reached` (a tool error
  the lead adapts to, mirroring the fan-out guard) if the run's sub-agents have
  already spent ≥ the cap.
- **`subAgentSpendMicrocents(parentCorr)`** — a single forward journal pass builds
  the parent→children links (from `subagent.spawned`) and per-run spend (from M47's
  `budget.consumed`, now correlation-stamped), then totals spend over parentCorr's
  *transitive* descendants (BFS, `seen`-guarded against a cyclic link), excluding
  the lead's own spend. **Stateless** — no in-memory tally to maintain or leak.
- **Daemon wiring (`cmd/agezt/main.go`)** — `AGEZT_SUBAGENT_SPEND_CAP=<usd>` sets
  the cap as a USD amount (matching the `*_DAILY_CEILING` convention), stored as
  microcents; a malformed value is a hard startup error. `0`/absent = unbounded.

## Design decisions

- **Stateless journal read, not an in-memory tally.** M46's count cap keeps an
  in-memory `fanout` map because a count must be exact and cheap; spend, by
  contrast, is *already* durably journaled per correlation (M47) — and
  `bus.Publish` appends to the journal *before it returns*, so by the time the next
  delegate runs, every prior child's spend is on disk. Reading it back is race-free
  and needs no counter, no cleanup defer, no lifecycle coupling. The cost is an
  O(journal) scan, but only when the cap is enabled (opt-in) and only per
  delegation (bounded by the M46 count cap) — so it never touches the default path.
- **Transitive descendants, lead excluded.** The cap bounds what a lead's
  *delegation subtree* costs, not the lead's own direct spend. The BFS sums every
  sub-agent and sub-sub-agent (correct for `SubAgentMaxDepth > 1`), so a deep tree
  can't evade the cap by nesting.
- **Refuse-the-next, like the fan-out guard.** Spend crosses the cap *during* a
  child's run; the cap is checked *before* the next delegation. So the run that
  tips it over completes, and the *following* delegate is refused — identical
  semantics to M46, and the refusal surfaces through the existing `tool.result`
  (no new event), so M47's spend metric and the journal both show what happened.
- **USD env, microcents internally.** `AGEZT_SUBAGENT_SPEND_CAP` takes dollars
  (operators think in dollars; matches `AGEZT_TENANT_DAILY_CEILING`) and stores
  `usd × 1e9` microcents — the same unit `fmtUSD` and the governor use.

## Tests

`kernel/runtime/subagent_test.go` (integration — a real Governor behind the mock
so scripted usage journals `budget.consumed`, shared via `SetBus`):
- `TestSubAgent_SpendGuard` — $0.0021/call, $0.0030 cap: two delegations admitted,
  the third refused once sub-agents have spent $0.0042; the lead still completes.
- `TestSubAgent_SpendUnboundedByDefault` — cap 0: three delegations, three spawns.

Test count: **1272 → 1274**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, added lines gofmt-clean.

## Live proof (offline mock + AGEZT_DEMO_DELEGATE=3 + AGEZT_SUBAGENT_SPEND_CAP=0.003)

```
$ agt runs stats
  delegations: 2 (from 1 run(s), max fan-out 2)
  spend      : $0.0126 (delegated: $0.0042)

$ grep 'max sub-agent spend' $AGEZT_HOME/journal/*.jsonl
  delegation failed: max sub-agent spend $0.0030 reached

$ grep -c 'subagent.spawned' $AGEZT_HOME/journal/*.jsonl
  2
```

The lead attempted three delegations; two ran (spending $0.0042 of sub-agent
budget), and the third was refused once that total crossed the $0.0030 cap.

## What's next

The multi-agent axis now has a complete governance triad — count (M46), cost
visibility (M47), cost cap (M48) — atop full observability (M41–M45). Sharpest
remaining frontiers:

1. **Surface the delegation ceilings** (LOW) — show the active depth / fan-out /
   spend caps in `agt status` and/or the boot banner, so an operator sees the
   governance in effect (today each cap is silent until a delegation is refused).
2. **Per-run spend in `agt runs list` / `show`** (LOW) — `runEntry.SpentMicrocents`
   (M47) is already computed; surface it per row and on the M44 delegation `↳`
   line, so spend is visible per-run, not only in aggregate.
3. **Journal the run answer** (MED) — `llm.response`/`task.completed` carry
   `text_chars`/`usage`, not the body; adding it lights up the M44 outcome and the
   "final answer:" arc section.
4. **Tenant-scoped `why`** (LOW-MED) — route `handleWhy` via `kernelFor` + a
   tenant-token allowlist; the last non-tenant-aware control surface.
