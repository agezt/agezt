// SPDX-License-Identifier: MIT

package delegation_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/delegation"
)

func TestDepthFromCtx_Default(t *testing.T) {
	if d := delegation.DepthFromCtx(context.Background()); d != 0 {
		t.Fatalf("DepthFromCtx = %d, want 0", d)
	}
}

func TestWithDepth_RoundTrip(t *testing.T) {
	ctx := delegation.WithDepth(context.Background(), 3)
	if d := delegation.DepthFromCtx(ctx); d != 3 {
		t.Fatalf("DepthFromCtx = %d, want 3", d)
	}
}

func TestSpawnLink_EmptyPayload(t *testing.T) {
	child, parent := delegation.SpawnLink(nil)
	if child != "" || parent != "" {
		t.Fatalf("SpawnLink(nil) = %q, %q, want empty", child, parent)
	}
}

func TestSpawnLink_ValidPayload(t *testing.T) {
	payload := []byte(`{"child_correlation":"c1","parent":"p1"}`)
	child, parent := delegation.SpawnLink(payload)
	if child != "c1" || parent != "p1" {
		t.Fatalf("SpawnLink = %q, %q, want c1, p1", child, parent)
	}
}

func TestBudgetCostMicrocents_EmptyPayload(t *testing.T) {
	if c := delegation.BudgetCostMicrocents(nil); c != 0 {
		t.Fatalf("BudgetCostMicrocents(nil) = %d, want 0", c)
	}
}

func TestBudgetCostMicrocents_ValidPayload(t *testing.T) {
	payload := []byte(`{"cost_microcents":5000}`)
	if c := delegation.BudgetCostMicrocents(payload); c != 5000 {
		t.Fatalf("BudgetCostMicrocents = %d, want 5000", c)
	}
}

func TestKeyedModelChain_ExplicitOverride(t *testing.T) {
	model, rest := delegation.KeyedModelChain("gpt-4", []string{"claude"}, func(string) bool { return true }, "default")
	if model != "gpt-4" {
		t.Fatalf("model = %q, want gpt-4", model)
	}
	if len(rest) != 1 || rest[0] != "claude" {
		t.Fatalf("rest = %v, want [claude]", rest)
	}
}

func TestKeyedModelChain_NoOverrideFallback(t *testing.T) {
	model, rest := delegation.KeyedModelChain("", nil, func(string) bool { return true }, "default")
	if model != "default" {
		t.Fatalf("model = %q, want default", model)
	}
	if rest != nil {
		t.Fatalf("rest = %v, want nil", rest)
	}
}

func TestValidateSpawnTask_Empty(t *testing.T) {
	if err := delegation.ValidateSpawnTask(""); err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestValidateSpawnTask_Valid(t *testing.T) {
	if err := delegation.ValidateSpawnTask("do something useful"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendUniqueStrings_Dedup(t *testing.T) {
	got := delegation.AppendUniqueStrings([]string{"a", "b"}, "b", "c")
	if len(got) != 3 {
		t.Fatalf("got %v, want [a b c]", got)
	}
}

func TestFormatDuration(t *testing.T) {
	if s := delegation.FormatDuration(500_000_000); s != "500ms" {
		t.Fatalf("FormatDuration = %q, want 500ms", s)
	}
	if s := delegation.FormatDuration(1_500_000_000); s != "1.5s" {
		t.Fatalf("FormatDuration = %q, want 1.5s", s)
	}
	if s := delegation.FormatDuration(150_000_000_000); s != "2.5m" {
		t.Fatalf("FormatDuration = %q, want 2.5m", s)
	}
}
