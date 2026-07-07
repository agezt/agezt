// SPDX-License-Identifier: MIT

package planner_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/planner"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// capturingMock wraps mock.New so the test can verify what the
// planner actually sent to the provider. The standard mock provider
// discards request details; we need the request body to confirm the
// refine prompt carries both the original plan and the feedback.
type capturingMock struct {
	inner       agent.Provider
	lastRequest agent.CompletionRequest
}

func (c *capturingMock) Name() string { return "capturing-mock" }
func (c *capturingMock) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	c.lastRequest = req
	return c.inner.Complete(ctx, req)
}

func TestRefine_HappyPath(t *testing.T) {
	original := planner.Plan{
		Name:        "draft brief",
		MaxParallel: 1,
		Nodes: []planner.Node{
			{ID: "research", Kind: "loop", Intent: "research X"},
			{ID: "draft", Kind: "loop", Intent: "draft based on research", Deps: []string{"research"}},
		},
	}
	revised := `{"name":"draft brief v2","max_parallel":2,"nodes":[
		{"id":"research","kind":"loop","intent":"research X with more depth","deps":[]},
		{"id":"sources","kind":"loop","intent":"add citations","deps":["research"]},
		{"id":"draft","kind":"loop","intent":"draft based on research","deps":["research","sources"]}
	]}`
	cap := &capturingMock{inner: mock.New(mock.FinalText(fencedJSON(revised)))}

	raw, p, err := planner.Refine(context.Background(), planner.Config{Provider: cap}, original,
		"add a sources/citations node between research and draft")
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if !strings.Contains(raw, "sources") {
		t.Errorf("revised plan should contain new node: %s", raw)
	}
	if len(p.Nodes) != 3 {
		t.Errorf("Nodes len = %d, want 3", len(p.Nodes))
	}

	// The LLM must have received the original plan JSON AND the
	// feedback as part of the user message; otherwise the refine
	// path is no better than calling Generate from scratch.
	if len(cap.lastRequest.Messages) != 1 {
		t.Fatalf("expected single user message, got %d", len(cap.lastRequest.Messages))
	}
	userMsg := cap.lastRequest.Messages[0].Content
	if !strings.Contains(userMsg, "CURRENT PLAN") {
		t.Errorf("user message missing CURRENT PLAN section: %s", userMsg)
	}
	if !strings.Contains(userMsg, `"id": "research"`) {
		t.Errorf("original plan not embedded in user message: %s", userMsg)
	}
	if !strings.Contains(userMsg, "add a sources/citations node") {
		t.Errorf("feedback not embedded in user message: %s", userMsg)
	}
	if cap.lastRequest.TaskType != planner.TaskType {
		t.Errorf("TaskType = %q, want %q (so per-task-type routing works for refine too)",
			cap.lastRequest.TaskType, planner.TaskType)
	}
}

func TestRefine_RejectsEmptyFeedback(t *testing.T) {
	original := planner.Plan{
		Nodes: []planner.Node{{ID: "x", Kind: "loop", Intent: "x"}},
	}
	prov := mock.New(mock.FinalText(""))
	_, _, err := planner.Refine(context.Background(), planner.Config{Provider: prov}, original, "")
	if err == nil {
		t.Fatal("expected error for empty feedback (refine without instructions is meaningless)")
	}
}

func TestRefine_RejectsEmptyOriginal(t *testing.T) {
	prov := mock.New(mock.FinalText(""))
	_, _, err := planner.Refine(context.Background(), planner.Config{Provider: prov}, planner.Plan{}, "do something")
	if err == nil {
		t.Fatal("expected error for empty original plan")
	}
}

// TestRefine_RevisedPlanValidatedSameAsInitial ensures the refined
// plan goes through the same validators Generate runs — a cycle
// from the LLM must be rejected even on a refinement call.
func TestRefine_RevisedPlanValidatedSameAsInitial(t *testing.T) {
	original := planner.Plan{
		Nodes: []planner.Node{{ID: "a", Kind: "loop", Intent: "a"}},
	}
	// Cycle: a depends on b, b depends on a.
	cyclic := `{"nodes":[
		{"id":"a","kind":"loop","intent":"a","deps":["b"]},
		{"id":"b","kind":"loop","intent":"b","deps":["a"]}
	]}`
	prov := mock.New(mock.FinalText(fencedJSON(cyclic)))
	_, _, err := planner.Refine(context.Background(), planner.Config{Provider: prov}, original, "introduce circular dep")
	if err == nil {
		t.Fatal("expected validation error on cyclic refined plan")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle: %v", err)
	}
}

func TestRefine_RejectsMissingProvider(t *testing.T) {
	original := planner.Plan{Nodes: []planner.Node{{ID: "x", Kind: "loop", Intent: "x"}}}
	_, _, err := planner.Refine(context.Background(), planner.Config{}, original, "do it")
	if err == nil || !strings.Contains(err.Error(), "Provider required") {
		t.Errorf("err = %v, want Provider-required", err)
	}
}

func TestRefine_LLMErrorPropagates(t *testing.T) {
	original := planner.Plan{Nodes: []planner.Node{{ID: "x", Kind: "loop", Intent: "x"}}}
	// Empty mock provider returns ErrExhausted on Complete.
	prov := mock.New()
	_, _, err := planner.Refine(context.Background(), planner.Config{Provider: prov}, original, "revise")
	if err == nil {
		t.Fatal("expected error from failing LLM call")
	}
}
