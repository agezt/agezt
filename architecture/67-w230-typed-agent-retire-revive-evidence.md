# W2.30q typed agent_retire and agent_revive

Moving an agent to and from the graveyard now runs on the shared dispatcher.
`approster.SetRetiredService` works over `SetRetiredPorts`:

- roster lookup;
- the impact summary from the W2.30p `ImpactService`;
- the journaled set-retired;
- trigger pausing and paused counts;
- the correlation source;
- the operator-action publisher;
- list cache invalidation.

`SetRetiredOperations` declares two primary-only specs that are not read-only
(`POST /api/agents/retire` and `POST /api/agents/revive`, matching the Web UI
write routes), with unknown input allowed. Shared dispatch therefore requires a
successful operation audit admission before either write.

The retire output is typed. `RetireImpactSummary` embeds the typed preview and
adds the paused counts. The journaled `impact_summary` stays the generic object the
event has always carried. The native shared set-retired handler, its two wrappers
and manual rows, and the native `agentImpactResult` wrapper are removed. Both
native wire entries are `AppOwned`, not `ReadOnly`, not tenant-allowed or routed.

## Preserved behavior

`ref` is required and must be a string. The reason is read leniently and trimmed,
and passed to both writes. A retirement:

- previews the impact before the change, and reports the standing-order labels as
  `impact` and the whole preview as `impact_summary`;
- pauses the agent's standing orders and schedules after the change. A pause error
  is returned without a journal entry or cache invalidation;
- adds the paused counts to the summary, the response and the `agent.retire` event,
  together with the stored reason and retirement time.

A revival first re-checks the agent's hierarchy references when the agent exists.
It then reports, and journals as `agent.revive`, how many triggers remain paused.
Both map a missing agent to `unknown agent: <ref>`, pass other errors through,
return the legacy profile view and invalidate the list cache after success. If the
lookup misses before a retirement, the impact and summary stay `null`, as before.
The only intended difference is the shared already-canceled admission, which now
rejects before audit and before any write.

## Runnable comparison and regression evidence

The cloned-kernel harness reuses the W2.30p base kernel: a three-level sub-agent
tree holding standing orders, schedules, memories, skills, configs, workspace
files, workflow references and mailbox messages. It runs the pre-slice handler and
impact wrapper on one copy and the registered operations on another, and compares
decoded and normalized raw responses and the full journal deltas, including the op
audit, roster events, the retire `impact_summary` and the revive counts.

102 steps repeated twenty times cover twelve sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- retire then revive at each level of the tree;
- a revival refused because the owner is retired;
- a repeated retirement;
- lenient and secret-named arguments;
- unknown agents;
- every ref error;
- a padded ref.

The harness also asserts that the first retirement really paused a trigger and
previewed the sub-agent tree. All responses and journals are equal except the 17
primary-token canceled steps, which return the admission error with an empty
shared journal.

Permanent tests cover:

- the retire step order (preview, write, pause, journal, invalidate), its outputs
  and journal payload, and the lenient reason;
- the null preview after a missed lookup, and ref, not-found, pass-through and
  pause errors with no journal entry;
- the revive order with the hierarchy re-check and the counts;
- both specs, audit admission blocking either write, non-primary and canceled
  admission, and audited records;
- the native registry entries;
- the native ports end to end, with one standing order and two schedules so the
  pause and count ports cannot be swapped unnoticed.

Twenty-nine independent mutations fail tests. The first run showed equal counts in
the native fixture hid a swapped count port; the fixture was made distinct and the
suite rerun from the start. The skipped native cache invalidation is the recorded
equivalent mutant, as in W2.30j: the list cache key already covers the profile's
`updated_ms`, which every retire and revive changes. Sources are restored
byte-for-byte. Fixtures use isolated temporary kernels only.
