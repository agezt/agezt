// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestChatSummarize_ReturnsBriefing exercises the round-trip: the Chat view
// sends the turns to fold, the server runs one bounded provider call, and the
// briefing comes back with the folded-turn count (M925).
func TestChatSummarize_ReturnsBriefing(t *testing.T) {
	prov := mock.New(mock.FinalText("Owner is planning a trip to Oslo; budget 2000 EUR; prefers trains."))
	_, _, c, _ := startPair(t, prov)

	res, err := c.Call(context.Background(), controlplane.CmdChatSummarize, map[string]any{
		"turns": []any{
			map[string]any{"role": "user", "text": "I want to plan a trip to Oslo."},
			map[string]any{"role": "assistant", "text": "Great — what's your budget?"},
			map[string]any{"role": "user", "text": "2000 EUR, and I prefer trains."},
		},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	summary, _ := res["summary"].(string)
	if !strings.Contains(summary, "Oslo") {
		t.Errorf("summary = %q, want the mock briefing", summary)
	}
	count, _ := res["turns"].(float64)
	if int(count) != 3 {
		t.Errorf("turns = %v, want 3", count)
	}
}

// TestChatSummarize_FoldsPriorSummaryAsSystem proves re-folding works: a prior
// briefing rides in as a "system" turn and reaches the provider hoisted to the
// front of the transcript (convo.TranscriptIntent's preamble rule).
func TestChatSummarize_FoldsPriorSummaryAsSystem(t *testing.T) {
	prov := mock.New(mock.FinalText("updated briefing"))
	var reqs []agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { reqs = append(reqs, r) }
	_, _, c, _ := startPair(t, prov)

	if _, err := c.Call(context.Background(), controlplane.CmdChatSummarize, map[string]any{
		"turns": []any{
			map[string]any{"role": "system", "text": "Previous summary: trip to Oslo, 2000 EUR."},
			map[string]any{"role": "user", "text": "Actually make it Bergen."},
			map[string]any{"role": "assistant", "text": "Bergen it is."},
		},
	}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(reqs))
	}
	got := reqs[0].Messages[0].Content
	prior := strings.Index(got, "Previous summary: trip to Oslo")
	dialogue := strings.Index(got, "User: Actually make it Bergen.")
	if prior < 0 || dialogue < 0 || prior > dialogue {
		t.Errorf("prior summary not hoisted before the dialogue:\n%s", got)
	}
	if reqs[0].TaskType != "summarize" {
		t.Errorf("TaskType = %q, want summarize (per-task routing)", reqs[0].TaskType)
	}
}

func TestChatSummarize_RejectsMissingTurns(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New())
	_, err := c.Call(context.Background(), controlplane.CmdChatSummarize, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "turns required") {
		t.Errorf("err = %v, want turns-required", err)
	}
}
