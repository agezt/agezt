# Phase M904 — Remote MCP servers over Streamable HTTP (#39)

## Goal
Close the registry's transport gap (#39, "3rd-party tool/MCP parity"): until now
the kernel MCP registry (`kernel/mcp`, M796) could only spawn **local stdio**
processes. Many popular MCP servers are **remote HTTP endpoints** (hosted
GitHub/Linear/etc.). M904 teaches the registry the **MCP Streamable HTTP
transport** (the 2025-03-26 spec's transport), so a registration can be a URL
instead of a command — attached, governed, and bridged into runs exactly like a
stdio server.

## What shipped

### Transport — `kernel/mcp/http.go`
A minimal Streamable HTTP client implementing the same `Conn` interface as the
stdio client:
- `DialHTTP(ctx, endpoint, headers)` → `initialize` → `notifications/initialized`
  → `tools/list`, then forwards `tools/call`.
- Each JSON-RPC request is **POSTed** to the one endpoint; the reply is decoded
  from **either** an `application/json` body **or** a `text/event-stream` (the
  client scans SSE `data:` lines for the message matching the request id).
- Captures the `Mcp-Session-Id` from `initialize` and echoes it (plus
  `MCP-Protocol-Version`) on every later request; `Close()` best-effort `DELETE`s
  the session.
- **Scope mirrors stdio deliberately:** request/reply only — no long-lived GET
  listening stream (this client makes no use of server-initiated
  requests/resources/prompts). Calls serialize under a mutex; frames are
  size-capped (`maxFrameBytes`); the SSE reader bounds total bytes.
- Operator opt-in `headers` (e.g. `Authorization: Bearer …`) ride every request.

### Registry — `kernel/mcp/store.go`
- `Server.URL` (remote endpoint) + `Server.Headers` (opt-in auth headers).
- `Validate`: **exactly one** of `Command` (stdio) / `URL` (http); URL must be
  http/https with a host; header names must be valid HTTP tokens; `maxHeaders`
  cap. `Command`/`url` JSON tags became `omitempty`.

### Wiring — `kernel/runtime`
- New `dialMCP(srv)` routes on transport: a URL attaches via the
  `MCPHTTPDialer` seam (default `mcp.DialHTTP`), everything else via the stdio
  `MCPDialer`. From there it's the same path — tools bridged as
  `mcp_<server>_<tool>`, the M899 allowlist, detach kill-switch, journal events.
- `mcp.added` payload now carries `transport` + `url`.
- `Config.MCPHTTPDialer` added (nil → production dialer; tests inject fakes).

### Surfaces — controlplane + CLI
- `mcpServerView` redacts `headers` like `env` (exposes sorted `header_keys`
  only) and adds a `transport` badge (`stdio`/`http`).
- `agt mcp add <name> --url URL [--header "K: V" ...]` registers a remote server;
  `agt mcp list` shows `http <url>` for remote rows. Usage updated.

## Tests
- `kernel/mcp/http_test.go` — a `httptest` Streamable HTTP server exercised over
  **both** JSON and SSE framings: tool discovery, a forwarded call, the opt-in
  header riding the request, the session id echoed back, `Close` issuing the
  `DELETE`, and a 401 propagating from the handshake.
- `kernel/mcp/store_test.go::TestValidateServer_Transport` — both/neither
  command+url rejected, non-http scheme rejected, hostless url rejected, bad
  header name rejected; remote+headers accepted.
- `kernel/runtime/mcptool_test.go::TestAttach_RemoteRoutesThroughHTTPDialer` — a
  URL registration routes through the HTTP dialer (never stdio), carrying its
  headers, and bridges `mcp_remote_<tool>`.
- `kernel/controlplane/mcp_test.go::TestMCP_RemoteServerViewRedactsHeaders` — the
  wire view badges `transport:http`, omits raw `headers`, exposes sorted
  `header_keys`, and never serializes the secret value.

## Gate
`go build ./...` ✓ · vet (mcp/runtime/controlplane/agt) ✓ · targeted + full
mcp/runtime/controlplane/webui suites ✓ · linux/amd64 cross-build ✓ · new files
gofmt-clean, all edits gofmt-clean modulo CRLF working copy · go.mod unchanged ·
no new `AGEZT_*` env var.

## Security posture
Default-allow holds: registering/attaching stays gated by `mcp.install` (Ask),
every call exercises `mcp.call`. No SSRF host-block on the URL — legitimate
self-hosted MCP servers run on localhost/LAN (task #51), and the URL is operator
config behind the install gate; only scheme/host are validated. Header values
are secrets: stored plaintext in the registry (use a low-scope token) and
**redacted** from every read API.

## Follow-up
- **M905 (UI):** Mcp.tsx form gains a stdio/remote toggle (URL + headers fields)
  and remote catalog presets, with a `transport` badge + `header_keys` on cards.
- Still deferred under #39: the old HTTP+SSE two-stream transport (the
  `mcpbridge` plugin already speaks it) and a fully-lazy on-demand MCP dispatcher.
