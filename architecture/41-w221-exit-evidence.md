# W2.21 MCP native exit evidence

Prepared on shared main. Protected delivery remains local: Git metadata is read-only,
and authenticated GitHub create_tree requires approval under never policy. No new
remote tree/commit/ref was created. This closes six native operations, not live MCP
peers/processes, all REST/SSE/OpenAI transports, client numeric codecs or all modules.

| Operation | Primary aggregate typed policy |
|---|---|
| mcp_list | Unaudited unary catalog read |
| mcp_add | Mandatory audited unary registration |
| mcp_attach | Mandatory audited unary peer attachment |
| mcp_detach | Mandatory audited unary live-attachment removal |
| mcp_set_enabled | Mandatory audited unary startup-flag change |
| mcp_remove | Mandatory audited unary registration removal |

## Registry and ownership

TestMCPNativeSixOperationsTypedPolicyAndTypes pins all six exact signatures, their
single AppOwned native registration, read/write, authorization, primary tenancy,
unknown-input and unary metadata. Six independent production registry omissions fail
that test. Adding either private env or headers response fields independently fails
the typed row schema contract. Production source bytes restore before exit checks.

MCPCatalog owns selected registration/attachment snapshots and the public view.
MCPLifecycle owns five selected writer calls, public lifecycle outputs, admission
prechecks and operation identity. Runtime retains durable registration validation,
store writes, transport selection/dial, concurrent attachment resolution, attachment
names, detach/close, remove and domain publication. It has no new public test-only
production seam or dependency. Native mcp.go/list/lifecycle wrappers and manual
registrations are removed. No allowance or app-to-runtime/agent import is added.

The official archcheck writer removes controlplane-to-mcp adapter bypass exactly,
with no additions and byte-identical call ratchet. Earlier toolforge deletion removed
its separate bypass. Current counts:236 packages,137 import/13 call exceptions,
28 public SDK findings plus one cross-package test seam,24 justified dependencies,
127 generated kernel packages.

## Public shapes and compatibility

MCPServerView has16 public properties, seven required id/name/enabled/created_ms/
updated_ms/transport/attached and nine optional command/args/url/description/lazy/
tool_allow/env_keys/header_keys/tool_count. Private environment/header values are
absent from its Go model and schema. Sorted nonempty key names remain public;
transport still follows raw nonempty URL and optional strings remain untrimmed.
Args, allowlists, key-name arrays and live-count pointers are owned. Pointer tool_count
keeps attached0/negative counts present, versus detached absence. List preserves
three required roots, [] empty server output, registration order, initial orphan-
inclusive attached count and fresh attachment snapshots for each row.

Five lifecycle codecs preserve raw refs, missing/null/error precedence, server null
as a zero registration passed to validation, and the historical enabled bool or
case-insensitive true/exact1 string conversion. Whitespace is not trimmed for enabled;
other/missing/null types choose false. Add forwards full transport values only to
the writer. Attach tools preserves null/empty/order; server/tools/detached/removed
roots remain required even for nil/false outputs. Unknown fields remain accepted.

54 normal catalog cases20 are byte-exact; nine large-timestamp catalog cases20
permit only asserted timestamp lexeme corrections (total63). The old JSON-to-map
view rounded2^53+1/maxint64; the int64 view now preserves both, proven by actual
unchanged before-fail/after20 native wire and source tests. Native terminal Number
preservation is shared framework behavior; this is not a client float-decoder fix.
The shared projection also renders lifecycle timestamps without the prior rounding.

Foundation and final typed lifecycle each have345 owned native response/store/
attachment/close comparison cases20. Assigned IDs/wall-clock times alone are
positively asserted then normalized; all other semantic values are compared. The
foundation's first5m proof budget expired after passing repetitions; its retained log
is excluded from acceptance. Its unchanged final count20 proof passes with15m, and
final typed dispatcher count20 passes separately. Production/CI timeouts are unchanged.
Both stdio/HTTP dialers are mocked; no remote tool call or real child/peer is made.

## Explicit admission, identity and context refinements

Actual old native proofs fail for all five closed-journal effects, five pre-canceled
effects, five empty domain correlations and background attach context. The same16
permanent cases pass20 after typed binding. Mandatory audit admission now prevents
writer/store/dial/close effects; pre-canceled callers stop before audit/factory.
Direct service methods reject pre-canceled contexts and prefer operation-owned
correlation over explicit fallback. Old-service cancellation/identity proofs fail;
unchanged after20 and individual writer mutations pin both corrections.

Caller values, deadline and identity reach the attach dialer. A blocking mock dial
cancels before attachment/state change and leaves one joined failed audit pair.
Remove's detach and remove domain events share the operation identity. Settlement
failure after an applied writer reports failure without claiming rollback. Runtime's
best-effort domain publication and in-progress non-context writer semantics are
unchanged; these checks do not prove cancellation/atomicity of every real transport.

Canonical metadata, typed schemas, seven public row/three list/required lifecycle
roots, live count/null-tools,50 codec cases,20 admission cases, unknown input,
factory context, tenant denial and audit settlement contracts pass20. Actual native
six-command tenant denials preserve primary registration/attachments/journal without
provider or dial activity; real owned local sockets are used for those denial checks.

## Validation and remaining scope

98 valid wave mutations: a16,b27,c12,d31,exit12. Initial unused-port compilation fixtures
are excluded; an initial d tools-root mutation survived a weak assertion, which was
strengthened with explicit required-root declaration checks. The final serial31 run
alone counts for d. Every production Go source path and byte matches the full validated
d state after exit mutations; exit adds two native regression test files and a direct CLI protocol test file.

Full all-Go/build/vet/static, focused source/native and related Tool/Compare CLI repeats, related race20 and
whole controlplane race1 pass at d. Exact original runtime Attach/Detach/
RemoveAndAttachEnabled/Allowlist/BridgedTool fixtures and the entire core MCP package
also pass20/race20: these name-based selectors cover tests the generic MCP regex
misses (all-Go1 already covered them). Exit repeats app/native MCP20 and race20,
whole controlplane race1, CLI, static/architecture/dead-code/dependency/official
structure/2771-file formatting gates; unchanged production does not require another
full Go writer. Docs/changelog, owned working-source and committed-range gitleaks,
diff/index and frozen delivery-chain checks pass. The committed-range scan covers
only the two old local commits, not the unpublished working tree; the owned scan
covers the current source and documents. Unrelated security-report deletions remain
outside delivery, and generated structure evidence uses its official writer.

The earlier generic MCP CLI selector ran no MCP tests; related Tool/Compare checks
were not direct MCP CLI coverage. Exit adds10 canned-endpoint CLI wire/alias/rendering
cases (all six native commands) and16 early malformed-argument cases, repeated20 and
race20 with the entire CLI package1/static validation. Four independent production
CLI mutations change remove command, enabled value, URL priority and raw arguments;
all fail the direct request assertions. Only owned runtime address/token files and a
canned local TCP endpoint are used; no real daemon, MCP peer, process or store starts.

BenchmarkDispatchWithoutAuditIO, three100ms runs: 6781/7627/6871 ns/op, 7607/7610/7611 B/op, 89/89/89 allocations; all below50us.

The benchmark is generic no-I/O mock-host dispatch. It excludes mandatory journal,
socket, catalog lookup, registration validation, MCP handshake/tool execution and
real provider latency. Owned TempDir stores/journals, local sockets and mock ports
support these proofs. No live MCP peer, spawned child, running daemon restart or
generated SDK publication is claimed. Final-head CI/main merge remains pending at
the recorded permission gate. Next: market/plugin order5, then remaining runs.Start,
W3 modules, W4 triggers, W5 surfaces and broader transports.
