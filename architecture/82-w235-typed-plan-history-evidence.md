# W2.35a typed plan history

The two plan-execution reads now run on the shared dispatcher in
`kernel/app/runs`, beside the run reads they mirror:

- `plan_history` lists plan executions;
- `plan_stats` aggregates them.

A plan run journals `plan.started` and a terminal `plan.completed` or
`plan.failed` under one plan correlation.

`Plans` works over a `journalview.Reader` bound to the kernel the dispatcher
routed to. `PlanOperations` declares two read-only, unaudited, tenant-owned and
caller-tenant-routed specs with unknown input allowed:

- `plan_history` on `GET /api/plan_history`, matching the Web UI read route;
- `plan_stats`, with no route, as before.

Removed with the move:

- the native `plan_history.go`;
- its registrations;
- the now-unused native `defaultRunsLimit` and `maxRunsLimit`.

No native journal-folded audit log remains in the control plane.

## Preserved behavior

- `plan_history` reads its arguments in this order:
  1. the lenient limit: 20 unless a JSON number, clamped to 1..1,000;
  2. the lenient cursor: a non-string or malformed cursor is ignored;
  3. the strict `status` string.
- Plans fold by correlation:
  - a plan with no terminal event is `running`;
  - the start supplies the name and node count;
  - a terminal event names the plan only when the start did not;
  - a malformed payload reads as zero.
- Rows are filtered by status and cut at the cursor. They are sorted newest
  start first, then by start sequence, and then limited. A plan whose start was
  never journaled sorts as the oldest.
- A duration is reported only when the plan has both a start and an end, and
  the end is not before the start.
- The next cursor is advertised only for a full page.
- `plan_stats` reports total, completed, failed, running and terminal counts,
  the success rate over terminal plans, and the duration distribution (count,
  average, minimum, maximum, p50 and p95) over the plans that have a duration.
- Neither read has a strict tenant argument. Routing follows the trimmed caller
  tenant.

## Intended differences

- The shared already-canceled admission rejects an authorized read before any
  work.
- Several start-less plans used to share the zero start and the zero sequence.
  `sort.Slice` then left their relative order arbitrary from call to call. They
  now break that tie by correlation id. Every order the old code could produce
  is still a valid one, but the new order is stable.

## Runnable comparison and regression evidence

The harness gives every cloned primary and `acme` journal twenty plans:

- completed, failed and running plans;
- names carried only by the terminal event;
- a malformed start;
- exactly one start-less plan, so the old arbitrary tie cannot occur;
- same-millisecond starts.

It runs the pre-slice handlers and helpers on one copy and the registered
operations on another.

180 steps repeated twenty times cover seven sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- unfiltered list and stats;
- every status filter;
- cursor paging, including a filtered page past a cursor;
- every limit and cursor shape;
- the `status` errors;
- tenant-routed, padded, foreign, `null`, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact, and neither side journals anything. The harness
asserts three things:

- failed and running rows, terminal-only names, node counts, a fractional
  success rate and a non-empty duration distribution were read;
- the status filter applied;
- the cursor paged.

The only exceptions are 31 canceled authorized steps, which return the
admission error.

Permanent tests cover:

- the joined rows, including the terminal-name fallback, the start-less plan
  and an end before its start;
- every status, every limit shape, the default page and the clamp on 1,005
  plans;
- cursor paging and the same-millisecond order;
- the stable start-less tie, checked over twenty repeats;
- the `status` errors, the empty wires and journal errors;
- the stats fold and distribution;
- the specs and output schemas.

Native tests cover the registry flags, plus routing: each token sees only its
own kernel's plans, and the strict `status` error.

Thirty-five independent mutations fail tests, including the native registration
and the routed-kernel binding. Mutation testing found one gap, now closed: the
malformed start had no well-typed field, so a partial decode looked the same as
the zeroing it must do. Sources are restored byte-for-byte. Fixtures use
isolated temporary kernels only.
