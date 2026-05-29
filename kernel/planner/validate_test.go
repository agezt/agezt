// SPDX-License-Identifier: MIT

package planner_test

import (
	"strings"
	"testing"

	"github.com/ersinkoc/agezt/kernel/planner"
)

// TestValidateJSON_AcceptsGoodPlan ensures the public API matches
// the daemon's pre-execution validator on a well-formed plan.
func TestValidateJSON_AcceptsGoodPlan(t *testing.T) {
	raw := []byte(`{
		"name": "test",
		"max_parallel": 2,
		"nodes": [
			{"id": "a", "kind": "loop", "intent": "x"},
			{"id": "b", "kind": "loop", "intent": "y", "deps": ["a"]}
		]
	}`)
	p, err := planner.ValidateJSON(raw)
	if err != nil {
		t.Fatalf("ValidateJSON: %v", err)
	}
	if len(p.Nodes) != 2 {
		t.Errorf("Nodes len = %d, want 2", len(p.Nodes))
	}
	if p.Name != "test" {
		t.Errorf("Name = %q", p.Name)
	}
}

// TestValidateJSON_RejectsCycle covers the load-bearing safety
// check operators care about — a cyclic plan would deadlock the
// scheduler, so the validator must catch it before submission.
func TestValidateJSON_RejectsCycle(t *testing.T) {
	raw := []byte(`{
		"nodes": [
			{"id": "a", "kind": "loop", "intent": "x", "deps": ["b"]},
			{"id": "b", "kind": "loop", "intent": "y", "deps": ["a"]}
		]
	}`)
	_, err := planner.ValidateJSON(raw)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err should mention cycle: %v", err)
	}
}

// TestValidate_AcceptsProgrammaticPlan covers the in-memory
// variant — useful for callers building Plans without going through
// JSON. Same rules apply.
func TestValidate_AcceptsProgrammaticPlan(t *testing.T) {
	p := planner.Plan{
		Name: "in-mem",
		Nodes: []planner.Node{
			{ID: "x", Kind: "loop", Intent: "do x"},
			{ID: "y", Kind: "gate", Description: "approve?", Deps: []string{"x"}},
		},
	}
	if err := planner.Validate(p); err != nil {
		t.Errorf("Validate on good plan: %v", err)
	}

	// And the rejection path — gate without description.
	bad := planner.Plan{Nodes: []planner.Node{{ID: "x", Kind: "gate"}}}
	if err := planner.Validate(bad); err == nil {
		t.Error("expected error for empty gate description")
	}
}
