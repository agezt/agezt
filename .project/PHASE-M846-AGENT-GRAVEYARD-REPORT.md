# PHASE M846 — Dead-Agent Graveyard (retire / revive)

**Status:** shipped
**Milestone:** M846
**Theme:** Reaper, part 2 — retire dead agents to a recoverable graveyard (after
the M845 dead-file collector). Owner ask: *"ölü ajan defincisi lazım, artık
gerekmeyen ölmüş agentları mezarlığa taşımak lazım… onaylı veya otonom ve haber
vererek ve etkileri ile"* — move no-longer-needed agents to a graveyard, with
approval, surfacing the impact.

## What shipped

A retire/revive lifecycle for roster agents that is **recoverable** (distinct
from `remove`, which deletes) and **impact-aware** (it tells you what depends on
the agent before you retire it).

- **Retire** moves an agent to the graveyard: it is paused (`Enabled=false`),
  stamped with `RetiredMS`, and any **delegation** to it is refused with a
  dedicated message — *"agent %q is retired — revive it first (agt agent revive
  %s)"*. The full profile is preserved.
- **Impact first.** Retiring computes the impact **before** the state change —
  the standing orders that fire that agent — and returns it so the operator sees
  what would be affected (the Roster confirm dialog and the CLI both surface it).
- **Revive** clears `RetiredMS`/`Retired` and brings the agent back **paused**,
  so the operator decides when to `resume` it.
- Every retire/revive is **journaled** as `roster.updated` with `action`
  `retired` / `revived`.

## Surface

### Kernel / roster
- `kernel/roster/roster.go` — `Profile.Retired bool` + `Profile.RetiredMS int64`
  (omitempty); `SetRetired(ref, retired) (Profile, error)` (retire stamps
  RetiredMS + pauses; revive clears it; rolls back on save failure). `Update()`
  preserves both fields across edits.
- `kernel/runtime/runtime.go` — `SetProfileRetired(ref, retired)` (journals the
  `roster.updated` retired/revived event); `AgentImpact(slug) []string` (scans
  standing orders for `o.Agent == slug`, returns `"name (id)"`).
- `kernel/runtime/subagent.go` — profile resolution refuses a retired agent
  before the enabled check, with the revive hint.

### Control plane
- `kernel/controlplane/protocol.go` — `CmdAgentImpact`, `CmdAgentRetire`,
  `CmdAgentRevive`.
- `kernel/controlplane/roster.go` — `handleAgentImpact` (returns
  `standing_orders` / `standing_count`), `handleAgentRetire` / `handleAgentRevive`
  via `handleAgentSetRetired` (computes impact before retire, returns
  `{profile, impact}`).
- `kernel/controlplane/server.go` — dispatch for the three commands.

### Web UI
- `kernel/webui/webui.go` — write routes `/api/agents/retire`,
  `/api/agents/revive`; read route `/api/agents/impact` (readArgs, `ref`). (None
  touch `apiRoutes`, so the read-only guard is unaffected.)
- `frontend/src/views/Roster.tsx` — `graveyard` marker (Skull) + struck-through
  slug for retired agents; retire flow fetches impact and folds it into the
  confirm; Archive/ArchiveRestore buttons swap retire↔revive; pause hidden when
  retired. dist rebuilt (committed LF).

### CLI
- `cmd/agt/agent.go` — `agt agent retire <slug|id>` (prints the impact warning)
  and `agt agent revive <slug|id>`.

## Verification

- **Gate:** `GOMAXPROCS=4 go test ./kernel/roster/ ./kernel/runtime/
  ./kernel/controlplane/ ./kernel/webui/` green; `go build ./...`, `go vet`,
  `staticcheck`, linux cross-build all clean; vitest **517 passed (76 files)**;
  dist committed LF.
- **Unit:** `TestSetRetired_GraveyardLifecycle` (retire pauses + stamps, Update
  preserves, revive clears).
- **Live (isolated AGEZT_HOME, copied catalog + creds):**
  1. `agt agent add deadguy` → created.
  2. `agt agent retire deadguy` → retired; profile shows `retired=true`,
     `enabled=false`, `retired_ms` stamped.
  3. `agt run --agent deadguy` → refused.
  4. Bound a standing order to the agent, retired again → impact warning listed
     the order: `⚠ 1 standing order(s) referenced this agent: nightly-deadguy (…)`.
  5. `agt agent revive deadguy` → `retired` cleared (back paused).
  6. Journal: `action:"retired"` ×2, `action:"revived"` ×1.

## Notes
- Default-allow posture preserved: retire/revive are operator actions; nothing is
  auto-restricted. The "autonomous + notify" reaper variant (auto-retire idle
  agents with a heads-up) remains a follow-up on task #53.
- No new `AGEZT_*` env vars; go.mod unchanged.
