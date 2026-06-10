# Phase M783 — Agent Roster: durable named agents

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity (gap #1
of the 2026-06-10 vision gap analysis — the prerequisite for A2A messaging,
per-agent budgets/tools, and the brain loop)

## What

Agents stop being ephemeral runs. `kernel/roster` is a durable registry of
named agent identities: slug (immutable address) + soul (system prompt) +
model (+ ordered fallbacks, stored for the routing arc) + default task type +
per-run spend ceiling (USD-microcents) + memory scope + workspace subdir +
description + enabled. `agt run --agent <slug>` runs AS the agent.

## Design

- **Store** (`kernel/roster/roster.go`): atomic-JSON file store, the
  kernel/standing pattern verbatim (mutex, rollback-on-save-failure,
  kernel-assigned id/timestamps, deterministic List). Lookup by id OR slug
  everywhere. Validation: slug `^[a-z0-9][a-z0-9._-]{0,63}$`, soul ≤ 64KiB,
  ≤ 8 fallbacks, non-negative cost, workdir must be relative and cannot
  escape (`..`/absolute rejected).
- **Kernel** (`kernel/runtime`): `roster` field + `Roster()` accessor +
  `AddProfile/UpdateProfile/SetProfileEnabled/RemoveProfile` publishing
  `roster.created/updated/removed` (new append-only event kinds).
- **Control plane**: `agent_list/add/edit/set_enabled/remove` commands
  (`kernel/controlplane/roster.go`), standing-handler shapes; edit applies
  mutable fields wholesale, identity/lifecycle protected by the store;
  `enabled` accepts bool or string (webui query transport).
- **Run seam** (`handleRun`): new `agent` arg resolved BEFORE the vision gate
  (so the gate judges the model the run actually uses). Profile fills the
  gaps — model when `--model` absent, soul when `--system` absent, cost
  ceiling when `--max-cost` absent; explicit flags always win. Unknown agent
  → usage error; paused agent → refused with `agt agent resume` hint.
- **CLI** (`cmd/agt/agent.go`): `agt agent list/add/show/set/pause/resume/
  remove` + `agt run --agent <slug>`. `set` is a partial edit: reads the
  current profile, overlays only the provided flags.
- **Web UI routes** (`kernel/webui`): GET `/api/agents`; POST
  `/api/agents/{add,edit}` (JSON body), `/api/agents/{enable,remove}`
  (query args). Console view = next milestone.

## Tests (10 new, all green)

- roster: slug rules (8 good/8 bad), bounds + workdir-escape table, identity
  defaults + duplicate-slug refusal, id/slug lookup, Update protects
  id/slug/created/enabled + rolls back invalid mutations, pause/resume/remove,
  persistence across reopen (all fields), deterministic order.
- controlplane: `TestAgent_CRUDRoundTrip` (add→list→edit→pause→remove over the
  wire + journal asserts roster.created/updated/removed + duplicate slug and
  unknown-ref errors); `TestRun_AsAgent` (dry-run plan proves profile model +
  soul applied from the SAME locals the real run uses; explicit `--model`
  wins; unknown/paused refused).
- webui guard test (`TestAPIReadOnly`) extended: `agent_list` registered as a
  read command.

## Runtime smoke (isolated AGEZT_HOME, demo echo)

add (with soul/desc/$0.50 ceiling) → list/show render → `run --agent
researcher --dry-run` showed `system prompt: per-run` + `cost cap: $0.50` →
real run completed → pause → run refused exit 1 with resume hint → resume →
set (soul v2) → remove → empty-roster hint. Journal: `roster.created`
seq=0. 0 panics, clean shutdown, smoke dir removed.

## Gate

Full suite `GOMAXPROCS=3 go test -p 2 ./...` green (85 pkgs incl. new roster);
`go vet` + staticcheck clean; `GOOS=linux` cross-build OK; go.mod unchanged;
frontend dist untouched. CI org-billing still blocked → local battery +
arc-authority merge (PRs #136+ pattern).

## Next in the arc

- M784+: Agents console view (roster CRUD from the Web UI — jsonRoute +
  `New<X>Form` recipe), delegate-tool `agent` param (sub-agents by slug),
  per-agent memory-scope + workdir wiring into the run, A2A ask/reply on the
  board, per-agent routing chains via Fallbacks.
