// SPDX-License-Identifier: MIT

package cohere

// Cohere ENCODE side: encodeRequest + canonicalToCohere. Carved out
// of cohere.go during the Day 191 god-file split so the main file
// can stay focused on the Provider dispatch (New/Name/Complete) +
// shared wire types, and the decode file can stay focused on
// response parsing.
// Public API unchanged.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func encodeRequest(model, system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	fwd, _ := toolname.Maps(tools)
	wire := cohereRequest{
		Model:     model,
		Stream:    false,
		MaxTokens: maxTok,
	}
	wire.applyParams(params)
	if s := strings.TrimSpace(system); s != "" {
		wire.Messages = append(wire.Messages, cohereMessage{Role: "system", Content: s})
	}
	for _, m := range msgs {
		cm, err := canonicalToCohere(m, fwd)
		if err != nil {
			return nil, err
		}
		if cm == nil {
			continue
		}
		wire.Messages = append(wire.Messages, *cm)
	}
	for _, t := range tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		wire.Tools = append(wire.Tools, cohereTool{
			Type: "function",
			Function: cohereToolFnDef{
				Name:        toolname.Wire(fwd, t.Name),
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

func canonicalToCohere(m agent.Message, fwd map[string]string) (*cohereMessage, error) {
	switch m.Role {
	case agent.RoleSystem:
		if strings.TrimSpace(m.Content) == "" {
			return nil, nil
		}
		return &cohereMessage{Role: "system", Content: m.Content}, nil
	case agent.RoleUser:
		return &cohereMessage{Role: "user", Content: m.Content}, nil
	case agent.RoleAssistant:
		cm := &cohereMessage{Role: "assistant", Content: m.Content}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			cm.ToolCalls = append(cm.ToolCalls, cohereToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: cohereToolCallFn{
					Name:      toolname.Wire(fwd, tc.Name),
					Arguments: string(args),
				},
			})
		}
		return cm, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("cohere: role=tool requires tool_call_id")
		}
		return &cohereMessage{
			Role:       "tool",
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return nil, fmt.Errorf("cohere: unknown role %q", m.Role)
	}
}

