# Phase M786 — agent memory scope: private notes follow the identity

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 4
(M783 roster → M784 delegate-by-slug → M785 console → this).

## What

A roster profile's `MemoryScope` (default: the slug) now flows into runs:

- **Context injection** (`RunWith`): `RecallScoped(corr, intent, topK,
  memory.ScopeFrom(runCtx))` — the named agent's private notes inject
  alongside shared memory; unscoped runs unchanged (scope "" ≡ old Recall).
- **Memory tool**: recall's scope DEFAULTS to the ctx scope (explicit param
  wins). Writes stay shared by default — M652's "shared brain, private
  notes": private writes remain opt-in via the param.
- **Run boundary**: `handleRun` sets `memory.WithScope(ctx, scope)` when an
  `agent` resolves; **delegate** sets it on the child ctx, so a
  `delegate(agent="researcher")` child reads researcher's notes too.

## Changes

- `kernel/memory/manager.go`: ctx key + `WithScope`/`ScopeFrom` (mirrors
  `WithCorrelation`); tool recall ctx-default.
- `kernel/runtime/runtime.go`: injection → `RecallScoped(..., ScopeFrom)`.
- `kernel/runtime/subagent.go`: child ctx scope from the profile.
- `kernel/controlplane/server.go`: run-as-agent ctx scope.

## Tests (4 new, all layers)

- memory: `TestTool_RecallDefaultsToCtxScope` (ctx scope sees shared+own,
  never another scope; explicit param wins; unscoped = shared only) and
  `TestTool_RememberStaysSharedByDefault` (scoped ctx does NOT privatise
  writes).
- runtime: `TestDelegate_AgentMemoryScopeFollowsChild` — real kernel, child's
  memory recall (journaled tool.result) surfaces the profile-private note.
- controlplane: `TestRun_AsAgent_MemoryScope` — end-to-end over the wire,
  asserted on the actual provider requests: the agent run's injected system
  contains the shared fact AND the private note; the plain run contains
  shared only (no leak).

## Gate

Full suite `-p 2 ./...` green; vet + staticcheck clean; linux cross-build OK;
go.mod unchanged; no frontend change. Isolated-daemon smoke: seeded shared +
scope-tagged memories via `agt memory add --tag scope=…`, ran `--agent` and
plain runs, clean, 0 panics (the behavioural proof lives in the wire-level
e2e test, which asserts the injected provider context directly). CI
org-billing still blocked → local battery + arc-authority merge.

## Next in the arc

Fallbacks → per-agent routing chain (governor) · workdir wiring · A2A
ask/reply on the board · chat: converse AS a named agent.
