# W2.26 configcenter nine-operation native exit evidence

Prepared on shared main. This closes configcenter native app migration and its CLI
verification; remaining order6, every transport and W0-W5 stay open. Git metadata is
read-only; prior GitHub tree publication required approval under never policy. No
remote tree/commit/ref/merge was written. Protected final-head CI/main publication
remains a separate incomplete delivery step.

| Operation | Native canonical primary unary policy |
|---|---|
| configcenter.get | ReadOnly unaudited entry read |
| configcenter.list | ReadOnly unaudited rating-filtered entries |
| configcenter.access-log | ReadOnly unaudited access history |
| configcenter.audit | ReadOnly unaudited audit history |
| configcenter.health | ReadOnly unaudited availability/stats |
| configcenter.set | Mandatory audited raw validation/persist/reread |
| configcenter.delete | Mandatory audited deletion |
| configcenter.set-rating | Mandatory audited classify/override/persist |
| configcenter.access | Mandatory audited ACL replace/persist/reread |

## Ownership, native binding and shapes

[Exact native metadata test](../kernel/controlplane/configcenter_typed_app_test.go)
pins nine unique AppOwned registrations/signatures, ReadOnly/Authz/Tenancy/unary and
unknown-input compatibility, plus no emission type/schema. Every operation and the
native aggregate independently omitted causes a permanent test to fail. Four native
handler/helper/registrar files are removed; selected Reader/Writer ports remain.
app/configcenter owns presentation and operator business ordering. Lower Center owns
CRUD/persistence/vault/classifier/agent ACL/policy/HITL/access events/log filters.
GetAutoRating is the existing exact Classifier.Classify forwarder. No new dependency,
allowance or layer bypass. Core loader's existing version reset through Store.Set is
preserved and explicitly tested; this migration does not redesign persistence.

EntryRow has12 properties/6 required key/value/rating/created_at/updated_at/version.
Secret values use creds.MaskValue and true-only optional masked. Description/tags/
policy/ACLs retain nonempty optional shape; Tags retains borrowed-slice semantics,
ACL output arrays copy. Raw strings/zero/negative/large int64 lexemes remain exact.
Get/Set/Access echoes have1 required root; List/AccessLog/Audit have2 roots and [] empty
collections. Access rows8 required, Audit rows9 required. Delete/rating boolean roots
stay present even false. Health has3 properties/2 required: unavailable omits stats,
healthy includes nil/empty/object stats and original dynamic map data. Native numeric
regression pins9007199254740993,0 and-2 through the generic terminal codec.

Per-command RawMessage requests and strict codecs retain missing/null/non-string/
blank ordering and raw text, unknown/irrelevant input compatibility. Set/List rating
errors keep short text and lower-case without trim; SetRating keeps ParseRating's
longer error. ACL input preserves comma/semicolon/space/newline/tab delimiters (not
CR), trim, case-folded dedup with first casing/order, dropping non-string members and
nil versus [] empty. Set defaults internal, constructs entry before availability,
persists then re-reads/masks. Rating lookup translates missing before classification,
reports override including false, mutates then persists. Access replaces ACLs without
changing other fields, persists/re-reads; Delete forwards raw keys and causes. Write
and post-write-read errors preserve already-applied effects; no rollback is added.

## Admission, audit and parity

The unchanged13-case real native proof fails20 before and passes20 after canonical
binding: four closed-journal writers and all nine pre-canceled calls reject before
state effects/provider calls. Canonical port counters independently pin no factory/
effects on tenant/pre-canceled rejection or failed mandatory audit, and preserve
caller context on admitted operations. Actual tenant socket tests cover all nine.
Direct services and in-progress cancellation retain legacy behavior. Four actual
writer fixtures close journal after persistence, return terminal audit failure and
retain in-memory plus reloaded disk effects. A zero-kernel writer with no audit host
is rejected at mandatory admission; normal parity uses real initialized runtime.

550 read foundation and368 writer foundation native cases20 preceded typed binding.
896 normal typed native cases20 cover28 input variants across empty/public/secret
manager states for nine operations plus unavailable manager for five reads. Complete
response/state/disk bytes match after independently asserting entry timestamps within
each operation interval, then replacing only created_at/updated_at clocks. State
snapshots sort map-backed entry lists; response bytes otherwise remain unnormalized.
Writer invocation/terminal audit kind, op/actor/subject, nonempty joined correlation
and key/value redaction are independently asserted after connection completion.
Reads remain unaudited/provider zero. Core agent allow/deny/config.access/secret mask
regressions stay green. These owned local stores do not exercise deployed vault/HITL.

## CLI proofs and corrections

[CLI fixture/regressions](../cmd/agt/configcenter_native_test.go) use owned loopback
TCP/token/home and validate every canned success result against the native operation
schema. No daemon, provider, external endpoint or owner's home is used. Exact status
probe then operation/token/args are checked.68 new cases pass20/race20:5 normal
formatter/filter before-proof cases,6 missing-value cases,25 rendering/empty/error
branches and23 invalid/9 help no-dial cases. The same initial11-case proof is red20
before and green20 after; initial harness encoding/nil-versus-empty expectations were
corrected and excluded from findings.

Entry/access/audit Unix seconds previously displayed1970 because CLI used UnixMilli.
Nonempty audit output asserted nonexistent actor/action and panicked; it now reads
agent_id/decision/policy/reason from the actual nine-field row. Access-log/audit first
validation loops rejected value tokens before parsing flags; single-pass parsers now
consume them. Missing/adjacent flag values in those filters/list/rating reject before
dial. Positional and flagged ratings remain supported, raw filter strings remain;
CLI set bytes/body/default internal/raw value/ACL flags are unchanged. Dedicated
native configcenter.access has no CLI subcommand; CLI set ACL flags keep existing
behavior, while actual native socket tests cover the standalone access operation.

## Mutation verification and dispatch measurement

20 read +17 writer +28 typed +13 CLI +10 exit omissions =88 final valid mutations.
All produce permanent test failures and source restores byte-for-byte. An initial
missing-get-key survivor was strengthened; unused-variable mutation fixtures were
corrected and excluded from product findings. Retained logs distinguish those stages.

Nine full typed Dispatch operations, fake Center and noop audit, Windows/amd64,
GOMAXPROCS=4,200ms x3; includes decoding, schema checks and service/presentation.
Excludes journal/persistence/network/vault/provider I/O. Every run is below the
roadmap's50us/op pipeline budget; these are local fixture measurements.

| Operation | Min ns/op | Max ns/op |
|---|---:|---:|
| configcenter.get | 16357 | 22245 |
| configcenter.list | 18021 | 20892 |
| configcenter.access-log | 11788 | 22523 |
| configcenter.audit | 16581 | 24161 |
| configcenter.health | 21017 | 25116 |
| configcenter.set | 28891 | 33862 |
| configcenter.delete | 11487 | 14454 |
| configcenter.set-rating | 10300 | 14516 |
| configcenter.access | 24270 | 31266 |

Fake Center ports and noop audit only; GOMAXPROCS=4/windows/amd64/200ms x3. No I/O or live daemon budget claim.

## Validation and remaining work

Full CLI20, new CLI race20, app/core/native-source20, MaskValue20, app-core race20,
whole controlplane race1, all Go count1, build/vet/scoped staticcheck, arch/dead/deps/
gofmt/generated structure and final doc/changelog/gitleaks/diff/index checks pass.
241 packages,136 import exceptions/13 call sites;28 SDK findings+1 test seam remain,
24 core dependencies;132 generated kernel packages/2831 current Go files. Generated
architecture evidence uses the official writer. Current cumulative payload and scoped
code/docs/PR plus proofs/mutation/benchmark/full gates live in ignored durable
.temp_files/architecture-delivery/w46d; historical Temp patch gaps remain documented.

Next: channel(s), webhook, tunnel and update in order6; then order7. Broader tool/
agent-loop/runs.Start convergence, module dissolution, raw-ref GC, triggers, generated
surfaces/other transports remain open. Local contract evidence does not certify live
vault/HITL, deployed daemon behavior or protected final-head CI/main publication.
