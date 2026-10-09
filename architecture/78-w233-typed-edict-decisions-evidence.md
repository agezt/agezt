# W2.33c typed edict decision reads

The two policy-decision reads now run on the shared dispatcher in
`kernel/app/edict`. They show what the rules decided, not what the rules are:

- `edict_log` lists `policy.decision` records newest first;
- `edict_stats` aggregates them.

`Decisions` works over a `journalview.Reader` and a clock, both bound to the
kernel the dispatcher routed to. `DecisionOperations` declares two read-only,
unaudited, tenant-owned and caller-tenant-routed specs with unknown input
allowed:

- `edict_log` on `GET /api/policy_log`;
- `edict_stats` on `GET /api/policy`.

Both routes match the Web UI read routes.

This removes the native `policy_log.go` and `projections.go`. The map-row
`projectJournal` wrapper had no other user, so every journal-folded log is now a
typed `journalview.ProjectValues` read. `sinceCutoff` moved beside the native
approvals folds that still use it. `argStrings` lost its last caller and is
removed; the harness carries the pre-slice copy. Every edict operation is now typed except
`edict_overlay` and `edict_compact`.

## Preserved behavior

- `edict_log` checks its arguments in this order:
  1. `denied`, a strict boolean (`null` included);
  2. `tool`, then `capability`, as strict strings;
  3. the lenient page: limit 20 unless a JSON number, clamped to 1..1,000; a
     truncated `since_ms` on the daemon clock; the opaque cursor.
- `edict_log` rows carry actor, correlation id, tool, capability, allow,
  reason, hard-denied, timestamp and sequence.
- `edict_stats` reads `since_ms` leniently. It echoes `window_ms` (negative
  values included) and treats a non-positive window as all-time. It then checks
  the strict tool and capability filters.
- `edict_stats` reports total, allowed, denied, hard-denied, the denial rate and
  denials by capability. A denial with no recorded capability counts as
  `unknown`. An empty map is `{}`.
- A missing or malformed payload, including a single mistyped field, zeroes the
  whole decision. That decision then counts as a denial with no tool or
  capability.
- Neither read has a strict tenant argument. Routing follows the trimmed caller
  tenant.

The only intended difference is the shared already-canceled admission, which
rejects an authorized read before any work.

## Runnable comparison and regression evidence

The harness gives every cloned primary and `acme` journal thirty policy
decisions:

- allowed, denied and hard-denied;
- blank tool and capability;
- same-millisecond pairs;
- six malformed or unusual payloads (a mistyped field, an array, a string, `{}`,
  `null` and an extra key).

It runs the pre-slice handlers and `projectJournal` on one copy and the
registered operations on another.

306 steps repeated twenty times cover nine sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- unfiltered log and stats;
- the denied, tool and capability filters, alone and combined;
- cursor paging and every limit shape;
- windows in both reads;
- every argument error in order, plus invalid cursors;
- tenant-routed, padded, foreign, `null`, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact, and neither side journals anything. The harness
asserts three things:

- hard denials, reasons, `unknown` and capability buckets were read;
- the denied filter dropped allowed rows;
- the cursor paged.

The only exceptions are 53 canceled authorized steps, which return the
admission error.

Permanent tests cover:

- newest-first rows;
- whole-decision zeroing;
- every filter, the truncated window on an injected clock, every limit shape
  and cursor paging;
- argument error order;
- the empty wires;
- journal read errors;
- the stats fold, rate, buckets and window echo;
- the specs and output schemas.

Native tests cover the registry flags, plus routing: each token sees only its
own kernel's decisions, the strict `denied` error, and a window on the daemon
clock.

Thirty-seven independent mutations fail tests. These include the native
registration, the routed-kernel binding and the daemon-clock binding. Mutation
testing found one gap, now closed: no fixture held more decisions than the
default page or the clamp. Sources are restored byte-for-byte. Fixtures use
isolated temporary kernels only.
