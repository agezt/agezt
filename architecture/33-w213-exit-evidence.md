# W2.13 schedule native exit evidence

Prepared locally on the shared main checkout. Source/docs remain separate
reviewable deliveries; protected merge is pending while Git metadata is read-only
and GitHub authentication returns HTTP 401. This evidence closes the ten-command
native schedule migration, not all run/trigger/module/generated-surface work.

| Operations | Ownership and native policy |
|---|---|
| schedule_list, schedule_system_tasks, schedule_test | Primary-only unaudited unary reads: list/catalog/forecast over cadence, journal annotation, validation and clock ports. |
| schedule_fires, schedule_stats | Tenant-owned/routed unaudited unary reads over the selected journal and the shared collectRuns snapshot port. |
| schedule_add, schedule_rm, schedule_run, schedule_enable, schedule_edit | Primary-only unary mutations; mandatory durable audit before cadence/provider effects. Run marks due; this wave does not start an engine. |

## Registry and boundaries

TestScheduleNativeMetadataComesFromTenTypedAppSpecs checks exact aggregate
coverage, bound input/output types and schemas, AppOwned native metadata, five
read/five-write policies, primary versus tenant routing, unary framing and legacy
unknown-input compatibility. Removing the schedule family or only schedule_fires
independently fails the exit test. Old native wrapper/registration/decode/write
helpers are gone. Selected host bridges retain actual cadence stores, live
agent/workflow/tool lookups, journal/run snapshots and clock/publication ownership.

The services own list/catalog/forecast and execution metadata, remove/run/enable
lifecycle, target/agent admission, five cadence creation/binding/compensation paths,
edit preflight/mutations and firing history/statistics/latest annotations. Resident
cadence run execution remains in the open W2.2 runs.Start/W4 trigger migration;
W3 module extraction, other adapters and W5 generated surfaces remain open.

## Compatibility and source proofs

Typed raw request fields preserve absent versus null, strict IDs/target strings,
selected numeric parsing, lenient edit string clearing, bool/string enable forms,
limit/since behavior, unknown args and early missing-edit updated:false. Intent
is the empty target constant. Workflow/agent references resolve at their original
points; edit's second live workflow lookup stays after field setters. Cadence
selection and timezone/future/minimum/window/day errors retain order, integer/
duration truncation and core continuous minimum behavior.

Record DTOs retain thirty required native fields including empty/zero/false values,
raw JSON payload and optional last-status/reason/timestamp presence. Forecast
missing responses remain only found:false; found responses keep false enabled,
empty mode, empty forecasts and count. Explicit record/list/edit output schemas
support arbitrary raw payload values and enforce required payload/updated presence.

Creation retains agent-before-target binding, original causes, zero failure output,
best-effort compensation and final refetch/local fallback. Editing retains
preflight-before-effects, intent/model/agent setter order, original handled causes,
ignored model/agent errors and false setter results, second target lookup/payload
serialization, late strict timezone errors, five reschedules and final projection.
Late errors can retain earlier updates. A missing final refetch still produces
zero-entry metadata with updated:true. These legacy failure/atomicity semantics
are explicitly preserved; their repair is separate work.

Firing views retain filter-before-sort/limit, case-insensitive intent matching,
floor-one/cap-1000, descending timestamp/sequence order, strictly older cursor and
oldest emitted cursor boundary. Completed/failed/abandoned/running precedence,
legacy duration behavior, spend/answer preview, required row fields, optional
nonempty runbook, malformed/legacy payload fallback and source classifier metadata
remain. Stats keep named schedule cardinality, unknown empty failure reasons,
terminal success rate, required empty map and original window value. Latest skips
missing IDs and keeps the first same-ms event; list annotation failures remain
best effort. Original run/range failures return zero outputs with original causes.

Exact old/native parity passes count=20 through successive foundations: three read
handlers x seven inputs plus eight target/agent projections; three lifecycle
handlers x four targets x ten inputs; 180 runnable/8,640 warning combinations;
36 then 40 create inputs; 42 then 49 edit inputs x four starting targets; three
firing views x 27 inputs. Final old handler/new typed dispatch parity covers ten
commands x thirteen inputs, normalizing generated IDs, bounded clock differences
and generic framework error-code fields only. Tests use actual owned cadence and
journal stores plus the shared run fold, not external providers or an engine.

## Audit, identity and tenant evidence

Actual closed-journal fixtures prove all five mutation commands stop before any
cadence/provider effect. Five reads retain unaudited unary behavior. Successful
enable produces one invoked/completed audit span and an operator action with the
same host-owned correlation. Raw corr/correlation_id cannot select that identity.
This durable-audit admission and joined operator identity are explicit changes
from legacy best-effort dispatch/publication.

Actual socket tests deny all eight primary-only commands to tenant credentials
without changing primary cadence/provider state. Fires/stats primary, owner-routed
and tenant-token reads join only the selected tenant journal/run results; foreign
tenant requests are denied. The selected-outcome-fold mutation independently
breaks this isolation regression.

## Validation and delivery limits

117 valid regression-breaking mutations across W2.13a-k cover projection,
admission, lifecycle, metadata, creation, edit, firing, audit, tenant routing,
schemas and aggregate exit. Compile-failed harness mutations were corrected and
rerun; they are not counted as product findings. Source/service/cadence/journal,
native/CLI/audit/tenant/admission suites and related package race pass count=20.
Full Go tests and complete controlplane race pass count=1; build/vet, scoped
staticcheck, gofmt, architecture/deadcode/dependencies/doc/changelog and official
structure checks pass. Production matches the frozen W2.13j delivery after exit
mutations; final full evidence is retained from that unchanged state, followed by
scoped exit reruns rather than redundant full-repository runs.

Current counts: 232 placed packages, 139 import/13 call exceptions, 28 public SDK
findings plus one test seam, 24 justified dependencies and 123 generated kernel
packages. No new allowance or dependency is added.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 7674 / 6024 / 6569 ns/op;
7610 / 7607 / 7609 B/op; 89 / 89 / 89 allocs/op. All satisfy the framework
budget below 50 us. This mock-host framework benchmark excludes construction,
audit/journal I/O and whole schedule operation/provider roundtrip latency.

Owned TempDir stores/journals, mock providers and socket clients are used. No
live-provider, physical-browser, restarted daemon, generated transport/SDK or
all-surface module certification is claimed. Task-owned caches support the
restricted session; build exits zero with a shared module stat-cache warning.
Protected delivery still requires authenticated final-head CI and normal merge.

Next: standing, workflow, pulse and autonomy in roadmap order 4. Broader run,
trigger, module, artifact raw-ref lease and generated-surface migration stay open.
