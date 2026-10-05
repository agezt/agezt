# W2.6 world native exit evidence

All nine world native commands bind typed app/world operations through the common
control-plane adapter. This completes world in roadmap §3 order 2. Taste, skill,
broader adapters, generated surfaces and W3–W5 remain open.

| Operations | Scope and app ownership |
|---|---|
| world_add, world_edit, world_relate, world_forget | Primary-only mutations; graph curation, replacement, relation resolution/creation and reversible deletion. |
| world_get, world_list, world_resolve, world_neighbors | Primary-only reads; typed graph projection, quiet ranking and directional neighbors. |
| world_log | OwnTenant/CallerTenant read; lifecycle fold against the host-selected journal through journalview. |

## Native coverage and compatibility

`TestWorldNativeExitCompleteTypedRegistry` verifies the exact nine-command set in
the common registry, actual input/output types/schemas, AppOwned binding,
StreamNone and derived native read-only/tenant flags. Five reads remain unaudited;
four mutations require successful audit before service effects. Independently
removing the world family or log operation fails the exit regression. Source
registry/protocol, authentication, tenant routing and operation-audit suites
continue to exercise the production host.

Old socket business wrappers/registrations and the now-unused argStringMap helper
are removed. CP → worldmodel debt is officially removed: no exception expansion.
The adapter handles transport/auth/routing/audit and typed encoding; domain logic
lives in app/world. Metadata retains existing HTTP hints rather than inventing
routes for native-only commands.

Source/parity tests retain content identity, permissive kind/verb normalization,
case-preserving aliases, editable-state replacement, relation endpoint creation,
direction/weight, quiet resolve, retained tombstones and optional provenance.
Required/type/null admission precedes effects. Unknown unused fields, alias
trim/filter behavior, resolve default/max/fractional-zero limits and lenient log
limits/windows/cursors retain compatibility. Projection preserves empty arrays,
present zero/empty fields and missing entity/update shapes.

Move parity covers eight graph handlers × eight argument sets and 13 native log
inputs over 20 repeats. Typed binding parity covers nine operations × three
compatible inputs over 20 repeats. Ranking clock drift and the generic typed
failure-code envelope are explicit boundaries; result fields and domain error
text remain checked. [NEXT.md](NEXT.md), W2.6a–W2.6d records individual mutations.

## Audit, tenant and event identity evidence

Restored legacy binding reproduced graph node/edge changes and successful results
with a closed journal. `TestWorldAppSocketRequiresAuditBeforeMutations` now checks
all four mutations fail without changing the graph when journal admission is
unavailable. Typed dispatcher tests independently prevent every service factory
effect and keep five reads unaudited.

`TestWorldOperationsSocketTenantJournalIsolation` uses actual isolated primary,
acme and other kernels/socket clients. World log selects the correct journal for
operator and tenant credentials and refuses a foreign tenant. Graph commands
remain unavailable to tenant tokens; an operator's primary-only list ignores an
unused tenant argument and reads the primary graph.

`TestWorldNativeMutationSharesOperationCorrelation` verifies ordered invocation →
graph effect → terminal audit with one nonempty identity. Four real store/bus/
journal fixtures cover add/edit/relate/forget, including endpoint events created
by relate. Four independent path mutations reject missing identity. Context-free
calls retain their legacy empty correlation.

## Exit validation

Windows amd64, GOMAXPROCS=4: related source, graph/log/typed/native, registry/
tenant/audit and identity suites pass count=20. Focused race suites pass count=20.
Full Go tests/build/vet, scoped staticcheck, formatting, architecture/deadcode/
dependency/documentation/changelog and official structure gates pass. Counts are
224 packages, 142 import/13 call exceptions, 28 public SDK findings plus one test
seam, 24 justified core dependencies and 115 generated kernel packages.

`BenchmarkDispatchWithoutAuditIO`, three 100 ms runs: 11,734 / 10,536 / 11,211
ns/op, 7,607–7,609 B/op and 89 allocs/op. The measured 10.5–11.7 us/op is below
the <50 us dispatch budget. Construction and real audit/journal I/O are excluded.

Fixtures use TempDir stores/journals, mock providers and owned socket clients.
This exit does not certify a live model/provider, physical browser, generated
transport/SDK coverage or W3 knowledge/runtime module dissolution. Protected
delivery requires every final-head CI check before normal merge.

Next: taste, then skill. Full runs.Start convergence, remaining native domains,
broader adapters and W3–W5 remain open.
