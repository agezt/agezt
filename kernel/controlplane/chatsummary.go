// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/convo"
)

// chatSummaryMaxTokens bounds the briefing call (M925). Generous on purpose:
// a reasoning model (deepseek-v4-pro, o-series) spends output tokens on its
// chain of thought BEFORE the briefing — a tight cap (512 was tried) gets
// entirely eaten by reasoning and yields empty content. The briefing itself
// stays a compact digest; the prompt asks for that.
const chatSummaryMaxTokens = 2048

// chatSummaryInputCap bounds how much transcript is fed to the summarizer.
// When the folded turns exceed it, the oldest text is dropped first — the tail
// of the fold is closest to the live conversation, so it matters most.
const chatSummaryInputCap = 24 << 10

// handleChatSummarize condenses older chat turns into one compact briefing
// (M925). The Chat view calls this when a thread outgrows the history window,
// then rides the briefing as a leading system turn on later runs instead of
// silently dropping the oldest turns. One bounded provider call, routed as
// TaskType "summarize" (per-task model routing applies); no tools, no loop.
func (s *Server) handleChatSummarize(ctx context.Context, conn net.Conn, req Request) {
	turns := summaryTurns(req.Args["turns"])
	if len(turns) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.turns required"})
		return
	}
	provider := s.k.Provider()
	if provider == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "daemon has no provider configured"})
		return
	}
	model, _ := req.Args["model"].(string)
	if model == "" {
		model = s.k.Model()
	}
	transcript := convo.TranscriptIntent(turns)
	if len(transcript) > chatSummaryInputCap {
		transcript = transcript[len(transcript)-chatSummaryInputCap:]
	}
	resp, err := provider.Complete(ctx, agent.CompletionRequest{
		Model:     model,
		TaskType:  "summarize",
		MaxTokens: chatSummaryMaxTokens,
		Messages: []agent.Message{{
			Role: agent.RoleUser,
			Content: "Condense this conversation into a compact briefing for the assistant's working memory. " +
				"Preserve facts, names, numbers, decisions, preferences, and open questions; drop pleasantries. " +
				"Output only the briefing.\n\n" + transcript,
		}},
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "summarizer returned an empty summary"})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"summary": summary,
		"turns":   len(turns),
	}})
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
