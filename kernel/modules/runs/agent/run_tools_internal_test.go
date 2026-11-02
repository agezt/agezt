// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

// Internal tests for the loop's tool phases (refactor Phase 3.2). These reach
// the state machine directly — the whole point of hoisting it out of Run's body
// is that the invariants below can now be asserted without driving a full run.

// stubTool is a minimal Tool whose output and error are fixtures.
type stubTool struct {
	def    ToolDef
	output string
	err    error
	calls  int
}

func (s *stubTool) Definition() ToolDef { return s.def }
func (s *stubTool) Invoke(_ context.Context, _ json.RawMessage) (Result, error) {
	s.calls++
	if s.err != nil {
		return Result{}, s.err
	}
	return Result{Output: s.output}, nil
}

func objSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)
}

// nopPublish satisfies runState's publish hook without a bus.
func nopPublish(_ event.Kind, _ string, _ any) (*event.Event, error) { return nil, nil }

func testState(cfg LoopConfig) *runState {
	return newRunState(cfg, nopPublish)
}

// The prompt-injection causal window: a directive-like untrusted observation
// gates actions for directiveWindow iterations after it arrived, then decays.
// Before the extraction this arithmetic was inline in the middle of a 300-line
// gating block and could only be exercised through a whole run.
func TestDirectiveActive_CausalWindowDecays(t *testing.T) {
	s := testState(LoopConfig{DirectiveTaintWindow: 2})

	// No observation yet ⇒ never active, at any iteration.
	if s.directiveActive(0) || s.directiveActive(99) {
		t.Fatal("directiveActive with no observation must be false")
	}

	// An observation at iteration 5 gates iterations 5..7 (window = 2).
	s.directiveObsIter = 5
	for _, iter := range []int{5, 6, 7} {
		if !s.directiveActive(iter) {
			t.Errorf("iter %d: want gated (within window of obs at 5)", iter)
		}
	}
	if s.directiveActive(8) {
		t.Error("iter 8: want decayed (outside a window of 2 from obs at 5)")
	}
}

func TestResolveDirectiveWindow_DefaultsWhenUnset(t *testing.T) {
	if got := resolveDirectiveWindow(LoopConfig{}); got != DefaultDirectiveTaintWindow {
		t.Errorf("unset window = %d, want the default %d", got, DefaultDirectiveTaintWindow)
	}
	if got := resolveDirectiveWindow(LoopConfig{DirectiveTaintWindow: -3}); got != DefaultDirectiveTaintWindow {
		t.Errorf("negative window = %d, want the default %d", got, DefaultDirectiveTaintWindow)
	}
	if got := resolveDirectiveWindow(LoopConfig{DirectiveTaintWindow: 7}); got != 7 {
		t.Errorf("explicit window = %d, want 7", got)
	}
}

// finalizeToolJobs must APPEND to the conversation it is handed and return the
// extension — it must never keep its own copy. A second copy inside runState
// would silently drop the turns the loop appends elsewhere (steering, the
// model's own message, the auto-continue nudge), which is exactly the shape of
// bug this signature exists to make impossible.
func TestFinalizeToolJobs_AppendsToTheCallersConversation(t *testing.T) {
	s := testState(LoopConfig{})
	prior := []Message{
		{Role: RoleUser, Content: "do it"},
		{Role: RoleAssistant, Content: "calling a tool"},
	}
	jobs := []*toolJob{
		{tc: ToolCall{ID: "c1", Name: "alpha"}, result: Result{Output: "A"}},
		{tc: ToolCall{ID: "c2", Name: "beta"}, result: Result{Output: "B"}},
	}

	out, err := s.finalizeToolJobs(context.Background(), jobs, 0, prior)
	if err != nil {
		t.Fatalf("finalizeToolJobs: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("conversation length = %d, want 4 (2 prior + 2 tool results)", len(out))
	}
	// The prior turns survive, in order.
	if out[0].Content != "do it" || out[1].Content != "calling a tool" {
		t.Errorf("prior turns lost or reordered: %+v", out[:2])
	}
	// Tool results follow in the ORIGINAL call order, keyed to their call ids.
	if out[2].Role != RoleTool || out[2].ToolCallID != "c1" || out[2].Content != "A" {
		t.Errorf("first tool turn = %+v", out[2])
	}
	if out[3].Role != RoleTool || out[3].ToolCallID != "c2" || out[3].Content != "B" {
		t.Errorf("second tool turn = %+v", out[3])
	}
}

// A tool that returns an error becomes an error Result the model can react to —
// the run continues. Only run-level terminals (panic, ctx end) return an error.
func TestFinalizeToolJobs_ToolErrorIsFedBackNotFatal(t *testing.T) {
	s := testState(LoopConfig{})
	jobs := []*toolJob{{
		tc:        ToolCall{ID: "c1", Name: "alpha"},
		tool:      &stubTool{def: ToolDef{Name: "alpha", InputSchema: objSchema()}},
		invokeErr: errors.New("boom"),
	}}
	out, err := s.finalizeToolJobs(context.Background(), jobs, 0, nil)
	if err != nil {
		t.Fatalf("a tool error must not fail the run: %v", err)
	}
	if len(out) != 1 || out[0].Content != "boom" {
		t.Fatalf("tool error not fed back to the model: %+v", out)
	}
}

func TestFinalizeToolJobs_PanicIsRunTerminal(t *testing.T) {
	s := testState(LoopConfig{})
	jobs := []*toolJob{{
		tc:        ToolCall{ID: "c1", Name: "alpha"},
		tool:      &stubTool{def: ToolDef{Name: "alpha", InputSchema: objSchema()}},
		panicked:  true,
		invokeErr: ErrPanic,
	}}
	if _, err := s.finalizeToolJobs(context.Background(), jobs, 0, nil); !errors.Is(err, ErrPanic) {
		t.Fatalf("panic must be run-terminal, got %v", err)
	}
}

// The M605 denial ladder: a hard-denied tool is dropped from the offer set
// immediately, a softly-refused one after maxToolDenials refusals.
func TestOfferedTools_DropsRepeatedlyRefusedTools(t *testing.T) {
	s := testState(LoopConfig{})
	all := []ToolDef{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}

	// Nothing refused yet: the caller's slice comes back untouched.
	if got := s.offeredTools(all); len(got) != 3 {
		t.Fatalf("with no denials, offered = %d tools, want 3", len(got))
	}

	// One soft refusal is not enough to drop a tool (the model may retry with
	// different input, which policy might allow).
	s.toolDenials["beta"] = 1
	if got := s.offeredTools(all); len(got) != 3 {
		t.Errorf("after 1 soft denial, offered = %d tools, want 3", len(got))
	}

	// Reaching the threshold drops it; a hard-deny reaches it in one step.
	s.toolDenials["beta"] = maxToolDenials
	s.toolDenials["gamma"] = maxToolDenials
	got := s.offeredTools(all)
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("offered = %+v, want only alpha", got)
	}
}

// gateToolCalls refuses an unknown tool and a schema-invalid call WITHOUT
// executing anything, and synthesizes the message the model will see.
func TestGateToolCalls_RefusalsShortCircuitExecution(t *testing.T) {
	known := &stubTool{
		def:    ToolDef{Name: "alpha", InputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"number"}},"required":["n"]}`)},
		output: "ok",
	}
	s := testState(LoopConfig{Tools: map[string]Tool{"alpha": known}})

	jobs, err := s.gateToolCalls(context.Background(), []ToolCall{
		{ID: "c1", Name: "nope", Input: json.RawMessage(`{}`)},
		{ID: "c2", Name: "alpha", Input: json.RawMessage(`{}`)}, // missing required "n"
		{ID: "c3", Name: "alpha", Input: json.RawMessage(`{"n":1}`)},
	}, 0)
	if err != nil {
		t.Fatalf("gateToolCalls: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3 (one per call, refusals included)", len(jobs))
	}
	if jobs[0].tool != nil || !jobs[0].result.IsError {
		t.Errorf("unknown tool must be refused with an error result: %+v", jobs[0])
	}
	if jobs[1].tool != nil || !jobs[1].result.IsError {
		t.Errorf("schema-invalid call must be refused: %+v", jobs[1])
	}
	if jobs[2].tool == nil {
		t.Error("the valid call must be gated through to execution")
	}

	// Only the gated-through job runs.
	executeToolJobs(context.Background(), s.cfg, jobs)
	if known.calls != 1 {
		t.Errorf("tool invoked %d times, want exactly 1 (refusals must not execute)", known.calls)
	}
}

// The M116 loop guard refuses an identical (tool,input) past the cap and tells
// the model why, instead of letting it burn iterations on a call whose result
// cannot change.
func TestGateToolCalls_LoopGuardRefusesIdenticalRepeat(t *testing.T) {
	tool := &stubTool{def: ToolDef{Name: "alpha", InputSchema: objSchema()}, output: "ok"}
	s := testState(LoopConfig{
		Tools:                 map[string]Tool{"alpha": tool},
		MaxIdenticalToolCalls: 2,
	})
	call := ToolCall{ID: "c", Name: "alpha", Input: json.RawMessage(`{"same":true}`)}

	for i := 1; i <= 2; i++ {
		jobs, err := s.gateToolCalls(context.Background(), []ToolCall{call}, i)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if jobs[0].tool == nil {
			t.Fatalf("call %d must be allowed (cap is 2)", i)
		}
	}
	jobs, err := s.gateToolCalls(context.Background(), []ToolCall{call}, 3)
	if err != nil {
		t.Fatalf("third call: %v", err)
	}
	if jobs[0].tool != nil {
		t.Fatal("the third identical call must be refused by the loop guard")
	}
	if !jobs[0].result.IsError || !strings.Contains(jobs[0].result.Output, "loop guard") {
		t.Errorf("loop-guard refusal must explain itself: %q", jobs[0].result.Output)
	}
}
