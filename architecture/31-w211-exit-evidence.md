# W2.11 OKR native exit evidence

Prepared locally: W2.11a-d code/docs are separate reviewable patches; protected
main delivery remains pending while Git metadata is read-only and GitHub
credentials are unavailable. Seven native OKR commands bind typed app/okr
operations through the common app host. Storage/artifacts, remaining native
domains, broader adapters/generated surfaces, runs.Start and W3-W5 remain open.

| Operations | Native scope and ownership |
|---|---|
| okr_list, okr_show | Two primary-only reads; selected store/live rollup, unaudited unary snapshots. |
| okr_create, okr_keyresult, okr_link, okr_unlink, okr_archive | Five primary-only mutations; mandatory audit before kernel/store/link/achievement effects. |

## Coverage and compatibility

TestOKRNativeExitCompleteTypedRegistry guards exact seven-operation common-registry
coverage, actual input/output types/schemas, AppOwned metadata, primary/read-only/
unary scope and legacy unknown-input compatibility. Family or archive removal
independently fails the exit regression. Old socket wrappers/registration and
objective view/response shims are removed. Correlation/integer helpers disappear
when their final OKR callers move; seat's string-list admission remains.

Typed records retain durable objective JSON plus required live progress, percent,
achieved and key-result count. Reads preserve filter/order/limit/count/empty-array
and show-error behavior. Live task completion and unlink tests distinguish current
rollup from cached objective status rather than substituting that cached status.

Lifecycle ports retain the real kernel facade, transitions, domain journaling,
original causes and returned-object projection. Linking an already-done task can
recompute durable achievement after the facade's returned snapshot was captured;
the native response projects that actual returned objective, without silently
refetching a different status. Actual kernel regressions guard this boundary,
create/KR/link/unlink/archive effects and six expected domain events.

Request DTOs retain lenient trimmed/nonstring title/ID/owner/tenant/link fields,
integer target admission (including zero/negative values passed to the kernel),
200 default/floor list limit, unknown status filters, strict booleans and ignored
unused fields. Native parity covers all seven handlers x three inputs count=20.
Only generated IDs, bounded clocks, common generic failure codes and the strict
boolean schema-message boundary are normalized; domain errors and remaining
result fields remain checked. A malformed boolean stays rejected, while its
error text now identifies the operation schema rather than legacy args parsing.

## Audit and identity evidence

Actual closed-journal regressions independently enumerate all five mutations,
ensuring unavailable audit prevents objective, KR, link/unlink, archive, rollup
and provider effects. Two reads retain no mutation audit and stay usable with a
closed journal. Actual primary/acme socket clients reject tenant credentials for
all seven commands without changing primary objectives. A tenant field in a
primary call remains an objective label/filter, not kernel routing.

Default domain events now share a single host-owned operation audit identity;
actual invoked/domain/completed records verify one ordered span. Explicit inbound
correlation remains the existing native bridge contract. Rollup and kernel event
semantics are retained by the service move; the later binding explicitly adds
mandatory audit admission before effects.

## Exit validation and boundaries

Twenty-six independent mutation proofs cover read/projection, lifecycle, native
binding and aggregate exit. Source/service/store/runtime/CLI/native/audit/tenant/
admission/identity/exit focused suites and related package race pass count=20.
Complete controlplane race and final full Go tests pass count=1; build/vet, scoped
staticcheck, formatting, architecture/deadcode/dependency/document/changelog and
official structure gates pass. Current counts: 229 packages, 140 import/13 call
exceptions, 28 SDK findings plus one test seam, 24 dependencies and 120 generated
kernel packages. The official archcheck updater removes the paid controlplane-to-
okr adapter edge (141 -> 140); no new exception is added. Package documentation
uses a dedicated doc.go so the official structure writer records the app purpose.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 9757 / 9743 / 12903 ns/op;
7608 / 7610 / 7609 B/op; 89 / 89 / 89 allocs/op. All satisfy the <50 us framework
budget. Construction and real audit/journal I/O are excluded; this does not
certify whole-operation roundtrip timing or provider latency.

Fixtures use owned TempDir stores/journals, mock providers and socket clients.
No live-provider, physical-browser or generated transport/SDK coverage is claimed.
Restricted-session validation uses task-owned Go/staticcheck caches; build exits
successfully with a warning about the unavailable shared module stat cache.
Protected delivery still requires all final-head CI gates before normal merge.

Next: storage/artifacts in roadmap order 3.
