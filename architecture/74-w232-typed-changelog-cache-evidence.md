# W2.32c typed changelog and cache statistics

`changelog` and `cache_stats` now run on the shared dispatcher, in
`kernel/app/journal`:

- `Service.Changelog` filters the journal to the material-change kinds, each with
  its stable label and a short payload detail.
- `Service.CacheStats` folds `budget.consumed` events into prompt-cache reads,
  writes and the saving versus the full input rate.

The full-rate pricing comes in as a `Cost` port, set with
`WithCost(governor.CostMicrocents)` by `journalReads`, so app/journal does not
import the governor.

`Operations` adds two read-only, unaudited, caller-tenant-routed specs with
unknown input allowed:

- `changelog` is primary-only, which derives the legacy `TenantRouted` without
  `TenantAllowed`.
- `cache_stats` is tenant-owned, so a tenant reads its own statistics.

Neither has an HTTP route, as before. The binder no longer forces primary-only,
which is the zero value, so each spec states its own authorization.
`changelog.go`, `cache_stats.go` and the two registrations are removed. The
trimFloat unit test moves with its helper. The decimal `itoa` helper is replaced
by `strconv`.

## Preserved behavior

`changelog`:

- `limit` is a JSON number truncated toward zero. Only a positive value counts,
  otherwise 20, and the value is capped at 1,000.
- `since_ms` windows to events stamped at or after now minus a positive value.
- Only the 17 material-change kinds appear, each with its label. The detail is
  the first non-empty string, or the number trimmed of `.0`, among `summary`,
  `name`, `skill_id`, `id`, `rule`, `change`, `reason`, `subject`, `provider`,
  `model` and `count`.
- Entries are newest first, and journal seq breaks a same-millisecond tie. Each
  carries its event and correlation ids. An empty result is `[]`.

`cache_stats`:

- Only `budget.consumed` events in the window count. Malformed payloads are
  skipped.
- The cached and written input tokens are summed. The saving is the full-rate
  price of the input and output tokens minus the recorded cost, floored at zero
  per call.
- `window_ms` echoes the truncated number, even when it is zero or negative.

The only intended difference is the shared already-canceled admission, which
rejects an authorized read before any work.

## Runnable comparison and regression evidence

The populated primary and `acme` journals gain the following:

- every interesting material-change kind, with name, count and reason payloads;
- priced, cached calls across four models (one unpriced);
- a malformed spend payload;
- an empty-payload policy change.

The harness runs the pre-slice handlers, kind table, detail probe and number
helpers on one copy and the registered operations on another.

174 steps repeated twenty times cover seven sequences under primary, wrong and
tenant tokens and normal and canceled contexts:

- full reads;
- every limit form and cap;
- windows, including stable values, negative and non-numeric ones;
- tenant-routed, padded, foreign, non-string and invalid tenants;
- secret-named arguments.

Responses are byte-exact, and neither side journals anything. The harness
asserts that ten changelog entries and non-zero calls and cached tokens were
read. The only exceptions are 29 canceled authorized steps, which return the
admission error.

Permanent tests cover:

- trimFloat;
- every detail probe rule;
- the changelog filters, limits, windows, tie order and labels, the exact entry
  and empty wires, and errors;
- the cache fold with the cost port's arguments, a malformed payload, the floor,
  windows and echoes, the exact empty wire and errors;
- the seven specs with their authorization and tenancy, and the output schemas;
- the native registry flags;
- native routing: the changelog follows the operator-named tenant and holds the
  primary's policy change, and a tenant token reads its own cache statistics.

The existing native changelog and cache suites still pass through the new path.

## Harness correction

Building this harness exposed a flaw in the shared population helper used since
W2.31c. Payloads went to the bus as Go `[]byte`, which the journal stores as a
base64 string, so payload-derived fields were empty on both sides. With the
helper fixed to journal raw JSON, the W2.31c run reads (288 steps x20, now
asserting intents, models, spend, delegation and tools) and the W2.32b grep and
export (216 steps x20, now asserting payload pattern matches) were re-proven
byte-exact. Their evidence documents record the correction. The W2.32a reads do
not depend on payload content and stay byte-exact.

Thirty independent mutations fail tests, including both specs' authorization
and tenancy and the native full-rate cost binding. One equivalent was recorded:
skipping trimFloat's whole-number branch, because `json.Marshal` already renders
whole float64 values without `.0`. Mutation testing found two gaps, now closed:
the default page size, and the material-kind table. The full label table is now
pinned as a contract. Sources are restored byte-for-byte.

Fixtures use isolated temporary kernels only.
