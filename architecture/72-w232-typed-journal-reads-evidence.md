# W2.32a typed journal head, tail and stats

Three journal reads now run on the shared dispatcher:

- `journal_head`: the head checkpoint;
- `journal_tail`: the last N events;
- `journal_stats`: the journal's size and shape.

The new package `kernel/app/journal` owns the head clamp, the tail codec and
clamp, and the statistics fold. They work over a `Journal` port (`Head`, `Tail`,
`Range`) and a best-effort `Disk` measure. `journalReads` binds both to the routed
kernel. The segment count and directory size stay the native helpers
(`countSegments`, the shared `dirSize`).

`Operations` declares three read-only, unaudited, primary-only specs with unknown
input allowed:

- head and tail with `Primary` tenancy, so they read the primary journal whatever
  tenant is named;
- stats with `CallerTenant` tenancy, so it follows an operator-named tenant. This
  derives the legacy wire exactly: `TenantRouted` without `TenantAllowed`.

None of the three has an HTTP route, as before.

The tail output schema is derived from a wire mirror of `event.Event` whose raw
payload is any JSON value. The legacy handler embedded the journal's Event
structs, so the adapter writes `events` from the typed output directly, keeping
the struct member order. `journal.go` (both handlers and their constants) and the
three registrations are removed.

## Preserved behavior

- `head` reports 0 for an empty journal, and its hash.
- `n` is a JSON number truncated toward zero. Anything else means 20, and the
  value is clamped to between 1 and 10,000.
- The head checkpoint is taken before the reverse segment read.
- An empty tail is `[]`, `count` is the number of events returned, and read
  errors are returned.
- `stats` counts every event, groups them by kind, and spans the stamped
  timestamps. It then measures segments and bytes, and a failed fold measures
  nothing. `by_kind` is an object, never null.
- A tenant token is refused for all three.

The only intended difference is the shared already-canceled admission, which
rejects a primary read before any work.

## Runnable comparison and regression evidence

The W2.31c harness journals a varied history into the primary kernel (40 runs)
and the `acme` tenant kernel (25 runs). It then runs the pre-slice handlers and
constants on one copy and the registered operations on another.

132 steps repeated twenty times cover five sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- full reads;
- every `n` form and clamp;
- tenant-named head, tail and stats, padded included;
- foreign, non-string and invalid tenants;
- secret-named and unknown arguments.

Responses are byte-exact, including the event member order and the on-disk
measures, and neither side journals anything. The harness asserts that the
populated journal was read: 20 events, and 40 `task.received` events. The only
exceptions are 21 canceled primary steps, which return the admission error.

Permanent tests cover:

- the head clamp and its wire;
- the tail codec and clamp, the head-before-read order, the empty wire and read
  errors;
- the stats fold, its time span and its empty wire, and no disk measure after a
  failed fold;
- the specs, and the output schemas against real outputs, including the tail
  schema against object, array, string and absent payloads;
- the native registry entries and their exact tenant flags;
- a native routing test: head and tail read the primary journal under a tenant
  argument and keep the event member order, stats follows the named (padded)
  tenant, and a tenant token is forbidden for all three.

The existing native journal suites still pass through the new path.

Twenty-six independent mutations fail tests. They include the native
registration, the event member order, the routed-kernel binding and both disk
port fields. One equivalent was recorded: counting zero timestamps, which cannot
move the unset-oldest sentinel or the maximum. Sources are restored byte-for-byte.
Fixtures use isolated temporary kernels only.
