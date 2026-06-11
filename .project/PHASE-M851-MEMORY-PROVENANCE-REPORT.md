# PHASE M851 — Memory Provenance (who added / updated it)

**Status:** shipped
**Milestone:** M851
**Theme:** Record WHO wrote each memory — the acting agent's slug, or "operator"
/ "distill" for non-agent writes. Owner ask: *"memory dahil her şeyin hangi agent
tarafından eklendiği update edildiği detayları lazım"* (task #35) — for
everything, including memory, show which agent added/updated it.

Memory is the highest-value, highest-churn store and the owner's explicit
example, so it leads. The reusable piece — an exported agent-identity context
helper — sets up the same treatment for skills/world/data-lake next.

## What shipped

- **`agent.WithAgent` / `agent.AgentFromContext`** (`kernel/agent/toolctx.go`) —
  a context helper for the acting agent's slug, alongside the existing
  correlation/workdir helpers. The runtime's `WithAgentIdent` now also stamps it,
  so any provenance-aware tool can read who is acting.
- **`memory.Record.AddedBy` / `UpdatedBy`** — `AddedBy` is first-writer-wins
  (the original author survives a reinforce by another agent, like `SourceEvent`);
  `UpdatedBy` tracks the most recent writer. `RememberSpec.Actor` carries it in;
  the `memory.written` event payload gains an `actor` field too.
- **Attribution at every write site:** the `memory` tool stamps the agent slug
  (`agent.AgentFromContext`, falling back to `"agent"`); auto-distilled summaries
  stamp `"distill"`; operator console/CLI writes (`Remember`/`Supersede` handlers)
  stamp `"operator"`. Actor-less reinforces (e.g. automatic recall-reinforce)
  never erase recorded provenance.
- **Surfaced:** control-plane `recordView` emits `added_by`/`updated_by`; the
  Memory view shows `by <author> · upd. <updater>` under each record.

## Surface

- `kernel/agent/toolctx.go` — `WithAgent` / `AgentFromContext`.
- `kernel/runtime/runtime.go` — `WithAgentIdent` stamps the agent key too.
- `kernel/memory/memory.go` — `Record.AddedBy` / `UpdatedBy`.
- `kernel/memory/manager.go` — `RememberSpec.Actor`, first-writer-wins +
  latest-updater logic, `actor` in the event payload, `toolActor` helper,
  distill actor.
- `kernel/controlplane/memory.go` — `operator` actor on add/supersede;
  `added_by`/`updated_by` in `recordView`.
- `frontend/src/views/Memory.tsx` — author/updater display; dist rebuilt (LF).
- `kernel/memory/provenance_test.go` — `TestRemember_ActorProvenance`.

## Verification

- **Gate:** `go build`, `go vet`, `staticcheck`, linux cross-build clean;
  `agent`, `memory`, `controlplane` green; vitest **517 passed**; dist rebuilt.
  No new env; go.mod unchanged.
- **Unit:** `AddedBy` first-writer-wins across a reinforce by a different agent;
  `UpdatedBy` = latest writer; an actor-less reinforce preserves both. Existing
  `SourceEvent` preservation test still green.
- **Live (isolated home):** `agt memory add` (operator path) → the record reads
  back over `/api/memory` with `added_by=operator`, `updated_by=operator`. The
  agent-slug path is exercised by the unit test + the context threading; a full
  LLM-driven agent write isn't reproducible in the build sandbox (offline-mock).

## Notes
- First-writer-wins matches the existing `SourceEvent` semantics, so a fact's
  original author is stable even as peers reinforce it.
- Follow-ups (the rest of "her şey"): skills already carry `SourceEvent`; the
  same `AgentFromContext` can stamp an author on skills, world-model entities, and
  data-lake records (which already have `CreatedBy`/`UpdatedBy` fields awaiting a
  writer).
