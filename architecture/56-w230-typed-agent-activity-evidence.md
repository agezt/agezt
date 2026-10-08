# W2.30f typed agent_activity

`agent_activity` is now a shared application operation. `approster.ActivityService`
owns the per-agent timeline over two ports — roster lookup by id or slug and a
journal range — and uses the `ActivitySummary` text moved in W2.30e.
`ActivityOperations` declares one primary-only, read-only spec with unknown input
allowed and `GET /api/agents/activity` metadata, matching the existing Web UI read
route. The native `handleAgentActivity` handler and its manual `commandSpec` row are
removed; the native wire entry is `AppOwned`, `ReadOnly`, not tenant-allowed or
routed. Repair status, escalations and the remaining roster commands stay native.

## Preserved behavior

Arguments stay raw so the legacy order holds: `ref` is required (absent, blank or
whitespace-only gives `args.ref required`; a non-string, including null, gives
`args.ref must be a string`), is looked up untrimmed, and an unknown agent is
reported before any `limit` error. `limit` must be a number (null rejected),
defaults to 50 (also for zero and negative values), truncates fractions — so a value
between 0 and 1 becomes an unlimited page, exactly as before — and caps at 500.
`cursor` is read leniently: non-strings are ignored, strings are trimmed, and only
positive integers filter to strictly older sequence numbers. The journal is read
only after argument admission.

One pass collects the agent's run correlations from `task.received` (only events
with a correlation id and a decodable payload) and every attributable event, newest
first. `total` counts the whole timeline before cursor and limit, `next_cursor`
appears only when a page is truncated, and the item fields are `seq`, `kind`,
`ts_unix_ms`, `correlation_id` and `summary`. A timeline with no attributable
events serializes `"activity":null`; a cursor that filters every item away gives
`[]` — both exactly as before. The intended wire change is the shared
already-canceled admission, which now rejects before the roster or journal is read.

## Runnable comparison and regression evidence

The native harness runs the pre-slice handler (reconstructed from full-path
snapshots) against the registered operation on the same initialized kernels. 792
cases repeated twenty times cover three kernel states (empty; two agents; two agents
with journaled task, council, retry, repair, wake, completion and failure events for
both), normal and canceled contexts, primary, wrong and tenant tokens, twenty-two
argument shapes (missing, blank, typed and null refs, unknown and padded refs, every
limit edge, padded/invalid/numeric/positive cursors, unknown keys) and two reads. 660
complete raw socket responses are byte-equal and the populated timeline is checked
to contain the agent's own council and retry lines and none of the other agent's;
the 132 differences are exactly the primary-token canceled admissions. Journal
head/hash and provider call counts stay unchanged.

Permanent tests pin argument order and messages, no journal read before admission,
the null and empty-array shapes, run-correlation scoping (including uncorrelated and
undecodable `task.received` events), newest-first order, totals, paging and every
cursor/limit edge, the 50 default and 500 cap over 600 events, spec metadata,
non-primary and canceled admission, codec errors through dispatch and the native
wire entry. Twenty-three independent mutations fail tests and sources are restored
byte-for-byte. One further candidate — dropping the payload-decode check in run
collection — is an equivalent mutant, because `json.Unmarshal` validates the whole
input before populating, so an undecodable payload leaves the map nil either way; it
is recorded and excluded. Fixtures use isolated temporary kernels and do not mutate
owner profiles, wake agents, run a provider or send channel messages.
