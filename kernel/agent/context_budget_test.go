// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRun_ContextBudgetCompacts: a run with several large tool outputs and a low
// ContextBudget journals a context.compacted event — the loop trimmed its own
// context (SPEC-10 §3) by eliding the oldest tool outputs.
func TestRun_ContextBudgetCompacts(t *testing.T) {
	b, j := newTestBus(t)
	big := strings.Repeat("Z", 4000)

	// Three tool rounds, then a final answer — enough history that the oldest
	// tool outputs fall outside the protected tail.
	prov := mock.New(
		mock.ToolUse("c1", "dump", map[string]any{}),
		mock.ToolUse("c2", "dump", map[string]any{}),
		mock.ToolUse("c3", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:           prov,
		Tools:              map[string]agent.Tool{"dump": bigTool{out: big}},
		Bus:                b,
		Actor:              "agent-1",
		CorrelationID:      "corr-ctx",
		ContextBudget:      5000, // far below 3×4000 of tool output
		ContextProtectLast: 2,
	}, "dump repeatedly"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got struct {
		Elided    int `json:"elided"`
		Reclaimed int `json:"reclaimed_chars"`
		Before    int `json:"context_chars_before"`
		After     int `json:"context_chars_after"`
		Budget    int `json:"budget"`
	}
	count := 0
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindContextCompacted {
			_ = json.Unmarshal(e.Payload, &got)
			count++
		}
		return nil
	})
	if count == 0 {
		t.Fatal("expected at least one context.compacted event")
	}
	if got.Elided == 0 || got.Reclaimed == 0 {
		t.Errorf("compaction payload looks empty: %+v", got)
	}
	if got.After > got.Before {
		t.Errorf("after %d should be < before %d", got.After, got.Before)
	}
	if got.Budget != 5000 {
		t.Errorf("budget echoed %d, want 5000", got.Budget)
	}
}

// TestRun_NoContextBudgetNoCompaction: without a budget there is no compaction,
// even with large outputs — the historical full-history behaviour is unchanged.
func TestRun_NoContextBudgetNoCompaction(t *testing.T) {
	b, j := newTestBus(t)
	big := strings.Repeat("Z", 4000)
	prov := mock.New(
		mock.ToolUse("c1", "dump", map[string]any{}),
		mock.ToolUse("c2", "dump", map[string]any{}),
		mock.FinalText("done"),
	)
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Tools:         map[string]agent.Tool{"dump": bigTool{out: big}},
		Bus:           b,
		Actor:         "agent-1",
		CorrelationID: "corr-nobudget",
	}, "dump"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	n := 0
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindContextCompacted {
			n++
		}
		return nil
	})
	if n != 0 {
		t.Errorf("no budget → no context.compacted, got %d", n)
	}
}
