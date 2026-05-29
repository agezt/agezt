# Phase Report — Milestone 1.bb (MCP bridge plugin)

> Status: **shipped** · Date: 2026-05-29
> Post-v1 work per the M1.aa "MCP bridge — picking up next" pointer.
> Continues [PHASE-M1.aa-REPORT.md](PHASE-M1.aa-REPORT.md).

## Scope

The agezt plugin host (M1.y) shipped a deliberately small
out-of-process protocol — line-delimited JSON, three methods
(`initialize`, `tool/invoke`, `shutdown`), no transport
negotiation. It works, but it isn't [MCP](https://modelcontextprotocol.io).
Every third-party tool that already speaks MCP (Postgres, Slack,
GitHub, filesystem, the `@modelcontextprotocol/*` family, the
growing ecosystem written for Claude Desktop / Cursor / Continue)
can't plug into agezt without a translator.

M1.bb ships **`mcpbridge`** — a standalone binary that:

- speaks the **agezt plugin protocol** on its stdio with the
  daemon (same wire as any other plugin),
- speaks **MCP 2024-11-05 over JSON-RPC 2.0** on the stdio of a
  configured MCP server,
- translates each `tool/invoke` from agezt → `tools/call` on the
  MCP side, and the result back the other way.

```
agezt daemon                  mcpbridge                  MCP server
  (kernel/plugin host) ──→  (stdio JSON, 3 methods)
                              ↓ exec.Cmd, stdin/stdout
                              MCP client  ──→  JSON-RPC 2.0, MCP wire
```

Operators wire it via the existing `AGEZT_PLUGINS=` env var:

```
AGEZT_PLUGINS="fs=/opt/agezt/mcpbridge" \
MCPBRIDGE_SERVER_CMD="npx -y @modelcontextprotocol/server-filesystem /tmp" \
agezt
```

Every tool the filesystem MCP server exposes appears in agezt's
registry under the `fs.` prefix.

| Concern | Status |
|---|---|
| Standalone binary with **zero** agezt imports | ✅ (mirrors echoplugin) |
| Agezt side: `initialize` / `tool/invoke` / `shutdown` lockstep | ✅ tested |
| MCP side: full handshake (`initialize` request + `notifications/initialized`) | ✅ tested |
| MCP `tools/list` → agezt tool defs, schema passed through verbatim | ✅ tested |
| MCP `tools/call` → agezt `Output`, content array flattened to text | ✅ tested |
| MCP `isError: true` propagates to agezt `IsError: true` | ✅ tested |
| Agezt host's prefix (e.g. `mcp.`) applies on top of bridged names | ✅ tested |
| Missing `MCPBRIDGE_SERVER_CMD` → clear stderr, non-zero exit | ✅ tested |
| Upstream JSON-RPC error surfaces as Go `error` (not silent) | ✅ tested |
| MCP server stderr forwarded to bridge stderr → agezt's plugin logger | ✅ |
| Concurrent in-flight calls supported (pending-map + id correlation) | ✅ (single-threaded by serve loop; structure ready) |
| Defaults overridable via env (`MCPBRIDGE_CLIENT_NAME`, `MCPBRIDGE_PROTOCOL_VERSION`) | ✅ |
| 110s call timeout vs host's 120s — bridge gives up just before host | ✅ |

## Why an external binary, not in-kernel MCP

Three reasons, in order of weight:

1. **Surface area.** MCP carries JSON-RPC 2.0, SSE transport,
   prompt/resource/sampling subprotocols, progress notifications,
   logging streams, etc. The kernel's plugin contract carries
   three methods. Bringing MCP into the kernel would mean every
   operator pays for the full MCP code surface even when they
   never run an MCP server. The bridge keeps that cost opt-in:
   no MCPBRIDGE_SERVER_CMD set → not even spawned.
2. **Versioning velocity.** MCP is a moving target (the spec date
   string in our handshake — `2024-11-05` — is already old as of
   shipping). When the next spec lands, the bridge can be
   rebuilt and dropped in without retesting the kernel. The
   kernel never has to carry deprecated MCP-specific code.
3. **The contract is the test.** The bridge satisfies the same
   `kernel/plugin` protocol any other plugin satisfies (echoplugin,
   future Python plugins, etc.). If the bridge regresses the
   contract, every plugin-using path breaks the same way — there's
   one debug story, not two.

## Files

### `plugins/external/mcpbridge/main.go` (~270 LoC)

Bridge entrypoint + agezt-side I/O loop:

- Reads `MCPBRIDGE_SERVER_CMD` (required), `MCPBRIDGE_CLIENT_NAME`,
  `MCPBRIDGE_PROTOCOL_VERSION` (both optional).
- Spawns the MCP server up front; if startup fails, exits with a
  clear stderr line (the host's WARNING log surfaces it to the
  operator).
- `serve(mcp)` is the agezt-side loop: one Request line in, one
  Response line out, in lockstep. Concurrent invocations from the
  host are serialised here (the MCP server is also single-stream
  over its stdio, so there's no win to parallelising at the
  bridge).
- `handleInitialize` → calls MCP `tools/list`, translates each
  `inputSchema`/`description`/`name` to `ageztToolDef`, defaults
  missing schemas to `{"type":"object"}`.
- `handleInvoke` → calls MCP `tools/call`, flattens the content
  array via `flattenContent` (text blocks concatenated with `\n`,
  non-text blocks become `[mcp:image content omitted]` markers).

### `plugins/external/mcpbridge/mcp.go` (~290 LoC)

The MCP client over the child's stdio. Self-contained — no
imports beyond stdlib:

- `jsonrpcReq` / `jsonrpcResp` / `jsonrpcError` envelope types,
  pointer-`id` field for notification vs request distinction.
- `mcpClient` with a `pending map[int64]chan *jsonrpcResp` + atomic
  next-id counter, identical structure to `kernel/plugin/host.go`'s
  pending map. The agezt side is single-threaded so we don't
  actually need this concurrency today, but the structure means a
  future bridge that handles progress callbacks doesn't need
  rewriting.
- `handshake` runs the two-message MCP startup: `initialize`
  request/response, then `notifications/initialized`. Many MCP
  servers stay in "starting" state until they see the
  notification, then reject every subsequent call — easy to miss.
- `listTools` sends `{}` for params explicitly (some servers reject
  a missing `params` field even though MCP allows omitting).
- `callTool` normalises `null` / empty args to `{}` for the same
  reason — MCP requires `arguments` to be an object even for
  zero-arg tools.
- `readLoop` routes responses by id; notifications (id-less
  responses from the server) are silently dropped — they're not
  part of v1's surface.
- Bridge-side timeouts: 15s for initialize round-trip, 110s for
  tool/call (intentionally 10s under the agezt host's 2-minute
  default so the bridge's "who timed out" answer is predictable —
  always the bridge if it's an MCP-side hang).

### `plugins/external/mcpbridge/testdata/mockmcp/main.go` (~150 LoC)

Minimal MCP server for tests. Implements enough of MCP 2024-11-05
to exercise the bridge end-to-end:

- `initialize` → returns capabilities + serverInfo
- `notifications/initialized` → flips internal `initialized` gate
- `tools/list` → returns two tools: `greet` (takes `{name}`) and
  `boom` (always returns `isError: true`)
- `tools/call` → dispatches to greet/boom; rejects pre-init calls
  with JSON-RPC error code `-32002`
- `shutdown` → exits clean

Zero imports of the bridge or agezt — its sole contract is the
MCP wire format, the same contract any real MCP server satisfies.
Prints `MOCKMCP_STARTED` to stderr on launch so a test can verify
the subprocess actually spawned via stderr capture.

### `plugins/external/mcpbridge/main_test.go` (~280 LoC, 6 tests)

End-to-end integration: agezt host ←agezt protocol→ bridge
←JSON-RPC 2.0→ mock MCP server. Both binaries built once per
`go test` invocation and cached (same one-shot `go build` pattern
as `kernel/plugin`'s echoplugin tests).

| Test | Locks in |
|---|---|
| `TestBridge_ListsToolsFromMCP` | Two MCP tools (`greet`, `boom`) round-trip through bridge → appear in `Plugin.Tools(...)` with descriptions + schemas intact |
| `TestBridge_InvokesUpstreamTool` | `greet({"name":"world"})` end-to-end produces `Output="hello, world"`, `IsError=false` |
| `TestBridge_PropagatesIsError` | MCP `isError: true` becomes agezt `IsError: true`, text content preserved |
| `TestBridge_PrefixNamespacing` | `Tools("mcp.")` produces `mcp.greet`, `mcp.boom`; unprefixed names absent |
| `TestBridge_MissingEnvVar` | Bridge with no `MCPBRIDGE_SERVER_CMD` exits non-zero and stderr names the missing variable (no daemon hang) |
| `TestBridge_UnknownToolError` | MCP JSON-RPC error response (not `isError`) surfaces as a Go `error` from `Invoke` — distinct from result-typed errors |

The `keys` helper uses Go generics to avoid an `agent.Tool` import
just for the type signature.

## Operator workflow examples

**Bridge a filesystem MCP server:**

```
AGEZT_PLUGINS="fs=/opt/agezt/mcpbridge" \
MCPBRIDGE_SERVER_CMD="npx -y @modelcontextprotocol/server-filesystem /home/me/docs" \
agezt
```

Then any agent run can call `fs.read_file`, `fs.write_file`, etc.

**Two MCP servers in parallel:**

```
AGEZT_PLUGINS="fs=/opt/agezt/mcpbridge,db=/opt/agezt/mcpbridge" \
... # both bridges share the same binary, distinguished by env
```

Each bridge is a separate `AGEZT_PLUGINS=` entry, but they share
the same binary — bridge instances are stateless across runs.
(Each instance reads `MCPBRIDGE_SERVER_CMD` from its own
environment, so the operator passes per-bridge env via a wrapper
script or systemd unit.)

**Custom client name (MCP servers that gate features on it):**

```
MCPBRIDGE_CLIENT_NAME="claude-ai" \
MCPBRIDGE_SERVER_CMD="..." \
... # some servers enable extra capabilities for Claude Desktop's name
```

## Test summary

```
go test ./plugins/external/mcpbridge/... -v -count=1
ok  	github.com/ersinkoc/agezt/plugins/external/mcpbridge	0.985s
(6 tests — all PASS)

go test ./... -count=1
(35 packages — all PASS, 467 unique top-level tests)
```

(The "534 tests" number in M1.aa's report counted subtests with
parent loops expanded; the canonical baseline is the top-level
function count above — 461 → 467 with M1.bb's +6.)

## What's intentionally NOT in M1.bb

- **MCP `resources/*` / `prompts/*` / sampling.** Only `tools/*`
  is bridged. Resources would map well onto a future agezt
  `resource/...` extension; that's a separate protocol expansion
  on the agezt side, not just a bridge feature.
- **MCP progress notifications.** Some MCP servers stream progress
  updates during long tool calls (e.g. file scans). The bridge
  drops them; agezt's `tool/invoke` is currently a single
  request/response. A future bridge revision could surface progress
  through `bus.PublishStreaming` once the plugin protocol gains a
  streaming mode.
- **MCP cancellation.** The 2024-11-05 MCP spec has no cancel
  message; a ctx-cancel on the agezt side cuts our wait but the
  MCP server keeps computing. The 2024-12 spec adds
  `notifications/cancelled` — picking that up is bridge-only work,
  no kernel change needed.
- **SSE transport.** MCP supports JSON-RPC over SSE (for remote
  servers). The bridge is stdio-only — every operator deployment
  we care about colocates the MCP server with agezt. Remote MCP
  is a separate bridge variant (`mcpbridge-sse`) when there's
  demand.
- **Schema validation in the bridge.** The bridge passes MCP
  `inputSchema` through to agezt verbatim; agezt's agent loop
  (and the provider) handle validation. Adding a second validation
  pass in the bridge would double the error story without catching
  anything the existing path doesn't.
- **MCP image / blob content.** Non-text MCP content blocks
  become `[mcp:image content omitted]` placeholders in the agezt
  `Output` string. Surfacing binary payloads needs a richer
  return type than `agent.Result.Output` (just a string today) —
  that's an agent-loop change, not a bridge change.
- **A `agt mcp ...` CLI command.** The bridge is configured by env
  only. Operators can wrap it in a script if they want better UX;
  building MCP-specific CLI surface into `agt` would push us back
  toward the in-kernel coupling the bridge was designed to avoid.

## Files touched

- [plugins/external/mcpbridge/main.go](../plugins/external/mcpbridge/main.go) — new, bridge entrypoint + agezt I/O.
- [plugins/external/mcpbridge/mcp.go](../plugins/external/mcpbridge/mcp.go) — new, MCP JSON-RPC 2.0 client.
- [plugins/external/mcpbridge/main_test.go](../plugins/external/mcpbridge/main_test.go) — new, 6 integration tests.
- [plugins/external/mcpbridge/testdata/mockmcp/main.go](../plugins/external/mcpbridge/testdata/mockmcp/main.go) — new, minimal MCP server for tests.

Zero changes to the kernel, the plugin host, or any other package.
The bridge sits cleanly on top of existing primitives (M1.y's
plugin protocol), is a pure additive feature, and can be deleted
or replaced without touching anything else.

## Deferrals after M1.bb

Unchanged from M1.aa's list, minus the MCP bridge just shipped:

- Pulse v3+ (TUI dashboard, until/last flags, replay rate limit,
  subject indexing).
- Planner v2 (re-planning, sub-planners, planner-side tools).
- Plugin sandboxing, signing, hot-reload, streaming, callbacks
  (the streaming bit would unblock the MCP progress-notification
  bridge).
- Browser: JS rendering, screenshots, search, cookies.
- AWS credential-provider chain.
- Non-Anthropic body shapes on Bedrock.
- Vault: OS-keychain integration, passphrase rotation, argon2.
- **Per-task-type routing.** Picking up next — the last
  routing/governor gap from the original DECISIONS doc.
- MCP bridge v2 deferrals listed above (resources, sampling,
  progress, cancellation, SSE, image content).
