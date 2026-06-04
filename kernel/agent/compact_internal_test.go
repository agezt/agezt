// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

func bigToolMsg(id string, n int) Message {
	return Message{Role: RoleTool, Content: strings.Repeat("x", n), ToolCallID: id}
}

// convo builds a realistic-ish transcript: user, then K (assistant, tool) pairs.
func convo(toolBytes int, pairs int) []Message {
	msgs := []Message{{Role: RoleUser, Content: "do the thing"}}
	for i := 0; i < pairs; i++ {
		msgs = append(msgs, Message{Role: RoleAssistant, Content: "calling tool"})
		msgs = append(msgs, bigToolMsg("call-"+strings.Repeat("a", i+1), toolBytes))
	}
	return msgs
}

func TestCompactMessages_DisabledAndUnderBudget(t *testing.T) {
	msgs := convo(100, 3)
	if out, el, rc := compactMessages("sys", msgs, 0, 0); el != 0 || rc != 0 || len(out) != len(msgs) {
		t.Errorf("budget 0 must be a no-op, got elided=%d reclaimed=%d", el, rc)
	}
	// Budget far above the actual size → no elision.
	if out, el, rc := compactMessages("sys", msgs, 1_000_000, 2); el != 0 || rc != 0 || len(out) != len(msgs) {
		t.Errorf("under-budget must not elide, got elided=%d", el)
	}
}

func TestCompactMessages_ElidesOldestToolOutputsFirst(t *testing.T) {
	// 5 tool outputs of 1000 chars each; budget forces eliding the oldest few.
	msgs := convo(1000, 5)
	before, _ := contextSize("sys", msgs)
	budget := before - 2500 // must drop ~3 outputs' worth
	out, elided, reclaimed := compactMessages("sys", msgs, budget, 2)

	if elided == 0 {
		t.Fatal("expected some elision")
	}
	after, _ := contextSize("sys", out)
	if after > budget {
		t.Errorf("after=%d still over budget=%d (elided=%d)", after, budget, budget)
	}
	if reclaimed != before-after {
		t.Errorf("reclaimed=%d but before-after=%d", reclaimed, before-after)
	}
	// Structure preserved: same count + roles + tool-call ids intact.
	if len(out) != len(msgs) {
		t.Fatalf("message count changed: %d -> %d", len(msgs), len(out))
	}
	for i := range out {
		if out[i].Role != msgs[i].Role || out[i].ToolCallID != msgs[i].ToolCallID {
			t.Errorf("message %d role/tool-call-id changed", i)
		}
	}
	// The OLDEST tool output (index 2) must be elided; eliding goes front-to-back.
	if !strings.HasPrefix(out[2].Content, elidedStubPrefix) {
		t.Errorf("oldest tool output should be elided first, got %q", out[2].Content[:20])
	}
	// The protected last 2 messages (the final assistant + tool) are untouched.
	last := len(out) - 1
	if strings.HasPrefix(out[last].Content, elidedStubPrefix) {
		t.Error("most-recent tool output must be protected, but it was elided")
	}
}

func TestCompactMessages_NeverElidesNonTool(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: strings.Repeat("u", 5000)},
		{Role: RoleAssistant, Content: strings.Repeat("a", 5000)},
		bigToolMsg("call-1", 5000),
		{Role: RoleAssistant, Content: "final"},
		bigToolMsg("call-2", 50),
	}
	out, _, _ := compactMessages("sys", msgs, 100, 0)
	if strings.HasPrefix(out[0].Content, elidedStubPrefix) || strings.HasPrefix(out[1].Content, elidedStubPrefix) {
		t.Error("user/assistant messages must never be elided")
	}
}

func TestAutoContextBudgetChars(t *testing.T) {
	cases := map[int]int{
		0:      0,      // unknown window → off
		-5:     0,      // garbage → off
		8192:   16384,  // 8K tokens → 8192*4*0.5
		200000: 400000, // 200K tokens
	}
	for tokens, want := range cases {
		if got := AutoContextBudgetChars(tokens); got != want {
			t.Errorf("AutoContextBudgetChars(%d) = %d, want %d", tokens, got, want)
		}
	}
}

func TestCompactMessages_Idempotent(t *testing.T) {
	msgs := convo(1000, 5)
	before, _ := contextSize("sys", msgs)
	out1, el1, _ := compactMessages("sys", msgs, before-2500, 1)
	out2, el2, rc2 := compactMessages("sys", out1, before-2500, 1)
	if el2 != 0 || rc2 != 0 {
		t.Errorf("re-compacting an already-compacted transcript must be a no-op, got elided=%d", el2)
	}
	if el1 == 0 {
		t.Error("first pass should have elided")
	}
	_ = out2
}
