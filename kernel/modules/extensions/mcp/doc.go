// SPDX-License-Identifier: MIT

// Package mcp is governed runtime self-install for MCP servers (M796): a
// durable registry of Model Context Protocol servers plus a minimal MCP
// client, so an agent (or operator) can ADD a server and ATTACH it while the
// daemon runs — no restart, no separate bridge binary, no env-var surgery.
// An attached server's tools are offered to every run as mcp_<server>_<tool>
// through the same dynamic per-run merge seam forged script tools use.
//
// Governance is the point: registering/attaching is gated by the
// `mcp.install` Edict capability (Ask by default — attaching spawns an
// arbitrary process), every forwarded call exercises `mcp.call`, the child
// gets a SCRUBBED environment (no AGEZT_* / secret-shaped vars), frames are
// size-capped, and every lifecycle transition is journaled (mcp.*) so
// `agt why` can explain how a server came to be attached. Detach is the
// instant kill switch.
//
// Storage mirrors kernel/roster: a single JSON file rewritten atomically.
package mcp
