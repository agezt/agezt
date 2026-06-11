# PHASE M854 — Per-agent activity log

**Status:** shipped
**Milestone:** M854
**Theme:** Show, per agent, *what it did* — the runs it executed, the council
consults and sub-agent delegations during them, the memory it wrote, its board
messages, and changes to its own profile. Owner ask: *"ne oldu ne bitti, hangi
agent fikir danıştı"* (#48 — the per-agent panel's missing activity half; the
provider/model config already existed in the Roster editor).

Derived entirely from the existing journal — no new store — joining the M851
memory `actor`, the board `from`, the M846 `subagent.spawned` agent, and a new
agent tag on `task.received`.

## What shipped

- **Runs are attributed to the agent.** `task.received` now carries the named
  agent it ran AS (`cfg.Agent`) in its payload (kernel/agent), so a run is
  traceable to its agent. Empty for the daemon's default identity.
- **`CmdAgentActivity {ref, limit}`** (kernel/controlplane/roster.go) — a
  two-pass journal scan: pass 1 collects the agent's run correlations (from the
  tagged `task.received`); pass 2 emits every attributable event with a one-line
  summary: started/completed/failed a run, consulted the council, delegated to a
  sub-agent (or ran as one), memory write, board message/DM, profile change.
  Newest first, capped; returns `{slug, activity, count, total}`.
- **Web UI:** an "activity" (Activity icon) toggle on each Roster row opens
  `AgentActivity`, a compact timeline fed by `/api/agents/activity`. Each line
  shows the event kind, the summary, and the time.

## Surface

- `kernel/agent/agent.go` — `agent` tag on the `task.received` payload.
- `kernel/controlplane/roster.go` — `handleAgentActivity`, `agentActivitySummary`,
  `plString`/`truncate` helpers.
- `kernel/controlplane/{protocol,server}.go` — `CmdAgentActivity` + dispatch.
- `kernel/webui/webui.go` — `/api/agents/activity` (readArgs, `ref`/`limit`).
- `frontend/src/views/Roster.tsx` — `AgentActivity` component + per-row toggle;
  dist rebuilt (LF).
- `kernel/controlplane/roster_test.go` — `TestAgentActivity_ShowsRuns`.

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `controlplane`, `agent`, `webui` green; vitest **517 passed**; dist rebuilt. No
  new env; go.mod unchanged.
- **Integration:** `TestAgentActivity_ShowsRuns` runs a real task AS `scout`
  (mock provider) and asserts the timeline attributes the run ("started a run:
  find the thing"); an unrelated agent's timeline is empty.

## Notes
- Attribution is by the slug fields events already carry plus the agent's run
  correlations, so council consults and delegations that happened *during* a
  run are correctly tied to the agent that owned the run.
- Runs that predate M854 won't carry the agent tag (their `task.received` was
  written before it); activity for them shows from the other signals (memory,
  board) only. New runs are fully attributed.
