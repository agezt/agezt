# W2.30n typed agent_repair

The operator's Repair action now runs on the shared dispatcher.
`approster.RepairService` works over the same four native ports as the wake:

- roster lookup;
- the kernel's correlation source;
- the operator-action publisher;
- a non-blocking launch.

`RepairOperations` declares a primary-only spec that is not read-only
(`POST /api/agents/repair`, matching the Web UI write route), with unknown input
allowed. Shared dispatch therefore requires a successful operation audit admission
before the repair is journaled or started.

The governed repair stays native. The former handler goroutine is now
`runAgentRepair`, unchanged. It keeps:

- its WF-001 panic firewall;
- the overseer repair source;
- the `failed`/`completed` events.

The launch adapter starts it on its own goroutine. The native handler and its
manual `commandSpec` row are removed. The native wire entry is `AppOwned`, not
`ReadOnly`, not tenant-allowed or routed.

Two things are shared with the wake:

- `directTarget` performs the check chain both actions use (ref, unknown, retired,
  paused, managed, with the action word in the managed refusal);
- the incident lineage type is renamed `IncidentLineage` and read by one lenient
  helper.

## Preserved behavior

`ref` is required and must be a string. The repair rejects an unknown, retired,
paused or managed agent in that order; the managed refusal says `cannot be repaired
directly`. `reason` and the three incident ids are read leniently and trimmed;
other arguments, such as an `intent`, are ignored. On acceptance the service:

- takes a new correlation;
- journals the `agent.repair` `requested` event with the reason and lineage;
- launches the repair;
- returns `{accepted, agent, correlation_id}`, with the resolved slug.

The repair run, its events and its panic containment are unchanged. The only
intended difference is the shared already-canceled admission, which now rejects
before audit. The legacy path ignored the context and started the repair anyway.

## Runnable comparison and regression evidence

The cloned-kernel harness from W2.30m runs the pre-slice handler on one copy of a
closed base kernel and the registered operation on another. Each copy has its own
scripted two-answer mock provider. The harness waits for every accepted repair's
terminal event.

It compares the journal grouped by correlation id, which covers the request audit,
the repair request, the whole asynchronous governed repair run, its terminal event
and the lifecycle roster events. It also compares raw response bytes and provider
call counts.

120 steps repeated twenty times cover sixteen sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- lineage, lenient and ignored arguments;
- secret-named arguments, for redaction;
- padded and case-different refs;
- every rejection;
- a single-cycle agent;
- consecutive repairs that exhaust the provider;
- a retry-policy agent.

All responses and journals are equal except the 20 primary-token canceled steps,
which return the canceled admission error with no audit, no journal events and no
provider call.

Permanent tests cover:

- every rejection without a correlation, publish or launch, including the
  `repaired` action word;
- the journal-then-launch order and the requested payload;
- lenient arguments and the resolved slug;
- spec metadata, and that a failed audit admission blocks the repair;
- non-primary and canceled admission, and audited success and failure records;
- the native registry entry;
- an end-to-end native repair. Its provider blocks until the response is read, so
  a synchronous launch fails. It also checks that the reason and the whole lineage
  reach the terminal event.

A new test proves the WF-001 panic firewall: `runAgentRepair` on a server with no
kernel panics inside the repair source, and the test passes only because the panic
is recovered. With the recover removed, it fails.

Thirty independent mutations fail tests:

- the repair service and spec;
- each step of the shared `directTarget` chain and the lineage helper, through both
  the wake and the repair;
- the native registration, the asynchronous launch and its arguments;
- the panic firewall and the terminal payload.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels with
scripted mock providers only.
