# Phase M790 — standing orders run AS a named agent

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 8
— completes the autonomous ask→wake→answer cycle (M788 board.dm.<slug> trigger
+ M783 identity).

## What

`Order.Agent` (roster slug): every firing — event, cron, manual — executes AS
that identity (soul → persona, model + fallback chain, memory scope, per-run
cost ceiling as the default; the order's own budget wins). Unknown/paused →
`standing.error` with reason, never a silent default-identity run.
`standing.fired` payload records the agent.

## Changes

- `kernel/standing`: Order += `Agent` (additive; persists via existing JSON).
- `kernel/runtime`: **`WithAgentProfile(ctx, p)`** — one-call identity
  application (soul/model/chain/scope; cost deliberately left to callers so
  explicit budgets win). The standing runner uses it; handleRun keeps its
  inline application (model resolves before the vision gate there).
- `cmd/agezt` fire path: resolve → refuse (standing.error) or apply; fired
  payload += agent; profile MaxCostMc as default when the order has none.
- CLI `agt standing add --agent <slug>` + usage; control-plane standing_edit
  += agent key ("" clears); webui /api/standing/edit allowlist += agent.

## Tests (2 new)

- `TestWithAgentProfile_AppliesIdentityToRun` (runtime, real kernel): the
  provider's actual completion request carries the soul in System, the
  profile model, the dupe-skipped chain, AND the memory-scoped private note
  in the injected context — all four identity facets in one run.
- `TestStanding_AgentFieldRoundTrips` (controlplane, wire): add with agent →
  edit to another → clear with "" (omitempty drops it).

## Runtime smoke (isolated AGEZT_HOME)

Two event-triggered orders on `kernel.resume`: "as-agent" (agent=researcher)
and "ghost-order" (agent=ghost). One halt/resume: journal shows
`standing.error` for the ghost (seq 5, no run) and `standing.fired` → full
run to `task.completed` (seq 6–12) for the real agent. 0 panics, clean
shutdown. (Payload-field extraction via journal-grep JSON didn't parse in the
shell; the flow is proven by the tail + the payload composition is the same
code path covered above. Soul/model application proven by the runtime test —
the fire path calls the same WithAgentProfile.)

LESSON (smoke hygiene): `export AGEZT_HOME` does NOT persist across Bash tool
calls — set it per command, or `agt` silently targets the default home.

## Gate

Full suite + vet + staticcheck green; linux cross-build OK; go.mod unchanged;
no frontend change (Standing view edit route accepts the key; form field =
follow-up). CI org-billing still blocked → local battery + arc-authority merge.

## The recipe (now real)

```
agt agent add researcher --soul "…" --fallbacks m2,m3 --max-cost 0.50
agt standing add --name "researcher answers" \
    --event "board.dm.researcher" --agent researcher \
    --plan "Use board op=inbox to=researcher; answer each waiting message with op=reply."
# any agent: board op=send to=researcher text="…" → wake → answer → op=replies
```

## Next in the arc

Standing view --agent form field · Board view threading · workdir wiring ·
per-agent daily budget ledger.
