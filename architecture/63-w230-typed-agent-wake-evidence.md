# W2.30m typed agent_wake

The operator's manual agent wake now runs on the shared dispatcher.
`approster.WakeService` works over four native ports:

- roster lookup;
- the kernel's correlation source;
- the operator-action publisher;
- a non-blocking launch.

`WakeOperations` declares a primary-only spec that is not read-only
(`POST /api/agents/wake`, matching the Web UI write route), with unknown input
allowed. Shared dispatch therefore requires a successful operation audit admission
before the wake is journaled or launched.

The run itself stays native. The launch adapter starts the existing
`runAgentWake` in a goroutine, so the agent profile, wake context, cost cap and
retry policy are untouched. The pure parts move to app/roster:

- the default wake intent becomes `BuildOperatorWakeIntent`, which now takes the
  lineage instead of raw arguments;
- the managed sub-agent refusal becomes `ManagedDirectCallError`. The native
  `managedSubagentDirectCallError` wraps it for the repair, run and workboard
  callers.

The native handler, the native `buildOperatorWakeIntent` and the manual
`commandSpec` row are removed. The native wire entry is `AppOwned`, not
`ReadOnly`, not tenant-allowed or routed.

## Preserved behavior

`ref` is required and must be a string. Then, in order, the wake rejects:

- an unknown agent;
- a retired agent (`revive it first`);
- a paused agent;
- a managed sub-agent, with the parent-then-owner hint.

Only after those checks must `intent` be a string. That order is unchanged, so a
typed intent on an unknown agent still reports the unknown agent. `reason` and the
three incident ids are read leniently: anything but a string is empty, and values
are trimmed. A blank intent falls back to the manual wake-up prompt, with
reason/root/hop lines. The `agent wake requires args.intent or args.reason` guard
is kept, although the fallback prompt is never empty, so it cannot fire. Its
removal is the one recorded equivalent mutant.

On acceptance the service:

- takes a new correlation;
- journals the `agent.wake` `requested` event, with the intent truncated to 240
  characters, the autonomy runbook and the lineage;
- launches the run with the full intent;
- returns `{accepted, agent, correlation_id}`, with the resolved slug.

The run's own events and its `completed`/`failed` event are unchanged. The only
intended difference is the shared already-canceled admission, which now rejects
before audit and before any wake. The legacy path ignored the context, and woke the
agent anyway.

## Runnable comparison and regression evidence

The cloned-kernel harness runs the pre-slice handler, intent builder and managed
error on one copy of a closed base kernel and the registered operation on another.
Each kernel has its own scripted mock provider with two answers, so later wakes
exhaust it and fail. After every accepted wake, the harness waits for that run's
terminal event.

The journal is compared grouped by raw correlation id, in order of first
appearance, with uncorrelated events last. That covers the request's audit, the
wake request, the whole asynchronous run and its terminal event, and the lifecycle
roster events. The async run may interleave with the audit tail, but each group's
own order is deterministic. Only times, durations, correlation ids and generated
ids are normalized, including the run id in subjects, actors and payload strings.
Raw response bytes are compared after the same normalization, and the provider call
counts must match.

138 steps repeated twenty times cover nineteen sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- lineage, lenient reasons and incident ids, explicit/long/blank intents;
- secret-named arguments, for redaction;
- padded and case-different refs;
- every rejection;
- a single-cycle agent that retires itself after the wake;
- consecutive wakes that exhaust the provider;
- a retry-policy agent.

All responses and journals are equal except the 23 primary-token canceled steps,
which return the canceled admission error with no audit, no journal events and no
provider call.

Permanent tests cover:

- every rejection without a correlation, publish or launch;
- the journal-then-launch order and the requested payload, including truncation
  and the runbook;
- the full launched intent, the lenient arguments, the resolved slug and the
  default intent text;
- spec metadata, and that a failed audit admission blocks the wake;
- non-primary and canceled admission, and audited success and failure records;
- the native registry entry;
- an end-to-end native wake. Its provider blocks until the response is read, so a
  synchronous launch fails. It also checks that the whole lineage reaches the
  terminal event.

Thirty-six independent mutations fail tests, including the native registration,
the asynchronous launch, the lineage mapping, the publisher and the managed-error
wrapper. Sources are restored byte-for-byte. Fixtures use isolated temporary
kernels with scripted mock providers only. They do not touch the owner's home,
agents or real providers, and they send no channel messages.
