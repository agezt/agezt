// SPDX-License-Identifier: MIT

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/convo"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// summaryMaxTokens bounds the briefing call (M925). Generous on purpose:
// a reasoning model (deepseek-v4-pro, o-series) spends output tokens on its
// chain of thought BEFORE the briefing — a tight cap (512 was tried) gets
// entirely eaten by reasoning and yields empty content. The briefing itself
// stays a compact digest; the prompt asks for that.
const summaryMaxTokens = 2048

// summaryInputCap bounds how much transcript is fed to the summarizer.
// When the folded turns exceed it, the oldest text is dropped first — the tail
// of the fold is closest to the live conversation, so it matters most.
const summaryInputCap = 24 << 10

// SummaryPorts reads the kernel's provider (nil when none is configured) and
// its default model.
type SummaryPorts struct {
	Provider func() llm.Provider
	Model    func() string
}

type Summarizer struct{ ports SummaryPorts }

func NewSummarizer(ports SummaryPorts) *Summarizer { return &Summarizer{ports: ports} }

// SummarizeRequest carries the turns to fold ([{role,text}]) and an optional
// model.
type SummarizeRequest struct {
	Turns json.RawMessage `json:"turns,omitempty"`
	Model json.RawMessage `json:"model,omitempty"`
}

type SummarizeOutput struct {
	Summary string `json:"summary"`
	Turns   int    `json:"turns"`
}

// Summarize condenses older chat turns into one compact briefing (M925). The
// Chat view calls this when a thread outgrows the history window, then rides
// the briefing as a leading system turn on later runs instead of silently
// dropping the oldest turns. One bounded provider call, routed as TaskType
// "summarize" (per-task model routing applies); no tools, no loop.
func (s *Summarizer) Summarize(ctx context.Context, in SummarizeRequest) (SummarizeOutput, error) {
	var raw any
	if len(in.Turns) != 0 {
		_ = json.Unmarshal(in.Turns, &raw)
	}
	turns := summaryTurns(raw)
	if len(turns) == 0 {
		return SummarizeOutput{}, errors.New("args.turns required")
	}
	provider := s.ports.Provider()
	if provider == nil {
		return SummarizeOutput{}, errors.New("daemon has no provider configured")
	}
	var model string
	if len(in.Model) != 0 {
		var v any
		_ = json.Unmarshal(in.Model, &v)
		str, ok := v.(string)
		if !ok {
			return SummarizeOutput{}, errors.New("args.model must be a string")
		}
		model = str
	}
	if model == "" {
		model = s.ports.Model()
	}
	transcript := convo.TranscriptIntent(turns)
	if len(transcript) > summaryInputCap {
		transcript = transcript[len(transcript)-summaryInputCap:]
	}
	resp, err := provider.Complete(ctx, llm.CompletionRequest{
		Model:     model,
		TaskType:  "summarize",
		MaxTokens: summaryMaxTokens,
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Content: "Condense this conversation into a compact briefing for the assistant's working memory. " +
				"Preserve facts, names, numbers, decisions, preferences, and open questions; drop pleasantries. " +
				"Output only the briefing.\n\n" + transcript,
		}},
	})
	if err != nil {
		return SummarizeOutput{}, err
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return SummarizeOutput{}, errors.New("summarizer returned an empty summary")
	}
	return SummarizeOutput{Summary: summary, Turns: len(turns)}, nil
}

// summaryTurns parses the request's turns array ([{role,text}], decoded as
// []any of map[string]any) into convo turns, skipping malformed/blank entries —
// the same tolerant shape as the webui's run `history` field. A prior summary
// rides in as a "system" turn, which TranscriptIntent hoists to the front.
func summaryTurns(raw any) []convo.Turn {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	turns := make([]convo.Turn, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		text, _ := m["text"].(string)
		if strings.TrimSpace(role) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		turns = append(turns, convo.Turn{Role: role, Text: text})
	}
	return turns
}

// SummaryOperations declares the audited, operator-only summarize call on its
// Web UI route. It is live so a disconnected client cancels the provider call;
// it never sends an event frame.
func SummaryOperations(provider func(context.Context) *Summarizer) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("summarizer provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[SummarizeOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewStreamingOperation(opapi.Spec{Name: "chat_summarize", Stream: opapi.StreamLive, OutputSchema: out, EmissionSchema: json.RawMessage(event.WireSchema), Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"turns":{},"model":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/chat/summarize"}}, func(ctx context.Context, in SummarizeRequest, _ func(event.Event) error) (SummarizeOutput, error) {
		return provider(ctx).Summarize(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
