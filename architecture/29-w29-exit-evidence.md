# W2.9 board native exit evidence

All seven board native commands bind typed app/board operations through the common
host. This completes board in roadmap order 3; workboard/OKR/storage/artifacts,
remaining native domains, broader adapters/generated surfaces, runs.Start and
W3-W5 remain open.

| Operations | Native scope and ownership |
|---|---|
| board_read, board_help, board_inbox, board_get, board_replies | Five primary-only reads; selected shared instance or fresh fallback, no mutation audit. |
| board_send, board_ack | Two primary-only mutations; shared instance required, mandatory audit before factory/store/notifier effects. |

## Coverage and compatibility

TestBoardNativeExitCompleteTypedRegistry guards exact seven-command coverage in
the common registry, actual input/output types/schemas, AppOwned metadata,
primary/read-only scope and StreamNone. Removing the family or ack operation
independently fails the exit regression. Old native business wrappers,
registrations, message projection and unused limit-admission helper are removed.

Typed records retain required topic/text/ts_unix_ms, optional identity/addressing/
help and owned acknowledgement slices. Read preserves full-store collection before
cursor filtering, descending timestamp/ID tie order, pre-filter total, actual topic
count map, optional next cursor and admitted zero as unbounded. Help/inbox/replies/
lookup retain case, missing-message, ack idempotence, chronology and empty arrays.

Send preserves reply -> help -> broadcast -> DM -> topic-post precedence, original
reply topic/recipient, source error causes and one success-only notification.
The explicit inbound correlation is a bridge contract: it is retained in notifier
and optional result, with the existing daemon notifier semantics. This migration
does not overwrite that correlation with the operation audit ID or redesign its
separate callback API.

Typed admission preserves lenient trimmed/nonstring text/ID/address fields and
limit admission (50 default, 500 cap, positive fractions truncate to zero/unbounded),
strict bool/read-string fields and ignored unused fields. Host factories preserve
shared-only writer availability and selected-reader errors. Only four existing
HTTP hints remain; inbox/get/replies stay native-only.

Seven old/current foundation handlers match count=20 across admission/pagination/
routing/ack/unavailable-writer/fresh-reader cases. Typed binding parity also passes
count=20 over compatible inputs. Fresh sent IDs and bounded creation clocks are
normalized; the generic typed failure-code envelope is an explicit boundary while
result fields/domain errors remain checked. Ten foundation and eight binding
mutations guard paging, projection, routing, notifier, error, audit and admission.

## Audit and host evidence

Real closed-journal fixtures proved legacy send/ack changed shared state and
returned success despite unavailable audit; send also notified. Permanent actual
host and dispatcher regressions now block both mutations before factory/store/
notifier effects. Five reads remain available without mutation audit. Selected
shared-store tests and fresh-open fallback tests retain writer ownership; unknown
or missing shared writers cannot silently open a competing instance.

Actual primary/acme socket clients reject tenant credentials for all seven commands
without changing the shared board or notifying. Operator calls with an unused
tenant argument still read/write the daemon's selected primary board.

## Exit validation and boundaries

Source/store/service/native/registry/tenant/audit/fallback/notifier and aggregate
exit suites pass count=20. Package race suites pass count=20. Final full Go tests/
build/vet, scoped staticcheck, formatting, architecture/deadcode/dependency/
documentation/changelog and official structure gates pass: 227 packages, 141
import/13 call exceptions, 28 SDK findings plus one test seam, 24 dependencies,
118 generated kernel packages. Roster still owns board limit constants and legacy
adapter debt; no allowlist exception is removed early or expanded.

BenchmarkDispatchWithoutAuditIO, three 100 ms runs: 7404 / 11799 / 8975 ns/op; 7609 / 7609 / 7612 B/op; 89 / 89 / 89 allocs/op.
All satisfy the <50 us framework dispatch budget. Construction and real audit/
journal I/O are excluded; this is not whole-operation roundtrip timing.

Fixtures use owned TempDir stores/journals, mock providers and socket clients.
No live provider, physical browser or generated transport/SDK coverage is claimed.
Protected delivery requires all final-head CI gates before normal merge.

Next: workboard, then OKR/storage/artifacts in roadmap order 3.
