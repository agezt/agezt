// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPrompts_GetSet round-trips the prompt library: empty by default, set persists
// (and round-trips through get), and blank/invalid entries are dropped.
func TestPrompts_GetSet(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Empty by default.
	res, err := c.Call(context.Background(), controlplane.CmdPromptsGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ps, _ := res["prompts"].([]any); len(ps) != 0 {
		t.Fatalf("fresh daemon should have no prompts, got %v", res["prompts"])
	}

	// Set three, one of which is blank (dropped).
	in := []any{
		map[string]any{"title": "Standup", "text": "Draft my standup from recent runs."},
		map[string]any{"title": "  ", "text": "no title — dropped"},
		map[string]any{"title": "Review", "text": "Review the working diff for bugs."},
	}
	setRes, err := c.Call(context.Background(), controlplane.CmdPromptsSet, map[string]any{"prompts": in})
	if err != nil {
		t.Fatal(err)
	}
	if cnt, _ := setRes["count"].(float64); cnt != 2 {
		t.Errorf("count = %v, want 2 (blank dropped)", setRes["count"])
	}

	// Get returns the two valid prompts in order.
	res, _ = c.Call(context.Background(), controlplane.CmdPromptsGet, nil)
	ps, _ := res["prompts"].([]any)
	if len(ps) != 2 {
		t.Fatalf("expected 2 prompts, got %d: %v", len(ps), ps)
	}
	first, _ := ps[0].(map[string]any)
	if first["title"] != "Standup" || first["text"] != "Draft my standup from recent runs." {
		t.Errorf("first prompt = %v", first)
	}
}

func TestPrompts_Set_RequiresArray(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdPromptsSet, nil); err == nil {
		t.Error("prompts_set without args.prompts should error")
	}
	if _, err := c.Call(context.Background(), controlplane.CmdPromptsSet, map[string]any{"prompts": "nope"}); err == nil {
		t.Error("prompts_set with a non-array should error")
	}
}
