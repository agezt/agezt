// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPlanGenerate_StreamsValidPlan exercises the round-trip:
// CLI sends an intent, server calls planner.Generate with the
// kernel's provider, planner round-trips through the mock LLM,
// the validated JSON comes back. Verifies wiring without
// re-testing the planner's validation rules (those have unit
// coverage in kernel/planner/planner_test.go).
func TestPlanGenerate_StreamsValidPlan(t *testing.T) {
	plan := `{
		"name": "test plan",
		"nodes": [
			{"id": "step1", "kind": "loop", "intent": "do a thing"},
			{"id": "step2", "kind": "loop", "intent": "do the next thing", "deps": ["step1"]}
		]
	}`
	prov := mock.New(mock.FinalText("```json\n" + plan + "\n```"))
	_, _, c, _ := startPair(t, prov)

	res, err := c.Call(context.Background(), controlplane.CmdPlanGenerate, map[string]any{
		"intent": "do two things in order",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := res["plan_json"].(string)
	if !strings.Contains(got, `"id": "step1"`) {
		t.Errorf("plan_json missing step1: %s", got)
	}
	count, _ := res["node_count"].(float64)
	if int(count) != 2 {
		t.Errorf("node_count = %v, want 2", count)
	}
}

func TestPlanGenerate_RejectsMissingIntent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New())
	_, err := c.Call(context.Background(), controlplane.CmdPlanGenerate, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "intent required") {
		t.Errorf("err = %v, want intent-required", err)
	}
}

func TestPlanGenerate_SurfacesPlannerValidation(t *testing.T) {
	// Mock LLM returns a plan with a dangling dep — planner.Generate
	// rejects it during validation, and the rejection must surface
	// to the client as a control-plane error.
	bad := `{"nodes":[{"id":"a","kind":"loop","intent":"x","deps":["ghost"]}]}`
	prov := mock.New(mock.FinalText("```json\n" + bad + "\n```"))
	_, _, c, _ := startPair(t, prov)

	_, err := c.Call(context.Background(), controlplane.CmdPlanGenerate, map[string]any{
		"intent": "whatever",
	})
	if err == nil {
		t.Fatal("expected error from validation failure")
	}
	if !strings.Contains(err.Error(), `dep "ghost"`) {
		t.Errorf("err = %v, want dep-does-not-exist message", err)
	}
}
