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
	case "schedule":
		return CapSchedule
	case "runs":
		return CapRunsRead
	case "standing":
		return CapStanding
	case "memory":
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
