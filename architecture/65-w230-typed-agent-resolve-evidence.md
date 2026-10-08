# W2.30o typed agent_resolve

The operator's incident resolution now runs on the shared dispatcher.
`approster.ResolveService` holds the whole decision. It runs over a `ResolvePorts`
set of native effects:

- roster lookup;
- the correlation source;
- the operator-action publisher;
- pause and retire;
- the shared-board help request;
- the doctor's exhausted-chain and prior force-generation journal lookups;
- the overseer routing-chain update.

`ResolveOperations` declares a primary-only spec that is not read-only
(`POST /api/agents/resolve`, matching the Web UI write route), with unknown input
allowed. Shared dispatch therefore requires a successful operation audit admission
before the request is journaled or applied. This completes the audit fix of
W2.30i: the command is now a typed write rather than a native handler.

The pure logic that moved into app/roster:

- delegate-target validation;
- task-model-chain normalization and comparison;
- the requested, failed and completed payloads.

The native file `roster_wake.go`, which held only the resolve handler after the
wake moved, becomes `roster_resolve.go` with the two native ports that need the
server (`postOperatorHelp` and `applyRoutingChain`). The native helpers that only
resolve used are removed:

- `validateOperatorDelegateTarget`;
- `argListAny`;
- `normalizeTaskModelChain`;
- `equalStringSlices`;
- `operatorIncidentLineage`;
- `firstNonEmptyStrings`.

The journal lookups stay native. The native wire entry is `AppOwned`, not
`ReadOnly`, not tenant-allowed or routed.

## Preserved behavior

`ref` is required and must be a string, and an unknown agent is reported before
the resolution is checked. The resolution is read leniently and must be exactly
`paused`, `retired`, `delegated` or `force_chain`. The summary, delegate, task type
and incident ids are read leniently and trimmed.

The `requested` event names `delegate_to` and `routing_task_type` when they are
set, and `routing_task_model_chain` whenever the argument is a non-empty array.
That chain can be an empty list when no element is a usable model.

The resolutions behave as before:

- **paused** refuses a retired agent.
- **retired** defaults its reason.
- **delegated** refuses, in order: an empty target, the agent itself
  (case-insensitive), its parent-then-owner, a missing target, a retired target and
  a managed target. It then posts a help request with the summary or the default
  text, notifies board watchers, and reports the trimmed message id.
- **force_chain** requires a task type and at least one model. It refuses the chain
  the doctor marked exhausted for any id in the incident lineage, compared
  case-insensitively, reads the prior generation, applies the chain, and reports
  the generation, the chains and the task type. It prefers the overseer's values
  and falls back to the request.

A failure journals `failed` with the reason and returns the error. Success
journals `completed` with only the non-empty result fields and returns
`{applied, agent, resolution, correlation_id}`. The only intended difference is the
shared already-canceled admission, which now rejects before audit and before any
effect.

## Runnable comparison and regression evidence

The harness runs the pre-slice handler and all its removed helpers on one copy of
a closed base kernel and the registered operation on another. Each kernel has a
routing-provider double with a `code` chain and a shared board whose notifier
journals each post. The base journal is seeded with a prior force generation and a
doctor-exhausted chain.

The harness compares:

- decoded responses and normalized raw response bytes;
- the full ordered journal delta: op audit, requested/failed/completed events,
  roster events and board posts;
- the final routing chains.

Only times, durations, correlation ids and generated ids, including board message
ids, are normalized. 222 steps repeated twenty times cover thirty-four sequences
under primary, wrong and tenant tokens and normal and canceled contexts:

- pause and retire, including retired, paused and repeated cases;
- every delegation refusal and two successful posts;
- force chains that apply twice, match the current chain, hit the exhausted chain
  through the root or incident id or with no lineage, lack a task type or usable
  models, name an unknown task type, or target a paused agent;
- every argument rejection;
- secret-named arguments, for redaction;
- lenient non-string fields and padded refs.

The harness also asserts that the force-chain sequence really changed the routing,
and that the delegation sequence posted twice. Writing that assertion corrected a
wrong assumption in the harness, not in the code: roster lookup is case-sensitive,
so an uppercase delegate is "does not exist" on both sides. That case is now its
own sequence. All responses, journals and chains are equal except the 37
primary-token canceled steps. Those return the canceled admission error with no
audit, no journal events and unchanged routing.

Permanent tests cover:

- every rejection before the requested event;
- pause and retire, including the retired guard and the default reason;
- every delegation refusal, then the post, notify and message id;
- each force-chain refusal, the exhausted-chain lineage and case folding, and the
  generation arithmetic;
- the result-versus-request fallbacks, and that empty fields are omitted;
- the requested chain rules;
- spec metadata, and that a failed audit admission blocks the resolution;
- non-primary and canceled admission, and audited records;
- the native ports end to end: generation and exhaustion read from the journal
  (exhaustion matched through the parent id only), the overseer chain update, the
  missing board, the post with its notify, and the pause and retire ports.

Fifty-two independent mutations fail tests. The first run showed that no test
journaled a one-element chain, so that case was added and the suite rerun from the
start. Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
and test doubles only.
