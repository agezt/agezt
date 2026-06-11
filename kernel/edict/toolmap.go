// SPDX-License-Identifier: MIT

package edict

import (
	"encoding/json"
	"strings"
)

// CapabilityForToolCall returns the Edict capability for one canonical
// agent.ToolCall (name + input bytes). It's the runtime's single source
// of truth for "which capability does THIS specific call exercise?" so
// the trust ladder can be checked once per call.
//
// Unknown tool names fall back to Capability(name), which is default-
// denied unless the caller explicitly granted a level.
//
// Kept in this package (not the runtime) so the mapping lives next to
// the Capability constants and can grow as new tools land.
func CapabilityForToolCall(toolName string, input json.RawMessage) Capability {
	// Forged script tools (M794): every promoted script is offered as
	// forge_<name> and EXECUTES code in the code-exec sandbox, so each call
	// exercises exactly the code.exec capability — promotion changes who can
	// be called, never what the sandbox allows.
	if strings.HasPrefix(toolName, "forge_") {
		return CapCodeExec
	}
	// Runtime-attached MCP servers (M796): every bridged mcp_<server>_<tool>
	// call is external code the daemon didn't ship → the mcp.call capability.
	if strings.HasPrefix(toolName, "mcp_") {
		return CapMCP
	}
	switch toolName {
	case "shell":
		return CapShell
	case "file":
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(input, &p)
		switch p.Op {
		case "read", "stat", "search":
			return CapFileRead
		case "list":
			return CapFileList
		case "write", "append", "replace":
			return CapFileWrite
		case "delete":
			return CapFileDelete
		}
		return Capability("file." + p.Op)
	case "browser.read":
		return CapBrowserRead
	case "web_search":
		return CapWebSearch
	case "fetch":
		// A download is a network GET that saves the bytes — same capability as http.get.
		return CapHTTPGet
	case "artifacts":
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(input, &p)
		if strings.EqualFold(strings.TrimSpace(p.Op), "delete") {
			// Removing a saved file is the file-delete axis.
			return CapFileDelete
		}
		// list/read (and anything unrecognised) reads stored files — the low-risk
		// default, so a garbled call lands on read rather than gaining delete.
		return CapFileRead
	case "schedule":
		return CapSchedule
	case "runs":
		return CapRunsRead
	case "standing":
		return CapStanding
	case "board":
		return CapBoard
	case "skill":
		return CapSkill
	case "tool_forge":
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(input, &p)
		if strings.EqualFold(strings.TrimSpace(p.Op), "test") {
			// op=test RUNS the draft's code in the sandbox — that's a real
			// code execution, so it exercises code.exec, not just authoring.
			return CapCodeExec
		}
		return CapToolForge
	case "mcp":
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(input, &p)
		if strings.EqualFold(strings.TrimSpace(p.Op), "list") {
			// Listing registrations + live attachments reads the daemon's own
			// state — the introspection axis, not an install.
			return CapIntrospect
		}
		// add/attach/detach/remove (and anything unrecognised) is the
		// self-install axis — the high-risk default, so a garbled call lands
		// on the gated capability rather than silently flowing.
		return CapMCPInstall
	case "workflow":
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(input, &p)
		switch strings.ToLower(strings.TrimSpace(p.Op)) {
		case "list", "show":
			// Reading the workflow library is the introspection axis.
			return CapIntrospect
		}
		// save/run/enable (and anything unrecognised) installs or fires
		// automation — the gated default, so a garbled call lands on the
		// capability rather than silently flowing.
		return CapWorkflow
	case "introspect":
		return CapIntrospect
	case "code_exec":
		return CapCodeExec
	case "memory":
		return CapMemory
	case "db":
		// The Personal Data Lake (M834) is structured durable knowledge the agent
		// keeps for itself — same axis as memory, so it inherits that allow-by-
		// default grant rather than introducing a new capability.
		return CapMemory
	case "world":
		return CapWorld
	case "delegate":
		return CapDelegate
	case "coding":
		return CapCoding
	case "acp_agent":
		return CapACPAgent
	case "remote_run":
		return CapRemoteRun
	case "notify":
		return CapNotify
	case "config":
		var p struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(input, &p)
		switch strings.ToLower(strings.TrimSpace(p.Op)) {
		case "set", "register", "unregister":
			return CapConfigWrite
		}
		// schema/get (and anything unrecognised) is the read axis — the low-risk
		// default, so a garbled call lands on read rather than silently gaining write.
		return CapConfigRead
	case "homeassistant":
		var p struct {
			Operation string `json:"operation"`
		}
		_ = json.Unmarshal(input, &p)
		if strings.EqualFold(strings.TrimSpace(p.Operation), "call_service") {
			return CapHomeAssistantCall
		}
		// get_states (and anything unrecognised) is the read axis: the low-risk
		// default, so an unparsed/garbled call lands on the more-restrictive-to-
		// escalate read capability rather than silently gaining actuation.
		return CapHomeAssistantRead
	case "http":
		var p struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(input, &p)
		if strings.EqualFold(strings.TrimSpace(p.Method), "POST") {
			return CapHTTPPost
		}
		return CapHTTPGet
	default:
		return Capability(toolName)
	}
}
