# W2.7 taste native exit evidence

All three taste commands bind app/taste operations through the common native host.
This completes taste in the ordered native migration; skill, remaining domains,
runs.Start convergence, broader adapters/generated surfaces and W3-W5 remain open.

| Operations | Policy and ownership |
|---|---|
| taste_list | Primary-only read; typed exemplar projection, scope/tag filters, default 200 limit. |
| taste_create, taste_delete | Primary-only mutations; successful mandatory audit admission precedes store effects. |

The exact three-command common registry regression verifies actual input/output
types/schemas, AppOwned metadata, read-only/primary flags and StreamNone. Removing
the family or delete operation independently fails the exit test. Old socket
wrappers and duplicate registration are deleted. No HTTP hints are invented for
this native-only family. Runtime exemplar selection remains unchanged.

Request structs preserve native lenient admission: strings are trimmed, nonstring
values are ignored, nonpositive/fractional-zero limits default to 200, mixed tag
arrays retain string items, CSV tags are split, and unused fields remain accepted.
Service outputs use actual exemplar fields, preserving optional scope/tags,
creation/update times, required identity/content and present-empty list arrays.
Store case-sensitive tag dedupe, filtering, validation, deletion causes and
persistence rollback are covered by source and service tests.

Three old/current native handlers match over 20 repeats. Fresh create/delete IDs
and bounded creation clocks are normalized explicitly; domain errors and result
fields remain checked. Eight independent binding mutations fail: mutation/read
flags, missing operation, tenancy, fractional limit, ID trim, CSV tags and unknown
fields. Eight service mutations additionally guard foundation behavior.

A controlled original binding reproduced successful create/delete and changed
state with a closed journal. Permanent actual host tests now retain the seeded
exemplar unchanged and return audit error for both mutations. Listing still works
with unavailable mutation audit. Dispatcher fixtures independently prove no service
factory effect before audit. One successful mutation has exactly one ordered
invoked/completed audit arc with a shared nonempty correlation ID. The taste store
has no separate domain journal events to join.

Real isolated socket clients and primary/acme kernels verify tenant credentials
cannot call any taste command, and an operator's unused tenant argument does not
select a tenant store or expose its private exemplars.

Source/service/store/native/registry/tenant/audit and aggregate exit suites pass
count=20. Taste app/store race suites pass count=20. Final full Go tests/build/vet,
scoped staticcheck, formatting, architecture/deadcode/dependency/documentation/
changelog and official structure gates pass: 225 packages, 141 import/13 call
exceptions, 28 SDK findings plus one test seam, 24 dependencies, 116 generated
kernel packages. No exception is expanded; CP -> taste debt was officially paid.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 11805 / 6889 / 10791 ns/op; 7610 / 7609 / 7608 B/op; 89 / 89 / 89 allocs/op.
All three measurements satisfy the <50 us dispatch budget. Construction and real
audit/journal I/O are excluded.

Fixtures use owned TempDir stores/journals, mock providers and socket clients.
No live provider, physical browser or generated transport/SDK coverage is claimed.
Protected delivery still requires every final-head CI gate before normal merge.
A separate Nostr test fixture repair is retained in its own concern.

Next: skill, then remaining domains in roadmap order.
