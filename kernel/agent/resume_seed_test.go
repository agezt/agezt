// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestResume_SeedsPriorMessagesWithoutDuplicatingIntent proves the continuity
// mechanism (M1002): a resumed run seeded with a prior conversation continues
// that conversation instead of starting fresh, and does NOT prepend a second
// copy of the original intent turn (the snapshot already opens with it).
func TestResume_SeedsPriorMessagesWithoutDuplicatingIntent(t *testing.T) {
	b, _ := newTestBus(t)
	prov := mock.New(mock.FinalText("continued"))
	var got []agent.Message
	var mu sync.Mutex
	prov.OnRequest = func(req agent.CompletionRequest) {
		mu.Lock()
		if got == nil { // capture the FIRST request only
			got = append([]agent.Message(nil), req.Messages...)
		}
		mu.Unlock()
	}

	prior := []agent.Message{
		{Role: agent.RoleUser, Content: "do the thing"},
		{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "c1", Name: "noop", Input: []byte(`{}`)}}},
		{Role: agent.RoleTool, ToolCallID: "c1", Content: "step done"},
	}

	answer, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "agent-resume",
		CorrelationID: "corr-resume",
		Model:         "test",
		MaxIter:       5,
		// The interrupted run had progressed to iteration 3.
		StartIter:     3,
		PriorMessages: prior,
	}, "do the thing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if answer != "continued" {
		t.Fatalf("answer = %q, want continued", answer)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("first request carried %d messages, want the 3 seeded ones: %+v", len(got), got)
	}
	// Exactly one user turn bearing the intent — not a duplicated one.
	intentTurns := 0
	for _, m := range got {
		if m.Role == agent.RoleUser && m.Content == "do the thing" {
			intentTurns++
		}
	}
	if intentTurns != 1 {
		t.Fatalf("intent turn appears %d times, want exactly 1 (no duplication)", intentTurns)
	}
	// The prior tool result must survive so the model has its earlier work.
	if got[2].Role != agent.RoleTool || got[2].Content != "step done" {
		t.Fatalf("prior tool result not preserved: %+v", got[2])
	}
}

// TestResume_CheckpointFiresAtSettledBoundary proves the snapshot hook is called
// at the top-of-loop boundary with a consistent conversation — the prior
// iteration's tool results are already folded in, so a snapshot never captures a
// dangling tool_call. This is what the daemon persists each iteration (M1002).
func TestResume_CheckpointFiresAtSettledBoundary(t *testing.T) {
	b, _ := newTestBus(t)
	// First response asks for the noop tool (→ a second iteration), second ends.
	prov := mock.New(testToolUse("c1", "noop", map[string]any{}), mock.FinalText("done"))

	type snap struct {
		iter int
		msgs []agent.Message
	}
	var snaps []snap
	var mu sync.Mutex

	answer, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "agent-ckpt",
		CorrelationID: "corr-ckpt",
		Model:         "test",
		MaxIter:       5,
		Tools:         map[string]agent.Tool{"noop": steerNoopTool{}},
		Checkpoint: func(iter int, messages []agent.Message) {
			mu.Lock()
			snaps = append(snaps, snap{iter: iter, msgs: append([]agent.Message(nil), messages...)})
			mu.Unlock()
		},
	}, "do the thing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if answer != "done" {
		t.Fatalf("answer = %q, want done", answer)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(snaps) < 2 {
		t.Fatalf("checkpoint fired %d times, want >= 2 (one per iteration)", len(snaps))
	}
	// Iterations are monotonic.
	for i := 1; i < len(snaps); i++ {
		if snaps[i].iter <= snaps[i-1].iter {
			t.Fatalf("checkpoint iters not monotonic: %d then %d", snaps[i-1].iter, snaps[i].iter)
		}
	}
	// Every snapshot ends on a settled turn — never an assistant turn still
	// carrying unanswered tool calls (which would be a dangling tool_call).
	for _, s := range snaps {
		last := s.msgs[len(s.msgs)-1]
		if last.Role == agent.RoleAssistant && len(last.ToolCalls) > 0 {
			t.Fatalf("snapshot at iter %d ends on unanswered tool calls: %+v", s.iter, last)
		}
	}
	// The second snapshot must include the tool result from iteration one.
	sawToolResult := false
	for _, m := range snaps[1].msgs {
		if m.Role == agent.RoleTool && m.ToolCallID == "c1" {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Fatalf("second snapshot missing the settled tool result: %+v", snaps[1].msgs)
	}
}
