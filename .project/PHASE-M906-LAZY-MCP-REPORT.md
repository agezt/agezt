# Phase M906 — Lazy MCP loading (#39 context-efficiency)

## Goal
Close the second clause of #39, "context-efficient MCP management". Until now
`mergeMCPTools` injected **every** attached server's tool with its **full input
schema** into **every** run. A chatty server (GitHub's MCP exposes ~30 tools)
bloats every run's context with schemas the run may never touch. M906 adds an
opt-in **lazy** mode that collapses one server's tools into a **single
dispatcher tool**.

## What shipped

### Registry — `kernel/mcp/store.go`
- `Server.Lazy bool` (opt-in; eager injection stays the default — default-allow
  posture). Composes with `ToolAllow` (M899).

### Runtime — `kernel/runtime/mcptool.go`
- `mergeMCPTools` refactored: per server it computes the **exposed subset** (after
  the allowlist), then —
  - **eager (default):** injects each `mcp_<server>_<tool>` with its full schema
    (unchanged behavior);
  - **lazy:** injects ONE `lazyMCPDispatch` named `mcp_<server>`.
- `lazyMCPDispatch` (new `agent.Tool`): its input schema is
  `{tool: enum(<exposed names>), arguments: object}`; its **description** lists
  each exposed tool name + one-line description so the model can choose. `Invoke`
  parses `{tool, arguments}`, rejects a tool not in the exposed set, and forwards
  to `conn.Call(tool, arguments)` — the remote server validates the arguments.
  N full schemas → 1 small one, correctness preserved (the server still
  validates), discovery preserved (names + descriptions + enum).
- The `mcp_<server>` name keeps the `mcp_` prefix, so the Edict toolmap still maps
  it to `mcp.call` — no policy change.

### Surfaces — CLI + UI
- `agt mcp add … --lazy`; `agt mcp list` appends a `lazy` marker.
- `Mcp.tsx`: a "Lazy load" checkbox in the register form (both transports) posting
  `lazy:true`; server cards badge `lazy`.

## Tests
- `kernel/runtime/mcptool_test.go::TestAttach_LazyCollapsesToDispatcher` — a Lazy
  server with an allowlist offers exactly **one** `mcp_fake` dispatcher (no
  per-tool `mcp_fake_*`), its enum contains the allowlisted tools and **not** the
  filtered-out one, and a dispatched `{tool, arguments}` call forwards to the
  connection with the arguments intact.
- `frontend/src/views/Mcp.test.tsx` — ticking "Lazy load" posts `lazy:true`. (17
  vitest pass.)

## Gate
`go build ./...` ✓ · vet ✓ · mcp/runtime/controlplane/webui suites ✓ · linux/amd64
cross-build ✓ · `tsc` ✓ · `vite build` → embedded dist rebuilt (LF) ✓ · gofmt-clean
(modulo CRLF) · go.mod unchanged · no new env var.

## Notes
This is the context-efficiency complement to M899's allowlist: the allowlist
*trims* which tools exist; lazy *collapses* how the surviving ones are presented.
The two compose — lazy + allowlist gives a one-tool dispatcher over exactly the
chosen subset. With M904/M905 (remote parity), the operator-facing depth of #39
is now substantial; remaining deferred: the HTTP+SSE two-stream transport on the
registry (the mcpbridge plugin already speaks it). See [[mcp-remote-parity-arc]].
