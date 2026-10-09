# W2.32b typed journal grep and export

`journal_grep` and `journal_export` now run on the shared dispatcher, next to the
W2.32a reads in `kernel/app/journal`:

- `Service.Grep` owns the filter codecs, the AND-of-filters walk and the limit.
- `Service.Export` owns the window, the correlation scope and the verification
  bundle, over the same `Journal` port and the daemon clock that `journalReads`
  injects.

`Operations` adds two primary-only, read-only, unaudited specs with `Primary`
tenancy, so both always read the primary journal:

- `journal_grep` on `GET /api/journal`, matching the Web UI read route;
- `journal_export`, with no route, as before.

The grep output reuses the window output (now `EventsOutput`, shared with the
tail). The export output schema comes from the same `event.Event` wire mirror.
The adapter writes `events` from both typed outputs directly, so the journal
Event member order is unchanged. The export cap is `appjournal.MaxExportN`, and
the CLI's `MaxJournalExportN` now returns it.

`journal_grep.go`, `journal_export.go` (with the stop sentinel and the pattern
matcher) and the two registrations are removed.

## Preserved behavior

`journal_grep`:

- `pattern`, `kind`, `subject`, `actor` and `correlation_id` are strict strings,
  checked in that order before any read.
- `limit` is a JSON number truncated toward zero. Anything else means 100, and
  the value is clamped to between 1 and 10,000.
- The head checkpoint is taken before the walk.
- `kind`, `subject`, `actor` and `correlation_id` match exactly. `pattern` matches
  a case-insensitive substring of the kind, subject, actor, correlation or raw
  payload.
- The walk runs from the oldest event and stops as soon as `limit` matches
  accumulate. Read errors are returned, and an empty result is `[]`.

`journal_export`:

- `since_ms` is a JSON number. Only a positive truncated value windows the
  export, to events stamped at or after now minus `since_ms`.
- `correlation` is a strict string that scopes the bundle, which is then
  deliberately non-contiguous, and it is echoed back.
- The head sequence and hash are taken before the walk.
- `first_seq` and `last_seq` are -1 when nothing matched.
- Events beyond the cap mark the bundle truncated. Exactly the cap is complete.

A tenant token is refused for both. The only intended difference is the shared
already-canceled admission, which rejects a primary read before any work.

## Runnable comparison and regression evidence

The harness reuses the populated primary (40 runs) and `acme` (25 runs) journals.
It runs the pre-slice handlers, constants, stop sentinel and pattern matcher on
one copy and the registered operations on another.

216 steps repeated twenty times cover nine sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- full reads;
- exact filters and limits;
- patterns across the kind, payload, correlation and model, and no match;
- every limit form;
- every codec error;
- export windows (stable values only, since the sides run moments apart) and
  correlation scopes, including unknown scopes and errors;
- tenant arguments;
- secret-named arguments.

Responses are byte-exact, including full event bundles, and neither side
journals anything. The harness asserts that the populated journal was read: 100
grep matches and a non-empty, untruncated bundle. The only exceptions are 36
canceled primary steps, which return the admission error.

Permanent tests cover:

- the grep codec order with no read on error;
- every filter, including exact-versus-prefix, the pattern fields and payload,
  every limit form and both caps;
- the head-before-walk order, the walk stopping at the limit, the empty wire and
  errors;
- export windows with an injected clock, non-positive and non-numeric windows,
  the scope with its echo and -1 bounds, and the exact empty wire;
- the strict correlation with no read on error, truncation at the first event
  past a small cap and an exact-cap bundle, the default cap, and errors;
- the specs, and the output schemas against real outputs;
- native routing: both commands read the primary journal under a tenant argument
  and keep the event member order, the export window is measured on the daemon
  clock, the CLI cap is the app cap, and a tenant token is forbidden.

The existing native grep and export suites still pass through the new path.

Thirty-five independent mutations fail tests. One equivalent was recorded:
clamping the grep limit to 0, because the walk appends before its stop check, so
0 still keeps one match. Mutation testing found a real gap, now closed: no test
told an exact subject, actor or correlation match from a prefix match. Sources
are restored byte-for-byte. Fixtures use isolated temporary kernels only.

## Harness correction (recorded in W2.32c)

The shared populated journal originally carried base64-encoded payloads (see the
W2.31c correction), so pattern searches never matched payload text. W2.32c reran
this comparison, 216 steps x20, with raw JSON payloads, and now asserts that the
payload patterns (`DEPLOY`, `second LINE`, the model) match events. The parity is
byte-exact.
