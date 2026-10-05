# W2.5 memory native exit evidence

All 16 memory/profile native commands now bind typed app/memory operations through
the common control-plane adapter. This completes the memory domain in roadmap §3
order 2. World, taste, skill, broader adapters, generated surfaces and W3–W5 remain
open.

| Operations | Scope and app ownership |
|---|---|
| memory_get, memory_list, memory_search, memory_find_related | Primary-only reads; typed record retrieval, active preparation/paging and search results. |
| memory_add, memory_supersede, memory_forget, memory_promote, memory_bulk_forget | Primary-only mutations; operator curation, revision, reversible deletion, sharing and bounded batches. |
| memory_prune, memory_tidy | Primary-only mutations, including dry-run admission; typed maintenance reports. |
| memory_audit | OwnTenant/CallerTenant read from the selected store. |
| memory_clean | OwnTenant/CallerTenant mutation of the selected store. |
| memory_log | OwnTenant/CallerTenant read from the selected journal. |
| memory_consolidate, profile_rebuild | Primary-only mutations; runtime distillation port, owned caller context and typed reports. |

## Native contract and admission

`TestMemoryNativeExitCompleteTypedRegistry` checks the exact 16-command set in
the common registry, actual input/output types and schemas, AppOwned binding,
StreamNone and read-only/tenant flags. Six operations remain reads and ten require
audit before service effects. Independent removal of the complete memory family
or profile operation fails this exit regression; restored source passes count=20.
Registry/protocol coverage, authentication, tenant routing and operation-audit
source suites continue to exercise production registration and transport.

Old memory/profile socket business handlers and duplicate registrations are
removed. The final jsonMap/Server.ok helpers became unreachable and are removed;
no dead-code exception was added. Known type/null/required admission precedes
effects. Unknown unused fields, bool/string dry_run, lenient day/log inputs and
bulk trim/blank filtering preserve the source contract. The list preparation move
retained its earlier read-before-native-admission order; typed dispatch now admits
schemas before invoking the business read.

Move/binding parity covers field presence, empty arrays, present null/zero values,
lifecycle/provenance, lookup/search ordering, revision/sharing semantics, age and
dry-run safety, pruning statistics, ID bounds/counts and partial stop-on-error
batches. Native binding parity covers 16 operations × three compatible inputs ×
20 repeats. Its explicit boundaries are wall-clock cutoff/ranking drift, freshly
generated identity and the generic typed failure-code envelope; result fields and
domain error text remain checked. Detailed earlier proofs/mutations are recorded
in [NEXT.md](NEXT.md), W2.5a–W2.5i.

## Audit, tenant, identity and lifetime evidence

Restored legacy native binding reproduced add/revise/forget/promote/clean effects
and success with a closed journal. `TestMemoryAppSocketRequiresAuditBeforeMutations`
now checks all ten mutations fail on unavailable admission without changing the
store. Typed dispatcher tests independently prevent every service factory effect
and keep all six read operations unaudited.

`TestMemoryOperationsSocketTenantStoreAndJournalIsolation` uses actual isolated
primary/acme/other kernels and socket clients. Audit/log/clean select the correct
store/journal for operator and tenant credentials, reject foreign tenant access,
and tenant execution removes only its own low-value record without writing the
other journals. Primary-only list remains unavailable to tenant credentials.

`TestMemoryNativeMutationSharesOperationCorrelation` checks ordered invocation →
memory effect → terminal audit with one nonempty identity. Real store/bus/journal
fixtures cover all eight store mutation paths; controlled distillation ports
retain admitted identity and generate legacy fallback identity only when absent.
Ten independent identity mutations fail.

Controlled direct and typed blocked ports reproduce and verify cancellation of
consolidate/profile rebuild, earlier caller deadlines, model context values and
pre-canceled admission. Both paths retain the owned five-minute ceiling and
deferred cleanup. Six independent context mutations fail; background callers
retain fallback identity and the ceiling. This is controlled runtime-port and
mock-provider evidence, without live model invocation.

## Exit validation

Windows amd64, GOMAXPROCS=4: related app/controlplane source, typed/native,
registry/tenant/audit, identity and context suites passed count=20. Focused race
suites passed count=20. Full Go tests/build/vet, scoped staticcheck, formatting,
archcheck, deadcodecheck, depscheck, docclaimscheck, changelog-lint and official
structure check passed. Counts remain 223 packages, 143 import/13 call exceptions,
28 public SDK dead-code findings plus one test seam, 24 justified dependencies and
114 kernel packages in the official structure.

`BenchmarkDispatchWithoutAuditIO`, three 100 ms runs: 5,673 / 7,167 / 5,845 ns/op,
7,606–7,609 B/op and 89 allocs/op. The measured 5.7–7.2 us/op is below the <50 us
dispatch budget. Construction and real audit/journal I/O are excluded.

Fixtures use TempDir homes, owned stores/journals, controlled ports and mock
providers. This exit does not certify a paid/live provider, physical browser,
generated transport/SDK coverage or W3 knowledge/runtime module dissolution.
Protected delivery still requires every final-head CI check before normal merge.

Next: world, then taste and skill in roadmap order. Full runs.Start convergence,
remaining native domains, broader adapters and W3–W5 remain open.
