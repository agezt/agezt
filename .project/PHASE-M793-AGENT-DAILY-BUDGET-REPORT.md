# Phase M793 — per-agent daily budget ledger

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity,
step 11 — the LAST gap-analysis item of this arc. The arc is complete.

## What

Profile += `MaxDailyMc` (USD/day). The Governor keeps `spentByAgentToday`
(slug-keyed, UTC-midnight rollover with the existing counters): every
completion an identity makes accrues; past the ceiling → refused with
`ErrAgentBudgetExceeded` (wraps `ErrBudgetExceeded`) + journaled
`budget.exceeded` scope=`agent` payload {agent, spent, ceiling} — so Alerts
and the M782 channel push say WHICH agent blew its allowance.

## Design

- Request fields `Agent` + `AgentDailyCeilingMc` (Governor-only, like
  TaskType/ModelChain; providers never see them); LoopConfig mirrors; loop
  copies. Ceiling travels with the request because the roster is live-edited
  — no governor-side config to stale.
- Wiring: `WithAgentIdent` ctx (runtime) set by `WithAgentProfile` +
  handleRun; delegate sets LoopConfig directly. Pre-check before routing;
  accrual at the same site as the per-task counters.
- Surface: `agt agent --max-daily` (add/set/show/list), Roster view forms +
  row (wholesale-edit coherence: the UI form MUST carry the field or a UI
  edit would clear a CLI-set ceiling — included for that reason).

## Tests (3 new/extended)

- governor `TestAgentDailyBudget_MetersAndRefuses`: meter → refuse (both
  errors.Is) → other identity flows → unattributed flows → ceiling-less
  metered-not-refused.
- runtime e2e extended: the actual provider request carries slug + ceiling.
- DE-FLAKE (drive-by): roster TestList_DeterministicOrder — same-millisecond
  Adds flipped the creation-time sort onto the ULID tiebreaker; injected a
  deterministic clock.

## Smoke

`agt agent add --max-daily 5.00 --max-cost 0.50` → show printed both
ceilings → run-as clean; 0 panics. (Refusal path is unit-proven; demo echo
is unpriced so a live trip needs a real provider.)

## Gate

Full suite + vet + staticcheck green; 457 vitest; dist rebuilt (LF); linux
cross-build OK; go.mod unchanged. CI org-billing still blocked → local
battery + arc-authority merge.

## ARC COMPLETE (M783–M793)

Identity · delegate-by-slug · console · memory · routing chain · A2A ·
chat · standing-as-agent · console catch-up · workdir · daily budget.
Next frontiers (gap analysis): code→tool Forge promotion (#4), governed
self-install (#3), vector memory (#5), brain distiller (#6).
