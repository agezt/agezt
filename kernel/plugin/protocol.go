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

import (
	"encoding/json"
)

// Request is the wire shape the host sends to the plugin.
type Request struct {
	// ID is a host-generated correlation token. Plugins must echo
	// it back in the matching Response. Empty for fire-and-forget
	// notifications (only `shutdown` qualifies in v1).
	ID string `json:"id"`
	// Method names the operation. Known: "initialize",
	// "tool/invoke", "shutdown".
	Method string `json:"method"`
	// Params is method-specific. JSON-encoded so plugins in
	// languages without strong typing can decode it ad-hoc.
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the wire shape the plugin sends to the host.
//
// **Terminal responses** populate exactly one of Result or Error
// and close the request: `{"id":"q-1","result":{...}}` or
// `{"id":"q-1","error":"..."}`.
//
// **Progress notifications** (M1.ss) populate Progress with a
// human-readable string and leave Result and Error empty:
// `{"id":"q-1","progress":"downloaded 17/42 chunks"}`. They share
// the request's id so the host can route them to the originating
// caller. Multiple progress lines may interleave between request
// and terminal response; the terminal response is always last.
//
// Backwards-compatible: plugins that don't emit progress are
// unaffected. Hosts that don't register a progress callback drop
// progress lines silently.
type Response struct {
	ID       string          `json:"id"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	Progress string          `json:"progress,omitempty"`
}

// ToolDef describes a tool the plugin exposes. Mirrors
// agent.ToolDef but lives here so plugin authors can target this
// package without importing kernel/agent.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	// Capability optionally declares which policy axis this tool belongs to
	// (M900) — one of the kernel's known Edict capabilities, e.g. "http.post",
	// "file.write", "shell". A declared tool joins that axis's trust level and
	// hard-deny rules instead of landing on the unknown-capability default, so
	// an operator's "http.post asks first" applies to a third-party plugin's
	// POST tool exactly like the built-in one. Empty (the default) keeps the
	// historical classification (the tool's own name as a one-off capability);
	// an UNKNOWN declared value is ignored the same way — a plugin cannot
	// invent axes, only join existing ones.
	Capability string `json:"capability,omitempty"`
}

// InitializeResult is the payload of the initialize response.
// Lists every tool the plugin offers. The host can re-call
// initialize later (e.g. after a plugin restart) and the new
// list replaces the old.
type InitializeResult struct {
	Tools []ToolDef `json:"tools"`
}

// InvokeParams is the payload of a tool/invoke request.
type InvokeParams struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// InvokeResult is the payload of a tool/invoke response.
// Mirrors agent.Result.
type InvokeResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

// Method names (single source of truth for both ends).
const (
	MethodInitialize = "initialize"
	MethodInvoke     = "tool/invoke"
	MethodShutdown   = "shutdown"

	// MethodHostInvoke (M1.cb) is the plugin→host direction: a
	// plugin sends `{"id":"p-N","method":"host/invoke","params":
	// {"name":"...","input":{...}}}` and the host replies with the
	// usual Response shape. Reuses InvokeParams + InvokeResult so
	// the wire shape is symmetric with tool/invoke.
	//
	// **Wire bidirectionality.** A plugin that wants callbacks
	// MUST be tolerant of receiving Request frames on its stdin
	// (interleaved with the Response frames it expects to get
	// back from its own host/invoke calls). The host's read loop
	// already handles this for the host→plugin direction; mirror
	// behavior on the plugin side is the plugin author's
	// responsibility — examples in testdata/echoplugin show one
	// way to structure it.
	MethodHostInvoke = "host/invoke"
)
