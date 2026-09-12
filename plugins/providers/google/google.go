// SPDX-License-Identifier: MIT

// Google Gemini provider: Provider type + Complete + encode/decode roundtrip + canonicalToGemini.
// Code extracted from google.go during the Day-104 god-file split.
// Public API unchanged.
package google


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
	wire := geminiRequest{}
	if s := strings.TrimSpace(system); s != "" {
		wire.SystemInstruction = &geminiContent{
			// No role on systemInstruction per Gemini spec.
			Parts: []geminiPart{{Text: s}},
		}
	}
	for _, m := range msgs {
		c, err := canonicalToGemini(m, fwd)
		if err != nil {
			return nil, err
		}
		if c == nil {
			continue
		}
		wire.Contents = append(wire.Contents, *c)
	}
	if len(tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(tools))
		for _, t := range tools {
			params := t.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			decls = append(decls, geminiFunctionDecl{
				Name:        toolname.Wire(fwd, t.Name),
				Description: t.Description,
				Parameters:  params,
			})
		}
		wire.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	if maxTok > 0 || jsonMode || thinkingBudget != 0 || !params.IsZero() {
		gc := &geminiGenConfig{MaxOutputTokens: maxTok}
		if jsonMode {
			gc.ResponseMimeType = "application/json"
		}
		if thinkingBudget != 0 {
			// Opt-in (M319). includeThoughts so the summaries come back and
			// land on ReasoningContent; the budget caps the thinking tokens
			// (-1 = let Gemini decide dynamically).
			gc.ThinkingConfig = &geminiThinkingConfig{
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

// parseImageDataURL splits an RFC 2397 data: URL of the form
// "data:<media-type>;base64,<payload>" into its media type and base64 payload,
// returning ok=false for anything else (including a legacy bare filename),
// which the caller skips. The CLI sends data: URLs (M241).
func parseImageDataURL(s string) (mediaType, data string, ok bool) {
	const prefix = "data:"
	if !strings.HasPrefix(s, prefix) {
		return "", "", false
	}
	meta, payload, found := strings.Cut(s[len(prefix):], ",")
	if !found || !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	mt := strings.TrimSuffix(meta, ";base64")
	if mt == "" || payload == "" {
		return "", "", false
	}
	return mt, payload, true
}

func canonicalToGemini(m agent.Message, fwd map[string]string) (*geminiContent, error) {
	switch m.Role {
	case agent.RoleSystem:
		// System messages fold into systemInstruction at the request
		// level; per-message system roles are ignored here.
		return nil, nil
	case agent.RoleUser:
		// Vision (M243): a user message may carry image attachments as RFC 2397
		// data: URLs. Emit each as an inlineData part before the text part. A
		// non-data-URL entry (e.g. a legacy bare filename) has no deliverable
		// payload and is skipped.
		parts := make([]geminiPart, 0, len(m.Images)+1)
		for _, img := range m.Images {
			if mt, data, ok := parseImageDataURL(img); ok {
				parts = append(parts, geminiPart{InlineData: &geminiInlineData{MimeType: mt, Data: data}})
			}
		}
		parts = append(parts, geminiPart{Text: m.Content})
		return &geminiContent{Role: "user", Parts: parts}, nil
	case agent.RoleAssistant:
		var parts []geminiPart
		if strings.TrimSpace(m.Content) != "" {
			parts = append(parts, geminiPart{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			args := tc.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			parts = append(parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: toolname.Wire(fwd, tc.Name),
					Args: args,
				},
			})
		}
		if len(parts) == 0 {
			// Gemini rejects empty content; insert a placeholder.
			parts = []geminiPart{{Text: ""}}
		}
		return &geminiContent{Role: "model", Parts: parts}, nil
	case agent.RoleTool:
		if m.ToolCallID == "" {
			return nil, errors.New("google: role=tool requires tool_call_id (used as functionResponse name lookup)")
		}
		// Gemini doesn't have a separate tool role. Tool results are
		// sent back as a user-role content with functionResponse parts.
		// The canonical Message carries the function name via the
		// preceding assistant turn — but Gemini's functionResponse
		// also requires the name. We don't have it in canonical
		// shape, so we rely on the loop to supply it via Content
		// being a JSON object or by setting Name in the canonical
		// Message (future). For now: pack the content under a
		// "result" key and use the tool_call_id as the function name
		// surrogate. Real callers that need name fidelity should
		// route through the tool registry. See ADR-???; tracked in
		// SPEC-15 "tool-result name binding".
		// Build the {"result": ...} object with encoding/json, NOT strconv.Quote:
		// strconv.Quote is a GO string-literal quoter, so a control byte (NUL, ESC
		// \x1b common in terminal/ANSI tool output, etc.) becomes a Go-only \xNN
		// escape that is INVALID JSON — which makes the whole request fail to encode
		// and wedges the agent loop on Gemini for any tool output containing one. (M481)
		quoted, err := json.Marshal(m.Content)
		if err != nil {
			return nil, fmt.Errorf("google: encode tool result: %w", err)
		}
		resp := json.RawMessage(`{"result":` + string(quoted) + `}`)
		return &geminiContent{
			Role: "user",
			Parts: []geminiPart{{
				FunctionResponse: &geminiFunctionResponse{
					Name:     m.ToolCallID, // surrogate; see comment above
					Response: resp,
				},
			}},
		}, nil
	default:
		return nil, fmt.Errorf("google: unknown role %q", m.Role)
	}
}

func decodeResponse(body []byte, model string) (*agent.CompletionResponse, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("google: parse response: %w", err)
	}
	if len(gr.Candidates) == 0 {
		return nil, fmt.Errorf("google: response has no candidates")
	}
	cand := gr.Candidates[0]

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
			// Gemini doesn't return per-call IDs; synthesize stable ones
			// (SPEC-15: canonical ToolCall.ID is always non-empty).
			toolCalls = append(toolCalls, agent.ToolCall{
				ID:    "call-" + strconv.Itoa(i),
				Name:  part.FunctionCall.Name,
				Input: args,
			})
		case part.Thought && part.Text != "":
			// A thought-summary part (M319) — reasoning, kept out of the answer.
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
	if gr.UsageMetadata != nil {
		usage.InputTokens = gr.UsageMetadata.PromptTokenCount
		usage.CachedInputTokens = gr.UsageMetadata.CachedContentTokenCount
		// Thinking tokens are billed as output but reported separately (M319);
		// fold them in so OutputTokens reflects the true billable output.
		usage.OutputTokens = gr.UsageMetadata.CandidatesTokenCount + gr.UsageMetadata.ThoughtsTokenCount
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
