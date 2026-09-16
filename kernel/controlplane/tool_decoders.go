// SPDX-License-Identifier: MIT
//
// kernel/controlplane tool-event payload decoders (decodeToolInvoked,
// decodedToolResult type, decodeToolResult).
// Extracted from tool_log.go during Day 211 god-file refactor (#78).
// Public API unchanged.
package controlplane

import (
	"encoding/json"
)

func decodeToolInvoked(payload json.RawMessage) (callID, input string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		CallID string          `json:"call_id"`
		Input  json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.CallID, previewString(string(p.Input))
}

type decodedToolResult struct {
	tool              string
	callID            string
	output            string
	isError           bool
	observationTrust  string
	observationSource string
	directiveLike     bool
	directiveMatches  []string
}

func decodeToolResult(payload json.RawMessage) decodedToolResult {
	if len(payload) == 0 {
		return decodedToolResult{}
	}
	var p struct {
		Tool              string   `json:"tool"`
		CallID            string   `json:"call_id"`
		Output            string   `json:"output"`
		Error             bool     `json:"error"`
		ObservationTrust  string   `json:"observation_trust"`
		ObservationSource string   `json:"observation_source"`
		DirectiveLike     bool     `json:"directive_like"`
		DirectiveMatches  []string `json:"directive_matches"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return decodedToolResult{}
	}
	return decodedToolResult{
		tool:              p.Tool,
		callID:            p.CallID,
		output:            previewString(p.Output),
		isError:           p.Error,
		observationTrust:  p.ObservationTrust,
		observationSource: p.ObservationSource,
		directiveLike:     p.DirectiveLike,
		directiveMatches:  p.DirectiveMatches,
	}
}
