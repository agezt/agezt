# Agent Loop Invariants

These invariants protect the observable AGEZT loop while we borrow more harness
discipline from DeerFlow. They are deliberately small and testable.

## Tool And Policy Order

1. The model may request a tool.
2. `policy.decision` must be emitted before the tool is invoked.
3. A denied tool must not call the underlying implementation.
4. `tool.result` must be emitted after invocation or denial, using the same
   `call_id` as the request.
5. Observation trust metadata must travel with the result, not as a separate
   uncorrelated note.

## Context Compaction

1. Compaction happens between completed observations and the next provider call.
2. A compaction event reports before/after size, elided tool-output count, and
   reclaimed characters.
3. Outputs marked with `agent.DefaultContextRescueMarker` remain available until
   the next prompt assembly needs them.
4. Skill rescue metrics are advisory counters; they must never hide that normal
   tool output was elided.

## Deferred Tool Discovery

1. `tool_search` is read-only capability lookup over the current run's allowed
   tool set.
2. It must not reveal tools hidden by agent policy.
3. It must not bypass the normal policy gate for the selected tool.
4. It is visible only as the pinned catalog tool when schema exposure is capped.

## Sub-Agent Delegation

1. `subagent.spawned` is journaled before a child run executes.
2. Async children report `subagent.completed` under the parent correlation.
3. `delegate_await` releases the result only after completion is journaled.
4. Depth, fan-out, total-tree, and spend guards apply before spawn, for both sync
   and async delegation.

## Config Reload Boundaries

1. A field marked `live` must have a real hot-apply path.
2. Registered skill/plugin config defaults to `restart` unless the owner provides
   an explicit hot-reload implementation.
3. `config_schema.reload_boundaries` is the UI and client contract for what
   applies live versus after restart.
