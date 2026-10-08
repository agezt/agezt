# W2.30i agent_resolve operation audit

`agent_resolve` pauses, retires, delegates or forces a routing chain for an agent
and publishes its own `agent.resolve` domain events, but its native registration
was marked `ReadOnly: true`. In the dispatch pipeline `ReadOnly` means "changes no
state", so the shared operation audit (`op.invoked` before the handler, then
`op.completed` or `op.failed`) was skipped for this command. The W2.1a audit change
that introduced the flag states the rule plainly — omitting the flag over-audits,
never under-audits — so marking a mutating command read-only is an under-audit: an
operator resolution left no operation record with caller, tenant and summarized
arguments, and a refused resolution left no failure record at all.

## Fix

The `ReadOnly` flag is removed from the `agent_resolve` command registration. The
handler, its arguments, its `agent.resolve` requested/completed/failed events and
its result are unchanged; dispatch now journals the operation around it like every
other state-changing native command.

## Evidence

`TestAgentResolve_IsOperationAudited` (native socket) resolves an agent as paused
and then submits an invalid resolution, and requires exactly `op.invoked` followed
by `op.completed`, respectively `op.failed`, sharing one correlation id, with the
operation name and operator caller in the invocation payload. On the unchanged
registration it failed three times out of three ("operation did not publish terminal
audit"); with the flag removed it passes twenty times out of twenty, together with
the existing resolve and dispatch-audit suites. Restoring the flag is exactly the
recorded red state.

A scan of every remaining `ReadOnly: true` native registration for handler bodies
that publish operator actions, apply resolutions or call store mutators found no
other mutating command; the three other textual hits were time arithmetic and
profile field reads. Typed binding of `agent_resolve` itself remains with the roster
lifecycle writes. Fixtures use isolated temporary kernels and do not touch the
owner's home, wake agents, run a provider or send channel messages.
