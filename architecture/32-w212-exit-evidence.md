# W2.12 storage/artifact native exit evidence

Prepared locally: source/docs are separate reviewable deliveries after the earlier
workboard/OKR patches. Protected main delivery remains pending while Git metadata
is read-only and GitHub authentication is unavailable. The five native commands
bind typed app/storage and app/artifacts operations through the common app host.
Remaining native domains, broader adapters/generated surfaces, runs.Start and
W3-W5 remain open.

| Operations | Native scope and ownership |
|---|---|
| storage_stats | Primary-only, read-only, unaudited unary inventory over selected filesystem/disk probe ports. |
| artifact_get, artifact_list | Primary-only, read-only, unaudited unary blob/index reads. |
| artifact_delete, artifact_collect | Primary-only mutations; mandatory durable audit before metadata/blob/collection effects, including dry-run admission. |

## Coverage and compatibility

TestStorageArtifactNativeExitCompleteTypedRegistry guards exact five-operation
common-registry coverage, actual input/output types/schemas, AppOwned metadata,
primary/read-only/unary scope and legacy unknown-input compatibility. Storage
family or artifact collection removal independently fails the exit regression.
Old business wrappers/registration are gone; actual filesystem/probe and blob/
index host bridges remain. Dead legacy dry-run admission is removed after its
final caller moves, with behavior preserved by the typed operation DTOs.

Storage keeps directory totals, root loose-file grouping, descending bytes/name
ordering, labels, required zero/false values, null empty dirs and available zero
optional disk fields. ReadDir/entry/walk/probe errors deliberately remain the
existing best-effort diagnostic contract; this move does not introduce a new
filesystem safety, symlink-following or failure policy. Exact native old/current
parity passes count=20 across five disk probe modes on owned homes.

Artifact DTOs retain base64 bytes/ref/size, integrity/sentinel messages and original
unclassified causes, all required empty metadata fields, empty arrays and the
dry-run-only candidate property. Actual index fixtures guard strict cutoff
inequality, unknown creation timestamps and deduplicated blob ownership until
the last index reference is removed. Four-handler x five-input native parity
passes count=20 with generated entry IDs and bounded cutoff clocks.

Admission retains missing/null strict string differences, raw ref/ID without
silent trimming, exact optional filter strings, integer/digit-string day parsing,
30-day nonpositive default and dry-run default true. Boolean and string forms are
kept exactly: only literal false or 0 strings disable dry-run; case/whitespace
variants retain the old interpretation. Availability check ordering remains.

## Audit and deletion failure evidence

Owned actual closed-journal fixtures showed legacy delete/collect returned
success while removing index entries. Permanent tests now require unavailable
audit to block both metadata and blob effects; dry-run admission is also audited.
Actual successful mutation records show one correlated invoked/completed span,
without duplicate native audit. Three reads remain usable with closed mutation
audit. Actual primary/acme socket clients deny every command to tenant credentials
without changing primary metadata/blob state.

A separate actual metadata failure proof replaced one owned metadata file with
a nonempty directory. Original Delete returned nil after forgetting the entry
and deleting its blob; Collect counted failure as reclaimed and native delete
reported success. Index.Delete now removes metadata under index ownership first,
returns the wrapped non-ENOENT failure before forgetting the entry/blob, and keeps
already-absent metadata idempotent. Collection counts successful removals and
retains failed candidates. Core/native regressions are red before the fix, pass
three verifier runs count=20, and fail for ignored removal or incorrect ENOENT.

This narrow fix does not redesign the legacy collection partial-count API,
certify every concurrent writer path or blob-GC error handling, or protect journal
raw_ref leases. Those broader concerns remain explicit modules/artifacts work;
no all-surface or full artifact-module completion is claimed here.

## Exit validation and boundaries

Twenty-eight independent mutation proofs cover storage aggregation, artifact
service, typed admission/audit, metadata failure and aggregate exit. Related
service/store/source/CLI/native/audit/tenant/admission/exit suites and package race
pass count=20. Complete controlplane race and final full Go tests pass count=1;
build/vet, scoped staticcheck, formatting, architecture/deadcode/dependency/doc/
changelog and official structure gates pass. Current counts: 231 packages,
139 import/13 call exceptions, 28 SDK findings plus one test seam, 24 dependencies
and 122 generated kernel packages. The official updater removes the paid
controlplane-to-artifact import allowance (140 -> 139), with no new exception.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 6811 / 8893 / 7451 ns/op;
7609 / 7608 / 7609 B/op; 89 / 89 / 89 allocs/op. All satisfy the <50 us framework
budget. Construction and audit/journal I/O are excluded; this is not whole-
operation roundtrip or provider latency.

Fixtures use owned TempDir stores/journals, mock providers and socket clients.
No live-provider, physical-browser, generated transport/SDK or physical filesystem
failure outside the owned fixtures is claimed. Restricted-session validation uses
task-owned Go/staticcheck caches; build succeeds with a warning about the shared
module stat cache. Protected delivery requires every final-head CI gate before
normal merge.

Next: schedule, then standing/workflow/pulse/autonomy in roadmap order 4;
module/reference convergence and broader migration remain open.
