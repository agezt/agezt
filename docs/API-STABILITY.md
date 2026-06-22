# AGEZT API Stability and SDK Parity

This document defines the compatibility expectations for AGEZT's public and semi-public interfaces. It is intentionally conservative: an interface is not called stable just because it exists. Stability means consumers can build on it without tracking every daemon commit.

## Stability labels

| Label | Meaning | Breaking-change rule |
|---|---|---|
| **stable** | Safe for external automation. Shape changes require versioning or a compatibility shim. | No breaking change without a new version or explicit migration path. |
| **beta** | Intended for external use, but the shape may still evolve. | Breaking changes are allowed, but should be documented and paired with release notes. |
| **compatibility target** | AGEZT emulates another API surface closely enough for clients to use it, but maps into AGEZT semantics. | Preserve common client behavior; document intentional divergences. |
| **internal** | Daemon/UI/CLI implementation detail. External callers should not depend on it. | May change at any time. |
| **experimental** | Feature exists to validate a design. | May change or disappear. |

## Surface matrix

| Surface | Status | Consumers | Notes |
|---|---|---|---|
| REST `/api/v1/*` SDK surface | **beta** | Python/TypeScript/Rust/Go SDKs, external tools | Intended external surface for daemon automation. Keep additive changes preferred; breaking changes should use `/api/v2` or a documented migration. |
| Web UI private APIs (`/api/*`) | **internal** | Embedded React console | These endpoints optimize the console and may change with UI releases. External integrations should use SDK/API surfaces instead. |
| OpenAI-compatible `/v1/chat/completions`, `/v1/responses`, `/v1/models` | **compatibility target** | OpenAI clients, IDEs, SDKs | AGEZT maps OpenAI-shaped requests into governed AGEZT runs. The mapping is intentionally lossy: AGEZT owns routing, policy, journal, and tool governance. |
| Control-plane protocol (`runtime/control.addr`, `control.token`, `controlplane.Cmd*`) | **internal** | `agt`, daemon internals, tests | Local daemon management protocol. Use `agt` or REST/SDKs instead of depending on the raw TCP protocol. |
| Plugin protocol (`kernel/plugin` JSON-RPC-ish stdio protocol) | **beta** | Out-of-process AGEZT plugins | Intended for plugin authors, but still evolving. Keep method names stable where possible; version plugin manifests as the protocol grows. |
| Plugin registry/index format | **beta** | `agt plugin registry`, marketplace tooling | Install flow verifies BLAKE3 pins before writing binaries. Index trust/signing policy may evolve. |
| MCP bridge | **beta** | MCP servers and tools | MCP is treated as an adapter surface. Security depends on the configured MCP server and transport. |
| SDK package APIs | **beta** | App developers and automation scripts | SDKs should follow semver and document parity with `/api/v1`. |
| Webhook/channel inbound payloads | **beta** | External messaging systems | Channel-specific payload handling may evolve. Public channel setup docs should note required fields. |
| Workflow graph JSON | **beta** | Web UI, workflow import/export | Intended to become stable after graph schema versioning is explicit. |
| Event/journal subjects | **beta** | Audit tooling, `agt why`, demos | Subjects are central to observability. Prefer additive fields; avoid renaming subjects without migration notes. |
| Frontend component props/types | **internal** | Web UI only | Not a public component library. |

## Current SDK versions

| SDK | Package | Version | Runtime/dependency posture | Status |
|---|---|---:|---|---|
| Python | `agezt` | `1.1.0` | Python `>=3.9`, stdlib-only runtime | beta |
| TypeScript/JavaScript | `@agezt/sdk` | `1.1.0` | Node `>=18`, platform fetch | beta |
| Rust | `agezt` | `1.0.0` | Rust `>=1.70`, stdlib-only runtime | beta |
| Go | `github.com/agezt/agezt/sdk` | module-coupled | Same Go module as daemon | beta |

SDK versions do not currently imply strict feature parity across languages. Treat this table as the compatibility baseline, not a promise that every SDK exposes every endpoint.

## SDK parity policy

For each externally supported `/api/v1` feature, SDK parity should be tracked across four dimensions:

| Dimension | Required for parity |
|---|---|
| Request/response types | SDK exposes typed request and response structs/classes/interfaces. |
| Authentication | SDK supports the documented token/header model without putting secrets in URLs. |
| Errors | SDK preserves AGEZT error codes/messages in a catchable form. |
| Tests | SDK has at least one unit or integration test for the feature. |

A feature is **SDK-complete** only when Python, TypeScript, Rust, and Go either support it or explicitly mark it unsupported with a reason.

## Versioning policy

### REST API

- Prefer additive changes: new fields, new endpoints, new enum values.
- Do not remove or rename fields from `/api/v1` without a compatibility window.
- For unavoidable breaking changes, add `/api/v2` or keep a translation shim.
- Error responses should keep a consistent shape: `{ "error": { "code", "message", "details"? } }` where practical.

### OpenAI-compatible API

- Preserve common OpenAI client expectations for route names, streaming shape, model listing, and error status codes.
- Document intentional divergences, especially where AGEZT governance overrides raw OpenAI semantics:
  - caller `model` may be routed by the Governor,
  - messages collapse into an AGEZT intent,
  - tools pass through Edict, approvals, budget, and journal.

### Control-plane protocol

- Treat raw control-plane commands as internal.
- CLI commands (`agt ...`) are the operator contract.
- If a control-plane response changes, update the corresponding CLI and tests in the same commit.

### Plugin protocol

- Keep existing method names stable where possible (`init`, `tool/list`, `tool/invoke`, `host/invoke`).
- Add manifest/protocol version fields before introducing incompatible plugin behavior.
- Preserve BLAKE3 pinning and allowlist enforcement as non-negotiable security controls.

### SDKs

- Use semver per package.
- Patch: bug fix, docs, non-breaking typing improvement.
- Minor: additive endpoint or optional field support.
- Major: breaking public SDK API change.
- Keep package READMEs aligned with supported surfaces and daemon version expectations.

## Public vs private guidance

Use this rule of thumb:

- If a human types it through `agt`, it is an **operator contract**.
- If an app calls it through an SDK, it should map to `/api/v1` and be **beta-or-better**.
- If only the Web UI calls it, it is **internal** unless explicitly documented otherwise.
- If a plugin process calls it over stdio, it is **plugin beta**.
- If a test reaches it directly, that does not make it public.

## Release checklist for API changes

Before merging a change to any external or beta surface:

- [ ] Update this stability matrix if the surface status changes.
- [ ] Add or update tests for the CLI/REST/SDK path that consumes it.
- [ ] Document breaking changes in `CHANGELOG.md`.
- [ ] Confirm SDK parity status: supported, unsupported-with-reason, or follow-up issue.
- [ ] Confirm auth behavior: no secrets in URLs unless the transport limitation is explicitly documented.
- [ ] Confirm audit behavior: externally triggered actions still produce journal/correlation evidence.

## SDK parity report

The generated parity report lives at `docs/SDK-PARITY.md` and is checked by:

```bash
go run ./tools/sdkparity -check docs/SDK-PARITY.md
```

Regenerate it after `/api/v1` route or SDK coverage changes:

```bash
go run ./tools/sdkparity -out docs/SDK-PARITY.md
```

The report is static route-string coverage only. It does not replace behavioral SDK tests.

## Known gaps

These are not blockers for using AGEZT, but they matter for platform positioning:

1. **SDK parity is split between static route coverage and behavioral conformance.** `docs/SDK-PARITY.md` distinguishes REST SDK route-string coverage from behavioral SDK tests, documents the native Go SDK's local control-plane path, and excludes implemented admin update routes from SDK coverage totals. New SDK-intended features still need per-SDK behavioral tests for typed request/response shape, auth behavior, error behavior, and streaming/event semantics before they are SDK-complete.
2. **Web UI APIs are broad and internal.** External callers may discover them, but they should not depend on them unless promoted to `/api/v1`.
3. **Plugin protocol versioning is explicit, but compatibility policy should stay documented.** The protocol has machine-checkable version fields; docs should continue to spell out compatibility expectations for plugin authors.
4. **Event/journal compatibility is policy-documented.** Events are central to `agt why` and demos; `docs/EVENT-SCHEMA.md` defines append-only compatibility rules and migration expectations. A global numeric schema version remains deferred until a concrete breaking migration requires it.
5. **OpenAI compatibility is behavioral, not identical.** AGEZT deliberately routes through governance, policy, and journal; this should stay visible in docs.

## Bottom line

AGEZT should treat public stability as a product promise, not an accident of implementation. The intended external path is SDKs over `/api/v1`, with OpenAI compatibility for client interoperability and plugins as a beta extension system. Control-plane and Web UI APIs remain internal unless explicitly promoted.
