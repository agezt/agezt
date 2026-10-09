# W2.37b typed why

`why` now runs on the shared dispatcher in `kernel/app/journal`, beside the
journal reads. For one event it returns:

- its correlation chain;
- the sub-agent parent's correlation;
- the causation chain, which may cross correlations.

`Trace` works over a `Tracer` port (`Why`, `ParentOf`, `Causes`) bound to the
routed kernel. `TraceOperations` declares one spec: read-only and unaudited,
with no route and unknown input allowed. It is OwnTenant and CallerTenant, so a
tenant token traces only its own journal and the operator can name a tenant.
These are the native `TenantAllowed`/`TenantRouted` flags. The output schema
comes from a wire mirror of `event.Event`, and the adapter keeps both event
lists in struct member order, as the native handler emitted them.

Removed with the move:

- the native `handleWhy`;
- its registration.

`server_commands.go` now holds only `handleWhoami`. `whoami` stays native by
design. It echoes the transport principal, and typing it would require
CallerTenant routing, which would resolve the tenant it only names.

## Preserved behavior

- `event_id` is read leniently: absent, empty, `null` or a non-string all
  return `args.event_id required`. A present string passes untrimmed, so a
  padded id is an unknown event.
- The kernel's error passes through unchanged, for example for an unknown event.
- `correlation` is the first event's correlation. `parent_correlation` is looked
  up only when that correlation is set.
- `causation_chain` is best-effort. It is reported only when it links more than
  the event itself, and a failed walk never fails the trace.
- Both lists are always arrays, never `null`.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before any journal read.

## Runnable comparison and regression evidence

The harness clones fixture kernels. Each has a primary journal and an open
`acme` journal, with:

- run histories and delegation trees, so a child run has a parent correlation;
- a three-event causation chain that crosses from a tick correlation into an
  initiative correlation.

It runs the pre-slice handler on one copy and the registered operation on
another.

120 steps repeated twenty times cover six sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- the chain's head, root and middle, plus a plain run event;
- a child-run event;
- every missing or malformed id, an unknown id and a padded id;
- tenant traces through `acme` (operator-named, padded and token-routed), plus a
  primary event looked up in the tenant journal;
- unknown, invalid, non-string and null tenants;
- secret-named arguments.

Responses are byte-exact, with events in struct member order. Neither kernel's
journal changes. The harness asserts that:

- the head's causation chain starts at the tick in the other correlation;
- the root reports no chain;
- the plain event returns its run;
- a tenant trace resolves in `acme`.

The exceptions are 22 canceled steps for authorized tokens. These return the
admission error.

Permanent tests cover:

- every missing-id form, with no tracer call;
- the untrimmed id;
- the full trace;
- the one-event and failed causation walks;
- an unknown event with no parent lookup;
- error passthrough;
- the spec and output schema.

The native routing test now also checks, on the daemon:

- a causation chain across correlations, with both event lists in member order;
- a tenant tracing its own event, but never a primary one.

The native registry test checks the wire flags. The existing `why` suites pass
unchanged through the typed path. These cover the correlation walk, the
cross-correlation chain, the sub-agent parent and tenant scoping.

Sixteen independent mutations fail tests, among them the native registration,
the routing and the member-order adapter case. That last one survived at first,
and the native member-order check was added for it. One equivalent is recorded:
reading the correlation from another index. The kernel returns one
correlation's events, so every index names the same chain.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
