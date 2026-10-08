# W2.30c typed agent_list and native binding

`agent_list` is now a shared application operation. `approster.ListOperations`
declares one primary-only, read-only spec (`GET /api/agents` metadata, unknown input
allowed) whose handler calls the application `ListService` selected from the native
host. The native `handleAgentList` wrapper and its manual `commandSpec` row are
removed; the command reaches the shared dispatcher through `registeredAppOperations`
and its native wire entry is `AppOwned`, `ReadOnly`, not tenant-allowed or routed.
The remaining sixteen roster commands, status source collection and lifecycle are
still native.

## Typed output

`ListOutput` replaces the untyped `map[string]any` page: profiles, count, global
total and enabled count, plus `next_cursor` only when a page is truncated. An empty
roster still serializes `"profiles":null`. `ProfileOutput` embeds the full lower
`core.Profile`, so the derived output schema covers every profile field, plus `kind`,
`managed` and optional `status`. Its marshaler keeps the legacy profile projection
through `ProfileView`: profile numbers round-trip through float64 (for example
`created_ms`/`max_cost_mc` 9007199254740993 still emit ...992), and the paging cursor
keeps the same float64 projection for both filtering and `next_cursor`.

`StatusOutput` types all 81 emitted status fields; the 18 always-present defaults
are non-pointer required fields and the rest are optional. Status integers stay
exact (`active_spent_mc` 9007199254740993 is not rounded). Only
`last_autonomy_runbook` and `mailbox_wakes` remain dynamic object values, because
they carry observed event payloads. The list service converts each rendered status
map with unknown-field rejection, so a renderer field missing from the typed model
fails the list instead of silently disappearing.

## Drift found and fixed during this slice

The partial slice converted status maps through JSON into pointer fields. Decoding
drops explicit nulls, so nil slices or maps in a rendered status were omitted where
the legacy wire emitted `null`. This is a real path: a schedule-only agent has a wake
row with nil event subjects, so legacy `agent_list` returns
`"wake_event_subjects":null`. Nil forced/current/previous routing chains and a nil
runbook map behave the same way. `statusOutput` now restores those keys as pointers
to nil values (which marshal as `null`) and rejects null for any non-nullable field.
A permanent byte-parity test covers seven snapshot variants across plain, managed
and system profiles; with the restoration disabled it fails on the schedule-only
case, and the native parity harness also fails on it.

## Admission and codecs

`ListRequest` keeps `limit` and `cursor` as raw values so the legacy delayed codec
order holds: the roster is read and the cache prepared before an argument error is
returned, `limit` is checked before `cursor`, null is rejected for both, fractional
limits truncate, non-positive limits mean unlimited and the service caps at 1000.
Unknown arguments are accepted. Non-primary principals are rejected before the
provider or roster is touched. The one intended wire change is the shared
already-canceled admission boundary: legacy ignored the connection context, the
shared dispatcher now returns `context canceled` before reading the roster.

## Runnable comparison and regression evidence

The native harness runs the pre-slice handler and list service (reconstructed from
full-path snapshots) against the current registered operation on the same
initialized kernels. 528 cases repeated twenty times cover four kernel states (empty,
profiles with paused/retired/rich fields, journaled repair/retry/policy/fallback/
runbook/live events, and a schedule-only wake row), normal and canceled contexts,
primary, wrong and tenant tokens, eleven argument shapes and two cache reads. 440
complete raw socket responses are byte-equal; the 88 differences are exactly the
primary-token canceled admissions. Journal head/hash and provider call counts stay
unchanged.

Permanent tests pin the spec metadata and native wire entry, full schema shape,
profile/status byte parity and presence, explicit-null restoration, status presence
for nil versus missing rows, float64 cursor rounding, the 1000 cap, page sizes for
fractional/negative/zero limits, codec ordering and admission. Twenty-nine
independent mutations across the operation, output model, list service and native
registration each produce a real test failure, and every source byte is restored.

The dispatch benchmark was run for the first time: about 150 µs and 1,236
allocations for one cached profile, and about 1.5 ms for fifty profiles with rich
status. Roughly 0.68 ms of the fifty-profile cost is output-schema validation, which
re-parses the declared schema on every call; this is shared by every migrated
operation and is recorded as a framework follow-up rather than changed here.
Fixtures use isolated temporary kernels; they do not mutate owner profiles, wake
agents, run a provider or send channel messages.
