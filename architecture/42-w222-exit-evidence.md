# W2.22 market native exit evidence

Prepared on shared main. Protected delivery remains local: Git metadata is read-only,
and authenticated GitHub create_tree requires approval under never policy. No new
remote tree/commit/ref was created. This closes eight native operations, not every
HTTP/SSE/OpenAI/channel binding, live marketplaces/MCP processes or the full roadmap.

| Operation | Primary aggregate typed policy |
|---|---|
| market_list | Unaudited unary catalogue read |
| market_show | Unaudited unary detail/manifest/review read |
| market_sources | Unaudited unary source read |
| market_install | Mandatory audited StreamEvents materialization |
| market_uninstall | Mandatory audited StreamEvents reverse materialization |
| market_add_source | Mandatory audited unary source registration |
| market_remove_source | Mandatory audited unary source/cache removal |
| market_sync | Mandatory audited unary source sync/partial results |

## Registry, ownership and preserved shapes

TestMarketNativeEightTypedOperationsPolicyAndSignatures pins exact eight input/output
signatures, one AppOwned registration per name, primary authorization/tenancy,
read-write/stream/no-emission metadata and unknown-input compatibility. Eight
independent production operation omissions fail that test. Actual owned tenant
socket tests deny all three reads/five writes without changing store/journal,
provider or live attachments; unary and stream socket paths are covered.

app/market Reads owns typed catalogue/detail/source snapshots. Writes owns selected
core calls, public result/progress/publication/partial presentation and admission/
identity boundaries. Core market retains library resolution, cache/provenance,
validation, signature checks, default-allow informational vet, skill creation/
promotion, MCP registration, optional reverse APIs, source sync and network screening.
Native market.go, manual market registration and host helper shims are removed.
parseCapList remains verbatim in args.go; unused strArg/mustJSONRaw are removed.
ACP keeps structToMap. No new app-to-runtime/agent import, dependency or allowance.

The official writer removes exactly CP-to-market adapter bypass with no additions.
Counts:237 packages,136 import/13 call exceptions,28 public SDK findings plus one
cross-package test seam,24 justified dependencies,128 generated kernel packages.

Read codecs retain trim/missing/null/nonstring-to-empty and unavailable-before-name
errors. List keeps [] empty output, order, two roots and17-property/eight-required
Listing rows. Sources has two roots/four-property/two-required rows. Show keeps ten
roots,11-property/two-required full pack, three-property/one-required skill summaries,
raw full MD/base64 resource representation and informational review. Valid summary
name/description are both present, including empty descriptions; malformed summary
has neither. Tools keeps null versus [] and optional/zero/raw fields persist.
Authored pack MCP manifest values are unchanged; this is not the separate MCP
management redacted projection. Deep typed copies retain former projection ownership;
root tools now also owns its collection, shown red against the old alias.

Writer codecs retain raw native compatibility/required/availability order. Typed
install/source/remove/uninstalled/sync terminal fields keep optional arrays/flags,
zero/false/name/order and partial_error presence (pointer preserves an empty error
message when one exists). Error with zero sync rows fails without success publication;
nonempty rows plus error retain completion publication and partial_error. Install
publication uses the reported record fields, source-add the reported source/URL,
remove publishes even absent=false, and sync sums packs rather than counting rows.
Progress preserves transient envelope/kind/subject/actor and core.Event payload
(stage+ok required; name/detail optional), using specialized event.WireSchema.
No progress correlation field is added; durable records carry operation identity.

## Explicit corrections and regression evidence

Actual read before-proofs fail for five large projected integer fields and three
canceled native readers; installed_at was already exact and stays an exact control.
Typed fields preserve2^53+1/maxint64 native wire values. Actual root-tools alias
before-proof fails; typed ownership and independent clone mutations retain it.
Direct/in-progress non-context manager reads keep their earlier behavior. Client
float/JavaScript decoding is not repaired by this native wire correction.

Actual17 writer before cases fail: five unavailable-journal admissions, five
pre-canceled callers, five split audit/domain identities and two first-progress
write failures. Unchanged permanent after-proofs pass20. Shared mandatory admission
now stops before factory/store/materializer/fetch. All direct writers precheck caller
context, install/uninstall prefer owned operation identity over explicit fallback,
and publication joins audit identity; raw caller correlation cannot choose it.

New core InstallContext/UninstallContext accept context/error-returning progress,
check before subsequent effects and provenance, and propagate sink/caller failures.
Old void callback methods delegate through Background/error-free wrappers and retain
legacy behavior. First install vet-write failure prevents materialization. Uninstall
first-progress failure retains its first quarantine but stops later MCP/provenance
removal. Caller cancellation after a skill/reverse effect stops the next subsystem.
Domain+sink causes are joined; a gap introduced during implementation was tested red
and repaired before acceptance. Optional reverse APIs and best-effort reverse failures
remain; applied resources are not rolled back. A journal closed after first install
progress returns failure after effects/provenance; publication/settlement failure
propagates without implying atomicity or rollback.

360 foundation read cases20 are byte-exact. Final342 normal read cases20 are219
byte-exact plus123 allowing only asserted integer lexeme corrections. Eighteen old
canceled read parity cases are retained at a and superseded by admission proofs.
255 foundation writer frame/store/effect/fetch/domain cases20 are byte-exact.
Final240 normal writer cases20 allow only three asserted timestamp lexemes and
independently asserted old/current correlations. Fifteen old canceled writer cases
are retained at c and superseded by admission proofs. Owned checked-root files,
mock skill/MCP materializers and an in-memory HTTP RoundTripper support writer
parity; no real marketplace fetch, process, peer or provider call is made there.

## CLI stream repair and final validation

Direct CLI coverage found install/uninstall using Client.Call for StreamEvents,
which aborted on the first event with unexpected response type. The unchanged CLI
wire test failed before both calls changed to Client.Stream and passes20 afterward.
The terminal rendering/JSON output stays the same; intermediate frames are drained,
and errors after progress produce exit1 without rendering a success result.
Canned owned TCP endpoint tests cover16 normal/alias/render/native request cases,
16 early invalid/no-dial cases and two terminal failures after progress, repeated20
and race20. The endpoint records requests and returns frames; it is not a daemon,
store or MCP peer. Eight native omissions and four CLI regressions fail separately.

157 valid wave mutations: a24,b45,c35,d41,exit12. Compile/unused-variable fixtures,
proof-loop setup errors and a redundant guard survivor are excluded. Caller-context
disconnection and effect-phase tests strengthen the final d control coverage; its
final serial41 run alone counts. Every production byte/path matches the saved exit
state after mutations. Relative to accepted d, only the explicit CLI stream repair
changes production. Two exit regression files add native/CLI coverage.

Full CLI/core/app/native20, CLI/related race20, whole controlplane race1, all Go/build/
vet/static (including CLI), architecture/dead-code/dependency/official structure/
2787-file formatting gates pass. Docs/changelog/current owned working-source and
committed-range gitleaks/diff/index/frozen-chain checks pass; committed-range covers
only the two old local commits, not unpublished source. Unrelated security-report
deletions remain outside delivery. Generated structure uses its official writer.

BenchmarkDispatchWithoutAuditIO, three100ms runs: 7005/6241/6594 ns/op, 7608/7607/7610 B/op, 89/89/89 allocations; all below50us.

The benchmark is generic no-I/O mock-host dispatch; it excludes mandatory audit,
socket, catalogue lookup, cryptographic checks/materialization/sync and real service
latency. No live marketplace/signing authority/host installation/MCP child/daemon
restart/generated SDK claim. Final-head CI/main merge remains at the recorded
permission gate. Next: plugin inventory in order5; full runs.Start/invocation,
W3 modules, W4 triggers, W5 surfaces and broader transports remain open.
