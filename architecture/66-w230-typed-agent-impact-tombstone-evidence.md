# W2.30p typed agent_impact and agent_tombstone

The teardown preview and the agent tombstone now run on the shared dispatcher.
`approster.ImpactService` builds both reads over an `ImpactSource` with three
methods: roster lookup, the sub-agent tree, and one agent's per-subsystem
holdings. `ImpactOperations` declares two primary-only read-only specs:
`agent_impact` (`GET /api/agents/impact`, matching the Web UI read route) and
`agent_tombstone` (no HTTP route, as before), both with unknown input allowed.

Both outputs are now typed:

- `ImpactOutput` has the thirty-nine irregular wire fields. The console reads them
  verbatim, and the existing key-pinning test still guards them.
- `TombstoneOutput` holds the identity, the lifecycle and retirement record, the
  footprint and the retained-by-design counts.

The sub-agent labels (`SubagentImpactLabels`) and the prefixed, sorted sub-agent
aggregation (`AggregateSubagentLabels`) moved into app/roster. The native
`cascadeSubsystems` table becomes `nativeImpactSource.Holdings`. The two remaining
native users go through the app:

- `agentImpactResult` converts the typed summary into the generic object that
  retire extends with its paused counts;
- `subagentImpact` lists each child for removal's retained workflow references.

The native impact and tombstone handlers, their manual rows,
`roster_tombstone.go` and `agentSubagentImpact` are removed. Both native wire
entries are `AppOwned` and `ReadOnly`, not tenant-allowed or routed. A doc comment
that had drifted onto the impact handler moved back to the status accumulator it
describes.

## Preserved behavior

`ref` is required and must be a string, and an unknown agent is reported. The
preview reports, for each of nine subsystems, the agent's own sorted labels and
count. It also reports the union of its sub-agents' labels, each prefixed with the
child's slug and sorted, with its count. A subsystem holding nothing stays `null`
on the wire; the sub-agent list is always an array. Sub-agent labels:

- name the child, or show its name followed by the slug;
- mark its owner, parent or descendant relation;
- flag a retired child.

The tombstone is unchanged: it trims the manager (parent before owner), lifecycle
mode, memory scope and model, and takes its footprint and retained counts from the
same preview. Retire's `impact` summary and its journaled `impact_summary` carry
the same members and values as before. The only intended difference is the shared
already-canceled admission, which now rejects the two reads.

## Runnable comparison and regression evidence

The cloned-kernel harness seeds a base kernel with a three-level sub-agent tree
holding something in every subsystem:

- standing orders and schedules;
- private, scope-less and authored shared memories;
- skills and agent configs;
- workspace files;
- a workflow naming two agents;
- mailbox messages on a shared board.

It runs the pre-slice handlers, cascade table and helpers on one copy and the
registered operations on another. Retire is still native, so its pre-slice
handler, with the old impact builder, is compared with the current one, which
builds the summary through the app.

120 steps repeated twenty times cover thirteen sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- previews and tombstones at each level of the tree and for a lone agent;
- every argument error;
- a padded ref;
- retiring a leaf and then reading its parent's preview and tombstones;
- retiring the root;
- retiring an unknown agent.

Decoded responses, normalized raw bytes and full journal deltas, including retire's
`impact_summary`, are equal. Only times, durations, correlation ids and generated
ids are normalized. The harness also asserts that the fixture really populated the
sub-agent aggregation. The 17 canceled typed reads return the admission error;
retire still ignores the context on both sides.

Permanent tests cover:

- the sub-agent labels and the aggregation;
- that every subsystem pairs with exactly its own four fields, using distinct
  labels per subsystem;
- one holdings read per agent in the tree, null versus empty arrays, and errors;
- the tombstone's fields, its manager fallback, and distinct per-subsystem counts
  reaching the right footprint fields;
- both specs, non-primary denial and canceled admission;
- the native registry entries and native wire bytes on a live kernel;
- the native sub-agent aggregation used by removal.

Thirty-six independent mutations fail tests. The first run showed that nothing
covered removal's sub-agent workflow aggregation, so a test was added and the
suite rerun from the start. Sources are restored byte-for-byte. Fixtures use
isolated temporary kernels only.
