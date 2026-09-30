// SPDX-License-Identifier: MIT

// Package plugin is the kernel's out-of-process plugin host (M1.y).
//
// Architectural goal (DECISIONS B0): third-party tools should not
// have to compile against the agezt binary. The plugin host
// spawns subprocesses, communicates over line-delimited JSON on
// stdio, and exposes each remote tool through the standard
// `agent.Tool` interface so the rest of the kernel doesn't need
// to know whether a tool is in-process or out-of-process.
//
// **Why not gRPC.** Three reasons:
//
//  1. Forces a transitive `google.golang.org/grpc` dep on every
//     plugin author. The lean-deps policy applies to plugins too;
//     a plugin written in Python or shell needs only "read a line
//     of JSON, write a line of JSON" to talk to agezt.
//  2. The control plane (kernel/controlplane) already uses
//     line-delimited JSON; we keep the wire shape consistent
//     across both edges of the kernel.
//  3. Plugins run on the same host as the kernel — there's no
//     network hop to amortise the JSON cost. Wire protocol
//     simplicity beats wire efficiency at this scale.
//
// **Why not the Anthropic MCP spec.** MCP is a great fit for
// cross-tool interoperability (tools written for Claude Desktop,
// Cursor, etc.). For agezt's *kernel*-internal plugin contract
// we want a smaller surface — no transport layer, no SSE, no
// JSON-RPC-2.0 envelope overhead. A future MCP bridge is a
// separate plugin that translates MCP wire → agezt protocol;
// the kernel's internal protocol stays small.
//
// **Process lifecycle.**
//
//   - Host.Spawn launches a child via os/exec with stdin/stdout
//     piped. The child binary is responsible for whatever
//     language runtime it needs (Python, Bun, statically-linked
//     Go, anything that can read/write stdio).
//   - Host sends `{"id":"i1","method":"initialize"}`; child
//     replies with `{"id":"i1","result":{"tools":[...defs...]}}`.
//   - For each tool def, Host registers a `remoteTool` wrapper
//     with the daemon's registry.
//   - Tool invocations send `{"id":"q-N","method":"tool/invoke",
//     "params":{"name":"X","input":{...}}}`; child replies with
//     `{"id":"q-N","result":{"output":"..."}}` or
//     `{"id":"q-N","error":"..."}`.
//   - Host.Close sends `{"id":"end","method":"shutdown"}`, waits
//     a short grace period, then kills the process if it didn't
//     exit on its own.
//
// **Crash handling.** A plugin that exits unexpectedly marks all
// its tools as unavailable; subsequent invocations return a
// clear error. The kernel keeps running (non-plugin tools and
// the other plugins continue to serve).
package plugin
