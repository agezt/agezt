// SPDX-License-Identifier: MIT

// openairesponses_helpers.go: contentText + toInput + toTools split off from
// openairesponses.go during the Day 211 god-file refactor (#144). Public API unchanged.
package openairesponses

import (
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func contentText(kind, text string) map[string]any {
	return map[string]any{"type": kind, "text": text}
}

// Tool-name conformance (the Responses API validates against
// ^[a-zA-Z0-9_-]+$) lives in the shared plugins/providers/internal/toolname
// package: toolname.Maps builds the injective original↔wire mapping,
// toolname.Wire applies it on encode, and toolname.RestoreCalls reverses it on
// the response so a tool_call still routes to the real tool. Agezt exposes names
// like "browser.read", which the backend rejects with a 400 that kills this
// provider's arm of a routing chain.

// toInput maps AGEZT messages to Responses input items. fwd is the tool-name
// mapping, so a replayed function_call goes out under the same wire name the
// tool was offered under.
func toInput(msgs []agent.Message, fwd map[string]string) []any {
	out := make([]any, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			out = append(out, map[string]any{
				"type": "message", "role": "user",
				"content": []any{contentText("input_text", m.Content)},
			})
		case agent.RoleSystem:
			out = append(out, map[string]any{
				"type": "message", "role": "developer",
				"content": []any{contentText("input_text", m.Content)},
			})
		case agent.RoleAssistant:
			if strings.TrimSpace(m.Content) != "" {
				out = append(out, map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{contentText("output_text", m.Content)},
				})
			}
			for _, tc := range m.ToolCalls {
				args := string(tc.Input)
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				out = append(out, map[string]any{
					"type": "function_call", "name": toolname.Wire(fwd, tc.Name),
					"arguments": args, "call_id": tc.ID,
				})
			}
		case agent.RoleTool:
			out = append(out, map[string]any{
				"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content,
			})
		}
	}
	return out
}

func toTools(defs []agent.ToolDef, fwd map[string]string) []toolDef {
	if len(defs) == 0 {
		return nil
	}
	out := make([]toolDef, 0, len(defs))
	for _, d := range defs {
		params := d.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, toolDef{
			Type: "function", Name: toolname.Wire(fwd, d.Name), Description: d.Description, Parameters: params,
		})
	}
	return out
}
