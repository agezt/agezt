# M553 — E2E: out-of-process plugin + MCP bridge + mesh delegation (criterion 7 complete)

## Context
Final batch of criterion 7 (runtime/E2E). With these, every product surface the
goal enumerates has been driven against the real daemon with 0 panics / 0
error-level journal events / graceful shutdown.

## Out-of-process plugin — PASS
`AGEZT_PLUGINS="ext=<echoplugin.exe>"` → the daemon spawned the plugin as a real
**separate subprocess** (confirmed by PID in the process table), completed the JSON
handshake, and registered all 4 advertised tools: `ext.echo`, `ext.callhost`
(host-callback), `ext.fail`, `ext.slowwork` (streaming progress). `agt plugin list`
and `agt tool list` show them; the boot banner shows `plugin:ext(4 tools)`. On
shutdown the subprocess exited (no orphan). The invoke / kernel-callback /
streaming-progress wire protocol is exercised by the `kernel/plugin` integration
tests (callback, progress, toolcap, deliver, flood) — all pass.

## MCP bridge — PASS
The `plugins/external/mcpbridge` suite is a genuine cross-process e2e: it builds
the bridge AND a mock MCP server (`testdata/mockmcp`), spawns the bridge through
the **real agezt plugin host**, and exercises `host ←agezt protocol→ bridge
←JSON-RPC 2.0→ MCP server`. All 7 `TestBridge_*` pass: lists tools from the MCP
server, invokes an upstream tool, propagates IsError, prefix-namespacing, missing
env var, resources surfaced as read-resource, unknown-tool error.

## Mesh peer delegation — PASS
Two real daemons: leader **A** (`AGEZT_PEERS="worker=http://127.0.0.1:18810|<B-token>"`)
and worker **B** (REST API + echo mock). `agt peers` from A → `worker  …  OK
(version 1.0.0)` (cross-node REST health). The remote_run delegation path
(`POST {base}/api/v1/runs` with `X-Agezt-Hop`, exactly what `peer.go:251` issues)
made B execute the delegated task and return the echo answer; **B's journal grew
seq 6 → 12**, i.e. the delegated run ran through B's own governed loop and was
journaled there. The peer tool's real-HTTP round-trip is also covered by
`TestLive_MeshRoundTripThroughRealRESTHandler`.

## Honesty note — a non-bug I almost flagged
First attempt configured the peer URL as `…/api/v1`; `agt peers` then probed
`…/api/v1/api/v1/health` → 404 ("UNREACHABLE"). Investigation showed the canonical
peer URL is **base-only** (`peer.go:52`: "no trailing /api/v1") — remote_run,
models, and the health probe all consistently append `/api/v1/…`. So the 404 was a
**harness misconfiguration, not a product defect**; with the correct base URL the
health check and delegation both succeed. (M530 lesson applied to e2e: verify the
config convention before concluding a defect.)

## Criterion 7 — COMPLETE
All 15 surfaces in `.project/ACCEPTANCE.md` §7 are PASS (Warden is N/A on Windows —
Linux `prlimit` facade, journaled downgrade). The one real defect this dimension
surfaced was M550 (OpenAI streaming dropped a non-streaming provider's answer),
now fixed. No code change this milestone.
