# W2.39d typed reaper scan

`reaper_scan` (M903) now runs on the shared dispatcher in the new
`kernel/app/reaper` package. It is the read-only detail behind the pulse reaper
observer. It finds dead, degraded, misconfigured and routing-troubled agents
and stale artifacts, and the operator still retires or collects. `agt doctor`
and the console's agent page call it.

`Service` works over two ports bound to the primary kernel:

- the kernel's `ReaperScan`, which takes absolute agent-idle and
  artifact-stale cutoffs;
- the daemon clock.

`Operations` declares one primary-only, primary-tenancy spec, read-only and
unaudited, with unknown input allowed, on `GET /api/reaper/scan`. That matches
the native registration and the Web UI read route.

Removed with the move:

- `reaper.go`, which held `handleReaperScan` and its `intArg` helper;
- its registration.

## Preserved behavior

- `idle_days` and `stale_days` are read leniently. A JSON number truncates
  toward zero. Any other value, or a result below one, is the 30-day default.
  Both cutoffs are measured back from the daemon clock, and both windows are
  echoed.
- Every finding family is a list, even when empty, and has a matching count:
  - dead agents;
  - degraded agents;
  - misconfigured agents;
  - retry-pressure agents;
  - routing-pressure agents;
  - forced-chain probation, failed and exhausted agents;
  - routing-unstable agents.
- Each row carries all of its fields, zero values included. Unset issue and
  chain lists stay `null`, as the native maps emitted them. Stale artifacts and
  stale bytes are reported unchanged.
- Any `tenant` argument is ignored, and tenant tokens are refused. The scan
  judges the primary roster and journal.

The only intended difference is the shared already-canceled admission. It
rejects an authorized call before the scan.

## Runnable comparison and regression evidence

The harness clones a fixture kernel whose roster and journal raise every
finding family that plain journal events can produce:

- agents created ninety days ago: one never active, one paused, one recently
  active;
- a degraded agent with two failed runs under its health policy, with an HTML
  failure reason;
- a retry-pressure agent with three retries;
- a misconfigured agent with invalid runtime overrides;
- an agent owned by a missing agent;
- a routing-pressure agent with three model fallbacks;
- an ancient artifact and a fresh one.

It runs the pre-slice handler on one copy and the registered operation on
another.

60 steps repeated twenty times cover three sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- the defaults, with and without arguments;
- a 60/1-day window;
- a 120-day window;
- a fractional window;
- zero and negative windows;
- string, `null`, boolean and list windows;
- a named tenant and secret-named arguments.

Responses are byte-exact, and neither journal changes. The harness asserts:

- the dormant agent is dead under the default and 60-day windows, but not
  under 120 days;
- the paused and active agents never are;
- one degraded, one retry-pressure, two misconfigured and one routing-pressure
  agent;
- the stale artifact and its bytes;
- the escaped failure reason.

The exceptions are 10 canceled primary-token steps. These return the admission
error.

The forced-chain and routing-unstable families need routing-force journal
histories that the runtime reaper suite already builds. Here they are pinned by
the row-mapping tests below.

Permanent tests cover:

- every window form and both cutoffs on an injected clock;
- every row family, with each field;
- zero fields and `null` lists in the wire form;
- empty findings as empty lists;
- counts and 2^53+1 stale bytes;
- the spec and output schema.

These are the first native `reaper_scan` tests, since none existed before. They
check:

- the registry flags;
- that a ten-day-old agent sits within the default grace, but is dead under a
  seven-day window, judged on the primary roster only;
- that a tenant token is refused.

The existing `agt doctor` tests pass unchanged.

Twenty-four independent mutations fail tests. They cover:

- the default, number, floor and truncation rules;
- each cutoff and the clock;
- a field in every row family, and both forced-chain sources;
- the array shape and the counts;
- stale bytes and the echoed window;
- the route and the provider guard;
- the native registration and clock binding.

Sources are restored byte-for-byte. Fixtures use isolated temporary kernels
only.
