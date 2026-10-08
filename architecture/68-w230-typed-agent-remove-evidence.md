# W2.30r typed agent_remove — roster domain complete

Removing an agent now runs on the shared dispatcher, and with it every command of
the native roster group is typed. `approster.RemoveService` owns the whole removal
order and its report, over `RemovePorts`:

- roster lookup;
- the sub-agent tree;
- the retained mailbox and workflow labels;
- the per-subsystem cascade effects: retire sub-agents, standing orders,
  schedules, memory, authored shared memory, skills, config, workspace;
- the journaled profile removal;
- the correlation source and the operator-action publisher;
- list cache invalidation.

`RemoveOperations` declares a primary-only spec that is not read-only
(`POST /api/agents/remove`, matching the Web UI write route), with unknown input
allowed. Shared dispatch therefore requires a successful operation audit admission
before the cascade starts.

The cascade parser (`ParseRemoveCascade`, with its lenient flags and the
`authored_shared_memory`/`workdir` aliases) and the typed `RemoveOutput` live in
app/roster. `RemoveOutput` embeds a nilable `*RemoveReport`, so an unknown agent
still reports only `{"removed": false}`.

The native removal handler, the empty `roster_lifecycle.go`, the now-empty
`registerRosterCommands` with its registry call, the native `subagentImpact`
wrapper, `boolish` and an orphaned comment are removed. The native cascade effect
helpers stay as the ports. The native wire entry is `AppOwned`, not `ReadOnly`, not
tenant-allowed or routed.

## Preserved behavior

`ref` is required and must be a string. An unknown agent returns only
`removed: false`, with no cascade and no invalidation. A system agent is refused.
An agent with sub-agents is refused unless `cascade.subagents` is set, and the
refusal names the count. The removal then runs in the legacy order:

1. the retained mailbox labels (deduplicated across the tree when sub-agents
   cascade), the agent's workflow labels and the prefixed sub-agent workflow
   labels;
2. retiring the sub-agents;
3. standing orders and schedules for the agent, then for each child;
4. memory, authored shared memory, skills, config (deleted and access-pruned) and
   workspace for the agent, then for each child;
5. the profile removal.

The first failure stops the cascade and returns its error without a journal entry
or invalidation. A completed removal journals `agent.remove` with the cascade view
and the full report. A profile that vanished meanwhile reports `removed: false`
with the report and is not journaled. Both invalidate the list cache. The only
intended difference is the shared already-canceled admission, which now rejects
before audit and before any cleanup.

## Runnable comparison and regression evidence

The cloned-kernel harness reuses the W2.30p/W2.30q base kernel: a three-level
sub-agent tree holding standing orders, schedules, memories, skills, configs,
workspace files, workflow references and mailbox messages, plus a system agent.
It runs the pre-slice handler, cascade parser, view, `subagentImpact` and
`boolish` on one copy and the registered operation on another. Because a removal
deletes across subsystems, the harness compares three things:

- decoded and normalized raw responses;
- the full journal delta;
- a post-run snapshot of the roster, standing orders, schedules, active memory,
  skills, config entries and workspace files. Timestamps and ids are blanked, and
  each subsystem is compared as a sorted multiset, because the config store lists
  from a map.

84 steps repeated twenty times cover thirteen sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- a refused removal of a parent;
- the full cascade with lenient flags;
- a sub-agents-only cascade;
- a middle node and a leaf with aliased flags;
- removing the same agent twice;
- non-object cascades and secret-named arguments;
- the system agent;
- an unknown agent;
- every ref error;
- a padded ref.

The harness also asserts that the full cascade really retired two sub-agents,
forgot memories and changed the state. All responses, journals and snapshots are
equal except the 14 primary-token canceled steps. Those return the admission error
with an empty journal and an unchanged snapshot.

Permanent tests cover:

- the cascade parser with its flag values and aliases;
- every rejection before any effect, and the exact `{"removed":false}` wire;
- the full step order with sub-agents and the summed report;
- the journal payload and the seventeen-key wire;
- each flag reaching only its own port, and null labels for a lone agent;
- the cascade stopping at each of sixteen failure points without journaling;
- a vanished profile;
- the spec, audit admission blocking the removal, non-primary and canceled
  admission, and audited records;
- the native registry entry. The existing native removal suites still run end to
  end through the new path.

Thirty-six independent mutations fail tests, including the native registration and
six native port bindings. Sources are restored byte-for-byte. Fixtures use isolated
temporary kernels only.
