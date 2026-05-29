// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/kernel/governor"
	"github.com/ersinkoc/agezt/plugins/providers/mock"
)

// stubProvider returns a fixed Usage so cost accounting is
// deterministic — each Complete call "costs" the same.
type stubProvider struct {
	name     string
	inToks   int
	outToks  int
	model    string
	callsSeen int
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	s.callsSeen++
	m := req.Model
	if m == "" {
		m = s.model
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: "ok"},
		StopReason: agent.StopEndTurn,
		Usage:      agent.Usage{Model: m, InputTokens: s.inToks, OutputTokens: s.outToks},
	}, nil
}

func newGovernorWithTaskBudgets(t *testing.T, taskBudgets map[string]int64, dailyCeiling int64, model string) (*governor.Governor, *stubProvider) {
	t.Helper()
	// Use a mock from the existing package as a sanity-anchor for
	// what a registered provider looks like.
	_ = mock.New(mock.FinalText("ok"))
	reg := governor.NewRegistry()
	stub := &stubProvider{name: "stub", model: model, inToks: 1_000_000, outToks: 1_000_000}
	if err := reg.Register(&governor.ProviderInfo{
		Name:     stub.name,
		Provider: stub,
		AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	g, err := governor.New(governor.Config{
		Registry:               reg,
		DailyCeilingMicrocents: dailyCeiling,
		TaskBudgets:            taskBudgets,
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	return g, stub
}

// TestTaskBudget_BlocksAfterCap verifies that a task type with a
// configured per-day cap stops admitting calls once spent crosses
// the cap. Other task types remain unaffected.
func TestTaskBudget_BlocksAfterCap(t *testing.T) {
	// 1M input + 1M output tokens at claude-opus-4-7 pricing is
	// well above any sensible cap; set the cap so the first call
	// blows through it.
	taskCap := int64(200_000) // 0.020 USD in microcents — tiny
	g, stub := newGovernorWithTaskBudgets(t,
		map[string]int64{"plan": taskCap},
		0, // global ceiling unlimited
		"claude-opus-4-7",
	)

	// First call against "plan" — succeeds (pre-check passes; spend
	// recorded post-call).
	_, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("first plan call: %v", err)
	}
	if stub.callsSeen != 1 {
		t.Errorf("provider called %d times, want 1", stub.callsSeen)
	}
	spent := g.SpentByTaskMicrocents("plan")
	if spent < taskCap {
		t.Errorf("post-call spent=%d, want >= cap=%d to exercise the gate", spent, taskCap)
	}

	// Second call against "plan" — rejected by the per-task budget
	// pre-check; provider must NOT be invoked.
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("second plan call: expected ErrTaskBudgetExceeded")
	}
	if !errors.Is(err, governor.ErrTaskBudgetExceeded) {
		t.Errorf("err = %v, want ErrTaskBudgetExceeded", err)
	}
	// ErrTaskBudgetExceeded wraps ErrBudgetExceeded so existing
	// chain-walk code keeps treating it as terminal.
	if !errors.Is(err, governor.ErrBudgetExceeded) {
		t.Errorf("ErrTaskBudgetExceeded should wrap ErrBudgetExceeded so shouldFallback honors it")
	}
	if stub.callsSeen != 1 {
		t.Errorf("provider was invoked despite cap (callsSeen=%d, want still 1)", stub.callsSeen)
	}

	// Third call against a DIFFERENT task type — should succeed
	// (per-task caps are scoped to their type).
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7",
		TaskType: "code",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Errorf("call with un-capped task type 'code' rejected: %v", err)
	}
	if stub.callsSeen != 2 {
		t.Errorf("after un-capped task call, callsSeen=%d, want 2", stub.callsSeen)
	}
}

// TestTaskBudget_NoTaskTypeBypassesCheck — a request with no
// TaskType must not be gated by per-task caps (otherwise un-tagged
// callers couldn't run at all once any cap was hit).
func TestTaskBudget_NoTaskTypeBypassesCheck(t *testing.T) {
	g, stub := newGovernorWithTaskBudgets(t,
		map[string]int64{"plan": 1}, // microscopic cap, will fire instantly
		0,
		"claude-opus-4-7",
	)

	// Spend the cap on "plan".
	_, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7",
		TaskType: "plan",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("seed plan call: %v", err)
	}
	// Untagged call — should NOT be blocked.
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-opus-4-7",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Errorf("untagged call rejected by per-task cap: %v", err)
	}
	if stub.callsSeen != 2 {
		t.Errorf("callsSeen=%d, want 2 (seed + untagged)", stub.callsSeen)
	}
}

// TestTaskBudget_NotConfiguredNeverBlocks — when TaskBudgets is
// nil, no per-task gate fires regardless of TaskType.
func TestTaskBudget_NotConfiguredNeverBlocks(t *testing.T) {
	g, stub := newGovernorWithTaskBudgets(t, nil, 0, "claude-opus-4-7")
	for range 3 {
		_, err := g.Complete(context.Background(), agent.CompletionRequest{
			Model:    "claude-opus-4-7",
			TaskType: "plan",
			Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
		})
		if err != nil {
			t.Errorf("with no TaskBudgets, call rejected: %v", err)
		}
	}
	if stub.callsSeen != 3 {
		t.Errorf("callsSeen=%d, want 3", stub.callsSeen)
	}
}

// TestSnapshot_PopulatesPerTaskEvenAtZeroSpend ensures Snapshot
// includes a row for every configured cap (even if no spend has
// happened yet) — operators want to confirm their config applied,
// not just see entries that have already burned budget.
func TestSnapshot_PopulatesPerTaskEvenAtZeroSpend(t *testing.T) {
	g, _ := newGovernorWithTaskBudgets(t,
		map[string]int64{"plan": 100_000, "code": 200_000, "salience": 50_000},
		0,
		"claude-opus-4-7",
	)
	snap := g.Snapshot()
	if len(snap.PerTask) != 3 {
		t.Fatalf("PerTask rows=%d, want 3", len(snap.PerTask))
	}
	seen := map[string]int64{}
	for _, row := range snap.PerTask {
		seen[row.TaskType] = row.CapMicrocents
		if row.SpentMicrocents != 0 {
			t.Errorf("row %s: spent=%d expected 0 (no calls made)", row.TaskType, row.SpentMicrocents)
		}
	}
	for k, v := range map[string]int64{"plan": 100_000, "code": 200_000, "salience": 50_000} {
		if seen[k] != v {
			t.Errorf("missing/wrong cap for %s: got %d, want %d", k, seen[k], v)
		}
	}
}

// TestParseTaskBudgetsEnv covers the env-var parser, including the
// strict rejection of zero/negative/empty values.
func TestParseTaskBudgetsEnv(t *testing.T) {
	good, err := governor.ParseTaskBudgetsEnv(" plan = 100000 ;  code=500000")
	if err != nil {
		t.Fatalf("parse good: %v", err)
	}
	if good["plan"] != 100000 || good["code"] != 500000 {
		t.Errorf("parsed = %v", good)
	}

	for _, bad := range []string{
		"plan",          // no '='
		"=100",          // empty key
		"plan=",         // empty value
		"plan=0",        // zero
		"plan=-50",      // negative
		"plan=notanum",  // non-numeric
	} {
		if _, err := governor.ParseTaskBudgetsEnv(bad); err == nil {
			t.Errorf("ParseTaskBudgetsEnv(%q) should error", bad)
		}
	}

	if out, err := governor.ParseTaskBudgetsEnv(""); err != nil || out != nil {
		t.Errorf("empty spec: got (%v, %v), want (nil, nil)", out, err)
	}
}
