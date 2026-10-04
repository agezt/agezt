# W2.1 operation framework exit evidence

W2.1 is the framework and the status/version pilot. Domain migration, generated
surfaces and additional transport adapters remain separate roadmap work.

| Requirement | Current implementation and executable evidence |
|---|---|
| Pure operation contract | `contract/opapi` contains metadata, actual input/output/emission types, caller/principal and auth/router/audit/emitter ports. Its imports are stdlib only; archcheck enforces the contract layer. |
| Typed immutable registration | `app.NewOperation` and `NewStreamingOperation` bind actual Go types and owned schemas. Registry construction rejects missing dependencies, unbound/duplicate operations and incompatible tenant metadata. `TestDispatchOrdersAdmissionBeforeEffects` and registry checks in `TestReadOnlyTenantAndStreamingContracts` cover registration and effects. |
| Ordered admission | Authentication, lookup/tenant denial opacity, authorization, routed context, decode/schema validation and cancellation precede effects. `TestDispatchOrdersAdmissionBeforeEffects` exercises failures and the successful trace; nullable/map/embedded/custom-schema tests dispatch actual typed values. |
| Mandatory mutation audit | App dispatch admits only after a successful owned audit span, contains handler panics, settles typed causes and joins terminal audit errors. `TestAppHostSocketMutationAndStreaming` uses the production registrar/factory/socket adapter and real isolated journals: one invocation before effects, one terminal event, no effects on preflight failure and no success response on terminal persistence failure. AppOwned registrations skip legacy socket audit; other commands retain it. |
| Tenant host and privacy | `TestAppHostTenantAuditUsesRoutedJournal` covers tenant and operator-selected tenant requests, rejects a foreign tenant and keeps primary/tenant journals separate. Socket fixtures retain scalar audit metadata and redact secret arguments using the existing sanitizer. Handler actor/correlation context matches its operation arc. |
| Independent streaming contract | Emission and terminal types/schemas are independent and copied. `TestOperationIndependentEmissionAndTerminalContracts`, custom-frame tests and unary-mode tests retain derived/explicit validation and transport causes. Native binding requires object result schemas and kernel Event frames; unsupported wire shapes fail before effects. `TestAppHostLiveDisconnectSettlesOwnedAudit` exercises the existing live socket cancellation wrapper and durable failed settlement after disconnect. |
| Transport-independent pilot | `app/system` status/version accept context and typed inputs and return typed output/error. Old socket handler functions are removed; the compatibility adapter binds fresh host metadata and encodes Result. Source status/version/tenant/registry/audit suites and socket-free handlers retain optional omission, enabled zero tenants, fallback dimensions, empty-head clamp and build provenance. |
| Dispatch budget | `BenchmarkDispatchWithoutAuditIO`, Windows amd64, GOMAXPROCS=4, three runs: 6,912 / 7,140 / 9,011 ns/op, 7,608–7,609 B/op, 89 allocs/op. This is 6.9–9.0 us/op against the <50 us budget. Construction and real audit/status/journal I/O are excluded. |

## Schema boundaries verified at exit

The derived subset includes scalars, exported JSON fields/tags, nested objects,
nullable pointers/slices/byte slices/maps, typed map values and anonymous/named
embeddings with encoding/json dominance and conflict omission. Required fields,
unknown-field admission, enum, items and additionalProperties use the existing
dependency-free validator.

Custom JSON and text codecs require explicit schemas. Exit checks reproduced
`netip.Addr` being falsely derived as an empty object while encoding/json writes a
string; value/pointer/nested/collection representations now fail derivation, and
explicit string schemas decode/dispatch/encode through the real codec. Separate
encoding-only and decoding-only mutation guards fail independently.

Recursive derivation, private embedded-pointer allocation and quoted json tags
remain explicit representation boundaries. An explicit schema overrides
derivation; it does not implement a new Go decoder. Agent/system principal kinds
remain denied by the current operator/tenant policies until their host adapters
and operation policies are introduced. Native wire constraints belong to the
control-plane adapter, not to the generic app emitter.

## Validation and delivery scope

App/schema contracts and source status/version/registry/auth/tenant/audit/host
suites pass count=20 with isolated mock providers and TempDir homes. Full
`go test ./... -count=1`, build, vet, focused staticcheck, gofmt, archcheck,
deadcodecheck, depscheck, docclaimscheck, changelog-lint, official structure check
and commit-range gitleaks pass. Architecture counts remain 218 packages,
145 allowlisted imports and 13 call sites; no allowlist expansion.

The status/version pilot is the shipped production domain. Mutation/stream
fixtures bind test operations through the production registrar, dispatcher
factory and real socket path; this does not claim that those existing domains
have migrated. No live provider, owner home or password-protected console test
was run. Protected delivery requires every final-head check, including all three
Go gates, before normal merge.

Next: catalog/provider domains in roadmap order, then the remaining domains,
generated surfaces, broader adapters and W3–W5. The overall migration remains open.
