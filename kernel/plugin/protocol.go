// SPDX-License-Identifier: MIT

package plugin

// Package documentation lives in doc.go.

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
	// ProtocolVersion is the wire protocol version the plugin speaks.
	// Plugins that omit it default to 1. The host rejects a major mismatch
	// at spawn so an incompatible plugin fails fast with a clear error.
	// Optional for back-compat.
	ProtocolVersion int       `json:"protocol_version,omitempty"`
	Tools           []ToolDef `json:"tools"`
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

// ProtocolVersion is the wire protocol version the host speaks. Plugins
// echo it back in their initialize response so the host can reject an
// incompatible plugin at spawn rather than failing cryptically mid-run.
//
// Versioning policy:
//   - A major version bump means a breaking wire change (new/removed
//     required fields, changed method semantics).
//   - A minor change (new optional field, new method) does NOT bump the
//     version — the host and plugin must tolerate unknown optional fields.
//
// Plugins that omit the field entirely are treated as v1 (back-compat with
// plugins written before this field was introduced).
const ProtocolVersion = 1
