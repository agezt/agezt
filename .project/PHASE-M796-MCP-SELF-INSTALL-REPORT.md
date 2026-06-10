# Phase M796 — governed MCP self-install (runtime attach)

**Date:** 2026-06-10 · **Status:** DONE · **Frontier:** gap-analysis #3
(governed self-install — "ajanlar MCP server bile kurup ekleyip kullanabilir").

## What

`kernel/mcp`: a durable registry of MCP servers + a minimal kernel-native
MCP client (stdio: initialize → notifications/initialized → tools/list →
tools/call; line-delimited JSON-RPC, 16 MiB frame cap, handshake/call
timeouts, stray-notification skipping). An agent (`mcp` tool) or operator
(`agt mcp` / control plane / webui API) can REGISTER a server and ATTACH it
while the daemon runs — no restart, no separate bridge binary, no env-var
surgery. From the next run on, the server's discovered tools are offered to
every run and delegate child as `mcp_<server>_<tool>` through the SAME
dynamic merge seam forged script tools ride (merge before the WithTools
filter). Detach is the kill switch; enabled servers auto-attach at daemon
boot (per-server failures reported, never fatal); Close detaches all.

## Governance

- **Edict:** new `mcp.install` capability (LevelAsk — attaching spawns an
  arbitrary process, approve every one) gates the `mcp` tool's
  add/attach/detach/remove ops (op=list → introspect axis); new `mcp.call`
  (LevelAskFirst) gates every bridged `mcp_*` call via the toolmap prefix
  rule. A garbled `mcp` op lands on the gated install axis.
- **Scrubbed child env:** the spawned server gets PATH/OS vars/per-user dirs
  only — never AGEZT_* or secret-shaped variables (mirrors the code-exec
  scrub; unit-tested).
- **Journal:** `mcp.added/updated/attached/detached/removed` under subject
  `mcp.<name>`, attach payload carries the discovered tool names.
- Server names are `[a-z][a-z0-9]{0,15}` — NO underscore/dash, so the
  toolmap can parse the server out of `mcp_<server>_<tool>` unambiguously;
  bridged tool names are sanitized to the provider alphabet and capped at 64.

## Design notes

- Kernel spawns the child directly (kernel/plugin precedent) — warden is
  run-to-completion, MCP needs a long-lived dialog.
- `Config.MCPDialer` is the test seam (fake Conn injection); `mcp.Conn` is
  an interface; the pipe-based core (`newClientConn`) is tested without any
  process.
- Registration ≠ attach: add persists config, attach spawns. `Enabled`
  means auto-attach at boot.

## Tests (16 new across 5 packages)

- kernel/mcp client: full dialogue over in-memory pipes incl. notification
  skipping + server-error surfacing; lost-connection mid-call; env scrub;
  **live python subprocess e2e** (Dial → handshake → discovery → call
  "hello ersin" → close; skips without python).
- kernel/mcp store: CRUD, persistence, validation table (underscore/dash
  names rejected — toolmap parse invariant).
- runtime: attach → run OFFERS `mcp_fake_greet` and the call FORWARDS raw
  args / returns server text (wire-level, mock provider); detach kill
  switch + double-attach refusal; remove detaches first; boot auto-attach;
  WithTools allowlist gates bridged tools both ways; hostile tool names
  sanitized + length-capped.
- edict toolmap: `mcp_*` → mcp.call; mcp tool op routing (list →
  introspect; garbled → install).
- mcptool: self-install loop add→attach→list(live)→detach→remove; error
  paths; unbound.
- controlplane: wire round-trip add → attach (names returned, list shows
  live) → disable auto-attach → detach (conn closed) → remove → ghost refs.

## Smoke (isolated AGEZT_HOME, real daemon, real python MCP server)

`agt mcp add fake --cmd python --arg server.py` → `agt mcp attach fake` →
**real spawn + handshake**: "attached fake — 2 tool(s): mcp_fake_greet,
mcp_fake_weather" → list shows ATTACHED (2 tools) → detach → **daemon
restart**: banner "mcp servers : 1 attached of 1 registered" (boot
auto-attach proven). Journal: mcp.added → mcp.attached → mcp.detached →
halt → mcp.attached. Graceful shutdown, smoke dir removed.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; frontend untouched (webui API routes added for the
M797 console view; dist unchanged); go.mod unchanged; no new env vars.
CI org-billing still blocked → local battery + arc-authority merge.

## Next

M797: MCP console view (register/attach/detach from the web UI). Then
gap #5 vector memory, #6 brain distiller. Future polish: per-server Edict
capabilities (`mcp.<name>`), per-server env (vault-backed), SSE transport.
