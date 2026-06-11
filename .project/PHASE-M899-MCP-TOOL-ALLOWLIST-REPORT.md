# PHASE M899 — Per-server MCP tool allowlist (context-efficient)

**Status:** shipped
**Milestone:** M899 (last of this session's M889–M899 range; branched from
`origin/main`).
**Theme:** Backlog **#39** — the "context-efficient MCP management" piece. A
chatty MCP server (github exposes ~30 tools) injects all its tool schemas into
*every* run's context. M899 lets the operator expose only the tools a server
actually needs, so the rest never reach the prompt.

## What shipped

- **`kernel/mcp/store.go`** — `Server.ToolAllow []string` (optional allowlist of
  the server's bare tool names; empty = expose all). `Validate` caps it
  (`maxToolAllow = 128`) and rejects blank names.
- **`kernel/runtime/mcptool.go`** — `mergeMCPTools` now builds a per-server
  allow-set from the store and **skips any discovered tool not on its server's
  allowlist**. Servers without an allowlist behave exactly as before (all tools).
  The call sites (`runtime.go`, `subagent.go`, `workflowrun.go`) are unchanged —
  the selectivity lives entirely inside the merge, so no contested file is touched.
- **`kernel/controlplane/mcp.go`** — no change needed: `ToolAllow` rides the
  existing `mcp.Server` JSON unmarshal (add route) and marshal (view); it isn't a
  secret, so it's shown plainly.
- **`frontend/src/views/Mcp.tsx`** — a **Tools allowlist** field in the register
  form (`splitTools` helper, space/comma separated); registered cards show
  `tools: a, b` when set. The hint tells the operator to attach first to discover
  tool names, then set the allowlist.

## Verification

- **Build/test (clean origin/main base):** `go build ./...` + linux/amd64 cross-
  build clean; `gofmt -l` + `go vet` clean. New tests: `TestAttach_ToolAllowFilters`
  (a server with `ToolAllow:["greet"]` offers `mcp_fake_greet` but **not**
  `mcp_fake_shout`); `TestValidateServer` gains a blank-tool case + a valid
  allowlist case. Existing MCP attach/detach suites green.
- **Frontend:** `tsc --noEmit` clean; `vitest run src/views/Mcp` green **12/12**
  (new `splitTools` test); `vite build` emits the committed-LF dist.

## Notes
- Combined with M897 (catalog) + M898 (per-server env), #39's operator-facing MCP
  story is now solid: discover popular servers → give one its credential → trim
  its tool surface. Two deeper pieces remain, both needing the contested runtime/
  bridge: **SSE/HTTP transport** for remote MCP servers (the registry/Dial path is
  stdio-only) and fully **lazy on-demand** tool loading (a discover-then-call
  dispatcher) — best done once the concurrent kernel arc reconciles.
