# W2.31c typed run reads

The two past-run reads now run on the shared dispatcher:

- `runs_list`: the filtered, cursor-paginated run list;
- `runs_stats`: run health statistics.

The new package `kernel/app/runs` owns their codecs, filters, sort, cursor
paging, row presentation and statistics. They work over a narrow `Run` snapshot
row and an injected clock.

`collectRuns`, the native journal fold, stays where it is. It is shared with the
schedule firing views, roster status, tenant listings and the run result, and
`runReads` binds it as the app's snapshot port for the kernel the dispatcher
routed to.

`Operations` declares two read-only, unaudited specs. Both are tenant-owned and
caller-tenant-routed, with unknown input allowed:

- `runs_list` on `GET /api/runs`, matching the Web UI read route;
- `runs_stats`, with no route, as before.

`runs_handlers.go`, `runs_handlers_stats.go` and their two registrations are
removed. An orphaned `extractIntent` comment moves onto its function. The native
wire entries are `AppOwned` and `ReadOnly`, and tenant-allowed and routed.

## Preserved behavior

`runs_list`:

- `limit` is a JSON number truncated toward zero. Anything else means 20, and the
  value is clamped to between 1 and 1,000.
- An unreadable or non-string `cursor` means the first page.
- `status`, `intent` and `model` are strict strings, checked in that order and
  before the journal fold.
- `min_cost_mc` and `max_cost_mc` are lenient numbers, and only positive values
  filter.
- Every filter applies before the limit. `intent` and `model` match
  case-insensitive substrings. The status precedence is completed, failed,
  abandoned, then running.
- Runs sort newest first; within one millisecond, the later journal seq comes
  first. Paging keeps runs strictly older than the cursor, and `next_cursor`
  appears only on a full page.
- A completed run reports its duration only with a known start. A failed run
  reports its reason, and a duration only when it ended at or after its start.
- `phase` and `tool` appear only on running runs and are omitted when empty.

`runs_stats`:

- The journal fold runs before the `intent` check.
- `since_ms` is a lenient number, and only a positive value windows the stats.
  The window excludes runs without a recorded start. `window_ms` echoes the
  truncated value, even when it is zero or negative.
- The success rate counts failed and abandoned runs against it, but not runs in
  flight.
- A failure without a reason buckets under `unknown`.
- `by_model` and `failed_by_reason` are objects, never null.
- Delegation counts, the maximum fan-out, total and delegated spend, and the
  spend and duration distributions are reported. Durations cover completed runs
  only, and spend covers priced runs only.

The only intended difference is the shared already-canceled admission, which
rejects an authorized read before any work.

## Runnable comparison and regression evidence

The harness journals a varied history into the primary kernel (40 runs) and the
`acme` tenant kernel (25 runs):

- every status and every failure reason, including an empty one;
- agents, delegation links and priced models, including unpriced runs;
- live phases with a tool;
- same-millisecond starts;
- an orphan completion, and spend for an unknown run.

It then runs the pre-slice `handleRunsList` and `handleRunsStats` on one copy and
the registered operations on another.

288 steps repeated twenty times cover 13 sequences under primary, wrong and tenant
tokens and normal and canceled contexts:

- full reads;
- three chained cursor pages;
- limit clamps;
- every status, intent and model filter, and cost bands;
- malformed cursors and every codec error;
- windows (positive, string, negative, fractional) and intent scopes;
- tenant-routed, padded, foreign, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact on both sides, and neither side journals anything. The
harness asserts that the populated history was read: 20 rows and 41 runs in
total. The only exceptions are 51 canceled authorized, routable steps, which
return the admission error.

Permanent tests cover:

- the status precedence, the codec errors and their order before the fold, and
  fold errors;
- the limit clamp;
- every filter, the sort and tie-break, cursor pages, short and unreadable
  cursors, the row presentation rules and the exact row and empty wires;
- the full statistics, windows, scopes, the fold-before-intent order and the
  empty statistics wire;
- the specs, and the output schemas against real outputs;
- the native registry entries;
- per-token routing across a primary and a tenant kernel, padded tenants
  included;
- a deterministic same-millisecond guard: eight starts journaled with a fixed
  clock must list in reverse journal order through the native port.

The existing native runs suites still pass through the new path.

Forty-seven independent mutations fail tests. They include the native
registration, the routed-kernel binding and three port fields. Sources are
restored byte-for-byte. Fixtures use isolated temporary kernels only.
