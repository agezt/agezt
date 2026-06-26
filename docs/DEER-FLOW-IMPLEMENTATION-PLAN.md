# DeerFlow Borrowing Implementation Plan

This is the AGEZT implementation plan for the DeerFlow review in
`DEER-FLOW-AGEZT-REPORT.md`. The goal is to borrow harness discipline without
copying DeerFlow's Python/LangGraph runtime.

## Phase 1 - Contract and Activation Foundation

Status: implemented in this slice.

- Add shared payload fixtures under `contract/fixtures`.
- Pin backend fixture shape with Go tests.
- Pin frontend context-compaction folding against the shared fixture.
- Add explicit skill activation via leading `/skill` and `/skills` directives.
- Journal skill activation mode with `activation: "explicit"` or `"auto"`.
- Strip explicit skill control lines from the task prompt before the model sees it.

## Phase 2 - Context Rescue

Status: implemented in this slice.

- Add `agent.DefaultContextRescueMarker`.
- Preserve marked tool outputs during context compaction.
- Add `skill_rescued_count` and `skill_rescued_chars` to `context.compacted`.
- Mark `skill show`, `skill files`, and `skill read` outputs so skill resources are
  not summarized away before the model can apply them.

## Phase 3 - Deferred Tool Discovery

Status: implemented as a first pass.

- Keep the existing lexical selector as the conservative default.
- Add deferred selector mode that keeps a pinned catalog tool visible and does not
  expose every schema on a no-match turn.
- Add runtime `tool_search` for read-only capability lookup over the current run's
  allowed tool set.
- Enable `tool_search` automatically when `AGEZT_TOOL_DISCOVERY_MAX` is active and
  the run has more tools than the visible schema cap.

## Phase 4 - Reload Boundary Registry

Status: implemented and surfaced.

- Add `settings.ReloadBoundaries` to group Config Center fields by live vs restart
  apply mode.
- Keep registered skill/plugin config restart-only unless the owning component adds
  a real hot-reload path.
- Return `reload_boundaries` from `config_schema`.
- Surface the live/restart counts in the Config Center UI.

## Phase 5 - Contract Visibility And Onboarding

Status: implemented in this slice.

- Render skill rescue counts in the Chat context inspector and compaction note.
- Add shared fixtures for async `subagent.completed` and `delegate_await` result
  payloads.
- Add `Install.md` with idempotent bootstrap, verify, and build commands.
- Document loop invariants for policy, tool invocation, compaction, delegation,
  deferred discovery, and reload boundaries.

## Phase 6 - Loop Invariants And Delegation Visibility

Status: implemented in this slice.

- Add a regression test for the loop's policy -> invoke -> result -> compact ->
  next request event order.
- Fold async `subagent.completed` events into the live Activity state, including
  success/failure outcome and parent/child linkage.
- Keep delegation visualization derived from AGEZT's existing journal/run data
  (`subagent.spawned`, `subagent.completed`, `parent_correlation`) instead of
  introducing a separate graph runtime.

## Phase 7 - Replay And Detail Contract Parity

Status: implemented in this slice.

- Format `subagent.spawned` and `subagent.completed` in Flight Recorder replay
  steps, including async success/failure outcomes.
- Surface delegation counts in Run Detail summaries.
- Add phase-timeline entries for delegated spawn/completion events.
- Reuse the shared `subagent_completed_async.json` contract fixture in frontend
  tests so replay/detail formatting stays tied to the wire payload.

## Next Slice

- Add a small plan/checkpoint event surface only if it can be backed by the
  existing journal without changing the agent loop into a graph engine.
- Add a replay/detail test for `context.compacted` skill rescue counters so Chat,
  Runs, and Flight Recorder explain the same compaction event consistently.
