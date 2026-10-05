# W2.8 skill native exit evidence

All fourteen skill native commands bind typed app/skill operations through the
common host. This completes skill in roadmap order 2. Board/workboard/OKR/storage
and remaining native domains follow; broader adapters/generated surfaces,
runs.Start convergence and W3-W5 remain open.

| Operations | Native scope and ownership |
|---|---|
| skill_list, skill_get, skill_history, skill_files, skill_read_file, skill_hygiene | Six primary-only reads; no mutation audit. |
| skill_promote, skill_quarantine, skill_archive, skill_revert, skill_restore | Five primary-only lifecycle mutations; mandatory audit before effects. |
| skill_share, skill_reassign, skill_import | Three primary-only curation mutations; roster admission and portable import behavior retained. |

## Coverage and native compatibility

TestSkillNativeExitCompleteTypedRegistry guards exact fourteen-command coverage
in the common registry, actual input/output types/schemas, AppOwned binding,
read-only/primary scope and StreamNone. Removing the family or import operation
independently fails the exit regression. Old socket business wrappers, duplicate
registrations, projection/history and resource-admission helpers are deleted.

Four services own transport-independent typed use cases. Records preserve empty
agent/description and six zero-valued metrics, optional body/provenance/arrays,
list order/active counts, present-empty arrays and found-only missing shape.
Lifecycle methods retain legal transitions, idempotent archive, lineage parent
restoration, restore target validation, output reasons and original error causes.
Curation retains roster admission before reassignment, empty-owner sharing,
new-draft/content identity, existing-content lifecycle/first ownership and portable
Unicode bundle contents/manifest. Files prefer successful nonnil disk listing and
preserve fallback/directory/content byte counts. Hygiene retains default 30 days,
cutoff, top-level usage plus typed record projection and empty arrays.

Typed admission preserves untrimmed ID/reason/path bytes, lenient import text,
strict nonnull string arrays, nullable string resource objects, numeric/digit-string
idle days and ignored unused fields. Actual output schemas retain wire projection.
Only nine existing HTTP hints remain; get/history/read_file/restore/reassign are
native-only. No routes are invented.

Foundation parity covers all fourteen original/current handlers count=20, including
five lifecycle handlers x three states x six inputs. Typed binding parity covers
fourteen operations x three compatible argument sets count=20. The generic typed
failure-code envelope is normalized explicitly; result fields/domain errors remain
checked. Thirty-two foundation and eight binding mutations guard the contracts.

## Audit, history and identity evidence

Actual closed-journal fixtures proved all eight legacy mutations returned success
and changed skill state despite unavailable audit. Both actual host and dispatcher
regressions now require audit before any factory/store effect; six reads remain
unaudited. Real isolated primary/acme socket clients refuse tenant credentials for
all fourteen commands. Operator requests with an unused tenant argument still
select the primary list/mutation store and preserve tenant-private records.

Real owned corrupt JSONL journals previously produced successful empty/partial
history. The service now forwards the original Range failure and discards failed
partial output. Permanent native empty/partial fixtures and exact-cause/zero-output
service cases pass; three independent mutations fail. Malformed individual payload
filtering, chronology, row projection and nil empty history remain compatible.

Eight direct caller paths and eight real native fixtures retain one trusted ordered
invocation -> domain event -> completion identity. Each correlation comes from the
host context, never client input. Context-free callers retain their empty legacy
correlation. Native ownership history includes both shared/reassigned events with
original event IDs/correlation/sequence; the CLI already had their renderers.
Ten independent mutations cover all eight identity paths and both history kinds.

## Exit validation and remaining boundaries

Source/service/store/native/registry/tenant/audit/history/identity and aggregate exit
suites pass count=20. CLI skill/rollback/compare and package race suites pass
count=20. Final full Go tests/build/vet, scoped staticcheck, formatting,
architecture/deadcode/dependency/documentation/changelog and official structure
gates pass: 226 packages, 141 import/13 call exceptions, 28 public SDK findings plus
one test seam, 24 justified dependencies and 117 generated kernel packages.
CP -> skill debt remains in roster teardown sources; no allowlist exception is
removed early or expanded. Knowledge/runtime module dissolution belongs to W3.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 8096 / 7097 / 6313 ns/op; 7606 / 7609 / 7607 B/op; 89 / 89 / 89 allocs/op.
All satisfy the <50 us framework dispatch budget. Construction and real audit/
journal I/O are excluded; this is not whole-operation roundtrip timing.

Fixtures use owned TempDir stores/journals, mock providers and socket clients.
No live provider, physical browser or generated transport/SDK coverage is claimed.
A transient local Windows bind failure in the history test round did not reproduce
in the isolated 20-repeat rerun; final full gates passed. Protected delivery still
requires all final-head CI checks before normal merge.

Next: board, then workboard/OKR/storage/artifacts in roadmap order 3.
