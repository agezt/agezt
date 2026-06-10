# Phase M784 — delegate(agent="slug"): sub-agents as named roster agents

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity, step 2
(builds on M783 roster).

## What

The `delegate` tool gains an optional `agent` param (roster slug). The
sub-agent runs AS that identity: soul → child persona (replaces the daemon
persona layer; sub-agent preamble stays on top), model + task_type as defaults
(explicit args win), MaxCostMc → child's `MaxRunCostMicrocents` (on top of the
M46/M48/M629 tree caps). `subagent.spawned` payload records `agent: <slug>`.
Unknown/paused agent → tool error the lead adapts to; zero spawns.

## Changes

- `kernel/runtime/subagent.go`: tool schema + Invoke + runner signature
  (`run(ctx, task, model, taskType, agentRef)`); profile resolution up front;
  soul/model/task-type/cost application; spawn payload `agent` field.
- No control-plane/CLI/webui changes (the tool is the surface).

## Tests (3 new, 4 cases, on a REAL kernel — journal+bus+loop, mock provider)

- `TestDelegate_AsNamedAgent`: child completion request carries profile model
  AND soul in System; spawn event payload has agent+model.
- `TestDelegate_ExplicitModelWinsOverProfile`: profile fills gaps only.
- `TestDelegate_UnknownOrPausedAgentRefused` (2 subtests): tool error, lead
  adapts, 0 subagent.spawned events.

## Gate

Full suite `-p 2 ./...` green; vet + staticcheck clean; linux cross-build OK;
go.mod unchanged. Isolated-daemon boot smoke with the extended schema: 0
panics, clean shutdown. CI org-billing still blocked → local battery +
arc-authority merge.

## Next in the arc

Agents console view (webui CRUD; routes live since M783) · memory_scope/
workdir wiring into runs · Fallbacks → per-agent routing chain · A2A
ask/reply on the board.
