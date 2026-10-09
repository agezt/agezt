# W2.34a typed approval history

The two human-in-the-loop history reads now run on the shared dispatcher, in the
new package `kernel/app/approvals`:

- `approvals_log` shows what was asked, how it resolved and who decided;
- `approvals_stats` aggregates the same history.

`History` works over a `journalview.Reader` and a clock, both bound to the kernel
the dispatcher routed to. `Operations` declares two read-only, unaudited,
tenant-owned and caller-tenant-routed specs with unknown input allowed:

- `approvals_log` on `GET /api/approvals_log`, matching the Web UI read route;
- `approvals_stats`, with no route, as before.

Removed with the move:

- the native `approvals_log.go`;
- its registration, along with the now-empty `registerJournalLogCommands`;
- `sinceCutoff` and `int64Arg`, whose last users were these folds.

Every journal-folded audit log in the control plane is now a typed operation,
except `plan_history`/`plan_stats` (`plan_history.go`). This slice also corrects
the codemap's stale `provider_log` rows: those three reads were already typed in
`app/providers`.

## Preserved behavior

- `approvals_log` reads its arguments in this order:
  1. the lenient limit: 20 unless a JSON number, clamped to 1..1,000;
  2. the lenient cursor: a non-string or malformed cursor is ignored;
  3. the strict `denied` boolean (`null` included);
  4. a truncated `since_ms` on the daemon clock.
- There is one row per approval id. The row is created at the approval's first
  event, so a resolution that arrives first still yields a row anchored at it.
  The request, whenever it comes, re-anchors the row and takes over its actor
  and correlation id.
- A resolution supplies the actor and correlation id only when the row has
  none. It sets the final status and `resolved_by`. Records without an
  approval id are skipped.
- Rows are filtered by request time and by `denied` (denials and timeouts),
  sorted newest first by timestamp and then sequence, cut at the cursor, then
  limited. The next cursor is advertised only for a full page.
- `approvals_stats` echoes `window_ms` (negative values included) and windows by
  request time only. A resolution without a request therefore drops out of any
  window. It reports:
  - total, granted, denied, timeout and pending;
  - resolved, and the grant rate over the resolved approvals;
  - denials and timeouts by capability (`unknown` when none was recorded);
    an empty map is `{}`.
- Neither read has a strict tenant argument. Routing follows the trimmed caller
  tenant.

The only intended difference is the shared already-canceled admission, which
rejects an authorized read before any work.

## Runnable comparison and regression evidence

The harness gives every cloned primary and `acme` journal twenty-four approvals:

- granted, denied, timed-out and pending approvals;
- resolutions published before their request;
- orphan resolutions;
- same-millisecond pairs;
- five malformed or id-less records.

It runs the pre-slice handlers and helpers on one copy and the registered
operations on another.

216 steps repeated twenty times cover eight sequences under primary, wrong and
tenant tokens, in normal and canceled contexts:

- unfiltered log and stats;
- the denied filter and cursor paging, including a denied page past a cursor;
- every limit and cursor shape;
- windows in both reads;
- every `denied` error;
- tenant-routed, padded, foreign, `null`, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact, and neither side journals anything. The harness
asserts three things:

- timeouts, pending rows, `resolved_by`, the `unknown` bucket and a fractional
  grant rate were read;
- the denied filter dropped granted and pending rows;
- the cursor paged.

The only exceptions are 37 canceled authorized steps, which return the
admission error.

Permanent tests cover:

- the joined rows, including re-anchoring and the actor and correlation
  fallbacks;
- orphan resolutions;
- the denied filter, the window on an injected clock, every limit shape, the
  default page and the clamp on 1,005 approvals;
- cursor paging to the last page, and same-millisecond ordering;
- the `denied` errors, the empty wires and journal errors;
- the stats fold, request-time windowing, the echoed window and the empty map;
- the specs and output schemas.

Native tests cover the registry flags, plus routing: each token sees only its
own kernel's approvals, the strict `denied` error, and a window on the daemon
clock.

Forty independent mutations fail tests, including the native registration, the
routed-kernel binding and the daemon-clock binding. Sources are restored
byte-for-byte. Fixtures use isolated temporary kernels only.
