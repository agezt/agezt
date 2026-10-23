// SPDX-License-Identifier: MIT

package overseer

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// --- findMisconfigured / findDegraded / findRoutingPressure ---

func TestFindMisconfigured_FoundAndNotFound(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{
			{Slug: "alpha", Issues: []string{"bad override"}},
			{Slug: "beta", Issues: []string{"missing model"}},
		},
	}
	if findMisconfigured(rep, "alpha") == nil {
		t.Fatal("findMisconfigured(alpha) should find it")
	}
	if findMisconfigured(rep, "gamma") != nil {
		t.Fatal("findMisconfigured(gamma) should be nil")
	}
	if findMisconfigured(kernelruntime.ReaperReport{}, "alpha") != nil {
		t.Fatal("findMisconfigured(empty) should be nil")
	}
}

func TestFindDegraded_FoundAndNotFound(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		DegradedAgents: []kernelruntime.DegradedAgent{
			{Slug: "alpha", Failures: 5, Window: 10, LastReason: "timeout"},
		},
	}
	if findDegraded(rep, "alpha") == nil {
		t.Fatal("findDegraded(alpha) should find it")
	}
	if findDegraded(rep, "beta") != nil {
		t.Fatal("findDegraded(beta) should be nil")
	}
}

func TestFindRoutingPressure_FoundAndNotFound(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "alpha", Count: 3, Threshold: 3, TaskType: "code", LastFailedModel: "gpt-5"},
		},
	}
	if findRoutingPressure(rep, "alpha") == nil {
		t.Fatal("findRoutingPressure(alpha) should find it")
	}
	if findRoutingPressure(rep, "beta") != nil {
		t.Fatal("findRoutingPressure(beta) should be nil")
	}
	if findRoutingPressure(kernelruntime.ReaperReport{}, "alpha") != nil {
		t.Fatal("findRoutingPressure(empty) should be nil")
	}
}

// --- repairTaskType ---

func TestRepairTaskType_UsesProfileTaskType(t *testing.T) {
	tt := repairTaskType(agentapi.Profile{Slug: "a", TaskType: "research"}, kernelruntime.ReaperReport{})
	if tt != "research" {
		t.Fatalf("repairTaskType = %q, want research", tt)
	}
}

func TestRepairTaskType_FallsBackToRoutingPressure(t *testing.T) {
	tt := repairTaskType(agentapi.Profile{Slug: "b"}, kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "b", TaskType: "code"},
		},
	})
	if tt != "code" {
		t.Fatalf("repairTaskType (fallback) = %q, want code", tt)
	}
}

func TestRepairTaskType_EmptyWhenNoSignal(t *testing.T) {
	tt := repairTaskType(agentapi.Profile{Slug: "c"}, kernelruntime.ReaperReport{})
	if tt != "" {
		t.Fatalf("repairTaskType with no signal = %q, want empty", tt)
	}
}

// --- managedSubAgentRepairHint ---

func TestManagedSubAgentRepairHint_WithParent(t *testing.T) {
	hint := managedSubAgentRepairHint(agentapi.Profile{Slug: "sub", ParentAgent: "parent"})
	if hint == "" {
		t.Fatal("hint should not be empty")
	}
	if !strings.Contains(hint, "parent") {
		t.Fatalf("hint = %q, want it to name the parent agent", hint)
	}
}

func TestManagedSubAgentRepairHint_WithOwnerOnly(t *testing.T) {
	hint := managedSubAgentRepairHint(agentapi.Profile{Slug: "sub", OwnerAgent: "owner"})
	if hint == "" {
		t.Fatal("hint should not be empty")
	}
	if !strings.Contains(hint, "owner") {
		t.Fatalf("hint = %q, want it to name the owner agent", hint)
	}
}

func TestManagedSubAgentRepairHint_NoManager(t *testing.T) {
	hint := managedSubAgentRepairHint(agentapi.Profile{Slug: "sub"})
	if hint == "" {
		t.Fatal("hint should not be empty even without manager")
	}
	if !strings.Contains(hint, "managed sub-agent") {
		t.Fatalf("hint = %q, want it to say the agent is managed", hint)
	}
}

// --- sanitizeTaskModelChain ---

func TestSanitizeTaskModelChain_RemovesEmpty(t *testing.T) {
	got := sanitizeTaskModelChain([]string{"model-1", "", "  ", "model-2"})
	if len(got) != 2 || got[0] != "model-1" || got[1] != "model-2" {
		t.Fatalf("sanitizeTaskModelChain = %#v", got)
	}
}

func TestSanitizeTaskModelChain_AllEmpty(t *testing.T) {
	got := sanitizeTaskModelChain([]string{"", "  "})
	if len(got) != 0 {
		t.Fatalf("sanitizeTaskModelChain(all empty) = %#v, want empty", got)
	}
}

// --- encodeTaskModelChains ---

func TestEncodeTaskModelChains_SortsByTaskType(t *testing.T) {
	got := encodeTaskModelChains(map[string][]string{
		"b": {"model-b"},
		"a": {"model-a"},
	})
	want := "a=model-a;b=model-b"
	if got != want {
		t.Fatalf("encodeTaskModelChains = %q, want %q", got, want)
	}
}

func TestEncodeTaskModelChains_Empty(t *testing.T) {
	if encodeTaskModelChains(nil) != "" {
		t.Error("encodeTaskModelChains(nil) should be empty")
	}
	if encodeTaskModelChains(map[string][]string{}) != "" {
		t.Error("encodeTaskModelChains(empty) should be empty")
	}
}

func TestEncodeTaskModelChains_SkipsEmptyValues(t *testing.T) {
	got := encodeTaskModelChains(map[string][]string{
		"":    {"model-a"},
		"b":   {},
		"c":   {"model-c"},
		"   ": {"model-d"},
	})
	if got != "c=model-c" {
		t.Fatalf("encodeTaskModelChains = %q, want %q", got, "c=model-c")
	}
}

// --- applyProfilePatchField ---

func TestApplyProfilePatchField_OnlyWhenProvided(t *testing.T) {
	var dst string
	applyProfilePatchField(map[string]bool{"key": true}, "key", &dst, "val")
	if dst != "val" {
		t.Fatalf("dst = %q, want val", dst)
	}
	applyProfilePatchField(map[string]bool{}, "key", &dst, "other")
	if dst != "val" {
		t.Fatalf("dst should NOT change when key not provided, got %q", dst)
	}
}

// --- clip ---

func TestClip_ShortAndLong(t *testing.T) {
	if clip("short", 10) != "short" {
		t.Error("clip should return the full string when shorter than max")
	}
	if clip("hello world", 5) != "hell…" {
		t.Errorf("clip = %q, want %q", clip("hello world", 5), "hell…")
	}
	got := clip("ab", 1)
	if got != "a" {
		t.Errorf("clip(ab, 1) = %q, want %q", got, "a")
	}
}

func TestClip_EdgeCases(t *testing.T) {
	if clip("x", 0) != "" {
		t.Errorf("clip('x', 0) = %q, want '' (empty from s[:0])", clip("x", 0))
	}
	if clip("xy", 1) != "x" {
		t.Errorf("clip('xy', 1) = %q, want 'x'", clip("xy", 1))
	}
}
