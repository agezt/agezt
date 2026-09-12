// SPDX-License-Identifier: MIT

// Vertex provider: Provider type + Complete + encode/decode roundtrip + canonicalToVertex.
// Code extracted from vertex.go during the Day-110 god-file split.
// Public API unchanged.
package vertex


import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/provopts"
	"github.com/agezt/agezt/plugins/providers/internal/toolname"
)

func encodeRequest(system string, msgs []agent.Message, tools []agent.ToolDef, maxTok int, jsonMode bool, thinkingBudget int, params agent.Params, extra json.RawMessage) ([]byte, error) {
	// A per-request reasoning effort (M997) overrides the construction-time
	// thinking budget when set; otherwise the env/default budget stands.
	if b, ok := provopts.ThinkingBudget(params.ReasoningEffort, maxTok); ok {
		thinkingBudget = b
	}
	fwd, _ := toolname.Maps(tools)
	wire := vxRequest{}
	if s := strings.TrimSpace(system); s != "" {
		wire.SystemInstruction = &vxContent{Parts: []vxPart{{Text: s}}}
	}
	for _, m := range msgs {
		c, err := canonicalToVertex(m, fwd)
		if err != nil {
			return nil, err
		}
		if c == nil {
			continue
		}
		wire.Contents = append(wire.Contents, *c)
	}
	if len(tools) > 0 {
		decls := make([]vxFunctionDecl, 0, len(tools))
		for _, t := range tools {
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			decls = append(decls, vxFunctionDecl{
				Name:        toolname.Wire(fwd, t.Name),
				Description: t.Description,
				Parameters:  params,
			})
		}
		wire.Tools = []vxTool{{FunctionDeclarations: decls}}
	}
	if maxTok > 0 || jsonMode || thinkingBudget != 0 || !params.IsZero() {
		gc := &vxGenConfig{MaxOutputTokens: maxTok}
		if jsonMode {
			gc.ResponseMimeType = "application/json"
		}
		if thinkingBudget != 0 {
			// Opt-in (M320). includeThoughts so the summaries return and land
			// on ReasoningContent; the budget caps thinking tokens (-1 = dynamic).
			gc.ThinkingConfig = &vxThinkingConfig{
				IncludeThoughts: true,
				ThinkingBudget:  thinkingBudget,
			}
		}
		gc.applyParams(params)
		wire.GenerationConfig = gc
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return provopts.Merge(body, extra)
}

func canonicalToVertex(m agent.Message, fwd map[string]string) (*vxContent, error) {
	switch m.Role {
	case agent.RoleSystem:
		return nil, nil
	case agent.RoleUser:
		// Vision (M245): emit each image attachment (data: URL) as an inlineData
		// part before the text part; skip non-data-URL entries.
		parts := make([]vxPart, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				parts = append(parts, vxPart{InlineData: &vxInlineData{MimeType: mt, Data: data}})
			}
		}
		parts = append(parts, vxPart{Text: m.Content})
		return &vxContent{Role: "user", Parts: parts}, nil
	case agent.RoleAssistant:
		var parts []vxPart
		if strings.TrimSpace(m.Content) != "" {
			parts = append(parts, vxPart{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			parts = append(parts, vxPart{
				FunctionCall: &vxFunctionCall{Name: toolname.Wire(fwd, tc.Name), Args: args},
			})
		}
		if len(parts) == 0 {
			parts = []vxPart{{Text: ""}}
		}
		return &vxContent{Role: "model", Parts: parts}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("vertex: role=tool requires tool_call_id")
		}
		// Surrogate name binding — same caveat as plugins/providers/google.
		// Build the {"result": ...} object with encoding/json, NOT strconv.Quote:
		// strconv.Quote emits Go-only \xNN escapes for control bytes (ANSI \x1b in
		// tool output, NUL, …) that are invalid JSON, which would fail the request
		// encode and wedge the agent loop on Vertex. (M483; same class as M481)
		quoted, err := json.Marshal(m.Content)
		if err != nil {
			return nil, fmt.Errorf("vertex: encode tool result: %w", err)
		}
		resp := json.RawMessage(`{"result":` + string(quoted) + `}`)
		return &vxContent{
			Role: "user",
			Parts: []vxPart{{
				FunctionResponse: &vxFunctionResponse{
					Name:     m.ToolCallID,
					Response: resp,
				},
			}},
		}, nil
	default:
		return nil, fmt.Errorf("vertex: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var vr vxResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return nil, fmt.Errorf("vertex: parse response: %w", err)
	}
	if len(vr.Candidates) == 0 {
		return nil, fmt.Errorf("vertex: response has no candidates")
	}
	cand := vr.Candidates[0]

	var (
		textParts      []string
		reasoningParts []string
		toolCalls      []agent.ToolCall
	)
	for i, part := range cand.Content.Parts {
		switch {
		case part.FunctionCall != nil:
			args := part.FunctionCall.Args
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			toolCalls = append(toolCalls, agent.ToolCall{
				ID:    "call-" + strconv.Itoa(i),
				Name:  part.FunctionCall.Name,
				Input: args,
			})
		case part.Thought && part.Text != "":
			// A thought-summary part (M320) — reasoning, kept out of the answer.
			reasoningParts = append(reasoningParts, part.Text)
		case part.Text != "":
			textParts = append(textParts, part.Text)
		}
	}

	var stop agent.StopReason
	switch {
	case len(toolCalls) > 0:
		stop = agent.StopToolUse
	default:
		switch cand.FinishReason {
		case "STOP", "":
			stop = agent.StopEndTurn
		case "MAX_TOKENS":
			stop = agent.StopMaxTokens
		default:
			stop = agent.StopEndTurn
		}
	}

	usage := agent.Usage{Model: model}
	if vr.UsageMetadata != nil {
		usage.InputTokens = vr.UsageMetadata.PromptTokenCount
		usage.CachedInputTokens = vr.UsageMetadata.CachedContentTokenCount
		// Thinking tokens are billed as output but reported separately (M320);
		// fold them in so OutputTokens reflects the true billable output.
		usage.OutputTokens = vr.UsageMetadata.CandidatesTokenCount + vr.UsageMetadata.ThoughtsTokenCount
	}

	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
		StopReason:       stop,
		Usage:            usage,
		ReasoningContent: strings.Join(reasoningParts, ""),
	}, nil
}
